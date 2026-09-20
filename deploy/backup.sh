#!/usr/bin/env bash
# ============================================================
# DWZ 短网址系统 - 数据库备份脚本
# 备份管理库 (short_urls 等) 与公共库 (wjoy_log/members 等)。
#
# 环境变量:
#   DB_HOST       数据库主机（默认 127.0.0.1）
#   DB_PORT       数据库端口（默认 3306，仅当非默认端口时需设置）
#   DB_ADMIN_USER 管理库账号
#   DB_ADMIN_PASS 管理库密码
#   DB_ADMIN_NAME 管理库名（默认 dwz_admin）
#   DB_PUBLIC_USER 公共库账号
#   DB_PUBLIC_PASS 公共库密码
#   DB_PUBLIC_NAME 公共库名（默认 1_xk7_cn）
#   BACKUP_DIR    备份目录（默认 /www/server/dwz-admin/backups）
#   KEEP          每个库各自保留的备份份数（默认 7，两个库合计最多 2*KEEP 份）
#
# 部署建议 cron（每日凌晨 3 点）:
#   0 3 * * * /www/server/dwz-admin/backup.sh >> /var/log/dwz-backup.log 2>&1
#
# 可靠性（#55 整改）：
#   - 口令全部走 MYSQL_PWD 环境变量，不出现在 ps / docker inspect
#   - dump 先写 .part 临时文件，校验「Dump completed」尾标记 + gzip -t 通过
#     才原子 mv 到最终文件；任何失败都删除残档，不会留下"看起来最新"的半份备份
#   - flock 串行化，避免 cron 抖动/人工重跑造成同一目录并发写
#   - 失败时写 syslog（logger）便于集中告警
# ============================================================
set -euo pipefail

DB_HOST="${DB_HOST:-127.0.0.1}"
DB_PORT="${DB_PORT:-3306}"
DB_ADMIN_USER="${DB_ADMIN_USER:?需要设置 DB_ADMIN_USER}"
DB_ADMIN_PASS="${DB_ADMIN_PASS:?需要设置 DB_ADMIN_PASS}"
DB_ADMIN_NAME="${DB_ADMIN_NAME:-dwz_admin}"
DB_PUBLIC_USER="${DB_PUBLIC_USER:?需要设置 DB_PUBLIC_USER}"
DB_PUBLIC_PASS="${DB_PUBLIC_PASS:?需要设置 DB_PUBLIC_PASS}"
DB_PUBLIC_NAME="${DB_PUBLIC_NAME:-1_xk7_cn}"
BACKUP_DIR="${BACKUP_DIR:-/www/server/dwz-admin/backups}"
KEEP="${KEEP:-7}"

# 最终产物用 .sql.gz；写入过程用 .part 中间态，校验通过才改名换姓。
SUFFIX=".sql.gz"
PART_SUFFIX=".sql.gz.part"

mkdir -p "$BACKUP_DIR"

# 串行化：同目录只允许一个备份进程。cron 抖动、人工重跑、任务重叠都会命中。
# 拿不到锁直接退出 0（不是错误，是"已有人在备"），避免告警风暴。
LOCK_FILE="$BACKUP_DIR/.backup.lock"
exec 9>"$LOCK_FILE"
if ! flock -n 9; then
  echo "==> 已有备份进程在运行（$LOCK_FILE 被占用），本次跳过"
  exit 0
fi

STAMP="$(date +%Y%m%d-%H%M%S)"

# 进程级清理：脚本被 kill / dump 中途失败时，删掉本次可能残留的 .part 文件。
# 用变量记录当前正在写的中间文件，避免误删他人文件。
CURRENT_PART=""
cleanup() {
  rc=$?
  [ -n "$CURRENT_PART" ] && rm -f "$CURRENT_PART"
  return $rc
}
trap cleanup EXIT INT TERM

# 失败时既打日志也进 syslog，便于被集中告警平台收敛。
fail_alert() {
  msg="$1"
  echo "!!! $msg" >&2
  if command -v logger >/dev/null 2>&1; then
    logger -t dwz-backup -p daemon.err "$msg"
  fi
}

echo "==> $(date '+%F %T') 开始备份"

for db in admin public; do
  case "$db" in
    admin)  USER="$DB_ADMIN_USER"; PASS="$DB_ADMIN_PASS"; NAME="$DB_ADMIN_NAME";;
    public) USER="$DB_PUBLIC_USER"; PASS="$DB_PUBLIC_PASS"; NAME="$DB_PUBLIC_NAME";;
  esac
  OUT="$BACKUP_DIR/${NAME}-${STAMP}${SUFFIX}"
  PART="$BACKUP_DIR/${NAME}-${STAMP}${PART_SUFFIX}"
  CURRENT_PART="$PART"

  # 单库模式下管理库与公共库可能配成同一个库名：同一个 STAMP 会算出同一个
  # OUT，第二次 dump 覆盖第一份。跳过重复并显式提示，避免"两份备份其实一份"。
  if [ -f "$OUT" ]; then
    echo "==> 跳过 $NAME：本批次已存在同名备份 $OUT（两库同名？）"
    CURRENT_PART=""
    continue
  fi

  # 口令走环境变量（MYSQL_PWD），不拼进 argv，ps / docker inspect 看不到明文。
  if ! MYSQL_PWD="$PASS" mysqldump \
        -h"$DB_HOST" -P"$DB_PORT" -u"$USER" \
        --single-transaction --routines --triggers --quick --no-tablespaces \
        "$NAME" | gzip -9 > "$PART"; then
    rm -f "$PART"
    CURRENT_PART=""
    fail_alert "备份失败：$NAME dump/gzip 非零退出，已删除残档 $PART"
    exit 1
  fi

  # 三重校验，缺一不可：
  #   1) 中间文件非空（空文件 gzip -t 也会"通过"）
  #   2) gzip 流可完整解压（截断的 gz 会失败）
  #   3) 明文尾部含 mysqldump 的完成标记 "Dump completed"
  #      —— 只有第 3 条能抓住"gzip 合法但 SQL 被截断"这类最危险的半份备份。
  if [ ! -s "$PART" ]; then
    rm -f "$PART"; CURRENT_PART=""
    fail_alert "备份失败：$NAME 产出文件为空，已删除残档"
    exit 1
  fi
  if ! gzip -t "$PART" 2>/dev/null; then
    rm -f "$PART"; CURRENT_PART=""
    fail_alert "备份失败：$NAME gzip 校验不通过（文件截断），已删除残档 $PART"
    exit 1
  fi
  if ! gzip -dc "$PART" | tail -c 4k | grep -q 'Dump completed'; then
    rm -f "$PART"; CURRENT_PART=""
    fail_alert "备份失败：$NAME 缺少 'Dump completed' 完成标记（dump 被中断），已删除残档 $PART"
    exit 1
  fi

  # 校验通过才原子落盘：mv 同目录内是 rename(2)，读者只会看到完整文件。
  mv -f "$PART" "$OUT"
  CURRENT_PART=""
  SIZE="$(du -h "$OUT" | cut -f1)"
  echo "==> 已备份 $NAME -> $OUT ($SIZE)"
done

# 清理旧备份：按库分别轮转，每个库各保留最近 $KEEP 份。
#
# 注意：一次运行会为两个库各产出一个文件，所以不能对 *.sql.gz 统一排序取
# 最近 N 份 —— 那样两个库会共用同一份配额，备份更频繁/更早的库会被提前删除。
# （历史行为：KEEP=7 时两个库合计只剩 7 份，管理库约 3 份先被删掉。）
for prefix in "$DB_ADMIN_NAME" "$DB_PUBLIC_NAME"; do
  # 按修改时间倒序，保留最近 $KEEP 份（文件名含时间戳，同一次运行的两个库
  # 时间戳相同，-t 排序稳定）。用 find -printf 而非 ls，避免文件名含空格/
  # 特殊字符时被拆词（SC2012）。
  # 只轮转最终产物（*.sql.gz），绝不匹配 .part 中间文件。
  find "$BACKUP_DIR" -maxdepth 1 -type f -name "${prefix}-*${SUFFIX}" \
       -printf '%T@ %p\n' 2>/dev/null \
    | sort -rn \
    | tail -n +$((KEEP + 1)) \
    | while read -r _ts old; do
        rm -f "$old"
        echo "==> 已清理旧备份 $old"
      done
done

echo "==> $(date '+%F %T') 备份完成"
