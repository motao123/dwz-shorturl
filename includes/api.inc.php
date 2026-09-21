<?php
define('SYSTEM_ROOT', __DIR__ . '/');
define('ROOT', dirname(SYSTEM_ROOT) . '/');
error_reporting(E_ALL);
ini_set('display_errors', '0');
ini_set('log_errors', '1');
if (!is_dir(ROOT . 'logs')) @mkdir(ROOT . 'logs', 0755, true);
// 错误日志落点可由 DWZ_PHP_ERROR_LOG 指定（#59）。容器里该目录不是卷，重建即丢，
// 而词库降级、限流器故障、member_secret 缺失这些告警恰恰只在容器部署下最 needed。
// Dockerfile.web 把它设成 /dev/stderr，交给 supervisor/docker 日志驱动收集；
// 裸机部署不设该变量则维持原来的文件路径，行为不变。
$phpErrorLog = getenv('DWZ_PHP_ERROR_LOG');
ini_set('error_log', $phpErrorLog !== false && $phpErrorLog !== '' ? $phpErrorLog : ROOT . 'logs/php_error.log');
define('IN_CRONLITE', true);
date_default_timezone_set('Asia/Shanghai');
$date = date('Y-m-d H:i:s');

require ROOT . 'config.php';
if (!isset($port)) $port = 3306;
if (!isset($public_base_url)) $public_base_url = '';
if (!isset($trusted_proxies) || !is_array($trusted_proxies)) $trusted_proxies = array();
if (!isset($rate_limit_dir) || !is_string($rate_limit_dir) || $rate_limit_dir === '') {
    $rate_limit_dir = ROOT . 'logs/ratelimit';
}
// 违规词库路径（#11）。留空则按默认候选顺序查找：/etc/dwz/violation_rules.json
// （Docker 镜像内落点）→ 仓库内 backend/internal/pkg/data/ 原件（git clone / 开发）。
// 只放在 web 根之外：词库一旦可被公开下载，等于把黑名单交给绕过方。
if (!isset($violation_rules_file) || !is_string($violation_rules_file)) {
    $violation_rules_file = '';
}
// Admin DB (dual-write target). Optional.
if (!isset($admin_db_host)) $admin_db_host = '127.0.0.1';
if (!isset($admin_db_port)) $admin_db_port = 3306;
if (!isset($admin_db_user)) $admin_db_user = '';
if (!isset($admin_db_pwd)) $admin_db_pwd = '';
if (!isset($admin_db_name)) $admin_db_name = '';
if (!isset($member_secret) || !is_string($member_secret)) $member_secret = '';
// 启动自检：member_secret 缺失时密码保护短链与会员鉴权会静默失效。
// 设置 DWZ_ALLOW_EMPTY_SECRET=1 可显式跳过（仅限不含密码短链的纯跳转部署）。
if ($member_secret === '' && getenv('DWZ_ALLOW_EMPTY_SECRET') !== '1') {
    error_log('[dwz] member_secret 未配置：密码保护短链与会员鉴权将不可用，请在 config.php 中设置随机密钥');
}

require SYSTEM_ROOT . 'db.class.php';
$DB = new DB($host, $user, $pwd, $dbname, $port);
if (empty($DB->link)) {
    error_log('Database connection failed: ' . $DB->connect_error);
    // 跳转端点在数据库故障时要回品牌化错误页，而不是一屏裸 JSON（#34）：访客点开的
    // 是别人分享的短链，裸 JSON 既没有身份、也没有回首页的出口。
    // redirect_error() 只在 do.php 里定义，而 PHP 在编译整个入口文件时就完成顶层函数
    // 声明，所以它在下面的 include 之前就已存在——接口端点（api/batch/member）没有这个
    // 函数，仍然按 JSON 返回。这是刻意的按名耦合，别把它当成通用钩子到处定义。
    if (function_exists('redirect_error')) {
        redirect_error(503, '服务暂时不可用，请稍后重试');
    }
    if (!headers_sent()) {
        http_response_code(503);
        header('Content-Type: application/json; charset=utf-8');
    }
    echo json_encode(array('code' => 0, 'msg' => '数据库暂时不可用', 'result' => 10000), JSON_UNESCAPED_UNICODE);
    exit();
}

// Optional admin DB connection for dual-writing short_urls.
$ADMIN_DB = null;
if ($admin_db_user !== '' && $admin_db_name !== '') {
    $ADMIN_DB = new DB($admin_db_host, $admin_db_user, $admin_db_pwd, $admin_db_name, $admin_db_port);
}

require SYSTEM_ROOT . 'function.php';
