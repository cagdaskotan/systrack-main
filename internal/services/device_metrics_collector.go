package services

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/gosnmp/gosnmp"
)

// DeviceMetrics represents system metrics for a monitored device
type DeviceMetrics struct {
	TargetID      int       `json:"target_id"`
	TargetName    string    `json:"target_name"`
	TargetAddress string    `json:"target_address"`
	CollectedAt   time.Time `json:"collected_at"`
	TsMs          int64     `json:"ts_ms"`

	CPUPercent *float64 `json:"cpu_percent,omitempty"`
	CPUCores   *int     `json:"cpu_cores,omitempty"`

	RAMTotalMB *int64   `json:"ram_total_mb,omitempty"`
	RAMUsedMB  *int64   `json:"ram_used_mb,omitempty"`
	RAMPercent *float64 `json:"ram_percent,omitempty"`

	DiskTotalGB *int64   `json:"disk_total_gb,omitempty"`
	DiskUsedGB  *int64   `json:"disk_used_gb,omitempty"`
	DiskPercent *float64 `json:"disk_percent,omitempty"`

	UptimeSeconds *int64   `json:"uptime_seconds,omitempty"`
	TemperatureC  *float64 `json:"temperature_c,omitempty"`

	Status   string `json:"status"`
	ErrorMsg string `json:"error_msg,omitempty"`
}

// Target represents a monitoring target from database
type Target struct {
	ID             int
	Name           string
	Address        string
	MetricsEnabled bool
	SNMPCommunity  string
	SNMPVersion    string
}

// DeviceMetricsCollector collects metrics from remote devices via SNMP
type DeviceMetricsCollector struct {
	db                   *sql.DB
	broadcastFunc        func(DeviceMetrics)
	stopChan             chan struct{}
	interval             time.Duration
	persistInterval      time.Duration
	lastPersist          map[int]time.Time
	persistMu            sync.Mutex
	serverStatusInterval time.Duration
	lastServerStatus     map[int]time.Time
	serverStatusMu       sync.Mutex
	windowsMetricsReader *WindowsMetricsReader
	linuxMetricsReader   *LinuxMetricsReader
}

// SNMP OIDs for system metrics (Host Resources MIB - RFC 2790)
const (
	// CPU - hrProcessorLoad (percentage)
	oidCPULoad = ".1.3.6.1.2.1.25.3.3.1.2"

	// Memory - hrStorageTable
	oidStorageType  = ".1.3.6.1.2.1.25.2.3.1.2"
	oidStorageDescr = ".1.3.6.1.2.1.25.2.3.1.3"
	oidStorageUnits = ".1.3.6.1.2.1.25.2.3.1.4"
	oidStorageSize  = ".1.3.6.1.2.1.25.2.3.1.5"
	oidStorageUsed  = ".1.3.6.1.2.1.25.2.3.1.6"

	// System uptime (in hundredths of seconds)
	oidSysUptime = ".1.3.6.1.2.1.1.3.0"

	// Storage type OIDs for identification
	hrStorageRAM       = ".1.3.6.1.2.1.25.2.1.2" // Physical RAM
	hrStorageFixedDisk = ".1.3.6.1.2.1.25.2.1.4" // Fixed disk
)

// NewDeviceMetricsCollector creates a new device metrics collector
func NewDeviceMetricsCollector(db *sql.DB, broadcastFunc func(DeviceMetrics)) *DeviceMetricsCollector {
	return &DeviceMetricsCollector{
		db:                   db,
		broadcastFunc:        broadcastFunc,
		stopChan:             make(chan struct{}),
		interval:             3 * time.Second, // Very fast polling for realtime WS updates (aggressive)
		persistInterval:      10 * time.Minute,
		lastPersist:          make(map[int]time.Time),
		serverStatusInterval: 2 * time.Minute,
		lastServerStatus:     make(map[int]time.Time),
		windowsMetricsReader: NewWindowsMetricsReader(),
		linuxMetricsReader:   NewLinuxMetricsReader(),
	}
}

// Start begins collecting metrics from enabled targets
func (dmc *DeviceMetricsCollector) Start() {
	go func() {
		ticker := time.NewTicker(dmc.interval)
		defer ticker.Stop()

		// First collection immediately
		dmc.collectAllMetrics()

		for {
			select {
			case <-ticker.C:
				dmc.collectAllMetrics()
			case <-dmc.stopChan:
				log.Println("Device metrics collector stopped")
				return
			}
		}
	}()
	log.Printf("Device metrics collector started (interval: %v)", dmc.interval)
}

// Stop halts the metrics collection
func (dmc *DeviceMetricsCollector) Stop() {
	close(dmc.stopChan)
}

// CollectMetrics collects metrics from a single target (public API for live queries)
func (dmc *DeviceMetricsCollector) CollectMetrics(target Target) DeviceMetrics {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	return dmc.collectMetrics(ctx, target)
}

// collectAllMetrics collects metrics from all enabled targets
func (dmc *DeviceMetricsCollector) collectAllMetrics() {
	targets, err := dmc.getEnabledTargets()
	if err != nil {
		log.Printf("Failed to get enabled targets: %v", err)
		return
	}

	if len(targets) == 0 {
		return
	}

	log.Printf("Collecting metrics from %d target(s)", len(targets))

	var wg sync.WaitGroup
	for _, target := range targets {
		wg.Add(1)
		go func(t Target) {
			defer wg.Done()
			dmc.collectAndSave(t)
		}(target)
	}
	wg.Wait()

	log.Println("Metrics collection cycle completed")
}

// getEnabledTargets retrieves all targets with metrics_enabled=1
func (dmc *DeviceMetricsCollector) getEnabledTargets() ([]Target, error) {
	query := `
		SELECT t.id, t.name, t.address, t.metrics_enabled,
		       COALESCE(t.snmp_community, 'public') as snmp_community,
		       COALESCE(t.snmp_version, 'v2c') as snmp_version
		FROM targets t
		WHERE t.enabled = 1
			AND (
				t.metrics_enabled = 1
				OR EXISTS (
					SELECT 1
					FROM target_credentials tc
					WHERE tc.target_id = t.id
				)
			)
	`

	rows, err := dmc.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var targets []Target
	for rows.Next() {
		var t Target
		if err := rows.Scan(&t.ID, &t.Name, &t.Address, &t.MetricsEnabled, &t.SNMPCommunity, &t.SNMPVersion); err != nil {
			log.Printf("Failed to scan target: %v", err)
			continue
		}
		targets = append(targets, t)
	}

	return targets, nil
}

// collectAndSave collects metrics from a single target and broadcasts via WebSocket
// Note: Database persistence is disabled - using live SNMP queries instead
func (dmc *DeviceMetricsCollector) collectAndSave(target Target) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	metrics := dmc.collectMetrics(ctx, target)

	// Database persistence disabled - we use live queries now
	// if dmc.shouldPersist(metrics.TargetID, metrics.CollectedAt) {
	// 	if err := dmc.saveMetrics(metrics); err != nil {
	// 		log.Printf("[%s] Failed to save metrics: %v", target.Address, err)
	// 	}
	// }
	if metrics.Status == "success" && dmc.shouldPersistServerStatus(metrics.TargetID, metrics.CollectedAt) {
		if err := dmc.saveServerStatus(metrics); err != nil {
			log.Printf("[%s] Failed to save server_status: %v", target.Address, err)
		}
	}

	// Broadcast via WebSocket for real-time updates
	if dmc.broadcastFunc != nil {
		dmc.broadcastFunc(metrics)
	}
}

func (dmc *DeviceMetricsCollector) shouldPersist(targetID int, collectedAt time.Time) bool {
	dmc.persistMu.Lock()
	defer dmc.persistMu.Unlock()

	last, ok := dmc.lastPersist[targetID]
	if !ok || collectedAt.Sub(last) >= dmc.persistInterval {
		dmc.lastPersist[targetID] = collectedAt
		return true
	}
	return false
}

func (dmc *DeviceMetricsCollector) shouldPersistServerStatus(targetID int, collectedAt time.Time) bool {
	dmc.serverStatusMu.Lock()
	defer dmc.serverStatusMu.Unlock()

	last, ok := dmc.lastServerStatus[targetID]
	if !ok || collectedAt.Sub(last) >= dmc.serverStatusInterval {
		dmc.lastServerStatus[targetID] = collectedAt
		return true
	}
	return false
}

// collectMetrics collects metrics from a device via SNMP
func (dmc *DeviceMetricsCollector) collectMetrics(ctx context.Context, target Target) DeviceMetrics {
	now := time.Now()
	metrics := DeviceMetrics{
		TargetID:      target.ID,
		TargetName:    target.Name,
		TargetAddress: target.Address,
		CollectedAt:   now,
		TsMs:          now.UnixMilli(),
		Status:        "success",
	}

	if dmc.tryCredentialsFirst(ctx, target, &metrics) {
		return metrics
	}

	// Determine SNMP version
	var version gosnmp.SnmpVersion
	switch target.SNMPVersion {
	case "v1":
		version = gosnmp.Version1
	case "v3":
		version = gosnmp.Version3
	default:
		version = gosnmp.Version2c
	}

	// Create SNMP client with aggressive timeouts for fast response
	client := &gosnmp.GoSNMP{
		Target:    target.Address,
		Port:      161,
		Community: target.SNMPCommunity,
		Version:   version,
		Timeout:   2 * time.Second, // Fast timeout
		Retries:   1,               // Single retry for speed
		MaxOids:   60,
	}

	log.Printf("[%s] Attempting SNMP connection (v:%s, comm:%s)", target.Address, target.SNMPVersion, target.SNMPCommunity)

	if err := client.Connect(); err != nil {
		metrics.Status = "unreachable"
		metrics.ErrorMsg = fmt.Sprintf("Connection failed: %v", err)
		log.Printf("[%s] ❌ SNMP connection FAILED: %v (community:%s, version:%s)",
			target.Address, err, target.SNMPCommunity, target.SNMPVersion)
		return metrics
	}
	defer client.Conn.Close()

	log.Printf("[%s] ✓ SNMP connected successfully", target.Address)

	// Collect CPU metrics
	cpuPercent, cpuCores := dmc.collectCPU(client)
	if cpuPercent != nil {
		metrics.CPUPercent = cpuPercent
	}
	if cpuCores != nil {
		metrics.CPUCores = cpuCores
	}

	// Collect Memory metrics
	ramTotal, ramUsed, ramPercent := dmc.collectMemory(client)
	metrics.RAMTotalMB = ramTotal
	metrics.RAMUsedMB = ramUsed
	metrics.RAMPercent = ramPercent

	// Collect Disk metrics
	diskTotal, diskUsed, diskPercent := dmc.collectDisk(client)
	metrics.DiskTotalGB = diskTotal
	metrics.DiskUsedGB = diskUsed
	metrics.DiskPercent = diskPercent

	// Collect System uptime
	if uptime := dmc.collectUptime(client); uptime != nil {
		metrics.UptimeSeconds = uptime
	}

	// Fallback: If Host Resources MIB didn't work, try vendor-specific methods
	if metrics.CPUPercent == nil && metrics.RAMTotalMB == nil {
		log.Printf("[%s] Host Resources MIB failed, trying vendor-specific fallback...", target.Address)

		// Detect device profile
		profile := dmc.detectDeviceProfile(client)

		// Try vendor-specific OIDs based on detected profile
		var fallbackCPU *float64
		var fallbackRAMTotal, fallbackRAMUsed *int64
		var fallbackRAMPercent *float64

		switch profile.Vendor {
		case "linux":
			fallbackCPU, fallbackRAMTotal, fallbackRAMUsed, fallbackRAMPercent = dmc.tryLinuxUCDSNMP(client)
		case "mikrotik":
			fallbackCPU, fallbackRAMTotal, fallbackRAMUsed, fallbackRAMPercent = dmc.tryMikrotikMIB(client)
		default:
			log.Printf("[%s] No vendor-specific fallback available for: %s", target.Address, profile.Vendor)
		}

		// Apply fallback values if successful
		if fallbackCPU != nil {
			metrics.CPUPercent = fallbackCPU
		}
		if fallbackRAMTotal != nil {
			metrics.RAMTotalMB = fallbackRAMTotal
			metrics.RAMUsedMB = fallbackRAMUsed
			metrics.RAMPercent = fallbackRAMPercent
		}

		// Log success or partial success
		if metrics.CPUPercent != nil || metrics.RAMTotalMB != nil {
			log.Printf("[%s] ✓ Vendor-specific fallback succeeded (%s)", target.Address, profile.Vendor)
		} else if metrics.UptimeSeconds != nil {
			// At least we got uptime, so SNMP is working
			metrics.Status = "success"
			metrics.ErrorMsg = fmt.Sprintf("Limited SNMP support (Vendor: %s, no CPU/RAM MIB)", profile.Vendor)
		}
	}

	// If absolutely no metrics were collected, mark as error
	if metrics.CPUPercent == nil && metrics.RAMTotalMB == nil && metrics.DiskTotalGB == nil && metrics.UptimeSeconds == nil {
		metrics.Status = "snmp_error"
		metrics.ErrorMsg = "No metrics could be retrieved - SNMP agent may not support Host Resources MIB"
	}

	return metrics
}

func (dmc *DeviceMetricsCollector) tryCredentialsFirst(ctx context.Context, target Target, metrics *DeviceMetrics) bool {
	if dmc.db == nil {
		return false
	}

	credCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	cred, err := dmc.getTargetCredential(credCtx, target.ID)
	if err != nil || cred == nil {
		if err != nil {
			log.Printf("[MetricsFallback] credential lookup failed for target %d: %v", target.ID, err)
		}
		return false
	}

	password, err := DecryptPassword(cred.PasswordEnc)
	if err != nil {
		log.Printf("[MetricsFallback] decrypt failed for target %d: %v", target.ID, err)
		return false
	}

	fallbackCtx, fallbackCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer fallbackCancel()

	var updated bool
	switch cred.OSType {
	case "windows":
		result, err := dmc.windowsMetricsReader.ReadMetrics(fallbackCtx, target.Address, cred.Username, password, cred.Domain)
		if err != nil {
			log.Printf("[MetricsFallback] windows metrics failed for target %d (%s): %v", target.ID, target.Address, err)
			return false
		}
		updated = applyWindowsMetrics(metrics, result)
	case "linux":
		port := 0
		if cred.Port.Valid {
			port = int(cred.Port.Int64)
		}
		result, err := dmc.linuxMetricsReader.ReadMetrics(fallbackCtx, target.Address, cred.Username, password, port)
		if err != nil {
			log.Printf("[MetricsFallback] linux metrics failed for target %d (%s): %v", target.ID, target.Address, err)
			return false
		}
		updated = applyLinuxMetrics(metrics, result)
	default:
		log.Printf("[MetricsFallback] unsupported os_type for target %d: %s", target.ID, cred.OSType)
		return false
	}

	if updated {
		metrics.Status = "success"
		metrics.ErrorMsg = ""
		return true
	}

	return false
}

type targetCredential struct {
	OSType      string
	Username    string
	PasswordEnc string
	Domain      string
	Port        sql.NullInt64
}

func (dmc *DeviceMetricsCollector) shouldTryCredentialFallback(metrics DeviceMetrics) bool {
	if dmc.db == nil {
		return false
	}
	if metrics.Status != "success" {
		return true
	}
	return metrics.CPUPercent == nil || metrics.RAMTotalMB == nil || metrics.DiskTotalGB == nil
}

func (dmc *DeviceMetricsCollector) applyCredentialFallback(ctx context.Context, target Target, metrics *DeviceMetrics) {
	credCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	cred, err := dmc.getTargetCredential(credCtx, target.ID)
	if err != nil || cred == nil {
		if err != nil {
			log.Printf("[MetricsFallback] credential lookup failed for target %d: %v", target.ID, err)
		}
		return
	}

	password, err := DecryptPassword(cred.PasswordEnc)
	if err != nil {
		log.Printf("[MetricsFallback] decrypt failed for target %d: %v", target.ID, err)
		return
	}

	var updated bool
	fallbackCtx, fallbackCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer fallbackCancel()

	switch cred.OSType {
	case "windows":
		result, err := dmc.windowsMetricsReader.ReadMetrics(fallbackCtx, target.Address, cred.Username, password, cred.Domain)
		if err != nil {
			log.Printf("[MetricsFallback] windows metrics failed for target %d (%s): %v", target.ID, target.Address, err)
			break
		}
		updated = applyWindowsMetrics(metrics, result)
	case "linux":
		port := 0
		if cred.Port.Valid {
			port = int(cred.Port.Int64)
		}
		result, err := dmc.linuxMetricsReader.ReadMetrics(fallbackCtx, target.Address, cred.Username, password, port)
		if err != nil {
			log.Printf("[MetricsFallback] linux metrics failed for target %d (%s): %v", target.ID, target.Address, err)
			break
		}
		updated = applyLinuxMetrics(metrics, result)
	default:
		log.Printf("[MetricsFallback] unsupported os_type for target %d: %s", target.ID, cred.OSType)
	}

	if updated {
		metrics.Status = "success"
		metrics.ErrorMsg = ""
	}
}

func (dmc *DeviceMetricsCollector) getTargetCredential(ctx context.Context, targetID int) (*targetCredential, error) {
	query := `
		SELECT os_type, username, password_encrypted, COALESCE(domain, ''), port
		FROM target_credentials
		WHERE target_id = ?
	`

	var cred targetCredential
	err := dmc.db.QueryRowContext(ctx, query, targetID).Scan(
		&cred.OSType, &cred.Username, &cred.PasswordEnc, &cred.Domain, &cred.Port,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return &cred, nil
}

func applyWindowsMetrics(metrics *DeviceMetrics, result WindowsMetricsResult) bool {
	updated := false
	if result.CPUPercent != nil && metrics.CPUPercent == nil {
		metrics.CPUPercent = result.CPUPercent
		updated = true
	}
	if result.CPUCores != nil && metrics.CPUCores == nil {
		metrics.CPUCores = result.CPUCores
		updated = true
	}
	if result.RAMTotalMB != nil && metrics.RAMTotalMB == nil {
		metrics.RAMTotalMB = result.RAMTotalMB
		updated = true
	}
	if result.RAMUsedMB != nil && metrics.RAMUsedMB == nil {
		metrics.RAMUsedMB = result.RAMUsedMB
		updated = true
	}
	if result.DiskTotalGB != nil && metrics.DiskTotalGB == nil {
		metrics.DiskTotalGB = result.DiskTotalGB
		updated = true
	}
	if result.DiskUsedGB != nil && metrics.DiskUsedGB == nil {
		metrics.DiskUsedGB = result.DiskUsedGB
		updated = true
	}
	if result.UptimeSeconds != nil && metrics.UptimeSeconds == nil {
		metrics.UptimeSeconds = result.UptimeSeconds
		updated = true
	}
	return updated
}

func applyLinuxMetrics(metrics *DeviceMetrics, result LinuxMetricsResult) bool {
	updated := false
	if result.CPUPercent != nil && metrics.CPUPercent == nil {
		metrics.CPUPercent = result.CPUPercent
		updated = true
	}
	if result.CPUCores != nil && metrics.CPUCores == nil {
		metrics.CPUCores = result.CPUCores
		updated = true
	}
	if result.RAMTotalMB != nil && metrics.RAMTotalMB == nil {
		metrics.RAMTotalMB = result.RAMTotalMB
		updated = true
	}
	if result.RAMUsedMB != nil && metrics.RAMUsedMB == nil {
		metrics.RAMUsedMB = result.RAMUsedMB
		updated = true
	}
	if result.DiskTotalGB != nil && metrics.DiskTotalGB == nil {
		metrics.DiskTotalGB = result.DiskTotalGB
		updated = true
	}
	if result.DiskUsedGB != nil && metrics.DiskUsedGB == nil {
		metrics.DiskUsedGB = result.DiskUsedGB
		updated = true
	}
	if result.UptimeSeconds != nil && metrics.UptimeSeconds == nil {
		metrics.UptimeSeconds = result.UptimeSeconds
		updated = true
	}
	if result.TemperatureC != nil && metrics.TemperatureC == nil {
		metrics.TemperatureC = result.TemperatureC
		updated = true
	}
	return updated
}

// getSystemDescription retrieves system description
func (dmc *DeviceMetricsCollector) getSystemDescription(client *gosnmp.GoSNMP) string {
	result, err := client.Get([]string{".1.3.6.1.2.1.1.1.0"}) // sysDescr
	if err != nil || len(result.Variables) == 0 {
		return ""
	}

	if descr, ok := result.Variables[0].Value.([]byte); ok {
		return string(descr)
	}
	if descr, ok := result.Variables[0].Value.(string); ok {
		return descr
	}
	return ""
}

// collectCPU retrieves CPU usage percentage and number of cores
func (dmc *DeviceMetricsCollector) collectCPU(client *gosnmp.GoSNMP) (*float64, *int) {
	// Walk the CPU load table to get all processor entries
	results, err := client.BulkWalkAll(oidCPULoad)
	if err != nil {
		return nil, nil
	}

	if len(results) == 0 {
		return nil, nil
	}

	// Calculate average CPU load across all cores
	var totalLoad int64
	var count int
	for _, variable := range results {
		if val := gosnmpToInt(variable); val != nil {
			totalLoad += *val
			count++
		}
	}

	if count == 0 {
		return nil, nil
	}

	avg := float64(totalLoad) / float64(count)
	return &avg, &count
}

// collectMemory retrieves RAM metrics
func (dmc *DeviceMetricsCollector) collectMemory(client *gosnmp.GoSNMP) (*int64, *int64, *float64) {
	// Walk storage table
	results, err := client.BulkWalkAll(oidStorageType)
	if err != nil {
		return nil, nil, nil
	}

	// Find RAM storage entries
	var ramTotal, ramUsed int64
	for _, variable := range results {
		if gosnmpToString(variable) == hrStorageRAM {
			// Extract index from OID (last segment)
			index := getOIDIndex(variable.Name)
			if index == "" {
				continue
			}

			// Get allocation units, size, and used
			units := dmc.getSNMPValue(client, oidStorageUnits+"."+index)
			size := dmc.getSNMPValue(client, oidStorageSize+"."+index)
			used := dmc.getSNMPValue(client, oidStorageUsed+"."+index)

			if units != nil && size != nil && used != nil {
				// Convert to MB
				totalMB := (*size * *units) / (1024 * 1024)
				usedMB := (*used * *units) / (1024 * 1024)

				ramTotal += totalMB
				ramUsed += usedMB
			}
		}
	}

	if ramTotal == 0 {
		return nil, nil, nil
	}

	percent := float64(ramUsed) / float64(ramTotal) * 100.0
	return &ramTotal, &ramUsed, &percent
}

// collectDisk retrieves disk metrics
func (dmc *DeviceMetricsCollector) collectDisk(client *gosnmp.GoSNMP) (*int64, *int64, *float64) {
	// Strategy 1: Try storage description table (works on most systems)
	results, err := client.BulkWalkAll(oidStorageDescr)
	if err != nil {
		log.Printf("[DISK] BulkWalk storageDescr failed: %v", err)
		return nil, nil, nil
	}

	if len(results) == 0 {
		log.Printf("[DISK] No storage entries found")
		return nil, nil, nil
	}

	var diskTotal, diskUsed int64
	foundDisks := 0

	for _, variable := range results {
		descr := gosnmpToString(variable)
		if descr == "" {
			continue
		}

		index := getOIDIndex(variable.Name)
		if index == "" {
			continue
		}

		// Check storage type first
		typeOID := oidStorageType + "." + index
		typeResult, err := client.Get([]string{typeOID})
		if err != nil {
			continue
		}

		typeStr := ""
		if len(typeResult.Variables) > 0 {
			typeStr = gosnmpToString(typeResult.Variables[0])
		}

		// Windows disk detection: Look for disk-like descriptions
		// Common patterns: "C:\", "D:\", "C:\ Label:Serial Number", "/"
		isDisk := false

		// Pattern 1: Drive letter (C:, D:, etc.)
		if len(descr) >= 2 && descr[1] == ':' {
			if (descr[0] >= 'A' && descr[0] <= 'Z') || (descr[0] >= 'a' && descr[0] <= 'z') {
				isDisk = true
			}
		}

		// Pattern 2: Unix paths (/, /home, etc.)
		if len(descr) > 0 && descr[0] == '/' {
			isDisk = true
		}

		// Pattern 3: Check if type indicates fixed disk
		if typeStr == hrStorageFixedDisk || typeStr == ".1.3.6.1.2.1.25.2.1.4" {
			isDisk = true
		}

		if !isDisk {
			continue
		}

		// Get metrics
		units := dmc.getSNMPValue(client, oidStorageUnits+"."+index)
		size := dmc.getSNMPValue(client, oidStorageSize+"."+index)
		used := dmc.getSNMPValue(client, oidStorageUsed+"."+index)

		if units != nil && size != nil && used != nil && *size > 0 {
			// Calculate bytes
			totalBytes := *size * *units
			usedBytes := *used * *units

			// Skip small entries (< 100MB likely virtual/temp)
			if totalBytes < 100*1024*1024 {
				continue
			}

			totalGB := totalBytes / (1024 * 1024 * 1024)
			usedGB := usedBytes / (1024 * 1024 * 1024)

			diskTotal += totalGB
			diskUsed += usedGB
			foundDisks++

			log.Printf("[DISK] Added: %s | Total: %d GB, Used: %d GB, Type: %s", descr, totalGB, usedGB, typeStr)
		}
	}

	if foundDisks == 0 {
		log.Printf("[DISK] No valid disk entries found")
		return nil, nil, nil
	}

	percent := float64(diskUsed) / float64(diskTotal) * 100.0
	log.Printf("[DISK] Final: %d disks, Total: %d GB, Used: %d GB (%.1f%%)", foundDisks, diskTotal, diskUsed, percent)
	return &diskTotal, &diskUsed, &percent
}

// collectUptime retrieves system uptime in seconds
func (dmc *DeviceMetricsCollector) collectUptime(client *gosnmp.GoSNMP) *int64 {
	result, err := client.Get([]string{oidSysUptime})
	if err != nil || len(result.Variables) == 0 {
		return nil
	}

	// Uptime is in hundredths of seconds, convert to seconds
	if val := gosnmpToInt(result.Variables[0]); val != nil {
		seconds := *val / 100
		return &seconds
	}

	return nil
}

// getSNMPValue retrieves a single SNMP value
func (dmc *DeviceMetricsCollector) getSNMPValue(client *gosnmp.GoSNMP, oid string) *int64 {
	result, err := client.Get([]string{oid})
	if err != nil || len(result.Variables) == 0 {
		return nil
	}

	return gosnmpToInt(result.Variables[0])
}

// saveMetrics saves metrics to database
func (dmc *DeviceMetricsCollector) saveMetrics(m DeviceMetrics) error {
	query := `
		INSERT INTO device_metrics (
			target_id, collected_at, ts_ms,
			cpu_percent, cpu_cores,
			ram_total_mb, ram_used_mb, ram_percent,
			disk_total_gb, disk_used_gb, disk_percent,
			uptime_seconds,
			status, error_msg
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err := dmc.db.Exec(query,
		m.TargetID, m.CollectedAt, m.TsMs,
		m.CPUPercent, m.CPUCores,
		m.RAMTotalMB, m.RAMUsedMB, m.RAMPercent,
		m.DiskTotalGB, m.DiskUsedGB, m.DiskPercent,
		m.UptimeSeconds,
		m.Status, m.ErrorMsg,
	)

	return err
}

// saveServerStatus stores a minimal snapshot for reporting
func (dmc *DeviceMetricsCollector) saveServerStatus(m DeviceMetrics) error {
	if dmc.db == nil {
		return nil
	}
	var ramGB *float64
	if m.RAMUsedMB != nil {
		val := float64(*m.RAMUsedMB) / 1024.0
		ramGB = &val
	}
	var diskGB *float64
	if m.DiskUsedGB != nil {
		val := float64(*m.DiskUsedGB)
		diskGB = &val
	}

	query := `
		INSERT INTO server_status (
			target_id, collected_at,
			cpu_percent, ram_gb, disk_gb, temperature_c
		) VALUES (?, ?, ?, ?, ?, ?)
	`

	_, err := dmc.db.Exec(query,
		m.TargetID, m.CollectedAt,
		m.CPUPercent, ramGB, diskGB, m.TemperatureC,
	)

	return err
}

// Helper functions for SNMP data conversion

func gosnmpToInt(variable gosnmp.SnmpPDU) *int64 {
	switch variable.Type {
	case gosnmp.Integer, gosnmp.Counter32, gosnmp.Gauge32, gosnmp.TimeTicks, gosnmp.Counter64:
		val := gosnmp.ToBigInt(variable.Value).Int64()
		return &val
	}
	return nil
}

func gosnmpToString(variable gosnmp.SnmpPDU) string {
	switch variable.Type {
	case gosnmp.OctetString:
		return string(variable.Value.([]byte))
	case gosnmp.ObjectIdentifier:
		return variable.Value.(string)
	}
	return ""
}

func getOIDIndex(oid string) string {
	// Extract last segment of OID (e.g., ".1.3.6.1.2.1.25.2.3.1.2.1" -> "1")
	for i := len(oid) - 1; i >= 0; i-- {
		if oid[i] == '.' {
			return oid[i+1:]
		}
	}
	return ""
}

// Helper function for case-insensitive string matching
func containsIgnoreCase(s, substr string) bool {
	s = strings.ToLower(s)
	substr = strings.ToLower(substr)
	return strings.Contains(s, substr)
}

// DeviceProfile represents detected device type
type DeviceProfile struct {
	Vendor      string
	Description string
	ObjectID    string
}

// detectDeviceProfile determines device type from SNMP
func (dmc *DeviceMetricsCollector) detectDeviceProfile(client *gosnmp.GoSNMP) DeviceProfile {
	profile := DeviceProfile{Vendor: "generic"}

	// Get sysDescr and sysObjectID
	result, err := client.Get([]string{
		".1.3.6.1.2.1.1.1.0", // sysDescr
		".1.3.6.1.2.1.1.2.0", // sysObjectID
	})

	if err != nil || len(result.Variables) < 2 {
		return profile
	}

	// Parse sysDescr
	if descr, ok := result.Variables[0].Value.([]byte); ok {
		profile.Description = string(descr)
	} else if descr, ok := result.Variables[0].Value.(string); ok {
		profile.Description = descr
	}

	// Parse sysObjectID
	if oid, ok := result.Variables[1].Value.(string); ok {
		profile.ObjectID = oid
	}

	// Detect vendor from ObjectID and Description
	switch {
	case containsIgnoreCase(profile.Description, "linux"):
		profile.Vendor = "linux"
	case containsIgnoreCase(profile.Description, "windows"):
		profile.Vendor = "windows"
	case containsIgnoreCase(profile.ObjectID, "1.3.6.1.4.1.9"):
		profile.Vendor = "cisco"
	case containsIgnoreCase(profile.ObjectID, "1.3.6.1.4.1.14988"):
		profile.Vendor = "mikrotik"
	case containsIgnoreCase(profile.ObjectID, "1.3.6.1.4.1.8072"):
		profile.Vendor = "linux" // NET-SNMP
	}

	log.Printf("[PROFILE] Detected vendor: %s (desc: %.50s, oid: %s)", profile.Vendor, profile.Description, profile.ObjectID)
	return profile
}

// tryLinuxUCDSNMP tries UCD-SNMP/NET-SNMP MIB for Linux systems
func (dmc *DeviceMetricsCollector) tryLinuxUCDSNMP(client *gosnmp.GoSNMP) (*float64, *int64, *int64, *float64) {
	log.Printf("[LINUX] Trying UCD-SNMP MIB...")

	// Try to get memory from UCD-SNMP
	result, err := client.Get([]string{
		".1.3.6.1.4.1.2021.4.5.0",    // memTotalReal (KB)
		".1.3.6.1.4.1.2021.4.6.0",    // memAvailReal (KB)
		".1.3.6.1.4.1.2021.10.1.3.1", // laLoad 1min
	})

	if err != nil || len(result.Variables) < 2 {
		log.Printf("[LINUX] UCD-SNMP query failed: %v", err)
		return nil, nil, nil, nil
	}

	// Parse memory
	var ramTotal, ramUsed *int64
	var ramPercent *float64

	if totalKB := gosnmpToInt(result.Variables[0]); totalKB != nil {
		if availKB := gosnmpToInt(result.Variables[1]); availKB != nil {
			totalMB := *totalKB / 1024
			usedMB := (*totalKB - *availKB) / 1024

			ramTotal = &totalMB
			ramUsed = &usedMB

			if totalMB > 0 {
				pct := float64(usedMB) / float64(totalMB) * 100.0
				ramPercent = &pct
			}

			log.Printf("[LINUX] RAM: %d MB total, %d MB used", totalMB, usedMB)
		}
	}

	// Parse CPU load (convert to percentage approximation)
	var cpuPercent *float64
	if len(result.Variables) >= 3 {
		if load := gosnmpToFloat(result.Variables[2]); load != nil {
			// Load average to percentage (rough): assume 1.0 load = 100% on 1 core
			// This is not perfect but gives an indication
			pct := *load * 100.0
			if pct > 100.0 {
				pct = 100.0
			}
			cpuPercent = &pct
			log.Printf("[LINUX] CPU load: %.2f (%.1f%%)", *load, pct)
		}
	}

	return cpuPercent, ramTotal, ramUsed, ramPercent
}

// tryMikrotikMIB tries MikroTik-specific OIDs
func (dmc *DeviceMetricsCollector) tryMikrotikMIB(client *gosnmp.GoSNMP) (*float64, *int64, *int64, *float64) {
	log.Printf("[MIKROTIK] Trying MikroTik MIB...")

	result, err := client.Get([]string{
		".1.3.6.1.4.1.14988.1.1.3.10.0", // mtxrCpuLoad
		".1.3.6.1.4.1.14988.1.1.3.8.0",  // mtxrMemTotal
		".1.3.6.1.4.1.14988.1.1.3.9.0",  // mtxrMemUsed (bytes)
	})

	if err != nil || len(result.Variables) < 3 {
		log.Printf("[MIKROTIK] MIB query failed: %v", err)
		return nil, nil, nil, nil
	}

	// Parse CPU
	var cpuPercent *float64
	if cpu := gosnmpToInt(result.Variables[0]); cpu != nil {
		pct := float64(*cpu)
		cpuPercent = &pct
		log.Printf("[MIKROTIK] CPU: %.1f%%", pct)
	}

	// Parse memory
	var ramTotal, ramUsed *int64
	var ramPercent *float64

	if totalBytes := gosnmpToInt(result.Variables[1]); totalBytes != nil {
		if usedBytes := gosnmpToInt(result.Variables[2]); usedBytes != nil {
			totalMB := *totalBytes / (1024 * 1024)
			usedMB := *usedBytes / (1024 * 1024)

			ramTotal = &totalMB
			ramUsed = &usedMB

			if totalMB > 0 {
				pct := float64(usedMB) / float64(totalMB) * 100.0
				ramPercent = &pct
			}

			log.Printf("[MIKROTIK] RAM: %d MB total, %d MB used", totalMB, usedMB)
		}
	}

	return cpuPercent, ramTotal, ramUsed, ramPercent
}

// gosnmpToFloat converts SNMP value to float64
func gosnmpToFloat(variable gosnmp.SnmpPDU) *float64 {
	switch v := variable.Value.(type) {
	case float64:
		return &v
	case float32:
		f := float64(v)
		return &f
	case int:
		f := float64(v)
		return &f
	case int64:
		f := float64(v)
		return &f
	case uint:
		f := float64(v)
		return &f
	case uint64:
		f := float64(v)
		return &f
	}
	return nil
}
