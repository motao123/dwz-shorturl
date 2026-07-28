<?php
/*
@name:dwz-shorturl Batch API
@description:批量生成短链接口
*/
include __DIR__ . '/includes/api.inc.php';

if ($_SERVER['REQUEST_METHOD'] !== 'POST') {
    if (!headers_sent()) { http_response_code(405); header('Allow: POST'); header('Content-Type: application/json; charset=utf-8'); }
    echo json_encode(array('code' => 0, 'msg' => 'method not allowed', 'result' => 10010));
    exit();
}
if (!headers_sent()) header('Content-Type: application/json; charset=utf-8');

$raw = isset($_POST['urls']) && is_string($_POST['urls']) ? $_POST['urls'] : '';
$domain_id = isset($_POST['domain']) && is_string($_POST['domain']) ? trim($_POST['domain']) : '';
$domain_id = $domain_id !== '' ? $domain_id : null;
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
    $r = create_short_url($DB, $u, null, 0, $domain_id);
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
