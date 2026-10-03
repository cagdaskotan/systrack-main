package license

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"systrack/internal/remote_access"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/host"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/shirou/gopsutil/v3/net"
)

// HeartbeatSender manages periodic heartbeat sending to management server
type HeartbeatSender struct {
	db                  *sql.DB
	serialNumber        string
	managementURL       string
	macAddress          string
	stopChan            chan struct{}
	isRunning           bool
	isBlocked           bool
	blockReason         string
	isExpired           bool
	remoteAccessManager *remote_access.Manager
}

// HeartbeatRequest matches backend's expectation (without hash fields for now)
type HeartbeatRequest struct {
	SerialNumber    string  `json:"serial_number"`
	SoftwareVersion string  `json:"software_version,omitempty"`
	Status          int     `json:"status"`
	UptimeSeconds   int64   `json:"uptime_seconds"`
	Timestamp       int64   `json:"timestamp"`
	CPUPercent      float64 `json:"cpu_percent,omitempty"`
	RAMUsedMB       int     `json:"ram_used_mb,omitempty"`
	RAMTotalMB      int     `json:"ram_total_mb,omitempty"`
	DiskUsedGB      int     `json:"disk_used_gb,omitempty"`
	DiskTotalGB     int     `json:"disk_total_gb,omitempty"`
	TargetsCount    int     `json:"targets_count,omitempty"`
	MacAddress      string  `json:"mac_address,omitempty"`
	LocalIPAddress  string  `json:"local_ip_address,omitempty"`
}

// RemoteAccessConfig represents remote access tunnel configuration from server
type RemoteAccessConfig struct {
	Enabled     bool   `json:"enabled"`
	TunnelHost  string `json:"tunnel_host,omitempty"`
	SSHUser     string `json:"ssh_user,omitempty"`
	SSHPassword string `json:"ssh_password,omitempty"`
	UIPort      int    `json:"ui_port,omitempty"`
	DBPort      *int   `json:"db_port,omitempty"`
}

// HeartbeatResponse matches backend's response
type HeartbeatResponse struct {
	Status              string `json:"status"`
	ExpiryDate          string `json:"expiry_date,omitempty"`
	Command             string `json:"command"`
	BlockReason         string `json:"block_reason,omitempty"` // "piracy_detected", "admin_blocked", etc.
	Message             string `json:"message,omitempty"`
	ServerTime          int64  `json:"server_time"`
	NextHeartbeat       int    `json:"next_heartbeat,omitempty"`
	ActivationConfirmed bool   `json:"activation_confirmed,omitempty"`

	// Remote access configuration
	RemoteAccess *RemoteAccessConfig `json:"remote_access,omitempty"`
}

// NewHeartbeatSender creates a new heartbeat sender
func NewHeartbeatSender(db *sql.DB, macAddress string) (*HeartbeatSender, error) {
	// Get serial number from database
	var serialNumber string
	err := db.QueryRow("SELECT serial_number FROM device_license LIMIT 1").Scan(&serialNumber)
	if err != nil {
		return nil, fmt.Errorf("failed to get serial number: %w", err)
	}

	// Get management server URL
	managementURL := os.Getenv("MANAGEMENT_SERVER_URL")
	if managementURL == "" {
		managementURL = DefaultManagementServerURL
	}

	log.Printf("💓 Heartbeat configured: Server=%s, Serial=%s", managementURL, serialNumber)

	// Create remote access manager
	raManager := remote_access.NewManager()

	return &HeartbeatSender{
		db:                  db,
		serialNumber:        serialNumber,
		managementURL:       managementURL,
		macAddress:          macAddress,
		stopChan:            make(chan struct{}),
		isRunning:           false,
		remoteAccessManager: raManager,
	}, nil
}

// Start begins sending heartbeats every 10 minutes
func (hs *HeartbeatSender) Start() {
	if hs.isRunning {
		log.Println("⚠️  Heartbeat sender already running")
		return
	}

	hs.isRunning = true
	go hs.heartbeatLoop()
	log.Println("💓 Heartbeat sender started (interval: 10 minutes)")
}

// Stop gracefully stops the heartbeat sender
func (hs *HeartbeatSender) Stop() {
	if !hs.isRunning {
		return
	}

	// Stop remote access tunnel if active
	if hs.remoteAccessManager != nil {
		hs.remoteAccessManager.Stop()
	}

	close(hs.stopChan)
	hs.isRunning = false
	log.Println("🛑 Heartbeat sender stopped")
}

// heartbeatLoop runs the periodic heartbeat sending
func (hs *HeartbeatSender) heartbeatLoop() {
	// Send first heartbeat immediately
	hs.sendHeartbeat()

	ticker := time.NewTicker(GetHeartbeatInterval())
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			hs.sendHeartbeat()
		case <-hs.stopChan:
			return
		}
	}
}

// sendHeartbeat collects metrics and sends to management server
func (hs *HeartbeatSender) sendHeartbeat() {
	// Collect system metrics
	metrics, err := hs.collectMetrics()
	if err != nil {
		log.Printf("⚠️  Failed to collect metrics: %v", err)
		// Continue anyway with minimal metrics
		metrics = HeartbeatRequest{
			SerialNumber:    hs.serialNumber,
			SoftwareVersion: getCurrentSoftwareVersionFromDB(hs.db),
			Status:          StatusActive,
			Timestamp:       time.Now().UnixMilli(),
		}
	}

	// Prepare request
	jsonData, err := json.Marshal(metrics)
	if err != nil {
		log.Printf("❌ Failed to marshal heartbeat: %v", err)
		return
	}

	// Send HTTP POST request
	url := fmt.Sprintf("%s/api/v1/heartbeat", hs.managementURL)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		log.Printf("❌ Failed to create heartbeat request: %v", err)
		return
	}

	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{
		Timeout: time.Duration(HeartbeatTimeoutSec) * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		log.Printf("❌ Heartbeat failed: %v", err)
		return
	}
	defer resp.Body.Close()

	// Parse response
	var hbResp HeartbeatResponse
	if err := json.NewDecoder(resp.Body).Decode(&hbResp); err != nil {
		log.Printf("⚠️  Failed to parse heartbeat response: %v", err)
		return
	}

	// Log response
	log.Printf("✅ Heartbeat sent successfully - Status: %s, Command: %s", hbResp.Status, hbResp.Command)

	// Handle server commands
	hs.handleServerCommand(hbResp)
}

func currentSoftwareVersion() string {
	if v := os.Getenv("SOFTWARE_VERSION"); v != "" {
		return v
	}
	return SoftwareVersion
}

// getCurrentSoftwareVersionFromDB reads the current software version from database
func getCurrentSoftwareVersionFromDB(db *sql.DB) string {
	var version sql.NullString
	err := db.QueryRow("SELECT initial_version FROM device_license LIMIT 1").Scan(&version)
	if err != nil || !version.Valid || version.String == "" {
		// Fallback to constant if database read fails
		return currentSoftwareVersion()
	}
	return version.String
}

// collectMetrics gathers system metrics
func (hs *HeartbeatSender) collectMetrics() (HeartbeatRequest, error) {
	req := HeartbeatRequest{
		SerialNumber:    hs.serialNumber,
		SoftwareVersion: getCurrentSoftwareVersionFromDB(hs.db),
		Status:          StatusActive,
		Timestamp:       time.Now().UnixMilli(),
	}

	// Get system uptime
	hostInfo, err := host.Info()
	if err == nil {
		req.UptimeSeconds = int64(hostInfo.Uptime)
	}

	// Get CPU usage
	cpuPercent, err := cpu.Percent(time.Second, false)
	if err == nil && len(cpuPercent) > 0 {
		req.CPUPercent = cpuPercent[0]
	}

	// Get RAM usage
	memInfo, err := mem.VirtualMemory()
	if err == nil {
		req.RAMUsedMB = int(memInfo.Used / (1024 * 1024))
		req.RAMTotalMB = int(memInfo.Total / (1024 * 1024))
	}

	// Get disk usage (Windows C:\ or Linux /)
	diskPath := "C:\\"
	diskInfo, err := disk.Usage(diskPath)
	if err != nil {
		// Try Linux path
		diskPath = "/"
		diskInfo, err = disk.Usage(diskPath)
	}
	if err == nil {
		req.DiskUsedGB = int(diskInfo.Used / (1024 * 1024 * 1024))
		req.DiskTotalGB = int(diskInfo.Total / (1024 * 1024 * 1024))
	}

	// Get targets count from database
	var targetsCount int
	err = hs.db.QueryRow("SELECT COUNT(*) FROM targets WHERE enabled = 1").Scan(&targetsCount)
	if err == nil {
		req.TargetsCount = targetsCount
	}

	// Get MAC and local IP address
	if hs.macAddress != "" {
		req.MacAddress = hs.macAddress
	} else if macAddr, err := GetCurrentMACAddress(); err == nil {
		req.MacAddress = macAddr
	}
	if localIP := getLocalIP(); localIP != "" {
		req.LocalIPAddress = localIP
	}

	return req, nil
}

// handleServerCommand processes commands from management server
func (hs *HeartbeatSender) handleServerCommand(resp HeartbeatResponse) {
	// Check for activation confirmation
	if resp.ActivationConfirmed {
		log.Printf("✅ Activation confirmed by server - updating local database")

		// Update device_license status to active
		_, err := hs.db.Exec(`
			UPDATE device_license
			SET status = ?,
			    activated_at = NOW()
			WHERE serial_number = ?
		`, StatusActive, hs.serialNumber)

		if err != nil {
			log.Printf("❌ Failed to update activation status: %v", err)
		} else {
			log.Printf("🎉 Device successfully activated! Serial=%s", hs.serialNumber)
		}
	}

	switch resp.Command {
	case "block":
		log.Printf("🚨 DEVICE BLOCKED BY SERVER: %s", resp.Message)
		log.Printf("   Block Reason: %s", resp.BlockReason)
		hs.isBlocked = true
		hs.blockReason = resp.BlockReason

		// Update local database status to blocked
		_, err := hs.db.Exec(`
			UPDATE device_license SET status = ? WHERE serial_number = ?
		`, StatusBlocked, hs.serialNumber)
		if err != nil {
			log.Printf("❌ Failed to update local blocked status: %v", err)
		}

	case "expire":
		log.Printf("⏰ LICENSE EXPIRED: %s", resp.Message)
		hs.isExpired = true

		// Update local database status to expired
		_, err := hs.db.Exec(`
			UPDATE device_license SET status = ? WHERE serial_number = ?
		`, StatusExpired, hs.serialNumber)
		if err != nil {
			log.Printf("❌ Failed to update local expired status: %v", err)
		}

	case "db_backup":
		log.Printf("📦 Database backup requested by server")
		go hs.executeDBBackup()

	case "update_available":
		// NOTE: Update handling is now done by the separate systrack-updater container
		log.Printf("📦 Update available (will be handled by systrack-updater container)")
	case "none":
	default:
		log.Printf("⚠️  Unknown command from server: %s", resp.Command)
	}

	// If server says active, always clear local blocked/expired state
	if resp.Status == "active" {
		if hs.isBlocked {
			log.Printf("✅ Device block has been lifted!")
			hs.isBlocked = false
			hs.blockReason = ""
			hs.db.Exec(`
				UPDATE device_license SET status = ? WHERE serial_number = ?
			`, StatusActive, hs.serialNumber)
		}

		if hs.isExpired {
			log.Printf("✅ License has been renewed!")
			hs.isExpired = false
			hs.db.Exec(`
				UPDATE device_license SET status = ? WHERE serial_number = ?
			`, StatusActive, hs.serialNumber)
		}
	}

	// Handle remote access configuration
	hs.handleRemoteAccess(resp.RemoteAccess)

	// Log status changes
	if resp.Status != "active" {
		log.Printf("⚠️  Device status changed: %s - %s", resp.Status, resp.Message)
	}
}

// handleRemoteAccess processes remote access configuration from server
func (hs *HeartbeatSender) handleRemoteAccess(config *RemoteAccessConfig) {
	if hs.remoteAccessManager == nil {
		return
	}

	// If no config or not enabled, stop any active tunnel
	if config == nil || !config.Enabled {
		if hs.remoteAccessManager.IsActive() {
			log.Printf("🔒 Remote access disabled by server, stopping tunnel...")
			if err := hs.remoteAccessManager.Stop(); err != nil {
				log.Printf("⚠️  Failed to stop remote access tunnel: %v", err)
			}
		}
		return
	}

	// Convert to remote_access.Config and start/update tunnel
	raConfig := remote_access.Config{
		Enabled:     config.Enabled,
		TunnelHost:  config.TunnelHost,
		SSHUser:     config.SSHUser,
		SSHPassword: config.SSHPassword,
		UIPort:      config.UIPort,
		DBPort:      config.DBPort,
	}

	if err := hs.remoteAccessManager.Start(raConfig); err != nil {
		log.Printf("❌ Failed to start remote access tunnel: %v", err)
	}
}

// getLocalIP returns the first non-loopback IPv4 address
func getLocalIP() string {
	interfaces, err := net.Interfaces()
	if err != nil {
		return ""
	}

	var fallback string
	for _, iface := range interfaces {
		if iface.Name == "lo" || isExcludedInterface(iface.Name) {
			continue
		}

		// Get addresses for this interface
		if len(iface.Addrs) > 0 {
			for _, addr := range iface.Addrs {
				ipStr := addr.Addr
				// Skip loopback and IPv6 and localhost
				if strings.Contains(ipStr, ":") {
					continue // IPv6
				}
				// Extract IP from CIDR notation (e.g., "10.20.11.29/23" -> "10.20.11.29")
				if strings.Contains(ipStr, "/") {
					ipStr = strings.Split(ipStr, "/")[0]
				}
				if ipStr == "" || strings.HasPrefix(ipStr, "127.") {
					continue
				}
				if isLinkLocalIPv4(ipStr) {
					continue
				}
				if isPrivateIPv4(ipStr) {
					return ipStr
				}
				if fallback == "" {
					fallback = ipStr
				}
			}
		}
	}

	return fallback
}

func isExcludedInterface(name string) bool {
	if strings.HasPrefix(name, "docker") {
		return true
	}
	if strings.HasPrefix(name, "veth") {
		return true
	}
	if strings.HasPrefix(name, "vir") {
		return true
	}
	// Docker creates bridges like br-<hash>. Allow br0 for real bridges.
	if strings.HasPrefix(name, "br-") {
		return true
	}
	return false
}

func isPrivateIPv4(ip string) bool {
	octets, ok := parseIPv4Octets(ip)
	if !ok {
		return false
	}
	if octets[0] == 10 {
		return true
	}
	if octets[0] == 172 && octets[1] >= 16 && octets[1] <= 31 {
		return true
	}
	if octets[0] == 192 && octets[1] == 168 {
		return true
	}
	return false
}

func isLinkLocalIPv4(ip string) bool {
	octets, ok := parseIPv4Octets(ip)
	return ok && octets[0] == 169 && octets[1] == 254
}

func parseIPv4Octets(ip string) ([4]int, bool) {
	var out [4]int
	parts := strings.Split(ip, ".")
	if len(parts) != 4 {
		return out, false
	}
	for i, part := range parts {
		val, err := strconv.Atoi(part)
		if err != nil || val < 0 || val > 255 {
			return out, false
		}
		out[i] = val
	}
	return out, true
}

// IsBlocked returns whether the device is currently blocked
func (hs *HeartbeatSender) IsBlocked() bool {
	return hs.isBlocked
}

// GetBlockReason returns the reason for blocking
func (hs *HeartbeatSender) GetBlockReason() string {
	return hs.blockReason
}

// SetBlocked sets the blocked state (used during initialization)
func (hs *HeartbeatSender) SetBlocked(blocked bool, reason string) {
	hs.isBlocked = blocked
	hs.blockReason = reason
}

// IsExpired returns whether the device license is currently expired
func (hs *HeartbeatSender) IsExpired() bool {
	return hs.isExpired
}

// SetExpired sets the expired state (used during initialization)
func (hs *HeartbeatSender) SetExpired(expired bool) {
	hs.isExpired = expired
}

// =====================
// DATABASE BACKUP
// =====================

// executeDBBackup runs mysqldump, compresses the output, and uploads it to management server
func (hs *HeartbeatSender) executeDBBackup() {
	log.Printf("📦 Starting database backup for %s...", hs.serialNumber)

	// 1. Parse DB credentials from DB_DSN
	dsn := os.Getenv("DB_DSN")
	if dsn == "" {
		log.Printf("❌ DB_DSN environment variable not set, cannot perform backup")
		return
	}

	dbUser, dbPass, dbHost, dbPort, dbName := parseDSN(dsn)
	if dbName == "" {
		log.Printf("❌ Failed to parse DB_DSN: %s", dsn)
		return
	}

	// 2. Create temp file for SQL dump
	tmpFile, err := os.CreateTemp("", "systrack-backup-*.sql")
	if err != nil {
		log.Printf("❌ Failed to create temp file for backup: %v", err)
		return
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(tmpPath)

	// 3. Run mysqldump
	args := []string{
		"-h", dbHost,
		"-P", dbPort,
		"-u", dbUser,
		fmt.Sprintf("-p%s", dbPass),
		"--single-transaction",
		"--routines",
		"--triggers",
		"--result-file=" + tmpPath,
		dbName,
	}

	cmd := exec.Command("mysqldump", args...)
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		log.Printf("❌ mysqldump failed: %v", err)
		return
	}

	// Check dump file size
	dumpInfo, err := os.Stat(tmpPath)
	if err != nil || dumpInfo.Size() == 0 {
		log.Printf("❌ mysqldump produced empty file")
		return
	}
	log.Printf("📄 SQL dump created: %.2f MB", float64(dumpInfo.Size())/(1024*1024))

	// 4. Gzip compress
	gzPath := tmpPath + ".gz"
	defer os.Remove(gzPath)

	if err := gzipFile(tmpPath, gzPath); err != nil {
		log.Printf("❌ gzip compression failed: %v", err)
		return
	}

	gzInfo, _ := os.Stat(gzPath)
	log.Printf("📦 Compressed: %.2f MB", float64(gzInfo.Size())/(1024*1024))

	// 5. Calculate SHA256 checksum
	checksum, err := calculateSHA256(gzPath)
	if err != nil {
		log.Printf("⚠️  Checksum calculation failed: %v", err)
		checksum = ""
	}

	// 6. Upload to management server
	if err := hs.uploadBackup(gzPath, checksum); err != nil {
		log.Printf("❌ Backup upload failed: %v", err)
		return
	}

	log.Printf("✅ Database backup completed and uploaded successfully for %s", hs.serialNumber)
}

// parseDSN extracts user, password, host, port, and database name from Go MySQL DSN
// Format: "user:pass@tcp(host:port)/dbname?params"
func parseDSN(dsn string) (user, pass, host, port, dbName string) {
	re := regexp.MustCompile(`^([^:]+):([^@]*)@tcp\(([^:]+):(\d+)\)/([^?]+)`)
	matches := re.FindStringSubmatch(dsn)
	if len(matches) >= 6 {
		return matches[1], matches[2], matches[3], matches[4], matches[5]
	}
	return "", "", "", "", ""
}

// gzipFile compresses src file to dst using gzip
func gzipFile(src, dst string) error {
	inFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer inFile.Close()

	outFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer outFile.Close()

	gzWriter := gzip.NewWriter(outFile)
	defer gzWriter.Close()

	_, err = io.Copy(gzWriter, inFile)
	return err
}

// calculateSHA256 computes the SHA256 hex digest of a file
func calculateSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

// uploadBackup sends the backup file to management server via multipart POST
func (hs *HeartbeatSender) uploadBackup(filePath, checksum string) error {
	url := fmt.Sprintf("%s/api/v1/backup/upload", strings.TrimRight(hs.managementURL, "/"))

	// Open the file
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open backup file: %w", err)
	}
	defer file.Close()

	// Create multipart body
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// Add serial_number field
	writer.WriteField("serial_number", hs.serialNumber)

	// Add checksum field
	if checksum != "" {
		writer.WriteField("checksum", checksum)
	}

	// Add file
	part, err := writer.CreateFormFile("backup_file", "backup.sql.gz")
	if err != nil {
		return fmt.Errorf("failed to create form file: %w", err)
	}

	if _, err := io.Copy(part, file); err != nil {
		return fmt.Errorf("failed to copy file to form: %w", err)
	}

	writer.Close()

	// Send request with extended timeout (backup files can be large)
	client := &http.Client{Timeout: 10 * time.Minute}
	req, err := http.NewRequest("POST", url, body)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("upload request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("upload failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	log.Printf("📤 Backup uploaded successfully to %s", url)
	return nil
}
