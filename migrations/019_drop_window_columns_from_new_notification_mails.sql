-- Remove unused window window columns from notification mail logs
ALTER TABLE new_notification_mails
    DROP COLUMN window_start,
    DROP COLUMN window_end;
