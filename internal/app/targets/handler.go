package targets

import (
	"database/sql"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

// TargetItem represents a simplified target payload for mobile clients.
type TargetItem struct {
	ID             int        `json:"id"`
	Name           string     `json:"name"`
	Address        string     `json:"address"`
	MonitoringType string     `json:"monitoring_type"`
	Enabled        bool       `json:"enabled"`
	Tags           *string    `json:"tags,omitempty"`
	IsOnline       bool       `json:"is_online"`
	LastResponseMs *int       `json:"last_response_ms,omitempty"`
	LastCheckedAt  *time.Time `json:"last_checked_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

// List returns monitoring targets with their latest status information.
func List(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		query := `
			WITH latest_ping AS (
				SELECT
					p.target_id,
					p.ok,
					p.rtt_ms,
					p.created_at,
					ROW_NUMBER() OVER (PARTITION BY p.target_id ORDER BY p.created_at DESC) AS rn
				FROM pings_raw p
			)
			SELECT
				t.id,
				t.name,
				t.address,
				t.monitoring_type,
				t.enabled,
				t.tags,
				COALESCE(lp.ok, 0) AS is_online,
				lp.rtt_ms,
				lp.created_at AS last_checked_at,
				t.created_at,
				t.updated_at
			FROM targets t
			LEFT JOIN latest_ping lp ON lp.target_id = t.id AND lp.rn = 1
			ORDER BY t.created_at DESC
		`

		rows, err := db.Query(query)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "database_error",
				"message": "Hedef listesi alınamadı.",
			})
			return
		}
		defer rows.Close()

		items := make([]TargetItem, 0)

		for rows.Next() {
			var item TargetItem
			var tags sql.NullString
			var lastResponseMs sql.NullInt32
			var lastCheckedAt sql.NullTime
			var isOnlineInt int

			if err := rows.Scan(
				&item.ID,
				&item.Name,
				&item.Address,
				&item.MonitoringType,
				&item.Enabled,
				&tags,
				&isOnlineInt,
				&lastResponseMs,
				&lastCheckedAt,
				&item.CreatedAt,
				&item.UpdatedAt,
			); err != nil {
				continue
			}

			if tags.Valid {
				item.Tags = &tags.String
			}
			item.IsOnline = isOnlineInt == 1

			if lastResponseMs.Valid {
				value := int(lastResponseMs.Int32)
				item.LastResponseMs = &value
			}
			if lastCheckedAt.Valid {
				item.LastCheckedAt = &lastCheckedAt.Time
			}

			items = append(items, item)
		}

		c.JSON(http.StatusOK, gin.H{
			"targets": items,
			"count":   len(items),
		})
	}
}

// TargetDetail represents a detailed target payload for mobile clients.
type TargetDetail struct {
	ID                 int        `json:"id"`
	Name               string     `json:"name"`
	Address            string     `json:"address"`
	Type               string     `json:"type"`
	MonitoringType     string     `json:"monitoring_type"`
	MetricsEnabled     bool       `json:"metrics_enabled"`
	SNMPCommunity      string     `json:"snmp_community"`
	SNMPVersion        string     `json:"snmp_version"`
	Port               *int       `json:"port,omitempty"`
	Path               *string    `json:"path,omitempty"`
	HTTPMethod         string     `json:"http_method"`
	HTTPPath           string     `json:"http_path"`
	HTTPHeaders        *string    `json:"http_headers,omitempty"`
	ExpectedStatusCode int        `json:"expected_status_code"`
	ExpectedContent    *string    `json:"expected_content,omitempty"`
	SSLCheck           bool       `json:"ssl_check"`
	FollowRedirects    bool       `json:"follow_redirects"`
	TimeoutSec         int        `json:"timeout_sec"`
	IntervalSec        int        `json:"interval_sec"`
	TimeoutMs          int        `json:"timeout_ms"`
	Enabled            bool       `json:"enabled"`
	Tags               *string    `json:"tags,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
	IsOnline           bool       `json:"is_online"`
	LastResponseMs     *int       `json:"last_response_ms,omitempty"`
	LastCheckedAt      *time.Time `json:"last_checked_at,omitempty"`
}

// TargetCreateRequest mirrors the parameters required to create a target.
type TargetCreateRequest struct {
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

// Get returns detailed information for a specific target.
func Get(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		if id == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "invalid_request",
				"message": "Target ID is required.",
			})
			return
		}

		query := `
			SELECT
				t.id,
				t.name,
				t.address,
				t.type,
				t.monitoring_type,
				t.metrics_enabled,
				COALESCE(t.snmp_community,'public') as snmp_community,
				COALESCE(t.snmp_version,'v2c') as snmp_version,
				t.port,
				t.path,
				t.http_method,
				t.http_path,
				t.http_headers,
				t.expected_status_code,
				t.expected_content,
				t.ssl_check,
				t.follow_redirects,
				t.timeout_sec,
				t.interval_sec,
				t.timeout_ms,
				t.enabled,
				t.tags,
				t.created_at,
				t.updated_at,
				COALESCE(p.ok, 0) AS is_online,
				p.rtt_ms,
				p.created_at AS last_checked_at
			FROM targets t
			LEFT JOIN (
				SELECT target_id, ok, rtt_ms, created_at
				FROM pings_raw
				WHERE target_id = ?
				ORDER BY created_at DESC
				LIMIT 1
			) p ON p.target_id = t.id
			WHERE t.id = ?
		`

		row := db.QueryRow(query, id, id)

		var detail TargetDetail
		var httpHeaders sql.NullString
		var expectedContent sql.NullString
		var tags sql.NullString
		var port sql.NullInt32
		var path sql.NullString
		var lastResponse sql.NullInt32
		var lastChecked sql.NullTime
		var isOnlineInt int

		if err := row.Scan(
			&detail.ID,
			&detail.Name,
			&detail.Address,
			&detail.Type,
			&detail.MonitoringType,
			&detail.MetricsEnabled,
			&detail.SNMPCommunity,
			&detail.SNMPVersion,
			&port,
			&path,
			&detail.HTTPMethod,
			&detail.HTTPPath,
			&httpHeaders,
			&detail.ExpectedStatusCode,
			&expectedContent,
			&detail.SSLCheck,
			&detail.FollowRedirects,
			&detail.TimeoutSec,
			&detail.IntervalSec,
			&detail.TimeoutMs,
			&detail.Enabled,
			&tags,
			&detail.CreatedAt,
			&detail.UpdatedAt,
			&isOnlineInt,
			&lastResponse,
			&lastChecked,
		); err != nil {
			if err == sql.ErrNoRows {
				c.JSON(http.StatusNotFound, gin.H{
					"error":   "not_found",
					"message": "Target not found.",
				})
				return
			}

			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "database_error",
				"message": "Target information could not be loaded.",
			})
			return
		}

		if httpHeaders.Valid {
			detail.HTTPHeaders = &httpHeaders.String
		}
		if expectedContent.Valid {
			detail.ExpectedContent = &expectedContent.String
		}
		if tags.Valid {
			detail.Tags = &tags.String
		}
		if port.Valid {
			portVal := int(port.Int32)
			detail.Port = &portVal
		}
		if path.Valid {
			detail.Path = &path.String
		}
		if lastResponse.Valid {
			value := int(lastResponse.Int32)
			detail.LastResponseMs = &value
		}
		if lastChecked.Valid {
			detail.LastCheckedAt = &lastChecked.Time
		}
		detail.IsOnline = isOnlineInt == 1

		c.JSON(http.StatusOK, detail)
	}
}

// History returns monitoring history for a target with statistics.
func History(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		idStr := c.Param("id")
		targetID, err := strconv.Atoi(idStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "invalid_request",
				"message": "Target ID must be numeric.",
			})
			return
		}

		period := c.DefaultQuery("period", "1d")
		var timeRange string
		switch period {
		case "1h":
			timeRange = "INTERVAL 1 HOUR"
		case "1d":
			timeRange = "INTERVAL 1 DAY"
		case "7d":
			timeRange = "INTERVAL 7 DAY"
		case "30d":
			timeRange = "INTERVAL 30 DAY"
		default:
			period = "1d"
			timeRange = "INTERVAL 1 DAY"
		}

		var targetName, targetAddr string
		if err := db.QueryRow("SELECT name, address FROM targets WHERE id = ?", targetID).Scan(&targetName, &targetAddr); err != nil {
			if err == sql.ErrNoRows {
				c.JSON(http.StatusNotFound, gin.H{
					"error":   "not_found",
					"message": "Target not found.",
				})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "database_error",
				"message": "Target information could not be loaded.",
			})
			return
		}

		query := `
			SELECT 
				p.created_at,
				p.rtt_ms,
				p.ok,
				p.error_msg,
				p.response_status_code,
				p.response_size_bytes,
				p.ssl_expiry_date,
				p.response_headers
			FROM pings_raw p
			WHERE p.target_id = ? 
			AND p.created_at >= DATE_SUB(NOW(), ` + timeRange + `)
			ORDER BY p.created_at ASC
		`

		rows, err := db.Query(query, targetID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "database_error",
				"message": "History data could not be loaded.",
			})
			return
		}
		defer rows.Close()

		type historyPoint struct {
			Timestamp       time.Time `json:"timestamp"`
			DurationMs      float64   `json:"duration_ms"`
			Success         bool      `json:"success"`
			ErrorMsg        *string   `json:"error_msg,omitempty"`
			StatusCode      *int      `json:"status_code,omitempty"`
			ResponseSize    *int      `json:"response_size,omitempty"`
			SSLExpiryDate   *string   `json:"ssl_expiry_date,omitempty"`
			ResponseHeaders *string   `json:"response_headers,omitempty"`
		}

		history := make([]historyPoint, 0)
		var total, success, fail int
		var totalDuration, minDuration, maxDuration float64
		minDuration = 999999

		for rows.Next() {
			var point historyPoint
			var rttMs sql.NullInt32
			var errorMsg sql.NullString
			var statusCode sql.NullInt32
			var responseSize sql.NullInt32
			var sslExpiry sql.NullTime
			var responseHeaders sql.NullString

			if err := rows.Scan(
				&point.Timestamp,
				&rttMs,
				&point.Success,
				&errorMsg,
				&statusCode,
				&responseSize,
				&sslExpiry,
				&responseHeaders,
			); err != nil {
				continue
			}

			if rttMs.Valid {
				point.DurationMs = float64(rttMs.Int32)
			}
			if errorMsg.Valid {
				point.ErrorMsg = &errorMsg.String
			}
			if statusCode.Valid {
				val := int(statusCode.Int32)
				point.StatusCode = &val
			}
			if responseSize.Valid {
				val := int(responseSize.Int32)
				point.ResponseSize = &val
			}
			if sslExpiry.Valid {
				str := sslExpiry.Time.Format("2006-01-02 15:04:05")
				point.SSLExpiryDate = &str
			}
			if responseHeaders.Valid {
				point.ResponseHeaders = &responseHeaders.String
			}

			if point.Success {
				success++
				totalDuration += point.DurationMs
				if point.DurationMs < minDuration {
					minDuration = point.DurationMs
				}
				if point.DurationMs > maxDuration {
					maxDuration = point.DurationMs
				}
			} else {
				fail++
			}
			total++
			history = append(history, point)
		}

		var avgDuration float64
		if success > 0 {
			avgDuration = totalDuration / float64(success)
		}
		if minDuration == 999999 {
			minDuration = 0
		}
		var uptime float64
		if total > 0 {
			uptime = float64(success) / float64(total) * 100
		}

		c.JSON(http.StatusOK, gin.H{
			"target_id":   targetID,
			"target_name": targetName,
			"target_addr": targetAddr,
			"period":      period,
			"data":        history,
			"stats": gin.H{
				"total_pings":    total,
				"success_pings":  success,
				"failed_pings":   fail,
				"avg_duration":   avgDuration,
				"min_duration":   minDuration,
				"max_duration":   maxDuration,
				"uptime_percent": uptime,
			},
		})
	}
}

// Create adds a new target (admin use from mobile).
func Create(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req TargetCreateRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "invalid_request",
				"message": err.Error(),
			})
			return
		}

		if req.MonitoringType == "" {
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
				c.JSON(http.StatusBadRequest, gin.H{
					"error":   "invalid_request",
					"message": "Invalid SNMP version.",
				})
				return
			}
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
				c.JSON(http.StatusBadRequest, gin.H{
					"error":   "invalid_request",
					"message": "Invalid SNMP version.",
				})
				return
			}
		}

		query := `INSERT INTO targets (
				name, address, type, monitoring_type, metrics_enabled, snmp_community, snmp_version, port, path,
				http_method, http_path, http_headers, expected_status_code,
				expected_content, ssl_check, follow_redirects,
				timeout_sec, interval_sec, timeout_ms, enabled, tags
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

		result, err := db.Exec(
			query,
			req.Name,
			req.Address,
			req.Type,
			req.MonitoringType,
			req.MetricsEnabled,
			req.SNMPCommunity,
			req.SNMPVersion,
			req.Port,
			req.Path,
			req.HTTPMethod,
			req.HTTPPath,
			req.HTTPHeaders,
			req.ExpectedStatusCode,
			req.ExpectedContent,
			req.SSLCheck,
			req.FollowRedirects,
			req.TimeoutSec,
			req.IntervalSec,
			req.TimeoutMs,
			req.Enabled,
			req.Tags,
		)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "database_error",
				"message": "Target could not be created.",
			})
			return
		}

		id, err := result.LastInsertId()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "database_error",
				"message": "Target created but ID could not be retrieved.",
			})
			return
		}

		c.JSON(http.StatusCreated, gin.H{
			"id":      id,
			"message": "Target created successfully.",
		})
	}
}

// Update modifies an existing target.
func Update(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		idStr := c.Param("id")
		id, err := strconv.Atoi(idStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "invalid_request",
				"message": "Target ID must be numeric.",
			})
			return
		}

		var req TargetCreateRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "invalid_request",
				"message": err.Error(),
			})
			return
		}

		if req.MonitoringType == "" {
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

		query := `UPDATE targets SET
				name = ?,
				address = ?,
				type = ?,
				monitoring_type = ?,
				metrics_enabled = ?,
				snmp_community = ?,
				snmp_version = ?,
				port = ?,
				path = ?,
				http_method = ?,
				http_path = ?,
				http_headers = ?,
				expected_status_code = ?,
				expected_content = ?,
				ssl_check = ?,
				follow_redirects = ?,
				timeout_sec = ?,
				interval_sec = ?,
				timeout_ms = ?,
				enabled = ?,
				tags = ?,
				updated_at = NOW()
			WHERE id = ?`

		result, err := db.Exec(
			query,
			req.Name,
			req.Address,
			req.Type,
			req.MonitoringType,
			req.MetricsEnabled,
			req.SNMPCommunity,
			req.SNMPVersion,
			req.Port,
			req.Path,
			req.HTTPMethod,
			req.HTTPPath,
			req.HTTPHeaders,
			req.ExpectedStatusCode,
			req.ExpectedContent,
			req.SSLCheck,
			req.FollowRedirects,
			req.TimeoutSec,
			req.IntervalSec,
			req.TimeoutMs,
			req.Enabled,
			req.Tags,
			id,
		)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "database_error",
				"message": "Target could not be updated.",
			})
			return
		}

		rows, _ := result.RowsAffected()
		if rows == 0 {
			c.JSON(http.StatusNotFound, gin.H{
				"error":   "not_found",
				"message": "Target not found.",
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"message": "Target updated successfully.",
		})
	}
}

// Delete removes a target.
func Delete(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		idStr := c.Param("id")
		id, err := strconv.Atoi(idStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "invalid_request",
				"message": "Target ID must be numeric.",
			})
			return
		}

		result, err := db.Exec("DELETE FROM targets WHERE id = ?", id)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "database_error",
				"message": "Target could not be deleted.",
			})
			return
		}

		rows, _ := result.RowsAffected()
		if rows == 0 {
			c.JSON(http.StatusNotFound, gin.H{
				"error":   "not_found",
				"message": "Target not found.",
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"message": "Target deleted successfully.",
		})
	}
}
