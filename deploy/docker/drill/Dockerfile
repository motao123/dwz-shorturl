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
#   1. mysql 服务端 —— 演练要在**本容器内**直起 mysqld（脚本连的就是它）
#   2. mysql / mysqldump 客户端 —— deploy/backup.sh 与 restore.sh 本体
#   3. bash + gzip —— 两个脚本依赖
#   4. docker CLI / git / coreutils —— 流水线在默认环境里拉代码、跑通用命令
#
# 为什么不拉 mysql:8.0 容器跑 MySQL：容器化后 MySQL 落在**别的 netns**，
# 脚本从本容器连 127.0.0.1:3306 永远连不上（详见 .cnb.yml「备份→恢复演练」
# stage 的三轮实测结论）。容器内直起 mysqld 天然满足脚本的连接方式。
#
# 注意：Alpine 的 `mysql-client` 包只提供 `mysql` 交互客户端，**不含 `mysqldump`**。
# 必须用 `mariadb-client`：它同时提供 mysql / mysqladmin / mysqldump(mariadb-dump)，
# 且与 MySQL 8.0 服务端协议兼容；服务端由 `mariadb` 包提供（Alpine 的 apk 源里
# 没有 `mysql-server`）。deploy/docker/Dockerfile.entrypoint 的客户端也是这么装的。
# ============================================================
FROM docker:27-cli

# DRILL_ENV_REV: 2 —— 依赖有变必须 +1，便于在 CI 日志里确认跑的是哪一版镜像。
LABEL cnb.drill.env.rev="2"

RUN apk add --no-cache bash gzip mariadb-client mariadb coreutils git \
    # 构建期预检：装完立刻验证命令真的存在，把"包内容与预期不符"拦在构建期，
    # 而不是留到运行期才发现（第一轮 CI 红灯的根因就是 mysql-client 不含 mysqldump）。
    && command -v mysql >/dev/null \
    && command -v mysqladmin >/dev/null \
    && command -v mysqldump >/dev/null \
    && command -v mysqld >/dev/null \
    && command -v gzip >/dev/null
