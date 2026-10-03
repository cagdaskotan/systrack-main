package services

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// SensorNotifyHook, her yeni sensör okumасında bildirim sistemini tetiklemek için
// main.go tarafından set edilir. Circular import'u önlemek için package-level var.
var SensorNotifyHook func(SensorReading)

// SensorReading ESP32 sensöründen gelen anlık okumaları tutar.
type SensorReading struct {
	Serial      string    `json:"serial"`
	Temperature *float64  `json:"temperature"`
	Humidity    *float64  `json:"humidity"`
	Pressure    *float64  `json:"pressure"`
	Gas         *float64  `json:"gas"`
	IAQ         *float64  `json:"iaq"`
	Online      bool      `json:"online"`
	ReceivedAt  time.Time `json:"received_at"`
}

func normalizeAirSensorFieldPath(pathParts []string) string {
	if len(pathParts) == 0 {
		return ""
	}
	parts := make([]string, 0, len(pathParts))
	for _, part := range pathParts {
		part = strings.ToLower(strings.TrimSpace(part))
		if part != "" {
			parts = append(parts, part)
		}
	}
	if len(parts) == 0 {
		return ""
	}

	switch strings.Join(parts, "_") {
	case "sicaklik", "temperature", "temp", "ortam_sicaklik":
		return "sicaklik"
	case "nem", "humidity", "hum", "ortam_nem":
		return "nem"
	case "basinc", "pressure", "press", "ortam_basinc":
		return "basinc"
	case "gaz_direnci", "gas", "ortam_gaz_direnci":
		return "gaz_direnci"
	case "hava_kalite", "hava_kalitesi", "air_quality", "iaq":
		return "hava_kalitesi"
	default:
		return ""
	}
}

// sensorSSEHub browser SSE istemcilerine gerçek zamanlı yayın yapar.
type sensorSSEHub struct {
	mu      sync.RWMutex
	clients map[chan []byte]struct{}
}

func newSensorSSEHub() *sensorSSEHub {
	return &sensorSSEHub{clients: make(map[chan []byte]struct{})}
}

func (h *sensorSSEHub) Add(ch chan []byte) {
	h.mu.Lock()
	h.clients[ch] = struct{}{}
	h.mu.Unlock()
}

func (h *sensorSSEHub) Remove(ch chan []byte) {
	h.mu.Lock()
	delete(h.clients, ch)
	h.mu.Unlock()
	close(ch)
}

func (h *sensorSSEHub) Broadcast(data []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for ch := range h.clients {
		select {
		case ch <- data:
		default:
		}
	}
}

// SensorMQTTService management sunucusunun SSE sensor akışını tüketir ve
// verileri broadcastFn üzerinden WebSocket istemcilerine iletir.
// İsim geriye dönük uyumluluk için korundu.
type SensorMQTTService struct {
	managementURL string
	db            *sql.DB
	broadcastFn   func(SensorReading)

	mu           sync.RWMutex
	linkedSerial string
	current      SensorReading
	cancelSSE    context.CancelFunc // aktif SSE bağlantısını iptal eder
	restartNow   chan struct{}      // manuel yeniden bağlanma sinyali

	SSEHub *sensorSSEHub // browser SSE istemcileri
}

// GetLinkedSerial bağlı sensör serisini döndürür; henüz bilinmiyorsa boş string.
func (s *SensorMQTTService) GetLinkedSerial() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.linkedSerial
}

// SetLinkedSerial sensör serisini günceller ve aktif SSE bağlantısını hemen yeniden başlatır.
func (s *SensorMQTTService) SetLinkedSerial(serial string) {
	s.mu.Lock()
	s.linkedSerial = serial
	s.current.Serial = serial
	cancel := s.cancelSSE
	s.mu.Unlock()

	if cancel != nil {
		cancel() // mevcut SSE bağlantısını kapat
	}
	// 10 saniyelik beklemeyi atla — hemen yeniden bağlan
	select {
	case s.restartNow <- struct{}{}:
	default:
	}
	log.Printf("🔗 Sensor serisi güncellendi: %s — SSE hemen yeniden bağlanıyor", serial)
}

// GetCurrentReading en son bilinen sensör okumasını döndürür.
func (s *SensorMQTTService) GetCurrentReading() SensorReading {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.current
}

// NewSensorMQTTService yeni bir SensorMQTTService oluşturur.
// brokerURL artık kullanılmıyor; management SSE akışı tercih edilir.
func NewSensorMQTTService(brokerURL string, db *sql.DB, managementURL string, broadcastFn func(SensorReading)) *SensorMQTTService {
	return &SensorMQTTService{
		managementURL: managementURL,
		db:            db,
		broadcastFn:   broadcastFn,
		restartNow:    make(chan struct{}, 1),
		SSEHub:        newSensorSSEHub(),
	}
}

// Start arka plan goroutine'lerini başlatır.
func (s *SensorMQTTService) Start() {
	if s.managementURL == "" {
		log.Println("⚠️  Sensor servisi devre dışı (MANAGEMENT_SERVER_URL boş)")
		return
	}
	go s.run()
}

func (s *SensorMQTTService) run() {
	ownSerial := s.waitForOwnSerial()
	log.Printf("📡 Sensor servisi başlatıldı, cihaz=%s management=%s", ownSerial, s.managementURL)

	// Paralel: sensör serisini management'tan proaktif çek.
	go s.resolveLinkedSerial(ownSerial)

	// Ana döngü: management SSE akışına bağlan, kopunca yeniden bağlan.
	sseURL := fmt.Sprintf("%s/api/v1/sensors/live/%s", s.managementURL, ownSerial)
	for {
		ctx, cancel := context.WithCancel(context.Background())
		s.mu.Lock()
		s.cancelSSE = cancel
		s.mu.Unlock()

		if err := s.consumeSSE(ctx, sseURL); err != nil {
			log.Printf("⚠️  Sensor SSE kesildi: %v — 10 sn sonra yeniden bağlanılıyor", err)
		}
		cancel()

		// Çevrimdışı bildirimi gönder
		s.mu.Lock()
		s.current.Online = false
		snap := s.current
		s.mu.Unlock()
		s.broadcastFn(snap)
		if b, err := json.Marshal(snap); err == nil {
			s.SSEHub.Broadcast(b)
		}

		// Manuel yeniden bağlanma sinyali varsa hemen bağlan, yoksa 10 sn bekle
		select {
		case <-s.restartNow:
			log.Println("⚡ Sensor SSE hemen yeniden bağlanıyor")
		case <-time.After(10 * time.Second):
		}
	}
}

// ── Seri çözümleme ────────────────────────────────────────────────────────────

// resolveLinkedSerial management'ın mevcut /api/v1/device-sensor/:serial
// endpoint'ini kullanarak sensör serisini önceden çeker.
// SSE eventlerinden bağımsız çalışır; hem eski hem yeni management sürümleriyle uyumludur.
func (s *SensorMQTTService) resolveLinkedSerial(ownSerial string) {
	for attempt := 1; ; attempt++ {
		serial, err := s.fetchSensorSerial(ownSerial)
		if err == nil && serial != "" {
			s.mu.Lock()
			if s.linkedSerial == "" {
				s.linkedSerial = serial
				s.current.Serial = serial
				log.Printf("✅ Sensor serisi bulundu: cihaz=%s → sensör=%s", ownSerial, serial)
			}
			s.mu.Unlock()
			return
		}
		if err != nil {
			log.Printf("⚠️  Sensor seri sorgusu başarısız (deneme %d): %v — 30 sn sonra", attempt, err)
			time.Sleep(30 * time.Second)
		} else {
			log.Printf("⏳ Henüz sensör atanmamış (deneme %d) — 10 dk sonra", attempt)
			time.Sleep(10 * time.Minute)
		}
	}
}

func (s *SensorMQTTService) fetchSensorSerial(ownSerial string) (string, error) {
	url := fmt.Sprintf("%s/api/v1/device-sensor/%s", s.managementURL, ownSerial)
	resp, err := http.Get(url) //nolint:gosec
	if err != nil {
		return "", fmt.Errorf("HTTP GET: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return "", nil // eski management — yok sayılabilir
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("response body: %w", err)
	}
	var result struct {
		SensorSerial *string `json:"sensor_serial"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("JSON: %w", err)
	}
	if result.SensorSerial == nil || *result.SensorSerial == "" {
		return "", nil
	}
	return *result.SensorSerial, nil
}

// ── SSE tüketici ──────────────────────────────────────────────────────────────

func (s *SensorMQTTService) consumeSSE(ctx context.Context, sseURL string) error {
	req, err := http.NewRequestWithContext(ctx, "GET", sseURL, nil)
	if err != nil {
		return fmt.Errorf("request oluşturulamadı: %w", err)
	}
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Cache-Control", "no-cache")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("bağlantı kurulamadı: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d döndü — management yeniden build edildi ve başlatıldı mı?", resp.StatusCode)
	}

	log.Printf("✅ Sensor SSE bağlantısı kuruldu: %s", sseURL)

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data: ") {
			s.processEvent(strings.TrimPrefix(line, "data: "))
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("okuma hatası: %w", err)
	}
	return fmt.Errorf("stream sunucu tarafından kapatıldı")
}

// ── Event işleme ──────────────────────────────────────────────────────────────

// processEvent management SSE'sinden gelen JSON event'i işler.
// Format: {"topic":"sensors/SERIAL/field","payload":"değer"}
func (s *SensorMQTTService) processEvent(data string) {
	var evt struct {
		Topic   string `json:"topic"`
		Payload string `json:"payload"`
	}
	if err := json.Unmarshal([]byte(data), &evt); err != nil {
		return
	}

	// Beklenen: sensors/{serial}/{field} veya sensors/{serial}/{group}/{field}
	parts := strings.Split(evt.Topic, "/")
	if len(parts) < 3 || parts[0] != "sensors" {
		return
	}
	msgSerial := parts[1]
	field := normalizeAirSensorFieldPath(parts[2:])
	if field == "" {
		return
	}

	s.mu.RLock()
	linked := s.linkedSerial
	s.mu.RUnlock()

	// resolveLinkedSerial henüz tamamlanmadıysa ilk gelen sensöre bağlan (fallback)
	if linked == "" {
		s.mu.Lock()
		if s.linkedSerial == "" {
			s.linkedSerial = msgSerial
			s.current.Serial = msgSerial
			log.Printf("📟 Sensor SSE fallback: ilk aktif sensör=%s", msgSerial)
		}
		linked = s.linkedSerial
		s.mu.Unlock()
	}

	if msgSerial != linked {
		return // bu cihazın sensörü değil
	}

	val, err := strconv.ParseFloat(strings.TrimSpace(evt.Payload), 64)
	if err != nil {
		return
	}

	s.mu.Lock()
	switch field {
	case "sicaklik":
		s.current.Temperature = &val
	case "nem":
		s.current.Humidity = &val
	case "basinc":
		s.current.Pressure = &val
	case "gaz_direnci":
		s.current.Gas = &val
	case "hava_kalitesi":
		s.current.IAQ = &val
	default:
		s.mu.Unlock()
		return
	}
	s.current.Online = true
	snap := s.current
	s.mu.Unlock()

	s.broadcastFn(snap)
	if SensorNotifyHook != nil {
		snap.ReceivedAt = time.Now()
		go SensorNotifyHook(snap)
	}
}

// ── Yardımcılar ───────────────────────────────────────────────────────────────

func (s *SensorMQTTService) waitForOwnSerial() string {
	for {
		var serial string
		err := s.db.QueryRow("SELECT serial_number FROM device_license LIMIT 1").Scan(&serial)
		if err == nil && serial != "" {
			return serial
		}
		log.Println("⏳ Sensor: device_license bekleniyor (aktivasyon tamamlanmamış) — 60 sn sonra")
		time.Sleep(60 * time.Second)
	}
}

// MarshalJSON WebSocket hub üzerinden yayın yapılırken kullanılır.
func (r SensorReading) MarshalJSON() ([]byte, error) {
	type Alias SensorReading
	return json.Marshal(Alias(r))
}
