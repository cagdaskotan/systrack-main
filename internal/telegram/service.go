package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"text/template"
	"time"
)

// Settings Telegram ayarları için JSON şeması
type Settings struct {
	Enabled   bool              `json:"enabled"`
	BotToken  string            `json:"bot_token"`
	ChatID    string            `json:"chat_id"`
	Templates map[string]string `json:"templates"`
}

// Service Telegram servisi
type Service struct {
	settings      Settings
	templateCache map[string]*template.Template
	configPath    string
	httpClient    *http.Client
}

// NewService yeni Telegram servisi oluşturur
func NewService() *Service {
	return &Service{
		templateCache: make(map[string]*template.Template),
		configPath:    "./config/telegram.json",
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// Load ayarları JSON dosyasından yükler
func (s *Service) Load() error {
	// Config klasörünü oluştur
	if err := os.MkdirAll(filepath.Dir(s.configPath), 0755); err != nil {
		return fmt.Errorf("config klasörü oluşturulamadı: %v", err)
	}

	// Dosya yoksa varsayılan ayarları oluştur
	if _, err := os.Stat(s.configPath); os.IsNotExist(err) {
		s.settings = s.getDefaultSettings()
		return s.Save()
	}

	// Dosyayı oku
	data, err := os.ReadFile(s.configPath)
	if err != nil {
		return fmt.Errorf("ayarlar dosyası okunamadı: %v", err)
	}

	// JSON parse et
	if err := json.Unmarshal(data, &s.settings); err != nil {
		return fmt.Errorf("ayarlar parse edilemedi: %v", err)
	}

	// Şablonları derle
	s.compileTemplates()
	return nil
}

// Save ayarları JSON dosyasına kaydeder
func (s *Service) Save() error {
	// Şablonları derle
	s.compileTemplates()

	// JSON'a çevir
	data, err := json.MarshalIndent(s.settings, "", "  ")
	if err != nil {
		return fmt.Errorf("ayarlar JSON'a çevrilemedi: %v", err)
	}

	// Dosyaya yaz (0600 izin)
	if err := os.WriteFile(s.configPath, data, 0600); err != nil {
		return fmt.Errorf("ayarlar dosyası yazılamadı: %v", err)
	}

	return nil
}

// GetSettings mevcut ayarları döndürür
func (s *Service) GetSettings() Settings {
	return s.settings
}

// UpdateSettings ayarları günceller
func (s *Service) UpdateSettings(settings Settings) error {
	s.settings = settings
	return s.Save()
}

// getDefaultSettings varsayılan ayarları döndürür
func (s *Service) getDefaultSettings() Settings {
	return Settings{
		Enabled:  false,
		BotToken: "",
		ChatID:   "",
		Templates: map[string]string{
			"TEST_MESSAGE":        "✅ *Systrack* test message ok.\nTime: {{.Time}}",
			"ALERT_UP":            "✅ Service *{{.Service}}* is *UP* on *{{.Host}}* ({{.IP}})",
			"ALERT_DOWN":          "❌ Service *{{.Service}}* is *DOWN* on *{{.Host}}* ({{.IP}}). Since: {{.Since}}",
			"NEW_DEVICE_DETECTED": "🆕 New device detected: *{{.Hostname}}* ({{.IP}})",
		},
	}
}

// compileTemplates şablonları derler ve cache'ler
func (s *Service) compileTemplates() {
	s.templateCache = make(map[string]*template.Template)

	for key, templateStr := range s.settings.Templates {
		tmpl, err := template.New(key).Parse(templateStr)
		if err != nil {
			// Hatalı şablon varsa varsayılan metin kullan
			s.templateCache[key] = template.Must(template.New(key).Parse(templateStr))
		} else {
			s.templateCache[key] = tmpl
		}
	}
}

// TestConnection bot bağlantısını test eder
func (s *Service) TestConnection(ctx context.Context) error {
	if s.settings.BotToken == "" {
		return fmt.Errorf("bot token boş")
	}

	url := fmt.Sprintf("https://api.telegram.org/bot%s/getMe", s.settings.BotToken)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return fmt.Errorf("istek oluşturulamadı: %v", err)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("bağlantı hatası: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("telegram API hatası (status: %d): %s", resp.StatusCode, string(body))
	}

	return nil
}

// SendTestMessage test mesajı gönderir
func (s *Service) SendTestMessage(ctx context.Context) error {
	if s.settings.BotToken == "" || s.settings.ChatID == "" {
		return fmt.Errorf("bot token veya chat ID boş")
	}

	payload := map[string]interface{}{
		"Time": time.Now().Format("2006-01-02 15:04:05"),
	}

	return s.SendTemplate(ctx, "TEST_MESSAGE", payload, "Markdown")
}

// SendTemplate şablonlu mesaj gönderir
func (s *Service) SendTemplate(ctx context.Context, key string, payload interface{}, parseMode string) error {
	if s.settings.BotToken == "" || s.settings.ChatID == "" {
		return fmt.Errorf("bot token veya chat ID boş")
	}

	// Şablonu bul
	tmpl, exists := s.templateCache[key]
	if !exists {
		return fmt.Errorf("şablon bulunamadı: %s", key)
	}

	// Şablonu render et
	var rendered bytes.Buffer
	if err := tmpl.Execute(&rendered, payload); err != nil {
		return fmt.Errorf("şablon render edilemedi: %v", err)
	}

	// Telegram API'ye gönder
	return s.sendMessage(ctx, rendered.String(), parseMode)
}

// sendMessage Telegram'a mesaj gönderir
func (s *Service) sendMessage(ctx context.Context, text, parseMode string) error {
	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", s.settings.BotToken)

	payload := map[string]interface{}{
		"chat_id": s.settings.ChatID,
		"text":    text,
	}

	if parseMode != "" {
		payload["parse_mode"] = parseMode
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("payload JSON'a çevrilemedi: %v", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("istek oluşturulamadı: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("bağlantı hatası: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("telegram API hatası (status: %d): %s", resp.StatusCode, string(body))
	}

	return nil
}
