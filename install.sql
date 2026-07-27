-- Fresh-install schema. This file is intentionally non-destructive:
-- existing tables and rows are never dropped or replaced.
-- Existing installations should run migrations/001_legacy_schema.php instead.

CREATE TABLE IF NOT EXISTS `wjoy_log` (
  `Id` int unsigned NOT NULL AUTO_INCREMENT,
  `uid` varchar(16) NOT NULL,
  `longurl` text NOT NULL,
  `url_hash` char(32) NOT NULL,
  `clicks` int unsigned NOT NULL DEFAULT 0,
  `created_at` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `expire_at` datetime DEFAULT NULL,
  PRIMARY KEY (`Id`),
  UNIQUE KEY `uniq_uid` (`uid`),
  UNIQUE KEY `uniq_hash` (`url_hash`),
  KEY `idx_created_at` (`created_at`),
  KEY `idx_clicks` (`clicks`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
