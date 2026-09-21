-- migrate: after add_member_link_quota.sql
-- drop_unused_stats_export_permission.sql
--
-- 删掉 `stats` / `export` 这条权限（#68）。
--
-- 原因：权限表里有这一行，但 backend/internal/router 里没有任何路由校验它——stats 组
-- 只有 read/update，也不存在统计导出端点。后果不是"少个功能"，而是**权限矩阵里多一个
-- 勾了不起作用的开关**：管理员给某角色勾上"导出统计"会以为自己授予（或收回）了能力，
-- 而实际行为完全不变。控制面说谎比缺一个权限位更糟，所以选择删除而不是补端点——
-- 真要做统计导出时，应连同端点一起加回来，那时这条权限才有意义。
--
-- 幂等：两条 DELETE 都是"没有匹配行则影响 0 行"，可重复执行；先删授权再删权限本体。
-- 只针对这一条 (resource, action)，不碰其他权限，也不删任何角色或账号。

DELETE rp
FROM `role_permissions` rp
JOIN `permissions` p ON p.`id` = rp.`permission_id`
WHERE p.`resource` = 'stats'
  AND p.`action` = 'export';

DELETE FROM `permissions`
WHERE `resource` = 'stats'
  AND `action` = 'export';
