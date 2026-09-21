<?php
/*
@name:dwz-shorturl Redirect
@description:dwz-shorturl跳转文件
*/
include __DIR__ . '/includes/api.inc.php';

$uid = isset($_GET['uid']) && is_string($_GET['uid']) ? trim($_GET['uid']) : '';
if ($uid === '' || !preg_match('/^[a-z0-5]{6,8}$/', $uid)) redirect_error(404, 'Not Found');

$stmt = $DB->prepare('SELECT longurl, expire_at, status, password_hash FROM wjoy_log WHERE uid=? LIMIT 1');
if (!$stmt) redirect_error(500, 'Internal Server Error');
mysqli_stmt_bind_param($stmt, 's', $uid);
if (!mysqli_stmt_execute($stmt)) { mysqli_stmt_close($stmt); redirect_error(500, 'Internal Server Error'); }
mysqli_stmt_bind_result($stmt, $t_url, $expire_at, $status, $password_hash);
if (!mysqli_stmt_fetch($stmt)) { mysqli_stmt_close($stmt); redirect_error(404, 'Not Found'); }
mysqli_stmt_close($stmt);

// Soft-deleted links (admin/member) are disabled and stop being served.
if ((int)$status !== 1) redirect_error(404, 'Not Found');

if (!empty($expire_at)) {
    $expires = strtotime($expire_at);
    if ($expires !== false && $expires <= time()) redirect_error(410, '短链已过期');
}

// 密码保护：未解锁前展示密码页，仅解锁成功才跳转/计数。
if (!empty($password_hash)) {
    if (!password_unlock_ok($uid)) {
        if (($_SERVER['REQUEST_METHOD'] ?? 'GET') === 'POST') {
            // 尝试限流：每个 IP + 短码 60 秒内最多 5 次，避免密码被离线暴力穷举。
            // 键里必须带 IP：只按 $uid 计数时，任何人 POST 5 次错密码就能让所有访客
            // 60 秒内解不开这条短链（可循环维持），Go 侧 redirect.go 一直是 IP+uid。
            // 限流器不可用时本分支选择拒绝（fail-closed）：`!== true` 同时覆盖「超限」
            // 与「故障」，不再依赖 false 的假值巧合。密码尝试属安全控制，被挡住不影响
            // 短链本身可用，所以不走 #18 的放行原则。
            $pwAllowed = function_exists('rate_limit')
                ? rate_limit('pwtry:' . real_ip() . ':' . $uid, 5, 60)
                : true;
            if ($pwAllowed !== true) {
                if (!headers_sent()) { http_response_code(429); header('Retry-After: 60'); }
                echo password_page_html($uid, '尝试次数过多，请稍后再试');
                exit;
            }
            $pw = isset($_POST['password']) && is_string($_POST['password']) ? $_POST['password'] : '';
            if (password_verify($pw, (string)$password_hash)) {
                set_password_unlock_cookie($uid);
                $back = rtrim($public_base_url, '/') . '/' . $uid;
                header('Location: ' . $back, true, 302);
                exit;
            }
            echo password_page_html($uid, '密码错误，请重试');
            exit;
        }
        echo password_page_html($uid, '');
        exit;
    }
}

// A5：已移除遗留的 base64 兼容分支。历史上被 base64 编码的旧数据应通过
// backend/migrations/php/legacy_schema.php 一次性清洗；在热路径上静默改写 302 目标会
// 导致跳转结果与库中记录不符（审计困难），且可绕过创建时的 SSRF 规则。
// 跳转不抓取目标，创建时已完成 SSRF/DNS 校验；此处跳过 DNS 解析以避免每次跳转的解析开销，
// 但始终拒绝云元数据地址（isPrivateHost 内部已在 skip_dns 下保留该分支）。
$validation = validate_long_url($t_url, true);
if (!$validation[0]) redirect_error(410, '短链目标无效');

// Send the redirect header first, then flush the response so the client gets
// the 302 immediately. Click analytics and webhooks run afterwards in the
// fastcgi background, so they never block the redirect hot path.
header('Cache-Control: no-store, private, max-age=0');
header('Pragma: no-cache');
header('Expires: 0');
// 短链跳转不向目标站点泄露来源页 Referer
header('Referrer-Policy: no-referrer');
// 短码是 302 跳板而非落地页，不应被搜索引擎索引（sitemap 也已移除短码）
header('X-Robots-Tag: noindex, nofollow');
header('Location: ' . $t_url, true, 302);

if (function_exists('fastcgi_finish_request')) {
    fastcgi_finish_request();
}

// A6：点击计数收敛为单一来源。
// 原先热路径同时直写 wjoy_log.clicks 与（通过 record_click_analytics）写
// short_urls.clicks，再叠加 Go ClickQueue，同一访问可能被计多次、
// 且 clicks 与 click_logs 行数无法对账。
// 现在只调用 record_click_analytics()：它写入权威的 click_logs 明细并递增
// short_urls.clicks；wjoy_log.clicks 由 cron reconcileClicks 从
// short_urls.clicks 单向回填，不再由跳转路径直接自增。
record_click_analytics($uid);

// Fire link.clicked webhook (best-effort).
dispatch_webhook_event('link.clicked', array('uid' => $uid, 'short_url' => public_short_url($uid)));

$DB->close();
exit();

function redirect_error($status, $message) {
    global $DB;
    if (!headers_sent()) {
        http_response_code($status);
        // 与 Go 侧 renderErrorPage 使用同一套 CSS 变量 + prefers-color-scheme，
        // 保证两条跳转路径的品牌化错误页观感一致（含暗色适配）。
        header('Content-Type: text/html; charset=utf-8');
        header('Cache-Control: no-store');
        header('Referrer-Policy: no-referrer');
        header('X-Robots-Tag: noindex, nofollow');
    }

    // 按语义映射到访客可读的文案；「目标无效」面向站长，与「已过期」区分开。
    $map = array(
        410 => array('⏳', '这个短链已过期', '链接的有效期已结束。请联系分享者重新生成一条。'),
        404 => array('🔍', '短链不存在', '这个短码没有对应的链接，可能输入有误或已被删除。'),
        500 => array('🛠️', '服务暂时不可用', '服务器出了点问题，请稍后重试。'),
        503 => array('🛠️', '服务暂时不可用', '我们这边出了点问题，链接暂时没能打开。请稍后再试。'),
    );
    $page = isset($map[$status]) ? $map[$status] : array('⚠️', '链接暂时无法访问', '该链接当前不可访问。');
    if ($message === '短链已禁用') {
        $page = array('🚫', '这个短链已被停用', '分享者或平台已停用该链接。如有疑问请联系分享者。');
    } elseif ($message === '短链目标无效') {
        $page = array('⚠️', '这个短链暂时无法访问', '目标地址未通过安全检查（可能指向内网或非法站点）。如果这是你自己的链接，请重新创建。');
    }
    $title = htmlspecialchars($page[1], ENT_QUOTES, 'UTF-8');
    $desc = htmlspecialchars($page[2], ENT_QUOTES, 'UTF-8');
    echo '<!doctype html><html lang="zh-CN"><head><meta charset="utf-8">'
        . '<meta name="viewport" content="width=device-width,initial-scale=1">'
        . '<title>' . $title . ' - 短网址</title>'
        . '<meta name="robots" content="noindex">'
        . '<meta name="referrer" content="no-referrer">'
        . '<style>'
        . ':root{--ep-page:#f2f5f7;--ep-card:#fff;--ep-line:#e4ecee;--ep-text:#16292b;--ep-dim:#5b6f76;--ep-brand:#0e6e75;--ep-brand-hover:#0a5a60}'
        . '@media (prefers-color-scheme:dark){:root{--ep-page:#0d1b20;--ep-card:#122027;--ep-line:#23343b;--ep-text:#e6edf0;--ep-dim:#9aa9ae;--ep-brand:#12909a;--ep-brand-hover:#0e6e75}}'
        . '*{box-sizing:border-box}body{margin:0;min-height:100vh;display:grid;place-items:center;background:var(--ep-page);font-family:-apple-system,"PingFang SC","Microsoft YaHei",sans-serif;color:var(--ep-text)}'
        . '.card{width:min(92vw,400px);background:var(--ep-card);border:1px solid var(--ep-line);border-radius:14px;padding:32px 26px;text-align:center;box-shadow:0 8px 30px rgba(14,110,117,.08)}'
        . '.icon{font-size:36px;margin:0 0 10px}h1{font-size:18px;margin:0 0 10px;font-weight:700}'
        . 'p{font-size:13.5px;line-height:1.7;color:var(--ep-dim);margin:0 0 22px}'
        . '.actions{display:flex;gap:10px;flex-direction:column}'
        . 'a.btn{display:block;padding:11px;border-radius:8px;font-size:14px;font-weight:600;text-decoration:none;background:var(--ep-brand);color:#fff}'
        . 'a.btn:hover{background:var(--ep-brand-hover)}'
        . 'a.btn.ghost{background:transparent;color:var(--ep-brand);border:1px solid var(--ep-line)}'
        . '.slogan{margin-top:18px;font-size:12px;color:var(--ep-dim)}'
        . '</style></head><body><div class="card">'
        . '<p class="icon">' . $page[0] . '</p>'
        . '<h1>' . $title . '</h1>'
        . '<p>' . $desc . '</p>'
        . '<div class="actions"><a class="btn" href="./">返回首页</a>'
        . '<a class="btn ghost" href="./#single">重新生成短链</a></div>'
        . '<p class="slogan">短网址 · 一次生成，随处链接</p>'
        . '</div></body></html>';
    if (isset($DB)) $DB->close();
    exit();
}
