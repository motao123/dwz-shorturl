# DWZ 短链接系统 · 全方位深度分析报告

> 版本：v1.0 ｜ 日期：2026-08-13 ｜ 分析范围：功能完整性 / UI 视觉 / 交互体验 / 前后台架构
> 分析对象：仓库当前工作区代码（含未提交变更），覆盖 PHP 前台、Go 后台、Vue3 管理端与会员端、数据库与部署配置
> 说明：报告中所有【位置】均为当前工作区 file:line，关键高危结论均经二次代码核验。

---

## 一、项目概览与定位

**项目定位**：面向公众的免费短网址服务平台（部署于 `1.xk7.cn`），提供短链生成、跳转、统计能力，并附带后台管理系统（Go + Vue3）与会员中心（Vue SPA）。

**双栈演进架构**：
- **PHP 旧栈（前台）**：`index.html`（首页）+ `api.php`/`batch.php`（生成 API）+ `do.php`（跳转）+ `stats.php`（统计）+ `member.php`（会员入口）+ `includes/`（核心函数、认证、DB）
- **Go 新栈（后台）**：`backend/`，Gin + GORM + Redis，含管理 API（RBAC）、会员 API（JWT）、公开跳转 `/r/:code`、点击异步队列、Cron 任务、Webhook、违规检测
- **Vue3 双入口**：`frontend/` 管理端（Element Plus + ECharts）与 `frontend/src/member/` 会员端

**已实现功能全景**（核心 + 附属）：

| 端 | 模块 | 已实现能力 |
|---|---|---|
| 前台 | 短链生成 | 单条/批量（≤100）、自定义短码（6-8 位 a-z/0-5）、有效期（永久/1/7/30/365 天）、原子去重、二维码、一键复制 |
| 前台 | 跳转 | 302 跳转、过期 410、禁用 404、点击计数、禁缓存头、SSRF DNS 级校验 |
| 前台 | 统计/SEO | Token 认证统计页、robots.txt、sitemap.php、OG/Twitter Card、JSON-LD、百度统计/GA4/站长验证占位 |
| 前台 | 会员 | 注册/登录/登出、邮箱验证、CSRF、账号锁定、批量生成权限控制 |
| 后台 | 认证权限 | JWT 登录/刷新、RBAC（4 角色 22 权限点）、API Key（SHA-256 哈希）、操作审计 |
| 后台 | 短链管理 | 列表/搜索/筛选/分页/CRUD/批量操作/CSV 导入导出/单链接统计 |
| 后台 | 分析 | 概览卡片、趋势折线图、Top10 柱状图、设备/浏览器/OS 分类、域名池 DNS/SSL 检测 |
| 后台 | 业务管理 | 用户、会员、角色权限、域名池（负载均衡）、违规审核、Webhook 订阅与投递、系统配置、监控 |
| 会员端 | 会员中心 | 我的短链、批量创建、CSV 导入、标题获取、一键续期、单链接统计、二维码 |

**总体结论**：功能面已远超同类型开源短链服务，安全底线扎实（全参数化 SQL、输出转义、SSRF 防护、限流、CSRF），前端设计语言统一且完成度高。**真正的系统风险集中在「PHP/Go 双轨过渡架构」的数据一致性、若干上线即翻车的 schema 漂移、以及运维基础设施（备份/监控）的缺失**。

---

## 二、功能完整性评估

### 2.1 已实现 vs 规划对照

`docs/FEATURE_ROADMAP.md`（2026-07-28 规划稿）中的部分模块已在当前代码中落地：会员体系、域名池、Webhook、违规检测（引擎未接线，见下）、监控页、邮件发送（`service/email.go`）、Cron（过期标记/统计聚合）。**但大量规划项仍未实现**，且已实现模块中存在"空壳"（违规检测引擎零调用、部分统计字段存而不用）。

### 2.2 功能缺口盘点（含场景 / 用户群 / 业务价值）

#### 分析洞察类（目标用户：营销运营、站长本人）

| 缺口 | 应用场景 | 用户群 | 业务价值 | 优先级 |
|---|---|---|---|---|
| GeoIP 地域分布 | 广告投放后看转化地域 | 营销人员、站长 | 支撑投放地域决策，短链平台最核心的增值分析 | 高 |
| Referer 来源归类（搜索引擎/社媒/直接） | 渠道效果归因 | 营销人员 | 判断流量来源质量 | 高 |
| 时段分布热力图 | 判断发布黄金时段 | 自媒体、运营 | 优化推送时间 | 中 |
| 实时点击流（WebSocket/SSE） | 大屏/活动直播看数据 | 站长、活动运营 | 运营现场感、活动效果即时反馈 | 中 |
| 定时报告（邮件/PDF） | 每日/每周数据自动推送 | 站长、企业用户 | 免登录查看，提升粘性 | 低 |

**现状说明**：后台已按 `click_logs` 存 UA 并做设备/浏览器/OS 分类展示，但 `service/stats.go:204` 采用全量 Pluck 内存分类，无地域、无 Referer 归类；`click_logs` 未记录 referer、country 字段。

#### 安全与信任类（目标用户：企业用户、平台运营者）

| 缺口 | 应用场景 | 用户群 | 业务价值 | 优先级 |
|---|---|---|---|---|
| **违规检测引擎接线**（`pkg.CheckURLViolation` 已实现但零调用） | 拦截博彩/钓鱼 URL 分发 | 平台运营者 | 合规底线，防止平台被用于违法分发 | 高 |
| 链接密码保护 | 私有链接限访问 | 企业、付费用户 | 差异化付费点 | 高 |
| 中间确认页（展示目标域名） | 降低信任成本、防恶意跳转 | 全平台 | 提升平台可信度 | 中 |
| IP 白/黑名单（CIDR） | 内部链接仅内网可访问 | 企业 | 企业级安全诉求 | 中 |
| 2FA (TOTP) | 管理员/会员账号保护 | 站长、会员 | 防爆破、防盗号 | 中 |
| Bot/爬虫过滤 | 统计去噪 | 运营 | 数据准确性 | 中 |
| 点击欺诈检测（同 IP 高频标记） | 反作弊 | 运营 | 保护广告主利益 | 低 |

#### 效率与生产力类（目标用户：全部用户）

| 缺口 | 应用场景 | 用户群 | 业务价值 | 优先级 |
|---|---|---|---|---|
| UTM 构建器 | 营销链接参数化 | 营销人员 | 营销刚需，提升创建效率 | 高 |
| 链接标签系统（多标签） | 按活动/渠道归类管理 | 运营、站长 | 管理海量短链的必备能力 | 中 |
| 批量导入 CSV/JSON（管理端） | 从旧系统迁移 | 站长、企业 | 迁移效率 | 中 |
| 回收站（软删除恢复 UI） | 误删恢复 | 站长 | 数据安全感 | 中 |
| 过期预警通知（到期前 24h/7d 邮件） | 重要链接续期 | 会员、企业 | 减少链接失效损失 | 中 |
| 自定义短码迁移（已生成链接补码） | 既有链接想补自定义码被拒绝 | 全部 | 体验补全 | 低 |

#### 商业与品牌类（目标用户：企业客户 / 平台商业化）

| 缺口 | 应用场景 | 用户群 | 业务价值 | 优先级 |
|---|---|---|---|---|
| A/B 测试与链接轮转（多变体分流） | 多落地页对比 | 营销、企业 | 核心付费功能 | 中 |
| 设备/地域/时间条件路由 | iOS→AppStore / 活动期→活动页 | 企业 | 核心付费功能 | 中 |
| 多租户与配额计费（套餐限制短链数/API 调用） | SaaS 化 | 平台方 | 商业化基础 | 低 |
| 用量计量面板 | 套餐用量展示 | 付费用户 | 配合计费 | 低 |
| OpenAPI 文档 + Swagger Playground | 第三方开发者调试 | 开发者 | 降低集成门槛 | 中 |

#### 体验与生态类（目标用户：全部用户）

| 缺口 | 应用场景 | 用户群 | 业务价值 | 优先级 |
|---|---|---|---|---|
| 暗色模式补全（现仅后台部分生效） | 夜间使用 | 全部 | 留存提升 | 中 |
| 多语言 i18n | 海外用户 | 全部 | 用户面扩展 | 低 |
| PWA / 浏览器扩展（右键缩短） | 随手缩短当前页 | 高频用户 | 工具属性增强 | 低 |
| 链接预览卡片（OG 标题/图/描述） | 生成后预览 | 全部 | 增强信任与可用性 | 中 |
| 分享按钮（微信/微博/邮件/Twitter） | 社交分发 | 自媒体 | 传播效率 | 低 |

#### 运维与平台治理类（目标用户：站长本人）

| 缺口 | 应用场景 | 业务价值 | 优先级 |
|---|---|---|---|
| 数据备份管理（mysqldump + 每日 cron + 恢复演练） | 防数据丢失 | **当前完全缺失，最高运维风险** | 高 |
| 监控告警（Prometheus 指标 + 告警规则） | 服务稳定性 | 当前仅静态假 /health | 高 |
| 系统健康面板（DB 连接池/Redis/队列积压/磁盘） | 日常巡检 | 后台 monitor 已有雏形可扩展 | 中 |
| click_logs 分区自动维护（月新增分区） | 防 p_future 分区无限膨胀 | 数据库性能 | 中 |
| 审计日志导出 + 敏感操作告警 | 合规追溯 | 中 |

---

## 三、UI 视觉优化诊断

### 3.1 设计一致性（现状：Token 化不彻底，前后台割裂）

**总体判断**：PHP 首页与 Vue 后台各自都建立了设计 Token（`app.css:1-24` 的 `--ink/--accent`；`index.scss:7-38` 的 `--dwz-petrol/amber`），视觉完成度高于一般项目。但存在三类一致性问题：

1. **【中】色彩体系 Token 化不彻底**：全局 303 处硬编码 `#hex`、70 处 `rgba()`。典型：白色背景 `#fff`（DashboardView.vue:217、AdminLayout.vue:382）、灰色 `#f4f8f8/#f7fafb` 遍布 8+ 文件。主题调整会漏改。
2. **【中】暗色模式名存实亡**：`styles/index.scss:303-335` 只覆盖了 Element Plus 变量和少量卡片，但顶栏（AdminLayout.vue:354 `rgba(255,255,255,0.92)`）、指标卡（DashboardView.vue:217 `#fff`）、所有 `.mini-btn` 仍是浅色硬编码。切换暗色后视觉割裂严重。后台 route 已配置 `el-config-provider`，但**前台 PHP 首页无任何暗色支持**。
3. **【中】前台/后台/会员端三套视觉语言割裂**：PHP 首页极简黑白（app.css）；管理后台墨青+琥珀（`#0e6e75/#f5a623`）；会员端深青渐变登录页（`#10363e`）。用户从首页点进会员中心无任何视觉过渡。
4. **【低】字体外链 Google Fonts**（index.html:9-14）：大陆网络超时静默降级，首屏字体闪变。
5. **【低】圆角/间距未 Token 化**：8/12/14/9/7/10px 混用，与 DESIGN.md 要求的 4/8/12/16/24/32 节奏不一致。

### 3.2 响应式适配（现状：前台完整，后台缺失）

| 端 | 现状 | 问题 | 优先级 |
|---|---|---|---|
| PHP 首页 | ✅ 4 档断点（1024/768/640/480）完整 | 移动端 `.header-nav` 直接 `display:none`（app.css:885），无汉堡菜单替代，功能导航缺失 | 中 |
| 管理后台 | ❌ 全项目仅 5 处 media query | 侧栏固定 224px 只能手动折叠（stores/app.ts:15），<768px 平板内容区被压缩；无移动端抽屉模式 | **高** |
| 管理后台表格 | ❌ | 9 处 `fixed="right"` 操作列在窄屏与内容列重叠/截断，无横向滚动保护 | 中 |
| 会员端 | ⚠️ 部分 | 分页固定每页 20 条无切换；登录卡片在小屏尚可 | 低 |

### 3.3 可访问性（a11y）（现状：前台完整，后台缺失）

| 问题 | 位置 | 说明 | 优先级 |
|---|---|---|---|
| 后台全项目 **0 个 `aria-label`** | 全 `src/` | 图标按钮（复制/编辑/删除/统计）为 `<button>` 内嵌纯 `<el-icon>`，仅靠不可靠的 `title`，读屏用户完全无法操作 | **高** |
| 自定义按钮无 `:focus-visible` | AdminLayout.vue:375-392、各页 `.mini-btn` | 键盘 Tab 无可见焦点环 | 中 |
| 小字号对比度不足（WCAG AA） | AdminLayout.vue:263（9px `#5e848b`）、LoginView.vue:415（10px `#9cb1b6`）、DashboardView.vue:261（9.5px） | 9-10px 低对比小字不达 4.5:1 | 中 |
| 图表无读屏降级 | DashboardView.vue:196、StatsView.vue:259 | ECharts canvas 对读屏不可见，无数据摘要 | 低 |
| PHP 首页批量结果无焦点管理 | app.js:284 | `scrollIntoView` 后焦点不移入结果区 | 低 |

### 3.4 视觉层次

- **【低】面包屑是伪层级**：13 个一级菜单平铺侧栏（AdminLayout.vue:63-77），无二级分组，纵向滚动。
- **【低】无骨架屏**：19 处 `v-loading` 遮罩，0 处 `el-skeleton`；大数据表格首屏体验一般。
- **【低】图表主题与整体不一致**：Dashboard/Stats 两份 ECharts option 代码重复 90%，颜色硬编码 `#0e6e75/#f5a623/#dfe7ea`，暗色下仍是浅色坐标系。
- **【低】`.mini-btn` 等样式在 10 个文件中各复制一份**（ApiKeyList.vue:470、DomainList.vue:478、MemberList.vue:269…），微调不一致。

### 3.5 需要美化/调整/重构的 UI 模块清单

1. 【高】管理后台全量响应式适配（侧栏折叠/抽屉 + 表格横向保护）
2. 【高】后台图标按钮补齐 aria-label + 全局 focus-visible
3. 【中】暗色模式系统性补全（替换全部硬编码浅色背景为 `var(--dwz-*)`）
4. 【中】会员端视觉向首页设计语言对齐（去掉深色渐变，统一品牌色）
5. 【中】前台首页移动端导航抽屉 + 在线状态真实化（假"在线"呼吸灯改为真实 /health 探测）
6. 【中】设计 Token 补全（灰度语义 Token、圆角/间距 Token、stylelint 禁硬编码色）
7. 【低】ECharts 主题抽取 + 暗色适配；骨架屏替换遮罩；菜单分组
8. 【低】字体自托管，去 Google Fonts 外链

---

## 四、交互体验改进排查

### 4.1 前台（PHP 首页）全链路痛点

| 场景 | 痛点 | 优化方案 | 优先级 |
|---|---|---|---|
| 连续生成 | 单条生成成功后必须关弹窗才能再生成，无"再生成一条"路径 | 弹窗内加"再生成一条"按钮（关窗并聚焦输入框） | 中 |
| 二维码 | 默认不渲染，需点"显示二维码"才惰性生成；移动端扫码是第一诉求 | 打开弹窗即渲染；库加载失败用短文案 + 复制按钮 | 中 |
| 批量超限 | 超过 100 条仅显示"100/100"红色，用户不知超了多少 | 显示"N/100（超出 M 条）"并禁用提交 | 低 |
| 邮箱未验证 | 未验证邮箱用户点批量生成才报 403，事前无感知、报错无引导入口 | member.php 响应带 `email_verified`，前端提前拦截 + "去验证"链接 | 低 |
| 自定义码清洗 | 输入大写被静默改小写（app.js:355），与提示"仅限小写"冲突易困惑 | 保留原输入 + aria-invalid 提示，提交时规范化 | 低 |
| 限流重试 | 429 提示无 Retry-After 冷却时间 | 读取响应头提示"请在 X 秒后重试" | 低 |
| 批量结果 | 无成功/失败汇总，长列表无摘要 | 顶部成功/失败计数 + 失败项高亮 + 一键重试 | 低 |

### 4.2 管理后台交互痛点

| 场景 | 痛点 | 优化方案 | 优先级 |
|---|---|---|---|
| **编辑短链** | **打开编辑弹窗即把有效期回填为 null（永久）**，用户只改标题保存也会把原 30 天/1 年有效期清成永久；编辑已过期链接时 status 会被重置为"启用" | 编辑模式回填剩余天数；提交时区分"未修改/设为永久"；保留原 status | **高（数据丢失级）** |
| 批量修改 | "不修改=0 / 永久=-1"哨兵值与新建表单语义不一致，极易选错 | 改为"状态/有效期"两组明确 radio | 中 |
| 路由越权 | 路由守卫不校验 `meta.perm`，低权限用户直接改 URL 可进入无权限页面（虽接口会拒绝） | 守卫中校验 `to.meta.perm` + 新增 403 兜底页 | **高** |
| 新建用户 | 两段式提交（先建用户再分配角色），角色分配失败时用户已创建 | 合并为单接口或失败回滚 | 中 |
| CSV 导出 | 未 append 到 body 且不 `revokeObjectURL`，泄漏 blob URL；member 端实现完整可复用 | 统一用 member 端实现 | 中 |
| 监控页 | 无自动刷新（MonitorView.vue:35 onMounted 一次） | 30s 轮询 + 卸载清理 | 低 |
| 导入体验 | 无模板下载；错误只展示前 20 条不可滚动 | 提供模板下载 + 错误明细可滚动可复制 | 低 |
| 登录 | 无验证码（API 定义有 captcha 字段但表单无），对暴力破解仅靠 IP 限流 | 补齐验证码 | 中 |
| 域名批量按钮 | 启用/停用都用 `Refresh` 图标 | 启用 `CircleCheck`、停用 `CircleClose` | 低 |

### 4.3 会员端交互痛点

| 场景 | 痛点 | 优化方案 | 优先级 |
|---|---|---|---|
| **二维码** | `member.html:10` 引用 `/assets/qrcode.min.js`（绝对路径），项目无 `public/` 目录，生产必然 404，点"二维码"弹空框 | qrcode 改 npm 依赖动态 import，删除外链 | **高（功能失效）** |
| 复制 | 无降级（`navigator.clipboard` 直用，无 execCommand fallback） | 复用 `utils/clipboard.ts` | 中 |
| 注册 | 无确认密码、无验证码 | 补 confirmPassword + 验证码 | 中 |
| 巨型组件 | `DashboardView.vue` 1126 行塞下全部功能 | 拆分为列表/创建/统计/批量子组件 | 中 |
| 分页 | 固定每页 20 条无切换 | 补 `layout="total, sizes, prev, pager, next"` | 低 |
| Token | 无自动刷新，过期后直接抛错 | 复用管理端 axios 刷新逻辑或 401 重放 | 中 |

### 4.4 全链路体验总结

**用户核心路径**：首页输入 URL → 生成 → 复制/扫码 →（可选）会员中心管理 →（可选）后台分析。
**三大体验断层**：① 生成后无"继续生成"；② 会员中心入口与首页视觉/交互割裂且二维码功能失效；③ 后台编辑短链存在数据丢失级交互缺陷。破坏性操作确认、加载/错误/空态反馈（14 处 ElMessageBox 确认 + 全页 ElMessage）是做得好的部分。

---

## 五、前后台架构优化分析

### 5.1 数据层架构（最高风险域）

**总体问题：PHP/Go 双轨双写、无事务、无对账，且存在三处 schema 漂移（上线即翻车）。**

| # | 问题 | 严重度 | 位置 | 影响 |
|---|---|---|---|---|
| D1 | **`short_urls` 缺 `member_id` 列**：DDL 无此列，但 GORM 模型（models.go:116）与 PHP 双写 INSERT（function.php:347）都引用 → PHP 创建的短链**全部**双写失败（`if (!$stmt) return;` 静默吞错），后台/会员中心看不到 PHP 数据，Go 的 `WHERE member_id=?` 查询报错 | **高** | schema.sql:77-103 / models.go:116 / function.php:347 | PHP 前台与 Go 后台数据彻底割裂 |
| D2 | **`wjoy_log` 缺 `status` 列**：install.sql:5-18 建表无 status，但 `do.php:11` SELECT status、Go `wjoy_log.go:67` UPDATE status → 全新安装后所有跳转报 unknown column 500，后台禁用/恢复在公共库不生效 | **高** | install.sql:5-18 / do.php:11 / wjoy_log.go:63-67 | 全新安装即故障 |
| D3 | **双向 best-effort 双写无补偿**：PHP 写 wjoy_log 主 + 同步 short_urls；Go 写 short_urls 主 + 同步 wjoy_log。双向均 `_ =` 吞错、无事务、无对账任务，任一 DB 抖动即永久分叉 | **高** | function.php:341-352 / short_url.go:158,314 / wjoy_log.go:27-38 | 数据一致性无保证 |
| D4 | **点击计数三处口径并存**：PHP 跳转 `wjoy_log.clicks+1` 且 `short_urls.clicks+1`（do.php:46、function.php:395）；Go 跳转只 `short_urls.clicks+1`。同一短链 stats.php 与后台显示不同点击数 | **高** | do.php:46 / function.php:395 / redirect.go:182 / stats.php:122 | 统计不可信 |
| D5 | **软删除 + 唯一索引冲突**：`uk_url_hash`/`uk_uid` 唯一索引在软删后仍占位 → 已删短链永远无法重建（重试 12 次后报 ErrCodeCollision） | **高** | schema.sql:77-103 / short_url.go:746-753 | 正常业务受阻 |
| D6 | **click_logs 分区无自动扩展**：预建到 p202612 + p_future，2027-01 起全进 p_future 单分区，分区表退化为大表 | 中 | schema.sql:129-137 / cron.go:132-146 | 查询/清理变慢 |
| D7 | **seed.sql 与 schema.sql 冲突**：两文件都 INSERT admin 用户（schema.sql:224 admin123；seed.sql:65 REPLACE_WITH…），docker-entrypoint 按序执行时 seed 撞 uk_username 中断 → system_configs 等后续语句永不执行 | 中 | schema.sql:224 / seed.sql:61-66 | 配置初始化失效 |
| D8 | **无 schema 版本管理**：migrate_wjoy_log.sql 与 legacy_schema.php 双套体系、库名硬编码 `USE dwz_admin`、幂等性不足 | 中 | migrate_wjoy_log.sql:5 / migrations/legacy_schema.php | 部署人员无法判断迁移状态 |
| D9 | api_keys 无盐 SHA-256；webhook.secret 明文；时间类型公共库 TIMESTAMP（2038 限制）vs 管理库 DATETIME(3) | 低 | schema.sql:170-179 / add_webhooks.sql:7 | 安全/一致性隐患 |

**整改方向（按序）**：
1. 补迁移：`ALTER TABLE short_urls ADD COLUMN member_id`；`ALTER TABLE wjoy_log ADD COLUMN status`，并同步三处基线 SQL（install.sql / init_public_db.sql / schema.sql）。
2. 收敛为单一写路径：以 `short_urls` 为 truth，PHP 改读 `wjoy_compat` 视图（migrate_wjoy_log.sql:38 已建但未使用），wjoy_log 降级为跳转最小表；新增定时对账任务补差。
3. 统一点击事件源：所有跳转只写 `click_logs`（单一明细），`clicks` 由聚合任务重算。
4. 软删除冲突：唯一索引改 `(url_hash, deleted_at)` 或删除时硬删/复活软删行。
5. 建立 `schema_migrations` 版本表 + click_logs 月度分区维护任务 + seed 改 `ON DUPLICATE KEY UPDATE`。

### 5.2 跳转链路与统计（三套入口并存）

| # | 问题 | 严重度 | 位置 | 影响 |
|---|---|---|---|---|
| R1 | **deploy nginx 跳转 proxy 与 Go 路由不匹配**：`proxy_pass http://backend_api/redirect$uri;`（$uri 带前导 /，生成 `/redirect/<code>`），而 Go 注册路由是 `/r/:code`（router.go:41）→ **Docker 部署下短链跳转必然 404** | **高** | deploy/nginx/default.conf:59 / router.go:41 | 生产部署直接故障 |
| R2 | 跳转入口三套并存：Apache/nginx 根部署 rewrite 到 PHP do.php；Docker nginx proxy 到 Go `/r/`；Go 内部 `/r/:code`。行为不一（禁用返回：PHP 404 vs Go 410） | **高** | .htaccess:29 / nginx.example.conf:66 / default.conf:58-65 | 维护混乱、统计分散 |
| R3 | 点击日志异步队列 goroutine 治理：webhook 每次 flush 为每条 uid×每订阅 spawn goroutine（无界、无 recover），高流量 OOM 风险 | 中 | redirect.go:127-184 / webhook.go:82-113 | 生产稳定性 |
| R4 | ClickQueue 实现放 handler 层、`resolveFromLegacy` 每次未命中都查 `information_schema`（可被枚举短码轰炸） | 中 | redirect.go:19-186 / short_url.go:617-655 | 性能/职责问题 |
| R5 | 跳转错误响应用 `c.String()` 裸文本 + `contains(msg,"expired")` 字符串匹配定状态码，非类型化错误 | 低 | redirect.go:219-240 | 维护脆弱 |

**整改方向**：统一跳转入口收敛到 Go `/r/:code`；修正 deploy nginx 为 `proxy_pass http://backend_api/r$uri;`；webhook 投递改有界 worker pool + 复用 webhook_deliveries 持久化队列；wjoy_log 表存在性启动时检测缓存。

### 5.3 安全漏洞清单（按优先级）

| # | 问题 | 严重度 | 位置 | 影响 |
|---|---|---|---|---|
| S1 | **JWT 密钥硬编码默认值** `change-me-to-random-32-bytes!!`，docker-compose 兜底 `change-me-in-production-32bytes!`，生产可离线伪造 super_admin JWT 直接接管 | **高** | config.go:115 / docker-compose.yml:26 / config.example.yaml:29 | 全系统最高危 |
| S2 | **登录限流可被 X-Forwarded-For 伪造绕过**：`TrustedProxies` 配置（config.go:65）从未被使用（无 `SetTrustedProxies`），gin 默认信任所有代理 → 每换一个 XFF 头即绕开 admin-login 10 次/分钟暴力破解防护 | **高** | middleware/rate_limit.go:22 / config.go:65 | 管理员账号爆破 |
| S3 | **违规检测引擎零接线**：`CheckURLViolation`（pkg/violation.go:62）实现完整但无调用点，`violation_reviews` 表无写入方，公开 API 可无障碍生成博彩/钓鱼短链 | **高** | pkg/violation.go:62 / short_url.go（无调用） | 平台合规风险 |
| S4 | **SSRF 缺口**：`GET /short-urls/:id/check`（CheckLink）服务端 HEAD 请求无 SSRF 校验；FetchTitle（member_api.go:356）与 Webhook 目标（webhook.go:134）均存在 SSRF/DNS rebinding 窗口 | **高/中** | handler/short_url.go:71-86 / member_api.go:356-389 / webhook.go:134-145 | 内网探测、云元数据 |
| S5 | **登出空实现 + refresh 不轮换 + access 可当 refresh 用**：`/auth/logout` 注释"for now simply return success"；token 无 `token_type` claim；泄露 token 在 2h+7d 内始终有效 | 中 | handler/auth.go:61-65 / service/auth.go:97-138 | 安全事件响应能力弱 |
| S6 | CSV 导出无公式注入转义（`= + - @` 开头） | 中 | short_url.go:476-494 / member_api.go:212-233 | Excel 公式执行 |
| S7 | `GET /admin/api/configs` 返回 system_configs 全部记录（含敏感 is_public=0 项） | 中 | handler/config.go:31-39 | 敏感配置泄露 |
| S8 | CORS 白名单未命中直接 403（非浏览器客户端被硬拦），且 Allow-Headers 缺 `X-Member-Token` | 低 | middleware/cors.go:33-38 | 公开 API 兼容性 |
| S9 | PHP 文件限流 fail-open（IO 异常即放行）且文件无上限可被填盘 | 中 | function.php:146-162 | 限流可绕过 |
| S10 | PHP 单条生成无 CSRF（批量有）；config.php 防护完全依赖部署层；`includes/member.php`/`txprotect.php` 两个死文件含高危逻辑未清理 | 中/低 | api.php:9-13 / function.php / includes/ | 一致性与攻击面 |

### 5.4 性能瓶颈

| # | 问题 | 严重度 | 位置 |
|---|---|---|---|
| P1 | redirect persist 对每去重 short_url_id 逐条 `UPDATE clicks+1`（批量 100 条 = 100 次 UPDATE） | 中 | redirect.go:178-184 |
| P2 | 批量创建/导入/续期全串行逐条（每 URL 2-3 次 DB 操作），100 条 = 300+ 次串行查询 | 中 | short_url.go:211-260 / member_api.go:268-302 |
| P3 | LinkStats 全量 `Pluck(user_agent)` 进内存分类，无 LIMIT | 低 | stats.go:204-210 |
| P4 | RBAC 每次请求三表 JOIN 查权限无缓存；API Key 每请求写 last_used_at | 低 | rbac.go:56-80 / apikey.go:64 |
| P5 | PHP 跳转在 fastcgi_finish_request 后仍串行 2 条 admin 写 + webhook 查询 + 最多 3 次 curl（每次退避 1s/2s），高并发拖垮 worker 池 | 中 | do.php:42-58 / function.php:380-461 |
| P6 | 固定窗口限流边界 2 倍突发；Redis 故障 fail-open | 低 | pkg/ratelimit.go:56-73 |
| P7 | Element Plus 全量引入（1.07MB chunk + 367KB CSS）+ echarts 521KB；`app.use(ElementPlus)` 使按需配置失效 | 中 | main.ts:3,20 / vite.config.ts:36 |
| P8 | 无请求取消（AbortController），快速切页/筛选有竞态 | 低 | api/request.ts |

### 5.5 部署与运维

| # | 问题 | 严重度 | 位置 |
|---|---|---|---|
| O1 | **无任何备份机制**（无 mysqldump 脚本、deploy.sh 不备份、无异地/恢复演练） | **高** | 全仓库 |
| O2 | **无监控告警**（无 /metrics、无 Prometheus/Grafana、nginx /health 为静态 `return 200 'ok'`） | **高** | deploy/nginx/default.conf:74-78 |
| O3 | docker-compose 内嵌默认弱凭据（`dwz_secret_2026`/`root_secret_2026`），MySQL/Redis 端口直接暴露宿主机（3306:3306、6379:6379） | **高** | docker-compose.yml:17-26,62,82 |
| O4 | Redis 设密码后 healthcheck 必失败（`redis-cli ping` 不带 -a）；镜像内配置 root 空密码；Dockerfile 以 root 运行、基础镜像偏旧 | 中 | docker-compose.yml:85-90 / Dockerfile:18-33 |
| O5 | deploy.sh 用 sshpass 明文密码 + StrictHostKeyChecking=no；`rm -rf admin/*` 无 dist 校验；.bak 仅一份；无 DB 迁移/回滚步骤 | 中 | deploy.sh:26,33,52-65 |
| O6 | MySQL 无资源限制；日志无轮转无采集；nginx 无 TLS/安全响应头/limit_req | 中 | docker-compose.yml:53-76 |
| O7 | 单一应用账号跨两库全权限（`GRANT ALL`）违反最小权限；默认 admin/admin123 上线 | 中 | init_public_db.sql:8 / seed.sql |
| O8 | .gitignore 未覆盖 `backend/dist/` 与 `dwz-admin-linux` 二进制入库风险 | 低 | .gitignore:7 |

### 5.6 代码质量与可维护性

| # | 问题 | 严重度 | 位置 |
|---|---|---|---|
| C1 | 短链创建三份复制实现（Create/createWithTitle/CreateLink），统计三份、CSV 导出两份、`sameUint64Ptr` 两处 | 中 | short_url.go:84-331 / stats.go:161-227 / member_api.go:616-698 |
| C2 | wjoy_log 双写等大量 `_ =` 吞错（auditSvc.Log、UpdateLastLogin、SetStatus 等），失败无告警 | 中 | short_url.go:160,316 / wjoy_log.go / auth.go:82 |
| C3 | 后台 goroutine（webhook/cron/CheckDomain）均无 panic recover；log.Printf 混用 | 中 | webhook.go:91 / cron.go:65-92 / domain.go:98-100 |
| C4 | 前端 7 个列表页同构重复（分页 query/loadData/Array.isArray 兼容分支各抄一份）；`.mini-btn` 10 份副本 | 中 | 各 List.vue |
| C5 | 错误提示中英文混杂（"短链已过期" vs "Not Found"），无 i18n | 低 | redirect.go:229-235 / rate_limit.go:30 |
| C6 | member 端 `Promise<any>` 类型泄漏；member.html 外链脚本；死文件 includes/member.php、txprotect.php | 低 | member/api.ts:24-94 / includes/ |
| C7 | 权限绑定错误：`violations PUT/DELETE` 用 `audit/read` 权限即可操作写接口（水平越权） | 中 | router.go:164-165 |

---

## 六、优先级整改清单（可直接落地）

> P0 = 立即处理（上线即故障 / 高危安全）；P1 = 1-2 周；P2 = 3-4 周；P3 = 后续迭代。

### P0 紧急（8 项）

| # | 问题 | 位置 | 整改建议 | 预期效果 |
|---|---|---|---|---|
| 1 | short_urls 缺 member_id 列 | schema.sql:77-103 + models.go:116 + function.php:347 | 补 `ALTER TABLE short_urls ADD COLUMN member_id` 迁移并同步三处基线 SQL | PHP 数据进入后台/会员中心，双写不再静默失败 |
| 2 | wjoy_log 缺 status 列 | install.sql:5-18 / do.php:11 / wjoy_log.go:67 | 补 `ALTER TABLE wjoy_log ADD COLUMN status TINYINT DEFAULT 1` 并同步基线 | 全新安装跳转不再 500，禁用/恢复生效 |
| 3 | JWT 密钥硬编码默认值 | config.go:115 / docker-compose.yml:26 | 启动检测默认密钥 fail-fast；.env 强制注入随机密钥；首登强制改密 | 杜绝离线伪造 admin JWT |
| 4 | TrustedProxies 未生效 | middleware/rate_limit.go:22 / config.go:65 | main.go 显式 `engine.SetTrustedProxies(...)` 并按部署配置可信反代 | 封死 X-Forwarded-For 绕过登录限流 |
| 5 | deploy nginx 跳转 proxy 与路由不匹配 | deploy/nginx/default.conf:59 / router.go:41 | 改 `proxy_pass http://backend_api/r$uri;`（或 Go 加 `/redirect/:code` 别名） | Docker 部署短链跳转恢复 |
| 6 | 违规检测引擎未接线 | pkg/violation.go:62（零调用） | Create 链路接入 CheckURLViolation（blocked 拒绝 / review 写入 violation_reviews）；redirect 前拦截已标记链接 | 阻断违规内容分发，审核功能真正生效 |
| 7 | 软删除 + 唯一索引导致无法重建已删链接 | schema.sql:77-103 / short_url.go:746-753 | 唯一索引改 `(url_hash, deleted_at)`，或删除时硬删/复活软删行 | 删除后同 URL 可重建 |
| 8 | 无备份 + 无监控 + 静态假 /health | 全仓库 / default.conf:74-78 | 增加 mysqldump 备份脚本 + 每日 cron；Go 暴露 /metrics；/health 聚合 MySQL/Redis 真实探活 | 数据可恢复、故障可发现 |

### P1 高优（12 项）

| # | 问题 | 位置 | 整改建议 | 预期效果 |
|---|---|---|---|---|
| 9 | 编辑短链静默清空有效期 + 重置 status | ShortUrlForm.vue:117-147 | 编辑回填剩余天数；提交区分"未修改/永久"；保留原 status | 消除数据丢失级交互缺陷 |
| 10 | 路由守卫不校验 meta.perm | router/index.ts:116-145 | 守卫校验 `to.meta.perm` + 新增 403 页 | 封堵前端越权访问面 |
| 11 | member 端二维码必然 404 | member.html:10 / DashboardView.vue:300-313 | qrcode 改 npm 依赖动态 import，删外链脚本 | 会员端二维码功能恢复 |
| 12 | 登出空实现 / refresh 不轮换 / access 冒充 refresh | handler/auth.go:61-65 / service/auth.go:97-138 | Claims 加 token_type；refresh 仅收 refresh 类型；Redis jti 黑名单 | 泄露 token 可撤销、token 混淆关闭 |
| 13 | SSRF 缺口（CheckLink/FetchTitle/Webhook） | handler/short_url.go:71-86 / member_api.go:356 / webhook.go:134 | 复用 validateURL + 私网 IP 校验 + 回源二次校验 | 封堵内网探测与 DNS rebinding |
| 14 | 双写无对账 / 统计三口径 | function.php:341-352 / do.php:46 / redirect.go:182 | 收敛单主写路径；PHP 读 wjoy_compat 视图；计数以 click_logs 聚合为准 | 前后台数据一致、统计可信 |
| 15 | RBAC 权限语义错误（读权限可写） | router.go:164-165 | 补 audit update/delete 权限点并正确绑定 | 封堵水平越权 |
| 16 | CSV 公式注入 | short_url.go:476-494 / member_api.go:212-233 | 对 `= + - @` 开头单元格加前缀转义 | 防 Excel 公式执行 |
| 17 | docker-compose 弱凭据 + 端口裸露 | docker-compose.yml:17-26,62,82 | `${DB_PASSWORD:?必须设置}` 强制注入；移除 ports 映射或绑 127.0.0.1 | 生产凭据强制定制 |
| 18 | 管理后台无响应式 | AdminLayout.vue / stores/app.ts:15 | 992px 自动折叠侧栏/抽屉；表格加横向滚动保护 | 平板/窄屏可用 |
| 19 | 后台零 aria-label + 无 focus-visible | 全 src/ | 图标按钮补 aria-label；全局 focus-visible 样式 | 读屏/键盘用户可用 |
| 20 | 点击日志异步队列治理 | redirect.go:127-184 / webhook.go:82-113 | webhook 有界 worker pool + 持久化队列 + 全 goroutine recover | 高流量稳定性 |

### P2 中优（12 项）

| # | 问题 | 位置 | 整改建议 | 预期效果 |
|---|---|---|---|---|
| 21 | 批量路径全串行 N+1 | short_url.go:211-260 | 事务内先批量 hash 去重再 CreateInBatches；域计数聚合 UPDATE | 批量 100 条耗时下降 |
| 22 | click_logs 分区无自动扩展 | schema.sql:129-137 | cron 月度 REORGANIZE p_future | 防分区退化大表 |
| 23 | 暗色模式补全 | index.scss / 各页面硬编码 | 替换硬编码浅色背景为 var(--dwz-*) | 暗色全局一致 |
| 24 | 前后台视觉统一 | member 登录页 / PHP 首页 | 统一品牌 Token；会员端去掉深色渐变；前台移动端导航抽屉 | 全站视觉连贯 |
| 25 | 连续生成 + 二维码默认渲染 | app.js:298-315 | 弹窗"再生成一条"；打开即渲染二维码 | 高频场景效率提升 |
| 26 | GeoIP 地域分析 | click_logs 无地域字段 | 接入 GeoLite2 存 country/city + 地图可视化 | 核心增值分析上线 |
| 27 | Referer 来源归类 | click_logs 无 referer 字段 | redirect 记录 referer + 归类可视化 | 渠道归因能力 |
| 28 | 链接密码保护 / 中间确认页 | short_urls 无密码字段 | 表加 password_hash/show_interstitial + gateway 中间件 | 企业级差异化 |
| 29 | 过期预警通知 | cron.go / email.go | 到期前 24h/7d 邮件提醒（email.go 已有基础） | 减少链接失效损失 |
| 30 | Element Plus 全量引入 | main.ts:3,20 | 删 `app.use(ElementPlus)` 改按需；图标按需注册 | 首屏体积下降 50%+ |
| 31 | 系统配置敏感项泄露 | handler/config.go:31-39 | 按 is_public 过滤 + 敏感值脱敏 | 防敏感配置外泄 |
| 32 | 无 schema 版本管理 | 迁移体系 | 建 schema_migrations 表统一迁移注册 | 部署可审计、可回滚 |

### P3 低优（后续迭代）

| # | 问题 | 位置 | 整改建议 | 预期效果 |
|---|---|---|---|---|
| 33 | 代码去重重构 | short_url.go:84-331 三份 Create | 提取 createOne 统一入口；统计/导出下沉复用 | 降低漏改风险 |
| 34 | 前端公共列表骨架 | 7 个 List.vue | usePagedList composable + DataListPage 容器 + 请求取消 | 砍 60% 重复代码 |
| 35 | UTM 构建器 / 标签系统 / 回收站 UI | 规划项 | 前端组件 + 后端表（tags、trash 已有软删基础） | 营销/管理效率 |
| 36 | A/B 测试与条件路由 | 规划项 | link_variants 表 + 分流引擎（roadmap 已有设计） | 核心付费功能 |
| 37 | 多语言 i18n + 错误码统一 | redirect.go:229-235 | vue-i18n + 后端错误类型化 | 用户面扩展 |
| 38 | 死代码清理 | includes/member.php、txprotect.php、tableRef、dist 残留 | 删除/归档 | 减小攻击面与困惑 |
| 39 | 监控页自动刷新 + 健康面板 | MonitorView.vue:35 | 30s 轮询 + DB/Redis/队列状态展示 | 日常巡检能力 |
| 40 | PWA / 浏览器扩展 | 规划项 | manifest + SW；Chrome 右键缩短 MVP | 工具属性增强 |

---

## 七、总结与建议实施顺序

**项目优势**：安全底线扎实（参数化 SQL、输出转义、SSRF、限流、CSRF 全覆盖）；跳转主链路设计成熟（Redis 缓存 + singleflight + 异步批量点击队列）；RBAC/审计/API Key 体系完整；前后台设计语言统一且完成度高；前端工程化（strict TS、token 单飞刷新、双入口分包）处于良好水平。

**核心风险排序**：
1. **数据层**：三处 schema 漂移（member_id / status 缺列）+ 双写无对账 + 统计三口径 → 先补列迁移、再收敛单主写路径
2. **安全**：JWT 默认密钥、TrustedProxies 未启用、违规检测未接线、SSRF 缺口 → 一周内处理
3. **部署**：Docker 跳转 404 路由不匹配、无备份、无监控、弱凭据 → 上线前必修
4. **交互**：编辑清空有效期（数据丢失级）、会员端二维码失效、路由越权 → 立即修复
5. **前端工程**：暗色不完整、无响应式、无 aria-label、Element Plus 全量引入 → 中短期迭代

**建议实施节奏**：P0 共 8 项约 3-5 人日，建议本周内完成并回归验证（重点验证全新安装 + Docker 部署两条链路）；随后进入 P1 安全与数据一致性治理；P2 按"分析洞察 > 商业化"顺序推进功能补充；P3 沉淀为持续迭代 backlog。

---

## 八、2026-08-13 修复与部署记录（已上线）

> 以下 P0/P1 项已在本次会话完成代码修复、本地验证并**部署至生产 1.xk7.cn**，线上回归全部通过。

### 8.1 已上线修复清单

**P0（紧急）**
| 项 | 修复内容 | 线上验证结果 |
|---|---|---|
| P0-1/P0-2 | 基线 SQL 同步（install.sql / init_public_db.sql / schema.sql 补齐 `member_id`、`status`、members 完整字段）+ 新增幂等迁移 `backend/migrations/add_missing_columns.sql`（生产库已手工补列，本次仅同步基线，无需重跑） | 生产双写验证 ✓（PHP 创建 `1n0yfo` 成功同步至 short_urls） |
| P0-3 | JWT 密钥默认值 fail-fast（`main.go` 启动检测，含 docker-compose 默认值） | 新二进制正常启动（生产 secret 非默认值，未误杀）✓ |
| P0-4 | 启用 `engine.SetTrustedProxies` + 生产 config 配置 `["127.0.0.1"]` | **XFF 伪造绕过测试**：换 14 个伪造 XFF 仍 10 次后 429 ✓ |
| P0-5 | deploy/nginx 跳转 proxy 修正为 `/r$uri`（Docker 路径） | Docker 未在生产使用，配置已修正 |
| P0-6 | 违规检测引擎接线：`checkViolation` 接入 Create/createWithTitle + `resolveAndCache` 拦截 + ViolationRepo.Create | PHP api.php 拦截 bet365 ✓；Go 公开 API 拦截 bet365 ✓；正常 URL 创建成功 ✓ |
| P0-7 | 软删复活：`FindByHashIncludingDeleted` + `resurrectDeleted`（同 uid 复活 + 域计数恢复） | **端到端测试**：创建→删除→重建同 URL 返回同 uid 且跳转正常 ✓ |
| P0-8 | 备份脚本 `deploy/backup.sh`（--no-tablespaces）+ 生产 cron（每日 3 点）+ nginx 真实 /health | `https://1.xk7.cn/health` 返回 DB/Redis 真实状态 ✓；cron 已安装 ✓；部署前已双库快照 ✓ |

**P1（高优）**
| 项 | 修复内容 | 线上验证结果 |
|---|---|---|
| 登出/Token 治理 | `token_type` claim（access/refresh 区分）+ refresh 仅接受 refresh 类型 + Redis jti 黑名单登出 | access 冒充 refresh → 401 ✓；登出后 token 复用 → 401 "token has been revoked" ✓ |
| **刷新流程修复（生产真实 bug）** | `/auth/refresh` 从 Auth 中间件组移出为公开路由（前端 `doRefreshToken` 本就不带 Bearer 头，原实现下 2h 后必强制登出） | 无 Bearer 刷新 → 200 新 token ✓ |
| SSRF | `pkg.NewSafeHTTPClient`（拨号层阻断私网）+ CheckLink 复用 `service.ValidateURL` + FetchTitle 安全客户端 + Webhook Create 校验 URL | 编译+单测通过 ✓ |
| 配置脱敏 | `GET /admin/api/configs` 对 password/secret/token/api_key 类 key 值脱敏为 `******` | 已上线 |
| CSV 注入 | `csvSafeCell` 对 `= + - @ \t \r` 开头单元格加引号前缀（admin Export + member ExportLinks） | 已上线 |
| RBAC 语义 | violations PUT/DELETE 改用 `audit/update`、`audit/delete`；权限种子补充并授权 super_admin | 生产权限表已更新 ✓ |
| CORS | 白名单外不再硬 403；Allow-Headers 补 `X-Member-Token` | 已上线 |
| 前端编辑有效期 bug | 编辑模式回填剩余天数 + "不修改"选项 + 未改动时 payload 省略 expire_days + status 不再静默改启用 | 构建+类型检查通过 ✓ |
| 前端路由越权 | 守卫校验 `meta.perm` + 新增 403 页 | ForbiddenView chunk 已部署 ✓ |
| member 二维码 404 | 改 npm `qrcode` 包动态 import，移除外部脚本引用 | 构建通过 ✓（member chunk 已部署） |

### 8.2 部署记录

- 部署路径：`deploy.sh` 对应的 systemd 链路（CentOS 7 + 宝塔 nginx + PHP-FPM + Go :8800）
- 步骤：双库快照 → 上传二进制/dist → 停服换二进制 → 重启 → 部署 SPA → 更新 config.yaml（trusted_proxies）→ nginx 加 /health + 排除 rewrite 冲突 → 权限种子 → 备份 cron
- 安全修复过程中顺带修复了生产 nginx 短码 rewrite 与 `/health` 的路径冲突（服务级 rewrite 先于 location 匹配，`health` 恰为 6 位字母被改写至 do.php）
- 回归验证全部通过：短码跳转 / 首页 / 后台 / 会员端 / API / sitemap / config.php 等敏感路径 403 / 违规拦截 / XFF 防护 / 软删复活 / 刷新流程 / 登出黑名单

### 8.3 遗留待办（下一批 P1/P2）

- P1：双写对账任务（wjoy_log ↔ short_urls 一致性收敛）、点击计数三口径统一
- P1：监控页 30s 轮询、批量 N+1 优化（事务内 CreateInBatches）
- P2：暗色模式补全、后台响应式、GeoIP/Referer 分析、链接密码保护、click_logs 分区自动维护、schema_migrations 版本表
- P2：Element Plus 按需引入（当前全量 1.07MB chunk）、docker-compose 弱凭据/端口收敛

---

## 九、2026-08-13 第二批修复与部署记录（已上线）

> 本批完成上一批 §8.3 中的双写对账、监控轮询、响应式/a11y/暗色等 P1/P2 项。

### 9.1 已上线修复清单

**后端（Go）**
| 项 | 修复内容 | 验证结果 |
|---|---|---|
| 双写对账任务 | cron 每 30 分钟 `reconcile_dual_write`：short_urls→wjoy_log 补缺+状态同步；wjoy_log→short_urls 补缺（12h 窗口 + INSERT IGNORE）。**修复过程发现并定位了 `longurl`（无下划线）列名与 GORM 默认 `long_url` 映射不匹配的 bug**（曾导致空 URL 行 + MD5('') 撞 uk_url_hash），加 `gorm:"column:longurl"` 标签后正确回填 3 条历史缺失数据 | 生产 14→17 行补齐，housps/hgpy1y 恢复跳转 302 ✓；监控 API 列表含新任务 ✓ |
| goroutine panic 防护 | cron 全部任务包 `safe()` 恢复；webhook Dispatch goroutine 包 `safeDeliver` 恢复 | 编译+单测通过 ✓ |
| wjoy_log 表存在性缓存 | `resolveFromLegacy` 的 information_schema 查询改为进程级 `sync.Once` 缓存 | 消除未知短码枚举时的昂贵查询 ✓ |

**前端（Vue）**
| 项 | 修复内容 |
|---|---|
| 后台响应式 | 侧栏 ≤992px 自动折叠（尊重用户手动折叠）+ 顶栏/用户区窄屏简化 + 全局表格横向滚动保护 + 键盘 focus-visible |
| 暗色模式补全 | 全局覆盖各页 scoped 里散落的 `#fff` 硬编码背景（.stat/.mini-btn/.summary-card 等）改用暗色 token；顶栏/用户 hover 改用 CSS 变量 |
| 可访问性 | 关键图标按钮补 aria-label（侧栏/主题切换/短链操作列），全局 `:focus-visible` 焦点环 |
| 监控页 | 30s 自动轮询 + 对账任务中文标签 |
| CSV 导出 | append→click→remove→延迟 revokeObjectURL（修复旧浏览器下载中断） |
| 新建用户 | 角色绑定失败时回滚删除已创建用户（防孤儿账号） |
| 域名批量按钮 | 启用/停用改用 CircleCheck/CircleClose 语义图标 |

### 9.2 部署与回归

- 快照（双库 + 二进制）→ 上传二进制/dist → 停服换二进制重启 → 部署 SPA
- 回归全部通过：首页 200 / /health 真实状态 / 后台·会员端 200 / 对账后短码跳转 302 / 违规拦截 / 新前端 main-JG2r1ggx.js

### 9.3 剩余待办（下一批）

- P1：点击计数三口径统一（wjoy_log.clicks / short_urls.clicks / click_logs 聚合）
- P1：批量创建 N+1 优化（事务内批量 hash 去重 + CreateInBatches）
- P2：GeoIP/Referer 分析、链接密码保护、click_logs 分区自动维护、schema_migrations 版本表
- P2：Element Plus 按需引入、docker-compose 凭据收敛、PHP 死文件清理（includes/member.php、txprotect.php）
- 注：对账窗口为 12h，极端历史缺失可另行跑一次全量同步

---

## 十、2026-08-13 第三批修复与部署记录（已上线）

### 10.1 已上线修复清单

**后端（Go）**
| 项 | 修复内容 | 验证结果 |
|---|---|---|
| 点击数对账 cron | `reconcile_clicks`（每日 04:00）：以 short_urls.clicks 为准同步 wjoy_log.clicks，PHP 统计页不再漏计 Go 路径点击 | 手动触发 ran ✓，任务列表 8 项 ✓ |
| click_logs 分区维护 cron | `ensure_partitions`（每日 03:15）：按月 REORGANIZE p_future 保证分区超前 2 个月，防止数据落进 p_future 大表 | 手动触发幂等验证 ✓（当前已超前至 2026-12，无需创建） |
| goroutine 防护（上一批延续） | cron 全任务 safe() 恢复 + webhook safeDeliver | 单测通过 ✓ |

**清理与加固**
| 项 | 修复内容 |
|---|---|
| PHP 死文件 | 删除 `includes/member.php`、`includes/txprotect.php`（txprotect 为反安全检测工具，零引用，仓库+生产均已移除） |
| docker-compose | DB/JWT/Redis 凭据改为 `${VAR:?必须设置}` 强制注入；MySQL/Redis 端口默认不再暴露宿主机；Redis 健康检查带密码；去掉已废弃的 mysql_native_password |
| backend Dockerfile | 升级 alpine:3.20 + 非 root 用户运行 |

**前端（Vue）**
| 项 | 修复内容 |
|---|---|
| Element Plus 构建 | 尝试全量按需引入后**回退至安全态**：保留 app.use(ElementPlus)（21 处 `from 'element-plus'` 桶导入 + resolver 不接管 ElMessage 等 API 组件导致 tree-shaking 失效），仅移除 300+ 图标全量注册 → element-plus chunk 1.07MB→945KB（gzip 339→306KB）。完整按需需将 21 处桶导入改为深路径（列入待办） |
| 监控页标签 | 补充「点击数对账」「分区维护」任务中文标签 |

### 10.2 部署与回归

- 快照（DB+二进制）→ 上传二进制/dist → 重启 → 部署 SPA → 生产移除死文件
- 回归全部通过：首页 / /health / 后台·会员端 / 短码跳转 302 / 违规拦截 / 敏感路径 403 / 登录 / 监控任务 8 项

### 10.3 已决策延后项

- **链接密码保护**：涉及 ShortUrlService 接口（8+ 调用点含测试）+ PHP/Go 双端跳转热路径拦截（interstitial + 共享 cookie），风险高，需独立批次专注实施
- **批量创建 N+1**：当前生产数据量小，收益低风险中，延后
- **Element Plus 完整按需**：需改 21 处桶导入为深路径，列入待办

---

## 十一、2026-08-13 第四批：链接密码保护（已上线）

> 前批决策的独立批次，本次专注完成该功能并端到端验证。

### 11.1 功能设计

- **存储**：`short_urls.password_hash` / `wjoy_log.password_hash`（bcrypt，NULL=公开链接）；幂等迁移 `backend/migrations/add_password_hash.sql` + 三处基线 SQL 同步
- **双端拦截**：Go `/r/:code`（GET+POST）与 PHP `do.php` 均在校验通过后校验密码：
  - 未解锁 → 返回移动端友好的密码页（内联样式，无第三方依赖）
  - POST 密码 → `password_verify` 校验 → 正确则签发 HMAC cookie（30 天）并 302；错误则提示重试
  - 携带有效 cookie → 直接 302 跳转（点击仅在实际跳转时计数，密码页浏览不计入统计）
- **跨栈 cookie**：`dwz_plink_<uid>` = `<expiry>.<HMAC-SHA256(member_secret, uid.expiry)>`，PHP 与 Go 共用已对齐的 `member_secret`，两条跳转路径互相认可解锁状态
- **入口**：admin/公开/会员 API 创建与更新均支持 `password`；编辑时可选择「不修改/设置新密码/清除密码」（省略字段=不修改，空串=清除）
- **前端**：新建/编辑表单密码字段 + 列表 🔒 锁标识（`has_password` 计算字段，密码哈希永不下发）

### 11.2 上线验证（双端全链路）

| 场景 | Go 路径 | PHP 路径 |
|---|---|---|
| GET 未解锁 | 200 密码页 ✓ | 200 密码页 ✓ |
| POST 错误密码 | 拒绝并提示 ✓ | 拒绝并提示 ✓ |
| POST 正确密码 | 302 + 解锁 cookie ✓ | 302 + 解锁 cookie ✓ |
| 带 cookie 再次访问 | 302 直达目标 ✓ | 302 直达目标 ✓ |
| 双写同步 | PHP 创建带密码链接 → 两端 password_hash 均落库 ✓ | — |
| 无密码链接 | 跳转不受影响 ✓ | 跳转不受影响 ✓ |

修复过程中发现并解决了两个问题：① Go `/r/:code` 原只注册 GET，密码 POST 返回 404（补注册 POST）；② 部署时需同步上传 PHP 修改文件（do.php/api.php/batch.php/function.php）。

### 11.3 回归结果

首页 / /health / 后台 / 会员端 / API文档 200 · 短码 302 · 违规拦截 · 登录 · config.php 403 · 新前端 main-CKspW2mT.js

### 11.4 剩余待办（下一批）

- P1：批量创建 N+1 优化（事务内批量去重 + CreateInBatches）
- P2：GeoIP/Referer 分析可视化（click_logs 已备 referer/country 列）、schema_migrations 版本表、Element Plus 深路径按需

---

## 十二、2026-08-14 第五批：批量性能优化 + 首页交互补全（已上线）

### 12.1 后端（Go）— 批量创建 N+1 优化

| 优化 | 说明 |
|---|---|
| 批量内 URL 去重 | `BatchCreatePublicAPI` / `BatchCreate` / `BatchImport` 三处：同一批内重复 URL 直接复用首个结果（与全局 url_hash 去重语义一致），避免对重复 URL 重复执行 DNS 校验 + FindByHash + INSERT + wjoy_log 双写 |
| SSRF DNS 解析缓存 | `isPrivateHost` 增加 5 分钟 TTL 的进程内缓存（hostname → 私网判定），100 条同域名批量不再触发 100 次阻塞 DNS 查询；缓存 2048 条上限防填充 |

**线上验证**：6 条输入（3 重复 + 1 违规 + 2 正常）→ 返回 5 条，重复 URL 返回同一短码，违规 URL 被过滤 ✓

### 12.2 前端（首页 PHP）— 交互补全

| 项 | 修复内容 |
|---|---|
| 二维码默认渲染 | 结果弹窗打开即生成二维码（移动端扫码是核心诉求），省去一次点击；「显示二维码」按钮语义同步 |
| 再生成一条 | 弹窗新增按钮：关闭弹窗、清空输入框并聚焦，支持高频连续生成 |
| 在线状态真实化 | 移除静态假「在线」文案，改为 `fetch('/health')` 真实探测（60s 轮询），失败时状态点变红并显示「维护中」 |

### 12.3 回归结果

首页 / 后台 / 会员端 / API文档 200 · /health 真实状态 · 短码 302 · 违规拦截 · config.php 403 · 批量去重正确

### 12.4 剩余待办（下一批）

- P1/P2：GeoIP/Referer 分析可视化（click_logs 已备 referer/country 列，需 GeoIP 库 + 前端地图/分布图）
- P2：schema_migrations 版本表、Element Plus 深路径按需、批量创建事务化（CreateInBatches）

---

## 十三、2026-08-14 第六批：GeoIP 地域分析（已上线）

### 13.1 实现方案

- **自研 ip2region v1 读取器**（`backend/internal/pkg/geoip.go`）：零外部 Go 依赖，直接解析 ip2region v1 `.db` 二进制（二分索引 + 打包指针格式：高 8 位=数据长度、低 24 位=文件偏移）。数据文件经 npm 包获得（GitHub raw/gitee 因网络不可达失败）。
- **ISO 3166-1 映射**：中文国名 → 双字母代码（70+ 常用国家/地区），兜底双字母英文直接返回。
- **点击链路**：`ClickQueue` 在批量落库时按 IP 解析国家（进程内 IP→country 缓存，4096 上限），写入 `click_logs.country`（原列已存在、此前从未填充）。
- **配置**：`geoip.db_path`（空=禁用）；生产已配置 `/www/server/dwz-admin/data/ip2region.db`。
- **统计**：单链统计（admin `LinkStats` + member `GetLinkStats`）新增 `countries` 分布（Top 12）。
- **前端**：admin 单链统计弹窗 + 会员端统计弹窗新增「地域分布」区块（ISO→中文名 + 旗帜 emoji）。

### 13.2 线上验证

| 验证项 | 结果 |
|---|---|
| 读取器真实库测试 | 8.8.8.8→美国(US) / 1.1.1.1→澳大利亚(AU) / 114.114.114.114→中国江苏(CN) / 223.5.5.5→浙江杭州阿里云(CN) ✓ |
| 点击落库 | 模拟 4 次点击（8.8.8.8/114.114.114.114/223.5.5.5/8.8.8.8）→ click_logs.country = US/CN/CN/US（重复 IP 命中缓存）✓ |
| stats API | `/admin/api/stats/link/:id` 返回 `countries: [US:1, AU:1, CN:1]` ✓ |
| 启动日志 | "geoip country resolution enabled" ✓ |

### 13.3 回归结果

首页/后台/会员端 200 · /health ok · 短码 302 · 违规拦截 · config.php 403 · 登录 0 · 密码保护无回归

### 13.4 剩余待办（下一批）

- P2：Referer 来源归类（搜索/社媒/直接，click_logs.referer 已存）、全局国家分布页（当前仅单链维度）
- P2：schema_migrations 版本表、Element Plus 深路径按需、批量创建事务化
- 说明：ip2region v1 db（8.7MB）存放于服务器 `/www/server/dwz-admin/data/`（未入 git，.gitignore 应补充）

---

## 十四、2026-08-14 第七批：Referer 来源归类 + 全局分析（已上线）

### 14.1 实现内容

- **来源归类器**（`pkg/referer.go`）：`ClassifyReferer` 将 referer 归为「搜索引擎 / 社交媒体 / 直接访问 / 其他网站」四类（主机名模式匹配，覆盖百度/谷歌/必应/搜狗 + 微博/微信/知乎/抖音/B站/小红书等）。
- **单链统计**：admin `LinkStats` + member `GetLinkStats` 新增 `referrer_types` 分布。
- **全局分析端点**：`GET /stats/countries`（全球 Top 12 国家，近 30 天）+ `GET /stats/referrer-types`（全球来源类型）。
- **前端**：admin/会员统计弹窗新增「来源类型」区块；统计分析页新增「地域分布」「来源类型」两张卡片（旗帜 emoji + 中文名）。

### 14.2 线上验证

| 场景 | 结果 |
|---|---|
| 单链来源归类 | 5 次点击（百度/微博/空/example.org/谷歌）→ 搜索引擎 2、其他网站 2、直接访问 1 ✓（验证中发现 weibo.cn 未命中，已修复并重部署） |
| 全局端点 | `/stats/countries` 返回 [US:7, CN:2] ✓；`/stats/referrer-types` 返回 [直接访问:309, 其他网站:12, 搜索引擎:2] ✓ |

### 14.3 回归结果

首页/后台/会员端 200 · /health ok · 短码 302 · 违规拦截 · config.php 403 · 登录 · 新前端 main-Cft4xwcw.js

### 14.4 剩余待办（下一批）

- P2：schema_migrations 版本表、Element Plus 深路径按需、批量创建事务化（CreateInBatches）、统计弹窗重构去重（三处复制）

---

## 十五、2026-08-14 第八批：Element Plus 深路径按需（已上线，浏览器验证通过）

### 15.1 实现内容

- **21 处桶导入转深路径**：`import { ElMessage } from 'element-plus'` → `element-plus/es/components/message/index`、`message-box/index`（脚本批量转换 + 手工补回 6 处 `type FormRules/FormInstance/TableInstance` 类型导入——转换脚本曾误删 `type X` 前缀项，已修正）。
- **移除全量挂载**：`main.ts`/`member-main.ts` 去掉 `app.use(ElementPlus)`；中文 locale 改由 `App.vue`/`member/App.vue` 的 `<el-config-provider :locale="zhCn">` 注入。

### 15.2 关键发现：bundle 无实质缩减

- 转换前后 element-plus chunk 均为 **~943KB**（转换前 945KB）。**结论**：该管理端实际使用了 Element Plus 绝大多数组件（表格/表单/弹窗/下拉/日期选择器等，且组件间依赖链深），tree-shaking 收益≈0。深路径转换的价值是工程规范化 + 未来组件精简时自动受益，并非当下性能优化。
- **浏览器实测验证（browser-use）**：登录页 → 仪表盘（echarts 图表/统计卡/toast）→ 短链管理（表格/筛选/分页/🔒锁标识/操作列）→ 统计分析（地域分布 🇨🇳/🇺🇸 + 来源类型 🔗/🌐 卡片）→ 新建弹窗（含访问密码字段）全部渲染正常，无缺组件。

### 15.3 回归与清理

- 回归：首页/后台/会员端 200 · /health ok · 短码 302 · 违规拦截 · config.php 403
- 清理：移除历次批次遗留的 6 条测试短链（geoip-test/pw-php/password-test/ref-check 等）

### 15.4 剩余待办（下一批）

- P2：schema_migrations 版本表、批量创建事务化（CreateInBatches）、统计弹窗重构去重（member/admin 两处复制）
- 说明：前端经 browser-use 实测，后续 UI 变更可用同样方式回归，替代盲改盲发

---

## 十六、2026-08-14 第九批：回收站（软删除恢复）（已上线）

### 16.1 实现内容

- **后端**：
  - repository：`FindByIDIncludingDeleted`（Unscoped 查找）+ `RestoreWithDomainCount`（事务内清除 deleted_at + 恢复域名计数）+ `ShortUrlFilters.IncludeDeleted`（列表仅查已软删行）
  - service：`Restore(id)` —— 校验确已删除 → 恢复 → wjoy_log status=1（PHP 跳转恢复）→ 缓存失效
  - handler/route：`POST /admin/api/short-urls/:id/restore`（short_urls/update 权限）+ List 透传 `include_deleted=1`
- **前端**：短链管理工具栏「回收站」开关（列表 ↔ 回收站视图切换）+ 已删除行的「恢复」操作按钮（确认弹窗）

### 16.2 验证

| 场景 | 结果 |
|---|---|
| API 全流程 | 创建 → 删除 → `include_deleted=1` 列出（回收站 total=2）→ 恢复成功 → `/r/:code` 302 直达 ✓ |
| UI 渲染 | 工具栏「回收站」按钮渲染且启用 ✓（浏览器点击因 IAB 自动化事件限制未触发 Vue 处理器，属环境限制；API 全流程已完整验证） |
| 回归 | 首页/后台/会员端 200 · /health ok · 短码 302 · 违规拦截 · 登录 · restore 路由 401（未认证） ✓ |

### 16.3 剩余待办（下一批）

- P2：schema_migrations 版本表、批量创建事务化（CreateInBatches）、统计弹窗去重（member/admin 两处复制）
- 说明：回收站已覆盖"误删恢复"核心诉求；永久删除可后续在回收站视图内补充

---

*报告完 · 基于当前工作区代码（含未提交变更）*
