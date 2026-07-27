<?php
/**
 * dwz-shorturl statistics page.
 *
 * Disabled by default. Enable in config.php with BOTH:
 *   $stats_enabled = true;
 *   $stats_token   = 'a-long-random-secret';   // required when enabled
 * Send the token in Authorization: Bearer <token> or X-Stats-Token.
 * Leaving $stats_token empty while $stats_enabled is true is treated as a
 * misconfiguration and the page is refused (never served unauthenticated).
 */
define('IN_CRONLITE', true);
define('SYSTEM_ROOT', __DIR__ . '/includes/');
define('ROOT', __DIR__ . '/');

// Never leak filesystem paths or stack traces to the public network.
error_reporting(E_ALL);
ini_set('display_errors', '0');

if (!is_file(ROOT . 'config.php') || !is_file(SYSTEM_ROOT . 'db.class.php')) {
    if (!headers_sent()) http_response_code(500);
    exit('Service unavailable');
}

require ROOT . 'config.php';

if (empty($stats_enabled)) {
    if (!headers_sent()) http_response_code(404);
    exit('Not Found');
}

// An enabled stats page MUST carry a non-empty token, otherwise it would be
// exposed to the public with no authentication. Refuse instead of serving it.
if (!isset($stats_token) || !is_string($stats_token) || $stats_token === '') {
    error_log('stats.php is enabled but $stats_token is empty; refusing to serve unauthenticated.');
    if (!headers_sent()) http_response_code(404);
    exit('Not Found');
}

$providedToken = isset($_SERVER['HTTP_X_STATS_TOKEN']) ? trim((string) $_SERVER['HTTP_X_STATS_TOKEN']) : '';
$authorization = isset($_SERVER['HTTP_AUTHORIZATION']) ? trim((string) $_SERVER['HTTP_AUTHORIZATION']) : '';
if ($providedToken === '' && preg_match('/^Bearer\s+(.+)$/i', $authorization, $matches)) {
    $providedToken = trim($matches[1]);
}
if ($providedToken === '' || !hash_equals($stats_token, $providedToken)) {
    if (!headers_sent()) http_response_code(404);
    exit('Not Found');
}

header('Cache-Control: no-store');
header('X-Robots-Tag: noindex, nofollow', true);
header('X-Content-Type-Options: nosniff');
header('Referrer-Policy: no-referrer');

if (!isset($port)) {
    $port = 3306;
}
require SYSTEM_ROOT . 'db.class.php';
$DB = new DB($host, $user, $pwd, $dbname, $port);
if (empty($DB->link)) {
    http_response_code(500);
    exit('统计服务暂时不可用');
}

function displayUrl($value)
{
    if (preg_match('#^https?://#i', $value)) {
        return $value;
    }
    $decoded = base64_decode($value, true);
    if ($decoded !== false
        && hash_equals(rtrim(base64_encode($decoded), '='), rtrim($value, '='))
        && filter_var($decoded, FILTER_VALIDATE_URL)) {
        return $decoded;
    }
    return $value;
}

function redactUrl($value)
{
    $url = displayUrl($value);
    $parts = parse_url($url);
    if ($parts === false || empty($parts['scheme']) || empty($parts['host'])) {
        return '[invalid URL]';
    }

    $redacted = strtolower($parts['scheme']) . '://';
    if (!empty($parts['user'])) {
        $redacted .= '[redacted]@';
    }
    $redacted .= $parts['host'];
    if (isset($parts['port'])) {
        $redacted .= ':' . (int) $parts['port'];
    }
    $redacted .= isset($parts['path']) && $parts['path'] !== '' ? $parts['path'] : '/';
    if (isset($parts['query'])) {
        $redacted .= '?[redacted]';
    }
    if (isset($parts['fragment'])) {
        $redacted .= '#[redacted]';
    }
    return $redacted;
}

function shortUrlLink($base, $uid)
{
    return $base . rawurlencode($uid);
}

function html($value)
{
    return htmlspecialchars((string) $value, ENT_QUOTES, 'UTF-8');
}

if (empty($public_base_url) || !is_string($public_base_url)
    || !preg_match('#^https?://#i', $public_base_url)) {
    http_response_code(500);
    exit('统计服务缺少 public_base_url 配置');
}
$base = rtrim($public_base_url, '/') . '/';

$totalLinks = (int) $DB->count('SELECT COUNT(*) FROM wjoy_log');
$totalClicks = (int) $DB->count('SELECT COALESCE(SUM(clicks),0) FROM wjoy_log');
$topRes = $DB->query('SELECT uid, longurl, clicks, created_at FROM wjoy_log ORDER BY clicks DESC LIMIT 10');
$recentRes = $DB->query('SELECT uid, longurl, clicks, created_at FROM wjoy_log ORDER BY created_at DESC LIMIT 20');

$top = array();
if ($topRes) {
    while ($row = mysqli_fetch_assoc($topRes)) {
        $top[] = $row;
    }
}
$recent = array();
if ($recentRes) {
    while ($row = mysqli_fetch_assoc($recentRes)) {
        $recent[] = $row;
    }
}

header('Content-Type: text/html; charset=utf-8');
?>
<!doctype html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<meta name="robots" content="noindex,nofollow">
<title>短链统计 - dwz-shorturl</title>
<style>
body{font-family:system-ui,-apple-system,"PingFang SC",sans-serif;max-width:880px;margin:32px auto;padding:0 16px;color:#222}h1{font-size:20px;margin-bottom:4px}a{color:#1565c0;text-decoration:none}.cards{display:flex;gap:12px;margin:16px 0;flex-wrap:wrap}.card{flex:1;min-width:160px;background:#f5f7fa;border:1px solid #e3e8ef;border-radius:10px;padding:16px}.card .n{font-size:26px;font-weight:700;color:#1b5e20}.card .t{font-size:13px;color:#607d8b;margin-top:4px}table{width:100%;border-collapse:collapse;margin-top:10px;font-size:13px}th,td{text-align:left;padding:8px 10px;border-bottom:1px solid #eee;vertical-align:top}th{color:#607d8b;font-weight:600}td.url{max-width:360px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.sec{margin-top:28px}.back{display:inline-block;margin-top:18px;font-size:13px}.empty{color:#999}
</style>
</head>
<body>
<h1>短链统计</h1>
<div class="cards">
  <div class="card"><div class="n"><?php echo $totalLinks; ?></div><div class="t">短链总数</div></div>
  <div class="card"><div class="n"><?php echo $totalClicks; ?></div><div class="t">累计点击</div></div>
</div>

<div class="sec">
  <h3>热门短链（点击 Top 10）</h3>
  <table>
    <tr><th>短链</th><th>目标地址（查询参数已隐藏）</th><th>点击</th><th>创建时间</th></tr>
    <?php if (empty($top)): ?>
      <tr><td colspan="4" class="empty">暂无数据</td></tr>
    <?php else: foreach ($top as $row): $label = redactUrl($row['longurl']); ?>
      <tr>
        <td><a href="<?php echo html(shortUrlLink($base, $row['uid'])); ?>" target="_blank" rel="noopener noreferrer"><?php echo html($row['uid']); ?></a></td>
        <td class="url" title="<?php echo html($label); ?>"><?php echo html($label); ?></td>
        <td><?php echo (int) $row['clicks']; ?></td>
        <td><?php echo html($row['created_at']); ?></td>
      </tr>
    <?php endforeach; endif; ?>
  </table>
</div>

<div class="sec">
  <h3>最近创建（20 条）</h3>
  <table>
    <tr><th>短链</th><th>目标地址（查询参数已隐藏）</th><th>点击</th><th>创建时间</th></tr>
    <?php if (empty($recent)): ?>
      <tr><td colspan="4" class="empty">暂无数据</td></tr>
    <?php else: foreach ($recent as $row): $label = redactUrl($row['longurl']); ?>
      <tr>
        <td><a href="<?php echo html(shortUrlLink($base, $row['uid'])); ?>" target="_blank" rel="noopener noreferrer"><?php echo html($row['uid']); ?></a></td>
        <td class="url" title="<?php echo html($label); ?>"><?php echo html($label); ?></td>
        <td><?php echo (int) $row['clicks']; ?></td>
        <td><?php echo html($row['created_at']); ?></td>
      </tr>
    <?php endforeach; endif; ?>
  </table>
</div>

<a class="back" href="index.html">返回生成页</a>
</body>
</html>
