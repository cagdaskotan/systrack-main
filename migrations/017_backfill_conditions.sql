-- Backfill conditions for existing rules: set default status=fail where conditions is NULL
UPDATE new_notification_rules
SET conditions = JSON_ARRAY(JSON_OBJECT('field','status','operator','equals','value','fail'))
WHERE conditions IS NULL;
