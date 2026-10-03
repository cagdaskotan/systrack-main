package handlers

import (
	"database/sql"
	"encoding/csv"
	"fmt"
	"io"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"systrack/internal/ping"

	"github.com/gin-gonic/gin"
)

// Global maps to store tag colors and descriptions (in-memory for demo)
// In production, use a proper database table
var (
	tagColors       map[string]string
	tagDescriptions map[string]string
)

type Target struct {
	ID                 int               `json:"id"`
	Name               string            `json:"name"`
	Address            string            `json:"address"`
	Type               string            `json:"type"`
	MonitoringType     string            `json:"monitoring_type"`
	MetricsEnabled     bool              `json:"metrics_enabled"`
	SNMPCommunity      string            `json:"snmp_community"`
	SNMPVersion        string            `json:"snmp_version"`
	Port               *int              `json:"port,omitempty"`
	Path               *string           `json:"path,omitempty"`
	HTTPMethod         string            `json:"http_method"`
	HTTPPath           string            `json:"http_path"`
	HTTPHeaders        *string           `json:"http_headers,omitempty"`
	ExpectedStatusCode int               `json:"expected_status_code"`
	ExpectedContent    *string           `json:"expected_content,omitempty"`
	SSLCheck           bool              `json:"ssl_check"`
	FollowRedirects    bool              `json:"follow_redirects"`
	TimeoutSec         int               `json:"timeout_sec"`
	IntervalSec        int               `json:"interval_sec"`
	TimeoutMs          int               `json:"timeout_ms"`
	Enabled            bool              `json:"enabled"`
	IsOnline           bool              `json:"is_online"`
	Tags               *string           `json:"tags,omitempty"`
	TagColors          map[string]string `json:"tag_colors,omitempty"`
	CreatedAt          time.Time         `json:"created_at"`
	UpdatedAt          time.Time         `json:"updated_at"`
}

func GetTargets(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Pagination parameters
		limit, _ := strconv.Atoi(c.DefaultQuery("limit", "25"))
		offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))

		// Validate and cap limit
		if limit > 100 {
			limit = 100
		}
		if limit < 1 {
			limit = 25
		}
		if offset < 0 {
			offset = 0
		}

		// Filter parameters
		searchFilter := c.Query("search")
		typeFilter := c.Query("type")
		statusFilter := c.Query("status")
		tagFilter := c.Query("tag")

		// Build WHERE clause for filters
		var whereConditions []string
		var queryArgs []interface{}
		var countArgs []interface{}

		if searchFilter != "" {
			searchPattern := "%" + searchFilter + "%"
			whereConditions = append(whereConditions, "(t.name LIKE ? OR t.address LIKE ? OR t.tags LIKE ?)")
			queryArgs = append(queryArgs, searchPattern, searchPattern, searchPattern)
			countArgs = append(countArgs, searchPattern, searchPattern, searchPattern)
		}

		if typeFilter != "" {
			// Normalize type filter to match backend storage
			normalizedType := strings.ToLower(typeFilter)
			if normalizedType == "ping" {
				whereConditions = append(whereConditions, "(t.monitoring_type IN ('ping', 'icmp', 'tcp') OR t.type IN ('ping', 'icmp', 'tcp'))")
			} else {
				whereConditions = append(whereConditions, "(t.monitoring_type = ? OR t.type = ?)")
				queryArgs = append(queryArgs, normalizedType, normalizedType)
				countArgs = append(countArgs, normalizedType, normalizedType)
			}
		}

		if statusFilter != "" {
			if strings.ToLower(statusFilter) == "online" {
				whereConditions = append(whereConditions, "p.ok = 1")
			} else if strings.ToLower(statusFilter) == "offline" {
				whereConditions = append(whereConditions, "(p.ok = 0 OR p.ok IS NULL)")
			}
		}

		if tagFilter != "" {
			tagPattern := "%" + tagFilter + "%"
			whereConditions = append(whereConditions, "t.tags LIKE ?")
			queryArgs = append(queryArgs, tagPattern)
			countArgs = append(countArgs, tagPattern)
		}

		whereClause := ""
		if len(whereConditions) > 0 {
			whereClause = " WHERE " + strings.Join(whereConditions, " AND ")
		}

		// Get total count with filters
		var total int
		countQuery := `
            SELECT COUNT(t.id)
            FROM targets t
            LEFT JOIN pings_raw_momentary p ON p.id = (
                SELECT id FROM pings_raw_momentary WHERE target_id = t.id ORDER BY created_at DESC LIMIT 1
            )` + whereClause
		err := db.QueryRow(countQuery, countArgs...).Scan(&total)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to count targets"})
			return
		}

		// Get paginated data with filters
		query := `
			SELECT
				t.id, t.name, t.address, t.type, t.monitoring_type,
				t.metrics_enabled, COALESCE(t.snmp_community, 'public') as snmp_community,
				COALESCE(t.snmp_version, 'v2c') as snmp_version,
				t.port, t.path,
				t.http_method, t.http_path, t.http_headers, t.expected_status_code,
				t.expected_content, t.ssl_check, t.follow_redirects, t.timeout_sec,
				t.interval_sec, t.timeout_ms, t.enabled, t.tags,
				t.created_at, t.updated_at,
				COALESCE(p.ok, 0) as is_online
			FROM targets t
			LEFT JOIN pings_raw_momentary p ON p.id = (
                SELECT id FROM pings_raw_momentary WHERE target_id = t.id ORDER BY created_at DESC LIMIT 1
            )` + whereClause + `
			ORDER BY t.created_at DESC
			LIMIT ? OFFSET ?
		`
		queryArgs = append(queryArgs, limit, offset)
		rows, err := db.Query(query, queryArgs...)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch targets"})
			return
		}
		defer rows.Close()

		// Initialize as empty slice instead of nil to avoid null in JSON
		targets := make([]Target, 0)
		for rows.Next() {
			var target Target
			var httpHeaders sql.NullString
			var expectedContent sql.NullString

			err := rows.Scan(
				&target.ID, &target.Name, &target.Address, &target.Type, &target.MonitoringType,
				&target.MetricsEnabled, &target.SNMPCommunity, &target.SNMPVersion,
				&target.Port, &target.Path, &target.HTTPMethod, &target.HTTPPath, &httpHeaders,
				&target.ExpectedStatusCode, &expectedContent, &target.SSLCheck, &target.FollowRedirects,
				&target.TimeoutSec, &target.IntervalSec, &target.TimeoutMs, &target.Enabled,
				&target.Tags, &target.CreatedAt, &target.UpdatedAt, &target.IsOnline,
			)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to scan target"})
				return
			}

			if httpHeaders.Valid {
				target.HTTPHeaders = &httpHeaders.String
			}
			if expectedContent.Valid {
				target.ExpectedContent = &expectedContent.String
			}

			// Add tag colors for this target
			target.TagColors = make(map[string]string)
			if target.Tags != nil && *target.Tags != "" {
				tagList := strings.Split(*target.Tags, ",")
				for _, tagName := range tagList {
					tagName = strings.TrimSpace(tagName)
					if tagName != "" {
						if tagColors != nil && tagColors[tagName] != "" {
							target.TagColors[tagName] = tagColors[tagName]
						} else {
							target.TagColors[tagName] = "#3B82F6" // Default blue
						}
					}
				}
			}

			targets = append(targets, target)
		}

		// Calculate if there are more pages
		hasMore := offset+limit < total

		c.JSON(http.StatusOK, gin.H{
			"targets": targets,
			"pagination": gin.H{
				"total":    total,
				"limit":    limit,
				"offset":   offset,
				"has_more": hasMore,
			},
		})
	}
}

func GetTarget(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		idStr := c.Param("id")
		id, err := strconv.Atoi(idStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid target ID"})
			return
		}

		var target Target
		query := `SELECT id, name, address, type, monitoring_type, metrics_enabled, COALESCE(snmp_community,'public'), COALESCE(snmp_version,'v2c'), port, path, interval_sec, timeout_ms, enabled, tags, created_at, updated_at FROM targets WHERE id = ?`
		err = db.QueryRow(query, id).Scan(&target.ID, &target.Name, &target.Address, &target.Type, &target.MonitoringType, &target.MetricsEnabled, &target.SNMPCommunity, &target.SNMPVersion, &target.Port, &target.Path, &target.IntervalSec, &target.TimeoutMs, &target.Enabled, &target.Tags, &target.CreatedAt, &target.UpdatedAt)
		if err != nil {
			if err == sql.ErrNoRows {
				c.JSON(http.StatusNotFound, gin.H{"error": "Target not found"})
			} else {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch target"})
			}
			return
		}

		c.JSON(http.StatusOK, target)
	}
}

func CreateTarget(db *sql.DB, scheduler interface{}, hub interface{}) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			Name               string  `json:"name" binding:"required"`
			Address            string  `json:"address" binding:"required"`
			Type               string  `json:"type" binding:"required,oneof=icmp tcp http https"`
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
			IntervalSec        int     `json:"interval_sec" binding:"min=10,max=3600"`
			TimeoutMs          int     `json:"timeout_ms" binding:"min=100,max=30000"`
			Enabled            bool    `json:"enabled"`
			Tags               *string `json:"tags,omitempty"`
		}

		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		// Default değerler
		if req.MonitoringType == "" {
			// Type'dan monitoring_type'a mapping yap
			switch req.Type {
			case "icmp":
				req.MonitoringType = "ping"
			case "http":
				req.MonitoringType = "http"
			case "https":
				req.MonitoringType = "https"
			case "tcp":
				req.MonitoringType = "tcp"
			default:
				req.MonitoringType = "ping"
			}
		}

		// Monitoring type'ı logla
		fmt.Printf("Creating target with type: %s, monitoring_type: %s\n", req.Type, req.MonitoringType)
		if req.HTTPMethod == "" {
			req.HTTPMethod = "GET"
		}
		if req.HTTPPath == "" {
			req.HTTPPath = "/"
		}
		if req.ExpectedStatusCode == 0 {
			req.ExpectedStatusCode = 200
		}
		if req.TimeoutSec == 0 {
			req.TimeoutSec = 10
		}
		if req.IntervalSec == 0 {
			req.IntervalSec = 30
		}
		if req.TimeoutMs == 0 {
			req.TimeoutMs = 1000
		}
		if req.SNMPCommunity == "" {
			req.SNMPCommunity = "public"
		}
		if req.SNMPVersion == "" {
			req.SNMPVersion = "v2c"
		} else {
			switch req.SNMPVersion {
			case "v1", "v2c", "v3":
			default:
				c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid SNMP version"})
				return
			}
		}

		var existingCount int
		err := db.QueryRow(
			"SELECT COUNT(*) FROM targets WHERE address = ? AND monitoring_type = ?",
			req.Address, req.MonitoringType,
		).Scan(&existingCount)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to validate target uniqueness"})
			return
		}
		if existingCount > 0 {
			c.JSON(http.StatusConflict, gin.H{
				"error": "duplicate_target",
			})
			return
		}

		query := `INSERT INTO targets (name, address, type, monitoring_type, metrics_enabled, snmp_community, snmp_version, port, path, http_method, http_path, http_headers, expected_status_code, expected_content, ssl_check, follow_redirects, timeout_sec, interval_sec, timeout_ms, enabled, tags) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
		result, err := db.Exec(query, req.Name, req.Address, req.Type, req.MonitoringType, req.MetricsEnabled, req.SNMPCommunity, req.SNMPVersion, req.Port, req.Path, req.HTTPMethod, req.HTTPPath, req.HTTPHeaders, req.ExpectedStatusCode, req.ExpectedContent, req.SSLCheck, req.FollowRedirects, req.TimeoutSec, req.IntervalSec, req.TimeoutMs, req.Enabled, req.Tags)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create target"})
			return
		}

		id, err := result.LastInsertId()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get target ID"})
			return
		}

		// Otomatik ping at (eğer enabled ise)
		if req.Enabled && scheduler != nil {
			targetID := int(id)
			log.Printf("🎯 Triggering immediate ping for new target ID: %d", targetID)
			go func() {
				// Scheduler'dan PingTarget methodunu çağır
				if s, ok := scheduler.(interface {
					PingTarget(int) (bool, time.Duration, error)
				}); ok {
					log.Printf("⚡ Executing immediate ping for target ID: %d", targetID)
					success, duration, err := s.PingTarget(targetID)
					if err != nil {
						log.Printf("❌ Immediate ping failed for target ID %d: %v", targetID, err)
					} else {
						log.Printf("✅ Immediate ping completed for target ID %d: success=%v duration=%v", targetID, success, duration)

						// WebSocket üzerinden status update gönder
						if hub != nil {
							if h, ok := hub.(interface{ BroadcastStatusUpdate(int, string) }); ok {
								status := "offline"
								if success {
									status = "online"
								}
								log.Printf("📡 Broadcasting initial status for new target ID %d: %s", targetID, status)
								h.BroadcastStatusUpdate(targetID, status)
							}
						}
					}
				}
			}()
		}

		c.JSON(http.StatusCreated, gin.H{"id": id, "message": "Target created successfully"})
	}
}

func UpdateTarget(db *sql.DB, scheduler interface{}) gin.HandlerFunc {
	return func(c *gin.Context) {
		idStr := c.Param("id")
		id, err := strconv.Atoi(idStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid target ID"})
			return
		}

		var req struct {
			Name               *string `json:"name,omitempty"`
			Address            *string `json:"address,omitempty"`
			Type               *string `json:"type,omitempty" binding:"omitempty,oneof=icmp tcp http https"`
			MonitoringType     *string `json:"monitoring_type,omitempty"`
			MetricsEnabled     *bool   `json:"metrics_enabled,omitempty"`
			SNMPCommunity      *string `json:"snmp_community,omitempty"`
			SNMPVersion        *string `json:"snmp_version,omitempty"`
			Port               *int    `json:"port,omitempty"`
			Path               *string `json:"path,omitempty"`
			HTTPMethod         *string `json:"http_method,omitempty"`
			HTTPPath           *string `json:"http_path,omitempty"`
			HTTPHeaders        *string `json:"http_headers,omitempty"`
			ExpectedStatusCode *int    `json:"expected_status_code,omitempty"`
			ExpectedContent    *string `json:"expected_content,omitempty"`
			SSLCheck           *bool   `json:"ssl_check,omitempty"`
			FollowRedirects    *bool   `json:"follow_redirects,omitempty"`
			TimeoutSec         *int    `json:"timeout_sec,omitempty"`
			IntervalSec        *int    `json:"interval_sec,omitempty" binding:"omitempty,min=10,max=3600"`
			TimeoutMs          *int    `json:"timeout_ms,omitempty" binding:"omitempty,min=100,max=30000"`
			Enabled            *bool   `json:"enabled,omitempty"`
			Tags               *string `json:"tags,omitempty"`
		}

		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		// Güncelleme sorgusu oluştur
		setParts := []string{}
		args := []interface{}{}

		if req.Name != nil {
			setParts = append(setParts, "name = ?")
			args = append(args, *req.Name)
		}
		if req.Address != nil {
			setParts = append(setParts, "address = ?")
			args = append(args, *req.Address)
		}
		if req.Type != nil {
			setParts = append(setParts, "type = ?")
			args = append(args, *req.Type)
		}
		if req.MonitoringType != nil {
			setParts = append(setParts, "monitoring_type = ?")
			args = append(args, *req.MonitoringType)
		}
		if req.MetricsEnabled != nil {
			setParts = append(setParts, "metrics_enabled = ?")
			args = append(args, *req.MetricsEnabled)
		}
		if req.SNMPCommunity != nil {
			setParts = append(setParts, "snmp_community = ?")
			args = append(args, *req.SNMPCommunity)
		}
		if req.SNMPVersion != nil {
			switch *req.SNMPVersion {
			case "v1", "v2c", "v3":
			default:
				c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid SNMP version"})
				return
			}
			setParts = append(setParts, "snmp_version = ?")
			args = append(args, *req.SNMPVersion)
		}
		if req.Port != nil {
			setParts = append(setParts, "port = ?")
			args = append(args, *req.Port)
		}
		if req.Path != nil {
			setParts = append(setParts, "path = ?")
			args = append(args, *req.Path)
		}
		if req.HTTPMethod != nil {
			setParts = append(setParts, "http_method = ?")
			args = append(args, *req.HTTPMethod)
		}
		if req.HTTPPath != nil {
			setParts = append(setParts, "http_path = ?")
			args = append(args, *req.HTTPPath)
		}
		if req.HTTPHeaders != nil {
			setParts = append(setParts, "http_headers = ?")
			args = append(args, *req.HTTPHeaders)
		}
		if req.ExpectedStatusCode != nil {
			setParts = append(setParts, "expected_status_code = ?")
			args = append(args, *req.ExpectedStatusCode)
		}
		if req.ExpectedContent != nil {
			setParts = append(setParts, "expected_content = ?")
			args = append(args, *req.ExpectedContent)
		}
		if req.SSLCheck != nil {
			setParts = append(setParts, "ssl_check = ?")
			args = append(args, *req.SSLCheck)
		}
		if req.FollowRedirects != nil {
			setParts = append(setParts, "follow_redirects = ?")
			args = append(args, *req.FollowRedirects)
		}
		if req.TimeoutSec != nil {
			setParts = append(setParts, "timeout_sec = ?")
			args = append(args, *req.TimeoutSec)
		}
		if req.IntervalSec != nil {
			setParts = append(setParts, "interval_sec = ?")
			args = append(args, *req.IntervalSec)
		}
		if req.TimeoutMs != nil {
			setParts = append(setParts, "timeout_ms = ?")
			args = append(args, *req.TimeoutMs)
		}
		if req.Enabled != nil {
			setParts = append(setParts, "enabled = ?")
			args = append(args, *req.Enabled)
		}
		if req.Tags != nil {
			setParts = append(setParts, "tags = ?")
			args = append(args, *req.Tags)
		}

		if len(setParts) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "No fields to update"})
			return
		}

		args = append(args, id)
		query := "UPDATE targets SET " + strings.Join(setParts, ", ") + " WHERE id = ?"

		_, err = db.Exec(query, args...)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update target"})
			return
		}

		// Hedefin enabled durumunu kontrol et ve otomatik ping at
		var enabled bool
		err = db.QueryRow("SELECT enabled FROM targets WHERE id = ?", id).Scan(&enabled)
		if err == nil && enabled && scheduler != nil {
			log.Printf("🎯 Triggering immediate ping for updated target ID: %d", id)
			go func() {
				// Scheduler'dan PingTarget methodunu çağır
				if s, ok := scheduler.(interface {
					PingTarget(int) (bool, time.Duration, error)
				}); ok {
					log.Printf("⚡ Executing immediate ping for updated target ID: %d", id)
					success, duration, err := s.PingTarget(id)
					if err != nil {
						log.Printf("❌ Immediate ping failed for target ID %d: %v", id, err)
					} else {
						log.Printf("✅ Immediate ping completed for target ID %d: success=%v duration=%v", id, success, duration)
					}
				}
			}()
		}

		c.JSON(http.StatusOK, gin.H{"message": "Target updated successfully"})
	}
}

func DeleteTarget(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		idStr := c.Param("id")
		id, err := strconv.Atoi(idStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid target ID"})
			return
		}

		deletedCount, err := deleteTargetsWithNotifications(db, []int{id})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete target and related notifications"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"message":       "Target deleted successfully",
			"deleted_count": deletedCount,
		})
	}
}

func PingNow(db *sql.DB, scheduler interface{}) gin.HandlerFunc {
	return func(c *gin.Context) {
		idStr := c.Param("id")
		id, err := strconv.Atoi(idStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid target ID"})
			return
		}

		// Target'ı kontrol et
		var target Target
		query := `SELECT id, name, address, type, monitoring_type, port, path, interval_sec, timeout_ms, enabled, 
		          http_method, http_path, http_headers, expected_status_code, expected_content, ssl_check, follow_redirects, timeout_sec 
		          FROM targets WHERE id = ?`
		err = db.QueryRow(query, id).Scan(&target.ID, &target.Name, &target.Address, &target.Type, &target.MonitoringType,
			&target.Port, &target.Path, &target.IntervalSec, &target.TimeoutMs, &target.Enabled,
			&target.HTTPMethod, &target.HTTPPath, &target.HTTPHeaders, &target.ExpectedStatusCode,
			&target.ExpectedContent, &target.SSLCheck, &target.FollowRedirects, &target.TimeoutSec)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "Target not found"})
			return
		}

		// HTTP/HTTPS monitoring için özel logic
		if target.MonitoringType == "http" || target.MonitoringType == "https" {
			// Default değerleri ayarla
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

			// HTTP monitoring engine kullan
			httpConfig := ping.HTTPMonitoringConfig{
				Method: target.HTTPMethod,
				Path:   target.HTTPPath,
				Headers: func() map[string]string {
					if target.HTTPHeaders != nil && *target.HTTPHeaders != "" {
						headers := make(map[string]string)
						// Simple JSON parsing for headers
						// TODO: Implement proper JSON parsing
						return headers
					}
					return make(map[string]string)
				}(),
				ExpectedStatus: target.ExpectedStatusCode,
				ExpectedContent: func() string {
					if target.ExpectedContent != nil {
						return *target.ExpectedContent
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
			httpEngine := ping.NewHTTPMonitoringEngine(httpConfig)

			// Target'ı HTTP engine için hazırla
			httpTarget := ping.Target{
				Address:        target.Address,
				MonitoringType: target.MonitoringType,
				HTTPMethod:     target.HTTPMethod,
				HTTPPath:       target.HTTPPath,
				HTTPHeaders: func() string {
					if target.HTTPHeaders != nil {
						return *target.HTTPHeaders
					}
					return ""
				}(),
				ExpectedStatusCode: target.ExpectedStatusCode,
				ExpectedContent: func() string {
					if target.ExpectedContent != nil {
						return *target.ExpectedContent
					}
					return ""
				}(),
				SSLCheck:        target.SSLCheck,
				FollowRedirects: target.FollowRedirects,
				TimeoutSec:      target.TimeoutSec,
			}

			// HTTP monitoring yap
			result, err := httpEngine.MonitorHTTP(httpTarget)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{
					"error":   "HTTP monitoring failed",
					"message": err.Error(),
				})
				return
			}

			// HTTP-specific response
			response := gin.H{
				"message":  "HTTP monitoring completed",
				"target":   target.Name,
				"success":  result.Success,
				"duration": result.Duration.Milliseconds(),
				"status": func() string {
					if result.Success {
						return "online"
					}
					return "offline"
				}(),
			}

			// HTTP-specific fields ekle
			if result.StatusCode > 0 {
				response["status_code"] = result.StatusCode
			}
			if result.ResponseSizeBytes > 0 {
				response["response_size"] = result.ResponseSizeBytes
			}
			if result.SSLExpiryDate != nil {
				response["ssl_expiry"] = result.SSLExpiryDate.Format("2006-01-02 15:04:05")
			}

			c.JSON(http.StatusOK, response)
			return
		}

		// Scheduler'dan ping yap (sadece ping monitoring için)
		if scheduler != nil {
			if pingMethod, ok := scheduler.(interface {
				PingTarget(targetID int) (bool, time.Duration, error)
			}); ok {
				success, duration, err := pingMethod.PingTarget(id)
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{
						"error":   "Ping failed",
						"message": err.Error(),
					})
					return
				}

				c.JSON(http.StatusOK, gin.H{
					"message":  "Ping completed",
					"target":   target.Name,
					"success":  success,
					"duration": duration.Milliseconds(),
					"status": func() string {
						if success {
							return "online"
						}
						return "offline"
					}(),
				})
				return
			}
		}

		// Fallback: Mock response
		c.JSON(http.StatusOK, gin.H{
			"message": "Ping initiated (mock)",
			"target":  target.Name,
			"status":  "success",
		})
	}
}

// GetFilterOptions returns dynamic filter options for targets
func GetFilterOptions(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Get distinct types from targets
		typeRows, err := db.Query(`SELECT DISTINCT type FROM targets WHERE type IS NOT NULL ORDER BY type`)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch types"})
			return
		}
		defer typeRows.Close()

		var types []string
		for typeRows.Next() {
			var targetType string
			if err := typeRows.Scan(&targetType); err == nil {
				types = append(types, targetType)
			}
		}

		// Get distinct tags from targets
		tagRows, err := db.Query(`SELECT DISTINCT tags FROM targets WHERE tags IS NOT NULL AND tags != ''`)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch tags"})
			return
		}
		defer tagRows.Close()

		tagSet := make(map[string]bool)
		for tagRows.Next() {
			var tags sql.NullString
			if err := tagRows.Scan(&tags); err == nil && tags.Valid {
				tagList := strings.Split(tags.String, ",")
				for _, tag := range tagList {
					tag = strings.TrimSpace(tag)
					if tag != "" {
						tagSet[tag] = true
					}
				}
			}
		}

		var tags []string
		for tag := range tagSet {
			tags = append(tags, tag)
		}
		sort.Strings(tags)

		// Status options are always the same
		statuses := []string{"online", "offline"}

		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"data": gin.H{
				"types":    types,
				"tags":     tags,
				"statuses": statuses,
			},
		})
	}
}

// GetAvailableTags returns all unique tags used in targets
func GetAvailableTags(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		query := `SELECT DISTINCT tags FROM targets WHERE tags IS NOT NULL AND tags != ''`

		rows, err := db.Query(query)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch tags"})
			return
		}
		defer rows.Close()

		tagSet := make(map[string]bool)
		for rows.Next() {
			var tags sql.NullString
			err := rows.Scan(&tags)
			if err != nil {
				continue
			}

			if tags.Valid && tags.String != "" {
				// Split tags by comma and add each to the set
				tagList := strings.Split(tags.String, ",")
				for _, tag := range tagList {
					tag = strings.TrimSpace(tag)
					if tag != "" {
						tagSet[tag] = true
					}
				}
			}
		}

		// Convert map to slice
		var availableTags []string
		for tag := range tagSet {
			availableTags = append(availableTags, tag)
		}

		// Sort tags alphabetically
		sort.Strings(availableTags)

		// Create detailed tag objects with colors and descriptions
		var detailedTags []gin.H
		for _, tagName := range availableTags {
			// Count targets that use this tag
			countQuery := `SELECT COUNT(*) FROM targets WHERE tags LIKE ?`
			var targetCount int
			err := db.QueryRow(countQuery, "%"+tagName+"%").Scan(&targetCount)
			if err != nil {
				targetCount = 0 // Default to 0 if error
			}

			tagInfo := gin.H{
				"name":         tagName,
				"color":        "#3B82F6", // Default blue
				"description":  "",
				"target_count": targetCount,
			}

			// Use stored color if available, otherwise assign default colors
			if tagColors != nil && tagColors[tagName] != "" {
				tagInfo["color"] = tagColors[tagName]
			} else {
				// Assign default colors for existing tags
				switch tagName {
				case "Ping":
					tagInfo["color"] = "#84cc16" // Lime green
				case "Web":
					tagInfo["color"] = "#8b5cf6" // Purple
				case "deneme":
					tagInfo["color"] = "#3B82F6" // Blue
				default:
					tagInfo["color"] = "#3B82F6" // Default blue
				}
			}

			// Use stored description if available
			if tagDescriptions != nil && tagDescriptions[tagName] != "" {
				tagInfo["description"] = tagDescriptions[tagName]
			}

			detailedTags = append(detailedTags, tagInfo)
		}

		c.JSON(http.StatusOK, gin.H{"tags": detailedTags})
	}
}

// CreateTag creates a new tag
func CreateTag(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var tag struct {
			Name        string `json:"name" binding:"required"`
			Color       string `json:"color"`
			Description string `json:"description"`
		}

		if err := c.ShouldBindJSON(&tag); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		// Store tag info in a simple in-memory map for demo purposes
		// In production, you'd use a proper database table
		if tagColors == nil {
			tagColors = make(map[string]string)
		}
		if tagDescriptions == nil {
			tagDescriptions = make(map[string]string)
		}

		tagColors[tag.Name] = tag.Color
		tagDescriptions[tag.Name] = tag.Description

		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "Tag created successfully",
			"tag": gin.H{
				"id":          fmt.Sprintf("%d", time.Now().Unix()),
				"name":        tag.Name,
				"color":       tag.Color,
				"description": tag.Description,
				"created_at":  time.Now().Format("2006-01-02 15:04:05"),
			},
		})
	}
}

// UpdateTag updates an existing tag
func UpdateTag(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		tagID := c.Param("id")

		var tag struct {
			Name        string `json:"name" binding:"required"`
			Color       string `json:"color"`
			Description string `json:"description"`
		}

		if err := c.ShouldBindJSON(&tag); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		// Store tag info in a simple in-memory map for demo purposes
		// In production, you'd use a proper database table
		if tagColors == nil {
			tagColors = make(map[string]string)
		}
		if tagDescriptions == nil {
			tagDescriptions = make(map[string]string)
		}

		tagColors[tag.Name] = tag.Color
		tagDescriptions[tag.Name] = tag.Description

		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "Tag updated successfully",
			"tag": gin.H{
				"id":          tagID,
				"name":        tag.Name,
				"color":       tag.Color,
				"description": tag.Description,
				"updated_at":  time.Now().Format("2006-01-02 15:04:05"),
			},
		})
	}
}

// DeleteTag deletes a tag
func DeleteTag(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		tagID := c.Param("id")

		// For now, we'll just return success since tags are stored in targets table
		// In a real implementation, you'd delete from a separate tags table
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "Tag deleted successfully",
			"tag_id":  tagID,
		})
	}
}

// ImportTargetsCSV handles CSV import for targets
func ImportTargetsCSV(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		file, _, err := c.Request.FormFile("file")
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "No file uploaded"})
			return
		}
		defer file.Close()

		reader := csv.NewReader(file)
		reader.FieldsPerRecord = -1 // Allow variable number of fields

		// Read header row
		header, err := reader.Read()
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to read CSV header"})
			return
		}

		// Expected columns: name,address,type,port,path,interval_sec,timeout_ms,enabled,tags
		expectedColumns := []string{"name", "address", "type", "port", "path", "interval_sec", "timeout_ms", "enabled", "tags", "metrics_enabled", "snmp_community", "snmp_version"}

		// Create column mapping
		columnMap := make(map[string]int)
		for i, col := range header {
			columnMap[strings.ToLower(strings.TrimSpace(col))] = i
		}

		// Validate required columns
		missingColumns := []string{}
		for _, col := range expectedColumns[:3] { // name, address, type are required
			if _, exists := columnMap[col]; !exists {
				missingColumns = append(missingColumns, col)
			}
		}
		if len(missingColumns) > 0 {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":    "Missing required columns",
				"missing":  missingColumns,
				"expected": expectedColumns,
			})
			return
		}

		var results struct {
			Total   int      `json:"total"`
			Success int      `json:"success"`
			Failed  int      `json:"failed"`
			Errors  []string `json:"errors"`
			Created []int    `json:"created"`
		}

		// Process each row
		for {
			record, err := reader.Read()
			if err == io.EOF {
				break
			}
			if err != nil {
				results.Errors = append(results.Errors, fmt.Sprintf("Row %d: %v", results.Total+1, err))
				results.Failed++
				results.Total++
				continue
			}

			results.Total++

			// Parse row data
			name := strings.TrimSpace(record[columnMap["name"]])
			address := strings.TrimSpace(record[columnMap["address"]])
			targetType := strings.TrimSpace(record[columnMap["type"]])

			if name == "" || address == "" || targetType == "" {
				results.Errors = append(results.Errors, fmt.Sprintf("Row %d: Missing required fields (name, address, type)", results.Total))
				results.Failed++
				continue
			}

			// Validate type
			validTypes := []string{"icmp", "tcp", "http", "https"}
			if !contains(validTypes, targetType) {
				results.Errors = append(results.Errors, fmt.Sprintf("Row %d: Invalid type '%s'. Must be one of: %v", results.Total, targetType, validTypes))
				results.Failed++
				continue
			}

			// Parse optional fields with defaults
			port := (*int)(nil)
			if portStr, exists := columnMap["port"]; exists && portStr < len(record) && strings.TrimSpace(record[portStr]) != "" {
				if p, err := strconv.Atoi(strings.TrimSpace(record[portStr])); err == nil {
					port = &p
				}
			}

			path := (*string)(nil)
			if pathStr, exists := columnMap["path"]; exists && pathStr < len(record) && strings.TrimSpace(record[pathStr]) != "" {
				p := strings.TrimSpace(record[pathStr])
				path = &p
			}

			intervalSec := 30 // default
			if intervalStr, exists := columnMap["interval_sec"]; exists && intervalStr < len(record) && strings.TrimSpace(record[intervalStr]) != "" {
				if i, err := strconv.Atoi(strings.TrimSpace(record[intervalStr])); err == nil && i >= 10 && i <= 3600 {
					intervalSec = i
				}
			}

			timeoutMs := 1000 // default
			if timeoutStr, exists := columnMap["timeout_ms"]; exists && timeoutStr < len(record) && strings.TrimSpace(record[timeoutStr]) != "" {
				if t, err := strconv.Atoi(strings.TrimSpace(record[timeoutStr])); err == nil && t >= 100 && t <= 30000 {
					timeoutMs = t
				}
			}

			enabled := true // default
			if enabledStr, exists := columnMap["enabled"]; exists && enabledStr < len(record) && strings.TrimSpace(record[enabledStr]) != "" {
				enabled = strings.ToLower(strings.TrimSpace(record[enabledStr])) == "true" || strings.TrimSpace(record[enabledStr]) == "1"
			}

			tags := (*string)(nil)
			if tagsStr, exists := columnMap["tags"]; exists && tagsStr < len(record) && strings.TrimSpace(record[tagsStr]) != "" {
				t := strings.TrimSpace(record[tagsStr])
				tags = &t
			}

			metricsEnabled := false
			if idx, exists := columnMap["metrics_enabled"]; exists && idx < len(record) && strings.TrimSpace(record[idx]) != "" {
				value := strings.ToLower(strings.TrimSpace(record[idx]))
				metricsEnabled = value == "1" || value == "true" || value == "yes"
			}

			snmpCommunity := "public"
			if idx, exists := columnMap["snmp_community"]; exists && idx < len(record) && strings.TrimSpace(record[idx]) != "" {
				snmpCommunity = strings.TrimSpace(record[idx])
			}

			snmpVersion := "v2c"
			if idx, exists := columnMap["snmp_version"]; exists && idx < len(record) && strings.TrimSpace(record[idx]) != "" {
				val := strings.TrimSpace(strings.ToLower(record[idx]))
				switch val {
				case "v1", "v2c", "v3":
					snmpVersion = val
				default:
					results.Errors = append(results.Errors, fmt.Sprintf("Row %d: Invalid SNMP version '%s'", results.Total, record[idx]))
					results.Failed++
					continue
				}
			}

			// Determine monitoring type
			monitoringType := targetType
			if targetType == "icmp" {
				monitoringType = "ping"
			}

			// Insert target
			query := `INSERT INTO targets (name, address, type, monitoring_type, metrics_enabled, snmp_community, snmp_version, port, path, interval_sec, timeout_ms, enabled, tags, http_method, http_path, expected_status_code, timeout_sec) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
			result, err := db.Exec(query, name, address, targetType, monitoringType, metricsEnabled, snmpCommunity, snmpVersion, port, path, intervalSec, timeoutMs, enabled, tags, "GET", "/", 200, 10)
			if err != nil {
				results.Errors = append(results.Errors, fmt.Sprintf("Row %d: Database error - %v", results.Total, err))
				results.Failed++
				continue
			}

			id, err := result.LastInsertId()
			if err != nil {
				results.Errors = append(results.Errors, fmt.Sprintf("Row %d: Failed to get ID - %v", results.Total, err))
				results.Failed++
				continue
			}

			results.Success++
			results.Created = append(results.Created, int(id))
		}

		c.JSON(http.StatusOK, results)
	}
}

// ExportTargetsCSV handles CSV export for targets
func ExportTargetsCSV(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		query := `
			SELECT 
				t.id, t.name, t.address, t.type, t.monitoring_type,
				t.metrics_enabled, COALESCE(t.snmp_community,'public'), COALESCE(t.snmp_version,'v2c'),
				t.port, t.path,
				t.http_method, t.http_path, t.http_headers, t.expected_status_code, 
				t.expected_content, t.ssl_check, t.follow_redirects, t.timeout_sec,
				t.interval_sec, t.timeout_ms, t.enabled, t.tags, 
				t.created_at, t.updated_at
			FROM targets t
			ORDER BY t.created_at DESC
		`
		rows, err := db.Query(query)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch targets"})
			return
		}
		defer rows.Close()

		// Set CSV headers
		c.Header("Content-Type", "text/csv")
		c.Header("Content-Disposition", "attachment; filename=targets_export.csv")

		writer := csv.NewWriter(c.Writer)
		defer writer.Flush()

		// Write header
		header := []string{
			"id", "name", "address", "type", "monitoring_type", "metrics_enabled", "snmp_community", "snmp_version", "port", "path",
			"http_method", "http_path", "http_headers", "expected_status_code",
			"expected_content", "ssl_check", "follow_redirects", "timeout_sec",
			"interval_sec", "timeout_ms", "enabled", "tags", "created_at", "updated_at",
		}
		writer.Write(header)

		// Write data rows
		for rows.Next() {
			var target Target
			var httpHeaders sql.NullString
			var expectedContent sql.NullString
			var port sql.NullInt64
			var path sql.NullString
			var metricsEnabled bool
			var snmpCommunity string
			var snmpVersion string

			err := rows.Scan(
				&target.ID, &target.Name, &target.Address, &target.Type, &target.MonitoringType,
				&metricsEnabled, &snmpCommunity, &snmpVersion,
				&port, &path, &target.HTTPMethod, &target.HTTPPath, &httpHeaders,
				&target.ExpectedStatusCode, &expectedContent, &target.SSLCheck, &target.FollowRedirects,
				&target.TimeoutSec, &target.IntervalSec, &target.TimeoutMs, &target.Enabled,
				&target.Tags, &target.CreatedAt, &target.UpdatedAt,
			)
			if err != nil {
				continue
			}

			// Convert to CSV row
			row := []string{
				strconv.Itoa(target.ID),
				target.Name,
				target.Address,
				target.Type,
				target.MonitoringType,
				strconv.FormatBool(metricsEnabled),
				snmpCommunity,
				snmpVersion,
				func() string {
					if port.Valid {
						return strconv.FormatInt(port.Int64, 10)
					}
					return ""
				}(),
				func() string {
					if path.Valid {
						return path.String
					}
					return ""
				}(),
				target.HTTPMethod,
				target.HTTPPath,
				func() string {
					if httpHeaders.Valid {
						return httpHeaders.String
					}
					return ""
				}(),
				strconv.Itoa(target.ExpectedStatusCode),
				func() string {
					if expectedContent.Valid {
						return expectedContent.String
					}
					return ""
				}(),
				strconv.FormatBool(target.SSLCheck),
				strconv.FormatBool(target.FollowRedirects),
				strconv.Itoa(target.TimeoutSec),
				strconv.Itoa(target.IntervalSec),
				strconv.Itoa(target.TimeoutMs),
				strconv.FormatBool(target.Enabled),
				func() string {
					if target.Tags != nil {
						return *target.Tags
					}
					return ""
				}(),
				target.CreatedAt.Format("2006-01-02 15:04:05"),
				target.UpdatedAt.Format("2006-01-02 15:04:05"),
			}
			writer.Write(row)
		}
	}
}

// BulkDeleteTargets handles bulk deletion of targets
func BulkDeleteTargets(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var request struct {
			TargetIDs []int `json:"target_ids"`
		}

		if err := c.ShouldBindJSON(&request); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request format"})
			return
		}

		if len(request.TargetIDs) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "No targets selected"})
			return
		}

		deletedCount, err := deleteTargetsWithNotifications(db, request.TargetIDs)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete targets"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"message":       fmt.Sprintf("Successfully deleted %d targets", deletedCount),
			"deleted_count": deletedCount,
		})
	}
}

// deleteTargetsWithNotifications removes targets and cleans up related notification records.
func deleteTargetsWithNotifications(db *sql.DB, targetIDs []int) (int64, error) {
	if len(targetIDs) == 0 {
		return 0, nil
	}

	placeholders := make([]string, len(targetIDs))
	args := make([]interface{}, len(targetIDs))
	for i, id := range targetIDs {
		placeholders[i] = "?"
		args[i] = id
	}
	whereClause := strings.Join(placeholders, ",")

	tx, err := db.Begin()
	if err != nil {
		return 0, err
	}

	// Hedefe bağlı tüm alt kayıtları transaction içinde açıkça temizle.
	// Veritabanı seviyesindeki ON DELETE CASCADE kısıtlamalarına güvenmek yerine
	// (bunlar dağıtımda eksik olabilir ve silmeyi FK hatasıyla engelleyebilir)
	// child tabloları elle temizliyoruz. Böylece etiketsiz/izlenen hedefler de
	// (servis, kimlik bilgisi, metrik kaydı olanlar) sorunsuz silinir.
	cleanupQueries := []string{
		// Servis izleme kayıtları (history önce, sonra servisler)
		"DELETE FROM device_service_history WHERE target_id IN (" + whereClause + ")",
		"DELETE FROM device_services WHERE target_id IN (" + whereClause + ")",
		"DELETE FROM target_credentials WHERE target_id IN (" + whereClause + ")",
		// Ping ve metrik geçmişi
		"DELETE FROM pings_raw WHERE target_id IN (" + whereClause + ")",
		"DELETE FROM pings_raw_momentary WHERE target_id IN (" + whereClause + ")",
		"DELETE FROM server_status WHERE target_id IN (" + whereClause + ")",
		// Bildirim kayıtları
		"DELETE FROM new_notification_mails WHERE target_id IN (" + whereClause + ")",
		"DELETE FROM new_notification_rules WHERE target_id IN (" + whereClause + ")",
		"DELETE FROM user_notification_settings WHERE target_id IN (" + whereClause + ")",
		// İlişkiyi koparıp korunacak kayıtlar (silme, sadece bağı kaldır)
		"UPDATE inventory SET target_id = NULL WHERE target_id IN (" + whereClause + ")",
		"UPDATE notifications SET target_id = NULL WHERE target_id IN (" + whereClause + ")",
	}

	for _, query := range cleanupQueries {
		if err := execIgnoreMissingTable(tx, query, args...); err != nil {
			tx.Rollback()
			return 0, err
		}
	}

	result, err := tx.Exec("DELETE FROM targets WHERE id IN ("+whereClause+")", args...)
	if err != nil {
		tx.Rollback()
		return 0, err
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}

	rowsAffected, _ := result.RowsAffected()
	return rowsAffected, nil
}

func execIgnoreMissingTable(tx *sql.Tx, query string, args ...interface{}) error {
	if _, err := tx.Exec(query, args...); err != nil {
		if isMissingTableErr(err) {
			return nil
		}
		return err
	}
	return nil
}

func isMissingTableErr(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "no such table") ||
		strings.Contains(msg, "doesn't exist") ||
		strings.Contains(msg, "no such column") ||
		strings.Contains(msg, "unknown column")
}

// BulkUpdateTargets handles bulk update of targets
func BulkUpdateTargets(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var request struct {
			TargetIDs      []int   `json:"target_ids"`
			IntervalSec    *int    `json:"interval_sec,omitempty"`
			TimeoutMs      *int    `json:"timeout_ms,omitempty"`
			Enabled        *bool   `json:"enabled,omitempty"`
			MetricsEnabled *bool   `json:"metrics_enabled,omitempty"`
			SNMPCommunity  *string `json:"snmp_community,omitempty"`
			SNMPVersion    *string `json:"snmp_version,omitempty"`
		}

		if err := c.ShouldBindJSON(&request); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request format"})
			return
		}

		if len(request.TargetIDs) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "No targets selected"})
			return
		}

		// Build update query dynamically based on provided fields.
		// NOT: Proje MySQL kullanıyor -> `?` placeholder ve IN listesi de
		// parametreli olmalı (önceki $1 PostgreSQL sözdizimi bu handler'ı bozuyordu).
		var updateFields []string
		var args []interface{}

		if request.IntervalSec != nil {
			updateFields = append(updateFields, "interval_sec = ?")
			args = append(args, *request.IntervalSec)
		}

		if request.TimeoutMs != nil {
			updateFields = append(updateFields, "timeout_ms = ?")
			args = append(args, *request.TimeoutMs)
		}

		if request.Enabled != nil {
			updateFields = append(updateFields, "enabled = ?")
			args = append(args, *request.Enabled)
		}

		if request.MetricsEnabled != nil {
			updateFields = append(updateFields, "metrics_enabled = ?")
			args = append(args, *request.MetricsEnabled)
		}

		if request.SNMPCommunity != nil {
			updateFields = append(updateFields, "snmp_community = ?")
			args = append(args, *request.SNMPCommunity)
		}

		if request.SNMPVersion != nil {
			switch *request.SNMPVersion {
			case "v1", "v2c", "v3":
			default:
				c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid SNMP version"})
				return
			}
			updateFields = append(updateFields, "snmp_version = ?")
			args = append(args, *request.SNMPVersion)
		}

		if len(updateFields) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "No fields to update"})
			return
		}

		// Add updated_at field
		updateFields = append(updateFields, "updated_at = ?")
		args = append(args, time.Now())

		// Parametreli IN listesi
		placeholders := make([]string, len(request.TargetIDs))
		for i, id := range request.TargetIDs {
			placeholders[i] = "?"
			args = append(args, id)
		}

		query := fmt.Sprintf("UPDATE targets SET %s WHERE id IN (%s)",
			strings.Join(updateFields, ", "), strings.Join(placeholders, ","))

		result, err := db.Exec(query, args...)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update targets"})
			return
		}

		rowsAffected, _ := result.RowsAffected()
		c.JSON(http.StatusOK, gin.H{
			"message":       fmt.Sprintf("Successfully updated %d targets", rowsAffected),
			"updated_count": rowsAffected,
		})
	}
}

// BulkUpdateTargetTags handles bulk tag management for targets
func BulkUpdateTargetTags(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var request struct {
			TargetIDs []int  `json:"target_ids"`
			Operation string `json:"operation"` // "add", "remove", "replace"
			Tags      string `json:"tags"`
		}

		if err := c.ShouldBindJSON(&request); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request format"})
			return
		}

		if len(request.TargetIDs) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "No targets selected"})
			return
		}

		if request.Operation == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Operation is required"})
			return
		}

		// Process tags
		newTags := strings.Split(request.Tags, ",")
		for i, tag := range newTags {
			newTags[i] = strings.TrimSpace(tag)
		}

		// Remove empty tags
		var cleanTags []string
		for _, tag := range newTags {
			if tag != "" {
				cleanTags = append(cleanTags, tag)
			}
		}

		updatedCount := 0
		for _, targetID := range request.TargetIDs {
			// Get current tags
			var currentTags *string
			err := db.QueryRow("SELECT tags FROM targets WHERE id = ?", targetID).Scan(&currentTags)
			if err != nil {
				continue
			}

			var finalTags []string
			if currentTags != nil && *currentTags != "" {
				currentTagsList := strings.Split(*currentTags, ",")
				for _, tag := range currentTagsList {
					finalTags = append(finalTags, strings.TrimSpace(tag))
				}
			}

			switch request.Operation {
			case "add":
				// Add new tags (avoid duplicates)
				for _, newTag := range cleanTags {
					if !contains(finalTags, newTag) {
						finalTags = append(finalTags, newTag)
					}
				}
			case "remove":
				// Remove specified tags
				for _, tagToRemove := range cleanTags {
					for i, tag := range finalTags {
						if tag == tagToRemove {
							finalTags = append(finalTags[:i], finalTags[i+1:]...)
							break
						}
					}
				}
			case "replace":
				// Replace all tags
				finalTags = cleanTags
			}

			// Update tags in database
			tagsStr := strings.Join(finalTags, ",")
			if tagsStr == "" {
				tagsStr = ""
			}

			_, err = db.Exec("UPDATE targets SET tags = ?, updated_at = ? WHERE id = ?",
				tagsStr, time.Now(), targetID)
			if err == nil {
				updatedCount++
			}
		}

		c.JSON(http.StatusOK, gin.H{
			"message":       fmt.Sprintf("Successfully updated tags for %d targets", updatedCount),
			"updated_count": updatedCount,
		})
	}
}

// Helper function to check if slice contains string
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// EnableMetricsRequest represents the request body for enabling metrics on targets
type EnableMetricsRequest struct {
	TargetIDs []int  `json:"target_ids" binding:"required"`
	Community string `json:"community"`
	Version   string `json:"version"`
}

// EnableMetrics enables SNMP metrics collection on multiple targets
func EnableMetrics(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req EnableMetricsRequest

		// Parse request body
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
			return
		}

		// Validate target_ids
		if len(req.TargetIDs) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "target_ids cannot be empty"})
			return
		}

		// Set defaults
		if req.Community == "" {
			req.Community = "public"
		}
		if req.Version == "" {
			req.Version = "v2c"
		}

		// Validate SNMP version
		validVersions := map[string]bool{"v1": true, "v2c": true, "v3": true}
		if !validVersions[req.Version] {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid SNMP version. Must be v1, v2c, or v3"})
			return
		}

		log.Printf("[Enable Metrics API] Enabling metrics for %d targets (community: %s, version: %s)",
			len(req.TargetIDs), req.Community, req.Version)

		// Build placeholders for SQL IN clause
		placeholders := make([]string, len(req.TargetIDs))
		args := make([]interface{}, len(req.TargetIDs)+3) // +3 for community, version, and timestamp
		for i, id := range req.TargetIDs {
			placeholders[i] = "?"
			args[i] = id
		}
		args[len(req.TargetIDs)] = req.Community
		args[len(req.TargetIDs)+1] = req.Version
		args[len(req.TargetIDs)+2] = time.Now()

		// Build and execute UPDATE query
		query := `
			UPDATE targets
			SET
				metrics_enabled = 1,
				snmp_community = ?,
				snmp_version = ?,
				updated_at = ?
			WHERE id IN (` + joinPlaceholders(placeholders) + `)`

		result, err := db.Exec(query, args...)
		if err != nil {
			log.Printf("[Enable Metrics API] Error updating targets: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to enable metrics"})
			return
		}

		// Get affected rows count
		rowsAffected, err := result.RowsAffected()
		if err != nil {
			log.Printf("[Enable Metrics API] Error getting affected rows: %v", err)
			rowsAffected = 0
		}

		log.Printf("[Enable Metrics API] Successfully enabled metrics for %d targets", rowsAffected)

		c.JSON(http.StatusOK, gin.H{
			"message":       fmt.Sprintf("%d target için SNMP bilgisi toplama aktifleştirildi", rowsAffected),
			"updated_count": rowsAffected,
		})
	}
}

// joinPlaceholders joins placeholders with commas for SQL IN clause
func joinPlaceholders(placeholders []string) string {
	result := ""
	for i, p := range placeholders {
		if i > 0 {
			result += ","
		}
		result += p
	}
	return result
}
