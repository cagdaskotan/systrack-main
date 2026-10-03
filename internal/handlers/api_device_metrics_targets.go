package handlers

import (
	"database/sql"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
)

// DeviceMetricsTarget represents a target with basic info (for fast initial load)
type DeviceMetricsTarget struct {
	TargetID      int    `json:"target_id"`
	TargetName    string `json:"target_name"`
	TargetAddress string `json:"target_address"`
	Status        string `json:"status"` // "pending" initially
}

// GetDeviceMetricsTargets returns list of targets with metrics_enabled (fast query)
func GetDeviceMetricsTargets(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		query := `
			SELECT t.id, t.name, t.address
			FROM targets t
			WHERE t.enabled = 1
				AND (
					t.metrics_enabled = 1
					OR EXISTS (
						SELECT 1
						FROM target_credentials tc
						WHERE tc.target_id = t.id
					)
				)
			ORDER BY t.name
		`

		rows, err := db.Query(query)
		if err != nil {
			log.Printf("[Metrics Targets] Database query error: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch targets"})
			return
		}
		defer rows.Close()

		var targets []DeviceMetricsTarget
		for rows.Next() {
			var t DeviceMetricsTarget
			if err := rows.Scan(&t.TargetID, &t.TargetName, &t.TargetAddress); err != nil {
				log.Printf("[Metrics Targets] Row scan error: %v", err)
				continue
			}
			t.Status = "pending" // Will be updated via WebSocket
			targets = append(targets, t)
		}

		log.Printf("[Metrics Targets] Returning %d targets", len(targets))
		c.JSON(http.StatusOK, targets)
	}
}
