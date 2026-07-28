<?php
/**
 * 动态生成 sitemap.xml
 * 包含首页 + 最近创建的短链（最多 500 条）
 */
header('Content-Type: application/xml; charset=utf-8');
header('Cache-Control: no-store, private, max-age=0');

define('SYSTEM_ROOT', __DIR__ . '/includes/');

if (!file_exists(__DIR__ . '/config.php')) {
    http_response_code(503);
    echo '<?xml version="1.0" encoding="UTF-8"?><urlset/>';
    exit;
}

require __DIR__ . '/config.php';
define('IN_CRONLITE', true);
require SYSTEM_ROOT . 'db.class.php';

$DB = new DB($host, $user, $pwd, $dbname, $port ?? 3306);
if (empty($DB->link)) {
    http_response_code(503);
    echo '<?xml version="1.0" encoding="UTF-8"?><urlset/>';
    exit;
}

$base = rtrim($public_base_url ?? 'https://' . $_SERVER['HTTP_HOST'], '/');
$today = date('Y-m-d');

echo '<?xml version="1.0" encoding="UTF-8"?>' . "\n";
echo '<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">' . "\n";

// 首页
echo "  <url>\n";
echo "    <loc>{$base}/</loc>\n";
echo "    <lastmod>{$today}</lastmod>\n";
echo "    <changefreq>daily</changefreq>\n";
echo "    <priority>1.0</priority>\n";
echo "  </url>\n";

// API 文档页
echo "  <url>\n";
echo "    <loc>{$base}/api.html</loc>\n";
echo "    <lastmod>{$today}</lastmod>\n";
echo "    <changefreq>monthly</changefreq>\n";
echo "    <priority>0.6</priority>\n";
echo "  </url>\n";

// 最近创建的短链（仅收录永久有效且来源为 web 的链接）
$stmt = $DB->link->prepare(
    "SELECT uid, created_at FROM wjoy_log
     WHERE (expire_at IS NULL OR expire_at = '' OR expire_at > NOW())
     ORDER BY created_at DESC LIMIT 500"
);

if ($stmt) {
    $stmt->execute();
    $result = $stmt->get_result();
    while ($row = $result->fetch_assoc()) {
        $uid = htmlspecialchars($row['uid'], ENT_XML1);
        $lastmod = date('Y-m-d', strtotime($row['created_at']));
        echo "  <url>\n";
        echo "    <loc>{$base}/{$uid}</loc>\n";
        echo "    <lastmod>{$lastmod}</lastmod>\n";
        echo "    <changefreq>weekly</changefreq>\n";
        echo "    <priority>0.4</priority>\n";
        echo "  </url>\n";
    }
    $stmt->close();
}

echo '</urlset>' . "\n";

$DB->close();
