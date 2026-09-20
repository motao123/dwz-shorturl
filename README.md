<div align="center">

# 🔗 DWZ ShortURL 短网址平台

**一次生成，随处链接 —— 高性能、高安全、可运营的企业级短链接服务**

![PHP](https://img.shields.io/badge/PHP-8.x-777BB4?style=flat-square&logo=php&logoColor=white)
![Go](https://img.shields.io/badge/Go-1.26-00ADD8?style=flat-square&logo=go&logoColor=white)
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
| 🔁 原子去重 | `url_hash` 唯一索引，**同一会员内**同 URL 复用短码（不同会员各建一条，互不影响） |
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
| 🗃️ **schema_migrations** | 单一迁移入口 `cmd/migrate` + 版本表，覆盖 PHP 侧与 Go 侧全部迁移，幂等可重跑 |
| ⚖️ **对账 Cron** | `short_urls` ↔ `wjoy_log` 双写对账、点击计数校准 |
| 📦 **分区维护** | `click_logs` 按月自动分区、始终超前 2 个月；状态可查（`GET /monitor/partitions`）、失败告警（webhook `system.partition_alert`）、幂等自愈 |
| 💾 **定时备份** | `deploy/backup.sh` mysqldump + 保留策略 |
| 📨 **Webhook 异步队列** | PHP 跳转入队（O(1)）+ `migrations/webhook_worker.php` 后台投递，指数退避重试 |
| 🎨 **静态资源构建** | `php migrations/build_assets.php` 压缩 CSS/JS 并注入内容哈希版本号，零 npm 依赖 |
| 📋 **迁移工具** | 目录扫描自动发现迁移，`-status` / `-dry-run` / `-all` / `-baseline` / `-config` 全参数 |

---

## 🧱 技术栈

| 层 | 技术 |
|----|------|
| 前台 | PHP 8 + MySQL + 原生 JS（零依赖） |
| 核心 | Go 1.26 · Gin · GORM · Redis |
| 管理台 | Vue 3 · TypeScript · Element Plus · Pinia · ECharts |
| 分析 | 自研 ip2region v1 读取器 + ISO 国家映射 + Referer 分类器 |
| 测试 | Go `go test ./...` · 前端 `vitest` + Vue Test Utils · 迁移注册表契约测试 |
| 构建 | Vite 6 双入口分包 · Element Plus 样式按需引入 · 静态资源内容哈希 |
| 部署 | Nginx · systemd · Docker Compose · GitHub Actions |

---

## 🚀 快速开始

### 一键 Docker 部署（推荐，单库模式）

只需填 4 个密码，起来就能用 —— **不需要建两个数据库，也不需要跑 setup.php**：

```bash
git clone https://github.com/motao123/dwz-shorturl.git
cd dwz-shorturl
cp .env.example .env      # 填 MYSQL_ROOT_PASSWORD / DB_PASSWORD / ADMIN_PASSWORD / REDIS_PASSWORD
docker compose up -d
docker compose logs init  # 看初始化结果，会打印管理台地址与登录账号
```

然后浏览器打开 `http://<主机IP>/admin/`，用 `.env` 里的 `ADMIN_USERNAME` / `ADMIN_PASSWORD` 直接登录。

一个 `up -d` 自动完成四件事：

| 步骤 | 说明 |
|---|---|
| ① 建库建表 | 单个数据库，管理表（`short_urls`/`users`…）与公共表（`wjoy_log`/`members`…）同库 |
| ② 执行迁移 | 统一入口 `cmd/migrate` 跑完全部迁移并记录 `schema_migrations`，幂等可重跑 |
| ③ 生成配置 | 自动生成后端 `config.yaml` 与前台 `config.php`，密钥同源、互相对得上 |
| ④ 创建管理员 | 用你给的 `ADMIN_PASSWORD` 建号并授予 `super_admin`，**不留任何默认口令** |

忘记管理台密码时：把 `.env` 里 `RESET_ADMIN_PASSWORD=1`，然后 `docker compose up -d --force-recreate init`。

> 进阶：需要单独拆分数据库/Redis/前后端时，用 `deploy/docker-compose.yml`（分库部署，行为与旧版一致）。

##### 邮件配置（可选，但强烈建议）

不配置 SMTP 的后果是**功能静默不可用**：

- 会员「忘记密码」发不出重置邮件 → 等于死路；
- 注册验证邮件发不出去 → 而批量生成要求邮箱已验证 → **批量功能对自助部署者永久不可用**。

在 `.env` 里补上这几项（其余按需）：

```ini
SMTP_HOST=smtp.qq.com          # 任意 SMTP 服务商
SMTP_PORT=465                  # 465=隐式 TLS，587=STARTTLS
SMTP_USER=noreply@example.com  # 完整邮箱地址
SMTP_PASSWORD=你的SMTP授权码    # 注意是授权码，不是邮箱登录密码
SMTP_FROM=noreply@example.com  # 留空则复用 SMTP_USER
SMTP_FROM_NAME=短网址
SMTP_SSL=true
```

改完执行 `docker compose up -d --force-recreate init` 让 init 重新生成 `config.yaml`（会打印 `📧 SMTP 已配置` 或未配置提醒）。

源码部署则在 `backend/configs/config.yaml` 里填写同名的 `smtp:` 段（见 `config.example.yaml`）。未配置时接口会返回「邮件服务未配置，请联系管理员」，而不是笼统的发送失败。

##### 单库 vs 分库

| | 单库（`docker-compose.yml`） | 分库（`deploy/docker-compose.yml`） |
|---|---|---|
| 数据库数量 | **1 个**，管理表与公共表同库 | 2 个（管理库 + 公共库） |
| 配置项 | 只填主库连接，`admin_db_*` 自动复用 | 需要分别配置两套连接 |
| 初始化 | `init` 容器全自动 | 手工跑迁移 + `setup.php` |
| 适用 | 绝大多数自建场景、宝塔/单机、Docker | 已有独立两库、多站点共用公共库 |

单库模式由 `$admin_db_same_as_main = true`（PHP）与 `public_db.dbname == database.dbname`（Go）表达；迁移工具识别到两者同名时会自动进入单库模式（也可用 `-same-db` 显式指定）。

### Docker Compose 分库部署

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

# 3. 数据库迁移（统一入口：backend/cmd/migrate）
cd backend
go run ./cmd/migrate -status -migrations ../backend/migrations   # 查看全部迁移及状态
go run ./cmd/migrate -all -migrations ../backend/migrations      # 执行管理库 + 公共库全部迁移
go run ./cmd/migrate -all -migrations ../backend/migrations -dry-run   # 只看会执行什么

# 老库升级（推荐）：一条命令完成全部上线迁移
#   备份 → 统一迁移工具 → 静态资源构建
#   幂等，可重复执行；库名留空时会自动从 config.php / config.yaml 读取
DWZ_DB_USER=root DWZ_DB_PASS='密码' ./ops/one_click_migrate.sh --public-db=<公共库> --admin-db=<管理库>

# 只想看会执行什么、不落库：
DWZ_DB_USER=root DWZ_DB_PASS='密码' ./ops/one_click_migrate.sh --public-db=<公共库> --admin-db=<管理库> --dry-run

# 若目标库此前是手工升级的（schema 已到位但没有版本记录），先补录一次：
cd backend && go run ./cmd/migrate -baseline -migrations ../backend/migrations

# 4. PHP 前台（需 PHP 8 + mysqli）
#    注意：迁移工具会在执行任何 DDL 之前先检查 PHP 解释器与 mysqli 扩展，
#    缺失时直接报错退出（而不是跑到一半才崩，留下半迁移的库）。
#    单库部署（推荐）：--single-db 让 config.php 直接复用主库连接，
#    不需要再配第二套 admin_db_*，也没有跨库权限/COLLATE 问题。
php setup.php --host=127.0.0.1 --port=3306 \
  --user=dwz --pwd='密码' --db=dwz \
  --public-url=https://localhost --single-db

#    分库部署（已有独立两库时）：不加 --single-db，随后编辑 config.php 填 admin_db_*

# 5. 创建管理员账号（重要：库表基线不再预置任何默认口令）
cd backend
go run ./cmd/createadmin -username=admin -password='你的强密码'
# 忘记密码时重置：
go run ./cmd/createadmin -username=admin -password='新密码' -reset
```

> ⚠️ 安全提示：`backend/migrations/schema.sql` **不会**创建带默认密码的 `admin`。
> 历史版本曾预置一个固定 bcrypt 哈希，等于给每个全新部署留了一把公开可用的
> 钥匙；现已移除，并由 `cmd/createadmin` / 一键部署的 `init.php` 按你提供的
> 密码建号。仓库内另有回归测试 `TestNoHardcodedAdminCredentialInSchema` 守住这一点。

### 部署到服务器

```bash
DWZ_SERVER=your.host DWZ_USER=root DWZ_PASS='密码' ./deploy.sh
```

一键完成：交叉编译 Go 二进制 → 构建 Vue → 上传 PHP/前端 dist → 重启服务。

> `deploy.sh` 只负责发布代码，**不碰数据库**。数据库侧的上线动作由
> `ops/one_click_migrate.sh` 一条命令完成（见上），两者互不依赖。

#### 三条部署路径怎么选

| 路径 | Web Server | 前台 PHP | 管理台 / 会员中心接口 | 说明 |
|---|---|---|---|---|
| **Docker**（推荐） | 容器内 nginx | ✅ | ✅ 开箱可用 | `docker compose up -d` 已配好全部反代，见上文 |
| **nginx / 宝塔** | nginx | ✅ | ⚠️ **需手工配 4 段反代** | 未配则接口全 404，见 [docs/deploy-baota.md](docs/deploy-baota.md) |
| **Apache** | Apache + .htaccess | ✅ | ⚠️ **需启用 mod_rewrite** | 默认走 `api_proxy.php` 兜底，开箱可用 |

#### Apache 部署（含共享虚拟主机）

`.htaccess` 已内置 Go 接口转发规则，默认走 `api_proxy.php` 兜底（curl 转发到 `127.0.0.1:8080`），**共享虚拟主机无需 `mod_proxy` 即可使用管理台与会员中心接口**。要求：

1. Apache 开启 `AllowOverride All`（至少允许 `FileInfo`）与 `mod_rewrite`；
2. PHP ≥ 8.0 且启用 `mysqli`、`curl` 扩展；
3. Go 后端已启动在 `127.0.0.1:8080`；若地址不同，设置环境变量 `DWZ_BACKEND`。

> 若主机**禁用了 `allow_url_fopen` 之外的一切网络能力**（极少数严格共享主机），或**纯 PHP 形态**（不起 Go 后端），请把 `.htaccess` 中「方案 B」整段注释掉。此时前台 `do.php` / `api.php` / `batch.php` / `member.php` 完全可用，仅管理台「域名池」与会员中心接口不可用。
>
> 若主机启用了 `mod_proxy`，建议改用 `.htaccess` 里的「方案 A」原生反代，少一次 PHP 往返。

**症状自查**：管理台「域名池」下拉框为空、短链永远落在主域名、会员中心登录后接口 404 —— 都指向接口未转发，请先确认上表第 3 列的配置。

### 迁移体系（单一事实来源）

全部迁移集中在 `backend/migrations/`，由 `backend/cmd/migrate` 执行并把版本写入
`schema_migrations`。**清单由目录扫描产出，不再有硬编码数组**：新增迁移文件无需改任何
`.go` 代码。

| 目录 | 版本号 | 目标库 |
|---|---|---|
| `backend/migrations/*.sql` | 文件名（如 `schema.sql`） | 管理库（`short_urls` / `users` / `roles` …） |
| `backend/migrations/php/*` | `php/` 前缀（如 `php/scope_url_hash.sql`） | 公共库（`wjoy_log` / `members` / `webhook_queue` …） |

**执行顺序由文件自己声明**，不依赖目录字典序：

- `schema.sql` / `public_schema.sql` 等基线文件在 Go 里按固定序列排定；
- 其余文件在 SQL 或 PHP 注释里写一行 `-- migrate: after <版本号>`，即插到该锚点之后；
- 没有任何声明的文件仍会被发现、排在最后，并在 `-status` 与执行日志中标记 `unclassified`，
  提醒补位置 —— 新迁移永远不会被静默忽略。

库名不再写死在迁移脚本里：脚本内用 `{{PUBLIC_DB}}` / `{{ADMIN_DB}}` 占位符，
执行时按当前配置注入，因此不再需要「先把 `USE` 的库名改对」这一步。

常用命令：

```bash
cd backend
go run ./cmd/migrate -status -migrations ../backend/migrations   # 全部迁移及已应用/待应用状态
go run ./cmd/migrate -all     -migrations ../backend/migrations  # 两库全部迁移 + 口径校验
go run ./cmd/migrate -only public -migrations ../backend/migrations   # 只处理公共库
go run ./cmd/migrate -baseline -migrations ../backend/migrations  # 手工升级过的库补录版本（不执行 DDL）
go run ./cmd/migrate -same-db -all -migrations ../backend/migrations  # 单库部署：两个角色解析到同一个库
```

**单库模式**：当 `config.yaml` 里 `public_db.dbname` 与 `database.dbname` 相同时，
工具自动判定为单库（等价于显式加 `-same-db`），此时两组迁移都写入那一个库，
版本只记在一张 `schema_migrations` 里 —— 不会出现「一边显示已应用、另一边显示待应用」。
实测覆盖：全新单库安装 → 19 张表 + 1 视图全部落库、幂等重跑 `no pending migrations`、
分库安装行为不变。

> `-baseline` 用于生产库「schema 已经手工改过、但没有 `schema_migrations` 记录」的场景：
> 它只把文件标记为已应用，**不执行任何 DDL**，避免重跑时冲突。
>
> 💡 跨库迁移（同时读写公共库与管理库）通过 `{{ADMIN_DB}}` 占位符引用管理库，
> 两个库的建库字符集不一致时脚本内已显式 `COLLATE`，不会出现
> `ERROR 1267 Illegal mix of collations`。
>
> 💡 部分迁移是 PHP 脚本（如 `php/legacy_schema.php`，需先扫描数据再决定能否建唯一索引），
> 迁移工具会调用 `php` 解释器执行它们并同样记入 `schema_migrations`；
> 可用 `DWZ_PHP_BIN` 指定解释器路径。

### 一条命令完成上线迁移

`ops/one_click_migrate.sh` 把老库升级需要的运维动作收敛成一条命令：

| 步骤 | 动作 |
|---|---|
| 0 | 检查两库连通性、确认库名存在 |
| 1 | 备份 `wjoy_log` 与 `short_urls` 到 `backups/migrate-<时间戳>/`（表不存在时跳过，适配首次安装） |
| 2 | 调用统一迁移工具 `backend/cmd/migrate` 执行全部待应用迁移（`url_hash` 作用域化、`webhook_queue`、`members`、`violation_reviews`、老库 `url_hash` 补列与清洗……） |
| 3 | 构建静态资源并注入内容哈希版本号 |

额外开关：`--baseline`（把目录内全部迁移标记为已应用，不执行 DDL）、`--skip-backup`、`--skip-assets`。

特点：

- **幂等**：中途失败可直接重跑；已完成的迁移记录在 `schema_migrations`，不会重复执行；
- **单一迁移入口**：迁移清单、顺序与版本记录全部由 `backend/cmd/migrate` 决定，脚本不再自己拼 `mysql < xxx.sql`；
- **零手工替换**：库名从 `config.php` / `backend/configs/config.yaml` 自动读取，也可 `--public-db=` / `--admin-db=` 显式指定；库名经 `{{PUBLIC_DB}}` / `{{ADMIN_DB}}` 占位符注入；
- **密码不落命令行**：通过 `MYSQL_PWD` 环境变量传给 mysql/mysqldump，`ps` 与 shell history 不可见；
- **有 `--dry-run`**：先看清要执行什么，再决定是否落库。

```bash
# 先干跑一遍确认
DWZ_DB_USER=root DWZ_DB_PASS='密码' ./ops/one_click_migrate.sh \
  --public-db=1_xk7_cn --admin-db=dwz_admin --dry-run

# 确认无误后正式执行
DWZ_DB_USER=root DWZ_DB_PASS='密码' ./ops/one_click_migrate.sh \
  --public-db=1_xk7_cn --admin-db=dwz_admin
```

> ⚠️ 第 2 步会重写哈希并重建唯一索引。脚本已自动备份，但仍建议挑流量低谷执行，
> 并确保跑的时候没有其他进程在写库。

### 后台任务（cron）

PHP 前台的 webhook 投递、静态资源构建均为可选增强，按需启用：

```bash
# 1) Webhook 异步投递队列（每分钟消费一批；也可 --loop 常驻）
php migrations/webhook_worker.php

# 2) 构建静态资源（压缩 + 内容哈希版本注入 index.html/api.html/stats.php）
php migrations/build_assets.php

# 3) CI/发布前校验产物是否最新（不写文件，过期则非零退出）
php migrations/build_assets.php --check
```

> 💡 `webhook_queue` 表由 `backend/migrations/php/add_webhook_queue.sql` 创建；未建表时程序会自动退化为同步投递并在 `logs/php_error.log` 告警，功能不中断。

---

## 🔐 短链去重作用域（url_hash）

`url_hash` 存的是 `MD5(long_url + 0x1F + scope_key)`，唯一索引语义为「**同一 owner 作用域内唯一**」，而非「URL 全局唯一」：

| scope_key | 含义 | 去重行为 |
|---|---|---|
| `w:0` | 匿名请求 / 历史遗留数据 | 沿用旧的全局语义，同一 URL 只保留一条 |
| `m:<member_id>` | 会员创建的短链 | 同一会员内同一 URL 复用一条；**不同会员各自独立**，可分别设置有效期与访问密码 |

对比改造前（裸 `MD5(url)` 全局唯一）：

- ✅ 同一 URL 可以按会员分别建链，会员各自管自己的有效期/密码；
- ✅ 业务键带上作用域后，哈希空间被区分，不同 URL 不会再因 MD5 碰撞而互相阻塞；
- ✅ 匿名路径行为不变，线上已有数据语义平滑。

> ⚠️ PHP (`includes/function.php` 的 `url_scope_hash()`)、Go (`urlHash()`) 与 SQL (`MD5(CONCAT(url, 0x1F, scope))`) 三处实现必须保持一致，否则同一条短链在两条跳转路径上会落到不同行。迁移脚本 `backend/migrations/php/scope_url_hash.sql` 负责把历史行重写为同一口径。

---

## 📖 文档

| 文档 | 说明 |
|------|------|
| [📘 API 文档](api.html) | 前台接口完整说明 + 错误码 + curl 示例 |
| [🎨 后台设计](docs/BACKEND_ADMIN_DESIGN.md) | 管理后台技术设计文档 |
| [🗺️ 功能路线图](docs/FEATURE_ROADMAP.md) | 5 个 Phase / 10 大模块 / 79.5 人日规划 |
| [🔬 深度分析报告](docs/ANALYSIS_REPORT_2026-08.md) | 功能/UI/交互/架构四维审计 + 13 批修复记录 |
| [🧩 列表页 composable](docs/frontend-list-composables.md) | `useListPage` 等组合式函数：分页/筛选/批量/导出的统一契约与用法 |
| [🧩 分区维护](docs/partition-maintenance.md) | click_logs 月度分区：覆盖目标、告警阈值、幂等补齐与生产注意事项 |
| [🖥️ 管理台使用手册](docs/admin-guide.md) | 面向运营：登录与 2FA、RBAC 角色矩阵、域名池、API 密钥、Webhook、违规复核、审计与监控 |
| [🧰 宝塔 / 单机部署](docs/deploy-baota.md) | 非 Docker 部署：nginx 反代四条规则、Go 后端启动、一键迁移与上线自查清单 |
| [🛡️ 依赖风险与安全扫描](docs/SECURITY_SCAN.md) | PR 增量门禁 / 仓库级降噪 / 存量告警分类：哪些会阻断、哪些只记录 |
| [🖥️ 项目官网](https://motao123.github.io/dwz-shorturl/) | GitHub Pages 宣传站（由 Actions 自动构建，Vercel 极简浅色设计语言，见 site/DESIGN.md） |

---

## 🚦 CI / CD 工作流

### GitHub Actions

| 工作流 | 触发 | 作用 |
|--------|------|------|
| `pages.yml` | push 到 `site/` 或手动 | 构建宣传站并发布到 GitHub Pages |

> 首次启用：仓库 **Settings → Pages → Source 选择 "GitHub Actions"**，之后每次 push `site/` 目录修改都会自动重新发布。

### CNB 云原生构建

| 流水线 | 触发 | 作用 |
|--------|------|------|
| `dependency-risk-check` | PR | **增量**依赖风险门禁：只拦本次 PR 新引入的 High/Critical 漏洞与高风险 License，存量告警不阻断 |

配套机制：

- **依赖自动升级** `.github/dependabot.yml`：每周对 Go / npm minor+patch 分组提 PR（major 需人工评估）；
- **仓库级扫描降噪** `.scanignore` + `.cnb/security/code_scan_config.yml`：排除第三方代码、构建产物、示例配置等确定无攻击面路径。

> 设计取舍与存量告警分类见 [🛡️ 依赖风险与安全扫描](docs/SECURITY_SCAN.md)。
> `dependency-risk-check` 对所有分支的 PR 生效（含 `master` 与 `auto/*` 特性分支）。

### 本地自查

提交前先跑一遍，避免 CI 往返：

```bash
# Go：构建 / 静态检查 / 单测
cd backend && go build ./... && go vet ./... && go test ./...

# 前端：构建 / 单测 / 依赖漏洞
cd frontend && npm run build && npm test && npm audit

# PHP：语法检查 + 静态资源产物是否最新
# ⚠️ 顺序很重要：首次需先 build 生成 assets/*.min.css|min.js 与版本号，再 check；
#    只跑 --check 会因为没有产物而失败（这不是代码问题）。
php migrations/build_assets.php          # 首次 / 改动 assets 下的源文件后
php -l api.php && php migrations/build_assets.php --check   # 提交前校验
```

> 单测覆盖：统计参数与点击计数、短链批量下标对齐、迁移注册表契约、GeoIP/SSRF/限流、分区维护；
> 前端覆盖列表页 composable（分页筛选/批量操作/竞态防护）。

---

## 🤝 参与贡献

- 🐛 提 Bug：打开 [Issues](https://github.com/motao123/dwz-shorturl/issues)
- 💡 提需求：说明业务场景 + 期望效果
- 🔀 提交代码：Fork → 分支 → PR，遵循既有代码风格
- ✅ 提交前自检：跑通上面的 [本地自查](#本地自查) 命令，PR 描述里说明验证方式
- 📚 改动约定：架构/迁移/分区等机制性变更，请同步更新 README 与 `docs/` 对应文档

---

## 📄 许可证

[MIT License](LICENSE) · 作者：**陌涛**

使用与转载请保留署名与项目说明。生产部署请使用强密码并妥善保管数据库凭据：管理台密码由 `ADMIN_PASSWORD`（Docker）或 `setup.php` 交互输入设置，**基线 schema 不预置任何默认口令**；忘记时用 `RESET_ADMIN_PASSWORD=1` 重启 init 容器，或运行 `go run ./cmd/createadmin -reset` 重置。

<div align="center">

**⭐ 如果这个项目帮到了你，欢迎 Star 支持！**

</div>
