-- migrate: after schema.sql
-- add_require_registration_config.sql
-- 「仅注册使用」开关：开启后匿名访客无法通过 api.php 生成短链
-- （batch.php 本就要求登录，不受影响）。管理员在 系统配置 中把
-- shorturl.require_registration 设为 true / false 即可切换。
-- INSERT IGNORE：重跑迁移不会覆盖管理员已修改过的值。

INSERT IGNORE INTO `system_configs` (`config_key`, `config_value`, `value_type`, `description`, `is_public`)
VALUES (
  'shorturl.require_registration',
  'false',
  'bool',
  '仅注册使用：开启后匿名访客无法生成短链，需注册或登录',
  0
);
