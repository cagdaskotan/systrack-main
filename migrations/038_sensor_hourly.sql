-- Her saat başında ESP32 sensör + CPU sıcaklık anlık görüntüsü
CREATE TABLE IF NOT EXISTS sensor_hourly (
    id              INT AUTO_INCREMENT PRIMARY KEY,
    hour_ts         DATETIME NOT NULL,
    temperature     FLOAT,
    cpu_temperature FLOAT,
    humidity        FLOAT,
    pressure        FLOAT,
    gas             FLOAT,
    UNIQUE KEY uq_hour (hour_ts)
);
