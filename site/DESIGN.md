---
name: "Stripe × Bento 杂交"
description: "DWZ 宣传站杂交设计规范：Stripe（白底/有机渐变/300 细字重/pill 主 CTA/深海军蓝产品面板）+ Bento（奶油画布/桃粉浅蓝模块卡片/8px 圆角/12 栏网格）。来源与 token 均取自百智云模板库详情页。"
language: zh-CN
sources:
  stripe: "https://baizhi.cloud/landing/design-prompt/detail/stripe"
  bento: "https://baizhi.cloud/landing/design-prompt/detail/bento"
---

## 杂交决策

- **页面骨架与 Hero**：Stripe —— 奶油/白画布、hero 有机渐变 mesh（占上三分之一）、display 300 细字重、tab 数字。
- **功能展示**：Bento —— 12 栏网格 + 不同跨度圆角卡片（桃粉/浅蓝/白/奶油表面），每卡单一任务。
- **主 CTA**：Stripe indigo filled **pill**（全页仅一个）；其余按钮为 Bento 8px 圆角次级样式。
- **产品面板**：Stripe DARK APP TRACK —— 深海军蓝 `#0d253d` 表面承载短链列表示意。
- **节间节奏**：Stripe CREAM BAND —— 暖色 feature band 穿插在蓝白节奏之间。

## Colors（源自两模板页实际展示的 swatch）

### Stripe 主色板
- Indigo `#533afd`（主 CTA / 链接 / 渐变主调）
- Indigo Deep `#4434d4`（渐变深端 / 主 CTA hover）
- Brand Navy `#1c1e54`
- Ruby `#ea2261`（少量点缀：状态、重点标签）
- Magenta `#f96bee` / Lemon `#9b6829`（渐变 mesh 点缀，不作为正文色）
- Ink Navy `#0d253d`（正文主色 / 深色产品面板）
- Ink Mute `#64748d`（次级文字）
- Canvas `#ffffff` / Canvas Soft `#f6f9fc` / Canvas Cream `#f5e9d4`
- Hairline `#e3e8ee`
- Gradient mesh：cream / orange / lavender / indigo / ruby 有机混融

### Bento 模块表面
- Peach `#fad4c0`（高优先级卡片表面，配深色文字）
- Muted Blue `#80a1c1`（次级模块 / 数据卡片，配深色文字 AA）
- Cream `#fff5e6`（页面画布 / 柔和分区）
- White `#ffffff`（主要内容卡片）
- Ink `#111827`（标题 / 正文 / 边框 / 高对比主行动）
- Success `#16a34a` / Warning `#d97706` / Danger `#dc2626`（仅语义状态）

## Typography

- 家族：Inter（display 与 body 同族）+ JetBrains Mono（label-caps 与代码）
- 大标题（Stripe display）：Inter 300 细字重 · 负字距 · 56px→48px，编辑感与空气感
- 卡片标题（Bento heading-card）：Inter 600 · -1.5%
- 正文（Bento body-default）：Inter 400 · 14px · 1.5
- label-caps（Bento）：JetBrains Mono 600 · 12px · **+8% 正字距**（大写标签）
- 数字一律 **tabular figures**（Stripe 规则）

## Shape & Elevation

- 主 CTA：**pill**（999px 圆角）—— Stripe 规则；全页仅一个 indigo filled pill
- 次级按钮：白底 + 1px 深色边框，**8px 圆角**（Bento button-secondary）
- 卡片：**8px 圆角**（Bento）；小控件 4px
- 层级：Level 0 奶油画布 → Level 1 白/桃粉/浅蓝模块 → Level 2 1px 细边框；**不依赖厚阴影**
- 产品面板：深海军蓝 `#0d253d` 圆角（16px），内嵌白/灰文字

## 组件

- **button-primary**：indigo `#533afd` 填充、白字、pill；hover → `#4434d4`
- **button-secondary**：白底、1px 深色边框、8px 圆角、深色文字
- **status-badge**：成功 `#16a34a` / 待审 `#d97706` / 过期 `#dc2626`，带文字标签
- **feature-card**（Bento）：Peach / Muted Blue / White 表面，8px 圆角，标题 600 + 正文 400 + mono 标签
- **app-panel**（Stripe DARK APP TRACK）：深海军蓝 `#0d253d`，内衬短链列表行（mono 短码 + tab 数字）
- **arch-panel**：深海军蓝，等宽 ASCII 架构图
- **code-block**：白底 + 1px hairline，终端式窗口标题

## Layout（Bento 网格 + Stripe 节奏）

- 桌面 ≥1100px：12 栏网格，卡片跨度不等（大卡 6 栏 / 中卡 4 栏 / 小卡 4 栏），间距 16px
- 平板 768–1099px：2 栏
- 移动 <768px：单列，触控目标 ≥44px
- 最大内容宽 1280px；section 间距 96px
- 每卡只承担一个任务；不同跨度表达优先级

## 使用边界（合取两模板）

**应该**
- hero 上三分之一要有有机渐变 mesh（cream/indigo/ruby 混融，低饱和克制）
- 每区块只放一个 indigo pill CTA；其他动作用次级按钮或链接
- 功能卡用不同跨度与桃粉/浅蓝表面对比建立层级
- 交互状态同时用颜色 + 文字 + focus-visible
- 数字用 tabular figures
- 深海军蓝产品面板作为每个核心区块的主角

**不要**
- 不要引入渐变停靠点之外的强调色；indigo 不用于正文
- 不要所有卡片同尺寸；不要一张卡混多任务
- 不要厚阴影 / 发光 / 玻璃拟态
- 不要在桃粉/浅蓝表面用低对比文字
- 不要省略 tabular figure
