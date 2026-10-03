package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"systrack/internal/auth"
	"systrack/internal/notifications"

	"github.com/gin-gonic/gin"
	"github.com/masterzen/winrm"
)

// ── Tipler ──────────────────────────────────────────────────────────────────

// CrackFinding tek bir lisans/kötüye kullanım bulgusunu temsil eder
type CrackFinding struct {
	Category    string `json:"category"` // activation | kms_tool | kms_host | license_server | signature | threat_intel | persistence | file_trace | defender_exclusion | historical_execution | event_log_trace | hosts_tampering | unverified_persistence
	Severity    string `json:"severity"` // high | medium | low
	Title       string `json:"title"`
	Description string `json:"description"`
	Evidence    string `json:"evidence,omitempty"`
	Signal      string `json:"signal,omitempty"`
	Confidence  int    `json:"confidence,omitempty"`
	Remediation string `json:"remediation,omitempty"`
	Excepted    bool   `json:"excepted,omitempty"`
}

// ── MalwareBazaar API tipleri ─────────────────────────────────────────────

type mbResponse struct {
	QueryStatus string     `json:"query_status"`
	Data        []mbSample `json:"data"`
}

type mbSample struct {
	Signature   string   `json:"signature"`
	Tags        []string `json:"tags"`
	VendorIntel struct {
		ReversingLabs *struct {
			ThreatName string `json:"threat_name"`
		} `json:"ReversingLabs,omitempty"`
	} `json:"vendor_intel"`
}

// ── AlienVault OTX API tipleri ────────────────────────────────────────────

type otxGeneralResponse struct {
	Reputation int `json:"reputation"`
	PulseInfo  struct {
		Count  int `json:"count"`
		Pulses []struct {
			MalwareFamilies []struct {
				DisplayName string `json:"display_name"`
			} `json:"malware_families"`
		} `json:"pulses"`
		Related struct {
			AlienVault struct {
				MalwareFamilies []string `json:"malware_families"`
			} `json:"alienvault"`
		} `json:"related"`
	} `json:"pulse_info"`
}

// otxAnalysisResult, OTX /analysis endpoint'inden çıkarılan statik analiz sinyallerini tutar.
// /general'in aksine bu endpoint dosyanın içeriğini analiz eder (Adobe sınıflandırıcı,
// PE anomali tespiti, VMProtect/UPX gibi packer bölümleri, ClamAV, YARA).
type otxAnalysisResult struct {
	MalwareAlerts []string // adobemalwareclassifier.results.alerts — en değerli, hash-bağımsız sinyal
	PEAnomalies   int      // peanomal.results.anomalies — anormal PE başlık sayısı
	VMProtected   bool     // .avm* (VMProtect) / .upx* (UPX) / Themida bölümleri var mı
	PackerName    string   // VMProtect, UPX, Themida...
	Signed        int      // pe32info.results.signed — 0=geçerli imza yok (OTX'in kendi doğrulaması)
	Imphash       string   // pe32info.results.imphash — aynı yapıdaki varyantları bulmak için
	ClamAVHit     string   // ClamAV tespiti (genellikle boş, ama var ise kesin sinyal)
	YARAHits      []string // Eşleşen YARA kuralları
}

// mbKeyInvalidWarned, geçersiz MB anahtarı uyarısının yalnızca bir kez loglanmasını sağlar.
var mbKeyInvalidWarned sync.Once

// ── Risk seviyesi hesaplayıcı ────────────────────────────────────────────────

func calcRiskLevel(findings []CrackFinding) string {
	if len(findings) == 0 {
		return "clean"
	}
	score := 0
	for _, f := range findings {
		if f.Excepted {
			continue
		}
		switch f.Severity {
		case "high":
			score += 70
		case "medium":
			score += 35
		case "low":
			score += 10
		}
	}
	if score >= 70 {
		return "high"
	}
	if score >= 35 {
		return "medium"
	}
	if score > 0 {
		return "low"
	}
	return "clean"
}

func saveCrackScanResult(db *sql.DB, inventoryID int, riskLevel string, findings []CrackFinding) error {
	findingsJSON := "[]"
	if len(findings) > 0 {
		if b, err := json.Marshal(findings); err == nil {
			findingsJSON = string(b)
		}
	}
	_, err := db.Exec(`
		UPDATE inventory
		SET crack_scan_at = NOW(),
		    crack_risk_level = ?,
		    crack_findings_json = ?
		WHERE id = ?
	`, riskLevel, findingsJSON, inventoryID)
	return err
}

// ScanInventoryCrack runs an on-demand compliance/crack scan for one inventory item.
func ScanInventoryCrack(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		inventoryID, err := strconv.Atoi(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Geçersiz envanter ID"})
			return
		}

		var hostname, ipAddress, assetName sql.NullString
		if err := db.QueryRow(`
			SELECT hostname, ip_address, asset_name
			FROM inventory
			WHERE id = ?
		`, inventoryID).Scan(&hostname, &ipAddress, &assetName); err != nil {
			if err == sql.ErrNoRows {
				c.JSON(http.StatusNotFound, gin.H{"error": "Envanter kaydı bulunamadı"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Envanter kaydı okunamadı: " + err.Error()})
			return
		}

		target := strings.TrimSpace(hostname.String)
		ipFallback := strings.TrimSpace(ipAddress.String)
		if target == "" {
			target = ipFallback
		}
		if target == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Bu envanter kaydında hostname/IP yok"})
			return
		}

		var username, encryptedPassword string
		var port, timeoutSeconds int
		var useSSL bool

		// Önce standalone host kaydına bak (domain dışı cihazlar için kendi credentials'ları)
		standaloneErr := db.QueryRow(`
			SELECT username, password_encrypted, port, use_ssl
			FROM winrm_standalone_hosts
			WHERE ip_address = ? AND is_enabled = 1
			LIMIT 1
		`, ipFallback).Scan(&username, &encryptedPassword, &port, &useSSL)

		if standaloneErr != nil {
			// Standalone bulunamadı, global winrm_settings'e bak
			var isEnabled bool
			err = db.QueryRow(`
				SELECT username, password_encrypted, port, use_ssl, timeout_seconds, is_enabled
				FROM winrm_settings
				WHERE id = 1
			`).Scan(&username, &encryptedPassword, &port, &useSSL, &timeoutSeconds, &isEnabled)
			if err != nil || !isEnabled {
				c.JSON(http.StatusBadRequest, gin.H{"error": "WinRM ayarları yapılandırılmamış veya etkin değil. Domain dışı cihazlar için Keşif Ayarları → Domain Dışı Windows Cihazlar bölümünden bu cihazı ekleyin."})
				return
			}
		}

		password, err := auth.DecryptPassword(encryptedPassword)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "WinRM şifresi çözülemedi: " + err.Error()})
			return
		}

		settings := map[string]interface{}{
			"username": username,
			"password": password,
			"port":     port,
			"use_ssl":  useSSL,
			"timeout":  timeoutSeconds,
		}
		client, err := tryWinRMClient(target, ipFallback, settings)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "WinRM bağlantısı kurulamadı: " + err.Error()})
			return
		}

		riskLevel, findings, durationMs := PerformCrackScan(client)
		if err := saveCrackScanResult(db, inventoryID, riskLevel, findings); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Tarama sonucu kaydedilemedi: " + err.Error()})
			return
		}

		findings, riskLevel = applyCrackScanExceptions(db, inventoryID, findings)

		if riskLevel != "clean" {
			deviceName := assetName.String
			if deviceName == "" {
				deviceName = target
			}
			notifications.NotifyCrackDetected(inventoryID, deviceName, ipFallback, riskLevel, len(findings))
		}

		c.JSON(http.StatusOK, gin.H{
			"inventory_id": inventoryID,
			"asset_name":   assetName.String,
			"risk_level":   riskLevel,
			"findings":     findings,
			"duration_ms":  durationMs,
		})
	}
}

// ── CIRCL hashlookup (NIST NSRL) ─────────────────────────────────────────

const (
	nsrlKnownGood = "known_good"
	nsrlNotFound  = "not_found"
	nsrlError     = "error"
)

// checkNSRL NIST NSRL'de hash arar. "error" durumunda asla "not_found" gibi davranılmaz (fail-open).
func checkNSRL(sha256 string) string {
	if sha256 == "" {
		return nsrlError
	}
	client := &http.Client{Timeout: 8 * time.Second}
	resp, err := client.Get("https://hashlookup.circl.lu/lookup/sha256/" + strings.ToLower(sha256))
	if err != nil {
		return nsrlError
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	switch resp.StatusCode {
	case 200:
		return nsrlKnownGood
	case 404:
		return nsrlNotFound
	default:
		return nsrlError
	}
}

// ── MalwareBazaar API ─────────────────────────────────────────────────────

// checkMalwareBazaar, SHA-256 hash'ini MalwareBazaar'da arar.
// Bulunan her hash kesinlikle onaylı malware'dir (DB'de yalnızca malware vardır).
// found=false ise "bilinmiyor" demektir, "temiz" değil.
func checkMalwareBazaar(sha256, apiKey string) (found bool, signature string, tags []string) {
	if sha256 == "" || apiKey == "" {
		return false, "", nil
	}
	body := strings.NewReader("query=get_info&hash=" + strings.ToLower(sha256))
	req, err := http.NewRequest("POST", "https://mb-api.abuse.ch/api/v1/", body)
	if err != nil {
		return false, "", nil
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Auth-Key", apiKey)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return false, "", nil
	}
	defer resp.Body.Close()

	var result mbResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return false, "", nil
	}
	if result.QueryStatus == "unknown_auth_key" || result.QueryStatus == "unauthorized" {
		mbKeyInvalidWarned.Do(func() {
			log.Printf("[crack-scan] ⚠️  MalwareBazaar API anahtarı geçersiz ('%s'). bazaar.abuse.ch > Profile sayfasından yeni anahtar alın ve MALWAREBAZAAR_KEY ortam değişkenini güncelleyin.", result.QueryStatus)
		})
		return false, "", nil
	}
	if result.QueryStatus != "ok" || len(result.Data) == 0 {
		return false, "", nil
	}

	sample := result.Data[0]
	sig := sample.Signature
	if sig == "" && sample.VendorIntel.ReversingLabs != nil {
		sig = sample.VendorIntel.ReversingLabs.ThreatName
	}
	return true, sig, sample.Tags
}

// checkMalwareBazaarImphash, import hash'ini MalwareBazaar'da arar.
// Aynı imphash'e sahip örnekler aynı DLL bağımlılık yapısını paylaşır: bu, özellikle
// bileşen aynı kalıp sadece gövde değiştiğinde (crack patch'i gibi) varyantları tespit etmeye yarar.
// Hash eşleşmesi yokken bile imphash eşleşmesi varsa binary aynı aileden bir araç.
func checkMalwareBazaarImphash(imphash, apiKey string) (found bool, families []string) {
	if imphash == "" || apiKey == "" {
		return false, nil
	}
	body := strings.NewReader("query=get_imphash&imphash=" + strings.ToLower(imphash))
	req, err := http.NewRequest("POST", "https://mb-api.abuse.ch/api/v1/", body)
	if err != nil {
		return false, nil
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Auth-Key", apiKey)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return false, nil
	}
	defer resp.Body.Close()

	var result mbResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return false, nil
	}
	if result.QueryStatus != "ok" || len(result.Data) == 0 {
		return false, nil
	}

	// Benzersiz imzaları topla (benzer binary ailesini temsil etmek için)
	seen := map[string]bool{}
	for _, s := range result.Data {
		sig := s.Signature
		if sig == "" && s.VendorIntel.ReversingLabs != nil {
			sig = s.VendorIntel.ReversingLabs.ThreatName
		}
		if sig != "" && !seen[sig] {
			families = append(families, sig)
			seen[sig] = true
		}
	}
	return true, families
}

// ── AlienVault OTX API ────────────────────────────────────────────────────

// checkOTX, SHA-256 hash'ini AlienVault OTX /general endpoint'inde arar.
// pulseCount > 0 → tehdit istihbaratı topluluğu tarafından işaretlenmiş.
// Eşik: 1-4 pulse = şüpheli, 5+ pulse = yüksek güvenli tespit.
// reputation < 0 → OTX topluluğu bu dosyayı zararlı/şüpheli buluyor (pulse olmasa da).
func checkOTX(sha256, apiKey string) (pulseCount int, families []string, reputation int) {
	if sha256 == "" || apiKey == "" {
		return 0, nil, 0
	}
	url := "https://otx.alienvault.com/api/v1/indicators/file/" + strings.ToLower(sha256) + "/general"
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return 0, nil, 0
	}
	req.Header.Set("X-OTX-API-KEY", apiKey)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, 0
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return 0, nil, 0
	}

	var result otxGeneralResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, nil, 0
	}

	// Malware ailelerini topla (related.alienvault önce, sonra pulse'lardan)
	families = result.PulseInfo.Related.AlienVault.MalwareFamilies
	if len(families) == 0 {
		seen := map[string]bool{}
		for _, pulse := range result.PulseInfo.Pulses {
			for _, mf := range pulse.MalwareFamilies {
				if mf.DisplayName != "" && !seen[mf.DisplayName] {
					families = append(families, mf.DisplayName)
					seen[mf.DisplayName] = true
				}
			}
		}
	}

	return result.PulseInfo.Count, families, result.Reputation
}

// checkOTXAnalysis, OTX'in /analysis endpoint'inden statik analiz sinyallerini çeker.
// Bu endpoint; Adobe Malware Classifier tespiti, PE anomalileri, VMProtect/UPX gibi
// packer bölümleri ve ClamAV/YARA sonuçlarını içerir — dosya hash'ine bağlı olmayan,
// içerik tabanlı sinyaller sağlar. /general'de 0 pulse olsa bile tespit üretebilir
// (FL Studio crack'i buna örnek: 0 pulse ama Adobe classifier "Malware detected" döndürür).
func checkOTXAnalysis(sha256, apiKey string) otxAnalysisResult {
	var r otxAnalysisResult
	if sha256 == "" || apiKey == "" {
		return r
	}

	req, err := http.NewRequest("GET",
		"https://otx.alienvault.com/api/v1/indicators/file/"+strings.ToLower(sha256)+"/analysis", nil)
	if err != nil {
		return r
	}
	req.Header.Set("X-OTX-API-KEY", apiKey)

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return r
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return r
	}

	var data struct {
		Analysis struct {
			Plugins struct {
				AdobeMalwareClassifier struct {
					Results struct {
						Alerts []string `json:"alerts"`
					} `json:"results"`
				} `json:"adobemalwareclassifier"`
				Peanomal struct {
					Results struct {
						Anomalies int `json:"anomalies"`
					} `json:"results"`
				} `json:"peanomal"`
				Pe32info struct {
					Results struct {
						Sections []struct {
							Name string `json:"Name"`
						} `json:"sections"`
						Signed  int    `json:"signed"`
						Imphash string `json:"imphash"`
					} `json:"results"`
				} `json:"pe32info"`
				ClamAV struct {
					Results map[string]interface{} `json:"results"`
				} `json:"clamav"`
				Yarad struct {
					Results struct {
						Detection []struct {
							Name string `json:"name"`
						} `json:"detection"`
					} `json:"results"`
				} `json:"yarad"`
			} `json:"plugins"`
		} `json:"analysis"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return r
	}

	p := data.Analysis.Plugins
	r.MalwareAlerts = p.AdobeMalwareClassifier.Results.Alerts
	r.PEAnomalies = p.Peanomal.Results.Anomalies
	r.Signed = p.Pe32info.Results.Signed
	r.Imphash = p.Pe32info.Results.Imphash

	// VMProtect (.avm0/.avm1), UPX (.upx0/.upx1), Themida bölüm tespiti.
	// Crack araçları genellikle AV tespitinden kaçmak için bu araçlarla paketlenir;
	// meşru ticari yazılımlar nadiren kullanır (özellikle kırık imzayla birlikte).
	for _, sec := range p.Pe32info.Results.Sections {
		name := strings.ToLower(sec.Name)
		switch {
		case strings.Contains(name, "avm") || strings.Contains(name, ".vmp"):
			r.VMProtected = true
			if r.PackerName == "" {
				r.PackerName = "VMProtect"
			}
		case strings.Contains(name, "upx"):
			r.VMProtected = true
			if r.PackerName == "" {
				r.PackerName = "UPX"
			}
		case strings.Contains(name, "themida") || strings.Contains(name, "winlicens"):
			r.VMProtected = true
			if r.PackerName == "" {
				r.PackerName = "Themida"
			}
		case strings.Contains(name, ".mpress") || strings.Contains(name, "mpress"):
			r.VMProtected = true
			if r.PackerName == "" {
				r.PackerName = "MPRESS"
			}
		}
	}

	// ClamAV hit — map boşsa tespit yok, dolu ise detection var
	for k := range p.ClamAV.Results {
		if k != "" {
			r.ClamAVHit = k
			break
		}
	}

	// YARA eşleşmeleri
	for _, d := range p.Yarad.Results.Detection {
		if d.Name != "" {
			r.YARAHits = append(r.YARAHits, d.Name)
		}
	}

	return r
}

// ── VirusTotal API ────────────────────────────────────────────────────────

// checkVirusTotal, SHA-256 hash'ini VirusTotal v3'te arar. maliciousCount, 70+ AV motoru
// arasından "malicious" veya "suspicious" diyenlerin toplamıdır — MalwareBazaar/OTX'ten
// bağımsız üçüncü bir tehdit istihbaratı kaynağıdır (çoğu motorun ortak görüşü olduğu için
// genelde en güvenilir/az yanlış pozitifli sinyal).
func checkVirusTotal(sha256, apiKey string) (maliciousCount int, threatLabel string) {
	if sha256 == "" || apiKey == "" {
		return 0, ""
	}
	req, err := http.NewRequest("GET", "https://www.virustotal.com/api/v3/files/"+strings.ToLower(sha256), nil)
	if err != nil {
		return 0, ""
	}
	req.Header.Set("x-apikey", apiKey)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return 0, ""
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return 0, ""
	}

	var result struct {
		Data struct {
			Attributes struct {
				LastAnalysisStats struct {
					Malicious  int `json:"malicious"`
					Suspicious int `json:"suspicious"`
				} `json:"last_analysis_stats"`
				PopularThreatClassification struct {
					SuggestedThreatLabel string `json:"suggested_threat_label"`
				} `json:"popular_threat_classification"`
			} `json:"attributes"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, ""
	}

	stats := result.Data.Attributes.LastAnalysisStats
	return stats.Malicious + stats.Suspicious, result.Data.Attributes.PopularThreatClassification.SuggestedThreatLabel
}

// ── Yardımcı fonksiyonlar ─────────────────────────────────────────────────

// isTrustedPublisher, Authenticode sertifikasının güvenilir bir yayıncıya
// ait olup olmadığını kontrol eder. Risk azaltıcı olarak kullanılır, hard
// whitelist değil — MB/OTX bulgusu varsa yayıncıdan bağımsız flaglenir.
func isTrustedPublisher(subject string) bool {
	s := strings.ToLower(subject)
	for _, trusted := range []string{
		"microsoft corporation",
		"microsoft windows",
		"adobe inc",
		"adobe systems",
	} {
		if strings.Contains(s, trusted) {
			return true
		}
	}
	return false
}

// licenseStatusText, Windows/Office LicenseStatus kodunu açıklayan metin döndürür.
func licenseStatusText(status int) (severity, description string) {
	switch status {
	case 0:
		return "high", "Lisanssız (Unlicensed) — aktif crack veya lisans hatası"
	case 1:
		return "", "Lisanslı"
	case 2:
		return "low", "OOB Grace süresi — yeni kurulum veya geçiş dönemi"
	case 3:
		return "medium", "OOT Grace süresi — lisans süresi dolmuş, yenileme gerekli"
	case 4:
		return "high", "Non-Genuine Grace — lisans orijinal değil"
	case 5:
		return "high", "Bildirim modu (Notification) — lisans geçersiz sayılıyor"
	case 6:
		return "medium", "Extended Grace — son kullanma tarihi yaklaşıyor"
	default:
		return "low", fmt.Sprintf("Bilinmeyen lisans durumu (kod: %d)", status)
	}
}

// shortHash, hash'in ilk 16 karakterini gösterir (log/UI için)
func shortHash(h string) string {
	if len(h) > 16 {
		return h[:16] + "..."
	}
	return h
}

// enrichAndDedupeFindings tekrarlanan kanıtları eler, eksik signal/confidence/remediation
// alanlarını varsayılan değerlerle doldurur.
func enrichAndDedupeFindings(findings []CrackFinding) []CrackFinding {
	seen := map[string]bool{}
	out := make([]CrackFinding, 0, len(findings))
	for _, f := range findings {
		key := findingSignature(f)
		if key == "||" || seen[key] {
			continue
		}
		seen[key] = true
		if f.Signal == "" {
			f.Signal = f.Category
		}
		if f.Confidence == 0 {
			f.Confidence = defaultFindingConfidence(f)
		}
		if f.Remediation == "" {
			f.Remediation = defaultFindingRemediation(f)
		}
		out = append(out, f)
	}
	return out
}

func defaultFindingConfidence(f CrackFinding) int {
	switch f.Category {
	case "threat_intel":
		if f.Severity == "high" {
			return 98
		}
		return 88
	case "kms_tool", "kms_host", "license_server", "defender_exclusion", "persistence", "hosts_tampering":
		return 92
	case "signature":
		if f.Severity == "high" {
			return 95
		}
		return 78
	case "activation":
		if f.Severity == "high" {
			return 85
		}
		return 65
	case "file_trace":
		return 72
	case "historical_execution":
		return 68
	case "event_log_trace":
		return 70
	case "unverified_persistence":
		return 55
	default:
		switch f.Severity {
		case "high":
			return 80
		case "medium":
			return 60
		default:
			return 40
		}
	}
}

func defaultFindingRemediation(f CrackFinding) string {
	switch f.Category {
	case "activation":
		return "Lisans durumunu doğrula, kurumsal lisans/KMS kaynağını kontrol et ve yetkisiz aktivasyon araçlarını kaldır."
	case "kms_host":
		return "KMS sunucusunu doğrula: kurumsal KMS host'u değilse, makinedeki yerel KMS emülatör/servisini kaldır ve yeniden aktivasyon yap."
	case "license_server":
		return "Bu makinede neden bir lisans sunucusu portu dinlendiğini araştır; bir uç nokta cihazı olmaması gerekir. İlgili süreci/servisi kaldır ve yazılımı resmi lisans yöntemiyle yeniden yapılandır."
	case "kms_tool", "persistence", "defender_exclusion", "file_trace", "historical_execution", "event_log_trace":
		return "İlgili aracı, servis/görev/startup izlerini ve Defender istisnalarını kaldır; ardından endpoint üzerinde AV/EDR taraması çalıştır."
	case "hosts_tampering":
		return "Hosts dosyasındaki ilgili satırı kaldır (C:\\Windows\\System32\\drivers\\etc\\hosts), DNS önbelleğini temizle ve lisans/aktivasyon durumunu yeniden doğrula."
	case "unverified_persistence":
		return "Bu servis/görevin ne olduğunu manuel doğrula (meşru bir iç araç olabilir). Bilinmiyorsa veya gereksizse kaldır, gerekiyorsa imzalı/resmi bir sürümle değiştir."
	case "signature", "threat_intel":
		return "Dosyayı karantinaya al ve yazılımı resmi kaynaktan yeniden kur. Sistem yöneticisini durumdan haberdar et."
	default:
		return "Bulguyu manuel olarak doğrula ve lisans uyumluluğu aksiyonlarını uygula."
	}
}

// threatIntelFindings, bir SHA256 için MalwareBazaar, OTX ve VirusTotal'i (cache
// üzerinden, paralel) sorgular ve bulgular döndürür. Servis adı ve binary path kanıt
// olarak eklenir.
func threatIntelFindings(sha256, displayName, mbKey, otxKey, vtKey string) []CrackFinding {
	var findings []CrackFinding
	if sha256 == "" {
		return findings
	}

	result := lookupThreatIntel(sha256, mbKey, otxKey, vtKey, false)

	if mbKey != "" && result.mbFound {
		evidence := ""
		detail := result.mbSignature
		if detail == "" && len(result.mbTags) > 0 {
			detail = strings.Join(result.mbTags, ", ")
		}
		if detail != "" {
			evidence = fmt.Sprintf("Zararlı yazılım ailesi: %s", detail)
		}
		findings = append(findings, CrackFinding{
			Category:    "threat_intel",
			Severity:    "high",
			Title:       fmt.Sprintf("%s zararlı yazılım veritabanlarında kayıtlı", displayName),
			Description: "Bu program, uluslararası güvenlik veritabanlarında onaylı zararlı yazılım olarak kayıtlıdır.",
			Evidence:    evidence,
		})
	}

	// MB imphash: hash eşleşmesi olmasa bile aynı kod yapısına sahip malware varyantları
	if mbKey != "" && result.mbImphashFound {
		familyStr := ""
		if len(result.mbImphashFamilies) > 0 {
			familyStr = fmt.Sprintf(" (bilinen aileler: %s)", strings.Join(result.mbImphashFamilies[:min(3, len(result.mbImphashFamilies))], ", "))
		}
		findings = append(findings, CrackFinding{
			Category:    "threat_intel",
			Severity:    "medium",
			Title:       fmt.Sprintf("%s bilinen zararlı yazılım araçlarıyla aynı kod yapısına sahip", displayName),
			Description: fmt.Sprintf("Bu program, tehdit istihbarat veritabanlarında kayıtlı zararlı yazılımlarla aynı kod yapısını paylaşıyor%s. Farklı bir varyant olsa da aynı araç ailesine ait olabilir.", familyStr),
			Evidence:    "",
		})
	}

	if otxKey != "" && result.otxPulseCount > 0 {
		severity := "medium"
		if result.otxPulseCount >= 5 {
			severity = "high"
		}
		familyStr := ""
		if len(result.otxFamilies) > 0 {
			familyStr = fmt.Sprintf(" — Aile: %s", strings.Join(result.otxFamilies, ", "))
		}
		findings = append(findings, CrackFinding{
			Category:    "threat_intel",
			Severity:    severity,
			Title:       fmt.Sprintf("%s birden fazla güvenlik kaynağında tehdit olarak işaretli", displayName),
			Description: fmt.Sprintf("Bu dosya %d bağımsız güvenlik kaynağında tehdit olarak işaretlenmiş%s.", result.otxPulseCount, familyStr),
			Evidence:    fmt.Sprintf("Eşleşen kaynak sayısı: %d", result.otxPulseCount),
		})
	}

	if vtKey != "" && result.vtMalicious > 0 {
		severity := "medium"
		if result.vtMalicious >= 5 {
			severity = "high"
		}
		labelStr := ""
		if result.vtLabel != "" {
			labelStr = fmt.Sprintf(" — Sınıflandırma: %s", result.vtLabel)
		}
		findings = append(findings, CrackFinding{
			Category:    "threat_intel",
			Severity:    severity,
			Title:       fmt.Sprintf("%s, %d antivirüs motoru tarafından tehdit olarak tanımlandı", displayName, result.vtMalicious),
			Description: fmt.Sprintf("Bağımsız %d antivirüs motoru bu dosyayı zararlı ya da şüpheli olarak işaretledi%s.", result.vtMalicious, labelStr),
			Evidence:    fmt.Sprintf("Tehdit olarak işaretleyen motor sayısı: %d", result.vtMalicious),
		})
	}

	// ── OTX /analysis endpoint bulgular ─────────────────────────────────────
	// /analysis, /general'den bağımsız çalışır: dosyanın hash'i tehdit veritabanında
	// olmasa bile (pulse_count=0) içerik analizi sinyal üretebilir.

	// Adobe Malware Classifier tespiti — en değerli sinyal: hash-bağımsız istatistiksel/
	// davranışsal sınıflandırma. Gerçek FL Studio crack'i (0 OTX pulse'a rağmen) bu
	// yöntemle high severity olarak tespit edildi.
	if otxKey != "" && len(result.otxAnalysis.MalwareAlerts) > 0 {
		findings = append(findings, CrackFinding{
			Category:    "threat_intel",
			Severity:    "high",
			Title:       fmt.Sprintf("%s istatistiksel analiz sonucunda zararlı olarak sınıflandırıldı", displayName),
			Description: "Bu program davranışsal ve istatistiksel analiz yöntemleriyle zararlı yazılım olarak sınıflandırıldı. Tespit dosya imzasına bağlı değil; içeriğin yapısal özelliklerine dayanmaktadır.",
			Evidence:    strings.Join(result.otxAnalysis.MalwareAlerts, "; "),
		})
	}

	// YARA kuralı eşleşmesi — OTX/YARA-Detect kuralları genellikle belirli malware aileleri için yazılır
	if otxKey != "" && len(result.otxAnalysis.YARAHits) > 0 {
		findings = append(findings, CrackFinding{
			Category:    "threat_intel",
			Severity:    "high",
			Title:       fmt.Sprintf("%s bilinen bir zararlı yazılım imzasıyla eşleşti", displayName),
			Description: "Bu program, bilinen zararlı yazılım ailelerine ait imza kurallarından biriyle örtüştü.",
			Evidence:    fmt.Sprintf("Eşleşen imzalar: %s", strings.Join(result.otxAnalysis.YARAHits, ", ")),
		})
	}

	// ClamAV tespiti — OTX'in ClamAV motoru zararlı olarak işaretlediyse
	if otxKey != "" && result.otxAnalysis.ClamAVHit != "" {
		findings = append(findings, CrackFinding{
			Category:    "threat_intel",
			Severity:    "high",
			Title:       fmt.Sprintf("%s antivirüs motoru tarafından tanındı", displayName),
			Description: "Bu program antivirüs motorları tarafından bilinen bir tehdit olarak tanımlandı.",
			Evidence:    fmt.Sprintf("Tespit: %s", result.otxAnalysis.ClamAVHit),
		})
	}

	// Kod sanallaştırma / packer tespiti — VMProtect, UPX, Themida.
	// Crack araçları AV tespitinden kaçmak için sıklıkla VMProtect/UPX kullanır.
	// Meşru ticari yazılımlar bu araçları nadiren kullanır. Kırık imzayla birlikte
	// (psProgramIntegrity zaten signature/medium üretmiş) bu medium güvenilir sinyale dönüşür.
	if otxKey != "" && result.otxAnalysis.VMProtected {
		anomalyStr := ""
		if result.otxAnalysis.PEAnomalies > 0 {
			anomalyStr = fmt.Sprintf(", %d PE anomalisi", result.otxAnalysis.PEAnomalies)
		}
		findings = append(findings, CrackFinding{
			Category:    "threat_intel",
			Severity:    "medium",
			Title:       fmt.Sprintf("%s içinde kod gizleme tekniği tespit edildi (%s)", displayName, result.otxAnalysis.PackerName),
			Description: fmt.Sprintf("%s tespit edildi. Meşru ticari yazılımlar bu tekniği nadiren kullanır; crack/keygen araçları antivirüs tespitinden kaçmak için sıklıkla bu yönteme başvurur.", result.otxAnalysis.PackerName),
			Evidence:    fmt.Sprintf("Teknik: %s%s", result.otxAnalysis.PackerName, anomalyStr),
		})
	}

	// OTX itibar puanı — negatif değerler OTX topluluğunun bu dosyayı zararlı bulduğunu gösterir.
	// Yalnızca pulse olmasa da itibar düşükse (bazı dosyalar pulses'sız yine de negatif itibar alır).
	if otxKey != "" && result.otxReputation < -20 {
		sev := "medium"
		if result.otxReputation < -50 {
			sev = "high"
		}
		findings = append(findings, CrackFinding{
			Category:    "threat_intel",
			Severity:    sev,
			Title:       fmt.Sprintf("%s güvenlik topluluğu tarafından tehdit olarak tanınıyor", displayName),
			Description: "Bu program, güvenlik araştırma topluluğu tarafından zararlı ya da şüpheli olarak değerlendirilmektedir.",
			Evidence:    "",
		})
	}

	return findings
}

// ── Ana Tarama Fonksiyonu ─────────────────────────────────────────────────

// crackScanTimeBudget caps how long PerformCrackScan keeps running the optional/heavier
// blocks (file traces, Defender exclusions, KMS host, Amcache, event log, Office check).
// A single slow or unreachable target should degrade to partial findings, not hang the
// whole HTTP request indefinitely. Each block is its own remote PowerShell round-trip
// (cold process start every time), so even a healthy target legitimately needs a few
// minutes for the full ~11-block sequence - this is a whole-scan ceiling, not a per-block one.
const crackScanTimeBudget = 4 * time.Minute

// PerformCrackScan, açık WinRM bağlantısı üzerinde kapsamlı lisans/crack taraması yapar.
// DB'ye yazmaz — sadece bulguları döndürür. Hiçbir manuel konfigürasyona (güvenilir host
// listesi, indikatör listesi girişi vb.) ihtiyaç duymaz; KMS host meşruiyeti gibi kontroller
// domain'in kendi DNS SRV kaydından otomatik öğrenilir (bkz. classifyKMSHost).
// API anahtarları ortam değişkenlerinden okunur (MALWAREBAZAAR_KEY, OTX_API_KEY,
// VIRUSTOTAL_API_KEY). Her biri opsiyoneldir; boşsa o kaynak sessizce atlanır.
func PerformCrackScan(client *winrm.Client) (riskLevel string, findings []CrackFinding, durationMs int64) {
	start := time.Now()
	findings = []CrackFinding{}
	budgetExceeded := func() bool { return time.Since(start) > crackScanTimeBudget }

	mbKey := os.Getenv("MALWAREBAZAAR_KEY")
	otxKey := os.Getenv("OTX_API_KEY")
	vtKey := os.Getenv("VIRUSTOTAL_API_KEY")

	// ── 1-5. Temel kontroller — tek PS oturumunda (batch) ────────────────────
	// Eski yaklaşım 7 ayrı WinRM round-trip yapıyordu: her biri 1-3s PS process
	// başlatma maliyeti taşır → toplamda 7-21s saf overhead. Tek batch call ile
	// tüm temel kontroller tek bir PS process'i içinde sırayla çalışır; combined
	// JSON döner. PS process maliyeti 7→1'e iner.
	psBatchCrack := fmt.Sprintf(
		`$rx='%s';$r=@{`+
			`ACT=@(try{Get-WmiObject -Query 'SELECT Name,LicenseStatus FROM SoftwareLicensingProduct WHERE PartialProductKey IS NOT NULL'|Where-Object{$_.Name -match 'Windows|Office'}|Select-Object Name,LicenseStatus}catch{@()});`+
			`OHOOK=(try{$dirs=@('C:\Program Files\Microsoft Office\root\Office16','C:\Program Files (x86)\Microsoft Office\root\Office16','C:\Program Files\Microsoft Office\Office16','C:\Program Files (x86)\Microsoft Office\Office16');$found=$null;foreach($d in $dirs){$dll=Join-Path $d 'sppc.dll';if(Test-Path $dll){$sig=Get-AuthenticodeSignature $dll -EA SilentlyContinue;$found=[PSCustomObject]@{Path=$dll;Status=if($sig){$sig.Status.ToString()}else{'Unknown'};Subject=if($sig -and $sig.SignerCertificate){$sig.SignerCertificate.Subject}else{''};Thumbprint=if($sig -and $sig.SignerCertificate){$sig.SignerCertificate.Thumbprint}else{''}};break}};$found}catch{$null});`+
			`OFFICESIG=(try{$exe=@('C:\Program Files\Microsoft Office\root\Office16\WINWORD.EXE','C:\Program Files (x86)\Microsoft Office\root\Office16\WINWORD.EXE','C:\Program Files\Microsoft Office\Office16\WINWORD.EXE','C:\Program Files (x86)\Microsoft Office\Office16\WINWORD.EXE')|Where-Object{Test-Path $_}|Select-Object -First 1;if($exe){$s=Get-AuthenticodeSignature $exe -EA SilentlyContinue;[PSCustomObject]@{Path=$exe;Status=if($s){$s.Status.ToString()}else{'Unknown'};Subject=if($s -and $s.SignerCertificate){$s.SignerCertificate.Subject}else{''};Thumbprint=if($s -and $s.SignerCertificate){$s.SignerCertificate.Thumbprint}else{''}}}else{$null}}catch{$null});`+
			`SVC=@(try{Get-WmiObject Win32_Service|Where-Object{$_.Name -imatch $rx -or $_.DisplayName -imatch $rx -or ($_.PathName -and ($_.PathName -imatch $rx))}|ForEach-Object{$raw=$_.PathName;$exe='';if($raw){if($raw.StartsWith('"')){$exe=($raw -replace '^"([^"]+)".*','$1')}else{$exe=($raw -split ' ')[0]}};$sigStatus='Unknown';$sigSubject='';$sigThumbprint='';$sha256='';if($exe -and (Test-Path $exe -EA SilentlyContinue)){$s=Get-AuthenticodeSignature $exe -EA SilentlyContinue;if($s){$sigStatus=$s.Status.ToString();if($s.SignerCertificate){$sigSubject=$s.SignerCertificate.Subject;$sigThumbprint=$s.SignerCertificate.Thumbprint}};$h=Get-FileHash -Path $exe -Algorithm SHA256 -EA SilentlyContinue;if($h){$sha256=$h.Hash}};[PSCustomObject]@{Name=$_.Name;DisplayName=$_.DisplayName;PathName=$raw;State=$_.State;SigStatus=$sigStatus;SigSubject=$sigSubject;SigThumbprint=$sigThumbprint;SHA256=$sha256}}catch{@()});`+
			`TASKS=@(try{Get-ScheduledTask|Where-Object{$_.TaskName -imatch $rx}|Select-Object TaskName,TaskPath,State}catch{@()});`+
			`INSTALLED=@(try{Get-ItemProperty 'HKLM:\Software\Microsoft\Windows\CurrentVersion\Uninstall\*','HKLM:\Software\Wow6432Node\Microsoft\Windows\CurrentVersion\Uninstall\*' -EA SilentlyContinue|Where-Object{$_.DisplayName -imatch $rx}|Select-Object DisplayName,Publisher}catch{@()});`+
			`STARTUP=@(try{$si=@();foreach($p in @('HKLM:\Software\Microsoft\Windows\CurrentVersion\Run','HKLM:\Software\Microsoft\Windows\CurrentVersion\RunOnce','HKLM:\Software\Wow6432Node\Microsoft\Windows\CurrentVersion\Run','HKLM:\Software\Wow6432Node\Microsoft\Windows\CurrentVersion\RunOnce','HKCU:\Software\Microsoft\Windows\CurrentVersion\Run','HKCU:\Software\Microsoft\Windows\CurrentVersion\RunOnce')){$props=Get-ItemProperty -Path $p -EA SilentlyContinue;if($props){$props.PSObject.Properties|Where-Object{$_.Name -notmatch '^PS' -and [string]$_.Value -imatch $rx}|ForEach-Object{$si+=[PSCustomObject]@{Source='RunKey';Name=$_.Name;Value=[string]$_.Value;Path=$p}}}};foreach($dir in @("$env:ProgramData\Microsoft\Windows\Start Menu\Programs\Startup","$env:APPDATA\Microsoft\Windows\Start Menu\Programs\Startup")){Get-ChildItem -Path $dir -EA SilentlyContinue|Where-Object{$_.Name -imatch $rx -or $_.FullName -imatch $rx}|ForEach-Object{$si+=[PSCustomObject]@{Source='StartupFolder';Name=$_.Name;Value=$_.FullName;Path=$dir}}};$si}catch{@()})}`+
			`;ConvertTo-Json -InputObject $r -Depth 6 -Compress`,
		crackToolIndicatorPattern)

	if batchOut, err := runWinRMCommand(client, psBatchCrack); err == nil && batchOut != "" {
		var batchResult map[string]json.RawMessage
		if json.Unmarshal([]byte(batchOut), &batchResult) == nil {

			// ACT – aktivasyon durumu
			if raw, ok := batchResult["ACT"]; ok {
				var records []map[string]interface{}
				json.Unmarshal(raw, &records)
				for _, rec := range records {
					name, _ := rec["Name"].(string)
					var statusInt int
					switch v := rec["LicenseStatus"].(type) {
					case float64:
						statusInt = int(v)
					case int:
						statusInt = v
					}
					severity, desc := licenseStatusText(statusInt)
					if severity != "" {
						findings = append(findings, CrackFinding{
							Category:    "activation",
							Severity:    severity,
							Title:       fmt.Sprintf("Lisans Durumu: %s", name),
							Description: desc,
							Evidence:    fmt.Sprintf("LicenseStatus=%d", statusInt),
						})
					}
				}
			}

			// OHOOK – sahte sppc.dll tespiti
			if raw, ok := batchResult["OHOOK"]; ok && string(raw) != "null" {
				var info map[string]interface{}
				if json.Unmarshal(raw, &info) == nil {
					path, _ := info["Path"].(string)
					status, _ := info["Status"].(string)
					subject, _ := info["Subject"].(string)
					thumbprint, _ := info["Thumbprint"].(string)
					isMicrosoftSigned := strings.Contains(strings.ToLower(subject), "microsoft")
					if !isMicrosoftSigned {
						evidence := fmt.Sprintf("Dosya: %s | İmza: %s", path, status)
						if subject != "" {
							evidence += fmt.Sprintf(" | Konu: %s", subject)
						}
						if thumbprint != "" && isCertBlocklisted(thumbprint) {
							evidence += " | CSCB: Kara listeli"
						}
						findings = append(findings, CrackFinding{
							Category:    "signature",
							Severity:    "high",
							Title:       "Ohook Office Aktivasyon Aracı Tespit Edildi",
							Description: "Office program klasöründe Microsoft tarafından imzalanmamış bir sppc.dll bulundu. Bu, MAS/Ohook C2R crack aracının imzasıdır — lisans doğrulamasını devre dışı bırakmak için Office DLL yükleme sırasını istismar eder.",
							Evidence:    evidence,
							Confidence:  97,
						})
					}
				}
			}

			// OFFICESIG – WINWORD.EXE imza kontrolü
			if raw, ok := batchResult["OFFICESIG"]; ok && string(raw) != "null" {
				var sigInfo map[string]interface{}
				if json.Unmarshal(raw, &sigInfo) == nil {
					status, _ := sigInfo["Status"].(string)
					path, _ := sigInfo["Path"].(string)
					subject, _ := sigInfo["Subject"].(string)
					thumbprint, _ := sigInfo["Thumbprint"].(string)
					if isCertBlocklisted(thumbprint) {
						findings = append(findings, CrackFinding{
							Category:    "signature",
							Severity:    "high",
							Title:       "Office Executable Kara Listeli Sertifikayla İmzalanmış",
							Description: "WINWORD.EXE imza sertifikası güvenlik kara listesinde kayıtlı.",
							Evidence:    fmt.Sprintf("Dosya: %s | Thumbprint: %s", path, thumbprint),
							Confidence:  98,
						})
					}
					if status == "HashMismatch" {
						findings = append(findings, CrackFinding{
							Category:    "signature",
							Severity:    "high",
							Title:       "Office Executable İmzası Geçersiz (Binary Patch Edilmiş)",
							Description: "WINWORD.EXE dijital imzası hash uyuşmazlığı gösteriyor — binary üzerine patch uygulanmış.",
							Evidence:    fmt.Sprintf("Dosya: %s | Durum: HashMismatch | Sertifika: %s", path, subject),
							Confidence:  99,
						})
					}
				}
			}

			// SVC – KMS bypass servisleri
			if raw, ok := batchResult["SVC"]; ok {
				var services []map[string]interface{}
				json.Unmarshal(raw, &services)
				for _, svc := range services {
					name, _ := svc["Name"].(string)
					display, _ := svc["DisplayName"].(string)
					pathName, _ := svc["PathName"].(string)
					sigStatus, _ := svc["SigStatus"].(string)
					sigSubject, _ := svc["SigSubject"].(string)
					sigThumbprint, _ := svc["SigThumbprint"].(string)
					sha256, _ := svc["SHA256"].(string)
					if name == "" {
						continue
					}
					isTrusted := sigStatus == "Valid" && isTrustedPublisher(sigSubject)
					if !isTrusted {
						evidence := fmt.Sprintf("Servis: %s (%s)", name, display)
						if pathName != "" {
							evidence += fmt.Sprintf(" | Path: %s", pathName)
						}
						if sigStatus != "" && sigStatus != "Unknown" {
							evidence += fmt.Sprintf(" | İmza: %s", sigStatus)
						}
						findings = append(findings, CrackFinding{
							Category:    "kms_tool",
							Severity:    "high",
							Title:       "KMS Bypass Servisi Tespit Edildi",
							Description: "Sistemde bilinen bir lisans kırma aracının servisi bulundu.",
							Evidence:    evidence,
						})
					}
					if isCertBlocklisted(sigThumbprint) {
						findings = append(findings, CrackFinding{
							Category:    "signature",
							Severity:    "high",
							Title:       "Servis Binary'si Kara Listeli Sertifikayla İmzalanmış",
							Description: "Bu servisin imza sertifikası MalwareBazaar Code Signing Certificate Blocklist'inde (CSCB) kayıtlı.",
							Evidence:    fmt.Sprintf("Servis: %s | Sertifika Thumbprint: %s", name, sigThumbprint),
						})
					}
					label := display
					if label == "" {
						label = name
					}
					findings = append(findings, threatIntelFindings(sha256, label, mbKey, otxKey, vtKey)...)
				}
			}

			// TASKS – KMS bypass zamanlanmış görevler
			if raw, ok := batchResult["TASKS"]; ok {
				var tasks []map[string]interface{}
				json.Unmarshal(raw, &tasks)
				for _, task := range tasks {
					taskName, _ := task["TaskName"].(string)
					taskPath, _ := task["TaskPath"].(string)
					if taskName == "" {
						continue
					}
					findings = append(findings, CrackFinding{
						Category:    "kms_tool",
						Severity:    "high",
						Title:       "KMS Bypass Zamanlanmış Görevi Tespit Edildi",
						Description: "Sistemde bilinen bir lisans kırma aracının kalıcı görevi mevcut.",
						Evidence:    fmt.Sprintf("Görev: %s%s", taskPath, taskName),
					})
				}
			}

			// INSTALLED – kurulu programlarda crack aracı
			if raw, ok := batchResult["INSTALLED"]; ok {
				var progs []map[string]interface{}
				json.Unmarshal(raw, &progs)
				for _, prog := range progs {
					name, _ := prog["DisplayName"].(string)
					publisher, _ := prog["Publisher"].(string)
					if name == "" || isTrustedPublisher(publisher) {
						continue
					}
					findings = append(findings, CrackFinding{
						Category:    "kms_tool",
						Severity:    "high",
						Title:       "Crack Aracı Kurulu Programlarda Listeleniyor",
						Description: "Sistemde bilinen bir lisans kırma aracı kurulu program olarak kaydedilmiş.",
						Evidence:    fmt.Sprintf("%s (Yayıncı: %s)", name, publisher),
					})
				}
			}

			// STARTUP – run key + startup folder kalıcılığı
			if raw, ok := batchResult["STARTUP"]; ok {
				var startItems []map[string]interface{}
				json.Unmarshal(raw, &startItems)
				for _, item := range startItems {
					name, _ := item["Name"].(string)
					value, _ := item["Value"].(string)
					path, _ := item["Path"].(string)
					source, _ := item["Source"].(string)
					if name == "" && value == "" {
						continue
					}
					findings = append(findings, CrackFinding{
						Category:    "persistence",
						Severity:    "high",
						Title:       "Crack Aracı Başlangıç Kalıcılığı Tespit Edildi",
						Description: "Bilinen lisans kırma aracına ait startup/run key kalıcılık izi bulundu.",
						Evidence:    fmt.Sprintf("%s: %s | Deger: %s | Konum: %s", source, name, value, path),
					})
				}
			}
		}
	}

	// ── 6. Dosya/klasör izleri (sınırlı derinlik) ─────────────────────────
	if !budgetExceeded() {
		psFileTraces := fmt.Sprintf(`$rx = '%s'
$roots = @()
foreach ($candidate in @($env:ProgramData, "$env:PUBLIC\Desktop", "$env:USERPROFILE\Desktop", "$env:USERPROFILE\Downloads", 'C:\SysTrackTest')) {
  if ($candidate -and (Test-Path -LiteralPath $candidate -ErrorAction SilentlyContinue)) { $roots += $candidate }
}
$out = @()
foreach ($root in $roots) {
  $out += Get-ChildItem -LiteralPath $root -Recurse -Force -ErrorAction SilentlyContinue |
    Where-Object { $_.Name -imatch $rx -or $_.FullName -imatch $rx } |
    Select-Object FullName, LastWriteTime, Length
}
$out = $out | Select-Object -First 40
ConvertTo-Json -InputObject @($out) -Compress`, crackToolIndicatorPattern)
		if out, err := runWinRMCommand(client, psFileTraces); err == nil && out != "" && out != "null" {
			var traces []map[string]interface{}
			if strings.HasPrefix(strings.TrimSpace(out), "[") {
				json.Unmarshal([]byte(out), &traces)
			} else {
				var trace map[string]interface{}
				if json.Unmarshal([]byte(out), &trace) == nil {
					traces = []map[string]interface{}{trace}
				}
			}
			for _, trace := range traces {
				fullName, _ := trace["FullName"].(string)
				if fullName == "" {
					continue
				}
				findings = append(findings, CrackFinding{
					Category:    "file_trace",
					Severity:    "medium",
					Title:       "Crack Aracı Dosya/Yol İzi Tespit Edildi",
					Description: "Disk üzerinde bilinen lisans kırma aracına benzeyen dosya veya klasör izi bulundu.",
					Evidence:    fullName,
				})
			}
		}
	}

	// ── 7. Defender istisnaları ────────────────────────────────────────────
	if !budgetExceeded() {
		psDefender := fmt.Sprintf(`$rx = '%s'
$pref = Get-MpPreference -ErrorAction SilentlyContinue
$items = @()
if ($pref) {
  @($pref.ExclusionPath) | Where-Object { $_ -and $_ -imatch $rx } | ForEach-Object {
    $items += [PSCustomObject]@{ Type='Path'; Value=$_ }
  }
  @($pref.ExclusionProcess) | Where-Object { $_ -and $_ -imatch $rx } | ForEach-Object {
    $items += [PSCustomObject]@{ Type='Process'; Value=$_ }
  }
  @($pref.ExclusionExtension) | Where-Object { $_ -and $_ -imatch $rx } | ForEach-Object {
    $items += [PSCustomObject]@{ Type='Extension'; Value=$_ }
  }
}
ConvertTo-Json -InputObject @($items) -Compress`, crackToolIndicatorPattern)
		if out, err := runWinRMCommand(client, psDefender); err == nil && out != "" && out != "null" {
			var items []map[string]interface{}
			if strings.HasPrefix(strings.TrimSpace(out), "[") {
				json.Unmarshal([]byte(out), &items)
			} else {
				var item map[string]interface{}
				if json.Unmarshal([]byte(out), &item) == nil {
					items = []map[string]interface{}{item}
				}
			}
			for _, item := range items {
				value, _ := item["Value"].(string)
				typ, _ := item["Type"].(string)
				if value == "" {
					continue
				}
				findings = append(findings, CrackFinding{
					Category:    "defender_exclusion",
					Severity:    "high",
					Title:       "Defender İstisnasında Crack Aracı İzi",
					Description: "Microsoft Defender istisnalarında bilinen lisans kırma aracına ait iz bulundu.",
					Evidence:    fmt.Sprintf("%s: %s", typ, value),
				})
			}
		}
	}

	// ── 8. KMS host meşruiyeti ─────────────────────────────────────────────
	// Yerel bir KMS emulator'a (KMSAuto/KMSpico/py-kms tipik davranışı) işaret eden veya
	// domain'in resmi olarak yayınlamadığı bir KMS host'una bağlanan makineler flaglenir.
	// Hiçbir manuel "güvenilir host" listesi yok: domain'in kendi DNS SRV kaydı
	// (_vlmcs._tcp.<domain> — Windows'un KMS otomatik bulma mekanizmasının ta kendisi)
	// referans alınır, böylece IT'nin hiçbir şey girmesine gerek kalmaz.
	if !budgetExceeded() {
		psKMSHost := `$sls = Get-CimInstance -ClassName SoftwareLicensingService -ErrorAction SilentlyContinue
$kmsHost = ''; $kmsPort = 0
if ($sls) {
  if ($sls.KeyManagementServiceMachine) { $kmsHost = $sls.KeyManagementServiceMachine; $kmsPort = $sls.KeyManagementServicePort }
  elseif ($sls.DiscoveredKeyManagementServiceMachineName) { $kmsHost = $sls.DiscoveredKeyManagementServiceMachineName; $kmsPort = $sls.DiscoveredKeyManagementServiceMachinePort }
}
$ips = @(Get-NetIPAddress -AddressFamily IPv4 -ErrorAction SilentlyContinue | Select-Object -ExpandProperty IPAddress)
$cs = Get-CimInstance -ClassName Win32_ComputerSystem -ErrorAction SilentlyContinue
$srvHosts = @()
if ($cs -and $cs.PartOfDomain -and $cs.Domain) {
  try {
    $srv = Resolve-DnsName -Name "_vlmcs._tcp.$($cs.Domain)" -Type SRV -ErrorAction SilentlyContinue
    if ($srv) { $srvHosts = @($srv | Where-Object { $_.NameTarget } | Select-Object -ExpandProperty NameTarget) }
  } catch {}
}
[PSCustomObject]@{ KMSHost=$kmsHost; KMSPort=$kmsPort; ComputerName=$env:COMPUTERNAME; IPs=$ips; SrvHosts=$srvHosts } | ConvertTo-Json -Compress`
		if out, err := runWinRMCommand(client, psKMSHost); err == nil && out != "" && out != "null" {
			var info struct {
				KMSHost      string      `json:"KMSHost"`
				ComputerName string      `json:"ComputerName"`
				IPs          interface{} `json:"IPs"`
				SrvHosts     interface{} `json:"SrvHosts"`
			}
			if json.Unmarshal([]byte(out), &info) == nil && info.KMSHost != "" {
				var ips []string
				for _, v := range interfaceToSlice(info.IPs) {
					if s, ok := v.(string); ok && s != "" {
						ips = append(ips, s)
					}
				}
				var srvHosts []string
				for _, v := range interfaceToSlice(info.SrvHosts) {
					if s, ok := v.(string); ok && s != "" {
						srvHosts = append(srvHosts, s)
					}
				}
				switch classifyKMSHost(info.KMSHost, info.ComputerName, ips, srvHosts) {
				case kmsHostLocalEmulator:
					findings = append(findings, CrackFinding{
						Category:    "kms_host",
						Severity:    "high",
						Title:       "Yerel KMS Sunucusu Tespit Edildi",
						Description: "Bu makine kendi üzerinde veya localhost'ta çalışan bir KMS sunucusundan aktivasyon alıyor — KMSAuto/KMSpico/py-kms gibi araçların tipik davranışı.",
						Evidence:    fmt.Sprintf("KMS Host: %s", info.KMSHost),
					})
				case kmsHostUnknown:
					findings = append(findings, CrackFinding{
						Category:    "kms_host",
						Severity:    "medium",
						Title:       "Tanınmayan KMS Sunucusu",
						Description: "Bu makine, domain'in DNS üzerinden resmi olarak yayınladığı KMS sunucusuyla eşleşmeyen bir host'tan aktivasyon alıyor. IT bu sunucuyu manuel doğrulamalı (DNS SRV kaydı yoksa veya host kasıtlı olarak GPO ile farklı atanmışsa normal olabilir).",
						Evidence:    fmt.Sprintf("KMS Host: %s", info.KMSHost),
					})
				}
			}
		}
	}

	// ── 9. Amcache — silinmiş/temizlenmiş crack araçlarının geçmiş izi ─────
	if !budgetExceeded() {
		psAmcache := fmt.Sprintf(`$rx = '%s'
$hiveName = "SysTrackAmcache_$PID"
$tempCopy = Join-Path $env:TEMP "$hiveName.hve"
$results = @()
try {
  $amcachePath = Join-Path $env:WINDIR 'AppCompat\Programs\Amcache.hve'
  if (Test-Path -LiteralPath $amcachePath -ErrorAction SilentlyContinue) {
    Copy-Item -LiteralPath $amcachePath -Destination $tempCopy -Force -ErrorAction Stop
    & reg.exe load "HKLM\$hiveName" "$tempCopy" 2>$null | Out-Null
    if (Test-Path "HKLM:\$hiveName\Root\InventoryApplicationFile" -ErrorAction SilentlyContinue) {
      Get-ChildItem -Path "HKLM:\$hiveName\Root\InventoryApplicationFile" -ErrorAction SilentlyContinue | ForEach-Object {
        $p = Get-ItemProperty -Path $_.PSPath -ErrorAction SilentlyContinue
        if ($p.LowerCaseLongPath -and ($p.LowerCaseLongPath -imatch $rx)) {
          $results += [PSCustomObject]@{ Path=$p.LowerCaseLongPath; Publisher=$p.Publisher }
        }
      }
    }
  }
} catch {
} finally {
  & reg.exe unload "HKLM\$hiveName" 2>$null | Out-Null
  Start-Sleep -Milliseconds 300
  if (Test-Path -LiteralPath $tempCopy -ErrorAction SilentlyContinue) { Remove-Item -LiteralPath $tempCopy -Force -ErrorAction SilentlyContinue }
}
$results = $results | Select-Object -First 25
ConvertTo-Json -InputObject @($results) -Compress`, crackToolIndicatorPattern)
		if out, err := runWinRMCommand(client, psAmcache); err == nil && out != "" && out != "null" {
			var entries []map[string]interface{}
			if strings.HasPrefix(strings.TrimSpace(out), "[") {
				json.Unmarshal([]byte(out), &entries)
			} else {
				var e map[string]interface{}
				if json.Unmarshal([]byte(out), &e) == nil {
					entries = []map[string]interface{}{e}
				}
			}
			for _, entry := range entries {
				path, _ := entry["Path"].(string)
				if path == "" {
					continue
				}
				findings = append(findings, CrackFinding{
					Category:    "historical_execution",
					Severity:    "medium",
					Title:       "Geçmişte Çalıştırılmış Crack Aracı İzi (Amcache)",
					Description: "Araç şu an sistemde aktif olmasa da Amcache kaydına göre bir zamanlar bu makinede çalıştırılmış.",
					Evidence:    fmt.Sprintf("Dosya: %s", path),
				})
			}
		}
	}

	// ── 10. Yakın zamanlı Event Log korelasyonu (temizlenmiş izler) ───────
	if !budgetExceeded() {
		psEventLog := fmt.Sprintf(`$rx = '%s'
$since = (Get-Date).AddDays(-30)
$results = @()
$logs = @(
  @{ Name='System'; Ids=@(7045) },
  @{ Name='Microsoft-Windows-TaskScheduler/Operational'; Ids=@(106,140) },
  @{ Name='Microsoft-Windows-Windows Defender/Operational'; Ids=@(5007) }
)
foreach ($log in $logs) {
  try {
    $events = Get-WinEvent -FilterHashtable @{ LogName=$log.Name; Id=$log.Ids; StartTime=$since } -MaxEvents 200 -ErrorAction SilentlyContinue
    foreach ($ev in $events) {
      $msg = $ev.Message
      if ($msg -and ($msg -imatch $rx)) {
        $snippet = $msg
        if ($snippet.Length -gt 200) { $snippet = $snippet.Substring(0,200) }
        $results += [PSCustomObject]@{ EventId=$ev.Id; LogName=$ev.LogName; TimeCreated=$ev.TimeCreated.ToString('o'); Message=$snippet }
      }
    }
  } catch {}
}
$results = $results | Select-Object -First 25
ConvertTo-Json -InputObject @($results) -Compress`, crackToolIndicatorPattern)
		if out, err := runWinRMCommand(client, psEventLog); err == nil && out != "" && out != "null" {
			var events []map[string]interface{}
			if strings.HasPrefix(strings.TrimSpace(out), "[") {
				json.Unmarshal([]byte(out), &events)
			} else {
				var e map[string]interface{}
				if json.Unmarshal([]byte(out), &e) == nil {
					events = []map[string]interface{}{e}
				}
			}
			for _, ev := range events {
				logName, _ := ev["LogName"].(string)
				timeCreated, _ := ev["TimeCreated"].(string)
				message, _ := ev["Message"].(string)
				var eventID int
				switch v := ev["EventId"].(type) {
				case float64:
					eventID = int(v)
				case int:
					eventID = v
				}
				if logName == "" {
					continue
				}
				findings = append(findings, CrackFinding{
					Category:    "event_log_trace",
					Severity:    "medium",
					Title:       "Event Log'da Crack Aracı İzi (Temizlenmiş Olabilir)",
					Description: "Servis/görev kurulumu veya Defender ayar değişikliği olay kaydında crack aracına ait bir iz bulundu — araç sonradan kaldırılmış olsa bile bu kayıt kalır.",
					Evidence:    fmt.Sprintf("EventID %d | %s | %s | %s", eventID, logName, timeCreated, message),
				})
			}
		}
	}

	// ── 11. Office binary — imza + CSCB + NSRL + MB + OTX ─────────────────
	if !budgetExceeded() {
		psOfficeFind := `$p = @('C:\Program Files\Microsoft Office\root\Office16\WINWORD.EXE','C:\Program Files (x86)\Microsoft Office\root\Office16\WINWORD.EXE','C:\Program Files\Microsoft Office\Office16\WINWORD.EXE','C:\Program Files (x86)\Microsoft Office\Office16\WINWORD.EXE') | Where-Object {Test-Path $_} | Select-Object -First 1; if ($p) { $s = Get-AuthenticodeSignature $p; [PSCustomObject]@{Path=$p;Status=$s.Status.ToString();Subject=if($s.SignerCertificate){$s.SignerCertificate.Subject}else{''};Thumbprint=if($s.SignerCertificate){$s.SignerCertificate.Thumbprint}else{''}} | ConvertTo-Json -Compress } else { 'null' }`
		if out, err := runWinRMCommand(client, psOfficeFind); err == nil && out != "" && out != "null" {
			var sigInfo map[string]interface{}
			if json.Unmarshal([]byte(out), &sigInfo) == nil {
				status, _ := sigInfo["Status"].(string)
				path, _ := sigInfo["Path"].(string)
				subject, _ := sigInfo["Subject"].(string)
				thumbprint, _ := sigInfo["Thumbprint"].(string)

				if isCertBlocklisted(thumbprint) {
					findings = append(findings, CrackFinding{
						Category:    "signature",
						Severity:    "high",
						Title:       "Office Executable Kara Listeli Sertifikayla İmzalanmış",
						Description: "WINWORD.EXE imza sertifikası MalwareBazaar Code Signing Certificate Blocklist'inde (CSCB) kayıtlı.",
						Evidence:    fmt.Sprintf("Dosya: %s | Sertifika Thumbprint: %s", path, thumbprint),
					})
				}

				switch status {
				case "HashMismatch":
					// Binary kesinlikle değiştirilmiş — yayıncıdan bağımsız flagle
					findings = append(findings, CrackFinding{
						Category:    "signature",
						Severity:    "high",
						Title:       "Office Executable İmzası Geçersiz (Hash Uyuşmazlığı)",
						Description: "WINWORD.EXE dijital imzası hash uyuşmazlığı gösteriyor — binary üzerine patch uygulanmış.",
						Evidence:    fmt.Sprintf("Dosya: %s | Durum: HashMismatch | Sertifika: %s", path, subject),
					})
					// MB ve OTX ile de sorgula — malware ailesini öğrenmek için
					psHash := fmt.Sprintf(`(Get-FileHash -Path '%s' -Algorithm SHA256).Hash`, path)
					if hashOut, err := runWinRMCommand(client, psHash); err == nil && len(strings.TrimSpace(hashOut)) == 64 {
						findings = append(findings, threatIntelFindings(strings.TrimSpace(hashOut), "WINWORD.EXE", mbKey, otxKey, vtKey)...)
					}

				case "NotSigned":
					// İmzasız — NSRL, MB ve OTX ile katmanlı kontrol (cache + paralel)
					psHash := fmt.Sprintf(`(Get-FileHash -Path '%s' -Algorithm SHA256).Hash`, path)
					if hashOut, err := runWinRMCommand(client, psHash); err == nil && len(strings.TrimSpace(hashOut)) == 64 {
						hash := strings.TrimSpace(hashOut)
						result := lookupThreatIntel(hash, mbKey, otxKey, vtKey, true)

						if result.mbFound {
							detail := result.mbSignature
							if detail == "" && len(result.mbTags) > 0 {
								detail = strings.Join(result.mbTags, ", ")
							}
							findings = append(findings, CrackFinding{
								Category:    "threat_intel",
								Severity:    "high",
								Title:       "Office Executable MalwareBazaar'da Tespit Edildi",
								Description: fmt.Sprintf("İmzasız WINWORD.EXE MalwareBazaar'da onaylı malware olarak kayıtlı. Aile: %s", detail),
								Evidence:    fmt.Sprintf("Dosya: %s | SHA256: %s", path, shortHash(hash)),
							})
						} else if result.nsrl == nsrlNotFound || result.otxPulseCount > 0 || result.vtMalicious > 0 {
							desc := "WINWORD.EXE imzasız"
							if result.nsrl == nsrlNotFound {
								desc += ", NIST NSRL'de kayıtlı değil"
							}
							if result.otxPulseCount > 0 {
								familyStr := ""
								if len(result.otxFamilies) > 0 {
									familyStr = fmt.Sprintf(" — Aile: %s", strings.Join(result.otxFamilies, ", "))
								}
								desc += fmt.Sprintf(", OTX'te %d kayıtta işaretli%s", result.otxPulseCount, familyStr)
							}
							if result.vtMalicious > 0 {
								desc += fmt.Sprintf(", VirusTotal'da %d motor tarafından zararlı işaretlendi", result.vtMalicious)
							}
							findings = append(findings, CrackFinding{
								Category:    "signature",
								Severity:    "medium",
								Title:       "Office Executable İmzasız ve Tehdit İstihbaratında Şüpheli",
								Description: desc + ".",
								Evidence:    fmt.Sprintf("Dosya: %s | SHA256: %s", path, shortHash(hash)),
							})
						}
					}
					// status == "Valid" → imza geçerli, flagleme
				}
			}
		}
	}

	// ── 12. Yerel lisans sunucusu emulator portları ───────────────────────
	// Bir uç noktanın bu portlarda dinlemesi için meşru bir sebep yoktur (bunlar yalnızca
	// kurumsal lisans HOST sunucularında dinlenir, sıradan bir iş istasyonunda asla):
	//   1688      → KMS (Windows/Office) — KMSAuto/KMSpico/py-kms
	//   2080      → Autodesk adskflex vendor daemon
	//   27000-27009 → FlexLM/FlexNet lmgrd + vendor daemon (Autodesk, MATLAB, ANSYS,
	//                 SolidWorks ve FlexLM kullanan onlarca mühendislik/CAD yazılımı)
	// Hangi ürün/yayıncı olduğuna bakılmaksızın bu portlardan biri yerelde dinleniyorsa
	// güçlü bir "yerel lisans emulator'ü çalışıyor" sinyalidir.
	if !budgetExceeded() {
		psLicensePorts := `$ports = @(1688,2080,27000,27001,27002,27003,27004,27005,27006,27007,27008,27009)
$conns = Get-NetTCPConnection -State Listen -ErrorAction SilentlyContinue | Where-Object { $ports -contains $_.LocalPort }
$out = $conns | ForEach-Object {
    $proc = Get-Process -Id $_.OwningProcess -ErrorAction SilentlyContinue
    [PSCustomObject]@{ LocalAddress=$_.LocalAddress; LocalPort=$_.LocalPort; ProcessName=$(if($proc){$proc.ProcessName}else{''}); ProcessPath=$(if($proc){$proc.Path}else{''}) }
}
ConvertTo-Json -InputObject @($out) -Compress`
		if out, err := runWinRMCommand(client, psLicensePorts); err == nil && out != "" && out != "null" {
			var listeners []map[string]interface{}
			if strings.HasPrefix(strings.TrimSpace(out), "[") {
				json.Unmarshal([]byte(out), &listeners)
			} else {
				var l map[string]interface{}
				if json.Unmarshal([]byte(out), &l) == nil {
					listeners = []map[string]interface{}{l}
				}
			}
			for _, l := range listeners {
				procName, _ := l["ProcessName"].(string)
				procPath, _ := l["ProcessPath"].(string)
				localAddr, _ := l["LocalAddress"].(string)
				var localPort int
				switch v := l["LocalPort"].(type) {
				case float64:
					localPort = int(v)
				case int:
					localPort = v
				}
				evidence := fmt.Sprintf("Local: %s:%d", localAddr, localPort)
				if procName != "" {
					evidence += fmt.Sprintf(" | İşlem: %s", procName)
				}
				if procPath != "" {
					evidence += fmt.Sprintf(" | Yol: %s", procPath)
				}

				category, title, description := "kms_host", "Yerel KMS Emulator Portu (1688) Dinleniyor", "Bu makinede TCP 1688 portunda bir işlem dinleme yapıyor — uç nokta cihazlarında bu yalnızca yerel bir KMS emulator (KMSAuto/KMSpico/py-kms vb.) tarafından açıklanabilir."
				if localPort != 1688 {
					category = "license_server"
					title = fmt.Sprintf("Yerel Lisans Sunucusu Emulator Portu (%d) Dinleniyor", localPort)
					description = "Bu makinede FlexLM/FlexNet lisans sunucusu portlarından biri dinleniyor (Autodesk/MATLAB/ANSYS/SolidWorks vb. mühendislik yazılımlarının lisans kontrolü için kullanılır) — bir uç nokta cihazının kendisi lisans HOST'u olamaz, bu yerel bir lisans emulator/crack aracına işaret eder."
				}
				findings = append(findings, CrackFinding{
					Category:    category,
					Severity:    "high",
					Title:       title,
					Description: description,
					Evidence:    evidence,
				})
			}
		}
	}

	// ── 13. Hosts dosyası tahrifatı — Microsoft/Adobe lisans doğrulama domainleri ──
	// Crack araçları sıklıkla bu domainleri loopback'e yönlendirip aktivasyon/lisans
	// sunucusuna hiç ulaşılamamasını sağlar (offline-gibi davranıp "genuine" gösterir).
	if !budgetExceeded() {
		psHosts := fmt.Sprintf(`$blocked = @(%s)
$hostsPath = Join-Path $env:WINDIR 'System32\drivers\etc\hosts'
$out = @()
if (Test-Path -LiteralPath $hostsPath -ErrorAction SilentlyContinue) {
  Get-Content -LiteralPath $hostsPath -ErrorAction SilentlyContinue | ForEach-Object {
    $line = $_.Trim()
    if ($line -and -not $line.StartsWith('#')) {
      $parts = $line -split '\s+'
      if ($parts.Count -ge 2 -and @('127.0.0.1','0.0.0.0','::1') -contains $parts[0]) {
        for ($i=1; $i -lt $parts.Count; $i++) {
          $h = $parts[$i].ToLower().TrimEnd('.')
          if ($blocked -contains $h) {
            $out += [PSCustomObject]@{ IP=$parts[0]; Domain=$h }
          }
        }
      }
    }
  }
}
ConvertTo-Json -InputObject @($out) -Compress`, licenseValidationDomainsPSArray())
		if out, err := runWinRMCommand(client, psHosts); err == nil && out != "" && out != "null" {
			var entries []map[string]interface{}
			if strings.HasPrefix(strings.TrimSpace(out), "[") {
				json.Unmarshal([]byte(out), &entries)
			} else {
				var e map[string]interface{}
				if json.Unmarshal([]byte(out), &e) == nil {
					entries = []map[string]interface{}{e}
				}
			}
			for _, e := range entries {
				domain, _ := e["Domain"].(string)
				ip, _ := e["IP"].(string)
				if domain == "" {
					continue
				}
				findings = append(findings, CrackFinding{
					Category:    "hosts_tampering",
					Severity:    "high",
					Title:       "Hosts Dosyasında Lisans Doğrulama Engeli",
					Description: "Resmi Microsoft/Adobe aktivasyon-doğrulama domaini hosts dosyasında loopback adresine yönlendirilmiş — lisans sunucusuna erişimi kasıtlı olarak engelleyen tipik bir crack tekniği.",
					Evidence:    fmt.Sprintf("%s -> %s", domain, ip),
				})
			}
		}
	}

	// ── 14. Çalışan işlem taraması ─────────────────────────────────────────
	// Servis/görev olarak kayıtlı olmayan, doğrudan arka planda çalışan crack araçlarını
	// yakalar (örn. kullanıcı oturumunda elle başlatılmış bir aktivasyon aracı).
	if !budgetExceeded() {
		psProcesses := fmt.Sprintf(`$rx = '%s'
Get-CimInstance Win32_Process | Where-Object {
    $_.Name -imatch $rx -or
    ($_.ExecutablePath -and $_.ExecutablePath -imatch $rx) -or
    ($_.CommandLine -and $_.CommandLine -imatch $rx)
} | Select-Object ProcessId, Name, ExecutablePath | ConvertTo-Json -Compress`, crackToolIndicatorPattern)
		if out, err := runWinRMCommand(client, psProcesses); err == nil && out != "" && out != "null" {
			var procs []map[string]interface{}
			if strings.HasPrefix(strings.TrimSpace(out), "[") {
				json.Unmarshal([]byte(out), &procs)
			} else {
				var p map[string]interface{}
				if json.Unmarshal([]byte(out), &p) == nil {
					procs = []map[string]interface{}{p}
				}
			}
			for _, p := range procs {
				name, _ := p["Name"].(string)
				execPath, _ := p["ExecutablePath"].(string)
				if name == "" {
					continue
				}
				evidence := fmt.Sprintf("İşlem: %s", name)
				if execPath != "" {
					evidence += fmt.Sprintf(" | Yol: %s", execPath)
				}
				findings = append(findings, CrackFinding{
					Category:    "kms_tool",
					Severity:    "high",
					Title:       "Crack Aracı Aktif Olarak Çalışıyor",
					Description: "Bilinen bir lisans kırma aracına ait işlem şu anda bellekte/çalışır durumda.",
					Evidence:    evidence,
				})
			}
		}
	}

	// ── 15-16. İsimden bağımsız genel sezgisel: imzasız + standart dışı konumlu
	// servis/görev kalıcılığı ────────────────────────────────────────────────
	// Yukarıdaki bloklar bilinen araç adlarına dayanır (KMSPico, TSforge, vb.). Bu blok
	// isim listesine hiç bakmaz: Program Files/Windows/ProgramData dışında, geçerli bir
	// dijital imzası olmayan HER servis/görevi flagler — adı değiştirilmiş veya hiç
	// bilinmeyen bir crack aracını da yakalayabilmek için. Orta güven/önem: yanlış
	// pozitif riski named-match'lerden yüksektir (örn. imzasız bir kurumsal iç araç da
	// tetikleyebilir), bu yüzden manuel doğrulama öneren ayrı bir kategori.
	const trustedDirsPS = `'C:\Windows\','C:\Program Files\','C:\Program Files (x86)\','C:\ProgramData\Microsoft\'`

	if !budgetExceeded() {
		psUnsignedServices := `$trustedDirs = @(` + trustedDirsPS + `)
$out = @()
Get-WmiObject Win32_Service | Where-Object { $_.PathName } | ForEach-Object {
    $raw = $_.PathName
    $exe = if ($raw.StartsWith('"')) { ($raw -replace '^"([^"]+)".*','$1') } else { ($raw -split ' ')[0] }
    if (-not $exe) { return }
    foreach ($d in $trustedDirs) { if ($exe.StartsWith($d, [StringComparison]::OrdinalIgnoreCase)) { return } }
    if (-not (Test-Path -LiteralPath $exe -ErrorAction SilentlyContinue)) { return }
    $sig = Get-AuthenticodeSignature -LiteralPath $exe -ErrorAction SilentlyContinue
    if ($sig -and $sig.Status.ToString() -eq 'Valid') { return }
    $sha256 = (Get-FileHash -LiteralPath $exe -Algorithm SHA256 -ErrorAction SilentlyContinue).Hash
    $out += [PSCustomObject]@{ Name=$_.Name; DisplayName=$_.DisplayName; PathName=$raw; SigStatus=$(if($sig){$sig.Status.ToString()}else{'Unknown'}); SHA256=$sha256 }
}
ConvertTo-Json -InputObject @($out) -Compress`
		if out, err := runWinRMCommand(client, psUnsignedServices); err == nil && out != "" && out != "null" {
			var items []map[string]interface{}
			if strings.HasPrefix(strings.TrimSpace(out), "[") {
				json.Unmarshal([]byte(out), &items)
			} else {
				var it map[string]interface{}
				if json.Unmarshal([]byte(out), &it) == nil {
					items = []map[string]interface{}{it}
				}
			}
			for _, it := range items {
				name, _ := it["Name"].(string)
				display, _ := it["DisplayName"].(string)
				pathName, _ := it["PathName"].(string)
				sigStatus, _ := it["SigStatus"].(string)
				sha256, _ := it["SHA256"].(string)
				if name == "" {
					continue
				}
				findings = append(findings, CrackFinding{
					Category:    "unverified_persistence",
					Severity:    "medium",
					Title:       "Standart Dışı Konumdan İmzasız Servis",
					Description: "Bu servis Program Files/Windows dışında bir konumdan çalışıyor ve geçerli bir dijital imzası yok. Bilinen bir crack aracı eşleşmesi değil, ama manuel doğrulama gerektiren genel bir şüpheli kalıcılık izi.",
					Evidence:    fmt.Sprintf("Servis: %s (%s) | Path: %s | İmza: %s", name, display, pathName, sigStatus),
				})
				// Zayıf sezgisel bulguyu hash tabanlı tehdit istihbaratıyla çapraz kontrol et —
				// herhangi bir kaynak onaylarsa bulgu threat_intel/high'a yükselir.
				label := display
				if label == "" {
					label = name
				}
				findings = append(findings, threatIntelFindings(sha256, label, mbKey, otxKey, vtKey)...)
			}
		}
	}

	if !budgetExceeded() {
		psUnsignedTasks := `$trustedDirs = @(` + trustedDirsPS + `)
$out = @()
Get-ScheduledTask | Where-Object { $_.TaskPath -notlike '\Microsoft\*' } | ForEach-Object {
    $task = $_
    foreach ($action in $task.Actions) {
        $exe = $action.Execute
        if (-not $exe) { continue }
        $trusted = $false
        foreach ($d in $trustedDirs) { if ($exe.StartsWith($d, [StringComparison]::OrdinalIgnoreCase)) { $trusted = $true; break } }
        if ($trusted) { continue }
        $resolved = $exe
        if (-not (Test-Path -LiteralPath $resolved -ErrorAction SilentlyContinue)) {
            $cmd = Get-Command $exe -ErrorAction SilentlyContinue
            if ($cmd) { $resolved = $cmd.Source } else { continue }
        }
        $trustedResolved = $false
        foreach ($d in $trustedDirs) { if ($resolved.StartsWith($d, [StringComparison]::OrdinalIgnoreCase)) { $trustedResolved = $true; break } }
        if ($trustedResolved) { continue }
        $sig = Get-AuthenticodeSignature -LiteralPath $resolved -ErrorAction SilentlyContinue
        if ($sig -and $sig.Status.ToString() -eq 'Valid') { continue }
        $sha256 = (Get-FileHash -LiteralPath $resolved -Algorithm SHA256 -ErrorAction SilentlyContinue).Hash
        $out += [PSCustomObject]@{ TaskName=$task.TaskName; TaskPath=$task.TaskPath; Execute=$exe; SigStatus=$(if($sig){$sig.Status.ToString()}else{'Unknown'}); SHA256=$sha256 }
    }
}
ConvertTo-Json -InputObject @($out) -Compress`
		if out, err := runWinRMCommand(client, psUnsignedTasks); err == nil && out != "" && out != "null" {
			var items []map[string]interface{}
			if strings.HasPrefix(strings.TrimSpace(out), "[") {
				json.Unmarshal([]byte(out), &items)
			} else {
				var it map[string]interface{}
				if json.Unmarshal([]byte(out), &it) == nil {
					items = []map[string]interface{}{it}
				}
			}
			for _, it := range items {
				taskName, _ := it["TaskName"].(string)
				taskPath, _ := it["TaskPath"].(string)
				execute, _ := it["Execute"].(string)
				sigStatus, _ := it["SigStatus"].(string)
				sha256, _ := it["SHA256"].(string)
				if taskName == "" {
					continue
				}
				findings = append(findings, CrackFinding{
					Category:    "unverified_persistence",
					Severity:    "medium",
					Title:       "Standart Dışı Konumdan İmzasız Zamanlanmış Görev",
					Description: "Bu görev Program Files/Windows dışında bir konumdaki, geçerli imzası olmayan bir çalıştırılabilir dosyayı tetikliyor. Bilinen bir crack aracı eşleşmesi değil, ama manuel doğrulama gerektiren genel bir şüpheli kalıcılık izi.",
					Evidence:    fmt.Sprintf("Görev: %s%s | Çalıştırılan: %s | İmza: %s", taskPath, taskName, execute, sigStatus),
				})
				findings = append(findings, threatIntelFindings(sha256, taskName, mbKey, otxKey, vtKey)...)
			}
		}
	}

	// ── 16b. İsimden bağımsız genel sezgisel: imzasız + standart dışı konumlu
	// ÇALIŞAN İŞLEM ────────────────────────────────────────────────────────
	// Servis/görev olarak hiç kayıtlı olmayan (kullanıcı elle başlattığı, veya tek seferlik
	// bir loader/patcher/keygen) ama şu an bellekte çalışan, imzasız + standart dışı
	// konumlu HER işlemi yakalar. Block 14'teki isim-eşleşmeli process taramasından farklı
	// olarak burada hiçbir indikatör listesine bakılmaz.
	if !budgetExceeded() {
		psUnsignedProcesses := `$trustedDirs = @(` + trustedDirsPS + `)
$out = @()
Get-CimInstance Win32_Process | Where-Object { $_.ExecutablePath } | ForEach-Object {
    $exe = $_.ExecutablePath
    $trusted = $false
    foreach ($d in $trustedDirs) { if ($exe.StartsWith($d, [StringComparison]::OrdinalIgnoreCase)) { $trusted = $true; break } }
    if ($trusted) { return }
    $sig = Get-AuthenticodeSignature -LiteralPath $exe -ErrorAction SilentlyContinue
    if ($sig -and $sig.Status.ToString() -eq 'Valid') { return }
    $sha256 = (Get-FileHash -LiteralPath $exe -Algorithm SHA256 -ErrorAction SilentlyContinue).Hash
    $out += [PSCustomObject]@{ Name=$_.Name; ExecutablePath=$exe; SigStatus=$(if($sig){$sig.Status.ToString()}else{'Unknown'}); SHA256=$sha256 }
}
ConvertTo-Json -InputObject @($out) -Compress`
		if out, err := runWinRMCommand(client, psUnsignedProcesses); err == nil && out != "" && out != "null" {
			var items []map[string]interface{}
			if strings.HasPrefix(strings.TrimSpace(out), "[") {
				json.Unmarshal([]byte(out), &items)
			} else {
				var it map[string]interface{}
				if json.Unmarshal([]byte(out), &it) == nil {
					items = []map[string]interface{}{it}
				}
			}
			for _, it := range items {
				name, _ := it["Name"].(string)
				execPath, _ := it["ExecutablePath"].(string)
				sigStatus, _ := it["SigStatus"].(string)
				sha256, _ := it["SHA256"].(string)
				if name == "" {
					continue
				}
				findings = append(findings, CrackFinding{
					Category:    "unverified_persistence",
					Severity:    "medium",
					Title:       "Standart Dışı Konumdan İmzasız Çalışan İşlem",
					Description: "Bu işlem Program Files/Windows dışında bir konumdan çalışıyor ve geçerli bir dijital imzası yok. Servis/görev olarak kayıtlı değil — elle başlatılmış bir loader/patcher/keygen olabilir. Bilinen bir crack aracı eşleşmesi değil, ama manuel doğrulama gerektiren genel bir şüpheli iz.",
					Evidence:    fmt.Sprintf("İşlem: %s | Yol: %s | İmza: %s", name, execPath, sigStatus),
				})
				findings = append(findings, threatIntelFindings(sha256, name, mbKey, otxKey, vtKey)...)
			}
		}
	}

	// ── 17. Adobe lisans dosyası (amtlib.dll) bütünlük kontrolü ───────────
	// Photoshop, Illustrator, Premiere Pro, After Effects, InDesign vb. TÜM Adobe
	// Creative Cloud uygulamaları lisans/aktivasyon kontrolünü amtlib.dll üzerinden yapar.
	// Bu dosyanın crack'lenmesi (değiştirilmesi) Adobe uygulamalarını "kırmanın" en yaygın
	// yöntemidir — orijinali Adobe tarafından imzalıdır, değiştirilmiş kopya imzasız/geçersiz
	// olur. Hangi Adobe uygulamasının kurulu olduğuna bakılmaksızın (isim listesi gerekmez)
	// Program Files altında joker karakterli arama yapılır.
	if !budgetExceeded() {
		psAmtlib := `$files = @()
foreach ($root in @('C:\Program Files\Adobe','C:\Program Files (x86)\Adobe')) {
  if (Test-Path -LiteralPath $root -ErrorAction SilentlyContinue) {
    $files += Get-ChildItem -LiteralPath $root -Filter 'amtlib.dll' -Recurse -Depth 4 -Force -ErrorAction SilentlyContinue
  }
}
$out = $files | Select-Object -First 15 | ForEach-Object {
    $sig = Get-AuthenticodeSignature -LiteralPath $_.FullName -ErrorAction SilentlyContinue
    $sha256 = (Get-FileHash -LiteralPath $_.FullName -Algorithm SHA256 -ErrorAction SilentlyContinue).Hash
    [PSCustomObject]@{
        Path=$_.FullName
        Status=$(if($sig){$sig.Status.ToString()}else{'Unknown'})
        Subject=$(if($sig -and $sig.SignerCertificate){$sig.SignerCertificate.Subject}else{''})
        SHA256=$sha256
    }
}
ConvertTo-Json -InputObject @($out) -Compress`
		if out, err := runWinRMCommand(client, psAmtlib); err == nil && out != "" && out != "null" {
			var files []map[string]interface{}
			if strings.HasPrefix(strings.TrimSpace(out), "[") {
				json.Unmarshal([]byte(out), &files)
			} else {
				var f map[string]interface{}
				if json.Unmarshal([]byte(out), &f) == nil {
					files = []map[string]interface{}{f}
				}
			}
			for _, f := range files {
				path, _ := f["Path"].(string)
				status, _ := f["Status"].(string)
				subject, _ := f["Subject"].(string)
				sha256, _ := f["SHA256"].(string)
				if path == "" {
					continue
				}
				// amtlib.dll'nin orijinali her zaman geçerli şekilde imzalıdır; bu yüzden
				// yayıncıdan bağımsız olarak Valid dışındaki her durum flaglenir.
				if status != "Valid" {
					findings = append(findings, CrackFinding{
						Category:    "signature",
						Severity:    "high",
						Title:       "Adobe Lisans Dosyası Değiştirilmiş (amtlib.dll)",
						Description: "Bu Adobe uygulamasının lisans/aktivasyon dosyası (amtlib.dll) imzasız veya geçersiz imzalı — orijinal Adobe dosyaları her zaman geçerli şekilde imzalıdır. Bu, Photoshop/Illustrator/Premiere gibi Adobe Creative Cloud uygulamalarını kırmak için en yaygın kullanılan tekniktir.",
						Evidence:    fmt.Sprintf("Dosya: %s | Durum: %s | Sertifika: %s", path, status, subject),
					})
					findings = append(findings, threatIntelFindings(sha256, "amtlib.dll", mbKey, otxKey, vtKey)...)
				}
			}
		}
	}

	// ── 17b. Kurulu HER programın ana exe'sinin imza bütünlüğü ────────────
	// amtlib.dll/WINWORD.EXE kontrolleri sadece Adobe/Office'i kapsar. Çoğu ticari
	// yazılım (FL Studio, Autodesk, JetBrains, vb.) crack'lenirken KURULUM KONUMU
	// değişmez — sadece ana çalıştırılabilir dosya yamalanır. Bu yüzden konum bazlı
	// (Program Files dışı) kontroller bunları kaçırır. Bu blok isim listesine hiç
	// bakmadan, HER kurulu programın (registry DisplayIcon/InstallLocation üzerinden
	// bulunan) ana exe'sinin imzasını kontrol eder: orijinali geçerli imzalı olması
	// beklenen bir programın imzası geçersiz/eksikse (özellikle HashMismatch — bir
	// zamanlar geçerli imzalanmış ama içeriği sonradan değiştirilmiş anlamına gelir)
	// flaglenir.
	if !budgetExceeded() {
		psProgramIntegrity := `$rxSkip = 'KB[0-9]{6,}|Update for |Security Update|Redistributable|Update Health|Hotfix|Microsoft Edge|WebView2|Visual C\+\+|\.NET (Core|Runtime|Framework)|Windows Driver|Survey'
$entries = @()
$entries += Get-ItemProperty 'HKLM:\Software\Microsoft\Windows\CurrentVersion\Uninstall\*' -ErrorAction SilentlyContinue
$entries += Get-ItemProperty 'HKLM:\Software\Wow6432Node\Microsoft\Windows\CurrentVersion\Uninstall\*' -ErrorAction SilentlyContinue
$entries += Get-ItemProperty 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Uninstall\*' -ErrorAction SilentlyContinue
$out = @()
foreach ($e in $entries) {
    if (-not $e.DisplayName -or $e.DisplayName -imatch $rxSkip) { continue }
    $exe = $null
    if ($e.DisplayIcon) {
        $candidate = ($e.DisplayIcon -split ',')[0].Trim('"',' ')
        if ($candidate -and $candidate.ToLower().EndsWith('.exe') -and (Test-Path -LiteralPath $candidate -ErrorAction SilentlyContinue)) {
            $exe = $candidate
        }
    }
    if (-not $exe -and $e.InstallLocation -and (Test-Path -LiteralPath $e.InstallLocation -ErrorAction SilentlyContinue)) {
        $cand = Get-ChildItem -LiteralPath $e.InstallLocation -File -Filter '*.exe' -ErrorAction SilentlyContinue |
            Where-Object { $_.Name -notmatch 'uninst|setup|launcher|update|helper|crash' } |
            Sort-Object Length -Descending | Select-Object -First 1
        if ($cand) { $exe = $cand.FullName }
    }
    if (-not $exe -and $e.Publisher) {
        # DisplayIcon bir .ico'ya isaret edebilir veya InstallLocation hic set edilmemis olabilir
        # (FL Studio dahil bircok gercek kurulumda boyle) - cogu yukleyici varsayilan olarak
        # 'Program Files\<Yayinci>\<Urun Adi>\' kullanir, bunu dene.
        foreach ($pf in @('C:\Program Files', 'C:\Program Files (x86)')) {
            foreach ($sub in @("$($e.Publisher)\$($e.DisplayName)", "$($e.Publisher)")) {
                $dir = Join-Path $pf $sub
                if ($exe) { break }
                if (Test-Path -LiteralPath $dir -ErrorAction SilentlyContinue) {
                    $cand = Get-ChildItem -LiteralPath $dir -File -Filter '*.exe' -ErrorAction SilentlyContinue |
                        Where-Object { $_.Name -notmatch 'uninst|setup|launcher|update|helper|crash' } |
                        Sort-Object Length -Descending | Select-Object -First 1
                    if ($cand) { $exe = $cand.FullName }
                }
            }
            if ($exe) { break }
        }
    }
    if (-not $exe -or $exe -imatch 'winword\.exe$') { continue }
    $sig = Get-AuthenticodeSignature -LiteralPath $exe -ErrorAction SilentlyContinue
    $status = if ($sig) { $sig.Status.ToString() } else { 'Unknown' }
    if ($status -eq 'Valid') { continue }
    $sha256 = (Get-FileHash -LiteralPath $exe -Algorithm SHA256 -ErrorAction SilentlyContinue).Hash
    $out += [PSCustomObject]@{ DisplayName=$e.DisplayName; Publisher=$e.Publisher; Exe=$exe; Status=$status; SHA256=$sha256 }
}
$out = $out | Select-Object -First 25
ConvertTo-Json -InputObject @($out) -Compress`
		if out, err := runWinRMCommand(client, psProgramIntegrity); err == nil && out != "" && out != "null" {
			var items []map[string]interface{}
			if strings.HasPrefix(strings.TrimSpace(out), "[") {
				json.Unmarshal([]byte(out), &items)
			} else {
				var it map[string]interface{}
				if json.Unmarshal([]byte(out), &it) == nil {
					items = []map[string]interface{}{it}
				}
			}
			for _, it := range items {
				displayName, _ := it["DisplayName"].(string)
				publisher, _ := it["Publisher"].(string)
				exe, _ := it["Exe"].(string)
				status, _ := it["Status"].(string)
				sha256, _ := it["SHA256"].(string)
				if displayName == "" || exe == "" {
					continue
				}
				severity := "medium"
				statusDesc := "imzasız"
				if status == "HashMismatch" {
					severity = "high"
					statusDesc = "geçerli imzalıyken sonradan değiştirilmiş (hash uyuşmazlığı)"
				}
				findings = append(findings, CrackFinding{
					Category:    "signature",
					Severity:    severity,
					Title:       fmt.Sprintf("Kurulu Program İmzası Geçersiz: %s", displayName),
					Description: fmt.Sprintf("Bu programın ana çalıştırılabilir dosyası %s. Ticari yazılımlar normalde üretici tarafından geçerli şekilde imzalanır; bu durum dosyanın kurulumdan sonra yamalanmış (crack'lenmiş) olabileceğine işaret eder.", statusDesc),
					Evidence:    fmt.Sprintf("Program: %s (%s) | Dosya: %s | Durum: %s", displayName, publisher, exe, status),
				})
				findings = append(findings, threatIntelFindings(sha256, displayName, mbKey, otxKey, vtKey)...)
			}
		}
	}

	// ── 18. Windows Defender'ın kendi motorunu tetikle ve sonucu oku ──────
	// Bu, hiçbir harici API/isim listesi gerektirmeyen en genel sinyaldir: Defender'ın
	// kendi imza/heuristik veritabanı (Microsoft'un bildiği HER crack/PUA aracını kapsar)
	// kullanıcının en sık crack/kurulum dosyası bıraktığı klasörlere karşı tetiklenir.
	// Gerçek zamanlı koruma bu klasörleri zaten taramış olabilir, ama (a) Defender
	// istisnası varsa (bkz. defender_exclusion) gerçek zamanlı koruma o dosyaya hiç
	// bakmaz, (b) PUA koruması özel olarak etkin değilse çoğu "HackTool" sınıfı crack
	// aracı varsayılan taramada atlanabilir — bu yüzden hedefli bir on-demand tarama hâlâ
	// ek değer taşır.
	if !budgetExceeded() {
		psDefenderScan := `$scanStart = Get-Date
$paths = @("$env:PUBLIC\Desktop", "$env:USERPROFILE\Desktop", "$env:USERPROFILE\Downloads", 'C:\SysTrackTest')
foreach ($p in $paths) {
  if ($p -and (Test-Path -LiteralPath $p -ErrorAction SilentlyContinue)) {
    try { Start-MpScan -ScanType CustomScan -ScanPath $p -ErrorAction SilentlyContinue } catch {}
  }
}
$detections = Get-MpThreatDetection -ErrorAction SilentlyContinue | Where-Object { $_.InitialDetectionTime -ge $scanStart }
$out = $detections | ForEach-Object {
    $threat = Get-MpThreat -ThreatID $_.ThreatID -ErrorAction SilentlyContinue
    [PSCustomObject]@{
        ThreatName = $(if($threat -and $threat.ThreatName){$threat.ThreatName}else{'Bilinmeyen'})
        Resources = ($_.Resources -join '; ')
        ProcessName = $_.ProcessName
        DetectionTime = $_.InitialDetectionTime.ToString('o')
    }
}
ConvertTo-Json -InputObject @($out) -Compress`
		if out, err := runWinRMCommand(client, psDefenderScan); err == nil && out != "" && out != "null" {
			var detections []map[string]interface{}
			if strings.HasPrefix(strings.TrimSpace(out), "[") {
				json.Unmarshal([]byte(out), &detections)
			} else {
				var d map[string]interface{}
				if json.Unmarshal([]byte(out), &d) == nil {
					detections = []map[string]interface{}{d}
				}
			}
			for _, d := range detections {
				threatName, _ := d["ThreatName"].(string)
				resources, _ := d["Resources"].(string)
				detectionTime, _ := d["DetectionTime"].(string)
				if threatName == "" {
					continue
				}
				findings = append(findings, CrackFinding{
					Category:    "threat_intel",
					Severity:    "high",
					Title:       fmt.Sprintf("Windows Defender Tespiti: %s", threatName),
					Description: "Microsoft Defender'ın kendi motoru bu makinede bir tehdit/istenmeyen yazılım (PUA) tespit etti. Bu, herhangi bir harici API'ye bağlı olmayan, doğrudan Microsoft'un imza veritabanından gelen bir doğrulamadır.",
					Evidence:    fmt.Sprintf("Tehdit: %s | Kaynak: %s | Tespit Zamanı: %s", threatName, resources, detectionTime),
				})
			}
		}
	}

	// ── 19. JetBrains crack: ja-netfilter / jetbrains-agent tespiti ─────────
	// JetBrains IDE'leri (IntelliJ, PyCharm, WebStorm, GoLand, Rider vb.) için
	// en yaygın crack yöntemi ana exe'yi yamalamamak, bunun yerine .vmoptions
	// dosyasına -javaagent: satırı ekleyerek bir Java proxy ajanı enjekte etmektir.
	// Bu yöntem imza kontrolünden (psProgramIntegrity) kaçar; ayrı bir blok gerektirir.
	if !budgetExceeded() {
		psJetBrains := `$found = @()
$pfDirs = @('C:\Program Files\JetBrains','C:\Program Files (x86)\JetBrains')
foreach ($pf in $pfDirs) {
    if (Test-Path -LiteralPath $pf -ErrorAction SilentlyContinue) {
        Get-ChildItem -LiteralPath $pf -Recurse -Depth 5 -Directory -ErrorAction SilentlyContinue |
            Where-Object { $_.Name -imatch 'ja-?netfilter|janetfilter|netfilter[-_]proxy' } |
            ForEach-Object { $found += [PSCustomObject]@{Path=$_.FullName; Type='ja-netfilter-dir'; Detail=''} }
    }
}
$jbDir = Join-Path $env:APPDATA 'JetBrains'
if (Test-Path -LiteralPath $jbDir -ErrorAction SilentlyContinue) {
    Get-ChildItem -LiteralPath $jbDir -Recurse -Depth 3 -Filter '*.vmoptions' -ErrorAction SilentlyContinue |
        ForEach-Object {
            $content = Get-Content -LiteralPath $_.FullName -ErrorAction SilentlyContinue
            $crackLine = $content | Where-Object { $_ -imatch '-javaagent:.*(?:ja-?netfilter|jetbrains-?agent|janetfilter)' } | Select-Object -First 1
            if ($crackLine) {
                $found += [PSCustomObject]@{Path=$_.FullName; Type='vmoptions-crack-agent'; Detail=$crackLine.Trim()}
            }
        }
}
ConvertTo-Json -InputObject @($found | Select-Object -First 20) -Compress`
		if out, err := runWinRMCommand(client, psJetBrains); err == nil && out != "" && out != "null" {
			var items []map[string]interface{}
			if strings.HasPrefix(strings.TrimSpace(out), "[") {
				json.Unmarshal([]byte(out), &items)
			} else {
				var it map[string]interface{}
				if json.Unmarshal([]byte(out), &it) == nil {
					items = []map[string]interface{}{it}
				}
			}
			for _, it := range items {
				path, _ := it["Path"].(string)
				typ, _ := it["Type"].(string)
				detail, _ := it["Detail"].(string)
				if path == "" {
					continue
				}
				title := "JetBrains IDE Crack Ajanı Tespit Edildi"
				desc := "JetBrains IDE'leri (IntelliJ, PyCharm, WebStorm, GoLand, Rider vb.) için lisans bypass ajanı (ja-netfilter) bulundu."
				evidence := fmt.Sprintf("Tür: %s | Yol: %s", typ, path)
				if detail != "" {
					evidence += fmt.Sprintf(" | Satır: %s", detail)
				}
				if typ == "vmoptions-crack-agent" {
					title = "JetBrains .vmoptions'ta Crack Agent Satırı"
					desc = "JetBrains IDE başlatma ayarlarında (-javaagent:) lisans bypass ajanına işaret eden bir satır bulundu."
				}
				findings = append(findings, CrackFinding{
					Category:    "kms_tool",
					Severity:    "high",
					Title:       title,
					Description: desc,
					Evidence:    evidence,
				})
			}
		}
	}

	findings = enrichAndDedupeFindings(findings)
	riskLevel = calcRiskLevel(findings)
	durationMs = time.Since(start).Milliseconds()
	return
}
