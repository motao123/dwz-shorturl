<?php
/*!
@name:dwz-shorturl Batch API
@description:批量生成短链接口
@author:陌涛
@version:1.0
@time:2026-07-27
@copyright:陌涛
*/
include __DIR__ . '/includes/api.inc.php';

if ($_SERVER['REQUEST_METHOD'] !== 'POST') {
    if (!headers_sent()) header('HTTP/1.1 405 Method Not Allowed');
    exit();
}
if (!headers_sent()) header('Content-Type: application/json; charset=utf-8');

$raw = isset($_POST['urls']) ? $_POST['urls'] : '';
$lines = preg_split('/\s+/', trim($raw), -1, PREG_SPLIT_NO_EMPTY);
$lines = array_slice($lines, 0, 100); // 单次上限 100 条

$results = array();
foreach ($lines as $u) {
    $u = trim($u);
    if ($u === '') continue;
    if (strlen($u) > 2048) {
        $results[] = array('url' => $u, 'code' => null, 'msg' => 'url too long');
        continue;
    }
    $parts = @parse_url($u);
    if (!$parts || !isset($parts['scheme']) || !in_array(strtolower($parts['scheme']), ['http', 'https'])) {
        $results[] = array('url' => $u, 'code' => null, 'msg' => 'url is incorrect');
        continue;
    }
    $host = isset($parts['host']) ? $parts['host'] : '';
    if (isPrivateHost($host)) {
        $results[] = array('url' => $u, 'code' => null, 'msg' => 'url host not allowed');
        continue;
    }
    if (!rate_limit(real_ip(), 50, 60)) {
        $results[] = array('url' => $u, 'code' => null, 'msg' => '请求过于频繁');
        continue;
    }
    $r = create_short_url($DB, $u);
    $results[] = array(
        'url' => $u,
        'code' => $r['result'] == 1 ? $r['code'] : null,
        'msg' => $r['msg']
    );
}

echo json_encode($results);
$DB->close();
