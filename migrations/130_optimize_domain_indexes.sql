-- optimize_domain_indexes.sql
-- DWZ-H-02: indexes matching the actual domain picker and filtered list queries.
-- Idempotent: skips indexes that already exist.

SET @schema = DATABASE();

SET @idx_exists = (SELECT COUNT(*) FROM information_schema.STATISTICS
  WHERE TABLE_SCHEMA = @schema AND TABLE_NAME = 'domains' AND INDEX_NAME = 'idx_pick_domain');
SET @sql = IF(@idx_exists = 0,
  'ALTER TABLE `domains` ADD KEY `idx_pick_domain` (`status`, `link_count`, `priority`, `id`)',
  'SELECT 1');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @idx_exists = (SELECT COUNT(*) FROM information_schema.STATISTICS
  WHERE TABLE_SCHEMA = @schema AND TABLE_NAME = 'short_urls' AND INDEX_NAME = 'idx_domain_created');
SET @sql = IF(@idx_exists = 0,
  'ALTER TABLE `short_urls` ADD KEY `idx_domain_created` (`domain_id`, `created_at`)',
  'SELECT 1');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;
