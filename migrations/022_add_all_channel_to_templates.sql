-- Migration: 022_add_all_channel_to_templates.sql
-- Bildirim şablonlarına 'all' kanal seçeneği ekleniyor
-- Bu, şablonların hem email hem telegram için kullanılabilmesini sağlar

-- notification_templates tablosundaki channel ENUM'una 'all' değeri eklenir
ALTER TABLE new_notification_templates
MODIFY COLUMN channel ENUM('email', 'telegram', 'whatsapp', 'webhook', 'all') NOT NULL;

-- Aynı şekilde notification_rules tablosundaki channel ENUM'una da 'all' değeri eklenir (opsiyonel)
-- NOT: Kurallar için 'all' mantıklı olmayabilir çünkü her kural bir kanal seçmeli
-- Sadece şablonlar için 'all' mantıklı
-- ALTER TABLE notification_rules
-- MODIFY COLUMN channel ENUM('email', 'telegram', 'whatsapp', 'webhook', 'all') NOT NULL;
