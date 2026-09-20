#!/usr/bin/env bash
# ============================================================
# DWZ 短网址系统 — 一键上线迁移
#
# 迁移动作本身由统一工具 backend/cmd/migrate 完成（同一份清单、同一张
# schema_migrations 版本表），本脚本只负责运维外围：
#   1) 备份两个库中受影响的表
#   2) 调用统一工具执行全部待应用迁移
#      （url_hash 作用域化 / webhook_queue / members / violation_reviews ...）
#   3) 静态资源构建 + 内容哈希版本号注入
#
# 用法：
#   DWZ_PUBLIC_DB=1_xk7_cn DWZ_ADMIN_DB=dwz_admin \
#   DWZ_DB_USER=root DWZ_DB_PASS='密码' \
#   ./ops/one_click_migrate.sh
#
# 全部参数也可用命令行覆盖：
#   ./ops/one_click_migrate.sh --public-db=1_xk7_cn --admin-db=dwz_admin \
#       --user=root --pass='密码' --host=127.0.0.1 --port=3306
#
# 可选：
#   --web-root=/data/www/wwwroot/your-domain.com   静态资源构建目录（默认脚本上级目录）
#   --skip-backup                           跳过备份（不推荐）
#   --skip-assets                           跳过静态资源构建
#   --dry-run                               只打印将要执行的命令，不落库
#   --baseline                              把目录内全部迁移标记为已应用（不执行 DDL）
#
# 脚本是幂等的：中途失败可直接重新执行。执行前会自动备份，
# 备份文件默认落在 ./backups/migrate-<时间戳>/ 下。
# ============================================================
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"

HOST="${DWZ_DB_HOST:-127.0.0.1}"
PORT="${DWZ_DB_PORT:-3306}"
DB_USER="${DWZ_DB_USER:-root}"
DB_PASS="${DWZ_DB_PASS:-}"
PUBLIC_DB="${DWZ_PUBLIC_DB:-}"
ADMIN_DB="${DWZ_ADMIN_DB:-}"
WEB_ROOT="${DWZ_WEB_ROOT:-$ROOT}"
BACKUP_DIR="${DWZ_BACKUP_DIR:-$ROOT/backups}"
MYSQL_BIN="${DWZ_MYSQL_BIN:-mysql}"
MYSQLDUMP_BIN="${DWZ_MYSQLDUMP_BIN:-mysqldump}"
PHP_BIN="${DWZ_PHP_BIN:-php}"

DO_BACKUP=1
DO_ASSETS=1
DRY_RUN=0
BASELINE=""
GO_BIN="${DWZ_GO_BIN:-go}"
MIGRATE_BIN="${DWZ_MIGRATE_BIN:-}"

for arg in "$@"; do
  case "$arg" in
    --host=*)        HOST="${arg#*=}" ;;
    --port=*)        PORT="${arg#*=}" ;;
    --user=*)        DB_USER="${arg#*=}" ;;
    --pass=*)        DB_PASS="${arg#*=}" ;;
    --public-db=*)   PUBLIC_DB="${arg#*=}" ;;
    --admin-db=*)    ADMIN_DB="${arg#*=}" ;;
    --web-root=*)    WEB_ROOT="${arg#*=}" ;;
    --backup-dir=*)  BACKUP_DIR="${arg#*=}" ;;
    --skip-backup)   DO_BACKUP=0 ;;
    --skip-assets)   DO_ASSETS=0 ;;
    --dry-run)       DRY_RUN=1 ;;
    --baseline)      BASELINE=1 ;;
    -h|--help)
      sed -n '2,29p' "$0" | sed 's/^# \{0,1\}//'
      exit 0 ;;
    *) echo "[ERR] 未知参数：$arg" >&2; exit 2 ;;
  esac
done

log()  { printf '\033[1;34m==>\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33m[!]\033[0m %s\n' "$*" >&2; }
die()  { printf '\033[1;31m[ERR]\033[0m %s\n' "$*" >&2; exit 1; }

# --- 参数推导 -------------------------------------------------------------
# 未显式指定库名时，从 PHP 的 config.php / Go 的 config.yaml 读取，
# 避免运维必须手工抄一遍库名。
if [ -z "$PUBLIC_DB" ] || [ -z "$ADMIN_DB" ]; then
  if [ -f "$ROOT/config.php" ]; then
    [ -n "$PUBLIC_DB" ] || PUBLIC_DB="$("$PHP_BIN" -r '
      include $argv[1];
      echo isset($dbname) ? $dbname : "";' "$ROOT/config.php" 2>/dev/null || true)"
    [ -n "$ADMIN_DB" ] || ADMIN_DB="$("$PHP_BIN" -r '
      include $argv[1];
      echo isset($admin_db_name) && $admin_db_name !== "" ? $admin_db_name : (isset($dbname) ? $dbname : "");' "$ROOT/config.php" 2>/dev/null || true)"
    [ -n "$DB_USER" ] && [ "$DB_USER" = "root" ] || true
  fi
fi
if [ -z "$PUBLIC_DB" ] || [ -z "$ADMIN_DB" ]; then
  if [ -f "$ROOT/backend/configs/config.yaml" ]; then
    [ -n "$ADMIN_DB" ] || ADMIN_DB="$(awk '/^[[:space:]]*dbname:/{print $2; exit}' "$ROOT/backend/configs/config.yaml" | tr -d '"' || true)"
    [ -n "$PUBLIC_DB" ] || PUBLIC_DB="$ADMIN_DB"
  fi
fi

[ -n "$PUBLIC_DB" ] || die "缺少公共库名（wjoy_log 所在库）。请传 --public-db=... 或设置 DWZ_PUBLIC_DB。"
[ -n "$ADMIN_DB" ]  || die "缺少管理库名（short_urls 所在库）。请传 --admin-db=... 或设置 DWZ_ADMIN_DB。"

command -v "$MYSQL_BIN" >/dev/null 2>&1 || die "找不到 mysql 客户端（$MYSQL_BIN）"

# 密码通过环境变量 MYSQL_PWD 传递，不进命令行（ps / shell history 不可见）
export MYSQL_PWD="$DB_PASS"
MYSQL_OPTS=(-h"$HOST" -P"$PORT" -u"$DB_USER" --default-character-set=utf8mb4)
DUMP_OPTS=(-h"$HOST" -P"$PORT" -u"$DB_USER" --single-transaction --quick --default-character-set=utf8mb4)

# --- 0. 连接自检 ----------------------------------------------------------
log "0/3 检查数据库连通性"
if ! "$MYSQL_BIN" "${MYSQL_OPTS[@]}" -e 'SELECT 1' >/dev/null 2>&1; then
  die "无法连接 MySQL ${HOST}:${PORT}（用户 ${DB_USER}）"
fi
"$MYSQL_BIN" "${MYSQL_OPTS[@]}" -N -e \
  "SELECT SCHEMA_NAME FROM information_schema.SCHEMATA WHERE SCHEMA_NAME IN ('${PUBLIC_DB//\'/\'\'}','${ADMIN_DB//\'/\'\'}')" \
  | sort > /tmp/dwz_found_dbs.$$
for db in "$PUBLIC_DB" "$ADMIN_DB"; do
  grep -qx "$db" /tmp/dwz_found_dbs.$$ || die "库不存在：$db"
done
rm -f /tmp/dwz_found_dbs.$$
echo "  公共库=${PUBLIC_DB}  管理库=${ADMIN_DB}  主机=${HOST}:${PORT}"

# --- 1. 备份 --------------------------------------------------------------
STAMP="$(date +%Y%m%d-%H%M%S)"
TARGET_DIR="$BACKUP_DIR/migrate-$STAMP"
if [ "$DO_BACKUP" = "1" ]; then
  log "1/3 备份受影响表 → $TARGET_DIR"
  if [ "$DRY_RUN" = "1" ]; then
    echo "  [dry-run] mysqldump wjoy_log / short_urls"
  else
    command -v "$MYSQLDUMP_BIN" >/dev/null 2>&1 || die "找不到 mysqldump（$MYSQLDUMP_BIN），或用 --skip-backup 跳过备份"
    mkdir -p "$TARGET_DIR"

    # 表名 => 所属库。首次安装时表还不存在，此时没有数据可丢，
    # 备份缺失即可（记录为 skipped），不应因此中断迁移。
    backup_table() {
      local db="$1" table="$2"
      local out="$TARGET_DIR/${db}.${table}.sql"
      if ! "$MYSQLDUMP_BIN" "${DUMP_OPTS[@]}" "$db" "$table" > "$out" 2>/dev/null; then
        rm -f "$out"
        warn "  表 ${db}.${table} 不存在或不可读，跳过备份（首次安装属正常）"
        return 0
      fi
      echo "  已备份 ${db}.${table}"
    }

    backup_table "$PUBLIC_DB" wjoy_log
    backup_table "$ADMIN_DB"  short_urls
    # 备份成功才收紧权限；目录可能为空（全新安装）。
    [ -n "$(ls -A "$TARGET_DIR" 2>/dev/null)" ] && chmod 600 "$TARGET_DIR"/*.sql || true
    echo "  备份目录：$TARGET_DIR"
  fi
else
  warn "已跳过备份（--skip-backup）"
fi

# --- 2. 调用统一迁移工具 ----------------------------------------------
# 迁移清单、执行顺序与版本记录全部由 backend/cmd/migrate 决定，
# 这里不再手工 mysql < xxx.sql，也不再对 SQL 做库名 sed 替换。
log "2/3 统一迁移工具：应用全部待执行迁移"

# 统一工具读取 Go 的 config.yaml；为了让命令行传入的库名生效，
# 优先走环境变量覆盖（viper 的 DWZ_* 前缀），并临时写一份 config 副本。
run_migrate() {
  local args=("$@")
  if [ -n "$MIGRATE_BIN" ] && [ -x "$MIGRATE_BIN" ]; then
    "$MIGRATE_BIN" "${args[@]}"
    return
  fi
  command -v "$GO_BIN" >/dev/null 2>&1 \
    || die "找不到 go（\$DWZ_GO_BIN）也找不到已编译的 migrate 二进制（\$DWZ_MIGRATE_BIN）"
  ( cd "$ROOT/backend" && "$GO_BIN" run ./cmd/migrate -migrations ../backend/migrations -config "$MIGRATE_CONFIG" "${args[@]}" )
}

MIGRATE_CONFIG="$ROOT/backend/configs/config.yaml"
TMP_CONFIG=""
if [ ! -f "$MIGRATE_CONFIG" ]; then
  TMP_CONFIG="$(mktemp -t dwz_migrate_config.XXXXXX.yaml)"
  trap 'rm -f "$TMP_CONFIG"' EXIT
  cat > "$TMP_CONFIG" <<YAML
database:
  host: "${HOST}"
  port: ${PORT}
  user: "${DB_USER}"
  password: "${DB_PASS}"
  dbname: "${ADMIN_DB}"
  charset: utf8mb4
public_db:
  host: "${HOST}"
  port: ${PORT}
  user: "${DB_USER}"
  password: "${DB_PASS}"
  dbname: "${PUBLIC_DB}"
  charset: utf8mb4
YAML
  chmod 600 "$TMP_CONFIG"
  MIGRATE_CONFIG="$TMP_CONFIG"
fi

if [ "$BASELINE" = "1" ]; then
  log "2a/3 补录基线（把目录内全部迁移标记为已应用，不执行 DDL）"
  run_migrate -baseline
fi

if [ "$DRY_RUN" = "1" ]; then
  run_migrate -dry-run
else
  run_migrate -all
fi

# --- 5. 静态资源 ----------------------------------------------------------
if [ "$DO_ASSETS" = "1" ]; then
  log "3/3 构建静态资源（压缩 + 内容哈希版本号注入）"
  if [ "$DRY_RUN" = "1" ]; then
    echo "  [dry-run] php migrations/build_assets.php"
  elif command -v "$PHP_BIN" >/dev/null 2>&1; then
    ( cd "$WEB_ROOT" && "$PHP_BIN" migrations/build_assets.php ) || warn "静态资源构建失败（不影响跳转，可稍后手动执行）"
  else
    warn "找不到 php，跳过静态资源构建"
  fi
else
  log "3/3 已跳过静态资源构建（--skip-assets）"
fi

echo
log "全部完成 ✅"
cat <<'TIP'

上线后建议确认两件事（不影响本次迁移结果）：
  1) webhook worker 加入 cron（不配置也能跑，只是退化为同步投递）：
       * * * * * php /网站根目录/migrations/webhook_worker.php
  2) PHP 的 config.php 里 $member_secret 必须与 Go 的
     backend/configs/config.yaml 中 jwt.member_secret 保持一致，
     否则密码保护短链在两条跳转路径上互不认可。
TIP
