<?php
/*
@name:dwz-shorturl Batch API
@description:批量生成短链接口
*/
include __DIR__ . '/includes/api.inc.php';
include __DIR__ . '/includes/auth.php';

if ($_SERVER['REQUEST_METHOD'] !== 'POST') {
    if (!headers_sent()) { http_response_code(405); header('Allow: POST'); header('Content-Type: application/json; charset=utf-8'); }
    echo json_encode(array('code' => 0, 'msg' => 'method not allowed', 'result' => 10010));
    exit();
}
if (!headers_sent()) header('Content-Type: application/json; charset=utf-8');

// 批量生成需登录后才能使用
$member = member_require_login($DB);

// 批量生成需邮箱已验证（防垃圾账号批量刷链）
if (empty($member['email_verified']) || (int)$member['email_verified'] !== 1) {
    batch_error('请先在会员中心验证邮箱后使用批量生成', 10022, 403);
}

// CSRF 防护：与 member.php 保持一致，校验会话中的 csrf token
$csrf = isset($_POST['csrf']) && is_string($_POST['csrf']) ? trim($_POST['csrf']) : '';
if ($csrf === '' || !hash_equals($_SESSION['member_csrf'] ?? '', $csrf)) {
    batch_error('页面已过期，请刷新后重试', 10020, 403);
}

$raw = isset($_POST['urls']) && is_string($_POST['urls']) ? $_POST['urls'] : '';
$domain_id = isset($_POST['domain']) && is_string($_POST['domain']) ? trim($_POST['domain']) : '';
// A3：批量接口同样要求 domain 必须是纯数字 ID 且调用方已登录（本接口已强制 CSRF+登录）。
if ($domain_id !== '' && (!ctype_digit($domain_id) || member_id() <= 0)) {
    $domain_id = null;
}
$password = isset($_POST['password']) && is_string($_POST['password']) ? $_POST['password'] : '';
if (strlen($password) > 72) batch_error('访问密码过长（最多 72 字节）', 10008, 422);
if (strlen($raw) > 210000) batch_error('request too large', 10011, 413);
$lines = preg_split('/\R+/', trim($raw), -1, PREG_SPLIT_NO_EMPTY);
if (!$lines) batch_error('urls cannot be empty', 10001, 400);
if (count($lines) > 100) batch_error('too many urls; maximum is 100', 10012, 422);
if (!rate_limit(real_ip(), 100, 60, count($lines))) {
    if (!headers_sent()) header('Retry-After: 60');
    batch_error('请求过于频繁，请稍后再试', 10005, 429);
}

$results = array();
foreach ($lines as $u) {
    $u = trim($u);
    $validation = validate_long_url($u);
    if (!$validation[0]) {
        $results[] = array('url' => $u, 'code' => null, 'short_url' => '', 'msg' => $validation[1], 'result' => $validation[2]);
        continue;
    }
    $violation = check_url_violation($u);
    if ($violation['blocked']) {
        log_violation($DB, $u, $violation['reason'], 'batch');
        $results[] = array('url' => $u, 'code' => null, 'short_url' => '', 'msg' => '目标网址包含违规内容，已拦截', 'result' => 10014);
        continue;
    }
    // C 类改造：批量接口作用于当前会员作用域，重复 URL 直接复用同一短链，
    // 同一会员内的重复提交不再产生冲突码。
    $owner_member = member_id();
    $owner_hash = url_scope_hash($u, url_scope_key($owner_member));
    $owner_uid = find_uid_by_scope($DB, $owner_hash);
    if ($owner_uid !== '') {
        $r = short_url_result($owner_uid, 'existence', 'existing', $domain_id);
    } else {
        $r = create_short_url($DB, $u, null, 0, $domain_id, $password, $owner_member, $owner_hash);
    }
    if ($r['result'] == 1) {
        $password_hash = $password !== '' ? password_hash($password, PASSWORD_DEFAULT) : null;
        // 仅在新建时回写 admin 表并派发事件；复用已有短链时整体跳过，
        // 避免重复提交同一行、重复触发 webhook。
        if ($r['state'] === 'created') {
            sync_short_url_to_admin($r['code'], $u, null, $owner_member, $password_hash);
            dispatch_webhook_event('link.created', array(
                'id' => $r['code'],
                'uid' => $r['code'],
                'long_url' => $u,
                'short_url' => isset($r['short_url']) ? $r['short_url'] : '',
            ));
        }
    }
    $results[] = array(
        'url' => $u,
        'code' => $r['result'] == 1 ? $r['code'] : null,
        'short_url' => isset($r['short_url']) ? $r['short_url'] : '',
        'msg' => $r['msg'],
        'result' => $r['result']
    );
}

http_response_code(200);
echo json_encode($results, JSON_UNESCAPED_UNICODE | JSON_UNESCAPED_SLASHES);
$DB->close();

function batch_error($msg, $result, $status) {
    global $DB;
    if (!headers_sent()) http_response_code($status);
    echo json_encode(array('code' => 0, 'msg' => $msg, 'result' => $result), JSON_UNESCAPED_UNICODE);
    if (isset($DB)) $DB->close();
    exit();
}
