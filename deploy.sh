#!/usr/bin/env bash
# ============================================================
# DWZ 短网址系统 - 统一部署脚本
# 用法:
#   DWZ_SERVER=1.xk7.cn DWZ_USER=root DWZ_SSH_KEY=~/.ssh/id_rsa ./deploy.sh
# 或（用密码时安装 sshpass）:
#   DWZ_SERVER=1.xk7.cn DWZ_USER=root DWZ_PASS='xxx' ./deploy.sh
#
# 环境变量:
#   DWZ_SERVER  服务器 IP/域名
#   DWZ_USER    SSH 用户（默认 root）
#   DWZ_PASS    密码（可选，需要 sshpass）
#   DWZ_SSH_KEY 私钥路径（可选）
#   DWZ_WEB_DIR 网站根目录（默认 /data/www/wwwroot/1.xk7.cn）
#   DWZ_APP_DIR 后端目录（默认 /www/server/dwz-admin）
# ============================================================
set -euo pipefail

SERVER="${DWZ_SERVER:?需要设置 DWZ_SERVER}"
USER="${DWZ_USER:-root}"
WEB_DIR="${DWZ_WEB_DIR:-/data/www/wwwroot/1.xk7.cn}"
APP_DIR="${DWZ_APP_DIR:-/www/server/dwz-admin}"
ROOT="$(cd "$(dirname "$0")" && pwd)"

if [ -n "${DWZ_PASS:-}" ]; then
  SSHPASS_CMD="sshpass -p '${DWZ_PASS}'"
  SCP="$SSHPASS_CMD scp"
  SSH="$SSHPASS_CMD ssh"
else
  SCP="scp"
  SSH="ssh"
fi
SSH_ARGS="-o StrictHostKeyChecking=no"
if [ -n "${DWZ_SSH_KEY:-}" ]; then SSH_ARGS="$SSH_ARGS -i $DWZ_SSH_KEY"; fi
DEST="$USER@$SERVER"

echo "==> 1/4 构建 Go 后端 (linux/amd64)"
mkdir -p "$ROOT/backend/dist"
(cd "$ROOT/backend" && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o "$ROOT/backend/dist/dwz-admin-linux" ./cmd/server)

echo "==> 2/4 构建前端"
(cd "$ROOT/frontend" && npm run build)

echo "==> 3/4 上传 PHP 与后端"
$SCP $SSH_ARGS "$ROOT"/do.php "$ROOT"/api.php "$ROOT"/batch.php "$ROOT"/member.php "$ROOT"/index.html "$DEST:$WEB_DIR/"
$SCP $SSH_ARGS -r "$ROOT"/includes "$ROOT"/assets "$DEST:$WEB_DIR/"
$SCP $SSH_ARGS "$ROOT/backend/dist/dwz-admin-linux" "$DEST:$APP_DIR/dwz-admin-linux.tmp"

echo "==> 4/4 上传前端 dist 并重启后端"
$SSH $SSH_ARGS "$DEST" "rm -rf /tmp/dwz-dist && mkdir -p /tmp/dwz-dist"
$SCP $SSH_ARGS -r "$ROOT/frontend/dist/." "$DEST:/tmp/dwz-dist/"
$SSH $SSH_ARGS "$DEST" "
  chown -R www:www '$WEB_DIR' >/dev/null 2>&1 || true
  mv '$APP_DIR/dwz-admin-linux' '$APP_DIR/dwz-admin-linux.bak'
  mv '$APP_DIR/dwz-admin-linux.tmp' '$APP_DIR/dwz-admin-linux'
  chmod +x '$APP_DIR/dwz-admin-linux' && chown www:www '$APP_DIR/dwz-admin-linux'
  systemctl restart dwz-admin.service
  rm -rf '$WEB_DIR/admin'/* '$WEB_DIR/member'/*
  cp -r /tmp/dwz-dist/* '$WEB_DIR/admin/'
  cp /tmp/dwz-dist/member.html '$WEB_DIR/member/index.html'
  mkdir -p '$WEB_DIR/member/assets'
  cp -r /tmp/dwz-dist/assets/* '$WEB_DIR/member/assets/'
  chown -R www:www '$WEB_DIR/admin' '$WEB_DIR/member'
  systemctl is-active dwz-admin.service
"
echo "==> 完成"
