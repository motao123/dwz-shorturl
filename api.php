<?php
/*!
@name:dwz-shorturl API
@description:dwz-shorturl接口文件
@author:陌涛 
@version:1.3
@time:2026-07-27
@copyright:陌涛
*/
include './includes/api.inc.php';

$longurl = isset($_GET['url']) ? $_GET['url'] : (isset($_POST['url']) ? $_POST['url'] : '');
$format = isset($_GET['format']) ? $_GET['format'] : (isset($_POST['format']) ? $_POST['format'] : '');

// 统一响应头（仅对 JSON）
if (!headers_sent()) {
    if (!isset($format) || $format !== 'txt') {
        header('Content-Type: application/json; charset=utf-8');
    }
}

if (!$longurl) {
    show_result(0, "the url cannot be empty", 10001);
    exit();
}
// 仅允许 http/https，限制长度，拒绝危险协议
if (strlen($longurl) > 2048) {
    show_result(0, "url too long", 10002);
    exit();
}
$parts = @parse_url($longurl);
if (!$parts || !isset($parts['scheme']) || !in_array(strtolower($parts['scheme']), ['http', 'https'])) {
    show_result(0, "url is incorrect", 10002);
    exit();
}
// SSRF 防护：拒绝指向私有/保留网段的目标
$host = isset($parts['host']) ? $parts['host'] : '';
if (isPrivateHost($host)) {
    show_result(0, "url host not allowed", 10004);
    exit();
}
// 接口限流（防滥用）
if (!rate_limit(real_ip(), 20, 60)) {
    if (!headers_sent()) header('HTTP/1.1 429 Too Many Requests');
    show_result(0, "请求过于频繁，请稍后再试", 10005);
    exit();
}

$r = create_short_url($DB, $longurl);
show_result($r['result'] == 1 ? $r['code'] : 0, $r['msg'], $r['result']);

$DB->close();

function show_result($code, $msg, $result) {
    global $format;
    if ($format === 'txt') {
        if ($code === 0) {
            echo $msg;
        } else {
            echo $code;
        }
    } else {
        $result = array("code" => $code, "msg" => $msg, "result" => $result);
        echo json_encode($result);
    }
}
