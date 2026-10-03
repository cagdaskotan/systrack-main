-- Add channel information to new_notification_mails for multi-channel logging
ALTER TABLE new_notification_mails
    ADD COLUMN channel ENUM('email','telegram') NOT NULL DEFAULT 'email' AFTER target_id;

-- Backfill existing rows as email
UPDATE new_notification_mails
SET channel = 'email'
WHERE channel IS NULL;
