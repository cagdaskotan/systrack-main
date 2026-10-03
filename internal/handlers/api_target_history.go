package handlers

import (
	"database/sql"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

// TargetHistoryData represents ping/HTTP data for a specific target
type TargetHistoryData struct {
	Timestamp       time.Time `json:"timestamp"`
	DurationMs      float64   `json:"duration_ms"`
	Success         bool      `json:"success"`
	ErrorMsg        *string   `json:"error_msg,omitempty"`
	StatusCode      *int      `json:"status_code,omitempty"`
	ResponseSize    *int      `json:"response_size,omitempty"`
	SSLExpiryDate   *string   `json:"ssl_expiry_date,omitempty"`
	ResponseHeaders *string   `json:"response_headers,omitempty"`
}

// TargetHistoryResponse represents the response for target history
type TargetHistoryResponse struct {
	TargetID   int                 `json:"target_id"`
	TargetName string              `json:"target_name"`
	TargetAddr string              `json:"target_addr"`
	Period     string              `json:"period"`
	Data       []TargetHistoryData `json:"data"`
	Stats      struct {
		TotalPings    int     `json:"total_pings"`
		SuccessPings  int     `json:"success_pings"`
		FailedPings   int     `json:"failed_pings"`
		AvgDuration   float64 `json:"avg_duration"`
		MinDuration   float64 `json:"min_duration"`
		MaxDuration   float64 `json:"max_duration"`
		UptimePercent float64 `json:"uptime_percent"`
	} `json:"stats"`
}

// GetTargetHistory returns ping history for a specific target with time filtering
func GetTargetHistory(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		targetIDStr := c.Param("target_id")
		targetID, err := strconv.Atoi(targetIDStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid target ID"})
			return
		}

		// Period parameter (1h, 1d, 7d, 30d)
		period := c.DefaultQuery("period", "1d")

		// Calculate time range
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
			timeRange = "INTERVAL 1 DAY"
		}

		// Get target info
		var targetName, targetAddr string
		err = db.QueryRow("SELECT name, address FROM targets WHERE id = ?", targetID).Scan(&targetName, &targetAddr)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "Target not found"})
			return
		}

		// Get ping/HTTP data for the specified period
		query := `
			SELECT 
				p.created_at,
				p.ts_ms,
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
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch ping data"})
			return
		}
		defer rows.Close()

		var historyData []TargetHistoryData
		var totalPings, successPings, failedPings int
		var totalDuration, minDuration, maxDuration float64
		minDuration = 999999 // Initialize with high value

		for rows.Next() {
			var data TargetHistoryData
			var tsMs sql.NullInt64
			var rttMs sql.NullInt32
			var errorMsg sql.NullString
			var statusCode sql.NullInt32
			var responseSize sql.NullInt32
			var sslExpiry sql.NullTime
			var responseHeaders sql.NullString

			err := rows.Scan(&data.Timestamp, &tsMs, &rttMs, &data.Success, &errorMsg,
				&statusCode, &responseSize, &sslExpiry, &responseHeaders)
			if err != nil {
				continue
			}

			// Normalize timestamp using ts_ms (epoch milliseconds) to avoid timezone drift
			if tsMs.Valid {
				data.Timestamp = time.Unix(0, tsMs.Int64*int64(time.Millisecond)).In(time.Local)
			}

			if rttMs.Valid {
				data.DurationMs = float64(rttMs.Int32)
			} else {
				data.DurationMs = 0
			}

			if errorMsg.Valid {
				data.ErrorMsg = &errorMsg.String
			}

			if statusCode.Valid {
				statusCodeInt := int(statusCode.Int32)
				data.StatusCode = &statusCodeInt
			}

			if responseSize.Valid {
				responseSizeInt := int(responseSize.Int32)
				data.ResponseSize = &responseSizeInt
			}

			if sslExpiry.Valid {
				sslExpiryStr := sslExpiry.Time.Format("2006-01-02 15:04:05")
				data.SSLExpiryDate = &sslExpiryStr
			}

			if responseHeaders.Valid {
				data.ResponseHeaders = &responseHeaders.String
			}

			historyData = append(historyData, data)
			totalPings++

			if data.Success {
				successPings++
				totalDuration += data.DurationMs

				if data.DurationMs < minDuration {
					minDuration = data.DurationMs
				}
				if data.DurationMs > maxDuration {
					maxDuration = data.DurationMs
				}
			} else {
				failedPings++
			}
		}

		// Calculate statistics
		var avgDuration float64
		var uptimePercent float64

		if successPings > 0 {
			avgDuration = totalDuration / float64(successPings)
		}

		if totalPings > 0 {
			uptimePercent = (float64(successPings) / float64(totalPings)) * 100
		}

		if minDuration == 999999 {
			minDuration = 0
		}

		response := TargetHistoryResponse{
			TargetID:   targetID,
			TargetName: targetName,
			TargetAddr: targetAddr,
			Period:     period,
			Data:       historyData,
		}

		response.Stats.TotalPings = totalPings
		response.Stats.SuccessPings = successPings
		response.Stats.FailedPings = failedPings
		response.Stats.AvgDuration = avgDuration
		response.Stats.MinDuration = minDuration
		response.Stats.MaxDuration = maxDuration
		response.Stats.UptimePercent = uptimePercent

		c.JSON(http.StatusOK, response)
	}
}
