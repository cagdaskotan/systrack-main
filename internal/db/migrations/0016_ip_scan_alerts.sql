-- Migration: 0016_ip_scan_alerts.sql
-- Description: Store last IP scan results and new IP alerts

CREATE TABLE ip_results (
  subnet VARCHAR(64) NOT NULL,
  ip VARCHAR(45) NOT NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (subnet, ip),
  INDEX idx_ip_results_created_at (created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE ip_alerts (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  subnet VARCHAR(64) NOT NULL,
  ip VARCHAR(45) NOT NULL,
  detected_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  acknowledged_at DATETIME NULL,
  UNIQUE KEY uniq_ip_alerts_subnet_ip (subnet, ip),
  INDEX idx_ip_alerts_acknowledged_at (acknowledged_at),
  INDEX idx_ip_alerts_detected_at (detected_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

