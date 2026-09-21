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

# --- 内部文档入库门禁的自证（防它被改松、也防它误伤源码） ---------------------
# 这道门禁自己就是"曾经形同虚设"的受害者：上一版把关键词 pattern 直接作用于
# git ls-files 全集，结果后台"操作审计日志"功能的 audit.go / AuditLogList.vue
# 全部命中，门禁在自己的主干上直接红。所以正反两个方向都要断言，缺一个都不算修好。
DOCS_GUARD="$ROOT/deploy/check_internal_docs.sh"
CNB_YML="$ROOT/.cnb.yml"

if [ ! -f "$DOCS_GUARD" ]; then
  echo "缺少内部文档入库门禁脚本：$DOCS_GUARD" >&2
  fail=1
elif [ ! -f "$CNB_YML" ]; then
  echo "找不到 $CNB_YML，无法断言门禁已接线" >&2
  fail=1
else
  # 只数"作为命令被调用"的那一行。用子串匹配会把 shellcheck 文件清单、
  # 以及注释里提到脚本名的地方一起算进来（实测：子串匹配数出 4，真调用只有 2）。
  calls=$(grep -c '^[[:space:]]*sh deploy/check_internal_docs\.sh[[:space:]]*$' "$CNB_YML" || true)
  if [ "$calls" -ne 2 ]; then
    echo "内部文档门禁应在 .cnb.yml 的两处 stage 被调用，实际 $calls 处：" >&2
    echo "  只接一处 = 另一条流水线照常放行。" >&2
    fail=1
  fi

  # 反向：当前仓库真值必须通过。
  if ! sh "$DOCS_GUARD" >/dev/null 2>&1; then
    echo "内部文档门禁在**当前仓库自身**上失败，说明它开始误伤正常文件：" >&2
    sh "$DOCS_GUARD" >&2 || true
    fail=1
  fi

  # 正向：受控文档必须被拒。少拒一个 = 门禁静默失效，而 CI 仍然全绿。
  for leaked in \
    "docs/AUDIT_2026-09.md" \
    "AUDIT_REPORT.md" \
    "notes/verification-2026-09.md" \
    "site/SECURITY_SCAN.txt" \
    "x/THREAT_MODEL.pdf"
  do
    if printf '%s\n' "$leaked" | sh "$DOCS_GUARD" - >/dev/null 2>&1; then
      echo "内部文档门禁放过了应被拒绝的路径：$leaked" >&2
      fail=1
    fi
  done

  # 误伤回归：源码与公开公告文档必须放行（这几个文件的存在本身就是门禁的用途）。
  if ! printf '%s\n' \
      "backend/internal/handler/audit.go" \
      "backend/internal/service/audit_test.go" \
      "frontend/src/views/audit/AuditLogList.vue" \
      "SECURITY.md" "CHANGELOG.md" "README.md" \
      | sh "$DOCS_GUARD" - >/dev/null 2>&1; then
    echo "内部文档门禁误伤了源码/公开文档（它只该管 docs/ 与含敏感词的文档类后缀）" >&2
    fail=1
  fi
fi

# --- 敏感词单一真源的一致性（防四处副本再次漂移） ---------------------------
# 起因：这份 pattern 曾同时在门禁脚本、.gitignore、.dockerignore、.scanignore
# 各写一份，其中 .gitignore 只写了 `docs/`，比门禁窄。于是"把 docs/ 里的文件
# 改名挪到根目录"本地不拦、只有 CI 事后才红 —— 而改名/换目录正是那次泄露的绕过形状。
# 真源收进 deploy/internal_docs_pattern 后，这里断言：
#   1) 门禁脚本本身不再内联 pattern（否则真源形同虚设）；
#   2) .gitignore 对每个敏感词 × 每个文档后缀都有对应 glob（与门禁判定口径一致）；
#   3) .gitignore 不得出现"裸关键词"glob（无后缀），否则会连源码一起忽略
#      （audit.go / AuditLogList.vue 就是这么被误伤的，上一版踩过）；
#   4) .dockerignore 与 .scanignore 引用了真源文件名（文档性同步）。
PATTERN_FILE="$ROOT/deploy/internal_docs_pattern"
GITIGNORE="$ROOT/.gitignore"
DOCKERIGNORE="$ROOT/.dockerignore"
SCANIGNORE="$ROOT/.scanignore"

# 与 check_internal_docs.sh 保持同一份"文档后缀"清单。
DOC_EXTS="md txt rst pdf docx"

# 把敏感词转成大小写不敏感的 glob 片段：AUDIT -> [Aa][Uu][Dd][Ii][Tt]
word_to_glob() {
  printf '%s' "$1" | awk '{
    out = ""
    n = length($0)
    for (i = 1; i <= n; i++) {
      c = substr($0, i, 1)
      if (c == "_") { out = out "_" }
      else if (c ~ /[A-Za-z]/) { out = out "[" toupper(c) tolower(c) "]" }
      else { out = out c }
    }
    print out
  }'
}

if [ ! -f "$PATTERN_FILE" ]; then
  echo "缺少敏感词真源：$PATTERN_FILE" >&2
  fail=1
else
  # 1) 门禁脚本不得再内联 pattern（真源唯一）。
  if grep -qE '(^|[^_])PATTERN='"'"'[A-Z]' "$DOCS_GUARD"; then
    echo "deploy/check_internal_docs.sh 仍内联 pattern，应从 $PATTERN_FILE 读取" >&2
    fail=1
  fi

  PATTERN="$(grep -vE '^[[:space:]]*(#|$)' "$PATTERN_FILE" | head -n 1)"
  if [ -z "$PATTERN" ]; then
    echo "敏感词真源 $PATTERN_FILE 没有有效的 pattern 行" >&2
    fail=1
  fi
fi

# 2) .gitignore 必须对每个敏感词 × 每个文档后缀都有 glob。
if [ -f "$GITIGNORE" ] && [ -n "${PATTERN:-}" ]; then
  OLD_IFS="$IFS"
  IFS='|'
  for word in $PATTERN; do
    IFS="$OLD_IFS"
    glob="$(word_to_glob "$word")"
    for ext in $DOC_EXTS; do
      if ! grep -qF "*${glob}*.${ext}" "$GITIGNORE"; then
        echo ".gitignore 未覆盖「${word} × .${ext}」：改名/换目录可绕过入库门禁" >&2
        echo "  期望含模式：*${glob}*.${ext}   （真源：deploy/internal_docs_pattern）" >&2
        fail=1
      fi
    done
    # 3) 禁止裸关键词 glob：整行恰好是 *<glob>*（不带 .后缀）的那种。
    #    它是源码误伤源（audit_new_test.go / views/audit/NewPage.vue 会被忽略）。
    #    用固定字符串整行比较，避免 glob 里的 [ ] 被当成正则字符类。
    if grep -qxF "*${glob}*" "$GITIGNORE"; then
      echo ".gitignore 存在裸关键词 glob：*${glob}* —— 会连源码一起忽略，必须带文档后缀" >&2
      fail=1
    fi
    IFS='|'
  done
  IFS="$OLD_IFS"
fi

# 4) 忽略文件必须点到真源，避免"下一份副本"再次无声明地出现。
for f in "$DOCKERIGNORE" "$SCANIGNORE"; do
  [ -f "$f" ] || continue
  if ! grep -q 'internal_docs_pattern' "$f"; then
    echo "$f 未引用敏感词真源 deploy/internal_docs_pattern" >&2
    fail=1
  fi
done

if [ "$fail" -ne 0 ]; then
  echo "部署门禁检查失败" >&2
  exit 1
fi

echo "演练镜像能力与脚本依赖一致性检查通过"
