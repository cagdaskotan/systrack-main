package notifications

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// whatsappService WhatsApp servisi implementasyonu
type whatsappService struct {
	config WhatsAppConfig
	client *http.Client
}

// WhatsAppMessage WhatsApp mesaj yapısı
type WhatsAppMessage struct {
	MessagingProduct string `json:"messaging_product"`
	To               string `json:"to"`
	Type             string `json:"type"`
	Text             struct {
		Body string `json:"body"`
	} `json:"text"`
}

// WhatsAppTemplateMessage WhatsApp template mesaj yapısı
type WhatsAppTemplateMessage struct {
	MessagingProduct string `json:"messaging_product"`
	To               string `json:"to"`
	Type             string `json:"type"`
	Template         struct {
		Name     string `json:"name"`
		Language struct {
			Code string `json:"code"`
		} `json:"language"`
		Components []struct {
			Type       string `json:"type"`
			Parameters []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"parameters"`
		} `json:"components"`
	} `json:"template"`
}

// WhatsAppResponse WhatsApp API yanıtı
type WhatsAppResponse struct {
	MessagingProduct string `json:"messaging_product"`
	Contacts         []struct {
		Input string `json:"input"`
		WAID  string `json:"wa_id"`
	} `json:"contacts"`
	Messages []struct {
		ID string `json:"id"`
	} `json:"messages"`
}

// NewWhatsAppService yeni WhatsApp servisi oluştur
func NewWhatsAppService(config WhatsAppConfig) WhatsAppService {
	return &whatsappService{
		config: config,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Send bildirim gönder
func (s *whatsappService) Send(ctx context.Context, notification *Notification) error {
	if !s.config.Enabled {
		return fmt.Errorf("whatsapp notifications are disabled")
	}

	// Mesaj formatla
	message := s.formatMessage(notification)

	// Her recipient için mesaj gönder
	for _, recipient := range notification.Recipients {
		err := s.SendMessage(ctx, recipient, message)
		if err != nil {
			return fmt.Errorf("failed to send to %s: %w", recipient, err)
		}
	}

	return nil
}

// SendMessage mesaj gönder
func (s *whatsappService) SendMessage(ctx context.Context, phoneNumber string, message string) error {
	if !s.config.Enabled {
		return fmt.Errorf("whatsapp notifications are disabled")
	}

	// API URL
	url := fmt.Sprintf("https://graph.facebook.com/v18.0/%s/messages", s.config.PhoneID)

	// Mesaj yapısı
	whatsappMsg := WhatsAppMessage{
		MessagingProduct: "whatsapp",
		To:               phoneNumber,
		Type:             "text",
		Text: struct {
			Body string `json:"body"`
		}{
			Body: message,
		},
	}

	// JSON encode
	jsonData, err := json.Marshal(whatsappMsg)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	// HTTP request
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.config.AccessToken)

	// Request gönder
	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Response oku
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	// Response parse et
	var whatsappResp WhatsAppResponse
	err = json.Unmarshal(body, &whatsappResp)
	if err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	// Başarı kontrolü
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("whatsapp API error: status %d, body: %s", resp.StatusCode, string(body))
	}

	return nil
}

// SendTemplate template mesaj gönder
func (s *whatsappService) SendTemplate(ctx context.Context, phoneNumber string, templateName string, parameters []string) error {
	if !s.config.Enabled {
		return fmt.Errorf("whatsapp notifications are disabled")
	}

	// API URL
	url := fmt.Sprintf("https://graph.facebook.com/v18.0/%s/messages", s.config.PhoneID)

	// Template mesaj yapısı
	whatsappMsg := WhatsAppTemplateMessage{
		MessagingProduct: "whatsapp",
		To:               phoneNumber,
		Type:             "template",
		Template: struct {
			Name     string `json:"name"`
			Language struct {
				Code string `json:"code"`
			} `json:"language"`
			Components []struct {
				Type       string `json:"type"`
				Parameters []struct {
					Type string `json:"type"`
					Text string `json:"text"`
				} `json:"parameters"`
			} `json:"components"`
		}{
			Name: templateName,
			Language: struct {
				Code string `json:"code"`
			}{
				Code: "tr",
			},
			Components: []struct {
				Type       string `json:"type"`
				Parameters []struct {
					Type string `json:"type"`
					Text string `json:"text"`
				} `json:"parameters"`
			}{
				{
					Type: "body",
					Parameters: func() []struct {
						Type string `json:"type"`
						Text string `json:"text"`
					} {
						var params []struct {
							Type string `json:"type"`
							Text string `json:"text"`
						}
						for _, param := range parameters {
							params = append(params, struct {
								Type string `json:"type"`
								Text string `json:"text"`
							}{
								Type: "text",
								Text: param,
							})
						}
						return params
					}(),
				},
			},
		},
	}

	// JSON encode
	jsonData, err := json.Marshal(whatsappMsg)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	// HTTP request
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.config.AccessToken)

	// Request gönder
	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Response oku
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	// Response parse et
	var whatsappResp WhatsAppResponse
	err = json.Unmarshal(body, &whatsappResp)
	if err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	// Başarı kontrolü
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("whatsapp API error: status %d, body: %s", resp.StatusCode, string(body))
	}

	return nil
}

// SetWebhook webhook ayarla
func (s *whatsappService) SetWebhook(ctx context.Context, webhookURL string) error {
	if !s.config.Enabled {
		return fmt.Errorf("whatsapp notifications are disabled")
	}

	// API URL
	url := fmt.Sprintf("https://graph.facebook.com/v18.0/%s/subscribed_apps", s.config.PhoneID)

	// Webhook data
	data := map[string]string{
		"callback_url": webhookURL,
		"verify_token": "systrack_webhook_verify",
		"fields":       "messages,messaging_postbacks",
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal data: %w", err)
	}

	// HTTP request
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.config.AccessToken)

	// Request gönder
	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Response oku
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	// Başarı kontrolü
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("whatsapp webhook error: status %d, body: %s", resp.StatusCode, string(body))
	}

	return nil
}

// ValidateConfig konfigürasyonu doğrula
func (s *whatsappService) ValidateConfig(config interface{}) error {
	whatsappConfig, ok := config.(WhatsAppConfig)
	if !ok {
		return fmt.Errorf("invalid config type")
	}

	if whatsappConfig.AccessToken == "" {
		return fmt.Errorf("access token is required")
	}

	if whatsappConfig.PhoneID == "" {
		return fmt.Errorf("phone ID is required")
	}

	return nil
}

// TestConnection bağlantıyı test et
func (s *whatsappService) TestConnection(ctx context.Context) error {
	if !s.config.Enabled {
		return fmt.Errorf("whatsapp notifications are disabled")
	}

	// Test mesajı gönder (kendi numarasına)
	testMessage := "🚀 SysTrack WhatsApp bildirim sistemi test ediliyor!\n\n✅ Bağlantı başarılı!"

	// Test için kendi numarasını kullan (config'den alınabilir)
	return s.SendMessage(ctx, "test_number", testMessage)
}

// formatMessage mesajı formatla
func (s *whatsappService) formatMessage(notification *Notification) string {
	var emoji string

	// Öncelik emojisi
	switch notification.Priority {
	case Critical:
		emoji = "🔴"
	case High:
		emoji = "🟠"
	case Medium:
		emoji = "🟡"
	case Low:
		emoji = "🟢"
	default:
		emoji = "📢"
	}

	// Tür emojisi
	switch notification.Type {
	case TargetStatusChange:
		emoji = "🔄"
	case SLAViolation:
		emoji = "⚠️"
	case AlertOpened:
		emoji = "🚨"
	case AlertClosed:
		emoji = "✅"
	case SystemHealth:
		emoji = "💚"
	case DailyReport:
		emoji = "📊"
	}

	// Mesaj formatla
	message := fmt.Sprintf(`%s *%s*

%s

*📋 Detaylar:*
• Tür: %s
• Öncelik: %s
• Zaman: %s

_SysTrack Monitoring Sistemi_`,
		emoji,
		notification.Title,
		notification.Message,
		notification.Type,
		notification.Priority,
		notification.CreatedAt.Format("2006-01-02 15:04:05"),
	)

	return message
}
