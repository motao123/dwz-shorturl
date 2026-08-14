-- Violation review log: records blocked URL submissions for manual review.
-- Non-destructive: created only if not already present.
CREATE TABLE IF NOT EXISTS `violation_reviews` (
  `id`          INT UNSIGNED NOT NULL AUTO_INCREMENT,
  `url`         TEXT         NOT NULL,
  `reason`      VARCHAR(64)  NOT NULL DEFAULT '',
  `ip`          VARCHAR(45)  NULL,
  `source`      VARCHAR(16)  NOT NULL DEFAULT 'api' COMMENT 'api|batch',
  `reviewed`    TINYINT      NOT NULL DEFAULT 0 COMMENT '0=pending 1=reviewed',
  `reviewed_at` DATETIME     NULL,
  `note`        VARCHAR(255) NULL,
  `created_at`  TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  KEY `idx_reviewed` (`reviewed`),
  KEY `idx_created_at` (`created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;