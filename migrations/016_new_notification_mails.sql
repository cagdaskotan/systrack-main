-- Log table for sent notification emails
CREATE TABLE IF NOT EXISTS new_notification_mails (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    rule_id INT NULL,
    target_id INT NULL,
    recipients JSON NOT NULL,
    subject VARCHAR(500) NOT NULL,
    body TEXT NOT NULL,
    window_start DATETIME NULL,
    window_end DATETIME NULL,
    status ENUM('sent','failed') NOT NULL,
    error TEXT NULL,
    sent_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_rule (rule_id),
    INDEX idx_target (target_id),
    INDEX idx_sent (sent_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

