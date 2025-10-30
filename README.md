## dwz-shorturl 短网址服务

dwz-shorturl 是一款基于 PHP + MySQL 的短网址服务，提供简单 API 生成短链，并通过伪静态完成跳转。作者：陌涛。

### 特性
- 短链生成（JSON/TXT 返回）
- 前端简洁，资源本地化（Bootstrap、favicon）
- 安装简单，依赖少

### 安装
1. 上传全部文件至站点根目录。
2. 将 `install.sql` 导入到 MySQL，创建表 `wjoy_log`。
3. 修改 `config.php` 数据库连接信息。
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
- 生成（JSON）：`GET /api.php?url={ENCODED_URL}` → `{"code":"<短码>","msg":"success","result":1}`
- 生成（TXT）：`GET /api.php?url={ENCODED_URL}&format=txt` → `<短码>` 或错误文本

说明：仅允许 `http/https` 链接，建议在网关做限流。

### 首发（1.0）
- 接口路径统一为 `/api.php`
- 本地化前端资源并移除外部统计脚本

### 作者与使用
- 作者：陌涛
- 使用与转载请保留署名与项目说明

