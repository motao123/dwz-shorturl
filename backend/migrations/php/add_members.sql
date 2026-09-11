-- migrate: after php/legacy_schema.php
--
-- 由统一迁移入口执行（连接默认库 = 公共库），不再包含 `USE <库名>`。
-- Public-facing member accounts for the PHP frontend.
-- Non-destructive: created only if not already present.
--
-- NOTE: this file is kept in sync with install.sql (the canonical fresh-install
-- schema). Older revisions of this file omitted the token_version / verify_* /
-- reset_* columns, which made member_current() (includes/auth.php) fail on
-- databases that only ran this migration. If you are upgrading an existing
-- database, run backend/migrations/add_missing_columns.sql as well — it
-- backfills any of these columns that are missing, idempotently.
CREATE TABLE IF NOT EXISTS `members` (
  `id`            INT UNSIGNED NOT NULL AUTO_INCREMENT,
  `username`      VARCHAR(32)  NOT NULL,
  `email`         VARCHAR(128) NOT NULL,
  `password_hash` VARCHAR(255) NOT NULL COMMENT 'bcrypt via password_hash()',
  `status`        TINYINT      NOT NULL DEFAULT 1 COMMENT '1=active 0=disabled',
  `last_login_at` DATETIME     NULL,
  `last_login_ip` VARCHAR(45)  NULL,
  `token_version` INT          NOT NULL DEFAULT 0 COMMENT 'increment to revoke all JWT sessions',
  `email_verified` TINYINT     NOT NULL DEFAULT 0 COMMENT '0=unverified 1=verified',
  `verify_token`  VARCHAR(64)  NULL,
  `verify_expires_at` DATETIME NULL,
  `reset_token`   VARCHAR(64)  NULL,
  `reset_expires_at` DATETIME  NULL,
  `created_at`    TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uniq_username` (`username`),
  UNIQUE KEY `uniq_email` (`email`),
  KEY `idx_status` (`status`),
  KEY `idx_verify_token` (`verify_token`),
  KEY `idx_reset_token` (`reset_token`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
