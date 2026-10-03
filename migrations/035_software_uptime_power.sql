-- 035_software_uptime_power.sql
-- Kurulu yazılım envanteri, açık kalma süresi ve tahmini güç tüketimi

ALTER TABLE inventory
    ADD COLUMN software_json          LONGTEXT  NULL COMMENT 'Kurulu yazılım listesi (JSON array)' AFTER ssh_json,
    ADD COLUMN software_scan_at       DATETIME  NULL COMMENT 'Son yazılım tarama zamanı'            AFTER software_json,
    ADD COLUMN last_boot_time         VARCHAR(50) NULL COMMENT 'Son sistem açılış zamanı'           AFTER software_scan_at,
    ADD COLUMN power_estimate_watts   INT         NULL COMMENT 'Tahmini güç tüketimi (Watt)'        AFTER last_boot_time;
