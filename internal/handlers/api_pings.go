package handlers

import (
	"database/sql"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

// PingData represents ping data for charts
type PingData struct {
	TargetID   int       `json:"target_id"`
	TargetName string    `json:"target_name"`
	DurationMs float64   `json:"duration_ms"`
	Success    bool      `json:"success"`
	Timestamp  time.Time `json:"timestamp"`
}

// RecentPingsResponse represents the response for recent pings
type RecentPingsResponse struct {
	Targets []TargetPingData `json:"targets"`
}

// TargetPingData represents ping data for a specific target
type TargetPingData struct {
	TargetID int        `json:"target_id"`
	Name     string     `json:"name"`
	Pings    []PingData `json:"pings"`
}

// GetRecentPings returns recent ping data for charts
func GetRecentPings(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Son 24 saatlik ping verilerini getir
		query := `
			SELECT 
				p.target_id,
				t.name as target_name,
				p.rtt_ms,
				p.ok,
				p.created_at
			FROM pings_raw p
			JOIN targets t ON p.target_id = t.id
			WHERE p.created_at >= DATE_SUB(NOW(), INTERVAL 24 HOUR)
			ORDER BY p.created_at ASC
		`

		rows, err := db.Query(query)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to query ping data"})
			return
		}
		defer rows.Close()

		// Verileri target'lara göre grupla
		targetPings := make(map[int][]PingData)
		targetNames := make(map[int]string)

		for rows.Next() {
			var ping PingData
			var targetID int
			var targetName string
			var rttMs sql.NullInt32
			var ok bool
			var timestamp time.Time

			err := rows.Scan(&targetID, &targetName, &rttMs, &ok, &timestamp)
			if err != nil {
				continue
			}

			ping.TargetID = targetID
			ping.TargetName = targetName
			ping.Success = ok
			ping.Timestamp = timestamp

			if rttMs.Valid {
				ping.DurationMs = float64(rttMs.Int32)
			} else {
				ping.DurationMs = 0
			}

			targetPings[targetID] = append(targetPings[targetID], ping)
			targetNames[targetID] = targetName
		}

		// Response formatına dönüştür
		var response RecentPingsResponse
		for targetID, pings := range targetPings {
			response.Targets = append(response.Targets, TargetPingData{
				TargetID: targetID,
				Name:     targetNames[targetID],
				Pings:    pings,
			})
		}

		c.JSON(http.StatusOK, response)
	}
}

// GetPings returns ping history for a specific target
func GetPings(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		targetIDStr := c.Param("target_id")
		targetID, err := strconv.Atoi(targetIDStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid target ID"})
			return
		}

		// Limit parametresi (varsayılan 100)
		limitStr := c.DefaultQuery("limit", "100")
		limit, err := strconv.Atoi(limitStr)
		if err != nil {
			limit = 100
		}

		query := `
			SELECT 
				p.rtt_ms,
				p.ok,
				p.created_at
			FROM pings_raw p
			WHERE p.target_id = ?
			ORDER BY p.created_at DESC
			LIMIT ?
		`

		rows, err := db.Query(query, targetID, limit)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to query ping data"})
			return
		}
		defer rows.Close()

		var pings []PingData
		for rows.Next() {
			var ping PingData
			var rttMs sql.NullInt32
			var ok bool
			var timestamp time.Time

			err := rows.Scan(&rttMs, &ok, &timestamp)
			if err != nil {
				continue
			}

			ping.TargetID = targetID
			ping.Success = ok
			ping.Timestamp = timestamp

			if rttMs.Valid {
				ping.DurationMs = float64(rttMs.Int32)
			} else {
				ping.DurationMs = 0
			}

			pings = append(pings, ping)
		}

		c.JSON(http.StatusOK, gin.H{"pings": pings})
	}
}
