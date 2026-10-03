package handlers

import (
	"database/sql"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	systemGraphTimeRangeMinutes = 10 // Son 10 dakikalık veri (momentary tablodan)
)

// SystemGraphAxis represents a single axis on the dashboard radar chart.
type SystemGraphAxis struct {
	Key          string  `json:"key"`
	Label        string  `json:"label"`
	SuccessCount int64   `json:"success_count"`
	FailureCount int64   `json:"failure_count"`
	TotalCount   int64   `json:"total_count"`
	SuccessRate  float64 `json:"success_rate"`
	FailureRate  float64 `json:"failure_rate"`
	Score        float64 `json:"score"`
	Static       bool    `json:"static"`
}

// SystemGraphResponse is the payload returned to the dashboard radar chart.
type SystemGraphResponse struct {
	GeneratedAt    time.Time         `json:"generated_at"`
	TimeRangeHours int               `json:"time_range_hours"`
	Axes           []SystemGraphAxis `json:"axes"`
	OverallScore   float64           `json:"overall_score"`
	OverallLabel   string            `json:"overall_label"`
}

type axisCounts struct {
	success int64
	failure int64
}

// GetSystemGraphStats aggregates monitoring and notification statistics for the dashboard radar chart.
func GetSystemGraphStats(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		windowStart := time.Now().Add(-time.Duration(systemGraphTimeRangeMinutes) * time.Minute)

		monitoringCounts, err := fetchMonitoringCounts(db, windowStart)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Monitoring istatistikleri getirilemedi"})
			return
		}

		notificationCounts, err := fetchNotificationCounts(db, windowStart)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Bildirim istatistikleri getirilemedi"})
			return
		}

		serviceHealthCounts := fetchServiceHealthCounts(db)

		axes := []SystemGraphAxis{
			buildSystemGraphAxis("http", "http", monitoringCounts["http"], false),
			buildSystemGraphAxis("https", "https", monitoringCounts["https"], false),
			buildSystemGraphAxis("services", "services", serviceHealthCounts, false),
			buildSystemGraphAxis("notifications", "notifications", notificationCounts, false),
			buildSystemGraphAxis("ping", "ping", monitoringCounts["ping"], false),
		}

		overallScore, overallLabel := calculateOverallHealth(axes)

		c.JSON(http.StatusOK, SystemGraphResponse{
			GeneratedAt:    time.Now(),
			TimeRangeHours: systemGraphTimeRangeMinutes, // Artık dakika cinsinden ama eski alan ismini koruyoruz
			Axes:           axes,
			OverallScore:   overallScore,
			OverallLabel:   overallLabel,
		})
	}
}

func fetchMonitoringCounts(db *sql.DB, windowStart time.Time) (map[string]axisCounts, error) {
	// pings_raw_momentary kullan - son 10 dakikalık veri için ÇOK DAHA HIZLI
	query := `
		SELECT
			COALESCE(t.monitoring_type, 'ping') AS monitoring_type,
			SUM(CASE WHEN p.ok = 1 THEN 1 ELSE 0 END) AS success_count,
			SUM(CASE WHEN p.ok = 0 THEN 1 ELSE 0 END) AS failure_count
		FROM pings_raw_momentary p
		INNER JOIN targets t ON t.id = p.target_id
		WHERE p.created_at >= ? AND t.enabled = 1
		GROUP BY monitoring_type
	`

	rows, err := db.Query(query, windowStart)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	stats := map[string]axisCounts{
		"ping":  {},
		"http":  {},
		"https": {},
	}

	for rows.Next() {
		var (
			monitoringType sql.NullString
			successCount   sql.NullInt64
			failureCount   sql.NullInt64
		)

		if err := rows.Scan(&monitoringType, &successCount, &failureCount); err != nil {
			continue
		}

		key := strings.ToLower(monitoringType.String)
		if key == "" {
			key = "ping"
		}
		// Normalize legacy/alternate types into radar axes
		switch key {
		case "icmp", "tcp":
			key = "ping"
		}

		// Accumulate into the normalized bucket
		bucket := stats[key]
		bucket.success += nullInt64Value(successCount)
		bucket.failure += nullInt64Value(failureCount)
		stats[key] = bucket
	}

	return stats, rows.Err()
}

func fetchNotificationCounts(db *sql.DB, windowStart time.Time) (axisCounts, error) {
	query := `
		SELECT 
			SUM(CASE WHEN status = 'sent' THEN 1 ELSE 0 END) AS sent_count,
			SUM(CASE WHEN status = 'failed' THEN 1 ELSE 0 END) AS failed_count
		FROM new_notification_mails
		WHERE sent_at >= ?
	`

	var (
		sentCount   sql.NullInt64
		failedCount sql.NullInt64
	)

	if err := db.QueryRow(query, windowStart).Scan(&sentCount, &failedCount); err != nil {
		return axisCounts{}, err
	}

	return axisCounts{
		success: nullInt64Value(sentCount),
		failure: nullInt64Value(failedCount),
	}, nil
}

func buildSystemGraphAxis(key, label string, counts axisCounts, isStatic bool) SystemGraphAxis {
	total := counts.success + counts.failure

	successRate := 1.0
	failureRate := 0.0
	if total > 0 {
		successRate = float64(counts.success) / float64(total)
		failureRate = float64(counts.failure) / float64(total)
	}

	score := successRate * 100
	score = math.Round(score*10) / 10 // One decimal precision

	return SystemGraphAxis{
		Key:          key,
		Label:        label,
		SuccessCount: counts.success,
		FailureCount: counts.failure,
		TotalCount:   total,
		SuccessRate:  math.Round(successRate*1000) / 1000,
		FailureRate:  math.Round(failureRate*1000) / 1000,
		Score:        score,
		Static:       isStatic,
	}
}

func fetchServiceHealthCounts(db *sql.DB) axisCounts {
	var running, stopped int64
	err := db.QueryRow(`
		SELECT
			COALESCE(SUM(CASE WHEN status = 'running' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN status != 'running' THEN 1 ELSE 0 END), 0)
		FROM device_services
		WHERE is_monitored = 1
		  AND DATE(last_checked) = CURDATE()
	`).Scan(&running, &stopped)
	if err != nil || (running == 0 && stopped == 0) {
		return axisCounts{success: 0, failure: 0}
	}
	return axisCounts{success: running, failure: stopped}
}

func calculateOverallHealth(axes []SystemGraphAxis) (float64, string) {
	if len(axes) == 0 {
		return 0, "unknown"
	}

	var sum float64
	for _, axis := range axes {
		sum += axis.Score
	}

	avg := sum / float64(len(axes))
	avg = math.Round(avg*10) / 10

	return avg, describeOverallHealth(avg)
}

// GetDashboardStats returns top card statistics in the same shape as WebSocket dashboard_stats
func GetDashboardStats(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		stats := make(map[string]interface{})

		// Total targets
		var totalTargets int
		db.QueryRow("SELECT COUNT(*) FROM targets").Scan(&totalTargets)
		stats["totalTargets"] = totalTargets

		// Online targets based on latest momentary record per target
		var onlineTargets int
		db.QueryRow(`
            SELECT COUNT(*)
            FROM targets t
            LEFT JOIN pings_raw_momentary p ON p.id = (
                SELECT id FROM pings_raw_momentary
                WHERE target_id = t.id
                ORDER BY created_at DESC
                LIMIT 1
            )
            WHERE t.enabled = 1 AND p.ok = 1
        `).Scan(&onlineTargets)
		stats["onlineTargets"] = onlineTargets
		stats["offlineTargets"] = totalTargets - onlineTargets

		// Open alerts: IP tespitleri + servis uyarıları (header badge ile aynı kaynaklar)
		var ipAlertsCount int
		db.QueryRow("SELECT COUNT(*) FROM ip_alerts WHERE acknowledged_at IS NULL").Scan(&ipAlertsCount)

		var serviceAlertsCount int
		db.QueryRow(`
			SELECT COUNT(*)
			FROM device_service_history h
			INNER JOIN device_services ds ON h.service_id = ds.id
			WHERE ds.is_monitored = TRUE
			  AND h.changed_at >= DATE_SUB(NOW(), INTERVAL 24 HOUR)
		`).Scan(&serviceAlertsCount)

		stats["openAlerts"] = ipAlertsCount + serviceAlertsCount
		stats["ipAlertsCount"] = ipAlertsCount
		stats["serviceAlertsCount"] = serviceAlertsCount

		// Ortalama uptime (son 10 dakika, pings_raw_momentary)
		var avgUptime sql.NullFloat64
		db.QueryRow(`
            SELECT 
                AVG(CASE WHEN p.ok = 1 THEN 100.0 ELSE 0.0 END) AS avg_uptime
            FROM pings_raw_momentary p
            INNER JOIN targets t ON t.id = p.target_id
            WHERE t.enabled = 1
              AND p.created_at >= DATE_SUB(NOW(), INTERVAL 10 MINUTE)
        `).Scan(&avgUptime)
		if avgUptime.Valid {
			stats["avgUptime"] = avgUptime.Float64
		} else {
			stats["avgUptime"] = 0.0
		}

		var slaBreachCount int
		db.QueryRow(`SELECT COUNT(DISTINCT target_id) FROM sla_calculations 
                     WHERE date >= DATE_SUB(CURDATE(), INTERVAL 7 DAY) 
                     AND uptime_percentage < 99.9`).Scan(&slaBreachCount)
		stats["slaBreachCount"] = slaBreachCount

		c.JSON(http.StatusOK, stats)
	}
}

func describeOverallHealth(score float64) string {
	switch {
	case score >= 85:
		return "excellent"
	case score >= 70:
		return "very_good"
	case score >= 60:
		return "healthy"
	case score >= 50:
		return "needs_improvement"
	case score > 0:
		return "critical"
	default:
		return "unknown"
	}
}

func nullInt64Value(v sql.NullInt64) int64 {
	if v.Valid {
		return v.Int64
	}
	return 0
}

// DashboardTargetResponse represents a target in the dashboard list
type DashboardTargetResponse struct {
	ID             int       `json:"id"`
	Name           string    `json:"name"`
	Address        string    `json:"address"`
	MonitoringType string    `json:"monitoring_type"`
	Type           string    `json:"type"`
	Status         string    `json:"status"`
	LastCheck      time.Time `json:"last_check"`
}

// DashboardTargetsListResponse represents the response for target lists
type DashboardTargetsListResponse struct {
	Targets   []DashboardTargetResponse `json:"targets"`
	Count     int                       `json:"count"`
	UpdatedAt time.Time                 `json:"updated_at"`
}

// GetOnlineTargets returns the list of online targets
func GetOnlineTargets(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Toplam online hedef sayısını al
		var totalOnlineCount int
		countQuery := `
		SELECT COUNT(*)
		FROM targets t
		LEFT JOIN pings_raw_momentary p ON p.id = (
			SELECT id FROM pings_raw_momentary
			WHERE target_id = t.id
			ORDER BY created_at DESC
			LIMIT 1
		)
		WHERE t.enabled = 1 AND p.ok = 1
	`
		if err := db.QueryRow(countQuery).Scan(&totalOnlineCount); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Online hedef sayısı alınamadı"})
			return
		}

		// Son 20 online hedefi getir (son eklenen hedefler önce)
		query := `
		SELECT
			t.id,
			t.name,
			t.address,
			t.monitoring_type,
			t.type,
			COALESCE(p.ts_ms, 0) as last_check
		FROM targets t
		LEFT JOIN pings_raw_momentary p ON p.id = (
			SELECT id FROM pings_raw_momentary
			WHERE target_id = t.id
			ORDER BY created_at DESC
			LIMIT 1
		)
		WHERE t.enabled = 1 AND p.ok = 1
		ORDER BY t.id DESC
		LIMIT 20
		`

		rows, err := db.Query(query)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Online hedefler getirilemedi"})
			return
		}
		defer rows.Close()

		var targets []DashboardTargetResponse
		for rows.Next() {
			var target DashboardTargetResponse
			var lastCheckMs int64
			if err := rows.Scan(
				&target.ID,
				&target.Name,
				&target.Address,
				&target.MonitoringType,
				&target.Type,
				&lastCheckMs,
			); err != nil {
				continue
			}
			target.Status = "online"
			if lastCheckMs > 0 {
				target.LastCheck = time.Unix(0, lastCheckMs*int64(time.Millisecond))
			}
			targets = append(targets, target)
		}

		c.JSON(http.StatusOK, DashboardTargetsListResponse{
			Targets:   targets,
			Count:     totalOnlineCount, // Toplam online hedef sayısı
			UpdatedAt: time.Now(),
		})
	}
}

// GetOfflineTargets returns the list of offline targets
func GetOfflineTargets(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Toplam offline hedef sayısını al
		var totalOfflineCount int
		countQuery := `
		SELECT COUNT(*)
		FROM targets t
		LEFT JOIN pings_raw_momentary p ON p.id = (
			SELECT id FROM pings_raw_momentary
			WHERE target_id = t.id
			ORDER BY created_at DESC
			LIMIT 1
		)
		WHERE t.enabled = 1 AND (p.ok IS NULL OR p.ok != 1)
	`
		if err := db.QueryRow(countQuery).Scan(&totalOfflineCount); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Offline hedef sayısı alınamadı"})
			return
		}

		// Son 20 offline hedefi getir (son eklenen hedefler önce)
		query := `
		SELECT
			t.id,
			t.name,
			t.address,
			t.monitoring_type,
			t.type,
			COALESCE(p.ts_ms, 0) as last_check
		FROM targets t
		LEFT JOIN pings_raw_momentary p ON p.id = (
			SELECT id FROM pings_raw_momentary
			WHERE target_id = t.id
			ORDER BY created_at DESC
			LIMIT 1
		)
		WHERE t.enabled = 1 AND (p.ok IS NULL OR p.ok != 1)
		ORDER BY t.id DESC
		LIMIT 20
		`

		rows, err := db.Query(query)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Offline hedefler getirilemedi"})
			return
		}
		defer rows.Close()

		var targets []DashboardTargetResponse
		for rows.Next() {
			var target DashboardTargetResponse
			var lastCheckMs int64
			if err := rows.Scan(
				&target.ID,
				&target.Name,
				&target.Address,
				&target.MonitoringType,
				&target.Type,
				&lastCheckMs,
			); err != nil {
				continue
			}
			target.Status = "offline"
			if lastCheckMs > 0 {
				target.LastCheck = time.Unix(0, lastCheckMs*int64(time.Millisecond))
			}
			targets = append(targets, target)
		}

		c.JSON(http.StatusOK, DashboardTargetsListResponse{
			Targets:   targets,
			Count:     totalOfflineCount, // Toplam offline hedef sayısı
			UpdatedAt: time.Now(),
		})
	}
}
