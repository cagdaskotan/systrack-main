package notifications

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// webhookService webhook servisi implementasyonu
type webhookService struct {
	config WebhookConfig
	client *http.Client
}

// WebhookPayload webhook payload yapısı
type WebhookPayload struct {
	ID         int                  `json:"id"`
	Type       NotificationType     `json:"type"`
	Channel    NotificationChannel  `json:"channel"`
	Priority   NotificationPriority `json:"priority"`
	Title      string               `json:"title"`
	Message    string               `json:"message"`
	Recipients []string             `json:"recipients"`
	TargetID   *int                 `json:"target_id"`
	AlertID    *int                 `json:"alert_id"`
	Status     string               `json:"status"`
	SentAt     *time.Time           `json:"sent_at"`
	CreatedAt  time.Time            `json:"created_at"`
	UpdatedAt  time.Time            `json:"updated_at"`
	Error      *string              `json:"error"`
	RetryCount int                  `json:"retry_count"`
	Timestamp  int64                `json:"timestamp"`
	Signature  string               `json:"signature,omitempty"`
}

// NewWebhookService yeni webhook servisi oluştur
func NewWebhookService(config WebhookConfig) WebhookService {
	return &webhookService{
		config: config,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Send bildirim gönder
func (s *webhookService) Send(ctx context.Context, notification *Notification) error {
	if !s.config.Enabled {
		return fmt.Errorf("webhook notifications are disabled")
	}

	// Payload oluştur
	payload := WebhookPayload{
		ID:         notification.ID,
		Type:       notification.Type,
		Channel:    notification.Channel,
		Priority:   notification.Priority,
		Title:      notification.Title,
		Message:    notification.Message,
		Recipients: notification.Recipients,
		TargetID:   notification.TargetID,
		AlertID:    notification.AlertID,
		Status:     notification.Status,
		SentAt:     notification.SentAt,
		CreatedAt:  notification.CreatedAt,
		UpdatedAt:  notification.UpdatedAt,
		Error:      notification.Error,
		RetryCount: notification.RetryCount,
		Timestamp:  time.Now().Unix(),
	}

	// Signature oluştur
	if s.config.Secret != "" {
		payload.Signature = s.generateSignature(payload)
	}

	// Webhook gönder
	return s.SendWebhook(ctx, s.config.URL, payload)
}

// SendWebhook webhook gönder
func (s *webhookService) SendWebhook(ctx context.Context, url string, payload interface{}) error {
	if !s.config.Enabled {
		return fmt.Errorf("webhook notifications are disabled")
	}

	// JSON encode
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	// HTTP request
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "SysTrack-Webhook/1.0")

	// Signature header ekle
	if s.config.Secret != "" {
		signature := s.generateSignatureFromBytes(jsonData)
		req.Header.Set("X-SysTrack-Signature", signature)
	}

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
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webhook returned status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// ValidateSignature imzayı doğrula
func (s *webhookService) ValidateSignature(payload []byte, signature string, secret string) bool {
	if secret == "" {
		return true // Secret yoksa doğrulama yapma
	}

	expectedSignature := s.generateSignatureFromBytes(payload)
	return hmac.Equal([]byte(signature), []byte(expectedSignature))
}

// ValidateConfig konfigürasyonu doğrula
func (s *webhookService) ValidateConfig(config interface{}) error {
	webhookConfig, ok := config.(WebhookConfig)
	if !ok {
		return fmt.Errorf("invalid config type")
	}

	if webhookConfig.URL == "" {
		return fmt.Errorf("webhook URL is required")
	}

	// URL formatını kontrol et
	if !(webhookConfig.URL[:7] == "http://" || webhookConfig.URL[:8] == "https://") {
		return fmt.Errorf("invalid webhook URL format")
	}

	return nil
}

// TestConnection bağlantıyı test et
func (s *webhookService) TestConnection(ctx context.Context) error {
	if !s.config.Enabled {
		return fmt.Errorf("webhook notifications are disabled")
	}

	// Test payload
	testPayload := map[string]interface{}{
		"type":      "test",
		"message":   "SysTrack webhook test",
		"timestamp": time.Now().Unix(),
		"test":      true,
	}

	return s.SendWebhook(ctx, s.config.URL, testPayload)
}

// generateSignature payload için signature oluştur
func (s *webhookService) generateSignature(payload WebhookPayload) string {
	if s.config.Secret == "" {
		return ""
	}

	// Payload'ı JSON'a çevir
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return ""
	}

	return s.generateSignatureFromBytes(jsonData)
}

// generateSignatureFromBytes byte array için signature oluştur
func (s *webhookService) generateSignatureFromBytes(data []byte) string {
	if s.config.Secret == "" {
		return ""
	}

	// HMAC-SHA256 ile signature oluştur
	h := hmac.New(sha256.New, []byte(s.config.Secret))
	h.Write(data)
	return "sha256=" + hex.EncodeToString(h.Sum(nil))
}
