-- 001_init.sql
-- Complete schema for dwz-admin short URL management system

CREATE TABLE IF NOT EXISTS `users` (
  `id`            BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  `username`      VARCHAR(32)  NOT NULL,
  `email`         VARCHAR(128) NOT NULL,
  `password_hash` VARCHAR(255) NOT NULL COMMENT 'bcrypt/argon2id',
  `display_name`  VARCHAR(64)  NULL,
  `avatar_url`    VARCHAR(512) NULL,
  `status`        TINYINT      NOT NULL DEFAULT 1 COMMENT '1=active 0=disabled',
  `last_login_at` DATETIME(3)  NULL,
  `last_login_ip` VARCHAR(45)  NULL,
  `created_at`    DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  `updated_at`    DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  `deleted_at`    DATETIME(3)  NULL,
  UNIQUE KEY `uk_username` (`username`),
  UNIQUE KEY `uk_email` (`email`),
  KEY `idx_status` (`status`),
  KEY `idx_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `roles` (
  `id`           BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  `name`         VARCHAR(32)  NOT NULL COMMENT 'unique identifier e.g. super_admin',
  `display_name` VARCHAR(64)  NOT NULL,
  `description`  VARCHAR(255) NULL,
  `is_system`    TINYINT      NOT NULL DEFAULT 0 COMMENT 'system built-in role cannot be deleted',
  `created_at`   DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  `updated_at`   DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  UNIQUE KEY `uk_name` (`name`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `permissions` (
  `id`          BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  `resource`    VARCHAR(64)  NOT NULL COMMENT 'resource identifier e.g. short_urls',
  `action`      VARCHAR(32)  NOT NULL COMMENT 'action e.g. create/read/update/delete',
  `description` VARCHAR(255) NULL,
  UNIQUE KEY `uk_resource_action` (`resource`, `action`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `role_permissions` (
  `role_id`       BIGINT UNSIGNED NOT NULL,
  `permission_id` BIGINT UNSIGNED NOT NULL,
  PRIMARY KEY (`role_id`, `permission_id`),
  KEY `idx_permission` (`permission_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `user_roles` (
  `user_id` BIGINT UNSIGNED NOT NULL,
  `role_id` BIGINT UNSIGNED NOT NULL,
  PRIMARY KEY (`user_id`, `role_id`),
  KEY `idx_role` (`role_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `short_urls` (
  `id`          BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  `uid`         VARCHAR(16)    NOT NULL COMMENT 'short code',
  `long_url`    TEXT           NOT NULL COMMENT 'target URL',
  `url_hash`    CHAR(32)       NOT NULL COMMENT 'MD5 dedup',
  `title`       VARCHAR(255)   NULL COMMENT 'user-defined title',
  `category_id` BIGINT UNSIGNED NULL COMMENT 'category ID',
  `clicks`      INT UNSIGNED   NOT NULL DEFAULT 0,
  `status`      TINYINT        NOT NULL DEFAULT 1 COMMENT '1=active 0=disabled 2=expired',
  `expire_at`   DATETIME(3)    NULL COMMENT 'NULL=permanent',
  `created_by`  BIGINT UNSIGNED NULL COMMENT 'creator user ID, NULL=anonymous',
  `source`      VARCHAR(16)    NOT NULL DEFAULT 'web' COMMENT 'web/api/batch/admin',
  `ip`          VARCHAR(45)    NULL COMMENT 'creator IP',
  `created_at`  DATETIME(3)    NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  `updated_at`  DATETIME(3)    NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  `deleted_at`  DATETIME(3)    NULL,
  UNIQUE KEY `uk_uid` (`uid`),
  UNIQUE KEY `uk_url_hash` (`url_hash`),
  KEY `idx_status_expire` (`status`, `expire_at`),
  KEY `idx_clicks` (`clicks` DESC),
  KEY `idx_created_at` (`created_at` DESC),
  KEY `idx_created_by` (`created_by`),
  KEY `idx_category` (`category_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `url_categories` (
  `id`         BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  `name`       VARCHAR(64)  NOT NULL,
  `color`      VARCHAR(7)   NULL COMMENT '#hex color',
  `sort_order` INT          NOT NULL DEFAULT 0,
  `created_by` BIGINT UNSIGNED NOT NULL,
  `created_at` DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  `updated_at` DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  `deleted_at` DATETIME(3)  NULL,
  KEY `idx_sort` (`sort_order`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `click_logs` (
  `id`           BIGINT UNSIGNED AUTO_INCREMENT,
  `short_url_id` BIGINT UNSIGNED NOT NULL,
  `ip`           VARCHAR(45)    NOT NULL,
  `user_agent`   VARCHAR(512)   NULL,
  `referer`      VARCHAR(512)   NULL,
  `country`      VARCHAR(2)     NULL COMMENT 'ISO 3166-1 alpha-2',
  `created_at`   DATETIME(3)    NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (`id`, `created_at`),
  KEY `idx_short_url` (`short_url_id`, `created_at`),
  KEY `idx_created_at` (`created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
PARTITION BY RANGE (TO_DAYS(`created_at`)) (
  PARTITION p202607 VALUES LESS THAN (TO_DAYS('2026-08-01')),
  PARTITION p202608 VALUES LESS THAN (TO_DAYS('2026-09-01')),
  PARTITION p202609 VALUES LESS THAN (TO_DAYS('2026-10-01')),
  PARTITION p202610 VALUES LESS THAN (TO_DAYS('2026-11-01')),
  PARTITION p202611 VALUES LESS THAN (TO_DAYS('2026-12-01')),
  PARTITION p202612 VALUES LESS THAN (TO_DAYS('2027-01-01')),
  PARTITION p_future VALUES LESS THAN MAXVALUE
);

CREATE TABLE IF NOT EXISTS `audit_logs` (
  `id`          BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  `user_id`     BIGINT UNSIGNED NULL COMMENT 'NULL=system operation',
  `action`      VARCHAR(64)    NOT NULL COMMENT 'e.g. short_url.create',
  `resource`    VARCHAR(64)    NULL,
  `resource_id` VARCHAR(64)    NULL,
  `detail`      JSON           NULL COMMENT 'operation detail snapshot',
  `ip`          VARCHAR(45)    NOT NULL,
  `user_agent`  VARCHAR(255)   NULL,
  `created_at`  DATETIME(3)    NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  KEY `idx_user` (`user_id`, `created_at` DESC),
  KEY `idx_action` (`action`, `created_at` DESC),
  KEY `idx_created_at` (`created_at` DESC)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `system_configs` (
  `id`           BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  `config_key`   VARCHAR(64)    NOT NULL,
  `config_value` TEXT           NOT NULL,
  `value_type`   VARCHAR(16)    NOT NULL DEFAULT 'string' COMMENT 'string/int/bool/json',
  `description`  VARCHAR(255)   NULL,
  `is_public`    TINYINT        NOT NULL DEFAULT 0 COMMENT 'whether readable by frontend',
  `updated_by`   BIGINT UNSIGNED NULL,
  `updated_at`   DATETIME(3)    NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  UNIQUE KEY `uk_key` (`config_key`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `api_keys` (
  `id`           BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  `user_id`      BIGINT UNSIGNED NOT NULL,
  `name`         VARCHAR(64)    NOT NULL COMMENT 'key purpose description',
  `key_prefix`   VARCHAR(8)     NOT NULL COMMENT 'first 8 chars plaintext for display',
  `key_hash`     CHAR(64)       NOT NULL COMMENT 'SHA-256 full hash',
  `permissions`  JSON           NULL COMMENT 'allowed API scope',
  `rate_limit`   INT            NOT NULL DEFAULT 100 COMMENT 'per-minute quota',
  `last_used_at` DATETIME(3)    NULL,
  `expires_at`   DATETIME(3)    NULL,
  `status`       TINYINT        NOT NULL DEFAULT 1 COMMENT '1=active 0=revoked',
  `created_at`   DATETIME(3)    NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  `deleted_at`   DATETIME(3)    NULL,
  UNIQUE KEY `uk_key_hash` (`key_hash`),
  KEY `idx_user` (`user_id`),
  KEY `idx_prefix` (`key_prefix`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Seed default super_admin role
INSERT INTO `roles` (`name`, `display_name`, `description`, `is_system`) VALUES
('super_admin', 'Super Administrator', 'Full system access', 1),
('admin', 'Administrator', 'Daily operations management', 0),
('operator', 'Operator', 'Content operations', 0),
('viewer', 'Viewer', 'Read-only access', 0)
ON DUPLICATE KEY UPDATE `display_name` = VALUES(`display_name`);

-- Seed permissions
INSERT INTO `permissions` (`resource`, `action`, `description`) VALUES
('short_urls', 'create', 'Create short URLs'),
('short_urls', 'read', 'View short URLs'),
('short_urls', 'update', 'Edit short URLs'),
('short_urls', 'delete', 'Delete short URLs'),
('short_urls', 'export', 'Export short URLs'),
('stats', 'read', 'View statistics'),
('stats', 'export', 'Export statistics'),
('users', 'create', 'Create users'),
('users', 'read', 'View users'),
('users', 'update', 'Edit users'),
('users', 'delete', 'Delete users'),
('users', 'assign_roles', 'Assign roles to users'),
('roles', 'create', 'Create roles'),
('roles', 'read', 'View roles'),
('roles', 'update', 'Edit roles'),
('roles', 'delete', 'Delete roles'),
('configs', 'read', 'View system configs'),
('configs', 'update', 'Edit system configs'),
('audit', 'read', 'View audit logs'),
('api_keys', 'create', 'Create API keys'),
('api_keys', 'read', 'View API keys'),
('api_keys', 'revoke', 'Revoke API keys')
ON DUPLICATE KEY UPDATE `description` = VALUES(`description`);

-- Grant all permissions to super_admin (role_id = 1)
INSERT INTO `role_permissions` (`role_id`, `permission_id`)
SELECT 1, `id` FROM `permissions`
ON DUPLICATE KEY UPDATE `role_id` = `role_id`;

-- Seed default admin user (password: admin123)
INSERT INTO `users` (`username`, `email`, `password_hash`, `display_name`, `status`) VALUES
('admin', 'admin@example.com', '$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy', 'System Admin', 1)
ON DUPLICATE KEY UPDATE `username` = `username`;

-- Assign super_admin role to admin user
INSERT INTO `user_roles` (`user_id`, `role_id`) VALUES (1, 1)
ON DUPLICATE KEY UPDATE `user_id` = `user_id`;
