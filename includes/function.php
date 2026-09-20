<?php
function trusted_proxy_match($ip, $trusted_proxies) {
	if (!filter_var($ip, FILTER_VALIDATE_IP) || !is_array($trusted_proxies)) return false;
	foreach ($trusted_proxies as $proxy) {
		$proxy = trim((string)$proxy);
		if ($proxy === $ip) return true;
		if (strpos($proxy, '/') === false) continue;
		list($network, $bits) = array_pad(explode('/', $proxy, 2), 2, null);
		$ip_bin = @inet_pton($ip);
		$network_bin = @inet_pton($network);
		if ($ip_bin === false || $network_bin === false || strlen($ip_bin) !== strlen($network_bin)) continue;
		$bits = (int)$bits;
		$max_bits = strlen($ip_bin) * 8;
		if ($bits < 0 || $bits > $max_bits) continue;
		$bytes = intdiv($bits, 8);
		$remainder = $bits % 8;
		if (substr($ip_bin, 0, $bytes) !== substr($network_bin, 0, $bytes)) continue;
		if ($remainder === 0) return true;
		$mask = (0xff << (8 - $remainder)) & 0xff;
		if ((ord($ip_bin[$bytes]) & $mask) === (ord($network_bin[$bytes]) & $mask)) return true;
	}
	return false;
}

function real_ip(){
	global $trusted_proxies;
	$remote = isset($_SERVER['REMOTE_ADDR']) ? trim($_SERVER['REMOTE_ADDR']) : '';
	if (!filter_var($remote, FILTER_VALIDATE_IP)) return '0.0.0.0';
	$trusted = isset($trusted_proxies) && is_array($trusted_proxies) ? $trusted_proxies : array();
	if (!trusted_proxy_match($remote, $trusted)) return $remote;

	$forwarded = isset($_SERVER['HTTP_X_FORWARDED_FOR']) ? explode(',', $_SERVER['HTTP_X_FORWARDED_FOR']) : array();
	$forwarded[] = $remote;
	for ($i = count($forwarded) - 1; $i >= 0; $i--) {
		$candidate = trim($forwarded[$i]);
		if (!filter_var($candidate, FILTER_VALIDATE_IP)) continue;
		if (!trusted_proxy_match($candidate, $trusted)) return $candidate;
	}
	return $remote;
}

// 系统配置（system_configs 表）运行时读取：每请求一次查询、进程内缓存。
// 读取失败（无管理库连接、表不存在等）一律回落到调用方给的默认值，
// 保证任何部署形态下功能都退化为原始硬编码行为，而不是报错。
function system_config_map() {
    static $map = null;
    if ($map !== null) return $map;
    global $ADMIN_DB;
    $map = array();
    if (!isset($ADMIN_DB) || !$ADMIN_DB || empty($ADMIN_DB->link)) return $map;
    $res = @$ADMIN_DB->query('SELECT config_key, config_value FROM system_configs');
    if ($res) {
        while ($row = $res->fetch_assoc()) $map[$row['config_key']] = $row['config_value'];
        $res->free();
    }
    return $map;
}

function system_config_string($key, $default = '') {
    $map = system_config_map();
    return isset($map[$key]) && $map[$key] !== '' ? $map[$key] : $default;
}

function system_config_int($key, $default) {
    $value = system_config_string($key, null);
    return ($value === null || !is_numeric($value)) ? $default : (int)$value;
}

function system_config_bool($key, $default) {
    $value = system_config_string($key, null);
    if ($value === null) return $default;
    return in_array(strtolower(trim($value)), array('1', 'true', 'on'), true);
}

// 「仅注册使用」开关：管理员在后台 系统配置 中把 shorturl.require_registration
// 设为 true 后，匿名访客不能建链（batch.php 本就要求登录）。
function member_only_create_enabled() {
    return system_config_bool('shorturl.require_registration', false);
}

function shorturl($input){
    $base32 = array('a', 'b', 'c', 'd', 'e', 'f', 'g', 'h', 'i', 'j', 'k', 'l', 'm', 'n', 'o', 'p', 'q', 'r', 's', 't', 'u', 'v', 'w', 'x', 'y', 'z', '0', '1', '2', '3', '4', '5');
    $hex = md5($input);
    $hexLen = strlen($hex);
    $subHexLen = $hexLen / 8;
    $output = array();
    for ($i = 0; $i < $subHexLen; $i++) {
        //把加密字符按照8位一组16进制与0x3FFFFFFF(30位1)进行位与运算
        $subHex = substr($hex, $i * 8, 8);
        $int = 0x3fffffff & hexdec($subHex);
        $out = '';
        for ($j = 0; $j < 6; $j++) {
            //把得到的值与0x0000001F进行位与运算，取得字符数组chars索引
            $val = 0x1f & $int;
            $out .= $base32[$val];
            $int = $int >> 5;
        }
        $output[] = $out;
    }
	return $output[1];
}

// Validate a destination once for both single and batch APIs.
function validate_long_url($url, $skip_dns = false) {
    if (!is_string($url) || $url === '') return array(false, '链接不能为空', 10001);
    if (strlen($url) > 2048) return array(false, '链接过长，请缩短后重试', 10002);
    if (preg_match('/[\x00-\x20\x7f]/', $url)) return array(false, '链接格式不正确，请输入完整的 http(s) 地址', 10002);
    $parts = parse_url($url);
    if (!is_array($parts) || empty($parts['scheme']) || empty($parts['host'])) return array(false, '链接格式不正确，请输入完整的 http(s) 地址', 10002);
    if (!in_array(strtolower($parts['scheme']), array('http', 'https'), true)) return array(false, '链接格式不正确，请输入完整的 http(s) 地址', 10002);
    if (isset($parts['user']) || isset($parts['pass'])) return array(false, '链接格式不正确，请输入完整的 http(s) 地址', 10002);
    if (isset($parts['port']) && ($parts['port'] < 1 || $parts['port'] > 65535)) return array(false, '链接格式不正确，请输入完整的 http(s) 地址', 10002);
    // 区分「域名解析不了」与「解析到内网/保留地址」两种失败。
    // 原来两者共用一句「该地址不允许被缩短（内网/本机/云元数据地址）」，
    // 用户少打一个字母（例如把 .com 打成 .con）就会被暗示在攻击内网，
    // 只会反复重试或直接流失，拿不到任何可自助修复的信息。
    $host_class = classify_host($parts['host'], $skip_dns);
    if ($host_class === 'private') return array(false, '该地址不允许被缩短（内网/本机/云元数据地址）', 10004);
    if ($host_class === 'unresolved') return array(false, '域名无法解析，请检查拼写是否正确', 10005);
    return array(true, '', 1);
}

// classify_host 返回 'private'（命中内网/保留/元数据地址）、
// 'unresolved'（域名解析失败）或 'public'。
// $skip_dns 为 true 时不解析（跳转热路径，服务端不会去访问目标）：
// 此时无法判断解析结果，一律视为 public，除非是显式的元数据地址。
function classify_host($host, $skip_dns = false) {
    // 先按名字/字面量拒掉明显的本机地址（localhost、*.local、字面 IP 段）。
    // 这一步不依赖 DNS，所以即使域名解析失败也能给出「private」。
    $literal = strtolower(trim((string)$host, "[] \t"));
    if ($literal === '' || $literal === 'localhost'
        || substr($literal, -6) === '.local'
        || $literal === '169.254.169.254' || substr($literal, -15) === '.169.254.169.254'
        || $literal === 'metadata.google.internal' || substr($literal, -24) === 'metadata.google.internal') {
        return 'private';
    }
    if (filter_var($literal, FILTER_VALIDATE_IP)) {
        return isPrivateHost($literal, $skip_dns) ? 'private' : 'public';
    }
    if ($skip_dns) return 'public';

    // 域名：先看能不能解析，解析不了是「拼写/网络」问题而不是攻击。
    $ipv4 = @gethostbynamel($literal);
    $v6 = array();
    if (function_exists('dns_get_record') && defined('DNS_AAAA')) {
        $recs = @dns_get_record($literal, DNS_AAAA);
        if (is_array($recs)) foreach ($recs as $r) if (!empty($r['ipv6'])) $v6[] = $r['ipv6'];
    }
    if ((!is_array($ipv4) || !$ipv4) && !$v6) return 'unresolved';

    return isPrivateHost($literal, false) ? 'private' : 'public';
}

// Reject the host if any address is private, reserved, or unresolved.
// When $skip_dns is true the hostname is not resolved (creation already did the
// SSRF check) — used on the redirect hot path where the server never fetches the
// target, so DNS resolution would be pure overhead.
function isPrivateHost($host, $skip_dns = false) {
    $host = strtolower(trim((string)$host, "[] \t"));
    if ($host === '' || $host === 'localhost' || substr($host, -6) === '.local') return true;
    $addresses = array();
    if (filter_var($host, FILTER_VALIDATE_IP)) {
        $addresses[] = $host;
    } elseif (!$skip_dns) {
        $ipv4 = @gethostbynamel($host);
        if (is_array($ipv4)) $addresses = array_merge($addresses, $ipv4);
        if (function_exists('dns_get_record') && defined('DNS_AAAA')) {
            $ipv6 = @dns_get_record($host, DNS_AAAA);
            if (is_array($ipv6)) foreach ($ipv6 as $record) if (!empty($record['ipv6'])) $addresses[] = $record['ipv6'];
        }
    } elseif ($host === '169.254.169.254' || substr($host, -15) === '.169.254.169.254'
        || $host === 'metadata.google.internal' || substr($host, -24) === 'metadata.google.internal') {
        // 纵深防御：即使跳过 DNS 也始终拒绝云元数据地址
        return true;
    }
    $addresses = array_unique($addresses);
    if (!$addresses) return ($skip_dns && !filter_var($host, FILTER_VALIDATE_IP)) ? false : true;
    foreach ($addresses as $ip) {
        if (!filter_var($ip, FILTER_VALIDATE_IP, FILTER_FLAG_NO_PRIV_RANGE | FILTER_FLAG_NO_RES_RANGE)) return true;
    }
    return false;
}

function validate_custom_code($custom) {
    return $custom === '' || preg_match('/^[a-z0-5]{6,8}$/', $custom) === 1;
}

function validate_expire_days($value) {
    if ($value === '' || $value === null) return 0;
    if (filter_var($value, FILTER_VALIDATE_INT) === false) return false;
    $days = (int)$value;
    return in_array($days, array(0, 1, 7, 30, 365), true) ? $days : false;
}

function public_short_url($uid, $domain_id = null) {
    global $public_base_url, $DB;

    // A3：仅登录会员才允许按 domain_id 选择域名池中的域名。
    // 匿名调用一律忽略外部传入的 domain_id，只用服务端 $public_base_url，
    // 避免匿名用户枚举 domains 表探测未公开的备用短链域名。
    // domain_id 还必须是纯数字，防止非预期类型进入查询。
    $is_numeric_domain = $domain_id !== null && $domain_id !== '' && ctype_digit((string)$domain_id);
    if ($is_numeric_domain && function_exists('member_id') && member_id() > 0 && isset($DB) && !empty($DB->link)) {
        $stmt = $DB->prepare('SELECT domain, scheme FROM domains WHERE id=? AND status=1 AND deleted_at IS NULL LIMIT 1');
        if ($stmt) {
            mysqli_stmt_bind_param($stmt, 's', $domain_id);
            mysqli_stmt_execute($stmt);
            mysqli_stmt_bind_result($stmt, $d_domain, $d_scheme);
            if (mysqli_stmt_fetch($stmt)) {
                mysqli_stmt_close($stmt);
                $scheme = !empty($d_scheme) ? $d_scheme : 'https';
                // 域名必须形如合法主机名，避免库中脏数据拼出非预期 URL
                if (preg_match('/^[a-z0-9.-]+$/i', (string)$d_domain) && strpos((string)$d_domain, '.') !== false) {
                    return $scheme . '://' . $d_domain . '/' . rawurlencode($uid);
                }
            } else {
                mysqli_stmt_close($stmt);
            }
        }
    }

    // Fallback to global base URL
    $base = isset($public_base_url) ? rtrim(trim((string)$public_base_url), '/') : '';
    if ($base === '' || !filter_var($base, FILTER_VALIDATE_URL)) return '';
    $parts = parse_url($base);
    if (!is_array($parts) || empty($parts['scheme']) || empty($parts['host']) || !in_array(strtolower($parts['scheme']), array('http', 'https'), true)) return '';
    return $base . '/' . rawurlencode($uid);
}

/**
 * 文件型限流。$key 是「用途命名空间 + 主体」的不透明串，例如 'api:1.2.3.4'、
 * 'batch:1.2.3.4'、'pwtry:1.2.3.4:ab12cd'、'user:motao'。
 * 不同用途必须带不同前缀：桶文件名只取 md5($key)，所以 #17 里匿名建链、会员注册、
 * 会员登录三处都传裸 real_ip()，共用同一个桶，三条各不相同的限额互相挤占。
 *
 * 返回三态，调用方要显式决定故障时怎么取舍（#18）：
 *   true  放行
 *   false 超出限额
 *   null  限流器自身不可用（目录不可写、拿不到锁等）
 * 历史实现把故障也返回 false，于是 logs/ 没有写权限的新部署上，每一个访客都会
 * 看到「请求过于频繁，请稍后再试」——一个冒充用户行为的系统故障，且无从绕行。
 */
function rate_limit($key, $max = 20, $window = 60, $cost = 1) {
    global $rate_limit_dir;
    $dir = isset($rate_limit_dir) && is_string($rate_limit_dir) && $rate_limit_dir !== '' ? $rate_limit_dir : ROOT . 'logs/ratelimit';
    if (!is_dir($dir) && !@mkdir($dir, 0755, true) && !is_dir($dir)) return null;
    $fp = @fopen(rtrim($dir, '/\\') . '/' . md5((string)$key) . '.rl', 'c+');
    if (!$fp) return null;
    if (!flock($fp, LOCK_EX)) { fclose($fp); return null; }
    $now = time();
    // 机会式清理：每约 1/200 次调用扫一次目录，删除 24h 未修改的过期桶文件，
    // 避免每个 IP 一个文件在 IPv6/代理场景下无限膨胀耗尽 inode。
    if (function_exists('rate_limit_gc')) rate_limit_gc($dir, $now);
    $data = json_decode(stream_get_contents($fp), true);
    if (!is_array($data) || !isset($data['start'], $data['count']) || ($now - (int)$data['start']) >= $window) $data = array('start' => $now, 'count' => 0);
    $cost = max(1, (int)$cost);
    $allowed = ((int)$data['count'] + $cost) <= $max;
    if ($allowed) $data['count'] += $cost;
    ftruncate($fp, 0); rewind($fp); fwrite($fp, json_encode($data)); fflush($fp);
    flock($fp, LOCK_UN); fclose($fp);
    return $allowed;
}

/**
 * 面向用户的主旅程入口用的包装：限流器自身故障时放行，并留一行可定位的日志（#18）。
 * 限流是防滥用手段、不是鉴权因子，所以它坏掉时不该把所有人挡在站外——那正是
 * 「logs/ 目录没写权限 → 全站 429」的成因。
 *
 * 账号锁定与密码尝试**不要**用它：那两类是安全控制，故障时应拒绝而非放行，
 * 调用方直接写 `if (rate_limit(...) !== true)` 把 fail-closed 说清楚。
 *
 * @return bool  true 可以继续，false 已被限额挡住。
 */
function rate_limit_allows($key, $max = 20, $window = 60, $cost = 1) {
    $allowed = rate_limit($key, $max, $window, $cost);
    if ($allowed === null) {
        error_log('[dwz] 限流器不可用：桶 ' . $key . ' 无法读写，本次按放行处理。'
            . '请检查 $rate_limit_dir（默认 ROOT/logs/ratelimit）的写权限。');
        return true;
    }
    return $allowed;
}

// Best-effort GC for the file-based rate limiter. Kept cheap: only runs on a
// small random fraction of calls and deletes buckets untouched for $ttl seconds.
function rate_limit_gc($dir, $now = null, $ttl = 86400) {
    if (random_int(1, 200) !== 1) return;
    $now = $now === null ? time() : (int)$now;
    $handle = @opendir($dir);
    if (!$handle) return;
    while (($entry = readdir($handle)) !== false) {
        if ($entry === '.' || $entry === '..') continue;
        if (substr($entry, -3) !== '.rl') continue;
        $path = rtrim($dir, '/\\') . '/' . $entry;
        $mtime = @filemtime($path);
        if ($mtime !== false && ($now - $mtime) > $ttl) @unlink($path);
    }
    closedir($handle);
}

// Reset a rate-limit key (e.g. after a successful login clears the failure count).
function rate_limit_reset($key) {
    global $rate_limit_dir;
    $dir = isset($rate_limit_dir) && is_string($rate_limit_dir) && $rate_limit_dir !== '' ? $rate_limit_dir : ROOT . 'logs/ratelimit';
    $file = rtrim($dir, '/\\') . '/' . md5((string)$key) . '.rl';
    if (is_file($file)) @unlink($file);
}

function short_url_result($uid, $msg, $state = 'existing', $domain_id = null) {
    return array(
        'code' => $uid,
        'short_url' => public_short_url($uid, $domain_id),
        'msg' => $msg,
        'result' => 1,
        'state' => $state,
        'created' => $state === 'created'
    );
}

// C 类改造：url_hash 的唯一索引由「URL 全局唯一」改为「同一 owner 作用域内唯一」。
// 哈希输入 = longurl + 0x1F + scope_key，与数据库端
//   MD5(CONCAT(longurl, 0x1F, scope_key))
// 完全一致，scope_key 取值：
//   'w:0'           -> 匿名 / 历史数据，保留改造前的全局去重语义
//   'm:<member_id>' -> 会员短链，同一会员内去重，不同会员可各建一条
// 这样既避免 MD5 碰撞导致「完全不同 URL 无法创建」，也让同一 URL 可按会员/有效期分别建链。
//
// ⚠️ 与 Go 侧 backend/internal/service/short_url.go 的 urlScopeKey() 必须保持
//    完全一致：Go 的后台管理台创建（createdBy 非空）同样落在 'w:0'，而不是
//    'w:<user_id>'。PHP 前台只持有 member_id，永远算不出 'w:<n>'；若 Go 用
//    管理员维度隔离，同一条 URL 经后台创建后，PHP 前台就再也匹配不到同一
//    url_hash，两条跳转路径会各自建链、互相不可见。
function url_scope_key($member_id = null) {
    $mid = $member_id === null ? 0 : (int)$member_id;
    return $mid > 0 ? 'm:' . $mid : 'w:' . 0;
}

// 计算作用域 MD5。与数据库端表达式
//   MD5(CONCAT(longurl, 0x1F, scope_key)) / MD5(CONCAT(long_url, 0x1F, scopeKey))
// 完全一致（utf8mb4 下 MD5(CONCAT(...)) 结果不受字符集影响）。
function url_scope_hash($longurl, $scope_key) {
    return md5((string)$longurl . "\x1f" . (string)$scope_key);
}

// 按作用域哈希查短码（返回 '' 表示不存在）。用于「同一会员/匿名池内同一 URL」
// 的幂等复用：先查再复用，避免重复 INSERT 触发唯一索引冲突。
function find_uid_by_scope($DB, $hash) {
    if (!isset($DB) || empty($DB->link) || !is_string($hash) || $hash === '') return '';
    $stmt = $DB->prepare('SELECT uid FROM wjoy_log WHERE url_hash=? AND status=1 LIMIT 1');
    if (!$stmt) return '';
    mysqli_stmt_bind_param($stmt, 's', $hash);
    if (!mysqli_stmt_execute($stmt)) { mysqli_stmt_close($stmt); return ''; }
    mysqli_stmt_bind_result($stmt, $uid);
    $uid = mysqli_stmt_fetch($stmt) ? (string)$uid : '';
    mysqli_stmt_close($stmt);
    return $uid;
}

function find_short_by_hash($DB, $hash) {
    $stmt = $DB->prepare('SELECT uid, expire_at FROM wjoy_log WHERE url_hash=? LIMIT 1');
    if (!$stmt) return false;
    mysqli_stmt_bind_param($stmt, 's', $hash);
    if (!mysqli_stmt_execute($stmt)) { mysqli_stmt_close($stmt); return false; }
    mysqli_stmt_bind_result($stmt, $uid, $expire_at);
    $found = mysqli_stmt_fetch($stmt);
    mysqli_stmt_close($stmt);
    return $found ? array('uid' => $uid, 'expire_at' => $expire_at) : null;
}

function renew_short_expiry($DB, $hash, $expire_at) {
    $stmt = $DB->prepare('UPDATE wjoy_log SET expire_at=? WHERE url_hash=?');
    if (!$stmt) return false;
    mysqli_stmt_bind_param($stmt, 'ss', $expire_at, $hash);
    $ok = mysqli_stmt_execute($stmt);
    mysqli_stmt_close($stmt);
    return $ok;
}

// Create or renew a short URL. Generated-code collisions are retried with the existing alphabet.
function create_short_url($DB, $longurl, $custom = null, $expire_days = 0, $domain_id = null, $password = '', $member_id = null, $known_hash = null) {
    $custom = $custom === null ? '' : trim((string)$custom);
    if (!validate_custom_code($custom)) return array('code' => 0, 'short_url' => '', 'msg' => '自定义短码格式错误（需 6-8 位，仅含 a-z 与 0-5）', 'result' => 10006);
    $expire_days = validate_expire_days($expire_days);
    if ($expire_days === false) return array('code' => 0, 'short_url' => '', 'msg' => '有效期仅支持 0、1、7、30 或 365 天', 'result' => 10008);
    $password = is_string($password) ? trim($password) : '';
    if (strlen($password) > 72) return array('code' => 0, 'short_url' => '', 'msg' => '访问密码过长（最多 72 字节）', 'result' => 10008);
    $password_hash = $password !== '' ? password_hash($password, PASSWORD_DEFAULT) : null;

    // $known_hash：由调用方（api.php / batch.php）预计算并传入的作用域哈希，
    // 避免同一请求内对同一 URL 重复计算，也保证「查重」与「写入」用的是同一个值。
    $hash = $known_hash !== null ? (string)$known_hash : url_scope_hash($longurl, url_scope_key($member_id));
    $expire_at = $expire_days > 0 ? date('Y-m-d H:i:s', time() + $expire_days * 86400) : null;
    $existing = find_short_by_hash($DB, $hash);
    if (is_array($existing) && !empty($existing['uid'])) {
        if ($custom !== '' && $custom !== $existing['uid']) {
            return array('code' => 0, 'short_url' => '', 'msg' => '该网址已有短链，不能改用其他自定义短码', 'result' => 10013);
        }
        $was_expired = !empty($existing['expire_at']) && strtotime($existing['expire_at']) !== false && strtotime($existing['expire_at']) <= time();
        if ($was_expired) {
            if (!renew_short_expiry($DB, $hash, $expire_at)) return array('code' => 0, 'short_url' => '', 'msg' => 'failure', 'result' => 10003);
            return short_url_result($existing['uid'], 'renewed', 'renewed', $domain_id);
        }
        return short_url_result($existing['uid'], 'existence', 'existing', $domain_id);
    }

    $attempts = $custom !== '' ? 1 : 12;
    for ($attempt = 0; $attempt < $attempts; $attempt++) {
        // 始终加随机盐：短码由 md5(长链 + 盐) 推导，盐为空时同一个长链永远
        // 得到同一个短码。此前只有第 2 次重试起才加盐，于是「首个短码」是可
        // 预测的——知道算法与长链即可推算出别人的短码，也方便批量探测已存在
        // 的链接。现在每次生成都是独立随机短码。
        // 幂等去重不受影响：调用方在本函数之前已按 url_hash 命中「已存在」分支，
        // 不会为同一长链反复建新码。
        try {
            $salt = '|' . $attempt . '|' . bin2hex(random_bytes(8));
        } catch (Throwable $e) {
            $salt = '|' . $attempt . '|' . uniqid('', true) . '|' . mt_rand();
        }
        $uid = $custom !== '' ? $custom : shorturl($longurl . $salt);
        $stmt = $DB->prepare('INSERT INTO wjoy_log (uid,longurl,url_hash,expire_at,password_hash) VALUES (?,?,?,?,?)');
        if (!$stmt) return array('code' => 0, 'short_url' => '', 'msg' => 'failure', 'result' => 10003);
        mysqli_stmt_bind_param($stmt, 'sssss', $uid, $longurl, $hash, $expire_at, $password_hash);
        $ok = mysqli_stmt_execute($stmt);
        $errno = mysqli_stmt_errno($stmt);
        mysqli_stmt_close($stmt);
        if ($ok) return short_url_result($uid, 'success', 'created', $domain_id);
        if ($errno !== 1062) return array('code' => 0, 'short_url' => '', 'msg' => 'failure', 'result' => 10003);
        $existing = find_short_by_hash($DB, $hash);
        if (is_array($existing) && !empty($existing['uid'])) {
            if ($custom !== '' && $custom !== $existing['uid']) {
                return array('code' => 0, 'short_url' => '', 'msg' => '该网址已有短链，不能改用其他自定义短码', 'result' => 10013);
            }
            $was_expired = !empty($existing['expire_at']) && strtotime($existing['expire_at']) !== false && strtotime($existing['expire_at']) <= time();
            if ($was_expired) {
                if (!renew_short_expiry($DB, $hash, $expire_at)) return array('code' => 0, 'short_url' => '', 'msg' => 'failure', 'result' => 10003);
                return short_url_result($existing['uid'], 'renewed', 'renewed', $domain_id);
            }
            return short_url_result($existing['uid'], 'existence', 'existing', $domain_id);
        }
        if ($custom !== '') return array('code' => 0, 'short_url' => '', 'msg' => '自定义短码已被占用', 'result' => 10007);
    }
    return array('code' => 0, 'short_url' => '', 'msg' => '短码生成冲突，请重试', 'result' => 10009);
}

// ---------------------------------------------------------------------------
// 违规检测（同步阻断）—— 移植自 backend/internal/pkg/violation.go
// 仅做 URL 本身的规则检查，不发起网络请求，安全且快速。
// 返回 array('blocked'=>bool, 'reason'=>string)
// ---------------------------------------------------------------------------

// 违规规则的唯一来源：backend/internal/pkg/data/violation_rules.json。
// Go 侧（internal/pkg/violation.go）通过 go:embed 读同一文件，PHP 侧在此加载，
// 两边共用一份数据，从结构上杜绝「词库双实现漂移」。
//
// 查找顺序（#11）：config.php 的 $violation_rules_file → /etc/dwz/violation_rules.json
// （Docker 镜像落点）→ 仓库内原件（git clone / 开发机）。
// 此前只认最后这一个路径，而 Docker 镜像与 deploy.sh 都不带 backend/，于是公开
// 入口（api.php / batch.php）的合规拦截在完全没有可见信号的情况下退化成 3 域 / 5 词。
// 词库刻意放在 web 根之外：黑名单一旦可被公开下载，等于把绕过方法交给提交方。
$blocked_domain_suffixes = array();
$blocked_keywords = array();

(function () {
    global $blocked_domain_suffixes, $blocked_keywords, $violation_rules_file;
    $fallback_domains = array("bet365.com", "phishing.com", "malware.com");
    $fallback_keywords = array("casino", "gambling", "博彩", "赌场", "刷单返利");
    $candidates = array();
    if (is_string($violation_rules_file) && $violation_rules_file !== '') {
        $candidates[] = $violation_rules_file;
    }
    $candidates[] = '/etc/dwz/violation_rules.json';
    $candidates[] = __DIR__ . "/../backend/internal/pkg/data/violation_rules.json";
    $data = null;
    foreach ($candidates as $path) {
        $raw = @file_get_contents($path);
        if ($raw === false) continue;
        $decoded = json_decode($raw, true);
        if (is_array($decoded)) { $data = $decoded; break; }
    }
    if (!is_array($data)) {
        // 按进程去重：降级期间每个请求都写一行会把日志刷满，反而盖掉真正的线索。
        static $warned = false;
        if (!$warned) {
            $warned = true;
            error_log('[dwz] 违规词库不可用（已查找：' . implode(', ', $candidates) . '），'
                . '退回内置最小黑名单，公开入口的合规拦截将明显弱于后台/Go 侧。'
                . '修复：部署 backend/internal/pkg/data/violation_rules.json 到上述任一路径，'
                . '或在 config.php 设置 $violation_rules_file。');
        }
        $blocked_domain_suffixes = $fallback_domains;
        $blocked_keywords = $fallback_keywords;
        return;
    }
    $blocked_domain_suffixes = isset($data["domain_suffixes"]) && is_array($data["domain_suffixes"])
        ? $data["domain_suffixes"] : $fallback_domains;
    $blocked_keywords = isset($data["keywords"]) && is_array($data["keywords"])
        ? $data["keywords"] : $fallback_keywords;
})();
function check_url_violation($url) {
    global $blocked_domain_suffixes, $blocked_keywords;

    $parts = parse_url((string)$url);
    $host = isset($parts['host']) ? strtolower(trim((string)$parts['host'], "[] \t")) : '';
    $lower = strtolower((string)$url);

    // 云元数据端点始终拦截（SSRF 纵深防御）
    if ($host === '169.254.169.254' || substr($host, -15) === '.169.254.169.254'
        || $host === 'metadata.google.internal' || substr($host, -24) === 'metadata.google.internal') {
        return array('blocked' => true, 'reason' => '云元数据地址不允许访问');
    }

    // 域名精确 / 后缀黑名单
    if ($host !== '') {
        foreach ($blocked_domain_suffixes as $d) {
            if ($host === $d || substr($host, -(strlen($d) + 1)) === '.' . $d) {
                return array('blocked' => true, 'reason' => 'domain is blocked');
            }
        }
    }

    // 全 URL 关键词黑名单
    foreach ($blocked_keywords as $kw) {
        if (strpos($lower, $kw) !== false) {
            return array('blocked' => true, 'reason' => 'url matches a blocked keyword');
        }
    }

    return array('blocked' => false, 'reason' => '');
}

// Record a blocked URL for later manual review. Best-effort: never aborts the
// request if the insert fails.
function log_violation($DB, $url, $reason, $source = 'api') {
    if (!$DB || empty($DB->link)) return;
    $ip = (string)real_ip();
    if (strlen($ip) > 45) $ip = substr($ip, 0, 45);
    if (strlen((string)$reason) > 64) $reason = substr((string)$reason, 0, 64);
    if (!in_array($source, array('api', 'batch'), true)) $source = 'api';
    $stmt = $DB->prepare('INSERT INTO violation_reviews (url, reason, ip, source) VALUES (?,?,?,?)');
    if (!$stmt) return;
    mysqli_stmt_bind_param($stmt, 'ssss', $url, $reason, $ip, $source);
    mysqli_stmt_execute($stmt);
    mysqli_stmt_close($stmt);
}

// Dual-write a new short link into the admin short_urls table so short_urls
// becomes the canonical data source. Best-effort: uses the optional $ADMIN_DB
// connection and never aborts the request on failure.
function sync_short_url_to_admin($uid, $longurl, $expire_at = null, $member_id = null, $password_hash = null) {
    global $ADMIN_DB;
    if (!$ADMIN_DB || empty($ADMIN_DB->link)) return;
    $hash = url_scope_hash($longurl, url_scope_key($member_id));
    $source = 'web';
    $member_id = $member_id > 0 ? (int)$member_id : null;
    $password_hash = $password_hash === null || $password_hash === '' ? null : (string)$password_hash;
    // A4：冲突时同步鉴权/可见性字段，避免 wjoy_log 与 short_urls 对同一条短链
    // 给出不同的鉴权结果（Go 路径 vs PHP 路径）。
    //   - long_url / password_hash：直接同步为本次提交的值。
    //   - expire_at：IFNULL 保留已有有效期，不覆盖永久/更长有效期。
    //   - status：仅当已过期（2）时复活为 1；管理员主动禁用（0）不被覆盖，
    //     避免 API 重复提交把人工下线的短链重新启用。
    // A5：password_hash 为 null 表示「本次请求未提供密码」，绝不能理解成
    // 「把密码清空」。原实现无条件 password_hash=VALUES(password_hash)，于是
    // 匿名重复提交同一长链会把管理库副本的密码抹成 NULL，Go 跳转路径随后免密。
    // 现在 null 走 IFNULL 保留旧值；只有真的提供了新密码才覆盖。
    $stmt = $ADMIN_DB->prepare('INSERT INTO short_urls (uid, long_url, url_hash, expire_at, member_id, source, status, password_hash) VALUES (?,?,?,?,?,?,1,?) ON DUPLICATE KEY UPDATE long_url=VALUES(long_url), password_hash=IFNULL(VALUES(password_hash), short_urls.password_hash), status=IF(short_urls.status=2, VALUES(status), short_urls.status), expire_at=IFNULL(short_urls.expire_at, VALUES(expire_at))');
    if (!$stmt) return;
    mysqli_stmt_bind_param($stmt, 'ssssiss', $uid, $longurl, $hash, $expire_at, $member_id, $source, $password_hash);
    mysqli_stmt_execute($stmt);
    mysqli_stmt_close($stmt);
}

// Issue an HS256 JWT for a public member (signed with $member_secret).
// $token_version is the member's current token_version; JWTs are invalidated
// when it changes (logout), enforced by the Go member_auth middleware.
function member_issue_token($member_id, $username, $token_version = 0) {
    global $member_secret;
    if (empty($member_secret)) return '';
    $header = base64url_encode(json_encode(array('alg' => 'HS256', 'typ' => 'JWT')));
    $payload = base64url_encode(json_encode(array(
        'member_id' => (int)$member_id,
        'username'  => (string)$username,
        'token_version' => (int)$token_version,
        'sub'       => 'member',
        'iat'       => time(),
        'exp'       => time() + 86400,
    )));
    $sig = base64url_encode(hash_hmac('sha256', $header . '.' . $payload, $member_secret, true));
    return $header . '.' . $payload . '.' . $sig;
}

function base64url_encode($data) {
    return rtrim(strtr(base64_encode($data), '+/', '-_'), '=');
}

// Record a click on the admin-side analytics (click_logs + short_urls counter)
// for the primary PHP redirect path. Best-effort: never blocks the redirect.
// Mirrors what the Go /r/:code path persists so admin stats stay consistent.
function record_click_analytics($uid) {
    global $ADMIN_DB;
    if (!$ADMIN_DB || empty($ADMIN_DB->link)) {
        // Analytics has no admin connection, so this click cannot be recorded
        // anywhere. Silently returning used to make a pure-PHP deployment look
        // "successful" while every stat stayed at zero, and the operator had no
        // signal at all. Log once per process (guarded so a busy redirect path
        // cannot flood php_error.log) and, in that state, still fall back to the
        // legacy wjoy_log counter so the frontend does not lose the click.
        record_click_analytics_degraded($uid);
        return;
    }
    $ip = isset($_SERVER['REMOTE_ADDR']) ? $_SERVER['REMOTE_ADDR'] : '';
    $ua = isset($_SERVER['HTTP_USER_AGENT']) ? substr($_SERVER['HTTP_USER_AGENT'], 0, 512) : '';
    $ref = isset($_SERVER['HTTP_REFERER']) ? substr($_SERVER['HTTP_REFERER'], 0, 512) : '';
    // 2 queries instead of 3: INSERT...SELECT resolves the short_url_id and only
    // inserts when the row exists (same semantics as the old SELECT guard).
    $stmt = $ADMIN_DB->prepare('INSERT INTO click_logs (short_url_id, ip, user_agent, referer, created_at) '
        . 'SELECT id, ?, ?, ?, NOW(3) FROM short_urls WHERE uid=? AND deleted_at IS NULL LIMIT 1');
    if ($stmt) {
        mysqli_stmt_bind_param($stmt, 'ssss', $ip, $ua, $ref, $uid);
        mysqli_stmt_execute($stmt);
        mysqli_stmt_close($stmt);
    }
    $stmt = $ADMIN_DB->prepare('UPDATE short_urls SET clicks = clicks + 1, updated_at = NOW(3) WHERE uid=? AND deleted_at IS NULL');
    if ($stmt) {
        mysqli_stmt_bind_param($stmt, 's', $uid);
        mysqli_stmt_execute($stmt);
        mysqli_stmt_close($stmt);
    }
}

// Degraded analytics path: the admin DB is not configured, so click_logs and
// short_urls.clicks are unreachable. Fall back to the legacy wjoy_log counter
// (the pre-dual-write behaviour) so a pure-PHP install still shows non-zero
// clicks, and emit a single actionable warning per request so the misconfig is
// visible instead of producing a silently empty dashboard.
function record_click_analytics_degraded($uid) {
    static $warned = false;
    if (!$warned) {
        $warned = true;
        error_log('[dwz] 点击统计已降级：未配置 ADMIN_DB（$admin_db_user / $admin_db_name 为空），'
            . 'click_logs 与 short_urls.clicks 无法写入，后台统计将长期为空。'
            . '请在 config.php 中配置后台库连接，或改用 Go 跳转网关 /r/:code。');
    }

    global $DB;
    if (!$DB || empty($DB->link)) return;
    $stmt = $DB->prepare('UPDATE wjoy_log SET clicks = clicks + 1 WHERE uid = ? LIMIT 1');
    if (!$stmt) return;
    mysqli_stmt_bind_param($stmt, 's', $uid);
    mysqli_stmt_execute($stmt);
    mysqli_stmt_close($stmt);
}

// ---------------------------------------------------------------------------
// Webhook 异步队列（P2）
// ---------------------------------------------------------------------------
// 背景：do.php 跳转热路径原先同步投递所有 webhook（最坏 3 次重试 ≈ 每秒级阻塞）。
// 现在派发方只做一次 O(1) 的 INSERT 入队，实际投递由
// migrations/webhook_worker.php 在后台完成（cron 或常驻循环），
// 用户跳转不再依赖任何外发 HTTP 的成功与否。

// 把事件写入 webhook_queue（按订阅者展开，入队时即完成订阅匹配与 SSRF 预检）。
// 热路径唯一开销：1 次 SELECT + N 次 INSERT，无网络 IO。
function dispatch_webhook_event($event, $payload) {
    global $ADMIN_DB, $webhook_dispatch_inline;
    if (!$ADMIN_DB || empty($ADMIN_DB->link)) return;
    $stmt = $ADMIN_DB->prepare('SELECT id, url, events FROM webhooks WHERE status=1 AND deleted_at IS NULL');
    if (!$stmt) return;
    mysqli_stmt_execute($stmt);
    $res = mysqli_stmt_get_result($stmt);
    if (!$res) { mysqli_stmt_close($stmt); return; }
    $body = json_encode(array(
        'id' => 'wh_' . time() . '_' . bin2hex(random_bytes(4)),
        'event' => $event,
        'timestamp' => time(),
        'data' => $payload,
    ), JSON_UNESCAPED_UNICODE | JSON_UNESCAPED_SLASHES);
    $rows = array();
    while ($row = mysqli_fetch_assoc($res)) {
        $events = json_decode($row['events'], true);
        if (!is_array($events) || !in_array($event, $events, true)) continue;
        // SSRF 预检（热路径低成本版）：只做协议/语法与字面 IP 校验，不做 DNS。
        // 完整 DNS/重绑定校验由 worker 投递前再用 validate_long_url(..., false)
        // 执行，避免在跳转热路径引入 DNS 解析开销。
        $target_check = validate_long_url((string)$row['url'], true);
        if (!$target_check[0]) {
            error_log('[dwz] webhook skipped (unsafe target): ' . (string)$row['url']);
            continue;
        }
        $rows[] = $row;
    }
    mysqli_stmt_close($stmt);

    $ins = $ADMIN_DB->prepare('INSERT INTO webhook_queue (webhook_id, event, payload, max_attempts, next_retry_at, status, created_at) VALUES (?,?,?,3,NOW(3),0,NOW(3))');
    if (!$ins) return;
    foreach ($rows as $row) {
        mysqli_stmt_bind_param($ins, 'iss', $row['id'], $event, $body);
        mysqli_stmt_execute($ins);
    }
    mysqli_stmt_close($ins);

    // 兼容/兜底：队列表尚未迁移（webhook_queue 不存在）时，退化为同步投递，
    // 保证功能不中断。可用 $webhook_dispatch_inline = false 显式关闭该兜底。
    if (webhook_queue_missing()) {
        if (!isset($webhook_dispatch_inline) || $webhook_dispatch_inline !== false) {
            foreach ($rows as $row) {
                webhook_deliver_now($row, $event, $body, 1);
            }
        }
    }
}

// webhook_queue 表是否存在（结果进程内缓存，避免热路径反复查 information_schema）。
function webhook_queue_missing() {
    static $missing = null;
    global $ADMIN_DB;
    if ($missing !== null) return $missing;
    if (!$ADMIN_DB || empty($ADMIN_DB->link)) return $missing = true;
    $res = @mysqli_query($ADMIN_DB->link, "SHOW TABLES LIKE 'webhook_queue'");
    $missing = !($res && mysqli_num_rows($res) > 0);
    if ($res) mysqli_free_result($res);
    if ($missing) error_log('[dwz] webhook_queue 表缺失，已退化为同步投递；请执行 cd backend && go run ./cmd/migrate（迁移 backend/migrations/php/add_webhook_queue.sql）');
    return $missing;
}

// 单次投递 + 落库 webhook_deliveries。返回 true 表示 2xx 成功。
// 供 worker 与同步兜底共用；超时为毫秒级，即使同步兜底也不会长阻塞。
function webhook_deliver_now($webhook, $event, $body, $attempt = 1) {
    global $ADMIN_DB;
    // curl 扩展缺失时不应致命中断（历史上直接 curl_init() 会 Fatal Error）。
    if (!function_exists('curl_init')) {
        return array(false, 'curl extension not available');
    }
    $headers = array('Content-Type: application/json', 'User-Agent: dwz-shorturl-webhook/1.0');
    if (!empty($webhook['secret'])) {
        $headers[] = 'X-Webhook-Signature: sha256=' . hash_hmac('sha256', $body, (string)$webhook['secret']);
    }
    $status = 0;
    $resp_body = '';
    $ch = curl_init((string)$webhook['url']);
    curl_setopt_array($ch, array(
        CURLOPT_POST => true,
        CURLOPT_POSTFIELDS => $body,
        CURLOPT_HTTPHEADER => $headers,
        CURLOPT_RETURNTRANSFER => true,
        CURLOPT_FOLLOWLOCATION => false,
        CURLOPT_TIMEOUT_MS => 800,
        CURLOPT_CONNECTTIMEOUT_MS => 400,
    ));
    $resp = curl_exec($ch);
    $http_code = (int)curl_getinfo($ch, CURLINFO_HTTP_CODE);
    curl_close($ch);
    $success = 0;
    $err = null;
    if ($resp !== false) {
        $status = $http_code;
        $resp_body = substr((string)$resp, 0, 512);
        if ($http_code >= 200 && $http_code < 300) $success = 1;
        else $err = 'HTTP ' . $http_code;
    } else {
        $err = 'curl error';
    }
    if ($ADMIN_DB && !empty($ADMIN_DB->link)) {
        $ins = $ADMIN_DB->prepare('INSERT INTO webhook_deliveries (webhook_id, event, payload, response_status, response_body, attempt, success, created_at) VALUES (?,?,?,?,?,?,?,NOW())');
        if ($ins) {
            mysqli_stmt_bind_param($ins, 'issisii', $webhook['id'], $event, $body, $status, $resp_body, $attempt, $success);
            mysqli_stmt_execute($ins);
            mysqli_stmt_close($ins);
        }
    }
    return array($success === 1, $err);
}

// 供 worker 使用：取一批到期的待投递任务（带行锁，避免多 worker 重复消费）。
function webhook_queue_fetch($limit = 20) {
    global $ADMIN_DB;
    if (!$ADMIN_DB || empty($ADMIN_DB->link)) return array();
    $limit = max(1, (int)$limit);
    $ADMIN_DB->query('BEGIN');
    $res = mysqli_query($ADMIN_DB->link, 'SELECT id, webhook_id, event, payload, attempts, max_attempts FROM webhook_queue '
        . 'WHERE status=0 AND next_retry_at <= NOW(3) ORDER BY id ASC LIMIT ' . $limit . ' FOR UPDATE SKIP LOCKED');
    if (!$res) { $ADMIN_DB->query('ROLLBACK'); return array(); }
    $rows = array();
    while ($r = mysqli_fetch_assoc($res)) $rows[] = $r;
    mysqli_free_result($res);
    foreach ($rows as $r) {
        mysqli_query($ADMIN_DB->link, 'UPDATE webhook_queue SET locked_at=NOW(3) WHERE id=' . (int)$r['id']);
    }
    $ADMIN_DB->query('COMMIT');
    return $rows;
}

// ---- 链接访问密码（与 Go 后端共用同一 HMAC cookie 算法） ----
// cookie 名: dwz_plink_<uid>，值: <expiry>.<hex(hmac_sha256(member_secret, uid.expiry))>
// PHP 与 Go 使用同一 $member_secret，因此两条跳转路径互相认可解锁状态。

function password_unlock_token($uid, $expiry) {
    global $member_secret;
    return (string)$expiry . '.' . hash_hmac('sha256', (string)$uid . '.' . (string)$expiry, (string)$member_secret);
}

function password_unlock_ok($uid) {
    global $member_secret;
    if ($member_secret === '') return false;
    $name = 'dwz_plink_' . (string)$uid;
    if (!isset($_COOKIE[$name])) return false;
    $raw = (string)$_COOKIE[$name];
    $parts = explode('.', $raw, 2);
    if (count($parts) !== 2 || !is_numeric($parts[0])) return false;
    $expiry = (int)$parts[0];
    if (time() > $expiry) return false;
    return hash_equals(password_unlock_token($uid, $expiry), $raw);
}

function set_password_unlock_cookie($uid) {
    $expiry = time() + 30 * 86400;
    $secure = !empty($_SERVER['HTTPS']) && $_SERVER['HTTPS'] !== 'off';
    setcookie('dwz_plink_' . (string)$uid, password_unlock_token($uid, $expiry), $expiry, '/', '', $secure, true);
}

function password_page_html($uid, $err = '') {
    $title = $err !== '' ? $err : '请输入访问密码';
    $msg = $err !== '' ? '<p style="color:#c0392b;font-size:13px;margin:0 0 14px;">' . htmlspecialchars($err, ENT_QUOTES) . '</p>' : '';
    $uid = htmlspecialchars((string)$uid, ENT_QUOTES);
    return '<!doctype html><html lang="zh-CN"><head><meta charset="utf-8">'
        . '<meta name="viewport" content="width=device-width,initial-scale=1">'
        . '<title>' . htmlspecialchars($title, ENT_QUOTES) . '</title>'
        . '<style>'
        // 使用 CSS 变量定义主题，并跟随系统 prefers-color-scheme，
        // 避免暗色环境下密码页仍是刺眼的白底（与全站暗色适配保持一致）。
        . ':root{--pw-page:#f2f5f7;--pw-card:#fff;--pw-line:#e4ecee;--pw-text:#16292b;--pw-dim:#6b7f86;--pw-input:#d3e0e3;--pw-brand:#0e6e75;--pw-brand-hover:#0a5a60}'
        . '@media (prefers-color-scheme:dark){:root{--pw-page:#0d1b20;--pw-card:#122027;--pw-line:#23343b;--pw-text:#e6edf0;--pw-dim:#9aa9ae;--pw-input:#31454d;--pw-brand:#12909a;--pw-brand-hover:#0e6e75}}'
        . '*{box-sizing:border-box}body{margin:0;min-height:100vh;display:grid;place-items:center;background:var(--pw-page);font-family:-apple-system,"PingFang SC","Microsoft YaHei",sans-serif;color:var(--pw-text)}'
        . '.card{width:min(92vw,360px);background:var(--pw-card);border:1px solid var(--pw-line);border-radius:14px;padding:28px 24px;box-shadow:0 8px 30px rgba(14,110,117,.08)}'
        . '.lock{font-size:34px;text-align:center;margin:0 0 6px}h1{font-size:17px;text-align:center;margin:0 0 6px;font-weight:700}'
        . '.sub{font-size:12.5px;text-align:center;color:var(--pw-dim);margin:0 0 18px}'
        . 'input{width:100%;padding:11px 12px;border:1px solid var(--pw-input);border-radius:8px;font-size:15px;outline:none;background:var(--pw-card);color:var(--pw-text)}'
        . 'input:focus{border-color:var(--pw-brand);box-shadow:0 0 0 3px rgba(14,110,117,.12)}'
        . 'button{width:100%;margin-top:12px;padding:11px;background:var(--pw-brand);color:#fff;border:0;border-radius:8px;font-size:15px;font-weight:600;cursor:pointer}'
        . 'button:hover{background:var(--pw-brand-hover)}'
        // 与 Go 侧 renderPasswordPage 保持同一品牌入口，避免两条跳转路径观感不一致。
        . '.brand{display:block;margin-top:16px;text-align:center;font-size:12.5px;color:var(--pw-dim);text-decoration:none}'
        . '.brand:hover{color:var(--pw-brand)}'
        . '</style></head><body><form class="card" method="post" action="/' . $uid . '">'
        . '<p class="lock">🔒</p><h1>此链接受密码保护</h1><p class="sub">请输入访问密码以继续</p>'
        . $msg
        . '<input type="password" name="password" placeholder="访问密码" required autofocus autocomplete="off">'
        . '<button type="submit">解锁访问</button>'
        . '<a class="brand" href="/">← 返回短网址首页</a>'
        . '</form></body></html>';
}
?>
