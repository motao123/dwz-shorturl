<?php
define('SYSTEM_ROOT', __DIR__ . '/');
define('ROOT', dirname(SYSTEM_ROOT) . '/');
error_reporting(E_ALL);
ini_set('display_errors', '0');
ini_set('log_errors', '1');
if (!is_dir(ROOT . 'logs')) @mkdir(ROOT . 'logs', 0755, true);
ini_set('error_log', ROOT . 'logs/php_error.log');
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

require SYSTEM_ROOT . 'db.class.php';
$DB = new DB($host, $user, $pwd, $dbname, $port);
if (empty($DB->link)) {
    error_log('Database connection failed: ' . $DB->connect_error);
    if (!headers_sent()) {
        http_response_code(503);
        header('Content-Type: application/json; charset=utf-8');
    }
    echo json_encode(array('code' => 0, 'msg' => '数据库暂时不可用', 'result' => 10000), JSON_UNESCAPED_UNICODE);
    exit();
}

require SYSTEM_ROOT . 'function.php';
