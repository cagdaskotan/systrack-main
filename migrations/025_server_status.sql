-- Migration: 025_server_status.sql
-- Server status time-series for reports (CPU/RAM/Disk/Temperature)

CREATE TABLE server_status (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    target_id INT NOT NULL,
    collected_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    cpu_percent DECIMAL(5,2) NULL,
    ram_gb DECIMAL(10,2) NULL,
    disk_gb DECIMAL(10,2) NULL,
    temperature_c DECIMAL(6,2) NULL,
    INDEX idx_server_status_target_time (target_id, collected_at),
    CONSTRAINT fk_server_status_target
        FOREIGN KEY (target_id) REFERENCES targets(id)
        ON DELETE CASCADE
);
