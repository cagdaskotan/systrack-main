-- ============================================================
-- systrack.sql  (SysTrack Kurulum aracı ile üretildi)
-- Kaynak DB: root@127.0.0.1:3306/systrack
-- Yapı: TÜM tablolar | Seed veri: yalnızca default tabloları
-- Şablon tablolarında yalnızca is_default=1 satırlar alınır; kullanıcı şablonları dahil edilmez.
-- ============================================================
SET NAMES utf8mb4;
SET FOREIGN_KEY_CHECKS=0;

-- ----- Tablo: access_allowlist -----
DROP TABLE IF EXISTS `access_allowlist`;
CREATE TABLE `access_allowlist` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `ip_address` varchar(45) COLLATE utf8mb4_unicode_ci NOT NULL,
  `label` varchar(120) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `is_enabled` tinyint(1) NOT NULL DEFAULT '1',
  `created_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_access_allowlist_ip` (`ip_address`),
  KEY `idx_access_allowlist_enabled` (`is_enabled`)
) ENGINE=InnoDB AUTO_INCREMENT=6 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ----- Tablo: ad_settings -----
DROP TABLE IF EXISTS `ad_settings`;
CREATE TABLE `ad_settings` (
  `id` int NOT NULL AUTO_INCREMENT,
  `domain_controller` varchar(255) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT 'DC adresi (dc.otel.local)',
  `port` int NOT NULL DEFAULT '389' COMMENT 'LDAP port (389 veya 636 for LDAPS)',
  `use_ssl` tinyint(1) NOT NULL DEFAULT '0' COMMENT 'LDAPS kullan',
  `base_dn` varchar(500) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT 'Base DN (DC=otel,DC=local)',
  `bind_username` varchar(255) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT 'Servis hesabı (systrack@otel.local)',
  `bind_password_encrypted` text COLLATE utf8mb4_unicode_ci NOT NULL COMMENT 'Şifrelenmiş parola',
  `search_filter` varchar(500) COLLATE utf8mb4_unicode_ci DEFAULT '(objectClass=computer)' COMMENT 'LDAP filtresi',
  `search_scope` enum('base','one','sub') COLLATE utf8mb4_unicode_ci DEFAULT 'sub' COMMENT 'Arama kapsamı',
  `ou_filter` text COLLATE utf8mb4_unicode_ci COMMENT 'Sadece belirli OU''ları tara (JSON array)',
  `location_from_ou` tinyint(1) DEFAULT '1' COMMENT 'OU''dan lokasyon çıkar',
  `ou_location_map` json DEFAULT NULL COMMENT 'OU -> Lokasyon eşlemesi',
  `is_enabled` tinyint(1) NOT NULL DEFAULT '1',
  `last_test_at` datetime DEFAULT NULL,
  `last_test_result` enum('success','failed') COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `last_test_error` text COLLATE utf8mb4_unicode_ci,
  `last_test_computer_count` int DEFAULT NULL COMMENT 'Son testte bulunan bilgisayar sayısı',
  `created_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB AUTO_INCREMENT=2 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ----- Tablo: backup_logs -----
DROP TABLE IF EXISTS `backup_logs`;
CREATE TABLE `backup_logs` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `user_id` int DEFAULT NULL,
  `user_email` varchar(255) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `action` enum('export','import') COLLATE utf8mb4_unicode_ci NOT NULL,
  `status` enum('success','error') COLLATE utf8mb4_unicode_ci NOT NULL,
  `message` varchar(500) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `metadata` json DEFAULT NULL,
  `file_name` varchar(255) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `ip_address` varchar(45) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `created_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  KEY `idx_backup_logs_created_at` (`created_at`),
  KEY `idx_backup_logs_action` (`action`),
  KEY `idx_backup_logs_user` (`user_id`),
  CONSTRAINT `fk_backup_logs_user` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`) ON DELETE SET NULL
) ENGINE=InnoDB AUTO_INCREMENT=50 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ----- Tablo: crack_scan_exceptions -----
DROP TABLE IF EXISTS `crack_scan_exceptions`;
CREATE TABLE `crack_scan_exceptions` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `inventory_id` int DEFAULT NULL COMMENT 'NULL = tüm envanter kayıtlarına uygulanan global istisna',
  `signature_hash` char(64) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT 'sha256(category|title|evidence), bkz. findingSignature',
  `signature_text` text COLLATE utf8mb4_unicode_ci NOT NULL,
  `reason` varchar(500) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `created_by` int DEFAULT NULL,
  `created_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  KEY `idx_crack_scan_exceptions_inventory` (`inventory_id`),
  KEY `idx_crack_scan_exceptions_hash` (`signature_hash`)
) ENGINE=InnoDB AUTO_INCREMENT=2 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ----- Tablo: device_license -----
DROP TABLE IF EXISTS `device_license`;
CREATE TABLE `device_license` (
  `id` int NOT NULL AUTO_INCREMENT,
  `serial_number` varchar(50) COLLATE utf8mb4_general_ci NOT NULL,
  `last_update_version` varchar(20) COLLATE utf8mb4_general_ci DEFAULT NULL,
  `show_update_notification` tinyint(1) NOT NULL DEFAULT '0',
  `mac_address` varchar(20) COLLATE utf8mb4_general_ci NOT NULL,
  `status` tinyint DEFAULT '0',
  `customer_email` varchar(255) COLLATE utf8mb4_general_ci DEFAULT NULL,
  `customer_company` varchar(255) COLLATE utf8mb4_general_ci DEFAULT NULL,
  `activated_at` datetime DEFAULT NULL,
  `created_at` datetime DEFAULT CURRENT_TIMESTAMP,
  `initial_version` varchar(20) COLLATE utf8mb4_general_ci DEFAULT NULL COMMENT 'Software version at device manufacturing/first install',
  PRIMARY KEY (`id`),
  UNIQUE KEY `serial_number` (`serial_number`),
  KEY `idx_device_license_update_notification` (`show_update_notification`)
) ENGINE=MyISAM AUTO_INCREMENT=2 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

-- ----- Tablo: device_service_history -----
DROP TABLE IF EXISTS `device_service_history`;
CREATE TABLE `device_service_history` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `service_id` bigint NOT NULL,
  `target_id` int NOT NULL COMMENT 'Denormalized for faster queries',
  `service_name` varchar(255) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT 'Denormalized for faster queries',
  `old_status` enum('running','stopped','unknown') COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `new_status` enum('running','stopped','unknown') COLLATE utf8mb4_unicode_ci NOT NULL,
  `changed_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  KEY `idx_service_history_service` (`service_id`),
  KEY `idx_service_history_target` (`target_id`),
  KEY `idx_service_history_changed` (`changed_at`),
  CONSTRAINT `device_service_history_ibfk_1` FOREIGN KEY (`service_id`) REFERENCES `device_services` (`id`) ON DELETE CASCADE,
  CONSTRAINT `device_service_history_ibfk_2` FOREIGN KEY (`target_id`) REFERENCES `targets` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB AUTO_INCREMENT=3229 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ----- Tablo: device_services -----
DROP TABLE IF EXISTS `device_services`;
CREATE TABLE `device_services` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `target_id` int NOT NULL,
  `service_name` varchar(255) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT 'Service internal name (e.g., wuauserv, nginx)',
  `display_name` varchar(500) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT 'Service display name (e.g., Windows Update)',
  `description` text COLLATE utf8mb4_unicode_ci COMMENT 'Service description',
  `status` enum('running','stopped','unknown') COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'unknown',
  `startup_type` varchar(50) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT 'Auto, Manual, Disabled, etc.',
  `pid` int DEFAULT NULL COMMENT 'Process ID (if running)',
  `last_checked` datetime NOT NULL COMMENT 'Last check timestamp',
  `is_monitored` tinyint(1) DEFAULT '0' COMMENT 'User marked as critical for alerts',
  `created_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `unique_service_per_target` (`target_id`,`service_name`),
  KEY `idx_device_services_target` (`target_id`),
  KEY `idx_device_services_status` (`status`),
  KEY `idx_device_services_monitored` (`is_monitored`),
  KEY `idx_device_services_last_checked` (`last_checked`),
  CONSTRAINT `device_services_ibfk_1` FOREIGN KEY (`target_id`) REFERENCES `targets` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB AUTO_INCREMENT=1287 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ----- Tablo: inventory -----
DROP TABLE IF EXISTS `inventory`;
CREATE TABLE `inventory` (
  `id` int NOT NULL AUTO_INCREMENT,
  `ad_object_guid` varchar(36) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT 'AD objectGUID (benzersiz kimlik)',
  `ad_distinguished_name` varchar(1000) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT 'AD DN (tam yol)',
  `ad_computer_name` varchar(255) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT 'AD sAMAccountName',
  `ad_description` text COLLATE utf8mb4_unicode_ci COMMENT 'AD description alanı',
  `ad_ou_path` varchar(1000) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT 'OU yolu (konum çıkarımı için)',
  `ad_os_name` varchar(255) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT 'AD operatingSystem',
  `ad_os_version` varchar(100) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT 'AD operatingSystemVersion',
  `ad_last_logon` datetime DEFAULT NULL COMMENT 'AD lastLogonTimestamp',
  `ad_when_created` datetime DEFAULT NULL COMMENT 'AD whenCreated',
  `hardware_json` json DEFAULT NULL COMMENT 'WMI donanım bilgileri (CPU, RAM, Disk, Monitors)',
  `snmp_json` json DEFAULT NULL COMMENT 'SNMP bilgileri (sysDescr, sysObjectID, interfaces)',
  `ssh_json` json DEFAULT NULL COMMENT 'SSH bilgileri (OS, kernel, architecture, CPU, uptime, network interfaces)',
  `software_json` longtext COLLATE utf8mb4_unicode_ci COMMENT 'Kurulu yazılım listesi (JSON array)',
  `software_scan_at` datetime DEFAULT NULL COMMENT 'Son yazılım tarama zamanı',
  `last_boot_time` varchar(50) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT 'Son sistem açılış zamanı',
  `power_estimate_watts` int DEFAULT NULL COMMENT 'Tahmini güç tüketimi (Watt)',
  `crack_scan_at` datetime DEFAULT NULL COMMENT 'Son lisans uyumluluk tarama zamanı',
  `crack_risk_level` varchar(20) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT 'Risk seviyesi: clean/low/medium/high',
  `crack_findings_json` longtext COLLATE utf8mb4_unicode_ci COMMENT 'Tespit edilen bulgular (JSON array)',
  `winrm_json` json DEFAULT NULL COMMENT 'WinRM donanım bilgileri (CPU, RAM, disk, GPU, çevre birimleri)',
  `active_username` varchar(255) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT 'WinRM ile okunan anlık kullanıcı (DOMAIN\\username)',
  `ip_address` varchar(45) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `mac_address` varchar(17) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `hostname` varchar(255) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `vendor` varchar(255) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `asset_name` varchar(255) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT 'Envanter adı (zorunlu)',
  `asset_tag` varchar(50) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT 'Envanter kodu (INV-00001)',
  `asset_type` enum('pc','laptop','printer','switch','router','access_point','pos','tv','phone','camera','server','tablet','other') COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'other',
  `brand` varchar(100) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT 'Marka (Dell, HP, Cisco...)',
  `model` varchar(255) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT 'Model',
  `serial_number` varchar(100) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT 'Cihaz seri numarası',
  `location` varchar(255) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT 'Lokasyon (Kat 2 - Oda 214)',
  `department` varchar(100) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT 'Departman',
  `assigned_to` varchar(255) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT 'Sorumlu kişi',
  `status` enum('active','maintenance','storage','faulty','retired') COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'active',
  `purchase_date` date DEFAULT NULL,
  `warranty_expiry` date DEFAULT NULL,
  `purchase_cost` decimal(10,2) DEFAULT NULL COMMENT 'Satın alma maliyeti',
  `notes` text COLLATE utf8mb4_unicode_ci,
  `source` enum('manual','ip_scanner','csv_import','ad_ldap','snmp','ssh') COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'manual',
  `discovery_source` enum('manual','ip_scanner','ad_ldap','snmp','ssh') COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT 'İlk keşif kaynağı',
  `last_ad_sync_at` datetime DEFAULT NULL COMMENT 'Son AD senkronizasyonu',
  `last_winrm_enrich_at` datetime DEFAULT NULL COMMENT 'Son WinRM zenginleştirmesi',
  `last_snmp_enrich_at` datetime DEFAULT NULL COMMENT 'Son SNMP zenginleştirmesi',
  `last_ssh_enrich_at` datetime DEFAULT NULL COMMENT 'Son SSH zenginleştirmesi zamanı',
  `enrichment_errors` json DEFAULT NULL COMMENT 'Son zenginleştirme hataları',
  `target_id` int DEFAULT NULL COMMENT 'Hedefler tablosuyla ilişki',
  `created_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  `ad_cn_name` varchar(255) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `ad_os_service_pack` varchar(100) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `ad_comment` text COLLATE utf8mb4_unicode_ci,
  `ad_location` varchar(255) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `ad_managed_by` varchar(500) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `ad_when_changed` timestamp NULL DEFAULT NULL,
  `ad_pwd_last_set` timestamp NULL DEFAULT NULL,
  `ad_service_principal_names` text COLLATE utf8mb4_unicode_ci,
  `owner_json` json DEFAULT NULL COMMENT 'Sorumlu kişi bilgileri (display_name, mail, department, title, sam_account, upn, managed_by_dn)',
  `discovery_methods` text COLLATE utf8mb4_unicode_ci COMMENT 'Kullanılan keşif metotları (JSON array)',
  `discovered_at` datetime DEFAULT NULL COMMENT 'İlk keşfedilme zamanı',
  PRIMARY KEY (`id`),
  UNIQUE KEY `idx_asset_tag` (`asset_tag`),
  UNIQUE KEY `idx_ad_object_guid` (`ad_object_guid`),
  KEY `idx_mac` (`mac_address`),
  KEY `idx_ip` (`ip_address`),
  KEY `idx_status` (`status`),
  KEY `idx_type` (`asset_type`),
  KEY `idx_location` (`location`),
  KEY `idx_target` (`target_id`),
  KEY `idx_warranty` (`warranty_expiry`),
  KEY `idx_inventory_active_username` (`active_username`),
  CONSTRAINT `fk_inventory_target` FOREIGN KEY (`target_id`) REFERENCES `targets` (`id`) ON DELETE SET NULL
) ENGINE=InnoDB AUTO_INCREMENT=50 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ----- Tablo: inventory_scan_settings -----
DROP TABLE IF EXISTS `inventory_scan_settings`;
CREATE TABLE `inventory_scan_settings` (
  `inventory_id` int NOT NULL,
  `auto_software_scan` tinyint(1) NOT NULL DEFAULT '0' COMMENT '1=günlük otomatik tarama aktif',
  `scan_hour` tinyint NOT NULL DEFAULT '3' COMMENT 'Taramanın yapılacağı saat (0-23)',
  `last_auto_scan_at` datetime DEFAULT NULL COMMENT 'Son otomatik tarama zamanı',
  PRIMARY KEY (`inventory_id`),
  CONSTRAINT `fk_iss_inventory` FOREIGN KEY (`inventory_id`) REFERENCES `inventory` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ----- Tablo: inventory_scans -----
DROP TABLE IF EXISTS `inventory_scans`;
CREATE TABLE `inventory_scans` (
  `id` int NOT NULL AUTO_INCREMENT,
  `scan_type` enum('ad_discovery','winrm_enrich','snmp_enrich','ssh_enrich','ip_scan') COLLATE utf8mb4_unicode_ci NOT NULL,
  `status` enum('pending','running','completed','failed','cancelled') COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'pending',
  `total_targets` int DEFAULT '0' COMMENT 'Toplam hedef sayısı',
  `processed` int DEFAULT '0' COMMENT 'İşlenen sayısı',
  `success_count` int DEFAULT '0' COMMENT 'Başarılı',
  `error_count` int DEFAULT '0' COMMENT 'Hatalı',
  `new_assets` int DEFAULT '0' COMMENT 'Yeni eklenen varlık',
  `updated_assets` int DEFAULT '0' COMMENT 'Güncellenen varlık',
  `errors_json` json DEFAULT NULL COMMENT 'Hata detayları',
  `scan_config` json DEFAULT NULL COMMENT 'Tarama konfigürasyonu',
  `started_at` datetime DEFAULT NULL,
  `completed_at` datetime DEFAULT NULL,
  `created_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `started_by` varchar(100) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_scan_type` (`scan_type`),
  KEY `idx_status` (`status`),
  KEY `idx_created` (`created_at`)
) ENGINE=InnoDB AUTO_INCREMENT=6 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ----- Tablo: ip_alerts -----
DROP TABLE IF EXISTS `ip_alerts`;
CREATE TABLE `ip_alerts` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `subnet` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `ip` varchar(45) COLLATE utf8mb4_unicode_ci NOT NULL,
  `detected_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `acknowledged_at` datetime DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uniq_ip_alerts_subnet_ip` (`subnet`,`ip`),
  KEY `idx_ip_alerts_acknowledged_at` (`acknowledged_at`),
  KEY `idx_ip_alerts_detected_at` (`detected_at`)
) ENGINE=InnoDB AUTO_INCREMENT=21568 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ----- Tablo: ip_results -----
DROP TABLE IF EXISTS `ip_results`;
CREATE TABLE `ip_results` (
  `subnet` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `ip` varchar(45) COLLATE utf8mb4_unicode_ci NOT NULL,
  `created_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`subnet`,`ip`),
  KEY `idx_ip_results_created_at` (`created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ----- Tablo: kms_trusted_hosts -----
DROP TABLE IF EXISTS `kms_trusted_hosts`;
CREATE TABLE `kms_trusted_hosts` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `host` varchar(255) COLLATE utf8mb4_unicode_ci NOT NULL,
  `label` varchar(120) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `is_enabled` tinyint(1) NOT NULL DEFAULT '1',
  `created_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_kms_trusted_hosts_host` (`host`),
  KEY `idx_kms_trusted_hosts_enabled` (`is_enabled`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ----- Tablo: liquid_sensor_history -----
DROP TABLE IF EXISTS `liquid_sensor_history`;
CREATE TABLE `liquid_sensor_history` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `sensor_serial` varchar(64) COLLATE utf8mb4_general_ci NOT NULL,
  `status_text` varchar(128) COLLATE utf8mb4_general_ci NOT NULL,
  `analog_value` int DEFAULT NULL,
  `contact_detected` tinyint(1) NOT NULL DEFAULT '0',
  `recorded_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `raw_payload` mediumtext COLLATE utf8mb4_general_ci,
  PRIMARY KEY (`id`),
  KEY `idx_liquid_sensor_history_serial_time` (`sensor_serial`,`recorded_at`)
) ENGINE=MyISAM AUTO_INCREMENT=43 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

-- ----- Tablo: module_permissions -----
DROP TABLE IF EXISTS `module_permissions`;
CREATE TABLE `module_permissions` (
  `id` int NOT NULL AUTO_INCREMENT,
  `user_id` int NOT NULL,
  `module_name` varchar(50) COLLATE utf8mb4_general_ci NOT NULL,
  `can_view` tinyint(1) DEFAULT '0',
  `can_edit` tinyint(1) DEFAULT '0',
  `created_at` timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` timestamp NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `unique_user_module` (`user_id`,`module_name`)
) ENGINE=MyISAM AUTO_INCREMENT=81 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

-- ----- Tablo: new_notification_config -----
DROP TABLE IF EXISTS `new_notification_config`;
CREATE TABLE `new_notification_config` (
  `id` int NOT NULL AUTO_INCREMENT,
  `channel` enum('email','telegram') COLLATE utf8mb4_unicode_ci NOT NULL,
  `config` json NOT NULL,
  `is_enabled` tinyint(1) NOT NULL DEFAULT '0',
  `created_at` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_new_notif_config_channel` (`channel`),
  KEY `idx_new_notif_config_channel` (`channel`),
  KEY `idx_new_notif_config_enabled` (`is_enabled`)
) ENGINE=InnoDB AUTO_INCREMENT=10 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ----- Tablo: new_notification_mails -----
DROP TABLE IF EXISTS `new_notification_mails`;
CREATE TABLE `new_notification_mails` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `rule_id` int DEFAULT NULL,
  `target_id` int DEFAULT NULL,
  `channel` enum('email','telegram') COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'email',
  `template_id` int DEFAULT NULL,
  `recipients` json NOT NULL,
  `subject` varchar(500) COLLATE utf8mb4_unicode_ci NOT NULL,
  `body` text COLLATE utf8mb4_unicode_ci NOT NULL,
  `status` enum('sent','failed') COLLATE utf8mb4_unicode_ci NOT NULL,
  `error` text COLLATE utf8mb4_unicode_ci,
  `sent_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  KEY `idx_rule` (`rule_id`),
  KEY `idx_target` (`target_id`),
  KEY `idx_sent` (`sent_at`),
  KEY `idx_new_notification_mails_template` (`template_id`),
  CONSTRAINT `fk_new_notification_mails_template` FOREIGN KEY (`template_id`) REFERENCES `new_notification_templates` (`id`) ON DELETE SET NULL
) ENGINE=InnoDB AUTO_INCREMENT=27053 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ----- Tablo: new_notification_rules -----
DROP TABLE IF EXISTS `new_notification_rules`;
CREATE TABLE `new_notification_rules` (
  `id` int NOT NULL AUTO_INCREMENT,
  `name` varchar(255) COLLATE utf8mb4_unicode_ci NOT NULL,
  `target_id` int DEFAULT NULL,
  `sensor_serial` varchar(100) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `inventory_id` int DEFAULT NULL,
  `entity_type` enum('target','sensor','crack_detection') COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'target',
  `channel` enum('email','telegram') COLLATE utf8mb4_unicode_ci NOT NULL,
  `schedule_interval_minutes` int NOT NULL DEFAULT '5',
  `recipients` json NOT NULL,
  `template_id` int DEFAULT NULL,
  `conditions` json DEFAULT NULL,
  `is_active` tinyint(1) NOT NULL DEFAULT '1',
  `created_by` int DEFAULT NULL,
  `last_sent_at` datetime DEFAULT NULL,
  `last_sent_status` enum('success','fail') COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT 'detects state changes (fail→success)',
  `created_at` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  KEY `idx_new_rules_active` (`is_active`),
  KEY `idx_new_rules_channel` (`channel`),
  KEY `idx_new_rules_created` (`created_at`),
  KEY `fk_new_rules_template` (`template_id`),
  KEY `idx_new_rules_target` (`target_id`),
  KEY `idx_notification_rules_status` (`last_sent_status`),
  CONSTRAINT `fk_new_rules_target` FOREIGN KEY (`target_id`) REFERENCES `targets` (`id`) ON DELETE SET NULL,
  CONSTRAINT `fk_new_rules_template` FOREIGN KEY (`template_id`) REFERENCES `new_notification_templates` (`id`) ON DELETE SET NULL
) ENGINE=InnoDB AUTO_INCREMENT=265 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ----- Tablo: new_notification_settings -----
DROP TABLE IF EXISTS `new_notification_settings`;
CREATE TABLE `new_notification_settings` (
  `id` int NOT NULL AUTO_INCREMENT,
  `user_id` int NOT NULL,
  `email_enabled` tinyint(1) NOT NULL DEFAULT '0',
  `email_recipient` varchar(255) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `telegram_enabled` tinyint(1) NOT NULL DEFAULT '0',
  `telegram_chat_id` varchar(100) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `types` json DEFAULT NULL,
  `created_at` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_new_notif_settings_user` (`user_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ----- Tablo: new_notification_templates -----
DROP TABLE IF EXISTS `new_notification_templates`;
CREATE TABLE `new_notification_templates` (
  `id` int NOT NULL AUTO_INCREMENT,
  `name` varchar(255) COLLATE utf8mb4_unicode_ci NOT NULL,
  `type` enum('target') COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'target',
  `channel` enum('email','telegram','whatsapp','webhook','all') COLLATE utf8mb4_unicode_ci NOT NULL,
  `subject` varchar(500) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `body` text COLLATE utf8mb4_unicode_ci NOT NULL,
  `variables` json DEFAULT NULL,
  `is_default` tinyint(1) NOT NULL DEFAULT '0',
  `is_active` tinyint(1) NOT NULL DEFAULT '1',
  `user_id` int DEFAULT NULL,
  `created_at` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  KEY `idx_new_tmpl_type_channel` (`type`,`channel`),
  KEY `idx_new_tmpl_user` (`user_id`),
  KEY `idx_new_tmpl_active` (`is_active`)
) ENGINE=InnoDB AUTO_INCREMENT=8 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ----- Tablo: ping_daily_stats -----
DROP TABLE IF EXISTS `ping_daily_stats`;
CREATE TABLE `ping_daily_stats` (
  `target_id` int NOT NULL,
  `day` date NOT NULL,
  `total` int NOT NULL DEFAULT '0',
  `successful` int NOT NULL DEFAULT '0',
  `failed` int NOT NULL DEFAULT '0',
  `sum_rtt_ms` bigint NOT NULL DEFAULT '0',
  `rtt_count` int NOT NULL DEFAULT '0',
  `min_rtt_ms` int DEFAULT NULL,
  `max_rtt_ms` int DEFAULT NULL,
  `incidents` int NOT NULL DEFAULT '0',
  `downtime_ms` bigint NOT NULL DEFAULT '0',
  `updated_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`target_id`,`day`),
  KEY `idx_pds_day` (`day`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ----- Tablo: pings_raw -----
DROP TABLE IF EXISTS `pings_raw`;
CREATE TABLE `pings_raw` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `target_id` int NOT NULL,
  `ts_ms` bigint NOT NULL,
  `ok` tinyint(1) NOT NULL,
  `rtt_ms` int DEFAULT NULL,
  `response_status_code` int DEFAULT NULL,
  `response_size_bytes` int DEFAULT NULL,
  `ssl_expiry_date` datetime DEFAULT NULL,
  `response_headers` json DEFAULT NULL,
  `error_msg` varchar(255) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `created_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  KEY `idx_pings_target_ts` (`target_id`,`ts_ms`),
  KEY `idx_pings_ts` (`ts_ms`),
  KEY `idx_pings_ok` (`ok`),
  KEY `idx_pings_status_code` (`response_status_code`),
  CONSTRAINT `pings_raw_ibfk_1` FOREIGN KEY (`target_id`) REFERENCES `targets` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB AUTO_INCREMENT=1438047 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ----- Tablo: pings_raw_momentary -----
DROP TABLE IF EXISTS `pings_raw_momentary`;
CREATE TABLE `pings_raw_momentary` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `target_id` int NOT NULL,
  `ts_ms` bigint NOT NULL,
  `ok` tinyint(1) NOT NULL,
  `rtt_ms` int DEFAULT NULL,
  `error_msg` varchar(255) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `response_status_code` int DEFAULT NULL,
  `response_size_bytes` int DEFAULT NULL,
  `ssl_expiry_date` datetime DEFAULT NULL,
  `response_headers` json DEFAULT NULL,
  `created_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  KEY `idx_momentary_created` (`created_at`),
  KEY `idx_momentary_target_created` (`target_id`,`created_at`),
  KEY `idx_momentary_ok` (`ok`),
  KEY `idx_momentary_status_code` (`response_status_code`),
  CONSTRAINT `pings_raw_momentary_ibfk_1` FOREIGN KEY (`target_id`) REFERENCES `targets` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB AUTO_INCREMENT=1124447 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ----- Tablo: reports -----
DROP TABLE IF EXISTS `reports`;
CREATE TABLE `reports` (
  `id` int NOT NULL AUTO_INCREMENT,
  `type` varchar(50) COLLATE utf8mb4_general_ci NOT NULL,
  `title` varchar(255) COLLATE utf8mb4_general_ci NOT NULL,
  `format` varchar(20) COLLATE utf8mb4_general_ci NOT NULL,
  `status` enum('generating','completed','failed') COLLATE utf8mb4_general_ci DEFAULT 'generating',
  `url` varchar(500) COLLATE utf8mb4_general_ci DEFAULT NULL,
  `config` json DEFAULT NULL,
  `created_at` datetime DEFAULT CURRENT_TIMESTAMP,
  `updated_at` datetime DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`)
) ENGINE=MyISAM AUTO_INCREMENT=1784294088 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

-- ----- Tablo: sensor_hourly -----
DROP TABLE IF EXISTS `sensor_hourly`;
CREATE TABLE `sensor_hourly` (
  `id` int NOT NULL AUTO_INCREMENT,
  `hour_ts` datetime NOT NULL,
  `temperature` float DEFAULT NULL,
  `cpu_temperature` float DEFAULT NULL,
  `humidity` float DEFAULT NULL,
  `pressure` float DEFAULT NULL,
  `gas` float DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_hour` (`hour_ts`)
) ENGINE=MyISAM AUTO_INCREMENT=303 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

-- ----- Tablo: server_status -----
DROP TABLE IF EXISTS `server_status`;
CREATE TABLE `server_status` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `target_id` int NOT NULL,
  `collected_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `cpu_percent` decimal(5,2) DEFAULT NULL,
  `ram_gb` decimal(10,2) DEFAULT NULL,
  `disk_gb` decimal(10,2) DEFAULT NULL,
  `temperature_c` decimal(6,2) DEFAULT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_server_status_target_time` (`target_id`,`collected_at`)
) ENGINE=MyISAM AUTO_INCREMENT=68865 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

-- ----- Tablo: settings -----
DROP TABLE IF EXISTS `settings`;
CREATE TABLE `settings` (
  `k` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `v` text COLLATE utf8mb4_unicode_ci NOT NULL,
  `updated_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`k`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ----- Tablo: snmp_settings -----
DROP TABLE IF EXISTS `snmp_settings`;
CREATE TABLE `snmp_settings` (
  `id` int NOT NULL AUTO_INCREMENT,
  `version` enum('v1','v2c','v3') COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'v2c',
  `community_strings` text COLLATE utf8mb4_unicode_ci NOT NULL COMMENT 'Denenecek community string listesi (JSON array)',
  `v3_username` varchar(255) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `v3_auth_protocol` enum('MD5','SHA') COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `v3_auth_password_encrypted` text COLLATE utf8mb4_unicode_ci,
  `v3_priv_protocol` enum('DES','AES') COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `v3_priv_password_encrypted` text COLLATE utf8mb4_unicode_ci,
  `port` int NOT NULL DEFAULT '161',
  `timeout_seconds` int NOT NULL DEFAULT '5',
  `retry_count` int NOT NULL DEFAULT '2',
  `concurrent_limit` int NOT NULL DEFAULT '10',
  `is_enabled` tinyint(1) NOT NULL DEFAULT '0',
  `last_test_at` datetime DEFAULT NULL COMMENT 'Son test zamanı',
  `last_test_result` enum('success','failed') COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT 'Son test sonucu',
  `last_test_error` text COLLATE utf8mb4_unicode_ci COMMENT 'Son test hatası',
  `last_test_ip` varchar(45) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT 'Son test edilen IP',
  `created_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB AUTO_INCREMENT=2 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ----- Tablo: software_licenses -----
DROP TABLE IF EXISTS `software_licenses`;
CREATE TABLE `software_licenses` (
  `id` int NOT NULL AUTO_INCREMENT,
  `inventory_id` int NOT NULL,
  `software_name` varchar(512) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT 'software_json içindeki Name ile eşleşir',
  `purchase_cost` decimal(12,2) DEFAULT NULL COMMENT 'Satın alma maliyeti',
  `cost_currency` varchar(8) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'TRY' COMMENT 'Para birimi (TRY/EUR/USD)',
  `purchase_date` date DEFAULT NULL COMMENT 'Satın alma tarihi',
  `license_expiry_date` date DEFAULT NULL COMMENT 'Lisans bitiş tarihi',
  `notes` text COLLATE utf8mb4_unicode_ci COMMENT 'Notlar',
  `created_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_inv_software` (`inventory_id`,`software_name`(255)),
  CONSTRAINT `fk_sl_inventory` FOREIGN KEY (`inventory_id`) REFERENCES `inventory` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB AUTO_INCREMENT=4 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ----- Tablo: software_scan_history -----
DROP TABLE IF EXISTS `software_scan_history`;
CREATE TABLE `software_scan_history` (
  `id` int NOT NULL AUTO_INCREMENT,
  `inventory_id` int NOT NULL,
  `scanned_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `software_name` varchar(512) COLLATE utf8mb4_unicode_ci NOT NULL,
  `publisher` varchar(256) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `change_type` enum('added','removed','version_changed') COLLATE utf8mb4_unicode_ci NOT NULL,
  `new_version` varchar(256) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT 'Yeni versiyon (added veya version_changed)',
  `prev_version` varchar(256) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT 'Önceki versiyon (removed veya version_changed)',
  PRIMARY KEY (`id`),
  KEY `idx_inv_scanned_at` (`inventory_id`,`scanned_at`),
  KEY `idx_inv_name` (`inventory_id`,`software_name`(255)),
  CONSTRAINT `fk_ssh_inventory` FOREIGN KEY (`inventory_id`) REFERENCES `inventory` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ----- Tablo: ssh_settings -----
DROP TABLE IF EXISTS `ssh_settings`;
CREATE TABLE `ssh_settings` (
  `id` int NOT NULL AUTO_INCREMENT,
  `username` varchar(255) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'root' COMMENT 'SSH kullanıcı adı',
  `password_encrypted` text COLLATE utf8mb4_unicode_ci COMMENT 'Şifrelenmiş SSH şifresi',
  `port` int NOT NULL DEFAULT '22' COMMENT 'SSH portu',
  `timeout_seconds` int NOT NULL DEFAULT '10' COMMENT 'Bağlantı timeout süresi (saniye)',
  `is_enabled` tinyint(1) NOT NULL DEFAULT '0' COMMENT 'SSH discovery aktif mi?',
  `created_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB AUTO_INCREMENT=2 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ----- Tablo: target_credentials -----
DROP TABLE IF EXISTS `target_credentials`;
CREATE TABLE `target_credentials` (
  `id` int NOT NULL AUTO_INCREMENT,
  `target_id` int NOT NULL,
  `os_type` enum('windows','linux') COLLATE utf8mb4_unicode_ci NOT NULL COMMENT 'Operating system type',
  `protocol` enum('winrm','ssh') COLLATE utf8mb4_unicode_ci NOT NULL COMMENT 'Connection protocol',
  `username` varchar(255) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT 'Username for authentication',
  `password_encrypted` text COLLATE utf8mb4_unicode_ci NOT NULL COMMENT 'AES-256 encrypted password',
  `port` int DEFAULT NULL COMMENT 'Custom port (default: WinRM=5985, SSH=22)',
  `domain` varchar(255) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT 'Windows domain (optional, for WinRM)',
  `last_test_at` datetime DEFAULT NULL COMMENT 'Last successful connection test',
  `last_test_success` tinyint(1) DEFAULT NULL COMMENT 'Last test result',
  `created_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `target_id` (`target_id`),
  KEY `idx_target_credentials_os_type` (`os_type`),
  KEY `idx_target_credentials_updated` (`updated_at`),
  CONSTRAINT `target_credentials_ibfk_1` FOREIGN KEY (`target_id`) REFERENCES `targets` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB AUTO_INCREMENT=21 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ----- Tablo: targets -----
DROP TABLE IF EXISTS `targets`;
CREATE TABLE `targets` (
  `id` int NOT NULL AUTO_INCREMENT,
  `name` varchar(128) COLLATE utf8mb4_unicode_ci NOT NULL,
  `address` varchar(255) COLLATE utf8mb4_unicode_ci NOT NULL,
  `type` enum('icmp','tcp','http','https') COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'icmp',
  `monitoring_type` enum('ping','http','https') COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'ping',
  `http_method` enum('GET','POST','PUT','DELETE','HEAD','OPTIONS') COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'GET',
  `http_path` varchar(255) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '/',
  `http_headers` json DEFAULT NULL,
  `expected_status_code` int NOT NULL DEFAULT '200',
  `expected_content` text COLLATE utf8mb4_unicode_ci,
  `ssl_check` tinyint(1) NOT NULL DEFAULT '0',
  `follow_redirects` tinyint(1) NOT NULL DEFAULT '1',
  `timeout_sec` int NOT NULL DEFAULT '10',
  `port` int DEFAULT NULL,
  `path` varchar(255) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `interval_sec` int NOT NULL DEFAULT '120' COMMENT 'Sabit 2 dakika',
  `timeout_ms` int NOT NULL DEFAULT '1000',
  `enabled` tinyint(1) NOT NULL DEFAULT '1',
  `metrics_enabled` tinyint(1) NOT NULL DEFAULT '0',
  `snmp_community` varchar(64) COLLATE utf8mb4_unicode_ci DEFAULT 'public',
  `snmp_version` enum('v1','v2c','v3') COLLATE utf8mb4_unicode_ci DEFAULT 'v2c',
  `tags` varchar(255) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `created_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  `user_id` int DEFAULT '1',
  PRIMARY KEY (`id`),
  KEY `idx_targets_enabled_type` (`enabled`,`type`),
  KEY `idx_targets_address` (`address`),
  KEY `idx_targets_tags` (`tags`),
  KEY `idx_targets_monitoring_type` (`monitoring_type`),
  KEY `fk_targets_user` (`user_id`),
  CONSTRAINT `fk_targets_user` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`) ON DELETE SET NULL
) ENGINE=InnoDB AUTO_INCREMENT=2953 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ----- Tablo: users -----
DROP TABLE IF EXISTS `users`;
CREATE TABLE `users` (
  `id` int NOT NULL AUTO_INCREMENT,
  `email` varchar(255) COLLATE utf8mb4_unicode_ci NOT NULL,
  `pass_hash` varchar(255) COLLATE utf8mb4_unicode_ci NOT NULL,
  `role` enum('admin','user','viewer') COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'admin',
  `limits_json` json DEFAULT NULL,
  `created_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  `ip_query_limit` int DEFAULT '10' COMMENT 'Maximum IP queries per day',
  `max_targets` int DEFAULT '10',
  `is_active` tinyint(1) DEFAULT '1',
  `license_expiry` datetime DEFAULT NULL,
  `last_login_at` datetime DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `email` (`email`)
) ENGINE=InnoDB AUTO_INCREMENT=10 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ----- Tablo: winrm_settings -----
DROP TABLE IF EXISTS `winrm_settings`;
CREATE TABLE `winrm_settings` (
  `id` int NOT NULL AUTO_INCREMENT,
  `username` varchar(255) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT 'WinRM kullanıcısı (domain\\admin)',
  `password_encrypted` text COLLATE utf8mb4_unicode_ci NOT NULL COMMENT 'Şifrelenmiş parola',
  `port` int NOT NULL DEFAULT '5985' COMMENT 'WinRM port (5985 HTTP, 5986 HTTPS)',
  `use_ssl` tinyint(1) NOT NULL DEFAULT '0',
  `timeout_seconds` int NOT NULL DEFAULT '30',
  `concurrent_limit` int NOT NULL DEFAULT '5' COMMENT 'Paralel bağlantı limiti',
  `retry_count` int NOT NULL DEFAULT '2',
  `is_enabled` tinyint(1) NOT NULL DEFAULT '0',
  `last_test_at` datetime DEFAULT NULL,
  `last_test_result` enum('success','failed') COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `last_test_error` text COLLATE utf8mb4_unicode_ci,
  `last_test_ip` varchar(45) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT 'Son test edilen IP',
  `created_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB AUTO_INCREMENT=2 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ----- Tablo: winrm_standalone_hosts -----
DROP TABLE IF EXISTS `winrm_standalone_hosts`;
CREATE TABLE `winrm_standalone_hosts` (
  `id` int NOT NULL AUTO_INCREMENT,
  `ip_address` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT 'Hedef IP veya hostname',
  `label` varchar(255) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT 'Kullanıcı tanımlı açıklama (opsiyonel)',
  `username` varchar(255) COLLATE utf8mb4_unicode_ci NOT NULL,
  `password_encrypted` text COLLATE utf8mb4_unicode_ci NOT NULL,
  `port` int NOT NULL DEFAULT '5985',
  `use_ssl` tinyint(1) NOT NULL DEFAULT '0',
  `is_enabled` tinyint(1) NOT NULL DEFAULT '1',
  `last_seen_at` datetime DEFAULT NULL COMMENT 'Son başarılı bağlantı zamanı',
  `created_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB AUTO_INCREMENT=2 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ---------- SEED (default) VERİ ----------
-- module_permissions (56 satır)
INSERT INTO `module_permissions` (`id`, `user_id`, `module_name`, `can_view`, `can_edit`, `created_at`, `updated_at`) VALUES (1, 1, 'dashboard', 1, 1, '2025-10-14 11:46:15', '2025-10-16 10:25:05');
INSERT INTO `module_permissions` (`id`, `user_id`, `module_name`, `can_view`, `can_edit`, `created_at`, `updated_at`) VALUES (2, 1, 'targets', 1, 1, '2025-10-14 11:46:15', '2025-11-18 14:05:57');
INSERT INTO `module_permissions` (`id`, `user_id`, `module_name`, `can_view`, `can_edit`, `created_at`, `updated_at`) VALUES (51, 2, 'backup', 0, 0, '2025-11-18 14:54:13', '2025-11-18 14:54:13');
INSERT INTO `module_permissions` (`id`, `user_id`, `module_name`, `can_view`, `can_edit`, `created_at`, `updated_at`) VALUES (50, 5, 'backup', 0, 0, '2025-11-18 14:54:13', '2025-11-18 14:54:13');
INSERT INTO `module_permissions` (`id`, `user_id`, `module_name`, `can_view`, `can_edit`, `created_at`, `updated_at`) VALUES (49, 2, 'module_permissions', 0, 0, '2025-11-18 14:54:13', '2025-11-18 14:54:13');
INSERT INTO `module_permissions` (`id`, `user_id`, `module_name`, `can_view`, `can_edit`, `created_at`, `updated_at`) VALUES (48, 5, 'module_permissions', 0, 0, '2025-11-18 14:54:13', '2025-11-18 14:54:13');
INSERT INTO `module_permissions` (`id`, `user_id`, `module_name`, `can_view`, `can_edit`, `created_at`, `updated_at`) VALUES (9, 1, 'reporting', 1, 1, '2025-10-14 11:46:15', '2025-10-16 10:25:05');
INSERT INTO `module_permissions` (`id`, `user_id`, `module_name`, `can_view`, `can_edit`, `created_at`, `updated_at`) VALUES (47, 2, 'user_management', 0, 0, '2025-11-18 14:54:13', '2025-11-18 14:54:13');
INSERT INTO `module_permissions` (`id`, `user_id`, `module_name`, `can_view`, `can_edit`, `created_at`, `updated_at`) VALUES (11, 1, 'notifications', 1, 1, '2025-10-14 11:46:15', '2025-10-16 10:25:05');
INSERT INTO `module_permissions` (`id`, `user_id`, `module_name`, `can_view`, `can_edit`, `created_at`, `updated_at`) VALUES (46, 5, 'user_management', 0, 0, '2025-11-18 14:54:13', '2025-11-18 14:54:13');
INSERT INTO `module_permissions` (`id`, `user_id`, `module_name`, `can_view`, `can_edit`, `created_at`, `updated_at`) VALUES (13, 2, 'dashboard', 1, 0, '2025-10-27 12:34:14', '2025-11-18 14:54:13');
INSERT INTO `module_permissions` (`id`, `user_id`, `module_name`, `can_view`, `can_edit`, `created_at`, `updated_at`) VALUES (14, 2, 'targets', 1, 1, '2025-10-27 12:34:14', '2025-10-27 12:34:23');
INSERT INTO `module_permissions` (`id`, `user_id`, `module_name`, `can_view`, `can_edit`, `created_at`, `updated_at`) VALUES (45, 2, 'notification_settings', 1, 0, '2025-11-18 14:54:13', '2025-11-18 14:54:13');
INSERT INTO `module_permissions` (`id`, `user_id`, `module_name`, `can_view`, `can_edit`, `created_at`, `updated_at`) VALUES (44, 5, 'notification_settings', 1, 0, '2025-11-18 14:54:13', '2025-11-18 14:54:13');
INSERT INTO `module_permissions` (`id`, `user_id`, `module_name`, `can_view`, `can_edit`, `created_at`, `updated_at`) VALUES (43, 2, 'ip_scanner', 1, 1, '2025-11-18 14:54:13', '2025-11-18 14:54:13');
INSERT INTO `module_permissions` (`id`, `user_id`, `module_name`, `can_view`, `can_edit`, `created_at`, `updated_at`) VALUES (21, 2, 'reporting', 1, 0, '2025-10-27 12:34:14', '2025-11-18 14:54:13');
INSERT INTO `module_permissions` (`id`, `user_id`, `module_name`, `can_view`, `can_edit`, `created_at`, `updated_at`) VALUES (42, 5, 'ip_scanner', 1, 1, '2025-11-18 14:54:13', '2025-11-18 14:54:13');
INSERT INTO `module_permissions` (`id`, `user_id`, `module_name`, `can_view`, `can_edit`, `created_at`, `updated_at`) VALUES (23, 2, 'notifications', 1, 0, '2025-10-27 12:34:14', '2025-11-18 14:54:13');
INSERT INTO `module_permissions` (`id`, `user_id`, `module_name`, `can_view`, `can_edit`, `created_at`, `updated_at`) VALUES (41, 1, 'backup', 1, 1, '2025-11-18 14:54:13', '2025-11-18 14:54:13');
INSERT INTO `module_permissions` (`id`, `user_id`, `module_name`, `can_view`, `can_edit`, `created_at`, `updated_at`) VALUES (25, 5, 'dashboard', 1, 0, '2025-11-06 18:54:59', '2025-11-06 18:54:59');
INSERT INTO `module_permissions` (`id`, `user_id`, `module_name`, `can_view`, `can_edit`, `created_at`, `updated_at`) VALUES (26, 5, 'targets', 1, 1, '2025-11-06 18:54:59', '2025-11-18 14:54:13');
INSERT INTO `module_permissions` (`id`, `user_id`, `module_name`, `can_view`, `can_edit`, `created_at`, `updated_at`) VALUES (40, 1, 'module_permissions', 1, 1, '2025-11-18 14:54:13', '2025-11-18 14:54:13');
INSERT INTO `module_permissions` (`id`, `user_id`, `module_name`, `can_view`, `can_edit`, `created_at`, `updated_at`) VALUES (39, 1, 'user_management', 1, 1, '2025-11-18 14:54:13', '2025-11-18 14:54:13');
INSERT INTO `module_permissions` (`id`, `user_id`, `module_name`, `can_view`, `can_edit`, `created_at`, `updated_at`) VALUES (38, 1, 'notification_settings', 1, 1, '2025-11-18 14:54:13', '2025-11-18 14:54:13');
INSERT INTO `module_permissions` (`id`, `user_id`, `module_name`, `can_view`, `can_edit`, `created_at`, `updated_at`) VALUES (33, 5, 'reporting', 1, 0, '2025-11-06 18:54:59', '2025-11-18 14:54:13');
INSERT INTO `module_permissions` (`id`, `user_id`, `module_name`, `can_view`, `can_edit`, `created_at`, `updated_at`) VALUES (35, 5, 'notifications', 1, 0, '2025-11-06 18:54:59', '2025-11-18 14:54:13');
INSERT INTO `module_permissions` (`id`, `user_id`, `module_name`, `can_view`, `can_edit`, `created_at`, `updated_at`) VALUES (37, 1, 'ip_scanner', 1, 1, '2025-11-18 14:54:13', '2025-11-18 14:54:13');
INSERT INTO `module_permissions` (`id`, `user_id`, `module_name`, `can_view`, `can_edit`, `created_at`, `updated_at`) VALUES (52, 3, 'dashboard', 1, 0, '2025-11-18 14:54:13', '2025-11-18 14:54:13');
INSERT INTO `module_permissions` (`id`, `user_id`, `module_name`, `can_view`, `can_edit`, `created_at`, `updated_at`) VALUES (53, 3, 'targets', 1, 0, '2025-11-18 14:54:13', '2025-11-18 14:54:13');
INSERT INTO `module_permissions` (`id`, `user_id`, `module_name`, `can_view`, `can_edit`, `created_at`, `updated_at`) VALUES (54, 3, 'ip_scanner', 1, 0, '2025-11-18 14:54:13', '2025-11-18 14:54:13');
INSERT INTO `module_permissions` (`id`, `user_id`, `module_name`, `can_view`, `can_edit`, `created_at`, `updated_at`) VALUES (55, 3, 'notifications', 1, 0, '2025-11-18 14:54:13', '2025-11-18 14:54:13');
INSERT INTO `module_permissions` (`id`, `user_id`, `module_name`, `can_view`, `can_edit`, `created_at`, `updated_at`) VALUES (56, 3, 'notification_settings', 1, 0, '2025-11-18 14:54:13', '2025-11-18 14:54:13');
INSERT INTO `module_permissions` (`id`, `user_id`, `module_name`, `can_view`, `can_edit`, `created_at`, `updated_at`) VALUES (57, 3, 'reporting', 1, 0, '2025-11-18 14:54:13', '2025-11-18 14:54:13');
INSERT INTO `module_permissions` (`id`, `user_id`, `module_name`, `can_view`, `can_edit`, `created_at`, `updated_at`) VALUES (58, 3, 'user_management', 0, 0, '2025-11-18 14:54:13', '2025-11-18 14:54:13');
INSERT INTO `module_permissions` (`id`, `user_id`, `module_name`, `can_view`, `can_edit`, `created_at`, `updated_at`) VALUES (59, 3, 'module_permissions', 0, 0, '2025-11-18 14:54:13', '2025-11-18 14:54:13');
INSERT INTO `module_permissions` (`id`, `user_id`, `module_name`, `can_view`, `can_edit`, `created_at`, `updated_at`) VALUES (60, 3, 'backup', 0, 0, '2025-11-18 14:54:13', '2025-11-18 14:54:13');
INSERT INTO `module_permissions` (`id`, `user_id`, `module_name`, `can_view`, `can_edit`, `created_at`, `updated_at`) VALUES (61, 6, 'dashboard', 1, 0, '2025-11-18 15:18:28', '2025-11-18 15:18:28');
INSERT INTO `module_permissions` (`id`, `user_id`, `module_name`, `can_view`, `can_edit`, `created_at`, `updated_at`) VALUES (62, 6, 'targets', 1, 0, '2025-11-18 15:18:28', '2025-11-19 17:43:22');
INSERT INTO `module_permissions` (`id`, `user_id`, `module_name`, `can_view`, `can_edit`, `created_at`, `updated_at`) VALUES (63, 6, 'ip_scanner', 1, 0, '2025-11-18 15:18:28', '2025-11-19 17:43:22');
INSERT INTO `module_permissions` (`id`, `user_id`, `module_name`, `can_view`, `can_edit`, `created_at`, `updated_at`) VALUES (64, 6, 'notifications', 1, 0, '2025-11-18 15:18:28', '2025-11-19 17:43:25');
INSERT INTO `module_permissions` (`id`, `user_id`, `module_name`, `can_view`, `can_edit`, `created_at`, `updated_at`) VALUES (65, 6, 'notification_settings', 1, 0, '2025-11-18 15:18:28', '2025-11-19 17:43:22');
INSERT INTO `module_permissions` (`id`, `user_id`, `module_name`, `can_view`, `can_edit`, `created_at`, `updated_at`) VALUES (66, 6, 'reporting', 1, 0, '2025-11-18 15:18:28', '2025-11-19 17:43:11');
INSERT INTO `module_permissions` (`id`, `user_id`, `module_name`, `can_view`, `can_edit`, `created_at`, `updated_at`) VALUES (67, 6, 'user_management', 0, 0, '2025-11-18 15:18:28', '2025-11-19 10:32:54');
INSERT INTO `module_permissions` (`id`, `user_id`, `module_name`, `can_view`, `can_edit`, `created_at`, `updated_at`) VALUES (68, 6, 'module_permissions', 0, 0, '2025-11-18 15:18:28', '2025-11-19 10:32:54');
INSERT INTO `module_permissions` (`id`, `user_id`, `module_name`, `can_view`, `can_edit`, `created_at`, `updated_at`) VALUES (69, 6, 'backup', 1, 1, '2025-11-18 15:18:28', '2025-11-20 09:10:45');
INSERT INTO `module_permissions` (`id`, `user_id`, `module_name`, `can_view`, `can_edit`, `created_at`, `updated_at`) VALUES (70, 7, 'dashboard', 1, 0, '2026-01-27 17:59:07', '2026-01-27 17:59:07');
INSERT INTO `module_permissions` (`id`, `user_id`, `module_name`, `can_view`, `can_edit`, `created_at`, `updated_at`) VALUES (71, 7, 'targets', 1, 1, '2026-01-27 17:59:07', '2026-01-27 17:59:07');
INSERT INTO `module_permissions` (`id`, `user_id`, `module_name`, `can_view`, `can_edit`, `created_at`, `updated_at`) VALUES (72, 7, 'ip_scanner', 1, 1, '2026-01-27 17:59:07', '2026-01-27 17:59:07');
INSERT INTO `module_permissions` (`id`, `user_id`, `module_name`, `can_view`, `can_edit`, `created_at`, `updated_at`) VALUES (73, 7, 'inventory', 1, 1, '2026-01-27 17:59:07', '2026-01-27 17:59:07');
INSERT INTO `module_permissions` (`id`, `user_id`, `module_name`, `can_view`, `can_edit`, `created_at`, `updated_at`) VALUES (74, 7, 'notifications', 1, 0, '2026-01-27 17:59:07', '2026-01-27 17:59:07');
INSERT INTO `module_permissions` (`id`, `user_id`, `module_name`, `can_view`, `can_edit`, `created_at`, `updated_at`) VALUES (75, 7, 'notification_settings', 1, 0, '2026-01-27 17:59:07', '2026-01-27 17:59:07');
INSERT INTO `module_permissions` (`id`, `user_id`, `module_name`, `can_view`, `can_edit`, `created_at`, `updated_at`) VALUES (76, 7, 'reporting', 1, 0, '2026-01-27 17:59:07', '2026-01-27 17:59:07');
INSERT INTO `module_permissions` (`id`, `user_id`, `module_name`, `can_view`, `can_edit`, `created_at`, `updated_at`) VALUES (77, 7, 'user_management', 0, 0, '2026-01-27 17:59:07', '2026-01-27 17:59:07');
INSERT INTO `module_permissions` (`id`, `user_id`, `module_name`, `can_view`, `can_edit`, `created_at`, `updated_at`) VALUES (78, 7, 'module_permissions', 0, 0, '2026-01-27 17:59:07', '2026-01-27 17:59:07');
INSERT INTO `module_permissions` (`id`, `user_id`, `module_name`, `can_view`, `can_edit`, `created_at`, `updated_at`) VALUES (79, 7, 'backup', 0, 0, '2026-01-27 17:59:07', '2026-01-27 17:59:07');
INSERT INTO `module_permissions` (`id`, `user_id`, `module_name`, `can_view`, `can_edit`, `created_at`, `updated_at`) VALUES (80, 7, 'network_settings', 0, 0, '2026-01-27 17:59:07', '2026-01-27 17:59:07');

-- new_notification_config (2 satır)
INSERT INTO `new_notification_config` (`id`, `channel`, `config`, `is_enabled`, `created_at`, `updated_at`) VALUES (1, 'email', '{"use_tls": true, "password": "sixh zmdl bsza tunl ", "username": "kotancagdas@gmail.com", "from_name": "Systrack", "smtp_host": "smtp.gmail.com", "smtp_port": 587, "from_email": "kotancagdas@gmail.com"}', 1, '2025-10-29 13:30:47', '2026-02-04 16:50:46');
INSERT INTO `new_notification_config` (`id`, `channel`, `config`, `is_enabled`, `created_at`, `updated_at`) VALUES (9, 'telegram', '{"enabled": true, "bot_token": "8318140949:AAH5W-U81m2MwaC_AI6tRAL8YHzUqiiiex4", "webhook_url": "", "default_chat": ""}', 1, '2025-11-03 10:20:01', '2025-11-03 10:20:01');

-- settings (19 satır)
INSERT INTO `settings` (`k`, `v`, `updated_at`) VALUES ('access_allowlist_enabled', '0', '2026-02-10 09:14:40');
INSERT INTO `settings` (`k`, `v`, `updated_at`) VALUES ('dashboard_refresh_sec', '30', '2025-10-14 11:14:52');
INSERT INTO `settings` (`k`, `v`, `updated_at`) VALUES ('http_connect_timeout_sec', '5', '2025-10-14 11:15:38');
INSERT INTO `settings` (`k`, `v`, `updated_at`) VALUES ('http_max_redirects', '5', '2025-10-14 11:15:38');
INSERT INTO `settings` (`k`, `v`, `updated_at`) VALUES ('http_read_timeout_sec', '10', '2025-10-14 11:15:38');
INSERT INTO `settings` (`k`, `v`, `updated_at`) VALUES ('http_user_agent', 'SysTrack-Monitor/1.0', '2025-10-14 11:15:38');
INSERT INTO `settings` (`k`, `v`, `updated_at`) VALUES ('ping_archive_retention_days', '90', '2025-11-12 16:54:46');
INSERT INTO `settings` (`k`, `v`, `updated_at`) VALUES ('ping_interval_sec', '120', '2025-11-12 14:22:23');
INSERT INTO `settings` (`k`, `v`, `updated_at`) VALUES ('ping_momentary_retention_minutes', '10', '2025-11-12 14:22:23');
INSERT INTO `settings` (`k`, `v`, `updated_at`) VALUES ('ping_timeout_sec', '30', '2025-11-12 14:22:23');
INSERT INTO `settings` (`k`, `v`, `updated_at`) VALUES ('smtp_host', '', '2025-10-14 11:14:52');
INSERT INTO `settings` (`k`, `v`, `updated_at`) VALUES ('smtp_password', '', '2025-10-14 11:14:52');
INSERT INTO `settings` (`k`, `v`, `updated_at`) VALUES ('smtp_port', '587', '2025-10-14 11:14:52');
INSERT INTO `settings` (`k`, `v`, `updated_at`) VALUES ('smtp_username', '', '2025-10-14 11:14:52');
INSERT INTO `settings` (`k`, `v`, `updated_at`) VALUES ('ssl_warning_days', '30', '2025-10-14 11:15:38');
INSERT INTO `settings` (`k`, `v`, `updated_at`) VALUES ('telegram_bot_token', '', '2025-10-14 11:14:52');
INSERT INTO `settings` (`k`, `v`, `updated_at`) VALUES ('telegram_chat_id', '', '2025-10-14 11:14:52');
INSERT INTO `settings` (`k`, `v`, `updated_at`) VALUES ('theme', 'light', '2025-10-23 14:41:11');
INSERT INTO `settings` (`k`, `v`, `updated_at`) VALUES ('webhook_url', '', '2025-10-14 11:14:52');

-- users (1 satır)
INSERT INTO `users` (`id`, `email`, `pass_hash`, `role`, `limits_json`, `created_at`, `updated_at`, `ip_query_limit`, `max_targets`, `is_active`, `license_expiry`, `last_login_at`) VALUES (1, 'admin', 'REPLACE_WITH_YOUR_OWN_BCRYPT_HASH', 'admin', NULL, '2025-10-14 08:20:25', '2026-07-23 12:47:56', 100, 1000, 1, NULL, '2026-07-23 12:47:56');


-- ---------- VARSAYILAN AYAR SATIRLARI (temiz; test verisi YOK) ----------
-- ad_settings (migration default)
INSERT INTO `ad_settings` (`id`, `domain_controller`, `port`, `use_ssl`, `base_dn`, `bind_username`, `bind_password_encrypted`, `is_enabled`) VALUES (1, '', 389, 0, '', '', '', 0);
-- winrm_settings (migration default)
INSERT INTO `winrm_settings` (`id`, `username`, `password_encrypted`, `port`, `use_ssl`, `timeout_seconds`, `concurrent_limit`, `retry_count`, `is_enabled`) VALUES (1, '', '', 5985, 0, 30, 5, 2, 0);
-- snmp_settings (migration default)
INSERT INTO `snmp_settings` (`id`, `version`, `community_strings`, `port`, `timeout_seconds`, `retry_count`, `concurrent_limit`, `is_enabled`) VALUES (1, 'v2c', '["public"]', 161, 5, 2, 10, 0);
-- ssh_settings (migration default)
INSERT INTO `ssh_settings` (`id`, `username`, `password_encrypted`, `port`, `timeout_seconds`, `is_enabled`) VALUES (1, 'root', '', 22, 10, 0);
SET FOREIGN_KEY_CHECKS=1;
