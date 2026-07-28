# dwz-shorturl 短网址服务

dwz-shorturl 是一款基于 PHP 与 MySQL 的短网址服务，提供单条/批量短链生成、短码、自定义有效期、二维码、点击统计以及 Apache/Nginx 短链重写。作者：陌涛。

## 功能

- 现代响应式首页：单条与批量生成、加载与错误状态、复制、居中结果弹窗、键盘可访问交互。
- 自定义短码：6–8 位，仅允许 `a-z` 与 `0-5`。
- 有效期：永久、1 天、7 天、30 天或 365 天；过期短链返回 HTTP 410。
- 批量生成：每次最多 100 条，每行一个 URL。
- 本地二维码生成：短链不会发送给第三方二维码服务。
- 原子去重：使用 `url_hash` 唯一索引，同一目标 URL 会复用既有短码；已过期记录可续期。
- 点击统计：记录访问次数，统计页默认关闭。
- 安全控制：POST-only API、SSRF 防护、可信代理配置、文件限流以及部署层敏感文件阻断。

## 运行要求

- PHP 7.0 或更高版本（建议使用仍受维护的 PHP 8.x）。
- PHP `mysqli` 扩展；应用不使用 PDO。
- MySQL 或兼容的 MariaDB，支持 InnoDB、`utf8mb4`、唯一索引与 `information_schema`。
- Web 服务器：Apache 2.4（启用 `mod_rewrite` 且允许 `.htaccess`）或 Nginx + PHP-FPM。
- PHP 进程必须能够写入错误日志目录和 `$rate_limit_dir`。
- 安装/迁移需要 PHP CLI；不提供浏览器安装入口。

## 全新安装（仅 CLI）

先将代码放到最终目录，再从项目根目录运行：

```bash
php setup.php \
  --host=127.0.0.1 \
  --port=3306 \
  --user=dwz \
  --pwd='数据库密码' \
  --db=Imotao \
  --public-url=https://s.example.com
```

`--host` 与 `--public-url` 是必填参数。`--public-url` 是所有 API 返回短链时使用的**唯一规范公开地址**，必须是 `http://` 或 `https://` URL，不得包含用户名、密码、查询参数或片段；部署在子目录时应包含子目录，例如：

```bash
--public-url=https://example.com/short
```

安装器会创建（或选择）数据库、非破坏性导入 `install.sql`，并以原子方式生成权限尽可能收紧的 `config.php`。它具有以下保护：

- 只能从 CLI 运行；通过 Web 请求会返回 403。
- 如果 `config.php` 已存在，会立即停止且不会覆盖。
- 如果发现既有但不兼容的 `wjoy_log`，会停止并要求先迁移，而不会删表或覆盖数据。

安装成功后立即从生产发布中删除 `setup.php`。若数据库账号没有建库权限，可由管理员预先创建数据库并给安装账号授予该库所需权限。

## 配置

也可以参考 `config.sample.php` 人工创建 `config.php`。除数据库参数外，主要选项如下：

```php
// 规范公开地址；可包含部署子目录，不要以 / 结尾。
$public_base_url = 'https://s.example.com';

// 仅当 REMOTE_ADDR 命中这些 IP/CIDR 时才信任 X-Forwarded-For。
// 直连部署必须保持空数组。
$trusted_proxies = array();

// 限流状态目录。生产环境建议放到 Web 根目录之外。
$rate_limit_dir = __DIR__ . '/logs/ratelimit';

// 统计页默认关闭；启用时应设置长随机令牌。
$stats_enabled = false;
$stats_token = '';
```

说明：

- `public_base_url` 必须与用户实际访问的协议、域名、端口和子目录一致。应用不会根据未经信任的 `Host` 或转发头拼装 API 返回的短链。
- `trusted_proxies` 支持单个 IPv4/IPv6 地址与 CIDR。不要加入不受你控制的代理网段，否则攻击者可能伪造客户端 IP 绕过限流。
- `rate_limit_dir` 必须可由 PHP 写入。多台应用服务器应使用能满足全局限流需求的共享方案；当前实现是单文件系统节点的计数器。
- `stats_enabled` 缺省为 `false`，关闭时 `stats.php` 返回 404。启用后，使用 `X-Stats-Token` 请求头传递 `$stats_token`，不要把令牌放入 URL、日志、书签或 Referer：

  ```bash
  curl -H 'X-Stats-Token: 你的长随机令牌' https://s.example.com/stats.php
  ```

- `config.php` 包含数据库凭据，不应提交到版本库或允许 Web 下载。

## 旧版本升级与非破坏性迁移

不要删除 `wjoy_log`，不要重新导入会替换数据的表结构。迁移脚本不会删除表或记录。

### 推荐发布顺序

对于仍使用旧表结构的生产站点，必须**先完成数据库迁移，再发布依赖新字段的应用代码**：

1. 进入维护窗口，暂停写流量。
2. 备份数据库，并另外备份现有 `config.php`；确认备份可恢复。
3. 使用与现有站点匹配的迁移脚本先预览：

   ```bash
   php migrations/legacy_schema.php --dry-run
   ```

4. 检查报告无误后执行：

   ```bash
   php migrations/legacy_schema.php
   ```

5. 处理脚本报告的重复/空值冲突，必要时再次运行迁移，确认所需唯一索引已创建。
6. 再发布其余应用文件，核对 `config.php` 中的新增配置并恢复流量。
7. 验证单条生成、批量生成、短链跳转和统计开关。

迁移脚本读取现有 `config.php`，会谨慎添加缺少的 `url_hash`、`clicks`、`created_at`、`expire_at` 字段，将可验证的旧 base64 URL 转成明文，并填充哈希。它可以重复运行。

### 冲突行为

- 无效或非 HTTP(S) 的旧数据保持不变，不会被删除。
- 哈希更新遇到已有唯一值冲突时，该行保持不变并计入报告。
- 如果存在重复/空短码，脚本跳过 `uniq_uid`。
- 如果存在重复 URL 或空哈希，脚本跳过 `uniq_hash`。
- 冲突必须由运维人员根据业务数据决定保留/合并哪条记录；脚本不会自动合并或删除。
- `--dry-run` 只报告结构变更，不扫描/修改 URL 数据，也不创建依赖数据状态的唯一索引。

## Web 服务器部署

### Apache：站点根目录

仓库根目录的 `.htaccess` 已包含：

- 禁止目录索引；
- 阻断隐藏路径、`includes/`、`migrations/`、`logs/`、配置、安装器、SQL、日志和备份文件；
- 将 6–8 位合法短码重写到 `do.php?uid=...`。

确保虚拟主机的站点目录指向项目目录、启用 `mod_rewrite`，并允许该目录使用 `.htaccess`（例如 `AllowOverride FileInfo Options`）。若 Apache 不允许 `Options -Indexes`，请在虚拟主机中设置 `Options -Indexes` 并按主机策略调整 `AllowOverride`。

### Apache：子目录

例如代码位于站点的 `/short/` 目录：

```php
$public_base_url = 'https://example.com/short';
```

`.htaccess` 使用相对重写目标，通常会自动留在该目录。若主机要求显式基路径，可在 `RewriteEngine On` 后增加：

```apache
RewriteBase /short/
```

然后验证 `/short/api.php`、`/short/batch.php` 与 `/short/abcdef`，不要把根目录短链规则同时套到整个主站。

### Nginx：站点根目录

仓库的 `nginx.example.conf` 是完整的根目录参考配置。修改以下值后合入实际站点配置：

- `server_name`；
- `root`；
- `fastcgi_pass`（PHP-FPM socket 或地址）。

应用前运行：

```bash
nginx -t && systemctl reload nginx
```

示例配置同时限制敏感路径，只执行真实 PHP 文件，并只把合法短码交给 `do.php`。不要只复制一条宽泛的 `rewrite ^/(.+)$`，否则可能吞掉静态文件、内部路径或主站路由。

### Nginx：子目录示例

以下示例假设主站根目录为 `/var/www/site`，项目实际位于 `/var/www/site/short`，公开路径为 `/short/`：

```nginx
root /var/www/site;
index index.html index.php;

location ~ (^|/)\. {
    deny all;
}

location ~ ^/short/(?:includes|migrations|logs)(?:/|$) {
    deny all;
}

location ~* ^/short/(?:config(?:\.sample)?\.php|setup\.php|install\.sql|nginx\.example\.conf|.*\.(?:log|sql|bak|old|orig|save|swp))$ {
    deny all;
}

location ~ ^/short/.*\.php$ {
    try_files $uri =404;
    include fastcgi_params;
    fastcgi_param SCRIPT_FILENAME $document_root$fastcgi_script_name;
    fastcgi_param HTTP_PROXY "";
    fastcgi_pass unix:/run/php/php-fpm.sock;
}

location /short/ {
    try_files $uri $uri/ @dwz_short_subdir;
}

location @dwz_short_subdir {
    rewrite ^/short/([a-z0-5]{6,8})$ /short/do.php?uid=$1 last;
    return 404;
}
```

同时设置：

```php
$public_base_url = 'https://example.com/short';
```

如果使用面板生成的 Nginx 配置，应把这些规则合入**实际生效的 server 块**，并保留面板自己的 PHP-FPM 参数；不要把根目录示例原样覆盖到子目录部署。

## 首页设计与静态资源缓存

首页与私有统计页采用浅色、克制的开发者工具视觉。设计约束来自长亭百智云模板库当前的 **Shadcn** 模板（通用设计 / 极简主义 / 工程工具）：

- 模板详情：<https://baizhi.cloud/landing/design-prompt/detail/shadcn>
- 完整提示词：项目根目录 [`DESIGN.md`](DESIGN.md)

项目仅迁移该模板的设计语言与组件规则，不复制来源品牌的商标、图片、文案或业务界面。实现继续使用原生 HTML、CSS 和 JavaScript，不依赖外部字体、UI 框架或构建工具。

首页由以下本地资源组成：

- `index.html`
- `assets/app.css`
- `assets/app.js`
- `assets/qrcode.min.js`

页面使用相对 URL，因此根目录和子目录部署均可工作。`index.html` 当前通过 `?v=2.5.0` 引用 CSS、JavaScript 与二维码脚本。Nginx 示例会缓存静态资源 7 天；更新资源后应同步提升查询版本（例如 `v=2.5.1`），并按需刷新 CDN/反向代理缓存。若部署后仍看到旧界面，先确认新 `index.html` 已生效，再清理边缘缓存或执行浏览器强制刷新。

## SEO 与站长统计

网站已内置以下 SEO 与统计能力：

- **`robots.txt`** - 允许搜索引擎收录首页与短链，屏蔽 `/admin/`、`/api.php`、`/includes/` 等敏感路径
- **`sitemap.php`** - 动态生成 `sitemap.xml`，包含首页、API 文档页和最近 500 条有效短链
- **Open Graph / Twitter Card** - 首页含完整社交分享元数据
- **JSON-LD 结构化数据** - `WebApplication` 类型，含功能列表与免费标识
- **Canonical** - 首页 `<link rel="canonical">` 指向 `https://1.xk7.cn/`
- **百度统计** - `index.html` 底部预埋百度统计代码（需替换 `PLACEHOLDER_BAIDU_TONGJI_ID`）
- **Google Analytics** - 预埋 GA4 代码（需替换 `G-PLACEHOLDER`）
- **站长平台验证** - 预埋百度/Google/Bing 三平台 meta 验证标签（需替换各自的 `PLACEHOLDER`）

### 启用站长统计

1. **百度统计**：前往 [tongji.baidu.com](https://tongji.baidu.com) 添加站点，获取统计代码中的 HM ID，替换 `index.html` 中的 `PLACEHOLDER_BAIDU_TONGJI_ID`
2. **Google Analytics**：前往 [analytics.google.com](https://analytics.google.com) 创建 GA4 数据流，获取 Measurement ID（`G-XXXXXXX`），替换 `index.html` 中的 `G-PLACEHOLDER`（两处）
3. **百度站长**：前往 [ziyuan.baidu.com](https://ziyuan.baidu.com) 添加站点 `https://1.xk7.cn`，选择 HTML 标签验证，将 `codeva-PLACEHOLDER` 替换为实际验证码
4. **Google Search Console**：前往 [search.google.com/search-console](https://search.google.com/search-console) 添加资源，选择 HTML 标签验证，将 `PLACEHOLDER` 替换为实际验证码
5. **Bing Webmaster**：前往 [bing.com/webmasters](https://www.bing.com/webmasters) 添加站点，选择 Meta Tag 验证，将 `PLACEHOLDER` 替换为实际验证码
6. **提交 Sitemap**：在各站长平台提交 `https://1.xk7.cn/sitemap.xml`

## API 文档

> 完整 API 接口文档（参数、响应、错误码、限流、SSRF 策略等）已独立为单独页面：
>
> - 本地文件：[`api.html`](api.html)
> - 在线访问：<https://1.xk7.cn/api.html>
>
> 后台管理系统技术设计文档：[`docs/BACKEND_ADMIN_DESIGN.md`](docs/BACKEND_ADMIN_DESIGN.md)

## 作者与使用

- 作者：陌涛
- 使用与转载请保留署名与项目说明
