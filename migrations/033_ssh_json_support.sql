-- =====================================================
-- Migration 033: SSH JSON Support
-- =====================================================
-- SSH discovery sonuçlarını envantere kaydetme desteği
-- Tarih: 2025-02-12
-- Bağımlılıklar: 032_ssh_settings.sql

-- =====================================================
-- 1. SSH JSON kolonu ekle
-- =====================================================
ALTER TABLE inventory
  ADD COLUMN ssh_json JSON DEFAULT NULL
  COMMENT 'SSH bilgileri (OS, kernel, architecture, CPU, uptime, network interfaces)'
  AFTER snmp_json;

-- =====================================================
-- 2. SSH enrichment timestamp kolonu ekle
-- =====================================================
ALTER TABLE inventory
  ADD COLUMN last_ssh_enrich_at DATETIME DEFAULT NULL
  COMMENT 'Son SSH zenginleştirmesi zamanı'
  AFTER last_snmp_enrich_at;

-- =====================================================
-- 3. Discovery source ENUM'ına SSH ekle
-- =====================================================
ALTER TABLE inventory
  MODIFY COLUMN discovery_source
  ENUM('manual','ip_scanner','ad_ldap','snmp','ssh') DEFAULT NULL
  COMMENT 'İlk keşif kaynağı';

-- =====================================================
-- 4. Inventory scans tablosuna SSH scan type ekle
-- =====================================================
ALTER TABLE inventory_scans
  MODIFY COLUMN scan_type
  ENUM('ad_discovery','winrm_enrich','snmp_enrich','ssh_enrich','ip_scan') NOT NULL;

-- =====================================================
-- Migration 033 tamamlandı! ✅
-- =====================================================
-- Artık SSH ile keşfedilen cihazların tüm bilgileri
-- ssh_json kolonunda JSON formatında kaydediliyor:
-- - os_pretty_name (Ubuntu 24.04.3 LTS)
-- - kernel_version (6.8.0-1047-raspi)
-- - architecture (aarch64, x86_64)
-- - cpu_model (Intel Core i7, ARM Cortex-A76)
-- - hostname (raspberrypi, server01)
-- - uptime (17:04:30 up 23:29...)
-- - network_interfaces ([{name, ip, mac}])
-- =====================================================
