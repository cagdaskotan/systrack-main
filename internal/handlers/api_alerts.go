package handlers

import (
	"database/sql"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

type Alert struct {
	ID         int64      `json:"id"`
	TargetID   int        `json:"target_id"`
	Level      string     `json:"level"`
	OpenedAt   time.Time  `json:"opened_at"`
	ClosedAt   *time.Time `json:"closed_at,omitempty"`
	Message    *string    `json:"message,omitempty"`
	Status     string     `json:"status"`
	TargetName string     `json:"target_name,omitempty"`
}

func GetAlerts(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		status := c.Query("status")
		level := c.Query("level")
		q := c.Query("q")
		page := c.DefaultQuery("page", "1")
		limit := c.DefaultQuery("limit", "50")

		query := `SELECT a.id, a.target_id, a.level, a.opened_at, a.closed_at, a.message, a.status, t.name as target_name 
				  FROM alerts a 
				  LEFT JOIN targets t ON a.target_id = t.id 
				  WHERE 1=1`
		args := []interface{}{}

		if status != "" {
			query += ` AND a.status = ?`
			args = append(args, status)
		}
		if level != "" {
			query += ` AND a.level = ?`
			args = append(args, level)
		}
		if q != "" {
			query += ` AND (t.name LIKE ? OR a.message LIKE ?)`
			searchTerm := "%" + q + "%"
			args = append(args, searchTerm, searchTerm)
		}

		query += ` ORDER BY a.opened_at DESC`

		// Pagination
		pageInt, err := strconv.Atoi(page)
		if err != nil {
			pageInt = 1
		}
		limitInt, err := strconv.Atoi(limit)
		if err != nil {
			limitInt = 50
		}
		offset := (pageInt - 1) * limitInt
		query += ` LIMIT ? OFFSET ?`
		args = append(args, limitInt, offset)

		rows, err := db.Query(query, args...)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch alerts"})
			return
		}
		defer rows.Close()

		var alerts []Alert
		for rows.Next() {
			var alert Alert
			err := rows.Scan(&alert.ID, &alert.TargetID, &alert.Level, &alert.OpenedAt, &alert.ClosedAt, &alert.Message, &alert.Status, &alert.TargetName)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to scan alert"})
				return
			}
			alerts = append(alerts, alert)
		}

		c.JSON(http.StatusOK, gin.H{"alerts": alerts})
	}
}

func CloseAlert(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		idStr := c.Param("id")
		id, err := strconv.Atoi(idStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid alert ID"})
			return
		}

		_, err = db.Exec("UPDATE alerts SET status = 'closed', closed_at = NOW() WHERE id = ? AND status = 'open'", id)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to close alert"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"message": "Alert closed successfully"})
	}
}
