-- Bildirim sistemi tabloları

-- Bildirimler tablosu
CREATE TABLE IF NOT EXISTS notifications (
    id INT AUTO_INCREMENT PRIMARY KEY,
    type ENUM('target_status_change', 'sla_violation', 'alert_opened', 'alert_closed', 'system_health', 'daily_report') NOT NULL,
    channel ENUM('email', 'telegram', 'whatsapp', 'webhook') NOT NULL,
    priority ENUM('low', 'medium', 'high', 'critical') NOT NULL DEFAULT 'medium',
    title VARCHAR(255) NOT NULL,
    message TEXT NOT NULL,
    recipients JSON NOT NULL,
    target_id INT NULL,
    alert_id INT NULL,
    status ENUM('pending', 'sent', 'failed') NOT NULL DEFAULT 'pending',
    sent_at TIMESTAMP NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    error TEXT NULL,
    retry_count INT DEFAULT 0,
    
    INDEX idx_type (type),
    INDEX idx_channel (channel),
    INDEX idx_status (status),
    INDEX idx_created_at (created_at),
    INDEX idx_target_id (target_id),
    INDEX idx_alert_id (alert_id),
    
    FOREIGN KEY (target_id) REFERENCES targets(id) ON DELETE SET NULL,
    FOREIGN KEY (alert_id) REFERENCES alerts(id) ON DELETE SET NULL
);

-- Bildirim şablonları tablosu
CREATE TABLE IF NOT EXISTS notification_templates (
    id INT AUTO_INCREMENT PRIMARY KEY,
    type ENUM('target_status_change', 'sla_violation', 'alert_opened', 'alert_closed', 'system_health', 'daily_report') NOT NULL,
    channel ENUM('email', 'telegram', 'whatsapp', 'webhook') NOT NULL,
    title VARCHAR(255) NOT NULL,
    message TEXT NOT NULL,
    is_active BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    
    UNIQUE KEY unique_template (type, channel),
    INDEX idx_type (type),
    INDEX idx_channel (channel),
    INDEX idx_is_active (is_active)
);

-- Kullanıcı bildirim ayarları tablosu
CREATE TABLE IF NOT EXISTS notification_settings (
    id INT AUTO_INCREMENT PRIMARY KEY,
    user_id INT NOT NULL,
    channel ENUM('email', 'telegram', 'whatsapp', 'webhook') NOT NULL,
    is_enabled BOOLEAN DEFAULT TRUE,
    recipient VARCHAR(255) NOT NULL, -- email, phone, chat_id
    types JSON NOT NULL, -- hangi bildirim türlerini almak istiyor
    priority ENUM('low', 'medium', 'high', 'critical') NOT NULL DEFAULT 'medium',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    
    UNIQUE KEY unique_user_channel (user_id, channel),
    INDEX idx_user_id (user_id),
    INDEX idx_channel (channel),
    INDEX idx_is_enabled (is_enabled),
    
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

-- Bildirim konfigürasyonu tablosu
CREATE TABLE IF NOT EXISTS notification_config (
    id INT AUTO_INCREMENT PRIMARY KEY,
    channel ENUM('email', 'telegram', 'whatsapp', 'webhook') NOT NULL,
    config JSON NOT NULL,
    is_enabled BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    
    UNIQUE KEY unique_channel (channel),
    INDEX idx_channel (channel),
    INDEX idx_is_enabled (is_enabled)
);

-- Varsayılan bildirim şablonlarını ekle
INSERT IGNORE INTO notification_templates (type, channel, title, message) VALUES
-- Target Status Change Templates
('target_status_change', 'email', 'Hedef Durumu Değişti - {{.TargetName}}', 
'<h2>Hedef Durumu Değişti</h2><p><strong>Hedef:</strong> {{.TargetName}}</p><p><strong>URL:</strong> {{.TargetURL}}</p><p><strong>Yeni Durum:</strong> {{.Status}}</p><p><strong>Zaman:</strong> {{.Timestamp}}</p>'),

('target_status_change', 'telegram', '🔄 Hedef Durumu Değişti - {{.TargetName}}', 
'🔄 *Hedef Durumu Değişti*\n\n*Hedef:* {{.TargetName}}\n*URL:* {{.TargetURL}}\n*Yeni Durum:* {{.Status}}\n*Zaman:* {{.Timestamp}}'),

('target_status_change', 'whatsapp', '🔄 Hedef Durumu Değişti - {{.TargetName}}', 
'🔄 *Hedef Durumu Değişti*\n\n*Hedef:* {{.TargetName}}\n*URL:* {{.TargetURL}}\n*Yeni Durum:* {{.Status}}\n*Zaman:* {{.Timestamp}}'),

-- SLA Violation Templates
('sla_violation', 'email', 'SLA İhlali - {{.TargetName}}', 
'<h2>SLA İhlali</h2><p><strong>Hedef:</strong> {{.TargetName}}</p><p><strong>Uptime:</strong> {{.UptimePercent}}%</p><p><strong>Zaman:</strong> {{.Timestamp}}</p>'),

('sla_violation', 'telegram', '⚠️ SLA İhlali - {{.TargetName}}', 
'⚠️ *SLA İhlali*\n\n*Hedef:* {{.TargetName}}\n*Uptime:* {{.UptimePercent}}%\n*Zaman:* {{.Timestamp}}'),

('sla_violation', 'whatsapp', '⚠️ SLA İhlali - {{.TargetName}}', 
'⚠️ *SLA İhlali*\n\n*Hedef:* {{.TargetName}}\n*Uptime:* {{.UptimePercent}}%\n*Zaman:* {{.Timestamp}}'),

-- Alert Opened Templates
('alert_opened', 'email', 'Yeni Uyarı - {{.AlertLevel}}', 
'<h2>Yeni Uyarı</h2><p><strong>Seviye:</strong> {{.AlertLevel}}</p><p><strong>Mesaj:</strong> {{.AlertMessage}}</p><p><strong>Zaman:</strong> {{.Timestamp}}</p>'),

('alert_opened', 'telegram', '🚨 Yeni Uyarı - {{.AlertLevel}}', 
'🚨 *Yeni Uyarı*\n\n*Seviye:* {{.AlertLevel}}\n*Mesaj:* {{.AlertMessage}}\n*Zaman:* {{.Timestamp}}'),

('alert_opened', 'whatsapp', '🚨 Yeni Uyarı - {{.AlertLevel}}', 
'🚨 *Yeni Uyarı*\n\n*Seviye:* {{.AlertLevel}}\n*Mesaj:* {{.AlertMessage}}\n*Zaman:* {{.Timestamp}}'),

-- Alert Closed Templates
('alert_closed', 'email', 'Uyarı Kapandı - {{.AlertLevel}}', 
'<h2>Uyarı Kapandı</h2><p><strong>Seviye:</strong> {{.AlertLevel}}</p><p><strong>Mesaj:</strong> {{.AlertMessage}}</p><p><strong>Zaman:</strong> {{.Timestamp}}</p>'),

('alert_closed', 'telegram', '✅ Uyarı Kapandı - {{.AlertLevel}}', 
'✅ *Uyarı Kapandı*\n\n*Seviye:* {{.AlertLevel}}\n*Mesaj:* {{.AlertMessage}}\n*Zaman:* {{.Timestamp}}'),

('alert_closed', 'whatsapp', '✅ Uyarı Kapandı - {{.AlertLevel}}', 
'✅ *Uyarı Kapandı*\n\n*Seviye:* {{.AlertLevel}}\n*Mesaj:* {{.AlertMessage}}\n*Zaman:* {{.Timestamp}}'),

-- System Health Templates
('system_health', 'email', 'Sistem Sağlık Durumu', 
'<h2>Sistem Sağlık Durumu</h2><p><strong>Durum:</strong> {{.Status}}</p><p><strong>Zaman:</strong> {{.Timestamp}}</p>'),

('system_health', 'telegram', '💚 Sistem Sağlık Durumu', 
'💚 *Sistem Sağlık Durumu*\n\n*Durum:* {{.Status}}\n*Zaman:* {{.Timestamp}}'),

('system_health', 'whatsapp', '💚 Sistem Sağlık Durumu', 
'💚 *Sistem Sağlık Durumu*\n\n*Durum:* {{.Status}}\n*Zaman:* {{.Timestamp}}'),

-- Daily Report Templates
('daily_report', 'email', 'Günlük Rapor', 
'<h2>Günlük Rapor</h2><p><strong>Tarih:</strong> {{.Timestamp}}</p><p><strong>Durum:</strong> {{.Status}}</p>'),

('daily_report', 'telegram', '📊 Günlük Rapor', 
'📊 *Günlük Rapor*\n\n*Tarih:* {{.Timestamp}}\n*Durum:* {{.Status}}'),

('daily_report', 'whatsapp', '📊 Günlük Rapor', 
'📊 *Günlük Rapor*\n\n*Tarih:* {{.Timestamp}}\n*Durum:* {{.Status}}');

-- Varsayılan bildirim konfigürasyonları
INSERT IGNORE INTO notification_config (channel, config, is_enabled) VALUES
('email', '{"enabled": false, "smtp_host": "", "smtp_port": 587, "username": "", "password": "", "from_email": "", "from_name": "SysTrack", "use_tls": true}', FALSE),
('telegram', '{"enabled": false, "bot_token": "", "default_chat": "", "webhook_url": ""}', FALSE),
('whatsapp', '{"enabled": false, "access_token": "", "phone_id": "", "webhook_url": ""}', FALSE),
('webhook', '{"enabled": false, "url": "", "secret": ""}', FALSE);
