-- migrate: after optimize_domain_indexes.sql
-- #22：为内置角色授予可用的最小权限矩阵。
--
-- 此前 schema.sql 只给 super_admin（role 1）授予全量权限，唯一例外是 add_domains.sql
-- 给 admin 三项、add_stats_update_permission.sql 给 admin 一项。结果是新装实例里
-- admin / operator / viewer 三个角色**一条权限都没有**：建出来的受限账号登录后每个
-- 接口都 403，管理台手册里写的建号流程因此根本走不通——运维只能全都用超管，
-- 而 #2/#3 修掉的正是超管泛滥带来的提权面。
--
-- 设计原则（有意保守，逐条给理由）：
--   * 不给任何非超管角色 roles.create/update/delete —— 改角色与权限就是提权通道（#3）。
--   * 不给 admin configs.update —— 参数目录里含 SMTP 口令与各类密钥，且这些开关现在
--     真的会改变全站行为（#4），改错的爆炸半径不止一次请求。
--   * 不给 admin users.delete、不给 operator audit.delete —— 删号有级联后果（#39），
--     删举报记录等于销毁证据。
--   * 只 INSERT 不 DELETE：本文件必须可重复执行，且不能把运维手工加过的权限抹掉。
--
-- 角色用 name 解析而非写死 role_id，避免存量实例里角色表被人工调整过后授错对象。

-- 注意：这里刻意**不授** stats.export。当时的事实是：权限表里有这一行，但 router 里没有任何
-- 路由校验它（stats 组只有 read/update），授出去是一条不起作用的可勾选权限（#68）。
-- 该不一致已由 drop_unused_stats_export_permission.sql 收口——那条权限被删掉了，本文件
-- 的排除条件因此变成"天然不匹配"，保留它是为了在旧实例上重复执行时也不要去授予它。

-- admin（日常运营负责人）---------------------------------------------------------
INSERT INTO `role_permissions` (`role_id`, `permission_id`)
SELECT r.`id`, p.`id`
FROM `roles` r
JOIN `permissions` p ON (
     (p.`resource` = 'short_urls' AND p.`action` IN ('create', 'read', 'update', 'delete', 'export'))
  OR (p.`resource` = 'stats'      AND p.`action` IN ('read', 'update'))
  OR (p.`resource` = 'users'      AND p.`action` IN ('create', 'read', 'update', 'assign_roles'))
  OR (p.`resource` = 'roles'      AND p.`action` IN ('read'))
  OR (p.`resource` = 'configs'    AND p.`action` IN ('read'))
  OR (p.`resource` = 'audit'      AND p.`action` IN ('read', 'update'))
  OR (p.`resource` = 'api_keys'   AND p.`action` IN ('create', 'read', 'revoke'))
  OR (p.`resource` = 'domains'    AND p.`action` IN ('read', 'create', 'update'))
)
WHERE r.`name` = 'admin'
ON DUPLICATE KEY UPDATE `role_id` = `role_id`;

-- operator（内容运营 / 举报处理）--------------------------------------------------
INSERT INTO `role_permissions` (`role_id`, `permission_id`)
SELECT r.`id`, p.`id`
FROM `roles` r
JOIN `permissions` p ON (
     (p.`resource` = 'short_urls' AND p.`action` IN ('read', 'update', 'delete'))
  OR (p.`resource` = 'stats'      AND p.`action` IN ('read'))
  OR (p.`resource` = 'audit'      AND p.`action` IN ('read', 'update'))
  OR (p.`resource` = 'domains'    AND p.`action` IN ('read'))
)
WHERE r.`name` = 'operator'
ON DUPLICATE KEY UPDATE `role_id` = `role_id`;

-- viewer（审计 / 值班只读）--------------------------------------------------------
INSERT INTO `role_permissions` (`role_id`, `permission_id`)
SELECT r.`id`, p.`id`
FROM `roles` r
JOIN `permissions` p ON (
     (p.`resource` = 'short_urls' AND p.`action` IN ('read'))
  OR (p.`resource` = 'stats'      AND p.`action` IN ('read'))
  OR (p.`resource` = 'users'      AND p.`action` IN ('read'))
  OR (p.`resource` = 'roles'      AND p.`action` IN ('read'))
  OR (p.`resource` = 'configs'    AND p.`action` IN ('read'))
  OR (p.`resource` = 'audit'      AND p.`action` IN ('read'))
  OR (p.`resource` = 'api_keys'   AND p.`action` IN ('read'))
  OR (p.`resource` = 'domains'    AND p.`action` IN ('read'))
)
WHERE r.`name` = 'viewer'
ON DUPLICATE KEY UPDATE `role_id` = `role_id`;
