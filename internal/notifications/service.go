package notifications

import "context"

// Service bildirim servisi interface'i
type Service interface {
	// SendNotification bildirim gönder
	SendNotification(ctx context.Context, notification *Notification) error

	// SendBulkNotifications toplu bildirim gönder
	SendBulkNotifications(ctx context.Context, notifications []*Notification) error

	// GetNotificationSettings kullanıcı bildirim ayarlarını getir
	GetNotificationSettings(ctx context.Context, userID int) ([]*NotificationSettings, error)

	// UpdateNotificationSettings bildirim ayarlarını güncelle
	UpdateNotificationSettings(ctx context.Context, settings *NotificationSettings) error

	// GetNotificationTemplates şablonları getir
	GetNotificationTemplates(ctx context.Context) ([]*NotificationTemplate, error)

	// UpdateNotificationTemplate şablonu güncelle
	UpdateNotificationTemplate(ctx context.Context, template *NotificationTemplate) error

	// GetNotificationHistory bildirim geçmişini getir
	GetNotificationHistory(ctx context.Context, limit int, offset int) ([]*Notification, error)

	// RetryFailedNotifications başarısız bildirimleri tekrar dene
	RetryFailedNotifications(ctx context.Context) error

	// ProcessNotificationQueue kuyruktaki bildirimleri işle
	ProcessNotificationQueue(ctx context.Context) error
}

// ChannelService kanal spesifik servis interface'i
type ChannelService interface {
	// Send kanala bildirim gönder
	Send(ctx context.Context, notification *Notification) error

	// ValidateConfig konfigürasyonu doğrula
	ValidateConfig(config interface{}) error

	// TestConnection bağlantıyı test et
	TestConnection(ctx context.Context) error
}

// EmailService mail servisi
type EmailService interface {
	ChannelService

	// SendEmail mail gönder
	SendEmail(ctx context.Context, to []string, subject, body string) error

	// SendHTMLEmail HTML mail gönder
	SendHTMLEmail(ctx context.Context, to []string, subject, htmlBody string) error
}

// TelegramService Telegram servisi
type TelegramService interface {
	ChannelService

	// SendMessage mesaj gönder
	SendMessage(ctx context.Context, chatID string, message string) error

	// SendPhoto fotoğraf gönder
	SendPhoto(ctx context.Context, chatID string, photo []byte, caption string) error

	// SetWebhook webhook ayarla
	SetWebhook(ctx context.Context, webhookURL string) error

	// GetUpdates güncellemeleri getir
	GetUpdates(ctx context.Context) error

	// SetConfig konfigürasyonu ayarla
	SetConfig(config interface{}) error

	// SendTestMessage test mesajı gönder
	SendTestMessage(ctx context.Context, chatID string) error
}

// WhatsAppService WhatsApp servisi
type WhatsAppService interface {
	ChannelService

	// SendMessage mesaj gönder
	SendMessage(ctx context.Context, phoneNumber string, message string) error

	// SendTemplate template mesaj gönder
	SendTemplate(ctx context.Context, phoneNumber string, templateName string, parameters []string) error

	// SetWebhook webhook ayarla
	SetWebhook(ctx context.Context, webhookURL string) error
}

// WebhookService webhook servisi
type WebhookService interface {
	ChannelService

	// SendWebhook webhook gönder
	SendWebhook(ctx context.Context, url string, payload interface{}) error

	// ValidateSignature imzayı doğrula
	ValidateSignature(payload []byte, signature string, secret string) bool
}
