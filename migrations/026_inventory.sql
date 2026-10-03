-- Envanter Yönetimi Tablosu
-- Ağ varlıklarının kalıcı kimlik kartı

CREATE TABLE IF NOT EXISTS `inventory` (
  `id` INT NOT NULL AUTO_INCREMENT,

  -- Otomatik (IP Tarayıcı'dan gelebilir)
  `ip_address` VARCHAR(45) DEFAULT NULL,
  `mac_address` VARCHAR(17) DEFAULT NULL,
  `hostname` VARCHAR(255) DEFAULT NULL,
  `vendor` VARCHAR(255) DEFAULT NULL,

  -- Manuel (İnsan Girişi)
  `asset_name` VARCHAR(255) NOT NULL COMMENT 'Envanter adı (zorunlu)',
  `asset_tag` VARCHAR(50) DEFAULT NULL COMMENT 'Envanter kodu (INV-00001)',
  `asset_type` ENUM('pc','laptop','printer','switch','router','access_point','pos','tv','phone','camera','server','tablet','other') NOT NULL DEFAULT 'other',
  `brand` VARCHAR(100) DEFAULT NULL COMMENT 'Marka (Dell, HP, Cisco...)',
  `model` VARCHAR(255) DEFAULT NULL COMMENT 'Model',
  `serial_number` VARCHAR(100) DEFAULT NULL COMMENT 'Cihaz seri numarası',

  `location` VARCHAR(255) DEFAULT NULL COMMENT 'Lokasyon (Kat 2 - Oda 214)',
  `department` VARCHAR(100) DEFAULT NULL COMMENT 'Departman',
  `assigned_to` VARCHAR(255) DEFAULT NULL COMMENT 'Sorumlu kişi',

  `status` ENUM('active','maintenance','storage','faulty','retired') NOT NULL DEFAULT 'active',

  `purchase_date` DATE DEFAULT NULL,
  `warranty_expiry` DATE DEFAULT NULL,
  `purchase_cost` DECIMAL(10,2) DEFAULT NULL COMMENT 'Satın alma maliyeti',

  `notes` TEXT DEFAULT NULL,

  -- Kaynak bilgisi
  `source` ENUM('manual','ip_scanner','csv_import') NOT NULL DEFAULT 'manual',

  -- İzleme bağlantısı (opsiyonel)
  `target_id` INT DEFAULT NULL COMMENT 'Hedefler tablosuyla ilişki',

  `created_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,

  PRIMARY KEY (`id`),
  UNIQUE KEY `idx_asset_tag` (`asset_tag`),
  KEY `idx_mac` (`mac_address`),
  KEY `idx_ip` (`ip_address`),
  KEY `idx_status` (`status`),
  KEY `idx_type` (`asset_type`),
  KEY `idx_location` (`location`),
  KEY `idx_target` (`target_id`),
  KEY `idx_warranty` (`warranty_expiry`),

  CONSTRAINT `fk_inventory_target` FOREIGN KEY (`target_id`) REFERENCES `targets` (`id`) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
