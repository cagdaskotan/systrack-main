-- Migration: User enhancements
-- Add new columns to users table

ALTER TABLE users 
ADD COLUMN max_targets INT DEFAULT 10,
ADD COLUMN is_active BOOLEAN DEFAULT TRUE,
ADD COLUMN license_expiry TIMESTAMP NULL,
ADD COLUMN last_login_at TIMESTAMP NULL,
ADD COLUMN updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP;

-- Update existing users with default values
UPDATE users SET 
    max_targets = CASE 
        WHEN role = 'admin' THEN 1000
        WHEN role = 'user' THEN 50
        ELSE 10
    END,
    is_active = TRUE,
    license_expiry = DATE_ADD(CURRENT_TIMESTAMP, INTERVAL 1 YEAR),
    updated_at = CURRENT_TIMESTAMP
WHERE max_targets IS NULL;

-- Add user_id column to targets table for user association
ALTER TABLE targets 
ADD COLUMN user_id INT DEFAULT 1,
ADD FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE SET NULL;

-- Update existing targets to belong to admin user
UPDATE targets SET user_id = 1 WHERE user_id IS NULL;
