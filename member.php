<?php
/*
@name:dwz-shorturl Member API
@description:公网用户注册/登录/登出/当前用户接口
*/
include __DIR__ . '/includes/api.inc.php';
include __DIR__ . '/includes/auth.php';

// 读取类 action 允许 GET：默认会话查询与 my_links 都是幂等的只读操作。
// 此前一律要求 POST，导致浏览器直接访问 /member.php、或任何 GET 调用都拿到
// 405，既不符合 HTTP 语义也不便于排障。写操作（register/login/logout）仍必须是 POST。
$method = isset($_SERVER['REQUEST_METHOD']) ? strtoupper((string)$_SERVER['REQUEST_METHOD']) : 'GET';
$preAction = isset($_REQUEST['action']) && is_string($_REQUEST['action']) ? trim($_REQUEST['action']) : '';
$readOnlyActions = array('', 'session', 'me', 'my_links');

if ($method === 'OPTIONS') {
    if (!headers_sent()) { http_response_code(204); header('Allow: GET, POST, OPTIONS'); }
    exit();
}

if ($method !== 'POST') {
    if ($method !== 'GET' && $method !== 'HEAD') member_method_not_allowed();
    if (!in_array($preAction, $readOnlyActions, true)) member_method_not_allowed();
}
if (!headers_sent()) {
    header('Content-Type: application/json; charset=utf-8');
    // 响应含 JWT/CSRF token，禁止缓存以防止 token 泄露
    header('Cache-Control: no-store, private');
    header('Pragma: no-cache');
}

// 从 $_REQUEST 取 action：GET 用 query string，POST 用表单，两者都能命中。
$action = $preAction;

/* CSRF 防护：登录/注册/登出需要先获取 token（GET /me） */
$csrf = isset($_POST['csrf']) && is_string($_POST['csrf']) ? trim($_POST['csrf']) : '';
if (in_array($action, array('register', 'login', 'logout'), true)) {
    if ($csrf === '' || !hash_equals($_SESSION['member_csrf'] ?? '', $csrf)) {
        member_result(0, '页面已过期，请刷新后重试', 10020, 403);
    }
}

if ($action === 'register') {
    // #17：注册与登录此前都传裸 real_ip()，和匿名建链共用同一个桶，三条限额互相挤占。
    if (!rate_limit_allows('member.register:' . real_ip(), 10, 3600)) member_result(0, '注册过于频繁，请稍后再试', 10005, 429);
    // 注册必须明示同意协议与隐私政策（合规要求，前端勾选框 + 后端兜底校验）
    $agree = isset($_POST['agree']) && (string)$_POST['agree'] === '1';
    if (!$agree) member_result(0, '请先阅读并同意《用户协议》与《隐私政策》', 10021, 400);
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
    if (!rate_limit_allows('member.login:' . real_ip(), 20, 60)) member_result(0, '请求过于频繁，请稍后再试', 10005, 429);
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
        // LIMIT/OFFSET 占位符在部分旧驱动/mysqlnd 未启用时会静默失败，这里直接拼接已强转 int 的值（$per/$offset 均来自 (int) 转换，无注入风险）。
        $perSql = max(1, (int)$per);
        $offsetSql = max(0, (int)$offset);
        $stmt = $ADMIN_DB->prepare('SELECT uid, long_url, clicks, expire_at, created_at FROM short_urls WHERE member_id=? AND deleted_at IS NULL ORDER BY id DESC LIMIT ' . $perSql . ' OFFSET ' . $offsetSql);
        if ($stmt) {
            mysqli_stmt_bind_param($stmt, 'i', $mid);
            if (mysqli_stmt_execute($stmt)) {
                $res = mysqli_stmt_get_result($stmt);
                if ($res) {
                    while ($row = mysqli_fetch_assoc($res)) {
                        $row['short_url'] = public_short_url($row['uid']);
                        $links[] = $row;
                    }
                }
            } else {
                error_log('[dwz] my_links query failed: ' . mysqli_stmt_error($stmt));
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
// 同时写入 HttpOnly cookie：Go 后端 MemberAuth 在请求头缺失时会回退读取它，
// 这样即使前端 token 未持久化（或页面刷新后），会员接口依然可用。
member_set_token_cookie($token);
member_result(1, 'ok', 1, 200, array(
    'member' => $member,
    'csrf' => $_SESSION['member_csrf'],
    'token' => $token,
    // 「仅注册使用」开关状态随登录态下发：首页据此把建链表单切换为登录
    // 引导（体验层）；api.php 的 401 拦截是真正的后端兜底。
    'require_registration' => member_only_create_enabled(),
));

// 统一的 405 响应：带上 Allow 头并指明允许的方法，便于调用方排障。
function member_method_not_allowed() {
    if (!headers_sent()) {
        http_response_code(405);
        header('Allow: GET, POST');
        header('Content-Type: application/json; charset=utf-8');
    }
    echo json_encode(array('code' => 0, 'msg' => 'method not allowed', 'result' => 10010));
    exit();
}

function member_result($code, $msg, $result, $status = 200, $data = null) {
    global $DB;
    if (!headers_sent()) http_response_code($status);
    $payload = array('code' => $code, 'msg' => $msg, 'result' => $result);
    if ($data !== null) $payload['data'] = $data;
    echo json_encode($payload, JSON_UNESCAPED_UNICODE | JSON_UNESCAPED_SLASHES);
    if (isset($DB)) $DB->close();
    exit();
}

// 同站点 HttpOnly cookie 承载会员 JWT（与 Go 侧 MemberAuth 的 cookie 名保持一致）。
function member_set_token_cookie($token) {
    if (headers_sent()) return;
    $name = 'dwz_member_token';
    $secure = !empty($_SERVER['HTTPS']) && $_SERVER['HTTPS'] !== 'off';
    if ($token === '' || $token === null) {
        setcookie($name, '', time() - 3600, '/', '', $secure, true);
        unset($_COOKIE[$name]);
        return;
    }
    setcookie($name, (string)$token, 0, '/', '', $secure, true);
}
