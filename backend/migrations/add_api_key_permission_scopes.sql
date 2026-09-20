-- migrate: after add_member_rate_config.sql
-- add_api_key_permission_scopes.sql
-- API Key 权限范围回填（#42）。
--
-- 背景：api_keys.permissions 这一列从建表起就存在，后台 UI 也会提交它、创建
-- 接口也会写入它 —— 但**没有任何代码读过它**。于是「权限」成了一个只存不用的
-- 摆设：一把 permissions=[] 的密钥照样能调用 /public/api 下的每个接口。
--
-- 现在 Go 侧按路由声明 scope 并在 ScopeApiKey 中间件里校验（见
-- middleware/apikey.go 与 router.go）。本次改动只影响「今后新建的密钥」，
-- 存量密钥如果 permissions 为空/NULL 会在校验时被判为空集而**全部 403**。
--
-- ⚠️ 为什么是「授予最小可用范围」而不是「保持为空（继续拒绝）」：
--   空权限的密钥在旧代码下是**能正常工作**的，直接拒绝等于一次静默的行为变更，
--   会让正在用这些密钥的调用方突然全部失败，且失败原因对运维不直观。
--   本迁移把存量空权限密钥显式回填成公开 API 当前实际提供的唯一能力
--   （创建短链），使其行为与升级前一致；想要更严的运维可以随后在后台
--   逐把收窄或直接吊销。
--
-- 幂等：只动 permissions 为 NULL / 空串 / 'null' / '[]' 的行，重复执行零影响。
-- 不动任何已有权限值的行（避免把运维手工配置覆盖掉）。

UPDATE `api_keys`
SET `permissions` = JSON_ARRAY('short_urls.create', 'short_urls.batch')
WHERE `permissions` IS NULL
   OR JSON_TYPE(`permissions`) = 'NULL'
   OR JSON_LENGTH(`permissions`) = 0;
