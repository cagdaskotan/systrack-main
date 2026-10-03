package services

import (
	"context"
	"fmt"
	"log"
	"strings"
)

// WindowsService represents a Windows service
type WindowsService struct {
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
	Description string `json:"description"`
	State       string `json:"state"`        // Running, Stopped, Paused (raw from WMI)
	Status      string `json:"status"`       // running, stopped, unknown (normalized)
	StartMode   string `json:"start_mode"`   // Auto, Manual, Disabled (raw from WMI)
	StartupType string `json:"startup_type"` // Auto, Manual, Disabled (normalized)
	ProcessID   int    `json:"process_id"`
	PID         int    `json:"pid"` // Alias for ProcessID
}

// WindowsServiceReader reads services from Windows machines via WinRM
type WindowsServiceReader struct {
	wmiDiscovery *WMIDiscovery
}

// NewWindowsServiceReader creates a new Windows service reader
func NewWindowsServiceReader() *WindowsServiceReader {
	return &WindowsServiceReader{
		wmiDiscovery: NewWMIDiscovery(),
	}
}

// ReadServices reads all services from a Windows machine
func (wsr *WindowsServiceReader) ReadServices(ctx context.Context, address, username, password, domain string) ([]WindowsService, error) {
	log.Printf("[Windows Service Reader] Reading services from %s", address)

	// Use WMI to query Windows services
	// PowerShell command: Get-Service | Select-Object Name, DisplayName, Status, StartType
	// Or WMI query: SELECT Name, DisplayName, State, StartMode FROM Win32_Service

	services, err := wsr.wmiDiscovery.GetAllWindowsServices(ctx, address, username, password, domain)
	if err != nil {
		return nil, fmt.Errorf("failed to read Windows services: %w", err)
	}

	log.Printf("[Windows Service Reader] Found %d services on %s", len(services), address)
	return services, nil
}

// CheckServiceStatus checks the status of a specific Windows service
func (wsr *WindowsServiceReader) CheckServiceStatus(ctx context.Context, address, username, password, domain, serviceName string) (string, error) {
	status, err := wsr.wmiDiscovery.CheckWindowsService(ctx, address, username, password, domain, serviceName)
	if err != nil {
		return "unknown", err
	}
	return status, nil
}

// NormalizeStatus converts various status formats to our standard: running, stopped, unknown
func (wsr *WindowsServiceReader) NormalizeStatus(status string) string {
	status = strings.ToLower(strings.TrimSpace(status))
	switch status {
	case "running", "started", "active":
		return "running"
	case "stopped", "inactive", "paused":
		return "stopped"
	default:
		return "unknown"
	}
}

// NormalizeStartupType converts various startup type formats to standard values
func (wsr *WindowsServiceReader) NormalizeStartupType(startType string) string {
	startType = strings.ToLower(strings.TrimSpace(startType))
	switch startType {
	case "auto", "automatic", "autostart":
		return "Auto"
	case "manual":
		return "Manual"
	case "disabled":
		return "Disabled"
	case "boot", "system":
		return startType // Keep as is for system services
	default:
		return "Unknown"
	}
}

// TestConnection tests WinRM connectivity
func (wsr *WindowsServiceReader) TestConnection(ctx context.Context, address, username, password, domain string) error {
	return wsr.wmiDiscovery.QuickWMICheck(ctx, address, username, password, domain)
}
