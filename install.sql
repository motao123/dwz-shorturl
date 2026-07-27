DROP TABLE IF EXISTS `wjoy_log`;
CREATE TABLE `wjoy_log` (
  `Id` int(11) NOT NULL AUTO_INCREMENT,
  `uid` varchar(16) DEFAULT NULL,
  `longurl` TEXT,
  `url_hash` char(32) NOT NULL DEFAULT '',
  `clicks` int(11) NOT NULL DEFAULT 0,
  `created_at` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `expire_at` datetime DEFAULT NULL,
  PRIMARY KEY (`Id`),
  UNIQUE KEY `uniq_uid` (`uid`),
  UNIQUE KEY `uniq_hash` (`url_hash`),
  KEY `idx_longurl` (`longurl`(255))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
