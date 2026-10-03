-- 036_software_licenses.sql
-- Yazılım lisans ve satın alma bilgileri

CREATE TABLE IF NOT EXISTS software_licenses (
    id                  INT          NOT NULL AUTO_INCREMENT,
    inventory_id        INT          NOT NULL,
    software_name       VARCHAR(512) NOT NULL COMMENT 'software_json içindeki Name ile eşleşir',
    purchase_cost       DECIMAL(12,2) NULL    COMMENT 'Satın alma maliyeti',
    cost_currency       VARCHAR(8)   NOT NULL DEFAULT 'TRY' COMMENT 'Para birimi (TRY/EUR/USD)',
    purchase_date       DATE         NULL     COMMENT 'Satın alma tarihi',
    license_expiry_date DATE         NULL     COMMENT 'Lisans bitiş tarihi',
    notes               TEXT         NULL     COMMENT 'Notlar',
    created_at          DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at          DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    UNIQUE KEY uq_inv_software (inventory_id, software_name(255)),
    CONSTRAINT fk_sl_inventory FOREIGN KEY (inventory_id) REFERENCES inventory(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
