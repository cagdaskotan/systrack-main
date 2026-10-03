package handlers

import (
	"database/sql"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

// DeviceMetricResponse represents a device metric response
type DeviceMetricResponse struct {
	ID            int64    `json:"id"`
	TargetID      int      `json:"target_id"`
	TargetName    string   `json:"target_name"`
	TargetAddress string   `json:"target_address"`
	CollectedAt   string   `json:"collected_at"`
	TsMs          int64    `json:"ts_ms"`
	CPUPercent    *float64 `json:"cpu_percent"`
	CPUCores      *int     `json:"cpu_cores"`
	RAMTotalMB    *int64   `json:"ram_total_mb"`
	RAMUsedMB     *int64   `json:"ram_used_mb"`
	RAMPercent    *float64 `json:"ram_percent"`
	DiskTotalGB   *int64   `json:"disk_total_gb"`
	DiskUsedGB    *int64   `json:"disk_used_gb"`
	DiskPercent   *float64 `json:"disk_percent"`
	UptimeSeconds *int64   `json:"uptime_seconds"`
	Status        string   `json:"status"`
	ErrorMsg      string   `json:"error_msg,omitempty"`
}

// GetDeviceMetrics returns metrics for a specific target
// GET /api/device-metrics/:target_id?from=...&to=...&limit=100
func GetDeviceMetrics(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		targetIDStr := c.Param("target_id")
		targetID, err := strconv.Atoi(targetIDStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid target ID"})
			return
		}

		// Parse query parameters
		fromStr := c.Query("from")
		toStr := c.Query("to")
		limitStr := c.DefaultQuery("limit", "100")

		limit, err := strconv.Atoi(limitStr)
		if err != nil || limit <= 0 || limit > 1000 {
			limit = 100
		}

		// Build query
		query := `
			SELECT
				dm.id, dm.target_id, t.name, t.address,
				dm.collected_at, dm.ts_ms,
				dm.cpu_percent, dm.cpu_cores,
				dm.ram_total_mb, dm.ram_used_mb, dm.ram_percent,
				dm.disk_total_gb, dm.disk_used_gb, dm.disk_percent,
				dm.uptime_seconds,
				dm.status, COALESCE(dm.error_msg, '') as error_msg
			FROM device_metrics dm
			INNER JOIN targets t ON t.id = dm.target_id
			WHERE dm.target_id = ?
		`

		args := []interface{}{targetID}

		// Add time filters if provided
		if fromStr != "" {
			query += " AND dm.collected_at >= ?"
			args = append(args, fromStr)
		}
		if toStr != "" {
			query += " AND dm.collected_at <= ?"
			args = append(args, toStr)
		}

		query += " ORDER BY dm.collected_at DESC LIMIT ?"
		args = append(args, limit)

		rows, err := db.Query(query, args...)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to query metrics"})
			return
		}
		defer rows.Close()

		var metrics []DeviceMetricResponse
		for rows.Next() {
			var m DeviceMetricResponse
			var collectedAt time.Time

			err := rows.Scan(
				&m.ID, &m.TargetID, &m.TargetName, &m.TargetAddress,
				&collectedAt, &m.TsMs,
				&m.CPUPercent, &m.CPUCores,
				&m.RAMTotalMB, &m.RAMUsedMB, &m.RAMPercent,
				&m.DiskTotalGB, &m.DiskUsedGB, &m.DiskPercent,
				&m.UptimeSeconds,
				&m.Status, &m.ErrorMsg,
			)
			if err != nil {
				continue
			}

			m.CollectedAt = collectedAt.Format(time.RFC3339)
			metrics = append(metrics, m)
		}

		c.JSON(http.StatusOK, metrics)
	}
}

// GetLatestDeviceMetrics returns the latest metrics for all enabled targets
// GET /api/device-metrics/latest
func GetLatestDeviceMetrics(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		query := `
			SELECT
				dm.id, dm.target_id, t.name, t.address,
				dm.collected_at, dm.ts_ms,
				dm.cpu_percent, dm.cpu_cores,
				dm.ram_total_mb, dm.ram_used_mb, dm.ram_percent,
				dm.disk_total_gb, dm.disk_used_gb, dm.disk_percent,
				dm.uptime_seconds,
				dm.status, COALESCE(dm.error_msg, '') as error_msg
			FROM device_metrics dm
			INNER JOIN targets t ON t.id = dm.target_id
			INNER JOIN (
				SELECT target_id, MAX(collected_at) as max_collected
				FROM device_metrics
				GROUP BY target_id
			) latest ON dm.target_id = latest.target_id
			         AND dm.collected_at = latest.max_collected
			WHERE t.metrics_enabled = 1 AND t.enabled = 1
			ORDER BY t.name ASC
		`

		rows, err := db.Query(query)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to query metrics"})
			return
		}
		defer rows.Close()

		var metrics []DeviceMetricResponse
		for rows.Next() {
			var m DeviceMetricResponse
			var collectedAt time.Time

			err := rows.Scan(
				&m.ID, &m.TargetID, &m.TargetName, &m.TargetAddress,
				&collectedAt, &m.TsMs,
				&m.CPUPercent, &m.CPUCores,
				&m.RAMTotalMB, &m.RAMUsedMB, &m.RAMPercent,
				&m.DiskTotalGB, &m.DiskUsedGB, &m.DiskPercent,
				&m.UptimeSeconds,
				&m.Status, &m.ErrorMsg,
			)
			if err != nil {
				continue
			}

			m.CollectedAt = collectedAt.Format(time.RFC3339)
			metrics = append(metrics, m)
		}

		c.JSON(http.StatusOK, metrics)
	}
}

// GetDeviceMetricsStats returns aggregated statistics for a target
// GET /api/device-metrics/:target_id/stats?period=24h
func GetDeviceMetricsStats(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		targetIDStr := c.Param("target_id")
		targetID, err := strconv.Atoi(targetIDStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid target ID"})
			return
		}

		period := c.DefaultQuery("period", "24h")

		// Determine time range
		var duration time.Duration
		switch period {
		case "1h":
			duration = 1 * time.Hour
		case "6h":
			duration = 6 * time.Hour
		case "24h":
			duration = 24 * time.Hour
		case "7d":
			duration = 7 * 24 * time.Hour
		default:
			duration = 24 * time.Hour
		}

		fromTime := time.Now().Add(-duration)

		query := `
			SELECT
				COUNT(*) as sample_count,
				AVG(cpu_percent) as cpu_avg,
				MIN(cpu_percent) as cpu_min,
				MAX(cpu_percent) as cpu_max,
				AVG(ram_percent) as ram_avg,
				MIN(ram_percent) as ram_min,
				MAX(ram_percent) as ram_max,
				AVG(disk_percent) as disk_avg,
				MIN(disk_percent) as disk_min,
				MAX(disk_percent) as disk_max
			FROM device_metrics
			WHERE target_id = ?
			  AND collected_at >= ?
			  AND status = 'success'
		`

		var stats struct {
			SampleCount int      `json:"sample_count"`
			CPUAvg      *float64 `json:"cpu_avg"`
			CPUMin      *float64 `json:"cpu_min"`
			CPUMax      *float64 `json:"cpu_max"`
			RAMAvg      *float64 `json:"ram_avg"`
			RAMMin      *float64 `json:"ram_min"`
			RAMMax      *float64 `json:"ram_max"`
			DiskAvg     *float64 `json:"disk_avg"`
			DiskMin     *float64 `json:"disk_min"`
			DiskMax     *float64 `json:"disk_max"`
		}

		err = db.QueryRow(query, targetID, fromTime).Scan(
			&stats.SampleCount,
			&stats.CPUAvg, &stats.CPUMin, &stats.CPUMax,
			&stats.RAMAvg, &stats.RAMMin, &stats.RAMMax,
			&stats.DiskAvg, &stats.DiskMin, &stats.DiskMax,
		)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to calculate stats"})
			return
		}

		c.JSON(http.StatusOK, stats)
	}
}
