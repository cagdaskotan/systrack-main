package handlers

import (
	"context"
	"database/sql"
	"log"
	"net"
	"net/http"
	"net/netip"
	"runtime"
	"strconv"
	"strings"
	"time"

	"systrack/internal/network/scanner"

	"github.com/gin-gonic/gin"
)

type ipScanRequest struct {
	Subnet             string `json:"subnet"`
	RangeStart         string `json:"range_start"`
	RangeEnd           string `json:"range_end"`
	TimeoutMs          int    `json:"timeout_ms"`
	WorkerCount        int    `json:"worker_count"`
	MaxHosts           int    `json:"max_hosts"`
	IncludeUnreachable bool   `json:"include_unreachable"`
	Ports              string `json:"ports"`
}

func GetIPScannerDefaults() gin.HandlerFunc {
	return func(c *gin.Context) {
		interfaces, err := scanner.ListInterfaces()
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"interfaces":     []scanner.InterfaceInfo{},
				"default_subnet": "",
				"error":          err.Error(),
			})
			return
		}

		defaultSubnet := ""
		for _, iface := range interfaces {
			if iface.Default {
				defaultSubnet = iface.CIDR
				break
			}
		}

		c.JSON(http.StatusOK, gin.H{
			"interfaces":     interfaces,
			"default_subnet": defaultSubnet,
		})
	}
}

func RunIPScan() gin.HandlerFunc {
	return RunIPScanWithDB(nil)
}

func RunIPScanWithDB(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var request ipScanRequest
		if err := c.ShouldBindJSON(&request); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
			return
		}

		request.Subnet = strings.TrimSpace(request.Subnet)
		request.RangeStart = strings.TrimSpace(request.RangeStart)
		request.RangeEnd = strings.TrimSpace(request.RangeEnd)
		request.Ports = strings.TrimSpace(request.Ports)

		useRange := request.RangeStart != "" || request.RangeEnd != ""
		if useRange && (request.RangeStart == "" || request.RangeEnd == "") {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Başlangıç ve bitiş IP adresleri birlikte verilmelidir"})
			return
		}

		if !useRange && request.Subnet == "" {
			iface, err := scanner.DetectDefaultSubnet()
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Subnet is required"})
				return
			}
			request.Subnet = iface.CIDR
		}

		if request.TimeoutMs <= 0 {
			request.TimeoutMs = 600
		} else if request.TimeoutMs > 5000 {
			request.TimeoutMs = 5000
		}

		if request.WorkerCount <= 0 {
			request.WorkerCount = runtime.NumCPU()
		} else if request.WorkerCount > 128 {
			request.WorkerCount = 128
		}

		if request.MaxHosts <= 0 {
			request.MaxHosts = 512
		} else if request.MaxHosts > 4096 {
			request.MaxHosts = 4096
		}

		var ports []int
		if request.Ports != "" {
			for _, part := range strings.Split(request.Ports, ",") {
				value := strings.TrimSpace(part)
				if value == "" {
					continue
				}
				p, err := strconv.Atoi(value)
				if err != nil || p <= 0 || p > 65535 {
					c.JSON(http.StatusBadRequest, gin.H{"error": "Geçerli port(lar) girin"})
					return
				}
				ports = append(ports, p)
			}
		}

		options := scanner.Options{
			Timeout:            time.Duration(request.TimeoutMs) * time.Millisecond,
			Retries:            1,
			Workers:            request.WorkerCount,
			MaxHosts:           request.MaxHosts,
			IncludeUnreachable: request.IncludeUnreachable,
			Ports:              ports,
			FilterByPorts:      len(ports) > 0,
		}

		var (
			result *scanner.Result
			err    error
		)

		if useRange {
			startAddr, parseErr := netip.ParseAddr(request.RangeStart)
			if parseErr != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Geçerli bir başlangıç IP girin"})
				return
			}
			endAddr, parseErr := netip.ParseAddr(request.RangeEnd)
			if parseErr != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Geçerli bir bitiş IP girin"})
				return
			}
			if startAddr.Compare(endAddr) > 0 {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Başlangıç IP adresi, bitiş IP adresinden küçük veya eşit olmalıdır"})
				return
			}
			result, err = scanner.ScanRange(c.Request.Context(), startAddr, endAddr, options)
		} else {
			result, err = scanner.ScanSubnet(c.Request.Context(), request.Subnet, options)
		}

		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		if db != nil && strings.Contains(result.Subnet, "/") {
			if err := saveIPScanResults(c.Request.Context(), db, result.Subnet, result.Devices); err != nil {
				log.Printf("ip-scanner: failed to persist scan results: %v", err)
			}
		}

		c.JSON(http.StatusOK, gin.H{
			"subnet":          result.Subnet,
			"host_count":      result.HostCount,
			"reachable_count": result.ReachableCount,
			"scanned_count":   result.ScannedCount,
			"duration_ms":     result.Duration.Milliseconds(),
			"generated_at":    time.Now().UTC(),
			"devices":         result.Devices,
		})
	}
}

func saveIPScanResults(ctx context.Context, db *sql.DB, subnet string, devices []scanner.DeviceInfo) error {
	subnet = strings.TrimSpace(subnet)
	if subnet == "" {
		return nil
	}

	seen := make(map[string]struct{}, len(devices))
	var ips []string
	for _, d := range devices {
		ip := strings.TrimSpace(d.IP)
		if ip == "" || net.ParseIP(ip) == nil {
			continue
		}
		if _, ok := seen[ip]; ok {
			continue
		}
		seen[ip] = struct{}{}
		ips = append(ips, ip)
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, "DELETE FROM ip_results WHERE subnet = ?", subnet); err != nil {
		return err
	}

	if len(ips) > 0 {
		stmt, err := tx.PrepareContext(ctx, "INSERT INTO ip_results (subnet, ip) VALUES (?, ?)")
		if err != nil {
			return err
		}
		defer stmt.Close()

		for _, ip := range ips {
			if _, err := stmt.ExecContext(ctx, subnet, ip); err != nil {
				return err
			}
		}
	}

	if _, err := tx.ExecContext(ctx, "UPDATE ip_alerts SET acknowledged_at = NOW() WHERE subnet = ? AND acknowledged_at IS NULL", subnet); err != nil {
		return err
	}

	return tx.Commit()
}

type createTargetsRequest struct {
	Devices []struct {
		IP       string `json:"ip"`
		Hostname string `json:"hostname"`
	} `json:"devices"`
	IntervalSec    int    `json:"interval_sec"`
	TimeoutMs      int    `json:"timeout_ms"`
	Tags           string `json:"tags"`
	MetricsEnabled *bool  `json:"metrics_enabled"`
	SNMPCommunity  string `json:"snmp_community"`
	SNMPVersion    string `json:"snmp_version"`
}

func CreateTargetsFromScan(db *sql.DB, scheduler interface{}, hub interface{}) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req createTargetsRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
			return
		}

		if len(req.Devices) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "En az bir IP seçmelisiniz"})
			return
		}

		if req.IntervalSec < 10 || req.IntervalSec > 3600 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Ping aralığı 10-3600 saniye olmalıdır"})
			return
		}

		if req.TimeoutMs < 100 || req.TimeoutMs > 30000 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Timeout 100-30000 ms olmalıdır"})
			return
		}

		ctx := c.Request.Context()
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Veritabanı bağlantısı kurulamadı"})
			return
		}
		defer tx.Rollback()

		tagValue := strings.TrimSpace(req.Tags)
		var tags *string
		if tagValue != "" {
			tags = &tagValue
		}

		metricsEnabled := true
		if req.MetricsEnabled != nil {
			metricsEnabled = *req.MetricsEnabled
		}

		snmpCommunity := strings.TrimSpace(req.SNMPCommunity)
		if snmpCommunity == "" {
			snmpCommunity = "public"
		}

		snmpVersion := strings.TrimSpace(req.SNMPVersion)
		if snmpVersion == "" {
			snmpVersion = "v2c"
		}

		inserted := 0
		var duplicates []string
		var failed []string
		var createdIDs []int64

		const insertQuery = `INSERT INTO targets (name, address, type, monitoring_type, metrics_enabled, snmp_community, snmp_version, port, path, http_method, http_path, http_headers, expected_status_code, expected_content, ssl_check, follow_redirects, timeout_sec, interval_sec, timeout_ms, enabled, tags) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

		for _, device := range req.Devices {
			ip := strings.TrimSpace(device.IP)
			if net.ParseIP(ip) == nil {
				failed = append(failed, ip)
				continue
			}

			var existing int
			if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM targets WHERE address = ? AND monitoring_type = 'ping'", ip).Scan(&existing); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Hedef kontrolü sırasında hata oluştu"})
				return
			}
			if existing > 0 {
				duplicates = append(duplicates, ip)
				continue
			}

			name := strings.TrimSpace(device.Hostname)
			if name == "" {
				name = ip
			}

			timeoutSec := req.TimeoutMs / 1000
			if timeoutSec < 5 {
				timeoutSec = 5
			}

			result, err := tx.ExecContext(ctx, insertQuery,
				name,
				ip,
				"icmp",
				"ping",
				metricsEnabled,
				snmpCommunity,
				snmpVersion,
				nil,
				nil,
				"GET",
				"/",
				nil,
				200,
				nil,
				false,
				false,
				timeoutSec,
				req.IntervalSec,
				req.TimeoutMs,
				true,
				tags,
			)
			if err != nil {
				failed = append(failed, ip)
				continue
			}

			// Oluşturulan hedefin ID'sini kaydet
			if lastID, err := result.LastInsertId(); err == nil {
				createdIDs = append(createdIDs, lastID)
			}

			inserted++
		}

		if err := tx.Commit(); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Veritabanı işlemi tamamlanamadı"})
			return
		}

		// Oluşturulan tüm hedeflere otomatik ping at
		if scheduler != nil && len(createdIDs) > 0 {
			log.Printf("🎯 Triggering immediate ping for %d newly created targets from IP scan", len(createdIDs))
			go func() {
				// Scheduler'dan PingTarget methodunu çağır
				if s, ok := scheduler.(interface{ PingTarget(int) (bool, time.Duration, error) }); ok {
					for _, targetID := range createdIDs {
						log.Printf("⚡ Executing immediate ping for scanned target ID: %d", targetID)
						success, duration, err := s.PingTarget(int(targetID))
						if err != nil {
							log.Printf("❌ Immediate ping failed for target ID %d: %v", targetID, err)
						} else {
							log.Printf("✅ Immediate ping completed for target ID %d: success=%v duration=%v", targetID, success, duration)
						}
					}
				}
			}()
		}

		c.JSON(http.StatusOK, gin.H{
			"created_count": inserted,
			"duplicates":    duplicates,
			"failed":        failed,
		})
	}
}
