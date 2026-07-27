<?php
/*!
@name:dwz-shorturl Demo
@description:dwz-shorturl跳转文件
@author:陌涛 
@version:1.1
@time:2026-07-27
@copyright:陌涛
*/
include './includes/api.inc.php';
$uid = htmlspecialchars($_GET['uid']);
// 校验短码格式（6-8位，a-z0-5）
if ($uid && !preg_match('/^[a-z0-5]{6,8}$/', $uid)) {
    @header("http/1.1 404 not found");
    @header("status: 404 not found");
    echo 'echo 404';
    exit();
}
if (!$uid) {
    @header("http/1.1 404 not found");
    @header("status: 404 not found");
    exit();
}
// 使用预处理语句查询，避免拼接注入
$stmt = mysqli_prepare($DB->link, "SELECT longurl, expire_at FROM wjoy_log WHERE uid=? LIMIT 1");
if ($stmt) {
    mysqli_stmt_bind_param($stmt, 's', $uid);
    mysqli_stmt_execute($stmt);
    mysqli_stmt_bind_result($stmt, $t_url, $expire_at);
    if (!mysqli_stmt_fetch($stmt)) {
        mysqli_stmt_close($stmt);
        @header("http/1.1 404 not found");
        @header("status: 404 not found");
        echo 'echo 404';
        exit();
    }
    mysqli_stmt_close($stmt);
    // 过期检查（旧表无 expire_at 列时已退化为不带该列的查询，不拦截）
    if (!empty($expire_at) && strtotime($expire_at) !== false && strtotime($expire_at) < time()) {
        @header("http/1.1 410 Gone");
        @header("status: 410 Gone");
        echo '短链已过期';
        exit();
    }
} else {
    // 旧表无 expire_at 列：退化为仅查 longurl
    $stmt2 = mysqli_prepare($DB->link, "SELECT longurl FROM wjoy_log WHERE uid=? LIMIT 1");
    mysqli_stmt_bind_param($stmt2, 's', $uid);
    mysqli_stmt_execute($stmt2);
    mysqli_stmt_bind_result($stmt2, $t_url);
    if (!mysqli_stmt_fetch($stmt2)) {
        mysqli_stmt_close($stmt2);
        @header("http/1.1 404 not found");
        @header("status: 404 not found");
        echo 'echo 404';
        exit();
    }
    mysqli_stmt_close($stmt2);
}

// 兼容旧数据：若存储的是 base64 编码的 URL，则解码
$decoded = base64_decode($t_url, true);
if ($decoded !== false && base64_encode($decoded) === $t_url && filter_var($decoded, FILTER_VALIDATE_URL)) {
    $t_url = $decoded;
}

// 记录点击数（列不存在时静默忽略）
mysqli_query($DB->link, "UPDATE wjoy_log SET clicks = clicks + 1 WHERE uid = '" . mysqli_real_escape_string($DB->link, $uid) . "'");

header("Location: " . $t_url, true, 301);
exit();
