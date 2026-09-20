# ============================================================
# 「备份→恢复演练」阶段的构建环境（CI 专用）
#
# 作为 test-gate 流水线的 **pipeline 级默认构建环境**（docker.build）：只有
# 「备份→恢复演练」stage 用默认环境，其余 stage 各自声明 image 覆盖。
# ⚠️ 不能写成 stage/job 级的 docker.build —— Stage 只支持 image，job 级 docker
# 会被静默忽略，本 stage 随即退回 cnbcool/default-build-env（无 mysqldump），
# backup.sh 直接 "mysqldump: command not found"。这是 PR #48 首次真跑的根因。
#
# 该 stage 需要四样东西同时可用：
#   1. docker CLI —— 用 DinD 起一个临时 MySQL 8.0（备份脚本要连真库）
#   2. bash + gzip —— deploy/backup.sh 与 restore.sh 依赖
#   3. mysql / mysqldump 客户端 —— 脚本本体
#   4. git / coreutils —— 流水线在默认环境里拉代码、跑通用命令要用
#
# 没有任何现成官方镜像同时满足这些（自带 docker CLI 的镜像都没有 mysql 客户端，
# 自带 mysql 客户端的镜像都没有 docker CLI）。所以用一个专用 Dockerfile 预装，
# 而不是在流水线里 apk add —— 后者每次运行都要联网装包，且会被平台校验器提示
# "手动安装系统依赖"。
#
# 注意：Alpine 的 mysql-client 包只提供 `mysql` 交互客户端，**不含 `mysqldump`**。
# 必须用 mariadb-client：它同时提供 mysql / mysqladmin / mysqldump(mariadb-dump)，
# 且与 MySQL 8.0 服务端协议兼容。deploy/docker/Dockerfile.entrypoint 也是这么装的。
# ============================================================
FROM docker:27-cli

RUN apk add --no-cache bash gzip mariadb-client coreutils git
