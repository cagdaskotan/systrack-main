CREATE TABLE IF NOT EXISTS liquid_sensor_history (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    sensor_serial VARCHAR(64) NOT NULL,
    status_text VARCHAR(128) NOT NULL,
    analog_value INT NULL,
    contact_detected TINYINT(1) NOT NULL DEFAULT 0,
    recorded_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    raw_payload MEDIUMTEXT NULL,
    KEY idx_liquid_sensor_history_serial_time (sensor_serial, recorded_at)
);
