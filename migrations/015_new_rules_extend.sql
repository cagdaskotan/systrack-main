-- Extend new_notification_rules with optional target and last_sent_at
ALTER TABLE new_notification_rules
    ADD COLUMN target_id INT NULL AFTER name,
    ADD COLUMN last_sent_at DATETIME NULL AFTER created_by;

ALTER TABLE new_notification_rules
    ADD INDEX idx_new_rules_target (target_id);

ALTER TABLE new_notification_rules
    ADD CONSTRAINT fk_new_rules_target
    FOREIGN KEY (target_id) REFERENCES targets(id)
    ON DELETE SET NULL;

