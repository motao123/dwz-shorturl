<?php
/**
 * dwz-shorturl 自托管真实用户监控（RUM）接收端。
 *
 * 仅接受前端 app.js 通过 sendBeacon 上报的 Core Web Vitals 聚合指标
 * （LCP/CLS/INP/TTFB），逐行追加到 logs/rum.jsonl 供运维离线分析。
 * 设计约束：
 *   - 不收 Cookie、不收 IP 明文（仅做限流键，不落盘）、不收页面内容；
 *   - 单 IP 限流防止刷写日志；文件超上限自动轮转保留一代；
 *   - 任何失败都静默 204，绝不影响用户浏览。
 */
define('ROOT', __DIR__ . '/');
define('SYSTEM_ROOT', __DIR__ . '/includes/');

header('Content-Type: application/json; charset=utf-8');
header('Cache-Control: no-store');

if (($_SERVER['REQUEST_METHOD'] ?? '') !== 'POST') {
    http_response_code(204);
    exit;
}

require ROOT . 'config.php';
require SYSTEM_ROOT . 'function.php';

// 与匿名建链共用限流桶命名空间之外的独立桶：RUM 上报频率远低于建链
if (function_exists('rate_limit') && !rate_limit('rum:' . real_ip(), 30, 60)) {
    http_response_code(204);
    exit;
}

$raw = file_get_contents('php://input');
if ($raw === false || strlen($raw) > 2048) {
    http_response_code(204);
    exit;
}
$m = json_decode($raw, true);
if (!is_array($m) || ($m['m'] ?? '') !== 'cwv') {
    http_response_code(204);
    exit;
}

$num = static function ($v) { return is_numeric($v) ? round((float)$v, 3) : null; };
$record = array(
    'ts'   => date('c'),
    'lcp'  => $num($m['lcp'] ?? null),
    'cls'  => $num($m['cls'] ?? null),
    'inp'  => $num($m['inp'] ?? null),
    'ttfb' => $num($m['ttfb'] ?? null),
    'url'  => substr((string)($m['url'] ?? ''), 0, 200),
    'ua'   => substr((string)($_SERVER['HTTP_USER_AGENT'] ?? ''), 0, 120),
);

$dir = ROOT . 'logs';
$file = $dir . '/rum.jsonl';
if (!is_dir($dir)) @mkdir($dir, 0755, true);
// 轮转：超过 8MB 保留一代历史，避免无限增长
if (is_file($file) && filesize($file) > 8 * 1024 * 1024) {
    @rename($file, $dir . '/rum.jsonl.1');
}
@file_put_contents($file, json_encode($record, JSON_UNESCAPED_SLASHES) . "\n", FILE_APPEND | LOCK_EX);

http_response_code(204);
exit;
