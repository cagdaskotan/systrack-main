package notifications

import (
	"database/sql"
	"fmt"
	"log"
	"strings"
	"sync"
	"text/template"
)

// TemplateEngine şablon motoru
type TemplateEngine struct {
	db        *sql.DB
	templates map[string]*NotificationTemplate
	mu        sync.RWMutex
}

// NewTemplateEngine yeni şablon motoru oluşturur
func NewTemplateEngine(db *sql.DB) *TemplateEngine {
	return &TemplateEngine{
		db:        db,
		templates: make(map[string]*NotificationTemplate),
	}
}

// GetTemplates şablonları getirir
func (te *TemplateEngine) GetTemplates() ([]*NotificationTemplate, error) {
	query := `
		SELECT id, type, channel, subject, body, is_active, created_at, updated_at
		FROM notification_templates
		ORDER BY type, channel, created_at
	`

	rows, err := te.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("şablonlar sorgulanamadı: %w", err)
	}
	defer rows.Close()

	var templates []*NotificationTemplate
	for rows.Next() {
		var template NotificationTemplate
		var subject sql.NullString

		err := rows.Scan(
			&template.ID,
			&template.Type,
			&template.Channel,
			&subject,
			&template.Message,
			&template.IsActive,
			&template.CreatedAt,
			&template.UpdatedAt,
		)
		if err != nil {
			log.Printf("Şablon scan edilemedi: %v", err)
			continue
		}

		// NULL değerleri kontrol et
		if subject.Valid {
			template.Title = subject.String
		} else {
			template.Title = fmt.Sprintf("%s Bildirimi", template.Type)
		}

		// Default değerler ata
		template.Name = fmt.Sprintf("%s - %s", template.Type, template.Channel)
		template.IsDefault = true

		templates = append(templates, &template)
	}

	return templates, nil
}

// CreateTemplate şablon oluşturur
func (te *TemplateEngine) CreateTemplate(template *NotificationTemplate) error {
	query := `
		INSERT INTO notification_templates (name, type, channel, subject, body, is_default, is_active, user_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, NOW(), NOW())
	`

	result, err := te.db.Exec(query,
		template.Name,
		template.Type,
		template.Channel,
		template.Title,
		template.Message,
		template.IsDefault,
		template.IsActive,
		template.UserID,
	)

	if err != nil {
		return fmt.Errorf("şablon oluşturulamadı: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("şablon ID alınamadı: %w", err)
	}

	template.ID = int(id)
	return nil
}

// UpdateTemplate şablonu günceller
func (te *TemplateEngine) UpdateTemplate(template *NotificationTemplate) error {
	query := `
		UPDATE notification_templates 
		SET type = ?, channel = ?, subject = ?, body = ?, is_active = ?, updated_at = NOW()
		WHERE id = ?
	`

	_, err := te.db.Exec(query,
		template.Type,
		template.Channel,
		template.Title,
		template.Message,
		template.IsActive,
		template.ID,
	)

	if err != nil {
		return fmt.Errorf("şablon güncellenemedi: %w", err)
	}

	return nil
}

// DeleteTemplate şablonu siler
func (te *TemplateEngine) DeleteTemplate(id int) error {
	query := `DELETE FROM notification_templates WHERE id = ?`

	_, err := te.db.Exec(query, id)
	if err != nil {
		return fmt.Errorf("şablon silinemedi: %w", err)
	}

	return nil
}

// LoadTemplates şablonları yükler
func (te *TemplateEngine) LoadTemplates() error {
	query := `
		SELECT id, type, channel, title, message, is_active, created_at, updated_at
		FROM notification_templates
		WHERE is_active = true
	`

	rows, err := te.db.Query(query)
	if err != nil {
		return fmt.Errorf("şablonlar sorgulanamadı: %w", err)
	}
	defer rows.Close()

	te.mu.Lock()
	defer te.mu.Unlock()

	te.templates = make(map[string]*NotificationTemplate)

	for rows.Next() {
		var template NotificationTemplate

		err := rows.Scan(
			&template.ID,
			&template.Type,
			&template.Channel,
			&template.Title,
			&template.Message,
			&template.IsActive,
			&template.CreatedAt,
			&template.UpdatedAt,
		)
		if err != nil {
			log.Printf("Şablon scan edilemedi: %v", err)
			continue
		}

		// Şablon anahtarı oluştur
		key := fmt.Sprintf("%s_%s", template.Type, template.Channel)
		te.templates[key] = &template
	}

	log.Printf("✅ %d şablon yüklendi", len(te.templates))
	return nil
}

// ApplyTemplate şablonu bildirime uygular
func (te *TemplateEngine) ApplyTemplate(notification *Notification, channel NotificationChannel) error {
	te.mu.RLock()
	defer te.mu.RUnlock()

	// Şablon anahtarı oluştur
	key := fmt.Sprintf("%s_%s", notification.Type, channel)

	template, exists := te.templates[key]
	if !exists {
		// Varsayılan şablon kullan
		return te.applyDefaultTemplate(notification, channel)
	}

	// Şablonu uygula
	err := te.renderTemplate(template, notification)
	if err != nil {
		return fmt.Errorf("şablon uygulanamadı: %w", err)
	}

	return nil
}

// applyDefaultTemplate varsayılan şablonu uygular
func (te *TemplateEngine) applyDefaultTemplate(notification *Notification, channel NotificationChannel) error {
	switch channel {
	case Email:
		return te.applyDefaultEmailTemplate(notification)
	case Telegram:
		return te.applyDefaultTelegramTemplate(notification)
	default:
		return fmt.Errorf("desteklenmeyen kanal: %s", channel)
	}
}

// applyDefaultEmailTemplate varsayılan email şablonunu uygular
func (te *TemplateEngine) applyDefaultEmailTemplate(notification *Notification) error {
	notification.Channel = Email

	// Basit HTML şablon
	htmlTemplate := `
<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <title>{{.Title}}</title>
    <style>
        body { font-family: Arial, sans-serif; line-height: 1.6; color: #333; }
        .container { max-width: 600px; margin: 0 auto; padding: 20px; }
        .header { background: #f4f4f4; padding: 20px; border-radius: 5px; }
        .content { padding: 20px 0; }
        .footer { background: #f4f4f4; padding: 10px; border-radius: 5px; font-size: 12px; color: #666; }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1>{{.Title}}</h1>
        </div>
        <div class="content">
            <p>{{.Message}}</p>
        </div>
        <div class="footer">
            <p>Bu bildirim SysTrack tarafından otomatik olarak gönderilmiştir.</p>
        </div>
    </div>
</body>
</html>`

	// Template'i render et
	tmpl, err := template.New("email").Parse(htmlTemplate)
	if err != nil {
		return fmt.Errorf("email şablonu parse edilemedi: %w", err)
	}

	var buf strings.Builder
	err = tmpl.Execute(&buf, notification)
	if err != nil {
		return fmt.Errorf("email şablonu render edilemedi: %w", err)
	}

	notification.Message = buf.String()
	return nil
}

// applyDefaultTelegramTemplate varsayılan telegram şablonunu uygular
func (te *TemplateEngine) applyDefaultTelegramTemplate(notification *Notification) error {
	notification.Channel = Telegram

	// Telegram için basit metin şablon
	textTemplate := `🎯 *{{.Title}}*

{{.Message}}

📅 {{.CreatedAt.Format "2006-01-02 15:04:05"}}`

	tmpl, err := template.New("telegram").Parse(textTemplate)
	if err != nil {
		return fmt.Errorf("telegram şablonu parse edilemedi: %w", err)
	}

	var buf strings.Builder
	err = tmpl.Execute(&buf, notification)
	if err != nil {
		return fmt.Errorf("telegram şablonu render edilemedi: %w", err)
	}

	notification.Message = buf.String()
	return nil
}

// renderTemplate şablonu render eder
func (te *TemplateEngine) renderTemplate(tmpl *NotificationTemplate, notification *Notification) error {
	// Title şablonunu render et
	titleTmpl, err := template.New("title").Parse(tmpl.Title)
	if err != nil {
		return fmt.Errorf("title şablonu parse edilemedi: %w", err)
	}

	var titleBuf strings.Builder
	err = titleTmpl.Execute(&titleBuf, notification)
	if err != nil {
		return fmt.Errorf("title şablonu render edilemedi: %w", err)
	}

	notification.Title = titleBuf.String()

	// Message şablonunu render et
	messageTmpl, err := template.New("message").Parse(tmpl.Message)
	if err != nil {
		return fmt.Errorf("message şablonu parse edilemedi: %w", err)
	}

	var messageBuf strings.Builder
	err = messageTmpl.Execute(&messageBuf, notification)
	if err != nil {
		return fmt.Errorf("message şablonu render edilemedi: %w", err)
	}

	notification.Message = messageBuf.String()
	notification.Channel = tmpl.Channel

	return nil
}

// CreateDefaultTemplates varsayılan şablonları oluşturur
func (te *TemplateEngine) CreateDefaultTemplates() error {
	templates := []NotificationTemplate{
		{
			Type:     TargetStatusChange,
			Channel:  Email,
			Title:    "🎯 Hedef Durum Değişikliği: {{.Title}}",
			Message:  te.getDefaultEmailTemplate(),
			IsActive: true,
		},
		{
			Type:     TargetStatusChange,
			Channel:  Telegram,
			Title:    "🎯 Hedef Durum Değişikliği",
			Message:  "🎯 *{{.Title}}*\n\n{{.Message}}\n\n📅 {{.CreatedAt.Format \"2006-01-02 15:04:05\"}}",
			IsActive: true,
		},
		{
			Type:     SLAViolation,
			Channel:  Email,
			Title:    "⚠️ SLA İhlali: {{.Title}}",
			Message:  te.getDefaultSLAEmailTemplate(),
			IsActive: true,
		},
		{
			Type:     SLAViolation,
			Channel:  Telegram,
			Title:    "⚠️ SLA İhlali",
			Message:  "⚠️ *SLA İhlali*\n\n{{.Message}}\n\n📅 {{.CreatedAt.Format \"2006-01-02 15:04:05\"}}",
			IsActive: true,
		},
	}

	for _, tmpl := range templates {
		query := `
			INSERT INTO notification_templates (type, channel, title, message, is_active, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, NOW(), NOW())
			ON DUPLICATE KEY UPDATE
			title = VALUES(title),
			message = VALUES(message),
			is_active = VALUES(is_active),
			updated_at = NOW()
		`

		_, err := te.db.Exec(query, tmpl.Type, tmpl.Channel, tmpl.Title, tmpl.Message, tmpl.IsActive)
		if err != nil {
			log.Printf("Şablon oluşturulamadı: %v", err)
		}
	}

	return nil
}

// getDefaultEmailTemplate varsayılan email şablonunu döndürür
func (te *TemplateEngine) getDefaultEmailTemplate() string {
	return `
<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <title>{{.Title}}</title>
    <style>
        body { font-family: 'Segoe UI', Tahoma, Geneva, Verdana, sans-serif; line-height: 1.6; color: #333; margin: 0; padding: 0; background-color: #f5f5f5; }
        .container { max-width: 600px; margin: 20px auto; background: white; border-radius: 12px; overflow: hidden; box-shadow: 0 4px 6px rgba(0,0,0,0.1); }
        .header { background: linear-gradient(135deg, #667eea 0%, #764ba2 100%); color: white; padding: 30px; text-align: center; }
        .header h1 { margin: 0; font-size: 24px; font-weight: 600; }
        .content { padding: 30px; }
        .status-badge { display: inline-block; padding: 8px 16px; border-radius: 20px; font-weight: bold; font-size: 14px; margin: 10px 0; }
        .status-online { background: #d4edda; color: #155724; }
        .status-offline { background: #f8d7da; color: #721c24; }
        .info-card { background: #f8f9fa; border-radius: 8px; padding: 20px; margin: 20px 0; border-left: 4px solid #007bff; }
        .footer { background: #f8f9fa; padding: 20px; text-align: center; color: #6c757d; font-size: 12px; }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1>🎯 Hedef Durum Değişikliği</h1>
            <p>SysTrack Monitoring Sistemi</p>
        </div>
        <div class="content">
            <h2>{{.Title}}</h2>
            <p>{{.Message}}</p>
        </div>
        <div class="footer">
            <p>Bu bildirim SysTrack monitoring sistemi tarafından otomatik olarak gönderilmiştir.</p>
        </div>
    </div>
</body>
</html>`
}

// getDefaultSLAEmailTemplate varsayılan SLA email şablonunu döndürür
func (te *TemplateEngine) getDefaultSLAEmailTemplate() string {
	return `
<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <title>{{.Title}}</title>
    <style>
        body { font-family: 'Segoe UI', Tahoma, Geneva, Verdana, sans-serif; line-height: 1.6; color: #333; margin: 0; padding: 0; background-color: #f5f5f5; }
        .container { max-width: 600px; margin: 20px auto; background: white; border-radius: 12px; overflow: hidden; box-shadow: 0 4px 6px rgba(0,0,0,0.1); }
        .header { background: linear-gradient(135deg, #dc3545 0%, #c82333 100%); color: white; padding: 30px; text-align: center; }
        .header h1 { margin: 0; font-size: 24px; font-weight: 600; }
        .content { padding: 30px; }
        .alert-card { background: #f8d7da; border-radius: 8px; padding: 20px; margin: 20px 0; border-left: 4px solid #dc3545; }
        .footer { background: #f8f9fa; padding: 20px; text-align: center; color: #6c757d; font-size: 12px; }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1>⚠️ SLA İhlali</h1>
            <p>SysTrack Monitoring Sistemi</p>
        </div>
        <div class="content">
            <h2>{{.Title}}</h2>
            <div class="alert-card">
                <p><strong>{{.Message}}</strong></p>
            </div>
        </div>
        <div class="footer">
            <p>Bu bildirim SysTrack monitoring sistemi tarafından otomatik olarak gönderilmiştir.</p>
        </div>
    </div>
</body>
</html>`
}
