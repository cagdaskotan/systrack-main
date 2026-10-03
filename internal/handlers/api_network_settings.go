package handlers

import (
	"bufio"
	"database/sql"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	networkInterfaceName  = "eth0"
	networkPendingDir     = "/app/network-pending"
	networkRequestFile    = "/app/network-pending/request.json"
	networkResultFile     = "/app/network-pending/result.json"
	networkWatcherTimeout = 10 * time.Second
	networkPollInterval   = 200 * time.Millisecond
)

type NetworkSettingsRequest struct {
	Mode       string   `json:"mode"`
	IP         string   `json:"ip"`
	SubnetMask string   `json:"subnet_mask"`
	Gateway    string   `json:"gateway"`
	DNS        []string `json:"dns"`
}

type NetworkSettingsResponse struct {
	Interface  string   `json:"interface"`
	Mode       string   `json:"mode"`
	IP         string   `json:"ip"`
	SubnetMask string   `json:"subnet_mask"`
	Gateway    string   `json:"gateway"`
	DNS        []string `json:"dns"`
}

type AccessAllowlistItem struct {
	ID        int    `json:"id"`
	IPAddress string `json:"ip_address"`
	Label     string `json:"label"`
	IsEnabled bool   `json:"is_enabled"`
}

type AccessAllowlistToggleRequest struct {
	Enabled bool `json:"enabled"`
}

type AccessAllowlistCreateRequest struct {
	IPAddress string `json:"ip_address"`
	Label     string `json:"label"`
}

// Host watcher'a gonderilecek istek
type networkWatcherRequest struct {
	Mode      string   `json:"mode"`
	IP        string   `json:"ip,omitempty"`
	Prefix    int      `json:"prefix,omitempty"`
	Gateway   string   `json:"gateway,omitempty"`
	DNS       []string `json:"dns,omitempty"`
	Timestamp int64    `json:"timestamp"`
}

// Host watcher'dan gelecek sonuc
type networkWatcherResult struct {
	Success   bool   `json:"success"`
	Message   string `json:"message"`
	Timestamp int64  `json:"timestamp"`
}

func GetNetworkSettings() gin.HandlerFunc {
	return func(c *gin.Context) {
		if runtime.GOOS != "linux" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Network settings only supported on Linux hosts"})
			return
		}

		ip, mask := getInterfaceIPv4(networkInterfaceName)
		gateway := getDefaultGateway(networkInterfaceName)
		dnsServers := getDNSServers()
		mode := detectCurrentMode()

		c.JSON(http.StatusOK, NetworkSettingsResponse{
			Interface:  networkInterfaceName,
			Mode:       mode,
			IP:         ip,
			SubnetMask: mask,
			Gateway:    gateway,
			DNS:        dnsServers,
		})
	}
}

func UpdateNetworkSettings() gin.HandlerFunc {
	return func(c *gin.Context) {
		if runtime.GOOS != "linux" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Network settings only supported on Linux hosts"})
			return
		}

		var req NetworkSettingsRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
			return
		}

		mode := strings.ToLower(strings.TrimSpace(req.Mode))
		if mode != "dhcp" && mode != "static" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid mode. Use 'dhcp' or 'static'."})
			return
		}

		// Watcher'a gonderilecek istegi hazirla
		watcherReq := networkWatcherRequest{
			Mode:      mode,
			Timestamp: time.Now().Unix(),
		}

		if mode == "static" {
			ip, err := normalizeIPv4(req.IP)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid IP address"})
				return
			}
			watcherReq.IP = ip

			mask, err := normalizeIPv4(req.SubnetMask)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid subnet mask"})
				return
			}

			prefix, err := maskToPrefix(mask)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid subnet mask"})
				return
			}
			watcherReq.Prefix = prefix

			if strings.TrimSpace(req.Gateway) != "" {
				gateway, err := normalizeIPv4(req.Gateway)
				if err != nil {
					c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid gateway address"})
					return
				}
				watcherReq.Gateway = gateway
			}

			dns := make([]string, 0, len(req.DNS))
			for _, entry := range req.DNS {
				if strings.TrimSpace(entry) == "" {
					continue
				}
				value, err := normalizeIPv4(entry)
				if err != nil {
					c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid DNS address"})
					return
				}
				dns = append(dns, value)
			}
			watcherReq.DNS = dns
		}

		// Eski result dosyasini sil (varsa)
		os.Remove(networkResultFile)

		// Request dosyasini yaz (atomic write: tmp -> mv)
		if err := writeNetworkRequest(watcherReq); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to write network request"})
			return
		}

		// Watcher'dan sonuc bekle
		result, err := waitForNetworkResult(watcherReq.Timestamp)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		if !result.Success {
			c.JSON(http.StatusInternalServerError, gin.H{"error": result.Message})
			return
		}

		c.JSON(http.StatusOK, gin.H{"message": result.Message})
	}
}

func GetAccessAllowlist(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		enabled := getSettingBool(db, "access_allowlist_enabled")

		rows, err := db.Query(`SELECT id, ip_address, label, is_enabled FROM access_allowlist ORDER BY id DESC`)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Allowlist okunamadı"})
			return
		}
		defer rows.Close()

		items := make([]AccessAllowlistItem, 0)
		for rows.Next() {
			var item AccessAllowlistItem
			if err := rows.Scan(&item.ID, &item.IPAddress, &item.Label, &item.IsEnabled); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Allowlist okunamadı"})
				return
			}
			items = append(items, item)
		}

		c.JSON(http.StatusOK, gin.H{
			"enabled": enabled,
			"items":   items,
		})
	}
}

func CreateAccessAllowlistItem(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req AccessAllowlistCreateRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Geçersiz istek"})
			return
		}

		ip := strings.TrimSpace(req.IPAddress)
		if ip == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "IP adresi gerekli"})
			return
		}
		if net.ParseIP(ip) == nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Geçersiz IP adresi"})
			return
		}

		label := strings.TrimSpace(req.Label)
		if label == "" {
			label = "IP"
		}

		_, err := db.Exec(`INSERT INTO access_allowlist (ip_address, label, is_enabled) VALUES (?, ?, 1)`, ip, label)
		if err != nil {
			if strings.Contains(strings.ToLower(err.Error()), "duplicate") {
				c.JSON(http.StatusOK, gin.H{"message": "IP zaten listede"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "IP eklenemedi"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"message": "IP eklendi"})
	}
}

func DeleteAccessAllowlistItem(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		if id == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "ID gerekli"})
			return
		}

		_, err := db.Exec(`DELETE FROM access_allowlist WHERE id = ?`, id)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "IP silinemedi"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"message": "IP silindi"})
	}
}

func UpdateAccessAllowlistToggle(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req AccessAllowlistToggleRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Geçersiz istek"})
			return
		}

		value := "0"
		if req.Enabled {
			value = "1"
		}

		_, err := db.Exec(`INSERT INTO settings (k, v) VALUES ('access_allowlist_enabled', ?) ON DUPLICATE KEY UPDATE v = VALUES(v)`, value)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Ayar güncellenemedi"})
			return
		}

		if req.Enabled {
			var count int
			_ = db.QueryRow(`SELECT COUNT(*) FROM access_allowlist`).Scan(&count)
			if count == 0 {
				clientIP := strings.TrimSpace(c.ClientIP())
				if clientIP != "" && net.ParseIP(clientIP) != nil {
					_, _ = db.Exec(`INSERT IGNORE INTO access_allowlist (ip_address, label, is_enabled) VALUES (?, 'Current IP', 1)`, clientIP)
				}
			}
		}

		c.JSON(http.StatusOK, gin.H{"message": "Ayar güncellendi"})
	}
}

func getSettingBool(db *sql.DB, key string) bool {
	var value string
	if err := db.QueryRow(`SELECT v FROM settings WHERE k = ?`, key).Scan(&value); err != nil {
		return false
	}
	value = strings.TrimSpace(strings.ToLower(value))
	return value == "1" || value == "true" || value == "yes" || value == "on"
}

// Atomic write: once tmp dosyasina yaz, sonra mv ile tasi
func writeNetworkRequest(req networkWatcherRequest) error {
	data, err := json.MarshalIndent(req, "", "  ")
	if err != nil {
		return err
	}

	tmpFile := networkRequestFile + ".tmp"
	if err := os.WriteFile(tmpFile, data, 0644); err != nil {
		return err
	}

	return os.Rename(tmpFile, networkRequestFile)
}

// Result dosyasini bekle ve oku
func waitForNetworkResult(requestTimestamp int64) (*networkWatcherResult, error) {
	deadline := time.Now().Add(networkWatcherTimeout)

	for time.Now().Before(deadline) {
		data, err := os.ReadFile(networkResultFile)
		if err != nil {
			time.Sleep(networkPollInterval)
			continue
		}

		var result networkWatcherResult
		if err := json.Unmarshal(data, &result); err != nil {
			time.Sleep(networkPollInterval)
			continue
		}

		// Eski bir result degil, bizim istegimize ait mi?
		if result.Timestamp >= requestTimestamp {
			return &result, nil
		}

		time.Sleep(networkPollInterval)
	}

	return nil, errors.New("Network configuration timeout. Host watcher may not be running.")
}

func normalizeIPv4(value string) (string, error) {
	parsed := net.ParseIP(strings.TrimSpace(value))
	if parsed == nil {
		return "", errors.New("invalid IP")
	}
	ip := parsed.To4()
	if ip == nil {
		return "", errors.New("invalid IPv4")
	}
	return ip.String(), nil
}

func maskToPrefix(mask string) (int, error) {
	ip := net.ParseIP(mask).To4()
	if ip == nil {
		return 0, errors.New("invalid mask")
	}
	ones, bits := net.IPMask(ip).Size()
	if bits != 32 || ones == 0 {
		return 0, errors.New("invalid mask")
	}
	return ones, nil
}

func getInterfaceIPv4(name string) (string, string) {
	iface, err := net.InterfaceByName(name)
	if err != nil {
		return "", ""
	}
	addrs, err := iface.Addrs()
	if err != nil {
		return "", ""
	}
	for _, addr := range addrs {
		ipNet, ok := addr.(*net.IPNet)
		if !ok || ipNet.IP == nil {
			continue
		}
		ip := ipNet.IP.To4()
		if ip == nil {
			continue
		}
		mask := net.IP(ipNet.Mask).String()
		return ip.String(), mask
	}
	return "", ""
}

func getDefaultGateway(iface string) string {
	cmd := exec.Command("ip", "route", "show", "default", "dev", iface)
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	fields := strings.Fields(string(output))
	for i, field := range fields {
		if field == "via" && i+1 < len(fields) {
			return strings.TrimSpace(fields[i+1])
		}
	}
	return ""
}

func getDNSServers() []string {
	// Once resolvectl ile gercek upstream DNS sunucularini oku
	// /etc/resolv.conf 127.0.0.53 (stub resolver) dondurur, bu yanilticidir
	if servers := getDNSFromResolvectl(); len(servers) > 0 {
		return servers
	}

	// Fallback: /etc/resolv.conf oku ama 127.x.x.x adreslerini filtrele
	file, err := os.Open("/etc/resolv.conf")
	if err != nil {
		return nil
	}
	defer file.Close()

	var servers []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "nameserver") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				addr := fields[1]
				// 127.x.x.x loopback adreslerini atla (stub resolver)
				if !strings.HasPrefix(addr, "127.") {
					servers = append(servers, addr)
				}
			}
		}
	}
	return servers
}

// resolvectl ile eth0 uzerindeki gercek upstream DNS sunucularini al
func getDNSFromResolvectl() []string {
	cmd := exec.Command("resolvectl", "dns", networkInterfaceName)
	output, err := cmd.Output()
	if err != nil {
		return nil
	}

	// Ornek cikti: "Link 2 (eth0): 10.20.10.10 10.20.10.1"
	line := strings.TrimSpace(string(output))
	idx := strings.Index(line, ":")
	if idx < 0 || idx+1 >= len(line) {
		return nil
	}

	parts := strings.Fields(strings.TrimSpace(line[idx+1:]))
	var servers []string
	for _, p := range parts {
		ip := net.ParseIP(p)
		if ip == nil {
			continue
		}
		// 127.x.x.x loopback adreslerini atla
		if ip.IsLoopback() {
			continue
		}
		servers = append(servers, p)
	}
	return servers
}

// Mevcut IP durumuna gore mod tespit et
func detectCurrentMode() string {
	// DHCP ile alinan IP'lerde lease dosyasi olur
	// Basit bir yaklasim: /run/systemd/netif/leases/ altinda dosya varsa DHCP
	leaseDir := "/run/systemd/netif/leases"
	entries, err := os.ReadDir(leaseDir)
	if err == nil && len(entries) > 0 {
		return "dhcp"
	}

	// IP varsa static kabul et
	ip, _ := getInterfaceIPv4(networkInterfaceName)
	if ip != "" {
		return "static"
	}

	return "dhcp"
}
