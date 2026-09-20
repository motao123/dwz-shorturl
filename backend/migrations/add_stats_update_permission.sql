-- migrate: after schema.sql
-- add_stats_update_permission.sql
--
-- router.go guards the maintenance actions with RequirePermission("stats","update")
-- (/monitor/run-task, /monitor/ensure-partitions), but that permission point was
-- never seeded: it is absent from schema.sql's permissions table and from
-- deploy/scripts/seed.sql. Consequences (#23):
--   * only super_admin could run maintenance, because the name short-circuits the
--     permission lookup;
--   * the admin UI's permission tree had no such checkbox, so an operator could
--     not be granted the action even deliberately.
--
-- Idempotent: INSERT ... ON DUPLICATE KEY UPDATE, plus INSERT ... SELECT for the
-- role grant, matching the style of add_domains.sql.
INSERT INTO `permissions` (`resource`, `action`, `description`) VALUES
('stats', 'update', '执行运维任务（清理/补分区）')
ON DUPLICATE KEY UPDATE `description` = VALUES(`description`);

-- Grant it to super_admin (role 1) and admin (role 2). It is a destructive
-- action (it deletes old data / runs DDL), so it is deliberately NOT granted to
-- operator/viewer.
INSERT INTO `role_permissions` (`role_id`, `permission_id`)
SELECT 1, `id` FROM `permissions` WHERE `resource` = 'stats' AND `action` = 'update'
ON DUPLICATE KEY UPDATE `role_id` = `role_id`;

INSERT INTO `role_permissions` (`role_id`, `permission_id`)
SELECT 2, `id` FROM `permissions` WHERE `resource` = 'stats' AND `action` = 'update'
ON DUPLICATE KEY UPDATE `role_id` = `role_id`;
