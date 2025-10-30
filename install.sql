DROP TABLE IF EXISTS `wjoy_log`;
CREATE TABLE `wjoy_log` (
  `Id` int(11) NOT NULL AUTO_INCREMENT,
  `uid` varchar(16) DEFAULT NULL,
  `longurl` TEXT,
  PRIMARY KEY (`Id`),
  UNIQUE KEY `uniq_uid` (`uid`),
  KEY `idx_longurl` (`longurl`(255))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
