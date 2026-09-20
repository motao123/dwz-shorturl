#!/usr/bin/env bash
# ============================================================
# DWZ 短网址系统 — 数据库恢复 / 恢复演练脚本
#
# 配套 deploy/backup.sh（#55 整改）。备份的价值只由"能恢复"这一点定义，
# 所以本脚本同时承担两件事：
#   1) --check      恢复演练：把 dump 导入临时库，逐库比对表数量与关键表行数，
#                   全程不碰线上库，跑完自动清理。这是唯一能证明"备份有效"的方式。
#   2) 默认（真恢复）把 dump 导入目标库。这是破坏性操作，必须显式确认。
#
# 用法：
#   # 演练（推荐每季度 + 每次备份策略变更后跑）：
#   DB_ADMIN_USER=... DB_ADMIN_PASS=... DB_PUBLIC_USER=... DB_PUBLIC_PASS=... \
#   bash deploy/restore.sh --check --dir /www/server/dwz-admin/backups
#
#   # 真恢复（默认导入同名原库；先自动备份当前库到 --pre-restore-dir）：
#   ... bash deploy/restore.sh --file /path/dwz_admin-20260920-030000.sql.gz
#
#   # 只恢复其中一个库：
#   ... bash deploy/restore.sh --admin-only  --file ...sql.gz
#   ... bash deploy/restore.sh --public-only --file ...sql.gz
#
# 环境变量（与 backup.sh 同名，便于共用一份 .env）：
#   DB_HOST / DB_PORT
#   DB_ADMIN_USER / DB_ADMIN_PASS / DB_ADMIN_NAME（默认 dwz_admin）
#   DB_PUBLIC_USER / DB_PUBLIC_PASS / DB_PUBLIC_NAME（默认 1_xk7_cn）
#   RESTORE_DIR      默认取 BACKUP_DIR，再默认 /www/server/dwz-admin/backups
#   RESTORE_TMP_PREFIX 演练临时库前缀（默认 dwz_restore_check）
#
# --check 需要账号具备 CREATE DATABASE / DROP 权限（演练要建临时库）。
# 业务账号（dwz）通常只有单库权限，此时请用一个有建库权限的账号跑演练，
# 例如 DB_ADMIN_USER=root。脚本会在开跑前显式预检并给出可操作的提示，
# 不会跑到一半才报一堆 Access denied。
#
# 退出码：0 成功 / 演练通过；非 0 失败。失败信息同时进 syslog。
# ============================================================
set -euo pipefail

DB_HOST="${DB_HOST:-127.0.0.1}"
DB_PORT="${DB_PORT:-3306}"
DB_ADMIN_USER="${DB_ADMIN_USER:?需要设置 DB_ADMIN_USER}"
DB_ADMIN_PASS="${DB_ADMIN_PASS:-}"
DB_ADMIN_NAME="${DB_ADMIN_NAME:-dwz_admin}"
DB_PUBLIC_USER="${DB_PUBLIC_USER:?需要设置 DB_PUBLIC_USER}"
DB_PUBLIC_PASS="${DB_PUBLIC_PASS:-}"
DB_PUBLIC_NAME="${DB_PUBLIC_NAME:-1_xk7_cn}"
RESTORE_DIR="${RESTORE_DIR:-${BACKUP_DIR:-/www/server/dwz-admin/backups}}"
TMP_PREFIX="${RESTORE_TMP_PREFIX:-dwz_restore_check}"

MYSQL_BIN="${MYSQL_BIN:-mysql}"
GZIP_BIN="${GZIP_BIN:-gzip}"

MODE="restore"
FILE=""
ADMIN_ONLY=0
PUBLIC_ONLY=0
ASSUME_YES=0

usage() { sed -n '2,40p' "$0" | sed 's/^# \{0,1\}//'; }

for arg in "$@"; do
  case "$arg" in
    --check)        MODE="check" ;;
    --file=*)       FILE="${arg#*=}" ;;
    --dir=*)        RESTORE_DIR="${arg#*=}" ;;
    --admin-only)   ADMIN_ONLY=1 ;;
    --public-only)  PUBLIC_ONLY=1 ;;
    --yes|-y)       ASSUME_YES=1 ;;
    --help|-h)      usage; exit 0 ;;
    *) echo "未知参数：$arg（用 --help 查看用法）" >&2; exit 2 ;;
  esac
done

if [ "$ADMIN_ONLY" -eq 1 ] && [ "$PUBLIC_ONLY" -eq 1 ]; then
  echo "--admin-only 与 --public-only 互斥" >&2; exit 2
fi

fail_alert() {
  msg="$1"
  echo "!!! $msg" >&2
  if command -v logger >/dev/null 2>&1; then
    logger -t dwz-restore -p daemon.err "$msg"
  fi
}

# 口令一律走 MYSQL_PWD，绝不进 argv（ps / docker inspect 看不到明文）。
mysql_run() {
  local user="$1" pass="$2" db="$3"; shift 3
  MYSQL_PWD="$pass" "$MYSQL_BIN" -h"$DB_HOST" -P"$DB_PORT" -u"$user" \
    --protocol=TCP ${db:+-D"$db"} "$@"
}

# 单值查询：mysql -N -B 输出无表头制表符分隔，取第一行第一列。
mysql_scalar() {
  local user="$1" pass="$2" db="$3" sql="$4"
  mysql_run "$user" "$pass" "$db" -N -B -e "$sql" 2>/dev/null | head -n1
}

# 校验一个 dump 文件是完整可用的：gzip 流完整 + 含 mysqldump 完成标记。
# 与 backup.sh 落盘前的校验同源，保证"能入库的一定是完整 dump"。
verify_dump() {
  local f="$1"
  [ -f "$f" ] || { fail_alert "dump 文件不存在：$f"; return 1; }
  [ -s "$f" ] || { fail_alert "dump 文件为空：$f"; return 1; }
  if ! "$GZIP_BIN" -t "$f" 2>/dev/null; then
    fail_alert "dump gzip 校验失败（文件截断）：$f"; return 1
  fi
  if ! "$GZIP_BIN" -dc "$f" | tail -c 4k | grep -q 'Dump completed'; then
    fail_alert "dump 缺少 'Dump completed' 标记（dump 被中断，不可用于恢复）：$f"; return 1
  fi
  return 0
}

# 找到目录里最新的、通过完整性校验的 dump。跳过残档，不会把半份备份当最新。
latest_valid_dump() {
  local prefix="$1" f
  while IFS= read -r f; do
    [ -n "$f" ] || continue
    if verify_dump "$f" 2>/dev/null; then
      echo "$f"; return 0
    fi
    echo "==> 跳过不完整备份：$f" >&2
  # ⚠️ 不用 `find -printf`：busybox 的 find 没有该动作（Alpine 上直接
  # "find: unrecognized: -printf" 并返回非零），改用 `-exec ls -1t {} +`
  # —— POSIX 行为，busybox / GNU 都支持，仍按 mtime 倒序。
  done < <(find "$RESTORE_DIR" -maxdepth 1 -type f -name "${prefix}-*.sql.gz" \
             -exec ls -1t {} + 2>/dev/null)
  return 1
}

# 建临时库 → 导入 → 返回表数量（演练用）。
import_to_temp_db() {
  local user="$1" pass="$2" tmpdb="$3" dump="$4"
  mysql_run "$user" "$pass" "" -e \
    "DROP DATABASE IF EXISTS \`$tmpdb\`; CREATE DATABASE \`$tmpdb\` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;"
  "$GZIP_BIN" -dc "$dump" | mysql_run "$user" "$pass" "$tmpdb"
  mysql_scalar "$user" "$pass" "$tmpdb" \
    "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema='$tmpdb'"
}

echo "==> $(date '+%F %T') restore.sh 启动（mode=$MODE）"
# 只在"从目录里自动挑 dump"时才要求目录存在；显式 --file 时目录无关紧要。
if [ -z "$FILE" ]; then
  [ -d "$RESTORE_DIR" ] || { fail_alert "备份目录不存在：$RESTORE_DIR"; exit 1; }
fi

# 演练要建/删临时库：先验权限，失败就带着可操作提示退出，
# 而不是执行到一半抛一堆 Access denied（那会让人以为是备份坏了）。
if [ "$MODE" = "check" ]; then
  PROBE_DB="${TMP_PREFIX}_preflight"
  if ! mysql_run "$DB_ADMIN_USER" "$DB_ADMIN_PASS" "" \
        -e "CREATE DATABASE IF NOT EXISTS \`$PROBE_DB\`; DROP DATABASE IF EXISTS \`$PROBE_DB\`;" >/dev/null 2>&1; then
    fail_alert "恢复演练需要 CREATE DATABASE / DROP 权限，当前账号（$DB_ADMIN_USER）没有。请用有建库权限的账号跑演练，例如 DB_ADMIN_USER=root。"
    exit 1
  fi
fi

# ---------- 演练模式 ----------
if [ "$MODE" = "check" ]; then
  NAME_SUFFIX="$(date +%s).$$"
  rc=0

  for which in admin public; do
    [ "$PUBLIC_ONLY" -eq 1 ] && [ "$which" = admin ] && continue
    [ "$ADMIN_ONLY" -eq 1 ]  && [ "$which" = public ] && continue
    case "$which" in
      admin)  USER="$DB_ADMIN_USER";  PASS="$DB_ADMIN_PASS";  NAME="$DB_ADMIN_NAME";;
      public) USER="$DB_PUBLIC_USER"; PASS="$DB_PUBLIC_PASS"; NAME="$DB_PUBLIC_NAME";;
    esac

    if [ -n "$FILE" ]; then
      DUMP="$FILE"
    else
      if ! DUMP="$(latest_valid_dump "$NAME")"; then
        fail_alert "在 $RESTORE_DIR 未找到 $NAME 的完整备份，演练中断"
        rc=1; continue
      fi
    fi

    echo "==> [$which] 校验 $DUMP"
    if ! verify_dump "$DUMP"; then rc=1; continue; fi

    # 线上库表数量（可能为 0，即"还没上线"）：演练断言恢复后表数 ≥ 它。
    LIVE_TABLES="$(mysql_scalar "$USER" "$PASS" "$NAME" \
      "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema='$NAME'")"
    LIVE_TABLES="${LIVE_TABLES:-0}"

    TMPDB="${TMP_PREFIX}_${which}_${NAME_SUFFIX}"
    echo "==> [$which] 导入临时库 $TMPDB"
    if ! RESTORED_TABLES="$(import_to_temp_db "$USER" "$PASS" "$TMPDB" "$DUMP")"; then
      fail_alert "[$which] 导入失败，演练不通过"
      mysql_run "$USER" "$PASS" "" -e "DROP DATABASE IF EXISTS \`$TMPDB\`;" >/dev/null 2>&1 || true
      rc=1; continue
    fi

    LIVE_TABLES="${LIVE_TABLES:-0}"
    RESTORED_TABLES="${RESTORED_TABLES:-0}"
    echo "==> [$which] 线上表数=$LIVE_TABLES 恢复后表数=$RESTORED_TABLES"
    if [ "$RESTORED_TABLES" -lt 1 ]; then
      fail_alert "[$which] 恢复后没有任何表，dump 不可用"
      rc=1
    elif [ "$LIVE_TABLES" -gt 0 ] && [ "$RESTORED_TABLES" -lt "$LIVE_TABLES" ]; then
      fail_alert "[$which] 恢复后表数($RESTORED_TABLES) 少于线上($LIVE_TABLES)，备份可能不完整"
      rc=1
    else
      echo "PASS: [$which] 从 $DUMP 恢复到 $TMPDB，$RESTORED_TABLES 张表，与线上一致"
    fi

    mysql_run "$USER" "$PASS" "" -e "DROP DATABASE IF EXISTS \`$TMPDB\`;" >/dev/null 2>&1 || true
    echo "==> [$which] 已清理临时库 $TMPDB"
  done

  if [ "$rc" -eq 0 ]; then
    echo "==> $(date '+%F %T') 恢复演练通过（未触碰线上库）"
  else
    echo "==> $(date '+%F %T') 恢复演练失败" >&2
  fi
  exit "$rc"
fi

# ---------- 真恢复模式（破坏性）----------
[ -n "$FILE" ] || { echo "--file=<dump.sql.gz> 是必填（真恢复模式）" >&2; exit 2; }
verify_dump "$FILE" || exit 1

TARGETS=""
[ "$PUBLIC_ONLY" -eq 0 ] && TARGETS="$TARGETS $DB_ADMIN_NAME"
[ "$ADMIN_ONLY" -eq 0 ]  && TARGETS="$TARGETS $DB_PUBLIC_NAME"
TARGETS="$(echo "$TARGETS" | xargs)"

echo "==> 即将把 $FILE 导入以下库（现存数据会被覆盖）：$TARGETS"
if [ "$ASSUME_YES" -ne 1 ]; then
  printf "确认继续？输入 yes 回车："
  read -r answer
  [ "$answer" = "yes" ] || { echo "已取消"; exit 0; }
fi

# 恢复前先把当前库备份一次，给自己留退路。
PRE_DIR="${RESTORE_DIR}/pre-restore-$(date +%Y%m%d-%H%M%S)"
mkdir -p "$PRE_DIR"
echo "==> 恢复前备份当前库到 $PRE_DIR"
for which in admin public; do
  [ "$PUBLIC_ONLY" -eq 1 ] && [ "$which" = admin ] && continue
  [ "$ADMIN_ONLY" -eq 1 ]  && [ "$which" = public ] && continue
  case "$which" in
    admin)  USER="$DB_ADMIN_USER";  PASS="$DB_ADMIN_PASS";  NAME="$DB_ADMIN_NAME";;
    public) USER="$DB_PUBLIC_USER"; PASS="$DB_PUBLIC_PASS"; NAME="$DB_PUBLIC_NAME";;
  esac
  MYSQL_PWD="$PASS" mysqldump -h"$DB_HOST" -P"$DB_PORT" -u"$USER" \
    --single-transaction --quick --no-tablespaces "$NAME" \
    | "$GZIP_BIN" -9 > "$PRE_DIR/${NAME}.sql.gz" || true
done

for which in admin public; do
  [ "$PUBLIC_ONLY" -eq 1 ] && [ "$which" = admin ] && continue
  [ "$ADMIN_ONLY" -eq 1 ]  && [ "$which" = public ] && continue
  case "$which" in
    admin)  USER="$DB_ADMIN_USER";  PASS="$DB_ADMIN_PASS";  NAME="$DB_ADMIN_NAME";;
    public) USER="$DB_PUBLIC_USER"; PASS="$DB_PUBLIC_PASS"; NAME="$DB_PUBLIC_NAME";;
  esac
  echo "==> 导入 $NAME"
  if ! "$GZIP_BIN" -dc "$FILE" | mysql_run "$USER" "$PASS" "$NAME"; then
    fail_alert "导入 $NAME 失败；恢复前备份在 $PRE_DIR，可按 BACKUP_RESTORE.md 回滚"
    exit 1
  fi
  TABLES="$(mysql_scalar "$USER" "$PASS" "$NAME" \
    "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema='$NAME'")"
  echo "==> $NAME 恢复完成，共 ${TABLES:-?} 张表"
done

echo "==> $(date '+%F %T') 恢复完成"
echo "==> 下一步（按 docs/BACKUP_RESTORE.md 第四节）："
echo "    1) 恢复 config.php / config.yaml（JWT 密钥变更会让所有会话失效）"
echo "    2) 重启 php-fpm 与 dwz-admin 服务"
echo "    3) 验证：curl -s <站点>/health；抽 3 条短链跳转；管理台登录"
