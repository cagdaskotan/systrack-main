package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"systrack/internal/notifications"

	"github.com/gin-gonic/gin"
)

// NotificationHandler bildirim API handler'ı
type NotificationHandler struct {
	notificationService notifications.Service
	db                  *sql.DB
}

// NewNotificationHandler yeni bildirim handler'ı oluştur
func NewNotificationHandler(notificationService notifications.Service, db *sql.DB) *NotificationHandler {
	return &NotificationHandler{
		notificationService: notificationService,
		db:                  db,
	}
}

// GetNotificationSettings kullanıcı bildirim ayarlarını getir
func (h *NotificationHandler) GetNotificationSettings(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	settings, err := h.notificationService.GetNotificationSettings(c.Request.Context(), userID.(int))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get notification settings"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": settings})
}

// UpdateNotificationSettings bildirim ayarlarını güncelle
func (h *NotificationHandler) UpdateNotificationSettings(c *gin.Context) {
	var settings notifications.NotificationSettings
	if err := c.ShouldBindJSON(&settings); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request data"})
		return
	}

	err := h.notificationService.UpdateNotificationSettings(c.Request.Context(), &settings)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update notification settings"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Notification settings updated successfully"})
}

// GetNotificationTemplates şablonları getir
func (h *NotificationHandler) GetNotificationTemplates(c *gin.Context) {
	templates, err := h.notificationService.GetNotificationTemplates(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get notification templates"})
		return
	}

	// Debug: Şablonları logla
	fmt.Printf("[DEBUG] GetNotificationTemplates returned %d templates:\n", len(templates))
	for i, t := range templates {
		fmt.Printf("  [%d] ID=%d Name=%s Channel=%s\n", i, t.ID, t.Name, t.Channel)
	}

	c.JSON(http.StatusOK, gin.H{"data": templates})
}

// UpdateNotificationTemplate şablonu güncelle
func (h *NotificationHandler) UpdateNotificationTemplate(c *gin.Context) {
	var template notifications.NotificationTemplate
	if err := c.ShouldBindJSON(&template); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request data"})
		return
	}

	err := h.notificationService.UpdateNotificationTemplate(c.Request.Context(), &template)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update notification template"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Notification template updated successfully"})
}

// GetNotificationHistory bildirim geçmişini getir
func (h *NotificationHandler) GetNotificationHistory(c *gin.Context) {
	limitStr := c.DefaultQuery("limit", "50")
	offsetStr := c.DefaultQuery("offset", "0")

	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit <= 0 || limit > 100 {
		limit = 50
	}

	offset, err := strconv.Atoi(offsetStr)
	if err != nil || offset < 0 {
		offset = 0
	}

	notifications, err := h.notificationService.GetNotificationHistory(c.Request.Context(), limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get notification history"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": notifications})
}

// SendTestNotification test bildirimi gönder
func (h *NotificationHandler) SendTestNotification(c *gin.Context) {
	var request struct {
		Channel    notifications.NotificationChannel `json:"channel" binding:"required"`
		Recipients []string                          `json:"recipients" binding:"required"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request data"})
		return
	}

	// Test bildirimi oluştur
	notification := &notifications.Notification{
		Type:       notifications.SystemHealth,
		Channel:    request.Channel,
		Priority:   notifications.Medium,
		Title:      "Test Bildirimi",
		Message:    "Bu bir test bildirimidir. SysTrack bildirim sistemi çalışıyor!",
		Recipients: request.Recipients,
		CreatedAt:  time.Now(),
	}

	err := h.notificationService.SendNotification(c.Request.Context(), notification)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to send test notification"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Test notification sent successfully"})
}

// RetryFailedNotifications başarısız bildirimleri tekrar dene
func (h *NotificationHandler) RetryFailedNotifications(c *gin.Context) {
	err := h.notificationService.RetryFailedNotifications(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retry failed notifications"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Failed notifications retry initiated"})
}

// ProcessNotificationQueue kuyruktaki bildirimleri işle
func (h *NotificationHandler) ProcessNotificationQueue(c *gin.Context) {
	err := h.notificationService.ProcessNotificationQueue(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to process notification queue"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Notification queue processed"})
}

// GetNotificationConfig bildirim konfigürasyonunu getir
func (h *NotificationHandler) GetNotificationConfig(c *gin.Context) {
	// Veritabanından konfigürasyonu oku
	config, err := h.loadNotificationConfig()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load configuration", "details": err.Error()})
		return
	}

	// Avoid logging secrets (SMTP password etc.) into service logs.
	c.JSON(http.StatusOK, gin.H{"data": config})
}

// UpdateNotificationConfig bildirim konfigürasyonunu güncelle
func (h *NotificationHandler) UpdateNotificationConfig(c *gin.Context) {
	var request struct {
		Email    notifications.EmailConfig    `json:"email"`
		Telegram notifications.TelegramConfig `json:"telegram"`
		WhatsApp notifications.WhatsAppConfig `json:"whatsapp"`
		Webhook  notifications.WebhookConfig  `json:"webhook"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request data", "details": err.Error()})
		return
	}

	// Konfigürasyonu oluştur
	config := notifications.NotificationConfig{
		Email:    request.Email,
		Telegram: request.Telegram,
		WhatsApp: request.WhatsApp,
		Webhook:  request.Webhook,
	}

	// Konfigürasyonu doğrula
	if err := validateNotificationConfig(&config); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Konfigürasyonu veritabanına kaydet
	if err := h.saveNotificationConfig(&config); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save configuration", "details": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Notification configuration updated successfully"})
}

// TestNotificationChannel kanal bağlantısını test et
func (h *NotificationHandler) TestNotificationChannel(c *gin.Context) {
	var request struct {
		Config notifications.EmailConfig `json:"config" binding:"required"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request data", "details": err.Error()})
		return
	}

	// Debug: Config'i logla
	// Avoid logging secrets (SMTP password etc.) into service logs.

	// Email servisini oluştur ve test et
	emailLogger := notifications.NewEmailLogger(h.db)
	emailService := notifications.NewEmailService(request.Config, emailLogger)
	err := emailService.TestConnection(c.Request.Context())

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Email connection test successful"})
}

// validateNotificationConfig konfigürasyonu doğrula
func validateNotificationConfig(config *notifications.NotificationConfig) error {
	// Email konfigürasyonu doğrula
	if config.Email.Enabled {
		if config.Email.SMTPHost == "" {
			return fmt.Errorf("SMTP host is required when email is enabled")
		}
		if config.Email.Username == "" {
			return fmt.Errorf("SMTP username is required when email is enabled")
		}
		if config.Email.FromEmail == "" {
			return fmt.Errorf("from email is required when email is enabled")
		}
	}

	// Telegram konfigürasyonu doğrula
	if config.Telegram.Enabled {
		if config.Telegram.BotToken == "" {
			return fmt.Errorf("bot token is required when telegram is enabled")
		}
	}

	// WhatsApp konfigürasyonu doğrula
	if config.WhatsApp.Enabled {
		if config.WhatsApp.AccessToken == "" {
			return fmt.Errorf("access token is required when whatsapp is enabled")
		}
		if config.WhatsApp.PhoneID == "" {
			return fmt.Errorf("phone ID is required when whatsapp is enabled")
		}
	}

	// Webhook konfigürasyonu doğrula
	if config.Webhook.Enabled {
		if config.Webhook.URL == "" {
			return fmt.Errorf("webhook URL is required when webhook is enabled")
		}
	}

	return nil
}

// saveNotificationConfig konfigürasyonu veritabanına kaydet
func (h *NotificationHandler) saveNotificationConfig(config *notifications.NotificationConfig) error {
	// JSON olarak serialize et
	configJSON, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// Veritabanına kaydet (INSERT ... ON DUPLICATE KEY UPDATE)
	query := `
		INSERT INTO notification_config (id, email_config, telegram_config, whatsapp_config, webhook_config, created_at, updated_at)
		VALUES (1, ?, ?, ?, ?, NOW(), NOW())
		ON DUPLICATE KEY UPDATE
		email_config = VALUES(email_config),
		telegram_config = VALUES(telegram_config),
		whatsapp_config = VALUES(whatsapp_config),
		webhook_config = VALUES(webhook_config),
		updated_at = NOW()
	`

	_, err = h.db.Exec(query, configJSON, configJSON, configJSON, configJSON)
	if err != nil {
		return fmt.Errorf("failed to save config to database: %w", err)
	}

	return nil
}

// loadNotificationConfig konfigürasyonu veritabanından yükle
func (h *NotificationHandler) loadNotificationConfig() (*notifications.NotificationConfig, error) {
	query := `SELECT email_config FROM notification_config WHERE id = 1`
	var configJSON string

	err := h.db.QueryRow(query).Scan(&configJSON)
	if err != nil {
		if err == sql.ErrNoRows {
			// Konfigürasyon bulunamadı, varsayılan değerleri döndür
			return &notifications.NotificationConfig{
				Email: notifications.EmailConfig{
					Enabled:           false,
					SMTPHost:          "",
					SMTPPort:          587,
					Username:          "",
					Password:          "",
					FromEmail:         "",
					FromName:          "SysTrack",
					UseTLS:            true,
					NotificationEmail: "",
				},
				Telegram: notifications.TelegramConfig{
					Enabled:     false,
					BotToken:    "",
					DefaultChat: "",
					WebhookURL:  "",
				},
				WhatsApp: notifications.WhatsAppConfig{
					Enabled:     false,
					AccessToken: "",
					PhoneID:     "",
					WebhookURL:  "",
				},
				Webhook: notifications.WebhookConfig{
					Enabled: false,
					URL:     "",
					Secret:  "",
				},
			}, nil
		}
		return nil, fmt.Errorf("failed to load config from database: %w", err)
	}

	// JSON'u parse et
	var config notifications.NotificationConfig
	if err := json.Unmarshal([]byte(configJSON), &config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	return &config, nil
}

// TestSLAViolationNotification SLA violation test bildirimi gönder
func (h *NotificationHandler) TestSLAViolationNotification(c *gin.Context) {
	var request struct {
		Recipient string `json:"recipient"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	if request.Recipient == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Recipient is required"})
		return
	}

	// Test SLA violation bildirimi oluştur
	notification := &notifications.Notification{
		Type:       notifications.SLAViolation,
		Channel:    notifications.Email,
		Priority:   notifications.Critical,
		Title:      "⚠️ Test SLA Violation - SysTrack",
		Message:    "Bu test mesajı SLA ihlal bildiriminin işleyişini doğrular.",
		Recipients: []string{request.Recipient},
		Status:     "pending",
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	// İlk target'ı al (test için)
	var targetID int
	err := h.db.QueryRow("SELECT id FROM targets LIMIT 1").Scan(&targetID)
	if err != nil {
		// Target yoksa varsayılan değer kullan
		targetID = 1
	}
	notification.TargetID = &targetID

	// Bildirimi gönder
	ctx := context.Background()
	err = h.notificationService.SendNotification(ctx, notification)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": fmt.Sprintf("Failed to send SLA test notification: %v", err)})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "SLA test notification sent successfully"})
}
