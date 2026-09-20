-- migrate: after add_domains.sql
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

-- idx_clicks backs the "Top N links" query (`ORDER BY clicks DESC LIMIT n`).
-- schema.sql already declares it, so a fresh install has it; this guard exists
-- for installs created before the index was added, where the query falls back to
-- a full scan + filesort that grows with the table.
--
-- Verified on MariaDB 10.11 with 20k rows: with the index the optimizer picks a
-- reverse index scan ("type: index", key=idx_clicks, no "Using filesort"). On a
-- near-empty table it still reports "Using filesort" because a full scan is
-- genuinely cheaper there — that is expected, not a missing index.
SET @idx_exists = (SELECT COUNT(*) FROM information_schema.STATISTICS
  WHERE TABLE_SCHEMA = @schema AND TABLE_NAME = 'short_urls' AND INDEX_NAME = 'idx_clicks');
SET @sql = IF(@idx_exists = 0,
  'ALTER TABLE `short_urls` ADD KEY `idx_clicks` (`clicks` DESC)',
  'SELECT 1');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;
