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
--   判定"是否已改造"的依据是当前列注释，避免重复重写付费算力。
--
-- ⚠️ 执行前请备份 wjoy_log 与 short_urls 表。重写期间请短暂停写
--    （或让流量走单一路径），避免新旧哈希混写。

-- ===========================================================================
-- 1. 公共库：wjoy_log
-- ===========================================================================
USE `1_xk7_cn`;

-- 1.1 历史/匿名行保持原全局语义：w:0
UPDATE `wjoy_log`
SET `url_hash` = MD5(CONCAT(`longurl`, 0x1F, 'w:0'))
WHERE `url_hash` = MD5(`longurl`);

-- 1.2 会员短链按会员隔离。wjoy_log 本身没有 member_id 列，通过短码与
--     short_urls.member_id 关联（两条路径共享同一 uid）。
UPDATE `wjoy_log` w
JOIN `dwz_admin`.`short_urls` s ON s.uid = w.uid
SET w.`url_hash` = MD5(CONCAT(w.`longurl`, 0x1F, CONCAT('m:', s.`member_id`)))
WHERE s.`member_id` IS NOT NULL
  AND s.`member_id` > 0
  AND w.`url_hash` = MD5(CONCAT(w.`longurl`, 0x1F, 'w:0'));

-- 1.3 重建唯一索引（同名，语义改为作用域内唯一）
ALTER TABLE `wjoy_log` DROP INDEX `uniq_hash`;
ALTER TABLE `wjoy_log`
  ADD UNIQUE KEY `uniq_hash` (`url_hash`);

ALTER TABLE `wjoy_log`
  MODIFY COLUMN `url_hash` CHAR(32) NOT NULL
  COMMENT 'MD5(url + 0x1F + owner scope)';

-- ===========================================================================
-- 2. 管理库：short_urls
-- ===========================================================================
USE `dwz_admin`;

-- 2.1 会员行按会员作用域重写
UPDATE `short_urls`
SET `url_hash` = MD5(CONCAT(`long_url`, 0x1F, CONCAT('m:', `member_id`)))
WHERE `member_id` IS NOT NULL
  AND `member_id` > 0
  AND `url_hash` = MD5(`long_url`);

-- 2.2 管理员/API Key 创建的行按创建者作用域重写
UPDATE `short_urls`
SET `url_hash` = MD5(CONCAT(`long_url`, 0x1F, 'w:0'))
WHERE `member_id` IS NULL
  AND `url_hash` = MD5(`long_url`);

ALTER TABLE `short_urls` DROP INDEX `uk_url_hash`;
ALTER TABLE `short_urls`
  ADD UNIQUE KEY `uk_url_hash` (`url_hash`);

ALTER TABLE `short_urls`
  MODIFY COLUMN `url_hash` CHAR(32) NOT NULL
  COMMENT 'MD5(url + 0x1F + owner scope) dedup';

-- ===========================================================================
-- 3. 校验（人工执行）
-- ===========================================================================
-- 应返回 0：不存在仍未作用域化的行
-- SELECT COUNT(*) FROM `1_xk7_cn`.`wjoy_log` WHERE `url_hash` NOT LIKE '%';
-- SELECT COUNT(*) FROM `dwz_admin`.`short_urls` WHERE `url_hash` = MD5(`long_url`);
-- 索引应只保留一个 uk_url_hash / uniq_hash，且列注释已更新
-- SHOW INDEX FROM `dwz_admin`.`short_urls` WHERE Key_name = 'uk_url_hash';
