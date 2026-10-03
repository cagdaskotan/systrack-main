ALTER TABLE new_notification_mails
    ADD COLUMN template_id INT NULL AFTER channel,
    ADD INDEX idx_new_notification_mails_template (template_id),
    ADD CONSTRAINT fk_new_notification_mails_template
        FOREIGN KEY (template_id) REFERENCES new_notification_templates(id)
        ON DELETE SET NULL;
