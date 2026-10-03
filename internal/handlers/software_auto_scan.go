package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"strings"
	"sync"
	"time"

	"systrack/internal/auth"

	"github.com/masterzen/winrm"
)

// SoftwareAutoScanner, inventory_scan_settings tablosundaki yapılandırmaya göre
// her gün belirli bir saatte cihazların yazılım listesini otomatik tarar.
type SoftwareAutoScanner struct {
	db   *sql.DB
	stop chan struct{}
	wg   sync.WaitGroup
}

func NewSoftwareAutoScanner(db *sql.DB) *SoftwareAutoScanner {
	return &SoftwareAutoScanner{db: db, stop: make(chan struct{})}
}

func (s *SoftwareAutoScanner) Start() {
	s.wg.Add(1)
	go s.loop()
	log.Println("[auto-scan] Yazılım otomatik tarama servisi başlatıldı")
}

func (s *SoftwareAutoScanner) Stop() {
	close(s.stop)
	s.wg.Wait()
}

func (s *SoftwareAutoScanner) loop() {
	defer s.wg.Done()
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-s.stop:
			return
		case now := <-ticker.C:
			s.runIfDue(now)
		}
	}
}

func (s *SoftwareAutoScanner) runIfDue(now time.Time) {
	currentHour := now.Hour()

	rows, err := s.db.Query(`
		SELECT iss.inventory_id, COALESCE(i.ip_address,''), COALESCE(i.hostname,'')
		FROM inventory_scan_settings iss
		JOIN inventory i ON i.id = iss.inventory_id
		WHERE iss.auto_software_scan = 1
		  AND iss.scan_hour = ?
		  AND (
		        iss.last_auto_scan_at IS NULL
		        OR iss.last_auto_scan_at < DATE_SUB(NOW(), INTERVAL 23 HOUR)
		  )
	`, currentHour)
	if err != nil {
		log.Printf("[auto-scan] Tarama listesi sorgu hatası: %v", err)
		return
	}

	type target struct {
		inventoryID int
		ip          string
		hostname    string
	}
	var targets []target
	for rows.Next() {
		var t target
		if err := rows.Scan(&t.inventoryID, &t.ip, &t.hostname); err == nil {
			targets = append(targets, t)
		}
	}
	rows.Close()

	if len(targets) == 0 {
		return
	}

	log.Printf("[auto-scan] Saat %02d:xx — %d cihaz için yazılım taraması başlatılıyor", currentHour, len(targets))

	creds, err := s.loadWinRMCredentials()
	if err != nil {
		log.Printf("[auto-scan] WinRM kimlik bilgileri okunamadı: %v", err)
		return
	}

	for _, t := range targets {
		select {
		case <-s.stop:
			return
		default:
		}
		s.scanDevice(t.inventoryID, t.ip, t.hostname, creds)
		time.Sleep(5 * time.Second)
	}
}

type autoScanCreds struct {
	username string
	password string
	port     int
	useSSL   bool
}

func (s *SoftwareAutoScanner) loadWinRMCredentials() (*autoScanCreds, error) {
	var username, encPass string
	var port sql.NullInt64
	var useSSL sql.NullBool

	err := s.db.QueryRow(`
		SELECT username, password_encrypted, port, use_ssl
		FROM winrm_settings WHERE id = 1
	`).Scan(&username, &encPass, &port, &useSSL)
	if err != nil {
		return nil, err
	}

	password, err := auth.DecryptPassword(encPass)
	if err != nil {
		return nil, err
	}

	creds := &autoScanCreds{username: username, password: password, port: 5985}
	if port.Valid {
		creds.port = int(port.Int64)
	}
	if useSSL.Valid {
		creds.useSSL = useSSL.Bool
	}
	return creds, nil
}

func (s *SoftwareAutoScanner) scanDevice(inventoryID int, ip, hostname string, creds *autoScanCreds) {
	target := ip
	if hostname != "" {
		target = hostname
	}

	client, err := s.connectWinRM(target, creds)
	if err != nil && hostname != "" && ip != "" {
		client, err = s.connectWinRM(ip, creds)
	}
	if err != nil {
		log.Printf("[auto-scan] Cihaz #%d (%s) bağlantı hatası: %v", inventoryID, target, err)
		return
	}

	psCmd := `$a = @(Get-ItemProperty 'HKLM:\Software\Microsoft\Windows\CurrentVersion\Uninstall\*' -EA SilentlyContinue) + @(Get-ItemProperty 'HKLM:\Software\Wow6432Node\Microsoft\Windows\CurrentVersion\Uninstall\*' -EA SilentlyContinue); $r = $a | Where-Object {$_.DisplayName} | Select-Object DisplayName,DisplayVersion,Publisher,InstallDate | Sort-Object DisplayName; ConvertTo-Json -InputObject @($r) -Compress`

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	encoded := encodePowerShellCommand("[System.Threading.Thread]::CurrentThread.CurrentCulture = [System.Globalization.CultureInfo]::InvariantCulture; " + psCmd)
	const bootstrap = `$b64=[Console]::In.ReadToEnd();$bytes=[Convert]::FromBase64String($b64);$s=[System.Text.Encoding]::Unicode.GetString($bytes);Invoke-Expression $s`
	psCommand := `powershell -NoProfile -NonInteractive -Command "` + bootstrap + `"`

	out, _, _, err := client.RunWithContextWithString(ctx, psCommand, encoded)
	if err != nil || strings.TrimSpace(out) == "" {
		log.Printf("[auto-scan] Cihaz #%d yazılım listesi alınamadı: %v", inventoryID, err)
		return
	}
	newJSON := strings.TrimSpace(out)

	var check interface{}
	if json.Unmarshal([]byte(newJSON), &check) != nil {
		return
	}

	var prevJSON sql.NullString
	s.db.QueryRow(`SELECT software_json FROM inventory WHERE id = ?`, inventoryID).Scan(&prevJSON)

	if err := RecordSoftwareChanges(s.db, inventoryID, newJSON, prevJSON.String); err != nil {
		log.Printf("[auto-scan] Cihaz #%d değişim kaydı hatası: %v", inventoryID, err)
	}

	s.db.Exec(`UPDATE inventory SET software_json = ?, software_scan_at = NOW() WHERE id = ?`, newJSON, inventoryID)
	s.db.Exec(`UPDATE inventory_scan_settings SET last_auto_scan_at = NOW() WHERE inventory_id = ?`, inventoryID)

	log.Printf("[auto-scan] Cihaz #%d (%s) taraması tamamlandı", inventoryID, target)
}

func (s *SoftwareAutoScanner) connectWinRM(target string, creds *autoScanCreds) (*winrm.Client, error) {
	endpoint := winrm.NewEndpoint(target, creds.port, creds.useSSL, creds.useSSL, nil, nil, nil, 0)
	params := winrm.DefaultParameters
	params.Timeout = "PT90S"
	return winrm.NewClientWithParameters(endpoint, creds.username, creds.password, params)
}
