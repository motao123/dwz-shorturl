<?php
/**
 * 首页动态渲染：读取数据库中的 SEO/统计配置，替换 index.html 中的 PLACEHOLDER
 * Nginx 优先解析 index.php（index index.php index.html），因此本文件会先于 index.html 生效
 */
define('SYSTEM_ROOT', __DIR__ . '/includes/');

if (!file_exists(__DIR__ . '/config.php')) {
    require __DIR__ . '/index.html';
    return;
}

require __DIR__ . '/config.php';
define('IN_CRONLITE', true);
require SYSTEM_ROOT . 'db.class.php';

$DB = new DB($host, $user, $pwd, $dbname, $port ?? 3306);

// 从 system_configs 读取公开配置（is_public=1）
$configs = [];
if (!empty($DB->link)) {
    $result = $DB->link->query(
        "SELECT config_key, config_value FROM system_configs WHERE is_public = 1"
    );
    if ($result) {
        while ($row = $result->fetch_assoc()) {
            $configs[$row['config_key']] = $row['config_value'];
        }
    }
}
$DB->close();

// 读取静态模板
$html = file_get_contents(__DIR__ . '/index.html');

// 替换占位符
$replacements = [
    'codeva-PLACEHOLDER'       => $configs['seo.baidu_verify'] ?? '',
    '<meta name="google-site-verification" content="PLACEHOLDER">'
        => !empty($configs['seo.google_verify'])
            ? '<meta name="google-site-verification" content="' . htmlspecialchars($configs['seo.google_verify']) . '">'
            : '',
    '<meta name="msvalidate.01" content="PLACEHOLDER">'
        => !empty($configs['seo.bing_verify'])
            ? '<meta name="msvalidate.01" content="' . htmlspecialchars($configs['seo.bing_verify']) . '">'
            : '',
    'PLACEHOLDER_BAIDU_TONGJI_ID' => $configs['seo.baidu_tongji_id'] ?? '',
    'G-PLACEHOLDER'              => $configs['seo.ga_id'] ?? 'G-PLACEHOLDER',
];

foreach ($replacements as $search => $replace) {
    $html = str_replace($search, $replace, $html);
}

// 如果百度统计 ID 为空，移除统计代码块
if (empty($configs['seo.baidu_tongji_id'])) {
    $html = preg_replace('/\s*<!-- 百度统计 -->.*?<\/script>\s*/s', "\n", $html);
}
// 如果 GA ID 仍为占位符，移除 GA 代码块
if (strpos($html, 'G-PLACEHOLDER') !== false) {
    $html = preg_replace('/\s*<!-- Google Analytics -->.*?<\/script>\s*<\/body>/s', "\n</body>", $html);
}
// 如果百度验证码为空，移除标签
if (empty($configs['seo.baidu_verify'])) {
    $html = preg_replace('/\s*<!-- 百度站长验证 -->\s*<meta[^>]*baidu-site-verification[^>]*>\s*/s', "\n  ", $html);
}
// 如果 Google 验证码为空，移除标签
if (empty($configs['seo.google_verify'])) {
    $html = preg_replace('/\s*<!-- Google Search Console 验证 -->\s*<meta[^>]*google-site-verification[^>]*>\s*/s', "\n  ", $html);
}
// 如果 Bing 验证码为空，移除标签
if (empty($configs['seo.bing_verify'])) {
    $html = preg_replace('/\s*<!-- Bing Webmaster 验证 -->\s*<meta[^>]*msvalidate[^>]*>\s*/s', "\n  ", $html);
}

header('Content-Type: text/html; charset=utf-8');
echo $html;
