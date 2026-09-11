-- migrate: after php/scope_url_hash.sql
--
-- ⚠️ 历史文件，仅为已应用过的旧安装保留，不要修改其内容。
--
-- 早期版本把这个脚本放在 backend/migrations/single_db_scope_fixups.sql（已在
-- 迁移顺序之外，会被判为 unclassified 并最后执行）。统一到 php/ 目录后，它
-- 与其他库内脚本一样在「公共库」连接上执行，文件内的语句因此全部改写为
-- 不依赖当前默认库、由迁移工具注入库名的方式。
--
-- 作用：单库部署（wjoy_log 与 short_urls 同库）时，把跨库 JOIN 变成同库 JOIN，
-- 既避免两个库的 COLLATE 不一致，也让 CREATE OR REPLACE VIEW 落在正确的库上。
-- 在分库部署下本脚本等价于原逻辑，行为不变。
--
-- 占位符 {{ADMIN_DB}} / {{PUBLIC_DB}} 由 backend/cmd/migrate 在执行前按配置替换。

-- 1. wjoy_compat 视图：分库时建在公共库，单库时建在唯一那个库。
--    视图内容指向管理库/唯一库的 short_urls。
SET @compat_schema = IF('{{ADMIN_DB}}' = '{{PUBLIC_DB}}', '{{PUBLIC_DB}}', DATABASE());
SET @sql = CONCAT(
  'CREATE OR REPLACE VIEW `', @compat_schema, '`.`wjoy_compat` AS ',
  'SELECT uid, long_url AS longurl, clicks, created_at, expire_at ',
  'FROM `{{ADMIN_DB}}`.`short_urls` WHERE deleted_at IS NULL');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

-- 2. 会员作用域哈希重写：同库时 wjoy_log ↔ short_urls 直接 JOIN，
--    不再需要跨库 COLLATE 归一（表在同一 schema，排序规则天然一致）。
--    分库部署仍走原来的跨库 JOIN + 显式 COLLATE 路径。
SET @sql = IF('{{ADMIN_DB}}' = '{{PUBLIC_DB}}',
  CONCAT('UPDATE `', DATABASE(), '`.`wjoy_log` w JOIN `', DATABASE(), '`.`short_urls` s ',
         'ON s.uid = w.uid ',
         'SET w.`url_hash` = MD5(CONCAT(w.`longurl`, 0x1F, CONCAT(''m:'', s.`member_id`))) ',
         'WHERE s.`member_id` IS NOT NULL AND s.`member_id` > 0 ',
         'AND w.`url_hash` = MD5(CONCAT(w.`longurl`, 0x1F, ''w:0''))'),
  CONCAT('UPDATE `', DATABASE(), '`.`wjoy_log` w JOIN `{{ADMIN_DB}}`.`short_urls` s ',
         'ON s.uid COLLATE utf8mb4_unicode_ci = w.uid COLLATE utf8mb4_unicode_ci ',
         'SET w.`url_hash` = MD5(CONCAT(w.`longurl`, 0x1F, CONCAT(''m:'', s.`member_id`))) ',
         'WHERE s.`member_id` IS NOT NULL AND s.`member_id` > 0 ',
         'AND w.`url_hash` = MD5(CONCAT(w.`longurl`, 0x1F, ''w:0''))'));
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;
