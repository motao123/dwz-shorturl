<?php
/*
@name:dwz-shorturl API
@description:dwz-shorturl接口文件
*/
include __DIR__ . '/includes/api.inc.php';
include __DIR__ . '/includes/auth.php';

$format = isset($_POST['format']) && is_string($_POST['format']) ? $_POST['format'] : '';
if (!isset($_SERVER['REQUEST_METHOD']) || $_SERVER['REQUEST_METHOD'] !== 'POST') {
    if (!headers_sent()) header('Allow: POST');
    api_result(0, 'method not allowed', 10010, 405);
}

$longurl = isset($_POST['url']) && is_string($_POST['url']) ? trim($_POST['url']) : '';
$custom = isset($_POST['custom']) && is_string($_POST['custom']) ? trim($_POST['custom']) : '';
$expire_raw = isset($_POST['expire']) ? $_POST['expire'] : 0;
$password = isset($_POST['password']) && is_string($_POST['password']) ? $_POST['password'] : '';
$domain_id = isset($_POST['domain']) && is_string($_POST['domain']) ? trim($_POST['domain']) : '';
$domain_id = $domain_id !== '' ? $domain_id : null;

if (!headers_sent() && $format !== 'txt') header('Content-Type: application/json; charset=utf-8');

$validation = validate_long_url($longurl);
if (!$validation[0]) api_result(0, $validation[1], $validation[2], 400);
$violation = check_url_violation($longurl);
if ($violation['blocked']) {
    log_violation($DB, $longurl, $violation['reason'], 'api');
    api_result(0, '目标网址包含违规内容，已拦截', 10014, 422);
}
if (!validate_custom_code($custom)) api_result(0, '自定义短码格式错误（需 6-8 位，仅含 a-z 与 0-5）', 10006, 422);
$expire = validate_expire_days($expire_raw);
if ($expire === false) api_result(0, '有效期仅支持 0、1、7、30 或 365 天', 10008, 422);
if (!rate_limit(real_ip(), 20, 60)) {
    if (!headers_sent()) header('Retry-After: 60');
    api_result(0, '请求过于频繁，请稍后再试', 10005, 429);
}

$r = create_short_url($DB, $longurl, $custom, $expire, $domain_id, $password);
if ($r['result'] == 1) {
    // 关联当前登录会员（未登录时为 0，sync 内部会转为 NULL），使单条生成的短链也能在会员中心“我的短链”看到
    $password_hash = $password !== '' ? password_hash($password, PASSWORD_DEFAULT) : null;
    sync_short_url_to_admin($r['code'], $longurl, $expire > 0 ? date('Y-m-d H:i:s', time() + $expire * 86400) : null, member_id(), $password_hash);
    // 新创建的短链派发 link.created 事件（与 Go 公开 API 行为对齐）
    if ($r['state'] === 'created') {
        dispatch_webhook_event('link.created', array(
            'id' => $r['code'],
            'uid' => $r['code'],
            'long_url' => $longurl,
            'short_url' => isset($r['short_url']) ? $r['short_url'] : '',
        ));
    }
}
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
