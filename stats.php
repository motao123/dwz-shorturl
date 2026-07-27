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
:root{color-scheme:light;--ink:#111827;--strong:#09090b;--muted:#606775;--line:#e4e4e7;--surface:#fff;--soft:#f4f4f5;--page:#fafafa;--success:#15803d;--radius:8px}
*{box-sizing:border-box}body{margin:0;min-width:320px;color:var(--ink);background:var(--page);font-family:Geist,Inter,ui-sans-serif,system-ui,-apple-system,"Segoe UI","PingFang SC",sans-serif;font-size:14px;line-height:1.55;-webkit-font-smoothing:antialiased}a{color:var(--strong);text-underline-offset:3px}a:focus-visible{outline:2px solid var(--strong);outline-offset:2px}.shell{width:min(1080px,calc(100% - 40px));margin:0 auto;padding:28px 0 40px}.topbar{display:flex;align-items:center;justify-content:space-between;gap:16px;padding-bottom:18px;border-bottom:1px solid var(--line)}.brand{display:inline-flex;align-items:center;gap:9px;color:var(--strong);font-weight:650;text-decoration:none}.brand-mark{display:grid;width:30px;height:30px;place-items:center;color:#fff;background:var(--strong);border-radius:7px}.eyebrow{margin:34px 0 5px;color:var(--muted);font:600 11px/1.4 "SFMono-Regular",Consolas,monospace;letter-spacing:.1em;text-transform:uppercase}h1{margin:0;color:var(--strong);font-size:clamp(28px,5vw,38px);line-height:1.2;letter-spacing:-.035em}.intro{margin:8px 0 0;color:var(--muted)}.cards{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:12px;margin:24px 0}.card{padding:18px;background:var(--surface);border:1px solid var(--line);border-radius:var(--radius);box-shadow:0 1px 2px rgba(0,0,0,.04)}.metric{color:var(--strong);font-size:28px;font-weight:700;letter-spacing:-.03em}.metric-label{margin-top:2px;color:var(--muted);font-size:12px}.section{margin-top:16px;padding:20px;background:var(--surface);border:1px solid var(--line);border-radius:var(--radius);box-shadow:0 1px 2px rgba(0,0,0,.04)}.section h2{margin:0 0 14px;color:var(--strong);font-size:16px}.table-wrap{max-width:100%;overflow-x:auto;border:1px solid var(--line);border-radius:6px}.table-wrap:focus-visible{outline:2px solid var(--strong);outline-offset:2px}table{width:100%;min-width:720px;border-collapse:collapse;font-size:12px}caption{position:absolute;width:1px;height:1px;overflow:hidden;clip:rect(0,0,0,0);white-space:nowrap}th,td{text-align:left;padding:10px 12px;border-bottom:1px solid var(--line);vertical-align:top}tbody tr:last-child td{border-bottom:0}th{color:var(--muted);background:var(--soft);font-size:11px;font-weight:600;white-space:nowrap}.code{font-family:"SFMono-Regular",Consolas,monospace;font-weight:600}.url{max-width:360px;overflow-wrap:anywhere}.number{text-align:right;font-variant-numeric:tabular-nums}.date{white-space:nowrap;font-variant-numeric:tabular-nums}.empty{color:var(--muted);text-align:center}.back{display:inline-flex;min-height:40px;align-items:center;margin-top:18px;padding:0 13px;color:var(--strong);background:var(--surface);border:1px solid #d4d4d8;border-radius:var(--radius);font-weight:600;text-decoration:none}.back:hover{background:var(--soft)}
@media(max-width:520px){.shell{width:calc(100% - 32px);padding-top:18px}.cards{grid-template-columns:1fr}.section{padding:16px}.eyebrow{margin-top:26px}}
</style>
</head>
<body>
<div class="shell">
  <header class="topbar">
    <a class="brand" href="index.html"><span class="brand-mark" aria-hidden="true">↗</span><span>短网址</span></a>
    <span>运营统计</span>
  </header>

  <main>
    <p class="eyebrow">PRIVATE ANALYTICS</p>
    <h1>短链统计</h1>
    <p class="intro">查看短链规模、累计点击和近期创建情况。目标网址中的查询参数已隐藏。</p>

    <div class="cards" aria-label="统计摘要">
      <div class="card"><div class="metric"><?php echo $totalLinks; ?></div><div class="metric-label">短链总数</div></div>
      <div class="card"><div class="metric"><?php echo $totalClicks; ?></div><div class="metric-label">累计点击</div></div>
    </div>

    <section class="section" aria-labelledby="top-title">
      <h2 id="top-title">热门短链（点击 Top 10）</h2>
      <div class="table-wrap" tabindex="0" role="region" aria-labelledby="top-title">
        <table>
          <caption>点击量最高的十条短链</caption>
          <thead><tr><th scope="col">短链</th><th scope="col">目标地址（查询参数已隐藏）</th><th scope="col" class="number">点击</th><th scope="col">创建时间</th></tr></thead>
          <tbody>
          <?php if (empty($top)): ?>
            <tr><td colspan="4" class="empty">暂无数据</td></tr>
          <?php else: foreach ($top as $row): $label = redactUrl($row['longurl']); ?>
            <tr>
              <td class="code"><a href="<?php echo html(shortUrlLink($base, $row['uid'])); ?>" target="_blank" rel="noopener noreferrer"><?php echo html($row['uid']); ?></a></td>
              <td class="url"><?php echo html($label); ?></td>
              <td class="number"><?php echo (int) $row['clicks']; ?></td>
              <td class="date"><?php echo html($row['created_at']); ?></td>
            </tr>
          <?php endforeach; endif; ?>
          </tbody>
        </table>
      </div>
    </section>

    <section class="section" aria-labelledby="recent-title">
      <h2 id="recent-title">最近创建（20 条）</h2>
      <div class="table-wrap" tabindex="0" role="region" aria-labelledby="recent-title">
        <table>
          <caption>最近创建的二十条短链</caption>
          <thead><tr><th scope="col">短链</th><th scope="col">目标地址（查询参数已隐藏）</th><th scope="col" class="number">点击</th><th scope="col">创建时间</th></tr></thead>
          <tbody>
          <?php if (empty($recent)): ?>
            <tr><td colspan="4" class="empty">暂无数据</td></tr>
          <?php else: foreach ($recent as $row): $label = redactUrl($row['longurl']); ?>
            <tr>
              <td class="code"><a href="<?php echo html(shortUrlLink($base, $row['uid'])); ?>" target="_blank" rel="noopener noreferrer"><?php echo html($row['uid']); ?></a></td>
              <td class="url"><?php echo html($label); ?></td>
              <td class="number"><?php echo (int) $row['clicks']; ?></td>
              <td class="date"><?php echo html($row['created_at']); ?></td>
            </tr>
          <?php endforeach; endif; ?>
          </tbody>
        </table>
      </div>
    </section>
  </main>

  <a class="back" href="index.html">← 返回生成页</a>
</div>
</body>
</html>
