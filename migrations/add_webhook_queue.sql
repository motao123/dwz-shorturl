-- 异步 webhook 投递队列。
--
-- 背景：PHP 跳转热路径（do.php）原先同步 curl 所有订阅者，最坏阻塞约 11s
-- （3 次重试 + 退避）。即使已把超时压到毫秒级，仍占用请求生命周期；在非
-- FastCGI SAPI 下会直接拖慢用户跳转。
--
-- 现在 do.php / api.php 只做一次 INSERT 入队（O(1)，无网络），由
-- migrations/webhook_worker.php（cron 每分钟 / 常驻循环）负责投递与重试。
-- 投递结果仍写入 webhook_deliveries，保持后台可见性不变。
CREATE TABLE IF NOT EXISTS `webhook_queue` (
  `id`          BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  `webhook_id`  BIGINT UNSIGNED NOT NULL,
  `event`       VARCHAR(32)  NOT NULL,
  `payload`     JSON         NOT NULL,
  `attempts`    INT          NOT NULL DEFAULT 0,
  `max_attempts` INT         NOT NULL DEFAULT 3,
  `next_retry_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  `locked_at`   DATETIME(3)  NULL,
  `status`      TINYINT      NOT NULL DEFAULT 0 COMMENT '0=pending,1=success,2=failed',
  `last_error`  VARCHAR(255) NULL,
  `created_at`  DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  `updated_at`  DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  KEY `idx_pending` (`status`, `next_retry_at`),
  KEY `idx_webhook` (`webhook_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
