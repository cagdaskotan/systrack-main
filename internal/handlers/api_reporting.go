package handlers

import (
	"bytes"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"html"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"systrack/internal/ping"

	"github.com/gin-gonic/gin"
	"github.com/jung-kurt/gofpdf"
)

type ReportType string

const (
	ReportTypeSystem ReportType = "system_overview"
)

type ReportTemplate struct {
	ID          int                    `json:"id"`
	Name        string                 `json:"name"`
	Type        ReportType             `json:"type"`
	Description string                 `json:"description"`
	Config      map[string]interface{} `json:"config"`
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
}

type DateRange struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

type ReportRequest struct {
	Type      ReportType `json:"type"`
	Format    string     `json:"format"`
	DateRange DateRange  `json:"date_range"`
	Targets   []int      `json:"targets"`
}

type ReportResponse struct {
	ID        int       `json:"id"`
	Type      string    `json:"type"`
	Title     string    `json:"title"`
	Format    string    `json:"format"`
	Status    string    `json:"status"`
	URL       string    `json:"url,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type reportConfig struct {
	Format    string    `json:"format"`
	DateRange DateRange `json:"date_range"`
	TargetIDs []int     `json:"target_ids,omitempty"`
}

type SystemReportData struct {
	GeneratedAt         string               `json:"generated_at"`
	DateRange           DateRange            `json:"date_range"`
	SystemHealth        SystemHealth         `json:"system_health"`
	TargetSummary       TargetSummary        `json:"target_summary"`
	Targets             []TargetSnapshot     `json:"targets"`
	NotificationSummary NotificationSummary  `json:"notification_summary"`
	Notifications       []NotificationRecord `json:"notifications"`
	BackupSummary       BackupSummary        `json:"backup_summary"`
	Backups             []BackupRecord       `json:"backups"`
}

type SystemHealth struct {
	OverallScore      float64 `json:"overall_score"`       // 0-100 genel sağlık skoru
	TotalUptime       float64 `json:"total_uptime"`        // Genel uptime yüzdesi
	AverageResponseMS float64 `json:"average_response_ms"` // Ortalama yanıt süresi
	TotalIncidents    int     `json:"total_incidents"`     // Toplam kesinti sayısı
	CriticalTargets   int     `json:"critical_targets"`    // Kritik durumda olan target sayısı
	HealthStatus      string  `json:"health_status"`       // "Mükemmel", "İyi", "Orta", "Kötü"
}

type TargetSummary struct {
	Total    int `json:"total"`
	Active   int `json:"active"`
	Inactive int `json:"inactive"`
	Online   int `json:"online"`
	Offline  int `json:"offline"`
	Unknown  int `json:"unknown"`
}

type TargetSnapshot struct {
	ID             int              `json:"id"`
	Name           string           `json:"name"`
	Address        string           `json:"address"`
	Type           string           `json:"type"`
	Enabled        bool             `json:"enabled"`
	Status         string           `json:"status"`
	LastCheck      string           `json:"last_check"`
	ResponseTimeMS *int             `json:"response_time_ms,omitempty"`
	SLAMetrics     TargetSLAMetrics `json:"sla_metrics"`
}

type TargetSLAMetrics struct {
	UptimePercent   float64 `json:"uptime_percent"`   // Uptime yüzdesi
	DowntimeMinutes int     `json:"downtime_minutes"` // Toplam kesinti süresi (dakika)
	TotalPings      int     `json:"total_pings"`      // Toplam ping sayısı
	SuccessfulPings int     `json:"successful_pings"` // Başarılı ping sayısı
	FailedPings     int     `json:"failed_pings"`     // Başarısız ping sayısı
	AvgResponseMS   float64 `json:"avg_response_ms"`  // Ortalama yanıt süresi
	MinResponseMS   int     `json:"min_response_ms"`  // En iyi yanıt süresi
	MaxResponseMS   int     `json:"max_response_ms"`  // En kötü yanıt süresi
	ResponseGrade   string  `json:"response_grade"`   // Yanıt süresi performansı: "Mükemmel", "İyi", "Orta", "Yavaş"
	IncidentCount   int     `json:"incident_count"`   // Kesinti sayısı
	HealthScore     float64 `json:"health_score"`     // 0-100 sağlık skoru
	HealthGrade     string  `json:"health_grade"`     // "Mükemmel", "İyi", "Orta", "Kötü"
}

type NotificationSummary struct {
	Total    int            `json:"total"`
	Sent     int            `json:"sent"`
	Failed   int            `json:"failed"`
	Channels map[string]int `json:"channels"`
}

type NotificationRecord struct {
	ID         int      `json:"id"`
	Channel    string   `json:"channel"`
	Status     string   `json:"status"`
	Subject    string   `json:"subject"`
	Recipients []string `json:"recipients"`
	TargetName string   `json:"target_name"`
	SentAt     string   `json:"sent_at"`
	Error      *string  `json:"error,omitempty"`
}

type BackupSummary struct {
	Total   int `json:"total"`
	Success int `json:"success"`
	Error   int `json:"error"`
}

type BackupRecord struct {
	ID        int    `json:"id"`
	Action    string `json:"action"`
	Status    string `json:"status"`
	FileName  string `json:"file_name"`
	Message   string `json:"message"`
	UserEmail string `json:"user_email"`
	CreatedAt string `json:"created_at"`
}

func GetReportTemplates(_ interface{}) gin.HandlerFunc {
	return func(c *gin.Context) {
		now := time.Now()
		templates := []ReportTemplate{
			{
				ID:          1,
				Name:        "Sistem Özeti",
				Type:        ReportTypeSystem,
				Description: "Hedefler, bildirimler ve yedekleme aktivitelerini tek bir raporda birleştirir.",
				Config: map[string]interface{}{
					"sections":          []string{"targets", "notifications", "backups"},
					"allows_date_range": true,
				},
				CreatedAt: now,
				UpdatedAt: now,
			},
		}

		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"data":    templates,
		})
	}
}

func GenerateReport(db interface{}) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req ReportRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Geçersiz rapor isteği"})
			return
		}

		reportType := req.Type
		if reportType == "" {
			reportType = ReportTypeSystem
		}
		if reportType != ReportTypeSystem {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Desteklenmeyen rapor türü"})
			return
		}

		format := strings.ToLower(strings.TrimSpace(req.Format))
		if format == "" {
			format = "pdf"
		}
		if format != "pdf" && format != "csv" && format != "html" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Geçersiz format. pdf, csv veya html seçebilirsiniz"})
			return
		}

		start, end, err := resolveDateRange(req.DateRange)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		targetIDs := normalizeIDs(req.Targets)

		sqlDB, ok := db.(*sql.DB)
		if !ok {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Veritabanı bağlantısı kurulamadı"})
			return
		}

		if err := ensureReportsTable(sqlDB); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Rapor tablosu oluşturulamadı"})
			return
		}

		config := reportConfig{
			Format: format,
			DateRange: DateRange{
				Start: start.Format(time.RFC3339),
				End:   end.Format(time.RFC3339),
			},
			TargetIDs: targetIDs,
		}
		configJSON, _ := json.Marshal(config)

		title := fmt.Sprintf("Sistem Özeti (%s - %s)", start.Format("02.01.2006"), end.Format("02.01.2006"))

		// ID'yi veritabanı üretir (AUTO_INCREMENT); böylece aynı saniyede
		// birden fazla rapor üretilse bile çakışma olmaz.
		result, err := sqlDB.Exec(`
			INSERT INTO reports (type, title, format, status, url, config, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, NOW(), NOW())
		`, string(reportType), title, format, "completed", "", string(configJSON))
		if err != nil {
			log.Printf("report: rapor kaydedilemedi: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Rapor kaydedilemedi"})
			return
		}

		lastID, err := result.LastInsertId()
		if err != nil {
			log.Printf("report: rapor ID'si alınamadı: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Rapor kaydedilemedi"})
			return
		}
		reportID := int(lastID)

		downloadURL := fmt.Sprintf("/api/reporting/reports/%d/download", reportID)
		if _, err := sqlDB.Exec(`UPDATE reports SET url = ? WHERE id = ?`, downloadURL, reportID); err != nil {
			log.Printf("report: rapor indirme adresi güncellenemedi: %v", err)
		}

		response := ReportResponse{
			ID:        reportID,
			Type:      string(reportType),
			Title:     title,
			Format:    format,
			Status:    "completed",
			URL:       downloadURL,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"data":    response,
		})
	}
}

func GetReports(db interface{}) gin.HandlerFunc {
	return func(c *gin.Context) {
		sqlDB, ok := db.(*sql.DB)
		if !ok {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Veritabanı bağlantısı kurulamadı"})
			return
		}

		page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
		if page < 1 {
			page = 1
		}
		limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
		if limit <= 0 {
			limit = 20
		}
		offset := (page - 1) * limit

		filterType := c.Query("type")
		filterStatus := c.Query("status")

		query := `
			SELECT id, type, title, format, status, url, created_at, updated_at
			FROM reports
			WHERE 1=1
		`
		args := make([]interface{}, 0, 4)
		if filterType != "" {
			query += " AND type = ?"
			args = append(args, filterType)
		}
		if filterStatus != "" {
			query += " AND status = ?"
			args = append(args, filterStatus)
		}
		query += " ORDER BY created_at DESC LIMIT ? OFFSET ?"
		args = append(args, limit, offset)

		rows, err := sqlDB.Query(query, args...)
		if err != nil {
			if isTableMissingError(err) {
				c.JSON(http.StatusOK, gin.H{
					"success": true,
					"data": gin.H{
						"reports": []ReportResponse{},
						"pagination": gin.H{
							"page":  page,
							"limit": limit,
							"total": 0,
							"pages": 0,
						},
					},
				})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Raporlar alınamadı"})
			return
		}
		defer rows.Close()

		reports := make([]ReportResponse, 0, limit)
		for rows.Next() {
			var (
				report ReportResponse
				urlVal sql.NullString
			)
			if err := rows.Scan(&report.ID, &report.Type, &report.Title, &report.Format, &report.Status, &urlVal, &report.CreatedAt, &report.UpdatedAt); err != nil {
				continue
			}
			if urlVal.Valid {
				report.URL = urlVal.String
			}
			reports = append(reports, report)
		}
		if err := rows.Err(); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Rapor listesi okunamadı"})
			return
		}

		countQuery := "SELECT COUNT(*) FROM reports WHERE 1=1"
		countArgs := make([]interface{}, 0, 2)
		if filterType != "" {
			countQuery += " AND type = ?"
			countArgs = append(countArgs, filterType)
		}
		if filterStatus != "" {
			countQuery += " AND status = ?"
			countArgs = append(countArgs, filterStatus)
		}

		total := 0
		if err := sqlDB.QueryRow(countQuery, countArgs...).Scan(&total); err != nil && !isTableMissingError(err) {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Rapor sayısı hesaplanamadı"})
			return
		}

		totalPages := 0
		if total > 0 {
			totalPages = (total + limit - 1) / limit
		}

		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"data": gin.H{
				"reports": reports,
				"pagination": gin.H{
					"page":  page,
					"limit": limit,
					"total": total,
					"pages": totalPages,
				},
			},
		})
	}
}

func GetReportStatus(db interface{}) gin.HandlerFunc {
	return func(c *gin.Context) {
		sqlDB, ok := db.(*sql.DB)
		if !ok {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Veritabanı bağlantısı kurulamadı"})
			return
		}

		id, err := strconv.Atoi(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Geçersiz rapor ID"})
			return
		}

		var (
			report ReportResponse
			urlVal sql.NullString
		)
		err = sqlDB.QueryRow(`
			SELECT id, type, title, format, status, url, created_at, updated_at
			FROM reports
			WHERE id = ?
		`, id).Scan(&report.ID, &report.Type, &report.Title, &report.Format, &report.Status, &urlVal, &report.CreatedAt, &report.UpdatedAt)
		if err != nil {
			if err == sql.ErrNoRows {
				c.JSON(http.StatusNotFound, gin.H{"error": "Rapor bulunamadı"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Rapor bilgisi alınamadı"})
			return
		}
		if urlVal.Valid {
			report.URL = urlVal.String
		}

		c.JSON(http.StatusOK, gin.H{"success": true, "data": report})
	}
}

func DownloadReport(db interface{}) gin.HandlerFunc {
	return func(c *gin.Context) {
		reportID, err := strconv.Atoi(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Geçersiz rapor ID"})
			return
		}

		sqlDB, ok := db.(*sql.DB)
		if !ok {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Veritabanı bağlantısı kurulamadı"})
			return
		}

		var (
			status    string
			format    string
			title     string
			configRaw sql.NullString
		)
		err = sqlDB.QueryRow(`
			SELECT status, format, title, config
			FROM reports
			WHERE id = ?
		`, reportID).Scan(&status, &format, &title, &configRaw)
		if err != nil {
			if err == sql.ErrNoRows {
				c.JSON(http.StatusNotFound, gin.H{"error": "Rapor bulunamadı"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Rapor bilgisi alınamadı"})
			return
		}

		if status != "completed" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Rapor henüz hazır değil"})
			return
		}

		cfg := reportConfig{Format: format}
		if configRaw.Valid && configRaw.String != "" {
			if err := json.Unmarshal([]byte(configRaw.String), &cfg); err != nil {
				cfg = reportConfig{Format: format}
			}
		}
		if cfg.Format != "" {
			format = strings.ToLower(strings.TrimSpace(cfg.Format))
		}

		start, end, err := resolveDateRange(cfg.DateRange)
		if err != nil {
			end = time.Now()
			start = end.AddDate(0, 0, -30)
		}

		data, err := fetchSystemReportData(sqlDB, start, end, cfg.TargetIDs)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Rapor verileri oluşturulamadı"})
			return
		}

		content, err := generateSystemReportContent(format, title, data)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Rapor içerik üretimi başarısız"})
			return
		}

		// Format değerini temizle ve dosya adını oluştur
		cleanFormat := strings.TrimSpace(format)
		filename := fmt.Sprintf("system_report_%d.%s", reportID, cleanFormat)

		switch cleanFormat {
		case "csv":
			c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
			c.Header("Content-Type", "text/csv; charset=utf-8")
			c.String(http.StatusOK, content)
		case "html":
			c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
			c.Header("Content-Type", "text/html; charset=utf-8")
			c.String(http.StatusOK, content)
		case "pdf":
			// PDF için HTML içeriği oluştur ve tarayıcının print özelliğini kullan
			htmlContent := content // HTML içeriği zaten var

			// Print butonu ile HTML döndür
			printableHTML := generatePrintableHTML(htmlContent, title)
			c.Header("Content-Type", "text/html; charset=utf-8")
			c.String(http.StatusOK, printableHTML)
		default:
			c.JSON(http.StatusBadRequest, gin.H{"error": "Desteklenmeyen format"})
		}
	}
}

func DeleteReport(db interface{}) gin.HandlerFunc {
	return func(c *gin.Context) {
		sqlDB, ok := db.(*sql.DB)
		if !ok {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Veritabanı bağlantısı kurulamadı"})
			return
		}

		id, err := strconv.Atoi(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Geçersiz rapor ID"})
			return
		}

		result, err := sqlDB.Exec("DELETE FROM reports WHERE id = ?", id)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Rapor silinemedi"})
			return
		}

		if rows, _ := result.RowsAffected(); rows == 0 {
			c.JSON(http.StatusNotFound, gin.H{"error": "Rapor bulunamadı"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "Rapor silindi",
		})
	}
}

func GetReportData(db interface{}) gin.HandlerFunc {
	return func(c *gin.Context) {
		sqlDB, ok := db.(*sql.DB)
		if !ok {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Veritabanı bağlantısı kurulamadı"})
			return
		}

		dr := DateRange{
			Start: c.Query("start"),
			End:   c.Query("end"),
		}
		start, end, err := resolveDateRange(dr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		targetIDs := parseTargetsParam(c.Query("targets"))

		data, err := fetchSystemReportData(sqlDB, start, end, targetIDs)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Rapor verileri toplanamadı"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"success": true, "data": data})
	}
}

func ensureReportsTable(db *sql.DB) error {
	var exists int
	if err := db.QueryRow(`
		SELECT COUNT(*)
		FROM information_schema.tables
		WHERE table_schema = DATABASE() AND table_name = 'reports'
	`).Scan(&exists); err != nil {
		return err
	}
	if exists > 0 {
		// Mevcut kurulumlarda id AUTO_INCREMENT değilse yükselt.
		// Eski sürüm id olarak unix saniye kullanıyordu; aynı saniyede iki rapor
		// üretilince primary key çakışıyor ve "Rapor kaydedilemedi" hatası veriyordu.
		var extra sql.NullString
		if err := db.QueryRow(`
			SELECT EXTRA
			FROM information_schema.columns
			WHERE table_schema = DATABASE() AND table_name = 'reports' AND column_name = 'id'
		`).Scan(&extra); err != nil {
			return err
		}
		if !strings.Contains(strings.ToLower(extra.String), "auto_increment") {
			if _, err := db.Exec(`ALTER TABLE reports MODIFY id INT NOT NULL AUTO_INCREMENT`); err != nil {
				// Yükseltme başarısız olsa da rapor üretimi çalışmaya devam etmeli.
				log.Printf("report: reports.id AUTO_INCREMENT'e yükseltilemedi: %v", err)
			}
		}
		return nil
	}

	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS reports (
			id INT AUTO_INCREMENT PRIMARY KEY,
			type VARCHAR(50) NOT NULL,
			title VARCHAR(255) NOT NULL,
			format VARCHAR(20) NOT NULL,
			status ENUM('generating','completed','failed') DEFAULT 'generating',
			url VARCHAR(500),
			config JSON,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
		)
	`)
	return err
}

func resolveDateRange(dr DateRange) (time.Time, time.Time, error) {
	now := time.Now()
	var (
		end time.Time
		err error
	)
	if strings.TrimSpace(dr.End) != "" {
		end, err = parseFlexibleDate(dr.End)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("Bitiş tarihi okunamadı")
		}
	} else {
		end = now
	}

	var start time.Time
	if strings.TrimSpace(dr.Start) != "" {
		start, err = parseFlexibleDate(dr.Start)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("Başlangıç tarihi okunamadı")
		}
	} else {
		start = end.AddDate(0, 0, -30)
	}

	if start.After(end) {
		return time.Time{}, time.Time{}, fmt.Errorf("Başlangıç tarihi bitiş tarihinden sonra olamaz")
	}

	start = time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, start.Location())
	end = time.Date(end.Year(), end.Month(), end.Day(), 23, 59, 59, 0, end.Location())

	return start, end, nil
}

func parseFlexibleDate(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	layouts := []string{
		time.RFC3339,
		time.RFC3339Nano,
		"2006-01-02 15:04:05",
		"2006-01-02 15:04",
		"2006-01-02",
		"02.01.2006",
		"02.01.2006 15:04",
		"02.01.2006 15:04:05",
		"2006-01-02T15:04",
		"2006-01-02T15:04:05",
	}
	for _, layout := range layouts {
		if t, err := time.ParseInLocation(layout, value, time.Local); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid date format")
}

func normalizeIDs(ids []int) []int {
	if len(ids) == 0 {
		return nil
	}
	seen := make(map[int]struct{}, len(ids))
	normalized := make([]int, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		normalized = append(normalized, id)
	}
	sort.Ints(normalized)
	return normalized
}

func parseTargetsParam(value string) []int {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	targets := make([]int, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if id, err := strconv.Atoi(part); err == nil && id > 0 {
			targets = append(targets, id)
		}
	}
	return normalizeIDs(targets)
}

func buildInClause(column string, ids []int) (string, []interface{}) {
	if len(ids) == 0 {
		return "", nil
	}
	placeholders := make([]string, len(ids))
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}
	return fmt.Sprintf("%s IN (%s)", column, strings.Join(placeholders, ",")), args
}

func fetchSystemReportData(db *sql.DB, start, end time.Time, targetIDs []int) (SystemReportData, error) {
	data := SystemReportData{
		GeneratedAt: time.Now().Format(time.RFC3339),
		DateRange: DateRange{
			Start: start.Format(time.RFC3339),
			End:   end.Format(time.RFC3339),
		},
		TargetSummary: TargetSummary{},
		Targets:       make([]TargetSnapshot, 0, 16),
		NotificationSummary: NotificationSummary{
			Channels: make(map[string]int),
		},
		Notifications: make([]NotificationRecord, 0, 10),
		BackupSummary: BackupSummary{},
		Backups:       make([]BackupRecord, 0, 5),
	}

	targetClause, targetArgs := buildInClause("id", targetIDs)
	targetSummaryQuery := "SELECT COUNT(*), SUM(CASE WHEN enabled = 1 THEN 1 ELSE 0 END), SUM(CASE WHEN enabled = 0 THEN 1 ELSE 0 END) FROM targets"
	if targetClause != "" {
		targetSummaryQuery += " WHERE " + targetClause
	}

	var (
		total    sql.NullInt64
		active   sql.NullInt64
		inactive sql.NullInt64
	)
	if err := db.QueryRow(targetSummaryQuery, targetArgs...).Scan(&total, &active, &inactive); err != nil {
		return data, err
	}
	if total.Valid {
		data.TargetSummary.Total = int(total.Int64)
	}
	if active.Valid {
		data.TargetSummary.Active = int(active.Int64)
	}
	if inactive.Valid {
		data.TargetSummary.Inactive = int(inactive.Int64)
	}

	// Tüm hedeflerin SLA metriklerini TEK sorguda hesapla.
	// (Önceden her hedef için ayrı ayrı iki sorgu çalıştırılıyor ve aralıktaki
	// bütün ping satırları Go tarafına çekiliyordu -> N+1 + milyonlarca satır.)
	slaByTarget, err := fetchAllTargetSLAMetrics(db, start, end, targetIDs)
	if err != nil {
		log.Printf("report: SLA metrikleri hesaplanamadı: %v", err)
		return data, fmt.Errorf("SLA metrikleri hesaplanamadı: %w", err)
	}

	targetClause, targetArgs = buildInClause("t.id", targetIDs)
	// Son ping'i bulmak için ROW_NUMBER ile tüm aralığı sıralamak yerine
	// (milyonlarca satırlık filesort), hedef başına MAX(ts_ms)'i idx_pings_target_ts
	// üzerinden alıp yalnızca o satırlara join yapıyoruz (loose index scan).
	recentQuery := `
		SELECT
			t.id, t.name, t.address, t.type, t.enabled,
			last_ping.ok, last_ping.rtt_ms, last_ping.ts_ms
		FROM targets t
		LEFT JOIN (
			SELECT p.target_id, p.ok, p.rtt_ms, p.ts_ms
			FROM pings_raw p
			JOIN (
				SELECT target_id, MAX(ts_ms) AS mx
				FROM pings_raw
				WHERE ts_ms BETWEEN ? AND ?
				GROUP BY target_id
			) m ON m.target_id = p.target_id AND m.mx = p.ts_ms
		) AS last_ping ON last_ping.target_id = t.id
	`
	args := []interface{}{start.UnixMilli(), end.UnixMilli()}
	if targetClause != "" {
		recentQuery += " WHERE " + targetClause
		args = append(args, targetArgs...)
	}
	recentQuery += " ORDER BY t.name ASC"

	targetRows, err := db.Query(recentQuery, args...)
	if err != nil {
		// Bu sorgu raporun çekirdeği; sessizce yutulursa rapor "her şey 0" görünür.
		log.Printf("report: hedef listesi sorgusu başarısız: %v", err)
		return data, fmt.Errorf("hedef listesi alınamadı: %w", err)
	}
	defer targetRows.Close()

	for targetRows.Next() {
		var (
			id         int
			name       string
			address    string
			targetType string
			enabled    bool
			okVal      sql.NullInt64
			rttVal     sql.NullInt64
			tsVal      sql.NullInt64
		)
		if err := targetRows.Scan(&id, &name, &address, &targetType, &enabled, &okVal, &rttVal, &tsVal); err != nil {
			log.Printf("report: hedef satırı okunamadı: %v", err)
			continue
		}

		status := "Pasif"
		if enabled {
			if !okVal.Valid {
				status = "Veri Yok"
				data.TargetSummary.Unknown++
			} else if okVal.Int64 == 1 {
				status = "Çevrim içi"
				data.TargetSummary.Online++
			} else {
				status = "Kapalı"
				data.TargetSummary.Offline++
			}
		}

		var responsePtr *int
		if rttVal.Valid && rttVal.Int64 > 0 {
			val := int(rttVal.Int64)
			responsePtr = &val
		}

		lastCheck := ""
		if tsVal.Valid && tsVal.Int64 > 0 {
			lastCheck = time.UnixMilli(tsVal.Int64).In(time.Local).Format("02.01.2006 15:04")
		}

		// Aralıkta ping'i olmayan hedefler map'te bulunmaz -> sıfır değerli metrik.
		slaMetrics := slaByTarget[id]

		data.Targets = append(data.Targets, TargetSnapshot{
			ID:             id,
			Name:           name,
			Address:        address,
			Type:           targetType,
			Enabled:        enabled,
			Status:         status,
			LastCheck:      lastCheck,
			ResponseTimeMS: responsePtr,
			SLAMetrics:     slaMetrics,
		})
	}
	if err := targetRows.Err(); err != nil {
		log.Printf("report: hedef listesi okunurken hata: %v", err)
		return data, fmt.Errorf("hedef listesi okunamadı: %w", err)
	}

	startStr := start.Format("2006-01-02 15:04:05")
	endStr := end.Format("2006-01-02 15:04:05")

	notifClause, notifArgs := buildInClause("m.target_id", targetIDs)
	notifSummaryQuery := `
		SELECT m.status, COUNT(*)
		FROM new_notification_mails m
		WHERE m.sent_at BETWEEN ? AND ?
	`
	summaryArgs := []interface{}{startStr, endStr}
	if notifClause != "" {
		notifSummaryQuery += " AND " + notifClause
		summaryArgs = append(summaryArgs, notifArgs...)
	}
	notifSummaryQuery += " GROUP BY m.status"
	notifSummaryRows, err := db.Query(notifSummaryQuery, summaryArgs...)
	if err != nil {
		log.Printf("report: bildirim özeti sorgusu başarısız: %v", err)
	}
	if rows := notifSummaryRows; err == nil {
		defer rows.Close()
		for rows.Next() {
			var status string
			var count int
			if err := rows.Scan(&status, &count); err != nil {
				continue
			}
			data.NotificationSummary.Total += count
			switch strings.ToLower(status) {
			case "sent", "delivered":
				data.NotificationSummary.Sent += count
			case "error", "failed":
				data.NotificationSummary.Failed += count
			}
		}
	}

	channelQuery := `
		SELECT m.channel, COUNT(*)
		FROM new_notification_mails m
		WHERE m.sent_at BETWEEN ? AND ?
	`
	channelArgs := []interface{}{startStr, endStr}
	if notifClause != "" {
		channelQuery += " AND " + notifClause
		channelArgs = append(channelArgs, notifArgs...)
	}
	channelQuery += " GROUP BY m.channel"
	channelRows, err := db.Query(channelQuery, channelArgs...)
	if err != nil {
		log.Printf("report: bildirim kanalı sorgusu başarısız: %v", err)
	}
	if rows := channelRows; err == nil {
		defer rows.Close()
		for rows.Next() {
			var channel string
			var count int
			if err := rows.Scan(&channel, &count); err != nil {
				continue
			}
			if channel == "" {
				channel = "bilinmiyor"
			}
			data.NotificationSummary.Channels[channel] += count
		}
	}

	notifDetailQuery := `
		SELECT 
			m.id, m.channel, m.status, m.subject, m.recipients, m.sent_at, m.error,
			IFNULL(t.name, '')
		FROM new_notification_mails m
		LEFT JOIN targets t ON t.id = m.target_id
		WHERE m.sent_at BETWEEN ? AND ?
	`
	detailArgs := []interface{}{startStr, endStr}
	if notifClause != "" {
		notifDetailQuery += " AND " + notifClause
		detailArgs = append(detailArgs, notifArgs...)
	}
	notifDetailQuery += " ORDER BY m.sent_at DESC LIMIT 10"
	notifDetailRows, err := db.Query(notifDetailQuery, detailArgs...)
	if err != nil {
		log.Printf("report: bildirim detay sorgusu başarısız: %v", err)
	}
	if rows := notifDetailRows; err == nil {
		defer rows.Close()
		for rows.Next() {
			var (
				record     NotificationRecord
				subjectVal sql.NullString
				recipients sql.NullString
				sentAtVal  sql.NullTime
				errorText  sql.NullString
				targetName sql.NullString
			)
			if err := rows.Scan(&record.ID, &record.Channel, &record.Status, &subjectVal, &recipients, &sentAtVal, &errorText, &targetName); err != nil {
				continue
			}
			if subjectVal.Valid {
				record.Subject = subjectVal.String
			}
			if targetName.Valid {
				record.TargetName = targetName.String
			}
			if recipients.Valid && recipients.String != "" {
				var parsed []string
				if err := json.Unmarshal([]byte(recipients.String), &parsed); err == nil {
					record.Recipients = parsed
				}
			}
			if sentAtVal.Valid {
				record.SentAt = sentAtVal.Time.In(time.Local).Format("02.01.2006 15:04")
			}
			if errorText.Valid && errorText.String != "" {
				msg := errorText.String
				record.Error = &msg
			}
			data.Notifications = append(data.Notifications, record)
		}
	}

	backupSummaryQuery := `
		SELECT status, COUNT(*)
		FROM backup_logs
		WHERE created_at BETWEEN ? AND ?
		GROUP BY status
	`
	backupSummaryRows, err := db.Query(backupSummaryQuery, startStr, endStr)
	if err != nil {
		log.Printf("report: yedek özeti sorgusu başarısız: %v", err)
	}
	if rows := backupSummaryRows; err == nil {
		defer rows.Close()
		for rows.Next() {
			var status string
			var count int
			if err := rows.Scan(&status, &count); err != nil {
				continue
			}
			data.BackupSummary.Total += count
			switch strings.ToLower(status) {
			case "success":
				data.BackupSummary.Success += count
			case "error":
				data.BackupSummary.Error += count
			}
		}
	}

	backupDetailsQuery := `
		SELECT id, action, status, file_name, message, user_email, created_at
		FROM backup_logs
		WHERE created_at BETWEEN ? AND ?
		ORDER BY created_at DESC
		LIMIT 5
	`
	backupDetailRows, err := db.Query(backupDetailsQuery, startStr, endStr)
	if err != nil {
		log.Printf("report: yedek detay sorgusu başarısız: %v", err)
	}
	if rows := backupDetailRows; err == nil {
		defer rows.Close()
		for rows.Next() {
			var (
				record     BackupRecord
				fileName   sql.NullString
				message    sql.NullString
				userEmail  sql.NullString
				createdVal sql.NullTime
			)
			if err := rows.Scan(&record.ID, &record.Action, &record.Status, &fileName, &message, &userEmail, &createdVal); err != nil {
				continue
			}
			if fileName.Valid {
				record.FileName = fileName.String
			}
			if message.Valid {
				record.Message = message.String
			}
			if userEmail.Valid {
				record.UserEmail = userEmail.String
			}
			if createdVal.Valid {
				record.CreatedAt = createdVal.Time.In(time.Local).Format("02.01.2006 15:04")
			}
			data.Backups = append(data.Backups, record)
		}
	}

	// Sistem sağlığını hesapla
	data.SystemHealth = calculateSystemHealth(data.Targets)

	return data, nil
}

func generateSystemReportContent(format, title string, data SystemReportData) (string, error) {
	switch format {
	case "csv":
		return generateSystemReportCSV(title, data), nil
	case "html", "pdf":
		return generateSystemReportHTML(title, data), nil
	default:
		return "", fmt.Errorf("unsupported format")
	}
}

func generateSystemReportCSV(title string, data SystemReportData) string {
	builder := &strings.Builder{}
	writer := csv.NewWriter(builder)

	writer.Write([]string{title})
	writer.Write([]string{"Oluşturulma", data.GeneratedAt})
	writer.Write([]string{"Veri Aralığı", fmt.Sprintf("%s - %s", data.DateRange.Start, data.DateRange.End)})
	writer.Write([]string{})

	// Sistem Sağlığı
	writer.Write([]string{"Sistem Sağlığı"})
	writer.Write([]string{"Metrik", "Değer"})
	writer.Write([]string{"Genel Sağlık Skoru", fmt.Sprintf("%.1f/100", data.SystemHealth.OverallScore)})
	writer.Write([]string{"Sağlık Durumu", data.SystemHealth.HealthStatus})
	writer.Write([]string{"Toplam Uptime", fmt.Sprintf("%.2f%%", data.SystemHealth.TotalUptime)})
	writer.Write([]string{"Ortalama Yanıt Süresi", fmt.Sprintf("%.0f ms", data.SystemHealth.AverageResponseMS)})
	writer.Write([]string{"Toplam Kesinti", strconv.Itoa(data.SystemHealth.TotalIncidents)})
	writer.Write([]string{"Kritik Hedef Sayısı", strconv.Itoa(data.SystemHealth.CriticalTargets)})
	writer.Write([]string{})

	writer.Write([]string{"Hedef Özeti"})
	writer.Write([]string{"Metrik", "Değer"})
	writer.Write([]string{"Toplam Hedef", strconv.Itoa(data.TargetSummary.Total)})
	writer.Write([]string{"Aktif", strconv.Itoa(data.TargetSummary.Active)})
	writer.Write([]string{"Pasif", strconv.Itoa(data.TargetSummary.Inactive)})
	writer.Write([]string{"Çevrim içi", strconv.Itoa(data.TargetSummary.Online)})
	writer.Write([]string{"Kapalı", strconv.Itoa(data.TargetSummary.Offline)})
	writer.Write([]string{"Veri Yok", strconv.Itoa(data.TargetSummary.Unknown)})
	writer.Write([]string{})

	if len(data.Targets) > 0 {
		writer.Write([]string{"Performans Detayları"})
		writer.Write([]string{"Hedef", "Adres", "Sağlık Skoru", "Derece", "Uptime %", "Kesinti Sayısı", "Downtime (dk)", "Ort. Yanıt (ms)", "Yanıt Performansı", "Min Yanıt (ms)", "Max Yanıt (ms)", "Durum"})
		for _, target := range data.Targets {
			if !target.Enabled {
				continue
			}

			downtimeStr := fmt.Sprintf("%d", target.SLAMetrics.DowntimeMinutes)
			if target.SLAMetrics.DowntimeMinutes >= 60 {
				hours := target.SLAMetrics.DowntimeMinutes / 60
				mins := target.SLAMetrics.DowntimeMinutes % 60
				downtimeStr = fmt.Sprintf("%d saat %d dk", hours, mins)
			}

			writer.Write([]string{
				target.Name,
				target.Address,
				fmt.Sprintf("%.0f", target.SLAMetrics.HealthScore),
				target.SLAMetrics.HealthGrade,
				fmt.Sprintf("%.2f", target.SLAMetrics.UptimePercent),
				strconv.Itoa(target.SLAMetrics.IncidentCount),
				downtimeStr,
				fmt.Sprintf("%.0f", target.SLAMetrics.AvgResponseMS),
				target.SLAMetrics.ResponseGrade,
				strconv.Itoa(target.SLAMetrics.MinResponseMS),
				strconv.Itoa(target.SLAMetrics.MaxResponseMS),
				target.Status,
			})
		}
		writer.Write([]string{})
	}

	writer.Write([]string{"Bildirim Özeti"})
	writer.Write([]string{"Toplam", strconv.Itoa(data.NotificationSummary.Total)})
	writer.Write([]string{"Başarılı", strconv.Itoa(data.NotificationSummary.Sent)})
	writer.Write([]string{"Hatalı", strconv.Itoa(data.NotificationSummary.Failed)})
	if len(data.NotificationSummary.Channels) > 0 {
		writer.Write([]string{})
		writer.Write([]string{"Kanal Dağılımı"})
		writer.Write([]string{"Kanal", "Adet"})
		keys := make([]string, 0, len(data.NotificationSummary.Channels))
		for channel := range data.NotificationSummary.Channels {
			keys = append(keys, channel)
		}
		sort.Strings(keys)
		for _, channel := range keys {
			writer.Write([]string{channel, strconv.Itoa(data.NotificationSummary.Channels[channel])})
		}
	}
	writer.Write([]string{})

	if len(data.Notifications) > 0 {
		writer.Write([]string{"Son Bildirimler"})
		writer.Write([]string{"Kanal", "Durum", "Başlık", "Hedef", "Gönderim", "Alıcılar", "Hata"})
		for _, notif := range data.Notifications {
			recipients := strings.Join(notif.Recipients, "; ")
			errorText := ""
			if notif.Error != nil {
				errorText = *notif.Error
			}
			writer.Write([]string{
				notif.Channel,
				notif.Status,
				notif.Subject,
				notif.TargetName,
				notif.SentAt,
				recipients,
				errorText,
			})
		}
		writer.Write([]string{})
	}

	writer.Write([]string{"Yedekleme Özeti"})
	writer.Write([]string{"Toplam", strconv.Itoa(data.BackupSummary.Total)})
	writer.Write([]string{"Başarılı", strconv.Itoa(data.BackupSummary.Success)})
	writer.Write([]string{"Hata", strconv.Itoa(data.BackupSummary.Error)})
	writer.Write([]string{})

	if len(data.Backups) > 0 {
		writer.Write([]string{"Son Yedekleme İşlemleri"})
		writer.Write([]string{"ID", "İşlem", "Durum", "Dosya", "Mesaj", "Kullanıcı", "Tarih"})
		for _, backup := range data.Backups {
			writer.Write([]string{
				strconv.Itoa(backup.ID),
				backup.Action,
				backup.Status,
				backup.FileName,
				backup.Message,
				backup.UserEmail,
				backup.CreatedAt,
			})
		}
	}

	writer.Flush()
	return builder.String()
}

func generateSystemReportHTML(title string, data SystemReportData) string {
	parseTime := func(val string) string {
		t, err := time.Parse(time.RFC3339, val)
		if err != nil {
			return val
		}
		return t.In(time.Local).Format("02.01.2006 15:04")
	}

	generatedAt := parseTime(data.GeneratedAt)
	dateRange := fmt.Sprintf("%s - %s", parseTime(data.DateRange.Start), parseTime(data.DateRange.End))

	builder := &strings.Builder{}
	builder.WriteString(`<!DOCTYPE html>
<html lang="tr">
<head>
    <meta charset="UTF-8">
    <title>`)
	builder.WriteString(html.EscapeString(title))
	builder.WriteString(`</title>
    <style>
        body { font-family: 'Segoe UI', Tahoma, Geneva, Verdana, sans-serif; margin: 0; background: #f4f6f9; color: #1f2937; }
        .container { max-width: 1200px; margin: 20px auto; background: white; border-radius: 12px; box-shadow: 0 6px 24px rgba(15, 23, 42, 0.08); overflow: hidden; }
        .header { padding: 32px; background: linear-gradient(120deg, #2563eb 0%, #7c3aed 100%); color: white; }
        .header h1 { margin: 0 0 8px 0; font-size: 28px; font-weight: 600; }
        .header p { margin: 4px 0; opacity: 0.9; font-size: 14px; }
        .content { padding: 32px; }
        .section { margin-bottom: 40px; }
        .section h2 { margin-bottom: 20px; font-size: 22px; font-weight: 600; color: #1f2937; border-bottom: 2px solid #e2e8f0; padding-bottom: 8px; }
        .summary-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(200px, 1fr)); gap: 16px; margin-top: 16px; }
        .card { background: #f8fafc; padding: 20px; border-radius: 10px; border: 1px solid #e2e8f0; transition: all 0.2s; }
        .card:hover { box-shadow: 0 4px 12px rgba(15, 23, 42, 0.12); transform: translateY(-2px); }
        .card h3 { margin: 0 0 8px 0; font-size: 13px; color: #64748b; text-transform: uppercase; letter-spacing: 0.05em; font-weight: 600; }
        .card p { margin: 0; font-size: 28px; font-weight: 700; color: #1f2937; }
        .health-card { background: linear-gradient(135deg, #f0f9ff 0%, #e0f2fe 100%); border: 2px solid #0ea5e9; }
        .health-score { font-size: 42px !important; color: #0369a1; line-height: 1; }
        .score-label { font-size: 18px; color: #64748b; font-weight: 400; }
        .health-badge { display: inline-block; margin-top: 8px; padding: 6px 14px; border-radius: 999px; font-size: 13px; font-weight: 700; text-transform: uppercase; }
        .health-badge.excellent { background: #dcfce7; color: #166534; }
        .health-badge.good { background: #dbeafe; color: #1e40af; }
        .health-badge.moderate { background: #fef3c7; color: #92400e; }
        .health-badge.poor { background: #fee2e2; color: #991b1b; }
        .alert-card { background: linear-gradient(135deg, #fef2f2 0%, #fee2e2 100%); border: 2px solid #f87171; }
        .alert-number { color: #dc2626 !important; }
        table { width: 100%; border-collapse: collapse; margin-top: 16px; border-radius: 10px; overflow: hidden; box-shadow: 0 2px 8px rgba(15, 23, 42, 0.1); font-size: 13px; }
        th, td { padding: 14px 12px; text-align: left; }
        th { background: #1e3a8a; color: white; font-weight: 600; letter-spacing: 0.03em; font-size: 12px; text-transform: uppercase; }
        tr:nth-child(even) { background: #f8fafc; }
        tr:hover { background: #eef2ff; }
        td { border-bottom: 1px solid #e2e8f0; }
        .badge { display: inline-block; padding: 5px 12px; border-radius: 999px; font-size: 11px; font-weight: 700; text-transform: uppercase; letter-spacing: 0.03em; }
        .badge-online { background: #dcfce7; color: #166534; }
        .badge-good { background: #dbeafe; color: #1e40af; }
        .badge-moderate { background: #fed7aa; color: #92400e; }
        .badge-offline { background: #fee2e2; color: #991b1b; }
        .badge-unknown { background: #fef9c3; color: #854d0e; }
        .badge-passive { background: #e2e8f0; color: #475569; }
        .pill { display: inline-block; padding: 6px 14px; border-radius: 999px; background: #e0e7ff; color: #3730a3; font-size: 12px; font-weight: 600; margin-right: 8px; margin-bottom: 6px; }
        .empty { padding: 24px; background: #f8fafc; border: 2px dashed #cbd5e1; border-radius: 10px; text-align: center; color: #64748b; margin-top: 16px; font-size: 14px; }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1>`)
	builder.WriteString(html.EscapeString(title))
	builder.WriteString(`</h1>
            <p>Oluşturulma: `)
	builder.WriteString(html.EscapeString(generatedAt))
	builder.WriteString(`</p>
            <p>Veri aralığı: `)
	builder.WriteString(html.EscapeString(dateRange))
	builder.WriteString(`</p>
        </div>
        <div class="content">`)

	// Sistem Sağlığı Bölümü
	builder.WriteString(`
            <div>
                <h2>📊 Sistem Sağlığı</h2>
                <div class="summary-grid">
                    <div class="card health-card">
                        <h3>Genel Sağlık Skoru</h3>
                        <p class="health-score">`)
	builder.WriteString(fmt.Sprintf("%.1f", data.SystemHealth.OverallScore))
	builder.WriteString(`<span class="score-label">/100</span></p>
                        <div class="health-badge `)
	healthClass := "excellent"
	if data.SystemHealth.OverallScore < 90 {
		healthClass = "good"
	}
	if data.SystemHealth.OverallScore < 75 {
		healthClass = "moderate"
	}
	if data.SystemHealth.OverallScore < 65 {
		healthClass = "poor"
	}
	builder.WriteString(healthClass)
	builder.WriteString(`">`)
	builder.WriteString(html.EscapeString(data.SystemHealth.HealthStatus))
	builder.WriteString(`</div>
                    </div>
                    <div class="card">
                        <h3>Toplam Uptime</h3>
                        <p>`)
	builder.WriteString(fmt.Sprintf("%.2f%%", data.SystemHealth.TotalUptime))
	builder.WriteString(`</p>
                    </div>
                    <div class="card">
                        <h3>Ortalama Yanıt Süresi</h3>
                        <p>`)
	builder.WriteString(fmt.Sprintf("%.0f ms", data.SystemHealth.AverageResponseMS))
	builder.WriteString(`</p>
                    </div>
                    <div class="card">
                        <h3>Toplam Kesinti</h3>
                        <p>`)
	builder.WriteString(strconv.Itoa(data.SystemHealth.TotalIncidents))
	builder.WriteString(`</p>
                    </div>
                    <div class="card alert-card">
                        <h3>Kritik Hedefler</h3>
                        <p class="alert-number">`)
	builder.WriteString(strconv.Itoa(data.SystemHealth.CriticalTargets))
	builder.WriteString(`</p>
                    </div>
                </div>
            </div>

            <div>
                <h2>🎯 Hedef Özeti</h2>
                <div class="summary-grid">
                    <div class="card">
                        <h3>Toplam Hedef</h3>
                        <p>`)
	builder.WriteString(strconv.Itoa(data.TargetSummary.Total))
	builder.WriteString(`</p>
                    </div>
                    <div class="card">
                        <h3>Çevrim içi</h3>
                        <p>`)
	builder.WriteString(strconv.Itoa(data.TargetSummary.Online))
	builder.WriteString(`</p>
                    </div>
                    <div class="card">
                        <h3>Kapalı</h3>
                        <p>`)
	builder.WriteString(strconv.Itoa(data.TargetSummary.Offline))
	builder.WriteString(`</p>
                    </div>
                    <div class="card">
                        <h3>Veri Yok</h3>
                        <p>`)
	builder.WriteString(strconv.Itoa(data.TargetSummary.Unknown))
	builder.WriteString(`</p>
                    </div>
                </div>
            </div>`)

	// Performans Detayları
	if len(data.Targets) > 0 {
		builder.WriteString(`

            <div class="section section-table">
                <h2>📈 Performans Detayları</h2>
                <table>
                    <thead>
                        <tr>
                            <th>Hedef</th>
                            <th>Sağlık</th>
                            <th>Uptime</th>
                            <th>Kesinti</th>
                            <th>Downtime</th>
                            <th>Ort. Yanıt</th>
                            <th>Yanıt Performansı</th>
                            <th>Min/Max</th>
                            <th>Durum</th>
                        </tr>
                    </thead>
                    <tbody>`)
		for _, target := range data.Targets {
			if !target.Enabled {
				continue
			}

			// Sağlık badge class
			healthBadge := "badge-online"
			if target.SLAMetrics.HealthScore < 95 {
				healthBadge = "badge-good"
			}
			if target.SLAMetrics.HealthScore < 85 {
				healthBadge = "badge-moderate"
			}
			if target.SLAMetrics.HealthScore < 70 {
				healthBadge = "badge-offline"
			}

			// Status badge class
			statusClass := "badge-online"
			switch target.Status {
			case "Kapalı":
				statusClass = "badge-offline"
			case "Veri Yok":
				statusClass = "badge-unknown"
			case "Pasif":
				statusClass = "badge-passive"
			}

			builder.WriteString(`
                        <tr>
                            <td><strong>`)
			builder.WriteString(html.EscapeString(target.Name))
			builder.WriteString(`</strong><br><small style="color: #64748b;">`)
			builder.WriteString(html.EscapeString(target.Address))
			builder.WriteString(`</small></td>
                            <td>
                                <span class="badge ` + healthBadge + `">`)
			builder.WriteString(fmt.Sprintf("%.0f", target.SLAMetrics.HealthScore))
			builder.WriteString(`</span><br>
                                <small>`)
			builder.WriteString(html.EscapeString(target.SLAMetrics.HealthGrade))
			builder.WriteString(`</small>
                            </td>
                            <td><strong>`)
			builder.WriteString(fmt.Sprintf("%.2f%%", target.SLAMetrics.UptimePercent))
			builder.WriteString(`</strong></td>
                            <td>`)
			builder.WriteString(strconv.Itoa(target.SLAMetrics.IncidentCount))
			builder.WriteString(` kez</td>
                            <td>`)
			if target.SLAMetrics.DowntimeMinutes >= 60 {
				hours := target.SLAMetrics.DowntimeMinutes / 60
				mins := target.SLAMetrics.DowntimeMinutes % 60
				builder.WriteString(fmt.Sprintf("%ds %ddk", hours, mins))
			} else {
				builder.WriteString(fmt.Sprintf("%d dk", target.SLAMetrics.DowntimeMinutes))
			}
			builder.WriteString(`</td>
                            <td>`)
			builder.WriteString(fmt.Sprintf("%.0f ms", target.SLAMetrics.AvgResponseMS))
			builder.WriteString(`</td>
                            <td><span class="pill">`)
			builder.WriteString(html.EscapeString(target.SLAMetrics.ResponseGrade))
			builder.WriteString(`</span></td>
                            <td><small>`)
			builder.WriteString(fmt.Sprintf("%d / %d", target.SLAMetrics.MinResponseMS, target.SLAMetrics.MaxResponseMS))
			builder.WriteString(`</small></td>
                            <td><span class="badge ` + statusClass + `">`)
			builder.WriteString(html.EscapeString(target.Status))
			builder.WriteString(`</span></td>
                        </tr>`)
		}
		builder.WriteString(`
                    </tbody>
                </table>`)
	} else {
		builder.WriteString(`<div class="empty">Seçilen aralıkta hedef bilgisi bulunamadı.</div>`)
	}
	builder.WriteString(`</div>`)

	builder.WriteString(`
            <div class="section section-table">
                <h2>📧 Bildirim Özeti</h2>
                <div class="summary-grid">
                    <div class="card">
                        <h3>Toplam</h3>
                        <p>`)
	builder.WriteString(strconv.Itoa(data.NotificationSummary.Total))
	builder.WriteString(`</p>
                    </div>
                    <div class="card">
                        <h3>Başarılı</h3>
                        <p>`)
	builder.WriteString(strconv.Itoa(data.NotificationSummary.Sent))
	builder.WriteString(`</p>
                    </div>
                    <div class="card">
                        <h3>Hatalı</h3>
                        <p>`)
	builder.WriteString(strconv.Itoa(data.NotificationSummary.Failed))
	builder.WriteString(`</p>
                    </div>
                </div>`)

	if len(data.NotificationSummary.Channels) > 0 {
		builder.WriteString(`<div style="margin-top: 16px;">`)
		keys := make([]string, 0, len(data.NotificationSummary.Channels))
		for channel := range data.NotificationSummary.Channels {
			keys = append(keys, channel)
		}
		sort.Strings(keys)
		for _, channel := range keys {
			builder.WriteString(`<span class="pill">`)
			builder.WriteString(html.EscapeString(channel))
			builder.WriteString(`: `)
			builder.WriteString(strconv.Itoa(data.NotificationSummary.Channels[channel]))
			builder.WriteString(`</span>`)
		}
		builder.WriteString(`</div>`)
	}

	if len(data.Notifications) > 0 {
		builder.WriteString(`
                <table>
                    <thead>
                        <tr>
                            <th>Kanal</th>
                            <th>Durum</th>
                            <th>Başlık</th>
                            <th>Hedef</th>
                            <th>Gönderim</th>
                            <th>Alıcılar</th>
                            <th>Hata</th>
                        </tr>
                    </thead>
                    <tbody>`)
		for _, notif := range data.Notifications {
			recipients := "-"
			if len(notif.Recipients) > 0 {
				recipients = html.EscapeString(strings.Join(notif.Recipients, ", "))
			}
			errorText := ""
			if notif.Error != nil {
				errorText = html.EscapeString(*notif.Error)
			}
			builder.WriteString(`
                        <tr>
                            <td>`)
			builder.WriteString(html.EscapeString(notif.Channel))
			builder.WriteString(`</td>
                            <td>`)
			builder.WriteString(html.EscapeString(notif.Status))
			builder.WriteString(`</td>
                            <td>`)
			builder.WriteString(html.EscapeString(notif.Subject))
			builder.WriteString(`</td>
                            <td>`)
			builder.WriteString(html.EscapeString(notif.TargetName))
			builder.WriteString(`</td>
                            <td>`)
			builder.WriteString(html.EscapeString(notif.SentAt))
			builder.WriteString(`</td>
                            <td>`)
			builder.WriteString(recipients)
			builder.WriteString(`</td>
                            <td>`)
			builder.WriteString(errorText)
			builder.WriteString(`</td>
                        </tr>`)
		}
		builder.WriteString(`
                    </tbody>
                </table>`)
	} else {
		builder.WriteString(`<div class="empty">Seçilen aralıkta bildirim bulunamadı.</div>`)
	}
	builder.WriteString(`</div>`)

	builder.WriteString(`
            <div class="section section-table">
                <h2>💾 Yedekleme Faaliyetleri</h2>
                <div class="summary-grid">
                    <div class="card">
                        <h3>Toplam</h3>
                        <p>`)
	builder.WriteString(strconv.Itoa(data.BackupSummary.Total))
	builder.WriteString(`</p>
                    </div>
                    <div class="card">
                        <h3>Başarılı</h3>
                        <p>`)
	builder.WriteString(strconv.Itoa(data.BackupSummary.Success))
	builder.WriteString(`</p>
                    </div>
                    <div class="card">
                        <h3>Hatalı</h3>
                        <p>`)
	builder.WriteString(strconv.Itoa(data.BackupSummary.Error))
	builder.WriteString(`</p>
                    </div>
                </div>`)

	if len(data.Backups) > 0 {
		builder.WriteString(`
                <table>
                    <thead>
                        <tr>
                            <th>ID</th>
                            <th>İşlem</th>
                            <th>Durum</th>
                            <th>Dosya</th>
                            <th>Mesaj</th>
                            <th>Kullanıcı</th>
                            <th>Tarih</th>
                        </tr>
                    </thead>
                    <tbody>`)
		for _, backup := range data.Backups {
			builder.WriteString(`
                        <tr>
                            <td>`)
			builder.WriteString(strconv.Itoa(backup.ID))
			builder.WriteString(`</td>
                            <td>`)
			builder.WriteString(html.EscapeString(backup.Action))
			builder.WriteString(`</td>
                            <td>`)
			builder.WriteString(html.EscapeString(backup.Status))
			builder.WriteString(`</td>
                            <td>`)
			builder.WriteString(html.EscapeString(backup.FileName))
			builder.WriteString(`</td>
                            <td>`)
			builder.WriteString(html.EscapeString(backup.Message))
			builder.WriteString(`</td>
                            <td>`)
			builder.WriteString(html.EscapeString(backup.UserEmail))
			builder.WriteString(`</td>
                            <td>`)
			builder.WriteString(html.EscapeString(backup.CreatedAt))
			builder.WriteString(`</td>
                        </tr>`)
		}
		builder.WriteString(`
                    </tbody>
                </table>`)
	} else {
		builder.WriteString(`<div class="empty">Seçilen aralıkta yedekleme kaydı bulunamadı.</div>`)
	}

	builder.WriteString(`
            </div>
        </div>
    </div>
</body>
</html>`)

	return builder.String()
}

// generatePDFReport - Profesyonel PDF raporu oluşturur
func generatePDFReport(data SystemReportData, title string, start, end time.Time) ([]byte, error) {
	// PDF oluştur - UTF-8 desteği ile
	pdf := gofpdf.New("P", "mm", "A4", "")
	if pdf == nil {
		return nil, fmt.Errorf("PDF nesnesi oluşturulamadı")
	}

	// UTF-8 desteği için translator ekle - cp1254 Türkçe kod sayfası
	tr := pdf.UnicodeTranslatorFromDescriptor("cp1254")

	pdf.SetMargins(15, 15, 15)
	pdf.SetAutoPageBreak(true, 15)

	// Sayfa 1: Genel Özet
	pdf.AddPage()

	// Hata kontrolü
	if pdf.Error() != nil {
		return nil, fmt.Errorf("sayfa eklenemedi: %v", pdf.Error())
	}

	// Başlık
	pdf.SetFont("Arial", "B", 16)
	pdf.SetTextColor(37, 99, 235) // Mavi
	pdf.CellFormat(0, 10, tr(title), "", 1, "C", false, 0, "")
	pdf.Ln(5)

	// Tarih aralığı
	pdf.SetFont("Arial", "", 10)
	pdf.SetTextColor(100, 100, 100)
	dateRange := fmt.Sprintf("Veri aralığı: %s - %s", start.Format("02.01.2006"), end.Format("02.01.2006"))
	pdf.CellFormat(0, 6, tr(dateRange), "", 1, "C", false, 0, "")
	pdf.Ln(10)

	// Sistem Sağlığı
	pdf.SetFont("Arial", "B", 14)
	pdf.SetTextColor(0, 0, 0)
	pdf.CellFormat(0, 8, tr("Sistem Sağlığı"), "", 1, "L", false, 0, "")
	pdf.Ln(3)

	// Sağlık kartları (2x3 grid)
	currentY := pdf.GetY()
	drawCard(pdf, tr, 15, currentY, 60, 30, "Genel Sağlık Skoru", fmt.Sprintf("%.1f/100", data.SystemHealth.OverallScore), data.SystemHealth.HealthStatus)
	drawCard(pdf, tr, 80, currentY, 60, 30, "Toplam Uptime", fmt.Sprintf("%.2f%%", data.SystemHealth.TotalUptime), "")
	drawCard(pdf, tr, 145, currentY, 50, 30, "Ortalama Yanıt", fmt.Sprintf("%.0f ms", data.SystemHealth.AverageResponseMS), "")

	pdf.SetY(currentY + 32)
	currentY = pdf.GetY()
	drawCard(pdf, tr, 15, currentY, 60, 30, "Toplam Kesinti", fmt.Sprintf("%d", data.SystemHealth.TotalIncidents), "")
	drawCard(pdf, tr, 80, currentY, 60, 30, "Kritik Hedefler", fmt.Sprintf("%d", data.SystemHealth.CriticalTargets), "alert")

	pdf.SetY(currentY + 35)

	// Hedef Özeti
	pdf.SetFont("Arial", "B", 14)
	pdf.SetTextColor(0, 0, 0)
	pdf.CellFormat(0, 8, tr("Hedef Özeti"), "", 1, "L", false, 0, "")
	pdf.Ln(3)

	currentY = pdf.GetY()
	drawCard(pdf, tr, 15, currentY, 45, 25, "Toplam", fmt.Sprintf("%d", data.TargetSummary.Total), "")
	drawCard(pdf, tr, 65, currentY, 45, 25, "Online", fmt.Sprintf("%d", data.TargetSummary.Online), "")
	drawCard(pdf, tr, 115, currentY, 40, 25, "Kapalı", fmt.Sprintf("%d", data.TargetSummary.Offline), "")
	drawCard(pdf, tr, 160, currentY, 35, 25, "Veri Yok", fmt.Sprintf("%d", data.TargetSummary.Unknown), "")

	pdf.SetY(currentY + 28)

	// Performans Detayları - Yeni sayfa, landscape
	if len(data.Targets) > 0 {
		pdf.AddPage()
		pdf.SetFont("Arial", "B", 14)
		pdf.CellFormat(0, 8, tr("Performans Detayları"), "", 1, "L", false, 0, "")
		pdf.Ln(3)

		// Tablo başlıkları
		pdf.SetFont("Arial", "B", 8)
		pdf.SetFillColor(37, 99, 235)
		pdf.SetTextColor(255, 255, 255)

		headers := []string{"Hedef", "Sağlık", "Uptime", "Kesinti", "Downtime", "Ort. Yanıt", "Min", "Max", "Durum"}
		widths := []float64{40, 15, 18, 15, 18, 18, 15, 15, 26}

		for i, header := range headers {
			pdf.CellFormat(widths[i], 7, tr(header), "1", 0, "C", true, 0, "")
		}
		pdf.Ln(-1)

		// Tablo içeriği
		pdf.SetFont("Arial", "", 7)
		pdf.SetTextColor(0, 0, 0)

		for idx, target := range data.Targets {
			if !target.Enabled {
				continue
			}

			// Zemin rengi (alternatif satırlar)
			if idx%2 == 0 {
				pdf.SetFillColor(249, 250, 251)
			} else {
				pdf.SetFillColor(255, 255, 255)
			}

			pdf.CellFormat(widths[0], 6, tr(target.Name), "1", 0, "L", true, 0, "")
			pdf.CellFormat(widths[1], 6, fmt.Sprintf("%.0f", target.SLAMetrics.HealthScore), "1", 0, "C", true, 0, "")
			pdf.CellFormat(widths[2], 6, fmt.Sprintf("%.1f%%", target.SLAMetrics.UptimePercent), "1", 0, "C", true, 0, "")
			pdf.CellFormat(widths[3], 6, fmt.Sprintf("%d", target.SLAMetrics.FailedPings), "1", 0, "C", true, 0, "")
			pdf.CellFormat(widths[4], 6, fmt.Sprintf("%d dk", target.SLAMetrics.DowntimeMinutes), "1", 0, "C", true, 0, "")
			pdf.CellFormat(widths[5], 6, fmt.Sprintf("%.0f ms", target.SLAMetrics.AvgResponseMS), "1", 0, "C", true, 0, "")
			pdf.CellFormat(widths[6], 6, fmt.Sprintf("%d", target.SLAMetrics.MinResponseMS), "1", 0, "C", true, 0, "")
			pdf.CellFormat(widths[7], 6, fmt.Sprintf("%d", target.SLAMetrics.MaxResponseMS), "1", 0, "C", true, 0, "")
			pdf.CellFormat(widths[8], 6, tr(target.Status), "1", 0, "C", true, 0, "")
			pdf.Ln(-1)
		}
	}

	// Bildirim Özeti - Yeni sayfa
	pdf.AddPage()
	pdf.SetFont("Arial", "B", 14)
	pdf.SetTextColor(0, 0, 0)
	pdf.CellFormat(0, 8, tr("Bildirim Özeti"), "", 1, "L", false, 0, "")
	pdf.Ln(3)

	currentY = pdf.GetY()
	drawCard(pdf, tr, 15, currentY, 55, 25, "Toplam", fmt.Sprintf("%d", data.NotificationSummary.Total), "")
	drawCard(pdf, tr, 75, currentY, 55, 25, "Başarılı", fmt.Sprintf("%d", data.NotificationSummary.Sent), "")
	drawCard(pdf, tr, 135, currentY, 55, 25, "Hatalı", fmt.Sprintf("%d", data.NotificationSummary.Failed), "")

	pdf.SetY(currentY + 28)

	// Bildirim detayları tablosu
	if len(data.Notifications) > 0 {
		pdf.Ln(5)
		pdf.SetFont("Arial", "B", 12)
		pdf.SetTextColor(0, 0, 0)
		pdf.CellFormat(0, 6, tr("Bildirim Detayları"), "", 1, "L", false, 0, "")
		pdf.Ln(2)

		// Tablo başlıkları
		pdf.SetFont("Arial", "B", 8)
		pdf.SetFillColor(37, 99, 235)
		pdf.SetTextColor(255, 255, 255)

		notifHeaders := []string{"Kanal", "Durum", "Konu", "Hedef", "Tarih"}
		notifWidths := []float64{25, 20, 60, 50, 25}

		for i, header := range notifHeaders {
			pdf.CellFormat(notifWidths[i], 7, tr(header), "1", 0, "C", true, 0, "")
		}
		pdf.Ln(-1)

		// Tablo içeriği
		pdf.SetFont("Arial", "", 7)
		pdf.SetTextColor(0, 0, 0)

		displayCount := 0
		for idx, notif := range data.Notifications {
			if displayCount >= 15 { // Maksimum 15 kayıt göster
				break
			}

			// Zemin rengi
			if idx%2 == 0 {
				pdf.SetFillColor(249, 250, 251)
			} else {
				pdf.SetFillColor(255, 255, 255)
			}

			// Konu ve hedef adını kısalt
			subject := notif.Subject
			if len(subject) > 40 {
				subject = subject[:37] + "..."
			}
			target := notif.TargetName
			if len(target) > 30 {
				target = target[:27] + "..."
			}

			pdf.CellFormat(notifWidths[0], 6, tr(notif.Channel), "1", 0, "L", true, 0, "")
			pdf.CellFormat(notifWidths[1], 6, tr(notif.Status), "1", 0, "C", true, 0, "")
			pdf.CellFormat(notifWidths[2], 6, tr(subject), "1", 0, "L", true, 0, "")
			pdf.CellFormat(notifWidths[3], 6, tr(target), "1", 0, "L", true, 0, "")
			pdf.CellFormat(notifWidths[4], 6, notif.SentAt, "1", 0, "C", true, 0, "")
			pdf.Ln(-1)

			displayCount++
		}
	}

	// Yedekleme - Yeni sayfa
	pdf.AddPage()
	pdf.SetFont("Arial", "B", 14)
	pdf.CellFormat(0, 8, tr("Yedekleme Faaliyetleri"), "", 1, "L", false, 0, "")
	pdf.Ln(3)

	currentY = pdf.GetY()
	drawCard(pdf, tr, 15, currentY, 55, 25, "Toplam", fmt.Sprintf("%d", data.BackupSummary.Total), "")
	drawCard(pdf, tr, 75, currentY, 55, 25, "Başarılı", fmt.Sprintf("%d", data.BackupSummary.Success), "")
	drawCard(pdf, tr, 135, currentY, 55, 25, "Hatalı", fmt.Sprintf("%d", data.BackupSummary.Error), "")

	pdf.SetY(currentY + 28)

	// Yedekleme detayları tablosu
	if len(data.Backups) > 0 {
		pdf.Ln(5)
		pdf.SetFont("Arial", "B", 12)
		pdf.SetTextColor(0, 0, 0)
		pdf.CellFormat(0, 6, tr("Yedekleme Detayları"), "", 1, "L", false, 0, "")
		pdf.Ln(2)

		// Tablo başlıkları
		pdf.SetFont("Arial", "B", 8)
		pdf.SetFillColor(37, 99, 235)
		pdf.SetTextColor(255, 255, 255)

		backupHeaders := []string{"İşlem", "Durum", "Dosya Adı", "Kullanıcı", "Tarih"}
		backupWidths := []float64{25, 20, 65, 35, 35}

		for i, header := range backupHeaders {
			pdf.CellFormat(backupWidths[i], 7, tr(header), "1", 0, "C", true, 0, "")
		}
		pdf.Ln(-1)

		// Tablo içeriği
		pdf.SetFont("Arial", "", 7)
		pdf.SetTextColor(0, 0, 0)

		displayCount := 0
		for idx, backup := range data.Backups {
			if displayCount >= 15 { // Maksimum 15 kayıt göster
				break
			}

			// Zemin rengi
			if idx%2 == 0 {
				pdf.SetFillColor(249, 250, 251)
			} else {
				pdf.SetFillColor(255, 255, 255)
			}

			// Dosya adını kısalt
			filename := backup.FileName
			if len(filename) > 40 {
				filename = filename[:37] + "..."
			}

			// Kullanıcı email'ini kısalt
			userEmail := backup.UserEmail
			if len(userEmail) > 25 {
				userEmail = userEmail[:22] + "..."
			}

			pdf.CellFormat(backupWidths[0], 6, tr(backup.Action), "1", 0, "L", true, 0, "")
			pdf.CellFormat(backupWidths[1], 6, tr(backup.Status), "1", 0, "C", true, 0, "")
			pdf.CellFormat(backupWidths[2], 6, tr(filename), "1", 0, "L", true, 0, "")
			pdf.CellFormat(backupWidths[3], 6, tr(userEmail), "1", 0, "L", true, 0, "")
			pdf.CellFormat(backupWidths[4], 6, backup.CreatedAt, "1", 0, "C", true, 0, "")
			pdf.Ln(-1)

			displayCount++
		}
	}

	// PDF oluşturma sırasında hata oldu mu?
	if pdf.Error() != nil {
		return nil, fmt.Errorf("PDF oluşturma hatası: %v", pdf.Error())
	}

	// PDF'i byte array'e dönüştür
	var buf bytes.Buffer
	err := pdf.Output(&buf)
	if err != nil {
		return nil, fmt.Errorf("PDF çıktısı alınırken hata: %v", err)
	}

	if buf.Len() == 0 {
		return nil, fmt.Errorf("Boş PDF oluşturuldu")
	}

	return buf.Bytes(), nil
}

// drawCard - PDF'e kart çizer
func drawCard(pdf *gofpdf.Fpdf, tr func(string) string, x, y, w, h float64, title, value, badge string) {
	if pdf == nil {
		return
	}

	// Kart arka planı
	pdf.SetFillColor(255, 255, 255)
	pdf.SetDrawColor(229, 231, 235)
	pdf.SetLineWidth(0.5)
	pdf.Rect(x, y, w, h, "D")

	// Başlık
	pdf.SetXY(x+2, y+2)
	pdf.SetFont("Arial", "", 8)
	pdf.SetTextColor(107, 114, 128)
	pdf.CellFormat(w-4, 5, tr(title), "", 0, "L", false, 0, "")

	// Değer
	pdf.SetXY(x+2, y+10)
	pdf.SetFont("Arial", "B", 12)
	pdf.SetTextColor(17, 24, 39)
	pdf.CellFormat(w-4, 6, tr(value), "", 0, "L", false, 0, "")

	// Badge varsa
	if badge != "" {
		pdf.SetXY(x+2, y+h-7)
		pdf.SetFont("Arial", "B", 7)

		if badge == "alert" {
			pdf.SetTextColor(220, 38, 38)
			pdf.CellFormat(w-4, 5, tr("KRİTİK"), "", 0, "L", false, 0, "")
		} else {
			pdf.SetTextColor(34, 197, 94)
			pdf.CellFormat(w-4, 5, tr(badge), "", 0, "L", false, 0, "")
		}
	}
}

// turkishToASCII - Artık dönüşüm yapmıyor, string'i olduğu gibi döndürüyor
// UTF-8 desteği PDF seviyesinde TranslatorFromDescriptor ile sağlanıyor
func tr(s string) string {
	// String'i olduğu gibi döndür, dönüşüm PDF'de yapılacak
	return s
}

// formatDuration - Saniyeyi okunabilir formata çevirir
func formatDuration(seconds int64) string {
	if seconds < 60 {
		return fmt.Sprintf("%ds", seconds)
	}
	minutes := seconds / 60
	if minutes < 60 {
		return fmt.Sprintf("%dm", minutes)
	}
	hours := minutes / 60
	return fmt.Sprintf("%dh", hours)
}

// generatePrintableHTML - HTML içeriğini tarayıcıda otomatik print dialog açacak şekilde hazırlar
func generatePrintableHTML(htmlContent, title string) string {
	// HTML içeriğine print için özel CSS ekle
	printCSS := `
    <style>
        /* Genel print ayarları */
        @media print {
            @page {
                size: A4 landscape;
                margin: 10mm;
            }

            body {
                margin: 0;
                padding: 10mm;
                -webkit-print-color-adjust: exact;
                print-color-adjust: exact;
                font-size: 9pt;
            }

            .no-print {
                display: none !important;
            }

            /* Container genişliği */
            .container {
                max-width: 100% !important;
                margin: 0 !important;
                box-shadow: none !important;
                page-break-inside: avoid;
            }

            /* Header küçült */
            .header {
                padding: 15px 20px !important;
                page-break-inside: avoid;
            }

            .header h1 {
                font-size: 18pt !important;
                margin: 0 0 5px 0 !important;
            }

            .header p {
                font-size: 9pt !important;
                margin: 2px 0 !important;
            }

            /* Content padding ayarla */
            .content {
                padding: 15px 20px !important;
            }

            /* İlk sayfa - Özetler yatay yerleşim */
            .summary-grid {
                display: grid !important;
                grid-template-columns: repeat(5, 1fr) !important;
                gap: 10px !important;
                page-break-inside: avoid !important;
                margin-bottom: 15px !important;
            }

            /* Kartları küçült */
            .card {
                padding: 10px 12px !important;
                margin: 0 !important;
                page-break-inside: avoid !important;
            }

            .card h3 {
                font-size: 8pt !important;
                margin: 0 0 5px 0 !important;
            }

            .card p {
                font-size: 16pt !important;
                margin: 0 !important;
            }

            .health-score {
                font-size: 20pt !important;
            }

            .score-label {
                font-size: 12pt !important;
            }

            /* Section başlıkları */
            .section h2 {
                font-size: 12pt !important;
                margin: 10px 0 10px 0 !important;
                padding-bottom: 5px !important;
                page-break-after: avoid !important;
            }

            /* İlk 2 section (Sistem Sağlığı ve Hedef Özeti) aynı sayfada */
            .content > div:nth-child(1),
            .content > div:nth-child(2) {
                page-break-after: avoid !important;
                page-break-inside: avoid !important;
            }

            /* 3. section'dan itibaren yeni sayfa */
            .content > div:nth-child(3) {
                page-break-before: always !important;
            }

            /* Tablolar */
            table {
                width: 100% !important;
                font-size: 7pt !important;
                page-break-inside: avoid !important;
                margin-top: 5px !important;
                border-collapse: collapse !important;
            }

            table th {
                font-size: 7pt !important;
                padding: 6px 4px !important;
                background-color: #1e3a8a !important;
                color: white !important;
            }

            table td {
                font-size: 7pt !important;
                padding: 5px 4px !important;
                word-wrap: break-word !important;
            }

            /* Section başlığı ve tablosu birlikte kalsın */
            .section {
                page-break-inside: avoid !important;
            }

            /* Tablo başlığı ile tablo arasında sayfa kırılmasın */
            h2 + table {
                page-break-before: avoid !important;
            }

            /* Badge'ler küçült */
            .badge, .health-badge, .pill {
                font-size: 7pt !important;
                padding: 3px 8px !important;
            }

            /* Boş alanlar */
            .empty {
                font-size: 9pt !important;
                padding: 15px !important;
            }
        }

        /* Ekran görünümü için */
        @media screen {
            body {
                background: #f0f0f0;
            }

            #print-button {
                position: fixed;
                top: 20px;
                right: 20px;
                z-index: 1000;
                padding: 15px 30px;
                background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
                color: white;
                border: none;
                border-radius: 8px;
                font-size: 16px;
                font-weight: bold;
                cursor: pointer;
                box-shadow: 0 4px 15px rgba(102, 126, 234, 0.4);
                transition: all 0.3s;
            }

            #print-button:hover {
                transform: translateY(-2px);
                box-shadow: 0 6px 20px rgba(102, 126, 234, 0.6);
            }
        }
    </style>`

	// HTML içeriğine CSS'i ekle (head tag'inden önce)
	htmlWithCSS := strings.Replace(htmlContent, "</head>", printCSS+"</head>", 1)

	// Print butonu ekle (body'nin başına)
	printButton := `
    <button id="print-button" class="no-print" onclick="window.print()">
        🖨️ PDF Olarak Kaydet
    </button>`

	htmlWithButton := strings.Replace(htmlWithCSS, "<body>", "<body>"+printButton, 1)

	return htmlWithButton
}

func convertHTMLToPDF(htmlContent string) ([]byte, error) {
	// HTML içeriğini parse etmek yerine, veriyi direkt PDF'e yazacağız
	// Bu daha basit ve harici bağımlılık gerektirmez
	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(10, 10, 10)
	pdf.AddPage()

	// Başlık
	pdf.SetFont("Arial", "B", 16)
	pdf.Cell(0, 10, "Sistem Raporu")
	pdf.Ln(12)

	// HTML içinden sadece text çıkaralım
	text := stripHTMLTags(htmlContent)

	// Normal metin
	pdf.SetFont("Arial", "", 10)
	pdf.MultiCell(0, 5, text, "", "", false)

	// PDF'i buffer'a yaz
	var buf strings.Builder
	err := pdf.Output(&buf)
	if err != nil {
		return nil, fmt.Errorf("failed to create PDF: %v", err)
	}

	return []byte(buf.String()), nil
}

// stripHTMLTags basit HTML tag temizleyici
func stripHTMLTags(htmlContent string) string {
	// HTML taglerini temizle
	text := htmlContent

	// Script ve style taglerini tamamen kaldır
	text = removeTag(text, "script")
	text = removeTag(text, "style")

	// Diğer HTML taglerini kaldır
	inTag := false
	var result strings.Builder
	for _, char := range text {
		if char == '<' {
			inTag = true
			continue
		}
		if char == '>' {
			inTag = false
			continue
		}
		if !inTag {
			result.WriteRune(char)
		}
	}

	// Fazla boşlukları temizle
	cleaned := result.String()
	cleaned = strings.ReplaceAll(cleaned, "\n\n\n", "\n\n")
	cleaned = strings.TrimSpace(cleaned)

	return cleaned
}

func removeTag(content, tag string) string {
	openTag := "<" + tag
	closeTag := "</" + tag + ">"

	for {
		start := strings.Index(content, openTag)
		if start == -1 {
			break
		}
		end := strings.Index(content[start:], closeTag)
		if end == -1 {
			break
		}
		content = content[:start] + content[start+end+len(closeTag):]
	}
	return content
}

// fetchAllTargetSLAMetrics tüm hedeflerin SLA metriklerini önceden hesaplanmış
// günlük özet tablosundan (ping_daily_stats) okur.
//
// Eskiden bu fonksiyon rapor anında pings_raw'daki milyonlarca ham satırı
// (30 günde ~4.7M) pencere fonksiyonlarıyla tarıyordu; Raspberry Pi üzerinde
// bu dakikalar/onlarca dakika sürüyordu. Artık rapor yalnızca gün bazında
// önceden toplanmış (hedef × gün) satırları okur; günlük özet bakımını arka
// planda ping.Cleaner yapar. İlk çağrıda eksik günler ping.EnsureRollupForRange
// ile doldurulur (bir kerelik), sonrası anlıktır.
func fetchAllTargetSLAMetrics(db *sql.DB, start, end time.Time, targetIDs []int) (map[int]TargetSLAMetrics, error) {
	result := make(map[int]TargetSLAMetrics)

	// Rapor aralığındaki eksik günlük özetleri garanti altına al.
	if err := ping.EnsureRollupForRange(db, start, end); err != nil {
		return nil, fmt.Errorf("günlük özet hazırlanamadı: %w", err)
	}

	where := "WHERE day BETWEEN ? AND ?"
	args := []interface{}{start.Format("2006-01-02"), end.Format("2006-01-02")}
	if clause, ids := buildInClause("target_id", targetIDs); clause != "" {
		where += " AND " + clause
		args = append(args, ids...)
	}

	query := `
		SELECT
			target_id,
			SUM(total)       AS total,
			SUM(successful)  AS successful,
			SUM(failed)      AS failed,
			SUM(sum_rtt_ms)  AS sum_rtt_ms,
			SUM(rtt_count)   AS rtt_count,
			MIN(min_rtt_ms)  AS min_rtt,
			MAX(max_rtt_ms)  AS max_rtt,
			SUM(incidents)   AS incidents,
			SUM(downtime_ms) AS downtime_ms
		FROM ping_daily_stats
		` + where + `
		GROUP BY target_id
	`

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var (
			targetID   int
			total      sql.NullInt64
			successful sql.NullInt64
			failed     sql.NullInt64
			sumRtt     sql.NullInt64
			rttCount   sql.NullInt64
			minRtt     sql.NullInt64
			maxRtt     sql.NullInt64
			incidents  sql.NullInt64
			downtimeMS sql.NullInt64
		)
		if err := rows.Scan(&targetID, &total, &successful, &failed, &sumRtt, &rttCount, &minRtt, &maxRtt, &incidents, &downtimeMS); err != nil {
			return nil, err
		}

		metrics := TargetSLAMetrics{
			TotalPings:      int(total.Int64),
			SuccessfulPings: int(successful.Int64),
			FailedPings:     int(failed.Int64),
			IncidentCount:   int(incidents.Int64),
		}
		if rttCount.Valid && rttCount.Int64 > 0 {
			metrics.AvgResponseMS = float64(sumRtt.Int64) / float64(rttCount.Int64)
		}
		if minRtt.Valid {
			metrics.MinResponseMS = int(minRtt.Int64)
		}
		if maxRtt.Valid {
			metrics.MaxResponseMS = int(maxRtt.Int64)
		}
		if metrics.TotalPings > 0 {
			metrics.UptimePercent = (float64(metrics.SuccessfulPings) / float64(metrics.TotalPings)) * 100
		}
		if downtimeMS.Valid && downtimeMS.Int64 > 0 {
			metrics.DowntimeMinutes = int(downtimeMS.Int64 / (1000 * 60))
		}
		// Her kesinti en az 1 dakika sayılır (eski davranışla uyumlu).
		if metrics.IncidentCount > 0 && metrics.DowntimeMinutes < metrics.IncidentCount {
			metrics.DowntimeMinutes = metrics.IncidentCount
		}

		metrics.ResponseGrade = getResponseGrade(metrics.AvgResponseMS)
		metrics.HealthScore = calculateHealthScore(metrics.UptimePercent, metrics.AvgResponseMS, metrics.IncidentCount)
		metrics.HealthGrade = getHealthGrade(metrics.HealthScore)

		result[targetID] = metrics
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return result, nil
}

// calculateHealthScore - Uptime, response time ve incident count'a göre sağlık skoru hesaplar
func calculateHealthScore(uptimePercent, avgResponseMS float64, incidentCount int) float64 {
	// Uptime skoru (60% ağırlık)
	uptimeScore := uptimePercent * 0.6

	// Response time skoru (25% ağırlık)
	responseScore := 25.0
	if avgResponseMS > 0 {
		if avgResponseMS < 50 {
			responseScore = 25.0
		} else if avgResponseMS < 100 {
			responseScore = 20.0
		} else if avgResponseMS < 200 {
			responseScore = 15.0
		} else if avgResponseMS < 500 {
			responseScore = 10.0
		} else {
			responseScore = 5.0
		}
	}

	// Incident skoru (15% ağırlık)
	incidentScore := 15.0
	if incidentCount > 0 {
		if incidentCount <= 2 {
			incidentScore = 15.0
		} else if incidentCount <= 5 {
			incidentScore = 10.0
		} else if incidentCount <= 10 {
			incidentScore = 5.0
		} else {
			incidentScore = 2.0
		}
	}

	totalScore := uptimeScore + responseScore + incidentScore

	// Normalize et
	if totalScore > 100 {
		totalScore = 100
	}
	if totalScore < 0 {
		totalScore = 0
	}

	return totalScore
}

// getHealthGrade - Sağlık skoruna göre derece döndürür
func getHealthGrade(score float64) string {
	if score >= 95 {
		return "Mükemmel"
	} else if score >= 85 {
		return "İyi"
	} else if score >= 70 {
		return "Orta"
	} else {
		return "Kötü"
	}
}

// getResponseGrade - Ortalama yanıt süresine göre performans kategorisi döndürür
func getResponseGrade(avgResponseMS float64) string {
	if avgResponseMS == 0 {
		return "Veri Yok"
	} else if avgResponseMS < 50 {
		return "Mükemmel"
	} else if avgResponseMS < 100 {
		return "İyi"
	} else if avgResponseMS < 200 {
		return "Orta"
	} else {
		return "Yavaş"
	}
}

// calculateSystemHealth - Tüm sistem için genel sağlık metriklerini hesaplar
func calculateSystemHealth(targets []TargetSnapshot) SystemHealth {
	health := SystemHealth{}

	if len(targets) == 0 {
		health.HealthStatus = "Veri Yok"
		return health
	}

	totalUptime := 0.0
	totalResponseTime := 0.0
	totalIncidents := 0
	totalScore := 0.0
	criticalCount := 0
	validTargets := 0

	for _, target := range targets {
		if !target.Enabled {
			continue
		}

		validTargets++
		totalUptime += target.SLAMetrics.UptimePercent
		totalResponseTime += target.SLAMetrics.AvgResponseMS
		totalIncidents += target.SLAMetrics.IncidentCount
		totalScore += target.SLAMetrics.HealthScore

		// Sağlık skoru 70'in altındaysa kritik kabul et
		if target.SLAMetrics.HealthScore < 70 {
			criticalCount++
		}
	}

	if validTargets > 0 {
		health.TotalUptime = totalUptime / float64(validTargets)
		health.AverageResponseMS = totalResponseTime / float64(validTargets)
		health.OverallScore = totalScore / float64(validTargets)
	}

	health.TotalIncidents = totalIncidents
	health.CriticalTargets = criticalCount
	health.HealthStatus = getHealthGrade(health.OverallScore)

	return health
}
