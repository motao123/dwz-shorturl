# ============================================================
# 「备份→恢复演练」阶段的构建环境（CI 专用）
#
# 该 stage 需要三样东西同时可用：
#   1. docker CLI —— 用 DinD 起一个临时 MySQL 8.0（备份脚本要连真库）
#   2. bash + gzip —— deploy/backup.sh 与 restore.sh 依赖
#   3. mysql / mysqldump 客户端 —— 脚本本体
#
# 没有任何现成官方镜像同时满足这三项（自带 docker CLI 的镜像都没有 mysql 客户端，
# 自带 mysql 客户端的镜像都没有 docker CLI）。所以用一个专用 Dockerfile 预装，
# 而不是在流水线里 apk add —— 后者每次运行都要联网装包，且会被平台校验器提示
# "手动安装系统依赖"。
# ============================================================
FROM docker:27-cli

RUN apk add --no-cache bash gzip mysql-client coreutils
