#!/usr/bin/env sh
# 内部文档入库门禁（CI 与本地共用同一实现，两处 stage 都调它）。
#
# 为什么需要：本仓库的审计/分析类文档逐条写着缺陷与可利用路径，曾被提交进公开仓库，
# 结果从任何一条还指向旧提交的 ref 都能匿名读到攻击面清单；事后用 git-filter-repo
# 重写历史也收不回已经公开的那段时间窗。
# .gitignore 里的 docs/ 只拦"新增文件"，拦不住改名、换目录、或从别的仓库把文档合进来，
# 所以这里再补一道独立门禁。
#
# 判定只看路径，不看内容，因此不会因为措辞误判：
#   1) docs/ 下的任何被跟踪文件 —— 内部文档目录整体不入库；
#   2) 路径中含敏感词的文档类文件（.md/.txt/.rst/.pdf/.docx）—— 改名换目录也照样命中。
# 源码不参与第 2 条：后台本身有"操作审计日志"功能，
# backend/internal/handler/audit.go 这类源码不是文档，拿关键词扫全部路径会全员误伤
# （这正是上一版把 pattern 直接作用于 git ls-files 的后果）。
#
# 敏感词不写在这里，从 deploy/internal_docs_pattern 读（唯一真源）：
# 同一份 pattern 还要被 .gitignore / .dockerignore / .scanignore 用，
# 各处各写一份必然漂移（本次泄露的根因之一）。
#
# 用法：
#   sh deploy/check_internal_docs.sh                                  # 检查当前仓库被跟踪文件
#   printf '%s\n' a.md b.md | sh deploy/check_internal_docs.sh -      # 从 stdin 读清单（自测用）

set -e

# 相对本脚本定位 pattern 文件，调用方 cd 到哪都不影响。
SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
PATTERN_FILE="$SCRIPT_DIR/internal_docs_pattern"

if [ ! -f "$PATTERN_FILE" ]; then
    echo "缺少敏感词真源文件：$PATTERN_FILE" >&2
    echo "它为 .gitignore / .dockerignore / .scanignore / 本门禁共用，不能缺。" >&2
    exit 1
fi

# 单行扩展正则：跳过注释行与空行后取第一行。
PATTERN=$(grep -vE '^[[:space:]]*(#|$)' "$PATTERN_FILE" | head -n 1)
if [ -z "$PATTERN" ]; then
    echo "敏感词真源 $PATTERN_FILE 没有有效的 pattern 行" >&2
    exit 1
fi

if [ "$1" = "-" ]; then
    files=$(cat)
else
    files=$(git ls-files)
fi

# 后缀判断用 awk 内联字面量：走 -v 传参时 awk 会先处理一遍转义，`\.` 会被吞成 `.`。
hits=$(printf '%s\n' "$files" | awk -v pat="$PATTERN" '
    /^docs\// { print; next }
    /\.(md|txt|rst|pdf|docx)$/ && toupper($0) ~ pat { print }
')

if [ -n "$hits" ]; then
    echo "发现内部审计/分析类文档被纳入版本管理："
    printf '%s\n' "$hits" | sed 's/^/  /'
    echo "这类文件含逐条缺陷与可利用路径，不得入库（披露方式见 SECURITY.md）。"
    echo "如需长期保存请放仓库外；如需公开请改写为按能力分组的说明（CHANGELOG.md 的写法）。"
    exit 1
fi

echo "内部文档入库检查通过"
