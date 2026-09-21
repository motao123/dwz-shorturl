-- migrate: after add_api_key_permission_scopes.sql
-- add_member_link_quota.sql
-- 每会员短链数量上限（#41）。
--
-- 背景：会员 API 有速率限流（member.api_rate_max，第 7 批），但**速率有限不等于总量有限**：
-- 一个注册即可用、且不要求验证邮箱的账号，只要把请求摊到足够长的时间窗里，就能无限
-- 累积短链——第 7 批收口的是"一秒能发多少请求"，这条收口的是"一个人能占多少库"。
-- 同时会员建链路径此前完全不校验 domain_id（后台管理路径校验），任意整数都能挂到未启用
-- 的域名上参与 link_count 记账。
--
--   member.max_links  单个会员可持有的有效短链上限（0=不限）
--
-- 软删除行不计入（删除即释放额度）；已过期/已停用仍占位，因为它们能被续期复活。
-- INSERT IGNORE：重跑迁移不覆盖管理员已改过的值。

INSERT IGNORE INTO `system_configs` (`config_key`, `config_value`, `value_type`, `description`, `is_public`) VALUES
('member.max_links', '1000', 'int', '单个会员可持有的有效短链上限（0=不限）', 0);
