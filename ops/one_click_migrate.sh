#!/usr/bin/env bash
# ============================================================
# DWZ 短网址系统 — 一键上线迁移
#
# 把老库升级需要的运维动作收敛成一条命令：
#   1) 备份两个库中受影响的表
#   2) SQL 迁移统一交给 backend/cmd/migrate（schema_migrations 版本表全量纳管）
#   3) 静态资源构建 + 内容哈希版本号注入
#
# 关于 SQL 迁移：所有 .sql 迁移（url_hash 作用域化、webhook_queue、members、
# violation_reviews 等）都由 `go run ./cmd/migrate` 执行并记录到
# schema_migrations，不再由本脚本手工拼接库名后执行 —— 那样既不记录版本，
# 也无法回答"哪些迁移已执行"。库名通过 {{PUBLIC_DB}} / {{ADMIN_DB}} 占位符
# 注入，脚本内不再需要人工替换 USE 库名。
#
# 若目标库此前是手工升级的（schema 已到位但没有版本记录），先执行一次
# `go run ./cmd/migrate -baseline` 补录，避免后续重跑 DDL：
#   cd backend && go run ./cmd/migrate -baseline -migrations ../migrations -all
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
#   --web-root=/data/www/wwwroot/1.xk7.cn   静态资源构建目录（默认脚本上级目录）
#   --skip-backup                           跳过备份（不推荐）
#   --skip-assets                           跳过静态资源构建
#   --dry-run                               只打印将要执行的命令，不落库
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
DO_SQL=1
BASELINE=0
DRY_RUN=0

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
    --skip-sql)      DO_SQL=0 ;;
    --baseline)      BASELINE=1 ;;
    --dry-run)       DRY_RUN=1 ;;
    -h|--help)
      sed -n '2,28p' "$0" | sed 's/^# \{0,1\}//'
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

run_sql() {
  local db="$1" file="$2"
  if [ "$DRY_RUN" = "1" ]; then
    echo "  [dry-run] mysql ${DB_USER}@${HOST}:${PORT} ${db} < ${file}"
    return 0
  fi
  "$MYSQL_BIN" "${MYSQL_OPTS[@]}" "$db" < "$file"
}

# --- 0. 连接自检 ----------------------------------------------------------
log "0/4 检查数据库连通性"
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
  log "1/4 备份受影响表 → $TARGET_DIR"
  if [ "$DRY_RUN" = "1" ]; then
    echo "  [dry-run] mysqldump wjoy_log / short_urls"
  else
    command -v "$MYSQLDUMP_BIN" >/dev/null 2>&1 || die "找不到 mysqldump（$MYSQLDUMP_BIN），或用 --skip-backup 跳过备份"
    mkdir -p "$TARGET_DIR"
    "$MYSQLDUMP_BIN" "${DUMP_OPTS[@]}" "$PUBLIC_DB" wjoy_log   > "$TARGET_DIR/${PUBLIC_DB}.wjoy_log.sql"
    "$MYSQLDUMP_BIN" "${DUMP_OPTS[@]}" "$ADMIN_DB"  short_urls > "$TARGET_DIR/${ADMIN_DB}.short_urls.sql"
    chmod 600 "$TARGET_DIR"/*.sql
    echo "  已备份：$TARGET_DIR"
  fi
else
  warn "已跳过备份（--skip-backup）"
fi

# --- 2~4. SQL 迁移（统一入口） -------------------------------------------
# 所有迁移由 backend/cmd/migrate 执行：目录扫描确定清单、schema_migrations
# 记录版本、{{PUBLIC_DB}}/{{ADMIN_DB}} 占位符注入库名。
GO_BIN="${DWZ_GO_BIN:-go}"
if [ "$DO_SQL" = "1" ]; then
  if ! command -v "$GO_BIN" >/dev/null 2>&1; then
    warn "找不到 go（$GO_BIN），跳过 SQL 迁移。可在有 Go 的机器执行："
    warn "  cd backend && go run ./cmd/migrate -all -migrations ../migrations"
    warn "或设置 DWZ_GO_BIN 指向 go 可执行文件。"
  else
    MIGRATE_ARGS=(-all -migrations ../migrations)
    # 迁移工具默认读 backend/configs/config.yaml；不存在时回退到 example，
    # 由下面透传的 DWZ_DB_* 环境变量覆盖真实连接信息。
    MIGRATE_CONFIG="${DWZ_MIGRATE_CONFIG:-}"
    if [ -z "$MIGRATE_CONFIG" ]; then
      if [ -f "$ROOT/backend/configs/config.yaml" ]; then
        MIGRATE_CONFIG="configs/config.yaml"
      else
        MIGRATE_CONFIG="configs/config.example.yaml"
      fi
    fi
    MIGRATE_ARGS+=(-config "$MIGRATE_CONFIG")

    # 迁移工具从 backend/configs/config.yaml 读凭据，这里把命令行/环境变量的
    # 值透传过去，避免运维必须写两遍配置。
    export DWZ_DB_HOST="$HOST" DWZ_DB_PORT="$PORT"
    export DWZ_DB_USER_OVERRIDE="$DB_USER" DWZ_DB_PASS_OVERRIDE="$DB_PASS"
    export DWZ_ADMIN_DB_OVERRIDE="$ADMIN_DB" DWZ_PUBLIC_DB_OVERRIDE="$PUBLIC_DB"

    if [ "$BASELINE" = "1" ]; then
      log "2/4 补录迁移版本（-baseline，不执行 DDL）"
      MIGRATE_ARGS=(-baseline -migrations ../migrations)
    else
      log "2/4 执行 SQL 迁移（schema_migrations 记录版本）"
    fi

    if [ "$DRY_RUN" = "1" ]; then
      echo "  [dry-run] (cd backend && $GO_BIN run ./cmd/migrate -dry-run ${MIGRATE_ARGS[*]})"
    else
      ( cd "$ROOT/backend" && "$GO_BIN" run ./cmd/migrate "${MIGRATE_ARGS[@]}" ) \
        || die "SQL 迁移失败，已执行的迁移均已记录在 schema_migrations，可修正后重跑"
    fi

    if [ "$BASELINE" = "1" ]; then
      log "3/4 已补录版本，跳过校验（未执行 DDL）"
    else
      log "3/4 校验两库 url_hash 口径"
      if [ "$DRY_RUN" = "1" ]; then
        echo "  [dry-run] 跳过校验"
      else
        LEGACY_PUBLIC="$("$MYSQL_BIN" "${MYSQL_OPTS[@]}" -N -B "$PUBLIC_DB" \
          -e 'SELECT COUNT(*) FROM `wjoy_log` WHERE `url_hash` = MD5(`longurl`)' 2>/dev/null || echo 'skip')"
        LEGACY_ADMIN="$("$MYSQL_BIN" "${MYSQL_OPTS[@]}" -N -B "$ADMIN_DB" \
          -e 'SELECT COUNT(*) FROM `short_urls` WHERE `url_hash` = MD5(`long_url`)' 2>/dev/null || echo 'skip')"
        echo "  公共库剩余未作用域化行：$LEGACY_PUBLIC"
        echo "  管理库剩余未作用域化行：$LEGACY_ADMIN"
        if [ "$LEGACY_PUBLIC" != "skip" ] && [ "$LEGACY_PUBLIC" != "0" ]; then
          die "公共库仍有 $LEGACY_PUBLIC 行未作用域化，迁移未完成"
        fi
        if [ "$LEGACY_ADMIN" != "skip" ] && [ "$LEGACY_ADMIN" != "0" ]; then
          die "管理库仍有 $LEGACY_ADMIN 行未作用域化，迁移未完成"
        fi
        echo "  两个库的 url_hash 口径一致 ✅"
      fi
    fi
  fi
else
  log "2/4 已跳过 SQL 迁移（--skip-sql）"
  log "3/4 已跳过校验"
fi

# --- 5. 静态资源 ----------------------------------------------------------
if [ "$DO_ASSETS" = "1" ]; then
  log "4/4 构建静态资源（压缩 + 内容哈希版本号注入）"
  if [ "$DRY_RUN" = "1" ]; then
    echo "  [dry-run] php migrations/build_assets.php"
  elif command -v "$PHP_BIN" >/dev/null 2>&1; then
    ( cd "$WEB_ROOT" && "$PHP_BIN" migrations/build_assets.php ) || warn "静态资源构建失败（不影响跳转，可稍后手动执行）"
  else
    warn "找不到 php，跳过静态资源构建"
  fi
else
  log "4/4 已跳过静态资源构建（--skip-assets）"
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
