-- AD/LDAP Entegrasyonu için Envanter Genişletmesi
-- Sprint 1: AD Discovery + Temel Yapı

-- =====================================================
-- 1. AD Ayarları Tablosu
-- =====================================================
CREATE TABLE IF NOT EXISTS `ad_settings` (
  `id` INT NOT NULL AUTO_INCREMENT,

  -- Bağlantı Bilgileri
  `domain_controller` VARCHAR(255) NOT NULL COMMENT 'DC adresi (dc.otel.local)',
  `port` INT NOT NULL DEFAULT 389 COMMENT 'LDAP port (389 veya 636 for LDAPS)',
  `use_ssl` TINYINT(1) NOT NULL DEFAULT 0 COMMENT 'LDAPS kullan',
  `base_dn` VARCHAR(500) NOT NULL COMMENT 'Base DN (DC=otel,DC=local)',

  -- Kimlik Bilgileri
  `bind_username` VARCHAR(255) NOT NULL COMMENT 'Servis hesabı (systrack@otel.local)',
  `bind_password_encrypted` TEXT NOT NULL COMMENT 'Şifrelenmiş parola',

  -- Tarama Ayarları
  `search_filter` VARCHAR(500) DEFAULT '(objectClass=computer)' COMMENT 'LDAP filtresi',
  `search_scope` ENUM('base','one','sub') DEFAULT 'sub' COMMENT 'Arama kapsamı',
  `ou_filter` TEXT DEFAULT NULL COMMENT 'Sadece belirli OU''ları tara (JSON array)',

  -- Eşleştirme Ayarları
  `location_from_ou` TINYINT(1) DEFAULT 1 COMMENT 'OU''dan lokasyon çıkar',
  `ou_location_map` JSON DEFAULT NULL COMMENT 'OU -> Lokasyon eşlemesi',

  -- Durum
  `is_enabled` TINYINT(1) NOT NULL DEFAULT 1,
  `last_test_at` DATETIME DEFAULT NULL,
  `last_test_result` ENUM('success','failed') DEFAULT NULL,
  `last_test_error` TEXT DEFAULT NULL,

  `created_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,

  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- =====================================================
-- 2. WinRM Ayarları Tablosu (Sprint 2 için hazırlık)
-- =====================================================
CREATE TABLE IF NOT EXISTS `winrm_settings` (
  `id` INT NOT NULL AUTO_INCREMENT,

  -- Kimlik Bilgileri
  `username` VARCHAR(255) NOT NULL COMMENT 'WinRM kullanıcısı (domain\\admin)',
  `password_encrypted` TEXT NOT NULL COMMENT 'Şifrelenmiş parola',

  -- Bağlantı Ayarları
  `port` INT NOT NULL DEFAULT 5985 COMMENT 'WinRM port (5985 HTTP, 5986 HTTPS)',
  `use_ssl` TINYINT(1) NOT NULL DEFAULT 0,
  `timeout_seconds` INT NOT NULL DEFAULT 30,

  -- Tarama Ayarları
  `concurrent_limit` INT NOT NULL DEFAULT 5 COMMENT 'Paralel bağlantı limiti',
  `retry_count` INT NOT NULL DEFAULT 2,

  -- Durum
  `is_enabled` TINYINT(1) NOT NULL DEFAULT 0,
  `last_test_at` DATETIME DEFAULT NULL,
  `last_test_result` ENUM('success','failed') DEFAULT NULL,
  `last_test_error` TEXT DEFAULT NULL,

  `created_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,

  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- =====================================================
-- 3. SNMP Ayarları Tablosu (Sprint 3 için hazırlık)
-- =====================================================
CREATE TABLE IF NOT EXISTS `snmp_settings` (
  `id` INT NOT NULL AUTO_INCREMENT,

  -- SNMP v2c Ayarları
  `version` ENUM('v1','v2c','v3') NOT NULL DEFAULT 'v2c',
  `community_strings` TEXT NOT NULL COMMENT 'Denenecek community string listesi (JSON array)',

  -- SNMP v3 Ayarları (ileride kullanılacak)
  `v3_username` VARCHAR(255) DEFAULT NULL,
  `v3_auth_protocol` ENUM('MD5','SHA') DEFAULT NULL,
  `v3_auth_password_encrypted` TEXT DEFAULT NULL,
  `v3_priv_protocol` ENUM('DES','AES') DEFAULT NULL,
  `v3_priv_password_encrypted` TEXT DEFAULT NULL,

  -- Bağlantı Ayarları
  `port` INT NOT NULL DEFAULT 161,
  `timeout_seconds` INT NOT NULL DEFAULT 5,
  `retry_count` INT NOT NULL DEFAULT 2,
  `concurrent_limit` INT NOT NULL DEFAULT 10,

  -- Durum
  `is_enabled` TINYINT(1) NOT NULL DEFAULT 0,

  `created_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,

  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- =====================================================
-- 4. Envanter Tablosuna AD Alanları Ekle
-- =====================================================
ALTER TABLE `inventory`
  -- AD'den gelen kimlik bilgileri
  ADD COLUMN `ad_object_guid` VARCHAR(36) DEFAULT NULL COMMENT 'AD objectGUID (benzersiz kimlik)' AFTER `id`,
  ADD COLUMN `ad_distinguished_name` VARCHAR(1000) DEFAULT NULL COMMENT 'AD DN (tam yol)' AFTER `ad_object_guid`,
  ADD COLUMN `ad_computer_name` VARCHAR(255) DEFAULT NULL COMMENT 'AD sAMAccountName' AFTER `ad_distinguished_name`,
  ADD COLUMN `ad_description` TEXT DEFAULT NULL COMMENT 'AD description alanı' AFTER `ad_computer_name`,
  ADD COLUMN `ad_ou_path` VARCHAR(1000) DEFAULT NULL COMMENT 'OU yolu (konum çıkarımı için)' AFTER `ad_description`,
  ADD COLUMN `ad_os_name` VARCHAR(255) DEFAULT NULL COMMENT 'AD operatingSystem' AFTER `ad_ou_path`,
  ADD COLUMN `ad_os_version` VARCHAR(100) DEFAULT NULL COMMENT 'AD operatingSystemVersion' AFTER `ad_os_name`,
  ADD COLUMN `ad_last_logon` DATETIME DEFAULT NULL COMMENT 'AD lastLogonTimestamp' AFTER `ad_os_version`,
  ADD COLUMN `ad_when_created` DATETIME DEFAULT NULL COMMENT 'AD whenCreated' AFTER `ad_last_logon`,

  -- WMI/WinRM'den gelen donanım bilgileri (JSON - esnek)
  ADD COLUMN `hardware_json` JSON DEFAULT NULL COMMENT 'WMI donanım bilgileri (CPU, RAM, Disk, Monitors)' AFTER `ad_when_created`,

  -- SNMP'den gelen bilgiler (JSON - esnek)
  ADD COLUMN `snmp_json` JSON DEFAULT NULL COMMENT 'SNMP bilgileri (sysDescr, sysObjectID, interfaces)' AFTER `hardware_json`,

  -- Keşif/Zenginleştirme meta bilgileri
  ADD COLUMN `discovery_source` ENUM('manual','ip_scanner','ad_ldap','snmp') DEFAULT NULL COMMENT 'İlk keşif kaynağı' AFTER `source`,
  ADD COLUMN `last_ad_sync_at` DATETIME DEFAULT NULL COMMENT 'Son AD senkronizasyonu' AFTER `discovery_source`,
  ADD COLUMN `last_winrm_enrich_at` DATETIME DEFAULT NULL COMMENT 'Son WinRM zenginleştirmesi' AFTER `last_ad_sync_at`,
  ADD COLUMN `last_snmp_enrich_at` DATETIME DEFAULT NULL COMMENT 'Son SNMP zenginleştirmesi' AFTER `last_winrm_enrich_at`,
  ADD COLUMN `enrichment_errors` JSON DEFAULT NULL COMMENT 'Son zenginleştirme hataları' AFTER `last_snmp_enrich_at`;

-- AD objectGUID için benzersiz index
ALTER TABLE `inventory`
  ADD UNIQUE KEY `idx_ad_object_guid` (`ad_object_guid`);

-- Mevcut source enum'ına yeni değerler ekle
ALTER TABLE `inventory`
  MODIFY COLUMN `source` ENUM('manual','ip_scanner','csv_import','ad_ldap','snmp') NOT NULL DEFAULT 'manual';

-- =====================================================
-- 5. Tarama/Senkronizasyon Logları
-- =====================================================
CREATE TABLE IF NOT EXISTS `inventory_scans` (
  `id` INT NOT NULL AUTO_INCREMENT,

  `scan_type` ENUM('ad_discovery','winrm_enrich','snmp_enrich','ip_scan') NOT NULL,
  `status` ENUM('pending','running','completed','failed','cancelled') NOT NULL DEFAULT 'pending',

  -- İstatistikler
  `total_targets` INT DEFAULT 0 COMMENT 'Toplam hedef sayısı',
  `processed` INT DEFAULT 0 COMMENT 'İşlenen sayısı',
  `success_count` INT DEFAULT 0 COMMENT 'Başarılı',
  `error_count` INT DEFAULT 0 COMMENT 'Hatalı',
  `new_assets` INT DEFAULT 0 COMMENT 'Yeni eklenen varlık',
  `updated_assets` INT DEFAULT 0 COMMENT 'Güncellenen varlık',

  -- Detaylar
  `errors_json` JSON DEFAULT NULL COMMENT 'Hata detayları',
  `scan_config` JSON DEFAULT NULL COMMENT 'Tarama konfigürasyonu',

  -- Zamanlar
  `started_at` DATETIME DEFAULT NULL,
  `completed_at` DATETIME DEFAULT NULL,
  `created_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,

  -- Başlatan kullanıcı
  `started_by` VARCHAR(100) DEFAULT NULL,

  PRIMARY KEY (`id`),
  KEY `idx_scan_type` (`scan_type`),
  KEY `idx_status` (`status`),
  KEY `idx_created` (`created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- =====================================================
-- 6. Varsayılan Ayarları Ekle (tek satır)
-- =====================================================
INSERT INTO `ad_settings` (`domain_controller`, `port`, `base_dn`, `bind_username`, `bind_password_encrypted`, `is_enabled`)
VALUES ('', 389, '', '', '', 0)
ON DUPLICATE KEY UPDATE `id` = `id`;

INSERT INTO `winrm_settings` (`username`, `password_encrypted`, `is_enabled`)
VALUES ('', '', 0)
ON DUPLICATE KEY UPDATE `id` = `id`;

INSERT INTO `snmp_settings` (`community_strings`, `is_enabled`)
VALUES ('["public"]', 0)
ON DUPLICATE KEY UPDATE `id` = `id`;
