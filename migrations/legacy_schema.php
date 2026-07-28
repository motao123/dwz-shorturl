<?php
/**
 * Non-destructive legacy schema migration for dwz-shorturl.
 *
 * Run from the project root:
 *   php migrations/legacy_schema.php
 * Preview without writes:
 *   php migrations/legacy_schema.php --dry-run
 *
 * The script reads the existing config.php, adds missing columns cautiously,
 * normalizes valid legacy base64 URLs to plain URLs, fills url_hash, and adds
 * indexes only when current data permits. It never drops tables or rows.
 */
if (PHP_SAPI !== 'cli') {
    http_response_code(403);
    exit("CLI only\n");
}

error_reporting(E_ALL);
ini_set('display_errors', 'stderr');
mysqli_report(MYSQLI_REPORT_OFF);

$projectRoot = dirname(__DIR__);
$configFile = $projectRoot . '/config.php';
$argv = isset($_SERVER['argv']) ? $_SERVER['argv'] : array();
$dryRun = in_array('--dry-run', $argv, true);
$cliParams = array();
foreach ($argv as $argument) {
    if (preg_match('/^--([a-zA-Z][a-zA-Z0-9_-]*)=(.*)$/s', $argument, $matches)) {
        $cliParams[$matches[1]] = $matches[2];
    }
}

function logLine($message, $error = false)
{
    fwrite($error ? STDERR : STDOUT, ($error ? '[ERR] ' : '[OK]  ') . $message . PHP_EOL);
}

function abortMigration($message)
{
    logLine($message, true);
    exit(1);
}

function quoteIdentifier($name)
{
    return '`' . str_replace('`', '``', $name) . '`';
}

function columnExists($link, $database, $table, $column)
{
    $statement = mysqli_prepare($link, 'SELECT 1 FROM information_schema.COLUMNS WHERE TABLE_SCHEMA=? AND TABLE_NAME=? AND COLUMN_NAME=? LIMIT 1');
    mysqli_stmt_bind_param($statement, 'sss', $database, $table, $column);
    mysqli_stmt_execute($statement);
    mysqli_stmt_store_result($statement);
    $exists = mysqli_stmt_num_rows($statement) > 0;
    mysqli_stmt_close($statement);
    return $exists;
}

function indexExists($link, $database, $table, $index)
{
    $statement = mysqli_prepare($link, 'SELECT 1 FROM information_schema.STATISTICS WHERE TABLE_SCHEMA=? AND TABLE_NAME=? AND INDEX_NAME=? LIMIT 1');
    mysqli_stmt_bind_param($statement, 'sss', $database, $table, $index);
    mysqli_stmt_execute($statement);
    mysqli_stmt_store_result($statement);
    $exists = mysqli_stmt_num_rows($statement) > 0;
    mysqli_stmt_close($statement);
    return $exists;
}

function runAlter($link, $sql, $description, $dryRun)
{
    if ($dryRun) {
        logLine('[dry-run] ' . $description . ': ' . $sql);
        return;
    }
    if (!mysqli_query($link, $sql)) {
        abortMigration($description . '失败：' . mysqli_error($link));
    }
    logLine($description);
}

// Run a COUNT(*) query and return the integer, or abort if the query fails.
// Returning 0 on a failed query would be dangerous: callers decide whether
// it is safe to create a UNIQUE index based on "0 duplicates".
function countSql($link, $sql, $description)
{
    $result = mysqli_query($link, $sql);
    if (!$result) {
        abortMigration($description . '失败：' . mysqli_error($link));
    }
    $row = mysqli_fetch_row($result);
    mysqli_free_result($result);
    return $row ? (int) $row[0] : 0;
}

function decodeLegacyUrl($value)
{
    if (preg_match('#^https?://#i', $value)) {
        return $value;
    }
    $decoded = base64_decode($value, true);
    if ($decoded === false || !filter_var($decoded, FILTER_VALIDATE_URL)) {
        return $value;
    }
    $canonicalInput = rtrim($value, '=');
    if (!hash_equals(rtrim(base64_encode($decoded), '='), $canonicalInput)) {
        return $value;
    }
    return $decoded;
}

if (is_file($configFile)) {
    require $configFile;
}
$host = isset($cliParams['host']) ? $cliParams['host'] : (isset($host) ? $host : null);
$user = isset($cliParams['user']) ? $cliParams['user'] : (isset($user) ? $user : null);
$pwd = isset($cliParams['pwd']) ? $cliParams['pwd'] : (isset($pwd) ? $pwd : '');
$dbname = isset($cliParams['db']) ? $cliParams['db'] : (isset($dbname) ? $dbname : null);
$port = isset($cliParams['port']) ? (int) $cliParams['port'] : (isset($port) ? (int) $port : 3306);
if ($host === null || $user === null || $dbname === null) {
    abortMigration('未找到完整数据库配置。请保留现有 config.php，或传入 --host --port --user --pwd --db 参数。');
}
if (!extension_loaded('mysqli')) {
    abortMigration('缺少 mysqli PHP 扩展');
}

$link = @mysqli_connect($host, $user, $pwd, $dbname, $port);
if (!$link) {
    abortMigration('数据库连接失败：' . mysqli_connect_error());
}
if (!mysqli_set_charset($link, 'utf8mb4')) {
    abortMigration('设置数据库字符集失败：' . mysqli_error($link));
}

$table = 'wjoy_log';
$tableEscaped = quoteIdentifier($table);
$tableCheck = mysqli_prepare($link, 'SELECT 1 FROM information_schema.TABLES WHERE TABLE_SCHEMA=? AND TABLE_NAME=? LIMIT 1');
mysqli_stmt_bind_param($tableCheck, 'ss', $dbname, $table);
mysqli_stmt_execute($tableCheck);
mysqli_stmt_store_result($tableCheck);
$tableExists = mysqli_stmt_num_rows($tableCheck) > 0;
mysqli_stmt_close($tableCheck);
if (!$tableExists) {
    abortMigration('未找到 wjoy_log 表；全新安装请运行 setup.php');
}

$columnDefinitions = array(
    'url_hash' => "char(32) NOT NULL DEFAULT '' AFTER `longurl`",
    'clicks' => 'int unsigned NOT NULL DEFAULT 0 AFTER `url_hash`',
    'created_at' => 'timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP AFTER `clicks`',
    'expire_at' => 'datetime DEFAULT NULL AFTER `created_at`',
);
foreach ($columnDefinitions as $column => $definition) {
    if (!columnExists($link, $dbname, $table, $column)) {
        runAlter($link, 'ALTER TABLE ' . $tableEscaped . ' ADD COLUMN ' . quoteIdentifier($column) . ' ' . $definition, '添加列 ' . $column, $dryRun);
    } else {
        logLine('列 ' . $column . ' 已存在');
    }
}

if ($dryRun) {
    logLine('dry-run 不扫描或修改 URL 数据，也不创建依赖数据状态的唯一索引');
    mysqli_close($link);
    exit(0);
}

$result = mysqli_query($link, 'SELECT `Id`, `longurl` FROM ' . $tableEscaped . ' ORDER BY `Id`');
if (!$result) {
    abortMigration('读取旧数据失败：' . mysqli_error($link));
}
$update = mysqli_prepare($link, 'UPDATE ' . $tableEscaped . ' SET `longurl`=?, `url_hash`=? WHERE `Id`=?');
if (!$update) {
    abortMigration('准备 URL 更新语句失败：' . mysqli_error($link));
}

$converted = 0;
$hashed = 0;
$invalid = 0;
$hashConflicts = 0;
while ($row = mysqli_fetch_assoc($result)) {
    $stored = (string) $row['longurl'];
    $plain = decodeLegacyUrl($stored);
    if (!filter_var($plain, FILTER_VALIDATE_URL) || !preg_match('#^https?://#i', $plain)) {
        $invalid++;
        continue;
    }
    $hash = md5($plain);
    $id = (int) $row['Id'];
    mysqli_stmt_bind_param($update, 'ssi', $plain, $hash, $id);
    if (!mysqli_stmt_execute($update)) {
        if (mysqli_stmt_errno($update) === 1062) {
            $hashConflicts++;
            continue;
        }
        abortMigration('更新 Id=' . $id . ' 失败：' . mysqli_stmt_error($update));
    }
    if ($plain !== $stored) {
        $converted++;
    }
    $hashed++;
}
mysqli_stmt_close($update);
mysqli_free_result($result);
logLine("URL 迁移完成：{$converted} 条 base64 转为明文，{$hashed} 条写入哈希，{$invalid} 条无效/非 HTTP(S) 数据保持不变，{$hashConflicts} 条因现有唯一哈希冲突保持不变");

$uidDuplicates = countSql($link, "SELECT COUNT(*) FROM (SELECT `uid` FROM {$tableEscaped} WHERE `uid` IS NOT NULL AND `uid` <> '' GROUP BY `uid` HAVING COUNT(*) > 1) AS d", '统计重复短码');
$hashDuplicates = countSql($link, "SELECT COUNT(*) FROM (SELECT `url_hash` FROM {$tableEscaped} WHERE `url_hash` <> '' GROUP BY `url_hash` HAVING COUNT(*) > 1) AS d", '统计重复 URL');
$emptyUids = countSql($link, "SELECT COUNT(*) FROM {$tableEscaped} WHERE `uid` IS NULL OR `uid` = ''", '统计空短码');
$emptyHashes = countSql($link, "SELECT COUNT(*) FROM {$tableEscaped} WHERE `url_hash` = ''", '统计空哈希');

$indexFailure = false;
if (!indexExists($link, $dbname, $table, 'uniq_uid')) {
    if ($uidDuplicates === 0 && $emptyUids === 0) {
        runAlter($link, 'ALTER TABLE ' . $tableEscaped . ' ADD UNIQUE KEY `uniq_uid` (`uid`)', '创建唯一索引 uniq_uid', false);
    } else {
        logLine("无法创建 uniq_uid：存在 {$uidDuplicates} 组重复短码、{$emptyUids} 条空短码，请人工处理后重跑迁移", true);
        $indexFailure = true;
    }
}
if (!indexExists($link, $dbname, $table, 'uniq_hash')) {
    if ($hashDuplicates === 0 && $emptyHashes === 0) {
        runAlter($link, 'ALTER TABLE ' . $tableEscaped . ' ADD UNIQUE KEY `uniq_hash` (`url_hash`)', '创建唯一索引 uniq_hash', false);
    } else {
        logLine("无法创建 uniq_hash：存在 {$hashDuplicates} 组重复 URL、{$emptyHashes} 条空哈希；数据未删除，请人工决定如何合并后重跑迁移", true);
        $indexFailure = true;
    }
}
if (!indexExists($link, $dbname, $table, 'idx_created_at')) {
    runAlter($link, 'ALTER TABLE ' . $tableEscaped . ' ADD KEY `idx_created_at` (`created_at`)', '创建索引 idx_created_at', false);
}
if (!indexExists($link, $dbname, $table, 'idx_clicks')) {
    runAlter($link, 'ALTER TABLE ' . $tableEscaped . ' ADD KEY `idx_clicks` (`clicks`)', '创建索引 idx_clicks', false);
}

mysqli_close($link);
if ($indexFailure) {
    abortMigration('迁移未完成：必需的唯一索引未全部安装。修复以上数据问题后重新运行本脚本。');
}
logLine('迁移完成。未删除任何表或记录。');
