<?php
/*
@name:dwz-shorturl Redirect
@description:dwz-shorturl跳转文件
*/
include __DIR__ . '/includes/api.inc.php';

$uid = isset($_GET['uid']) && is_string($_GET['uid']) ? trim($_GET['uid']) : '';
if ($uid === '' || !preg_match('/^[a-z0-5]{6,8}$/', $uid)) redirect_error(404, 'Not Found');

$stmt = $DB->prepare('SELECT longurl, expire_at FROM wjoy_log WHERE uid=? LIMIT 1');
if (!$stmt) redirect_error(500, 'Internal Server Error');
mysqli_stmt_bind_param($stmt, 's', $uid);
if (!mysqli_stmt_execute($stmt)) { mysqli_stmt_close($stmt); redirect_error(500, 'Internal Server Error'); }
mysqli_stmt_bind_result($stmt, $t_url, $expire_at);
if (!mysqli_stmt_fetch($stmt)) { mysqli_stmt_close($stmt); redirect_error(404, 'Not Found'); }
mysqli_stmt_close($stmt);

if (!empty($expire_at)) {
    $expires = strtotime($expire_at);
    if ($expires !== false && $expires <= time()) redirect_error(410, '短链已过期');
}

// Compatibility with older rows that stored the target as base64.
$decoded = base64_decode($t_url, true);
if ($decoded !== false && base64_encode($decoded) === $t_url && filter_var($decoded, FILTER_VALIDATE_URL)) $t_url = $decoded;
$validation = validate_long_url($t_url);
if (!$validation[0]) redirect_error(410, '短链目标无效');

$click = $DB->prepare('UPDATE wjoy_log SET clicks = clicks + 1 WHERE uid=?');
if ($click) {
    mysqli_stmt_bind_param($click, 's', $uid);
    mysqli_stmt_execute($click);
    mysqli_stmt_close($click);
}

header('Cache-Control: no-store, private, max-age=0');
header('Pragma: no-cache');
header('Expires: 0');
header('Location: ' . $t_url, true, 302);
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
