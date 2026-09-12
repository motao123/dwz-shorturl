<?php
/**
 * Go 后端 PHP 代理兜底入口（仅用于共享虚拟主机 / 无 mod_proxy 的 Apache 场景）
 *
 * 背景：管理台 /admin/api、会员中心 /member/api、对外 /public/api 与 /health
 * 全部由 Go 后端提供。非 Docker 部署若无法配置反代（Apache 未启用 mod_proxy、
 * 宝塔未配 location 等），这些接口会全部 404，且前端不会有任何提示
 * —— 表现为「域名池选不中、短链永远落在主域名」。
 *
 * 本文件用 curl 把请求原样转发到 Go 后端，让上述功能在不支持反代的主机上
 * 也能使用。部署方式（二选一）：
 *
 * 1) .htaccess（推荐，已内置规则，只需把 DWZ_BACKEND 指向真实地址）
 *      RewriteRule ^(admin/api|member/api|public/api|health)(/.*)?$ api_proxy.php [L,QSA]
 *    由 api_proxy.php 从 REQUEST_URI 推断目标路径。
 *
 * 2) nginx：直接把 location 反代到 http://127.0.0.1:8080（性能更好，优先）
 *
 * ⚠️ 性能提示：PHP 代理会多一次进程与网络往返，仅作兜底。有 nginx 时请优先用
 *    nginx 反代（见 nginx.example.conf 与 docs/deploy-baota.md）。
 *
 * ⚠️ 安全：仅允许白名单前缀，且只允许转发到本机后端，避免被当作开放代理。
 */

// Go 后端地址（本机回环地址）。宝塔/共享主机上一般无需修改。
$DWZ_BACKEND = getenv('DWZ_BACKEND') ?: 'http://127.0.0.1:8080';

// 允许转发的路径前缀白名单。
$ALLOWED_PREFIXES = array('/admin/api/', '/member/api/', '/public/api/', '/health');

if (PHP_SAPI === 'cli') {
    fwrite(STDERR, "This endpoint must be called over HTTP.\n");
    exit(1);
}

// 1) 推断目标路径：优先取 REQUEST_URI，其次支持 ?__path= 显式指定。
$path = '';
if (!empty($_SERVER['REQUEST_URI'])) {
    $path = parse_url($_SERVER['REQUEST_URI'], PHP_URL_PATH);
}
if (empty($path) && !empty($_GET['__path'])) {
    $path = $_GET['__path'];
}
$path = '/' . ltrim((string)$path, '/');

// 2) 白名单校验，防止变成开放代理。
$allowed = false;
foreach ($ALLOWED_PREFIXES as $prefix) {
    if ($prefix === '/health') {
        if ($path === '/health') { $allowed = true; break; }
        continue;
    }
    if (strpos($path, $prefix) === 0) { $allowed = true; break; }
}
if (!$allowed) {
    http_response_code(404);
    header('Content-Type: application/json; charset=utf-8');
    echo json_encode(array('code' => 404, 'message' => 'Not Found'), JSON_UNESCAPED_UNICODE);
    exit;
}

if (!function_exists('curl_init')) {
    http_response_code(502);
    header('Content-Type: application/json; charset=utf-8');
    echo json_encode(array(
        'code'    => 502,
        'message' => 'PHP 未启用 curl 扩展，无法代理到 Go 后端。请启用 curl，或改用 nginx 反代。',
    ), JSON_UNESCAPED_UNICODE);
    exit;
}

// 3) 组装查询串（剔除内部参数 __path）。
$query = $_GET;
unset($query['__path']);
$target = rtrim($DWZ_BACKEND, '/') . $path;
if (!empty($query)) {
    $target .= '?' . http_build_query($query);
}

// 4) 透传请求头：剔除 Hop-by-Hop 头，保留 Authorization / Content-Type / X-API-Key 等。
$forwardHeaders = array();
foreach ($_SERVER as $key => $value) {
    if (strpos($key, 'HTTP_') !== 0) continue;
    $name = str_replace('_', '-', substr($key, 5));
    $lower = strtolower($name);
    if (in_array($lower, array('connection', 'keep-alive', 'proxy-authenticate',
        'proxy-authorization', 'te', 'trailer', 'transfer-encoding', 'upgrade',
        'host', 'content-length'), true)) {
        continue;
    }
    $forwardHeaders[] = $name . ': ' . $value;
}
// 覆盖式传入真实客户端 IP，避免访客伪造 X-Forwarded-For 绕过限流。
$remoteIp = isset($_SERVER['REMOTE_ADDR']) ? $_SERVER['REMOTE_ADDR'] : '';
$forwardHeaders = array_values(array_filter($forwardHeaders, function ($h) {
    return stripos($h, 'x-forwarded-for:') !== 0
        && stripos($h, 'x-real-ip:') !== 0;
}));
if ($remoteIp !== '') {
    $forwardHeaders[] = 'X-Forwarded-For: ' . $remoteIp;
    $forwardHeaders[] = 'X-Real-IP: ' . $remoteIp;
}
if (!empty($_SERVER['REQUEST_SCHEME'])) {
    $forwardHeaders[] = 'X-Forwarded-Proto: ' . $_SERVER['REQUEST_SCHEME'];
}

// 5) 读取请求体。
$body = file_get_contents('php://input');
if ($body === false) $body = '';

$ch = curl_init($target);
curl_setopt_array($ch, array(
    CURLOPT_CUSTOMREQUEST  => isset($_SERVER['REQUEST_METHOD']) ? $_SERVER['REQUEST_METHOD'] : 'GET',
    CURLOPT_HTTPHEADER     => $forwardHeaders,
    CURLOPT_POSTFIELDS     => $body,
    CURLOPT_RETURNTRANSFER => true,
    CURLOPT_HEADER         => true,
    CURLOPT_FOLLOWLOCATION => false,
    CURLOPT_CONNECTTIMEOUT => 10,
    CURLOPT_TIMEOUT        => 60,
    // 只允许本机后端，避免被诱导成 SSRF 跳板。
    CURLOPT_PROTOCOLS      => CURLPROTO_HTTP | CURLPROTO_HTTPS,
));

$response = curl_exec($ch);
if ($response === false) {
    $err = curl_error($ch);
    curl_close($ch);
    http_response_code(502);
    header('Content-Type: application/json; charset=utf-8');
    echo json_encode(array(
        'code'    => 502,
        'message' => '无法连接 Go 后端（' . $DWZ_BACKEND . '）。请确认后端已启动，或改用 nginx 反代。',
        'detail'  => $err,
    ), JSON_UNESCAPED_UNICODE);
    exit;
}

$status   = curl_getinfo($ch, CURLINFO_HTTP_CODE);
$hdrSize  = curl_getinfo($ch, CURLINFO_HEADER_SIZE);
curl_close($ch);

$rawHeaders = substr($response, 0, $hdrSize);
$respBody   = substr($response, $hdrSize);

http_response_code($status);
foreach (explode("\r\n", $rawHeaders) as $line) {
    if ($line === '' || stripos($line, 'HTTP/') === 0) continue;
    $l = strtolower($line);
    // 由 PHP 自行处理编码与长度，避免与压缩/长度不一致。
    if (strpos($l, 'transfer-encoding:') === 0
        || strpos($l, 'content-length:') === 0
        || strpos($l, 'connection:') === 0
        || strpos($l, 'content-encoding:') === 0) {
        continue;
    }
    header($line, false);
}
echo $respBody;
