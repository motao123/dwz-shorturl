-- ============================================================
-- Idempotent migration: add password protection columns.
-- short_urls.password_hash  (admin DB)  - bcrypt hash, NULL = open
-- wjoy_log.password_hash    (public DB) - bcrypt hash, NULL = open
--
-- Usage:
--   mysql -uADMIN_USER -p ADMIN_DB < add_password_hash.sql      (short_urls part)
--   mysql -uPUBLIC_USER -p PUBLIC_DB < add_password_hash.sql    (wjoy_log part)
-- Safe to re-run; each statement checks table + column existence first,
-- so running the whole file against the wrong schema is a no-op for the
-- tables that do not live there.
-- ============================================================

SET @schema = DATABASE();

-- short_urls.password_hash (admin DB; skipped when the table is absent)
SET @tbl_exists = (SELECT COUNT(*) FROM information_schema.TABLES
  WHERE TABLE_SCHEMA = @schema AND TABLE_NAME = 'short_urls');
SET @col_exists = IF(@tbl_exists = 0, 1, (SELECT COUNT(*) FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = @schema AND TABLE_NAME = 'short_urls' AND COLUMN_NAME = 'password_hash'));
SET @sql = IF(@col_exists = 0,
  'ALTER TABLE short_urls ADD COLUMN password_hash VARCHAR(255) NULL COMMENT ''bcrypt; non-NULL = password protected'' AFTER expire_at',
  'SELECT 1');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

-- wjoy_log.password_hash (public DB; skipped when the table is absent)
SET @tbl_exists = (SELECT COUNT(*) FROM information_schema.TABLES
  WHERE TABLE_SCHEMA = @schema AND TABLE_NAME = 'wjoy_log');
SET @col_exists = IF(@tbl_exists = 0, 1, (SELECT COUNT(*) FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = @schema AND TABLE_NAME = 'wjoy_log' AND COLUMN_NAME = 'password_hash'));
SET @sql = IF(@col_exists = 0,
  'ALTER TABLE wjoy_log ADD COLUMN password_hash VARCHAR(255) NULL COMMENT ''bcrypt; non-NULL = password protected'' AFTER expire_at',
  'SELECT 1');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;
