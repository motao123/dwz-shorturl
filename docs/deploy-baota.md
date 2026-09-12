# 宝塔 / 单机（非 Docker）部署指南

> 适用：宝塔面板、LNMP 一键包、自建 nginx/Apache 的**非 Docker** 部署。
> Docker 一键部署见 [README「Docker 部署」](../README.md#-docker-一键部署推荐)；本文只讲手工路径。

---

## 0. 先选部署形态

| 形态 | 需要 Go 后端 | 管理台 / 会员中心 | 前台生成 / 跳转 | 适用 |
|---|---|---|---|---|
| **纯 PHP** | ❌ | 管理台不可用（SPA 打不开接口） | ✅ 全可用 | 只想要短链生成 + 跳转 |
| **PHP + Go**（推荐） | ✅ | ✅ | ✅ | 需要后台/会员/统计/域名池 |

下面按「PHP + Go」写；纯 PHP 形态请把第 3 步的三段 location 去掉，并在 `config.php` 里关闭管理台入口（`$admin_enabled = false`）。

---

## 1. 站点与运行环境

1. 宝塔 → 网站 → 添加站点，**根目录指向本仓库根目录**（例如 `/www/wwwroot/dwz-shorturl`）。
2. 软件商店安装：`PHP 8.0+`（需 `mysqli`、`bcmath`、`openssl` 扩展）、`MySQL 5.7+ / 8.0`、`Redis 7`（可选，未装则限流退化为文件计数）。
3. 关闭宝塔对该站点的「防跨站攻击 open_basedir」限制，或在 `open_basedir` 里加上仓库根目录 —— 否则 `includes/`、`logs/` 读写会被拦。
4. 给 `logs/` 可写权限（限流计数器与错误日志）：`chown -R www:www logs && chmod -R 755 logs`。

---

## 2. 初始化数据库与配置

```bash
# 1) 建库并导入基线 schema（宝塔「数据库」面板也可建库）
mysql -uroot -p -e "CREATE DATABASE dwz DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci"

# 2) 生成前台配置 config.php（交互式 CLI，会写入数据库 / 站点地址 / jwt secret）
php setup.php

# 3) 执行全部迁移（PHP 侧 + Go 侧共用一套 schema_migrations）
cd backend && go run ./cmd/migrate
```

`setup.php` 生成后请确认 `config.php` 中至少这几项：

```php
$host / $user / $pwd / $dbname / $port   // 数据库连接
$public_base_url = 'https://你的域名';    // 短链前缀，必须是外网可达的 https 地址
$trusted_proxies = array();              // 前面还有一层 nginx/CDN 时填代理地址
```

> `config.php` 已在 `.gitignore` 内，**不要提交**，也不要放在 web 可直接下载的位置（本仓库自带的 nginx/.htaccess 规则已显式 deny）。

---

## 3. nginx 反代配置（最关键的一步）

把 `nginx.example.conf` 的主 server 块拷进宝塔站点的「配置文件」，并把 Go 后端那四段 location 的注释打开（默认是注释掉的 `return 502` 占位）：

```nginx
upstream dwz_backend {
    server 127.0.0.1:8080;
    keepalive 32;
}

# 管理台 API
location /admin/api/ { proxy_pass http://dwz_backend; include /www/wwwroot/dwz-shorturl/deploy/docker/snippets/dwz-proxy.conf; }
# 会员中心 API
location /member/api/ { proxy_pass http://dwz_backend; include /www/wwwroot/dwz-shorturl/deploy/docker/snippets/dwz-proxy.conf; }
# 对外 API Key 接口
location /public/api/ { proxy_pass http://dwz_backend; include /www/wwwroot/dwz-shorturl/deploy/docker/snippets/dwz-proxy.conf; }
# 健康检查
location = /health { proxy_pass http://dwz_backend/health; }
```

**必须配齐这四段，否则症状如下且前端不会报错**：

- 管理台「域名池」下拉框为空，短链永远落在主域名；
- 管理台列表/统计/用户页面全部报接口错误；
- 会员中心登录后接口 404。

在宝塔面板直接粘贴时，`include` 路径也可以换成内联的 4 行 `proxy_set_header`（见 `nginx.example.conf` 里的注释版本），避免依赖外部文件。

> ⚠️ 务必使用**覆盖式** `proxy_set_header X-Forwarded-For $remote_addr;`，
> 不要用 `$proxy_add_x_forwarded_for`，否则访客可伪造 XFF 绕过限流。

### Apache 用户

`.htaccess` 里已给出 `mod_proxy` 版本的反代规则（默认注释）。共享虚拟主机通常禁用 `mod_proxy`，此时管理台/会员中心不可用，只能走纯 PHP 形态。

### 短链跳转路径

`nginx.example.conf` 已把 `/^[a-z0-5]{6,8}$/` rewrite 到 `do.php?uid=`。若希望走 Go 网关（`/r/:code`，带 GeoIP/Referer 分析），把 `location @short` 改为：

```nginx
location @short {
    add_header Referrer-Policy "no-referrer" always;
    proxy_pass http://dwz_backend/r$uri;
}
```

两条路径可任选其一：**PHP 路径零依赖，Go 路径分析维度更全**。

---

## 4. 启动 Go 后端

```bash
cd backend
go build -o dwz-server ./cmd/server
# 宝塔 → 计划任务/守护进程，或 systemd：
./dwz-server -config configs/config.yaml
```

配置文件 `backend/configs/config.yaml` 至少填写：`database`、`public_db`、`jwt.secret`、`public.base_url`；邮件能力见 `smtp` 段（不填则忘记密码/邮箱验证不可用）。

用宝塔「进程守护管理器」添加守护进程指向 `dwz-server` 即可开机自启。

---

## 5. 一键迁移脚本与回滚

已有历史数据（旧版 `wjoy_log` 结构）时按顺序跑：

```bash
# 迁移到 scoped url_hash（PHP / Go / SQL 三处口径必须一致）
php backend/migrations/php/scope_url_hash.php
# 或使用统一入口（幂等，可重复执行）
bash ops/one_click_migrate.sh

# 回滚：脚本会在执行前把原表备份为 wjoy_log_bak_<时间戳>
mysql -uroot -p dwz -e "RENAME TABLE wjoy_log=wjoy_log_new, wjoy_log_bak_20260101_120000=wjoy_log"
```

> 迁移前请**先全量备份**：`mysqldump -uroot -p dwz > dwz-backup.sql`。

---

## 6. 上线自查清单

- [ ] `https://域名/` 首页可打开，生成一条短链能复制、能跳转
- [ ] `https://域名/api.html` API 文档可打开
- [ ] `https://域名/admin/` 能登录（账号密码见 `setup.php` 输出）
- [ ] 管理台 → 域名池 下拉有数据（说明 `/admin/api/` 反代成功）
- [ ] 会员中心注册 → 登录 → 创建短链正常
- [ ] 忘记密码能收到邮件（需配置 SMTP，见 README「邮件配置」）
- [ ] `https://域名/health` 返回 `{"status":"ok"}`
- [ ] 短码二次访问点击计数 +1
- [ ] `config.php`、`install.sql`、`setup.php` 直接访问返回 403/404
