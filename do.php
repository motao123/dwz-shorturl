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
$stmt = mysqli_prepare($DB->link, "SELECT longurl FROM wjoy_log WHERE uid=? LIMIT 1");
mysqli_stmt_bind_param($stmt, 's', $uid);
mysqli_stmt_execute($stmt);
mysqli_stmt_bind_result($stmt, $t_url);
if (!mysqli_stmt_fetch($stmt)) {
    mysqli_stmt_close($stmt);
    @header("http/1.1 404 not found");
    @header("status: 404 not found");
    echo 'echo 404';
    exit();
}
mysqli_stmt_close($stmt);

// 兼容旧数据：若存储的是 base64 编码的 URL，则解码
$decoded = base64_decode($t_url, true);
if ($decoded !== false && base64_encode($decoded) === $t_url && filter_var($decoded, FILTER_VALIDATE_URL)) {
    $t_url = $decoded;
}

// 记录点击数（列不存在时静默忽略）
mysqli_query($DB->link, "UPDATE wjoy_log SET clicks = clicks + 1 WHERE uid = '" . mysqli_real_escape_string($DB->link, $uid) . "'");

header("Location: " . $t_url, true, 301);
exit();
