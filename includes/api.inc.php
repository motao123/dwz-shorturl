<?php
//error_reporting(E_ALL); ini_set("display_errors", 1);
error_reporting(0);
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