package services

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/masterzen/winrm"
)

// WinRM Error Categories
const (
	ErrCategoryDNS          = "dns_error"
	ErrCategoryTimeout      = "timeout"
	ErrCategoryAuthFailed   = "auth_failed"
	ErrCategoryWinRMDisabled = "winrm_disabled"
	ErrCategorySSLError     = "ssl_error"
	ErrCategoryConnection   = "connection_refused"
	ErrCategoryUnknown      = "unknown_error"
)

// WinRMError represents a categorized WinRM error
type WinRMError struct {
	Category string `json:"category"`
	Message  string `json:"message"`
	Detail   string `json:"detail,omitempty"`
}

// ClassifyError categorizes a WinRM error for better troubleshooting
func ClassifyError(err error) WinRMError {
	if err == nil {
		return WinRMError{}
	}

	errStr := strings.ToLower(err.Error())

	switch {
	case strings.Contains(errStr, "no such host") || strings.Contains(errStr, "dns"):
		return WinRMError{
			Category: ErrCategoryDNS,
			Message:  "DNS çözümleme hatası",
			Detail:   err.Error(),
		}
	case strings.Contains(errStr, "timeout") || strings.Contains(errStr, "deadline exceeded"):
		return WinRMError{
			Category: ErrCategoryTimeout,
			Message:  "Bağlantı zaman aşımı",
			Detail:   err.Error(),
		}
	case strings.Contains(errStr, "401") || strings.Contains(errStr, "unauthorized") || strings.Contains(errStr, "authentication"):
		return WinRMError{
			Category: ErrCategoryAuthFailed,
			Message:  "Kimlik doğrulama başarısız",
			Detail:   err.Error(),
		}
	case strings.Contains(errStr, "connection refused") || strings.Contains(errStr, "refused"):
		return WinRMError{
			Category: ErrCategoryWinRMDisabled,
			Message:  "WinRM servisi devre dışı veya erişilemiyor",
			Detail:   err.Error(),
		}
	case strings.Contains(errStr, "certificate") || strings.Contains(errStr, "tls") || strings.Contains(errStr, "ssl"):
		return WinRMError{
			Category: ErrCategorySSLError,
			Message:  "SSL/TLS sertifika hatası",
			Detail:   err.Error(),
		}
	case strings.Contains(errStr, "connect"):
		return WinRMError{
			Category: ErrCategoryConnection,
			Message:  "Bağlantı kurulamadı",
			Detail:   err.Error(),
		}
	default:
		return WinRMError{
			Category: ErrCategoryUnknown,
			Message:  "Bilinmeyen hata",
			Detail:   err.Error(),
		}
	}
}

// ToJSON converts WinRMError to JSON string
func (e WinRMError) ToJSON() string {
	data, _ := json.Marshal(e)
	return string(data)
}

// WinRMClient handles WinRM connections to Windows machines
type WinRMClient struct {
	Username        string
	Password        string
	Port            int
	UseSSL          bool
	TimeoutSeconds  int
	ConcurrentLimit int
}

// HardwareInfo represents collected hardware information from a Windows machine
type HardwareInfo struct {
	// Computer System
	Manufacturer string `json:"manufacturer"`
	Model        string `json:"model"`
	SystemType   string `json:"system_type"`

	// BIOS
	SerialNumber  string `json:"serial_number"`
	BIOSVersion   string `json:"bios_version"`
	BIOSVendor    string `json:"bios_vendor"`

	// Processor
	CPUName       string `json:"cpu_name"`
	CPUCores      int    `json:"cpu_cores"`
	CPUThreads    int    `json:"cpu_threads"`
	CPUSpeed      int    `json:"cpu_speed_mhz"`

	// Memory
	TotalRAMGB    float64 `json:"total_ram_gb"`
	RAMSlots      int     `json:"ram_slots"`
	RAMModules    []RAMModule `json:"ram_modules,omitempty"`

	// Disk
	Disks         []PhysicalDisk `json:"disks,omitempty"`
	TotalDiskGB   float64        `json:"total_disk_gb"`

	// Monitors
	Monitors      []MonitorInfo `json:"monitors,omitempty"`

	// OS Info
	Hostname      string `json:"hostname"`
	OSName        string `json:"os_name"`
	OSVersion     string `json:"os_version"`
	OSBuild       string `json:"os_build"`

	// Collection metadata
	CollectedAt   time.Time `json:"collected_at"`
	CollectionMs  int64     `json:"collection_ms"`
	Errors        []string  `json:"errors,omitempty"`
}

// RAMModule represents a single RAM module
type RAMModule struct {
	Capacity    int64  `json:"capacity_gb"`
	Speed       int    `json:"speed_mhz"`
	Manufacturer string `json:"manufacturer"`
	PartNumber   string `json:"part_number"`
}

// PhysicalDisk represents a physical disk from WinRM/WMI
type PhysicalDisk struct {
	Model        string  `json:"model"`
	SizeGB       float64 `json:"size_gb"`
	MediaType    string  `json:"media_type"`
	SerialNumber string  `json:"serial_number"`
}

// MonitorInfo represents a connected monitor
type MonitorInfo struct {
	Name          string `json:"name"`
	Manufacturer  string `json:"manufacturer"`
	ScreenWidth   int    `json:"screen_width"`
	ScreenHeight  int    `json:"screen_height"`
	SerialNumber  string `json:"serial_number"`
}

// NewWinRMClient creates a new WinRM client with the given settings
func NewWinRMClient(username, password string, port int, useSSL bool, timeoutSeconds int) *WinRMClient {
	return &WinRMClient{
		Username:       username,
		Password:       password,
		Port:           port,
		UseSSL:         useSSL,
		TimeoutSeconds: timeoutSeconds,
	}
}

// TestConnection tests the WinRM connection to a target
func (c *WinRMClient) TestConnection(targetIP string) (*HardwareInfo, error) {
	startTime := time.Now()

	// Create WinRM endpoint
	endpoint := winrm.NewEndpoint(
		targetIP,
		c.Port,
		c.UseSSL,
		true, // Insecure (skip cert verification)
		nil,  // CA cert
		nil,  // Client cert
		nil,  // Client key
		time.Duration(c.TimeoutSeconds)*time.Second,
	)

	// Create client
	client, err := winrm.NewClient(endpoint, c.Username, c.Password)
	if err != nil {
		return nil, fmt.Errorf("WinRM client oluşturulamadı: %v", err)
	}

	// Test with a simple command
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(c.TimeoutSeconds)*time.Second)
	defer cancel()

	stdout, stderr, _, err := client.RunWithContextWithString(ctx, "hostname", "")
	if err != nil {
		return nil, fmt.Errorf("WinRM bağlantı hatası: %v", err)
	}

	if stderr != "" && !strings.Contains(stderr, "CLIXML") {
		return nil, fmt.Errorf("Komut hatası: %s", stderr)
	}

	// Return basic info (hostname command output)
	info := &HardwareInfo{
		Hostname:     strings.TrimSpace(stdout),
		CollectedAt:  time.Now(),
		CollectionMs: time.Since(startTime).Milliseconds(),
	}

	return info, nil
}

// CollectHardwareInfo collects comprehensive hardware information from a Windows machine
func (c *WinRMClient) CollectHardwareInfo(targetIP string) (*HardwareInfo, error) {
	startTime := time.Now()
	info := &HardwareInfo{
		CollectedAt: time.Now(),
		Errors:      []string{},
	}

	// Create WinRM endpoint
	endpoint := winrm.NewEndpoint(
		targetIP,
		c.Port,
		c.UseSSL,
		true,
		nil, nil, nil,
		time.Duration(c.TimeoutSeconds)*time.Second,
	)

	client, err := winrm.NewClient(endpoint, c.Username, c.Password)
	if err != nil {
		return nil, fmt.Errorf("WinRM client oluşturulamadı: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(c.TimeoutSeconds)*time.Second)
	defer cancel()

	// PowerShell script to collect all hardware info at once
	psScript := `
$ErrorActionPreference = 'SilentlyContinue'

$cs = Get-WmiObject Win32_ComputerSystem | Select-Object Manufacturer, Model, SystemType, TotalPhysicalMemory
$bios = Get-WmiObject Win32_BIOS | Select-Object SerialNumber, SMBIOSBIOSVersion, Manufacturer
$cpu = Get-WmiObject Win32_Processor | Select-Object Name, NumberOfCores, NumberOfLogicalProcessors, MaxClockSpeed -First 1
$os = Get-WmiObject Win32_OperatingSystem | Select-Object Caption, Version, BuildNumber
$disks = Get-WmiObject Win32_DiskDrive | Select-Object Model, Size, MediaType, SerialNumber
$ram = Get-WmiObject Win32_PhysicalMemory | Select-Object Capacity, Speed, Manufacturer, PartNumber
$monitors = Get-WmiObject WmiMonitorID -Namespace root\wmi | ForEach-Object {
    @{
        SerialNumber = [System.Text.Encoding]::ASCII.GetString($_.SerialNumberID).Trim([char]0)
        Manufacturer = [System.Text.Encoding]::ASCII.GetString($_.ManufacturerName).Trim([char]0)
        Name = [System.Text.Encoding]::ASCII.GetString($_.UserFriendlyName).Trim([char]0)
    }
}

@{
    ComputerSystem = $cs
    BIOS = $bios
    CPU = $cpu
    OS = $os
    Disks = @($disks)
    RAM = @($ram)
    Monitors = @($monitors)
} | ConvertTo-Json -Depth 3
`

	stdout, stderr, _, err := client.RunWithContextWithString(ctx, "powershell -NoProfile -Command \""+strings.ReplaceAll(psScript, "\"", "`\"")+"\"", "")
	if err != nil {
		info.Errors = append(info.Errors, fmt.Sprintf("PowerShell hatası: %v", err))
		info.CollectionMs = time.Since(startTime).Milliseconds()
		return info, err
	}

	if stderr != "" && !strings.Contains(stderr, "CLIXML") {
		info.Errors = append(info.Errors, fmt.Sprintf("PowerShell stderr: %s", stderr))
	}

	// Parse JSON output
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		info.Errors = append(info.Errors, fmt.Sprintf("JSON parse hatası: %v", err))
		info.CollectionMs = time.Since(startTime).Milliseconds()
		return info, nil
	}

	// Parse Computer System
	if cs, ok := result["ComputerSystem"].(map[string]interface{}); ok {
		info.Manufacturer = getString(cs, "Manufacturer")
		info.Model = getString(cs, "Model")
		info.SystemType = getString(cs, "SystemType")
		if mem, ok := cs["TotalPhysicalMemory"].(float64); ok {
			info.TotalRAMGB = mem / (1024 * 1024 * 1024)
		}
	}

	// Parse BIOS
	if bios, ok := result["BIOS"].(map[string]interface{}); ok {
		info.SerialNumber = getString(bios, "SerialNumber")
		info.BIOSVersion = getString(bios, "SMBIOSBIOSVersion")
		info.BIOSVendor = getString(bios, "Manufacturer")
	}

	// Parse CPU
	if cpu, ok := result["CPU"].(map[string]interface{}); ok {
		info.CPUName = getString(cpu, "Name")
		info.CPUCores = getInt(cpu, "NumberOfCores")
		info.CPUThreads = getInt(cpu, "NumberOfLogicalProcessors")
		info.CPUSpeed = getInt(cpu, "MaxClockSpeed")
	}

	// Parse OS
	if osInfo, ok := result["OS"].(map[string]interface{}); ok {
		info.OSName = getString(osInfo, "Caption")
		info.OSVersion = getString(osInfo, "Version")
		info.OSBuild = getString(osInfo, "BuildNumber")
	}

	// Parse Disks
	if disks, ok := result["Disks"].([]interface{}); ok {
		for _, d := range disks {
			if disk, ok := d.(map[string]interface{}); ok {
				sizeGB := 0.0
				if size, ok := disk["Size"].(float64); ok {
					sizeGB = size / (1024 * 1024 * 1024)
				}
				info.Disks = append(info.Disks, PhysicalDisk{
					Model:        getString(disk, "Model"),
					SizeGB:       sizeGB,
					MediaType:    getString(disk, "MediaType"),
					SerialNumber: getString(disk, "SerialNumber"),
				})
				info.TotalDiskGB += sizeGB
			}
		}
	}

	// Parse RAM modules
	if rams, ok := result["RAM"].([]interface{}); ok {
		info.RAMSlots = len(rams)
		for _, r := range rams {
			if ram, ok := r.(map[string]interface{}); ok {
				capacityGB := int64(0)
				if cap, ok := ram["Capacity"].(float64); ok {
					capacityGB = int64(cap / (1024 * 1024 * 1024))
				}
				info.RAMModules = append(info.RAMModules, RAMModule{
					Capacity:     capacityGB,
					Speed:        getInt(ram, "Speed"),
					Manufacturer: getString(ram, "Manufacturer"),
					PartNumber:   strings.TrimSpace(getString(ram, "PartNumber")),
				})
			}
		}
	}

	// Parse Monitors
	if monitors, ok := result["Monitors"].([]interface{}); ok {
		for _, m := range monitors {
			if mon, ok := m.(map[string]interface{}); ok {
				info.Monitors = append(info.Monitors, MonitorInfo{
					Name:         getString(mon, "Name"),
					Manufacturer: getString(mon, "Manufacturer"),
					SerialNumber: getString(mon, "SerialNumber"),
				})
			}
		}
	}

	info.CollectionMs = time.Since(startTime).Milliseconds()
	return info, nil
}

// Helper functions
func getString(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return cleanString(v)
	}
	return ""
}

func getInt(m map[string]interface{}, key string) int {
	if v, ok := m[key].(float64); ok {
		return int(v)
	}
	return 0
}

func cleanString(s string) string {
	// Remove control characters and trim
	re := regexp.MustCompile(`[\x00-\x1F\x7F]`)
	return strings.TrimSpace(re.ReplaceAllString(s, ""))
}

// FormatRAMSize formats RAM size for display
func FormatRAMSize(gb float64) string {
	if gb >= 1 {
		return fmt.Sprintf("%.0f GB", gb)
	}
	return fmt.Sprintf("%.0f MB", gb*1024)
}

// FormatDiskSize formats disk size for display
func FormatDiskSize(gb float64) string {
	if gb >= 1000 {
		return fmt.Sprintf("%.1f TB", gb/1000)
	}
	return fmt.Sprintf("%.0f GB", gb)
}

// ParseSerialFromBIOS extracts clean serial number
func ParseSerialFromBIOS(serial string) string {
	// Clean up common placeholder values
	serial = strings.TrimSpace(serial)
	placeholders := []string{"To Be Filled By O.E.M.", "Default string", "None", "N/A", "0", ""}
	for _, p := range placeholders {
		if strings.EqualFold(serial, p) {
			return ""
		}
	}
	return serial
}

// ToJSON converts HardwareInfo to JSON string
func (h *HardwareInfo) ToJSON() string {
	data, err := json.Marshal(h)
	if err != nil {
		return "{}"
	}
	return string(data)
}

// GetSummary returns a human-readable summary
func (h *HardwareInfo) GetSummary() string {
	return fmt.Sprintf("%s %s | CPU: %s (%d cores) | RAM: %.0f GB | Disk: %.0f GB",
		h.Manufacturer, h.Model, h.CPUName, h.CPUCores, h.TotalRAMGB, h.TotalDiskGB)
}

// EnrichmentResult contains the result of a single enrichment attempt
type EnrichmentResult struct {
	IP           string        `json:"ip"`
	Success      bool          `json:"success"`
	HardwareInfo *HardwareInfo `json:"hardware_info,omitempty"`
	Error        *WinRMError   `json:"error,omitempty"`
	DurationMs   int64         `json:"duration_ms"`
}

// BatchEnrichmentResult contains the overall batch enrichment results
type BatchEnrichmentResult struct {
	TotalTargets   int                          `json:"total_targets"`
	SuccessCount   int                          `json:"success_count"`
	ErrorCount     int                          `json:"error_count"`
	Results        map[string]*EnrichmentResult `json:"results"`
	TotalDurationMs int64                       `json:"total_duration_ms"`
}

// EnrichInventoryItems collects hardware info from multiple targets concurrently
func (c *WinRMClient) EnrichInventoryItems(targetIPs []string) *BatchEnrichmentResult {
	startTime := time.Now()

	result := &BatchEnrichmentResult{
		TotalTargets: len(targetIPs),
		Results:      make(map[string]*EnrichmentResult),
	}

	if len(targetIPs) == 0 {
		return result
	}

	// Determine concurrent limit (default 5 if not set)
	concurrentLimit := c.ConcurrentLimit
	if concurrentLimit <= 0 {
		concurrentLimit = 5
	}

	// Use semaphore pattern for concurrency control
	semaphore := make(chan struct{}, concurrentLimit)
	var wg sync.WaitGroup
	var mu sync.Mutex // Protects results map

	for _, ip := range targetIPs {
		wg.Add(1)
		go func(targetIP string) {
			defer wg.Done()

			// Acquire semaphore
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			itemStart := time.Now()
			info, err := c.CollectHardwareInfo(targetIP)
			duration := time.Since(itemStart).Milliseconds()

			enrichResult := &EnrichmentResult{
				IP:         targetIP,
				DurationMs: duration,
			}

			if err != nil {
				winrmErr := ClassifyError(err)
				enrichResult.Success = false
				enrichResult.Error = &winrmErr

				// Also store error in HardwareInfo for backward compatibility
				enrichResult.HardwareInfo = &HardwareInfo{
					Errors:       []string{err.Error()},
					CollectedAt:  time.Now(),
					CollectionMs: duration,
				}
			} else {
				enrichResult.Success = true
				enrichResult.HardwareInfo = info
			}

			mu.Lock()
			result.Results[targetIP] = enrichResult
			if enrichResult.Success {
				result.SuccessCount++
			} else {
				result.ErrorCount++
			}
			mu.Unlock()
		}(ip)
	}

	wg.Wait()
	result.TotalDurationMs = time.Since(startTime).Milliseconds()

	return result
}

// TestConnectionWithCategory tests connection and returns categorized error
func (c *WinRMClient) TestConnectionWithCategory(targetIP string) (*HardwareInfo, *WinRMError) {
	info, err := c.TestConnection(targetIP)
	if err != nil {
		winrmErr := ClassifyError(err)
		return nil, &winrmErr
	}
	return info, nil
}

// CheckWinRMReachable performs a quick TCP check before WinRM connection
func (c *WinRMClient) CheckWinRMReachable(targetIP string) error {
	address := fmt.Sprintf("%s:%d", targetIP, c.Port)
	timeout := time.Duration(5) * time.Second

	conn, err := net.DialTimeout("tcp", address, timeout)
	if err != nil {
		return fmt.Errorf("WinRM portu erişilemez (%d): %v", c.Port, err)
	}
	conn.Close()
	return nil
}
