<?php
/**
 * dwz-shorturl 短链统计页
 * 展示短链总数、累计点击、热门短链与最近创建的短链。
 */
define('IN_CRONLITE', true);
define('SYSTEM_ROOT', __DIR__ . '/includes/');
define('ROOT', __DIR__ . '/');   // stats.php 位于项目根目录

require(ROOT . 'config.php');
if (!isset($port)) $port = '3306';
require(SYSTEM_ROOT . 'db.class.php');
$DB = new DB($host, $user, $pwd, $dbname, $port);
if (empty($DB->link)) {
    http_response_code(500);
    exit('数据库连接失败：' . ($DB->connect_error ?: '未知错误'));
}
require(SYSTEM_ROOT . 'function.php');

// 站点根地址（用于拼接短链）
$scheme = (!empty($_SERVER['HTTPS']) && $_SERVER['HTTPS'] !== 'off') ? 'https' : 'http';
$base = $scheme . '://' . $_SERVER['HTTP_HOST'] . rtrim(dirname($_SERVER['SCRIPT_NAME']), '/') . '/';

// 兼容旧数据：若 longurl 存的是 base64 编码则解码
function dispUrl($u) {
    if (preg_match('#^https?://#i', $u)) return $u;
    $dec = base64_decode($u, true);
    if ($dec !== false && base64_encode($dec) === $u && filter_var($dec, FILTER_VALIDATE_URL)) return $dec;
    return $u;
}
function shortUrlLink($base, $uid) { return $base . $uid; }

$totalLinks = (int)$DB->count("SELECT COUNT(*) FROM wjoy_log");
$totalClicks = (int)$DB->count("SELECT COALESCE(SUM(clicks),0) FROM wjoy_log");
$topRes = $DB->query("SELECT uid, longurl, clicks, created_at FROM wjoy_log ORDER BY clicks DESC LIMIT 10");
$recentRes = $DB->query("SELECT uid, longurl, clicks, created_at FROM wjoy_log ORDER BY created_at DESC LIMIT 20");

$top = []; while ($r = mysqli_fetch_assoc($topRes)) $top[] = $r;
$recent = []; while ($r = mysqli_fetch_assoc($recentRes)) $recent[] = $r;

header('Content-Type: text/html; charset=utf-8');
?>
<!doctype html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>短链统计 - dwz-shorturl</title>
<style>
 body{font-family:system-ui,-apple-system,"PingFang SC",sans-serif;max-width:880px;margin:32px auto;padding:0 16px;color:#222}
 h1{font-size:20px;margin-bottom:4px} a{color:#1565c0;text-decoration:none}
 .cards{display:flex;gap:12px;margin:16px 0;flex-wrap:wrap}
 .card{flex:1;min-width:160px;background:#f5f7fa;border:1px solid #e3e8ef;border-radius:10px;padding:16px}
 .card .n{font-size:26px;font-weight:700;color:#1b5e20}
 .card .t{font-size:13px;color:#607d8b;margin-top:4px}
 table{width:100%;border-collapse:collapse;margin-top:10px;font-size:13px}
 th,td{text-align:left;padding:8px 10px;border-bottom:1px solid #eee;vertical-align:top}
 th{color:#607d8b;font-weight:600}
 td.url{max-width:360px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
 .sec{margin-top:28px}
 .back{display:inline-block;margin-top:18px;font-size:13px}
 .empty{color:#999}
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
    <tr><th>短链</th><th>目标地址</th><th>点击</th><th>创建时间</th></tr>
    <?php if (empty($top)): ?>
      <tr><td colspan="4" class="empty">暂无数据</td></tr>
    <?php else: foreach ($top as $r): ?>
      <tr>
        <td><a href="<?php echo htmlspecialchars(shortUrlLink($base, $r['uid'])); ?>" target="_blank"><?php echo htmlspecialchars($r['uid']); ?></a></td>
        <td class="url"><a href="<?php echo htmlspecialchars(dispUrl($r['longurl'])); ?>" target="_blank" title="<?php echo htmlspecialchars(dispUrl($r['longurl'])); ?>"><?php echo htmlspecialchars(dispUrl($r['longurl'])); ?></a></td>
        <td><?php echo (int)$r['clicks']; ?></td>
        <td><?php echo htmlspecialchars($r['created_at']); ?></td>
      </tr>
    <?php endforeach; endif; ?>
  </table>
</div>

<div class="sec">
  <h3>最近创建（20 条）</h3>
  <table>
    <tr><th>短链</th><th>目标地址</th><th>点击</th><th>创建时间</th></tr>
    <?php if (empty($recent)): ?>
      <tr><td colspan="4" class="empty">暂无数据</td></tr>
    <?php else: foreach ($recent as $r): ?>
      <tr>
        <td><a href="<?php echo htmlspecialchars(shortUrlLink($base, $r['uid'])); ?>" target="_blank"><?php echo htmlspecialchars($r['uid']); ?></a></td>
        <td class="url"><a href="<?php echo htmlspecialchars(dispUrl($r['longurl'])); ?>" target="_blank" title="<?php echo htmlspecialchars(dispUrl($r['longurl'])); ?>"><?php echo htmlspecialchars(dispUrl($r['longurl'])); ?></a></td>
        <td><?php echo (int)$r['clicks']; ?></td>
        <td><?php echo htmlspecialchars($r['created_at']); ?></td>
      </tr>
    <?php endforeach; endif; ?>
  </table>
</div>

<a class="back" href="index.html">← 返回生成页</a>
</body>
</html>
