<?php
/*!
@name:dwz-shorturl Demo
@description:dwz-shorturl跳转文件
@author:陌涛 
@version:1.0
@time:2025-10-30
@copyright:陌涛
*/
include './includes/api.inc.php';
$uid=htmlspecialchars($_GET['uid']);
// 校验短码格式（6-8位，a-z0-5）
if($uid && !preg_match('/^[a-z0-5]{6,8}$/', $uid)){
    @header("http/1.1 404 not found"); 
    @header("status: 404 not found"); 
    echo 'echo 404'; 
    exit(); 
}
if(!$uid){
	@header("http/1.1 404 not found"); 
	@header("status: 404 not found"); 
}
$myrow=$DB->get_row("select * from wjoy_log where uid='$uid' limit 1");
if(!$myrow){
	@header("http/1.1 404 not found"); 
	@header("status: 404 not found"); 
    echo 'echo 404'; 
    exit(); 
	
}else{
	$t_url=$myrow['longurl'];
	if ($t_url == base64_encode(base64_decode($t_url))) {
        $t_url =  base64_decode($t_url);
    }
    header("Location: ".$t_url, true, 301);
    exit();
}