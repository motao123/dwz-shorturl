<?php
/*!
@name:dwz-shorturl API
@description:dwz-shorturl接口文件
@author:陌涛 
@version:1.2
@time:2025-10-30
@copyright:陌涛
*/
include './includes/api.inc.php';
$longurl = (isset($_GET['url'])) ?$_GET['url']:$_POST['url'];
$format = (isset($_GET['format'])) ?$_GET['format']:$_POST['format'];

// 统一响应头（仅对 JSON）
if (!headers_sent()) {
    if (!isset($format) || $format !== 'txt') {
        header('Content-Type: application/json; charset=utf-8');
    }
}

if(!$longurl){
	show_result(0,"the url cannot be empty",10001);
  	exit();
}
// 仅允许 http/https，限制长度，拒绝危险协议
if (strlen($longurl) > 2048) {
    show_result(0,"url too long",10002);
    exit();
}
$parts = @parse_url($longurl);
if (!$parts || !isset($parts['scheme']) || !in_array(strtolower($parts['scheme']), ['http','https'])) {
    show_result(0,"url is incorrect",10002);
    exit();
}
$uid=shorturl($longurl);

// 使用预处理：优先通过 longurl 查询是否已存在
if (isset($DB->link) && function_exists('mysqli_prepare')) {
    $enc = base64_encode($longurl);
    if ($stmt = mysqli_prepare($DB->link, "SELECT uid FROM wjoy_log WHERE longurl=? LIMIT 1")) {
        mysqli_stmt_bind_param($stmt, 's', $enc);
        mysqli_stmt_execute($stmt);
        mysqli_stmt_bind_result($stmt, $exist_uid);
        if (mysqli_stmt_fetch($stmt)) {
            mysqli_stmt_close($stmt);
            show_result($exist_uid?:$uid,"existence",1);
        } else {
            mysqli_stmt_close($stmt);
            // 插入
            if ($stmt2 = mysqli_prepare($DB->link, "INSERT INTO wjoy_log (uid,longurl) VALUES (?,?)")) {
                mysqli_stmt_bind_param($stmt2, 'ss', $uid, $enc);
                if (mysqli_stmt_execute($stmt2)) {
                    mysqli_stmt_close($stmt2);
                    show_result($uid,"success",1);
                } else {
                    mysqli_stmt_close($stmt2);
                    show_result(0,"failure",10003);
                }
            } else {
                show_result(0,"failure",10003);
            }
        }
    } else {
        // 退化到转义拼接
        $enc_safe = $DB->escape(base64_encode($longurl));
        $myrow=$DB->get_row("select * from wjoy_log where longurl='".$enc_safe."' limit 1");
        if(!$myrow){
            $sql=$DB->query("insert into `wjoy_log` (`uid`,`longurl`) values ('".$DB->escape($uid)."','".$enc_safe."')");
            if($sql){
                show_result($uid,"success",1);
            }else{
                show_result(0,"failure",10003);
            }
        }else{
            show_result($myrow['uid']?:$uid,"existence",1);
        }
    }
} else {
    // 环境不支持 mysqli_prepare 的保底路径
    $enc_safe = $DB->escape(base64_encode($longurl));
    $myrow=$DB->get_row("select * from wjoy_log where longurl='".$enc_safe."' limit 1");
    if(!$myrow){
        $sql=$DB->query("insert into `wjoy_log` (`uid`,`longurl`) values ('".$DB->escape($uid)."','".$enc_safe."')");
        if($sql){
            show_result($uid,"success",1);
        }else{
            show_result(0,"failure",10003);
        }
    }else{
        show_result($myrow['uid']?:$uid,"existence",1);
    }
}

$DB->close();

function show_result($code,$msg,$result){
	global $format;
	if ($format === 'txt') {
		if ($code === 0 ){
			echo $msg;
		}else{
			echo $code;
		}
	}else{
		$result=array("code"=>$code,"msg"=>$msg,"result"=>$result);
		echo json_encode($result);
	}

}