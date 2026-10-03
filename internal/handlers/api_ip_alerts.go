package handlers

import (
	"database/sql"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

type ipAlertRow struct {
	ID             int64      `json:"id"`
	Subnet         string     `json:"subnet"`
	IP             string     `json:"ip"`
	DetectedAt     time.Time  `json:"detected_at"`
	AcknowledgedAt *time.Time `json:"acknowledged_at,omitempty"`
}

func GetIPAlertsUnreadCount(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var count int64
		if err := db.QueryRowContext(c.Request.Context(), "SELECT COUNT(*) FROM ip_alerts WHERE acknowledged_at IS NULL").Scan(&count); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "count_failed"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"count": count})
	}
}

func ListIPAlerts(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		limit := 20
		if v := c.Query("limit"); v != "" {
			if n, err := strconv.Atoi(v); err == nil {
				if n < 1 {
					n = 1
				}
				if n > 200 {
					n = 200
				}
				limit = n
			}
		}

		unackedOnly := c.Query("unacked") == "1"
		query := "SELECT id, subnet, ip, detected_at, acknowledged_at FROM ip_alerts"
		if unackedOnly {
			query += " WHERE acknowledged_at IS NULL"
		}
		query += " ORDER BY detected_at DESC LIMIT ?"

		rows, err := db.QueryContext(c.Request.Context(), query, limit)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "list_failed"})
			return
		}
		defer rows.Close()

		var out []ipAlertRow
		for rows.Next() {
			var r ipAlertRow
			if err := rows.Scan(&r.ID, &r.Subnet, &r.IP, &r.DetectedAt, &r.AcknowledgedAt); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "scan_failed"})
				return
			}
			out = append(out, r)
		}
		if err := rows.Err(); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "rows_failed"})
			return
		}

		c.JSON(http.StatusOK, out)
	}
}

func AcknowledgeAllIPAlerts(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		if _, err := db.ExecContext(c.Request.Context(), "UPDATE ip_alerts SET acknowledged_at = NOW() WHERE acknowledged_at IS NULL"); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "ack_failed"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true})
	}
}

func AcknowledgeIPAlert(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil || id <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_id"})
			return
		}

		if _, err := db.ExecContext(c.Request.Context(), "UPDATE ip_alerts SET acknowledged_at = NOW() WHERE id = ? AND acknowledged_at IS NULL", id); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "ack_failed"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true})
	}
}

