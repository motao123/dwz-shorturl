-- Webhook subscriptions and delivery logs for outbound event notifications.
CREATE TABLE IF NOT EXISTS `webhooks` (
  `id`         BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  `name`       VARCHAR(64)  NOT NULL,
  `url`        VARCHAR(512) NOT NULL,
  `events`     JSON         NOT NULL,
  `secret`     VARCHAR(64)  NULL,
  `status`     TINYINT      NOT NULL DEFAULT 1,
  `created_by` BIGINT UNSIGNED NULL,
  `created_at` DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  `updated_at` DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  `deleted_at` DATETIME(3)  NULL,
  KEY `idx_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `webhook_deliveries` (
  `id`              BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  `webhook_id`      BIGINT UNSIGNED NOT NULL,
  `event`           VARCHAR(32)  NOT NULL,
  `payload`         JSON         NOT NULL,
  `response_status` INT          NULL,
  `response_body`   TEXT         NULL,
  `attempt`         INT          NOT NULL DEFAULT 1,
  `success`         TINYINT      NOT NULL DEFAULT 0,
  `created_at`      DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  KEY `idx_webhook` (`webhook_id`, `created_at` DESC)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;