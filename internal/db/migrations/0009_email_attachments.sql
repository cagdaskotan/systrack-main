-- Migration: Email Attachments Support
-- Description: Adds email attachment functionality

-- Create email_attachments table for storing email attachments
CREATE TABLE IF NOT EXISTS email_attachments (
    id INT AUTO_INCREMENT PRIMARY KEY,
    notification_id INT,
    email_log_id INT,
    filename VARCHAR(255) NOT NULL,
    content_type VARCHAR(100) NOT NULL,
    size BIGINT NOT NULL,
    data LONGBLOB NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    INDEX idx_notification_id (notification_id),
    INDEX idx_email_log_id (email_log_id),
    INDEX idx_filename (filename),
    INDEX idx_created_at (created_at)
);

-- Add attachment_count column to email_logs table
ALTER TABLE email_logs 
ADD COLUMN attachment_count INT DEFAULT 0 AFTER response_time_ms;

-- Add attachment_size column to email_logs table  
ALTER TABLE email_logs
ADD COLUMN attachment_size BIGINT DEFAULT 0 AFTER attachment_count;

