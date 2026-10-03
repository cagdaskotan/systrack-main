-- Migration: 0001_init.sql
-- Description: Initial database schema for SysTrack

-- Users table
CREATE TABLE users (
  id INT AUTO_INCREMENT PRIMARY KEY,
  email VARCHAR(255) NOT NULL UNIQUE,
  pass_hash VARCHAR(255) NOT NULL,
  role ENUM('admin','user','viewer') NOT NULL DEFAULT 'admin',
  limits_json JSON NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Targets table
CREATE TABLE targets (
  id INT AUTO_INCREMENT PRIMARY KEY,
  name VARCHAR(128) NOT NULL,
  address VARCHAR(255) NOT NULL,
  type ENUM('icmp','tcp','http') NOT NULL DEFAULT 'icmp',
  port INT NULL,
  path VARCHAR(255) NULL,
  interval_sec INT NOT NULL DEFAULT 30,
  timeout_ms INT NOT NULL DEFAULT 1000,
  enabled TINYINT(1) NOT NULL DEFAULT 1,
  tags VARCHAR(255) NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  INDEX idx_targets_enabled_type (enabled, type),
  INDEX idx_targets_address (address),
  INDEX idx_targets_tags (tags)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Raw ping results table (short retention)
CREATE TABLE pings_raw (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  target_id INT NOT NULL,
  ts_ms BIGINT NOT NULL,
  ok TINYINT(1) NOT NULL,
  rtt_ms INT NULL,
  error_msg VARCHAR(255) NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  FOREIGN KEY (target_id) REFERENCES targets(id) ON DELETE CASCADE,
  INDEX idx_pings_target_ts (target_id, ts_ms),
  INDEX idx_pings_ts (ts_ms),
  INDEX idx_pings_ok (ok)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Alerts table
CREATE TABLE alerts (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  target_id INT NOT NULL,
  level ENUM('minor','major') NOT NULL,
  opened_at DATETIME NOT NULL,
  closed_at DATETIME NULL,
  message VARCHAR(255) NULL,
  status ENUM('open','closed') NOT NULL DEFAULT 'open',
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  FOREIGN KEY (target_id) REFERENCES targets(id) ON DELETE CASCADE,
  INDEX idx_alerts_target_status (target_id, status),
  INDEX idx_alerts_opened (opened_at),
  INDEX idx_alerts_level (level),
  INDEX idx_alerts_status (status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Settings table (key-value store)
CREATE TABLE settings (
  k VARCHAR(64) PRIMARY KEY,
  v TEXT NOT NULL,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Insert default settings
INSERT INTO settings (k, v) VALUES 
('telegram_bot_token', ''),
('telegram_chat_id', ''),
('smtp_host', ''),
('smtp_port', '587'),
('smtp_username', ''),
('smtp_password', ''),
('webhook_url', ''),
('theme', 'light'),
('dashboard_refresh_sec', '30');
