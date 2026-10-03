package alerts

import (
	"database/sql"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// AlertItem represents an alert payload tailored for mobile clients.
type AlertItem struct {
	ID         int64      `json:"id"`
	TargetID   int        `json:"target_id"`
	Level      string     `json:"level"`
	Status     string     `json:"status"`
	OpenedAt   time.Time  `json:"opened_at"`
	ClosedAt   *time.Time `json:"closed_at,omitempty"`
	Message    *string    `json:"message,omitempty"`
	TargetName *string    `json:"target_name,omitempty"`
}

// List returns alerts with optional filtering by status, level, and search query.
func List(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		status := strings.TrimSpace(c.Query("status"))
		level := strings.TrimSpace(c.Query("level"))
		q := strings.TrimSpace(c.Query("q"))

		page, err := strconv.Atoi(c.DefaultQuery("page", "1"))
		if err != nil || page < 1 {
			page = 1
		}

		limit, err := strconv.Atoi(c.DefaultQuery("limit", "50"))
		if err != nil || limit <= 0 || limit > 200 {
			limit = 50
		}

		offset := (page - 1) * limit

		query := `
			SELECT
				a.id,
				a.target_id,
				a.level,
				a.status,
				a.opened_at,
				a.closed_at,
				a.message,
				t.name AS target_name
			FROM alerts a
			LEFT JOIN targets t ON a.target_id = t.id
			WHERE 1=1
		`
		args := make([]interface{}, 0, 5)

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
			search := "%" + q + "%"
			args = append(args, search, search)
		}

		query += ` ORDER BY a.opened_at DESC LIMIT ? OFFSET ?`
		args = append(args, limit, offset)

		rows, err := db.Query(query, args...)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "database_error",
				"message": "Uyarı listesi alınamadı.",
			})
			return
		}
		defer rows.Close()

		alerts := make([]AlertItem, 0, limit)
		for rows.Next() {
			var (
				item       AlertItem
				openedAt   time.Time
				closedAt   sql.NullTime
				message    sql.NullString
				targetName sql.NullString
			)

			if err := rows.Scan(
				&item.ID,
				&item.TargetID,
				&item.Level,
				&item.Status,
				&openedAt,
				&closedAt,
				&message,
				&targetName,
			); err != nil {
				continue
			}

			item.OpenedAt = openedAt
			if closedAt.Valid {
				t := closedAt.Time
				item.ClosedAt = &t
			}
			if message.Valid {
				value := message.String
				item.Message = &value
			}
			if targetName.Valid {
				value := targetName.String
				item.TargetName = &value
			}

			alerts = append(alerts, item)
		}

		c.JSON(http.StatusOK, gin.H{
			"alerts": alerts,
			"meta": gin.H{
				"page":     page,
				"limit":    limit,
				"returned": len(alerts),
				"has_more": len(alerts) == limit,
			},
		})
	}
}

// Close marks an alert as closed if it is currently open.
func Close(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := strconv.Atoi(c.Param("id"))
		if err != nil || id <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "invalid_request",
				"message": "Geçersiz uyarı kimliği.",
			})
			return
		}

		result, err := db.Exec(
			"UPDATE alerts SET status = 'closed', closed_at = NOW() WHERE id = ? AND status = 'open'",
			id,
		)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "database_error",
				"message": "Uyarı kapatılamadı.",
			})
			return
		}

		affected, err := result.RowsAffected()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "database_error",
				"message": "Uyarı işlemi tamamlanamadı.",
			})
			return
		}

		if affected == 0 {
			c.JSON(http.StatusNotFound, gin.H{
				"error":   "not_found",
				"message": "Uyarı bulunamadı veya zaten kapalı.",
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"message": "Uyarı başarıyla kapatıldı.",
		})
	}
}
