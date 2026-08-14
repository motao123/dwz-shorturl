<?php
/*
@name:dwz-shorturl Member API
@description:公网用户注册/登录/登出/当前用户接口
*/
include __DIR__ . '/includes/api.inc.php';
include __DIR__ . '/includes/auth.php';

if ($_SERVER['REQUEST_METHOD'] !== 'POST') {
    if (!headers_sent()) { http_response_code(405); header('Allow: POST'); header('Content-Type: application/json; charset=utf-8'); }
    echo json_encode(array('code' => 0, 'msg' => 'method not allowed', 'result' => 10010));
    exit();
}
if (!headers_sent()) {
    header('Content-Type: application/json; charset=utf-8');
    // 响应含 JWT/CSRF token，禁止缓存以防止 token 泄露
    header('Cache-Control: no-store, private');
    header('Pragma: no-cache');
}

$action = isset($_POST['action']) && is_string($_POST['action']) ? trim($_POST['action']) : '';

/* CSRF 防护：登录/注册/登出需要先获取 token（GET /me） */
$csrf = isset($_POST['csrf']) && is_string($_POST['csrf']) ? trim($_POST['csrf']) : '';
if (in_array($action, array('register', 'login', 'logout'), true)) {
    if ($csrf === '' || !hash_equals($_SESSION['member_csrf'] ?? '', $csrf)) {
        member_result(0, '页面已过期，请刷新后重试', 10020, 403);
    }
}

if ($action === 'register') {
    if (!rate_limit(real_ip(), 10, 3600)) member_result(0, '注册过于频繁，请稍后再试', 10005, 429);
    $username = isset($_POST['username']) ? trim($_POST['username']) : '';
    $email = isset($_POST['email']) ? trim($_POST['email']) : '';
    $password = isset($_POST['password']) ? $_POST['password'] : '';
    $r = member_register($DB, $username, $email, $password);
    if (!$r['ok']) member_result(0, $r['msg'], $r['code'], 400);
    // 注册成功后自动登录
    $login = member_login($DB, $username, $password, real_ip());
    if ($login['ok']) {
        $member = member_current($DB);
        member_result(1, '注册成功', 1, 200, array('member' => $member, 'token' => member_issue_token($member['id'], $member['username'], (int)($member['token_version'] ?? 0))));
    }
    member_result(1, '注册成功，请登录', 1, 200);
}

if ($action === 'login') {
    if (!rate_limit(real_ip(), 20, 60)) member_result(0, '请求过于频繁，请稍后再试', 10005, 429);
    $username = isset($_POST['username']) ? trim($_POST['username']) : '';
    $password = isset($_POST['password']) ? $_POST['password'] : '';
    $r = member_login($DB, $username, $password, real_ip());
    if (!$r['ok']) member_result(0, $r['msg'], $r['code'], 401);
    $member = member_current($DB);
    member_result(1, '登录成功', 1, 200, array('member' => $member, 'token' => member_issue_token($member['id'], $member['username'], (int)($member['token_version'] ?? 0))));
}

if ($action === 'logout') {
    member_logout($DB);
    member_result(1, '已退出登录', 1, 200);
}

// 我的短链：需登录，列出该会员创建的双写短链
if ($action === 'my_links') {
    $member = member_current($DB);
    if (!$member) member_result(0, '请先登录', 10015, 401);
    $page = isset($_POST['page']) ? max(1, (int)$_POST['page']) : 1;
    $per = isset($_POST['per_page']) ? min(100, max(1, (int)$_POST['per_page'])) : 20;
    $offset = ($page - 1) * $per;
    $links = array();
    $total = 0;
    global $ADMIN_DB;
    if ($ADMIN_DB && !empty($ADMIN_DB->link)) {
        $mid = (int)$member['id'];
        $stmt = $ADMIN_DB->prepare('SELECT COUNT(*) FROM short_urls WHERE member_id=? AND deleted_at IS NULL');
        if ($stmt) {
            mysqli_stmt_bind_param($stmt, 'i', $mid);
            mysqli_stmt_execute($stmt);
            mysqli_stmt_bind_result($stmt, $total);
            mysqli_stmt_fetch($stmt);
            mysqli_stmt_close($stmt);
        }
        $stmt = $ADMIN_DB->prepare('SELECT uid, long_url, clicks, expire_at, created_at FROM short_urls WHERE member_id=? AND deleted_at IS NULL ORDER BY id DESC LIMIT ? OFFSET ?');
        if ($stmt) {
            mysqli_stmt_bind_param($stmt, 'iii', $mid, $per, $offset);
            mysqli_stmt_execute($stmt);
            $res = mysqli_stmt_get_result($stmt);
            if ($res) {
                while ($row = mysqli_fetch_assoc($res)) {
                    $row['short_url'] = public_short_url($row['uid']);
                    $links[] = $row;
                }
            }
            mysqli_stmt_close($stmt);
        }
    }
    member_result(1, 'ok', 1, 200, array('list' => $links, 'total' => $total, 'page' => $page, 'per_page' => $per));
}

// 默认：返回当前登录用户 + CSRF token + 会员 JWT
$member = member_current($DB);
$_SESSION['member_csrf'] = bin2hex(random_bytes(16));
$token = $member ? member_issue_token($member['id'], $member['username'], (int)($member['token_version'] ?? 0)) : '';
member_result(1, 'ok', 1, 200, array(
    'member' => $member,
    'csrf' => $_SESSION['member_csrf'],
    'token' => $token,
));

function member_result($code, $msg, $result, $status = 200, $data = null) {
    global $DB;
    if (!headers_sent()) http_response_code($status);
    $payload = array('code' => $code, 'msg' => $msg, 'result' => $result);
    if ($data !== null) $payload['data'] = $data;
    echo json_encode($payload, JSON_UNESCAPED_UNICODE | JSON_UNESCAPED_SLASHES);
    if (isset($DB)) $DB->close();
    exit();
}