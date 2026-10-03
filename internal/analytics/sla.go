package analytics

import (
	"database/sql"
	"fmt"
	"log"
	"time"
)

// SLAConfig holds SLA configuration
type SLAConfig struct {
	TargetID      int     `json:"target_id"`
	TargetName    string  `json:"target_name"`
	TargetAddr    string  `json:"target_addr"`
	SLATarget     float64 `json:"sla_target"` // e.g., 99.9 for 99.9%
	SLABreach     bool    `json:"sla_breach"`
	CurrentUptime float64 `json:"current_uptime"`
}

// SLACalculation represents SLA calculation result
type SLACalculation struct {
	TargetID         int       `json:"target_id"`
	Date             time.Time `json:"date"`
	TotalChecks      int       `json:"total_checks"`
	SuccessfulChecks int       `json:"successful_checks"`
	UptimePercentage float64   `json:"uptime_percentage"`
	SLABreach        bool      `json:"sla_breach"`
	SLATarget        float64   `json:"sla_target"`
}

// SLATrend represents SLA trend data
type SLATrend struct {
	Date             time.Time `json:"date"`
	UptimePercentage float64   `json:"uptime_percentage"`
	TotalChecks      int       `json:"total_checks"`
	SuccessfulChecks int       `json:"successful_checks"`
}

// SLAService handles SLA calculations
type SLAService struct {
	db          *sql.DB
	onSLABreach func(targetID int, targetName string, slaTarget float64, currentUptime float64) // Callback for SLA breaches
}

// NewSLAService creates a new SLA service
func NewSLAService(db *sql.DB) *SLAService {
	return &SLAService{
		db: db,
	}
}

// SetSLABreachCallback callback fonksiyonunu ayarlar
func (s *SLAService) SetSLABreachCallback(callback func(targetID int, targetName string, slaTarget float64, currentUptime float64)) {
	s.onSLABreach = callback
}

// GetSLACalculation gets SLA calculation for a specific target and date
func (s *SLAService) GetSLACalculation(targetID int, date time.Time) (*SLACalculation, error) {
	query := `
		SELECT target_id, date, total_checks, successful_checks, uptime_percentage
		FROM sla_calculations 
		WHERE target_id = ? AND date = ?
	`

	var calc SLACalculation
	err := s.db.QueryRow(query, targetID, date.Format("2006-01-02")).Scan(
		&calc.TargetID, &calc.Date, &calc.TotalChecks, &calc.SuccessfulChecks, &calc.UptimePercentage,
	)
	if err != nil {
		return nil, err
	}

	// Default SLA target is 99.9%
	calc.SLATarget = 99.9
	calc.SLABreach = calc.UptimePercentage < calc.SLATarget

	return &calc, nil
}

// GetSLATrends gets SLA trends for a target over a period
func (s *SLAService) GetSLATrends(targetID int, days int) ([]SLATrend, error) {
	query := `
		SELECT date, uptime_percentage, total_checks, successful_checks
		FROM sla_calculations 
		WHERE target_id = ? 
		AND date >= DATE_SUB(CURDATE(), INTERVAL ? DAY)
		ORDER BY date ASC
	`

	rows, err := s.db.Query(query, targetID, days)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var trends []SLATrend
	for rows.Next() {
		var trend SLATrend
		var dateStr string

		err := rows.Scan(&dateStr, &trend.UptimePercentage, &trend.TotalChecks, &trend.SuccessfulChecks)
		if err != nil {
			continue
		}

		trend.Date, _ = time.Parse("2006-01-02", dateStr)
		trends = append(trends, trend)
	}

	return trends, nil
}

// GetSLASummary gets SLA summary for all targets
func (s *SLAService) GetSLASummary() ([]SLAConfig, error) {
	query := `
		SELECT 
			t.id,
			t.name,
			t.address,
			COALESCE(AVG(sc.uptime_percentage), 0) as avg_uptime
		FROM targets t
		LEFT JOIN sla_calculations sc ON t.id = sc.target_id
		WHERE t.enabled = true
		GROUP BY t.id, t.name, t.address
		ORDER BY t.name
	`

	rows, err := s.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var configs []SLAConfig
	for rows.Next() {
		var config SLAConfig
		err := rows.Scan(&config.TargetID, &config.TargetName, &config.TargetAddr, &config.CurrentUptime)
		if err != nil {
			continue
		}

		config.SLATarget = 99.9 // Default SLA target
		config.SLABreach = config.CurrentUptime < config.SLATarget
		configs = append(configs, config)
	}

	return configs, nil
}

// GetWeeklySLATrends gets weekly SLA trends for a target
func (s *SLAService) GetWeeklySLATrends(targetID int, weeks int) ([]SLATrend, error) {
	query := `
		SELECT 
			CONCAT(year, '-', LPAD(week, 2, '0'), '-01') as week_start,
			uptime_percentage,
			total_checks,
			successful_checks
		FROM sla_weekly 
		WHERE target_id = ? 
		AND (year * 100 + week) >= (YEAR(CURDATE()) * 100 + WEEK(CURDATE()) - ?)
		ORDER BY year, week ASC
	`

	rows, err := s.db.Query(query, targetID, weeks)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var trends []SLATrend
	for rows.Next() {
		var trend SLATrend
		var weekStr string

		err := rows.Scan(&weekStr, &trend.UptimePercentage, &trend.TotalChecks, &trend.SuccessfulChecks)
		if err != nil {
			continue
		}

		trend.Date, _ = time.Parse("2006-01-02", weekStr)
		trends = append(trends, trend)
	}

	return trends, nil
}

// GetMonthlySLATrends gets monthly SLA trends for a target
func (s *SLAService) GetMonthlySLATrends(targetID int, months int) ([]SLATrend, error) {
	query := `
		SELECT 
			CONCAT(year, '-', LPAD(month, 2, '0'), '-01') as month_start,
			uptime_percentage,
			total_checks,
			successful_checks
		FROM sla_monthly 
		WHERE target_id = ? 
		AND (year * 100 + month) >= (YEAR(CURDATE()) * 100 + MONTH(CURDATE()) - ?)
		ORDER BY year, month ASC
	`

	rows, err := s.db.Query(query, targetID, months)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var trends []SLATrend
	for rows.Next() {
		var trend SLATrend
		var monthStr string

		err := rows.Scan(&monthStr, &trend.UptimePercentage, &trend.TotalChecks, &trend.SuccessfulChecks)
		if err != nil {
			continue
		}

		trend.Date, _ = time.Parse("2006-01-02", monthStr)
		trends = append(trends, trend)
	}

	return trends, nil
}

// CalculateSLABreach checks if SLA is breached for a target
func (s *SLAService) CalculateSLABreach(targetID int, slaTarget float64, period string) (bool, float64, error) {
	var query string
	var args []interface{}

	switch period {
	case "daily":
		query = `
			SELECT AVG(uptime_percentage) 
			FROM sla_calculations 
			WHERE target_id = ? AND date >= DATE_SUB(CURDATE(), INTERVAL 1 DAY)
		`
		args = []interface{}{targetID}
	case "weekly":
		query = `
			SELECT AVG(uptime_percentage) 
			FROM sla_weekly 
			WHERE target_id = ? AND (year * 100 + week) >= (YEAR(CURDATE()) * 100 + WEEK(CURDATE()) - 1)
		`
		args = []interface{}{targetID}
	case "monthly":
		query = `
			SELECT AVG(uptime_percentage) 
			FROM sla_monthly 
			WHERE target_id = ? AND (year * 100 + month) >= (YEAR(CURDATE()) * 100 + MONTH(CURDATE()) - 1)
		`
		args = []interface{}{targetID}
	default:
		return false, 0, fmt.Errorf("invalid period: %s", period)
	}

	var avgUptime sql.NullFloat64
	err := s.db.QueryRow(query, args...).Scan(&avgUptime)
	if err != nil {
		return false, 0, err
	}

	if !avgUptime.Valid {
		return false, 0, nil
	}

	breach := avgUptime.Float64 < slaTarget
	return breach, avgUptime.Float64, nil
}

// GetSLAMetrics gets comprehensive SLA metrics for a target
func (s *SLAService) GetSLAMetrics(targetID int) (map[string]interface{}, error) {
	metrics := make(map[string]interface{})

	// Daily metrics (last 7 days)
	dailyTrends, err := s.GetSLATrends(targetID, 7)
	if err != nil {
		return nil, err
	}
	metrics["daily_trends"] = dailyTrends

	// Weekly metrics (last 4 weeks)
	weeklyTrends, err := s.GetWeeklySLATrends(targetID, 4)
	if err != nil {
		return nil, err
	}
	metrics["weekly_trends"] = weeklyTrends

	// Monthly metrics (last 6 months)
	monthlyTrends, err := s.GetMonthlySLATrends(targetID, 6)
	if err != nil {
		return nil, err
	}
	metrics["monthly_trends"] = monthlyTrends

	// Current uptime
	if len(dailyTrends) > 0 {
		metrics["current_uptime"] = dailyTrends[len(dailyTrends)-1].UptimePercentage
	} else {
		metrics["current_uptime"] = 0.0
	}

	// SLA breach status
	breach, uptime, err := s.CalculateSLABreach(targetID, 99.9, "daily")
	if err != nil {
		return nil, err
	}
	metrics["sla_breach"] = breach
	metrics["sla_target"] = 99.9
	metrics["avg_uptime"] = uptime

	return metrics, nil
}

// CalculateCurrentUptime calculates current uptime for a target
func (s *SLAService) CalculateCurrentUptime(targetID int) (float64, error) {
	query := `
		SELECT 
			COUNT(*) as total_checks,
			SUM(CASE WHEN ok = 1 THEN 1 ELSE 0 END) as successful_checks
		FROM pings_raw 
		WHERE target_id = ? AND FROM_UNIXTIME(ts_ms/1000) >= DATE_SUB(NOW(), INTERVAL 24 HOUR)
	`

	var totalChecks, successfulChecks int
	err := s.db.QueryRow(query, targetID).Scan(&totalChecks, &successfulChecks)
	if err != nil {
		return 0.0, err
	}

	if totalChecks == 0 {
		return 0.0, nil
	}

	uptime := float64(successfulChecks) / float64(totalChecks) * 100
	return uptime, nil
}

// CheckAndNotifySLABreaches tüm target'lar için SLA ihlali kontrolü yapar ve bildirim gönderir
func (s *SLAService) CheckAndNotifySLABreaches() error {
	// Tüm target'ları al
	query := `SELECT id, name, address FROM targets WHERE is_active = true`
	rows, err := s.db.Query(query)
	if err != nil {
		return fmt.Errorf("failed to get targets: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var targetID int
		var targetName, targetAddress string

		err := rows.Scan(&targetID, &targetName, &targetAddress)
		if err != nil {
			log.Printf("Failed to scan target: %v", err)
			continue
		}

		// SLA ihlali kontrolü
		breach, uptime, err := s.CalculateSLABreach(targetID, 99.9, "daily")
		if err != nil {
			log.Printf("Failed to calculate SLA breach for target %d: %v", targetID, err)
			continue
		}

		// SLA ihlali varsa bildirim gönder
		if breach && s.onSLABreach != nil {
			log.Printf("SLA breach detected for target %s (ID: %d): %.2f%% < 99.9%%", targetName, targetID, uptime)
			s.onSLABreach(targetID, targetName, 99.9, uptime)
		}
	}

	return nil
}
