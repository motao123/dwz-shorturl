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

// A1：站点根地址必须来自配置且通过校验，禁止用客户端可控的 Host 头兜底。
// 缺失或非法时直接 503，避免 Host 头注入 XML / XSS。
$base = isset($public_base_url) ? rtrim(trim((string)$public_base_url), '/') : '';
if ($base === '' || !preg_match('#^https?://#i', $base) || !filter_var($base, FILTER_VALIDATE_URL) || strpbrk($base, "<>\"'&") !== false) {
    http_response_code(503);
    echo '<?xml version="1.0" encoding="UTF-8"?><urlset/>';
    exit;
}
// A1：即使 base 合法，也按 XML 规则转义后再拼接，纵深防御。
$base_xml = htmlspecialchars($base, ENT_QUOTES | ENT_XML1, 'UTF-8');
$today = date('Y-m-d');

echo '<?xml version="1.0" encoding="UTF-8"?>' . "\n";
echo '<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">' . "\n";

// 首页
echo "  <url>\n";
echo "    <loc>{$base_xml}/</loc>\n";
echo "    <lastmod>{$today}</lastmod>\n";
echo "    <changefreq>daily</changefreq>\n";
echo "    <priority>1.0</priority>\n";
echo "  </url>\n";

// API 文档页
echo "  <url>\n";
echo "    <loc>{$base_xml}/api.html</loc>\n";
echo "    <lastmod>{$today}</lastmod>\n";
echo "    <changefreq>monthly</changefreq>\n";
echo "    <priority>0.6</priority>\n";
echo "  </url>\n";

// 最近创建的短链（仅收录启用中且未过期的链接）
// A2：增加 status=1 过滤，避免已禁用/已软删除的短链被收录并向搜索引擎提交。
$stmt = $DB->link->prepare(
    "SELECT uid, created_at FROM wjoy_log
     WHERE status = 1
       AND (expire_at IS NULL OR expire_at = '' OR expire_at > NOW())
     ORDER BY created_at DESC LIMIT 500"
);

if ($stmt) {
    $stmt->execute();
    $result = $stmt->get_result();
    while ($row = $result->fetch_assoc()) {
        $uid = htmlspecialchars($row['uid'], ENT_XML1);
        $lastmod = date('Y-m-d', strtotime($row['created_at']));
        echo "  <url>\n";
        echo "    <loc>{$base_xml}/{$uid}</loc>\n";
        echo "    <lastmod>{$lastmod}</lastmod>\n";
        echo "    <changefreq>weekly</changefreq>\n";
        echo "    <priority>0.4</priority>\n";
        echo "  </url>\n";
    }
    $stmt->close();
}

echo '</urlset>' . "\n";

$DB->close();
