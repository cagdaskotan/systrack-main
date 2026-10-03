package handlers

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"html"
	"html/template"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"systrack/internal/notifications"

	"github.com/gin-gonic/gin"
)

type NewTemplate struct {
	ID        int      `json:"id"`
	Name      string   `json:"name"`
	Type      string   `json:"type"`
	Channel   string   `json:"channel"`
	Subject   *string  `json:"subject"`
	Body      string   `json:"body"`
	Variables []string `json:"variables"`
	IsDefault bool     `json:"is_default"`
	IsActive  bool     `json:"is_active"`
	UpdatedAt *string  `json:"updated_at,omitempty"`
	CreatedAt *string  `json:"created_at,omitempty"`
}

type NewRule struct {
	ID                      int      `json:"id"`
	Name                    string   `json:"name"`
	TargetID                *int     `json:"target_id"`
	TargetIDs               []int    `json:"target_ids,omitempty"`
	TargetName              string   `json:"target_name,omitempty"`
	EntityType              string   `json:"entity_type"`
	Channel                 string   `json:"channel"`
	ScheduleIntervalMinutes int      `json:"schedule_interval_minutes"`
	Recipients              []string `json:"recipients"`
	TemplateID              *int     `json:"template_id"`
	Conditions              any      `json:"conditions"`
	LastSentAt              *string  `json:"last_sent_at,omitempty"`
	IsActive                bool     `json:"is_active"`
	SensorSerial            *string  `json:"sensor_serial,omitempty"`
	InventoryID             *int     `json:"inventory_id,omitempty"`
}

const maxNotificationRuleTargets = 50

func validateRuleTargetLimit(targetIDs []int) error {
	if len(targetIDs) > maxNotificationRuleTargets {
		return fmt.Errorf("en fazla %d hedef seçebilirsiniz", maxNotificationRuleTargets)
	}
	return nil
}

type TemplatePlaceholder struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Description string `json:"description"`
	Example     string `json:"example"`
}

var notificationTemplatePlaceholders = []TemplatePlaceholder{
	// Temel Bilgiler (Önerilen)
	{Key: "hedef_adi", Label: "Hedef Adı", Description: "İzlenen hedefin adı", Example: "Ana Ofis Router"},
	{Key: "hedef_adresi", Label: "Hedef Adresi", Description: "IP adresi veya domain", Example: "192.168.1.254"},
	{Key: "durum", Label: "Durum", Description: "Hedefin anlık durum açıklaması", Example: "Cihaz offline (Hatalı ping)"},
	{Key: "izleme_tipi", Label: "İzleme Tipi", Description: "Kullanılan izleme yöntemi", Example: "ICMP Ping"},
	{Key: "tarih_saat", Label: "Tarih & Saat", Description: "Bildirimin gönderilme zamanı", Example: "27.11.2025 14:32:10"},

	// Kural & Alıcı Bilgileri
	{Key: "kural_adi", Label: "Kural Adı", Description: "Bildirim kuralının adı", Example: "Kritik Cihaz İzleme"},
	{Key: "alicilar", Label: "Alıcılar", Description: "Bildirim alan kişiler", Example: "admin@firma.com, destek@firma.com"},

	// Yanıt Süresi Koşulları (Opsiyonel - sadece RT koşulunda)
	{Key: "yanit_suresi", Label: "Yanıt Süresi", Description: "Ölçülen yanıt süresi (varsa)", Example: "245 ms"},
	{Key: "kosul", Label: "Koşul Açıklaması", Description: "Tetiklenen koşulun açıklaması", Example: "Yanıt süresi > 200 ms"},

	// Sistem Metrikleri (CPU/RAM/Disk/S?cakl?k)
	{Key: "metrik_turu", Label: "Metrik Türü", Description: "Tetiklenen metrik türü", Example: "CPU (%)"},
	{Key: "metrik_degeri", Label: "Metrik Değeri", Description: "Ölçülen metrik değeri", Example: "82.5 %"},
	{Key: "metrik_esik", Label: "Metrik Eşik", Description: "Koşul eşik değeri", Example: "80 %"},
	{Key: "metrik_birim", Label: "Metrik Birimi", Description: "Metrik birimi", Example: "%"},
	{Key: "metrik_kosul", Label: "Metrik Koşulu", Description: "Tetiklenen koşul metni", Example: "CPU (%) > 80 %"},
	{Key: "cpu_yuzde", Label: "CPU (%)", Description: "Anlık CPU kullanımı", Example: "72.3"},
	{Key: "ram_gb", Label: "RAM (GB)", Description: "Anlık RAM kullanımı", Example: "6.8"},
	{Key: "disk_gb", Label: "Disk (GB)", Description: "Anlık disk kullanımı", Example: "120.4"},
	{Key: "sicaklik_c", Label: "Sıcaklık (°C)", Description: "Anlık sıcaklık değeri", Example: "54.2"},

	// Servis Bilgileri (Servis bildirimlerinde)
	{Key: "servis_adi", Label: "Servis Adı", Description: "Servisin görünen adı", Example: "nginx"},
	{Key: "servis_kodu", Label: "Servis Kodu", Description: "Servisin sistem adı", Example: "nginx"},
	{Key: "servis_onceki_durum", Label: "Önceki Durum", Description: "Servisin önceki durumu", Example: "running"},
	{Key: "servis_yeni_durum", Label: "Yeni Durum", Description: "Servisin yeni durumu", Example: "stopped"},
	{Key: "servis_degisim_zamani", Label: "Değişim Zamanı", Description: "Servis durum değişim zamanı", Example: "27.11.2025 14:32:10"},
	{Key: "servis_listesi", Label: "Servis Listesi", Description: "Değişen servislerin listesi", Example: "- nginx (running -> stopped)"},
	{Key: "servis_sayisi", Label: "Servis Sayısı", Description: "Değişen servis sayısı", Example: "2"},
}

func defaultTemplateVariableKeys() []string {
	keys := make([]string, 0, len(notificationTemplatePlaceholders))
	for _, ph := range notificationTemplatePlaceholders {
		keys = append(keys, ph.Key)
	}
	return keys
}

func normalizeConditionsWithTargets(conditions any, targetIDs []int) any {
	if len(targetIDs) == 0 {
		return conditions
	}
	entry := map[string]interface{}{
		"field":    "target_ids",
		"operator": "in",
		"value":    targetIDs,
	}

	var slice []interface{}
	switch v := conditions.(type) {
	case nil:
		slice = []interface{}{}
	case []interface{}:
		slice = v
	default:
		slice = []interface{}{v}
	}
	slice = append(slice, entry)
	return slice
}

func extractTargetIDsFromRawConditions(raw string) (string, []int) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" {
		return "", nil
	}

	var arr []map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &arr); err != nil {
		return raw, nil
	}

	targetIDs := make([]int, 0)
	filtered := make([]map[string]interface{}, 0, len(arr))
	for _, cond := range arr {
		field, _ := cond["field"].(string)
		if strings.EqualFold(field, "target_ids") {
			targetIDs = append(targetIDs, parseIntSliceFromInterface(cond["value"])...)
			continue
		}
		filtered = append(filtered, cond)
	}

	clean := ""
	if len(filtered) > 0 {
		if buf, err := json.Marshal(filtered); err == nil {
			clean = string(buf)
		}
	}

	return clean, targetIDs
}

func parseIntSliceFromInterface(val interface{}) []int {
	result := make([]int, 0)
	switch v := val.(type) {
	case []interface{}:
		for _, item := range v {
			switch t := item.(type) {
			case float64:
				result = append(result, int(t))
			case int:
				result = append(result, t)
			case json.Number:
				if i, err := t.Int64(); err == nil {
					result = append(result, int(i))
				}
			default:
				if s, ok := t.(string); ok {
					if i, err := strconv.Atoi(s); err == nil {
						result = append(result, i)
					}
				}
			}
		}
	case []int:
		result = append(result, v...)
	case []float64:
		for _, item := range v {
			result = append(result, int(item))
		}
	case string:
		if strings.TrimSpace(v) == "" {
			break
		}
		var arr []int
		if err := json.Unmarshal([]byte(v), &arr); err == nil {
			result = append(result, arr...)
		}
	}
	return result
}

type NotificationMailLog struct {
	ID         int      `json:"id"`
	RuleID     *int     `json:"rule_id,omitempty"`
	RuleName   *string  `json:"rule_name,omitempty"`
	TargetID   *int     `json:"target_id,omitempty"`
	TargetName string   `json:"target_name"`
	Channel    string   `json:"channel"`
	Recipients []string `json:"recipients"`
	Subject    string   `json:"subject"`
	Body       string   `json:"body"`
	Status     string   `json:"status"`
	Error      *string  `json:"error,omitempty"`
	SentAt     string   `json:"sent_at"`
}

type NotificationMailSummary struct {
	Total       int     `json:"total"`
	Sent        int     `json:"sent"`
	Failed      int     `json:"failed"`
	Email       int     `json:"email"`
	Telegram    int     `json:"telegram"`
	Last24hSent int     `json:"last_24h_sent,omitempty"`
	LastSentAt  *string `json:"last_sent_at,omitempty"`
	From        *string `json:"from,omitempty"`
	To          *string `json:"to,omitempty"`
	Channel     string  `json:"channel,omitempty"`
	Status      string  `json:"status,omitempty"`
	SearchQuery string  `json:"search,omitempty"`
	RuleID      *int    `json:"rule_id,omitempty"`
	TargetID    *int    `json:"target_id,omitempty"`
}

// NewNotificationsAPI provides CRUD for new_ tables
type NewNotificationsAPI struct {
	db *sql.DB
}

func NewNewNotificationsAPI(db *sql.DB) *NewNotificationsAPI {
	return &NewNotificationsAPI{db: db}
}

// Settings payloads
type NewSettings struct {
	EmailEnabled    bool     `json:"email_enabled"`
	EmailRecipient  string   `json:"email_recipient"`
	TelegramEnabled bool     `json:"telegram_enabled"`
	TelegramChatID  string   `json:"telegram_chat_id"`
	Types           []string `json:"types"`
}

// Get per-user settings from new_notification_settings
func (h *NewNotificationsAPI) GetSettings(c *gin.Context) {
	userID := c.GetInt("user_id")
	var s NewSettings
	var typesJSON sql.NullString
	err := h.db.QueryRow(`
        SELECT email_enabled, email_recipient, telegram_enabled, telegram_chat_id, types
        FROM new_notification_settings WHERE user_id = ?
    `, userID).Scan(&s.EmailEnabled, &s.EmailRecipient, &s.TelegramEnabled, &s.TelegramChatID, &typesJSON)
	if err != nil {
		// If no rows, return defaults
		if err == sql.ErrNoRows {
			c.JSON(http.StatusOK, gin.H{"success": true, "data": s})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	if typesJSON.Valid {
		_ = json.Unmarshal([]byte(typesJSON.String), &s.Types)
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": s})
}

// Upsert per-user settings
func (h *NewNotificationsAPI) UpdateSettings(c *gin.Context) {
	userID := c.GetInt("user_id")
	var in struct {
		Channel   string   `json:"channel"` // 'email' or 'telegram'
		IsEnabled bool     `json:"is_enabled"`
		Recipient string   `json:"recipient"`
		Types     []string `json:"types"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid payload"})
		return
	}
	typesJSON, _ := json.Marshal(in.Types)

	// Build upsert according to channel
	if in.Channel == "email" {
		_, err := h.db.Exec(`
            INSERT INTO new_notification_settings
                (user_id, email_enabled, email_recipient, types)
            VALUES (?, ?, ?, ?)
            ON DUPLICATE KEY UPDATE
                email_enabled = VALUES(email_enabled),
                email_recipient = VALUES(email_recipient),
                types = VALUES(types),
                updated_at = CURRENT_TIMESTAMP
        `, userID, in.IsEnabled, in.Recipient, string(typesJSON))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
			return
		}
	} else if in.Channel == "telegram" {
		_, err := h.db.Exec(`
            INSERT INTO new_notification_settings
                (user_id, telegram_enabled, telegram_chat_id, types)
            VALUES (?, ?, ?, ?)
            ON DUPLICATE KEY UPDATE
                telegram_enabled = VALUES(telegram_enabled),
                telegram_chat_id = VALUES(telegram_chat_id),
                types = VALUES(types),
                updated_at = CURRENT_TIMESTAMP
        `, userID, in.IsEnabled, in.Recipient, string(typesJSON))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
			return
		}
	} else {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Unsupported channel"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

// Config payloads
type NewChannelConfig struct {
	Channel   string                 `json:"channel"`
	IsEnabled bool                   `json:"is_enabled"`
	Config    map[string]interface{} `json:"config"`
}

// Get channel config from new_notification_config (by query: channel=email|telegram)
func (h *NewNotificationsAPI) GetChannelConfig(c *gin.Context) {
	channel := c.DefaultQuery("channel", "email")
	var isEnabled bool
	var cfgJSON sql.NullString
	err := h.db.QueryRow(`SELECT is_enabled, config FROM new_notification_config WHERE channel = ?`, channel).Scan(&isEnabled, &cfgJSON)
	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"channel": channel, "is_enabled": false, "config": gin.H{}}})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	var cfg map[string]interface{}
	if cfgJSON.Valid {
		_ = json.Unmarshal([]byte(cfgJSON.String), &cfg)
	}
	hasPassword := false
	// Never return secrets to the browser. The UI can use has_password to show a placeholder.
	if strings.EqualFold(channel, "email") {
		if v, ok := cfg["password"]; ok {
			hasPassword = strings.TrimSpace(stringOr(v)) != ""
			delete(cfg, "password")
		}
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"channel": channel, "is_enabled": isEnabled, "has_password": hasPassword, "config": cfg}})
}

// Upsert channel config
func (h *NewNotificationsAPI) UpsertChannelConfig(c *gin.Context) {
	var in NewChannelConfig
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid payload"})
		return
	}
	if in.Channel == "" {
		in.Channel = "email"
	}

	// Merge with existing config to preserve secrets when frontend sends empty/masked values.
	existingCfg := map[string]interface{}{}
	var existingJSON sql.NullString
	if err := h.db.QueryRow(`SELECT config FROM new_notification_config WHERE channel = ?`, in.Channel).Scan(&existingJSON); err == nil {
		if existingJSON.Valid {
			_ = json.Unmarshal([]byte(existingJSON.String), &existingCfg)
		}
	} else if err != sql.ErrNoRows {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	if in.Config == nil {
		in.Config = map[string]interface{}{}
	}

	// Email password: update only when explicitly provided and non-empty.
	if strings.EqualFold(in.Channel, "email") {
		if raw, ok := in.Config["password"]; ok {
			next := strings.TrimSpace(stringOr(raw))
			if next == "" || next == "********" {
				delete(in.Config, "password")
			}
		}
		if raw, ok := in.Config["password"]; ok {
			existingCfg["password"] = strings.TrimSpace(stringOr(raw))
			delete(in.Config, "password")
		}
	}

	// Shallow-merge remaining keys.
	for k, v := range in.Config {
		existingCfg[k] = v
	}

	cfgBytes, _ := json.Marshal(existingCfg)
	_, err := h.db.Exec(`
        INSERT INTO new_notification_config (channel, config, is_enabled)
        VALUES (?, ?, ?)
        ON DUPLICATE KEY UPDATE
            config = VALUES(config),
            is_enabled = VALUES(is_enabled),
            updated_at = CURRENT_TIMESTAMP
    `, in.Channel, string(cfgBytes), in.IsEnabled)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// Test email using new_notification_config
func (h *NewNotificationsAPI) TestEmail(c *gin.Context) {
	var payload struct {
		Recipient string `json:"recipient" binding:"required"`
		Subject   string `json:"subject"`
		Body      string `json:"body"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "recipient required"})
		return
	}

	// Load email config
	var isEnabled bool
	var cfgJSON sql.NullString
	err := h.db.QueryRow(`SELECT is_enabled, config FROM new_notification_config WHERE channel='email'`).Scan(&isEnabled, &cfgJSON)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	if !isEnabled {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "email channel disabled"})
		return
	}
	var cfg map[string]interface{}
	if cfgJSON.Valid {
		_ = json.Unmarshal([]byte(cfgJSON.String), &cfg)
	}
	// Map to notifications.EmailConfig
	emailCfg := notifications.EmailConfig{
		Enabled:   true,
		SMTPHost:  stringOr(cfg["smtp_host"]),
		SMTPPort:  intOr(cfg["smtp_port"], 587),
		Username:  stringOr(cfg["username"]),
		Password:  stringOr(cfg["password"]),
		FromEmail: stringOr(cfg["from_email"]),
		FromName:  stringOr(cfg["from_name"]),
		UseTLS:    boolOr(cfg["use_tls"], true),
	}

	logger := notifications.NewEmailLogger(h.db)
	svc := notifications.NewEmailServiceWithDB(emailCfg, logger, h.db)
	subject := payload.Subject
	if subject == "" {
		subject = "SysTrack — Test Bildirimi"
	}
	body := payload.Body
	if body == "" {
		body = "Bu bir SysTrack test bildirimidir.\nBu e-postayı aldıysanız bildirimleriniz doğru çalışıyor."
	}
	ctx := context.Background()
	if err := svc.SendEmail(ctx, []string{payload.Recipient}, subject, body); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Test email sent"})
}

// TestTelegram girilen chat_id'ye gerçek bir test mesajı gönderir.
// Gerçek bildirimlerin kullandığı bot_token'ı (new_notification_config, channel='telegram')
// kullanır; böylece uçtan uca gerçek bir doğrulama olur. Telegram API'sinin döndürdüğü
// hata (chat not found, bot blocked, bot grupta değil) kullanıcıya aynen iletilir.
func (h *NewNotificationsAPI) TestTelegram(c *gin.Context) {
	var payload struct {
		ChatID  string `json:"chat_id" binding:"required"`
		Message string `json:"message"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "chat_id gerekli"})
		return
	}
	chatID := strings.TrimSpace(payload.ChatID)
	if chatID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "chat_id gerekli"})
		return
	}

	// Telegram kanal config'ini yükle (gerçek bildirimlerle aynı kaynak)
	var isEnabled bool
	var cfgJSON sql.NullString
	err := h.db.QueryRow(`SELECT is_enabled, config FROM new_notification_config WHERE channel='telegram'`).Scan(&isEnabled, &cfgJSON)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Telegram kanalı yapılandırılmamış (bot token yok)"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	var cfg map[string]interface{}
	if cfgJSON.Valid {
		_ = json.Unmarshal([]byte(cfgJSON.String), &cfg)
	}
	botToken := stringOr(cfg["bot_token"])
	if botToken == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Bot token ayarlı değil. Önce Telegram bot token'ını kaydedin."})
		return
	}

	svc := notifications.NewTelegramService(h.db)
	impl, ok := svc.(*notifications.TelegramServiceImpl)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "telegram servisi başlatılamadı"})
		return
	}
	if err := impl.SetConfig(notifications.TelegramConfig{Enabled: true, BotToken: botToken}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	message := payload.Message
	if message == "" {
		message = "✅ <b>SysTrack test bildirimi</b>\nBu mesajı görüyorsanız Telegram bildirimleriniz doğru çalışıyor."
	}

	if err := impl.SendMessage(c.Request.Context(), chatID, message); err != nil {
		// Telegram API hatası (chat not found / bot blocked vb.) aynen iletilir.
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Test mesajı gönderildi"})
}

// helpers to coerce types
func stringOr(v interface{}) string {
	if v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return t
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	default:
		b, _ := json.Marshal(v)
		return string(b)
	}
}
func intOr(v interface{}, def int) int {
	if v == nil {
		return def
	}
	switch t := v.(type) {
	case float64:
		return int(t)
	case string:
		i, _ := strconv.Atoi(t)
		if i == 0 {
			return def
		}
		return i
	default:
		return def
	}
}
func boolOr(v interface{}, def bool) bool {
	if v == nil {
		return def
	}
	switch t := v.(type) {
	case bool:
		return t
	case string:
		b, _ := strconv.ParseBool(t)
		return b
	default:
		return def
	}
}

// List templates
func (h *NewNotificationsAPI) ListTemplates(c *gin.Context) {
	rows, err := h.db.Query(`
        SELECT id, name, type, channel, subject, body, variables, is_default, is_active, created_at, updated_at
        FROM new_notification_templates
        ORDER BY created_at DESC`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	defer rows.Close()

	var out []NewTemplate
	for rows.Next() {
		var t NewTemplate
		var subject sql.NullString
		var variables sql.NullString
		var createdAt, updatedAt time.Time
		if err := rows.Scan(&t.ID, &t.Name, &t.Type, &t.Channel, &subject, &t.Body, &variables, &t.IsDefault, &t.IsActive, &createdAt, &updatedAt); err != nil {
			continue
		}
		if subject.Valid {
			s := subject.String
			t.Subject = &s
		}
		if variables.Valid {
			var parsed []string
			if err := json.Unmarshal([]byte(variables.String), &parsed); err == nil && len(parsed) > 0 {
				t.Variables = parsed
			}
		}
		if len(t.Variables) == 0 {
			t.Variables = defaultTemplateVariableKeys()
		}
		createdStr := createdAt.Format("2006-01-02 15:04:05")
		updatedStr := updatedAt.Format("2006-01-02 15:04:05")
		t.CreatedAt = &createdStr
		t.UpdatedAt = &updatedStr
		out = append(out, t)
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": out})
}

// Create template
func (h *NewNotificationsAPI) CreateTemplate(c *gin.Context) {
	var t NewTemplate
	if err := c.ShouldBindJSON(&t); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid payload"})
		return
	}
	var variables interface{}
	if len(t.Variables) > 0 {
		if buf, err := json.Marshal(t.Variables); err == nil {
			variables = string(buf)
		}
	}

	// Insert
	_, err := h.db.Exec(`
        INSERT INTO new_notification_templates
        (name, type, channel, subject, body, variables, is_default, is_active)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?)
    `, t.Name, t.Type, t.Channel, t.Subject, t.Body, variables, t.IsDefault, t.IsActive)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "message": "Template created"})
}

func (h *NewNotificationsAPI) UpdateTemplate(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid id"})
		return
	}
	var t NewTemplate
	if err := c.ShouldBindJSON(&t); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid payload"})
		return
	}
	var variables interface{}
	if len(t.Variables) > 0 {
		if buf, err := json.Marshal(t.Variables); err == nil {
			variables = string(buf)
		}
	}
	_, err = h.db.Exec(`
        UPDATE new_notification_templates
        SET name = ?, type = ?, channel = ?, subject = ?, body = ?, variables = ?, is_default = ?, is_active = ?
        WHERE id = ?
    `, t.Name, t.Type, t.Channel, t.Subject, t.Body, variables, t.IsDefault, t.IsActive, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Template updated"})
}

func (h *NewNotificationsAPI) DeleteTemplate(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid id"})
		return
	}
	if _, err := h.db.Exec("DELETE FROM new_notification_templates WHERE id = ?", id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Template deleted"})
}

func (h *NewNotificationsAPI) ListTemplatePlaceholders(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"success": true, "data": notificationTemplatePlaceholders})
}

// List rules
func (h *NewNotificationsAPI) ListRules(c *gin.Context) {
	rows, err := h.db.Query(`
        SELECT r.id, r.name, r.target_id, r.entity_type, r.channel, r.schedule_interval_minutes,
               r.recipients, r.template_id, r.conditions, r.is_active, r.last_sent_at, r.created_at, r.updated_at,
               COALESCE(t.name, '') AS target_name,
               r.sensor_serial, r.inventory_id
        FROM new_notification_rules r
        LEFT JOIN targets t ON t.id = r.target_id
        ORDER BY r.created_at DESC`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	defer rows.Close()

	var out []NewRule
	for rows.Next() {
		var r NewRule
		var recipientsJSON, conditionsJSON sql.NullString
		var tplID sql.NullInt64
		var tgtID sql.NullInt64
		var lastSent sql.NullTime
		var createdAt, updatedAt time.Time
		var targetName sql.NullString
		var sensorSerial sql.NullString
		var inventoryID sql.NullInt64
		if err := rows.Scan(&r.ID, &r.Name, &tgtID, &r.EntityType, &r.Channel, &r.ScheduleIntervalMinutes, &recipientsJSON, &tplID, &conditionsJSON, &r.IsActive, &lastSent, &createdAt, &updatedAt, &targetName, &sensorSerial, &inventoryID); err != nil {
			continue
		}
		if tgtID.Valid {
			tid := int(tgtID.Int64)
			r.TargetID = &tid
		}
		if recipientsJSON.Valid {
			_ = json.Unmarshal([]byte(recipientsJSON.String), &r.Recipients)
		}
		if targetName.Valid {
			r.TargetName = targetName.String
		}
		if sensorSerial.Valid && sensorSerial.String != "" {
			r.SensorSerial = &sensorSerial.String
		}
		if inventoryID.Valid {
			id := int(inventoryID.Int64)
			r.InventoryID = &id
		}
		if tplID.Valid {
			id := int(tplID.Int64)
			r.TemplateID = &id
		}
		if conditionsJSON.Valid {
			clean, ids := extractTargetIDsFromRawConditions(conditionsJSON.String)
			if len(ids) > 0 {
				r.TargetIDs = ids
			}
			if clean != "" {
				var anyMap any
				if err := json.Unmarshal([]byte(clean), &anyMap); err == nil {
					r.Conditions = anyMap
				}
			}
		}
		if len(r.TargetIDs) > 1 {
			r.TargetName = fmt.Sprintf("%d hedef", len(r.TargetIDs))
		}
		if r.Conditions == nil {
			r.Conditions = map[string]any{}
		}
		if lastSent.Valid {
			formatted := lastSent.Time.Format("2006-01-02 15:04:05")
			r.LastSentAt = &formatted
		}
		if m, ok := r.Conditions.(map[string]any); ok {
			if lastSent.Valid {
				m["last_sent_at"] = lastSent.Time
			}
			m["created_at"] = createdAt
			m["updated_at"] = updatedAt
		}
		out = append(out, r)
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": out})
}

// Update rule
func (h *NewNotificationsAPI) UpdateRule(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid id"})
		return
	}
	var r NewRule
	if err := c.ShouldBindJSON(&r); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid payload"})
		return
	}
	if err := validateRuleTargetLimit(r.TargetIDs); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}
	recJSON, _ := json.Marshal(r.Recipients)
	condPayload := normalizeConditionsWithTargets(r.Conditions, r.TargetIDs)
	condJSON, _ := json.Marshal(condPayload)

	if len(r.TargetIDs) == 1 {
		single := r.TargetIDs[0]
		r.TargetID = &single
	} else if len(r.TargetIDs) > 1 {
		r.TargetID = nil
	}
	_, err = h.db.Exec(`
        UPDATE new_notification_rules SET
            name = ?, target_id = ?, entity_type = ?, channel = ?,
            schedule_interval_minutes = ?, recipients = ?, template_id = ?,
            conditions = ?, is_active = ?, sensor_serial = ?, inventory_id = ?
        WHERE id = ?
    `, r.Name, r.TargetID, r.EntityType, r.Channel, r.ScheduleIntervalMinutes, string(recJSON), r.TemplateID, string(condJSON), r.IsActive, r.SensorSerial, r.InventoryID, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	notifications.TriggerSimpleRule(id)
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// Delete rule
func (h *NewNotificationsAPI) DeleteRule(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid id"})
		return
	}
	_, err = h.db.Exec("DELETE FROM new_notification_rules WHERE id = ?", id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// Create rule
func (h *NewNotificationsAPI) CreateRule(c *gin.Context) {
	userID := c.GetInt("user_id")
	var r NewRule
	if err := c.ShouldBindJSON(&r); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid payload"})
		return
	}
	if err := validateRuleTargetLimit(r.TargetIDs); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}
	// Marshal recipients and conditions
	recJSON, _ := json.Marshal(r.Recipients)
	log.Printf("🔍 CreateRule - Original conditions from frontend: %+v", r.Conditions)
	log.Printf("🔍 CreateRule - TargetIDs: %+v", r.TargetIDs)
	condPayload := normalizeConditionsWithTargets(r.Conditions, r.TargetIDs)
	condJSON, _ := json.Marshal(condPayload)
	log.Printf("🔍 CreateRule - Final condJSON to be saved: %s", string(condJSON))

	if len(r.TargetIDs) == 1 {
		single := r.TargetIDs[0]
		r.TargetID = &single
	} else if len(r.TargetIDs) > 1 {
		r.TargetID = nil
	}

	res, err := h.db.Exec(`
        INSERT INTO new_notification_rules
        (name, target_id, entity_type, channel, schedule_interval_minutes, recipients, template_id, conditions, is_active, created_by, last_sent_at, sensor_serial, inventory_id)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULL, ?, ?)
    `, r.Name, r.TargetID, r.EntityType, r.Channel, r.ScheduleIntervalMinutes, string(recJSON), r.TemplateID, string(condJSON), r.IsActive, userID, r.SensorSerial, r.InventoryID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	if insertID, err := res.LastInsertId(); err == nil {
		notifications.TriggerSimpleRule(int(insertID))
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "message": "Rule created"})
}

// ListMailLogs returns paginated notification delivery logs.
func (h *NewNotificationsAPI) ListMailLogs(c *gin.Context) {
	limit := 50
	if limitStr := c.DefaultQuery("limit", "50"); limitStr != "" {
		if v, err := strconv.Atoi(limitStr); err == nil && v > 0 {
			if v > 200 {
				v = 200
			}
			limit = v
		}
	}

	offset := 0
	if offsetStr := c.DefaultQuery("offset", "0"); offsetStr != "" {
		if v, err := strconv.Atoi(offsetStr); err == nil && v >= 0 {
			offset = v
		}
	}

	channel := strings.TrimSpace(c.Query("channel"))
	status := strings.TrimSpace(c.Query("status"))
	search := strings.TrimSpace(c.Query("search"))

	var targetID *int
	if targetIDStr := strings.TrimSpace(c.Query("target_id")); targetIDStr != "" {
		if v, err := strconv.Atoi(targetIDStr); err == nil {
			targetID = &v
		} else {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid target_id"})
			return
		}
	}

	var ruleID *int
	if ruleIDStr := strings.TrimSpace(c.Query("rule_id")); ruleIDStr != "" {
		if v, err := strconv.Atoi(ruleIDStr); err == nil {
			ruleID = &v
		} else {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid rule_id"})
			return
		}
	}

	startStr := strings.TrimSpace(c.Query("start"))
	endStr := strings.TrimSpace(c.Query("end"))

	startTime, err := parseFlexibleTime(startStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid start date"})
		return
	}

	var endTime *time.Time
	if endParsed, err := parseFlexibleTime(endStr); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid end date"})
		return
	} else if endParsed != nil {
		if len(endStr) == len("2006-01-02") {
			adjusted := endParsed.Add(24*time.Hour - time.Nanosecond)
			endTime = &adjusted
		} else {
			endTime = endParsed
		}
	}

	if startTime != nil && endTime != nil && endTime.Before(*startTime) {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "end date must be after start date"})
		return
	}

	whereParts := []string{"1=1"}
	args := make([]interface{}, 0, 8)

	if channel != "" {
		whereParts = append(whereParts, "m.channel = ?")
		args = append(args, channel)
	}
	if status != "" {
		whereParts = append(whereParts, "m.status = ?")
		args = append(args, status)
	}
	if targetID != nil {
		whereParts = append(whereParts, "m.target_id = ?")
		args = append(args, *targetID)
	}
	if ruleID != nil {
		whereParts = append(whereParts, "m.rule_id = ?")
		args = append(args, *ruleID)
	}
	if startTime != nil {
		whereParts = append(whereParts, "m.sent_at >= ?")
		args = append(args, startTime.Format("2006-01-02 15:04:05"))
	}
	if endTime != nil {
		whereParts = append(whereParts, "m.sent_at <= ?")
		args = append(args, endTime.Format("2006-01-02 15:04:05"))
	}
	if search != "" {
		like := "%" + search + "%"
		whereParts = append(whereParts, "(m.subject LIKE ? OR m.body LIKE ? OR IFNULL(t.name, '') LIKE ? OR IFNULL(r.name, '') LIKE ?)")
		args = append(args, like, like, like, like)
	}

	whereClause := strings.Join(whereParts, " AND ")
	baseFrom := `
        FROM new_notification_mails m
        LEFT JOIN targets t ON t.id = m.target_id
        LEFT JOIN new_notification_rules r ON r.id = m.rule_id
    `

	dataQuery := `
        SELECT m.id, m.rule_id, r.name, m.target_id, t.name, m.channel,
               m.recipients, m.subject, m.body, m.status, m.error, m.sent_at
    ` + baseFrom + `
        WHERE ` + whereClause + `
        ORDER BY m.sent_at DESC
        LIMIT ? OFFSET ?
    `

	dataArgs := append([]interface{}{}, args...)
	dataArgs = append(dataArgs, limit, offset)

	rows, err := h.db.Query(dataQuery, dataArgs...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	defer rows.Close()

	logs := make([]NotificationMailLog, 0, limit)
	for rows.Next() {
		var (
			id             int
			ruleIDVal      sql.NullInt64
			ruleName       sql.NullString
			targetIDVal    sql.NullInt64
			targetName     sql.NullString
			channelVal     string
			recipientsJSON string
			subject        string
			body           string
			statusVal      string
			errorText      sql.NullString
			sentAt         time.Time
		)
		if err := rows.Scan(
			&id,
			&ruleIDVal,
			&ruleName,
			&targetIDVal,
			&targetName,
			&channelVal,
			&recipientsJSON,
			&subject,
			&body,
			&statusVal,
			&errorText,
			&sentAt,
		); err != nil {
			continue
		}

		logEntry := NotificationMailLog{
			ID:         id,
			TargetName: strings.TrimSpace(targetName.String),
			Channel:    channelVal,
			Subject:    subject,
			Body:       body,
			Status:     statusVal,
			SentAt:     sentAt.Format(time.RFC3339),
		}

		if ruleIDVal.Valid {
			rid := int(ruleIDVal.Int64)
			logEntry.RuleID = &rid
			if ruleName.Valid && ruleName.String != "" {
				rule := ruleName.String
				logEntry.RuleName = &rule
			}
		}
		if targetIDVal.Valid {
			tid := int(targetIDVal.Int64)
			logEntry.TargetID = &tid
		}
		if recipientsJSON != "" {
			var recipients []string
			if err := json.Unmarshal([]byte(recipientsJSON), &recipients); err == nil {
				logEntry.Recipients = recipients
			}
		}
		if errorText.Valid && strings.TrimSpace(errorText.String) != "" {
			errMsg := strings.TrimSpace(errorText.String)
			logEntry.Error = &errMsg
		}
		if logEntry.TargetName == "" {
			logEntry.TargetName = "-"
		}

		logs = append(logs, logEntry)
	}

	if err := rows.Err(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	summaryQuery := `
        SELECT 
            COUNT(*) AS total,
            SUM(CASE WHEN m.status = 'sent' THEN 1 ELSE 0 END) AS sent_count,
            SUM(CASE WHEN m.status = 'failed' THEN 1 ELSE 0 END) AS failed_count,
            SUM(CASE WHEN m.channel = 'email' THEN 1 ELSE 0 END) AS email_count,
            SUM(CASE WHEN m.channel = 'telegram' THEN 1 ELSE 0 END) AS telegram_count,
            SUM(CASE WHEN m.status = 'sent' AND m.sent_at >= DATE_SUB(NOW(), INTERVAL 24 HOUR) THEN 1 ELSE 0 END) AS sent_last_24h,
            MAX(m.sent_at) AS last_sent_at
    ` + baseFrom + `
        WHERE ` + whereClause + `
    `

	var (
		totalRows    sql.NullInt64
		sentRows     sql.NullInt64
		failedRows   sql.NullInt64
		emailRows    sql.NullInt64
		telegramRows sql.NullInt64
		last24Rows   sql.NullInt64
		lastSent     sql.NullTime
	)

	if err := h.db.QueryRow(summaryQuery, args...).Scan(
		&totalRows,
		&sentRows,
		&failedRows,
		&emailRows,
		&telegramRows,
		&last24Rows,
		&lastSent,
	); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	summary := NotificationMailSummary{
		Total:       int(totalRows.Int64),
		Sent:        int(sentRows.Int64),
		Failed:      int(failedRows.Int64),
		Email:       int(emailRows.Int64),
		Telegram:    int(telegramRows.Int64),
		Last24hSent: int(last24Rows.Int64),
		Channel:     channel,
		Status:      status,
		SearchQuery: search,
	}

	if startTime != nil {
		formatted := startTime.Format(time.RFC3339)
		summary.From = &formatted
	}
	if endTime != nil {
		formatted := endTime.Format(time.RFC3339)
		summary.To = &formatted
	}
	if ruleID != nil {
		summary.RuleID = ruleID
	}
	if targetID != nil {
		summary.TargetID = targetID
	}
	if lastSent.Valid {
		formatted := lastSent.Time.Format(time.RFC3339)
		summary.LastSentAt = &formatted
	}

	total := summary.Total
	hasMore := offset+limit < total

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"logs":    logs,
			"summary": summary,
			"pagination": gin.H{
				"total":    total,
				"limit":    limit,
				"offset":   offset,
				"has_more": hasMore,
			},
		},
	})
}

func parseFlexibleTime(value string) (*time.Time, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	layouts := []string{
		time.RFC3339,
		"2006-01-02 15:04:05",
		"2006-01-02",
	}
	for _, layout := range layouts {
		if parsed, err := time.ParseInLocation(layout, value, time.Local); err == nil {
			return &parsed, nil
		}
	}
	return nil, fmt.Errorf("invalid time format")
}

// SendNotification sends a notification (with bulk support)
// POST /api/new-notifications/send
type sendNotificationRequest struct {
	Type       string                                 `json:"type" binding:"required"`
	Channel    string                                 `json:"channel" binding:"required"`
	Priority   string                                 `json:"priority"`
	Title      string                                 `json:"title" binding:"required"`
	Message    string                                 `json:"message" binding:"required"`
	Recipients []string                               `json:"recipients" binding:"required"`
	TargetID   *int                                   `json:"target_id"`
	TargetIDs  []int                                  `json:"target_ids"`
	Targets    []notifications.TargetNotificationData `json:"targets"`
}

func (api *NewNotificationsAPI) SendNotification(c *gin.Context) {
	var req sendNotificationRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Geçersiz bildirim verisi: " + err.Error(),
		})
		return
	}

	recipientsPayload := req.Recipients
	if len(req.TargetIDs) > 0 || len(req.Targets) > 0 {
		isBulk := false
		if len(req.Recipients) == 1 {
			var bulk notifications.BulkNotificationRecipients
			if err := json.Unmarshal([]byte(req.Recipients[0]), &bulk); err == nil && len(bulk.Targets) > 0 {
				isBulk = true
			}
		}

		if !isBulk {
			targets, err := api.buildBulkTargets(req.Targets, req.TargetIDs)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{
					"success": false,
					"error":   "Hedef bilgileri yüklenemedi: " + err.Error(),
				})
				return
			}

			if len(targets) > 0 {
				bulk := notifications.BulkNotificationRecipients{
					Emails:  req.Recipients,
					Targets: targets,
				}
				bulkJSON, err := json.Marshal(bulk)
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{
						"success": false,
						"error":   "Bulk bildirim verisi oluşturulamadı: " + err.Error(),
					})
					return
				}
				recipientsPayload = []string{string(bulkJSON)}
			}
		}
	}

	priority := req.Priority
	if priority == "" {
		priority = "medium"
	}
	req.Priority = priority

	status, err := api.dispatchNewNotification(c.Request.Context(), &req, recipientsPayload)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Bildirim gönderilemedi: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Bildirim başarıyla gönderildi",
		"data": gin.H{
			"type":            req.Type,
			"channel":         req.Channel,
			"title":           req.Title,
			"recipient_count": len(req.Recipients),
			"status":          status,
		},
	})
}

func (api *NewNotificationsAPI) dispatchNewNotification(ctx context.Context, req *sendNotificationRequest, recipientsPayload []string) (string, error) {
	switch strings.ToLower(req.Channel) {
	case "email":
		return api.sendEmailNotification(ctx, req, recipientsPayload)
	default:
		return "", fmt.Errorf("desteklenmeyen kanal: %s", req.Channel)
	}
}

func (api *NewNotificationsAPI) sendEmailNotification(ctx context.Context, req *sendNotificationRequest, recipientsPayload []string) (string, error) {
	emailCfg, err := api.loadEmailConfig()
	if err != nil {
		return "", err
	}
	if !emailCfg.Enabled {
		return "", fmt.Errorf("email kanalı devre dışı")
	}

	logger := notifications.NewEmailLogger(api.db)
	emailService := notifications.NewEmailServiceWithDB(emailCfg, logger, api.db)

	recipients, bulkTargets := api.extractRecipients(recipientsPayload)
	if len(recipients) == 0 {
		return "", fmt.Errorf("alıcı listesi boş")
	}

	var htmlBody string
	switch {
	case len(bulkTargets) > 1:
		htmlBody, err = api.renderBulkEmailBody(req, bulkTargets)
		if err != nil {
			return "", err
		}
	case len(bulkTargets) == 1:
		htmlBody, err = api.renderStyledEmailBody(req, &bulkTargets[0])
		if err != nil {
			log.Printf("styled email render failed, using fallback: %v", err)
			htmlBody = api.renderPlainEmailBody(req)
		}
	default:
		htmlBody, err = api.renderStyledEmailBody(req, nil)
		if err != nil {
			log.Printf("styled email render failed, using fallback: %v", err)
			htmlBody = api.renderPlainEmailBody(req)
		}
	}

	err = emailService.SendHTMLEmail(ctx, recipients, req.Title, htmlBody)
	status := "sent"
	var errMsg *string
	if err != nil {
		status = "failed"
		msg := err.Error()
		errMsg = &msg
	}

	if logErr := api.logManualNotificationMail(nil, nil, "email", nil, recipients, req.Title, htmlBody, status, errMsg); logErr != nil {
		log.Printf("failed to log manual notification: %v", logErr)
	}

	if err != nil {
		return status, err
	}

	return status, nil
}

func (api *NewNotificationsAPI) loadEmailConfig() (notifications.EmailConfig, error) {
	var isEnabled bool
	var cfgJSON sql.NullString
	err := api.db.QueryRow(`SELECT is_enabled, config FROM new_notification_config WHERE channel='email'`).Scan(&isEnabled, &cfgJSON)
	if err != nil {
		if err == sql.ErrNoRows {
			return notifications.EmailConfig{}, fmt.Errorf("email kanalı yapılandırılmamış")
		}
		return notifications.EmailConfig{}, err
	}

	var cfg map[string]interface{}
	if cfgJSON.Valid {
		_ = json.Unmarshal([]byte(cfgJSON.String), &cfg)
	}

	return notifications.EmailConfig{
		Enabled:   isEnabled && boolOr(cfg["enabled"], true),
		SMTPHost:  stringOr(cfg["smtp_host"]),
		SMTPPort:  intOr(cfg["smtp_port"], 587),
		Username:  stringOr(cfg["username"]),
		Password:  stringOr(cfg["password"]),
		FromEmail: stringOr(cfg["from_email"]),
		FromName:  stringOr(cfg["from_name"]),
		UseTLS:    boolOr(cfg["use_tls"], true),
	}, nil
}

func (api *NewNotificationsAPI) renderPlainEmailBody(req *sendNotificationRequest) string {
	escapedTitle := html.EscapeString(req.Title)
	escapedMessage := html.EscapeString(req.Message)
	escapedMessage = strings.ReplaceAll(escapedMessage, "\n", "<br>")
	timestamp := time.Now().Format("02.01.2006 15:04:05")

	return fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <title>%s</title>
    <style>
        body { font-family: 'Segoe UI', Tahoma, Geneva, Verdana, sans-serif; background-color: #f5f5f5; color: #333; margin: 0; padding: 0; }
        .container { max-width: 600px; margin: 20px auto; background: white; border-radius: 12px; box-shadow: 0 4px 6px rgba(0,0,0,0.1); overflow: hidden; }
        .header { background: linear-gradient(135deg, #2b5876 0%%, #4e4376 100%%); color: white; padding: 24px; }
        .content { padding: 24px; line-height: 1.6; }
        .footer { background: #f8f9fa; padding: 16px; text-align: center; font-size: 12px; color: #6c757d; }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h2>%s</h2>
            <p>%s</p>
        </div>
        <div class="content">
            %s
        </div>
        <div class="footer">
            Gonderim zamani: %s
        </div>
    </div>
</body>
</html>`, escapedTitle, escapedTitle, html.EscapeString(req.Priority), escapedMessage, timestamp)
}

type manualEmailData struct {
	Title         string
	MessageHTML   template.HTML
	Timestamp     string
	Priority      string
	Status        string
	TargetName    string
	TargetDetail  string
	Recipients    []string
	HasRecipients bool
}

var manualEmailTemplate = template.Must(template.New("manual_inline_email").Parse(`
<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <title>{{.Title}}</title>
</head>
<body style="margin:0;padding:32px 12px;background:#0f172a;font-family:'Segoe UI',Arial,sans-serif;color:#0f172a;">
    <table role="presentation" width="100%" cellpadding="0" cellspacing="0">
        <tr>
            <td align="center">
                <table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="max-width:660px;background:#ffffff;border-radius:28px;overflow:hidden;box-shadow:0 30px 60px rgba(15,23,42,0.25);">
                    <tr>
                        <td style="padding:36px 32px;background:linear-gradient(135deg,#1d4ed8,#7c3aed);color:#ffffff;">
                            <div style="text-transform:uppercase;letter-spacing:0.24em;font-size:11px;opacity:0.8;margin-bottom:12px;">SysTrack Bildirimi</div>
                            <div style="font-size:26px;font-weight:600;margin:0 0 8px;">{{.Title}}</div>
                            <div style="opacity:0.9;font-size:15px;line-height:1.6;margin:0;">{{.MessageHTML}}</div>
                            <div style="margin-top:18px;display:inline-block;padding:6px 14px;border:1px solid rgba(255,255,255,0.5);border-radius:999px;font-size:12px;letter-spacing:0.18em;">{{.Timestamp}}</div>
                        </td>
                    </tr>
                    <tr>
                        <td style="padding:32px 32px 12px 32px;background:#ffffff;">
                            <table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="border-collapse:separate;border-spacing:0 12px;">
                                <tr>
                                    <td style="width:33.33%;padding:16px;border:1px solid #e5e7eb;border-radius:18px;background:linear-gradient(180deg,#f8fafc 0%,#ffffff 100%);">
                                        <div style="font-size:12px;text-transform:uppercase;letter-spacing:0.14em;color:#94a3b8;margin-bottom:6px;">Oncelik</div>
                                        <div style="font-size:16px;font-weight:600;color:#0f172a;margin:0;">{{.Priority}}</div>
                                    </td>
                                    <td style="width:33.33%;padding:16px;border:1px solid #e5e7eb;border-radius:18px;background:linear-gradient(180deg,#f8fafc 0%,#ffffff 100%);">
                                        <div style="font-size:12px;text-transform:uppercase;letter-spacing:0.14em;color:#94a3b8;margin-bottom:6px;">Durum</div>
                                        <div style="font-size:16px;font-weight:600;color:#0f172a;margin:0;">{{.Status}}</div>
                                    </td>
                                    <td style="width:33.33%;padding:16px;border:1px solid #e5e7eb;border-radius:18px;background:linear-gradient(180deg,#f8fafc 0%,#ffffff 100%);">
                                        <div style="font-size:12px;text-transform:uppercase;letter-spacing:0.14em;color:#94a3b8;margin-bottom:6px;">Hedef</div>
                                        <div style="font-size:16px;font-weight:600;color:#0f172a;margin:0;">{{.TargetName}}</div>
                                    </td>
                                </tr>
                            </table>
                            <div style="margin-top:18px;font-size:13px;color:#475569;">{{.TargetDetail}}</div>
                        </td>
                    </tr>
                    {{if .HasRecipients}}
                    <tr>
                        <td style="padding:0 32px 32px 32px;background:#ffffff;">
                            <div style="font-size:12px;text-transform:uppercase;letter-spacing:0.2em;color:#94a3b8;margin-bottom:10px;">Alicilar</div>
                            <table role="presentation" cellpadding="0" cellspacing="0" style="width:100%;border-collapse:separate;border-spacing:8px 8px;">
                                <tr>
                                {{range .Recipients}}
                                    <td style="padding:6px 14px;border-radius:999px;background:#eef2ff;color:#3730a3;font-size:12px;font-weight:600;">{{.}}</td>
                                {{end}}
                                </tr>
                            </table>
                        </td>
                    </tr>
                    {{end}}
                    <tr>
                        <td style="padding:24px 32px;background:#f8fafc;text-align:center;font-size:12px;color:#94a3b8;">
                            Bu bildirim SysTrack tarafindan otomatik olarak gonderildi.
                        </td>
                    </tr>
                </table>
            </td>
        </tr>
    </table>
</body>
</html>
`))

func (api *NewNotificationsAPI) renderStyledEmailBody(req *sendNotificationRequest, target *notifications.TargetNotificationData) (string, error) {
	now := time.Now()
	status := deriveTemplateStatus(target, req)

	targetName := "-"
	targetDetail := "Hedef detayi paylasilmadi."
	if target != nil {
		targetName = target.Name
		detailParts := []string{}
		if target.Address != "" {
			detailParts = append(detailParts, fmt.Sprintf("Adres: %s", html.EscapeString(target.Address)))
		}
		if target.Status != "" {
			detailParts = append(detailParts, fmt.Sprintf("Durum: %s", html.EscapeString(target.Status)))
		}
		if target.UptimePercent != nil {
			detailParts = append(detailParts, fmt.Sprintf("Uptime: %.1f%%", *target.UptimePercent))
		}
		if target.ResponseTime != nil {
			detailParts = append(detailParts, fmt.Sprintf("Yanıt: %.0f ms", *target.ResponseTime))
		}
		if len(detailParts) > 0 {
			targetDetail = strings.Join(detailParts, " | ")
		}
	}

	message := html.EscapeString(strings.TrimSpace(req.Message))
	if message == "" {
		message = "Bildirim icerigi paylasilmadi."
	}
	message = strings.ReplaceAll(message, "\n", "<br>")

	data := manualEmailData{
		Title:         html.EscapeString(req.Title),
		MessageHTML:   template.HTML(message),
		Timestamp:     now.Format("02.01.2006 15:04:05"),
		Priority:      string(normalizePriority(req.Priority)),
		Status:        strings.Title(status),
		TargetName:    html.EscapeString(targetName),
		TargetDetail:  targetDetail,
		Recipients:    req.Recipients,
		HasRecipients: len(req.Recipients) > 0,
	}

	var buf bytes.Buffer
	if err := manualEmailTemplate.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func (api *NewNotificationsAPI) renderBulkEmailBody(req *sendNotificationRequest, targets []notifications.TargetNotificationData) (string, error) {
	tm := notifications.NewEmailTemplateManager()
	notificationType := notifications.NotificationType(req.Type)
	if notificationType == "" {
		notificationType = notifications.TargetStatusChange
	}
	templateStr := tm.GetBulkTemplate(notificationType)
	tmpl, err := template.New("bulk_email").Parse(templateStr)
	if err != nil {
		return "", err
	}

	templateData := map[string]interface{}{
		"Title":       req.Title,
		"Message":     req.Message,
		"Type":        req.Type,
		"Priority":    req.Priority,
		"Timestamp":   time.Now().Format("02.01.2006 15:04:05"),
		"TargetCount": len(targets),
		"Targets":     api.prepareTargetsData(targets),
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, templateData); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func (api *NewNotificationsAPI) extractRecipients(payload []string) ([]string, []notifications.TargetNotificationData) {
	recipients := make([]string, 0, len(payload))
	var targets []notifications.TargetNotificationData

	if len(payload) == 1 {
		var bulk notifications.BulkNotificationRecipients
		if err := json.Unmarshal([]byte(payload[0]), &bulk); err == nil && (len(bulk.Emails) > 0 || len(bulk.Targets) > 0) {
			recipients = append(recipients, bulk.Emails...)
			targets = bulk.Targets
		}
	}

	if len(recipients) == 0 {
		recipients = append(recipients, payload...)
	}

	for i := range targets {
		if targets[i].StatusClass == "" {
			targets[i].StatusClass = deriveStatusClass(targets[i].Status)
		}
	}

	return recipients, targets
}

func (api *NewNotificationsAPI) logManualNotificationMail(ruleID, targetID *int, channel string, templateID *int, recipients []string, subject, body, status string, errMsg *string) error {
	recJSON, _ := json.Marshal(recipients)

	var ruleVal interface{}
	if ruleID != nil {
		ruleVal = *ruleID
	}
	var targetVal interface{}
	if targetID != nil {
		targetVal = *targetID
	}
	var tplVal interface{}
	if templateID != nil {
		tplVal = *templateID
	}
	errorText := ""
	if errMsg != nil {
		errorText = *errMsg
	}

	_, err := api.db.Exec(`
        INSERT INTO new_notification_mails (rule_id, target_id, channel, template_id, recipients, subject, body, status, error, sent_at)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, NOW())
    `, ruleVal, targetVal, channel, tplVal, string(recJSON), subject, body, status, errorText)
	return err
}

func (api *NewNotificationsAPI) prepareTargetsData(targets []notifications.TargetNotificationData) []map[string]interface{} {
	result := make([]map[string]interface{}, 0, len(targets))
	for _, target := range targets {
		entry := map[string]interface{}{
			"Name":        target.Name,
			"Address":     target.Address,
			"Status":      target.Status,
			"StatusClass": deriveStatusClass(target.Status),
		}
		if target.UptimePercent != nil {
			entry["UptimePercent"] = *target.UptimePercent
		}
		if target.ResponseTime != nil {
			entry["ResponseTime"] = *target.ResponseTime
		}
		result = append(result, entry)
	}
	return result
}

func (api *NewNotificationsAPI) buildBulkTargets(existing []notifications.TargetNotificationData, ids []int) ([]notifications.TargetNotificationData, error) {
	result := make([]notifications.TargetNotificationData, 0, len(existing)+len(ids))
	seen := make(map[int]struct{})

	for _, t := range existing {
		if t.ID > 0 {
			if _, ok := seen[t.ID]; ok {
				continue
			}
			seen[t.ID] = struct{}{}
		}
		if t.StatusClass == "" {
			t.StatusClass = deriveStatusClass(t.Status)
		}
		result = append(result, t)
	}

	pending := make([]int, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		pending = append(pending, id)
	}

	if len(pending) > 0 {
		fetched, err := api.fetchTargetNotificationData(pending)
		if err != nil {
			return nil, err
		}
		result = append(result, fetched...)
	}

	return result, nil
}

func (api *NewNotificationsAPI) fetchTargetNotificationData(ids []int) ([]notifications.TargetNotificationData, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	placeholders := make([]string, len(ids))
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}

	query := fmt.Sprintf(`
        SELECT 
            t.id, 
            t.name, 
            t.address,
            (
                SELECT ok FROM pings_raw_momentary 
                WHERE target_id = t.id 
                ORDER BY ts_ms DESC 
                LIMIT 1
            ) AS last_ok,
            (
                SELECT rtt_ms FROM pings_raw_momentary 
                WHERE target_id = t.id 
                ORDER BY ts_ms DESC 
                LIMIT 1
            ) AS last_rtt
        FROM targets t
        WHERE t.id IN (%s)
    `, strings.Join(placeholders, ","))

	rows, err := api.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var targets []notifications.TargetNotificationData
	for rows.Next() {
		var (
			id      int
			name    string
			address string
			lastOK  sql.NullInt64
			lastRT  sql.NullFloat64
		)
		if err := rows.Scan(&id, &name, &address, &lastOK, &lastRT); err != nil {
			return nil, err
		}

		status := "Unknown"
		statusClass := "unknown"
		if lastOK.Valid {
			if lastOK.Int64 == 1 {
				status = "Online"
				statusClass = "online"
			} else {
				status = "Offline"
				statusClass = "offline"
			}
		}

		var responsePtr *float64
		if lastRT.Valid {
			val := lastRT.Float64
			responsePtr = &val
		}

		targets = append(targets, notifications.TargetNotificationData{
			ID:           id,
			Name:         name,
			Address:      address,
			Status:       status,
			StatusClass:  statusClass,
			ResponseTime: responsePtr,
		})
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return targets, nil
}

func deriveStatusClass(status string) string {
	switch strings.ToLower(status) {
	case "online", "success", "up", "ok":
		return "online"
	case "offline", "failed", "fail", "down":
		return "offline"
	case "warning", "degraded":
		return "warning"
	default:
		return "unknown"
	}
}

func normalizePriority(priority string) notifications.NotificationPriority {
	switch strings.ToLower(strings.TrimSpace(priority)) {
	case "low":
		return notifications.Low
	case "high":
		return notifications.High
	case "critical":
		return notifications.Critical
	case "":
		return notifications.Medium
	default:
		return notifications.NotificationPriority(strings.ToLower(priority))
	}
}

func deriveTemplateStatus(target *notifications.TargetNotificationData, req *sendNotificationRequest) string {
	if target != nil && target.Status != "" {
		return strings.ToLower(target.Status)
	}
	if strings.TrimSpace(req.Type) == "" {
		return "info"
	}
	return strings.ToLower(req.Type)
}
