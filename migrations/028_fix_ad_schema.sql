-- Schema Fixes for AD/WinRM/SNMP Settings
-- Addresses missing columns and improves test result tracking

-- =====================================================
-- 1. Add missing last_test_computer_count to ad_settings
-- =====================================================
ALTER TABLE `ad_settings`
  ADD COLUMN `last_test_computer_count` INT DEFAULT NULL COMMENT 'Son testte bulunan bilgisayar sayısı' AFTER `last_test_error`;

-- =====================================================
-- 2. Add test result tracking to winrm_settings
-- =====================================================
-- WinRM için de benzer test sonucu alanları ekleyelim
ALTER TABLE `winrm_settings`
  ADD COLUMN `last_test_ip` VARCHAR(45) DEFAULT NULL COMMENT 'Son test edilen IP' AFTER `last_test_error`;

-- =====================================================
-- 3. Add test result tracking to snmp_settings
-- =====================================================
ALTER TABLE `snmp_settings`
  ADD COLUMN `last_test_at` DATETIME DEFAULT NULL COMMENT 'Son test zamanı' AFTER `is_enabled`,
  ADD COLUMN `last_test_result` ENUM('success','failed') DEFAULT NULL COMMENT 'Son test sonucu' AFTER `last_test_at`,
  ADD COLUMN `last_test_error` TEXT DEFAULT NULL COMMENT 'Son test hatası' AFTER `last_test_result`,
  ADD COLUMN `last_test_ip` VARCHAR(45) DEFAULT NULL COMMENT 'Son test edilen IP' AFTER `last_test_error`;
