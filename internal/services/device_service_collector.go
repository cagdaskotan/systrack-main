package services

import (
	"context"
	"database/sql"
	"log"
	"time"
)

// DeviceServiceCollector collects service information from devices with credentials
// Runs periodically (default: every 10 minutes)
type DeviceServiceCollector struct {
	db               *sql.DB
	interval         time.Duration
	stopChan         chan struct{}
	windowsReader    *WindowsServiceReader
	linuxReader      *LinuxServiceReader
	credentialCrypto *CredentialCrypto
}

// NewDeviceServiceCollector creates a new service collector
func NewDeviceServiceCollector(db *sql.DB, interval time.Duration) *DeviceServiceCollector {
	if interval <= 0 {
		interval = 10 * time.Minute // Default: 10 minutes
	}

	crypto, err := NewCredentialCrypto()
	if err != nil {
		log.Printf("⚠️  Failed to initialize credential crypto: %v", err)
		crypto = nil
	}

	return &DeviceServiceCollector{
		db:               db,
		interval:         interval,
		stopChan:         make(chan struct{}),
		windowsReader:    NewWindowsServiceReader(),
		linuxReader:      NewLinuxServiceReader(),
		credentialCrypto: crypto,
	}
}

// Start starts the background collector
func (dsc *DeviceServiceCollector) Start() {
	go func() {
		ticker := time.NewTicker(dsc.interval)
		defer ticker.Stop()

		log.Printf("🔄 Device Service Collector started (interval: %s)", dsc.interval)

		// Run immediately on start
		dsc.CollectAll()

		for {
			select {
			case <-ticker.C:
				dsc.CollectAll()
			case <-dsc.stopChan:
				log.Println("🛑 Device Service Collector stopped")
				return
			}
		}
	}()
}

// Stop stops the collector
func (dsc *DeviceServiceCollector) Stop() {
	close(dsc.stopChan)
}

// CollectAll collects services from all targets with credentials
func (dsc *DeviceServiceCollector) CollectAll() {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	log.Println("📊 Starting device service collection cycle...")

	// Get all targets with credentials
	query := `
		SELECT tc.target_id, t.address, tc.os_type, tc.protocol,
		       tc.username, tc.password_encrypted, tc.domain, tc.port
		FROM target_credentials tc
		INNER JOIN targets t ON tc.target_id = t.id
		WHERE t.enabled = 1
	`

	rows, err := dsc.db.QueryContext(ctx, query)
	if err != nil {
		log.Printf("❌ Failed to query targets with credentials: %v", err)
		return
	}
	defer rows.Close()

	totalTargets := 0
	successCount := 0
	failCount := 0

	for rows.Next() {
		var targetID int
		var address, osType, protocol, username, passwordEnc, domain string
		var port sql.NullInt64

		err := rows.Scan(&targetID, &address, &osType, &protocol, &username, &passwordEnc, &domain, &port)
		if err != nil {
			log.Printf("❌ Failed to scan target row: %v", err)
			continue
		}

		totalTargets++

		// Decrypt password
		password, err := dsc.decryptPassword(passwordEnc)
		if err != nil {
			log.Printf("❌ Failed to decrypt password for target %d (%s): %v", targetID, address, err)
			failCount++
			continue
		}

		// Collect services based on OS type
		err = dsc.CollectForTarget(ctx, targetID, address, osType, username, password, domain)
		if err != nil {
			log.Printf("❌ Failed to collect services for target %d (%s): %v", targetID, address, err)
			failCount++
		} else {
			successCount++
		}
	}

	log.Printf("✅ Service collection cycle completed: %d targets (%d success, %d failed)",
		totalTargets, successCount, failCount)
}

// CollectForTarget collects services from a single target
func (dsc *DeviceServiceCollector) CollectForTarget(ctx context.Context, targetID int, address, osType, username, password, domain string) error {
	log.Printf("🔍 Collecting services for target %d (%s, OS: %s)", targetID, address, osType)

	var services []interface{}
	var err error

	switch osType {
	case "windows":
		winServices, winErr := dsc.windowsReader.ReadServices(ctx, address, username, password, domain)
		if winErr != nil {
			return winErr
		}
		// Convert to generic interface
		for _, svc := range winServices {
			services = append(services, svc)
		}

	case "linux":
		linuxServices, linuxErr := dsc.linuxReader.ReadServices(ctx, address, username, password)
		if linuxErr != nil {
			return linuxErr
		}
		// Convert to generic interface
		for _, svc := range linuxServices {
			services = append(services, svc)
		}

	default:
		return nil // Unknown OS type, skip
	}

	// Save services to database
	err = dsc.saveServices(ctx, targetID, osType, services)
	if err != nil {
		return err
	}

	log.Printf("✅ Saved %d services for target %d (%s)", len(services), targetID, address)
	return nil
}

// saveServices saves collected services to the database
func (dsc *DeviceServiceCollector) saveServices(ctx context.Context, targetID int, osType string, services []interface{}) error {
	tx, err := dsc.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	now := time.Now()

	for _, svc := range services {
		var serviceName, displayName, description, status, startupType string
		var pid int

		// Extract fields based on service type
		switch osType {
		case "windows":
			winSvc, ok := svc.(WindowsService)
			if !ok {
				continue
			}
			serviceName = winSvc.Name
			displayName = winSvc.DisplayName
			description = winSvc.Description
			status = dsc.windowsReader.NormalizeStatus(winSvc.State)
			startupType = dsc.windowsReader.NormalizeStartupType(winSvc.StartMode)
			pid = winSvc.ProcessID

		case "linux":
			linuxSvc, ok := svc.(LinuxService)
			if !ok {
				continue
			}
			serviceName = linuxSvc.Name
			displayName = linuxSvc.DisplayName
			description = linuxSvc.Description
			status = dsc.linuxReader.NormalizeStatus(linuxSvc.Status)
			startupType = dsc.linuxReader.NormalizeStartupType(linuxSvc.StartupType)
			pid = linuxSvc.PID
		}

		// Check if service already exists
		var existingID int64
		var existingStatus string
		checkQuery := `SELECT id, status FROM device_services WHERE target_id = ? AND service_name = ?`
		err := tx.QueryRowContext(ctx, checkQuery, targetID, serviceName).Scan(&existingID, &existingStatus)

		if err == sql.ErrNoRows {
			// Insert new service
			insertQuery := `
				INSERT INTO device_services
				(target_id, service_name, display_name, description, status, startup_type, pid, last_checked, is_monitored)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, FALSE)
			`
			_, err = tx.ExecContext(ctx, insertQuery, targetID, serviceName, displayName, description, status, startupType, pid, now)
			if err != nil {
				log.Printf("⚠️  Failed to insert service %s: %v", serviceName, err)
			}
		} else if err == nil {
			// Update existing service
			updateQuery := `
				UPDATE device_services
				SET display_name = ?, description = ?, status = ?, startup_type = ?, pid = ?, last_checked = ?, updated_at = ?
				WHERE id = ?
			`
			_, err = tx.ExecContext(ctx, updateQuery, displayName, description, status, startupType, pid, now, now, existingID)
			if err != nil {
				log.Printf("⚠️  Failed to update service %s: %v", serviceName, err)
			}

			// If status changed, record in history
			if existingStatus != status {
				historyQuery := `
					INSERT INTO device_service_history
					(service_id, target_id, service_name, old_status, new_status, changed_at)
					VALUES (?, ?, ?, ?, ?, ?)
				`
				_, err = tx.ExecContext(ctx, historyQuery, existingID, targetID, serviceName, existingStatus, status, now)
				if err != nil {
					log.Printf("⚠️  Failed to insert service history for %s: %v", serviceName, err)
				} else {
					log.Printf("📝 Service status changed: %s (%s → %s)", serviceName, existingStatus, status)
				}
			}
		}
	}

	return tx.Commit()
}

// decryptPassword decrypts an encrypted password
func (dsc *DeviceServiceCollector) decryptPassword(encrypted string) (string, error) {
	if dsc.credentialCrypto == nil {
		// Fallback to global crypto
		return DecryptPassword(encrypted)
	}
	return dsc.credentialCrypto.Decrypt(encrypted)
}

// CollectNow triggers immediate collection for a specific target (on-demand)
func (dsc *DeviceServiceCollector) CollectNow(targetID int) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Get target credentials
	var address, osType, protocol, username, passwordEnc, domain string
	var port sql.NullInt64

	query := `
		SELECT t.address, tc.os_type, tc.protocol, tc.username, tc.password_encrypted, tc.domain, tc.port
		FROM targets t
		INNER JOIN target_credentials tc ON t.id = tc.target_id
		WHERE t.id = ? AND t.enabled = 1
	`

	err := dsc.db.QueryRowContext(ctx, query, targetID).Scan(&address, &osType, &protocol, &username, &passwordEnc, &domain, &port)
	if err != nil {
		return err
	}

	// Decrypt password
	password, err := dsc.decryptPassword(passwordEnc)
	if err != nil {
		return err
	}

	// Collect services
	return dsc.CollectForTarget(ctx, targetID, address, osType, username, password, domain)
}
