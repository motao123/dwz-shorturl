# 短网址后台管理系统 — 技术设计文档

> 版本：v1.0 | 日期：2026-07-27 | 状态：设计稿

---

## 目录

1. [项目概述与业务分析](#1-项目概述与业务分析)
2. [技术栈选型](#2-技术栈选型)
3. [系统架构设计](#3-系统架构设计)
4. [数据库表结构设计](#4-数据库表结构设计)
5. [后台 API 接口清单](#5-后台-api-接口清单)
6. [权限管理体系](#6-权限管理体系)
7. [部署运维方案](#7-部署运维方案)
8. [迁移方案](#8-迁移方案)

---

## 1. 项目概述与业务分析

### 1.1 当前系统业务逻辑梳理

当前系统为 PHP 单体应用，核心业务包括：

| 模块 | 文件 | 职责 |
|------|------|------|
| 单条短链生成 | `api.php` | 接收 URL，校验 SSRF，生成/复用短码，写入数据库 |
| 批量短链生成 | `batch.php` | 批量处理最多 100 条 URL，逐条调用生成逻辑 |
| 短链跳转 | `do.php` | 根据短码查询目标 URL，校验有效期和安全性，302 重定向 |
| 统计展示 | `stats.php` | Token 认证的只读统计页，展示总量/Top10/最近20 |
| 公共逻辑 | `includes/` | 数据库封装、SSRF 校验、限流、短码算法、配置加载 |

**数据模型**：单表 `wjoy_log`，字段包括 `uid`(短码)、`longurl`(目标URL)、`url_hash`(MD5去重)、`clicks`(点击数)、`expire_at`(过期时间)。

**安全机制**：POST-only 接口、文件级速率限制、SSRF 双向校验（创建+跳转）、可信代理 IP 白名单、统计页 Token 认证。

### 1.2 前端交互流程分析

```
用户访问首页 (index.html)
  │
  ├─ 单条生成流程
  │   ├─ 输入 URL（必填）
  │   ├─ 可选：自定义短码、有效期
  │   ├─ 点击「生成短链接」
  │   ├─ JS POST → api.php
  │   ├─ 成功 → 弹窗展示短链 + 复制 + 二维码
  │   └─ 失败 → Toast 错误提示
  │
  └─ 批量生成流程
      ├─ 输入多行 URL（每行一个）
      ├─ 实时计数 0/100
      ├─ 点击「批量生成」
      ├─ JS POST → batch.php
      └─ 成功 → 列表展示每条结果 + 独立复制按钮
```

### 1.3 后台管理需求定义

| 需求类别 | 功能点 |
|----------|--------|
| 短链管理 | 列表查询（分页/搜索/筛选）、详情查看、编辑（目标URL/有效期/短码）、删除/批量删除、手动创建、导出 CSV |
| 统计分析 | 实时概览仪表盘、点击趋势图（按小时/天/月）、地域分布（预留）、Top N 排行、自定义时间范围查询 |
| 用户管理 | 管理员账户 CRUD、角色分配、登录日志 |
| 权限控制 | RBAC 角色模型、接口级权限、操作审计 |
| 系统配置 | 全局参数在线编辑（限流阈值、SSRF 白名单、统计开关等） |
| 开放 API | API Key 管理、调用量统计、独立限流配额 |
| 安全运维 | 操作审计日志、异常告警（预留）、数据备份管理 |

---

## 2. 技术栈选型

### 2.1 后端框架

| 方案 | 优势 | 劣势 | 推荐度 |
|------|------|------|--------|
| **Go (Gin/Echo)** | 高性能、低内存、原生并发、编译型部署简单 | 生态较 PHP 年轻、ORM 不够成熟 | ⭐⭐⭐⭐⭐ |
| Node.js (Fastify) | 异步 I/O 适合高并发跳转、npm 生态丰富 | 长期维护成本、内存泄漏风险 | ⭐⭐⭐⭐ |
| PHP (Laravel) | 与现有代码同语言、迁移成本低 | 性能瓶颈、FPM 进程模型限制并发 | ⭐⭐⭐ |

**推荐：Go + Gin**

理由：
- 短链跳转是高频读操作（302 重定向），Go 的零分配路由和原生 HTTP 性能远优于 PHP-FPM
- 单二进制部署，无需 PHP-FPM/Nginx FastCGI 桥接
- goroutine 模型天然适合并发点击日志写入
- 与现有 PHP 前端生成接口可并行运行，渐进迁移

### 2.2 数据库

| 方案 | 适用场景 | 推荐度 |
|------|----------|--------|
| **MySQL 8.0** | 与现有系统兼容、运维成熟、JSON 支持 | ⭐⭐⭐⭐⭐ |
| PostgreSQL 15 | 更丰富的数据类型、分区表更灵活 | ⭐⭐⭐⭐ |

**推荐：MySQL 8.0**（平滑迁移现有数据，团队熟悉度高）

### 2.3 缓存

**Redis 7** — 用途：
- 热点短码 → 目标 URL 映射（跳转加速，TTL 可配）
- 速率限制计数器（替代当前文件锁方案，支持多实例）
- Session / Token 黑名单
- 统计数据预聚合缓存

### 2.4 消息队列（可选，V2 引入）

**Redis Stream** — 用途：
- 点击日志异步写入（解耦跳转响应与 DB 写入）
- 批量任务异步处理

### 2.5 前端管理面板

**Vue 3 + Element Plus + Vite**

理由：
- 国内生态成熟，中文文档完善
- Element Plus 提供完整的后台组件（表格/表单/图表/权限指令）
- Vite 构建速度快，开发体验好
- 可独立部署为 SPA，通过 Nginx 与后端 API 同域

---

## 3. 系统架构设计

### 3.1 整体架构

```
┌─────────────────────────────────────────────────────────┐
│                      接入层 (Nginx)                       │
│  SSL终止 / 静态资源 / 反向代理 / 限流 / WAF              │
└────────────────────┬────────────────────────────────────┘
                     │
        ┌────────────┼────────────┐
        ▼            ▼            ▼
┌──────────┐  ┌──────────┐  ┌──────────┐
│ 前端 SPA  │  │ 公开 API  │  │ 管理 API  │
│ (Vue 3)  │  │ (PHP/Go) │  │  (Go)    │
└──────────┘  └─────────┘  └─────┬────┘
                    │              │
                    ▼              ▼
            ┌──────────────────────────┐
            │      应用服务层 (Go)       │
            │  短链服务 / 统计服务 /     │
            │  用户服务 / 配置服务       │
            └───────────┬──────────────┘
                        │
           ┌────────────┼────────────┐
           ▼            ▼            ▼
    ┌──────────┐  ┌──────────┐  ┌──────────
    │  MySQL   │  │  Redis   │  │  对象存储  │
    │  8.0     │  │  7       │  │ (可选)    │
    └──────────┘  └──────────┘  └──────────┘
```

### 3.2 核心模块划分

| 模块 | 职责 | 依赖 |
|------|------|------|
| **短链管理模块** | CRUD、批量操作、短码生成、去重、过期管理 | DB、Redis |
| **跳转服务模块** | 短码解析、缓存查询、SSRF 二次校验、302 重定向、点击计数 | Redis、DB、MQ |
| **统计分析模块** | 实时聚合、趋势计算、Top N、数据导出 | DB、Redis |
| **用户与权限模块** | 注册/登录、JWT 签发、RBAC 校验 | DB、Redis |
| **系统配置模块** | 全局参数 CRUD、热加载 | DB、Redis |
| **审计日志模块** | 操作记录、登录记录、异常记录 | DB |
| **开放 API 模块** | API Key 管理、独立限流、调用统计 | DB、Redis |

### 3.3 数据流转

**短链创建流程：**
```
客户端 → Nginx → 管理API/公开API
  → 参数校验 → SSRF校验 → 去重查询(url_hash)
  → [已存在] 返回已有短码
  → [不存在] 生成短码 → 写入DB → 写入Redis缓存
  → 记录审计日志 → 返回结果
```

**短链跳转流程：**
```
客户端 → Nginx → 跳转服务
  → 查询Redis缓存(短码→目标URL)
  → [命中] 直接302
  → [未命中] 查询DB → 校验有效期 → 校验SSRF
  → 写入Redis缓存(TTL) → 异步写入点击日志(MQ)
  → 302重定向
```

---

## 4. 数据库表结构设计

### 4.1 设计规范

- **命名**：表名 `snake_case` 复数形式；字段名 `snake_case`
- **主键**：`BIGINT UNSIGNED AUTO_INCREMENT` 或 `UUID v7`（有序）
- **时间字段**：`created_at`、`updated_at` 使用 `DATETIME(3)` 精确到毫秒
- **软删除**：`deleted_at DATETIME(3) NULL`，NULL 表示未删除
- **字符集**：`utf8mb4` + `utf8mb4_unicode_ci`
- **引擎**：InnoDB
- **审计字段**：每张业务表包含 `created_by`、`updated_by`

### 4.2 核心表设计

#### users — 管理员账户

```sql
CREATE TABLE users (
  id            BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  username      VARCHAR(32)  NOT NULL,
  email         VARCHAR(128) NOT NULL,
  password_hash VARCHAR(255) NOT NULL COMMENT 'bcrypt/argon2id',
  display_name  VARCHAR(64)  NULL,
  avatar_url    VARCHAR(512) NULL,
  status        TINYINT      NOT NULL DEFAULT 1 COMMENT '1=active 0=disabled',
  last_login_at DATETIME(3)  NULL,
  last_login_ip VARCHAR(45)  NULL,
  created_at    DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at    DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  deleted_at    DATETIME(3)  NULL,
  UNIQUE KEY uk_username (username),
  UNIQUE KEY uk_email (email),
  KEY idx_status (status),
  KEY idx_deleted_at (deleted_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
```

#### roles — 角色

```sql
CREATE TABLE roles (
  id          BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  name        VARCHAR(32)  NOT NULL COMMENT '唯一标识如 super_admin',
  display_name VARCHAR(64) NOT NULL,
  description VARCHAR(255) NULL,
  is_system   TINYINT      NOT NULL DEFAULT 0 COMMENT '系统内置角色不可删除',
  created_at  DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at  DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  UNIQUE KEY uk_name (name)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
```

#### permissions — 权限

```sql
CREATE TABLE permissions (
  id          BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  resource    VARCHAR(64)  NOT NULL COMMENT '资源标识如 short_urls',
  action      VARCHAR(32)  NOT NULL COMMENT '操作如 create/read/update/delete',
  description VARCHAR(255) NULL,
  UNIQUE KEY uk_resource_action (resource, action)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
```

#### role_permissions — 角色权限关联

```sql
CREATE TABLE role_permissions (
  role_id       BIGINT UNSIGNED NOT NULL,
  permission_id BIGINT UNSIGNED NOT NULL,
  PRIMARY KEY (role_id, permission_id),
  KEY idx_permission (permission_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
```

#### user_roles — 用户角色关联

```sql
CREATE TABLE user_roles (
  user_id BIGINT UNSIGNED NOT NULL,
  role_id BIGINT UNSIGNED NOT NULL,
  PRIMARY KEY (user_id, role_id),
  KEY idx_role (role_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
```

#### short_urls — 短链主表（扩展现有 wjoy_log）

```sql
CREATE TABLE short_urls (
  id          BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  uid         VARCHAR(16)    NOT NULL COMMENT '短码',
  long_url    TEXT           NOT NULL COMMENT '目标URL',
  url_hash    CHAR(32)       NOT NULL COMMENT 'MD5去重',
  title       VARCHAR(255)   NULL COMMENT '用户自定义标题',
  category_id BIGINT UNSIGNED NULL COMMENT '分组ID',
  clicks      INT UNSIGNED   NOT NULL DEFAULT 0,
  status      TINYINT        NOT NULL DEFAULT 1 COMMENT '1=active 0=disabled 2=expired',
  expire_at   DATETIME(3)    NULL COMMENT 'NULL=永久',
  created_by  BIGINT UNSIGNED NULL COMMENT '创建者用户ID，NULL=匿名',
  source      VARCHAR(16)    NOT NULL DEFAULT 'web' COMMENT 'web/api/batch/admin',
  ip          VARCHAR(45)    NULL COMMENT '创建者IP',
  created_at  DATETIME(3)    NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at  DATETIME(3)    NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  deleted_at  DATETIME(3)    NULL,
  UNIQUE KEY uk_uid (uid),
  UNIQUE KEY uk_url_hash (url_hash),
  KEY idx_status_expire (status, expire_at),
  KEY idx_clicks (clicks DESC),
  KEY idx_created_at (created_at DESC),
  KEY idx_created_by (created_by),
  KEY idx_category (category_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
```

#### url_categories — 短链分组

```sql
CREATE TABLE url_categories (
  id          BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  name        VARCHAR(64)  NOT NULL,
  color       VARCHAR(7)   NULL COMMENT '#hex颜色',
  sort_order  INT          NOT NULL DEFAULT 0,
  created_by  BIGINT UNSIGNED NOT NULL,
  created_at  DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at  DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  deleted_at  DATETIME(3)  NULL,
  KEY idx_sort (sort_order)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
```

#### click_logs — 点击明细（按月分区）

```sql
CREATE TABLE click_logs (
  id          BIGINT UNSIGNED AUTO_INCREMENT,
  short_url_id BIGINT UNSIGNED NOT NULL,
  ip          VARCHAR(45)    NOT NULL,
  user_agent  VARCHAR(512)   NULL,
  referer     VARCHAR(512)   NULL,
  country     VARCHAR(2)     NULL COMMENT 'ISO 3166-1 alpha-2',
  created_at  DATETIME(3)    NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id, created_at),
  KEY idx_short_url (short_url_id, created_at),
  KEY idx_created_at (created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
PARTITION BY RANGE (TO_DAYS(created_at)) (
  PARTITION p202607 VALUES LESS THAN (TO_DAYS('2026-08-01')),
  PARTITION p202608 VALUES LESS THAN (TO_DAYS('2026-09-01')),
  PARTITION p_future VALUES LESS THAN MAXVALUE
);
```

#### audit_logs — 操作审计

```sql
CREATE TABLE audit_logs (
  id          BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  user_id     BIGINT UNSIGNED NULL COMMENT 'NULL=系统操作',
  action      VARCHAR(64)    NOT NULL COMMENT '如 short_url.create',
  resource    VARCHAR(64)    NULL,
  resource_id VARCHAR(64)    NULL,
  detail      JSON           NULL COMMENT '操作详情快照',
  ip          VARCHAR(45)    NOT NULL,
  user_agent  VARCHAR(255)   NULL,
  created_at  DATETIME(3)    NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  KEY idx_user (user_id, created_at DESC),
  KEY idx_action (action, created_at DESC),
  KEY idx_created_at (created_at DESC)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
```

#### system_configs — 系统配置 KV

```sql
CREATE TABLE system_configs (
  id          BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  config_key  VARCHAR(64)    NOT NULL,
  config_value TEXT          NOT NULL,
  value_type  VARCHAR(16)    NOT NULL DEFAULT 'string' COMMENT 'string/int/bool/json',
  description VARCHAR(255)   NULL,
  is_public   TINYINT        NOT NULL DEFAULT 0 COMMENT '是否可被前端读取',
  updated_by  BIGINT UNSIGNED NULL,
  updated_at  DATETIME(3)    NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  UNIQUE KEY uk_key (config_key)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
```

#### api_keys — 开放 API 密钥

```sql
CREATE TABLE api_keys (
  id          BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  user_id     BIGINT UNSIGNED NOT NULL,
  name        VARCHAR(64)    NOT NULL COMMENT '密钥用途描述',
  key_prefix  VARCHAR(8)     NOT NULL COMMENT '前8位明文，用于展示',
  key_hash    CHAR(64)       NOT NULL COMMENT 'SHA-256完整哈希',
  permissions JSON           NULL COMMENT '允许的接口范围',
  rate_limit  INT            NOT NULL DEFAULT 100 COMMENT '每分钟限额',
  last_used_at DATETIME(3)   NULL,
  expires_at  DATETIME(3)    NULL,
  status      TINYINT        NOT NULL DEFAULT 1 COMMENT '1=active 0=revoked',
  created_at  DATETIME(3)    NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  deleted_at  DATETIME(3)    NULL,
  UNIQUE KEY uk_key_hash (key_hash),
  KEY idx_user (user_id),
  KEY idx_prefix (key_prefix)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
```

### 4.3 索引策略

| 表 | 策略 |
|----|------|
| short_urls | `uk_uid` 覆盖跳转查询；`uk_url_hash` 覆盖去重查询；`idx_clicks DESC` 覆盖 Top N |
| click_logs | 按月 RANGE 分区，分区键 `created_at`；冷数据归档到对象存储 |
| audit_logs | 仅追加写入，按 `created_at` 定期清理 90 天前数据 |
| 所有表 | 避免在 TEXT/BLOB 列建索引；长 URL 通过 `url_hash` 间接索引 |

---

## 5. 后台 API 接口清单

### 5.1 认证接口

| Method | Path | 说明 | 权限 |
|--------|------|------|------|
| POST | `/admin/api/auth/login` | 用户名密码登录，返回 JWT | 公开 |
| POST | `/admin/api/auth/refresh` | 刷新 Access Token | 已认证 |
| POST | `/admin/api/auth/logout` | 登出，Token 加入黑名单 | 已认证 |
| GET | `/admin/api/auth/me` | 获取当前用户信息+权限列表 | 已认证 |

**登录入参：**
```json
{"username": "admin", "password": "xxx", "captcha": "xxx", "captcha_id": "xxx"}
```

**登录出参：**
```json
{
  "access_token": "eyJ...",
  "refresh_token": "eyJ...",
  "expires_in": 7200,
  "user": {"id": 1, "username": "admin", "roles": ["super_admin"]}
}
```

### 5.2 短链管理接口

| Method | Path | 说明 | 权限 |
|--------|------|------|------|
| GET | `/admin/api/short-urls` | 分页列表（支持搜索/筛选/排序） | `short_urls.read` |
| GET | `/admin/api/short-urls/:id` | 详情 | `short_urls.read` |
| POST | `/admin/api/short-urls` | 创建短链 | `short_urls.create` |
| PUT | `/admin/api/short-urls/:id` | 编辑（URL/有效期/标题/分组/状态） | `short_urls.update` |
| DELETE | `/admin/api/short-urls/:id` | 软删除 | `short_urls.delete` |
| POST | `/admin/api/short-urls/batch-delete` | 批量删除 | `short_urls.delete` |
| POST | `/admin/api/short-urls/batch-create` | 批量创建 | `short_urls.create` |
| GET | `/admin/api/short-urls/export` | 导出 CSV | `short_urls.export` |

**列表查询参数：**
```
page=1&per_page=20&keyword=xxx&status=1&category_id=5
&date_from=2026-01-01&date_to=2026-07-27&sort=created_at&order=desc
```

### 5.3 统计接口

| Method | Path | 说明 | 权限 |
|--------|------|------|------|
| GET | `/admin/api/stats/overview` | 概览（总数/今日新增/今日点击/活跃率） | `stats.read` |
| GET | `/admin/api/stats/trend` | 点击趋势（按小时/天/月聚合） | `stats.read` |
| GET | `/admin/api/stats/top` | Top N 短链 | `stats.read` |
| GET | `/admin/api/stats/recent` | 最近创建列表 | `stats.read` |
| GET | `/admin/api/stats/export` | 统计数据导出 | `stats.export` |

### 5.4 用户管理接口

| Method | Path | 说明 | 权限 |
|--------|------|------|------|
| GET | `/admin/api/users` | 用户列表 | `users.read` |
| POST | `/admin/api/users` | 创建用户 | `users.create` |
| PUT | `/admin/api/users/:id` | 编辑用户 | `users.update` |
| DELETE | `/admin/api/users/:id` | 禁用/删除用户 | `users.delete` |
| PUT | `/admin/api/users/:id/password` | 重置密码 | `users.update` |
| PUT | `/admin/api/users/:id/roles` | 分配角色 | `users.assign_roles` |

### 5.5 角色权限接口

| Method | Path | 说明 | 权限 |
|--------|------|------|------|
| GET | `/admin/api/roles` | 角色列表 | `roles.read` |
| POST | `/admin/api/roles` | 创建角色 | `roles.create` |
| PUT | `/admin/api/roles/:id` | 编辑角色 | `roles.update` |
| DELETE | `/admin/api/roles/:id` | 删除角色 | `roles.delete` |
| PUT | `/admin/api/roles/:id/permissions` | 设置角色权限 | `roles.update` |
| GET | `/admin/api/permissions` | 全部权限列表 | `roles.read` |

### 5.6 系统配置接口

| Method | Path | 说明 | 权限 |
|--------|------|------|------|
| GET | `/admin/api/configs` | 配置列表 | `configs.read` |
| PUT | `/admin/api/configs` | 批量更新配置 | `configs.update` |

### 5.7 审计日志接口

| Method | Path | 说明 | 权限 |
|--------|------|------|------|
| GET | `/admin/api/audit-logs` | 审计日志列表（分页/筛选） | `audit.read` |
| GET | `/admin/api/audit-logs/:id` | 日志详情 | `audit.read` |

### 5.8 开放 API 密钥管理

| Method | Path | 说明 | 权限 |
|--------|------|------|------|
| GET | `/admin/api/api-keys` | 密钥列表 | `api_keys.read` |
| POST | `/admin/api/api-keys` | 创建密钥（返回明文一次） | `api_keys.create` |
| DELETE | `/admin/api/api-keys/:id` | 吊销密钥 | `api_keys.revoke` |
| GET | `/admin/api/api-keys/:id/stats` | 密钥调用统计 | `api_keys.read` |

---

## 6. 权限管理体系

### 6.1 认证方案

```
登录成功
  → 签发 Access Token (JWT, 有效期 2h)
  → 签发 Refresh Token (JWT, 有效期 7d, 存 Redis)
  → 前端存储于 httpOnly Cookie 或 localStorage

请求携带 Authorization: Bearer <access_token>
  → 中间件校验签名 + 过期 + 黑名单
  → 解析 user_id + roles → 注入 context

Access Token 过期
  → 前端用 Refresh Token 调用 /auth/refresh
  → 服务端校验 Refresh Token 有效性 + 轮转
```

**JWT Payload：**
```json
{
  "sub": 1,
  "username": "admin",
  "roles": ["super_admin"],
  "iat": 1722100000,
  "exp": 1722107200,
  "jti": "uuid-v4"
}
```

### 6.2 RBAC 角色模型

| 角色 | 权限范围 | 说明 |
|------|----------|------|
| `super_admin` | 全部 | 系统内置，不可删除 |
| `admin` | 短链管理 + 统计 + 配置 + API Key | 日常运营 |
| `operator` | 短链 CRUD + 统计只读 | 内容运营 |
| `viewer` | 全部只读 | 审计/观察 |

### 6.3 接口级权限控制

```go
// 中间件伪代码
func RequirePermission(resource, action string) gin.HandlerFunc {
    return func(c *gin.Context) {
        perms := c.GetStringSlice("permissions")
        required := resource + "." + action
        if !contains(perms, required) && !contains(perms, "*") {
            c.AbortWithStatusJSON(403, gin.H{"code": 40301, "msg": "权限不足"})
            return
        }
        c.Next()
    }
}

// 路由注册
router.PUT("/short-urls/:id", RequirePermission("short_urls", "update"), handler.UpdateShortUrl)
```

### 6.4 数据级权限（V2 预留）

- 多租户场景下，通过 `tenant_id` 字段隔离数据
- 中间件自动注入 `WHERE tenant_id = ?` 条件
- 当前版本暂不实现，表结构预留 `tenant_id` 字段位置

---

## 7. 部署运维方案

### 7.1 环境要求

| 组件 | 版本 | 最低配置 |
|------|------|----------|
| Go | 1.22+ | — |
| MySQL | 8.0+ | 2C4G |
| Redis | 7.0+ | 1C2G |
| Nginx | 1.24+ | — |
| Node.js | 20 LTS | 构建前端用 |

### 7.2 Docker 容器化

```yaml
# docker-compose.yml
version: "3.9"
services:
  app:
    build: ./backend
    ports: ["8080:8080"]
    environment:
      - DB_DSN=root:pass@tcp(mysql:3306)/dwz?charset=utf8mb4&parseTime=true
      - REDIS_ADDR=redis:6379
      - JWT_SECRET=${JWT_SECRET}
    depends_on: [mysql, redis]
    restart: unless-stopped

  admin-web:
    build: ./frontend
    ports: ["3000:80"]
    restart: unless-stopped

  mysql:
    image: mysql:8.0
    volumes: ["mysql_data:/var/lib/mysql"]
    environment:
      - MYSQL_ROOT_PASSWORD=${DB_ROOT_PASS}
      - MYSQL_DATABASE=dwz
    restart: unless-stopped

  redis:
    image: redis:7-alpine
    command: redis-server --appendonly yes --maxmemory 256mb
    volumes: ["redis_data:/data"]
    restart: unless-stopped

volumes:
  mysql_data:
  redis_data:
```

### 7.3 Nginx 反向代理

```nginx
server {
    listen 443 ssl http2;
    server_name admin.example.com;

    # 前端 SPA
    location / {
        root /var/www/admin-web/dist;
        try_files $uri $uri/ /index.html;
    }

    # 管理 API
    location /admin/api/ {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }

    # 公开短链 API（兼容现有 PHP 或代理到 Go）
    location ~ ^/(api|batch)\.php$ {
        proxy_pass http://127.0.0.1:8080;
    }

    # 短链跳转
    location ~ ^/[a-z0-5]{6,8}$ {
        proxy_pass http://127.0.0.1:8080/redirect$uri;
    }
}
```

### 7.4 日志方案

- **应用日志**：结构化 JSON 格式，按天轮转，保留 30 天
- **访问日志**：Nginx JSON 格式，含 request_id 串联
- **审计日志**：写入 DB `audit_logs` 表，90 天后归档
- **集中收集**（可选）：Promtail → Loki → Grafana 或 Filebeat → ELK

### 7.5 监控告警

| 指标 | 工具 | 告警阈值 |
|------|------|----------|
| 接口 P99 延迟 | Prometheus + Grafana | > 500ms |
| 错误率 (5xx) | Prometheus | > 1% / 5min |
| Redis 内存使用 | Prometheus | > 80% |
| MySQL 连接数 | Prometheus | > 80% max |
| 磁盘使用率 | Node Exporter | > 85% |
| 证书到期 | Blackbox Exporter | < 14 天 |

### 7.6 性能优化方向

| 优化点 | 方案 | 预期收益 |
|--------|------|----------|
| 跳转加速 | Redis 缓存短码→URL，TTL 1h | P99 < 5ms |
| 点击写入 | MQ 异步批量写入 | 跳转响应不受 DB 影响 |
| 统计查询 | 预聚合表（每小时/天汇总） | 仪表盘 < 100ms |
| 静态资源 | CDN + 长缓存 + 版本指纹 | 带宽降低 90% |
| 读写分离 | MySQL 主从，统计查询走从库 | 主库压力降低 |
| 连接池 | Go sql.DB + Redis pool 调优 | 减少连接开销 |

### 7.7 备份与灾备

- **MySQL**：每日全量备份（mysqldump/xtrabackup）+ binlog 增量，保留 7 天
- **Redis**：RDB + AOF 持久化，每日备份 RDB 文件
- **配置**：Git 管理，Nginx/环境变量纳入版本控制
- **灾备**：异地备份到对象存储（OSS/S3），RPO < 1h，RTO < 4h

---

## 8. 迁移方案

### 8.1 渐进迁移路径

```
Phase 1（当前）: PHP 单体运行
  ↓
Phase 2: Go 后台管理系统上线，与 PHP 共存
  - Go 管理 API + Vue 前端
  - PHP 继续处理公开 API (api.php/batch.php/do.php)
  - 共享同一 MySQL 数据库
  ↓
Phase 3: Go 接管公开 API
  - 跳转服务迁移到 Go（Redis 缓存加速）
  - 生成 API 迁移到 Go
  - PHP 代码保留为 fallback
  ↓
Phase 4: PHP 下线
  - 全部流量走 Go
  - 清理 PHP 代码
```

### 8.2 数据迁移脚本

```sql
-- 将现有 wjoy_log 数据迁移到 short_urls 表
INSERT INTO short_urls (uid, long_url, url_hash, clicks, expire_at, source, created_at, updated_at)
SELECT
  uid,
  longurl,
  url_hash,
  clicks,
  expire_at,
  'legacy',
  COALESCE(created_at, NOW(3)),
  COALESCE(created_at, NOW(3))
FROM wjoy_log
WHERE uid NOT IN (SELECT uid FROM short_urls)
ON DUPLICATE KEY UPDATE updated_at = NOW(3);
```

### 8.3 灰度切换方案

- 使用 Nginx `split_clients` 按比例将跳转流量路由到 Go 服务
- 初始 5% → 20% → 50% → 100%，每阶段观察 24h
- 监控对比：错误率、P99 延迟、302 成功率
- 异常时一键回切到 PHP（修改 Nginx upstream 权重）

---

## 附录 A：环境变量清单

| 变量 | 说明 | 示例 |
|------|------|------|
| `DB_DSN` | MySQL 连接串 | `user:pass@tcp(127.0.0.1:3306)/dwz` |
| `REDIS_ADDR` | Redis 地址 | `127.0.0.1:6379` |
| `REDIS_PASSWORD` | Redis 密码 | — |
| `JWT_SECRET` | JWT 签名密钥 | 32+ 字节随机串 |
| `JWT_EXPIRY` | Access Token 有效期 | `2h` |
| `REFRESH_EXPIRY` | Refresh Token 有效期 | `168h` |
| `PUBLIC_BASE_URL` | 短链公开基础 URL | `https://1.xk7.cn` |
| `TRUSTED_PROXIES` | 可信代理 CIDR | `127.0.0.1/32,10.0.0.0/8` |
| `RATE_LIMIT_SINGLE` | 单条 API 限流 | `20/60s` |
| `RATE_LIMIT_BATCH` | 批量 API 限流 | `100/60s` |
| `LOG_LEVEL` | 日志级别 | `info` |
| `LOG_FILE` | 日志文件路径 | `/var/log/dwz/app.log` |

---

## 附录 B：项目目录结构建议

```
dwz-shorturl/
├── backend/
│   ├── cmd/
│   │   └── server/main.go          # 入口
│   ├── internal/
│   │   ├── config/                 # 配置加载
│   │   ├── middleware/             # 认证/权限/限流/日志
│   │   ├── handler/                # HTTP Handler
│   │   ├── service/                # 业务逻辑
│   │   ├── repository/             # 数据访问
│   │   ├── model/                  # 数据模型
│   │   ├── dto/                    # 请求/响应 DTO
│   │   └── pkg/                    # 内部工具包
│   ├── migrations/                 # 数据库迁移文件
│   ├── configs/
│   │   └── config.yaml             # 默认配置
│   ├── Dockerfile
│   └── go.mod
├── frontend/
│   ├── src/
│   │   ├── views/                  # 页面
│   │   ├── components/             # 组件
│   │   ├── api/                    # 接口封装
│   │   ├── stores/                 # 状态管理
│   │   ├── router/                 # 路由
│   │   └── utils/                  # 工具
│   ├── package.json
│   ├── vite.config.ts
│   └── Dockerfile
├── deploy/
│   ├── docker-compose.yml
│   ├── nginx/
│   └── scripts/
├── docs/
│   └── BACKEND_ADMIN_DESIGN.md     # 本文档
├── api.html                        # 公开 API 文档
├── index.html                      # 公开首页
└── README.md
```

---

*文档结束。本设计文档可直接指导后续 Go 后端 + Vue 前端的代码开发工作。*
