-- add_ops_config_catalog.sql
-- 系统配置目录：把运营常用的可调参数种进 system_configs，
-- 后台「系统配置」页据此渲染开关/下拉/输入控件，且每个键都有真实
-- 的代码执行点（PHP 前台运行时读取，读取失败回落到同名默认值）。
-- INSERT IGNORE：重跑迁移不会覆盖管理员已修改过的值。
--
--   shorturl.default_expire_days   未显式传有效期时的默认值（0=永久）
--   shorturl.allow_custom_code     自定义短码总开关
--   shorturl.anon_rate_max         匿名建链限流：窗口内最大次数
--   shorturl.anon_rate_window      匿名建链限流：窗口秒数
--   batch.max_urls                 单次批量生成的 URL 上限（clamp 1..1000）
--   member.batch_requires_verified 批量生成是否要求邮箱已验证

INSERT IGNORE INTO `system_configs` (`config_key`, `config_value`, `value_type`, `description`, `is_public`) VALUES
('shorturl.default_expire_days',   '0',    'int',  '新建短链未指定有效期时的默认值：0 永久 / 1 / 7 / 30 / 365 天', 0),
('shorturl.allow_custom_code',     'true', 'bool', '自定义短码总开关：关闭后所有自定义短码请求被拒绝', 0),
('shorturl.anon_rate_max',         '20',   'int',  '匿名建链限流：单 IP 在窗口内允许的最大次数', 0),
('shorturl.anon_rate_window',      '60',   'int',  '匿名建链限流：计数窗口秒数', 0),
('batch.max_urls',                 '100',  'int',  '单次批量生成的 URL 条数上限（1-1000）', 0),
('member.batch_requires_verified', 'true', 'bool', '批量生成是否要求会员邮箱已验证', 0);
