<?php
// 记录错误日志但不向浏览器输出，便于排查（前端统一收 JSON 错误）
error_reporting(E_ALL);
ini_set('display_errors', 0);
ini_set('log_errors', 1);
if (!is_dir(ROOT . 'logs')) @mkdir(ROOT . 'logs', 0755, true);
ini_set('error_log', ROOT . 'logs/php_error.log');
define('IN_CRONLITE', true);
define('SYSTEM_ROOT', dirname(__FILE__).'/');
define('ROOT', dirname(SYSTEM_ROOT).'/');
date_default_timezone_set("PRC");
$date = date("Y-m-j H:i:s");
session_start();

require (ROOT.'config.php');

if(!isset($port))$port='3306';
//连接数据库
require(SYSTEM_ROOT."db.class.php");
$DB=new DB($host,$user,$pwd,$dbname,$port);
if (empty($DB->link)) {
    if (!headers_sent()) header('Content-Type: application/json; charset=utf-8');
    echo json_encode(array('code'=>0,'msg'=>'数据库连接失败：'.($DB->connect_error?$DB->connect_error:'未知错误'),'result'=>0));
    exit();
}

require(SYSTEM_ROOT.'function.php');
require(SYSTEM_ROOT.'member.php');
require(SYSTEM_ROOT.'txprotect.php');

?>