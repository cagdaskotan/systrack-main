-- 037_crack_scan.sql
-- Adds persisted license/crack scan results to inventory.

SET @schema_name = DATABASE();

SET @sql = (
    SELECT IF(
        COUNT(*) = 0,
        'ALTER TABLE inventory ADD COLUMN crack_scan_at DATETIME NULL COMMENT ''Last license compliance scan time'' AFTER power_estimate_watts',
        'SELECT 1'
    )
    FROM information_schema.columns
    WHERE table_schema = @schema_name
      AND table_name = 'inventory'
      AND column_name = 'crack_scan_at'
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @sql = (
    SELECT IF(
        COUNT(*) = 0,
        'ALTER TABLE inventory ADD COLUMN crack_risk_level VARCHAR(20) NULL COMMENT ''Risk level: clean/low/medium/high'' AFTER crack_scan_at',
        'SELECT 1'
    )
    FROM information_schema.columns
    WHERE table_schema = @schema_name
      AND table_name = 'inventory'
      AND column_name = 'crack_risk_level'
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @sql = (
    SELECT IF(
        COUNT(*) = 0,
        'ALTER TABLE inventory ADD COLUMN crack_findings_json LONGTEXT NULL COMMENT ''Detected findings (JSON array)'' AFTER crack_risk_level',
        'SELECT 1'
    )
    FROM information_schema.columns
    WHERE table_schema = @schema_name
      AND table_name = 'inventory'
      AND column_name = 'crack_findings_json'
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
