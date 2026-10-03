-- Expand AD fields in inventory table for comprehensive AD integration
-- Sprint 4: Comprehensive LDAP attribute extraction (like Lansweeper, SCCM, ManageEngine)
--
-- Note: Basic AD columns already exist in 027_ad_integration.sql
-- This migration adds EXTENDED AD attributes for enterprise-grade inventory
--
-- Compatible with older MySQL versions (no IF NOT EXISTS)

-- Add extended AD columns (these are NEW - not in 027)
ALTER TABLE inventory ADD COLUMN ad_cn_name VARCHAR(255);
ALTER TABLE inventory ADD COLUMN ad_os_service_pack VARCHAR(100);
ALTER TABLE inventory ADD COLUMN ad_comment TEXT;
ALTER TABLE inventory ADD COLUMN ad_location VARCHAR(255);
ALTER TABLE inventory ADD COLUMN ad_managed_by VARCHAR(500);
ALTER TABLE inventory ADD COLUMN ad_when_changed TIMESTAMP NULL;
ALTER TABLE inventory ADD COLUMN ad_pwd_last_set TIMESTAMP NULL;
ALTER TABLE inventory ADD COLUMN ad_service_principal_names TEXT;

-- Add indexes
ALTER TABLE inventory ADD INDEX idx_inventory_ad_location (ad_location);
ALTER TABLE inventory ADD INDEX idx_inventory_ad_when_changed (ad_when_changed);
