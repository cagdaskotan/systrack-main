package notifications

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"html"
	"log"
	"math"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"systrack/internal/services"
)

// SimpleRuleRunner periodically checks new_notification_rules and sends emails when targets have failing pings
type SimpleRuleRunner struct {
	db       *sql.DB
	ctx      context.Context
	cancel   context.CancelFunc
	interval time.Duration

	// Backoff modu emniyet kilidi: (kural,hedef) başına son gönderim denemesi.
	// DB tarafında ne olursa olsun aynı hedefe backoffMinResendGap'ten sık
	// gönderim yapılmasını engeller (spam sigortası).
	backoffMu       sync.Mutex
	backoffLastSend map[string]time.Time
}

var defaultSimpleRunner *SimpleRuleRunner

// SetSimpleRuleRunner registers the global runner for out-of-band triggers.
func SetSimpleRuleRunner(r *SimpleRuleRunner) {
	defaultSimpleRunner = r
}

// TriggerSimpleRule triggers evaluation for the given rule ID if a runner is registered.
func TriggerSimpleRule(ruleID int) {
	if defaultSimpleRunner != nil {
		go defaultSimpleRunner.RunRuleNow(ruleID)
	}
}

// TriggerAllSimpleRules triggers evaluation for all active rules if a runner is registered.
func TriggerAllSimpleRules() {
	if defaultSimpleRunner != nil {
		go defaultSimpleRunner.RunAllNow()
	}
}

func NewSimpleRuleRunner(db *sql.DB) *SimpleRuleRunner {
	ctx, cancel := context.WithCancel(context.Background())
	return &SimpleRuleRunner{db: db, ctx: ctx, cancel: cancel, interval: time.Minute, backoffLastSend: map[string]time.Time{}}
}

func (r *SimpleRuleRunner) Start() {
	// Gönderim logu tablosunun modern şemada olduğunu garanti et (runtime self-heal).
	// Eski kurulumlarda channel/template_id kolonları yoktu; onlarsız INSERT patlar,
	// log boş kalır ve backoff dedup'u çalışmaz.
	r.ensureMailLogSchema()
	go r.loop()
}

func (r *SimpleRuleRunner) Stop() {
	r.cancel()
}

type newRuleRow struct {
	ID             int
	Name           string
	TargetID       *int
	TargetIDs      []int
	Channel        string
	IntervalMin    int
	Recipients     []string
	TemplateID     *int
	Conditions     []ruleCondition
	LastSent       *time.Time
	LastSentStatus *string // "success" or "fail" - track last notification status
	IsActive       bool
}

type notificationTemplate struct {
	ID        int
	Name      string
	Channel   string
	Subject   *string
	Body      string
	Variables []string
	IsDefault bool
	IsActive  bool
	UpdatedAt time.Time
	CreatedAt time.Time
}

// ruleCondition represents a simple condition structure stored in JSON
type ruleCondition struct {
	Field    string      `json:"field"`
	Operator string      `json:"operator"`
	Value    interface{} `json:"value"`
}

func ruleWindow(rr *newRuleRow) time.Duration {
	if rr.IntervalMin <= 0 {
		return time.Minute
	}
	return time.Duration(rr.IntervalMin) * time.Minute
}
func windowMinutes(d time.Duration) int {
	mins := int(math.Ceil(d.Minutes()))
	if mins < 1 {
		mins = 1
	}
	return mins
}

func (r *SimpleRuleRunner) loop() {
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			r.processFiltered(nil, false)
			r.CheckCameraRules()
		case <-r.ctx.Done():
			return
		}
	}
}

func (r *SimpleRuleRunner) processFiltered(ruleID *int, force bool) {
	query := `
        SELECT id, name, target_id, channel, schedule_interval_minutes, recipients, template_id, conditions, last_sent_at, last_sent_status, is_active
        FROM new_notification_rules
        WHERE is_active = 1`
	var rows *sql.Rows
	var err error
	if ruleID != nil {
		query += ` AND id = ?`
		rows, err = r.db.Query(query, *ruleID)
	} else {
		rows, err = r.db.Query(query)
	}
	if err != nil {
		log.Printf("rule runner query err: %v", err)
		return
	}
	defer rows.Close()

	now := time.Now()
	var metricsCollector *services.DeviceMetricsCollector
	for rows.Next() {
		var rr newRuleRow
		var recJSON sql.NullString
		var tplID sql.NullInt64
		var condJSON sql.NullString
		var tgtID sql.NullInt64
		var lastSent sql.NullTime
		var lastSentStatus sql.NullString
		if err := rows.Scan(&rr.ID, &rr.Name, &tgtID, &rr.Channel, &rr.IntervalMin, &recJSON, &tplID, &condJSON, &lastSent, &lastSentStatus, &rr.IsActive); err != nil {
			continue
		}
		if lastSentStatus.Valid {
			rr.LastSentStatus = &lastSentStatus.String
		}
		if recJSON.Valid {
			_ = json.Unmarshal([]byte(recJSON.String), &rr.Recipients)
		}
		if tplID.Valid {
			id := int(tplID.Int64)
			rr.TemplateID = &id
		}
		if tgtID.Valid {
			id := int(tgtID.Int64)
			rr.TargetID = &id
		}
		if lastSent.Valid {
			rr.LastSent = &lastSent.Time
		}
		if condJSON.Valid {
			cleanRaw, ids := stripTargetIDsFromConditions([]byte(condJSON.String))
			if len(ids) > 0 {
				rr.TargetIDs = ids
			}
			rr.Conditions = parseRuleConditions(cleanRaw)
		}
		if len(rr.TargetIDs) == 1 {
			// treat as single target rule
			if rr.TargetID == nil {
				single := rr.TargetIDs[0]
				rr.TargetID = &single
			}
			rr.TargetIDs = nil
		}

		// Check if target exists
		if rr.TargetID == nil && len(rr.TargetIDs) == 0 {
			log.Printf("Skipping rule %d because target selection missing", rr.ID)
			continue
		}

		// AKILLI (AZALAN) SIKLIK ("backoff") modu — ekstra bir seçenek:
		// Sabit aralık yerine, cihaz çevrimdışı kaldıkça seyrekleşen bildirim gönderir
		// (0-3 saat: saatte bir, 3-24 saat: 2 saatte bir, 24 saat sonrası: günde bir;
		// çevrimiçine dönünce toplam kesinti süresiyle tek kurtarma bildirimi).
		// Kaynaklar: son ping (pings_raw) + gönderim logu (new_notification_mails); şemaya ek yok.
		// Yalnızca saf durum (online/offline) kuralları için geçerlidir; diğer kurallar
		// ve bayrağı olmayan tüm kurallar mevcut sabit akışla AYNEN devam eder.
		if !force && hasBackoffMode(rr.Conditions) && isPureStatusConditions(rr.Conditions) {
			r.processBackoffRule(&rr, now)
			continue
		}

		// SIMPLE PERIODIC NOTIFICATION LOGIC:
		// Only check if schedule interval has passed since last_sent_at
		// No status checks, no condition checks - just time-based periodic notifications

		if !force {
			// If last_sent_at is null, start the interval without sending
			if rr.LastSent == nil {
				log.Printf("⏸️ Rule %d - Initial schedule started (last_sent_at is NULL)", rr.ID)
				_, _ = r.db.Exec("UPDATE new_notification_rules SET last_sent_at = NOW() WHERE id = ?", rr.ID)
				continue
			} else {
				// Check if enough time has passed since last notification
				intervalDuration := time.Duration(rr.IntervalMin) * time.Minute
				nextAllowed := rr.LastSent.Add(intervalDuration)

				// Debug log to see actual values
				log.Printf("🔍 Rule %d - Debug: last_sent_at=%s, interval=%d min, next_allowed=%s, now=%s",
					rr.ID, rr.LastSent.Format("2006-01-02 15:04:05"), rr.IntervalMin,
					nextAllowed.Format("2006-01-02 15:04:05"), now.Format("2006-01-02 15:04:05"))

				if now.Before(nextAllowed) {
					log.Printf("⏸️ Rule %d - Skipping (next allowed: %s, remaining: %v)",
						rr.ID, nextAllowed.Format("2006-01-02 15:04:05"), nextAllowed.Sub(now).Round(time.Second))
					continue
				}

				log.Printf("⏰ Rule %d - Interval passed (%d min), sending notification", rr.ID, rr.IntervalMin)
			}
		} else {
			log.Printf("🚀 Rule %d - Force triggered, sending immediately", rr.ID)
		}

		serviceMode, serviceIDs, hasService := findServiceSelection(rr.Conditions)
		if hasService {
			if rr.TargetID == nil {
				log.Printf("Skipping rule %d because service notifications require a single target", rr.ID)
				continue
			}

			windowStart := time.Now().Add(-ruleWindow(&rr))
			if rr.LastSent != nil {
				windowStart = *rr.LastSent
			}

			changes, err := r.loadServiceChanges(*rr.TargetID, windowStart, serviceMode, serviceIDs)
			if err != nil {
				log.Printf("Failed to load service changes for rule %d: %v", rr.ID, err)
				continue
			}
			if len(changes) == 0 {
				log.Printf("🔕 Rule %d - No service changes detected", rr.ID)
				continue
			}

			if err := r.sendServiceNotification(rr, *rr.TargetID, changes); err != nil {
				log.Printf("Failed to send service notification for rule %d: %v", rr.ID, err)
			} else {
				log.Printf("✅ Service notification sent for rule %d (%d changes)", rr.ID, len(changes))
				_, _ = r.db.Exec("UPDATE new_notification_rules SET last_sent_at = NOW() WHERE id = ?", rr.ID)
			}
			continue
		}

		metricCond, hasMetric := findMetricCondition(rr.Conditions)
		if hasMetric {
			if metricsCollector == nil {
				metricsCollector = services.NewDeviceMetricsCollector(r.db, nil)
			}
			targetIDs := make([]int, 0, len(rr.TargetIDs)+1)
			if len(rr.TargetIDs) > 0 {
				targetIDs = append(targetIDs, rr.TargetIDs...)
			} else if rr.TargetID != nil {
				targetIDs = append(targetIDs, *rr.TargetID)
			}
			sections := make([]messagePayload, 0, len(targetIDs))
			for _, targetID := range targetIDs {
				metricsTarget, ok := r.loadMetricsTarget(targetID)
				if !ok {
					log.Printf("Skipping rule %d because target %d is not eligible for metrics", rr.ID, targetID)
					continue
				}
				metrics := metricsCollector.CollectMetrics(metricsTarget)
				if metrics.Status != "success" {
					log.Printf("[%s] Metrics unavailable for target %d (%s)", metricsTarget.Address, targetID, metrics.Status)
					continue
				}
				measured, ok := metricValueFromMetrics(metrics, metricCond.Field)
				if !ok {
					log.Printf("[%s] Metric %s missing for target %d", metricsTarget.Address, metricCond.Field, targetID)
					continue
				}
				if !meetsMetricCondition(measured, metricCond.Operator, metricCond.Value) {
					continue
				}

				mc := &messageContext{
					kind:            "metric",
					metricKind:      metricCond.Field,
					metricOp:        metricCond.Operator,
					metricThreshold: metricCond.Value,
					metricValue:     measured,
					metricUnit:      metricUnit(metricCond.Field),
					cpuPercent:      metrics.CPUPercent,
					ramGB:           nil,
					diskGB:          nil,
					temperatureC:    metrics.TemperatureC,
				}
				if metrics.RAMUsedMB != nil {
					val := float64(*metrics.RAMUsedMB) / 1024.0
					mc.ramGB = &val
				}
				if metrics.DiskUsedGB != nil {
					val := float64(*metrics.DiskUsedGB)
					mc.diskGB = &val
				}

				name, addr, _, _, _ := r.loadTargetWithStatus(targetID)
				payload := r.buildNotificationPayload(rr, targetID, name, addr, mc)
				sections = append(sections, payload)
			}

			if len(sections) == 0 {
				continue
			}

			if len(targetIDs) > 1 {
				subject := fmt.Sprintf("[SysTrack] %s - %d hedef", rr.Name, len(sections))
				var bodyBuilder strings.Builder
				var textBuilder strings.Builder
				for i, section := range sections {
					if i > 0 {
						bodyBuilder.WriteString("\n\n----------------------------------------\n\n")
						textBuilder.WriteString("\n\n----------------------------------------\n\n")
					}
					bodyBuilder.WriteString(section.Body)
					textBuilder.WriteString(section.Text)
				}
				payload := messagePayload{
					Subject: subject,
					Body:    bodyBuilder.String(),
					Text:    textBuilder.String(),
				}
				if err := r.dispatchChannel(rr, targetIDs[0], payload); err != nil {
					log.Printf("Failed to send metric notification for rule %d: %v", rr.ID, err)
				} else {
					log.Printf("Metric notification sent for rule %d (%d targets)", rr.ID, len(sections))
					_, _ = r.db.Exec("UPDATE new_notification_rules SET last_sent_at = NOW() WHERE id = ?", rr.ID)
				}
			} else {
				if err := r.dispatchChannel(rr, targetIDs[0], sections[0]); err != nil {
					log.Printf("Failed to send metric notification for rule %d: %v", rr.ID, err)
				} else {
					log.Printf("Metric notification sent for rule %d", rr.ID)
					_, _ = r.db.Exec("UPDATE new_notification_rules SET last_sent_at = NOW() WHERE id = ?", rr.ID)
				}
			}
			continue
		}

		// Send notification for single or multiple targets
		if len(rr.TargetIDs) > 1 {
			// Multiple targets - send bulk notification
			if err := r.sendBulkRule(rr); err != nil {
				log.Printf("❌ Failed to send bulk notification for rule %d: %v", rr.ID, err)
			} else {
				log.Printf("✅ Bulk notification sent for rule %d (%d targets)", rr.ID, len(rr.TargetIDs))
				_, _ = r.db.Exec("UPDATE new_notification_rules SET last_sent_at = NOW() WHERE id = ?", rr.ID)
			}
		} else {
			// Single target notification
			var targetExists bool
			if err := r.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM targets WHERE id = ?)`, *rr.TargetID).Scan(&targetExists); err != nil || !targetExists {
				log.Printf("Skipping rule %d because target %d is missing", rr.ID, *rr.TargetID)
				continue
			}

			// Get current target status for notification message
			name, _, monitoringType, statusCode, lastSuccess := r.loadTargetWithStatus(*rr.TargetID)

			var currentStatus string
			if lastSuccess {
				currentStatus = "success"
			} else {
				currentStatus = "fail"
			}

			// Online ise ve önceki durum da online ise bildirim gönderme
			if rr.LastSentStatus != nil && currentStatus == "success" && *rr.LastSentStatus == "success" {
				log.Printf("⏭️ Rule %d - Target %d (%s) is ONLINE and was already ONLINE - Skipping notification", rr.ID, *rr.TargetID, name)
				continue
			}

			// İlk bildirimde hedef online ise bildirim gönderme
			if rr.LastSentStatus == nil && currentStatus == "success" {
				log.Printf("⏭️ Rule %d - Target %d (%s) is ONLINE on first run - Skipping initial notification", rr.ID, *rr.TargetID, name)
				continue
			}

			// Durum değişikliğini logla
			if rr.LastSentStatus != nil {
				if *rr.LastSentStatus == "fail" && currentStatus == "success" {
					log.Printf("🔄 Rule %d - Status change detected: OFFLINE → ONLINE for target %d (%s)", rr.ID, *rr.TargetID, name)
				} else if *rr.LastSentStatus == "success" && currentStatus == "fail" {
					log.Printf("🔄 Rule %d - Status change detected: ONLINE → OFFLINE for target %d (%s)", rr.ID, *rr.TargetID, name)
				} else if currentStatus == "fail" {
					log.Printf("🔁 Rule %d - Target %d (%s) is still OFFLINE - Sending periodic notification", rr.ID, *rr.TargetID, name)
				}
			} else {
				log.Printf("🆕 Rule %d - First notification for target %d (%s) - Status: %s", rr.ID, *rr.TargetID, name, currentStatus)
			}

			mc := &messageContext{
				kind:           "status",
				status:         currentStatus,
				monitoringType: monitoringType,
				statusCode:     statusCode,
				lastSuccess:    lastSuccess,
			}

			// Send notification
			winStart, winEnd, _ := r.timeWindow(time.Duration(rr.IntervalMin) * time.Minute)
			if err := r.sendForTargetWithContext(rr, *rr.TargetID, winStart, winEnd, mc); err != nil {
				log.Printf("❌ Failed to send notification for rule %d, target %d: %v", rr.ID, *rr.TargetID, err)
			} else {
				log.Printf("✅ Notification sent for rule %d, target %d (%s) - Status: %s", rr.ID, *rr.TargetID, name, currentStatus)

				// Update last_sent_at and last_sent_status
				_, _ = r.db.Exec("UPDATE new_notification_rules SET last_sent_at = NOW(), last_sent_status = ? WHERE id = ?", currentStatus, rr.ID)
			}
		}
	}
}

// RunRuleNow triggers an immediate evaluation for a specific rule.
func (r *SimpleRuleRunner) RunRuleNow(ruleID int) {
	r.processFiltered(&ruleID, true)
}

// RunAllNow triggers an immediate evaluation for all rules.
func (r *SimpleRuleRunner) RunAllNow() {
	r.processFiltered(nil, true)
}

func (r *SimpleRuleRunner) failureWindow(targetID int, window time.Duration) (time.Time, time.Time, bool) {
	mins := windowMinutes(window)
	winStart := time.Now().Add(-time.Duration(mins) * time.Minute)
	winEnd := time.Now()
	query := fmt.Sprintf(`
        SELECT COUNT(*) FROM pings_raw
        WHERE target_id = ? AND ok = 0 AND ts_ms > UNIX_TIMESTAMP(NOW() - INTERVAL %d MINUTE)*1000
    `, mins)
	var cnt int
	err := r.db.QueryRow(query, targetID).Scan(&cnt)
	if err != nil {
		return winStart, winEnd, false
	}
	return winStart, winEnd, cnt > 0
}

func (r *SimpleRuleRunner) targetsWithFailures(window time.Duration) ([]int, error) {
	mins := windowMinutes(window)
	query := fmt.Sprintf(`
        SELECT DISTINCT target_id FROM pings_raw
        WHERE ok = 0 AND ts_ms > UNIX_TIMESTAMP(NOW() - INTERVAL %d MINUTE)*1000
    `, mins)
	rows, err := r.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := []int{}
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err == nil {
			ids = append(ids, id)
		}
	}
	return ids, nil
}

// getAllTargets returns all active target IDs
func (r *SimpleRuleRunner) getAllTargets() ([]int, error) {
	query := `SELECT id FROM targets WHERE enabled = 1`
	rows, err := r.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := []int{}
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err == nil {
			ids = append(ids, id)
		}
	}
	return ids, nil
}

// statusWindow returns whether there exists a ping with desired status in the given window
func (r *SimpleRuleRunner) statusWindow(targetID int, want string, window time.Duration) (time.Time, time.Time, bool) {
	mins := windowMinutes(window)
	winStart := time.Now().Add(-time.Duration(mins) * time.Minute)
	winEnd := time.Now()

	if want == "success" {
		query := fmt.Sprintf(`
            SELECT COUNT(*) FROM alerts
            WHERE target_id = ? AND status = 'closed' AND closed_at IS NOT NULL
              AND closed_at > NOW() - INTERVAL %d MINUTE
        `, mins)
		var cnt int
		err := r.db.QueryRow(query, targetID).Scan(&cnt)
		if err != nil {
			return winStart, winEnd, false
		}
		return winStart, winEnd, cnt > 0
	}

	query := fmt.Sprintf(`
        SELECT COUNT(*) FROM pings_raw
        WHERE target_id = ? AND ok = 0 AND ts_ms > UNIX_TIMESTAMP(NOW() - INTERVAL %d MINUTE)*1000
    `, mins)
	var cnt int
	err := r.db.QueryRow(query, targetID).Scan(&cnt)
	if err != nil {
		return winStart, winEnd, false
	}
	return winStart, winEnd, cnt > 0
}

func (r *SimpleRuleRunner) targetsWithStatus(want string, window time.Duration) ([]int, error) {
	mins := windowMinutes(window)
	var query string
	if want == "success" {
		query = fmt.Sprintf(`
            SELECT DISTINCT target_id FROM alerts
            WHERE status = 'closed' AND closed_at IS NOT NULL
              AND closed_at > NOW() - INTERVAL %d MINUTE
        `, mins)
	} else {
		query = fmt.Sprintf(`
            SELECT DISTINCT target_id FROM pings_raw
            WHERE ok = 0 AND ts_ms > UNIX_TIMESTAMP(NOW() - INTERVAL %d MINUTE)*1000
        `, mins)
	}
	rows, err := r.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := []int{}
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err == nil {
			ids = append(ids, id)
		}
	}
	return ids, nil
}

// findStatusCondition extracts a status condition value if present
func findStatusCondition(conds []ruleCondition) (string, bool) {
	for _, c := range conds {
		if strings.EqualFold(c.Field, "status") {
			// accept value as string
			switch v := c.Value.(type) {
			case string:
				return v, true
			default:
				return "", true
			}
		}
	}
	return "", false
}

type rtCond struct {
	Operator string
	Value    int
}

type metricCond struct {
	Field    string
	Operator string
	Value    float64
}

type serviceChange struct {
	ServiceID   int64
	ServiceName string
	DisplayName string
	OldStatus   string
	NewStatus   string
	ChangedAt   time.Time
}

// findRTCondition extracts response time condition if present
func findRTCondition(conds []ruleCondition) (rtCond, bool) {
	log.Printf("🔍 findRTCondition: Checking %d conditions", len(conds))
	for i, c := range conds {
		log.Printf("🔍 findRTCondition: Condition %d - Field='%s', Operator='%s', Value=%v (type=%T)", i, c.Field, c.Operator, c.Value, c.Value)
		if strings.EqualFold(c.Field, "response_time_ms") || strings.EqualFold(c.Field, "rtt_ms") {
			// coerce numeric
			val := 0
			switch v := c.Value.(type) {
			case float64:
				val = int(v)
			case string:
				fmt.Sscanf(v, "%d", &val)
			}
			log.Printf("✅ findRTCondition: FOUND RT condition! Operator=%s, Value=%d", c.Operator, val)
			return rtCond{Operator: c.Operator, Value: val}, true
		}
	}
	log.Printf("❌ findRTCondition: NO RT condition found")
	return rtCond{}, false
}

func findMetricCondition(conds []ruleCondition) (metricCond, bool) {
	for _, c := range conds {
		field := strings.ToLower(strings.TrimSpace(c.Field))
		if field != "cpu_percent" && field != "ram_gb" && field != "disk_gb" && field != "temperature_c" {
			continue
		}
		val := 0.0
		switch v := c.Value.(type) {
		case float64:
			val = v
		case string:
			fmt.Sscanf(v, "%f", &val)
		}
		return metricCond{Field: field, Operator: c.Operator, Value: val}, true
	}
	return metricCond{}, false
}

func findServiceSelection(conds []ruleCondition) (string, []int, bool) {
	mode := ""
	ids := []int{}
	found := false

	for _, c := range conds {
		field := strings.ToLower(strings.TrimSpace(c.Field))
		switch field {
		case "service_mode", "service_filter":
			mode = fmt.Sprintf("%v", c.Value)
			found = true
		case "service_ids":
			ids = append(ids, parseIDsFromValue(c.Value)...)
			found = true
		case "services":
			if m, ok := c.Value.(map[string]interface{}); ok {
				if v, ok := m["mode"]; ok {
					mode = fmt.Sprintf("%v", v)
					found = true
				}
				if v, ok := m["ids"]; ok {
					ids = append(ids, parseIDsFromValue(v)...)
					found = true
				}
			}
		}
	}
	if mode == "" && len(ids) > 0 {
		mode = "selected"
	}
	return strings.ToLower(strings.TrimSpace(mode)), ids, found
}

func metricUnit(field string) string {
	switch strings.ToLower(strings.TrimSpace(field)) {
	case "cpu_percent":
		return "%"
	case "ram_gb", "disk_gb":
		return "GB"
	case "temperature_c":
		return "°C"
	default:
		return ""
	}
}

func metricLabel(field string) string {
	switch strings.ToLower(strings.TrimSpace(field)) {
	case "cpu_percent":
		return "CPU (%)"
	case "ram_gb":
		return "RAM (GB)"
	case "disk_gb":
		return "Disk (GB)"
	case "temperature_c":
		return "Sıcaklık (°C)"
	default:
		return field
	}
}

func metricValueFromMetrics(metrics services.DeviceMetrics, field string) (float64, bool) {
	switch strings.ToLower(strings.TrimSpace(field)) {
	case "cpu_percent":
		if metrics.CPUPercent == nil {
			return 0, false
		}
		return *metrics.CPUPercent, true
	case "ram_gb":
		if metrics.RAMUsedMB == nil {
			return 0, false
		}
		return float64(*metrics.RAMUsedMB) / 1024.0, true
	case "disk_gb":
		if metrics.DiskUsedGB == nil {
			return 0, false
		}
		return float64(*metrics.DiskUsedGB), true
	case "temperature_c":
		if metrics.TemperatureC == nil {
			return 0, false
		}
		return *metrics.TemperatureC, true
	}
	return 0, false
}

func meetsMetricCondition(value float64, op string, threshold float64) bool {
	switch op {
	case "<":
		return value < threshold
	case "<=":
		return value <= threshold
	case ">":
		return value > threshold
	case ">=":
		return value >= threshold
	case "=", "==":
		return value == threshold
	case "!=", "<>":
		return value != threshold
	default:
		return false
	}
}

func mapToRuleCondition(m map[string]interface{}) ruleCondition {
	cond := ruleCondition{}
	if v, ok := m["field"].(string); ok {
		cond.Field = v
	} else if v, ok := m["Field"].(string); ok {
		cond.Field = v
	}
	if v, ok := m["operator"].(string); ok {
		cond.Operator = v
	} else if v, ok := m["Operator"].(string); ok {
		cond.Operator = v
	}
	if val, ok := m["value"]; ok {
		cond.Value = val
	} else if val, ok := m["Value"]; ok {
		cond.Value = val
	}
	return cond
}

func parseRuleConditions(raw []byte) []ruleCondition {
	log.Printf("🔍 parseRuleConditions: Input raw=%s", string(raw))
	if len(raw) == 0 {
		log.Printf("🔍 parseRuleConditions: Empty raw, returning nil")
		return nil
	}
	var arr []ruleCondition
	if err := json.Unmarshal(raw, &arr); err == nil && len(arr) > 0 {
		log.Printf("✅ parseRuleConditions: Parsed as array, count=%d", len(arr))
		for i, c := range arr {
			log.Printf("   - Condition %d: Field='%s', Operator='%s', Value=%v", i, c.Field, c.Operator, c.Value)
		}
		return arr
	}
	var single ruleCondition
	if err := json.Unmarshal(raw, &single); err == nil && single.Field != "" {
		log.Printf("✅ parseRuleConditions: Parsed as single condition: Field='%s'", single.Field)
		return []ruleCondition{single}
	}
	var generic map[string]interface{}
	if err := json.Unmarshal(raw, &generic); err == nil {
		cond := mapToRuleCondition(generic)
		if cond.Field != "" {
			log.Printf("✅ parseRuleConditions: Parsed as generic map: Field='%s'", cond.Field)
			return []ruleCondition{cond}
		}
	}
	log.Printf("❌ parseRuleConditions: Failed to parse, returning nil")
	return nil
}

// timeWindow returns a standard 1-minute evaluation window
func (r *SimpleRuleRunner) timeWindow(window time.Duration) (time.Time, time.Time, bool) {
	if window <= 0 {
		window = time.Minute
	}
	winStart := time.Now().Add(-window)
	winEnd := time.Now()
	return winStart, winEnd, true
}

// meetsResponseTime checks last successful rtt against operator/value
func (r *SimpleRuleRunner) meetsResponseTime(targetID int, operator string, value int, window time.Duration) (bool, int) {
	mins := windowMinutes(window)
	query := fmt.Sprintf(`
        SELECT rtt_ms FROM pings_raw
        WHERE target_id = ? AND ok = 1 AND rtt_ms IS NOT NULL AND ts_ms > UNIX_TIMESTAMP(NOW() - INTERVAL %d MINUTE)*1000
        ORDER BY ts_ms DESC LIMIT 1
    `, mins)
	var rtt sql.NullInt32
	err := r.db.QueryRow(query, targetID).Scan(&rtt)
	if err != nil || !rtt.Valid {
		return false, 0
	}
	rt := int(rtt.Int32)
	switch operator {
	case "<":
		return rt < value, rt
	case "<=":
		return rt <= value, rt
	case ">":
		return rt > value, rt
	case ">=":
		return rt >= value, rt
	case "=", "==":
		return rt == value, rt
	case "!=", "<>":
		return rt != value, rt
	default:
		return false, rt
	}
}

// targetsWithResponseTime returns all target ids meeting the RT condition in last minute
func (r *SimpleRuleRunner) targetsWithResponseTime(operator string, value int, window time.Duration) ([]int, error) {
	// guard operator
	allowed := map[string]string{"<": "<", "<=": "<=", ">": ">", ">=": ">=", "=": "=", "==": "=", "!=": "!=", "<>": "<>"}
	op, ok := allowed[operator]
	if !ok {
		return []int{}, nil
	}
	mins := windowMinutes(window)
	query := fmt.Sprintf(`
        SELECT DISTINCT target_id FROM pings_raw
        WHERE ok = 1 AND rtt_ms IS NOT NULL AND ts_ms > UNIX_TIMESTAMP(NOW() - INTERVAL %d MINUTE)*1000 AND rtt_ms %s ?
    `, mins, op)
	rows, err := r.db.Query(query, value)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := []int{}
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err == nil {
			ids = append(ids, id)
		}
	}
	return ids, nil
}

func (r *SimpleRuleRunner) loadServiceChanges(targetID int, since time.Time, mode string, ids []int) ([]serviceChange, error) {
	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode == "selected" && len(ids) == 0 {
		return nil, nil
	}

	query := `
        SELECT h.service_id, h.service_name, h.old_status, h.new_status, h.changed_at,
               COALESCE(ds.display_name, '')
        FROM device_service_history h
        LEFT JOIN device_services ds ON ds.id = h.service_id
        WHERE h.target_id = ? AND h.changed_at > ?
    `
	args := []interface{}{targetID, since}

	if mode == "monitored" {
		query += " AND ds.is_monitored = 1"
	}
	if mode == "selected" {
		placeholders := make([]string, 0, len(ids))
		for _, id := range ids {
			if id <= 0 {
				continue
			}
			placeholders = append(placeholders, "?")
			args = append(args, id)
		}
		if len(placeholders) == 0 {
			return nil, nil
		}
		query += " AND h.service_id IN (" + strings.Join(placeholders, ",") + ")"
	}
	query += " ORDER BY h.changed_at ASC"

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	changes := []serviceChange{}
	for rows.Next() {
		var (
			serviceID   int64
			serviceName string
			oldStatus   sql.NullString
			newStatus   string
			changedAt   time.Time
			displayName string
		)
		if err := rows.Scan(&serviceID, &serviceName, &oldStatus, &newStatus, &changedAt, &displayName); err != nil {
			continue
		}
		changes = append(changes, serviceChange{
			ServiceID:   serviceID,
			ServiceName: serviceName,
			DisplayName: displayName,
			OldStatus:   oldStatus.String,
			NewStatus:   newStatus,
			ChangedAt:   changedAt,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return changes, nil
}

// messageContext used to render dynamic messages
type messageContext struct {
	kind            string
	op              string
	val             int
	measuredRT      *int
	status          string
	monitoringType  string // ping, http, https
	statusCode      int    // HTTP status code (for http/https)
	lastSuccess     bool   // Last ping result
	serviceChanges  []serviceChange
	metricKind      string
	metricOp        string
	metricThreshold float64
	metricValue     float64
	metricUnit      string
	cpuPercent      *float64
	ramGB           *float64
	diskGB          *float64
	temperatureC    *float64
	offlineFor      *time.Duration // backoff modu: şu ana dek çevrimdışı kalınan süre
	recoveredAfter  *time.Duration // backoff modu: kurtarma — toplam kesinti süresi
}

func (mc *messageContext) measuredRTValue() int {
	if mc == nil || mc.measuredRT == nil {
		return 0
	}
	return *mc.measuredRT
}

type messagePayload struct {
	Subject    string
	Body       string
	Text       string
	TemplateID *int
}

// sendForTargetWithContext sends with dynamic message based on context
func (r *SimpleRuleRunner) sendForTargetWithContext(rr newRuleRow, targetID int, _, _ time.Time, mc *messageContext) error {
	name, addr, monitoringType, statusCode, lastSuccess := r.loadTargetWithStatus(targetID)
	mc.monitoringType = monitoringType
	mc.statusCode = statusCode
	mc.lastSuccess = lastSuccess

	// Check for status change ONLY if mc.kind is NOT "rt" (response time)
	// Eğer response_time condition'ı varsa, status change detection'ı atlayalım
	if mc.kind == "" && rr.LastSentStatus != nil {
		prevSuccess := (*rr.LastSentStatus == "success")
		if prevSuccess != lastSuccess {
			// Status changed!
			if !prevSuccess && lastSuccess {
				// Changed from FAIL to SUCCESS (offline→online)
				mc.kind = "status"
				mc.status = "success"
				log.Printf("✅ Status change detected for target %d: offline→online", targetID)
			} else if prevSuccess && !lastSuccess {
				// Changed from SUCCESS to FAIL (online→offline)
				mc.kind = "status"
				mc.status = "fail"
				log.Printf("⚠️ Status change detected for target %d: online→offline", targetID)
			}
		}
	}

	// Template desteği: buildNotificationPayload kullan
	payload := r.buildNotificationPayload(rr, targetID, name, addr, mc)

	// Akıllı (azalan) sıklık modu: süre bilgisini mesajın sonuna ekle (şablondan bağımsız)
	if mc.offlineFor != nil {
		line := "\nÇevrimdışı süresi: " + formatDurationTR(*mc.offlineFor)
		payload.Body += line
		payload.Text += line
	}
	if mc.recoveredAfter != nil {
		line := "\nToplam çevrimdışı kalınan süre: " + formatDurationTR(*mc.recoveredAfter)
		payload.Body += line
		payload.Text += line
	}
	return r.dispatchChannel(rr, targetID, payload)
}

func (r *SimpleRuleRunner) sendForTarget(rr newRuleRow, targetID int, _, _ time.Time) error {
	name, addr, monitoringType, statusCode, lastSuccess := r.loadTargetWithStatus(targetID)
	mc := &messageContext{
		monitoringType: monitoringType,
		statusCode:     statusCode,
		lastSuccess:    lastSuccess,
	}

	// Check for status change (offline→online or online→offline)
	// ONLY if not a response_time condition (mc.kind would be set by caller if RT)
	if mc.kind == "" && rr.LastSentStatus != nil {
		prevSuccess := (*rr.LastSentStatus == "success")
		if prevSuccess != lastSuccess {
			// Status changed!
			if !prevSuccess && lastSuccess {
				// Changed from FAIL to SUCCESS (offline→online)
				mc.kind = "status"
				mc.status = "success"
				log.Printf("✅ Status change detected for target %d: offline→online", targetID)
			} else if prevSuccess && !lastSuccess {
				// Changed from SUCCESS to FAIL (online→offline)
				mc.kind = "status"
				mc.status = "fail"
				log.Printf("⚠️ Status change detected for target %d: online→offline", targetID)
			}
		}
	}

	payload := r.buildNotificationPayload(rr, targetID, name, addr, mc)
	return r.dispatchChannel(rr, targetID, payload)
}

func (r *SimpleRuleRunner) sendServiceNotification(rr newRuleRow, targetID int, changes []serviceChange) error {
	name, addr, _, _, _ := r.loadTargetWithStatus(targetID)
	mc := &messageContext{
		kind:           "service",
		serviceChanges: changes,
	}
	payload := r.buildNotificationPayload(rr, targetID, name, addr, mc)
	return r.dispatchChannel(rr, targetID, payload)
}

func (r *SimpleRuleRunner) sendBulkRule(rr newRuleRow) error {
	if len(rr.TargetIDs) == 0 {
		return fmt.Errorf("bulk rule %d has no targets", rr.ID)
	}

	// Önce condition'ları kontrol et - response_time var mı?
	_, hasStatus := findStatusCondition(rr.Conditions)
	condRT, hasRT := findRTCondition(rr.Conditions)
	log.Printf("🔍 sendBulkRule: Rule %d - hasStatus=%v, hasRT=%v", rr.ID, hasStatus, hasRT)

	sections := make([]messagePayload, 0, len(rr.TargetIDs))
	skippedOnline := 0
	for _, targetID := range rr.TargetIDs {
		name, addr, monitoringType, statusCode, lastSuccess := r.loadTargetWithStatus(targetID)

		// Status bazlı bulk bildirimlerde online hedefleri atla
		if !hasRT && lastSuccess {
			log.Printf("⏭️ sendBulkRule: Target %d (%s) is ONLINE - Skipping from bulk notification", targetID, name)
			skippedOnline++
			continue
		}

		mc := &messageContext{
			monitoringType: monitoringType,
			statusCode:     statusCode,
			lastSuccess:    lastSuccess,
		}

		// Response time condition varsa, mc.kind'ı set et
		if hasRT {
			// Response time için gerekli değerleri kontrol et
			window := ruleWindow(&rr)
			if ok, lastRT := r.meetsResponseTime(targetID, condRT.Operator, condRT.Value, window); ok {
				mc.kind = "rt"
				mc.op = condRT.Operator
				mc.val = condRT.Value
				mc.measuredRT = &lastRT
				log.Printf("✅ sendBulkRule: Target %d meets RT condition (measured=%d ms)", targetID, lastRT)
			} else {
				// RT condition karşılanmıyor, bu target'ı atla
				log.Printf("⚠️ sendBulkRule: Target %d does NOT meet RT condition (measured=%d ms)", targetID, lastRT)
				continue
			}
		}

		payload := r.buildNotificationPayload(rr, targetID, name, addr, mc)
		log.Printf("📦 sendBulkRule: Payload for target %d - Subject='%s', Body length=%d",
			targetID, payload.Subject, len(payload.Body))
		if len(payload.Body) > 0 {
			preview := payload.Body
			if len(preview) > 150 {
				preview = preview[:150] + "..."
			}
			log.Printf("   Payload body preview: %s", preview)
		} else {
			log.Printf("   ⚠️ WARNING: Payload body is EMPTY for target %d!", targetID)
		}
		sections = append(sections, payload)
	}

	if len(sections) == 0 {
		return fmt.Errorf("bulk rule %d resolved to zero targets", rr.ID)
	}

	if skippedOnline > 0 {
		log.Printf("📊 sendBulkRule: Skipped %d online targets from bulk notification", skippedOnline)
	}

	subject := fmt.Sprintf("[SysTrack] %s - %d hedef", rr.Name, len(sections))
	var bodyBuilder strings.Builder
	var textBuilder strings.Builder

	for i, section := range sections {
		if i > 0 {
			bodyBuilder.WriteString("\n\n----------------------------------------\n\n")
			textBuilder.WriteString("\n\n----------------------------------------\n\n")
		}
		bodyBuilder.WriteString(section.Body)
		textBuilder.WriteString(section.Text)
	}

	payload := messagePayload{
		Subject: subject,
		Body:    bodyBuilder.String(),
		Text:    textBuilder.String(),
	}

	// Log payload details to debug email delivery issue
	log.Printf("📧 sendBulkRule: Final payload for rule %d", rr.ID)
	log.Printf("   Subject: '%s'", subject)
	log.Printf("   Body length: %d chars", len(payload.Body))
	log.Printf("   Text length: %d chars", len(payload.Text))
	log.Printf("   Sections count: %d", len(sections))
	if len(payload.Body) > 0 {
		preview := payload.Body
		if len(preview) > 300 {
			preview = preview[:300] + "..."
		}
		log.Printf("   Body preview: %s", preview)
	} else {
		log.Printf("   ⚠️ WARNING: Body is EMPTY!")
	}

	return r.dispatchChannel(rr, rr.TargetIDs[0], payload)
}

func (r *SimpleRuleRunner) buildNotificationPayload(rr newRuleRow, targetID int, name, addr string, mc *messageContext) messagePayload {
	base := composeMessage(rr.Channel, name, addr, mc)
	if rr.TemplateID == nil {
		return base
	}
	payload, err := r.applyTemplatePayload(*rr.TemplateID, rr, targetID, name, addr, mc, base)
	if err != nil {
		log.Printf("template render failed for rule %d (template %d): %v", rr.ID, *rr.TemplateID, err)
		return base
	}
	return payload
}

func (r *SimpleRuleRunner) applyTemplatePayload(templateID int, rr newRuleRow, targetID int, name, addr string, mc *messageContext, base messagePayload) (messagePayload, error) {
	tpl, err := r.loadNotificationTemplate(templateID)
	if err != nil {
		return messagePayload{}, err
	}
	if !tpl.IsActive {
		return messagePayload{}, fmt.Errorf("template %d is inactive", templateID)
	}
	// Kanal kontrolü kaldırıldı - kanal seçimi sadece görsel etiket amaçlı
	// Tüm şablonlar tüm kanallar için kullanılabilir

	ctx := r.buildTemplateContext(rr, targetID, name, addr, mc)

	// Template'i render et
	renderedBody := strings.TrimSpace(renderTemplateString(tpl.Body, ctx))
	if renderedBody == "" {
		renderedBody = base.Body
	}

	renderedSubject := base.Subject
	if tpl.Subject != nil && strings.TrimSpace(*tpl.Subject) != "" {
		renderedSubjectCandidate := strings.TrimSpace(renderTemplateString(*tpl.Subject, ctx))
		if renderedSubjectCandidate != "" {
			renderedSubject = renderedSubjectCandidate
		}
	}

	// Kullanılmayan önemli bilgileri mesajın altına ekle
	fullTemplate := renderedBody
	if tpl.Subject != nil {
		fullTemplate = *tpl.Subject + " " + tpl.Body
	} else {
		fullTemplate = tpl.Body
	}

	additionalInfo := r.buildAdditionalInfo(fullTemplate, ctx)
	if additionalInfo != "" {
		renderedBody = renderedBody + "\n\n" + additionalInfo
	}

	text := renderedBody
	if renderedSubject != "" {
		text = fmt.Sprintf("%s\n\n%s", renderedSubject, renderedBody)
	}

	payload := messagePayload{
		Subject:    renderedSubject,
		Body:       renderedBody,
		Text:       text,
		TemplateID: &templateID,
	}
	return payload, nil
}

// buildAdditionalInfo kullanılmayan önemli değişkenleri "Ek Bilgiler" olarak ekler
func (r *SimpleRuleRunner) buildAdditionalInfo(template string, ctx map[string]string) string {
	// Temel önemli değişkenler (her zaman kontrol edilecekler)
	importantVars := map[string]string{
		"hedef_adi":    "Hedef",
		"hedef_adresi": "Adres",
		"izleme_tipi":  "İzleme",
		"tarih_saat":   "Zaman",
		"kural_adi":    "Kural",
		"alicilar":     "Alıcılar",
	}

	if servisList, ok := ctx["servis_listesi"]; ok && strings.TrimSpace(servisList) != "" {
		importantVars["servis_listesi"] = "Servisler"
	}
	if servisAdi, ok := ctx["servis_adi"]; ok && strings.TrimSpace(servisAdi) != "" {
		importantVars["servis_adi"] = "Servis"
		importantVars["servis_yeni_durum"] = "Yeni Durum"
	}
	if metrikDeger, ok := ctx["metrik_degeri"]; ok && strings.TrimSpace(metrikDeger) != "" {
		importantVars["metrik_turu"] = "Metrik"
		importantVars["metrik_degeri"] = "Değer"
		importantVars["metrik_kosul"] = "Koşul"
	}

	// Yanıt süresi bilgisi varsa, yanıt süresini ekle; durum ekleme
	if yanitSuresi, ok := ctx["yanit_suresi"]; ok && yanitSuresi != "" {
		importantVars["yanit_suresi"] = "Yanıt Süresi"
		if kosul, ok := ctx["kosul"]; ok && kosul != "" {
			importantVars["kosul"] = "Koşul"
		}
	} else {
		// Yanıt süresi yoksa, durumu ekle
		importantVars["durum"] = "Durum"
	}

	var unused []string
	for key, label := range importantVars {
		// Şablonda kullanılmamış mı?
		if !strings.Contains(template, "{"+key+"}") {
			// Ve değeri var mı?
			if val, ok := ctx[key]; ok && val != "" {
				unused = append(unused, fmt.Sprintf("%s: %s", label, val))
			}
		}
	}

	if len(unused) == 0 {
		return ""
	}

	return "━━━━━━━━━━━━━━━━━━━━━━━━\nEk Bilgiler:\n" + strings.Join(unused, "\n")
}

func (r *SimpleRuleRunner) loadNotificationTemplate(id int) (*notificationTemplate, error) {
	row := r.db.QueryRow(`
        SELECT id, name, channel, subject, body, variables, is_default, is_active, created_at, updated_at
        FROM new_notification_templates
        WHERE id = ?
    `, id)
	var tpl notificationTemplate
	var subject sql.NullString
	var variables sql.NullString
	if err := row.Scan(&tpl.ID, &tpl.Name, &tpl.Channel, &subject, &tpl.Body, &variables, &tpl.IsDefault, &tpl.IsActive, &tpl.CreatedAt, &tpl.UpdatedAt); err != nil {
		return nil, err
	}
	if subject.Valid && strings.TrimSpace(subject.String) != "" {
		str := subject.String
		tpl.Subject = &str
	}
	if variables.Valid && strings.TrimSpace(variables.String) != "" && strings.TrimSpace(variables.String) != "null" {
		var parsed []string
		if err := json.Unmarshal([]byte(variables.String), &parsed); err == nil {
			tpl.Variables = parsed
		}
	}
	return &tpl, nil
}

func (r *SimpleRuleRunner) buildTemplateContext(rr newRuleRow, targetID int, name, addr string, mc *messageContext) map[string]string {
	now := time.Now()

	// Status bilgisi
	_, statusText := deriveStatusInfo(mc)
	if mc != nil && mc.kind == "service" {
		statusText = "Servis durumu değişti"
	}
	if mc != nil && mc.kind == "metric" && mc.metricKind != "" {
		statusText = fmt.Sprintf("%s koşulu sağlandı", metricLabel(mc.metricKind))
	}

	// İzleme tipi metni (Türkçe)
	monitoringTypeText := "ICMP Ping"
	if mc != nil && mc.monitoringType != "" {
		switch strings.ToLower(mc.monitoringType) {
		case "http":
			monitoringTypeText = "HTTP İzleme"
		case "https":
			monitoringTypeText = "HTTPS İzleme"
		case "ping":
			monitoringTypeText = "ICMP Ping"
		default:
			monitoringTypeText = mc.monitoringType
		}
	}

	// Yanıt süresi metni
	yanitSuresi := ""
	if mc != nil && mc.measuredRT != nil {
		yanitSuresi = fmt.Sprintf("%d ms", *mc.measuredRT)
	}

	// Koşul açıklaması
	kosul := ""
	if mc != nil && mc.kind == "rt" && mc.op != "" {
		kosul = fmt.Sprintf("Yanıt süresi %s %d ms", mc.op, mc.val)
	}
	if mc != nil && mc.kind == "metric" && mc.metricKind != "" {
		label := metricLabel(mc.metricKind)
		if mc.metricUnit != "" {
			kosul = fmt.Sprintf("%s %s %.2f %s", label, mc.metricOp, mc.metricThreshold, mc.metricUnit)
		} else {
			kosul = fmt.Sprintf("%s %s %.2f", label, mc.metricOp, mc.metricThreshold)
		}
	}

	// Yeni basitleştirilmiş değişkenler (Türkçe key'ler)
	ctx := map[string]string{
		// Temel bilgiler
		"hedef_adi":    name,
		"hedef_adresi": addr,
		"durum":        statusText,
		"izleme_tipi":  monitoringTypeText,
		"tarih_saat":   now.Format("02.01.2006 15:04:05"),

		// Kural & Alıcı
		"kural_adi": rr.Name,
		"alicilar":  strings.Join(rr.Recipients, ", "),

		// Yanıt süresi (opsiyonel)
		"yanit_suresi": yanitSuresi,
		"kosul":        kosul,

		// Geriye dönük uyumluluk için eski key'leri de tut
		"target_name":     name,
		"target_address":  addr,
		"status_text":     statusText,
		"monitoring_type": monitoringTypeText,
		"timestamp":       now.Format("2006-01-02 15:04:05"),
		"rule_name":       rr.Name,

		// Yanıt süresi detayları
		"recipients": strings.Join(rr.Recipients, ", "),
	}

	// Yanıt süresi detayları
	if mc != nil {
		if mc.cpuPercent != nil {
			ctx["cpu_yuzde"] = fmt.Sprintf("%.2f", *mc.cpuPercent)
		}
		if mc.ramGB != nil {
			ctx["ram_gb"] = fmt.Sprintf("%.2f", *mc.ramGB)
		}
		if mc.diskGB != nil {
			ctx["disk_gb"] = fmt.Sprintf("%.2f", *mc.diskGB)
		}
		if mc.temperatureC != nil {
			ctx["sicaklik_c"] = fmt.Sprintf("%.2f", *mc.temperatureC)
		}
		if mc.metricKind != "" {
			label := metricLabel(mc.metricKind)
			unit := mc.metricUnit
			ctx["metrik_turu"] = label
			if unit != "" {
				ctx["metrik_degeri"] = fmt.Sprintf("%.2f %s", mc.metricValue, unit)
				ctx["metrik_esik"] = fmt.Sprintf("%.2f %s", mc.metricThreshold, unit)
			} else {
				ctx["metrik_degeri"] = fmt.Sprintf("%.2f", mc.metricValue)
				ctx["metrik_esik"] = fmt.Sprintf("%.2f", mc.metricThreshold)
			}
			ctx["metrik_birim"] = unit
			ctx["metrik_kosul"] = kosul
		}
	}

	if mc != nil && mc.measuredRT != nil {
		ctx["response_time_ms"] = strconv.Itoa(*mc.measuredRT)
	}

	if mc != nil && len(mc.serviceChanges) > 0 {
		first := mc.serviceChanges[0]
		serviceLabel := first.ServiceName
		if strings.TrimSpace(first.DisplayName) != "" {
			serviceLabel = first.DisplayName
		}
		oldStatus := strings.TrimSpace(first.OldStatus)
		if oldStatus == "" {
			oldStatus = "unknown"
		}
		ctx["servis_adi"] = serviceLabel
		ctx["servis_kodu"] = first.ServiceName
		ctx["servis_onceki_durum"] = oldStatus
		ctx["servis_yeni_durum"] = first.NewStatus
		ctx["servis_degisim_zamani"] = first.ChangedAt.Format("02.01.2006 15:04:05")
		ctx["servis_listesi"] = formatServiceChangeList(mc.serviceChanges)
		ctx["servis_sayisi"] = strconv.Itoa(len(mc.serviceChanges))
	}

	return ctx
}

var templateTokenPattern = regexp.MustCompile(`\{([a-zA-Z0-9_]+)\}`)

func renderTemplateString(input string, ctx map[string]string) string {
	if input == "" || len(ctx) == 0 {
		return input
	}
	return templateTokenPattern.ReplaceAllStringFunc(input, func(token string) string {
		key := strings.ToLower(strings.Trim(token, "{}"))
		if val, ok := ctx[key]; ok {
			return val
		}
		return token
	})
}

func deriveStatusInfo(mc *messageContext) (string, string) {
	if mc == nil {
		return "unknown", ""
	}
	if mc.kind == "rt" {
		return "rt", fmt.Sprintf("Yanıt süresi: %d ms", mc.measuredRTValue())
	}
	status := "unknown"
	if mc.kind == "status" && mc.status != "" {
		status = strings.ToLower(mc.status)
	} else if mc.lastSuccess {
		status = "success"
	} else {
		status = "fail"
	}

	var statusText string
	if status == "success" {
		if mc.monitoringType == "http" || mc.monitoringType == "https" {
			statusText = fmt.Sprintf("Durum kodu %d - %s", mc.statusCode, httpStatusCategory(mc.statusCode))
		} else {
			statusText = "Cihaz online"
		}
	} else if status == "fail" {
		if mc.monitoringType == "http" || mc.monitoringType == "https" {
			statusText = fmt.Sprintf("Erişim hatası (kod %d - %s)", mc.statusCode, httpStatusCategory(mc.statusCode))
		} else {
			statusText = "Cihaz offline"
		}
	}
	return status, statusText
}

func httpStatusCategory(code int) string {
	switch {
	case code >= 200 && code < 300:
		return "Başarılı"
	case code >= 300 && code < 400:
		return "Yönlendirme"
	case code >= 400 && code < 500:
		return "İstemci Hatası"
	case code >= 500:
		return "Sunucu Hatası"
	default:
		return "Bilinmeyen"
	}
}

// loadTargetInfo fetches target information including monitoring type
func (r *SimpleRuleRunner) loadTargetInfo(targetID int) (name, addr, monitoringType string) {
	_ = r.db.QueryRow("SELECT name, address, monitoring_type FROM targets WHERE id = ?", targetID).Scan(&name, &addr, &monitoringType)
	return name, addr, monitoringType
}

// loadTargetWithStatus fetches target info along with last ping status and status code
func (r *SimpleRuleRunner) loadTargetWithStatus(targetID int) (name, addr, monitoringType string, lastStatusCode int, lastSuccess bool) {
	// Get target basic info
	_ = r.db.QueryRow("SELECT name, address, monitoring_type FROM targets WHERE id = ?", targetID).Scan(&name, &addr, &monitoringType)

	// Get last ping result
	_ = r.db.QueryRow(`
		SELECT ok, COALESCE(response_status_code, 0)
		FROM pings_raw
		WHERE target_id = ?
		ORDER BY ts_ms DESC
		LIMIT 1
	`, targetID).Scan(&lastSuccess, &lastStatusCode)

	return
}

func (r *SimpleRuleRunner) loadMetricsTarget(targetID int) (services.Target, bool) {
	query := `
		SELECT t.id, t.name, t.address, t.metrics_enabled,
		       COALESCE(t.snmp_community, 'public') as snmp_community,
		       COALESCE(t.snmp_version, 'v2c') as snmp_version
		FROM targets t
		WHERE t.enabled = 1
		  AND t.id = ?
		  AND (
			t.metrics_enabled = 1
			OR EXISTS (
				SELECT 1 FROM target_credentials tc WHERE tc.target_id = t.id
			)
		  )
	`
	var t services.Target
	if err := r.db.QueryRow(query, targetID).Scan(&t.ID, &t.Name, &t.Address, &t.MetricsEnabled, &t.SNMPCommunity, &t.SNMPVersion); err != nil {
		return services.Target{}, false
	}
	return t, true
}

func formatServiceChangeList(changes []serviceChange) string {
	if len(changes) == 0 {
		return "-"
	}
	lines := make([]string, 0, len(changes))
	for _, c := range changes {
		label := c.ServiceName
		if strings.TrimSpace(c.DisplayName) != "" {
			label = c.DisplayName
		}
		oldStatus := c.OldStatus
		if strings.TrimSpace(oldStatus) == "" {
			oldStatus = "unknown"
		}
		lines = append(lines, fmt.Sprintf("- %s (%s -> %s)", label, oldStatus, c.NewStatus))
	}
	return strings.Join(lines, "\n")
}

func composeMessage(channel, name, addr string, mc *messageContext) messagePayload {
	ts := time.Now().Format("2006-01-02 15:04:05")
	var subject, body string

	if mc != nil && mc.kind == "service" {
		subject = fmt.Sprintf("[SysTrack] %s - Servis Durum Değişimi", name)
		body = fmt.Sprintf(`Hedef: %s (%s)
Değişen Servisler:
%s
Zaman: %s`,
			name, addr, formatServiceChangeList(mc.serviceChanges), ts)
		text := fmt.Sprintf("%s\n\n%s", subject, body)
		return messagePayload{Subject: subject, Body: body, Text: text}
	}

	if mc != nil && mc.kind == "metric" {
		label := metricLabel(mc.metricKind)
		unit := mc.metricUnit
		threshold := fmt.Sprintf("%.2f", mc.metricThreshold)
		measured := fmt.Sprintf("%.2f", mc.metricValue)
		if unit != "" {
			threshold = fmt.Sprintf("%.2f %s", mc.metricThreshold, unit)
			measured = fmt.Sprintf("%.2f %s", mc.metricValue, unit)
		}
		subject = fmt.Sprintf("[SysTrack] %s - %s koşulu", name, label)
		body = fmt.Sprintf(`Hedef: %s (%s)
Koşul: %s %s %s
Gerçek: %s
Zaman: %s`,
			name, addr, label, mc.metricOp, threshold, measured, ts)
		text := fmt.Sprintf("%s\n\n%s", subject, body)
		return messagePayload{Subject: subject, Body: body, Text: text}
	}

	if mc != nil && mc.kind == "rt" {
		// Response Time notification
		opText := mc.op
		measured := "-"
		if mc.measuredRT != nil {
			measured = fmt.Sprintf("%d ms", *mc.measuredRT)
		}
		subject = fmt.Sprintf("[SysTrack] %s yanıt süresi koşulu", name)
		body = fmt.Sprintf(`Hedef: %s (%s)
Koşul: Yanıt Süresi (ms) %s %d
Gerçek: %s
Zaman: %s`,
			name, addr, opText, mc.val, measured, ts)
	} else if mc != nil && mc.kind == "status" {
		// Status change notification
		if strings.EqualFold(mc.status, "success") {
			// Device came back online
			if mc.monitoringType == "http" || mc.monitoringType == "https" {
				statusText := httpStatusCategory(mc.statusCode)
				subject = fmt.Sprintf("[SysTrack] %s - Cihaz Online", name)
				body = fmt.Sprintf(`Hedef: %s (%s)
Durum: Cihaz online
Status Code: %d - %s
Zaman: %s`,
					name, addr, mc.statusCode, statusText, ts)
			} else {
				subject = fmt.Sprintf("[SysTrack] %s - Cihaz Online", name)
				body = fmt.Sprintf(`Hedef: %s (%s)
Durum: Cihaz online
Zaman: %s`,
					name, addr, ts)
			}
		} else {
			// Device is offline or failed
			if mc.monitoringType == "http" || mc.monitoringType == "https" {
				statusText := httpStatusCategory(mc.statusCode)
				subject = fmt.Sprintf("[SysTrack] %s - Erişim Hatası", name)
				body = fmt.Sprintf(`Hedef: %s (%s)
Durum: Erişim hatası
Status Code: %d - %s
Zaman: %s`,
					name, addr, mc.statusCode, statusText, ts)
			} else {
				subject = fmt.Sprintf("[SysTrack] %s - Cihaz Offline", name)
				body = fmt.Sprintf(`Hedef: %s (%s)
Durum: Cihaz offline (Hatalı ping)
Zaman: %s`,
					name, addr, ts)
			}
		}
	} else {
		// Default/fallback notification
		if mc != nil && (mc.monitoringType == "http" || mc.monitoringType == "https") {
			// HTTP/HTTPS monitoring
			if mc.lastSuccess {
				statusText := httpStatusCategory(mc.statusCode)
				subject = fmt.Sprintf("[SysTrack] %s - Cihaz Online", name)
				body = fmt.Sprintf(`Hedef: %s (%s)
Durum: Cihaz online
Status Code: %d - %s
Zaman: %s`,
					name, addr, mc.statusCode, statusText, ts)
			} else {
				statusText := httpStatusCategory(mc.statusCode)
				subject = fmt.Sprintf("[SysTrack] %s - Cihaz Offline", name)
				body = fmt.Sprintf(`Hedef: %s (%s)
Durum: Cihaz offline
Status Code: %d - %s
Zaman: %s`,
					name, addr, mc.statusCode, statusText, ts)
			}
		} else {
			// ICMP Ping monitoring
			if mc != nil && mc.lastSuccess {
				subject = fmt.Sprintf("[SysTrack] %s - Cihaz Online", name)
				body = fmt.Sprintf(`Hedef: %s (%s)
Durum: Cihaz online
Zaman: %s`,
					name, addr, ts)
			} else {
				subject = fmt.Sprintf("[SysTrack] %s - Cihaz Offline", name)
				body = fmt.Sprintf(`Hedef: %s (%s)
Durum: Cihaz offline (Hatalı ping)
Zaman: %s`,
					name, addr, ts)
			}
		}
	}
	text := fmt.Sprintf("%s\n\n%s", subject, body)
	return messagePayload{Subject: subject, Body: body, Text: text}
}

func (r *SimpleRuleRunner) dispatchChannel(rr newRuleRow, targetID int, payload messagePayload) error {
	channel := strings.TrimSpace(strings.ToLower(rr.Channel))
	if channel == "" {
		channel = "email"
	}
	switch channel {
	case "telegram":
		cfg, err := r.loadTelegramConfig()
		if err != nil {
			return err
		}
		if !cfg.Enabled || cfg.BotToken == "" {
			return fmt.Errorf("telegram channel disabled")
		}
		svc := NewTelegramService(r.db)
		if err := svc.SetConfig(cfg); err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(r.ctx, 20*time.Second)
		defer cancel()
		telegramText := fmt.Sprintf("<pre>%s</pre>", html.EscapeString(payload.Text))
		err = r.sendTelegramMessages(ctx, svc, rr.Recipients, telegramText)
		r.logNotification(rr.ID, targetID, channel, rr.Recipients, payload.Subject, payload.Body, payload.TemplateID, err)
		return err
	case "email":
		log.Printf("📧 dispatchChannel: Sending email for rule %d, target %d", rr.ID, targetID)
		emailCfg, err := r.loadEmailConfig()
		if err != nil {
			log.Printf("❌ dispatchChannel: Failed to load email config: %v", err)
			return err
		}
		if !emailCfg.Enabled {
			log.Printf("❌ dispatchChannel: Email channel is disabled")
			return fmt.Errorf("email channel disabled")
		}
		log.Printf("✅ dispatchChannel: Email config loaded, sending to %d recipients", len(rr.Recipients))
		log.Printf("📧 dispatchChannel: Payload details:")
		log.Printf("   Subject: '%s'", payload.Subject)
		log.Printf("   Body length: %d chars", len(payload.Body))
		log.Printf("   Text length: %d chars", len(payload.Text))
		if len(payload.Body) > 0 {
			preview := payload.Body
			if len(preview) > 200 {
				preview = preview[:200] + "..."
			}
			log.Printf("   Body preview: %s", preview)
		} else {
			log.Printf("   ⚠️ WARNING: Body is EMPTY!")
		}
		logger := NewEmailLogger(r.db)
		svc := NewEmailServiceWithDB(emailCfg, logger, r.db)
		ctx, cancel := context.WithTimeout(r.ctx, 20*time.Second)
		defer cancel()
		err = svc.SendEmail(ctx, rr.Recipients, payload.Subject, payload.Body)
		if err != nil {
			log.Printf("❌ dispatchChannel: SendEmail failed: %v", err)
		} else {
			log.Printf("✅ dispatchChannel: Email sent successfully!")
		}
		r.logNotification(rr.ID, targetID, "email", rr.Recipients, payload.Subject, payload.Body, payload.TemplateID, err)
		return err
	default:
		err := fmt.Errorf("unsupported notification channel: %s", rr.Channel)
		r.logNotification(rr.ID, targetID, channel, rr.Recipients, payload.Subject, payload.Body, payload.TemplateID, err)
		return err
	}
}

func (r *SimpleRuleRunner) loadEmailConfig() (EmailConfig, error) {
	var isEnabled bool
	var cfgJSON sql.NullString
	err := r.db.QueryRow(`SELECT is_enabled, config FROM new_notification_config WHERE channel='email'`).Scan(&isEnabled, &cfgJSON)
	if err != nil {
		if err == sql.ErrNoRows {
			return EmailConfig{}, fmt.Errorf("email channel not configured")
		}
		return EmailConfig{}, err
	}
	var cfg map[string]interface{}
	if cfgJSON.Valid {
		_ = json.Unmarshal([]byte(cfgJSON.String), &cfg)
	}
	return EmailConfig{
		Enabled:   isEnabled && boolOr(cfg["enabled"], true),
		SMTPHost:  stringOr(cfg["smtp_host"]),
		SMTPPort:  intOr(cfg["smtp_port"], 587),
		Username:  stringOr(cfg["username"]),
		Password:  stringOr(cfg["password"]),
		FromEmail: stringOr(cfg["from_email"]),
		FromName:  stringOr(cfg["from_name"]),
		UseTLS:    boolOr(cfg["use_tls"], true),
	}, nil
}

func (r *SimpleRuleRunner) loadTelegramConfig() (TelegramConfig, error) {
	var isEnabled bool
	var cfgJSON sql.NullString
	err := r.db.QueryRow(`SELECT is_enabled, config FROM new_notification_config WHERE channel='telegram'`).Scan(&isEnabled, &cfgJSON)
	if err != nil {
		if err == sql.ErrNoRows {
			return TelegramConfig{}, fmt.Errorf("telegram channel not configured")
		}
		return TelegramConfig{}, err
	}
	var cfg map[string]interface{}
	if cfgJSON.Valid {
		_ = json.Unmarshal([]byte(cfgJSON.String), &cfg)
	}
	return TelegramConfig{
		Enabled:     isEnabled && boolOr(cfg["enabled"], true),
		BotToken:    stringOr(cfg["bot_token"]),
		DefaultChat: stringOr(cfg["default_chat"]),
		WebhookURL:  stringOr(cfg["webhook_url"]),
	}, nil
}

func (r *SimpleRuleRunner) sendTelegramMessages(ctx context.Context, svc TelegramService, recipients []string, message string) error {
	chats := make([]string, 0, len(recipients))
	for _, chat := range recipients {
		trimmed := strings.TrimSpace(chat)
		if trimmed != "" {
			chats = append(chats, trimmed)
		}
	}
	if len(chats) == 0 {
		return fmt.Errorf("no telegram chat_id provided")
	}
	var firstErr error
	for _, chatID := range chats {
		if err := svc.SendMessage(ctx, chatID, message); err != nil {
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

func (r *SimpleRuleRunner) logNotification(ruleID, targetID int, channel string, recipients []string, subject, body string, templateID *int, sendErr error) {
	recJSON, _ := json.Marshal(recipients)
	status := "sent"
	errText := ""
	if sendErr != nil {
		status = "failed"
		errText = sendErr.Error()
	}
	var tpl interface{}
	if templateID != nil {
		tpl = *templateID
	}
	if _, err := r.db.Exec(`
        INSERT INTO new_notification_mails (rule_id, target_id, channel, template_id, recipients, subject, body, status, error, sent_at)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, NOW())
    `, ruleID, targetID, channel, tpl, string(recJSON), subject, body, status, errText); err != nil {
		log.Printf("failed to log notification mail (rule=%d, channel=%s): %v", ruleID, channel, err)
	}
}

func stripTargetIDsFromConditions(raw []byte) ([]byte, []int) {
	log.Printf("🔍 stripTargetIDsFromConditions: Input raw=%s", string(raw))
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		log.Printf("🔍 stripTargetIDsFromConditions: Empty or null, returning as-is")
		return raw, nil
	}
	var arr []map[string]interface{}
	if err := json.Unmarshal(raw, &arr); err != nil {
		log.Printf("❌ stripTargetIDsFromConditions: Failed to unmarshal as array: %v", err)
		return raw, nil
	}
	targetIDs := make([]int, 0)
	filtered := make([]map[string]interface{}, 0, len(arr))
	for i, cond := range arr {
		field, _ := cond["field"].(string)
		log.Printf("🔍 stripTargetIDsFromConditions: Condition %d - Field='%s'", i, field)
		if strings.EqualFold(field, "target_ids") {
			targetIDs = append(targetIDs, parseIDsFromValue(cond["value"])...)
			log.Printf("   - Stripped target_ids, found IDs=%v", targetIDs)
			continue
		}
		filtered = append(filtered, cond)
		log.Printf("   - Kept condition: %+v", cond)
	}
	clean := raw
	if buf, err := json.Marshal(filtered); err == nil {
		log.Printf("✅ stripTargetIDsFromConditions: Clean=%s, TargetIDs=%v", string(buf), targetIDs)
		clean = buf
	}
	return clean, targetIDs
}

func parseIDsFromValue(val interface{}) []int {
	result := make([]int, 0)
	switch v := val.(type) {
	case []interface{}:
		for _, item := range v {
			switch t := item.(type) {
			case float64:
				result = append(result, int(t))
			case json.Number:
				if i, err := t.Int64(); err == nil {
					result = append(result, int(i))
				}
			case string:
				if i, err := strconv.Atoi(t); err == nil {
					result = append(result, i)
				}
			}
		}
	case []int:
		result = append(result, v...)
	case []float64:
		for _, item := range v {
			result = append(result, int(item))
		}
	case string:
		if strings.TrimSpace(v) == "" {
			break
		}
		var arr []int
		if err := json.Unmarshal([]byte(v), &arr); err == nil {
			result = append(result, arr...)
		}
	}
	return result
}

// reuse small helpers
func stringOr(v interface{}) string {
	if v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return t
	case float64:
		return fmt.Sprintf("%v", t)
	default:
		b, _ := json.Marshal(v)
		return string(b)
	}
}
func intOr(v interface{}, def int) int {
	if v == nil {
		return def
	}
	switch t := v.(type) {
	case float64:
		return int(t)
	case string:
		var i int
		fmt.Sscanf(t, "%d", &i)
		if i == 0 {
			return def
		}
		return i
	default:
		return def
	}
}
func boolOr(v interface{}, def bool) bool {
	if v == nil {
		return def
	}
	switch t := v.(type) {
	case bool:
		return t
	case string:
		return t == "true" || t == "1"
	default:
		return def
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// AKILLI (AZALAN) SIKLIK ("backoff") MODU
// Sabit aralığa ek, opsiyonel bir bildirim zamanlaması. Kural conditions içinde
// {"field":"schedule_mode","value":"backoff"} bayrağıyla etkinleşir.
// Kaynaklar tamamen mevcut tablolar: pings_raw (son ping + kesinti başlangıcı)
// ve new_notification_mails (hedef başına son gönderim). DB şemasına ek yoktur.
// ─────────────────────────────────────────────────────────────────────────────

// hasBackoffMode, koşullar arasında schedule_mode=backoff bayrağı olup olmadığına bakar.
func hasBackoffMode(conds []ruleCondition) bool {
	for _, c := range conds {
		if strings.EqualFold(c.Field, "schedule_mode") {
			if s, ok := c.Value.(string); ok && strings.EqualFold(s, "backoff") {
				return true
			}
		}
	}
	return false
}

// isPureStatusConditions: backoff modu yalnızca durum (online/offline) kuralları içindir.
// Servis/metrik/yanıt-süresi gibi başka koşul varsa sabit akışa bırakılır.
func isPureStatusConditions(conds []ruleCondition) bool {
	for _, c := range conds {
		switch strings.ToLower(strings.TrimSpace(c.Field)) {
		case "", "schedule_mode", "status":
			continue
		default:
			return false
		}
	}
	return true
}

// backoffGapSeconds: çevrimdışı kalınan süreye göre iki hatırlatma arası bekleme.
// 0-3 saat: saatte bir · 3-24 saat: 2 saatte bir · 24 saat sonrası: günde bir.
func backoffGapSeconds(offlineForSec int64) int64 {
	switch {
	case offlineForSec <= 3*3600:
		return 3600
	case offlineForSec <= 24*3600:
		return 2 * 3600
	default:
		return 24 * 3600
	}
}

// formatDurationTR süreyi "2 gün 3 sa 14 dk" biçiminde yazar.
func formatDurationTR(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	mins := int(d.Minutes()) % 60
	parts := make([]string, 0, 3)
	if days > 0 {
		parts = append(parts, fmt.Sprintf("%d gün", days))
	}
	if hours > 0 {
		parts = append(parts, fmt.Sprintf("%d sa", hours))
	}
	if mins > 0 || len(parts) == 0 {
		parts = append(parts, fmt.Sprintf("%d dk", mins))
	}
	return strings.Join(parts, " ")
}

// backoffMinResendGap: bellek içi emniyet kilidi — DB tarafında ne olursa olsun
// aynı (kural, hedef) çiftine bu aralıktan daha sık gönderim YAPILMAZ.
// Meşru en kısa kademe 1 saat olduğundan bu kilit normal akışı hiç etkilemez;
// yalnızca öngörülemeyen bir hata durumunda spam'i keser.
const backoffMinResendGap = 5 * time.Minute

// ensureMailLogSchema: new_notification_mails tablosunu modern şemaya getirir.
// Migration'lar cihazlarda otomatik çalışmadığı için eski kurulumlarda
// channel/template_id kolonları eksik olabilir; bu durumda logNotification'ın
// INSERT'i patlar, log boş kalır ve backoff dedup'u devre dışı kalırdı.
func (r *SimpleRuleRunner) ensureMailLogSchema() {
	if _, err := r.db.Exec(`
        CREATE TABLE IF NOT EXISTS new_notification_mails (
            id BIGINT AUTO_INCREMENT PRIMARY KEY,
            rule_id INT NULL,
            target_id INT NULL,
            channel ENUM('email','telegram') NOT NULL DEFAULT 'email',
            template_id INT NULL,
            recipients JSON NOT NULL,
            subject VARCHAR(500) NOT NULL,
            body TEXT NOT NULL,
            status ENUM('sent','failed') NOT NULL,
            error TEXT NULL,
            sent_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
            INDEX idx_rule (rule_id),
            INDEX idx_target (target_id),
            INDEX idx_sent (sent_at)
        ) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`); err != nil {
		log.Printf("⚠️ ensureMailLogSchema: tablo oluşturulamadı: %v", err)
	}
	// Eski şemadan gelen kurulumlarda eksik kolonları tamamla.
	// SIRALI slice: channel önce eklenmeli (template_id "AFTER channel" kullanır).
	for _, mig := range []struct{ col, ddl string }{
		{"channel", "ALTER TABLE new_notification_mails ADD COLUMN channel ENUM('email','telegram') NOT NULL DEFAULT 'email' AFTER target_id"},
		{"template_id", "ALTER TABLE new_notification_mails ADD COLUMN template_id INT NULL AFTER channel"},
	} {
		col, ddl := mig.col, mig.ddl
		var n int
		if err := r.db.QueryRow(`
            SELECT COUNT(*) FROM information_schema.COLUMNS
            WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'new_notification_mails' AND COLUMN_NAME = ?`, col).Scan(&n); err != nil {
			log.Printf("⚠️ ensureMailLogSchema: %s kolonu kontrol edilemedi: %v", col, err)
			continue
		}
		if n == 0 {
			if _, err := r.db.Exec(ddl); err != nil {
				log.Printf("⚠️ ensureMailLogSchema: %s kolonu eklenemedi: %v", col, err)
			} else {
				log.Printf("✅ ensureMailLogSchema: eksik '%s' kolonu eklendi", col)
			}
		}
	}
}

// backoffRecentlyTried: bellek içi emniyet kilidi kontrolü + kayıt.
func (r *SimpleRuleRunner) backoffRecentlyTried(ruleID, targetID int, now time.Time) bool {
	key := fmt.Sprintf("%d:%d", ruleID, targetID)
	r.backoffMu.Lock()
	defer r.backoffMu.Unlock()
	if last, ok := r.backoffLastSend[key]; ok && now.Sub(last) < backoffMinResendGap {
		return true
	}
	return false
}

func (r *SimpleRuleRunner) backoffMarkTried(ruleID, targetID int, now time.Time) {
	key := fmt.Sprintf("%d:%d", ruleID, targetID)
	r.backoffMu.Lock()
	r.backoffLastSend[key] = now
	r.backoffMu.Unlock()
}

// offlineSinceMs: hedefin içinde bulunduğu kesintinin başlangıcı (son başarılı
// ping'ten sonraki ilk başarısız ping'in ts_ms değeri). Hiç başarılı ping yoksa
// ilk başarısız ping alınır. Sorgu hatasında err döner — çağıran GÖNDERMEMELİDİR.
func (r *SimpleRuleRunner) offlineSinceMs(targetID int) (int64, bool, error) {
	var lastOk sql.NullInt64
	if err := r.db.QueryRow(`SELECT MAX(ts_ms) FROM pings_raw WHERE target_id = ? AND ok = 1`, targetID).Scan(&lastOk); err != nil {
		return 0, false, err
	}
	var since sql.NullInt64
	var err error
	if lastOk.Valid {
		err = r.db.QueryRow(`SELECT MIN(ts_ms) FROM pings_raw WHERE target_id = ? AND ok = 0 AND ts_ms > ?`, targetID, lastOk.Int64).Scan(&since)
	} else {
		err = r.db.QueryRow(`SELECT MIN(ts_ms) FROM pings_raw WHERE target_id = ? AND ok = 0`, targetID).Scan(&since)
	}
	if err != nil {
		return 0, false, err
	}
	if !since.Valid {
		return 0, false, nil
	}
	return since.Int64, true, nil
}

// lastOutageWindowMs: hedef şu an çevrimiçiyken, en son yaşanan kesintinin
// [başlangıç, bitiş] aralığını döndürür (bitiş = kesinti sonrası ilk başarılı ping).
func (r *SimpleRuleRunner) lastOutageWindowMs(targetID int) (int64, int64, bool, error) {
	var lastFail sql.NullInt64
	if err := r.db.QueryRow(`SELECT MAX(ts_ms) FROM pings_raw WHERE target_id = ? AND ok = 0`, targetID).Scan(&lastFail); err != nil {
		return 0, 0, false, err
	}
	if !lastFail.Valid {
		return 0, 0, false, nil // hiç kesinti yaşanmamış
	}
	var end sql.NullInt64
	if err := r.db.QueryRow(`SELECT MIN(ts_ms) FROM pings_raw WHERE target_id = ? AND ok = 1 AND ts_ms > ?`, targetID, lastFail.Int64).Scan(&end); err != nil {
		return 0, 0, false, err
	}
	if !end.Valid {
		return 0, 0, false, nil // kurtarma ping'i henüz yok
	}
	var prevOk sql.NullInt64
	if err := r.db.QueryRow(`SELECT MAX(ts_ms) FROM pings_raw WHERE target_id = ? AND ok = 1 AND ts_ms < ?`, targetID, lastFail.Int64).Scan(&prevOk); err != nil {
		return 0, 0, false, err
	}
	var start sql.NullInt64
	var err error
	if prevOk.Valid {
		err = r.db.QueryRow(`SELECT MIN(ts_ms) FROM pings_raw WHERE target_id = ? AND ok = 0 AND ts_ms > ?`, targetID, prevOk.Int64).Scan(&start)
	} else {
		err = r.db.QueryRow(`SELECT MIN(ts_ms) FROM pings_raw WHERE target_id = ? AND ok = 0`, targetID).Scan(&start)
	}
	if err != nil {
		return 0, 0, false, err
	}
	if !start.Valid {
		return 0, 0, false, nil
	}
	return start.Int64, end.Int64, true, nil
}

// backoffMailState: bu (kural, hedef) için gönderim logu özeti.
// Zaman karşılaştırmaları TZ tutarlılığı için SQL tarafında yapılır (FROM_UNIXTIME).
// Sorgu hatasında err döner — çağıran GÖNDERMEMELİDİR (spam'e karşı güvenli taraf).
func (r *SimpleRuleRunner) backoffMailState(ruleID, targetID int, sinceSec int64) (notifiedSince bool, secondsSinceLast int64, hasAny bool, err error) {
	var cnt int
	var after sql.NullInt64
	var sinceLast sql.NullInt64
	err = r.db.QueryRow(`
        SELECT COUNT(*),
               COALESCE(SUM(sent_at >= FROM_UNIXTIME(?)), 0),
               TIMESTAMPDIFF(SECOND, MAX(sent_at), NOW())
        FROM new_notification_mails
        WHERE rule_id = ? AND target_id = ? AND status = 'sent'`,
		sinceSec, ruleID, targetID).Scan(&cnt, &after, &sinceLast)
	if err != nil {
		return false, 0, false, err
	}
	if cnt == 0 {
		return false, 0, false, nil
	}
	return after.Valid && after.Int64 > 0, sinceLast.Int64, true, nil
}

// backoffRecoveryPending: kesinti sırasında bildirim gitmiş ama kurtarma sonrası
// hiç bildirim gönderilmemişse true (kurtarma bildirimi bir kez gider).
func (r *SimpleRuleRunner) backoffRecoveryPending(ruleID, targetID int, startSec, endSec int64) (bool, error) {
	var during, after int64
	err := r.db.QueryRow(`
        SELECT COALESCE(SUM(sent_at >= FROM_UNIXTIME(?) AND sent_at < FROM_UNIXTIME(?)), 0),
               COALESCE(SUM(sent_at >= FROM_UNIXTIME(?)), 0)
        FROM new_notification_mails
        WHERE rule_id = ? AND target_id = ? AND status = 'sent'`,
		startSec, endSec, endSec, ruleID, targetID).Scan(&during, &after)
	if err != nil {
		return false, err
	}
	return during > 0 && after == 0, nil
}

// processBackoffRule: backoff modundaki bir durum kuralını hedef başına işler.
// Çevrimdışı: kesintinin ilk bildirimi hemen; sonrası kademeli seyrekleşen hatırlatma.
// Çevrimiçi: kesinti bildirilmişse toplam süreyle tek kurtarma bildirimi.
// GÜVENLİK İLKESİ: herhangi bir sorgu hatasında o hedef bu tikte ATLANIR
// (hata "gönder" değil "gönderme" yönünde çözülür) ve bellek içi kilit
// hiçbir koşulda backoffMinResendGap'ten sık gönderime izin vermez.
func (r *SimpleRuleRunner) processBackoffRule(rr *newRuleRow, now time.Time) {
	targetIDs := make([]int, 0, len(rr.TargetIDs)+1)
	if len(rr.TargetIDs) > 0 {
		targetIDs = append(targetIDs, rr.TargetIDs...)
	} else if rr.TargetID != nil {
		targetIDs = append(targetIDs, *rr.TargetID)
	}
	sentAny := false
	nowMs := now.UnixMilli()
	for _, targetID := range targetIDs {
		if r.backoffRecentlyTried(rr.ID, targetID, now) {
			continue // emniyet kilidi: kısa süre önce denendi
		}
		name, _, _, _, lastSuccess := r.loadTargetWithStatus(targetID)
		if !lastSuccess {
			// ÇEVRİMDIŞI
			sinceMs, ok, err := r.offlineSinceMs(targetID)
			if err != nil {
				log.Printf("⚠️ Backoff rule %d - target %d: kesinti başlangıcı okunamadı, bu tik atlanıyor: %v", rr.ID, targetID, err)
				continue
			}
			if !ok {
				sinceMs = nowMs
			}
			offlineFor := time.Duration(nowMs-sinceMs) * time.Millisecond
			notifiedSince, secondsSinceLast, hasAny, err := r.backoffMailState(rr.ID, targetID, sinceMs/1000)
			if err != nil {
				log.Printf("⚠️ Backoff rule %d - target %d: gönderim logu okunamadı, bu tik atlanıyor: %v", rr.ID, targetID, err)
				continue
			}
			if hasAny && notifiedSince {
				// Bu kesinti için daha önce bildirim gitti → kademe süresi dolmadan tekrar gönderme
				if secondsSinceLast < backoffGapSeconds(int64(offlineFor.Seconds())) {
					continue
				}
			}
			r.backoffMarkTried(rr.ID, targetID, now)
			mc := &messageContext{kind: "status", status: "fail", offlineFor: &offlineFor}
			if err := r.sendForTargetWithContext(*rr, targetID, now, now, mc); err != nil {
				log.Printf("❌ Backoff rule %d - target %d (%s): offline bildirimi gönderilemedi: %v", rr.ID, targetID, name, err)
			} else {
				log.Printf("📉 Backoff rule %d - target %d (%s): offline bildirimi gönderildi (çevrimdışı: %s)", rr.ID, targetID, name, formatDurationTR(offlineFor))
				sentAny = true
			}
		} else {
			// ÇEVRİMİÇİ — kurtarma bildirimi gerekiyor mu?
			startMs, endMs, ok, err := r.lastOutageWindowMs(targetID)
			if err != nil {
				log.Printf("⚠️ Backoff rule %d - target %d: kesinti aralığı okunamadı, bu tik atlanıyor: %v", rr.ID, targetID, err)
				continue
			}
			if !ok {
				continue
			}
			pending, err := r.backoffRecoveryPending(rr.ID, targetID, startMs/1000, endMs/1000)
			if err != nil {
				log.Printf("⚠️ Backoff rule %d - target %d: kurtarma durumu okunamadı, bu tik atlanıyor: %v", rr.ID, targetID, err)
				continue
			}
			if !pending {
				continue
			}
			r.backoffMarkTried(rr.ID, targetID, now)
			total := time.Duration(endMs-startMs) * time.Millisecond
			mc := &messageContext{kind: "status", status: "success", recoveredAfter: &total}
			if err := r.sendForTargetWithContext(*rr, targetID, now, now, mc); err != nil {
				log.Printf("❌ Backoff rule %d - target %d (%s): kurtarma bildirimi gönderilemedi: %v", rr.ID, targetID, name, err)
			} else {
				log.Printf("📈 Backoff rule %d - target %d (%s): tekrar çevrimiçi bildirimi gönderildi (toplam kesinti: %s)", rr.ID, targetID, name, formatDurationTR(total))
				sentAny = true
			}
		}
	}
	if sentAny {
		// UI'daki "son gönderim" bilgisi için; backoff zamanlaması bunu KULLANMAZ
		_, _ = r.db.Exec("UPDATE new_notification_rules SET last_sent_at = NOW() WHERE id = ?", rr.ID)
	}
}
