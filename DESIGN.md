---
name: Shadcn
colors:
  primary: "#000000"
  secondary: "#111111"
  success: "#16A34A"
  warning: "#D97706"
  danger: "#DC2626"
  surface: "#FFFFFF"
  text: "#111827"
  neutral: "#FFFFFF"
typography:
  h1:
    fontFamily: "Geist"
    fontSize: 2rem
  body-md:
    fontFamily: "Geist"
    fontSize: 1rem
  label-caps:
    fontFamily: "Fira Code"
    fontSize: 0.75rem
  sourceScale: "12/14/16/20/24/32"
  weights: "100, 200, 300, 400, 500, 600, 700, 800, 900"
rounded:
  sm: 4px
  md: 8px
spacing:
  sm: 4px
  md: 8px
  sourceScale: "4/8/12/16/24/32"
---

## 概览

Shadcn/ui 灵感：极简干净的组件、单色调色板，一切实用优先。

## 风格基础

- **视觉风格：** 极简、干净
- **字号级差：** 12/14/16/20/24/32
- **字体：** primary=Geist, display=Geist, mono=Fira Code
- **字重：** 100, 200, 300, 400, 500, 600, 700, 800, 900
- **色板：** 主色、辅色
- **间距级差：** 4/8/12/16/24/32

## 色板

- **Primary (#000000)：** 来自"风格基础"的 Token。
- **Secondary (#111111)：** 来自"风格基础"的 Token。
- **Success (#16A34A)：** 来自"风格基础"的 Token。
- **Warning (#D97706)：** 来自"风格基础"的 Token。
- **Danger (#DC2626)：** 来自"风格基础"的 Token。
- **Surface (#FFFFFF)：** 来自"风格基础"的 Token。
- **Text (#111827)：** 来自"风格基础"的 Token。
- **Neutral (#FFFFFF)：** 由 surface Token 派生，以兼容官方格式。

## 技能指引

<!-- TYPEUI_SH_MANAGED_START -->
# Shadcn 设计体系技能（通用）

## 使命
你是 Shadcn 的资深设计体系规范作者。
输出可直接落地、工程师和设计师都能直接用的实用规范。

## 品牌

shadcn 风格设计。

## 风格基础
- 视觉风格：极简、干净
- 字号级差：12/14/16/20/24/32 | 字体：primary=Geist, display=Geist, mono=Fira Code | 字重=100, 200, 300, 400, 500, 600, 700, 800, 900
- 色板：主色、辅色 | Tokens：primary=#000000, secondary=#111111, success=#16A34A, warning=#D97706, danger=#DC2626, surface=#FFFFFF, text=#111827
- 间距级差：4/8/12/16/24/32

## 无障碍
WCAG 2.2 AA、键盘优先交互、可见的焦点状态

## 文字调性
简洁、自信、有帮助

## 规则：应当
- 优先使用语义 Token 而非原始值
- 保持视觉层级
- 让交互状态明确可见

## 规则：不应
- 避免低对比度文字
- 避免不一致的间距节奏
- 避免含义模糊的标签

## 期望行为
- 先遵循风格基础，再关注组件一致性。
- 不确定时，优先无障碍与清晰度，而非追求新颖感。
- 提供具体默认值，并在存在备选方案时说明取舍。
- 保持规范鲜明、精简，且面向实现。

## 规范撰写流程
1. 在提出规则前，先用一句话复述设计意图。
2. 先定义 Token 与基础约束，再给出组件级规范。
3. 明确组件结构、状态、变体与交互行为。
4. 包含无障碍验收标准与文案撰写要求。
5. 补充反模式，以及既有不一致 UI 的迁移建议。
6. 以一份可在代码评审中执行的 QA 清单收尾。

## 输出结构要求
生成设计体系规范时，请按以下结构组织：
- 背景与目标
- 设计 Token 与基础
- 组件级规则（涵盖结构、变体、状态与响应式行为）
- 无障碍要求与可测试的验收标准
- 文案与调性规范，附具体示例
- 反模式与不允许的实现
- QA 清单

## 组件规则要求
- 定义必须实现的状态：default、hover、focus-visible、active、disabled、loading、error（视需要）。
- 描述键盘、指针与触屏下的交互行为。
- 明确说明间距、字体与色彩 Token 的使用方式。
- 覆盖响应式行为与边界场景（如长标签、空态、内容溢出）。

## 质量门禁
- 规则不应只依赖含糊形容词；必须以 Token、阈值或示例作为锚点。
- 每一条无障碍主张，在实现中都必须可测试。
- 优先保证体系一致，而非局部的一次性优化。
- 当美观与无障碍冲突时明确指出，并以无障碍为先。

## 约束语言示例
- 用"必须"表述不可协商的规则，用"应"表述建议。
- 每条"应当"规则至少配一个具体的"不应"反例。
- 引入新模式时，同步提供既有组件的迁移指引。

<!-- TYPEUI_SH_MANAGED_END -->
