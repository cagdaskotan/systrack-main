-- 039_crack_scan_v2.sql
-- Crack/lisans tespit motoru v2: KMS güvenilir host listesi + bulgu istisnaları.

CREATE TABLE IF NOT EXISTS kms_trusted_hosts (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  host VARCHAR(255) NOT NULL,
  label VARCHAR(120) NOT NULL DEFAULT '',
  is_enabled TINYINT(1) NOT NULL DEFAULT 1,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE KEY uq_kms_trusted_hosts_host (host),
  INDEX idx_kms_trusted_hosts_enabled (is_enabled)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS crack_scan_exceptions (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  inventory_id INT NULL COMMENT 'NULL = tüm envanter kayıtlarına uygulanan global istisna',
  signature_hash CHAR(64) NOT NULL COMMENT 'sha256(category|title|evidence), bkz. findingSignature',
  signature_text TEXT NOT NULL,
  reason VARCHAR(500) NOT NULL DEFAULT '',
  created_by INT NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  INDEX idx_crack_scan_exceptions_inventory (inventory_id),
  INDEX idx_crack_scan_exceptions_hash (signature_hash)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
