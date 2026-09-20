#!/usr/bin/env bash
# ============================================================
# DWZ 短网址系统 - 数据库备份脚本
# 备份管理库 (short_urls 等) 与公共库 (wjoy_log/members 等)。
#
# 环境变量:
#   DB_HOST       数据库主机（默认 127.0.0.1）
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
# ============================================================
set -euo pipefail

DB_HOST="${DB_HOST:-127.0.0.1}"
DB_ADMIN_USER="${DB_ADMIN_USER:?需要设置 DB_ADMIN_USER}"
DB_ADMIN_PASS="${DB_ADMIN_PASS:?需要设置 DB_ADMIN_PASS}"
DB_ADMIN_NAME="${DB_ADMIN_NAME:-dwz_admin}"
DB_PUBLIC_USER="${DB_PUBLIC_USER:?需要设置 DB_PUBLIC_USER}"
DB_PUBLIC_PASS="${DB_PUBLIC_PASS:?需要设置 DB_PUBLIC_PASS}"
DB_PUBLIC_NAME="${DB_PUBLIC_NAME:-1_xk7_cn}"
BACKUP_DIR="${BACKUP_DIR:-/www/server/dwz-admin/backups}"
KEEP="${KEEP:-7}"

mkdir -p "$BACKUP_DIR"
STAMP="$(date +%Y%m%d-%H%M%S)"

echo "==> $(date '+%F %T') 开始备份"

for db in admin public; do
  case "$db" in
    admin)  USER="$DB_ADMIN_USER"; PASS="$DB_ADMIN_PASS"; NAME="$DB_ADMIN_NAME";;
    public) USER="$DB_PUBLIC_USER"; PASS="$DB_PUBLIC_PASS"; NAME="$DB_PUBLIC_NAME";;
  esac
  OUT="$BACKUP_DIR/${NAME}-${STAMP}.sql.gz"
  mysqldump -h"$DB_HOST" -u"$USER" -p"$PASS" \
    --single-transaction --routines --triggers --quick --no-tablespaces \
    "$NAME" | gzip -9 > "$OUT"
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
  find "$BACKUP_DIR" -maxdepth 1 -type f -name "${prefix}-*.sql.gz" \
       -printf '%T@ %p\n' 2>/dev/null \
    | sort -rn \
    | tail -n +$((KEEP + 1)) \
    | while read -r _ts old; do
        rm -f "$old"
        echo "==> 已清理旧备份 $old"
      done
done

echo "==> $(date '+%F %T') 备份完成"
