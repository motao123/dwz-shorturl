<div align="center">

# 🔗 DWZ ShortURL 短网址平台

**一次生成，随处链接 —— 高性能、高安全、可运营的企业级短链接服务**

![PHP](https://img.shields.io/badge/PHP-8.x-777BB4?style=flat-square&logo=php&logoColor=white)
![Go](https://img.shields.io/badge/Go-1.22-00ADD8?style=flat-square&logo=go&logoColor=white)
![Vue3](https://img.shields.io/badge/Vue-3.x-42B883?style=flat-square&logo=vuedotjs&logoColor=white)
![Element Plus](https://img.shields.io/badge/Element_Plus-2.x-409EFF?style=flat-square&logo=element&logoColor=white)
![MySQL](https://img.shields.io/badge/MySQL-8.0-4479A1?style=flat-square&logo=mysql&logoColor=white)
![Redis](https://img.shields.io/badge/Redis-7-FF4438?style=flat-square&logo=redis&logoColor=white)
![License](https://img.shields.io/badge/License-MIT-16A34A?style=flat-square)

**🖥️ 项目官网 · GitHub Pages：<https://motao123.github.io/dwz-shorturl/>**

</div>

---

## ✨ 这是什么？

DWZ 短网址平台是一套 **PHP 前台 + Go 核心 + Vue3 管理台** 的三层混合架构短链接系统，从普通短链生成到企业级安全管控、流量洞察、自动化运维全覆盖：

- 🚀 **双引擎跳转**：`do.php`（PHP 轻量前台）与 `/r/:code`（Go 高性能网关）双路径，Nginx 智能分流
- 🛡️ **企业级安全**：SSRF 双向防护、2FA (TOTP)、JWT 刷新令牌、链接密码保护、违规内容检测
- 📊 **多维分析**：GeoIP 地域分布（自研 ip2region 读取器）、Referer 来源归类、时段热力图
- 🏢 **运营后台**：RBAC 4 级角色 22 权限点、审计日志、API 密钥、Webhook、域名池负载均衡
- 🔧 **自动化运维**：schema_migrations 版本管理、点击队列背压、双写对账 Cron、自动分区、定时备份

---

## 🏗️ 系统架构

```
                  ┌─────────────┐   ┌─────────────┐   ┌─────────────┐
                  │  ☁️ 访客浏览器 │   │  🎛️ 管理后台  │   │  👤 会员中心  │
                  │  PHP 首页     │   │  Vue3 + EP   │   │  Vue3 SPA   │
                  └──────┬──────┘   └──────┬──────┘   └──────┬──────┘
                         │                  │                  │
                         ▼                  ▼                  ▼
                  ┌────────────────────────────────────────────────┐
                  │              🌐 Nginx 反向代理                  │
                  └───┬──────────────┬────────────────┬───────────┘
                      │ /do.php      │ /admin/api     │ /r/:code
                      ▼              ▼                ▼
               ┌────────────┐  ┌──────────────────────────┐  ┌──────────────┐
               │  PHP 前台   │  │   ⚙️ Go 后端 (Gin + GORM)  │  │  跳转网关     │
               │ 限流/SSRF  │  │ JWT/RBAC/2FA/Cron/Webhook │  │ GeoIP/Referer│
               └─────┬──────┘  └────┬────┬────┬────┬──────┘  └──────┬───────┘
                     │              │    │    │    │                │
                     ▼              ▼    ▼    ▼    ▼                ▼
               ┌────────┐    ┌────────┐ ┌──────┐ ┌────────┐  ┌──────────┐
               │ MySQL  │◄──►│ MySQL  │ │ Redis│ │MySQL   │  │ ip2region│
               │wjoy_log│ 双写│ dwz    │ │JWT黑名│ │click_  │  │ GeoIP 库 │
               │ 对账    │     │ admin  │ │单/限流│ │logs 分区│  └──────────┘
               └────────┘    └────────┘ └──────┘ └────────┘
```

**双写对账机制**：PHP 前台写 `wjoy_log`，Go 后台写 `short_urls`，由 Cron 任务定期对账合并，点击计数双向校准 —— 前后台数据永远一致。

---

## 🎯 功能矩阵

### 🧩 前台用户端（PHP + 原生 JS）

| 能力 | 说明 |
|------|------|
| ⚡ 单条/批量生成 | 单条秒出，批量最多 100 条/次，实时计数 |
| 🎨 自定义短码 | 6–8 位 `a-z0-5`，可续期、可回收 |
| 📱 本地二维码 | 不依赖第三方服务，隐私无忧 |
| 🔁 原子去重 | `url_hash` 唯一索引，同 URL 复用短码 |
| ⏰ 有效期管理 | 永久/1/7/30/365 天，过期返回 410 |
| 📊 点击统计 | 独立 Token 统计页，总量/Top10/最近 20 |

### 🎛️ 管理后台（Go + Vue3 + Element Plus）

| 能力 | 说明 |
|------|------|
| 🔐 认证体系 | JWT 双令牌 + Redis 登出黑名单 + **TOTP 2FA** |
| 👥 RBAC 权限 | 4 级角色、22 个权限点、树形授权 |
| 🔗 短链管理 | 列表/搜索/筛选/分页/CRUD/批量/**CSV 导出**/回收站恢复 |
| 📈 数据统计 | ECharts 概览卡片/趋势折线/Top10 柱状 |
| 🛰️ 域名池 | 多域名短链 + 负载均衡 + DNS/SSL 健康检测 |
| 🧾 审计日志 | 全操作记录 + JSON 详情 + 多维筛选 |
| 🔑 API 密钥 | SHA-256 哈希存储、用量统计、吊销管理 |
| 📡 Webhook | link.created/clicked/expired/deleted 事件投递 + 重试 |
| 🚨 违规检测 | 菠菜/仿冒/恶意关键词库，blocked/review/passed 三态 |
| 🖥️ 实时监控 | 系统健康面板（DB 连接池/Redis/Goroutine） |

### 🛡️ 安全体系

| 防线 | 实现 |
|------|------|
| **SSRF 防护** | DNS 解析 + 拨号层私网/元数据地址阻断（`169.254.169.254` 等） |
| **链接密码** | 共享 HMAC Cookie 校验，访问需输入密码 |
| **限流** | 单 IP 窗口限流，Redis/文件双实现，支持可信代理 |
| **输入校验** | 短码正则、URL 协议白名单、Go 侧深度校验 |
| **凭据隔离** | 生产配置 `config.yaml`/`config.php` 全部 gitignored，密钥轮换支持 |

### 🔧 自动化运维

| 能力 | 说明 |
|------|------|
| 🗃️ **schema_migrations** | `cmd/migrate` 工具 + `schema_migrations` 版本表，幂等迁移 |
| ⚖️ **对账 Cron** | `short_urls` ↔ `wjoy_log` 双写对账、点击计数校准 |
| 📦 **分区维护** | `click_logs` 按月自动分区，防单表膨胀 |
| 💾 **定时备份** | `deploy/backup.sh` mysqldump + 保留策略 |
| 📋 **迁移工具** | `-status` / `-dry-run` / `-migrations` / `-config` 全参数 |

---

## 🧱 技术栈

| 层 | 技术 |
|----|------|
| 前台 | PHP 8 + MySQL + 原生 JS（零依赖） |
| 核心 | Go 1.22 · Gin · GORM · Redis |
| 管理台 | Vue 3 · TypeScript · Element Plus · Pinia · ECharts |
| 分析 | 自研 ip2region v1 读取器 + ISO 国家映射 + Referer 分类器 |
| 部署 | Nginx · systemd · Docker Compose · GitHub Actions |

---

## 🚀 快速开始

### Docker Compose（推荐）

```bash
git clone https://github.com/motao123/dwz-shorturl.git
cd dwz-shorturl
cp deploy/.env.example deploy/.env   # 填写数据库/Redis/JWT 配置
docker compose -f deploy/docker-compose.yml up -d
```

### 本地开发

```bash
# 1. 后端（Go）
cd backend
cp configs/config.example.yaml configs/config.yaml   # 填数据库/Redis 配置
go run ./cmd/server

# 2. 前端管理台（Vue3）
cd frontend
npm install
npm run dev        # http://localhost:5173

# 3. 数据库迁移
cd backend
go run ./cmd/migrate -status          # 查看待执行迁移
go run ./cmd/migrate                  # 执行全部幂等迁移

# 4. PHP 前台（需 PHP 8 + mysqli）
php setup.php --host=127.0.0.1 --port=3306 \
  --user=dwz --pwd='密码' --db=dwz_admin \
  --public-url=https://localhost
```

### 部署到服务器

```bash
DWZ_SERVER=your.host DWZ_USER=root DWZ_PASS='密码' ./deploy.sh
```

一键完成：交叉编译 Go 二进制 → 构建 Vue → 上传 PHP/前端 dist → 重启服务。

---

## 📖 文档

| 文档 | 说明 |
|------|------|
| [📘 API 文档](api.html) | 前台接口完整说明 + 错误码 + curl 示例 |
| [🎨 后台设计](docs/BACKEND_ADMIN_DESIGN.md) | 管理后台技术设计文档 |
| [🗺️ 功能路线图](docs/FEATURE_ROADMAP.md) | 5 个 Phase / 10 大模块 / 79.5 人日规划 |
| [🔬 深度分析报告](docs/ANALYSIS_REPORT_2026-08.md) | 功能/UI/交互/架构四维审计 + 13 批修复记录 |
| [🖥️ 项目官网](https://motao123.github.io/dwz-shorturl/) | GitHub Pages 宣传站（由 Actions 自动构建） |

---

## 🚦 GitHub Actions 工作流

| 工作流 | 触发 | 作用 |
|--------|------|------|
| `pages.yml` | push 到 `site/` 或手动 | 构建宣传站并发布到 GitHub Pages |

> 首次启用：仓库 **Settings → Pages → Source 选择 "GitHub Actions"**，之后每次 push `site/` 目录修改都会自动重新发布。

---

## 🤝 参与贡献

- 🐛 提 Bug：打开 [Issues](https://github.com/motao123/dwz-shorturl/issues)
- 💡 提需求：说明业务场景 + 期望效果
- 🔀 提交代码：Fork → 分支 → PR，遵循既有代码风格

---

## 📄 许可证

[MIT License](LICENSE) · 作者：**陌涛**

使用与转载请保留署名与项目说明。生产部署请务必修改默认密码（`admin123`）并妥善保管数据库凭据。

<div align="center">

**⭐ 如果这个项目帮到了你，欢迎 Star 支持！**

</div>
