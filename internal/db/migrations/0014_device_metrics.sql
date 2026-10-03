-- Migration: 0014_device_metrics.sql
-- Description: Add device metrics monitoring capability

-- 1. Add metrics columns to targets table
ALTER TABLE targets
ADD COLUMN metrics_enabled TINYINT(1) NOT NULL DEFAULT 0 AFTER enabled,
ADD COLUMN snmp_community VARCHAR(64) NULL DEFAULT 'public' AFTER metrics_enabled,
ADD COLUMN snmp_version ENUM('v1','v2c','v3') NULL DEFAULT 'v2c' AFTER snmp_community;

-- 2. Create device_metrics table for storing time-series metrics data
CREATE TABLE device_metrics (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    target_id INT NOT NULL,

    -- Timestamp information
    collected_at DATETIME NOT NULL,
    ts_ms BIGINT NOT NULL COMMENT 'Timestamp in milliseconds for charting',

    -- CPU Metrics
    cpu_percent DECIMAL(5,2) NULL COMMENT 'CPU usage percentage (0.00-100.00)',
    cpu_cores INT NULL COMMENT 'Number of CPU cores',

    -- RAM Metrics
    ram_total_mb BIGINT NULL COMMENT 'Total RAM in MB',
    ram_used_mb BIGINT NULL COMMENT 'Used RAM in MB',
    ram_percent DECIMAL(5,2) NULL COMMENT 'RAM usage percentage',

    -- Disk Metrics (primary partition)
    disk_total_gb BIGINT NULL COMMENT 'Total disk space in GB',
    disk_used_gb BIGINT NULL COMMENT 'Used disk space in GB',
    disk_percent DECIMAL(5,2) NULL COMMENT 'Disk usage percentage',

    -- System Information
    uptime_seconds BIGINT NULL COMMENT 'System uptime in seconds',

    -- Collection status
    status ENUM('success', 'timeout', 'snmp_error', 'unreachable') NOT NULL DEFAULT 'success',
    error_msg VARCHAR(255) NULL,

    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,

    -- Indexes for performance
    INDEX idx_target_collected (target_id, collected_at),
    INDEX idx_collected_at (collected_at),
    INDEX idx_ts_ms (ts_ms),

    FOREIGN KEY (target_id) REFERENCES targets(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
COMMENT='Time-series metrics data for monitored devices';
