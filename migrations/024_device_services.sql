-- Migration: 024_device_services.sql
-- Description: Device service monitoring with credentials (WinRM/SSH)
-- Purpose: Monitor services running inside devices (services.msc on Windows, systemctl on Linux)

-- Drop old tables if they exist (from previous port-scanning approach)
DROP TABLE IF EXISTS target_services;
DROP TABLE IF EXISTS target_credentials;

-- ============================================
-- Target Credentials Table
-- ============================================
-- Stores encrypted credentials for accessing device services
-- One credential per target (UNIQUE constraint on target_id)
CREATE TABLE IF NOT EXISTS target_credentials (
  id INT AUTO_INCREMENT PRIMARY KEY,
  target_id INT NOT NULL UNIQUE,
  os_type ENUM('windows', 'linux') NOT NULL COMMENT 'Operating system type',
  protocol ENUM('winrm', 'ssh') NOT NULL COMMENT 'Connection protocol',
  username VARCHAR(255) NOT NULL COMMENT 'Username for authentication',
  password_encrypted TEXT NOT NULL COMMENT 'AES-256 encrypted password',
  port INT DEFAULT NULL COMMENT 'Custom port (default: WinRM=5985, SSH=22)',
  domain VARCHAR(255) DEFAULT NULL COMMENT 'Windows domain (optional, for WinRM)',
  last_test_at DATETIME DEFAULT NULL COMMENT 'Last successful connection test',
  last_test_success BOOLEAN DEFAULT NULL COMMENT 'Last test result',
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,

  FOREIGN KEY (target_id) REFERENCES targets(id) ON DELETE CASCADE,
  INDEX idx_target_credentials_os_type (os_type),
  INDEX idx_target_credentials_updated (updated_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ============================================
-- Device Services Table
-- ============================================
-- Stores discovered services from devices
-- Auto-populated by background collector
CREATE TABLE IF NOT EXISTS device_services (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  target_id INT NOT NULL,
  service_name VARCHAR(255) NOT NULL COMMENT 'Service internal name (e.g., wuauserv, nginx)',
  display_name VARCHAR(500) DEFAULT NULL COMMENT 'Service display name (e.g., Windows Update)',
  description TEXT DEFAULT NULL COMMENT 'Service description',
  status ENUM('running', 'stopped', 'unknown') NOT NULL DEFAULT 'unknown',
  startup_type VARCHAR(50) DEFAULT NULL COMMENT 'Auto, Manual, Disabled, etc.',
  pid INT DEFAULT NULL COMMENT 'Process ID (if running)',
  last_checked DATETIME NOT NULL COMMENT 'Last check timestamp',
  is_monitored BOOLEAN DEFAULT FALSE COMMENT 'User marked as critical for alerts',
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,

  FOREIGN KEY (target_id) REFERENCES targets(id) ON DELETE CASCADE,
  UNIQUE KEY unique_service_per_target (target_id, service_name),
  INDEX idx_device_services_target (target_id),
  INDEX idx_device_services_status (status),
  INDEX idx_device_services_monitored (is_monitored),
  INDEX idx_device_services_last_checked (last_checked)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ============================================
-- Device Service History Table
-- ============================================
-- Tracks service status changes over time
-- Used for alerts and historical analysis
CREATE TABLE IF NOT EXISTS device_service_history (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  service_id BIGINT NOT NULL,
  target_id INT NOT NULL COMMENT 'Denormalized for faster queries',
  service_name VARCHAR(255) NOT NULL COMMENT 'Denormalized for faster queries',
  old_status ENUM('running', 'stopped', 'unknown') DEFAULT NULL,
  new_status ENUM('running', 'stopped', 'unknown') NOT NULL,
  changed_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,

  FOREIGN KEY (service_id) REFERENCES device_services(id) ON DELETE CASCADE,
  FOREIGN KEY (target_id) REFERENCES targets(id) ON DELETE CASCADE,
  INDEX idx_service_history_service (service_id),
  INDEX idx_service_history_target (target_id),
  INDEX idx_service_history_changed (changed_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
