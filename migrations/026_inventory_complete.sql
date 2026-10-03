-- =====================================================
-- Migration 026 (Complete): Envanter Yönetimi Tam Kurulum
-- =====================================================
-- Kapsam: 026 + 027 + 028 + 030 + 031 + 032 + 033 + 034

-- =====================================================
-- 1) ENVANTER ANA TABLOSU
-- =====================================================
CREATE TABLE IF NOT EXISTS `inventory` (
  `id` INT NOT NULL AUTO_INCREMENT,

  -- AD / LDAP çekirdek kimlik
  `ad_object_guid` VARCHAR(64) DEFAULT NULL COMMENT 'AD objectGUID',
  `ad_distinguished_name` VARCHAR(1000) DEFAULT NULL COMMENT 'AD DN (tam yol)',
  `ad_computer_name` VARCHAR(255) DEFAULT NULL COMMENT 'AD sAMAccountName',
  `ad_cn_name` VARCHAR(255) DEFAULT NULL COMMENT 'AD CN',
  `ad_ou_path` VARCHAR(1000) DEFAULT NULL COMMENT 'OU yolu',

  -- AD detay alanları
  `ad_description` TEXT DEFAULT NULL,
  `ad_comment` TEXT DEFAULT NULL,
  `ad_location` VARCHAR(255) DEFAULT NULL,
  `ad_managed_by` VARCHAR(500) DEFAULT NULL,
  `ad_os_name` VARCHAR(255) DEFAULT NULL,
  `ad_os_version` VARCHAR(100) DEFAULT NULL,
  `ad_os_service_pack` VARCHAR(100) DEFAULT NULL,
  `ad_last_logon` DATETIME DEFAULT NULL,
  `ad_when_created` DATETIME DEFAULT NULL,
  `ad_when_changed` DATETIME DEFAULT NULL,
  `ad_pwd_last_set` DATETIME DEFAULT NULL,
  `ad_service_principal_names` TEXT DEFAULT NULL COMMENT 'JSON metin',

  -- Ağ / cihaz temel alanları
  `ip_address` VARCHAR(45) DEFAULT NULL,
  `mac_address` VARCHAR(17) DEFAULT NULL,
  `hostname` VARCHAR(255) DEFAULT NULL,
  `vendor` VARCHAR(255) DEFAULT NULL,

  -- Kullanıcı/manüel alanlar
  `asset_name` VARCHAR(255) NOT NULL,
  `asset_tag` VARCHAR(50) DEFAULT NULL,
  `asset_type` ENUM('pc','laptop','printer','switch','router','access_point','pos','tv','phone','camera','server','tablet','other') NOT NULL DEFAULT 'other',
  `brand` VARCHAR(100) DEFAULT NULL,
  `model` VARCHAR(255) DEFAULT NULL,
  `serial_number` VARCHAR(100) DEFAULT NULL,
  `location` VARCHAR(255) DEFAULT NULL,
  `department` VARCHAR(100) DEFAULT NULL,
  `assigned_to` VARCHAR(255) DEFAULT NULL,
  `status` ENUM('active','maintenance','storage','faulty','retired') NOT NULL DEFAULT 'active',
  `purchase_date` DATE DEFAULT NULL,
  `warranty_expiry` DATE DEFAULT NULL,
  `purchase_cost` DECIMAL(10,2) DEFAULT NULL,
  `notes` TEXT DEFAULT NULL,

  -- Keşif / kaynak
  `source` VARCHAR(50) NOT NULL DEFAULT 'manual',
  `discovery_source` VARCHAR(50) DEFAULT NULL,
  `discovery_methods` TEXT DEFAULT NULL COMMENT 'JSON array',
  `discovered_at` DATETIME DEFAULT NULL,

  -- Zenginleştirme JSON alanları
  `snmp_json` JSON DEFAULT NULL,
  `ssh_json` JSON DEFAULT NULL,
  `winrm_json` JSON DEFAULT NULL,
  `owner_json` JSON DEFAULT NULL,
  `enrichment_errors` JSON DEFAULT NULL,

  -- İzleme bağlantısı
  `target_id` INT DEFAULT NULL,

  -- Enrichment zamanları
  `last_ad_sync_at` DATETIME DEFAULT NULL,
  `last_winrm_enrich_at` DATETIME DEFAULT NULL,
  `last_snmp_enrich_at` DATETIME DEFAULT NULL,
  `last_ssh_enrich_at` DATETIME DEFAULT NULL,

  `created_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,

  PRIMARY KEY (`id`),
  UNIQUE KEY `idx_asset_tag` (`asset_tag`),
  UNIQUE KEY `idx_ad_object_guid` (`ad_object_guid`),
  KEY `idx_inventory_ip` (`ip_address`),
  KEY `idx_inventory_mac` (`mac_address`),
  KEY `idx_inventory_hostname` (`hostname`),
  KEY `idx_inventory_status` (`status`),
  KEY `idx_inventory_type` (`asset_type`),
  KEY `idx_inventory_location` (`location`),
  KEY `idx_inventory_target` (`target_id`),
  KEY `idx_inventory_warranty` (`warranty_expiry`),
  KEY `idx_inventory_ad_location` (`ad_location`),
  KEY `idx_inventory_ad_when_changed` (`ad_when_changed`),
  KEY `idx_inventory_discovery_source` (`discovery_source`),
  KEY `idx_inventory_discovered_at` (`discovered_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- =====================================================
-- 2) AD AYARLARI
-- =====================================================
CREATE TABLE IF NOT EXISTS `ad_settings` (
  `id` INT NOT NULL AUTO_INCREMENT,
  `domain_controller` VARCHAR(255) NOT NULL,
  `port` INT NOT NULL DEFAULT 389,
  `use_ssl` TINYINT(1) NOT NULL DEFAULT 0,
  `base_dn` VARCHAR(500) NOT NULL,
  `bind_username` VARCHAR(255) NOT NULL,
  `bind_password_encrypted` TEXT NOT NULL,
  `search_filter` VARCHAR(500) DEFAULT '(objectClass=computer)',
  `search_scope` ENUM('base','one','sub') DEFAULT 'sub',
  `ou_filter` TEXT DEFAULT NULL,
  `location_from_ou` TINYINT(1) DEFAULT 1,
  `ou_location_map` JSON DEFAULT NULL,
  `is_enabled` TINYINT(1) NOT NULL DEFAULT 1,
  `last_test_at` DATETIME DEFAULT NULL,
  `last_test_result` ENUM('success','failed') DEFAULT NULL,
  `last_test_error` TEXT DEFAULT NULL,
  `last_test_computer_count` INT DEFAULT NULL,
  `created_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- =====================================================
-- 3) WINRM AYARLARI
-- =====================================================
CREATE TABLE IF NOT EXISTS `winrm_settings` (
  `id` INT NOT NULL AUTO_INCREMENT,
  `username` VARCHAR(255) NOT NULL,
  `password_encrypted` TEXT NOT NULL,
  `port` INT NOT NULL DEFAULT 5985,
  `use_ssl` TINYINT(1) NOT NULL DEFAULT 0,
  `timeout_seconds` INT NOT NULL DEFAULT 30,
  `concurrent_limit` INT NOT NULL DEFAULT 5,
  `retry_count` INT NOT NULL DEFAULT 2,
  `is_enabled` TINYINT(1) NOT NULL DEFAULT 0,
  `last_test_at` DATETIME DEFAULT NULL,
  `last_test_result` ENUM('success','failed') DEFAULT NULL,
  `last_test_error` TEXT DEFAULT NULL,
  `last_test_ip` VARCHAR(45) DEFAULT NULL,
  `created_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- =====================================================
-- 4) SNMP AYARLARI
-- =====================================================
CREATE TABLE IF NOT EXISTS `snmp_settings` (
  `id` INT NOT NULL AUTO_INCREMENT,
  `version` ENUM('v1','v2c','v3') NOT NULL DEFAULT 'v2c',
  `community_strings` TEXT NOT NULL,
  `v3_username` VARCHAR(255) DEFAULT NULL,
  `v3_auth_protocol` ENUM('MD5','SHA') DEFAULT NULL,
  `v3_auth_password_encrypted` TEXT DEFAULT NULL,
  `v3_priv_protocol` ENUM('DES','AES') DEFAULT NULL,
  `v3_priv_password_encrypted` TEXT DEFAULT NULL,
  `port` INT NOT NULL DEFAULT 161,
  `timeout_seconds` INT NOT NULL DEFAULT 5,
  `retry_count` INT NOT NULL DEFAULT 2,
  `concurrent_limit` INT NOT NULL DEFAULT 10,
  `scan_targets` TEXT DEFAULT NULL,
  `is_enabled` TINYINT(1) NOT NULL DEFAULT 0,
  `last_test_at` DATETIME DEFAULT NULL,
  `last_test_result` ENUM('success','failed') DEFAULT NULL,
  `last_test_error` TEXT DEFAULT NULL,
  `last_test_ip` VARCHAR(45) DEFAULT NULL,
  `created_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- =====================================================
-- 5) SSH AYARLARI
-- =====================================================
CREATE TABLE IF NOT EXISTS `ssh_settings` (
  `id` INT NOT NULL AUTO_INCREMENT,
  `username` VARCHAR(255) NOT NULL DEFAULT 'root',
  `password_encrypted` TEXT DEFAULT NULL,
  `port` INT NOT NULL DEFAULT 22,
  `timeout_seconds` INT NOT NULL DEFAULT 10,
  `is_enabled` TINYINT(1) NOT NULL DEFAULT 0,
  `created_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- =====================================================
-- 6) KEŞİF TARAMA TAKİP TABLOSU
-- =====================================================
CREATE TABLE IF NOT EXISTS `inventory_scans` (
  `id` INT NOT NULL AUTO_INCREMENT,
  `scan_type` VARCHAR(50) NOT NULL,
  `status` ENUM('pending','running','completed','failed','cancelled') NOT NULL DEFAULT 'pending',
  `total_targets` INT DEFAULT 0,
  `processed` INT DEFAULT 0,
  `success_count` INT DEFAULT 0,
  `error_count` INT DEFAULT 0,
  `new_assets` INT DEFAULT 0,
  `updated_assets` INT DEFAULT 0,
  `errors_json` JSON DEFAULT NULL,
  `scan_config` JSON DEFAULT NULL,
  `started_at` DATETIME DEFAULT NULL,
  `completed_at` DATETIME DEFAULT NULL,
  `created_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `started_by` VARCHAR(100) DEFAULT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_inventory_scans_type` (`scan_type`),
  KEY `idx_inventory_scans_status` (`status`),
  KEY `idx_inventory_scans_created` (`created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- =====================================================
-- 7) VARSAYILAN AYAR KAYITLARI (id=1)
-- =====================================================
INSERT INTO `ad_settings`
(`id`, `domain_controller`, `port`, `use_ssl`, `base_dn`, `bind_username`, `bind_password_encrypted`, `is_enabled`)
VALUES
(1, '', 389, 0, '', '', '', 0)
ON DUPLICATE KEY UPDATE `id` = `id`;

INSERT INTO `winrm_settings`
(`id`, `username`, `password_encrypted`, `port`, `use_ssl`, `timeout_seconds`, `concurrent_limit`, `retry_count`, `is_enabled`)
VALUES
(1, '', '', 5985, 0, 30, 5, 2, 0)
ON DUPLICATE KEY UPDATE `id` = `id`;

INSERT INTO `snmp_settings`
(`id`, `version`, `community_strings`, `port`, `timeout_seconds`, `retry_count`, `concurrent_limit`, `scan_targets`, `is_enabled`)
VALUES
(1, 'v2c', '["public"]', 161, 5, 2, 10, '', 0)
ON DUPLICATE KEY UPDATE `id` = `id`;

INSERT INTO `ssh_settings`
(`id`, `username`, `password_encrypted`, `port`, `timeout_seconds`, `is_enabled`)
VALUES
(1, 'root', '', 22, 10, 0)
ON DUPLICATE KEY UPDATE `id` = `id`;

-- =====================================================
-- 8) YAZILIM / UPTIME / GUC ALANLARI
-- =====================================================
ALTER TABLE `inventory`
  ADD COLUMN IF NOT EXISTS `software_json` LONGTEXT NULL
    COMMENT 'Kurulu yazilim listesi (JSON array)' AFTER `ssh_json`,
  ADD COLUMN IF NOT EXISTS `software_scan_at` DATETIME NULL
    COMMENT 'Son yazilim tarama zamani' AFTER `software_json`,
  ADD COLUMN IF NOT EXISTS `last_boot_time` VARCHAR(50) NULL
    COMMENT 'Son sistem acilis zamani' AFTER `software_scan_at`,
  ADD COLUMN IF NOT EXISTS `power_estimate_watts` INT NULL
    COMMENT 'Tahmini guc tuketimi (Watt)' AFTER `last_boot_time`;

-- =====================================================
-- 9) YAZILIM LISANS TABLOSU
-- =====================================================
CREATE TABLE IF NOT EXISTS `software_licenses` (
  `id` INT NOT NULL AUTO_INCREMENT,
  `inventory_id` INT NOT NULL,
  `software_name` VARCHAR(512) NOT NULL COMMENT 'software_json icindeki Name ile eslesir',
  `purchase_cost` DECIMAL(12,2) DEFAULT NULL COMMENT 'Satın alma maliyeti',
  `cost_currency` VARCHAR(8) NOT NULL DEFAULT 'TRY' COMMENT 'Para birimi (TRY/EUR/USD)',
  `purchase_date` DATE DEFAULT NULL COMMENT 'Satın alma tarihi',
  `license_expiry_date` DATE DEFAULT NULL COMMENT 'Lisans bitis tarihi',
  `notes` TEXT DEFAULT NULL COMMENT 'Notlar',
  `created_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_inv_software` (`inventory_id`, `software_name`(255)),
  CONSTRAINT `fk_sl_inventory` FOREIGN KEY (`inventory_id`) REFERENCES `inventory`(`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- =====================================================
-- 10) CRACK / LISANS UYUMLULUK ALANLARI
-- =====================================================
ALTER TABLE `inventory`
  ADD COLUMN IF NOT EXISTS `crack_scan_at` DATETIME NULL
    COMMENT 'Son lisans uyumluluk tarama zamani' AFTER `power_estimate_watts`,
  ADD COLUMN IF NOT EXISTS `crack_risk_level` VARCHAR(20) NULL
    COMMENT 'Risk seviyesi: clean/low/medium/high' AFTER `crack_scan_at`,
  ADD COLUMN IF NOT EXISTS `crack_findings_json` LONGTEXT NULL
    COMMENT 'Tespit edilen bulgular (JSON array)' AFTER `crack_risk_level`;
