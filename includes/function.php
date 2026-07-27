<?php
function curl_get($url,$ip){
    $ch=curl_init($url);
    curl_setopt($ch, CURLOPT_URL, $url);
    curl_setopt($ch, CURLOPT_SSL_VERIFYPEER, false);
    curl_setopt($ch, CURLOPT_SSL_VERIFYHOST, false);
    $ip = rand(0,255).'.'.rand(0,255).'.'.rand(0,255).'.'.rand(0,255) ;
    $httpheader[] = 'X-FORWARDED-FOR:'.$ip.',CLIENT-IP:'.$ip;
   	if($ip === 1){
   		curl_setopt($ch, CURLOPT_HTTPHEADER, $httpheader);
   	}
	curl_setopt($ch, CURLOPT_RETURNTRANSFER, true);
	curl_setopt($ch, CURLOPT_USERAGENT, 'Mozilla/5.0 (Linux; U; Android 4.4.1; zh-cn; R815T Build/JOP40D) AppleWebKit/533.1 (KHTML, like Gecko)Version/4.0 MQQBrowser/4.5 Mobile Safari/533.1');
	curl_setopt($ch, CURLOPT_REFERER, $url);
	curl_setopt($ch, CURLOPT_TIMEOUT, 30);
	$content=curl_exec($ch);
	curl_close($ch);
	return($content);
}
function real_ip(){
	$ip = $_SERVER['REMOTE_ADDR'];
	if (isset($_SERVER['HTTP_CLIENT_IP']) && preg_match('/^([0-9]{1,3}\.){3}[0-9]{1,3}$/', $_SERVER['HTTP_CLIENT_IP'])) {
		$ip = $_SERVER['HTTP_CLIENT_IP'];
	} elseif(isset($_SERVER['HTTP_X_FORWARDED_FOR']) AND preg_match_all('#\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}#s', $_SERVER['HTTP_X_FORWARDED_FOR'], $matches)) {
		foreach ($matches[0] AS $xip) {
			if (!preg_match('#^(10|172\.16|192\.168)\.#', $xip)) {
				$ip = $xip;
				break;
			}
		}
	}
	return $ip;
}
function get_ip_city($ip){
    $url = 'http://int.dpool.sina.com.cn/iplookup/iplookup.php?format=json&ip=';
    @$city = curl_get($url . $ip);
    $city = json_decode($city, true);
    if ($city['city']) {
        $location = $city['province'].$city['city'];
    } else {
        $location = $city['province'];
    }
	if($location){
		return $location;
	}else{
		return false;
	}
}
function send_mail($to, $sub, $msg) {
	global $conf;
	include_once ROOT.'includes/smtp.class.php';
	$From = $conf['mail_name'];
	$Host = $conf['mail_stmp'];
	$Port = $conf['mail_port'];
	$SMTPAuth = 1;
	$Username = $conf['mail_name'];
	$Password = $conf['mail_pwd'];
	$Nickname = $conf['sitename'];
	$SSL = false;
	$mail = new SMTP($Host , $Port , $SMTPAuth , $Username , $Password , $SSL);
	$mail->att = array();
	if($mail->send($to , $From , $sub , $msg, $Nickname)) {
		return true;
	} else {
		return $mail->log;
	}
}
function daddslashes($string, $force = 0, $strip = FALSE) {
	!defined('MAGIC_QUOTES_GPC') && define('MAGIC_QUOTES_GPC', false);
	if(!MAGIC_QUOTES_GPC || $force) {
		if(is_array($string)) {
			foreach($string as $key => $val) {
				$string[$key] = daddslashes($val, $force, $strip);
			}
		} else {
			$string = addslashes($strip ? stripslashes($string) : $string);
		}
	}
	return $string;
}

function strexists($string, $find) {
	return !(strpos($string, $find) === FALSE);
}
function authcode($string, $operation = 'DECODE', $key = '', $expiry = 0) {
	$ckey_length = 4;
	$key = md5($key ? $key : ENCRYPT_KEY);
	$keya = md5(substr($key, 0, 16));
	$keyb = md5(substr($key, 16, 16));
	$keyc = $ckey_length ? ($operation == 'DECODE' ? substr($string, 0, $ckey_length): substr(md5(microtime()), -$ckey_length)) : '';
	$cryptkey = $keya.md5($keya.$keyc);
	$key_length = strlen($cryptkey);
	$string = $operation == 'DECODE' ? base64_decode(substr($string, $ckey_length)) : sprintf('%010d', $expiry ? $expiry + time() : 0).substr(md5($string.$keyb), 0, 16).$string;
	$string_length = strlen($string);
	$result = '';
	$box = range(0, 255);
	$rndkey = array();
	for($i = 0; $i <= 255; $i++) {
		$rndkey[$i] = ord($cryptkey[$i % $key_length]);
	}
	for($j = $i = 0; $i < 256; $i++) {
		$j = ($j + $box[$i] + $rndkey[$i]) % 256;
		$tmp = $box[$i];
		$box[$i] = $box[$j];
		$box[$j] = $tmp;
	}
	for($a = $j = $i = 0; $i < $string_length; $i++) {
		$a = ($a + 1) % 256;
		$j = ($j + $box[$a]) % 256;
		$tmp = $box[$a];
		$box[$a] = $box[$j];
		$box[$j] = $tmp;
		$result .= chr(ord($string[$i]) ^ ($box[($box[$a] + $box[$j]) % 256]));
	}
	if($operation == 'DECODE') {
		if((substr($result, 0, 10) == 0 || substr($result, 0, 10) - time() > 0) && substr($result, 10, 16) == substr(md5(substr($result, 26).$keyb), 0, 16)) {
			return substr($result, 26);
		} else {
			return '';
		}
	} else {
		return $keyc.str_replace('=', '', base64_encode($result));
	}
}

function random($length, $numeric = 0) {
	$seed = base_convert(md5(microtime().$_SERVER['DOCUMENT_ROOT']), 16, $numeric ? 10 : 35);
	$seed = $numeric ? (str_replace('0', '', $seed).'012340567890') : ($seed.'zZ'.strtoupper($seed));
	$hash = '';
	$max = strlen($seed) - 1;
	for($i = 0; $i < $length; $i++) {
		$hash .= $seed[mt_rand(0, $max)];
	}
	return $hash;
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

// 校验目标主机是否为私有/保留地址，防止生成可跳转内网的短链（SSRF 防护）
function isPrivateHost($host) {
    $host = strtolower(trim($host, "[] \t"));
    $ip = $host;
    if (filter_var($host, FILTER_VALIDATE_IP) === false) {
        $ip = gethostbyname($host);
        if ($ip === $host || $ip === false) {
            return true; // 解析失败，保守拦截
        }
    }
    // 私有地址或保留地址（如 127.0.0.1 / 10.x / 192.168.x / 169.254.169.254）-> 拦截
    if (filter_var($ip, FILTER_VALIDATE_IP, FILTER_FLAG_NO_PRIV_RANGE | FILTER_FLAG_NO_RES_RANGE) === false) {
        return true;
    }
    // 补充：部分 PHP 版本 filter_var 未覆盖的文档/保留段与 CGNAT
    $long = ip2long($ip);
    if ($long !== false) {
        $reserved = array(
            '192.0.2.0'    => '192.0.2.255',      // TEST-NET-1
            '198.51.100.0' => '198.51.100.255',   // TEST-NET-2
            '203.0.113.0'  => '203.0.113.255',    // TEST-NET-3
            '100.64.0.0'   => '100.127.255.255',  // CGNAT
            '192.88.99.0'  => '192.88.99.255',    // 6to4 中继
        );
        foreach ($reserved as $start => $end) {
            $s = ip2long($start);
            $e = ip2long($end);
            if ($long >= $s && $long <= $e) return true;
        }
    }
    return false;
}

// 按 IP 的滑动窗口限流（文件锁保证并发安全）；失败放行避免误杀
function rate_limit($ip, $max = 20, $window = 60) {
    $dir = ROOT . 'logs/ratelimit';
    if (!is_dir($dir)) @mkdir($dir, 0755, true);
    $file = $dir . '/' . md5($ip) . '.rl';
    $fp = @fopen($file, 'c+');
    if (!$fp) return true;
    flock($fp, LOCK_EX);
    $now = time();
    $data = @json_decode(stream_get_contents($fp), true);
    if (!is_array($data) || !isset($data['start']) || ($now - $data['start']) >= $window) {
        $data = array('start' => $now, 'count' => 1);
    } else {
        $data['count']++;
    }
    ftruncate($fp, 0);
    fseek($fp, 0);
    fwrite($fp, json_encode($data));
    flock($fp, LOCK_UN);
    fclose($fp);
    return $data['count'] <= $max;
}

// 创建短链：原子去重（依赖 url_hash 唯一索引），旧表无该列时自动退化为按 longurl 去重
function create_short_url($DB, $longurl) {
    $hash = md5($longurl);
    if (isset($DB->link) && function_exists('mysqli_prepare')) {
        $uid = shorturl($longurl);
        if ($stmt = mysqli_prepare($DB->link, "INSERT INTO wjoy_log (uid,longurl,url_hash) VALUES (?,?,?)")) {
            mysqli_stmt_bind_param($stmt, 'sss', $uid, $longurl, $hash);
            if (mysqli_stmt_execute($stmt)) {
                mysqli_stmt_close($stmt);
                return array('code' => $uid, 'msg' => 'success', 'result' => 1);
            }
            $errno = mysqli_errno($DB->link);
            if ($errno == 1062) { // 唯一键冲突：同 URL 已存在
                mysqli_stmt_close($stmt);
                if ($s2 = mysqli_prepare($DB->link, "SELECT uid FROM wjoy_log WHERE url_hash=? LIMIT 1")) {
                    mysqli_stmt_bind_param($s2, 's', $hash);
                    mysqli_stmt_execute($s2);
                    mysqli_stmt_bind_result($s2, $exist_uid);
                    mysqli_stmt_fetch($s2);
                    mysqli_stmt_close($s2);
                    if (!empty($exist_uid)) return array('code' => $exist_uid, 'msg' => 'existence', 'result' => 1);
                }
                return array('code' => $uid, 'msg' => 'existence', 'result' => 1);
            }
            mysqli_stmt_close($stmt);
            // 非唯一键错误（如 url_hash 列不存在）-> 退化为仅按 longurl 去重
        }
        $enc = $DB->escape($longurl);
        $myrow = $DB->get_row("SELECT uid FROM wjoy_log WHERE longurl='" . $enc . "' LIMIT 1");
        if (!$myrow) {
            $sql = $DB->query("INSERT INTO wjoy_log (uid,longurl) VALUES ('" . $DB->escape($uid) . "','" . $enc . "')");
            return $sql ? array('code' => $uid, 'msg' => 'success', 'result' => 1) : array('code' => 0, 'msg' => 'failure', 'result' => 10003);
        }
        return array('code' => ($myrow['uid'] ?: $uid), 'msg' => 'existence', 'result' => 1);
    }
    // 无 mysqli_prepare 保底
    $enc = $DB->escape($longurl);
    $myrow = $DB->get_row("SELECT uid FROM wjoy_log WHERE longurl='" . $enc . "' LIMIT 1");
    if (!$myrow) {
        $uid = shorturl($longurl);
        $sql = $DB->query("INSERT INTO wjoy_log (uid,longurl) VALUES ('" . $DB->escape($uid) . "','" . $enc . "')");
        return $sql ? array('code' => $uid, 'msg' => 'success', 'result' => 1) : array('code' => 0, 'msg' => 'failure', 'result' => 10003);
    }
    return array('code' => ($myrow['uid'] ?: shorturl($longurl)), 'msg' => 'existence', 'result' => 1);
}
?>