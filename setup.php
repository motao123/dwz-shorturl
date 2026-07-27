<?php
/**
 * dwz-shorturl 一键部署 / 安装脚本
 * ---------------------------------------------------------------
 * Web 方式：浏览器访问 setup.php，填写数据库信息后一键完成
 *           ① 创建数据库  ② 导入数据表结构(install.sql)  ③ 生成 config.php
 * CLI 方式：php setup.php --host=127.0.0.1 --port=3306 --user=root --pwd=root --db=Imotao
 *
 * ⚠️ 安全提示：安装完成后请删除本文件，避免被他人重新写入配置。
 */
error_reporting(E_ALL);
ini_set('display_errors', 1);

$isCli = (PHP_SAPI === 'cli');

// 收集参数
$params = [];
if ($isCli) {
    foreach ($_SERVER['argv'] as $i => $a) {
        if ($i === 0) continue;
        if (preg_match('/^--([a-zA-Z]+)=(.*)$/', $a, $m)) $params[$m[1]] = $m[2];
    }
} else {
    $params = $_POST ?: $_GET;
}

function out($msg, $ok = true) {
    if (PHP_SAPI === 'cli') {
        fwrite(STDOUT, ($ok ? "[OK]   " : "[ERR]  ") . $msg . "\n");
    } else {
        $c = $ok ? '#2e7d32' : '#c62828';
        echo "<div style='padding:8px 12px;margin:6px 0;border-left:4px solid {$c};background:#fafafa;font-family:monospace;white-space:pre-wrap'>" . htmlspecialchars($msg) . "</div>";
    }
}

function doInstall($host, $port, $user, $pwd, $dbname) {
    $link = @mysqli_connect($host, $user, $pwd, '', (int)$port);
    if (!$link) return "无法连接 MySQL 服务器：" . mysqli_connect_error();

    mysqli_set_charset($link, 'utf8mb4');

    // ① 建库
    $escDb = mysqli_real_escape_string($link, $dbname);
    if (!mysqli_query($link, "CREATE DATABASE IF NOT EXISTS `{$escDb}` DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci")) {
        return "创建数据库失败：" . mysqli_error($link);
    }
    if (!mysqli_select_db($link, $dbname)) return "选择数据库失败：" . mysqli_error($link);

    // ② 导入结构
    if (!is_file(__DIR__ . '/install.sql')) return "未找到 install.sql";
    $sql = ltrim(file_get_contents(__DIR__ . '/install.sql'), "\xEF\xBB\xBF");
    $stmts = preg_split('/;\s*$/m', $sql);
    $okCount = 0;
    $failMsg = '';
    foreach ($stmts as $st) {
        $st = trim($st);
        if ($st === '' || strpos($st, '--') === 0) continue;
        if (mysqli_query($link, $st)) {
            $okCount++;
        } else {
            $failMsg = mysqli_error($link) . "  <near: " . substr($st, 0, 50) . ">";
            break;
        }
    }
    if ($failMsg !== '') return "导入数据表结构失败：{$failMsg}（此前已成功 {$okCount} 条）";

    // ③ 生成 config.php
    $cfg = "<?php\n";
    $cfg .= "//数据库信息\n";
    $cfg .= '$host = ' . var_export($host, true) . "; //数据库地址\n";
    $cfg .= '$port = ' . ((int)$port) . "; //端口\n";
    $cfg .= '$user = ' . var_export($user, true) . "; //用户名\n";
    $cfg .= '$pwd = ' . var_export($pwd, true) . "; //密码\n";
    $cfg .= '$dbname = ' . var_export($dbname, true) . "; //库名称\n";
    $cfg .= "//其他配置\n";
    $cfg .= "?>";
    if (file_put_contents(__DIR__ . '/config.php', $cfg) === false) {
        return "写入 config.php 失败，请检查目录写权限";
    }
    mysqli_close($link);
    return true;
}

$installed = file_exists(__DIR__ . '/config.php');
$ran = false;
$result = null;
if (!empty($params['host'])) {
    $ran = true;
    $result = doInstall(
        $params['host'],
        isset($params['port']) ? $params['port'] : 3306,
        isset($params['user']) ? $params['user'] : 'root',
        isset($params['pwd']) ? $params['pwd'] : '',
        isset($params['db']) ? $params['db'] : 'Imotao'
    );
}

if ($isCli) {
    if (!$ran) {
        echo "用法: php setup.php --host=127.0.0.1 --port=3306 --user=root --pwd=密码 --db=Imotao\n";
        exit(1);
    }
    if ($result === true) {
        out("安装成功！config.php 已生成，请删除 setup.php");
        exit(0);
    }
    out($result, false);
    exit(1);
}

// ---- Web 输出 ----
header('Content-Type: text/html; charset=utf-8');
?>
<!doctype html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>dwz-shorturl 安装</title>
<style>
 body{font-family:system-ui,-apple-system,"PingFang SC",sans-serif;max-width:560px;margin:40px auto;padding:0 16px;color:#222}
 h1{font-size:20px} label{display:block;margin:12px 0 4px;font-size:13px;color:#555}
 input{width:100%;padding:8px 10px;border:1px solid #ccc;border-radius:6px;box-sizing:border-box;font-size:14px}
 button{margin-top:18px;width:100%;padding:10px;background:#2e7d32;color:#fff;border:0;border-radius:6px;font-size:15px;cursor:pointer}
 .warn{background:#fff8e1;border:1px solid #ffe082;padding:10px 12px;border-radius:6px;font-size:13px;color:#795548;margin:10px 0}
 .ok{background:#e8f5e9;border:1px solid #a5d6a7;color:#1b5e20}
</style>
</head>
<body>
<h1>dwz-shorturl 一键安装</h1>
<?php if ($installed): ?>
<div class="warn">检测到 <b>config.php</b> 已存在。重新填写将覆盖该文件；如需全新安装，请先删除 config.php。</div>
<?php endif; ?>
<?php if ($ran): ?>
  <?php if ($result === true): ?>
    <div class="warn ok">✅ 安装成功！config.php 已生成。请立即<b>删除 setup.php</b>，然后访问 <a href="index.html">index.html</a> 使用。</div>
  <?php else: ?>
    <?php out($result, false); ?>
  <?php endif; ?>
<?php endif; ?>
<form method="post">
  <label>数据库地址</label><input name="host" value="127.0.0.1" required>
  <label>端口</label><input name="port" value="3306" required>
  <label>用户名</label><input name="user" value="root" required>
  <label>密码</label><input name="pwd" type="password">
  <label>数据库名</label><input name="db" value="Imotao" required>
  <button type="submit">一键安装</button>
</form>
<p style="font-size:12px;color:#999">本脚本会创建数据库、导入 install.sql 并生成 config.php。完成后请删除本文件。</p>
</body>
</html>
