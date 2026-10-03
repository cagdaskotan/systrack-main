-- Bildirim sistemi için veritabanı şeması
-- Migration: 0011_notification_system.sql

-- Tags tablosu (hedef etiketleri)
CREATE TABLE tags (
    id INT AUTO_INCREMENT PRIMARY KEY,
    name VARCHAR(100) NOT NULL UNIQUE,
    description TEXT,
    color VARCHAR(7) DEFAULT '#6c757d', -- Hex color code
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    INDEX idx_tags_name (name)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Notification templates tablosu (bildirim şablonları)
CREATE TABLE notification_templates (
    id INT AUTO_INCREMENT PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    type ENUM('target_status_change', 'sla_violation', 'alert_opened', 'alert_closed', 'system_health', 'daily_report') NOT NULL,
    channel ENUM('email', 'telegram', 'whatsapp', 'webhook') NOT NULL,
    subject VARCHAR(500) NOT NULL,
    body TEXT NOT NULL,
    variables JSON NULL, -- Kullanılabilir değişkenler
    is_default BOOLEAN DEFAULT false,
    is_active BOOLEAN DEFAULT true,
    user_id INT NULL, -- NULL = global template, user_id = user specific
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    INDEX idx_templates_type_channel (type, channel),
    INDEX idx_templates_user (user_id),
    INDEX idx_templates_active (is_active)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Notification rules tablosu (bildirim kuralları)
CREATE TABLE notification_rules (
    id INT AUTO_INCREMENT PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    type ENUM('target_status_change', 'sla_violation', 'alert_opened', 'alert_closed', 'system_health', 'daily_report') NOT NULL,
    channel ENUM('email', 'telegram', 'whatsapp', 'webhook') NOT NULL,
    priority ENUM('low', 'medium', 'high', 'critical') DEFAULT 'medium',
    conditions JSON NOT NULL, -- Kural koşulları
    recipients JSON NOT NULL, -- Alıcı listesi
    template_id INT NULL, -- Kullanılacak şablon
    is_active BOOLEAN DEFAULT true,
    cooldown INT DEFAULT 15, -- dakika
    last_triggered DATETIME NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    FOREIGN KEY (template_id) REFERENCES notification_templates(id) ON DELETE SET NULL,
    INDEX idx_rules_type_channel (type, channel),
    INDEX idx_rules_active (is_active),
    INDEX idx_rules_priority (priority),
    INDEX idx_rules_last_triggered (last_triggered)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- User notification settings tablosu (kullanıcı bildirim ayarları)
CREATE TABLE user_notification_settings (
    id INT AUTO_INCREMENT PRIMARY KEY,
    user_id INT NOT NULL,
    target_id INT NULL, -- NULL = tüm hedefler, specific ID = sadece o hedef
    tags JSON NULL, -- Tag bazlı filtreleme
    channels JSON NOT NULL, -- ['email', 'telegram']
    events JSON NOT NULL, -- ['target_status_change', 'sla_violation']
    enabled BOOLEAN DEFAULT true,
    cooldown_minutes INT DEFAULT 5,
    quiet_hours_start TIME NULL, -- Sessiz saatler başlangıç
    quiet_hours_end TIME NULL, -- Sessiz saatler bitiş
    preferred_channel ENUM('email', 'telegram', 'whatsapp', 'webhook') DEFAULT 'email',
    email VARCHAR(255) NULL,
    telegram_chat_id VARCHAR(100) NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    FOREIGN KEY (target_id) REFERENCES targets(id) ON DELETE CASCADE,
    INDEX idx_user_settings_user (user_id),
    INDEX idx_user_settings_target (target_id),
    INDEX idx_user_settings_enabled (enabled),
    UNIQUE KEY unique_user_target (user_id, target_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Notifications tablosu (bildirim geçmişi)
CREATE TABLE notifications (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    type ENUM('target_status_change', 'sla_violation', 'alert_opened', 'alert_closed', 'system_health', 'daily_report') NOT NULL,
    channel ENUM('email', 'telegram', 'whatsapp', 'webhook') NOT NULL,
    priority ENUM('low', 'medium', 'high', 'critical') DEFAULT 'medium',
    title VARCHAR(500) NOT NULL,
    message TEXT NOT NULL,
    recipients JSON NOT NULL, -- Alıcı listesi
    target_id INT NULL,
    alert_id INT NULL,
    rule_id INT NULL,
    user_id INT NULL,
    status ENUM('pending', 'sent', 'failed', 'delivered') DEFAULT 'pending',
    error_message TEXT NULL,
    sent_at DATETIME NULL,
    delivered_at DATETIME NULL,
    retry_count INT DEFAULT 0,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    FOREIGN KEY (target_id) REFERENCES targets(id) ON DELETE SET NULL,
    FOREIGN KEY (alert_id) REFERENCES alerts(id) ON DELETE SET NULL,
    FOREIGN KEY (rule_id) REFERENCES notification_rules(id) ON DELETE SET NULL,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE SET NULL,
    INDEX idx_notifications_type (type),
    INDEX idx_notifications_channel (channel),
    INDEX idx_notifications_status (status),
    INDEX idx_notifications_target (target_id),
    INDEX idx_notifications_user (user_id),
    INDEX idx_notifications_created (created_at),
    INDEX idx_notifications_sent (sent_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Email attachments tablosu (email ekleri)
CREATE TABLE email_attachments (
    id INT AUTO_INCREMENT PRIMARY KEY,
    notification_id BIGINT NOT NULL,
    filename VARCHAR(255) NOT NULL,
    content_type VARCHAR(100) NOT NULL,
    size BIGINT NOT NULL,
    data LONGBLOB NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (notification_id) REFERENCES notifications(id) ON DELETE CASCADE,
    INDEX idx_attachments_notification (notification_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Notification logs tablosu (detaylı log)
CREATE TABLE notification_logs (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    notification_id BIGINT NOT NULL,
    level ENUM('debug', 'info', 'warn', 'error') DEFAULT 'info',
    message TEXT NOT NULL,
    context JSON NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (notification_id) REFERENCES notifications(id) ON DELETE CASCADE,
    INDEX idx_logs_notification (notification_id),
    INDEX idx_logs_level (level),
    INDEX idx_logs_created (created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Targets tablosuna tags kolonu ekle (eğer yoksa)
ALTER TABLE targets 
ADD COLUMN IF NOT EXISTS tags VARCHAR(500) NULL COMMENT 'Comma-separated tags';

-- Index'leri ekle
CREATE INDEX IF NOT EXISTS idx_targets_tags ON targets(tags);

-- Varsayılan tag'leri ekle
INSERT IGNORE INTO tags (name, description, color) VALUES
('production', 'Production ortamı', '#dc3545'),
('staging', 'Staging ortamı', '#fd7e14'),
('development', 'Development ortamı', '#ffc107'),
('critical', 'Kritik sistemler', '#6f42c1'),
('monitoring', 'Monitoring sistemleri', '#20c997'),
('database', 'Veritabanı sistemleri', '#17a2b8'),
('web', 'Web servisleri', '#28a745'),
('api', 'API servisleri', '#6c757d');

-- Varsayılan şablonları ekle
INSERT IGNORE INTO notification_templates (name, type, channel, subject, body, is_default, is_active) VALUES
('Hedef Durum Değişikliği - Email', 'target_status_change', 'email', 
 '🎯 {{.Title}}', 
 '<!DOCTYPE html><html><head><meta charset="UTF-8"><title>{{.Title}}</title><style>body{font-family:Arial,sans-serif;line-height:1.6;color:#333;margin:0;padding:0;background-color:#f5f5f5}.container{max-width:600px;margin:20px auto;background:white;border-radius:12px;overflow:hidden;box-shadow:0 4px 6px rgba(0,0,0,0.1)}.header{background:linear-gradient(135deg,#667eea 0%,#764ba2 100%);color:white;padding:30px;text-align:center}.header h1{margin:0;font-size:24px;font-weight:600}.content{padding:30px}.status-badge{display:inline-block;padding:8px 16px;border-radius:20px;font-weight:bold;font-size:14px;margin:10px 0}.status-online{background:#d4edda;color:#155724}.status-offline{background:#f8d7da;color:#721c24}.footer{background:#f8f9fa;padding:20px;text-align:center;color:#6c757d;font-size:12px}</style></head><body><div class="container"><div class="header"><h1>🎯 Hedef Durum Değişikliği</h1><p>SysTrack Monitoring Sistemi</p></div><div class="content"><h2>{{.Title}}</h2><p>{{.Message}}</p></div><div class="footer"><p>Bu bildirim SysTrack monitoring sistemi tarafından otomatik olarak gönderilmiştir.</p></div></div></body></html>',
 true, true),

('Hedef Durum Değişikliği - Telegram', 'target_status_change', 'telegram',
 '🎯 Hedef Durum Değişikliği',
 '🎯 *{{.Title}}*\n\n{{.Message}}\n\n📅 {{.CreatedAt.Format "2006-01-02 15:04:05"}}',
 true, true),

('SLA İhlali - Email', 'sla_violation', 'email',
 '⚠️ SLA İhlali: {{.Title}}',
 '<!DOCTYPE html><html><head><meta charset="UTF-8"><title>{{.Title}}</title><style>body{font-family:Arial,sans-serif;line-height:1.6;color:#333;margin:0;padding:0;background-color:#f5f5f5}.container{max-width:600px;margin:20px auto;background:white;border-radius:12px;overflow:hidden;box-shadow:0 4px 6px rgba(0,0,0,0.1)}.header{background:linear-gradient(135deg,#dc3545 0%,#c82333 100%);color:white;padding:30px;text-align:center}.header h1{margin:0;font-size:24px;font-weight:600}.content{padding:30px}.alert-card{background:#f8d7da;border-radius:8px;padding:20px;margin:20px 0;border-left:4px solid #dc3545}.footer{background:#f8f9fa;padding:20px;text-align:center;color:#6c757d;font-size:12px}</style></head><body><div class="container"><div class="header"><h1>⚠️ SLA İhlali</h1><p>SysTrack Monitoring Sistemi</p></div><div class="content"><h2>{{.Title}}</h2><div class="alert-card"><p><strong>{{.Message}}</strong></p></div></div><div class="footer"><p>Bu bildirim SysTrack monitoring sistemi tarafından otomatik olarak gönderilmiştir.</p></div></div></body></html>',
 true, true),

('SLA İhlali - Telegram', 'sla_violation', 'telegram',
 '⚠️ SLA İhlali',
 '⚠️ *SLA İhlali*\n\n{{.Message}}\n\n📅 {{.CreatedAt.Format "2006-01-02 15:04:05"}}',
 true, true);

-- Varsayılan kuralı ekle
INSERT IGNORE INTO notification_rules (name, description, type, channel, priority, conditions, recipients, is_active, cooldown) VALUES
('Hedef Durum Değişikliği Kuralı', 'Hedeflerin online/offline durum değişikliklerinde bildirim gönder', 
 'target_status_change', 'email', 'medium', 
 '{"conditions": [{"field": "status", "operator": "not_equals", "value": "unknown"}]}',
 '["admin@systrack.local"]', true, 5);
