package services

import (
	"context"
	"fmt"
	"log"
	"strings"
)

// LinuxService represents a Linux/Unix service
type LinuxService struct {
	Name        string
	DisplayName string
	Description string
	Status      string // running, stopped, unknown
	StartupType string // enabled, disabled, static
	PID         int
}

// LinuxServiceReader reads services from Linux machines via SSH
type LinuxServiceReader struct {
	sshDiscovery *SSHDiscovery
}

// NewLinuxServiceReader creates a new Linux service reader
func NewLinuxServiceReader() *LinuxServiceReader {
	return &LinuxServiceReader{
		sshDiscovery: NewSSHDiscovery(),
	}
}

// ReadServices reads all services from a Linux machine using systemctl
func (lsr *LinuxServiceReader) ReadServices(ctx context.Context, address, username, password string) ([]LinuxService, error) {
	log.Printf("[Linux Service Reader] Reading services from %s", address)

	// Use SSH to run: systemctl list-units --type=service --all --no-pager --plain
	services, err := lsr.sshDiscovery.GetAllLinuxServices(ctx, address, username, password)
	if err != nil {
		return nil, fmt.Errorf("failed to read Linux services: %w", err)
	}

	log.Printf("[Linux Service Reader] Found %d services on %s", len(services), address)
	return services, nil
}

// CheckServiceStatus checks the status of a specific Linux service
func (lsr *LinuxServiceReader) CheckServiceStatus(ctx context.Context, address, username, password, serviceName string) (string, error) {
	status, err := lsr.sshDiscovery.CheckLinuxService(ctx, address, username, password, serviceName)
	if err != nil {
		return "unknown", err
	}
	return status, nil
}

// NormalizeStatus converts various status formats to our standard: running, stopped, unknown
func (lsr *LinuxServiceReader) NormalizeStatus(status string) string {
	status = strings.ToLower(strings.TrimSpace(status))
	switch status {
	case "running", "active", "started":
		return "running"
	case "stopped", "inactive", "dead", "failed":
		return "stopped"
	default:
		return "unknown"
	}
}

// NormalizeStartupType converts systemd startup types to standard values
func (lsr *LinuxServiceReader) NormalizeStartupType(startType string) string {
	startType = strings.ToLower(strings.TrimSpace(startType))
	switch startType {
	case "enabled":
		return "Enabled"
	case "disabled":
		return "Disabled"
	case "static":
		return "Static"
	case "masked":
		return "Masked"
	default:
		return "Unknown"
	}
}

// TestConnection tests SSH connectivity
func (lsr *LinuxServiceReader) TestConnection(ctx context.Context, address, username, password string) error {
	return lsr.sshDiscovery.QuickSSHCheck(ctx, address, username, password)
}
