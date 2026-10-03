package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// convertDateTimeToMySQL converts ISO 8601 datetime to MySQL format
func convertDateTimeToMySQL(dateTime string) string {
	// If already in MySQL format, return as is
	if len(dateTime) == 19 && strings.Count(dateTime, "-") == 2 && strings.Count(dateTime, ":") == 2 {
		return dateTime
	}

	// Try to parse ISO 8601 format with Z suffix
	if t, err := time.Parse("2006-01-02T15:04:05Z", dateTime); err == nil {
		return t.Format("2006-01-02 15:04:05")
	}

	// Try to parse ISO 8601 format with timezone offset
	if t, err := time.Parse("2006-01-02T15:04:05-07:00", dateTime); err == nil {
		return t.Format("2006-01-02 15:04:05")
	}

	// Try to parse RFC3339 format
	if t, err := time.Parse(time.RFC3339, dateTime); err == nil {
		return t.Format("2006-01-02 15:04:05")
	}

	// Try to parse RFC3339Nano format
	if t, err := time.Parse(time.RFC3339Nano, dateTime); err == nil {
		return t.Format("2006-01-02 15:04:05")
	}

	// Try to parse with milliseconds
	if t, err := time.Parse("2006-01-02T15:04:05.000Z", dateTime); err == nil {
		return t.Format("2006-01-02 15:04:05")
	}

	// If all else fails, return original
	return dateTime
}

// BackupData represents the structure of backup data
type BackupData struct {
	Version           string                    `json:"version"`
	ExportDate        string                    `json:"export_date"`
	Targets           []TargetBackup            `json:"targets"`
	NotificationRules []NotificationRulesBackup `json:"notification_rules"`
}

// TargetBackup represents target data for backup
type TargetBackup struct {
	ID                 int     `json:"id"`
	Name               string  `json:"name"`
	Address            string  `json:"address"`
	Type               string  `json:"type"`
	MonitoringType     string  `json:"monitoring_type"`
	MetricsEnabled     bool    `json:"metrics_enabled"`
	SNMPCommunity      string  `json:"snmp_community"`
	SNMPVersion        string  `json:"snmp_version"`
	Port               *int    `json:"port,omitempty"`
	Path               *string `json:"path,omitempty"`
	HTTPMethod         string  `json:"http_method"`
	HTTPPath           string  `json:"http_path"`
	HTTPHeaders        *string `json:"http_headers,omitempty"`
	ExpectedStatusCode int     `json:"expected_status_code"`
	ExpectedContent    *string `json:"expected_content,omitempty"`
	SSLCheck           bool    `json:"ssl_check"`
	FollowRedirects    bool    `json:"follow_redirects"`
	TimeoutSec         int     `json:"timeout_sec"`
	IntervalSec        int     `json:"interval_sec"`
	TimeoutMs          int     `json:"timeout_ms"`
	Enabled            bool    `json:"enabled"`
	Tags               *string `json:"tags,omitempty"`
	CreatedAt          string  `json:"created_at"`
	UpdatedAt          string  `json:"updated_at"`
}

// NotificationRulesBackup represents notification rules data for backup
type NotificationRulesBackup struct {
	ID                      int     `json:"id"`
	Name                    string  `json:"name"`
	TargetID                *int    `json:"target_id,omitempty"`
	EntityType              string  `json:"entity_type"`
	Channel                 string  `json:"channel"`
	ScheduleIntervalMinutes int     `json:"schedule_interval_minutes"`
	Recipients              *string `json:"recipients,omitempty"`         // JSON string
	TemplateID              *int    `json:"template_id,omitempty"`
	Conditions              *string `json:"conditions,omitempty"`         // JSON string
	IsActive                bool    `json:"is_active"`
	CreatedBy               *int    `json:"created_by,omitempty"`
	LastSentAt              *string `json:"last_sent_at,omitempty"`
	CreatedAt               string  `json:"created_at"`
	UpdatedAt               string  `json:"updated_at"`
}

// ExportBackup exports all backup data (targets, IP blacklist, domain registry)
type BackupLogEntry struct {
	ID        int64                  `json:"id"`
	UserID    *int                   `json:"user_id,omitempty"`
	UserEmail string                 `json:"user_email,omitempty"`
	Action    string                 `json:"action"`
	Status    string                 `json:"status"`
	Message   string                 `json:"message"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
	FileName  string                 `json:"file_name,omitempty"`
	IPAddress string                 `json:"ip_address,omitempty"`
	CreatedAt string                 `json:"created_at"`
}

func fetchUserRoleAndEmail(db *sql.DB, userID int) (string, string, error) {
	var role, email string
	err := db.QueryRow("SELECT role, email FROM users WHERE id = ?", userID).Scan(&role, &email)
	if err != nil {
		return "", "", err
	}
	return role, email, nil
}

func recordBackupLog(db *sql.DB, userID int, userEmail, action, status, message, fileName, ipAddress string, metadata map[string]interface{}) error {
	var metaValue interface{}
	if len(metadata) > 0 {
		if metaBytes, err := json.Marshal(metadata); err == nil {
			metaValue = metaBytes
		} else {
			log.Printf("failed to marshal backup metadata: %v", err)
		}
	}

	_, err := db.Exec(`
		INSERT INTO backup_logs (user_id, user_email, action, status, message, metadata, file_name, ip_address, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, NOW())
	`, userID, userEmail, action, status, message, metaValue, fileName, ipAddress)

	if err != nil {
		if isTableMissingError(err) {
			return nil
		}
		return err
	}
	return nil
}

func ExportBackup(db interface{}) gin.HandlerFunc {
	return func(c *gin.Context) {
		sqlDB, ok := db.(*sql.DB)
		if !ok {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Database connection error"})
			return
		}

		var (
			currentUserIDInt int
			userEmail        string
			status           = "success"
			logMessage       = "Backup exported"
			metadata         = map[string]interface{}{}
			fileName         string
			logEnabled       bool
		)
		ipAddress := c.ClientIP()

		defer func() {
			if logEnabled {
				if err := recordBackupLog(sqlDB, currentUserIDInt, userEmail, "export", status, logMessage, fileName, ipAddress, metadata); err != nil {
					log.Printf("failed to record backup export log: %v", err)
				}
			}
		}()

		respondError := func(code int, message string) {
			status = "error"
			logMessage = message
			c.JSON(code, gin.H{"error": message})
		}

		currentUserID, exists := c.Get("user_id")
		if !exists {
			respondError(http.StatusUnauthorized, "User not authenticated")
			return
		}

		id, ok := currentUserID.(int)
		if !ok {
			respondError(http.StatusInternalServerError, "Invalid user ID format")
			return
		}
		currentUserIDInt = id
		logEnabled = true

		_, email, err := fetchUserRoleAndEmail(sqlDB, currentUserIDInt)
		if err != nil {
			respondError(http.StatusInternalServerError, "Failed to get user email")
			return
		}
		userEmail = email

		backupData := BackupData{
			Version:    "1.0",
			ExportDate: time.Now().Format("2006-01-02 15:04:05"),
		}

		targets, err := exportTargets(sqlDB)
		if err != nil {
			respondError(http.StatusInternalServerError, "Failed to export targets: "+err.Error())
			return
		}
		backupData.Targets = targets

		notificationRules, err := exportNotificationRules(sqlDB)
		if err != nil {
			respondError(http.StatusInternalServerError, "Failed to export notification rules: "+err.Error())
			return
		}
		backupData.NotificationRules = notificationRules

		jsonData, err := json.MarshalIndent(backupData, "", "  ")
		if err != nil {
			respondError(http.StatusInternalServerError, "Failed to serialize backup data")
			return
		}

		fileName = fmt.Sprintf("systrack_backup_%s.json", time.Now().Format("2006-01-02_15-04-05"))
		metadata["targets_count"] = len(backupData.Targets)
		metadata["notification_rules_count"] = len(backupData.NotificationRules)
		logMessage = fmt.Sprintf("Backup exported (%d targets, %d notification rules)", len(backupData.Targets), len(backupData.NotificationRules))

		c.Header("Content-Disposition", "attachment; filename="+fileName)
		c.Header("Content-Type", "application/json")
		c.Header("Content-Length", fmt.Sprintf("%d", len(jsonData)))

		c.Data(http.StatusOK, "application/json", jsonData)
	}
}

// ImportBackup imports backup data
func ImportBackup(db interface{}) gin.HandlerFunc {
	return func(c *gin.Context) {
		sqlDB, ok := db.(*sql.DB)
		if !ok {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Database connection error"})
			return
		}

		var (
			currentUserIDInt int
			userEmail        string
			status           = "success"
			logMessage       = "Backup imported"
			metadata         = map[string]interface{}{}
			fileName         string
			logEnabled       bool
		)
		ipAddress := c.ClientIP()

		defer func() {
			if logEnabled {
				if err := recordBackupLog(sqlDB, currentUserIDInt, userEmail, "import", status, logMessage, fileName, ipAddress, metadata); err != nil {
					log.Printf("failed to record backup import log: %v", err)
				}
			}
		}()

		respondError := func(code int, message string) {
			status = "error"
			logMessage = message
			metadata["error"] = message
			c.JSON(code, gin.H{"error": message})
		}

		currentUserID, exists := c.Get("user_id")
		if !exists {
			respondError(http.StatusUnauthorized, "User not authenticated")
			return
		}

		id, ok := currentUserID.(int)
		if !ok {
			respondError(http.StatusInternalServerError, "Invalid user ID format")
			return
		}
		currentUserIDInt = id
		logEnabled = true

		_, email, err := fetchUserRoleAndEmail(sqlDB, currentUserIDInt)
		if err != nil {
			respondError(http.StatusInternalServerError, "Failed to get user email")
			return
		}
		userEmail = email

		file, err := c.FormFile("file")
		if err != nil {
			respondError(http.StatusBadRequest, "No file uploaded")
			return
		}
		fileName = file.Filename

		src, err := file.Open()
		if err != nil {
			respondError(http.StatusInternalServerError, "Failed to open uploaded file")
			return
		}
		defer src.Close()

		fileContent, err := io.ReadAll(src)
		if err != nil {
			respondError(http.StatusInternalServerError, "Failed to read uploaded file")
			return
		}

		var backupData BackupData
		if err := json.Unmarshal(fileContent, &backupData); err != nil {
			respondError(http.StatusBadRequest, "Invalid backup file format")
			return
		}

		tx, err := sqlDB.Begin()
		if err != nil {
			respondError(http.StatusInternalServerError, "Failed to start transaction")
			return
		}
		defer tx.Rollback()

		if err := importTargets(tx, backupData.Targets); err != nil {
			respondError(http.StatusInternalServerError, "Failed to import targets: "+err.Error())
			return
		}

		if err := importNotificationRules(tx, backupData.NotificationRules); err != nil {
			respondError(http.StatusInternalServerError, "Failed to import notification rules: "+err.Error())
			return
		}

		if err := tx.Commit(); err != nil {
			respondError(http.StatusInternalServerError, "Failed to commit transaction")
			return
		}

		metadata = map[string]interface{}{
			"targets_count":            len(backupData.Targets),
			"notification_rules_count": len(backupData.NotificationRules),
		}
		logMessage = fmt.Sprintf("Backup imported (%d targets, %d notification rules)", len(backupData.Targets), len(backupData.NotificationRules))

		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "Backup imported successfully",
			"data":    metadata,
		})
	}
}

func GetBackupLogs(db interface{}) gin.HandlerFunc {
	return func(c *gin.Context) {
		sqlDB, ok := db.(*sql.DB)
		if !ok {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Database connection error"})
			return
		}

		rows, err := sqlDB.Query(`
			SELECT id, user_id, user_email, action, status, message, metadata, file_name, ip_address, created_at
			FROM backup_logs
			ORDER BY created_at DESC
		`)
		if err != nil {
			if isTableMissingError(err) {
				c.JSON(http.StatusOK, gin.H{"logs": []BackupLogEntry{}})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch backup logs"})
			return
		}
		defer rows.Close()

		logs := make([]BackupLogEntry, 0)

		for rows.Next() {
			var (
				entry     BackupLogEntry
				userID    sql.NullInt64
				userEmail sql.NullString
				message   sql.NullString
				metadata  []byte
				fileName  sql.NullString
				ip        sql.NullString
				createdAt time.Time
			)

			if err := rows.Scan(&entry.ID, &userID, &userEmail, &entry.Action, &entry.Status, &message, &metadata, &fileName, &ip, &createdAt); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to parse backup logs"})
				return
			}

			if userID.Valid {
				uid := int(userID.Int64)
				entry.UserID = &uid
			}
			if userEmail.Valid {
				entry.UserEmail = userEmail.String
			}
			if message.Valid {
				entry.Message = message.String
			}
			if fileName.Valid {
				entry.FileName = fileName.String
			}
			if ip.Valid {
				entry.IPAddress = ip.String
			}

			if len(metadata) > 0 {
				var meta map[string]interface{}
				if err := json.Unmarshal(metadata, &meta); err == nil {
					entry.Metadata = meta
				} else {
					entry.Metadata = map[string]interface{}{"raw": string(metadata)}
				}
			}

			entry.CreatedAt = createdAt.Format(time.RFC3339)
			logs = append(logs, entry)
		}

		if err := rows.Err(); err != nil {
			if isTableMissingError(err) {
				c.JSON(http.StatusOK, gin.H{"logs": []BackupLogEntry{}})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to read backup logs"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"logs": logs})
	}
}

func isTableMissingError(err error) bool {
	if err == nil {
		return false
	}
	errMsg := strings.ToLower(err.Error())
	return strings.Contains(errMsg, "error 1146") ||
		strings.Contains(errMsg, "no such table") ||
		(strings.Contains(errMsg, "table") && strings.Contains(errMsg, "doesn't exist"))
}

// exportTargets exports all targets
func exportTargets(db *sql.DB) ([]TargetBackup, error) {
	query := `
			SELECT id, name, address, type, monitoring_type, metrics_enabled,
			       COALESCE(snmp_community,'public'), COALESCE(snmp_version,'v2c'),
			       port, path,
			       http_method, http_path, http_headers, expected_status_code,
			       expected_content, ssl_check, follow_redirects, timeout_sec,
			       interval_sec, timeout_ms, enabled, tags, 
			       DATE_FORMAT(created_at, '%Y-%m-%d %H:%i:%s') as created_at,
			       DATE_FORMAT(updated_at, '%Y-%m-%d %H:%i:%s') as updated_at
			FROM targets
			ORDER BY id
		`
	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var targets []TargetBackup
	for rows.Next() {
		var target TargetBackup
		err := rows.Scan(
			&target.ID, &target.Name, &target.Address, &target.Type, &target.MonitoringType,
			&target.MetricsEnabled, &target.SNMPCommunity, &target.SNMPVersion,
			&target.Port, &target.Path, &target.HTTPMethod, &target.HTTPPath, &target.HTTPHeaders,
			&target.ExpectedStatusCode, &target.ExpectedContent, &target.SSLCheck, &target.FollowRedirects,
			&target.TimeoutSec, &target.IntervalSec, &target.TimeoutMs, &target.Enabled, &target.Tags,
			&target.CreatedAt, &target.UpdatedAt,
		)
		if err != nil {
			continue
		}
		targets = append(targets, target)
	}

	return targets, nil
}

// exportNotificationRules exports all notification rules
func exportNotificationRules(db *sql.DB) ([]NotificationRulesBackup, error) {
	query := `
			SELECT id, name, target_id, entity_type, channel, schedule_interval_minutes,
			       recipients, template_id, conditions, is_active, created_by,
			       CASE WHEN last_sent_at IS NOT NULL THEN DATE_FORMAT(last_sent_at, '%Y-%m-%d %H:%i:%s') ELSE NULL END as last_sent_at,
			       DATE_FORMAT(created_at, '%Y-%m-%d %H:%i:%s') as created_at,
			       DATE_FORMAT(updated_at, '%Y-%m-%d %H:%i:%s') as updated_at
			FROM new_notification_rules
			ORDER BY id
		`
	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rules []NotificationRulesBackup
	for rows.Next() {
		var rule NotificationRulesBackup
		err := rows.Scan(
			&rule.ID, &rule.Name, &rule.TargetID, &rule.EntityType, &rule.Channel, &rule.ScheduleIntervalMinutes,
			&rule.Recipients, &rule.TemplateID, &rule.Conditions, &rule.IsActive, &rule.CreatedBy,
			&rule.LastSentAt, &rule.CreatedAt, &rule.UpdatedAt,
		)
		if err != nil {
			continue
		}
		rules = append(rules, rule)
	}

	return rules, nil
}

// importTargets imports targets data
func importTargets(tx *sql.Tx, targets []TargetBackup) error {
	hasMetricsColumns, err := targetColumnsExist(tx, "metrics_enabled", "snmp_community", "snmp_version")
	if err != nil {
		return err
	}

	queryWithMetrics := `
		INSERT INTO targets (id, name, address, type, monitoring_type, metrics_enabled, snmp_community, snmp_version, port, path,
		                    http_method, http_path, http_headers, expected_status_code,
		                    expected_content, ssl_check, follow_redirects, timeout_sec,
		                    interval_sec, timeout_ms, enabled, tags, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
		name = VALUES(name),
		address = VALUES(address),
		type = VALUES(type),
		monitoring_type = VALUES(monitoring_type),
		metrics_enabled = VALUES(metrics_enabled),
		snmp_community = VALUES(snmp_community),
		snmp_version = VALUES(snmp_version),
		port = VALUES(port),
		path = VALUES(path),
		http_method = VALUES(http_method),
		http_path = VALUES(http_path),
		http_headers = VALUES(http_headers),
		expected_status_code = VALUES(expected_status_code),
		expected_content = VALUES(expected_content),
		ssl_check = VALUES(ssl_check),
		follow_redirects = VALUES(follow_redirects),
		timeout_sec = VALUES(timeout_sec),
		interval_sec = VALUES(interval_sec),
		timeout_ms = VALUES(timeout_ms),
		enabled = VALUES(enabled),
		tags = VALUES(tags),
		updated_at = VALUES(updated_at)
	`

	queryWithoutMetrics := `
		INSERT INTO targets (id, name, address, type, monitoring_type, port, path,
		                    http_method, http_path, http_headers, expected_status_code,
		                    expected_content, ssl_check, follow_redirects, timeout_sec,
		                    interval_sec, timeout_ms, enabled, tags, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
		name = VALUES(name),
		address = VALUES(address),
		type = VALUES(type),
		monitoring_type = VALUES(monitoring_type),
		port = VALUES(port),
		path = VALUES(path),
		http_method = VALUES(http_method),
		http_path = VALUES(http_path),
		http_headers = VALUES(http_headers),
		expected_status_code = VALUES(expected_status_code),
		expected_content = VALUES(expected_content),
		ssl_check = VALUES(ssl_check),
		follow_redirects = VALUES(follow_redirects),
		timeout_sec = VALUES(timeout_sec),
		interval_sec = VALUES(interval_sec),
		timeout_ms = VALUES(timeout_ms),
		enabled = VALUES(enabled),
		tags = VALUES(tags),
		updated_at = VALUES(updated_at)
	`

	for _, target := range targets {
		// Convert datetime strings to MySQL format
		createdAt := convertDateTimeToMySQL(target.CreatedAt)
		updatedAt := convertDateTimeToMySQL(target.UpdatedAt)

		if target.SNMPCommunity == "" {
			target.SNMPCommunity = "public"
		}
		if target.SNMPVersion == "" {
			target.SNMPVersion = "v2c"
		}

		if hasMetricsColumns {
			_, err := tx.Exec(queryWithMetrics,
				target.ID, target.Name, target.Address, target.Type, target.MonitoringType,
				target.MetricsEnabled, target.SNMPCommunity, target.SNMPVersion,
				target.Port, target.Path, target.HTTPMethod, target.HTTPPath, target.HTTPHeaders,
				target.ExpectedStatusCode, target.ExpectedContent, target.SSLCheck, target.FollowRedirects,
				target.TimeoutSec, target.IntervalSec, target.TimeoutMs, target.Enabled, target.Tags,
				createdAt, updatedAt,
			)
			if err != nil {
				return err
			}
			continue
		}

		_, err := tx.Exec(queryWithoutMetrics,
			target.ID, target.Name, target.Address, target.Type, target.MonitoringType,
			target.Port, target.Path, target.HTTPMethod, target.HTTPPath, target.HTTPHeaders,
			target.ExpectedStatusCode, target.ExpectedContent, target.SSLCheck, target.FollowRedirects,
			target.TimeoutSec, target.IntervalSec, target.TimeoutMs, target.Enabled, target.Tags,
			createdAt, updatedAt,
		)
		if err != nil {
			return err
		}
	}
	return nil
}

func targetColumnsExist(tx *sql.Tx, columns ...string) (bool, error) {
	if len(columns) == 0 {
		return true, nil
	}

	for _, column := range columns {
		var count int
		err := tx.QueryRow(`
			SELECT COUNT(*) FROM information_schema.COLUMNS
			WHERE table_schema = DATABASE() AND table_name = 'targets' AND column_name = ?
		`, column).Scan(&count)
		if err != nil {
			return false, err
		}
		if count == 0 {
			return false, nil
		}
	}

	return true, nil
}

// importNotificationRules imports notification rules data
func importNotificationRules(tx *sql.Tx, rules []NotificationRulesBackup) error {
	for _, rule := range rules {
		// Convert datetime strings to MySQL format
		createdAt := convertDateTimeToMySQL(rule.CreatedAt)
		updatedAt := convertDateTimeToMySQL(rule.UpdatedAt)

		var lastSentAt interface{}
		if rule.LastSentAt != nil && *rule.LastSentAt != "" {
			converted := convertDateTimeToMySQL(*rule.LastSentAt)
			lastSentAt = converted
		} else {
			lastSentAt = nil
		}

		query := `
			INSERT INTO new_notification_rules (id, name, target_id, entity_type, channel, schedule_interval_minutes,
			                                     recipients, template_id, conditions, is_active, created_by,
			                                     last_sent_at, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON DUPLICATE KEY UPDATE
			name = VALUES(name),
			target_id = VALUES(target_id),
			entity_type = VALUES(entity_type),
			channel = VALUES(channel),
			schedule_interval_minutes = VALUES(schedule_interval_minutes),
			recipients = VALUES(recipients),
			template_id = VALUES(template_id),
			conditions = VALUES(conditions),
			is_active = VALUES(is_active),
			created_by = VALUES(created_by),
			last_sent_at = VALUES(last_sent_at),
			updated_at = VALUES(updated_at)
		`
		_, err := tx.Exec(query,
			rule.ID, rule.Name, rule.TargetID, rule.EntityType, rule.Channel, rule.ScheduleIntervalMinutes,
			rule.Recipients, rule.TemplateID, rule.Conditions, rule.IsActive, rule.CreatedBy,
			lastSentAt, createdAt, updatedAt,
		)
		if err != nil {
			return err
		}
	}
	return nil
}
