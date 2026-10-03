-- New notification channel configuration
CREATE TABLE IF NOT EXISTS new_notification_config (
    id INT AUTO_INCREMENT PRIMARY KEY,
    channel ENUM('email','telegram') NOT NULL,
    config JSON NOT NULL,
    is_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    UNIQUE KEY uq_new_notif_config_channel (channel),
    INDEX idx_new_notif_config_channel (channel),
    INDEX idx_new_notif_config_enabled (is_enabled)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

