#!/bin/sh
# ============================================================
# Web 层启动脚本
#
# 关键前提：config.php 由 init 容器生成在共享卷 /app/configs 下。
# 这里等它出现后软链到站点根目录，因为 PHP 前台（includes/api.inc.php）
# 是 require ROOT . 'config.php'，路径固定在仓库根目录，不能改。
# ============================================================
set -e

CONFIG_SRC=/app/configs/config.php
CONFIG_DST=/var/www/dwz/config.php

echo "[web] 等待初始化容器生成 config.php ..."
i=0
while [ ! -f "$CONFIG_SRC" ]; do
  i=$((i + 1))
  if [ "$i" -gt 150 ]; then
    echo "[web] 等待 config.php 超时（300s）。请检查 init 容器日志：docker compose logs init" >&2
    exit 1
  fi
  sleep 2
done

ln -sf "$CONFIG_SRC" "$CONFIG_DST"
echo "[web] 已链接 config.php"

# 限流文件目录：config.php 的 $rate_limit_dir 指向 /var/www/dwz/logs/ratelimit
# （nginx 已 deny /logs/ 的 Web 访问），并保证 php-fpm 的 www-data 用户可写。
mkdir -p /var/www/dwz/logs/ratelimit
chown -R www-data:www-data /var/www/dwz/logs

exec "$@"
