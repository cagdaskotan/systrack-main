-- Migration: 0002_http_monitoring.sql
-- Description: Add HTTP/HTTPS monitoring support to targets table

-- Add HTTP monitoring columns to targets table
ALTER TABLE targets 
ADD COLUMN monitoring_type ENUM('ping', 'http', 'https') NOT NULL DEFAULT 'ping' AFTER type,
ADD COLUMN http_method ENUM('GET', 'POST', 'PUT', 'DELETE', 'HEAD', 'OPTIONS') NOT NULL DEFAULT 'GET' AFTER monitoring_type,
ADD COLUMN http_path VARCHAR(255) NOT NULL DEFAULT '/' AFTER http_method,
ADD COLUMN http_headers JSON NULL AFTER http_path,
ADD COLUMN expected_status_code INT NOT NULL DEFAULT 200 AFTER http_headers,
ADD COLUMN expected_content TEXT NULL AFTER expected_status_code,
ADD COLUMN ssl_check BOOLEAN NOT NULL DEFAULT FALSE AFTER expected_content,
ADD COLUMN follow_redirects BOOLEAN NOT NULL DEFAULT TRUE AFTER ssl_check,
ADD COLUMN timeout_sec INT NOT NULL DEFAULT 10 AFTER follow_redirects;

-- Update existing targets to use ping monitoring type
UPDATE targets SET monitoring_type = 'ping' WHERE monitoring_type = 'ping';

-- Add index for monitoring type
ALTER TABLE targets ADD INDEX idx_targets_monitoring_type (monitoring_type);

-- Update pings_raw table to support HTTP monitoring data
ALTER TABLE pings_raw 
ADD COLUMN response_status_code INT NULL AFTER rtt_ms,
ADD COLUMN response_size_bytes INT NULL AFTER response_status_code,
ADD COLUMN ssl_expiry_date DATETIME NULL AFTER response_size_bytes,
ADD COLUMN response_headers JSON NULL AFTER ssl_expiry_date;

-- Add index for response status code
ALTER TABLE pings_raw ADD INDEX idx_pings_status_code (response_status_code);

-- Add HTTP monitoring settings
INSERT INTO settings (k, v) VALUES 
('http_user_agent', 'SysTrack-Monitor/1.0'),
('http_max_redirects', '5'),
('http_connect_timeout_sec', '5'),
('http_read_timeout_sec', '10'),
('ssl_warning_days', '30');
