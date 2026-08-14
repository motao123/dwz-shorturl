<?php
// 数据库信息（请复制本文件为 config.php 并填写真实信息；config.php 不应进入版本库）
$host = '127.0.0.1';
$port = 3306;
$user = 'root';
$pwd = 'your_password';
$dbname = 'Imotao';

// 对外短链的唯一规范地址。必须包含 http:// 或 https://，可包含子目录，不要以 / 结尾。
// 例如：https://s.example.com 或 https://example.com/short
$public_base_url = 'https://s.example.com';

// 只有来自这些代理地址或 CIDR 网段时，程序才会信任 X-Forwarded-For。
// 直连部署请保持空数组。示例：array('127.0.0.1', '10.0.0.0/8', '2001:db8::/32')
$trusted_proxies = array();

// 限流文件目录。生产环境建议放在 Web 根目录之外，并授予 PHP 进程写权限。
$rate_limit_dir = __DIR__ . '/logs/ratelimit';

// 管理后台数据库（用于把新短链双写到 short_urls，收敛数据源）。
// 留空 user/name 则关闭双写（仅写 wjoy_log）。
$admin_db_host = '127.0.0.1';
$admin_db_port = 3306;
$admin_db_user = '';
$admin_db_pwd = '';
$admin_db_name = '';

// 会员 JWT 密钥（与 Go 后端 config.yaml 的 jwt.member_secret 保持一致）。
// 用于会员后台鉴权，签发会员 token。
$member_secret = '';

// 统计页默认关闭。若要启用，建议同时设置足够长的随机令牌。
$stats_enabled = false;
$stats_token = '';
