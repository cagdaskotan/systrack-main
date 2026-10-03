package notifications

import "time"

// NotificationType bildirim türlerini tanımlar
type NotificationType string

const (
	// TargetStatusChange hedef durumu değişikliği
	TargetStatusChange NotificationType = "target_status_change"
	// SLAViolation SLA ihlali
	SLAViolation NotificationType = "sla_violation"
	// AlertOpened yeni uyarı açıldı
	AlertOpened NotificationType = "alert_opened"
	// AlertClosed uyarı kapandı
	AlertClosed NotificationType = "alert_closed"
	// SystemHealth sistem sağlık durumu
	SystemHealth NotificationType = "system_health"
	// DailyReport günlük rapor
	DailyReport NotificationType = "daily_report"
)

// NotificationChannel bildirim kanallarını tanımlar
type NotificationChannel string

const (
	// Email mail bildirimi
	Email NotificationChannel = "email"
	// Telegram Telegram bildirimi
	Telegram NotificationChannel = "telegram"
	// WhatsApp WhatsApp bildirimi
	WhatsApp NotificationChannel = "whatsapp"
	// Webhook webhook bildirimi
	Webhook NotificationChannel = "webhook"
	// All tüm kanallar (şablonlar için)
	All NotificationChannel = "all"
)

// NotificationPriority bildirim önceliğini tanımlar
type NotificationPriority string

const (
	// Low düşük öncelik
	Low NotificationPriority = "low"
	// Medium orta öncelik
	Medium NotificationPriority = "medium"
	// High yüksek öncelik
	High NotificationPriority = "high"
	// Critical kritik öncelik
	Critical NotificationPriority = "critical"
)

// EmailAttachment email ek dosyası
type EmailAttachment struct {
	ID          int       `json:"id" db:"id"`
	Filename    string    `json:"filename" db:"filename"`
	ContentType string    `json:"content_type" db:"content_type"`
	Size        int64     `json:"size" db:"size"`
	Data        []byte    `json:"data" db:"data"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
}

// Notification bildirim yapısı
type Notification struct {
	ID          int                  `json:"id" db:"id"`
	Type        NotificationType     `json:"type" db:"type"`
	Channel     NotificationChannel  `json:"channel" db:"channel"`
	Priority    NotificationPriority `json:"priority" db:"priority"`
	Title       string               `json:"title" db:"title"`
	Message     string               `json:"message" db:"message"`
	Recipients  []string             `json:"recipients" db:"recipients"`
	TargetID    *int                 `json:"target_id" db:"target_id"`
	AlertID     *int                 `json:"alert_id" db:"alert_id"`
	Status      string               `json:"status" db:"status"` // pending, sent, failed
	SentAt      *time.Time           `json:"sent_at" db:"sent_at"`
	CreatedAt   time.Time            `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time            `json:"updated_at" db:"updated_at"`
	Error       *string              `json:"error" db:"error"`
	RetryCount  int                  `json:"retry_count" db:"retry_count"`
	Attachments []EmailAttachment    `json:"attachments,omitempty" db:"attachments"`
}

// NotificationTemplate bildirim şablonu
type NotificationTemplate struct {
	ID        int                 `json:"id" db:"id"`
	Name      string              `json:"name" db:"name"`
	Type      NotificationType    `json:"type" db:"type"`
	Channel   NotificationChannel `json:"channel" db:"channel"`
	Title     string              `json:"title" db:"title"`
	Message   string              `json:"message" db:"message"`
	IsDefault bool                `json:"is_default" db:"is_default"`
	IsActive  bool                `json:"is_active" db:"is_active"`
	UserID    *int                `json:"user_id" db:"user_id"`
	CreatedAt time.Time           `json:"created_at" db:"created_at"`
	UpdatedAt time.Time           `json:"updated_at" db:"updated_at"`
}

// NotificationSettings kullanıcı bildirim ayarları
type NotificationSettings struct {
	ID        int                  `json:"id" db:"id"`
	UserID    int                  `json:"user_id" db:"user_id"`
	Channel   NotificationChannel  `json:"channel" db:"channel"`
	IsEnabled bool                 `json:"is_enabled" db:"is_enabled"`
	Recipient string               `json:"recipient" db:"recipient"` // email, phone, chat_id
	Types     []NotificationType   `json:"types" db:"types"`
	Priority  NotificationPriority `json:"priority" db:"priority"`
	CreatedAt time.Time            `json:"created_at" db:"created_at"`
	UpdatedAt time.Time            `json:"updated_at" db:"updated_at"`
}

// NotificationData bildirim için veri yapısı
type NotificationData struct {
	TargetName    string  `json:"target_name"`
	TargetURL     string  `json:"target_url"`
	Status        string  `json:"status"`
	ResponseTime  float64 `json:"response_time"`
	UptimePercent float64 `json:"uptime_percent"`
	AlertMessage  string  `json:"alert_message"`
	AlertLevel    string  `json:"alert_level"`
	Timestamp     string  `json:"timestamp"`
	SLATarget     float64 `json:"sla_target"` // SLA threshold değeri
	SLABreach     bool    `json:"sla_breach"` // SLA ihlal durumu
}

// BulkNotificationRecipients bulk notification için recipients yapısı
type BulkNotificationRecipients struct {
	Emails  []string                  `json:"emails"`
	Targets []TargetNotificationData  `json:"targets,omitempty"`
}

// TargetNotificationData tek bir hedefin bildirim verisi
type TargetNotificationData struct {
	ID            int      `json:"id"`
	Name          string   `json:"name"`
	Address       string   `json:"address"`
	Status        string   `json:"status"`
	StatusClass   string   `json:"status_class"`
	UptimePercent *float64 `json:"uptime_percent,omitempty"`
	ResponseTime  *float64 `json:"response_time,omitempty"`
}

// NotificationConfig bildirim konfigürasyonu
type NotificationConfig struct {
	Email    EmailConfig    `json:"email"`
	Telegram TelegramConfig `json:"telegram"`
	WhatsApp WhatsAppConfig `json:"whatsapp"`
	Webhook  WebhookConfig  `json:"webhook"`
}

// EmailConfig mail konfigürasyonu
type EmailConfig struct {
	Enabled           bool   `json:"enabled"`
	SMTPHost          string `json:"smtp_host"`
	SMTPPort          int    `json:"smtp_port"`
	Username          string `json:"username"`
	Password          string `json:"password"`
	FromEmail         string `json:"from_email"`
	FromName          string `json:"from_name"`
	UseTLS            bool   `json:"use_tls"`
	NotificationEmail string `json:"notification_email"`
}

// TelegramConfig Telegram konfigürasyonu
type TelegramConfig struct {
	Enabled     bool   `json:"enabled"`
	BotToken    string `json:"bot_token"`
	DefaultChat string `json:"default_chat"`
	WebhookURL  string `json:"webhook_url"`
}

// WhatsAppConfig WhatsApp konfigürasyonu
type WhatsAppConfig struct {
	Enabled     bool   `json:"enabled"`
	AccessToken string `json:"access_token"`
	PhoneID     string `json:"phone_id"`
	WebhookURL  string `json:"webhook_url"`
}

// WebhookConfig webhook konfigürasyonu
type WebhookConfig struct {
	Enabled bool   `json:"enabled"`
	URL     string `json:"url"`
	Secret  string `json:"secret"`
}
