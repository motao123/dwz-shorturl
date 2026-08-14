-- ============================================================
-- Idempotent migration: backfill columns that drift between the
-- GORM models / PHP write paths and the hand-written DDL.
-- Safe to run repeatedly; each statement first checks
-- information_schema so nothing is altered twice.
--
-- Usage (MySQL):
--   mysql -uADMIN_USER -p ADMIN_DB < add_missing_columns.sql
--   mysql -uPUBLIC_USER -p PUBLIC_DB < add_missing_columns.sql
--
-- The two halves target different schemas; run each half against
-- the matching database (admin DB for short_urls, public DB for
-- wjoy_log / members). See deploy/scripts/apply_migrations.sh.
-- ============================================================

-- ---------- Admin schema: short_urls ----------
SET @schema = DATABASE();

-- member_id
SET @col_exists = (SELECT COUNT(*) FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = @schema AND TABLE_NAME = 'short_urls' AND COLUMN_NAME = 'member_id');
SET @sql = IF(@col_exists = 0,
  'ALTER TABLE short_urls ADD COLUMN member_id BIGINT UNSIGNED NULL COMMENT ''public member ID'' AFTER created_by',
  'SELECT 1');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

-- idx_member index
SET @idx_exists = (SELECT COUNT(*) FROM information_schema.STATISTICS
  WHERE TABLE_SCHEMA = @schema AND TABLE_NAME = 'short_urls' AND INDEX_NAME = 'idx_member');
SET @sql = IF(@idx_exists = 0,
  'ALTER TABLE short_urls ADD KEY idx_member (member_id)',
  'SELECT 1');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

-- reminder_sent_at
SET @col_exists = (SELECT COUNT(*) FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = @schema AND TABLE_NAME = 'short_urls' AND COLUMN_NAME = 'reminder_sent_at');
SET @sql = IF(@col_exists = 0,
  'ALTER TABLE short_urls ADD COLUMN reminder_sent_at DATETIME(3) NULL COMMENT ''last expiry reminder sent at'' AFTER ip',
  'SELECT 1');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

-- ---------- Public schema: wjoy_log / members ----------
-- Run this section against the public (frontend) database.

-- wjoy_log.status
SET @col_exists = (SELECT COUNT(*) FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = @schema AND TABLE_NAME = 'wjoy_log' AND COLUMN_NAME = 'status');
SET @sql = IF(@col_exists = 0,
  'ALTER TABLE wjoy_log ADD COLUMN status TINYINT NOT NULL DEFAULT 1 COMMENT ''1=active 0=disabled'' AFTER expire_at, ADD KEY idx_status (status)',
  'SELECT 1');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

-- members.token_version
SET @col_exists = (SELECT COUNT(*) FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = @schema AND TABLE_NAME = 'members' AND COLUMN_NAME = 'token_version');
SET @sql = IF(@col_exists = 0,
  'ALTER TABLE members ADD COLUMN token_version INT NOT NULL DEFAULT 0 COMMENT ''increment to revoke all JWT sessions'' AFTER last_login_ip',
  'SELECT 1');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

-- members.email_verified
SET @col_exists = (SELECT COUNT(*) FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = @schema AND TABLE_NAME = 'members' AND COLUMN_NAME = 'email_verified');
SET @sql = IF(@col_exists = 0,
  'ALTER TABLE members ADD COLUMN email_verified TINYINT NOT NULL DEFAULT 0 COMMENT ''0=unverified 1=verified'' AFTER token_version',
  'SELECT 1');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

-- members.verify_token + verify_expires_at
SET @col_exists = (SELECT COUNT(*) FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = @schema AND TABLE_NAME = 'members' AND COLUMN_NAME = 'verify_token');
SET @sql = IF(@col_exists = 0,
  'ALTER TABLE members ADD COLUMN verify_token VARCHAR(64) NULL, ADD COLUMN verify_expires_at DATETIME NULL, ADD KEY idx_verify_token (verify_token)',
  'SELECT 1');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

-- members.reset_token + reset_expires_at
SET @col_exists = (SELECT COUNT(*) FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = @schema AND TABLE_NAME = 'members' AND COLUMN_NAME = 'reset_token');
SET @sql = IF(@col_exists = 0,
  'ALTER TABLE members ADD COLUMN reset_token VARCHAR(64) NULL, ADD COLUMN reset_expires_at DATETIME NULL, ADD KEY idx_reset_token (reset_token)',
  'SELECT 1');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;
