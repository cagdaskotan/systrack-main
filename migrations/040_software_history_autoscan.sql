-- 040_software_history_autoscan.sql
-- Yazılım değişim geçmişi ve otomatik tarama ayarları

-- Yazılım versiyon/durum geçmişi: her taramada değişen yazılımlar buraya kaydedilir.
-- Bir yazılım eklendi, kaldırıldı veya versiyonu değiştiyse tek bir satır düşer.
CREATE TABLE IF NOT EXISTS software_scan_history (
    id              INT          NOT NULL AUTO_INCREMENT,
    inventory_id    INT          NOT NULL,
    scanned_at      DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    software_name   VARCHAR(512) NOT NULL,
    publisher       VARCHAR(256) NULL,
    change_type     ENUM('added','removed','version_changed') NOT NULL,
    new_version     VARCHAR(256) NULL COMMENT 'Yeni versiyon (added veya version_changed)',
    prev_version    VARCHAR(256) NULL COMMENT 'Önceki versiyon (removed veya version_changed)',
    PRIMARY KEY (id),
    KEY idx_inv_scanned_at (inventory_id, scanned_at),
    KEY idx_inv_name       (inventory_id, software_name(255)),
    CONSTRAINT fk_ssh_inventory FOREIGN KEY (inventory_id)
        REFERENCES inventory(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Otomatik yazılım tarama ayarları: her cihaz için ayrı ayrı ayarlanabilir.
-- auto_software_scan = 1 ise sistem her gün scan_hour'da o cihazın yazılım listesini tarar.
CREATE TABLE IF NOT EXISTS inventory_scan_settings (
    inventory_id        INT         NOT NULL,
    auto_software_scan  TINYINT(1)  NOT NULL DEFAULT 0 COMMENT '1=günlük otomatik tarama aktif',
    scan_hour           TINYINT     NOT NULL DEFAULT 3  COMMENT 'Taramanın yapılacağı saat (0-23)',
    last_auto_scan_at   DATETIME    NULL COMMENT 'Son otomatik tarama zamanı',
    PRIMARY KEY (inventory_id),
    CONSTRAINT fk_iss_inventory FOREIGN KEY (inventory_id)
        REFERENCES inventory(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
