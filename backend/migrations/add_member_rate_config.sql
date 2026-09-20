-- migrate: after add_ops_config_catalog.sql
-- add_member_rate_config.sql
-- 会员 API 限流配额（#8 / #37）。
--
-- 背景：`/member/api/**` 是唯一没有任何限流的外部可达分组 —— 管理员登录与匿名建链
-- 都有 IP 限流，而持有会员 token 的调用方可以无限循环调用建链/导出等接口。
-- Go 侧现在按「会员 token 维度 + IP 兜底」限流（middleware/member_rate_limit.go），
-- 配额值在此种入 system_configs，后台「系统配置」页可直接调整，无需重启。
--
--   member.api_rate_max     单会员 token 在窗口内允许的最大请求数（0=不限流）
--   member.api_rate_window  计数窗口秒数（1-3600）
--
-- INSERT IGNORE：重跑迁移不会覆盖管理员已修改过的值。

INSERT IGNORE INTO `system_configs` (`config_key`, `config_value`, `value_type`, `description`, `is_public`) VALUES
('member.api_rate_max',    '120', 'int', '会员 API 限流：单个会员 token 在窗口内允许的最大请求数（0=不限流）', 0),
('member.api_rate_window', '60',  'int', '会员 API 限流：计数窗口秒数（1-3600）', 0);
