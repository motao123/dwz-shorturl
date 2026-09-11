-- public_schema.sql
-- Schema for the *public* frontend database, i.e. the one holding `wjoy_log`.
--
-- The PHP frontend writes its short links into `wjoy_log`; the Go admin backend
-- writes into `short_urls`. In single-database deployments both live in the same
-- schema; in split deployments this file is what makes the public side
-- self-provisioning instead of requiring a hand-written CREATE TABLE.
--
-- The scoped url_hash contract (MD5(longurl + 0x1F + scope_key)) must match
-- includes/function.php:url_scope_hash() and backend/internal/service/short_url.go.
-- Historical rows are rewritten by migrations/scope_url_hash.sql, which is run
-- separately by ops/one_click_migrate.sh because it crosses both databases.

CREATE TABLE IF NOT EXISTS `wjoy_log` (
  `Id`            INT UNSIGNED NOT NULL AUTO_INCREMENT,
  `uid`           VARCHAR(16)  NOT NULL COMMENT 'short code',
  `longurl`       TEXT         NOT NULL COMMENT 'target URL',
  `url_hash`      CHAR(32)     NOT NULL COMMENT 'MD5(url + 0x1F + owner scope)',
  `clicks`        INT UNSIGNED NOT NULL DEFAULT 0,
  `created_at`    TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `expire_at`     DATETIME     NULL,
  `status`        TINYINT      NOT NULL DEFAULT 1 COMMENT '1=active 0=disabled',
  `password_hash` VARCHAR(255) NULL COMMENT 'bcrypt; non-NULL = password protected',
  PRIMARY KEY (`Id`),
  UNIQUE KEY `uniq_uid`  (`uid`),
  UNIQUE KEY `uniq_hash` (`url_hash`),
  KEY `idx_created_at` (`created_at`),
  KEY `idx_clicks`     (`clicks`),
  KEY `idx_status`     (`status`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
