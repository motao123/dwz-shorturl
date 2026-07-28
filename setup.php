<?php
/**
 * dwz-shorturl CLI-only installer.
 *
 * php setup.php --host=127.0.0.1 --port=3306 --user=root --pwd=secret \
 *   --db=Imotao --public-url=https://s.example.com
 */
if (PHP_SAPI !== 'cli') {
    http_response_code(403);
    header('Content-Type: text/plain; charset=utf-8');
    exit("Web installation is disabled. Run setup.php from the command line.\n");
}

error_reporting(E_ALL);
// CLI installer: keep errors on stderr so diagnostics reach the operator
// without mixing into stdout or leaking paths through other channels.
ini_set('display_errors', 'stderr');
mysqli_report(MYSQLI_REPORT_OFF);

$configFile = __DIR__ . '/config.php';
$sqlFile = __DIR__ . '/install.sql';

function outputMessage($message, $ok = true)
{
    fwrite($ok ? STDOUT : STDERR, ($ok ? '[OK]  ' : '[ERR] ') . $message . PHP_EOL);
}

function failInstall($message)
{
    outputMessage($message, false);
    exit(1);
}

function parseCliArguments($argv)
{
    $params = array();
    foreach ($argv as $index => $argument) {
        if ($index > 0 && preg_match('/^--([a-zA-Z][a-zA-Z0-9_-]*)=(.*)$/s', $argument, $matches)) {
            $params[$matches[1]] = $matches[2];
        }
    }
    return $params;
}

function validDatabaseName($name)
{
    return is_string($name) && preg_match('/^[A-Za-z0-9_]{1,64}$/D', $name);
}

function normalizePublicUrl($url)
{
    $url = rtrim(trim($url), '/');
    $parts = parse_url($url);
    if ($parts === false || empty($parts['scheme']) || empty($parts['host'])
        || !in_array(strtolower($parts['scheme']), array('http', 'https'), true)
        || isset($parts['user']) || isset($parts['pass']) || isset($parts['query']) || isset($parts['fragment'])) {
        return false;
    }
    return $url;
}

function tableExists($link, $database, $table)
{
    $statement = mysqli_prepare($link, 'SELECT 1 FROM information_schema.TABLES WHERE TABLE_SCHEMA=? AND TABLE_NAME=? LIMIT 1');
    mysqli_stmt_bind_param($statement, 'ss', $database, $table);
    mysqli_stmt_execute($statement);
    mysqli_stmt_store_result($statement);
    $exists = mysqli_stmt_num_rows($statement) > 0;
    mysqli_stmt_close($statement);
    return $exists;
}

function schemaIsCompatible($link, $database)
{
    $requiredColumns = array('Id', 'uid', 'longurl', 'url_hash', 'clicks', 'created_at', 'expire_at');
    $statement = mysqli_prepare($link, "SELECT COLUMN_NAME FROM information_schema.COLUMNS WHERE TABLE_SCHEMA=? AND TABLE_NAME='wjoy_log'");
    mysqli_stmt_bind_param($statement, 's', $database);
    mysqli_stmt_execute($statement);
    mysqli_stmt_bind_result($statement, $column);
    $columns = array();
    while (mysqli_stmt_fetch($statement)) {
        $columns[] = $column;
    }
    mysqli_stmt_close($statement);
    foreach ($requiredColumns as $required) {
        if (!in_array($required, $columns, true)) {
            return false;
        }
    }

    $requiredIndexes = array('PRIMARY', 'uniq_uid', 'uniq_hash');
    $statement = mysqli_prepare($link, "SELECT DISTINCT INDEX_NAME FROM information_schema.STATISTICS WHERE TABLE_SCHEMA=? AND TABLE_NAME='wjoy_log'");
    mysqli_stmt_bind_param($statement, 's', $database);
    mysqli_stmt_execute($statement);
    mysqli_stmt_bind_result($statement, $index);
    $indexes = array();
    while (mysqli_stmt_fetch($statement)) {
        $indexes[] = $index;
    }
    mysqli_stmt_close($statement);
    foreach ($requiredIndexes as $required) {
        if (!in_array($required, $indexes, true)) {
            return false;
        }
    }
    return true;
}

function importSchema($link, $sqlFile)
{
    if (!is_file($sqlFile) || !is_readable($sqlFile)) {
        return '找不到或无法读取 install.sql';
    }
    $sql = file_get_contents($sqlFile);
    if ($sql === false) {
        return '读取 install.sql 失败';
    }
    $sql = preg_replace('/^\xEF\xBB\xBF/', '', $sql);
    if (!mysqli_multi_query($link, $sql)) {
        return '导入表结构失败：' . mysqli_error($link);
    }
    do {
        if ($result = mysqli_store_result($link)) {
            mysqli_free_result($result);
        }
        if (!mysqli_more_results($link)) {
            break;
        }
    } while (mysqli_next_result($link));
    return mysqli_errno($link) ? '导入表结构失败：' . mysqli_error($link) : true;
}

if (file_exists($configFile)) {
    failInstall('检测到 config.php 已存在，安装已硬停止。现有站点升级请运行 php migrations/legacy_schema.php。');
}
if (!extension_loaded('mysqli')) {
    failInstall('缺少 mysqli PHP 扩展');
}

$params = parseCliArguments($_SERVER['argv']);
if (empty($params['host']) || empty($params['public-url'])) {
    failInstall('用法: php setup.php --host=127.0.0.1 --port=3306 --user=root --pwd=密码 --db=Imotao --public-url=https://s.example.com');
}

$host = trim($params['host']);
$port = isset($params['port']) ? (int) $params['port'] : 3306;
$user = isset($params['user']) ? $params['user'] : 'root';
$password = isset($params['pwd']) ? $params['pwd'] : '';
$database = isset($params['db']) ? $params['db'] : 'Imotao';
$publicBaseUrl = normalizePublicUrl($params['public-url']);
if ($host === '' || $port < 1 || $port > 65535 || !validDatabaseName($database) || $publicBaseUrl === false) {
    failInstall('参数无效：port 必须为 1-65535，db 只能含字母/数字/下划线；public-url 必须为不含凭据、查询或片段的 http/https 地址。');
}

$link = @mysqli_connect($host, $user, $password, '', $port);
if (!$link) {
    failInstall('无法连接 MySQL 服务器：' . mysqli_connect_error());
}
if (!mysqli_set_charset($link, 'utf8mb4')) {
    failInstall('设置数据库字符集失败：' . mysqli_error($link));
}
$quotedDatabase = '`' . str_replace('`', '``', $database) . '`';
if (!mysqli_query($link, 'CREATE DATABASE IF NOT EXISTS ' . $quotedDatabase . ' DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci')
    || !mysqli_select_db($link, $database)) {
    failInstall('创建或选择数据库失败：' . mysqli_error($link));
}

if (tableExists($link, $database, 'wjoy_log') && !schemaIsCompatible($link, $database)) {
    mysqli_close($link);
    failInstall(
        "检测到现有但不兼容的 wjoy_log，安装已停止且未写入 config.php。\n"
        . "先运行：php migrations/legacy_schema.php --host=" . escapeshellarg($host)
        . " --port={$port} --user=" . escapeshellarg($user) . " --pwd=... --db=" . escapeshellarg($database)
        . "\n处理迁移报告中的重复/空值后重跑迁移，再重新运行 setup.php。"
    );
}

$schemaResult = importSchema($link, $sqlFile);
if ($schemaResult !== true) {
    mysqli_close($link);
    failInstall($schemaResult);
}

$config = "<?php\n";
$config .= "// 数据库信息\n";
$config .= '$host = ' . var_export($host, true) . ";\n";
$config .= '$port = ' . $port . ";\n";
$config .= '$user = ' . var_export($user, true) . ";\n";
$config .= '$pwd = ' . var_export($password, true) . ";\n";
$config .= '$dbname = ' . var_export($database, true) . ";\n";
$config .= "\n// 对外短链规范地址，可包含子目录。\n";
$config .= '$public_base_url = ' . var_export($publicBaseUrl, true) . ";\n";
$config .= "\$trusted_proxies = array();\n";
$config .= "\$rate_limit_dir = __DIR__ . '/logs/ratelimit';\n";
$config .= "\$stats_enabled = false;\n";
$config .= "\$stats_token = '';\n";

$temporary = $configFile . '.tmp.' . bin2hex(random_bytes(6));
if (file_put_contents($temporary, $config, LOCK_EX) === false) {
    mysqli_close($link);
    failInstall('写入临时配置失败，请检查项目目录权限');
}
@chmod($temporary, 0600);
if (file_exists($configFile) || !@rename($temporary, $configFile)) {
    @unlink($temporary);
    mysqli_close($link);
    failInstall('config.php 在安装期间出现或无法原子写入；未覆盖任何配置');
}

mysqli_close($link);
outputMessage('安装成功。已生成 config.php；请立即删除 setup.php。');
