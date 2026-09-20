# ============================================================
# 「部署脚本与清单门禁」阶段的构建环境（CI 专用）
#
# 该 stage 要跑：sh 起脚本 → bash -n / shellcheck / check_manifest.sh / grep 断言。
# 平台用 `sh -c <script>` 起 stage 容器，脚本里又显式调用 bash，
# 因此镜像**必须同时具备 sh、bash、shellcheck、grep、git、coreutils**。
#
# 为什么不用官方 shellcheck 镜像（该 stage 连红两轮都因此）：
#
#   koalaman/shellcheck:stable
#     → **FROM scratch**，整个镜像只有一个文件 /bin/shellcheck
#       （2 层共 ~3.8MB，Entrypoint=["/bin/shellcheck"]）。
#       没有 sh / bash / env / grep / coreutils。平台起容器即失败：
#         exec: "sh": executable file not found in $PATH   (返回码 127)
#       注意 `docker pull` 是成功的，失败发生在 runc create 阶段，
#       报错看起来和"镜像不存在"完全不像。
#
#   koalaman/shellcheck-alpine:stable
#     → alpine-minirootfs + shellcheck，有 sh/grep，但**没有 bash**
#       （alpine 默认不装 bash）→ `bash -n` 报 `sh: bash: not found`，同样 127。
#
# 两个官方 tag 各缺一半，所以这里自建：alpine + bash + shellcheck + grep + git。
# 也不想在 stage 里"运行期现下 shellcheck 二进制"：那会把 CI 出网
# （github release / npm registry 任一不通）变成门禁的单点依赖，且每次都要等下载。
# 镜像预装是构建期一次性成本，运行期零网络、结果可复现。
#
# 与 drill 镜像同底（Alpine），也顺带对齐了生产容器（Alpine）的工具链语义。
#
# GATE_ENV_REV: 1 —— 依赖有变必须 +1，便于在 CI 日志里确认跑的是哪一版镜像。
# ============================================================
FROM alpine:3.22

LABEL cnb.gate.env.rev="1"

# 构建期预检：把"包内容与预期不符"拦在构建期，而不是留到运行期才发现。
# （本 stage 两次红灯都是"镜像里没有预期的命令"，必须在这里自证。）
RUN apk add --no-cache bash shellcheck grep git coreutils \
    && command -v sh >/dev/null \
    && command -v bash >/dev/null \
    && command -v shellcheck >/dev/null \
    && command -v grep >/dev/null \
    && command -v git >/dev/null
