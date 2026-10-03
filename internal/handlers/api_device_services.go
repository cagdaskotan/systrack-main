package handlers

import (
	"context"
	"database/sql"
	"log"
	"net/http"
	"strconv"
	"time"

	"systrack/internal/services"

	"github.com/gin-gonic/gin"
)

// ==========================================
// Credential Management Endpoints
// ==========================================

// GetTargetCredentials retrieves credentials for a target (without password)
func GetTargetCredentials(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		targetID, err := strconv.Atoi(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_target_id"})
			return
		}

		var osType, protocol, username, domain string
		var port sql.NullInt64
		var lastTestAt sql.NullTime
		var lastTestSuccess sql.NullBool

		query := `
			SELECT os_type, protocol, username, domain, port, last_test_at, last_test_success
			FROM target_credentials
			WHERE target_id = ?
		`
		err = db.QueryRowContext(c.Request.Context(), query, targetID).Scan(
			&osType, &protocol, &username, &domain, &port, &lastTestAt, &lastTestSuccess,
		)

		if err == sql.ErrNoRows {
			c.JSON(http.StatusOK, gin.H{"exists": false})
			return
		}

		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "database_error"})
			return
		}

		response := gin.H{
			"exists":   true,
			"os_type":  osType,
			"protocol": protocol,
			"username": username,
			"domain":   domain,
		}

		if port.Valid {
			response["port"] = port.Int64
		}
		if lastTestAt.Valid {
			response["last_test_at"] = lastTestAt.Time
		}
		if lastTestSuccess.Valid {
			response["last_test_success"] = lastTestSuccess.Bool
		}

		c.JSON(http.StatusOK, response)
	}
}

// SaveTargetCredentials saves or updates credentials for a target
func SaveTargetCredentials(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		targetID, err := strconv.Atoi(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_target_id"})
			return
		}

		var req struct {
			OSType   string `json:"os_type" binding:"required,oneof=windows linux"`
			Username string `json:"username" binding:"required"`
			Password string `json:"password" binding:"required"`
			Domain   string `json:"domain"`
			Port     *int   `json:"port"`
		}

		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request", "details": err.Error()})
			return
		}

		// Determine protocol based on OS type
		protocol := "ssh"
		if req.OSType == "windows" {
			protocol = "winrm"
		}

		// Encrypt password
		encryptedPassword, err := services.EncryptPassword(req.Password)
		if err != nil {
			log.Printf("Failed to encrypt password: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "encryption_failed"})
			return
		}

		// Upsert credentials
		query := `
			INSERT INTO target_credentials (target_id, os_type, protocol, username, password_encrypted, domain, port)
			VALUES (?, ?, ?, ?, ?, ?, ?)
			ON DUPLICATE KEY UPDATE
				os_type = VALUES(os_type),
				protocol = VALUES(protocol),
				username = VALUES(username),
				password_encrypted = VALUES(password_encrypted),
				domain = VALUES(domain),
				port = VALUES(port),
				updated_at = CURRENT_TIMESTAMP
		`

		_, err = db.ExecContext(c.Request.Context(), query,
			targetID, req.OSType, protocol, req.Username, encryptedPassword, req.Domain, req.Port,
		)

		if err != nil {
			log.Printf("Failed to save credentials: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "database_error"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"success": true, "message": "Credentials saved successfully"})
	}
}

// TestTargetCredentials tests the connection with provided credentials
func TestTargetCredentials(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		targetID, err := strconv.Atoi(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_target_id"})
			return
		}

		// Get target address and credentials
		var targetAddress, osType, username, passwordEnc, domain string
		query := `
			SELECT t.address, tc.os_type, tc.username, tc.password_encrypted, tc.domain
			FROM targets t
			INNER JOIN target_credentials tc ON t.id = tc.target_id
			WHERE t.id = ?
		`
		err = db.QueryRowContext(c.Request.Context(), query, targetID).Scan(
			&targetAddress, &osType, &username, &passwordEnc, &domain,
		)

		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "target_or_credentials_not_found"})
			return
		}

		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "database_error"})
			return
		}

		// Decrypt password
		password, err := services.DecryptPassword(passwordEnc)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "decryption_failed"})
			return
		}

		// Test connection based on OS type
		ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
		defer cancel()

		var testErr error
		if osType == "windows" {
			reader := services.NewWindowsServiceReader()
			testErr = reader.TestConnection(ctx, targetAddress, username, password, domain)
		} else if osType == "linux" {
			reader := services.NewLinuxServiceReader()
			testErr = reader.TestConnection(ctx, targetAddress, username, password)
		} else {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_os_type"})
			return
		}

		// Update test results in database
		success := testErr == nil
		updateQuery := `
			UPDATE target_credentials
			SET last_test_at = ?, last_test_success = ?
			WHERE target_id = ?
		`
		db.ExecContext(c.Request.Context(), updateQuery, time.Now(), success, targetID)

		if testErr != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"error":   testErr.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "Connection successful",
		})
	}
}

// DeleteTargetCredentials deletes credentials for a target
func DeleteTargetCredentials(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		targetID, err := strconv.Atoi(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_target_id"})
			return
		}

		result, err := db.ExecContext(c.Request.Context(),
			"DELETE FROM target_credentials WHERE target_id = ?", targetID)

		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "database_error"})
			return
		}

		rowsAffected, _ := result.RowsAffected()
		if rowsAffected == 0 {
			c.JSON(http.StatusNotFound, gin.H{"error": "credentials_not_found"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"success": true})
	}
}

// ==========================================
// Device Service Endpoints
// ==========================================

// GetDeviceServices retrieves all services for a target
func GetDeviceServices(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		targetID, err := strconv.Atoi(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_target_id"})
			return
		}

		// Optional filters
		statusFilter := c.Query("status") // running, stopped, unknown
		monitoredOnly := c.Query("monitored") == "true"

		query := `
			SELECT id, service_name, display_name, description, status, startup_type, pid,
			       last_checked, is_monitored, created_at, updated_at
			FROM device_services
			WHERE target_id = ?
		`
		args := []interface{}{targetID}

		if statusFilter != "" {
			query += " AND status = ?"
			args = append(args, statusFilter)
		}

		if monitoredOnly {
			query += " AND is_monitored = TRUE"
		}

		query += " ORDER BY service_name ASC"

		rows, err := db.QueryContext(c.Request.Context(), query, args...)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "database_error"})
			return
		}
		defer rows.Close()

		services := []gin.H{}
		for rows.Next() {
			var id int64
			var serviceName, status string
			var displayName, description, startupType sql.NullString
			var pid sql.NullInt64
			var lastChecked sql.NullTime
			var isMonitored bool
			var createdAt, updatedAt time.Time

			err := rows.Scan(&id, &serviceName, &displayName, &description, &status, &startupType,
				&pid, &lastChecked, &isMonitored, &createdAt, &updatedAt)
			if err != nil {
				continue
			}

			service := gin.H{
				"id":           id,
				"service_name": serviceName,
				"status":       status,
				"is_monitored": isMonitored,
				"created_at":   createdAt,
				"updated_at":   updatedAt,
			}

			if displayName.Valid {
				service["display_name"] = displayName.String
			}
			if description.Valid {
				service["description"] = description.String
			}
			if startupType.Valid {
				service["startup_type"] = startupType.String
			}
			if pid.Valid {
				service["pid"] = pid.Int64
			}
			if lastChecked.Valid {
				service["last_checked"] = lastChecked.Time
			}

			services = append(services, service)
		}

		c.JSON(http.StatusOK, gin.H{
			"services": services,
			"total":    len(services),
		})
	}
}

// ScanDeviceServices triggers immediate service collection for a target
func ScanDeviceServices(db *sql.DB, collector *services.DeviceServiceCollector) gin.HandlerFunc {
	return func(c *gin.Context) {
		targetID, err := strconv.Atoi(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_target_id"})
			return
		}

		// Trigger immediate collection
		err = collector.CollectNow(targetID)
		if err != nil {
			log.Printf("Failed to scan services for target %d: %v", targetID, err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "scan_failed",
				"message": err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "Service scan completed successfully",
		})
	}
}

// UpdateServiceMonitoring toggles the monitored flag for a service
func UpdateServiceMonitoring(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		serviceID, err := strconv.ParseInt(c.Param("serviceId"), 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_service_id"})
			return
		}

		var req struct {
			IsMonitored bool `json:"is_monitored"`
		}

		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request"})
			return
		}

		_, err = db.ExecContext(c.Request.Context(),
			"UPDATE device_services SET is_monitored = ?, updated_at = ? WHERE id = ?",
			req.IsMonitored, time.Now(), serviceID)

		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "database_error"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"success": true})
	}
}

// GetServiceHistory retrieves status change history for a service
func GetServiceHistory(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		serviceID, err := strconv.ParseInt(c.Param("serviceId"), 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_service_id"})
			return
		}

		limit := 50
		if limitStr := c.Query("limit"); limitStr != "" {
			if parsedLimit, err := strconv.Atoi(limitStr); err == nil && parsedLimit > 0 && parsedLimit <= 100 {
				limit = parsedLimit
			}
		}

		query := `
			SELECT id, old_status, new_status, changed_at
			FROM device_service_history
			WHERE service_id = ?
			ORDER BY changed_at DESC
			LIMIT ?
		`

		rows, err := db.QueryContext(c.Request.Context(), query, serviceID, limit)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "database_error"})
			return
		}
		defer rows.Close()

		history := []gin.H{}
		for rows.Next() {
			var id int64
			var newStatus string
			var oldStatus sql.NullString
			var changedAt time.Time

			if err := rows.Scan(&id, &oldStatus, &newStatus, &changedAt); err != nil {
				continue
			}

			entry := gin.H{
				"id":         id,
				"new_status": newStatus,
				"changed_at": changedAt,
			}

			if oldStatus.Valid {
				entry["old_status"] = oldStatus.String
			}

			history = append(history, entry)
		}

		c.JSON(http.StatusOK, gin.H{
			"history": history,
			"total":   len(history),
		})
	}
}

// GetTargetServiceHistory retrieves status change history for a target (paginated)
func GetTargetServiceHistory(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		targetID, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_target_id"})
			return
		}

		page := 1
		if pageStr := c.Query("page"); pageStr != "" {
			if parsedPage, err := strconv.Atoi(pageStr); err == nil && parsedPage > 0 {
				page = parsedPage
			}
		}

		limit := 20
		if limitStr := c.Query("limit"); limitStr != "" {
			if parsedLimit, err := strconv.Atoi(limitStr); err == nil && parsedLimit > 0 && parsedLimit <= 200 {
				limit = parsedLimit
			}
		}

		offset := (page - 1) * limit

		var total int
		if err := db.QueryRowContext(c.Request.Context(),
			`SELECT COUNT(*) FROM device_service_history WHERE target_id = ?`, targetID).Scan(&total); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "database_error"})
			return
		}

		query := `
			SELECT
				h.id,
				h.service_id,
				h.service_name,
				h.old_status,
				h.new_status,
				h.changed_at,
				ds.display_name
			FROM device_service_history h
			LEFT JOIN device_services ds ON h.service_id = ds.id
			WHERE h.target_id = ?
			ORDER BY h.changed_at DESC
			LIMIT ? OFFSET ?
		`

		rows, err := db.QueryContext(c.Request.Context(), query, targetID, limit, offset)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "database_error"})
			return
		}
		defer rows.Close()

		history := []gin.H{}
		for rows.Next() {
			var id, serviceID int64
			var serviceName, newStatus string
			var oldStatus sql.NullString
			var changedAt time.Time
			var displayName sql.NullString

			if err := rows.Scan(&id, &serviceID, &serviceName, &oldStatus, &newStatus, &changedAt, &displayName); err != nil {
				continue
			}

			entry := gin.H{
				"id":           id,
				"service_id":   serviceID,
				"service_name": serviceName,
				"new_status":   newStatus,
				"changed_at":   changedAt,
			}

			if oldStatus.Valid {
				entry["old_status"] = oldStatus.String
			}
			if displayName.Valid {
				entry["display_name"] = displayName.String
			}

			history = append(history, entry)
		}

		c.JSON(http.StatusOK, gin.H{
			"history": history,
			"total":   total,
			"page":    page,
			"limit":   limit,
		})
	}
}

// GetMonitoredServiceAlerts retrieves recent status changes for monitored services
func GetMonitoredServiceAlerts(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Get only unacknowledged or recent alerts (last 24h by default)
		hoursBack := 24
		if hoursStr := c.Query("hours"); hoursStr != "" {
			if parsed, err := strconv.Atoi(hoursStr); err == nil && parsed > 0 && parsed <= 168 {
				hoursBack = parsed
			}
		}

		limit := 100
		if limitStr := c.Query("limit"); limitStr != "" {
			if parsedLimit, err := strconv.Atoi(limitStr); err == nil && parsedLimit > 0 && parsedLimit <= 500 {
				limit = parsedLimit
			}
		}

		query := `
			SELECT
				h.id,
				h.service_id,
				h.target_id,
				h.service_name,
				h.old_status,
				h.new_status,
				h.changed_at,
				ds.display_name,
				ds.is_monitored,
				t.name as target_name,
				t.address as target_address
			FROM device_service_history h
			INNER JOIN device_services ds ON h.service_id = ds.id
			INNER JOIN targets t ON h.target_id = t.id
			WHERE ds.is_monitored = TRUE
				AND h.changed_at >= DATE_SUB(NOW(), INTERVAL ? HOUR)
			ORDER BY h.changed_at DESC
			LIMIT ?
		`

		rows, err := db.QueryContext(c.Request.Context(), query, hoursBack, limit)
		if err != nil {
			log.Printf("Failed to get monitored service alerts: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "database_error"})
			return
		}
		defer rows.Close()

		alerts := []gin.H{}
		for rows.Next() {
			var id, serviceID, targetID int64
			var serviceName, newStatus, targetName, targetAddress string
			var oldStatus sql.NullString
			var displayName sql.NullString
			var isMonitored bool
			var changedAt time.Time

			if err := rows.Scan(&id, &serviceID, &targetID, &serviceName, &oldStatus, &newStatus,
				&changedAt, &displayName, &isMonitored, &targetName, &targetAddress); err != nil {
				continue
			}

			alert := gin.H{
				"id":             id,
				"service_id":     serviceID,
				"target_id":      targetID,
				"service_name":   serviceName,
				"new_status":     newStatus,
				"changed_at":     changedAt,
				"is_monitored":   isMonitored,
				"target_name":    targetName,
				"target_address": targetAddress,
			}

			if oldStatus.Valid {
				alert["old_status"] = oldStatus.String
			}
			if displayName.Valid {
				alert["display_name"] = displayName.String
			}

			alerts = append(alerts, alert)
		}

		c.JSON(http.StatusOK, gin.H{
			"alerts": alerts,
			"total":  len(alerts),
		})
	}
}

// GetMonitoredServiceAlertsCount retrieves count of unread service alerts
func GetMonitoredServiceAlertsCount(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Count alerts from last 24 hours for monitored services
		hoursBack := 24
		if hoursStr := c.Query("hours"); hoursStr != "" {
			if parsed, err := strconv.Atoi(hoursStr); err == nil && parsed > 0 && parsed <= 168 {
				hoursBack = parsed
			}
		}

		query := `
			SELECT COUNT(*)
			FROM device_service_history h
			INNER JOIN device_services ds ON h.service_id = ds.id
			WHERE ds.is_monitored = TRUE
				AND h.changed_at >= DATE_SUB(NOW(), INTERVAL ? HOUR)
		`

		var count int
		err := db.QueryRowContext(c.Request.Context(), query, hoursBack).Scan(&count)
		if err != nil {
			log.Printf("Failed to get monitored service alerts count: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "database_error"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"count": count,
		})
	}
}
