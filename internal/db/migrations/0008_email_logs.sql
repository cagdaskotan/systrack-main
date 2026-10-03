-- Migration: Email Logging System
-- Description: Adds comprehensive email logging functionality

-- Create email_logs table for tracking all email activities
CREATE TABLE IF NOT EXISTS email_logs (
    id INT AUTO_INCREMENT PRIMARY KEY,
    notification_id INT,
    recipient_email VARCHAR(255) NOT NULL,
    subject VARCHAR(500) NOT NULL,
    message_type ENUM('text', 'html') DEFAULT 'html',
    status ENUM('pending', 'sent', 'failed', 'bounced') DEFAULT 'pending',
    smtp_host VARCHAR(255),
    smtp_port INT,
    use_tls BOOLEAN DEFAULT FALSE,
    sent_at TIMESTAMP NULL,
    failed_at TIMESTAMP NULL,
    error_message TEXT,
    retry_count INT DEFAULT 0,
    message_id VARCHAR(255), -- SMTP message ID
    response_time_ms INT, -- SMTP response time
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    INDEX idx_recipient (recipient_email),
    INDEX idx_status (status),
    INDEX idx_sent_at (sent_at),
    INDEX idx_created_at (created_at),
    INDEX idx_notification_id (notification_id)
);

-- Create email_statistics table for aggregated stats
CREATE TABLE IF NOT EXISTS email_statistics (
    id INT AUTO_INCREMENT PRIMARY KEY,
    date DATE NOT NULL,
    total_sent INT DEFAULT 0,
    total_failed INT DEFAULT 0,
    total_bounced INT DEFAULT 0,
    avg_response_time_ms DECIMAL(10,2) DEFAULT 0,
    unique_recipients INT DEFAULT 0,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    UNIQUE KEY unique_date (date),
    INDEX idx_date (date)
);

-- Create email_recipients table for managing recipient lists
CREATE TABLE IF NOT EXISTS email_recipients (
    id INT AUTO_INCREMENT PRIMARY KEY,
    email VARCHAR(255) NOT NULL UNIQUE,
    name VARCHAR(255),
    is_active BOOLEAN DEFAULT TRUE,
    is_verified BOOLEAN DEFAULT FALSE,
    verification_token VARCHAR(255),
    verification_expires_at TIMESTAMP NULL,
    bounce_count INT DEFAULT 0,
    last_bounce_at TIMESTAMP NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    INDEX idx_email (email),
    INDEX idx_active (is_active),
    INDEX idx_verified (is_verified)
);

-- Create email_templates_log table for template usage tracking
CREATE TABLE IF NOT EXISTS email_templates_log (
    id INT AUTO_INCREMENT PRIMARY KEY,
    template_type VARCHAR(50) NOT NULL,
    template_name VARCHAR(100),
    usage_count INT DEFAULT 1,
    last_used_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    UNIQUE KEY unique_template (template_type),
    INDEX idx_template_type (template_type),
    INDEX idx_last_used (last_used_at)
);

-- Insert default email templates log entries
INSERT INTO email_templates_log (template_type, template_name) VALUES
('target_status_change', 'Hedef Durum Değişikliği'),
('sla_violation', 'SLA İhlali'),
('alert_opened', 'Yeni Uyarı'),
('alert_closed', 'Uyarı Kapanma'),
('system_health', 'Sistem Sağlık Durumu'),
('daily_report', 'Günlük Rapor'),
('test_email', 'Test Emaili')
ON DUPLICATE KEY UPDATE usage_count = usage_count + 1, last_used_at = CURRENT_TIMESTAMP;
