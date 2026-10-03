-- Add update notification tracking columns to device_license table
-- These columns will be used to show update notifications in the web UI

ALTER TABLE device_license
    ADD COLUMN last_update_version VARCHAR(20) NULL AFTER serial_number,
    ADD COLUMN show_update_notification BOOLEAN NOT NULL DEFAULT FALSE AFTER last_update_version;

-- Add index for efficient querying of pending notifications
CREATE INDEX idx_device_license_update_notification
ON device_license(show_update_notification);
