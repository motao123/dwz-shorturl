-- Public frontend database initialisation for Docker.
-- Creates the public schema and the tables the admin backend manages via
-- public_db (members, violation_reviews) plus the PHP-facing wjoy_log.
CREATE DATABASE IF NOT EXISTS `1_xk7_cn` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;

-- Grant the app user (default: dwz) access to the public schema. This runs as
-- root during docker-entrypoint-initdb.d. Keep in sync with MYSQL_USER.
GRANT ALL PRIVILEGES ON `1_xk7_cn`.* TO 'dwz'@'%';
FLUSH PRIVILEGES;

USE `1_xk7_cn`;

-- Public short links (PHP-era write path)
CREATE TABLE IF NOT EXISTS `wjoy_log` (
  `Id` int unsigned NOT NULL AUTO_INCREMENT,
  `uid` varchar(16) NOT NULL,
  `longurl` text NOT NULL,
  `url_hash` char(32) NOT NULL,
  `clicks` int unsigned NOT NULL DEFAULT 0,
  `created_at` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `expire_at` datetime DEFAULT NULL,
  `status` tinyint NOT NULL DEFAULT 1 COMMENT '1=active 0=disabled',
  `password_hash` varchar(255) DEFAULT NULL COMMENT 'bcrypt; non-NULL = password protected',
  PRIMARY KEY (`Id`),
  UNIQUE KEY `uniq_uid` (`uid`),
  UNIQUE KEY `uniq_hash` (`url_hash`),
  KEY `idx_created_at` (`created_at`),
  KEY `idx_clicks` (`clicks`),
  KEY `idx_status` (`status`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Public-facing member accounts
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

-- Violation review log (blocked URLs awaiting manual review)
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