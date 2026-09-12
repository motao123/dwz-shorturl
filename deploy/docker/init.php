<?php
/**
 * dwz-shorturl 一键部署初始化器（一次性容器）。
 *
 * 职责：让「拉起来就能用」成立，把原本需要手工做的四件事自动化：
 *   1. 等待 MySQL / Redis 就绪
 *   2. 在【单个数据库】里跑完全部迁移（管理表 + 公共表同库，
 *      即 $admin_db_same_as_main = true 的单库模式）
 *   3. 生成 PHP 前台用的 config.php（含随机 member_secret）
 *   4. 创建管理员账号（用户名/密码来自环境变量），并打印一次性口令
 *
 * 设计约束：
 *   - 幂等：重复执行不重复建表、不覆盖已改过的管理员密码（除非显式重置）
 *   - 不打印任何密码到非 stdout 通道，避免进入容器日志以外的位置
 *   - 失败即非 0 退出，让编排层停在「未就绪」而不是「看起来起来了」
 */

error_reporting(E_ALL);
ini_set('display_errors', 'stderr');
mysqli_report(MYSQLI_REPORT_OFF);

// ---------------- 环境变量 ----------------
function env(string $key, string $default = ''): string
{
    $v = getenv($key);
    return ($v === false || $v === '') ? $default : $v;
}

$dbHost = env('DB_HOST', 'mysql');
$dbPort = (int) env('DB_PORT', '3306');
$dbUser = env('DB_USER', 'dwz');
$dbPass = env('DB_PASSWORD');
$dbName = env('DB_NAME', 'dwz');
$publicBaseUrl = rtrim(env('PUBLIC_BASE_URL', 'http://localhost'), '/');
$adminUser = env('ADMIN_USERNAME', 'admin');
$adminPass = env('ADMIN_PASSWORD');
$adminEmail = env('ADMIN_EMAIL', 'admin@localhost');
$appKey = env('APP_SECRET_KEY');
$resetAdmin = env('RESET_ADMIN_PASSWORD', '') === '1';

function logLine(string $msg, bool $err = false): void
{
    fwrite($err ? STDERR : STDOUT, ($err ? '[ERR] ' : '[OK]  ') . $msg . PHP_EOL);
}

function fatal(string $msg): void
{
    logLine($msg, true);
    exit(1);
}

if ($dbPass === '') {
    fatal('缺少 DB_PASSWORD。请在 .env 中设置（见 deploy/.env.example）。');
}
if ($adminPass === '') {
    fatal('缺少 ADMIN_PASSWORD。一键部署必须显式设置管理员初始密码，不提供默认弱口令。');
}
if (strlen($adminPass) < 8) {
    fatal('ADMIN_PASSWORD 至少 8 位。');
}
if ($appKey === '') {
    // 固定密钥很重要：容器重建后旧会话/密码保护短链仍然可用。
    $appKey = bin2hex(random_bytes(32));
    logLine('未提供 APP_SECRET_KEY，本次随机生成（容器重建后会失效，建议在 .env 中固定）。');
}

$dbNameSafe = preg_match('/^[A-Za-z0-9_]{1,64}$/D', $dbName) === 1;
if (!$dbNameSafe) {
    fatal('DB_NAME 只允许字母/数字/下划线。');
}

// ---------------- 1. 等待数据库 ----------------
$deadline = time() + (int) env('DB_WAIT_SECONDS', '120');
$link = null;
while (true) {
    $link = @mysqli_connect($dbHost, $dbUser, $dbPass, '', $dbPort);
    if ($link) {
        break;
    }
    if (time() > $deadline) {
        fatal('等待 MySQL 超时：' . mysqli_connect_error());
    }
    sleep(2);
}
logLine("已连接 MySQL {$dbHost}:{$dbPort}");

if (!mysqli_set_charset($link, 'utf8mb4')) {
    fatal('设置字符集失败：' . mysqli_error($link));
}
$quoted = '`' . str_replace('`', '``', $dbName) . '`';
if (!mysqli_query($link, "CREATE DATABASE IF NOT EXISTS {$quoted} DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci")
    || !mysqli_select_db($link, $dbName)) {
    fatal('创建/选择数据库失败：' . mysqli_error($link));
}
logLine("数据库就绪：{$dbName}（单库模式：管理表与公共表同库）");

// ---------------- 2. 执行统一迁移入口 ----------------
// 迁移二进制由镜像内置（backend/cmd/migrate）。它按目录扫描执行全部迁移，
// 并把版本写入 schema_migrations；单库模式下两个角色解析到同一个库，
// 因此版本表只有一张、不会出现「一边已应用一边待应用」。
// 邮件配置：全部来自环境变量（见 .env.example「邮件（SMTP）」段）。
$smtpHost = env('SMTP_HOST', '');
$smtpPort = env('SMTP_PORT', '465');
$smtpUser = env('SMTP_USER', '');
$smtpPassword = env('SMTP_PASSWORD', '');
$smtpFrom = env('SMTP_FROM', '') !== '' ? env('SMTP_FROM', '') : $smtpUser;
$smtpFromName = env('SMTP_FROM_NAME', '陌涛短链');
$smtpSSL = !in_array(strtolower((string) env('SMTP_SSL', 'true')), array('0', 'false', 'no'), true);

$configDir = '/app/configs';
if (!is_dir($configDir) && !@mkdir($configDir, 0700, true)) {
    fatal("无法创建配置目录 {$configDir}");
}
$yamlPath = $configDir . '/config.yaml';
$yaml = "server:\n"
    . "  port: 8080\n"
    . "  mode: release\n"
    . "database:\n"
    . "  host: " . yamlQuote($dbHost) . "\n"
    . "  port: {$dbPort}\n"
    . "  user: " . yamlQuote($dbUser) . "\n"
    . "  password: " . yamlQuote($dbPass) . "\n"
    . "  dbname: " . yamlQuote($dbName) . "\n"
    . "  charset: utf8mb4\n"
    // 单库模式：public_db 指向同一个库，迁移工具据此把两个角色折叠成一个 schema。
    . "public_db:\n"
    . "  host: " . yamlQuote($dbHost) . "\n"
    . "  port: {$dbPort}\n"
    . "  user: " . yamlQuote($dbUser) . "\n"
    . "  password: " . yamlQuote($dbPass) . "\n"
    . "  dbname: " . yamlQuote($dbName) . "\n"
    . "  charset: utf8mb4\n"
    . "jwt:\n"
    . "  secret: " . yamlQuote($appKey) . "\n"
    . "  member_secret: " . yamlQuote($appKey) . "\n"
    . "public:\n"
    . "  base_url: " . yamlQuote($publicBaseUrl) . "\n"
    . "redis:\n"
    . "  addr: " . yamlQuote(env('REDIS_ADDR', 'redis:6379')) . "\n"
    . "  password: " . yamlQuote(env('REDIS_PASSWORD', '')) . "\n"
    . "rate_limit:\n"
    . "  single_max: 20\n"
    . "  single_window: 60\n"
    . "  batch_max: 100\n"
    . "  batch_window: 60\n"
    . "log:\n"
    . "  level: info\n"
    // 邮件（可选）：不填 host/user/password 时后端会明确提示「邮件服务未配置」，
    // 而不是泛化的发送失败，方便自助部署者定位。
    . "smtp:\n"
    . "  host: " . yamlQuote($smtpHost) . "\n"
    . "  port: " . (int) $smtpPort . "\n"
    . "  user: " . yamlQuote($smtpUser) . "\n"
    . "  password: " . yamlQuote($smtpPassword) . "\n"
    . "  from: " . yamlQuote($smtpFrom) . "\n"
    . "  from_name: " . yamlQuote($smtpFromName) . "\n"
    . "  ssl: " . ($smtpSSL ? 'true' : 'false') . "\n";
if (file_put_contents($yamlPath, $yaml, LOCK_EX) === false) {
    fatal("写入 {$yamlPath} 失败");
}
@chmod($yamlPath, 0600);
logLine('已生成迁移用 config.yaml（0700 目录 / 0600 文件）');
if ($smtpHost === '' || $smtpUser === '' || $smtpPassword === '') {
    logLine('⚠️ 未配置 SMTP：会员「忘记密码」与邮箱验证不可用（批量生成需邮箱已验证）。');
    logLine('   如需启用，请在 .env 填写 SMTP_HOST / SMTP_USER / SMTP_PASSWORD 后重启 init 容器。');
} else {
    logLine("📧 SMTP 已配置：{$smtpHost}:{$smtpPort}（发件人 {$smtpFrom}）");
}

function yamlQuote(string $v): string
{
    return '"' . str_replace(['\\', '"'], ['\\\\', '\\"'], $v) . '"';
}

$migrateOut = [];
$migrateCode = 0;
exec('/usr/local/bin/migrate -config ' . escapeshellarg($yamlPath)
    . ' -migrations /app/backend/migrations -all 2>&1', $migrateOut, $migrateCode);
foreach ($migrateOut as $line) {
    logLine($line);
}
if ($migrateCode !== 0) {
    fatal("迁移失败（退出码 {$migrateCode}），请查看上方输出。");
}
logLine('数据库迁移完成');

// ---------------- 3. 设置管理员账号密码 ----------------
// users 表由 schema.sql 建立；密码用 bcrypt（与 Go 侧 pkg.HashPassword 一致）。
$hash = password_hash($adminPass, PASSWORD_BCRYPT);
if ($hash === false) {
    fatal('生成密码哈希失败');
}

$stmt = mysqli_prepare($link, 'SELECT id FROM users WHERE username = ? LIMIT 1');
mysqli_stmt_bind_param($stmt, 's', $adminUser);
mysqli_stmt_execute($stmt);
mysqli_stmt_store_result($stmt);
$exists = mysqli_stmt_num_rows($stmt) > 0;
mysqli_stmt_close($stmt);

if ($exists && !$resetAdmin) {
    logLine("管理员 {$adminUser} 已存在，保留现有密码（如需重置请设 RESET_ADMIN_PASSWORD=1）");
} else {
    if ($exists) {
        $stmt = mysqli_prepare($link, 'UPDATE users SET password_hash = ?, status = 1 WHERE username = ?');
        mysqli_stmt_bind_param($stmt, 'ss', $hash, $adminUser);
        $ok = mysqli_stmt_execute($stmt);
        mysqli_stmt_close($stmt);
        if (!$ok) {
            fatal('重置管理员密码失败：' . mysqli_error($link));
        }
        logLine("已重置管理员 {$adminUser} 的密码");
    } else {
        $stmt = mysqli_prepare($link,
            'INSERT INTO users (username, email, password_hash, display_name, status) VALUES (?,?,?,?,1)');
        $display = '系统管理员';
        mysqli_stmt_bind_param($stmt, 'ssss', $adminUser, $adminEmail, $hash, $display);
        if (!mysqli_stmt_execute($stmt)) {
            fatal('创建管理员失败：' . mysqli_stmt_error($stmt));
        }
        $adminId = (int) mysqli_insert_id($link);
        mysqli_stmt_close($stmt);
        logLine("已创建管理员 {$adminUser}（id={$adminId}）");

        // 绑定 super_admin（seed 由 schema.sql 之外的 seed 逻辑负责；这里直接按名字找，
        // 找不到就直接以 all-permissions 的方式插入角色关联，避免依赖角色 id 顺序）。
        $stmt = mysqli_prepare($link, 'SELECT id FROM roles WHERE name = ? LIMIT 1');
        $roleName = 'super_admin';
        mysqli_stmt_bind_param($stmt, 's', $roleName);
        mysqli_stmt_execute($stmt);
        mysqli_stmt_bind_result($stmt, $roleId);
        $found = mysqli_stmt_fetch($stmt);
        mysqli_stmt_close($stmt);
        if ($found) {
            $stmt = mysqli_prepare($link,
                'INSERT IGNORE INTO user_roles (user_id, role_id) VALUES (?,?)');
            mysqli_stmt_bind_param($stmt, 'ii', $adminId, $roleId);
            if (mysqli_stmt_execute($stmt)) {
                logLine("已授予 {$adminUser} 角色 super_admin（role_id={$roleId}）");
            } else {
                logLine('授予角色失败：' . mysqli_stmt_error($stmt), true);
            }
            mysqli_stmt_close($stmt);
        } else {
            logLine('未找到 super_admin 角色，跳过授权（请检查 seed 数据）', true);
        }
    }
}

// ---------------- 4. 生成 PHP 前台 config.php ----------------
// PHP 跳转路径（do.php）与 Go 路径共享同一份库表，因此这里必须是单库配置：
// $admin_db_* 直接复用主库连接（$admin_db_same_as_main = true），
// 双写退化为同库写入，不再需要第二个库的账号与权限。
// 用占位符模板而不是逐行拼接：PHP 里 '\$host' 是字面量反斜杠，很容易把生成的
// 配置文件写坏（曾实际产出 '\$host = ...'）。模板 + strtr 只替换值，不碰代码本身。
$phpConfigTemplate = <<<'PHPTPL'
<?php
// 由一键部署初始化器自动生成（deploy/docker/init.php）。请勿手工提交到版本库。
$host = '{{DB_HOST}}';
$port = {{DB_PORT}};
$user = '{{DB_USER}}';
$pwd = '{{DB_PASSWORD}}';
$dbname = '{{DB_NAME}}';

// 对外短链规范地址（反向代理后填真实对外地址，如 https://s.example.com）。
$public_base_url = '{{PUBLIC_BASE_URL}}';

// 反向代理 IP 白名单：仅当 REMOTE_ADDR 命中此处才信任 X-Forwarded-For。
// 代理层请使用覆盖式赋值 proxy_set_header X-Forwarded-For $remote_addr;
// 不要用 $proxy_add_x_forwarded_for，否则客户端可自行注入 XFF 绕过限流。
$trusted_proxies = array();

// 单库模式：管理库与公共库是同一个库，双写连接复用主库。
// 不需要第二个库的账号与权限，也不会有跨库 COLLATE 差异。
$admin_db_same_as_main = true;
if ($admin_db_same_as_main) {
    $admin_db_host = $host;
    $admin_db_port = $port;
    $admin_db_user = $user;
    $admin_db_pwd = $pwd;
    $admin_db_name = $dbname;
} else {
    $admin_db_host = '127.0.0.1';
    $admin_db_port = 3306;
    $admin_db_user = '';
    $admin_db_pwd = '';
    $admin_db_name = '';
}

$rate_limit_dir = __DIR__ . '/logs/ratelimit';

// 必须与后端 config.yaml 的 jwt.member_secret 一致，否则 PHP 与 Go 两条跳转
// 路径的密码解锁 cookie 互不认可（设置了访问密码的短链永远打不开）。
$member_secret = '{{APP_SECRET_KEY}}';

$stats_enabled = false;
$stats_token = '';
PHPTPL;

// 值里的单引号会截断 PHP 单引号字符串，转义后再替换。
$phpEscape = static function (string $v): string {
    return str_replace(['\\', "'"], ['\\\\', "\\'"], $v);
};

$phpConfig = strtr($phpConfigTemplate, [
    '{{DB_HOST}}'         => $phpEscape($dbHost),
    '{{DB_PORT}}'         => (string) $dbPort,
    '{{DB_USER}}'         => $phpEscape($dbUser),
    '{{DB_PASSWORD}}'     => $phpEscape($dbPass),
    '{{DB_NAME}}'         => $phpEscape($dbName),
    '{{PUBLIC_BASE_URL}}' => $phpEscape($publicBaseUrl),
    '{{APP_SECRET_KEY}}'  => $phpEscape($appKey),
]);

$phpConfigPath = $configDir . '/config.php';
if (file_put_contents($phpConfigPath, $phpConfig, LOCK_EX) === false) {
    fatal("写入 {$phpConfigPath} 失败");
}
@chmod($phpConfigPath, 0600);
@chmod($configDir, 0755); // web 容器需要读到软链目标
logLine('已生成前台 config.php（单库模式，0600）');

// ---------------- 4. 打印访问信息 ----------------
$backendUrl = 'http://127.0.0.1:' . env('HTTP_PORT', '8080');
logLine('============================================================');
logLine(' 初始化完成 —— 可以直接登录使用了');
logLine("   管理台地址 : {$publicBaseUrl}/admin/");
logLine("   登录账号   : {$adminUser}");
logLine("   登录密码   : （即 .env 中的 ADMIN_PASSWORD）");
logLine('   单库模式   : 管理表与公共表同库（' . $dbName . '）');
logLine('============================================================');
logLine('提示：登录后请立即在「系统设置」中修改密码；首次登录建议开启两步验证。');

mysqli_close($link);
exit(0);
