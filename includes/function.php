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
function validate_long_url($url) {
    if (!is_string($url) || $url === '') return array(false, 'the url cannot be empty', 10001);
    if (strlen($url) > 2048) return array(false, 'url too long', 10002);
    if (preg_match('/[\x00-\x20\x7f]/', $url)) return array(false, 'url is incorrect', 10002);
    $parts = parse_url($url);
    if (!is_array($parts) || empty($parts['scheme']) || empty($parts['host'])) return array(false, 'url is incorrect', 10002);
    if (!in_array(strtolower($parts['scheme']), array('http', 'https'), true)) return array(false, 'url is incorrect', 10002);
    if (isset($parts['user']) || isset($parts['pass'])) return array(false, 'url is incorrect', 10002);
    if (isset($parts['port']) && ($parts['port'] < 1 || $parts['port'] > 65535)) return array(false, 'url is incorrect', 10002);
    if (isPrivateHost($parts['host'])) return array(false, 'url host not allowed', 10004);
    return array(true, '', 1);
}

// Resolve every address and reject the host if any answer is private, reserved, or unresolved.
function isPrivateHost($host) {
    $host = strtolower(trim((string)$host, "[] \t"));
    if ($host === '' || $host === 'localhost' || substr($host, -6) === '.local') return true;
    $addresses = array();
    if (filter_var($host, FILTER_VALIDATE_IP)) {
        $addresses[] = $host;
    } else {
        $ipv4 = @gethostbynamel($host);
        if (is_array($ipv4)) $addresses = array_merge($addresses, $ipv4);
        if (function_exists('dns_get_record') && defined('DNS_AAAA')) {
            $ipv6 = @dns_get_record($host, DNS_AAAA);
            if (is_array($ipv6)) foreach ($ipv6 as $record) if (!empty($record['ipv6'])) $addresses[] = $record['ipv6'];
        }
    }
    $addresses = array_unique($addresses);
    if (!$addresses) return true;
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

function public_short_url($uid) {
    global $public_base_url;
    $base = isset($public_base_url) ? rtrim(trim((string)$public_base_url), '/') : '';
    if ($base === '' || !filter_var($base, FILTER_VALIDATE_URL)) return '';
    $parts = parse_url($base);
    if (!is_array($parts) || empty($parts['scheme']) || empty($parts['host']) || !in_array(strtolower($parts['scheme']), array('http', 'https'), true)) return '';
    return $base . '/' . rawurlencode($uid);
}

function rate_limit($ip, $max = 20, $window = 60, $cost = 1) {
    global $rate_limit_dir;
    $dir = isset($rate_limit_dir) && is_string($rate_limit_dir) && $rate_limit_dir !== '' ? $rate_limit_dir : ROOT . 'logs/ratelimit';
    if (!is_dir($dir) && !@mkdir($dir, 0755, true) && !is_dir($dir)) return false;
    $fp = @fopen(rtrim($dir, '/\\') . '/' . md5((string)$ip) . '.rl', 'c+');
    if (!$fp) return false;
    if (!flock($fp, LOCK_EX)) { fclose($fp); return false; }
    $now = time();
    $data = json_decode(stream_get_contents($fp), true);
    if (!is_array($data) || !isset($data['start'], $data['count']) || ($now - (int)$data['start']) >= $window) $data = array('start' => $now, 'count' => 0);
    $cost = max(1, (int)$cost);
    $allowed = ((int)$data['count'] + $cost) <= $max;
    if ($allowed) $data['count'] += $cost;
    ftruncate($fp, 0); rewind($fp); fwrite($fp, json_encode($data)); fflush($fp);
    flock($fp, LOCK_UN); fclose($fp);
    return $allowed;
}

function short_url_result($uid, $msg, $state = 'existing') {
    return array(
        'code' => $uid,
        'short_url' => public_short_url($uid),
        'msg' => $msg,
        'result' => 1,
        'state' => $state,
        'created' => $state === 'created'
    );
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
function create_short_url($DB, $longurl, $custom = null, $expire_days = 0) {
    $custom = $custom === null ? '' : trim((string)$custom);
    if (!validate_custom_code($custom)) return array('code' => 0, 'short_url' => '', 'msg' => '自定义短码格式错误（需 6-8 位，仅含 a-z 与 0-5）', 'result' => 10006);
    $expire_days = validate_expire_days($expire_days);
    if ($expire_days === false) return array('code' => 0, 'short_url' => '', 'msg' => '有效期仅支持 0、1、7、30 或 365 天', 'result' => 10008);

    $hash = md5($longurl);
    $expire_at = $expire_days > 0 ? date('Y-m-d H:i:s', time() + $expire_days * 86400) : null;
    $existing = find_short_by_hash($DB, $hash);
    if (is_array($existing) && !empty($existing['uid'])) {
        if ($custom !== '' && $custom !== $existing['uid']) {
            return array('code' => 0, 'short_url' => '', 'msg' => '该网址已有短链，不能改用其他自定义短码', 'result' => 10013);
        }
        $was_expired = !empty($existing['expire_at']) && strtotime($existing['expire_at']) !== false && strtotime($existing['expire_at']) <= time();
        if ($was_expired) {
            if (!renew_short_expiry($DB, $hash, $expire_at)) return array('code' => 0, 'short_url' => '', 'msg' => 'failure', 'result' => 10003);
            return short_url_result($existing['uid'], 'renewed', 'renewed');
        }
        return short_url_result($existing['uid'], 'existence', 'existing');
    }

    $attempts = $custom !== '' ? 1 : 12;
    for ($attempt = 0; $attempt < $attempts; $attempt++) {
        try {
            $salt = $attempt === 0 ? '' : '|' . $attempt . '|' . bin2hex(random_bytes(8));
        } catch (Throwable $e) {
            $salt = '|' . $attempt . '|' . uniqid('', true);
        }
        $uid = $custom !== '' ? $custom : shorturl($longurl . $salt);
        $stmt = $DB->prepare('INSERT INTO wjoy_log (uid,longurl,url_hash,expire_at) VALUES (?,?,?,?)');
        if (!$stmt) return array('code' => 0, 'short_url' => '', 'msg' => 'failure', 'result' => 10003);
        mysqli_stmt_bind_param($stmt, 'ssss', $uid, $longurl, $hash, $expire_at);
        $ok = mysqli_stmt_execute($stmt);
        $errno = mysqli_stmt_errno($stmt);
        mysqli_stmt_close($stmt);
        if ($ok) return short_url_result($uid, 'success', 'created');
        if ($errno !== 1062) return array('code' => 0, 'short_url' => '', 'msg' => 'failure', 'result' => 10003);
        $existing = find_short_by_hash($DB, $hash);
        if (is_array($existing) && !empty($existing['uid'])) {
            if ($custom !== '' && $custom !== $existing['uid']) {
                return array('code' => 0, 'short_url' => '', 'msg' => '该网址已有短链，不能改用其他自定义短码', 'result' => 10013);
            }
            $was_expired = !empty($existing['expire_at']) && strtotime($existing['expire_at']) !== false && strtotime($existing['expire_at']) <= time();
            if ($was_expired) {
                if (!renew_short_expiry($DB, $hash, $expire_at)) return array('code' => 0, 'short_url' => '', 'msg' => 'failure', 'result' => 10003);
                return short_url_result($existing['uid'], 'renewed', 'renewed');
            }
            return short_url_result($existing['uid'], 'existence', 'existing');
        }
        if ($custom !== '') return array('code' => 0, 'short_url' => '', 'msg' => '自定义短码已被占用', 'result' => 10007);
    }
    return array('code' => 0, 'short_url' => '', 'msg' => '短码生成冲突，请重试', 'result' => 10009);
}
?>
