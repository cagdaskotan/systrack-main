ALTER TABLE `snmp_settings`
  ADD COLUMN `scan_targets` TEXT DEFAULT NULL AFTER `concurrent_limit`;

ALTER TABLE `inventory`
  MODIFY COLUMN `ad_object_guid` VARCHAR(64) DEFAULT NULL;

INSERT INTO `ad_settings`
(`id`, `domain_controller`, `port`, `use_ssl`, `base_dn`, `bind_username`, `bind_password_encrypted`, `is_enabled`)
VALUES
(1, '', 389, 0, '', '', '', 0)
ON DUPLICATE KEY UPDATE `id` = `id`;

INSERT INTO `winrm_settings`
(`id`, `username`, `password_encrypted`, `port`, `use_ssl`, `timeout_seconds`, `concurrent_limit`, `retry_count`, `is_enabled`)
VALUES
(1, '', '', 5985, 0, 30, 5, 2, 0)
ON DUPLICATE KEY UPDATE `id` = `id`;

INSERT INTO `snmp_settings`
(`id`, `version`, `community_strings`, `port`, `timeout_seconds`, `retry_count`, `concurrent_limit`, `scan_targets`, `is_enabled`)
VALUES
(1, 'v2c', '["public"]', 161, 5, 2, 10, '', 0)
ON DUPLICATE KEY UPDATE `id` = `id`;

INSERT INTO `ssh_settings`
(`id`, `username`, `password_encrypted`, `port`, `timeout_seconds`, `is_enabled`)
VALUES
(1, 'root', '', 22, 10, 0)
ON DUPLICATE KEY UPDATE `id` = `id`;
