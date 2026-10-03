-- Per-user new notification settings (simplified for email/telegram)
CREATE TABLE IF NOT EXISTS new_notification_settings (
    id INT AUTO_INCREMENT PRIMARY KEY,
    user_id INT NOT NULL,
    email_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    email_recipient VARCHAR(255) NULL,
    telegram_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    telegram_chat_id VARCHAR(100) NULL,
    types JSON NULL, -- e.g. ["target_status_change"]
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    UNIQUE KEY uq_new_notif_settings_user (user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

