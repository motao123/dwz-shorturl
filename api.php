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
// 未显式传有效期时取后台配置的默认值（0 = 永久有效）
$expire_raw = isset($_POST['expire']) && $_POST['expire'] !== ''
    ? $_POST['expire']
    : system_config_int('shorturl.default_expire_days', 0);
$password = isset($_POST['password']) && is_string($_POST['password']) ? $_POST['password'] : '';
$domain_id = isset($_POST['domain']) && is_string($_POST['domain']) ? trim($_POST['domain']) : '';
// A3：domain 参数必须是纯数字 ID，且仅登录会员可用（匿名传 domain 一律忽略，
// 避免匿名枚举域名池）。非法值直接忽略。
if ($domain_id !== '' && !ctype_digit($domain_id)) {
    $domain_id = null;
}
if ($domain_id !== null && $domain_id !== '' && member_id() <= 0) {
    $domain_id = null;
}

if (!headers_sent() && $format !== 'txt') header('Content-Type: application/json; charset=utf-8');

// CSRF 防护：仅当请求携带已登录会员会话时校验（与 member.php / batch.php 一致），
// 避免攻击者借受害者身份建链。匿名 API 调用（curl、API Key 等无会话请求）不受影响。
if (member_id() > 0) {
    $csrf = isset($_POST['csrf']) && is_string($_POST['csrf']) ? trim($_POST['csrf']) : '';
    if ($csrf === '' || !hash_equals($_SESSION['member_csrf'] ?? '', $csrf)) {
        api_result(0, '页面已过期，请刷新后重试', 10020, 403);
    }
}

// B11：限流必须在任何昂贵的校验（validate_long_url 含 DNS 解析）之前执行，
// 否则攻击者可用解析开销打满请求，导致限流失效。
if (!rate_limit(real_ip(), system_config_int('shorturl.anon_rate_max', 20), system_config_int('shorturl.anon_rate_window', 60))) {
    if (!headers_sent()) header('Retry-After: 60');
    api_result(0, '请求过于频繁，请稍后再试', 10005, 429);
}

// 自定义短码总开关：关闭后所有 custom 参数一律拒绝（后台可即时切换）
if ($custom !== '' && !system_config_bool('shorturl.allow_custom_code', true)) {
    api_result(0, '站点已关闭自定义短码，请使用系统生成的短码', 10006, 422);
}

// 注册制开关：管理员在后台系统配置开启后，匿名建链一律引导注册/登录
// （batch.php 本就要求登录，不受影响）。会员 CSRF 校验在上方不受影响。
if (member_id() <= 0 && member_only_create_enabled()) {
    api_result(0, '当前站点已开启「仅注册使用」，请先注册或登录后再生成短链', 10015, 401);
}

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

// C 类改造：短链唯一性从「URL 全局唯一」改为「同一会员内唯一」。
// 会员已登录时，其短链独立于匿名池与其他会员，可复用已有短码而不报冲突；
// 匿名请求仍走全局池，语义与改造前一致（避免历史匿名短链被重复创建）。
$owner_member = member_id();
$owner_scope = url_scope_key($owner_member);
$owner_hash = url_scope_hash($longurl, $owner_scope);
$owner_uid = find_uid_by_scope($DB, $owner_hash);
if ($owner_uid !== '' && ($custom === '' || $custom === $owner_uid)) {
    // 该作用域下已存在同一目标 URL 的短链：直接复用，不再重复 INSERT。
    $expire_at = $expire > 0 ? date('Y-m-d H:i:s', time() + $expire * 86400) : null;
    if ($expire > 0 && isset($DB) && !empty($DB->link)) {
        $stmt = $DB->prepare('UPDATE wjoy_log SET expire_at=? WHERE url_hash=?');
        if ($stmt) {
            mysqli_stmt_bind_param($stmt, 'ss', $expire_at, $owner_hash);
            mysqli_stmt_execute($stmt);
            mysqli_stmt_close($stmt);
        }
        sync_short_url_to_admin($owner_uid, $longurl, $expire_at, $owner_member, null);
    }
    $r = short_url_result($owner_uid, $expire > 0 ? 'renewed' : 'existence', $expire > 0 ? 'renewed' : 'existing', $domain_id);
} else {
    $r = create_short_url($DB, $longurl, $custom, $expire, $domain_id, $password, $owner_member, $owner_hash);
}
if ($r['result'] == 1) {
    // 关联当前登录会员（未登录时为 0，sync 内部会转为 NULL），使单条生成的短链也能在会员中心“我的短链”看到
    $password_hash = $password !== '' ? password_hash($password, PASSWORD_DEFAULT) : null;
    // 仅在新建时回写 admin 表；复用已有短链时不做无意义的重复提交。
    if ($r['state'] === 'created') {
        sync_short_url_to_admin($r['code'], $longurl, $expire > 0 ? date('Y-m-d H:i:s', time() + $expire * 86400) : null, $owner_member, $password_hash);
        // 新创建的短链派发 link.created 事件（与 Go 公开 API 行为对齐）
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
