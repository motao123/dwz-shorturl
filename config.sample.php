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

// ⚠️ 反向代理部署必填：只有来自这些代理地址或 CIDR 网段时，程序才会信任 X-Forwarded-For。
// - 直连部署：保持空数组。
// - 反向代理 / 负载均衡部署：**必须**填写代理自身的地址，否则所有访客会被
//   归并为同一个限流桶（全体用户共享 20 次/分钟的额度），且违规记录 IP 失真。
// - 代理层请使用覆盖式赋值 proxy_set_header X-Forwarded-For $remote_addr;，
//   不要用 $proxy_add_x_forwarded_for，否则客户端可自行注入 XFF 绕过限流。
// 示例：array('127.0.0.1', '10.0.0.0/8', '2001:db8::/32')
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

// 链接访问密码 + 会员 JWT 的 HMAC 密钥（≥32 字节随机值）。
// - 必须与 Go 后端 config.yaml 的 jwt.member_secret 保持一致，否则 PHP 与 Go
//   两条跳转路径的密码解锁 cookie 互不认可。
// - 留空会导致「设置了访问密码的短链永远无法解锁」（静默失效），
//   因此请务必填写。setup.php 安装时会自动生成随机值。
// 生成示例：php -r "echo bin2hex(random_bytes(32));"
$member_secret = '';

// 统计页默认关闭。若要启用，建议同时设置足够长的随机令牌。
$stats_enabled = false;
$stats_token = '';
