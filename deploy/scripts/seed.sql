-- Seed data for dwz-admin
-- Initial roles, permissions, and super_admin user
-- Password for admin: admin123 (bcrypt hash)

USE dwz_admin;

-- Permissions
INSERT INTO permissions (resource, action, description) VALUES
('short_urls', 'read', '查看短链列表'),
('short_urls', 'create', '创建短链'),
('short_urls', 'update', '编辑短链'),
('short_urls', 'delete', '删除短链'),
('short_urls', 'export', '导出短链'),
('stats', 'read', '查看统计数据'),
('stats', 'export', '导出统计数据'),
('users', 'read', '查看用户列表'),
('users', 'create', '创建用户'),
('users', 'update', '编辑用户'),
('users', 'delete', '删除用户'),
('users', 'assign_roles', '分配角色'),
('roles', 'read', '查看角色列表'),
('roles', 'create', '创建角色'),
('roles', 'update', '编辑角色'),
('roles', 'delete', '删除角色'),
('configs', 'read', '查看系统配置'),
('configs', 'update', '修改系统配置'),
('audit', 'read', '查看审计日志'),
('api_keys', 'read', '查看API密钥'),
('api_keys', 'create', '创建API密钥'),
('api_keys', 'revoke', '吊销API密钥');

-- Roles
INSERT INTO roles (name, display_name, description, is_system) VALUES
('super_admin', '超级管理员', '拥有所有权限，不可删除', 1),
('admin', '管理员', '日常运营管理', 1),
('operator', '运营', '短链管理与统计查看', 1),
('viewer', '观察者', '只读权限', 1);

-- super_admin gets all permissions
INSERT INTO role_permissions (role_id, permission_id)
SELECT 1, id FROM permissions;

-- admin gets all except user delete and role delete
INSERT INTO role_permissions (role_id, permission_id)
SELECT 2, id FROM permissions
WHERE NOT (resource = 'users' AND action = 'delete')
  AND NOT (resource = 'roles' AND action = 'delete');

-- operator gets short_urls CRUD + stats read
INSERT INTO role_permissions (role_id, permission_id)
SELECT 3, id FROM permissions
WHERE (resource = 'short_urls')
   OR (resource = 'stats' AND action = 'read');

-- viewer gets all read permissions
INSERT INTO role_permissions (role_id, permission_id)
SELECT 4, id FROM permissions
WHERE action = 'read';

-- Default admin user (password: admin123)
-- bcrypt hash of "admin123" with cost 10
INSERT INTO users (username, email, password_hash, display_name, status) VALUES
('admin', 'admin@localhost', '$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy', '系统管理员', 1);

-- Assign super_admin role to admin user
INSERT INTO user_roles (user_id, role_id) VALUES (1, 1);

-- Default system configs
INSERT INTO system_configs (config_key, config_value, value_type, description, is_public) VALUES
('rate_limit.single_max', '20', 'int', '单条API每窗口最大请求数', 0),
('rate_limit.single_window', '60', 'int', '单条API限流窗口(秒)', 0),
('rate_limit.batch_max', '100', 'int', '批量API每窗口最大cost', 0),
('rate_limit.batch_window', '60', 'int', '批量API限流窗口(秒)', 0),
('short_url.max_custom_length', '8', 'int', '自定义短码最大长度', 0),
('short_url.min_custom_length', '6', 'int', '自定义短码最小长度', 0),
('short_url.allowed_expire_days', '[0,1,7,30,365]', 'json', '允许的有效期天数', 0),
('short_url.cache_ttl', '3600', 'int', 'Redis缓存TTL(秒)', 0),
('ssrf.enabled', 'true', 'bool', '是否启用SSRF防护', 0),
('stats.click_log_retention_days', '90', 'int', '点击日志保留天数', 0),
('site.name', '短网址管理', 'string', '站点名称', 1),
('site.public_base_url', 'https://1.xk7.cn', 'string', '短链公开基础URL', 1);
