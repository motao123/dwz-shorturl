-- Public-facing member accounts for the PHP frontend.
-- Non-destructive: created only if not already present.
CREATE TABLE IF NOT EXISTS `members` (
  `id`            INT UNSIGNED NOT NULL AUTO_INCREMENT,
  `username`      VARCHAR(32)  NOT NULL,
  `email`         VARCHAR(128) NOT NULL,
  `password_hash` VARCHAR(255) NOT NULL COMMENT 'bcrypt via password_hash()',
  `status`        TINYINT      NOT NULL DEFAULT 1 COMMENT '1=active 0=disabled',
  `last_login_at` DATETIME     NULL,
  `last_login_ip` VARCHAR(45)  NULL,
  `created_at`    TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uniq_username` (`username`),
  UNIQUE KEY `uniq_email` (`email`),
  KEY `idx_status` (`status`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;