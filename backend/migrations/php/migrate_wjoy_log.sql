-- migrate: after php/add_webhook_queue.sql
--
-- 由统一迁移入口执行（连接默认库 = 公共库），不再包含 `USE <库名>`。
-- Migration 002: Migrate legacy wjoy_log data into short_urls
-- Idempotent: uses WHERE NOT EXISTS to avoid duplicates on re-run.
-- This bridges the PHP era (table wjoy_log) and the Go admin backend (table short_urls).
--
-- 本文件同时读写两个库：源表 wjoy_log 在公共库（当前默认库），目标表
-- short_urls / 兼容视图在管理库。管理库名通过 {{ADMIN_DB}} 占位符表达，
-- 由迁移工具按当前配置注入，因此文件里没有任何硬编码库名。
--
-- 位置：必须排在 php/scope_url_hash.sql 之前。本文件把老 wjoy_log 行搬进
-- short_urls（此时哈希仍是裸 MD5），随后 scope_url_hash.sql 统一把两库的
-- 历史行重写为作用域哈希；反过来 short_urls 会残留未作用域化的行。

-- 库名在下方一律通过 @admin_schema 引用：CONCAT 只能拼接字符串字面量，
-- 所以先把注入的库名（迁移工具已把 {{ADMIN_DB}} 替换成配置里的库名）
-- 存进一个用户变量。
SET @admin_schema = '{{ADMIN_DB}}';

-- 1. Copy legacy rows that don't already exist in short_urls
SET @sql = CONCAT(
  'INSERT IGNORE INTO `', @admin_schema, '`.`short_urls` ',
  '(uid, long_url, url_hash, clicks, status, source, created_at, updated_at, expire_at) ',
  'SELECT w.uid, w.longurl, MD5(w.longurl), COALESCE(w.clicks, 0), 1, ''legacy'', ',
  'COALESCE(w.created_at, NOW(3)), COALESCE(w.created_at, NOW(3)), w.expire_at ',
  'FROM `wjoy_log` w ',
  'WHERE w.uid IS NOT NULL AND w.uid != '''' ',
  -- 两个库的建库字符集可能不同（general_ci vs unicode_ci），跨库比较必须
  -- 显式统一 COLLATE，否则报 ERROR 1267 Illegal mix of collations。
  'AND w.uid COLLATE utf8mb4_unicode_ci NOT IN ',
  '(SELECT uid COLLATE utf8mb4_unicode_ci FROM `', @admin_schema, '`.`short_urls`)');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

-- 2. If wjoy_log has base64-encoded longurl, decode and fix long_url (MySQL 8 only)
--    This is a no-op for URLs already in plain text.
SET @sql = CONCAT(
  'UPDATE `', @admin_schema, '`.`short_urls` s JOIN `wjoy_log` w ',
  'ON s.uid COLLATE utf8mb4_unicode_ci = w.uid COLLATE utf8mb4_unicode_ci ',
  'SET s.long_url = FROM_BASE64(w.longurl), s.url_hash = MD5(FROM_BASE64(w.longurl)) ',
  'WHERE s.source = ''legacy'' AND w.longurl REGEXP ''^[A-Za-z0-9+/=]+$'' ',
  'AND FROM_BASE64(w.longurl) REGEXP ''^https?://'' AND CHAR_LENGTH(w.longurl) > 40');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

-- 3. Create a compatibility view so PHP code can SELECT from short_urls using old
--    column names, allowing a gradual cutover without breaking existing queries.
--    The view lives in the public database so the PHP frontend can query it
--    without cross-schema privileges.
SET @sql = CONCAT(
  'CREATE OR REPLACE VIEW `wjoy_compat` AS ',
  'SELECT uid, long_url AS longurl, clicks, created_at, expire_at ',
  'FROM `', @admin_schema, '`.`short_urls` WHERE deleted_at IS NULL');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

-- Verification (run manually to confirm):
-- SELECT COUNT(*) AS migrated_count FROM short_urls WHERE source = 'legacy';
-- SELECT COUNT(*) AS total_in_wjoy_log FROM wjoy_log;
