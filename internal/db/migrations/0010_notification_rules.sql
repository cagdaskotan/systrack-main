-- 0010_notification_rules.sql
-- Notification Rules Migration (requires table: notification_templates)

CREATE TABLE IF NOT EXISTS notification_rules (
    id INT AUTO_INCREMENT PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    type VARCHAR(50) NOT NULL,
    channel VARCHAR(50) NOT NULL,
    priority VARCHAR(20) NOT NULL DEFAULT 'medium',
    conditions JSON NOT NULL,
    recipients JSON NOT NULL,
    template_id INT NULL,
    is_active TINYINT(1) NOT NULL DEFAULT 1,
    cooldown INT NOT NULL DEFAULT 0 COMMENT 'Cooldown in minutes',
    last_triggered TIMESTAMP NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,

    INDEX idx_type_channel (type, channel),
    INDEX idx_active (is_active),
    INDEX idx_priority (priority),

    CONSTRAINT fk_notification_rules_template
        FOREIGN KEY (template_id) REFERENCES notification_templates(id)
        ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Seed defaults (idempotent-ish)
INSERT INTO notification_rules
(name, description, type, channel, priority, conditions, recipients, is_active, cooldown)
VALUES
('Target Offline Alert',
 'Target çevrimdışı olduğunda email bildirimi gönder',
 'target_status_change', 'email', 'high',
 JSON_ARRAY(JSON_OBJECT('field','status','operator','equals','value','offline')),
 JSON_ARRAY('admin@systrack.local'), 1, 5),

('SLA Violation Alert',
 'SLA ihlali olduğunda email bildirimi gönder',
 'sla_violation', 'email', 'critical',
 JSON_ARRAY(JSON_OBJECT('field','sla_breach','operator','equals','value','true')),
 JSON_ARRAY('admin@systrack.local'), 1, 10),

('High Response Time Alert',
 'Yanıt süresi yüksek olduğunda email bildirimi gönder',
 'target_status_change', 'email', 'medium',
 JSON_ARRAY(JSON_OBJECT('field','response_time','operator','greater_than','value','5000')),
 JSON_ARRAY('admin@systrack.local'), 1, 15),

('Daily Report',
 'Günlük sistem raporu gönder',
 'daily_report', 'email', 'low',
 JSON_ARRAY(JSON_OBJECT('field','time','operator','equals','value','daily')),
 JSON_ARRAY('admin@systrack.local'), 1, 1440);
