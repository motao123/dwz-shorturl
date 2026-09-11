#!/usr/bin/env bash
# ============================================================
# DWZ 短网址系统 — 一键上线迁移
#
# 把原先需要人工依次执行的 4 步运维动作收敛成一条命令：
#   1) 备份两个库中受影响的表
#   2) 把 url_hash 从「URL 全局唯一」重写为「同一 owner 作用域内唯一」
#      （同时作用于公共库 wjoy_log 与管理库 short_urls）
#   3) 建 webhook_queue 表（异步投递队列）
#   4) 静态资源构建 + 内容哈希版本号注入
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
log "0/5 检查数据库连通性"
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
  log "1/5 备份受影响表 → $TARGET_DIR"
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

# --- 2. url_hash 作用域化 -------------------------------------------------
log "2/5 重写 url_hash（URL 全局唯一 → 同一 owner 作用域内唯一）"
SQL_TMP="$(mktemp -t dwz_scope_hash.XXXXXX.sql)"
trap 'rm -f "$SQL_TMP"' EXIT
# 迁移脚本原本把库名写死为 1_xk7_cn / dwz_admin，这里按实际配置替换，
# 使其在任何库名组合下都能直接执行。
sed \
  -e "s/^USE \`1_xk7_cn\`;/USE \`${PUBLIC_DB}\`;/" \
  -e "s/^USE \`dwz_admin\`;/USE \`${ADMIN_DB}\`;/" \
  -e "s/\`dwz_admin\`\.\`short_urls\`/\`${ADMIN_DB}\`.\`short_urls\`/g" \
  "$ROOT/migrations/scope_url_hash.sql" > "$SQL_TMP"
grep -q "USE \`${PUBLIC_DB}\`;" "$SQL_TMP" || die "迁移脚本库名替换失败（公共库）"
grep -q "USE \`${ADMIN_DB}\`;"  "$SQL_TMP" || die "迁移脚本库名替换失败（管理库）"
run_sql "$PUBLIC_DB" "$SQL_TMP"

# --- 3. webhook_queue -----------------------------------------------------
log "3/5 建立 webhook 异步投递队列表"
WEBHOOK_SQL="$ROOT/migrations/add_webhook_queue.sql"
if [ -f "$WEBHOOK_SQL" ]; then
  HOOK_TMP="$(mktemp -t dwz_webhook_queue.XXXXXX.sql)"
  trap 'rm -f "$SQL_TMP" "$HOOK_TMP"' EXIT
  sed -e "s/^USE \`1_xk7_cn\`;/USE \`${PUBLIC_DB}\`;/" \
      -e "s/^USE \`dwz_admin\`;/USE \`${ADMIN_DB}\`;/" \
      "$WEBHOOK_SQL" > "$HOOK_TMP"
  run_sql "$PUBLIC_DB" "$HOOK_TMP" || warn "webhook_queue 建表失败（程序会自动退化为同步投递，可忽略后手动重试）"
else
  warn "未找到 migrations/add_webhook_queue.sql，跳过"
fi

# --- 4. 校验 --------------------------------------------------------------
log "4/5 校验迁移结果"
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

# --- 5. 静态资源 ----------------------------------------------------------
if [ "$DO_ASSETS" = "1" ]; then
  log "5/5 构建静态资源（压缩 + 内容哈希版本号注入）"
  if [ "$DRY_RUN" = "1" ]; then
    echo "  [dry-run] php migrations/build_assets.php"
  elif command -v "$PHP_BIN" >/dev/null 2>&1; then
    ( cd "$WEB_ROOT" && "$PHP_BIN" migrations/build_assets.php ) || warn "静态资源构建失败（不影响跳转，可稍后手动执行）"
  else
    warn "找不到 php，跳过静态资源构建"
  fi
else
  log "5/5 已跳过静态资源构建（--skip-assets）"
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
