package notifications

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"
)

// TelegramServiceImpl Telegram servisi implementasyonu
type TelegramServiceImpl struct {
	db     *sql.DB
	config TelegramConfig
	client *http.Client
}

// NewTelegramService yeni Telegram servisi oluşturur
func NewTelegramService(db *sql.DB) TelegramService {
	return &TelegramServiceImpl{
		db: db,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// SetConfig Telegram konfigürasyonunu ayarlar
func (s *TelegramServiceImpl) SetConfig(config interface{}) error {
	telegramConfig, ok := config.(TelegramConfig)
	if !ok {
		return fmt.Errorf("invalid telegram config type")
	}
	s.config = telegramConfig
	return nil
}

// SendMessage Telegram mesajı gönderir
func (s *TelegramServiceImpl) SendMessage(ctx context.Context, chatID string, message string) error {
	if !s.config.Enabled || s.config.BotToken == "" {
		return fmt.Errorf("telegram service not configured or disabled")
	}

	// Chat ID belirtilmemişse varsayılan kullan
	if chatID == "" {
		chatID = s.config.DefaultChat
	}

	telegramMsg := map[string]interface{}{
		"chat_id":    chatID,
		"text":       message,
		"parse_mode": "HTML",
	}

	jsonData, err := json.Marshal(telegramMsg)
	if err != nil {
		return fmt.Errorf("failed to marshal telegram message: %w", err)
	}

	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", s.config.BotToken)
	
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create telegram request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send telegram message: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("telegram API returned status %d", resp.StatusCode)
	}

	log.Printf("Telegram message sent to chat %s", chatID)
	return nil
}

// SendPhoto fotoğraf gönderir
func (s *TelegramServiceImpl) SendPhoto(ctx context.Context, chatID string, photo []byte, caption string) error {
	// Şimdilik implement edilmedi
	return fmt.Errorf("send photo not implemented")
}

// SetWebhook webhook ayarlar
func (s *TelegramServiceImpl) SetWebhook(ctx context.Context, webhookURL string) error {
	// Şimdilik implement edilmedi
	return fmt.Errorf("set webhook not implemented")
}

// GetUpdates güncellemeleri getirir
func (s *TelegramServiceImpl) GetUpdates(ctx context.Context) error {
	// Şimdilik implement edilmedi
	return fmt.Errorf("get updates not implemented")
}

// Send bildirim gönderir
func (s *TelegramServiceImpl) Send(ctx context.Context, notification *Notification) error {
	message := s.formatNotificationMessage(notification)
	return s.SendMessage(ctx, "", message)
}

// SendTestMessage test mesajı gönderir
func (s *TelegramServiceImpl) SendTestMessage(ctx context.Context, chatID string) error {
	message := fmt.Sprintf(`
🤖 <b>Systrack Test Mesajı</b>

Bu bir test mesajıdır. Telegram entegrasyonu başarıyla çalışıyor!

📅 Zaman: %s
✅ Durum: Aktif
	`, time.Now().Format("2006-01-02 15:04:05"))
	
	return s.SendMessage(ctx, chatID, message)
}

// formatNotificationMessage bildirimi Telegram formatında düzenler
func (s *TelegramServiceImpl) formatNotificationMessage(notification *Notification) string {
	var emoji string
	var priorityText string
	
	switch notification.Priority {
	case "low":
		emoji = "ℹ️"
		priorityText = "Düşük"
	case "medium":
		emoji = "⚠️"
		priorityText = "Orta"
	case "high":
		emoji = "🚨"
		priorityText = "Yüksek"
	case "critical":
		emoji = "🔥"
		priorityText = "Kritik"
	default:
		emoji = "📢"
		priorityText = "Normal"
	}

	message := fmt.Sprintf(`
%s <b>Systrack Bildirimi</b>

<b>Başlık:</b> %s
<b>Öncelik:</b> %s
<b>Tür:</b> %s
<b>Kanal:</b> %s

<b>Mesaj:</b>
%s

📅 <i>%s</i>
	`, emoji, notification.Title, priorityText, notification.Type, notification.Channel, 
	   notification.Message, time.Now().Format("2006-01-02 15:04:05"))

	return message
}

// SendTargetStatusMessage target durum değişikliği mesajı gönderir
func (s *TelegramServiceImpl) SendTargetStatusMessage(ctx context.Context, targetName string, status string, targetID int) error {
	var emoji string
	var statusText string
	
	if status == "online" {
		emoji = "✅"
		statusText = "Çevrimiçi"
	} else {
		emoji = "❌"
		statusText = "Çevrimdışı"
	}

	message := fmt.Sprintf(`
%s <b>Target Durum Değişikliği</b>

<b>Target:</b> %s
<b>ID:</b> %d
<b>Durum:</b> %s

📅 <i>%s</i>
	`, emoji, targetName, targetID, statusText, time.Now().Format("2006-01-02 15:04:05"))

	return s.SendMessage(ctx, "", message)
}

// SendSLABreachMessage SLA ihlali mesajı gönderir
func (s *TelegramServiceImpl) SendSLABreachMessage(ctx context.Context, targetName string, targetID int, slaTarget float64, currentUptime float64) error {
	message := fmt.Sprintf(`
🔥 <b>SLA İhlali Bildirimi</b>

<b>Target:</b> %s
<b>ID:</b> %d
<b>SLA Hedefi:</b> %.2f%%
<b>Mevcut Uptime:</b> %.2f%%
<b>İhlal Miktarı:</b> %.2f%%

⚠️ <b>Bu kritik bir SLA ihlalidir!</b>

📅 <i>%s</i>
	`, targetName, targetID, slaTarget, currentUptime, slaTarget-currentUptime, 
	   time.Now().Format("2006-01-02 15:04:05"))

	return s.SendMessage(ctx, "", message)
}

// TestConnection Telegram bağlantısını test eder
func (s *TelegramServiceImpl) TestConnection(ctx context.Context) error {
	if !s.config.Enabled || s.config.BotToken == "" {
		return fmt.Errorf("telegram service not configured")
	}

	url := fmt.Sprintf("https://api.telegram.org/bot%s/getMe", s.config.BotToken)
	
	resp, err := s.client.Get(url)
	if err != nil {
		return fmt.Errorf("failed to test telegram connection: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("telegram API returned status %d", resp.StatusCode)
	}

	return nil
}

// ValidateConfig konfigürasyonu doğrular
func (s *TelegramServiceImpl) ValidateConfig(config interface{}) error {
	telegramConfig, ok := config.(TelegramConfig)
	if !ok {
		return fmt.Errorf("invalid telegram config type")
	}

	if telegramConfig.BotToken == "" {
		return fmt.Errorf("bot token is required")
	}

	if telegramConfig.DefaultChat == "" {
		return fmt.Errorf("default chat is required")
	}

	return nil
}
