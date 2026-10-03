-- Migration 031: Discovery Tracking
-- Keşif ile eklenen cihazları takip etmek için alanlar

ALTER TABLE inventory ADD COLUMN discovery_source VARCHAR(50);  -- 'manual', 'discovery'
ALTER TABLE inventory ADD COLUMN discovery_methods TEXT;         -- JSON array: ["ldap", "winrm", "ssh"]
ALTER TABLE inventory ADD COLUMN discovered_at TIMESTAMP NULL;  -- Keşfedilme zamanı
