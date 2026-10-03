-- =====================================================================
-- 043_missing_tables.sql
-- Catch-up migration: 025-042 arası tüm tablolar ve kolonlar
-- Envanter Yönetimi + Ortam İzleme + Bildirim güncellemeleri
-- Her satır IF NOT EXISTS / ADD COLUMN IF NOT EXISTS ile güvenli.
-- Aynı cihazda iki kez çalıştırılsa da hata vermez.
-- =====================================================================

-- ═══════════════════════════════════════════════════════════════════
-- BÖLÜM 1: TEMEL TABLOLAR (CREATE TABLE IF NOT EXISTS)
-- ═══════════════════════════════════════════════════════════════════

-- 025: Sunucu kaynak tarihçesi
CREATE TABLE IF NOT EXISTS server_status (
  id           BIGINT       NOT NULL AUTO_INCREMENT PRIMARY KEY,
  target_id    INT          NOT NULL,
  collected_at DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  cpu_percent  DECIMAL(5,2) NULL,
  ram_gb       DECIMAL(10,2) NULL,
  disk_gb      DECIMAL(10,2) NULL,
  temperature_c DECIMAL(6,2) NULL,
  INDEX idx_server_status_target_time (target_id, collected_at),
  CONSTRAINT fk_server_status_target
    FOREIGN KEY (target_id) REFERENCES targets(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- 026/027/028/030/031/032/033/034: Envanter ana tablosu (tam şema)
CREATE TABLE IF NOT EXISTS `inventory` (
  `id`                          INT          NOT NULL AUTO_INCREMENT,
  -- AD kimlik
  `ad_object_guid`              VARCHAR(64)  DEFAULT NULL,
  `ad_distinguished_name`       VARCHAR(1000) DEFAULT NULL,
  `ad_computer_name`            VARCHAR(255) DEFAULT NULL,
  `ad_cn_name`                  VARCHAR(255) DEFAULT NULL,
  `ad_ou_path`                  VARCHAR(1000) DEFAULT NULL,
  -- AD detay
  `ad_description`              TEXT         DEFAULT NULL,
  `ad_comment`                  TEXT         DEFAULT NULL,
  `ad_location`                 VARCHAR(255) DEFAULT NULL,
  `ad_managed_by`               VARCHAR(500) DEFAULT NULL,
  `ad_os_name`                  VARCHAR(255) DEFAULT NULL,
  `ad_os_version`               VARCHAR(100) DEFAULT NULL,
  `ad_os_service_pack`          VARCHAR(100) DEFAULT NULL,
  `ad_last_logon`               DATETIME     DEFAULT NULL,
  `ad_when_created`             DATETIME     DEFAULT NULL,
  `ad_when_changed`             DATETIME     DEFAULT NULL,
  `ad_pwd_last_set`             DATETIME     DEFAULT NULL,
  `ad_service_principal_names`  TEXT         DEFAULT NULL,
  -- Sahip bilgisi (034)
  `owner_json`                  JSON         DEFAULT NULL,
  -- Ağ / cihaz
  `ip_address`                  VARCHAR(45)  DEFAULT NULL,
  `mac_address`                 VARCHAR(17)  DEFAULT NULL,
  `hostname`                    VARCHAR(255) DEFAULT NULL,
  `vendor`                      VARCHAR(255) DEFAULT NULL,
  -- Manuel alanlar
  `asset_name`                  VARCHAR(255) NOT NULL,
  `asset_tag`                   VARCHAR(50)  DEFAULT NULL,
  `asset_type`                  ENUM('pc','laptop','printer','switch','router','access_point','pos','tv','phone','camera','server','tablet','other') NOT NULL DEFAULT 'other',
  `brand`                       VARCHAR(100) DEFAULT NULL,
  `model`                       VARCHAR(255) DEFAULT NULL,
  `serial_number`               VARCHAR(100) DEFAULT NULL,
  `location`                    VARCHAR(255) DEFAULT NULL,
  `department`                  VARCHAR(100) DEFAULT NULL,
  `assigned_to`                 VARCHAR(255) DEFAULT NULL,
  `status`                      ENUM('active','maintenance','storage','faulty','retired') NOT NULL DEFAULT 'active',
  `purchase_date`               DATE         DEFAULT NULL,
  `warranty_expiry`             DATE         DEFAULT NULL,
  `purchase_cost`               DECIMAL(10,2) DEFAULT NULL,
  `notes`                       TEXT         DEFAULT NULL,
  -- Kaynak
  `source`                      VARCHAR(50)  NOT NULL DEFAULT 'manual',
  `discovery_source`            VARCHAR(50)  DEFAULT NULL,
  `discovery_methods`           TEXT         DEFAULT NULL,
  `discovered_at`               DATETIME     DEFAULT NULL,
  -- İzleme hedefi
  `target_id`                   INT          DEFAULT NULL,
  -- Zenginleştirme JSON
  `hardware_json`               JSON         DEFAULT NULL,
  `snmp_json`                   JSON         DEFAULT NULL,
  `ssh_json`                    JSON         DEFAULT NULL,
  `winrm_json`                  JSON         DEFAULT NULL,
  `enrichment_errors`           JSON         DEFAULT NULL,
  -- Yazılım/uptime/güç (035)
  `software_json`               LONGTEXT     DEFAULT NULL,
  `software_scan_at`            DATETIME     DEFAULT NULL,
  `last_boot_time`              VARCHAR(50)  DEFAULT NULL,
  `power_estimate_watts`        INT          DEFAULT NULL,
  -- Crack/lisans tarama (037)
  `crack_scan_at`               DATETIME     DEFAULT NULL,
  `crack_risk_level`            VARCHAR(20)  DEFAULT NULL,
  `crack_findings_json`         LONGTEXT     DEFAULT NULL,
  -- Enrichment zamanları
  `last_ad_sync_at`             DATETIME     DEFAULT NULL,
  `last_winrm_enrich_at`        DATETIME     DEFAULT NULL,
  `last_snmp_enrich_at`         DATETIME     DEFAULT NULL,
  `last_ssh_enrich_at`          DATETIME     DEFAULT NULL,
  `created_at`                  DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at`                  DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `idx_asset_tag`        (`asset_tag`),
  UNIQUE KEY `idx_ad_object_guid`   (`ad_object_guid`),
  KEY `idx_inventory_ip`            (`ip_address`),
  KEY `idx_inventory_mac`           (`mac_address`),
  KEY `idx_inventory_hostname`      (`hostname`),
  KEY `idx_inventory_status`        (`status`),
  KEY `idx_inventory_type`          (`asset_type`),
  KEY `idx_inventory_location`      (`location`),
  KEY `idx_inventory_target`        (`target_id`),
  KEY `idx_inventory_warranty`      (`warranty_expiry`),
  KEY `idx_inventory_discovery_src` (`discovery_source`),
  KEY `idx_inventory_discovered_at` (`discovered_at`),
  CONSTRAINT `fk_inventory_target` FOREIGN KEY (`target_id`)
    REFERENCES `targets`(`id`) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- 027: AD ayarları (tam şema, 028 kolonları dahil)
CREATE TABLE IF NOT EXISTS `ad_settings` (
  `id`                      INT          NOT NULL AUTO_INCREMENT,
  `domain_controller`       VARCHAR(255) NOT NULL,
  `port`                    INT          NOT NULL DEFAULT 389,
  `use_ssl`                 TINYINT(1)   NOT NULL DEFAULT 0,
  `base_dn`                 VARCHAR(500) NOT NULL,
  `bind_username`           VARCHAR(255) NOT NULL,
  `bind_password_encrypted` TEXT         NOT NULL,
  `search_filter`           VARCHAR(500) DEFAULT '(objectClass=computer)',
  `search_scope`            ENUM('base','one','sub') DEFAULT 'sub',
  `ou_filter`               TEXT         DEFAULT NULL,
  `location_from_ou`        TINYINT(1)   DEFAULT 1,
  `ou_location_map`         JSON         DEFAULT NULL,
  `is_enabled`              TINYINT(1)   NOT NULL DEFAULT 0,
  `last_test_at`            DATETIME     DEFAULT NULL,
  `last_test_result`        ENUM('success','failed') DEFAULT NULL,
  `last_test_error`         TEXT         DEFAULT NULL,
  `last_test_computer_count` INT         DEFAULT NULL,
  `created_at`              DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at`              DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

INSERT INTO `ad_settings`
  (`id`, `domain_controller`, `port`, `use_ssl`, `base_dn`, `bind_username`, `bind_password_encrypted`, `is_enabled`)
VALUES
  (1, '', 389, 0, '', '', '', 0)
ON DUPLICATE KEY UPDATE `id` = `id`;

-- 027: WinRM ayarları (tam şema, 028 kolonları dahil)
CREATE TABLE IF NOT EXISTS `winrm_settings` (
  `id`                   INT          NOT NULL AUTO_INCREMENT,
  `username`             VARCHAR(255) NOT NULL,
  `password_encrypted`   TEXT         NOT NULL,
  `port`                 INT          NOT NULL DEFAULT 5985,
  `use_ssl`              TINYINT(1)   NOT NULL DEFAULT 0,
  `timeout_seconds`      INT          NOT NULL DEFAULT 30,
  `concurrent_limit`     INT          NOT NULL DEFAULT 5,
  `retry_count`          INT          NOT NULL DEFAULT 2,
  `is_enabled`           TINYINT(1)   NOT NULL DEFAULT 0,
  `last_test_at`         DATETIME     DEFAULT NULL,
  `last_test_result`     ENUM('success','failed') DEFAULT NULL,
  `last_test_error`      TEXT         DEFAULT NULL,
  `last_test_ip`         VARCHAR(45)  DEFAULT NULL,
  `created_at`           DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at`           DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

INSERT INTO `winrm_settings`
  (`id`, `username`, `password_encrypted`, `port`, `use_ssl`, `timeout_seconds`, `concurrent_limit`, `retry_count`, `is_enabled`)
VALUES
  (1, '', '', 5985, 0, 30, 5, 2, 0)
ON DUPLICATE KEY UPDATE `id` = `id`;

-- 027: SNMP ayarları (tam şema, 028 kolonları dahil)
CREATE TABLE IF NOT EXISTS `snmp_settings` (
  `id`                           INT          NOT NULL AUTO_INCREMENT,
  `version`                      ENUM('v1','v2c','v3') NOT NULL DEFAULT 'v2c',
  `community_strings`            TEXT         NOT NULL,
  `v3_username`                  VARCHAR(255) DEFAULT NULL,
  `v3_auth_protocol`             ENUM('MD5','SHA') DEFAULT NULL,
  `v3_auth_password_encrypted`   TEXT         DEFAULT NULL,
  `v3_priv_protocol`             ENUM('DES','AES') DEFAULT NULL,
  `v3_priv_password_encrypted`   TEXT         DEFAULT NULL,
  `port`                         INT          NOT NULL DEFAULT 161,
  `timeout_seconds`              INT          NOT NULL DEFAULT 5,
  `retry_count`                  INT          NOT NULL DEFAULT 2,
  `concurrent_limit`             INT          NOT NULL DEFAULT 10,
  `scan_targets`                 TEXT         DEFAULT NULL,
  `is_enabled`                   TINYINT(1)   NOT NULL DEFAULT 0,
  `last_test_at`                 DATETIME     DEFAULT NULL,
  `last_test_result`             ENUM('success','failed') DEFAULT NULL,
  `last_test_error`              TEXT         DEFAULT NULL,
  `last_test_ip`                 VARCHAR(45)  DEFAULT NULL,
  `created_at`                   DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at`                   DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

INSERT INTO `snmp_settings`
  (`id`, `version`, `community_strings`, `port`, `timeout_seconds`, `retry_count`, `concurrent_limit`, `scan_targets`, `is_enabled`)
VALUES
  (1, 'v2c', '["public"]', 161, 5, 2, 10, '', 0)
ON DUPLICATE KEY UPDATE `id` = `id`;

-- 032: SSH ayarları
CREATE TABLE IF NOT EXISTS `ssh_settings` (
  `id`                   INT          NOT NULL AUTO_INCREMENT PRIMARY KEY,
  `username`             VARCHAR(255) NOT NULL DEFAULT 'root',
  `password_encrypted`   TEXT         DEFAULT NULL,
  `port`                 INT          NOT NULL DEFAULT 22,
  `timeout_seconds`      INT          NOT NULL DEFAULT 10,
  `is_enabled`           TINYINT(1)   NOT NULL DEFAULT 0,
  `created_at`           DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at`           DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

INSERT INTO `ssh_settings`
  (`id`, `username`, `password_encrypted`, `port`, `timeout_seconds`, `is_enabled`)
VALUES
  (1, 'root', '', 22, 10, 0)
ON DUPLICATE KEY UPDATE `id` = `id`;

-- 027: Tarama/senkronizasyon log tablosu
CREATE TABLE IF NOT EXISTS `inventory_scans` (
  `id`             INT          NOT NULL AUTO_INCREMENT,
  `scan_type`      VARCHAR(50)  NOT NULL,
  `status`         ENUM('pending','running','completed','failed','cancelled') NOT NULL DEFAULT 'pending',
  `total_targets`  INT          DEFAULT 0,
  `processed`      INT          DEFAULT 0,
  `success_count`  INT          DEFAULT 0,
  `error_count`    INT          DEFAULT 0,
  `new_assets`     INT          DEFAULT 0,
  `updated_assets` INT          DEFAULT 0,
  `errors_json`    JSON         DEFAULT NULL,
  `scan_config`    JSON         DEFAULT NULL,
  `started_at`     DATETIME     DEFAULT NULL,
  `completed_at`   DATETIME     DEFAULT NULL,
  `created_at`     DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `started_by`     VARCHAR(100) DEFAULT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_inventory_scans_type`    (`scan_type`),
  KEY `idx_inventory_scans_status`  (`status`),
  KEY `idx_inventory_scans_created` (`created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- 029: IP erişim izin listesi
CREATE TABLE IF NOT EXISTS access_allowlist (
  id         BIGINT       NOT NULL AUTO_INCREMENT PRIMARY KEY,
  ip_address VARCHAR(45)  NOT NULL,
  label      VARCHAR(120) NOT NULL DEFAULT '',
  is_enabled TINYINT(1)   NOT NULL DEFAULT 1,
  created_at DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  UNIQUE KEY uq_access_allowlist_ip (ip_address),
  INDEX idx_access_allowlist_enabled (is_enabled)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

INSERT INTO settings (k, v)
VALUES ('access_allowlist_enabled', '0')
ON DUPLICATE KEY UPDATE v = v;

-- 036: Yazılım lisans tablosu
CREATE TABLE IF NOT EXISTS software_licenses (
  id                  INT           NOT NULL AUTO_INCREMENT,
  inventory_id        INT           NOT NULL,
  software_name       VARCHAR(512)  NOT NULL,
  purchase_cost       DECIMAL(12,2) NULL,
  cost_currency       VARCHAR(8)    NOT NULL DEFAULT 'TRY',
  purchase_date       DATE          NULL,
  license_expiry_date DATE          NULL,
  notes               TEXT          NULL,
  created_at          DATETIME      NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at          DATETIME      NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  UNIQUE KEY uq_inv_software (inventory_id, software_name(255)),
  CONSTRAINT fk_sl_inventory FOREIGN KEY (inventory_id)
    REFERENCES inventory(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- 038: Ortam izleme — saatlik sensör anlık görüntüsü
CREATE TABLE IF NOT EXISTS sensor_hourly (
  id              INT   NOT NULL AUTO_INCREMENT PRIMARY KEY,
  hour_ts         DATETIME NOT NULL,
  temperature     FLOAT NULL,
  cpu_temperature FLOAT NULL,
  humidity        FLOAT NULL,
  pressure        FLOAT NULL,
  gas             FLOAT NULL,
  UNIQUE KEY uq_hour (hour_ts)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ═══════════════════════════════════════════════════════════════════
-- BÖLÜM 2: YENİ TABLOLAR (039-041 + 043)
-- ═══════════════════════════════════════════════════════════════════

-- 039: KMS güvenilir host listesi
CREATE TABLE IF NOT EXISTS kms_trusted_hosts (
  id         BIGINT       NOT NULL AUTO_INCREMENT PRIMARY KEY,
  host       VARCHAR(255) NOT NULL,
  label      VARCHAR(120) NOT NULL DEFAULT '',
  is_enabled TINYINT(1)   NOT NULL DEFAULT 1,
  created_at DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE KEY uq_kms_trusted_hosts_host (host),
  INDEX idx_kms_trusted_hosts_enabled (is_enabled)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- 039: Crack tarama istisnaları
CREATE TABLE IF NOT EXISTS crack_scan_exceptions (
  id             BIGINT       NOT NULL AUTO_INCREMENT PRIMARY KEY,
  inventory_id   INT          NULL COMMENT 'NULL = global istisna',
  signature_hash CHAR(64)     NOT NULL,
  signature_text TEXT         NOT NULL,
  reason         VARCHAR(500) NOT NULL DEFAULT '',
  created_by     INT          NULL,
  created_at     DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  INDEX idx_crack_scan_exceptions_inventory (inventory_id),
  INDEX idx_crack_scan_exceptions_hash (signature_hash)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- 040: Yazılım değişim geçmişi
CREATE TABLE IF NOT EXISTS software_scan_history (
  id            INT          NOT NULL AUTO_INCREMENT PRIMARY KEY,
  inventory_id  INT          NOT NULL,
  scanned_at    DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  software_name VARCHAR(512) NOT NULL,
  publisher     VARCHAR(256) NULL,
  change_type   ENUM('added','removed','version_changed') NOT NULL,
  new_version   VARCHAR(256) NULL,
  prev_version  VARCHAR(256) NULL,
  KEY idx_ssh_inv_time (inventory_id, scanned_at),
  KEY idx_ssh_inv_name (inventory_id, software_name(255)),
  CONSTRAINT fk_ssh_inventory FOREIGN KEY (inventory_id)
    REFERENCES inventory(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- 040: Otomatik yazılım tarama ayarları (cihaz başına)
CREATE TABLE IF NOT EXISTS inventory_scan_settings (
  inventory_id       INT        NOT NULL PRIMARY KEY,
  auto_software_scan TINYINT(1) NOT NULL DEFAULT 0,
  scan_hour          TINYINT    NOT NULL DEFAULT 3,
  last_auto_scan_at  DATETIME   NULL,
  CONSTRAINT fk_iss_inventory FOREIGN KEY (inventory_id)
    REFERENCES inventory(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- 041: Domain dışı (standalone) WinRM host'lar
CREATE TABLE IF NOT EXISTS winrm_standalone_hosts (
  id                 INT          NOT NULL AUTO_INCREMENT PRIMARY KEY,
  ip_address         VARCHAR(64)  NOT NULL,
  label              VARCHAR(255) NULL,
  username           VARCHAR(255) NOT NULL,
  password_encrypted TEXT         NOT NULL,
  port               INT          NOT NULL DEFAULT 5985,
  use_ssl            TINYINT(1)   NOT NULL DEFAULT 0,
  is_enabled         TINYINT(1)   NOT NULL DEFAULT 1,
  last_seen_at       DATETIME     NULL,
  created_at         DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at         DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- 043: Sıvı temas sensörü geçmişi
CREATE TABLE IF NOT EXISTS liquid_sensor_history (
  id               BIGINT      NOT NULL AUTO_INCREMENT PRIMARY KEY,
  sensor_serial    VARCHAR(64) NOT NULL,
  status_text      VARCHAR(128) NOT NULL,
  analog_value     INT         NULL,
  contact_detected TINYINT(1)  NOT NULL DEFAULT 0,
  recorded_at      DATETIME    NOT NULL DEFAULT CURRENT_TIMESTAMP,
  raw_payload      MEDIUMTEXT  NULL,
  KEY idx_liquid_sensor_serial_time (sensor_serial, recorded_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ═══════════════════════════════════════════════════════════════════
-- BÖLÜM 3: EKSİK KOLONLAR (mevcut tablolara ADD COLUMN IF NOT EXISTS)
-- Sadece 026_inventory.sql + 027 tek tek uygulayan cihazları kapsar.
-- 026_inventory_complete.sql ile kurulanlarda bunlar zaten var.
-- ═══════════════════════════════════════════════════════════════════

-- ── 027: inventory — AD + donanım/SNMP/zenginleştirme kolonları ──────
ALTER TABLE `inventory`
  ADD COLUMN IF NOT EXISTS `ad_object_guid`        VARCHAR(64)   DEFAULT NULL AFTER `id`,
  ADD COLUMN IF NOT EXISTS `ad_distinguished_name` VARCHAR(1000) DEFAULT NULL AFTER `ad_object_guid`,
  ADD COLUMN IF NOT EXISTS `ad_computer_name`      VARCHAR(255)  DEFAULT NULL AFTER `ad_distinguished_name`,
  ADD COLUMN IF NOT EXISTS `ad_description`        TEXT          DEFAULT NULL AFTER `ad_computer_name`,
  ADD COLUMN IF NOT EXISTS `ad_ou_path`            VARCHAR(1000) DEFAULT NULL AFTER `ad_description`,
  ADD COLUMN IF NOT EXISTS `ad_os_name`            VARCHAR(255)  DEFAULT NULL AFTER `ad_ou_path`,
  ADD COLUMN IF NOT EXISTS `ad_os_version`         VARCHAR(100)  DEFAULT NULL AFTER `ad_os_name`,
  ADD COLUMN IF NOT EXISTS `ad_last_logon`         DATETIME      DEFAULT NULL AFTER `ad_os_version`,
  ADD COLUMN IF NOT EXISTS `ad_when_created`       DATETIME      DEFAULT NULL AFTER `ad_last_logon`,
  ADD COLUMN IF NOT EXISTS `hardware_json`         JSON          DEFAULT NULL AFTER `ad_when_created`,
  ADD COLUMN IF NOT EXISTS `snmp_json`             JSON          DEFAULT NULL AFTER `hardware_json`,
  ADD COLUMN IF NOT EXISTS `discovery_source`      VARCHAR(50)   DEFAULT NULL AFTER `source`,
  ADD COLUMN IF NOT EXISTS `last_ad_sync_at`       DATETIME      DEFAULT NULL AFTER `discovery_source`,
  ADD COLUMN IF NOT EXISTS `last_winrm_enrich_at`  DATETIME      DEFAULT NULL AFTER `last_ad_sync_at`,
  ADD COLUMN IF NOT EXISTS `last_snmp_enrich_at`   DATETIME      DEFAULT NULL AFTER `last_winrm_enrich_at`,
  ADD COLUMN IF NOT EXISTS `enrichment_errors`     JSON          DEFAULT NULL AFTER `last_snmp_enrich_at`;

-- ── 028: ad_settings eksik kolon ────────────────────────────────────
ALTER TABLE `ad_settings`
  ADD COLUMN IF NOT EXISTS `last_test_computer_count` INT DEFAULT NULL AFTER `last_test_error`;

-- ── 028: winrm_settings eksik kolon ─────────────────────────────────
ALTER TABLE `winrm_settings`
  ADD COLUMN IF NOT EXISTS `last_test_ip` VARCHAR(45) DEFAULT NULL AFTER `last_test_error`;

-- ── 028: snmp_settings eksik kolonlar ───────────────────────────────
ALTER TABLE `snmp_settings`
  ADD COLUMN IF NOT EXISTS `scan_targets`     TEXT                    DEFAULT NULL AFTER `concurrent_limit`,
  ADD COLUMN IF NOT EXISTS `last_test_at`     DATETIME                DEFAULT NULL AFTER `is_enabled`,
  ADD COLUMN IF NOT EXISTS `last_test_result` ENUM('success','failed') DEFAULT NULL AFTER `last_test_at`,
  ADD COLUMN IF NOT EXISTS `last_test_error`  TEXT                    DEFAULT NULL AFTER `last_test_result`,
  ADD COLUMN IF NOT EXISTS `last_test_ip`     VARCHAR(45)             DEFAULT NULL AFTER `last_test_error`;

-- ── 030: inventory — genişletilmiş AD alanları ───────────────────────
ALTER TABLE `inventory`
  ADD COLUMN IF NOT EXISTS `ad_cn_name`              VARCHAR(255) DEFAULT NULL AFTER `ad_computer_name`,
  ADD COLUMN IF NOT EXISTS `ad_os_service_pack`      VARCHAR(100) DEFAULT NULL AFTER `ad_os_version`,
  ADD COLUMN IF NOT EXISTS `ad_comment`              TEXT         DEFAULT NULL AFTER `ad_description`,
  ADD COLUMN IF NOT EXISTS `ad_location`             VARCHAR(255) DEFAULT NULL AFTER `ad_comment`,
  ADD COLUMN IF NOT EXISTS `ad_managed_by`           VARCHAR(500) DEFAULT NULL AFTER `ad_location`,
  ADD COLUMN IF NOT EXISTS `ad_when_changed`         DATETIME     DEFAULT NULL AFTER `ad_when_created`,
  ADD COLUMN IF NOT EXISTS `ad_pwd_last_set`         DATETIME     DEFAULT NULL AFTER `ad_when_changed`,
  ADD COLUMN IF NOT EXISTS `ad_service_principal_names` TEXT      DEFAULT NULL AFTER `ad_pwd_last_set`;

-- ── 031: inventory — keşif takip alanları ────────────────────────────
ALTER TABLE `inventory`
  ADD COLUMN IF NOT EXISTS `discovery_methods` TEXT     DEFAULT NULL AFTER `discovery_source`,
  ADD COLUMN IF NOT EXISTS `discovered_at`     DATETIME DEFAULT NULL AFTER `discovery_methods`;

-- ── 033: inventory — SSH JSON + timestamp ────────────────────────────
ALTER TABLE `inventory`
  ADD COLUMN IF NOT EXISTS `ssh_json`           JSON     DEFAULT NULL AFTER `snmp_json`,
  ADD COLUMN IF NOT EXISTS `last_ssh_enrich_at` DATETIME DEFAULT NULL AFTER `last_snmp_enrich_at`;

-- ── 034: inventory — owner + WinRM JSON ─────────────────────────────
ALTER TABLE `inventory`
  ADD COLUMN IF NOT EXISTS `owner_json` JSON DEFAULT NULL AFTER `ad_service_principal_names`,
  ADD COLUMN IF NOT EXISTS `winrm_json` JSON DEFAULT NULL AFTER `ssh_json`;

-- ── 035: inventory — yazılım / uptime / güç ─────────────────────────
ALTER TABLE `inventory`
  ADD COLUMN IF NOT EXISTS `software_json`        LONGTEXT    DEFAULT NULL AFTER `winrm_json`,
  ADD COLUMN IF NOT EXISTS `software_scan_at`     DATETIME    DEFAULT NULL AFTER `software_json`,
  ADD COLUMN IF NOT EXISTS `last_boot_time`       VARCHAR(50) DEFAULT NULL AFTER `software_scan_at`,
  ADD COLUMN IF NOT EXISTS `power_estimate_watts` INT         DEFAULT NULL AFTER `last_boot_time`;

-- ── 037: inventory — crack/lisans tarama ────────────────────────────
ALTER TABLE `inventory`
  ADD COLUMN IF NOT EXISTS `crack_scan_at`       DATETIME    DEFAULT NULL AFTER `power_estimate_watts`,
  ADD COLUMN IF NOT EXISTS `crack_risk_level`    VARCHAR(20) DEFAULT NULL AFTER `crack_scan_at`,
  ADD COLUMN IF NOT EXISTS `crack_findings_json` LONGTEXT    DEFAULT NULL AFTER `crack_risk_level`;

-- ═══════════════════════════════════════════════════════════════════
-- BÖLÜM 4: ENUM GENİŞLETMELERİ (MODIFY COLUMN)
-- ═══════════════════════════════════════════════════════════════════

-- 033: inventory_scans — ssh_enrich tarama tipi
ALTER TABLE `inventory_scans`
  MODIFY COLUMN `scan_type`
    ENUM('ad_discovery','winrm_enrich','snmp_enrich','ssh_enrich','ip_scan') NOT NULL;

-- 042: new_notification_rules — sensör + crack kural desteği
ALTER TABLE `new_notification_rules`
  MODIFY COLUMN `entity_type`
    ENUM('target','sensor','crack_detection') NOT NULL DEFAULT 'target',
  ADD COLUMN IF NOT EXISTS `sensor_serial` VARCHAR(100) NULL AFTER `target_id`,
  ADD COLUMN IF NOT EXISTS `inventory_id`  INT          NULL AFTER `sensor_serial`;
