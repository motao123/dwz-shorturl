# Issue #40 体验清单收尾验证记录 — 2026-09-12

## 范围

承接 [PR #41](https://cnb.cool/code_free/dwz-shorturl/-/pulls/41)（26 项体验修复）之后的 4 项未闭环问题：

- **F1** Apache 部署下 `/admin/api`、`/member/api`、`/public/api`、`/health` 无可用转发实现（`.htaccess` 原规则为注释）。
- **F2** README 未提示 Apache / 共享虚拟主机的部署限制与症状。
- **F3** 「PHP 与 Go 错误页字节一致」的描述不成立，且无测试保护。
- **F4** `api.html` 仍使用内联 `<style>`，未抽到 `assets/` 复用版本号与缓存。
- **F5** 品牌名与移动端断点的收尾（邮件签名「陌涛短链」、会员端品牌、768px 单列、注册引导）。

## 功能验证

### F1 PHP 代理兜底（`api_proxy.php`）

用 mock Go 后端（Node，监听 127.0.0.1:18080）+ PHP 内置服务器实测：

| 场景 | 期望 | 结果 |
|---|---|---|
| `POST /admin/api/domains/active?foo=bar` | 路径与 query 原样转发 | PASS（`path=/admin/api/domains/active?foo=bar`）|
| 请求携带伪造 `X-Forwarded-For: 1.2.3.4` | 被覆盖为真实 `REMOTE_ADDR`，不可伪造 | PASS（后端收到 `xff=127.0.0.1`）|
| `X-API-Key` 请求头 | 透传 | PASS（后端收到 `apiKey=dwz_test`）|
| 请求体 | 透传 | PASS（后端收到 `body=a=1`）|
| `GET /health` | 精确匹配放行 | PASS |
| `GET /do.php`、`/foo` 等非白名单路径 | 404，不成为开放代理 | PASS（`{"code":404,"message":"Not Found"}`）|
| 后端未启动 | 502 + 明确中文提示，而非静默失败 | PASS |
| 未安装 curl 扩展 | 502 + 明确的扩展缺失提示 | PASS |

### F3 品牌页结构契约

新增 `backend/internal/handler/brand_pages_contract_test.go`，锁定两条跳转路径的**结构契约**（不承诺字节一致）：

| 断言 | 结果 |
|---|---|
| PHP 错误页（`do.php`）含 `--ep-*` 品牌变量、暗色适配、`X-Robots-Tag: noindex` | PASS |
| Go 错误页（`redirect.go`）含同一套变量与标记 | PASS |
| PHP 密码页（`includes/function.php`）含 `--pw-*` 变量、暗色适配、品牌返回入口 | PASS |
| Go 密码页含同一套变量与品牌返回入口 | PASS |
| 错误页与密码页浅色色板无漂移 | PASS |

### F4 api.html 样式外置

| 断言 | 结果 |
|---|---|
| `api.html` 不再含 `<style>`，改为引用 `assets/api.min.css` | PASS |
| `build_assets.php` 将 `api.css` 压缩并注入内容哈希 | PASS（`api.min.css` v=80d5eec5）|
| `php migrations/build_assets.php --check` | PASS（`assets up to date`）|

### F5 品牌与断点

| 项 | 结果 |
|---|---|
| 邮件签名/主题由「陌涛短链」统一为「短网址」 | PASS（`member_api.go`、`cron.go`）|
| SMTP 默认发件人 `from_name` 统一为「短网址」 | PASS（`config.example.yaml`、`.env.example`、`init.php`、`docker-compose.yml`、README）|
| 会员端标题统一为「短网址 - 会员中心」 | PASS（`member.html`、`LoginView.vue`）|
| `feature-grid` ≤768px 单列 | PASS（`assets/app.css`）|
| 未验证邮箱提示明确「验证后可解锁批量生成」 | PASS（`DashboardView.vue`）|

## 回归

| 项目 | 命令 | 结果 |
|---|---|---|
| Go 构建/静态检查/单测 | `go build ./... && go vet ./... && go test ./...` | PASS（新增 3 项契约测试）|
| 前端构建/类型检查 | `npm run build`（vite + vue-tsc） | PASS |
| 前端单测 | `npm test` | PASS（53/53）|
| PHP 语法 | `php -l` × 全部 `.php` | PASS |
| 静态资源产物 | `php migrations/build_assets.php --check` | PASS |

## 遗留与取舍

- Apache 代理走 PHP 时多一次进程与网络往返，性能低于 nginx 反代；`.htaccess` 保留 `mod_proxy` 原生方案供有余力的主机选用。
- 「PHP/Go 错误页字节一致」为**结构一致**，由上面的契约测试保证；若日后需要更强保证，应抽出共享模板而非继续维护两份实现。
