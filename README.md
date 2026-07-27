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
   php migrations/001_legacy_schema.php --dry-run
   ```

4. 检查报告无误后执行：

   ```bash
   php migrations/001_legacy_schema.php
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

## 首页静态资源与缓存刷新

现代首页由以下本地资源组成：

- `index.html`
- `assets/app.css`
- `assets/app.js`
- `assets/qrcode.min.js`

页面使用相对 URL，因此根目录和子目录部署均可工作。`index.html` 当前通过 `?v=2.0.0` 引用 CSS、JavaScript 与二维码脚本。Nginx 示例会缓存静态资源 7 天；更新资源后应同步提升查询版本（例如 `v=2.0.1`），并按需刷新 CDN/反向代理缓存。若部署后仍看到旧界面，先确认新 `index.html` 已生效，再清理边缘缓存或执行浏览器强制刷新。

## API

所有生成接口均为 `application/x-www-form-urlencoded` 的 **POST-only** 接口。GET 或其他方法返回 HTTP 405 和 `Allow: POST`。

### 单条生成：`POST api.php`

参数：

| 参数 | 必填 | 说明 |
|---|---:|---|
| `url` | 是 | HTTP(S) URL，最长 2048 字节 |
| `custom` | 否 | 6–8 位 `a-z0-5` 自定义短码 |
| `expire` | 否 | `0`、`1`、`7`、`30`、`365`，默认 `0` |
| `format` | 否 | 传 `txt` 返回纯文本；其他值返回 JSON |

示例：

```bash
curl -X POST https://s.example.com/api.php \
  -H 'Content-Type: application/x-www-form-urlencoded' \
  --data-urlencode 'url=https://example.org/article' \
  --data 'expire=30'
```

成功 JSON：

```json
{
  "code": "abc123",
  "msg": "success",
  "result": 1,
  "short_url": "https://s.example.com/abc123",
  "state": "created"
}
```

`code`、`msg`、`result` 保持原有兼容语义；`short_url` 与 `state` 是增量字段。客户端应优先使用服务端返回的 `short_url`，不要自行从请求 Host 拼接。

`state` 可能为：

- `created`：新建记录；
- `existing`：同一目标已存在，复用短码并按本次请求更新有效期；
- `renewed`：同一目标的既有记录已过期，本次请求续期。

`format=txt` 成功时返回规范完整短链，失败时返回错误文本。

### 批量生成：`POST batch.php`

请求体参数 `urls`，每行一个 URL。最多 100 条，原始请求体上限为 210000 字节；批量项目使用随机短码并永久有效。

```bash
curl -X POST https://s.example.com/batch.php \
  -H 'Content-Type: application/x-www-form-urlencoded' \
  --data-urlencode $'urls=https://example.org/one\nhttps://example.org/two'
```

接口在请求整体合法时返回 HTTP 200 和结果数组；每一项独立成功或失败：

```json
[
  {
    "url": "https://example.org/one",
    "code": "abc123",
    "short_url": "https://s.example.com/abc123",
    "msg": "success",
    "result": 1
  },
  {
    "url": "http://127.0.0.1/",
    "code": null,
    "short_url": "",
    "msg": "url host not allowed",
    "result": 10004
  }
]
```

批量响应中单项失败不会把整个 HTTP 响应改成 4xx；客户端必须检查每项的 `result`。只有请求级错误（空输入、过大、超过 100 条、限流、方法错误等）返回单个错误对象和对应非 2xx 状态。

### HTTP 状态与业务错误码

`result` 是业务错误码；不要把它与 HTTP 状态或成功响应中的短码字段 `code` 混淆。

| HTTP | `result` | 含义 |
|---:|---:|---|
| 200 | 1 | 成功；批量项目级失败也包含在 HTTP 200 数组中 |
| 400 | 10001 | URL/URL 列表为空 |
| 400 | 10002 | URL 过长、格式/协议/端口不正确，或含控制字符/凭据 |
| 400 | 10004 | 目标主机不允许、解析失败或解析到私有/保留地址 |
| 405 | 10010 | 方法不允许，仅接受 POST |
| 409 | 10007 | 自定义短码已占用 |
| 413 | 10011 | 批量原始请求过大 |
| 422 | 10006 | 自定义短码格式错误 |
| 422 | 10008 | 有效期不在允许集合中 |
| 422 | 10012 | 批量超过 100 条 |
| 429 | 10005 | 请求过于频繁；响应带 `Retry-After: 60` |
| 500 | 10003 | 数据库写入/续期失败 |
| 500 | 10009 | 自动短码多次冲突后仍无法生成 |
| 503 | 10000 | 数据库当前不可用 |

错误 JSON 统一采用类似结构：

```json
{"code":0,"msg":"错误说明","result":10005}
```

## 短链跳转

`/<短码>`（子目录部署时为 `/short/<短码>`）经过重写进入 `do.php`：

- 不存在或格式错误：HTTP 404；
- 已过期或目标已不再通过安全校验：HTTP 410；
- 内部查询错误：HTTP 500；
- 成功：HTTP **302**，并递增点击数。

成功和错误跳转均设置禁止缓存头；成功响应包含 `Cache-Control: no-store, private, max-age=0`，避免浏览器、CDN 或共享代理把临时 302 固化为长期跳转。

## 统计页

启用并通过令牌鉴权后，`stats.php` 提供：

- 短链总数与累计点击；
- 点击 Top 10；
- 最近创建 20 条。

页面设置 `no-store`、`noindex, nofollow`、`nosniff` 和 `no-referrer`。目标 URL 的查询参数与片段不会显示，目标链接也不会携带这些敏感部分。统计页仍属于运营面，不应当作公开页面；建议再叠加 VPN、IP allowlist 或 Web 服务器认证。

## 安全与运维限制

- 仅接受 `http`/`https` 目标；拒绝 URL 凭据、控制字符、无效端口、`localhost`、`.local`、DNS 解析失败以及任何解析到私有/保留 IPv4/IPv6 的主机。
- SSRF 检查是创建和跳转时的安全门槛，但不能替代主机/容器级出站防火墙；生产环境应继续阻断云元数据地址和内部网段。
- API 只接受 POST，避免通过链接预取、爬虫或缓存意外创建记录。
- 单条默认每 IP 每 60 秒最多 20 次；批量按项目数计费，每 60 秒总成本最多 100。限流文件不可写时请求会按失败处理。
- 只有 `REMOTE_ADDR` 命中 `$trusted_proxies` 才读取 `X-Forwarded-For`。
- Apache/Nginx 必须阻断 `config.php`、`setup.php`、SQL、日志、备份、隐藏文件以及 `includes/`、`migrations/`、`logs/`。
- 生产环境删除 `setup.php`；迁移脚本保留在 Web 根目录时必须确保 `migrations/` 无法通过 HTTP 访问。
- 将 `$rate_limit_dir` 和 PHP 错误日志放在 Web 根目录之外更安全；限制目录权限，不要向客户端回显数据库错误。
- `logs/php_error.log` 可能包含运维信息，应限制读取、配置轮转并避免纳入发布包。
- 统计页默认关闭；启用时使用长随机令牌和 `X-Stats-Token` 请求头，并考虑额外网络层访问控制。
- 数据库账号使用最小权限；安装账号可与运行账号分离，日常运行不需要全局建库权限。

## 作者与使用

- 作者：陌涛
- 使用与转载请保留署名与项目说明
