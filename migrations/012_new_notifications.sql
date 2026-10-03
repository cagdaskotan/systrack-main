-- New notification system (scoped to targets) - tables prefixed with new_

-- Templates table
CREATE TABLE IF NOT EXISTS new_notification_templates (
    id INT AUTO_INCREMENT PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    type ENUM('target') NOT NULL DEFAULT 'target',
    channel ENUM('email','telegram') NOT NULL,
    subject VARCHAR(500) NULL, -- email only
    body TEXT NOT NULL,
    variables JSON NULL,
    is_default BOOLEAN NOT NULL DEFAULT FALSE,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    user_id INT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    INDEX idx_new_tmpl_type_channel (type, channel),
    INDEX idx_new_tmpl_user (user_id),
    INDEX idx_new_tmpl_active (is_active)
);

-- Rules table
CREATE TABLE IF NOT EXISTS new_notification_rules (
    id INT AUTO_INCREMENT PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    entity_type ENUM('target') NOT NULL DEFAULT 'target',
    channel ENUM('email','telegram') NOT NULL,
    schedule_interval_minutes INT NOT NULL DEFAULT 5,
    recipients JSON NOT NULL, -- e.g. ["user@example.com"] or ["123456789"]
    template_id INT NULL,
    conditions JSON NULL, -- optional additional conditions structure
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_by INT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    INDEX idx_new_rules_active (is_active),
    INDEX idx_new_rules_channel (channel),
    INDEX idx_new_rules_created (created_at)
);
