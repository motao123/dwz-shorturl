# 短网址平台 — 功能规划与落地执行方案

> 版本：v1.0 | 日期：2026-07-28 | 状态：规划稿

---

## 目录

1. [项目全维度诊断](#1-项目全维度诊断)
2. [新增功能与模块规划](#2-新增功能与模块规划)
3. [现有模块精细化补全](#3-现有模块精细化补全)
4. [全链路交互体验升级](#4-全链路交互体验升级)
5. [落地执行方案](#5-落地执行方案)

---

## 1. 项目全维度诊断

### 1.1 已上线功能全景

#### 前台用户端（PHP + 原生 JS）

| 模块 | 已实现能力 | 技术实现 |
|------|-----------|----------|
| 单条短链生成 | URL 输入 + 自动补全协议 + 自定义短码 + 有效期选择 | `api.php` POST + 前端 fetch |
| 批量短链生成 | 最多 100 条 + 实时计数 + 逐条结果展示 | `batch.php` POST + 事件委托渲染 |
| 结果交互 | 模态弹窗 + 焦点陷阱 + 一键复制 + QR 码生成 | Dialog + Clipboard API + qrcode.js |
| 短链跳转 | 302 重定向 + 过期校验 + 点击计数 + 禁缓存头 | `do.php` + Nginx rewrite |
| 安全防护 | SSRF 双向校验 + IP 限流 + POST-only + 可信代理 | `includes/function.php` |
| 统计页 | Token 认证 + 总量/Top10/最近20 + 安全响应头 | `stats.php` |
| API 文档 | 完整接口说明 + 错误码表 + curl 示例 | `api.html` 静态页 |
| 无障碍 | Skip link + ARIA + 焦点管理 + reduced-motion | 全页面覆盖 |

#### 后台管理端（Go + Vue 3）

| 模块 | 已实现能力 | 技术实现 |
|------|-----------|----------|
| 认证系统 | JWT 登录/刷新/登出 + Token 黑名单预留 | Gin middleware + golang-jwt |
| 短链管理 | 列表/搜索/筛选/分页/CRUD/批量操作/CSV导出 | GORM + Element Plus Table |
| 数据统计 | 概览卡片 + 趋势折线图 + Top10 柱状图 + 最近列表 | ECharts + vue-echarts |
| 用户管理 | CRUD + 角色分配 + 密码重置 + 状态管控 | RBAC 中间件 |
| 角色权限 | 4 级角色 + 22 权限点 + 树形权限分配 | el-tree + 中间件守卫 |
| 系统配置 | 分组 KV 编辑 + 类型感知 + 脏标记 + 批量保存 | system_configs 表 |
| 审计日志 | 全操作记录 + JSON 详情 + 多维筛选 | audit_logs 表 |
| API 密钥 | 创建/吊销/用量统计 + 一次性明文展示 | SHA-256 哈希存储 |

### 1.2 技术债务与架构风险

| # | 问题 | 严重度 | 影响 |
|---|------|--------|------|
| 1 | **双架构数据不一致** — PHP 用 `wjoy_log`，Go 用 `short_urls`，两套表可能数据不同步 | 🔴 高 | 前后台数据割裂 |
| 2 | **双跳转路径** — PHP `/do.php?uid=` vs Go `/r/:code`，Nginx 路由边界不清 | 🔴 高 | 维护混乱、统计分散 |
| 3 | **点击日志无背压** — Go redirect 用裸 goroutine 写 DB，高并发下 OOM 风险 | 🟡 中 | 生产稳定性 |
| 4 | **Redis 缓存未启用** — 注入但未使用，每次跳转都查 DB | 🟡 中 | 性能瓶颈 |
| 5 | **文件限流不跨节点** — PHP 限流基于本地文件，多实例部署失效 | 🟡 中 | 扩展性 |
| 6 | **Go 跳转缺正则校验** — `/r/:code` 未校验短码格式 | 🟡 中 | 安全隐患 |
| 7 | **Base64 旧数据兼容** — `do.php` 兼容旧编码，无清理迁移计划 | 🟢 低 | 技术债 |
| 8 | **硬编码中文字符串** — 无 i18n 框架，后台错误信息英文 | 🟢 低 | 国际化障碍 |

### 1.3 功能缺口盘点

| 类别 | 缺失能力 | 业务影响 |
|------|----------|----------|
| 分析 | GeoIP 地域分布、设备/浏览器/OS 解析、Referer 来源可视化 | 无法做精准运营 |
| 分析 | 实时数据推送（WebSocket）、定时报告（邮件/PDF） | 运营效率低 |
| 安全 | 链接密码保护、IP 白/黑名单、2FA、Bot 过滤 | 企业场景缺失 |
| 安全 | 链接健康检测（目标 URL 存活监控）、点击欺诈检测 | 无法保障链路质量 |
| 效率 | UTM 构建器、批量导入（CSV/JSON）、链接标签系统 | 批量操作不便 |
| 效率 | 定时过期自动清理、过期预警通知 | 数据库膨胀 |
| 体验 | 暗色模式、多语言、PWA 离线、浏览器扩展 | 用户留存受限 |
| 商业 | 自定义品牌域名、A/B 测试、链接轮转、中间页 | 无法商业化 |
| 架构 | API 版本控制、游标分页、多租户、Webhook 事件 | 规模化瓶颈 |

---

## 2. 新增功能与模块规划

### 2.1 核心业务功能（P0 — 直接提升产品价值）

#### F1: 智能点击分析引擎

**业务价值**：从"简单计数"升级为"多维洞察"，支撑运营决策。

**核心能力**：
- GeoIP 地域解析（MaxMind GeoLite2 免费库）→ 国家/省份/城市
- User-Agent 解析（uaparser）→ 设备类型/浏览器/操作系统
- Referer 来源归类（搜索引擎/社交媒体/直接访问/其他）
- 时段分布热力图（24h × 7d）
- 实时点击流（WebSocket 推送，后台大屏）

**技术方案**：
- 新增 `click_analytics` 服务，在 redirect 时异步解析 UA + GeoIP
- Redis Stream 作为事件总线，消费者批量写入 `click_logs` 扩展字段
- 前端新增 `AnalyticsView.vue`，含地图组件（ECharts map）+ 设备饼图 + 来源桑基图

**数据模型扩展**：
```sql
ALTER TABLE click_logs ADD COLUMN device_type VARCHAR(16);  -- desktop/mobile/tablet
ALTER TABLE click_logs ADD COLUMN browser VARCHAR(32);
ALTER TABLE click_logs ADD COLUMN os VARCHAR(32);
ALTER TABLE click_logs ADD COLUMN city VARCHAR(64);
ALTER TABLE click_logs ADD COLUMN referer_type VARCHAR(16); -- search/social/direct/other
```

---

#### F2: 链接安全网关

**业务价值**：满足企业级安全合规需求，防止短链被滥用。

**核心能力**：
- 链接密码保护（访问时弹出密码输入页）
- IP 白名单/黑名单（CIDR 格式，支持按链接或全局配置）
- Bot/爬虫过滤（基于 UA 规则 + 行为频率异常检测）
- 点击欺诈检测（同 IP 短时间大量点击 → 标记 + 告警）
- 中间确认页（可选，显示目标域名让用户确认后再跳转）

**技术方案**：
- `short_urls` 表新增 `password_hash`、`ip_whitelist`、`ip_blacklist`、`show_interstitial` 字段
- 新增 `gateway` 中间件链：密码校验 → IP 校验 → Bot 检测 → 欺诈检测 → 跳转/中间页
- 中间页为服务端渲染的轻量 HTML（不依赖 SPA）
- 前端短链表单新增"安全设置"折叠面板

---

#### F3: 自动化运营工具

**业务价值**：减少人工操作，提升运营效率。

**核心能力**：
- 定时任务引擎（Cron）：
  - 过期链接自动标记 + 清理（可配置保留天数）
  - 死链检测（定期 HEAD 请求目标 URL，标记 4xx/5xx）
  - 数据库统计预聚合（每小时/每天汇总到 `stats_hourly`/`stats_daily` 表）
- 通知系统：
  - 链接即将过期预警（到期前 24h/7d 通知创建者）
  - 死链告警（目标 URL 不可达时通知）
  - 异常流量告警（单链接点击突增 N 倍）
- 通知渠道：站内信 → 邮件（SMTP）→ Webhook → 企业微信/钉钉（预留）

**技术方案**：
- Go 内置 `robfig/cron` 调度器，main.go 启动时注册任务
- 新增 `notifications` 表 + `notification_preferences` 用户配置
- 邮件使用 `net/smtp` + HTML 模板
- Webhook 使用异步 HTTP POST + 重试策略（3 次指数退避）

---

#### F4: UTM 构建器 + 批量导入

**业务价值**：营销场景刚需，提升创建效率 10 倍。

**核心能力**：
- UTM 参数可视化构建（source/medium/campaign/term/content）
- 实时预览最终 URL
- 一键将 UTM URL 送入短链生成
- CSV/JSON 批量导入（支持映射字段：url/title/category/custom_code/expire）
- 导入预览 + 错误行标注 + 部分成功

**技术方案**：
- 前端新增 `UtmBuilder.vue` 组件（可嵌入短链表单或独立页面）
- 后端新增 `POST /admin/api/short-urls/import`（multipart/form-data 接收文件）
- 解析逻辑：逐行校验 + 批量事务 + 返回逐行结果

---

### 2.2 支撑性模块（P1 — 提升平台能力边界）

#### F5: 自定义品牌域名

**业务价值**：企业客户核心需求，提升品牌辨识度。

**核心能力**：
- 域名管理（添加/验证/删除自定义域名）
- DNS 验证（CNAME 或 TXT 记录校验）
- 按域名隔离短链命名空间
- SSL 证书自动签发（Let's Encrypt / ACME）
- 默认域名 fallback

**技术方案**：
- 新增 `domains` 表（domain, status, ssl_status, verified_at, user_id）
- Nginx 动态 server 块 或 Go 内置 TLS + SNI 路由
- 验证流程：生成 CNAME 目标 → 用户配置 → 定时检测 → 标记已验证

---

#### F6: 链接高级路由

**业务价值**：精准流量分发，支持 A/B 测试和个性化投放。

**核心能力**：
- A/B 测试：按百分比分流到多个目标 URL，自动统计各变体转化
- 设备路由：iOS → App Store / Android → Play Store / 其他 → 落地页
- 地域路由：不同国家/地区跳转不同目标
- 时间路由：活动期内 → 活动页 / 活动结束 → 默认页
- 轮转策略：Round-Robin / 加权随机

**技术方案**：
- 新增 `link_variants` 表（short_url_id, target_url, weight, device_filter, geo_filter, time_range）
- 跳转时根据请求上下文匹配 variant → 302 到对应目标
- 前端新增"高级路由"配置面板（可视化条件编辑器）

---

#### F7: 开放平台升级

**业务价值**：支撑第三方集成，构建生态。

**核心能力**：
- API 版本控制（`/v1/`、`/v2/` 前缀 + 版本协商头）
- 按 API Key 独立限流（Redis 令牌桶，替代全局 IP 限流）
- Webhook 事件订阅（link.created / link.clicked / link.expired / link.deleted）
- SDK 生成（JavaScript / Python / Go / PHP 客户端库）
- API Playground（在线调试工具，Swagger/OpenAPI UI）

**技术方案**：
- 新增 `webhooks` 表 + `webhook_deliveries` 投递记录表
- 事件总线：Redis Pub/Sub 或内存 channel → 异步投递 worker
- OpenAPI 3.0 spec 自动生成（swaggo/swag 注解）

---

### 2.3 体验增强模块（P2 — 提升用户粘性）

#### F8: 暗色模式 + 多语言

- CSS 变量双主题（`prefers-color-scheme` + 手动切换）
- vue-i18n 集成，默认中文 + 英文，JSON 语言包
- 后端错误信息 i18n（Accept-Language 头协商）

#### F9: PWA + 浏览器扩展

- PWA：manifest.json + Service Worker（离线缓存管理页）
- Chrome 扩展：右键缩短当前页 / 选中文字 / 批量缩短 / 快捷面板
- 扩展与后台通过 API Key 通信

#### F10: 实时数据大屏

- WebSocket 连接推送实时点击事件
- 后台新增"实时监控"页面：滚动点击流 + 实时计数器 + 地域地图动态打点
- 可选全屏投屏模式（运营展示用）

---

## 3. 现有模块精细化补全

### 3.1 前台用户端补全

| 补全项 | 当前状态 | 目标状态 |
|--------|----------|----------|
| 链接预览卡片 | 无 | 生成后展示 OG 标题/图片/描述的预览卡 |
| 历史记录 | 无（无用户体系） | 基于 localStorage 的最近 20 条 + 可选登录同步 |
| 链接二维码美化 | 基础 QR | 支持 Logo 嵌入 + 颜色自定义 + 下载 PNG/SVG |
| 分享按钮 | 仅复制 | 增加微博/微信/Twitter/邮件一键分享 |
| 输入增强 | 纯文本 | 拖拽链接 / 粘贴自动识别 / 多 URL 智能拆分 |
| 移动端适配 | 基本响应式 | 底部安全区适配 + 触控优化 + 键盘弹起处理 |

### 3.2 后台管理端补全

| 补全项 | 当前状态 | 目标状态 |
|--------|----------|----------|
| 软删除恢复 | GORM 支持但无 UI | 新增"回收站"Tab + 恢复/永久删除操作 |
| 批量导入 | 仅导出 | CSV/JSON 上传 + 字段映射 + 预览确认 |
| 链接标签 | 仅单分类 | 多标签系统（tag 表 + 多选组件） |
| Referer 分析 | 存储但未展示 | 来源域名 Top N + 类型分布饼图 |
| 审计日志导出 | 无 | CSV/JSON 导出 + 按条件筛选导出 |
| 用户操作统计 | 无 | 用户维度短链数/点击贡献/活跃度排行 |
| 配置热加载 | 需重启 | Redis 缓存配置 + 变更时 Pub/Sub 通知 |
| 数据备份管理 | 无 UI | 后台触发备份 + 备份列表 + 一键恢复 |
| 系统健康检查 | 仅 /health | 详细面板：DB 连接池/Redis 内存/Goroutine 数/磁盘 |

### 3.3 权限精细化补全

| 补全项 | 当前状态 | 目标状态 |
|--------|----------|----------|
| 数据级权限 | 无 | 用户只能看自己创建的短链（created_by 过滤） |
| 字段级脱敏 | 无 | 敏感字段（IP/UA）按角色脱敏展示 |
| 操作二次确认 | 部分 | 所有破坏性操作统一确认弹窗 + 输入确认码 |
| 登录安全 | 密码 only | 2FA (TOTP) + 登录设备管理 + 异地登录告警 |
| 会话管理 | 无 | 活跃会话列表 + 强制下线 + 登录历史 |

---

## 4. 全链路交互体验升级

### 4.1 前台交互优化

| 场景 | 当前痛点 | 优化方案 |
|------|----------|----------|
| 首次访问 | 无引导 | 新增 3 步新手引导浮层（输入→配置→获取） |
| 输入 URL | 无即时反馈 | 输入后实时显示域名 favicon + 页面标题预览 |
| 生成等待 | 无加载态 | 按钮 Skeleton + 进度条动画 |
| 生成失败 | Toast 一闪而过 | 内联错误提示 + 修复建议（如"该域名不可达"） |
| 批量结果 | 长列表无摘要 | 顶部成功/失败计数 + 失败项高亮 + 一键重试失败项 |
| 复制反馈 | 无差异化 | 复制成功按钮变绿 + ✓ 图标 1.5s 后恢复 |
| 页面跳转 | 无过渡 | 路由切换 fade 动画 + 骨架屏占位 |
| 异常跳转 | 404/410 纯文本 | 品牌化错误页 + 返回首页/重新生成引导 |

### 4.2 后台交互优化

| 场景 | 当前痛点 | 优化方案 |
|------|----------|----------|
| 表格操作 | 无快捷键 | 支持键盘导航（↑↓ 选行、Enter 编辑、Del 删除） |
| 表单填写 | 无自动保存 | 草稿自动保存（localStorage 5s 间隔） |
| 长列表 | 传统分页 | 虚拟滚动（>1000 行时）+ 无限加载可选 |
| 筛选条件 | 重置不便 | 筛选条件标签化 + 一键清除 + 保存常用筛选 |
| 数据导出 | 同步阻塞 | 异步导出 + 进度通知 + 下载中心 |
| 操作反馈 | ElMessage 短暂 | 重要操作增加操作记录侧边栏（最近 10 条可撤销） |
| 空状态 | 默认 el-empty | 品牌化空状态插画 + 引导操作按钮 |
| 加载状态 | v-loading 遮罩 | 骨架屏替代遮罩（减少视觉打断） |

### 4.3 无障碍与多端适配

| 维度 | 当前状态 | 目标 |
|------|----------|------|
| 键盘导航 | 前台完整，后台部分 | 全组件 Tab 序正确 + 快捷键文档 |
| 屏幕阅读器 | 前台 ARIA 完整 | 后台所有表格/图表补充 aria-label + 数据摘要 |
| 对比度 | 未审计 | WCAG 2.1 AA 全通过（axe-core 自动检测） |
| 触控目标 | 前台 44px | 后台按钮/链接最小 36px 触控区 |
| 响应式断点 | 前台 4 档 | 后台增加 768px 平板适配（侧栏自动折叠） |
| 高对比度模式 | 前台 forced-colors | 后台同步支持 |
| 动效偏好 | 前台 reduced-motion | 全局统一尊重 prefers-reduced-motion |

---

## 5. 落地执行方案

### 5.1 优先级排序与分期规划

#### Phase 1 — 基础加固（第 1-2 周）

> 目标：解决架构风险，统一数据层，补齐运维自动化

| 任务 | 工时 | 验收标准 |
|------|------|----------|
| 统一数据层：PHP 迁移到 `short_urls` 表，废弃 `wjoy_log` | 3d | 前后台读写同一表，数据一致 |
| 统一跳转入口：Nginx 全部走 Go `/r/:code`，废弃 `do.php` | 1d | 单入口，点击统计归一 |
| Redis 缓存启用：短码→URL 映射缓存（TTL 1h） | 1d | 跳转 P99 < 10ms |
| 点击日志队列化：Redis Stream + 批量消费者 | 2d | 无裸 goroutine，支持背压 |
| Go 跳转正则校验 + 输入安全加固 | 0.5d | 非法短码直接 404 |
| Cron 调度器：过期清理 + 统计预聚合 | 2d | 每日自动清理过期链接 |
| 系统健康面板 | 1d | 后台可查看 DB/Redis/Goroutine 状态 |

**Phase 1 小计：~10.5 人日**

---

#### Phase 2 — 分析能力（第 3-4 周）

> 目标：从"计数"升级为"洞察"

| 任务 | 工时 | 验收标准 |
|------|------|----------|
| GeoIP 集成（MaxMind GeoLite2） | 2d | 点击日志含国家/城市 |
| UA 解析（设备/浏览器/OS） | 1.5d | 点击日志含 device_type/browser/os |
| Referer 归类 + 可视化 | 2d | 来源分布饼图 + Top 域名列表 |
| 分析仪表盘页面 | 3d | 地图 + 设备图 + 来源图 + 时段热力图 |
| 单链接详情页（分析维度） | 2d | 点击该链接进入多维分析视图 |
| 实时点击流（WebSocket） | 2d | 后台实时滚动展示最新点击 |

**Phase 2 小计：~12.5 人日**

---

#### Phase 3 — 安全与效率（第 5-6 周）

> 目标：企业级安全 + 批量操作效率

| 任务 | 工时 | 验收标准 |
|------|------|----------|
| 链接密码保护 | 2d | 设密码后访问需输入正确密码 |
| IP 白/黑名单 | 1.5d | 非白名单 IP 访问返回 403 |
| Bot 过滤 + 欺诈检测 | 2d | 已知 Bot UA 不计入统计 |
| 中间确认页 | 1d | 可选开启，展示目标域名 |
| UTM 构建器 | 2d | 可视化构建 + 预览 + 一键生成 |
| 批量导入（CSV/JSON） | 2d | 上传 → 预览 → 确认 → 逐行结果 |
| 链接标签系统 | 1.5d | 多标签 CRUD + 按标签筛选 |
| 2FA (TOTP) | 2d | 登录时可启用 Google Authenticator |
| 死链检测 Cron | 1d | 每日检测 + 标记 + 通知 |

**Phase 3 小计：~15 人日**

---

#### Phase 4 — 体验与生态（第 7-9 周）

> 目标：用户体验跃升 + 开放生态

| 任务 | 工时 | 验收标准 |
|------|------|----------|
| 暗色模式 | 2d | 全局切换 + 跟随系统 |
| 多语言 (i18n) | 3d | 中/英双语，前后端完整覆盖 |
| 前台新手引导 | 1d | 首次访问 3 步引导 |
| 异常页面品牌化 | 1d | 404/410/403 品牌页 + 引导 |
| Webhook 事件系统 | 3d | 4 种事件 + 投递记录 + 重试 |
| OpenAPI 文档 + Playground | 2d | Swagger UI 在线调试 |
| PWA 支持 | 1.5d | manifest + SW + 离线管理页 |
| 浏览器扩展 MVP | 3d | Chrome 扩展：右键缩短 + 弹窗面板 |
| 通知系统（邮件 + 站内信） | 3d | 过期预警 + 死链告警 + 异常流量 |

**Phase 4 小计：~19.5 人日**

---

#### Phase 5 — 商业化能力（第 10-12 周）

> 目标：支撑付费场景

| 任务 | 工时 | 验收标准 |
|------|------|----------|
| 自定义品牌域名 | 5d | DNS 验证 + SSL 自动签发 + 域名隔离 |
| A/B 测试 + 链接轮转 | 4d | 多变体配置 + 分流 + 转化对比 |
| 设备/地域/时间路由 | 3d | 条件路由规则编辑器 |
| 多租户架构 | 5d | tenant_id 隔离 + 独立配额 |
| 用量计量 + 配额管理 | 3d | 按套餐限制短链数/API 调用/域名数 |
| 数据大屏（投屏模式） | 2d | 全屏实时数据展示 |

**Phase 5 小计：~22 人日**

---

### 5.2 技术实现路线图

```
Week 1-2    [Phase 1] 架构加固
              ├── 数据层统一 (migration script)
              ├── Redis 缓存 + Stream 队列
              ├── Cron 调度器
              └── 健康面板

Week 3-4    [Phase 2] 分析引擎
              ├── GeoIP + UA 解析 pipeline
              ├── click_logs 扩展字段回填
              ├── 分析仪表盘 (ECharts map/pie/heatmap)
              └── WebSocket 实时推送

Week 5-6    [Phase 3] 安全 + 效率
              ├── 密码保护 + IP 管控 + Bot 过滤
              ├── UTM 构建器 + 批量导入
              ├── 标签系统 + 2FA
              └── 死链检测

Week 7-9    [Phase 4] 体验 + 生态
              ├── 暗色模式 + i18n
              ├── Webhook + OpenAPI
              ├── PWA + 浏览器扩展
              └── 通知系统

Week 10-12  [Phase 5] 商业化
              ├── 品牌域名 + SSL 自动化
              ├── A/B 测试 + 高级路由
              ├── 多租户 + 配额
              └── 数据大屏
```

### 5.3 效果验收标准

| 维度 | 指标 | 基线（当前） | 目标（Phase 1-3 后） | 目标（全部完成后） |
|------|------|-------------|---------------------|-------------------|
| 性能 | 跳转 P99 延迟 | ~50ms (DB 直查) | < 10ms (Redis) | < 5ms |
| 性能 | 管理后台首屏 | ~2s | < 1.5s | < 1s |
| 可用性 | 月度 uptime | 99% (估) | 99.9% | 99.95% |
| 分析 | 点击数据维度 | 1 (计数) | 6 (地域/设备/来源/时段) | 10+ |
| 安全 | 安全特性数 | 3 (SSRF/限流/POST-only) | 8 (+密码/IP/2FA/Bot/欺诈) | 12+ |
| 效率 | 批量创建 100 条耗时 | ~5s | ~3s | < 1s (异步) |
| 体验 | Lighthouse 评分 | ~85 | > 90 | > 95 |
| 体验 | 无障碍 WCAG | 部分 AA | 全 AA | 全 AA + AAA 关键路径 |
| 生态 | 集成方式 | 1 (HTTP API) | 4 (+Webhook/SDK/扩展/PWA) | 6+ |
| 商业 | 可收费功能点 | 0 | 2 (品牌域名/配额) | 5+ |

### 5.4 风险与依赖

| 风险 | 影响 | 缓解措施 |
|------|------|----------|
| GeoLite2 数据库许可变更 | 地域分析不可用 | 备选 ip2location-lite / DB-IP |
| 多租户改造涉及全表加字段 | 迁移风险高 | Phase 5 前完成全量备份 + 灰度 |
| WebSocket 长连接在宝塔 Nginx 配置复杂 | 实时功能受阻 | 备选 SSE (Server-Sent Events) |
| 浏览器扩展 Chrome Web Store 审核 | 上架延迟 | 先提供 CRX 侧载 + 后续提审 |
| SSL 自动签发需要 80 端口验证 | 与现有站点冲突 | 使用 DNS-01 challenge |

---

## 附录 A：新增数据模型预览

```sql
-- 链接变体（A/B 测试 + 路由）
CREATE TABLE link_variants (
  id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  short_url_id BIGINT UNSIGNED NOT NULL,
  target_url TEXT NOT NULL,
  weight INT NOT NULL DEFAULT 100,
  device_filter VARCHAR(32) NULL,      -- NULL=all, ios, android, desktop
  geo_filter JSON NULL,                -- ["CN","US"]
  time_start DATETIME NULL,
  time_end DATETIME NULL,
  clicks INT UNSIGNED DEFAULT 0,
  conversions INT UNSIGNED DEFAULT 0,  -- 预留转化追踪
  created_at DATETIME(3) DEFAULT CURRENT_TIMESTAMP(3),
  KEY idx_short_url (short_url_id)
);

-- Webhook 订阅
CREATE TABLE webhooks (
  id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  user_id BIGINT UNSIGNED NOT NULL,
  url VARCHAR(512) NOT NULL,
  events JSON NOT NULL,                -- ["link.created","link.clicked"]
  secret VARCHAR(64) NOT NULL,         -- HMAC 签名密钥
  status TINYINT DEFAULT 1,
  last_triggered_at DATETIME(3) NULL,
  failure_count INT DEFAULT 0,
  created_at DATETIME(3) DEFAULT CURRENT_TIMESTAMP(3),
  KEY idx_user (user_id)
);

-- Webhook 投递记录
CREATE TABLE webhook_deliveries (
  id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  webhook_id BIGINT UNSIGNED NOT NULL,
  event VARCHAR(32) NOT NULL,
  payload JSON NOT NULL,
  response_status INT NULL,
  response_body TEXT NULL,
  attempt INT DEFAULT 1,
  created_at DATETIME(3) DEFAULT CURRENT_TIMESTAMP(3),
  KEY idx_webhook (webhook_id, created_at DESC)
);

-- 自定义域名
CREATE TABLE domains (
  id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  user_id BIGINT UNSIGNED NOT NULL,
  domain VARCHAR(128) NOT NULL,
  status ENUM('pending','verified','failed','ssl_pending','active') DEFAULT 'pending',
  verification_token VARCHAR(64) NOT NULL,
  ssl_status ENUM('none','pending','active','failed') DEFAULT 'none',
  ssl_expires_at DATETIME NULL,
  verified_at DATETIME(3) NULL,
  created_at DATETIME(3) DEFAULT CURRENT_TIMESTAMP(3),
  UNIQUE KEY uk_domain (domain),
  KEY idx_user (user_id)
);

-- 链接标签
CREATE TABLE tags (
  id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  name VARCHAR(32) NOT NULL,
  color VARCHAR(7) NULL,
  user_id BIGINT UNSIGNED NOT NULL,
  created_at DATETIME(3) DEFAULT CURRENT_TIMESTAMP(3),
  UNIQUE KEY uk_user_name (user_id, name)
);

CREATE TABLE short_url_tags (
  short_url_id BIGINT UNSIGNED NOT NULL,
  tag_id BIGINT UNSIGNED NOT NULL,
  PRIMARY KEY (short_url_id, tag_id)
);

-- 通知
CREATE TABLE notifications (
  id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  user_id BIGINT UNSIGNED NOT NULL,
  type VARCHAR(32) NOT NULL,           -- expiry_warning, dead_link, anomaly
  title VARCHAR(128) NOT NULL,
  content TEXT NOT NULL,
  resource_type VARCHAR(32) NULL,
  resource_id BIGINT UNSIGNED NULL,
  is_read TINYINT DEFAULT 0,
  created_at DATETIME(3) DEFAULT CURRENT_TIMESTAMP(3),
  KEY idx_user_read (user_id, is_read, created_at DESC)
);

-- 统计预聚合
CREATE TABLE stats_hourly (
  id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  short_url_id BIGINT UNSIGNED NOT NULL,
  hour DATETIME NOT NULL,              -- 截断到小时
  clicks INT UNSIGNED DEFAULT 0,
  unique_ips INT UNSIGNED DEFAULT 0,
  KEY idx_url_hour (short_url_id, hour),
  UNIQUE KEY uk_url_hour (short_url_id, hour)
);
```

---

## 附录 B：技术栈补充清单

| 新增依赖 | 用途 | 许可 |
|----------|------|------|
| `oschwald/geoip2-golang` | GeoIP 解析 | Apache 2.0 |
| `mssola/useragent` 或 `ua-parser/uap-go` | UA 解析 | MIT / Apache 2.0 |
| `robfig/cron/v3` | 定时任务 | MIT |
| `gorilla/websocket` | 实时推送 | BSD-3 |
| `swaggo/swag` + `swaggo/gin-swagger` | OpenAPI 生成 | MIT |
| `go-playground/validator/v10` | 请求校验增强 | MIT |
| `pquerna/otp` | TOTP 2FA | Apache 2.0 |
| `golang.org/x/crypto/acme` | SSL 自动签发 | BSD-3 |
| `vue-i18n` | 前端多语言 | MIT |
| `@vueuse/core` | 暗色模式/LocalStorage 等 | MIT |
| `pinia-plugin-persistedstate` | Store 持久化 | MIT |

---

*文档结束。本规划覆盖 5 个 Phase、10 个核心新功能模块、20+ 项现有功能补全、全链路交互优化方案，总估算约 79.5 人日（约 4 个月单人 / 2 个月双人并行），可直接指导迭代开发。*
