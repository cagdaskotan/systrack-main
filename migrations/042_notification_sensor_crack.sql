-- Bildirim sistemine sensör eşiği ve crack tespiti kural tipleri ekleniyor.
-- entity_type ENUM genişletildi; sensor_serial ve inventory_id kolonları eklendi.

ALTER TABLE new_notification_rules
  MODIFY entity_type ENUM('target', 'sensor', 'crack_detection') NOT NULL DEFAULT 'target',
  ADD COLUMN sensor_serial VARCHAR(100) NULL AFTER target_id,
  ADD COLUMN inventory_id INT NULL AFTER sensor_serial;
