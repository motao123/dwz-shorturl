-- add_domains.sql
-- Domain pool management feature: adds the domains table, domain_id on short_urls,
-- and seeds the domains.* permissions with grants to super_admin (role 1) and admin (role 2).

-- 1. domains table -----------------------------------------------------------
CREATE TABLE IF NOT EXISTS `domains` (
  `id`          BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  `domain`      VARCHAR(128) NOT NULL COMMENT 'domain name e.g. 1.xk7.cn',
  `scheme`      VARCHAR(8)   NOT NULL DEFAULT 'https' COMMENT 'http or https',
  `name`        VARCHAR(64)  NULL COMMENT 'display name',
  `project`     VARCHAR(64)  NULL COMMENT 'owning project / group',
  `status`      TINYINT      NOT NULL DEFAULT 1 COMMENT '1=active 0=disabled',
  `priority`    INT          NOT NULL DEFAULT 100 COMMENT 'lower = higher priority',
  `dns_status`  VARCHAR(16)  NOT NULL DEFAULT 'pending' COMMENT 'pending/ok/fail',
  `ssl_status`  VARCHAR(16)  NOT NULL DEFAULT 'pending' COMMENT 'pending/ok/fail',
  `link_count`  INT UNSIGNED NOT NULL DEFAULT 0 COMMENT 'number of short links using this domain',
  `created_at`  DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  `updated_at`  DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  `deleted_at`  DATETIME(3)  NULL,
  UNIQUE KEY `uk_domain` (`domain`),
  KEY `idx_status_priority` (`status`, `priority`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- 2. domain_id on short_urls -------------------------------------------------
ALTER TABLE `short_urls`
  ADD COLUMN `domain_id` BIGINT UNSIGNED NULL COMMENT 'domain pool entry this link belongs to' AFTER `category_id`;

ALTER TABLE `short_urls`
  ADD KEY `idx_domain` (`domain_id`);

-- 3. permissions -------------------------------------------------------------
INSERT INTO `permissions` (`resource`, `action`, `description`) VALUES
('domains', 'read',   'View domains'),
('domains', 'create', 'Add domains'),
('domains', 'update', 'Edit domains'),
('domains', 'delete', 'Delete domains')
ON DUPLICATE KEY UPDATE `description` = VALUES(`description`);

-- Grant all domains permissions to super_admin (role_id = 1)
INSERT INTO `role_permissions` (`role_id`, `permission_id`)
SELECT 1, `id` FROM `permissions` WHERE `resource` = 'domains'
ON DUPLICATE KEY UPDATE `role_id` = `role_id`;

-- Grant domains read/create/update to admin (role_id = 2), exclude delete
INSERT INTO `role_permissions` (`role_id`, `permission_id`)
SELECT 2, `id` FROM `permissions`
WHERE `resource` = 'domains' AND `action` IN ('read', 'create', 'update')
ON DUPLICATE KEY UPDATE `role_id` = `role_id`;
