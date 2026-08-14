<?php
/*
 * Public-facing member authentication (login / register / logout).
 * Uses native PHP sessions with hardened cookie flags.
 * Requires the DB wrapper ($DB) to be available (include after api.inc.php).
 */
if (!defined('IN_CRONLITE')) exit();

if (session_status() === PHP_SESSION_NONE) {
    // 站点强制 HTTPS；反向代理场景下以 X-Forwarded-Proto 判断，确保 cookie 带 Secure 标志
    $isHttps = (!empty($_SERVER['HTTPS']) && $_SERVER['HTTPS'] !== 'off')
        || (isset($_SERVER['X_FORWARDED_PROTO']) && $_SERVER['X_FORWARDED_PROTO'] === 'https')
        || (isset($_SERVER['HTTP_X_FORWARDED_PROTO']) && $_SERVER['HTTP_X_FORWARDED_PROTO'] === 'https');
    session_set_cookie_params([
        'httponly' => true,
        'samesite' => 'Lax',
        'secure'   => $isHttps,
        'path'     => '/',
    ]);
    session_name('dwz_member');
    session_start();
}

/* Current session member id, or 0 when not logged in. */
function member_id() {
    return isset($_SESSION['member_id']) ? (int)$_SESSION['member_id'] : 0;
}

/* Return the logged-in member row, or null. */
function member_current($DB) {
    $id = member_id();
    if ($id <= 0 || !$DB || empty($DB->link)) return null;
    $stmt = $DB->prepare('SELECT id, username, email, status, email_verified, token_version, created_at FROM members WHERE id=? AND status=1 LIMIT 1');
    if (!$stmt) return null;
    mysqli_stmt_bind_param($stmt, 'i', $id);
    if (!mysqli_stmt_execute($stmt)) { mysqli_stmt_close($stmt); return null; }
    $row = mysqli_fetch_assoc($stmt->get_result());
    mysqli_stmt_close($stmt);
    return $row ?: null;
}

/* Return the logged-in member row, or null. */
function member_require_login($DB) {
    $member = member_current($DB);
    if ($member) return $member;
    if (!headers_sent()) {
        http_response_code(401);
        header('Content-Type: application/json; charset=utf-8');
    }
    echo json_encode(array('code' => 0, 'msg' => '请先登录后再使用', 'result' => 10015), JSON_UNESCAPED_UNICODE);
    if (isset($DB)) $DB->close();
    exit();
}

/* Register a new member. Returns array('ok'=>bool,'msg'=>string,'code'=>int). */
function member_register($DB, $username, $email, $password) {
    $username = trim((string)$username);
    $email = trim((string)$email);
    if (strlen($username) < 2 || strlen($username) > 32) return array('ok' => false, 'msg' => '用户名长度需为 2-32 位', 'code' => 10016);
    if (!preg_match('/^[A-Za-z0-9_\x{4e00}-\x{9fa5}]+$/u', $username)) return array('ok' => false, 'msg' => '用户名仅支持字母、数字、下划线或中文', 'code' => 10016);
    if (!filter_var($email, FILTER_VALIDATE_EMAIL)) return array('ok' => false, 'msg' => '邮箱格式不正确', 'code' => 10016);
    if (strlen((string)$password) < 8 || strlen((string)$password) > 64) return array('ok' => false, 'msg' => '密码长度需为 8-64 位', 'code' => 10016);
    if (!preg_match('/[A-Za-z]/', $password) || !preg_match('/[0-9]/', $password)) {
        return array('ok' => false, 'msg' => '密码需同时包含字母和数字', 'code' => 10016);
    }

    // Uniqueness checks.
    $stmt = $DB->prepare('SELECT id FROM members WHERE username=? OR email=? LIMIT 1');
    if (!$stmt) return array('ok' => false, 'msg' => '系统繁忙，请稍后再试', 'code' => 10003);
    mysqli_stmt_bind_param($stmt, 'ss', $username, $email);
    mysqli_stmt_execute($stmt);
    $exists = mysqli_fetch_assoc($stmt->get_result());
    mysqli_stmt_close($stmt);
    // 统一提示，避免通过注册接口枚举已存在的用户名/邮箱
    if ($exists) return array('ok' => false, 'msg' => '注册信息已被占用，请更换后重试', 'code' => 10017);

    $hash = password_hash((string)$password, PASSWORD_DEFAULT);
    $stmt = $DB->prepare('INSERT INTO members (username, email, password_hash) VALUES (?,?,?)');
    if (!$stmt) return array('ok' => false, 'msg' => '系统繁忙，请稍后再试', 'code' => 10003);
    mysqli_stmt_bind_param($stmt, 'sss', $username, $email, $hash);
    $ok = mysqli_stmt_execute($stmt);
    $errno = mysqli_stmt_errno($stmt);
    mysqli_stmt_close($stmt);
    if (!$ok) return array('ok' => false, 'msg' => $errno === 1062 ? '注册信息已被占用，请更换后重试' : '注册失败，请稍后再试', 'code' => $errno === 1062 ? 10017 : 10003);
    return array('ok' => true, 'msg' => '注册成功', 'code' => 1);
}

/* Log a member in. Returns array('ok'=>bool,'msg'=>string,'code'=>int). */
function member_login($DB, $username, $password, $ip) {
    $username = trim((string)$username);
    // 账号级失败锁定：同一用户名 15 分钟内最多 8 次失败，成功后清零。
    // 复用 rate_limit 文件机制，key 为 user:<username>。
    if (!rate_limit('user:' . strtolower($username), 8, 900)) {
        return array('ok' => false, 'msg' => '尝试次数过多，账号已临时锁定，请 15 分钟后再试', 'code' => 10021);
    }
    $stmt = $DB->prepare('SELECT id, username, password_hash, status FROM members WHERE username=? LIMIT 1');
    if (!$stmt) return array('ok' => false, 'msg' => '用户名或密码错误', 'code' => 10018);
    mysqli_stmt_bind_param($stmt, 's', $username);
    mysqli_stmt_execute($stmt);
    $row = mysqli_fetch_assoc($stmt->get_result());
    mysqli_stmt_close($stmt);
    if (!$row) {
        // 用户不存在也执行一次空验证，抹平时间差，避免用户名枚举
        $dummy = '$2y$10$invalidinvalidinvalidinvalidinvalidinvalidinval';
        password_verify((string)$password, $dummy);
        return array('ok' => false, 'msg' => '用户名或密码错误', 'code' => 10018);
    }
    if (!password_verify((string)$password, $row['password_hash'])) {
        return array('ok' => false, 'msg' => '用户名或密码错误', 'code' => 10018);
    }
    if ((int)$row['status'] !== 1) return array('ok' => false, 'msg' => '账号已被禁用', 'code' => 10019);

    // 登录成功，清零该账号的失败计数
    rate_limit_reset('user:' . strtolower($username));

    session_regenerate_id(true);
    $_SESSION['member_id'] = (int)$row['id'];

    $ip = strlen((string)$ip) > 45 ? substr((string)$ip, 0, 45) : (string)$ip;
    $stmt = $DB->prepare('UPDATE members SET last_login_at=NOW(), last_login_ip=? WHERE id=?');
    if ($stmt) {
        mysqli_stmt_bind_param($stmt, 'si', $ip, $row['id']);
        mysqli_stmt_execute($stmt);
        mysqli_stmt_close($stmt);
    }
    return array('ok' => true, 'msg' => '登录成功', 'code' => 1);
}

/* Log the current member out. Bumps token_version so all previously issued
   member JWTs are invalidated (enforced by the Go member_auth middleware). */
function member_logout($DB = null) {
    $id = (int)member_id();
    if ($id > 0 && $DB && !empty($DB->link)) {
        $stmt = $DB->prepare('UPDATE members SET token_version = token_version + 1 WHERE id=?');
        if ($stmt) {
            mysqli_stmt_bind_param($stmt, 'i', $id);
            mysqli_stmt_execute($stmt);
            mysqli_stmt_close($stmt);
        }
    }
    $_SESSION = array();
    if (ini_get('session.use_cookies')) {
        $p = session_get_cookie_params();
        setcookie(session_name(), '', time() - 42000, $p['path'], $p['domain'], $p['secure'], $p['httponly']);
    }
    session_destroy();
}