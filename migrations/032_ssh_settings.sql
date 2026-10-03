-- SSH Settings Table
-- Stores SSH connection settings for Linux/Unix device discovery

CREATE TABLE IF NOT EXISTS `ssh_settings` (
  `id` INT NOT NULL AUTO_INCREMENT PRIMARY KEY,

  -- Bağlantı Ayarları
  `username` VARCHAR(255) NOT NULL DEFAULT 'root' COMMENT 'SSH kullanıcı adı',
  `password_encrypted` TEXT DEFAULT NULL COMMENT 'Şifrelenmiş SSH şifresi',
  `port` INT NOT NULL DEFAULT 22 COMMENT 'SSH portu',
  `timeout_seconds` INT NOT NULL DEFAULT 10 COMMENT 'Bağlantı timeout süresi (saniye)',

  -- Durum
  `is_enabled` TINYINT(1) NOT NULL DEFAULT 0 COMMENT 'SSH discovery aktif mi?',

  -- Timestamps
  `created_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Varsayılan kayıt ekle
INSERT INTO `ssh_settings` (`id`, `username`, `password_encrypted`, `port`, `timeout_seconds`, `is_enabled`)
VALUES (1, 'root', '', 22, 10, 0)
ON DUPLICATE KEY UPDATE `id` = `id`;
