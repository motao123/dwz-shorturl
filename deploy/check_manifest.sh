#!/usr/bin/env bash
# ============================================================
# 部署清单一致性门禁（#16）
#
# 断言 deploy/manifest.txt 与 deploy/docker/Dockerfile.web 的 COPY 列表
# 完全一致：多一个（清单里有但镜像没拷）或少一个（镜像拷了但清单没列）都失败。
#
# 为什么需要它：清单与 Dockerfile 是"同一份事实"的两处表达，人一定会改漏一处
# （这次就是 deploy.sh 与 Dockerfile.web 漂移）。把一致性交给 CI，比靠人记可靠。
#
# 用法：bash deploy/check_manifest.sh
# 退出码：0 一致；1 不一致（打印差异）；2 解析失败
# ============================================================
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
MANIFEST="$ROOT/deploy/manifest.txt"
DOCKERFILE="$ROOT/deploy/docker/Dockerfile.web"

[ -f "$MANIFEST" ]   || { echo "缺少 $MANIFEST" >&2; exit 2; }
[ -f "$DOCKERFILE" ] || { echo "缺少 $DOCKERFILE" >&2; exit 2; }

MAN_FILES="$(sed 's/#.*//' "$MANIFEST" | tr -d '\r' | sed 's/^[[:space:]]*//; s/[[:space:]]*$//' | grep -v '/$' | grep -v '^$' | sort)"
MAN_DIRS="$(sed 's/#.*//' "$MANIFEST" | tr -d '\r' | sed 's/^[[:space:]]*//; s/[[:space:]]*$//' | grep '/$' | sed 's:/$::' | grep -v '^$' | sort)"

# Dockerfile.web 里前台文件的 COPY 行：以 .php/.html/.txt/.webmanifest 结尾、
# 目标是 /var/www/dwz/ 的那一行。
DK_FILES="$(grep -E '^COPY .*[.](php|html|txt|webmanifest)[[:space:]]+/var/www/dwz/$' "$DOCKERFILE" \
  | sed -E 's/^COPY //; s/ [^ ]+$//' | tr ' ' '\n' | grep -v '^$' | sort)"

# 目录 COPY：`COPY xxx/ /var/www/dwz/xxx/`
DK_DIRS="$(grep -E '^COPY [A-Za-z0-9_.-]+/ /var/www/dwz/[A-Za-z0-9_.-]+/$' "$DOCKERFILE" \
  | sed -E 's/^COPY ([A-Za-z0-9_.-]+)\/ .*/\1/' | sort)"

fail=0

if [ "$MAN_FILES" != "$DK_FILES" ]; then
  echo "文件清单不一致：deploy/manifest.txt vs deploy/docker/Dockerfile.web" >&2
  diff <(printf '%s\n' "$MAN_FILES") <(printf '%s\n' "$DK_FILES") >&2 || true
  fail=1
fi

if [ "$MAN_DIRS" != "$DK_DIRS" ]; then
  echo "目录清单不一致：deploy/manifest.txt vs deploy/docker/Dockerfile.web" >&2
  diff <(printf '%s\n' "$MAN_DIRS") <(printf '%s\n' "$DK_DIRS") >&2 || true
  fail=1
fi

# deploy.sh 必须真的读清单，而不是退回手写列表（防回退）。
if ! grep -q 'deploy/manifest.txt' "$ROOT/deploy.sh"; then
  echo "deploy.sh 未引用 deploy/manifest.txt（部署清单必须单一来源）" >&2
  fail=1
fi

# 违规词库（#11）：它刻意不在上面的清单里——manifest 描述的是「web 根下的前台
# 文件」，而词库一旦被公开下载就等于把黑名单交给提交方。正因它走的是另一条
# 通道，这里必须单独断言两条部署路径都带着它，否则又会退化成静默降级。
RULES_SRC="backend/internal/pkg/data/violation_rules.json"
if [ ! -f "$ROOT/$RULES_SRC" ]; then
  echo "违规词库源文件不存在：$RULES_SRC" >&2
  fail=1
fi
if ! grep -q "$RULES_SRC" "$DOCKERFILE"; then
  echo "Dockerfile.web 未把违规词库带进镜像，公开入口合规拦截会退化成 3 域/5 词（#11）" >&2
  fail=1
fi
if ! grep -q "$RULES_SRC" "$ROOT/deploy.sh"; then
  echo "deploy.sh 未上传违规词库，宝塔/裸机部署的合规拦截会退化（#11）" >&2
  fail=1
fi

# 运行时不得内嵌上游维护者的域名（#20）。这类值一旦出现在默认配置、外发邮件、
# 建库脚本或示例里，每一个下游部署都会把自己的用户送到作者的域上——
# public.base_url 的默认值与续期邮件里的会员中心链接都栽过。
# docs/ 与 site/ 是作者本人的文档与产品站，不在禁止之列。
# 用 -F 是按字面匹配：`.` 在正则里是通配符，会把公共库名 `1_xk7_cn` 也算命中，
# 而那个下划线标识符改名等于给存量部署做一次建库迁移，属另一回事（另立条目跟踪）。
UPSTREAM_HOST="1.xk7.cn"
HITS="$(grep -rlFI --exclude-dir=node_modules --exclude-dir=.git --exclude-dir=docs \
  --exclude-dir=site --exclude-dir=.zcode --exclude-dir=dist --exclude-dir=vendor \
  --exclude='*_test.go' --exclude='*.md' --exclude='check_manifest.sh' \
  -- "$UPSTREAM_HOST" "$ROOT" 2>/dev/null || true)"
if [ -n "$HITS" ]; then
  echo "运行时文件里仍出现上游维护者域名 $UPSTREAM_HOST（#20），请改为配置驱动或中性占位：" >&2
  printf '  %s\n' $HITS >&2
  fail=1
fi

if [ "$fail" -ne 0 ]; then
  echo "部署清单一致性检查失败" >&2
  exit 1
fi

echo "部署清单一致性检查通过（文件 $(printf '%s\n' "$MAN_FILES" | wc -l | tr -d ' ') 个，目录 $(printf '%s\n' "$MAN_DIRS" | wc -l | tr -d ' ') 个）"

# ------------------------------------------------------------
# 流水线 stage 缩进一致性（防回退）
#
# 背景：PR #48 期间，`.cnb.yml` 里「部署脚本与清单门禁」stage 整个被缩进成了
# 演练 stage 的续行 —— YAML 仍能解析（内容被当成上一条 script 的字符串），
# 但这条门禁**从来没有真正跑过**，shellcheck / 清单一致性断言全部形同虚设，
# 且平台校验器不报错。
#
# 断言方式：取所有 stage 名（缩进 > 4 的 `- name:` 行）的缩进宽度，必须唯一。
# 流水线名（缩进 4）与 stage（缩进 8）是两种合法层级，所以只比较 >4 的那组；
# 同一组内出现两种宽度即说明有条目被意外缩进（多为被续行吞并），直接失败。
# ------------------------------------------------------------
CNB_YML="$ROOT/.cnb.yml"
if [ -f "$CNB_YML" ]; then
  widths="$(grep -E '^[[:space:]]*- name: ' "$CNB_YML" \
    | sed -E 's/^( *)- name: .*/\1/' \
    | awk '{print length}' \
    | awk '$1 > 4' \
    | sort -u)"
  width_count="$(printf '%s\n' "$widths" | grep -c . || true)"
  if [ "$width_count" -gt 1 ]; then
    echo ".cnb.yml 中 stage（'- name:'）存在多种缩进宽度，可能有 stage 被误缩进成续行：" >&2
    grep -nE '^[[:space:]]*- name: ' "$CNB_YML" >&2
    fail=1
  fi
fi

if [ "$fail" -ne 0 ]; then
  echo "部署门禁检查失败" >&2
  exit 1
fi

echo "流水线 stage 缩进一致性检查通过"

# ------------------------------------------------------------
# 演练镜像能力与脚本依赖的一致性（防回退）
#
# 背景：deploy/backup.sh 的旧档轮转用 `find ... -printf '%T@ %p\n'` 按 mtime 取
# 最近 KEEP 份，而 Alpine（drill 镜像基镜像）自带的 BusyBox `find` **不实现
# `-printf`** —— 缺 GNU findutils 时这一步 `find: unrecognized: -printf` 并以 1
# 退出，让一份已经备份成功的运行整体判失败。这类"脚本用到了镜像里没有的能力"
# 属于静默事故，人工记不住，所以在这里按能力对映断言：
#   backup.sh 里出现真正的 `find ... -printf` 调用 ⇒ drill 镜像必须装 findutils。
#
# 判断按**逻辑行**做：先把 `\` 续行拼起来、去掉整行注释。否则
#   find "$DIR" ... \
#     -printf '%T@ %p\n'
# 这种写法里 `-printf` 与 `find` 不在同一物理行，断言会漏判成"没用到"而形同虚设。
#
# 用 awk 而不是 grep 正则：`\`、`-printf` 里的 `-`、以及中英文混排的 grep 行为
# 在不同 busybox/GNU 实现下不一致，第一版就是这么写废的。awk 按字段判断最稳。
# ------------------------------------------------------------
BACKUP_SH="$ROOT/deploy/backup.sh"
DRILL_DOCKERFILE="$ROOT/deploy/docker/drill/Dockerfile"
if [ -f "$BACKUP_SH" ] && [ -f "$DRILL_DOCKERFILE" ]; then
  # 逻辑行里是否出现 "find ... -printf"：awk 先拼续行、扔注释，再看同一行内
  # 是否同时有字段 find 与 -printf（find 在 -printf 之前）。
  uses_find_printf="$(
    awk '
      { line = $0; sub(/[ \t]+$/, "", line) }
      pending != "" { line = pending " " line; pending = "" }
      line ~ /\\$/ { sub(/\\$/, "", line); pending = line; next }
      line ~ /^[ \t]*#/ { next }
      { n = split(line, f, /[ \t]+/); fi = 0; pi = 0
        for (i = 1; i <= n; i++) {
          if (f[i] == "find") fi = i
          if (f[i] == "-printf" && pi == 0) pi = i
        }
        if (fi > 0 && pi > fi) { print FILENAME ": " line; found = 1 }
      }
      END { exit(found ? 0 : 1) }
    ' "$BACKUP_SH" 2>/dev/null || true
  )"
  if [ -n "$uses_find_printf" ]; then
    # `apk add` 那一行（含续行）里必须出现 findutils。
    has_findutils="$(
      awk '/^[ \t]*RUN[ \t]+apk add/ { print; exit }' "$DRILL_DOCKERFILE" 2>/dev/null
    )"
    case "$has_findutils" in
      *findutils*) ;;
      *)
        echo "deploy/backup.sh 以 find -printf 轮转旧档，但 drill 镜像未安装 findutils：" >&2
        echo "  Alpine 的 BusyBox find 不支持 -printf（find: unrecognized: -printf），" >&2
        echo "  会让演练在 backup.sh 里以非 0 退出。见 $DRILL_DOCKERFILE" >&2
        printf '%s\n' "$uses_find_printf" >&2
        fail=1
        ;;
    esac
  fi
fi

if [ "$fail" -ne 0 ]; then
  echo "部署门禁检查失败" >&2
  exit 1
fi

echo "演练镜像能力与脚本依赖一致性检查通过"
