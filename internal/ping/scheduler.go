package ping

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"systrack/internal/ws"
)

// Sabit ping ayarları - artık kullanıcı tarafından değiştirilemez
const (
	PING_INTERVAL_SEC = 120 // 2 dakika - tüm hedefler için sabit
	PING_TIMEOUT_SEC  = 30  // 30 saniye timeout - false positive'leri önlemek için yeterli
)

// Scheduler manages periodic ping operations
type Scheduler struct {
	db             *sql.DB
	engine         PingEngine
	hub            *ws.Hub
	config         SchedulerConfig
	running        bool
	stopChan       chan struct{}
	wg             sync.WaitGroup
	targetStates   map[int]*TargetState
	mu             sync.RWMutex
	onStatusChange func(targetID int, targetName string, status string) // Callback for status changes
}

// SchedulerConfig holds scheduler configuration
type SchedulerConfig struct {
	BatchSize        int
	PacingMs         int
	TimeoutMs        int
	Retry            int
	FailOpen         int // Consecutive failures to open alert
	SuccessClose     int // Consecutive successes to close alert
	BatchIntervalSec int
}

// TargetState tracks the state of a target
type TargetState struct {
	ID                 int
	Name               string
	Address            string
	Type               string
	MonitoringType     string
	HTTPMethod         string
	HTTPPath           string
	HTTPHeaders        string
	ExpectedStatusCode int
	ExpectedContent    string
	SSLCheck           bool
	FollowRedirects    bool
	TimeoutSec         int
	IntervalSec        int
	LastPing           time.Time
	LastSuccess        time.Time
	LastFailure        time.Time
	ConsecutiveFail    int
	ConsecutiveOk      int
	IsOnline           bool
	LastAlertID        *int
}

// NewScheduler creates a new ping scheduler
func NewScheduler(db *sql.DB, engine PingEngine, hub *ws.Hub, config SchedulerConfig) *Scheduler {
	return &Scheduler{
		db:           db,
		engine:       engine,
		hub:          hub,
		config:       config,
		targetStates: make(map[int]*TargetState),
		stopChan:     make(chan struct{}),
	}
}

// Start begins the scheduler
func (s *Scheduler) Start() error {
	if s.running {
		return fmt.Errorf("scheduler already running")
	}

	s.running = true
	log.Printf("🚀 Starting ping scheduler with engine: %s", s.engine.Name())

	// Load initial target states
	if err := s.loadTargetStates(); err != nil {
		return fmt.Errorf("failed to load target states: %w", err)
	}

	// Start the main scheduler loop
	s.wg.Add(1)
	go s.run()

	return nil
}

// SetStatusChangeCallback callback fonksiyonunu ayarlar
func (s *Scheduler) SetStatusChangeCallback(callback func(targetID int, targetName string, status string)) {
	s.onStatusChange = callback
}

// Stop stops the scheduler
func (s *Scheduler) Stop() {
	if !s.running {
		return
	}

	log.Printf("🛑 Stopping ping scheduler")
	s.running = false
	close(s.stopChan)
	s.wg.Wait()
	log.Printf("✅ Ping scheduler stopped")
}

// loadTargetStates loads all enabled targets from database
func (s *Scheduler) loadTargetStates() error {
	query := `SELECT id, name, address, type, monitoring_type, http_method, http_path, http_headers, 
	          expected_status_code, expected_content, ssl_check, follow_redirects, timeout_sec, interval_sec 
	          FROM targets WHERE enabled = true`
	rows, err := s.db.Query(query)
	if err != nil {
		return err
	}
	defer rows.Close()

	s.mu.Lock()
	defer s.mu.Unlock()

	// Clear existing target states
	s.targetStates = make(map[int]*TargetState)

	for rows.Next() {
		var target TargetState
		var httpHeaders sql.NullString
		var expectedContent sql.NullString

		err := rows.Scan(&target.ID, &target.Name, &target.Address, &target.Type, &target.MonitoringType,
			&target.HTTPMethod, &target.HTTPPath, &httpHeaders, &target.ExpectedStatusCode,
			&expectedContent, &target.SSLCheck, &target.FollowRedirects, &target.TimeoutSec, &target.IntervalSec)
		if err != nil {
			continue
		}

		if httpHeaders.Valid {
			target.HTTPHeaders = httpHeaders.String
		}
		if expectedContent.Valid {
			target.ExpectedContent = expectedContent.String
		}

		// Load last ping result
		s.loadLastPingResult(&target)
		s.targetStates[target.ID] = &target
	}

	log.Printf("📊 Loaded %d targets", len(s.targetStates))
	return nil
}

// loadLastPingResult loads the last ping result for a target
func (s *Scheduler) loadLastPingResult(target *TargetState) {
	query := `SELECT ok, ts_ms FROM pings_raw WHERE target_id = ? ORDER BY ts_ms DESC LIMIT 1`
	var ok bool
	var tsMs int64

	err := s.db.QueryRow(query, target.ID).Scan(&ok, &tsMs)
	if err != nil {
		return
	}

	target.LastPing = time.Unix(tsMs/1000, (tsMs%1000)*1000000)
	if ok {
		target.LastSuccess = target.LastPing
		target.IsOnline = true
		target.ConsecutiveOk = 1
	} else {
		target.LastFailure = target.LastPing
		target.IsOnline = false
		target.ConsecutiveFail = 1
	}
}

// run is the main scheduler loop
func (s *Scheduler) run() {
	defer s.wg.Done()

	ticker := time.NewTicker(time.Duration(s.config.BatchIntervalSec) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.processBatch()
		case <-s.stopChan:
			return
		}
	}
}

// processBatch processes a batch of targets
func (s *Scheduler) processBatch() {
	// Her batch'te hedefleri yeniden yükle (yeni eklenen hedefler için)
	if err := s.loadTargetStates(); err != nil {
		log.Printf("❌ Failed to reload targets: %v", err)
		return
	}

	s.mu.RLock()
	targets := make([]*TargetState, 0)
	now := time.Now()

	for _, target := range s.targetStates {
		// Check if target needs to be pinged (sabit 2 dakika interval kullan)
		if now.Sub(target.LastPing) >= time.Duration(PING_INTERVAL_SEC)*time.Second {
			targets = append(targets, target)
		}
	}
	s.mu.RUnlock()

	if len(targets) == 0 {
		return
	}

	// Limit batch size
	if len(targets) > s.config.BatchSize {
		targets = targets[:s.config.BatchSize]
	}

	start := time.Now()
	log.Printf("🔄 Processing batch: %d targets", len(targets))

	// Extract hosts for ping
	hosts := make([]string, len(targets))
	for i, target := range targets {
		hosts[i] = target.Address
	}

	// Perform ping
	cfg := PingConfig{
		TimeoutMs: s.config.TimeoutMs,
		Retry:     s.config.Retry,
		PacingMs:  s.config.PacingMs,
	}

	results, err := s.engine.Probe(hosts, cfg)
	if err != nil {
		log.Printf("❌ Ping batch failed: %v", err)
		return
	}

	// Process results
	successCount := 0
	failureCount := 0

	for i, result := range results {
		if i >= len(targets) {
			break
		}

		target := targets[i]
		success := result.Success

		// Handle different monitoring types
		if target.MonitoringType == "ping" {
			// 4-Ping Validation: If first ping fails, retry 3 more times immediately
			finalSuccess := success
			finalDuration := int(result.Duration.Milliseconds())

			if !success {
				log.Printf("⚠️ First ping failed for %s (%s), retrying 3 more times...", target.Name, target.Address)
				retryResults := s.retryPing(target.Address, 3)

				// Check if at least one retry succeeded
				successfulRetries := 0
				totalDuration := int(result.Duration.Milliseconds())

				for _, retry := range retryResults {
					if retry.Success {
						successfulRetries++
						totalDuration += int(retry.Duration.Milliseconds())
					}
				}

				// If at least 1 out of 4 pings succeeded, consider it online
				if successfulRetries > 0 {
					finalSuccess = true
					finalDuration = totalDuration / (successfulRetries + 1) // Average of successful pings
					log.Printf("✅ Target %s recovered: %d/4 pings successful", target.Name, successfulRetries)
				} else {
					log.Printf("❌ Target %s confirmed offline: 0/4 pings successful", target.Name)
				}
			}

			// Update target state with final result
			s.updateTargetState(target, finalSuccess, finalDuration)

			// Broadcast ping result via WebSocket
			s.hub.BroadcastPingResult(target.ID, finalDuration, finalSuccess)

			// Save to database (only final result)
			s.savePingResult(target.ID, finalSuccess, finalDuration, 0, 0, nil, nil)
		} else if target.MonitoringType == "http" || target.MonitoringType == "https" {
			// Process HTTP monitoring with 4-request validation
			s.processHTTPMonitoring(target, result)
		}

		// Check for alerts
		s.checkAlerts(target)

		if success {
			successCount++
		} else {
			failureCount++
		}
	}

	duration := time.Since(start)
	log.Printf("✅ Batch done: hosts=%d ok=%d fail=%d dur=%v",
		len(targets), successCount, failureCount, duration)
}

// updateTargetState updates the state of a target
func (s *Scheduler) updateTargetState(target *TargetState, success bool, duration int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	target.LastPing = now

	if success {
		target.LastSuccess = now
		target.ConsecutiveOk++
		target.ConsecutiveFail = 0

		if !target.IsOnline {
			target.IsOnline = true
			log.Printf("🟢 Target %s (%s) came online", target.Name, target.Address)

			// Send WebSocket status update
			s.hub.BroadcastStatusUpdate(target.ID, "online")
		}
	} else {
		target.LastFailure = now
		target.ConsecutiveFail++
		target.ConsecutiveOk = 0

		if target.IsOnline {
			target.IsOnline = false
			log.Printf("🔴 Target %s (%s) went offline", target.Name, target.Address)

			// Send WebSocket status update
			s.hub.BroadcastStatusUpdate(target.ID, "offline")
		}
	}
}

// processHTTPMonitoring processes HTTP monitoring results with 4-request validation
func (s *Scheduler) processHTTPMonitoring(target *TargetState, result PingResult) {
	// Use HTTP monitoring engine to perform actual HTTP request
	httpConfig := HTTPMonitoringConfig{
		Method: target.HTTPMethod,
		Path:   target.HTTPPath,
		Headers: func() map[string]string {
			if target.HTTPHeaders != "" {
				headers := make(map[string]string)
				// Simple JSON parsing for headers
				// TODO: Implement proper JSON parsing
				return headers
			}
			return make(map[string]string)
		}(),
		ExpectedStatus: target.ExpectedStatusCode,
		ExpectedContent: func() string {
			if target.ExpectedContent != "" {
				return target.ExpectedContent
			}
			return ""
		}(),
		SSLCheck:          target.SSLCheck,
		FollowRedirects:   target.FollowRedirects,
		TimeoutSec:        target.TimeoutSec,
		UserAgent:         "SysTrack-Monitor/1.0",
		MaxRedirects:      5,
		ConnectTimeoutSec: target.TimeoutSec,
		ReadTimeoutSec:    target.TimeoutSec,
	}
	httpEngine := NewHTTPMonitoringEngine(httpConfig)

	// Create target for HTTP engine
	httpTarget := Target{
		Address:            target.Address,
		MonitoringType:     target.MonitoringType,
		HTTPMethod:         target.HTTPMethod,
		HTTPPath:           target.HTTPPath,
		HTTPHeaders:        target.HTTPHeaders,
		ExpectedStatusCode: target.ExpectedStatusCode,
		ExpectedContent:    target.ExpectedContent,
		SSLCheck:           target.SSLCheck,
		FollowRedirects:    target.FollowRedirects,
		TimeoutSec:         target.TimeoutSec,
	}

	// Perform HTTP monitoring
	httpResult, err := httpEngine.MonitorHTTP(httpTarget)
	if err != nil {
		// HTTP monitoring failed - retry 3 more times
		log.Printf("⚠️ First HTTP request failed for %s (%s), retrying 3 more times...", target.Name, target.Address)
		retryResults := s.retryHTTP(httpEngine, httpTarget, 3)

		// Check if at least one retry succeeded
		successfulRetries := 0
		var lastSuccessResult *PingResult

		for _, retry := range retryResults {
			if retry.Success {
				successfulRetries++
				lastSuccessResult = &retry
			}
		}

		// If at least 1 out of 4 requests succeeded, use the successful result
		if successfulRetries > 0 && lastSuccessResult != nil {
			log.Printf("✅ Target %s recovered: %d/4 HTTP requests successful", target.Name, successfulRetries)
			s.updateTargetState(target, true, int(lastSuccessResult.Duration.Milliseconds()))
			s.hub.BroadcastPingResult(target.ID, int(lastSuccessResult.Duration.Milliseconds()), true)
			s.savePingResult(target.ID, true, int(lastSuccessResult.Duration.Milliseconds()),
				lastSuccessResult.StatusCode, lastSuccessResult.ResponseSizeBytes, lastSuccessResult.SSLExpiryDate, lastSuccessResult.ResponseHeaders)
		} else {
			log.Printf("❌ Target %s confirmed offline: 0/4 HTTP requests successful", target.Name)
			s.updateTargetState(target, false, 0)
			s.savePingResult(target.ID, false, 0, 0, 0, nil, nil)
		}
		return
	}

	// 4-Request Validation: If first request fails (wrong status code), retry 3 more times
	finalSuccess := httpResult.Success
	finalResult := httpResult

	if !httpResult.Success {
		log.Printf("⚠️ First HTTP request returned unexpected status code %d for %s, retrying 3 more times...",
			httpResult.StatusCode, target.Name)
		retryResults := s.retryHTTP(httpEngine, httpTarget, 3)

		// Check if at least one retry succeeded
		successfulRetries := 0
		for _, retry := range retryResults {
			if retry.Success {
				successfulRetries++
				finalResult = retry
				finalSuccess = true
				break // Use first successful retry
			}
		}

		if successfulRetries > 0 {
			log.Printf("✅ Target %s recovered: %d/4 HTTP requests successful (status code: %d)",
				target.Name, successfulRetries, finalResult.StatusCode)
		} else {
			log.Printf("❌ Target %s confirmed failed: 0/4 HTTP requests returned expected status code", target.Name)
		}
	}

	// Update target state with final result
	s.updateTargetState(target, finalSuccess, int(finalResult.Duration.Milliseconds()))

	// Broadcast ping result via WebSocket
	s.hub.BroadcastPingResult(target.ID, int(finalResult.Duration.Milliseconds()), finalSuccess)

	// Save to database with HTTP-specific data (only final result)
	s.savePingResult(target.ID, finalSuccess, int(finalResult.Duration.Milliseconds()),
		finalResult.StatusCode, finalResult.ResponseSizeBytes, finalResult.SSLExpiryDate, finalResult.ResponseHeaders)
}

// retryHTTP performs immediate retry HTTP requests for validation (4-request mechanism)
func (s *Scheduler) retryHTTP(httpEngine *HTTPMonitoringEngine, target Target, retryCount int) []PingResult {
	results := make([]PingResult, 0, retryCount)

	for i := 0; i < retryCount; i++ {
		httpResult, err := httpEngine.MonitorHTTP(target)
		if err != nil {
			log.Printf("⚠️ Retry HTTP request %d/%d failed for %s: %v", i+1, retryCount, target.Address, err)
			results = append(results, PingResult{
				Host:    target.Address,
				Success: false,
				Error:   err.Error(),
			})
		} else {
			results = append(results, httpResult)
			log.Printf("🔄 Retry HTTP request %d/%d for %s: success=%v, status_code=%d, rtt=%v",
				i+1, retryCount, target.Address, httpResult.Success, httpResult.StatusCode, httpResult.Duration)
		}

		// Small delay between retries
		if i < retryCount-1 {
			time.Sleep(100 * time.Millisecond)
		}
	}

	return results
}

// savePingResult saves ping result to database with HTTP monitoring support
// İki tabloya birden yazar: pings_raw (arşiv) ve pings_raw_momentary (dashboard için)
func (s *Scheduler) savePingResult(targetID int, success bool, duration int, statusCode int, responseSize int, sslExpiry *time.Time, responseHeaders map[string]string) {
	// Check if target exists before saving
	var exists bool
	checkQuery := `SELECT EXISTS(SELECT 1 FROM targets WHERE id = ?)`
	err := s.db.QueryRow(checkQuery, targetID).Scan(&exists)
	if err != nil || !exists {
		log.Printf("❌ Target ID %d does not exist, skipping ping result", targetID)
		return
	}

	now := time.Now()
	tsMs := now.UTC().UnixMilli()

	var errorMsg sql.NullString
	if !success {
		errorMsg = sql.NullString{String: "Request failed", Valid: true}
	}

	var rttMs sql.NullInt32
	if success {
		rttMs = sql.NullInt32{Int32: int32(duration), Valid: true}
	}

	var statusCodeNull sql.NullInt32
	if statusCode > 0 {
		statusCodeNull = sql.NullInt32{Int32: int32(statusCode), Valid: true}
	}

	var responseSizeNull sql.NullInt32
	if responseSize > 0 {
		responseSizeNull = sql.NullInt32{Int32: int32(responseSize), Valid: true}
	}

	var sslExpiryNull sql.NullTime
	if sslExpiry != nil {
		sslExpiryNull = sql.NullTime{Time: *sslExpiry, Valid: true}
	}

	var headersJSON sql.NullString
	if responseHeaders != nil {
		headersBytes, _ := json.Marshal(responseHeaders)
		headersJSON = sql.NullString{String: string(headersBytes), Valid: true}
	}

	// Transaction başlat - iki tabloya da yazılmalı
	tx, err := s.db.Begin()
	if err != nil {
		log.Printf("❌ Failed to begin transaction: %v", err)
		return
	}
	defer tx.Rollback()

	insertQuery := `INSERT INTO %s (target_id, ts_ms, ok, rtt_ms, error_msg, response_status_code, response_size_bytes, ssl_expiry_date, response_headers)
	                VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`

	// 1. pings_raw'a yaz (arşiv için)
	_, err = tx.Exec(fmt.Sprintf(insertQuery, "pings_raw"),
		targetID, tsMs, success, rttMs, errorMsg, statusCodeNull, responseSizeNull, sslExpiryNull, headersJSON)
	if err != nil {
		log.Printf("❌ Failed to save ping result to pings_raw: %v", err)
		return
	}

	// 2. pings_raw_momentary'e yaz (dashboard için)
	_, err = tx.Exec(fmt.Sprintf(insertQuery, "pings_raw_momentary"),
		targetID, tsMs, success, rttMs, errorMsg, statusCodeNull, responseSizeNull, sslExpiryNull, headersJSON)
	if err != nil {
		log.Printf("❌ Failed to save ping result to pings_raw_momentary: %v", err)
		return
	}

	// Transaction commit
	if err = tx.Commit(); err != nil {
		log.Printf("❌ Failed to commit transaction: %v", err)
	}
}

// checkAlerts checks if alerts should be created or closed
func (s *Scheduler) checkAlerts(target *TargetState) {
	if target.IsOnline {
		// Target is online, check if we should close an alert
		if target.ConsecutiveOk >= s.config.SuccessClose && target.LastAlertID != nil {
			s.closeAlert(*target.LastAlertID)
			target.LastAlertID = nil

			// Trigger notification for target recovery
			if s.onStatusChange != nil {
				s.onStatusChange(target.ID, target.Name, "online")
			}
		}
	} else {
		// Target is offline, check if we should create an alert
		if target.ConsecutiveFail >= s.config.FailOpen && target.LastAlertID == nil {
			alertID := s.createAlert(target)
			if alertID != nil {
				target.LastAlertID = alertID

				// Trigger notification for target failure
				if s.onStatusChange != nil {
					s.onStatusChange(target.ID, target.Name, "offline")
				}
			}
		}
	}
}

// createAlert creates a new alert for offline target
func (s *Scheduler) createAlert(target *TargetState) *int {
	// Check if target exists before creating alert
	var exists bool
	checkQuery := `SELECT EXISTS(SELECT 1 FROM targets WHERE id = ?)`
	err := s.db.QueryRow(checkQuery, target.ID).Scan(&exists)
	if err != nil || !exists {
		log.Printf("❌ Target ID %d does not exist, skipping alert creation", target.ID)
		return nil
	}

	query := `INSERT INTO alerts (target_id, level, message, status, opened_at) VALUES (?, 'major', ?, 'open', NOW())`
	message := fmt.Sprintf("Target %s (%s) is offline", target.Name, target.Address)

	result, err := s.db.Exec(query, target.ID, message)
	if err != nil {
		log.Printf("❌ Failed to create alert: %v", err)
		return nil
	}

	id, err := result.LastInsertId()
	if err != nil {
		log.Printf("❌ Failed to get alert ID: %v", err)
		return nil
	}

	alertID := int(id)
	log.Printf("🚨 Created alert %d for target %s", alertID, target.Name)

	// Send WebSocket notification
	s.hub.BroadcastAlert("alert", target.ID, map[string]interface{}{
		"id":      alertID,
		"target":  target.Name,
		"level":   "critical",
		"message": message,
	})

	return &alertID
}

// closeAlert closes an alert
func (s *Scheduler) closeAlert(alertID int) {
	query := `UPDATE alerts SET status = 'closed', closed_at = NOW() WHERE id = ?`
	_, err := s.db.Exec(query, alertID)
	if err != nil {
		log.Printf("❌ Failed to close alert: %v", err)
		return
	}

	log.Printf("✅ Closed alert %d", alertID)

	// Send WebSocket notification
	s.hub.BroadcastAlert("alert_closed", alertID, map[string]interface{}{
		"id": alertID,
	})
}

// retryPing performs immediate retry pings for validation (4-ping mechanism)
func (s *Scheduler) retryPing(address string, retryCount int) []PingResult {
	results := make([]PingResult, 0, retryCount)

	cfg := PingConfig{
		TimeoutMs: s.config.TimeoutMs,
		Retry:     0, // No additional retries for validation pings
		PacingMs:  100, // Small delay between validation pings
	}

	for i := 0; i < retryCount; i++ {
		retryResults, err := s.engine.Probe([]string{address}, cfg)
		if err != nil {
			log.Printf("⚠️ Retry ping %d/%d failed for %s: %v", i+1, retryCount, address, err)
			results = append(results, PingResult{
				Host:    address,
				Success: false,
				Error:   err.Error(),
			})
			continue
		}

		if len(retryResults) > 0 {
			results = append(results, retryResults[0])
			log.Printf("🔄 Retry ping %d/%d for %s: success=%v, rtt=%v",
				i+1, retryCount, address, retryResults[0].Success, retryResults[0].Duration)
		}

		// Small delay between retries
		if i < retryCount-1 {
			time.Sleep(time.Duration(cfg.PacingMs) * time.Millisecond)
		}
	}

	return results
}

// PingTarget performs a single ping for a specific target
func (s *Scheduler) PingTarget(targetID int) (bool, time.Duration, error) {
	// Get target info with full details
	var target TargetState
	var httpHeaders sql.NullString
	var expectedContent sql.NullString

	query := `SELECT id, name, address, type, monitoring_type, http_method, http_path, http_headers,
	          expected_status_code, expected_content, ssl_check, follow_redirects, timeout_sec
	          FROM targets WHERE id = ? AND enabled = true`

	err := s.db.QueryRow(query, targetID).Scan(
		&target.ID, &target.Name, &target.Address, &target.Type, &target.MonitoringType,
		&target.HTTPMethod, &target.HTTPPath, &httpHeaders,
		&target.ExpectedStatusCode, &expectedContent, &target.SSLCheck,
		&target.FollowRedirects, &target.TimeoutSec)

	if err != nil {
		return false, 0, fmt.Errorf("target not found or disabled: %v", err)
	}

	if httpHeaders.Valid {
		target.HTTPHeaders = httpHeaders.String
	}
	if expectedContent.Valid {
		target.ExpectedContent = expectedContent.String
	}

	// Default değerler
	if target.TimeoutSec == 0 {
		target.TimeoutSec = 10
	}
	if target.HTTPMethod == "" {
		target.HTTPMethod = "GET"
	}
	if target.HTTPPath == "" {
		target.HTTPPath = "/"
	}
	if target.ExpectedStatusCode == 0 {
		target.ExpectedStatusCode = 200
	}

	// HTTP/HTTPS monitoring için
	if target.MonitoringType == "http" || target.MonitoringType == "https" {
		httpConfig := HTTPMonitoringConfig{
			Method:            target.HTTPMethod,
			Path:              target.HTTPPath,
			Headers:           make(map[string]string),
			ExpectedStatus:    target.ExpectedStatusCode,
			ExpectedContent:   target.ExpectedContent,
			SSLCheck:          target.SSLCheck,
			FollowRedirects:   target.FollowRedirects,
			TimeoutSec:        target.TimeoutSec,
			UserAgent:         "SysTrack-Monitor/1.0",
			MaxRedirects:      5,
			ConnectTimeoutSec: target.TimeoutSec,
			ReadTimeoutSec:    target.TimeoutSec,
		}
		httpEngine := NewHTTPMonitoringEngine(httpConfig)

		httpTarget := Target{
			Address:            target.Address,
			MonitoringType:     target.MonitoringType,
			HTTPMethod:         target.HTTPMethod,
			HTTPPath:           target.HTTPPath,
			HTTPHeaders:        target.HTTPHeaders,
			ExpectedStatusCode: target.ExpectedStatusCode,
			ExpectedContent:    target.ExpectedContent,
			SSLCheck:           target.SSLCheck,
			FollowRedirects:    target.FollowRedirects,
			TimeoutSec:         target.TimeoutSec,
		}

		httpResult, err := httpEngine.MonitorHTTP(httpTarget)
		if err != nil {
			s.savePingResult(targetID, false, 0, 0, 0, nil, nil)
			return false, 0, err
		}

		// Save HTTP result with full details
		s.savePingResult(targetID, httpResult.Success, int(httpResult.Duration.Milliseconds()),
			httpResult.StatusCode, httpResult.ResponseSizeBytes, httpResult.SSLExpiryDate, httpResult.ResponseHeaders)

		// Broadcast via WebSocket
		s.hub.BroadcastPingResult(targetID, int(httpResult.Duration.Milliseconds()), httpResult.Success)

		return httpResult.Success, httpResult.Duration, nil
	}

	// Normal ping için
	cfg := PingConfig{
		TimeoutMs: s.config.TimeoutMs,
		Retry:     s.config.Retry,
		PacingMs:  s.config.PacingMs,
	}

	results, err := s.engine.Probe([]string{target.Address}, cfg)
	if err != nil {
		return false, 0, err
	}

	if len(results) == 0 {
		return false, 0, fmt.Errorf("no ping result")
	}

	result := results[0]

	// Save result to database
	s.savePingResult(targetID, result.Success, int(result.Duration.Milliseconds()), 0, 0, nil, nil)

	// Broadcast via WebSocket
	s.hub.BroadcastPingResult(targetID, int(result.Duration.Milliseconds()), result.Success)

	return result.Success, result.Duration, nil
}

// GetStatus returns the current status of the scheduler
func (s *Scheduler) GetStatus() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	onlineCount := 0
	offlineCount := 0

	for _, target := range s.targetStates {
		if target.IsOnline {
			onlineCount++
		} else {
			offlineCount++
		}
	}

	return map[string]interface{}{
		"engine":        s.engine.Name(),
		"running":       s.running,
		"total_targets": len(s.targetStates),
		"online":        onlineCount,
		"offline":       offlineCount,
	}
}
