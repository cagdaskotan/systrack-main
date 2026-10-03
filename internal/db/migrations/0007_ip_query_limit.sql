-- Migration: IP Query Limit System
-- Description: Adds IP query limit functionality for users

-- Add ip_query_limit column to users table
ALTER TABLE users ADD COLUMN ip_query_limit INT DEFAULT 10 COMMENT 'Maximum IP queries per day';

-- Create ip_query_logs table to track daily queries
CREATE TABLE IF NOT EXISTS ip_query_logs (
    id INT AUTO_INCREMENT PRIMARY KEY,
    user_id INT NOT NULL,
    ip_address VARCHAR(45) NOT NULL,
    query_date DATE NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    INDEX idx_user_date (user_id, query_date),
    INDEX idx_date (query_date)
);

-- Set default limits for existing users based on role
UPDATE users SET ip_query_limit = CASE 
    WHEN role = 'admin' THEN 100
    WHEN role = 'user' THEN 20
    WHEN role = 'viewer' THEN 5
    ELSE 10
END WHERE ip_query_limit IS NULL;
