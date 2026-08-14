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
function validate_long_url($url, $skip_dns = false) {
    if (!is_string($url) || $url === '') return array(false, 'the url cannot be empty', 10001);
    if (strlen($url) > 2048) return array(false, 'url too long', 10002);
    if (preg_match('/[\x00-\x20\x7f]/', $url)) return array(false, 'url is incorrect', 10002);
    $parts = parse_url($url);
    if (!is_array($parts) || empty($parts['scheme']) || empty($parts['host'])) return array(false, 'url is incorrect', 10002);
    if (!in_array(strtolower($parts['scheme']), array('http', 'https'), true)) return array(false, 'url is incorrect', 10002);
    if (isset($parts['user']) || isset($parts['pass'])) return array(false, 'url is incorrect', 10002);
    if (isset($parts['port']) && ($parts['port'] < 1 || $parts['port'] > 65535)) return array(false, 'url is incorrect', 10002);
    if (isPrivateHost($parts['host'], $skip_dns)) return array(false, 'url host not allowed', 10004);
    return array(true, '', 1);
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

    // If a domain_id is specified, try to look up the domain from the domains table
    if ($domain_id !== null && $domain_id !== '' && isset($DB) && !empty($DB->link)) {
        $stmt = $DB->prepare('SELECT domain, scheme FROM domains WHERE id=? AND status=1 LIMIT 1');
        if ($stmt) {
            mysqli_stmt_bind_param($stmt, 's', $domain_id);
            mysqli_stmt_execute($stmt);
            mysqli_stmt_bind_result($stmt, $d_domain, $d_scheme);
            if (mysqli_stmt_fetch($stmt)) {
                mysqli_stmt_close($stmt);
                $scheme = !empty($d_scheme) ? $d_scheme : 'https';
                return $scheme . '://' . $d_domain . '/' . rawurlencode($uid);
            }
            mysqli_stmt_close($stmt);
        }
    }

    // Fallback to global base URL
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
function create_short_url($DB, $longurl, $custom = null, $expire_days = 0, $domain_id = null, $password = '') {
    $custom = $custom === null ? '' : trim((string)$custom);
    if (!validate_custom_code($custom)) return array('code' => 0, 'short_url' => '', 'msg' => '自定义短码格式错误（需 6-8 位，仅含 a-z 与 0-5）', 'result' => 10006);
    $expire_days = validate_expire_days($expire_days);
    if ($expire_days === false) return array('code' => 0, 'short_url' => '', 'msg' => '有效期仅支持 0、1、7、30 或 365 天', 'result' => 10008);
    $password = is_string($password) ? trim($password) : '';
    if (strlen($password) > 72) return array('code' => 0, 'short_url' => '', 'msg' => '访问密码过长（最多 72 字节）', 'result' => 10008);
    $password_hash = $password !== '' ? password_hash($password, PASSWORD_DEFAULT) : null;

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
            return short_url_result($existing['uid'], 'renewed', 'renewed', $domain_id);
        }
        return short_url_result($existing['uid'], 'existence', 'existing', $domain_id);
    }

    $attempts = $custom !== '' ? 1 : 12;
    for ($attempt = 0; $attempt < $attempts; $attempt++) {
        try {
            $salt = $attempt === 0 ? '' : '|' . $attempt . '|' . bin2hex(random_bytes(8));
        } catch (Throwable $e) {
            $salt = '|' . $attempt . '|' . uniqid('', true);
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

$blocked_domain_suffixes = array(
    // 菠菜 / gambling
    'bet365.com', '188bet.com', 'bwin.com', 'dafabet.com', 'm88.com',
    '18win.com', 'betway.com', 'w88.com', 'fun88.com', '12bet.com',
    '10bet.com', '365bet.com', 'betflik.com', 'ufa999.com', 'pgslot.com',
    'slotxo.com', 'milton88.com', 'boss888.com', 'mgm888.com', 'jackpot.com',
    // 仿冒 / phishing / scam
    'phishing.com', 'scam.com', 'bitcoin-cash.cc', 'secure-login-verify.com',
    'account-verify.com', 'paypal-verify.com', 'appleid-verify.com',
    // 恶意下载 / malware
    'malware.com', 'trojan-download.com', 'spam-site.com',
);

$blocked_keywords = array(
    // 菠菜 / gambling
    'betting', 'casino', 'poker', 'slot', 'gambling', '老虎机', '百家乐',
    '轮盘', '德州扑克', '棋牌充值', '博彩', '赌场', '网络赌博', '太阳城',
    '澳门娱乐场', '六合彩', '外围赌', '重庆时时彩', '彩票投注',
    // 仿冒 / phishing
    'password reset please login', 'verify your account immediately',
    '登录验证您的账户', '您的账户已被冻结', '账户异常请立即验证',
    // 恶意下载 / malware
    'download-virus', 'free-crack', 'keygen', '盗版激活码',
    // 诈骗 / scam
    '刷单返利', '稳赚不赔', '高额回报', '保本保息', '先付后取',
);

function check_url_violation($url) {
    global $blocked_domain_suffixes, $blocked_keywords;

    $parts = parse_url((string)$url);
    $host = isset($parts['host']) ? strtolower(trim((string)$parts['host'], "[] \t")) : '';
    $lower = strtolower((string)$url);

    // 云元数据端点始终拦截（SSRF 纵深防御）
    if ($host === '169.254.169.254' || substr($host, -15) === '.169.254.169.254'
        || $host === 'metadata.google.internal' || substr($host, -24) === 'metadata.google.internal') {
        return array('blocked' => true, 'reason' => 'cloud metadata address is not allowed');
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
    $hash = md5($longurl);
    $source = 'web';
    $member_id = $member_id > 0 ? (int)$member_id : null;
    $password_hash = $password_hash === null || $password_hash === '' ? null : (string)$password_hash;
    $stmt = $ADMIN_DB->prepare('INSERT INTO short_urls (uid, long_url, url_hash, expire_at, member_id, source, status, password_hash) VALUES (?,?,?,?,?,?,1,?) ON DUPLICATE KEY UPDATE expire_at=IFNULL(short_urls.expire_at, VALUES(expire_at))');
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
    if (!$ADMIN_DB || empty($ADMIN_DB->link)) return;
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

// Dispatch an event to subscribed webhooks (admin DB).
// Records every attempt in webhook_deliveries and retries up to 3 times with
// backoff so a temporarily down receiver doesn't silently lose the event.
// Does nothing when the admin DB is unavailable or there are no matching webhooks.
function dispatch_webhook_event($event, $payload) {
    global $ADMIN_DB;
    if (!$ADMIN_DB || empty($ADMIN_DB->link)) return;
    $stmt = $ADMIN_DB->prepare('SELECT id, url, secret, events FROM webhooks WHERE status=1 AND deleted_at IS NULL');
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
    while ($row = mysqli_fetch_assoc($res)) {
        $events = json_decode($row['events'], true);
        if (!is_array($events) || !in_array($event, $events, true)) continue;
        $headers = array('Content-Type: application/json', 'User-Agent: dwz-shorturl-webhook/1.0');
        if (!empty($row['secret'])) {
            $headers[] = 'X-Webhook-Signature: sha256=' . hash_hmac('sha256', $body, $row['secret']);
        }
        $max_attempts = 3;
        for ($attempt = 1; $attempt <= $max_attempts; $attempt++) {
            $status = 0;
            $success = 0;
            $resp_body = '';
            $ch = curl_init($row['url']);
            curl_setopt_array($ch, array(
                CURLOPT_POST => true,
                CURLOPT_POSTFIELDS => $body,
                CURLOPT_HTTPHEADER => $headers,
                CURLOPT_RETURNTRANSFER => true,
                CURLOPT_TIMEOUT => 3,
                CURLOPT_CONNECTTIMEOUT => 2,
            ));
            $resp = curl_exec($ch);
            $http_code = (int)curl_getinfo($ch, CURLINFO_HTTP_CODE);
            curl_close($ch);
            if ($resp !== false) {
                $status = $http_code;
                $resp_body = substr($resp, 0, 512);
                if ($http_code >= 200 && $http_code < 300) $success = 1;
            }
            $ins = $ADMIN_DB->prepare('INSERT INTO webhook_deliveries (webhook_id, event, payload, response_status, response_body, attempt, success, created_at) VALUES (?,?,?,?,?,?,?,NOW())');
            if ($ins) {
                mysqli_stmt_bind_param($ins, 'issisii', $row['id'], $event, $body, $status, $resp_body, $attempt, $success);
                mysqli_stmt_execute($ins);
                mysqli_stmt_close($ins);
            }
            if ($success === 1) break;
            if ($attempt < $max_attempts) usleep($attempt * 1000000); // 1s, 2s backoff
        }
    }
    mysqli_stmt_close($stmt);
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
        . '*{box-sizing:border-box}body{margin:0;min-height:100vh;display:grid;place-items:center;background:#f2f5f7;font-family:-apple-system,"PingFang SC","Microsoft YaHei",sans-serif;color:#16292b}'
        . '.card{width:min(92vw,360px);background:#fff;border:1px solid #e4ecee;border-radius:14px;padding:28px 24px;box-shadow:0 8px 30px rgba(14,110,117,.08)}'
        . '.lock{font-size:34px;text-align:center;margin:0 0 6px}h1{font-size:17px;text-align:center;margin:0 0 6px;font-weight:700}'
        . '.sub{font-size:12.5px;text-align:center;color:#6b7f86;margin:0 0 18px}'
        . 'input{width:100%;padding:11px 12px;border:1px solid #d3e0e3;border-radius:8px;font-size:15px;outline:none}'
        . 'input:focus{border-color:#0e6e75;box-shadow:0 0 0 3px rgba(14,110,117,.12)}'
        . 'button{width:100%;margin-top:12px;padding:11px;background:#0e6e75;color:#fff;border:0;border-radius:8px;font-size:15px;font-weight:600;cursor:pointer}'
        . 'button:hover{background:#0a5a60}'
        . '</style></head><body><form class="card" method="post" action="/' . $uid . '">'
        . '<p class="lock">🔒</p><h1>此链接受密码保护</h1><p class="sub">请输入访问密码以继续</p>'
        . $msg
        . '<input type="password" name="password" placeholder="访问密码" required autofocus autocomplete="off">'
        . '<button type="submit">解锁访问</button>'
        . '</form></body></html>';
}
?>
