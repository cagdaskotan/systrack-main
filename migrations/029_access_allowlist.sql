-- Access allowlist (optional)
CREATE TABLE IF NOT EXISTS access_allowlist (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  ip_address VARCHAR(45) NOT NULL,
  label VARCHAR(120) NOT NULL DEFAULT '',
  is_enabled TINYINT(1) NOT NULL DEFAULT 1,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  UNIQUE KEY uq_access_allowlist_ip (ip_address),
  INDEX idx_access_allowlist_enabled (is_enabled)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

INSERT INTO settings (k, v)
VALUES ('access_allowlist_enabled', '0')
ON DUPLICATE KEY UPDATE v = v;
