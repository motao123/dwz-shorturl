<?php
/*
@name:dwz-shorturl Redirect
@description:dwz-shorturl跳转文件
*/
include __DIR__ . '/includes/api.inc.php';

$uid = isset($_GET['uid']) && is_string($_GET['uid']) ? trim($_GET['uid']) : '';
if ($uid === '' || !preg_match('/^[a-z0-5]{6,8}$/', $uid)) redirect_error(404, 'Not Found');

$stmt = $DB->prepare('SELECT longurl, expire_at, status, password_hash FROM wjoy_log WHERE uid=? LIMIT 1');
if (!$stmt) redirect_error(500, 'Internal Server Error');
mysqli_stmt_bind_param($stmt, 's', $uid);
if (!mysqli_stmt_execute($stmt)) { mysqli_stmt_close($stmt); redirect_error(500, 'Internal Server Error'); }
mysqli_stmt_bind_result($stmt, $t_url, $expire_at, $status, $password_hash);
if (!mysqli_stmt_fetch($stmt)) { mysqli_stmt_close($stmt); redirect_error(404, 'Not Found'); }
mysqli_stmt_close($stmt);

// Soft-deleted links (admin/member) are disabled and stop being served.
if ((int)$status !== 1) redirect_error(404, 'Not Found');

if (!empty($expire_at)) {
    $expires = strtotime($expire_at);
    if ($expires !== false && $expires <= time()) redirect_error(410, '短链已过期');
}

// 密码保护：未解锁前展示密码页，仅解锁成功才跳转/计数。
if (!empty($password_hash)) {
    if (!password_unlock_ok($uid)) {
        if (($_SERVER['REQUEST_METHOD'] ?? 'GET') === 'POST') {
            $pw = isset($_POST['password']) && is_string($_POST['password']) ? $_POST['password'] : '';
            if (password_verify($pw, (string)$password_hash)) {
                set_password_unlock_cookie($uid);
                $back = rtrim($public_base_url, '/') . '/' . $uid;
                header('Location: ' . $back, true, 302);
                exit;
            }
            echo password_page_html($uid, '密码错误，请重试');
            exit;
        }
        echo password_page_html($uid, '');
        exit;
    }
}

// Compatibility with older rows that stored the target as base64.
$decoded = base64_decode($t_url, true);
if ($decoded !== false && base64_encode($decoded) === $t_url && filter_var($decoded, FILTER_VALIDATE_URL)) $t_url = $decoded;
// 跳转不抓取目标，创建时已完成 SSRF/DNS 校验；此处跳过 DNS 解析以避免每次跳转的解析开销。
$validation = validate_long_url($t_url, true);
if (!$validation[0]) redirect_error(410, '短链目标无效');

// Send the redirect header first, then flush the response so the client gets
// the 302 immediately. Click analytics and webhooks run afterwards in the
// fastcgi background, so they never block the redirect hot path.
header('Cache-Control: no-store, private, max-age=0');
header('Pragma: no-cache');
header('Expires: 0');
header('Location: ' . $t_url, true, 302);

if (function_exists('fastcgi_finish_request')) {
    fastcgi_finish_request();
}

$click = $DB->prepare('UPDATE wjoy_log SET clicks = clicks + 1 WHERE uid=?');
if ($click) {
    mysqli_stmt_bind_param($click, 's', $uid);
    mysqli_stmt_execute($click);
    mysqli_stmt_close($click);
}

// Persist the click on the admin side (click_logs + counter) so admin stats
// reflect the primary PHP path. Best-effort.
record_click_analytics($uid);

// Fire link.clicked webhook (best-effort).
dispatch_webhook_event('link.clicked', array('uid' => $uid, 'short_url' => public_short_url($uid)));

$DB->close();
exit();

function redirect_error($status, $message) {
    global $DB;
    if (!headers_sent()) {
        http_response_code($status);
        header('Content-Type: text/plain; charset=utf-8');
        header('Cache-Control: no-store');
    }
    echo $message;
    if (isset($DB)) $DB->close();
    exit();
}
