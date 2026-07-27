## dwz-shorturl 短网址服务

dwz-shorturl 是一款基于 PHP + MySQL 的短网址服务，提供简洁 API 生成短链，并通过伪静态完成跳转。作者：陌涛。

### 特性
- 短链生成（JSON / TXT 返回）
- 批量生成（前端多行输入或 `POST /batch.php`）
- SSRF 防护：拒绝指向私有/保留网段（内网、元数据地址）的目标
- 接口限流：默认单 IP 每分钟 20 次（批量 50 次），超限返回 429
- 数据库错误不再 `die` 纯文本，统一返回 JSON 错误并记录日志
- 前端一键复制、加载态、真实错误提示
- 前端资源本地化（Bootstrap、favicon）

### 安装
1. 上传全部文件至站点根目录。
2. 将 `install.sql` 导入到 MySQL，创建表 `wjoy_log`（含 `url_hash` 唯一索引、`clicks`、`created_at` 等字段）。
   > 若已存在旧表，请先 `DROP TABLE wjoy_log;` 再导入，或手动 `ALTER` 增加新字段。
3. 复制 `config.sample.php` 为 `config.php`，填写数据库连接信息。
   > `config.php` 已在 `.gitignore` 中，不会进入版本库。
4. 放置站点图标到根目录：`/favicon.ico`。
5. `assets/bootstrap.min.css` 已为格式化版本，可直接使用或替换为官方 3.3.7 min 版。

### Nginx 伪静态
```
location / {
    index index.php index.html;
    if (!-e $request_filename) {
        rewrite ^/(.+)$ /do.php?uid=$1 last;
    }
}
```

### API
- 生成（JSON）：`POST /api.php` ，参数 `url={ENCODED_URL}` → `{"code":"<短码>","msg":"success","result":1}`
- 生成（TXT）：`POST /api.php` ，参数 `url={ENCODED_URL}&format=txt` → `<短码>` 或错误文本
- 批量生成（JSON）：`POST /batch.php` ，参数 `urls={每行一个网址}` → `[{"url":"...","code":"<短码>","msg":"success"}, ...]`

参数说明：
- 仅允许 `http/https` 链接，长度 ≤ 2048。
- 目标主机不得为私有/保留地址（SSRF 防护）。
- 超过限流频率返回 `{"code":0,"msg":"请求过于频繁，请稍后再试","result":10005}`，HTTP 429。

### 错误码
| code | 含义 |
|------|------|
| 10001 | url 为空 |
| 10002 | url 过长或格式不正确 |
| 10003 | 数据库写入失败 |
| 10004 | 目标主机为私有/保留地址（已拦截） |
| 10005 | 请求过于频繁 |

### 日志
- 运行错误写入 `logs/php_error.log`（已在 `.gitignore` 中忽略）。
- 限流计数写入 `logs/ratelimit/`。

### 首发（1.0）
- 接口路径统一为 `/api.php`
- 本地化前端资源并移除外部统计脚本

### 更新（1.3）
- 新增 SSRF 防护、接口限流、错误日志
- 数据库错误返回 JSON 而非 `die` 纯文本
- 存储改为原文（去除 base64，节省体积并避免解码歧义）
- 去重写入原子化（依赖 `url_hash` 唯一索引）
- 新增 `batch.php` 批量生成接口与前端批量入口
- `do.php` 跳转走预处理语句，并记录点击数 `clicks`

### 作者与使用
- 作者：陌涛
- 使用与转载请保留署名与项目说明
