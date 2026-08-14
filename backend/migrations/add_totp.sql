-- ============================================================
-- Idempotent migration: admin 2FA (TOTP) shared secret.
-- users.totp_secret - base32 TOTP secret, NULL/empty = 2FA disabled
--
-- Usage: mysql -uADMIN_USER -p ADMIN_DB < add_totp.sql
-- Safe to re-run.
-- ============================================================

SET @schema = DATABASE();
SET @col_exists = (SELECT COUNT(*) FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = @schema AND TABLE_NAME = 'users' AND COLUMN_NAME = 'totp_secret');
SET @sql = IF(@col_exists = 0,
  'ALTER TABLE users ADD COLUMN totp_secret VARCHAR(64) NULL COMMENT ''base32 TOTP secret; non-empty = 2FA enabled'' AFTER status',
  'SELECT 1');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;
