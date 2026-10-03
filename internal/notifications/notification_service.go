package notifications

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"time"
)

// notificationService ana bildirim servisi
type notificationService struct {
	db              *sql.DB
	emailService    EmailService
	telegramService TelegramService
	whatsappService WhatsAppService
	webhookService  WebhookService
}

// NewNotificationService yeni bildirim servisi oluştur
func NewNotificationService(db *sql.DB, config NotificationConfig) Service {
	// Email logger oluştur
	emailLogger := NewEmailLogger(db)

	return &notificationService{
		db:              db,
		emailService:    NewEmailServiceWithDB(config.Email, emailLogger, db),
		telegramService: NewTelegramService(db),
		whatsappService: NewWhatsAppService(config.WhatsApp),
		webhookService:  NewWebhookService(config.Webhook),
	}
}

// SendNotification bildirim gönder
func (s *notificationService) SendNotification(ctx context.Context, notification *Notification) error {
	// Bildirimi veritabanına kaydet
	err := s.saveNotification(notification)
	if err != nil {
		return fmt.Errorf("failed to save notification: %w", err)
	}

	// Recipients'ı parse et - bulk notification kontrolü
	var recipients interface{}
	if len(notification.Recipients) > 0 && len(notification.Recipients[0]) > 0 {
		err := json.Unmarshal([]byte(notification.Recipients[0]), &recipients)
		if err == nil {
			// JSON parse edildi, bulk notification olabilir
			if recipMap, ok := recipients.(map[string]interface{}); ok {
				if targets, hasTargets := recipMap["targets"]; hasTargets {
					// Bulk notification
					if targetsList, ok := targets.([]interface{}); ok && len(targetsList) > 1 {
						return s.sendBulkNotification(ctx, notification)
					}
				}
			}
		}
	}

	// Normal (single) notification gönder
	// Kanal servisine göre bildirim gönder
	switch notification.Channel {
	case Email:
		err = s.emailService.Send(ctx, notification)
	case Telegram:
		err = s.telegramService.Send(ctx, notification)
	case WhatsApp:
		err = s.whatsappService.Send(ctx, notification)
	case Webhook:
		err = s.webhookService.Send(ctx, notification)
	default:
		err = fmt.Errorf("unsupported notification channel: %s", notification.Channel)
	}

	// Sonucu güncelle
	if err != nil {
		s.updateNotificationStatus(notification.ID, "failed", err.Error())
		return fmt.Errorf("failed to send notification: %w", err)
	}

	s.updateNotificationStatus(notification.ID, "sent", "")
	return nil
}

// sendBulkNotification bulk notification gönder
func (s *notificationService) sendBulkNotification(ctx context.Context, notification *Notification) error {
	// Recipients JSON'unu BulkNotificationRecipients'a parse et
	var bulkRecipients BulkNotificationRecipients
	if len(notification.Recipients) > 0 {
		err := json.Unmarshal([]byte(notification.Recipients[0]), &bulkRecipients)
		if err != nil {
			s.updateNotificationStatus(notification.ID, "failed", "invalid bulk recipients format")
			return fmt.Errorf("failed to parse bulk recipients: %w", err)
		}
	}

	// Sadece email kanalını destekliyoruz şimdilik
	var err error
	switch notification.Channel {
	case Email:
		err = s.emailService.(*emailService).SendBulk(ctx, notification, &bulkRecipients)
	default:
		err = fmt.Errorf("bulk notifications not supported for channel: %s", notification.Channel)
	}

	// Sonucu güncelle
	if err != nil {
		s.updateNotificationStatus(notification.ID, "failed", err.Error())
		return fmt.Errorf("failed to send bulk notification: %w", err)
	}

	s.updateNotificationStatus(notification.ID, "sent", "")
	return nil
}

// SendBulkNotifications toplu bildirim gönder
func (s *notificationService) SendBulkNotifications(ctx context.Context, notifications []*Notification) error {
	for _, notification := range notifications {
		err := s.SendNotification(ctx, notification)
		if err != nil {
			log.Printf("Failed to send notification %d: %v", notification.ID, err)
			// Devam et, diğer bildirimleri göndermeye çalış
		}
	}
	return nil
}

// GetNotificationSettings kullanıcı bildirim ayarlarını getir
func (s *notificationService) GetNotificationSettings(ctx context.Context, userID int) ([]*NotificationSettings, error) {
	query := `
		SELECT id, user_id, channel, is_enabled, recipient, types, priority, created_at, updated_at
		FROM notification_settings 
		WHERE user_id = ?
		ORDER BY channel, created_at
	`

	rows, err := s.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to query notification settings: %w", err)
	}
	defer rows.Close()

	var settings []*NotificationSettings
	for rows.Next() {
		var setting NotificationSettings
		var typesJSON string

		err := rows.Scan(
			&setting.ID,
			&setting.UserID,
			&setting.Channel,
			&setting.IsEnabled,
			&setting.Recipient,
			&typesJSON,
			&setting.Priority,
			&setting.CreatedAt,
			&setting.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan notification settings: %w", err)
		}

		// Types JSON'unu parse et
		err = json.Unmarshal([]byte(typesJSON), &setting.Types)
		if err != nil {
			return nil, fmt.Errorf("failed to parse types JSON: %w", err)
		}

		settings = append(settings, &setting)
	}

	return settings, nil
}

// UpdateNotificationSettings bildirim ayarlarını güncelle
func (s *notificationService) UpdateNotificationSettings(ctx context.Context, settings *NotificationSettings) error {
	// Types'ı JSON'a çevir
	typesJSON, err := json.Marshal(settings.Types)
	if err != nil {
		return fmt.Errorf("failed to marshal types: %w", err)
	}

	query := `
		INSERT INTO notification_settings (user_id, channel, is_enabled, recipient, types, priority, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, NOW(), NOW())
		ON DUPLICATE KEY UPDATE
		is_enabled = VALUES(is_enabled),
		recipient = VALUES(recipient),
		types = VALUES(types),
		priority = VALUES(priority),
		updated_at = NOW()
	`

	_, err = s.db.ExecContext(ctx, query,
		settings.UserID,
		settings.Channel,
		settings.IsEnabled,
		settings.Recipient,
		string(typesJSON),
		settings.Priority,
	)

	if err != nil {
		return fmt.Errorf("failed to update notification settings: %w", err)
	}

	return nil
}

// GetNotificationTemplates şablonları getir
func (s *notificationService) GetNotificationTemplates(ctx context.Context) ([]*NotificationTemplate, error) {
	query := `
		SELECT id, type, channel, title, message, is_active, created_at, updated_at
		FROM notification_templates
		ORDER BY type, channel, created_at
	`

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query notification templates: %w", err)
	}
	defer rows.Close()

	var templates []*NotificationTemplate
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
			return nil, fmt.Errorf("failed to scan notification template: %w", err)
		}

		templates = append(templates, &template)
	}

	return templates, nil
}

// UpdateNotificationTemplate şablonu güncelle
func (s *notificationService) UpdateNotificationTemplate(ctx context.Context, template *NotificationTemplate) error {
	query := `
		INSERT INTO notification_templates (type, channel, title, message, is_active, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, NOW(), NOW())
		ON DUPLICATE KEY UPDATE
		title = VALUES(title),
		message = VALUES(message),
		is_active = VALUES(is_active),
		updated_at = NOW()
	`

	_, err := s.db.ExecContext(ctx, query,
		template.Type,
		template.Channel,
		template.Title,
		template.Message,
		template.IsActive,
	)

	if err != nil {
		return fmt.Errorf("failed to update notification template: %w", err)
	}

	return nil
}

// GetNotificationHistory bildirim geçmişini getir
func (s *notificationService) GetNotificationHistory(ctx context.Context, limit int, offset int) ([]*Notification, error) {
	query := `
		SELECT id, type, channel, priority, title, message, recipients, target_id, alert_id, 
		       status, sent_at, created_at, updated_at, error, retry_count
		FROM notifications
		ORDER BY created_at DESC
		LIMIT ? OFFSET ?
	`

	rows, err := s.db.QueryContext(ctx, query, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to query notification history: %w", err)
	}
	defer rows.Close()

	var notifications []*Notification
	for rows.Next() {
		var notification Notification
		var recipientsJSON string

		err := rows.Scan(
			&notification.ID,
			&notification.Type,
			&notification.Channel,
			&notification.Priority,
			&notification.Title,
			&notification.Message,
			&recipientsJSON,
			&notification.TargetID,
			&notification.AlertID,
			&notification.Status,
			&notification.SentAt,
			&notification.CreatedAt,
			&notification.UpdatedAt,
			&notification.Error,
			&notification.RetryCount,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan notification: %w", err)
		}

		// Recipients JSON'unu parse et
		err = json.Unmarshal([]byte(recipientsJSON), &notification.Recipients)
		if err != nil {
			return nil, fmt.Errorf("failed to parse recipients JSON: %w", err)
		}

		notifications = append(notifications, &notification)
	}

	return notifications, nil
}

// RetryFailedNotifications başarısız bildirimleri tekrar dene
func (s *notificationService) RetryFailedNotifications(ctx context.Context) error {
	query := `
		SELECT id, type, channel, priority, title, message, recipients, target_id, alert_id, 
		       status, created_at, retry_count
		FROM notifications
		WHERE status = 'failed' AND retry_count < 3
		ORDER BY created_at ASC
		LIMIT 10
	`

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to query failed notifications: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var notification Notification
		var recipientsJSON string

		err := rows.Scan(
			&notification.ID,
			&notification.Type,
			&notification.Channel,
			&notification.Priority,
			&notification.Title,
			&notification.Message,
			&recipientsJSON,
			&notification.TargetID,
			&notification.AlertID,
			&notification.Status,
			&notification.CreatedAt,
			&notification.RetryCount,
		)
		if err != nil {
			log.Printf("Failed to scan notification: %v", err)
			continue
		}

		// Recipients JSON'unu parse et
		err = json.Unmarshal([]byte(recipientsJSON), &notification.Recipients)
		if err != nil {
			log.Printf("Failed to parse recipients JSON: %v", err)
			continue
		}

		// Retry count'u artır
		notification.RetryCount++

		// Tekrar gönder
		err = s.SendNotification(ctx, &notification)
		if err != nil {
			log.Printf("Failed to retry notification %d: %v", notification.ID, err)
		}
	}

	return nil
}

// ProcessNotificationQueue kuyruktaki bildirimleri işle
func (s *notificationService) ProcessNotificationQueue(ctx context.Context) error {
	// Pending bildirimleri getir
	query := `
		SELECT id, type, channel, priority, title, message, recipients, target_id, alert_id, 
		       status, created_at, retry_count
		FROM notifications
		WHERE status = 'pending'
		ORDER BY priority DESC, created_at ASC
		LIMIT 50
	`

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to query pending notifications: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var notification Notification
		var recipientsJSON string

		err := rows.Scan(
			&notification.ID,
			&notification.Type,
			&notification.Channel,
			&notification.Priority,
			&notification.Title,
			&notification.Message,
			&recipientsJSON,
			&notification.TargetID,
			&notification.AlertID,
			&notification.Status,
			&notification.CreatedAt,
			&notification.RetryCount,
		)
		if err != nil {
			log.Printf("Failed to scan notification: %v", err)
			continue
		}

		// Recipients JSON'unu parse et
		err = json.Unmarshal([]byte(recipientsJSON), &notification.Recipients)
		if err != nil {
			log.Printf("Failed to parse recipients JSON: %v", err)
			continue
		}

		// Bildirimi gönder
		err = s.SendNotification(ctx, &notification)
		if err != nil {
			log.Printf("Failed to process notification %d: %v", notification.ID, err)
		}
	}

	return nil
}

// saveNotification bildirimi veritabanına kaydet
func (s *notificationService) saveNotification(notification *Notification) error {
	// Recipients'ı JSON'a çevir
	recipientsJSON, err := json.Marshal(notification.Recipients)
	if err != nil {
		return fmt.Errorf("failed to marshal recipients: %w", err)
	}

	query := `
		INSERT INTO notifications (type, channel, priority, title, message, recipients, target_id, alert_id, status, created_at, updated_at, retry_count)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'pending', NOW(), NOW(), 0)
	`

	result, err := s.db.Exec(query,
		notification.Type,
		notification.Channel,
		notification.Priority,
		notification.Title,
		notification.Message,
		string(recipientsJSON),
		notification.TargetID,
		notification.AlertID,
	)

	if err != nil {
		return fmt.Errorf("failed to insert notification: %w", err)
	}

	// ID'yi al
	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("failed to get last insert id: %w", err)
	}

	notification.ID = int(id)
	return nil
}

// updateNotificationStatus bildirim durumunu güncelle
func (s *notificationService) updateNotificationStatus(id int, status string, errorMsg string) {
	var sentAt interface{}
	if status == "sent" {
		sentAt = time.Now()
	}

	query := `
		UPDATE notifications 
		SET status = ?, sent_at = ?, error = ?, updated_at = NOW()
		WHERE id = ?
	`

	_, err := s.db.Exec(query, status, sentAt, errorMsg, id)
	if err != nil {
		log.Printf("Failed to update notification status: %v", err)
	}
}
