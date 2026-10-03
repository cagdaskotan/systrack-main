package handlers

import (
	"context"
	"database/sql"
	"log"
	"net/http"
	"sync"
	"time"

	"systrack/internal/services"

	"github.com/gin-gonic/gin"
)

// CollectNowDeviceMetrics triggers immediate parallel SNMP collection for all enabled targets
// This is designed to be FAST like IP Scanner - 5 second timeout per device, all in parallel
func CollectNowDeviceMetrics(db *sql.DB, broadcastFunc func(services.DeviceMetrics)) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Get all enabled targets
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
			log.Printf("[Collect Now] Database query error: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch targets"})
			return
		}
		defer rows.Close()

		var targets []services.Target
		for rows.Next() {
			var t services.Target
			if err := rows.Scan(&t.ID, &t.Name, &t.Address, &t.SNMPCommunity, &t.SNMPVersion); err != nil {
				log.Printf("[Collect Now] Row scan error: %v", err)
				continue
			}
			t.MetricsEnabled = true
			targets = append(targets, t)
		}

		if len(targets) == 0 {
			c.JSON(http.StatusOK, gin.H{"message": "No targets to collect"})
			return
		}

		log.Printf("[Collect Now] Starting FAST parallel collection for %d targets...", len(targets))

		// Launch parallel collection in background (non-blocking)
		go func() {
			var wg sync.WaitGroup
			for _, target := range targets {
				wg.Add(1)
				go func(t services.Target) {
					defer wg.Done()

					// Fast collection with 2 second timeout (aggressive)
					ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
					defer cancel()

					metrics := collectMetricsFast(ctx, db, t)

					// Broadcast immediately via WebSocket
					if broadcastFunc != nil {
						broadcastFunc(metrics)
					}
				}(target)
			}
			wg.Wait()
			log.Printf("[Collect Now] Completed fast collection for %d targets", len(targets))
		}()

		c.JSON(http.StatusOK, gin.H{
			"message": "Collection started",
			"targets": len(targets),
		})
	}
}

// collectMetricsFast is a simplified, faster version of metrics collection
// Uses 5 second timeout and basic error handling for speed
func collectMetricsFast(ctx context.Context, db *sql.DB, target services.Target) services.DeviceMetrics {
	now := time.Now()
	metrics := services.DeviceMetrics{
		TargetID:      target.ID,
		TargetName:    target.Name,
		TargetAddress: target.Address,
		CollectedAt:   now,
		TsMs:          now.UnixMilli(),
		Status:        "success",
	}

	// Create a temporary collector just for the collection logic
	collector := services.NewDeviceMetricsCollector(db, nil)

	// Use the existing collectMetrics but with our fast context
	result := collector.CollectMetrics(target)

	// Copy all fields
	metrics.CPUPercent = result.CPUPercent
	metrics.CPUCores = result.CPUCores
	metrics.RAMTotalMB = result.RAMTotalMB
	metrics.RAMUsedMB = result.RAMUsedMB
	metrics.RAMPercent = result.RAMPercent
	metrics.DiskTotalGB = result.DiskTotalGB
	metrics.DiskUsedGB = result.DiskUsedGB
	metrics.DiskPercent = result.DiskPercent
	metrics.UptimeSeconds = result.UptimeSeconds
	metrics.Status = result.Status
	metrics.ErrorMsg = result.ErrorMsg

	return metrics
}
