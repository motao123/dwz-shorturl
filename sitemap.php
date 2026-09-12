<?php
/**
 * 动态生成 sitemap.xml
 *
 * 只收录「有真实内容的落地页」：首页、API 文档页。
 *
 * 为什么不收录短码（历史行为）：
 *   1) 短码是 302 跳板，不是落地页，搜索引擎不会把它当作有效内容；
 *   2) 短码属于用户生成内容，可能指向违规站点，主动提交等于给爬虫喂料；
 *   3) 大批量跳转页会触发「跳转农场」判定，反噬主站 SEO。
 * 短码跳转响应同时携带 X-Robots-Tag: noindex（do.php / Go redirect.go）。
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

// API 文档与宣传页均为可索引内容页，纳入 sitemap
echo "  <url>
";
echo "    <loc>{$base_xml}/api.html</loc>
";
echo "    <lastmod>{$today}</lastmod>
";
echo "    <changefreq>weekly</changefreq>
";
echo "    <priority>0.8</priority>
";
echo "  </url>
";

echo "  <url>
";
echo "    <loc>{$base_xml}/site/</loc>
";
echo "    <lastmod>{$today}</lastmod>
";
echo "    <changefreq>weekly</changefreq>
";
echo "    <priority>0.6</priority>
";
echo "  </url>
";

echo '</urlset>' . "\n";

$DB->close();
