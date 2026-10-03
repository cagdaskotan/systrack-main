package handlers

import (
	"net/http"
	"systrack/internal/telegram"

	"github.com/gin-gonic/gin"
)

// TelegramSettingsHandler Telegram ayarları HTTP handler'ları
type TelegramSettingsHandler struct {
	service *telegram.Service
}

// NewTelegramSettingsHandler yeni Telegram ayarları handler oluşturur
func NewTelegramSettingsHandler() *TelegramSettingsHandler {
	service := telegram.NewService()
	if err := service.Load(); err != nil {
		// Log error but continue
	}

	return &TelegramSettingsHandler{
		service: service,
	}
}

// GetSettings Telegram ayarlarını döndürür
func (h *TelegramSettingsHandler) GetSettings(c *gin.Context) {
	settings := h.service.GetSettings()
	c.JSON(http.StatusOK, settings)
}

// UpdateSettings Telegram ayarlarını günceller
func (h *TelegramSettingsHandler) UpdateSettings(c *gin.Context) {
	var settings telegram.Settings
	if err := c.ShouldBindJSON(&settings); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Geçersiz JSON formatı"})
		return
	}

	if err := h.service.UpdateSettings(settings); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ayarlar kaydedilemedi: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// TestConnection bot bağlantısını test eder
func (h *TelegramSettingsHandler) TestConnection(c *gin.Context) {
	if err := h.service.TestConnection(c.Request.Context()); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Bağlantı testi başarısız: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// SendTestMessage test mesajı gönderir
func (h *TelegramSettingsHandler) SendTestMessage(c *gin.Context) {
	if err := h.service.SendTestMessage(c.Request.Context()); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Test mesajı gönderilemedi: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "sent"})
}

// RenderSendRequest şablonlu mesaj gönderme isteği
type RenderSendRequest struct {
	Key     string      `json:"key"`
	Payload interface{} `json:"payload"`
	Mode    string      `json:"mode"`
}

// RenderSend şablonlu mesaj gönderir
func (h *TelegramSettingsHandler) RenderSend(c *gin.Context) {
	var req RenderSendRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Geçersiz JSON formatı"})
		return
	}

	// Parse mode varsayılanı
	if req.Mode == "" {
		req.Mode = "Markdown"
	}

	if err := h.service.SendTemplate(c.Request.Context(), req.Key, req.Payload, req.Mode); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Mesaj gönderilemedi: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "sent"})
}
