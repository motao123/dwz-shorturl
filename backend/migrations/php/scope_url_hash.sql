-- migrate: after php/migrate_wjoy_log.sql
--
-- 本文件不再包含 `USE <库名>`：统一的迁移入口 (backend/cmd/migrate) 会把
-- 连接默认库切到「公共库」后整文件执行；跨库引用管理库时通过 {{ADMIN_DB}}
-- 占位符表达（执行时按当前配置注入实际库名）。
--
-- Migration: scope the short-URL dedup hash per owner (C 类改造)
--
-- 背景
--   wjoy_log.url_hash 原先直接存 MD5(longurl)，并配 UNIQUE KEY。这带来两个问题：
--     1. MD5 碰撞：两个不同的 URL 若 MD5 相同，第二个 URL 永远无法创建短链；
--     2. 全局去重：同一长链接在任意会员下都只能存在一条短链，无法按会员/有效期
--        分别管理，也无法为不同会员设置不同的访问密码。
--
-- 方案
--   url_hash 改为 MD5(CONCAT(longurl, 0x1F, scope_key))，唯一索引语义从
--   「URL 全局唯一」变为「同一 owner 作用域内唯一」：
--     scope_key = 'w:0'          匿名 / 历史数据（保留原全局语义，行为不变）
--     scope_key = 'm:<member_id>' 会员短链，同一会员内唯一，不同会员互不影响
--   Go 侧 (short_urls) 使用同一表达式，PHP 与 Go 两条路径对同一 (URL, owner)
--   得到同一个 url_hash。
--
-- 幂等性
--   本脚本可重复执行。未引入新列，只重写已有列的值并重建同名唯一索引。
--   重写语句均带 WHERE `url_hash` = MD5(裸 URL) 条件，第二次执行时已无匹配行；
--   索引的 DROP / ADD 都先查 information_schema 再执行，脚本重跑不会报
--   1061 Duplicate key name 或 1091 Can't DROP。
--
-- 兼容性
--   1) 脚本开头 SET NAMES utf8mb4，避免连接字符集影响哈希输入。
--   2) 跨库 JOIN（wjoy_log 在公共库、short_urls 在管理库）对 uid 两侧显式
--      COLLATE，兼容两库建库字符集不一致（如 general_ci vs unicode_ci）的
--      实际情况 —— 否则会报 ERROR 1267 Illegal mix of collations。
--   3) 不使用 DROP INDEX IF EXISTS（MySQL 8 不支持），改用本仓库其它迁移
--      统一的 information_schema + PREPARE 写法。
--
-- ⚠️ 本脚本会在执行中途重建唯一索引，一旦中断可能留下"半改造"状态
--    （部分行已是作用域哈希、部分仍是裸 MD5）。请勿手工 Ctrl-C；
--    若确实中断，直接重新执行本脚本即可（幂等），必要时先回滚备份。
--
-- ⚠️ 执行前请备份 wjoy_log 与 short_urls 表。重写期间请短暂停写
--    （或让流量走单一路径），避免新旧哈希混写。

-- ===========================================================================
-- 1. 公共库：wjoy_log
-- ===========================================================================
SET NAMES utf8mb4;

-- 库名在下方一律通过 @admin_schema 引用：CONCAT 只能拼接字符串字面量，
-- 因此先把注入的库名（迁移工具已把 {{ADMIN_DB}} 替换成配置里的库名）
-- 存进一个用户变量。
SET @admin_schema = '{{ADMIN_DB}}';

-- 1.1 历史/匿名行保持原全局语义：w:0
UPDATE `wjoy_log`
SET `url_hash` = MD5(CONCAT(`longurl`, 0x1F, 'w:0'))
WHERE `url_hash` = MD5(`longurl`);

-- 1.2 会员短链按会员隔离。wjoy_log 本身没有 member_id 列，通过短码与
--     short_urls.member_id 关联（两条路径共享同一 uid）。
--     ⚠️ 两个库的建库字符集可能不同（例如 wjoy_log=utf8mb4_general_ci、
--     short_urls=utf8mb4_unicode_ci），跨库 JOIN 的 uid 比较会触发
--     ERROR 1267 Illegal mix of collations。这里对两侧显式做 COLLATE 统一，
--     保证脚本在任意建库字符集组合下都能执行。
-- 跨库引用管理库统一走上面的 @admin_schema，无需人工替换库名。
SET @scope_sql = CONCAT(
  'UPDATE `wjoy_log` w JOIN `', @admin_schema, '`.`short_urls` s ',
  'ON s.uid COLLATE utf8mb4_unicode_ci = w.uid COLLATE utf8mb4_unicode_ci ',
  'SET w.`url_hash` = MD5(CONCAT(w.`longurl`, 0x1F, CONCAT(''m:'', s.`member_id`))) ',
  'WHERE s.`member_id` IS NOT NULL AND s.`member_id` > 0 ',
  'AND w.`url_hash` = MD5(CONCAT(w.`longurl`, 0x1F, ''w:0''))');
PREPARE scope_stmt FROM @scope_sql;
EXECUTE scope_stmt;
DEALLOCATE PREPARE scope_stmt;

-- 1.3 重建唯一索引（同名，语义改为作用域内唯一）
-- 幂等：只有索引存在时才 DROP（MySQL 8 不支持 DROP INDEX IF EXISTS，
-- 沿用本仓库其它迁移的 information_schema + PREPARE 写法保证跨版本可执行）。
SET @idx_exists = (SELECT COUNT(*) FROM information_schema.STATISTICS
  WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'wjoy_log' AND INDEX_NAME = 'uniq_hash');
SET @sql = IF(@idx_exists > 0, 'ALTER TABLE `wjoy_log` DROP INDEX `uniq_hash`', 'SELECT 1');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

-- ADD 同样做存在性判断，避免脚本重跑时因索引已建而报 1061 Duplicate key name。
SET @idx_exists = (SELECT COUNT(*) FROM information_schema.STATISTICS
  WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'wjoy_log' AND INDEX_NAME = 'uniq_hash');
SET @sql = IF(@idx_exists = 0, 'ALTER TABLE `wjoy_log` ADD UNIQUE KEY `uniq_hash` (`url_hash`)', 'SELECT 1');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

ALTER TABLE `wjoy_log`
  MODIFY COLUMN `url_hash` CHAR(32) NOT NULL
  COMMENT 'MD5(url + 0x1F + owner scope)';

-- ===========================================================================
-- 2. 管理库：short_urls
-- ===========================================================================
-- 本段作用于管理库，但连接的默认库是公共库（见文件头说明），
-- 因此所有语句都通过 @admin_schema 限定库名，不再使用 `USE`。

-- 2.1 会员行按会员作用域重写
SET @sql = CONCAT('UPDATE `', @admin_schema, '`.`short_urls` ',
  'SET `url_hash` = MD5(CONCAT(`long_url`, 0x1F, CONCAT(''m:'', `member_id`))) ',
  'WHERE `member_id` IS NOT NULL AND `member_id` > 0 AND `url_hash` = MD5(`long_url`)');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

-- 2.2 管理员/API Key 创建的行按创建者作用域重写
SET @sql = CONCAT('UPDATE `', @admin_schema, '`.`short_urls` ',
  'SET `url_hash` = MD5(CONCAT(`long_url`, 0x1F, ''w:0'')) ',
  'WHERE `member_id` IS NULL AND `url_hash` = MD5(`long_url`)');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @idx_exists = (SELECT COUNT(*) FROM information_schema.STATISTICS
  WHERE TABLE_SCHEMA = @admin_schema AND TABLE_NAME = 'short_urls' AND INDEX_NAME = 'uk_url_hash');
SET @sql = IF(@idx_exists > 0, CONCAT('ALTER TABLE `', @admin_schema, '`.`short_urls` DROP INDEX `uk_url_hash`'), 'SELECT 1');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @idx_exists = (SELECT COUNT(*) FROM information_schema.STATISTICS
  WHERE TABLE_SCHEMA = @admin_schema AND TABLE_NAME = 'short_urls' AND INDEX_NAME = 'uk_url_hash');
SET @sql = IF(@idx_exists = 0, CONCAT('ALTER TABLE `', @admin_schema, '`.`short_urls` ADD UNIQUE KEY `uk_url_hash` (`url_hash`)'), 'SELECT 1');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @sql = CONCAT('ALTER TABLE `', @admin_schema, '`.`short_urls` ',
  'MODIFY COLUMN `url_hash` CHAR(32) NOT NULL COMMENT ''MD5(url + 0x1F + owner scope) dedup''');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

-- ===========================================================================
-- 3. 校验（人工执行）
-- ===========================================================================
-- 应返回 0：不存在仍未作用域化的行
-- SELECT COUNT(*) FROM `wjoy_log` WHERE `url_hash` NOT LIKE CONCAT('_%\_%') ESCAPE '\\';
-- SELECT COUNT(*) FROM `short_urls` WHERE `url_hash` = MD5(`long_url`);
-- 索引应只保留一个 uk_url_hash / uniq_hash，且列注释已更新
-- SHOW INDEX FROM `short_urls` WHERE Key_name = 'uk_url_hash';
