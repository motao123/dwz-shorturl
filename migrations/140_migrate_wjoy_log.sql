-- Migration 002: Migrate legacy wjoy_log data into short_urls
-- Idempotent: uses WHERE NOT EXISTS to avoid duplicates on re-run.
-- This bridges the PHP era (table wjoy_log) and the Go admin backend (table short_urls).

-- 本迁移跨库：读取公共库 wjoy_log，写入管理库 short_urls。
-- 两库可能不是同一个 schema，因此这里显式限定库名，运行库（connection 默认库）
-- 不影响结果。占位符由 cmd/migrate 注入实际库名。
SET NAMES utf8mb4;

-- 1. Copy legacy rows that don't already exist in short_urls
INSERT IGNORE INTO `{{ADMIN_DB}}`.`short_urls` (uid, long_url, url_hash, clicks, status, source, created_at, updated_at, expire_at)
SELECT
    w.uid,
    w.longurl                           AS long_url,
    MD5(w.longurl)                      AS url_hash,
    COALESCE(w.clicks, 0)               AS clicks,
    1                                   AS status,       -- active
    'legacy'                            AS source,       -- migrated from PHP
    COALESCE(w.created_at, NOW(3))      AS created_at,
    COALESCE(w.created_at, NOW(3))      AS updated_at,
    w.expire_at
FROM `{{PUBLIC_DB}}`.`wjoy_log` w
WHERE w.uid IS NOT NULL
  AND w.uid != ''
  AND w.uid NOT IN (SELECT uid FROM `{{ADMIN_DB}}`.`short_urls`);

-- 2. If wjoy_log has base64-encoded longurl, decode and fix long_url (MySQL 8 only)
--    This is a no-op for URLs already in plain text.
UPDATE `{{ADMIN_DB}}`.`short_urls` s
JOIN `{{PUBLIC_DB}}`.`wjoy_log` w ON w.uid = s.uid
SET
    s.long_url = FROM_BASE64(w.longurl),
    s.url_hash = MD5(FROM_BASE64(w.longurl))
WHERE s.source = 'legacy'
  AND w.longurl REGEXP '^[A-Za-z0-9+/=]+$'
  AND FROM_BASE64(w.longurl) REGEXP '^https?://'
  AND CHAR_LENGTH(w.longurl) > 40;

-- 3. Create a compatibility view so PHP code can SELECT from short_urls using old column names
--    This allows a gradual cutover without breaking existing PHP queries.
CREATE OR REPLACE VIEW `{{PUBLIC_DB}}`.`wjoy_compat` AS
SELECT
    uid,
    long_url   AS longurl,
    clicks,
    created_at,
    expire_at
FROM `{{ADMIN_DB}}`.`short_urls`
WHERE deleted_at IS NULL;

-- Verification (run manually to confirm):
-- SELECT COUNT(*) AS migrated_count FROM short_urls WHERE source = 'legacy';
-- SELECT COUNT(*) AS total_in_wjoy_log FROM wjoy_log;
