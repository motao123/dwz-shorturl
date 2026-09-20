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
//
// 【推荐】单库模式：short_urls 与 wjoy_log 放在同一个库里。
// 只需要打开下面这个开关，其余连接参数自动复用上面的主库配置，
// 不用再填第二套账号密码，也没有跨库权限与 COLLATE 问题。
// Docker 一键部署（docker compose up -d）默认就是这个模式。
$admin_db_same_as_main = true;
if ($admin_db_same_as_main) {
    $admin_db_host = $host;
    $admin_db_port = $port;
    $admin_db_user = $user;
    $admin_db_pwd = $pwd;
    $admin_db_name = $dbname;
} else {
    // 分库模式：独立的管理库。留空 user/name 则关闭双写（仅写 wjoy_log）。
    $admin_db_host = '127.0.0.1';
    $admin_db_port = 3306;
    $admin_db_user = '';
    $admin_db_pwd = '';
    $admin_db_name = '';
}

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

// 合规页联系方式（#20）。
// report.html / privacy.html 是静态页、不由 PHP 渲染，所以这两个值**目前没有任何
// 消费方**——此前这里的注释声称"页面取自这里"，是假的。两页里现在写的是
// `abuse@your-domain.com` / `privacy@your-domain.com` 占位地址，部署时请直接改那
// 两个文件。若你希望它由配置驱动，需要把两页改成 PHP 渲染（并在 nginx/Apache 各
// 加一条路由），届时再把这两个键接上。
$abuse_email = '';
$privacy_email = '';

// 违规词库文件路径（可选，#11）。
// PHP 前台与 Go 后台共用同一份 violation_rules.json：Go 侧已 go:embed 编译进二进制，
// PHP 侧在运行时读取。留空即按顺序自动查找：
//   1) /etc/dwz/violation_rules.json（Docker 镜像与 deploy.sh 的落点）
//   2) 仓库内 backend/internal/pkg/data/violation_rules.json（git clone / 开发机）
// 全部找不到时退回内置最小黑名单（3 域名 / 5 关键词）并写一行错误日志——那时
// 公开 api.php、batch.php 的拦截强度会明显低于后台与 Go 侧，务必在部署后确认。
// 不要把它指到 web 根下的路径：词库可被下载等于把黑名单交给提交方。
$violation_rules_file = '';
