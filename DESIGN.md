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

---

## 加载态规范（骨架屏 / 遮罩 / 静默刷新）

> 实现载体：`frontend/src/components/TableSkeleton.vue`、`frontend/src/components/CardSkeleton.vue`、
> `frontend/src/styles/index.scss`（`.sk-block` / `.dwz-silent*`）。

### 背景与目标

改造前：列表与卡片一律使用 `v-loading` 整块遮罩，首屏是"空白 + 转圈"，数据到达后布局整体跳动（CLS 明显）；
监控页 30s 轮询每次都用遮罩覆盖整表后重绘，形成**周期性闪烁**。

目标：首屏用骨架屏稳定布局；二次加载与轮询用局部指示，不遮罩内容、不阻塞交互。

### 设计 Token

| Token | 亮色 | 暗色 | 用途 |
| --- | --- | --- | --- |
| `--dwz-skeleton-bg` | `#e3eaed` | `#1d2d34` | 骨架占位块底色 |
| `--dwz-skeleton-bg-strong` | `#d5e0e4` | `#24363d` | 强调占位块（可选） |
| `--dwz-skeleton-shine` | `rgba(255,255,255,.65)` | `rgba(255,255,255,.07)` | 单向微光 |
| `--dwz-skeleton-alt` | `#fafcfc` | `#16262d` | 骨架斑马纹 |

- 占位块圆角 `4px`，与 `.sk-block` 一致；微光周期 `1.35s ease-in-out`。
- 微光以 `background-position` 实现，**只改变绘制不改变布局**，因此数据到达时不会产生位移。

### 判定规则：何时用骨架屏 / 何时用 loading 遮罩 / 何时静默

1. **首屏加载（该区块尚无任何数据）→ 骨架屏。**
   用在容器内部原位渲染，**禁止**同时叠加 `v-loading` 遮罩（避免遮罩层级叠加与点击穿透）。
2. **二次加载（已有数据，仅内容替换）→ 静默刷新。**
   翻页、排序、筛选、手动刷新、轮询一律默认静默：保留上一帧数据，顶部显示 2px 进度条（`.dwz-silent__bar`），数字类字段用 `.dwz-silent-value.is-refreshing` 淡入。
   理由：已有数据时遮罩会让用户看到"闪白"，而等待期间点击又会被遮罩吞掉。
3. **写操作（提交中）→ `v-loading` 遮罩或按钮 `loading`。**
   表单提交、弹窗内操作、删除确认等**必须阻断重复提交**，此时遮罩是正确选择。
4. **图表 / 无稳定尺寸的容器 → `v-loading` 遮罩。**
   图表高度依赖数据，骨架无法保证同尺寸，强行骨架反而制造跳动。

一句话：**空白首屏用骨架，有内容用静默，要阻断用遮罩。**

### 必须遵守的实现约束

- **尺寸对齐**：骨架行数必须等于当前分页 `page-size`（上限 20 行）；骨架列宽必须与真实列 `width`/`min-width` 对齐（列容器用 `minmax()` 表达 `min-width`）。
  不应出现"骨架 8 行 → 真实数据 20 行"这类高度突变。
- **切换方式**：骨架与真实内容之间用 `v-if` / `v-show` 切换，切换前后容器高度变化必须为 0。
- **不确定态表达**：骨架容器必须带 `role="status"`、`aria-busy="true"`、`aria-label`，并提供 `.sk-sr` 屏幕阅读器文案。
- **禁止遮罩叠加**：不得在同一容器上同时出现骨架与 `v-loading`；嵌套遮罩会导致用户误点下层控件。
- **加载中禁用交互**：分页器在加载中必须 `:disabled="isBusy"`（`isBusy = loading || refreshing`），并在容器上以 `.is-loading { pointer-events: none }` 兜底。
- **轮询必须清理定时器**：`setInterval` 需在 `onBeforeUnmount` 中 `clearInterval`；并加在途请求守卫，防止慢请求叠加（本仓库采用模块内 `inFlight` Promise）。
- **轮询失败不清空数据**：静默刷新失败时保留上一帧数据，仅提示错误，避免整页闪空。
- **动效降级**：`@media (prefers-reduced-motion: reduce)` 下必须关闭微光与进度条动画，改为静态占位块与静态进度条，信息不丢失。

### 组件 API

```vue
<!-- 表格骨架 -->
<TableSkeleton :rows="20" :widths="['44px', 'minmax(140px,1fr)', '90px']" :header="true" />

<!-- 卡片骨架 -->
<CardSkeleton :lines="3" title-width="84px" :icon="true" />
```

### 迁移指引（既有 `v-loading` 列表）

1. 新增 `refreshing` / `initialized` 两个状态，`showSkeleton = loading && !initialized`。
2. `loadData(silent = initialized.value)`：`silent` 时置 `refreshing` 而非 `loading`；**必须在 `catch` 中判断 `silent`**，静默失败不得清空 `rows`（`loadData` 会被分页器直接调用，禁止用可选参数接收页码）。
3. 分页器事件改为 `@current-change="() => loadData()"`，避免事件对象被当作 `silent` 传入。
4. 模板中 `v-loading` 换成 `TableSkeleton v-if="showSkeleton"` + `el-table v-show="!showSkeleton"`，并加 `.dwz-silent__bar`。
5. `onMounted(() => loadData(false))` 保证首屏走骨架分支。

### 反模式

- ❌ 首屏用 `v-loading` 全屏遮罩，加载完成再整体撑开（CLS）。
- ❌ 骨架行数写死 8 行而分页是 20 条。
- ❌ 轮询时调用非静默 `loadData()`，导致整表周期性闪烁。
- ❌ 静默刷新失败时清空列表，用户看到"闪一下空了"。
- ❌ 在骨架容器上再套一层 `v-loading`。

### QA 清单

- [ ] 首屏（清缓存强刷）看到骨架，且骨架高度 = 数据到达后高度，Network 慢速 3G 下无跳动。
- [ ] 翻页 / 排序 / 筛选不出现整块遮罩闪白，仅顶部进度条。
- [ ] 监控页停留 90s（≥3 次轮询），定时任务表无整表闪烁，数字变化以淡入呈现。
- [ ] 加载中快速点击分页器与操作按钮，无重复请求、无误触。
- [ ] 系统开启"减少动态效果"后，微光与进度条停止动画，占位块仍可见。
- [ ] 暗色模式下骨架对比度可辨（占位块与卡片底有明显区分）。
- [ ] `npm run build` 通过。
