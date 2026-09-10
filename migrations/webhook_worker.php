<?php
/*
@name: dwz-shorturl Webhook Worker
@description: 消费 webhook_queue，异步投递 webhook 事件（支持重试与指数退避）。

用法：
  # 单次消费一批（推荐由 system cron 每分钟调用）
  php migrations/webhook_worker.php

  # 常驻循环（无 cron 环境，配合 supervisor/systemd 使用）
  php migrations/webhook_worker.php --loop --interval=5

  # 指定单批数量 / 最大运行时长（秒），适合受限的 cron 环境
  php migrations/webhook_worker.php --batch=50 --max-runtime=50

与 Go 后端共用同一套表（webhooks / webhook_deliveries / webhook_queue），
PHP 跳转路径与 Go /r/:code 路径的事件最终都由本 worker 或 Go 侧 ClickQueue 投递。
*/

if (PHP_SAPI !== 'cli') {
    http_response_code(403);
    exit("This script can only be run from the command line.\n");
}

require __DIR__ . '/../includes/api.inc.php';

$opts = getopt('', array('loop', 'interval::', 'batch::', 'max-runtime::'));
$loop = isset($opts['loop']);
$interval = isset($opts['interval']) ? max(1, (int)$opts['interval']) : 5;
$batch = isset($opts['batch']) ? max(1, (int)$opts['batch']) : 20;
$maxRuntime = isset($opts['max-runtime']) ? max(1, (int)$opts['max-runtime']) : 0;

if (!$ADMIN_DB || empty($ADMIN_DB->link)) {
    fwrite(STDERR, "admin DB unavailable, nothing to do\n");
    exit(1);
}

if (webhook_queue_missing()) {
    fwrite(STDERR, "webhook_queue table missing, run migrations/add_webhook_queue.sql first\n");
    exit(1);
}

$startedAt = time();

// 投递一批到期任务；返回本次处理条数。
function webhook_queue_run_batch($batch) {
    global $ADMIN_DB;
    $rows = webhook_queue_fetch($batch);
    foreach ($rows as $task) {
        $wid = (int)$task['webhook_id'];
        // 目标可能在入队后被删除/禁用，投递前再确认一次。
        $stmt = $ADMIN_DB->prepare('SELECT id, url, secret FROM webhooks WHERE id=? AND status=1 AND deleted_at IS NULL LIMIT 1');
        if (!$stmt) continue;
        mysqli_stmt_bind_param($stmt, 'i', $wid);
        mysqli_stmt_execute($stmt);
        $res = mysqli_stmt_get_result($stmt);
        $webhook = $res ? mysqli_fetch_assoc($res) : null;
        mysqli_stmt_close($stmt);

        $attempt = (int)$task['attempts'] + 1;
        $maxAttempts = max(1, (int)$task['max_attempts']);
        $success = false;
        $lastError = 'webhook not found or disabled';

        if ($webhook) {
            // 二次 SSRF 校验，防 DNS 重绑定 / 历史脏数据。
            $check = validate_long_url((string)$webhook['url'], false);
            if (!$check[0]) {
                $lastError = 'unsafe target';
            } else {
                // 队列里存的是完整的事件 envelope（含 id/event/timestamp/data），
                // 直接作为 body 投递，保证签名与入队时一致。
                $body = (string)$task['payload'];
                list($success, $err) = webhook_deliver_now($webhook, (string)$task['event'], $body, $attempt);
                if (!$success) $lastError = $err ?: 'delivery failed';
            }
        }

        $id = (int)$task['id'];
        if ($success) {
            mysqli_query($ADMIN_DB->link, 'UPDATE webhook_queue SET status=1, attempts=' . $attempt . ', locked_at=NULL, last_error=NULL WHERE id=' . $id);
        } elseif ($attempt >= $maxAttempts) {
            $errSql = $ADMIN_DB->escape(substr((string)$lastError, 0, 255));
            mysqli_query($ADMIN_DB->link, "UPDATE webhook_queue SET status=2, attempts=" . $attempt . ", locked_at=NULL, last_error='" . $errSql . "' WHERE id=" . $id);
        } else {
            // 指数退避：1、4、16… 分钟（封顶 30 分钟）。
            $delay = min(1800, (int)pow(4, $attempt - 1) * 60);
            $errSql = $ADMIN_DB->escape(substr((string)$lastError, 0, 255));
            mysqli_query($ADMIN_DB->link, "UPDATE webhook_queue SET attempts=" . $attempt . ", locked_at=NULL, last_error='" . $errSql . "', next_retry_at=DATE_ADD(NOW(3), INTERVAL " . $delay . " SECOND) WHERE id=" . $id);
        }
    }
    return count($rows);
}

if (!$loop) {
    $n = webhook_queue_run_batch($batch);
    fwrite(STDOUT, date('c') . " webhook worker: processed {$n} task(s)\n");
    exit(0);
}

fwrite(STDOUT, date('c') . " webhook worker loop started (interval={$interval}s, batch={$batch})\n");
while (true) {
    $n = webhook_queue_run_batch($batch);
    if ($maxRuntime > 0 && (time() - $startedAt) >= $maxRuntime) {
        fwrite(STDOUT, date('c') . " max runtime reached, exiting\n");
        exit(0);
    }
    if ($n === 0) sleep($interval);
    else usleep(200000);
}
