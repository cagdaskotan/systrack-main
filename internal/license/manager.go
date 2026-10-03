package license

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/shirou/gopsutil/v3/net"
)

// Manager handles license operations
type Manager struct {
	db              *sql.DB
	license         *License
	macAddress      string
	stopChan        chan struct{}
	activationMode  bool // True if device is waiting for activation
	blockedMode     bool // True if device is blocked (piracy detection)
	expiredMode     bool // True if device license has expired
	heartbeatSender *HeartbeatSender
}

// NewManager creates a new license manager
func NewManager(db *sql.DB) (*Manager, error) {
	// Get MAC address
	macAddress, err := getFirstMACAddress()
	if err != nil {
		return nil, fmt.Errorf("failed to get MAC address: %w", err)
	}

	log.Printf("📌 MAC Address: %s", macAddress)

	return &Manager{
		db:         db,
		macAddress: macAddress,
		stopChan:   make(chan struct{}),
	}, nil
}

// Initialize loads license from database and validates
func (m *Manager) Initialize() error {
	license, err := m.loadLicenseFromDB()
	if err != nil {
		if err == sql.ErrNoRows {
			log.Println("⚠️  No license found in database")
			m.activationMode = true
			m.startActivationPolling()

			// Start heartbeat sender even in activation mode (to receive activation confirmation)
			heartbeatSender, err := NewHeartbeatSender(m.db, m.macAddress)
			if err != nil {
				log.Printf("⚠️  Failed to create heartbeat sender: %v", err)
			} else {
				m.heartbeatSender = heartbeatSender
				heartbeatSender.Start()
				log.Println("💓 Heartbeat started in activation mode (waiting for confirmation)")
			}

			return nil
		}
		return fmt.Errorf("failed to load license: %w", err)
	}

	m.license = license

	// If activation timestamp exists but status is still uninitialized, treat as active.
	if license.ActivatedAt.Valid && license.Status == StatusUninitialized {
		_, err := m.db.Exec(`
			UPDATE device_license
			SET status = ?
			WHERE id = ?
		`, StatusActive, license.ID)
		if err != nil {
			log.Printf("Failed to promote license status to active: %v", err)
		} else {
			license.Status = StatusActive
			m.license.Status = StatusActive
		}
	}

	// Check if device is blocked (piracy detection or admin action)
	if license.Status == StatusBlocked {
		log.Println("🚨 Device is BLOCKED - access denied")
		m.blockedMode = true

		// Start heartbeat sender to receive unblock command
		heartbeatSender, err := NewHeartbeatSender(m.db, m.macAddress)
		if err != nil {
			log.Printf("⚠️  Failed to create heartbeat sender: %v", err)
		} else {
			m.heartbeatSender = heartbeatSender
			heartbeatSender.SetBlocked(true, "piracy_detected")
			heartbeatSender.Start()
			log.Println("💓 Heartbeat started in blocked mode (waiting for unblock)")
		}

		return nil
	}

	// Check if device license has expired
	if license.Status == StatusExpired {
		log.Println("⏰ Device license EXPIRED - access denied until renewal")
		m.expiredMode = true

		// Start heartbeat sender to receive renewal notification
		heartbeatSender, err := NewHeartbeatSender(m.db, m.macAddress)
		if err != nil {
			log.Printf("⚠️  Failed to create heartbeat sender: %v", err)
		} else {
			m.heartbeatSender = heartbeatSender
			heartbeatSender.SetExpired(true)
			heartbeatSender.Start()
			log.Println("💓 Heartbeat started in expired mode (waiting for renewal)")
		}

		return nil
	}

	// Check if status is 0 (uninitialized) - needs activation
	if license.Status == StatusUninitialized {
		log.Println("🔒 Device not activated yet - activation required")

		// Auto-detect and write MAC address if empty (first boot)
		if license.MACAddress == "" || license.MACAddress == "00:00:00:00:00:00" {
			log.Printf("📝 First boot detected - writing MAC address to database")
			log.Printf("   Detected MAC: %s", m.macAddress)

			_, err := m.db.Exec(`
				UPDATE device_license
				SET mac_address = ?
				WHERE id = ?
			`, m.macAddress, license.ID)

			if err != nil {
				log.Printf("⚠️  Failed to update MAC address: %v", err)
				// Don't fail - continue with activation
			} else {
				log.Printf("✅ MAC address written to database successfully")
				license.MACAddress = m.macAddress
				m.license.MACAddress = m.macAddress
			}
		}

		m.activationMode = true
		m.startActivationPolling()

		// Start heartbeat sender even in activation mode (to receive activation confirmation)
		heartbeatSender, err := NewHeartbeatSender(m.db, m.macAddress)
		if err != nil {
			log.Printf("⚠️  Failed to create heartbeat sender: %v", err)
		} else {
			m.heartbeatSender = heartbeatSender
			heartbeatSender.Start()
			log.Println("💓 Heartbeat started in activation mode (waiting for confirmation)")
		}

		return nil
	}

	// Validate MAC address - CRITICAL SECURITY CHECK
	// MAC is pre-configured during manufacturing and must match
	if license.MACAddress != "" && license.MACAddress != m.macAddress {
		log.Printf("🚨 MAC address mismatch detected!")
		log.Printf("   Database MAC: %s", license.MACAddress)
		log.Printf("   System MAC:   %s", m.macAddress)
		return fmt.Errorf("MAC address mismatch - hardware changed or unauthorized device")
	}

	log.Printf("✅ License loaded: Serial=%s, Status=%s",
		license.SerialNumber, license.GetStatusName())

	m.activationMode = false

	// Start heartbeat sender for activated devices
	if !m.activationMode {
		heartbeatSender, err := NewHeartbeatSender(m.db, m.macAddress)
		if err != nil {
			log.Printf("⚠️  Failed to create heartbeat sender: %v", err)
			// Don't fail initialization, just skip heartbeat
			return nil
		}
		m.heartbeatSender = heartbeatSender
		heartbeatSender.Start()
	}

	return nil
}

// loadLicenseFromDB loads license from database
func (m *Manager) loadLicenseFromDB() (*License, error) {
	var license License

	err := m.db.QueryRow(`
		SELECT id, serial_number, mac_address, status,
		       customer_email, customer_company, activated_at, created_at
		FROM device_license
		LIMIT 1
	`).Scan(
		&license.ID,
		&license.SerialNumber,
		&license.MACAddress,
		&license.Status,
		&license.CustomerEmail,
		&license.CustomerCompany,
		&license.ActivatedAt,
		&license.CreatedAt,
	)

	if err != nil {
		return nil, err
	}

	return &license, nil
}

// ensureActivationState reloads the latest license info so UI reacts to DB changes
func (m *Manager) ensureActivationState() {
	license, err := m.loadLicenseFromDB()
	if err != nil {
		if err == sql.ErrNoRows {
			if !m.activationMode {
				log.Println("🔁 No license row found - switching to activation mode")
			}
			m.activationMode = true
			m.license = nil
			return
		}
		log.Printf("⚠️ License refresh failed: %v", err)
		return
	}

	m.license = license
	if license.ActivatedAt.Valid {
		m.activationMode = false
		return
	}
	switch license.Status {
	case StatusActive, StatusTrial:
		m.activationMode = false
	default:
		if !m.activationMode {
			log.Printf("🔐 License status %s - activation required", license.GetStatusName())
		}
		m.activationMode = true
	}
}

// IsActivationMode returns true if device is in activation mode
func (m *Manager) IsActivationMode() bool {
	m.ensureActivationState()
	return m.activationMode
}

// IsBlocked returns true if device is blocked (piracy detection or admin action)
func (m *Manager) IsBlocked() bool {
	// Check current state from database
	var status int
	err := m.db.QueryRow(`SELECT status FROM device_license LIMIT 1`).Scan(&status)
	if err != nil {
		return m.blockedMode
	}

	m.blockedMode = (status == StatusBlocked)
	return m.blockedMode
}

// IsExpired returns true if device license has expired
func (m *Manager) IsExpired() bool {
	// Check current state from database
	var status int
	err := m.db.QueryRow(`SELECT status FROM device_license LIMIT 1`).Scan(&status)
	if err != nil {
		return m.expiredMode
	}

	m.expiredMode = (status == StatusExpired)
	return m.expiredMode
}

// startActivationPolling checks periodically if device has been activated
func (m *Manager) startActivationPolling() {
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		log.Println("🔄 Started activation polling (checking every 30 seconds)...")

		for {
			select {
			case <-ticker.C:
				// Check local database first
				license, err := m.loadLicenseFromDB()
				if err != nil {
					log.Printf("⚠️  Activation poll failed: %v", err)
					continue
				}

				// If already activated locally, stop polling
				if license.Status == StatusActive || license.Status == StatusTrial {
					log.Println("🎉 Device activated! Reloading license...")
					m.license = license
					m.activationMode = false
					return
				}

				// Check management server for activation status
				if err := m.checkManagementServerActivation(); err != nil {
					log.Printf("⚠️  Management server check failed: %v", err)
					continue
				}

			case <-m.stopChan:
				log.Println("🛑 Stopping activation polling...")
				return
			}
		}
	}()
}

// checkManagementServerActivation queries management server for activation status
func (m *Manager) checkManagementServerActivation() error {
	// Get serial number
	var serialNumber string
	err := m.db.QueryRow(`SELECT serial_number FROM device_license LIMIT 1`).Scan(&serialNumber)
	if err != nil {
		return fmt.Errorf("failed to get serial number: %w", err)
	}

	// Get management server URL from environment
	managementURL := getManagementServerURL()
	if managementURL == "" {
		return fmt.Errorf("MANAGEMENT_SERVER_URL not configured")
	}

	// Query management server
	url := fmt.Sprintf("%s/api/v1/activation/status/%s", managementURL, serialNumber)
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("management server returned status %d", resp.StatusCode)
	}

	// Parse response
	var result struct {
		Success   bool   `json:"success"`
		Status    string `json:"status"`
		Activated bool   `json:"activated"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	// If activated on management server, update local database
	if result.Activated && result.Status == "email_confirmed" {
		log.Printf("✅ Activation confirmed by management server! Updating local database...")

		_, err = m.db.Exec(`
			UPDATE device_license
			SET status = ?, activated_at = NOW()
			WHERE serial_number = ?
		`, StatusActive, serialNumber)

		if err != nil {
			return fmt.Errorf("failed to update local database: %w", err)
		}

		// Update internal state
		m.activationMode = false

		// Reload license
		license, err := m.loadLicenseFromDB()
		if err == nil {
			m.license = license
		}

		// Start heartbeat sender now that device is activated
		if m.heartbeatSender == nil {
			heartbeatSender, err := NewHeartbeatSender(m.db, m.macAddress)
			if err != nil {
				log.Printf("⚠️  Failed to create heartbeat sender: %v", err)
			} else {
				m.heartbeatSender = heartbeatSender
				heartbeatSender.Start()
			}
		}

		log.Println("🎉 Device activated successfully!")
		return nil
	}

	return nil
}

// GetLicense returns current license
func (m *Manager) GetLicense() *License {
	return m.license
}

// GetMACAddress returns device MAC address
func (m *Manager) GetMACAddress() string {
	return m.macAddress
}

// Stop stops the license manager
func (m *Manager) Stop() {
	// Stop heartbeat sender if running
	if m.heartbeatSender != nil {
		m.heartbeatSender.Stop()
	}
	close(m.stopChan)
}

// getFirstMACAddress gets the first non-loopback MAC address
func getFirstMACAddress() (string, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return "", err
	}

	for _, iface := range interfaces {
		// Skip loopback and empty MAC addresses
		// gopsutil returns HardwareAddr as string (e.g., "da:f3:bc:bc:52:59")
		if iface.HardwareAddr != "" && iface.HardwareAddr != "00:00:00:00:00:00" {
			log.Printf("🔍 Found MAC: %s (interface: %s)", iface.HardwareAddr, iface.Name)
			return iface.HardwareAddr, nil
		}
	}

	return "", fmt.Errorf("no valid MAC address found")
}

// GetCurrentMACAddress is a public wrapper for getFirstMACAddress
func GetCurrentMACAddress() (string, error) {
	return getFirstMACAddress()
}

// getManagementServerURL returns management server URL from environment
func getManagementServerURL() string {
	url := os.Getenv("MANAGEMENT_SERVER_URL")
	if url == "" {
		return DefaultManagementServerURL
	}
	return url
}

// ManagementServerURL exposes current management server URL for other packages
func ManagementServerURL() string {
	return getManagementServerURL()
}
