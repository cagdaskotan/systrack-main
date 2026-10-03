-- Migration: 0004_notifications.sql
-- Description: Bildirim sistemi tabloları

-- Bildirimler tablosu
CREATE TABLE IF NOT EXISTS notifications (
    id INT AUTO_INCREMENT PRIMARY KEY,
    user_id INT NOT NULL,
    type VARCHAR(50) NOT NULL,
    channel VARCHAR(50) NOT NULL,
    priority VARCHAR(20) NOT NULL DEFAULT 'medium',
    subject VARCHAR(255),
    message TEXT NOT NULL,
    payload JSON,
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    retries INT NOT NULL DEFAULT 0,
    last_error TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    sent_at TIMESTAMP NULL,
    
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE INDEX idx_notifications_user_id ON notifications (user_id);
CREATE INDEX idx_notifications_status_priority ON notifications (status, priority);

-- Kullanıcı bildirim ayarları tablosu
CREATE TABLE IF NOT EXISTS user_notification_settings (
    user_id INT PRIMARY KEY,
    email_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    telegram_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    whatsapp_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    webhook_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

-- Bildirim şablonları tablosu
CREATE TABLE IF NOT EXISTS notification_templates (
    id INT AUTO_INCREMENT PRIMARY KEY,
    type VARCHAR(50) NOT NULL,
    channel VARCHAR(50) NOT NULL,
    subject VARCHAR(255),
    body TEXT NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    
    UNIQUE KEY unique_type_channel (type, channel)
);

-- Bildirim konfigürasyonu tablosu
CREATE TABLE IF NOT EXISTS notification_config (
    id INT AUTO_INCREMENT PRIMARY KEY,
    email_config JSON,
    telegram_config JSON,
    whatsapp_config JSON,
    webhook_config JSON,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
);

-- Varsayılan konfigürasyonu ekle
INSERT IGNORE INTO notification_config (id, email_config, telegram_config, whatsapp_config, webhook_config) VALUES
(1, 
 '{"enabled":false,"smtp_host":"","smtp_port":587,"username":"","password":"","from_email":"","from_name":"SysTrack","use_tls":true,"notification_email":""}',
 '{"enabled":false,"bot_token":"","default_chat":"","webhook_url":""}',
 '{"enabled":false,"access_token":"","phone_id":"","webhook_url":""}',
 '{"enabled":false,"url":"","secret":""}'
);

-- Varsayılan şablonları ekle
INSERT IGNORE INTO notification_templates (type, channel, subject, body) VALUES
('system_health', 'email', 'SysTrack Sistem Sağlığı Bildirimi', '<h1>Sistem Sağlığı Durumu</h1><p>Merhaba,</p><p>SysTrack sistem sağlığı durumu: <strong>{{.Status}}</strong></p><p>Detaylar: {{.Details}}</p><p>Saygılarımızla,</p><p>SysTrack Ekibi</p>'),
('system_health', 'telegram', NULL, '<b>SysTrack Sistem Sağlığı Bildirimi</b>\nMerhaba,\nSysTrack sistem sağlığı durumu: <b>{{.Status}}</b>\nDetaylar: {{.Details}}'),
('system_health', 'whatsapp', NULL, 'SysTrack Sistem Sağlığı Bildirimi\nMerhaba,\nSysTrack sistem sağlığı durumu: {{.Status}}\nDetaylar: {{.Details}}'),
('alert_opened', 'email', 'SysTrack Yeni Uyarı: {{.TargetName}}', '<h1>Yeni Uyarı Açıldı!</h1><p>Merhaba,</p><p>Hedef <strong>{{.TargetName}}</strong> için yeni bir uyarı açıldı.</p><p>Mesaj: {{.Message}}</p><p>Seviye: {{.Level}}</p><p>Açılma Zamanı: {{.OpenedAt}}</p><p>Saygılarımızla,</p><p>SysTrack Ekibi</p>'),
('alert_opened', 'telegram', NULL, '<b>SysTrack Yeni Uyarı: {{.TargetName}}</b>\nMerhaba,\nHedef <b>{{.TargetName}}</b> için yeni bir uyarı açıldı.\nMesaj: {{.Message}}\nSeviye: {{.Level}}\nAçılma Zamanı: {{.OpenedAt}}'),
('alert_opened', 'whatsapp', NULL, 'SysTrack Yeni Uyarı: {{.TargetName}}\nMerhaba,\nHedef {{.TargetName}} için yeni bir uyarı açıldı.\nMesaj: {{.Message}}\nSeviye: {{.Level}}\nAçılma Zamanı: {{.OpenedAt}}');
