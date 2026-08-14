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
#   KEEP          保留备份份数（默认 7）
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

# 清理旧备份，仅保留最近 $KEEP 份
ls -1t "$BACKUP_DIR"/*.sql.gz 2>/dev/null | tail -n +$((KEEP + 1)) | while read -r old; do
  rm -f "$old"
  echo "==> 已清理旧备份 $old"
done

echo "==> $(date '+%F %T') 备份完成"
