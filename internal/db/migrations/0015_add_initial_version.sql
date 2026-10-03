-- Migration: Add initial_version to device_license table
-- Purpose: Track the version the device was manufactured/first installed with

ALTER TABLE device_license
ADD COLUMN initial_version VARCHAR(20)
COMMENT 'Software version at device manufacturing';

-- Set initial_version to current version for existing devices
UPDATE device_license
SET initial_version = '1.0.0'
WHERE initial_version IS NULL;

INSERT INTO `device_license` (`id`, `serial_number`, `mac_address`, `status`, `customer_email`, `customer_company`, `activated_at`, `created_at`, `initial_version`) VALUES
(1, 'ST-2025-A001-TEST3', '2c:cf:67:cd:88:53', 0, NULL, NULL, NULL, '2025-12-10 09:54:54', '1.0.0');