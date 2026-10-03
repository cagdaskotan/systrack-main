package handlers

import (
	"database/sql"
	"log"
	"net/http"
	"sync"

	"systrack/internal/services"

	"github.com/gin-gonic/gin"
)

// GetLiveDeviceMetrics fetches real-time SNMP metrics without database
func GetLiveDeviceMetrics(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Fetch all targets with metrics_enabled = 1
		query := `
			SELECT t.id, t.name, t.address,
			       COALESCE(t.snmp_community, 'public') as snmp_community,
			       COALESCE(t.snmp_version, 'v2c') as snmp_version
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
			log.Printf("[Live Metrics] Database query error: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch targets"})
			return
		}
		defer rows.Close()

		var targets []services.Target
		for rows.Next() {
			var t services.Target
			if err := rows.Scan(&t.ID, &t.Name, &t.Address, &t.SNMPCommunity, &t.SNMPVersion); err != nil {
				log.Printf("[Live Metrics] Row scan error: %v", err)
				continue
			}
			t.MetricsEnabled = true
			targets = append(targets, t)
		}

		if err := rows.Err(); err != nil {
			log.Printf("[Live Metrics] Rows iteration error: %v", err)
		}

		log.Printf("[Live Metrics] Found %d enabled targets", len(targets))

		// If no targets, return empty array
		if len(targets) == 0 {
			c.JSON(http.StatusOK, []services.DeviceMetrics{})
			return
		}

		// Create a single collector instance to reuse SNMP collection logic
		collector := services.NewDeviceMetricsCollector(db, nil)

		// Collect metrics from all targets in parallel
		var wg sync.WaitGroup
		results := make([]services.DeviceMetrics, len(targets))

		for i, target := range targets {
			wg.Add(1)
			go func(idx int, tgt services.Target) {
				defer wg.Done()
				metrics := collector.CollectMetrics(tgt)
				results[idx] = metrics
			}(i, target)
		}

		// Wait for all collections to complete (each has 30s timeout internally)
		wg.Wait()

		log.Printf("[Live Metrics] Completed collection for %d targets", len(results))

		// Return results
		c.JSON(http.StatusOK, results)
	}
}
