<?php
/*
@name:dwz-shorturl API
@description:dwz-shorturl接口文件
*/
include __DIR__ . '/includes/api.inc.php';

$format = isset($_POST['format']) && is_string($_POST['format']) ? $_POST['format'] : '';
if (!isset($_SERVER['REQUEST_METHOD']) || $_SERVER['REQUEST_METHOD'] !== 'POST') {
    if (!headers_sent()) header('Allow: POST');
    api_result(0, 'method not allowed', 10010, 405);
}

$longurl = isset($_POST['url']) && is_string($_POST['url']) ? trim($_POST['url']) : '';
$custom = isset($_POST['custom']) && is_string($_POST['custom']) ? trim($_POST['custom']) : '';
$expire_raw = isset($_POST['expire']) ? $_POST['expire'] : 0;
$domain_id = isset($_POST['domain']) && is_string($_POST['domain']) ? trim($_POST['domain']) : '';
$domain_id = $domain_id !== '' ? $domain_id : null;

if (!headers_sent() && $format !== 'txt') header('Content-Type: application/json; charset=utf-8');

$validation = validate_long_url($longurl);
if (!$validation[0]) api_result(0, $validation[1], $validation[2], 400);
if (!validate_custom_code($custom)) api_result(0, '自定义短码格式错误（需 6-8 位，仅含 a-z 与 0-5）', 10006, 422);
$expire = validate_expire_days($expire_raw);
if ($expire === false) api_result(0, '有效期仅支持 0、1、7、30 或 365 天', 10008, 422);
if (!rate_limit(real_ip(), 20, 60)) {
    if (!headers_sent()) header('Retry-After: 60');
    api_result(0, '请求过于频繁，请稍后再试', 10005, 429);
}

$r = create_short_url($DB, $longurl, $custom, $expire, $domain_id);
$status = $r['result'] == 1 ? 200 : (in_array($r['result'], array(10007, 10013), true) ? 409 : 500);
api_result(
    $r['result'] == 1 ? $r['code'] : 0,
    $r['msg'],
    $r['result'],
    $status,
    isset($r['short_url']) ? $r['short_url'] : '',
    isset($r['state']) ? $r['state'] : ''
);

function api_result($code, $msg, $result, $status = 200, $short_url = '', $state = '') {
    global $format, $DB;
    if (!headers_sent()) http_response_code($status);
    if ($format === 'txt') {
        echo $code === 0 ? $msg : ($short_url !== '' ? $short_url : $code);
    } else {
        $payload = array('code' => $code, 'msg' => $msg, 'result' => $result);
        if ($short_url !== '') $payload['short_url'] = $short_url;
        if ($state !== '') $payload['state'] = $state;
        echo json_encode($payload, JSON_UNESCAPED_UNICODE | JSON_UNESCAPED_SLASHES);
    }
    if (isset($DB)) $DB->close();
    exit();
}
