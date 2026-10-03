package notifications

import (
	"database/sql"
	"time"
)

// EmailLog email log yapısı
type EmailLog struct {
	ID              int        `json:"id" db:"id"`
	NotificationID  *int       `json:"notification_id" db:"notification_id"`
	RecipientEmail  string     `json:"recipient_email" db:"recipient_email"`
	Subject         string     `json:"subject" db:"subject"`
	MessageType     string     `json:"message_type" db:"message_type"`
	Status          string     `json:"status" db:"status"`
	SMTPHost        string     `json:"smtp_host" db:"smtp_host"`
	SMTPPort        int        `json:"smtp_port" db:"smtp_port"`
	UseTLS          bool       `json:"use_tls" db:"use_tls"`
	SentAt          *time.Time `json:"sent_at" db:"sent_at"`
	FailedAt        *time.Time `json:"failed_at" db:"failed_at"`
	ErrorMessage    *string    `json:"error_message" db:"error_message"`
	RetryCount      int        `json:"retry_count" db:"retry_count"`
	MessageID       *string    `json:"message_id" db:"message_id"`
	ResponseTimeMs  *int       `json:"response_time_ms" db:"response_time_ms"`
	AttachmentCount int        `json:"attachment_count" db:"attachment_count"`
	AttachmentSize  int64      `json:"attachment_size" db:"attachment_size"`
	CreatedAt       time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at" db:"updated_at"`
}

// EmailStatistics email istatistikleri
type EmailStatistics struct {
	ID                int       `json:"id" db:"id"`
	Date              time.Time `json:"date" db:"date"`
	TotalSent         int       `json:"total_sent" db:"total_sent"`
	TotalFailed       int       `json:"total_failed" db:"total_failed"`
	TotalBounced      int       `json:"total_bounced" db:"total_bounced"`
	AvgResponseTimeMs *float64  `json:"avg_response_time_ms" db:"avg_response_time_ms"`
	UniqueRecipients  int       `json:"unique_recipients" db:"unique_recipients"`
	CreatedAt         time.Time `json:"created_at" db:"created_at"`
	UpdatedAt         time.Time `json:"updated_at" db:"updated_at"`
}

// EmailRecipient email alıcısı
type EmailRecipient struct {
	ID                    int        `json:"id" db:"id"`
	Email                 string     `json:"email" db:"email"`
	Name                  *string    `json:"name" db:"name"`
	IsActive              bool       `json:"is_active" db:"is_active"`
	IsVerified            bool       `json:"is_verified" db:"is_verified"`
	VerificationToken     *string    `json:"verification_token" db:"verification_token"`
	VerificationExpiresAt *time.Time `json:"verification_expires_at" db:"verification_expires_at"`
	BounceCount           int        `json:"bounce_count" db:"bounce_count"`
	LastBounceAt          *time.Time `json:"last_bounce_at" db:"last_bounce_at"`
	CreatedAt             time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt             time.Time  `json:"updated_at" db:"updated_at"`
}

// EmailTemplateLog email şablon kullanım logu
type EmailTemplateLog struct {
	ID           int       `json:"id" db:"id"`
	TemplateType string    `json:"template_type" db:"template_type"`
	TemplateName string    `json:"template_name" db:"template_name"`
	UsageCount   int       `json:"usage_count" db:"usage_count"`
	LastUsedAt   time.Time `json:"last_used_at" db:"last_used_at"`
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time `json:"updated_at" db:"updated_at"`
}

// EmailLogger email log servisi
type EmailLogger struct {
	db *sql.DB
}

// NewEmailLogger yeni email logger oluştur
func NewEmailLogger(db *sql.DB) *EmailLogger {
	return &EmailLogger{
		db: db,
	}
}

// LogEmailAttempt email gönderim denemesini logla
func (el *EmailLogger) LogEmailAttempt(log *EmailLog) (int, error) {
	query := `
		INSERT INTO email_logs (
			notification_id, recipient_email, subject, message_type, 
			status, smtp_host, smtp_port, use_tls, retry_count,
			attachment_count, attachment_size
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	result, err := el.db.Exec(query,
		log.NotificationID,
		log.RecipientEmail,
		log.Subject,
		log.MessageType,
		log.Status,
		log.SMTPHost,
		log.SMTPPort,
		log.UseTLS,
		log.RetryCount,
		log.AttachmentCount,
		log.AttachmentSize,
	)

	if err != nil {
		return 0, err
	}

	id, err := result.LastInsertId()
	return int(id), err
}

// UpdateEmailLogStatus email log durumunu güncelle
func (el *EmailLogger) UpdateEmailLogStatus(id int, status string, errorMsg *string, responseTimeMs *int, messageID *string) error {
	var query string
	var args []interface{}

	if status == "sent" {
		query = `
			UPDATE email_logs 
			SET status = ?, sent_at = NOW(), response_time_ms = ?, message_id = ?, error_message = NULL
			WHERE id = ?
		`
		args = []interface{}{status, responseTimeMs, messageID, id}
	} else if status == "failed" {
		query = `
			UPDATE email_logs 
			SET status = ?, failed_at = NOW(), error_message = ?, response_time_ms = ?
			WHERE id = ?
		`
		args = []interface{}{status, errorMsg, responseTimeMs, id}
	} else {
		query = `UPDATE email_logs SET status = ? WHERE id = ?`
		args = []interface{}{status, id}
	}

	_, err := el.db.Exec(query, args...)
	return err
}

// GetEmailLogs email loglarını getir
func (el *EmailLogger) GetEmailLogs(limit, offset int, status, recipient string) ([]EmailLog, error) {
	query := `
		SELECT id, notification_id, recipient_email, subject, message_type, 
		       status, smtp_host, smtp_port, use_tls, sent_at, failed_at, 
		       error_message, retry_count, message_id, response_time_ms, 
		       created_at, updated_at
		FROM email_logs
		WHERE 1=1
	`
	args := []interface{}{}

	if status != "" {
		query += " AND status = ?"
		args = append(args, status)
	}

	if recipient != "" {
		query += " AND recipient_email LIKE ?"
		args = append(args, "%"+recipient+"%")
	}

	query += " ORDER BY created_at DESC LIMIT ? OFFSET ?"
	args = append(args, limit, offset)

	rows, err := el.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []EmailLog
	for rows.Next() {
		var log EmailLog
		err := rows.Scan(
			&log.ID, &log.NotificationID, &log.RecipientEmail, &log.Subject,
			&log.MessageType, &log.Status, &log.SMTPHost, &log.SMTPPort,
			&log.UseTLS, &log.SentAt, &log.FailedAt, &log.ErrorMessage,
			&log.RetryCount, &log.MessageID, &log.ResponseTimeMs,
			&log.CreatedAt, &log.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		logs = append(logs, log)
	}

	return logs, nil
}

// GetEmailStatistics email istatistiklerini getir
func (el *EmailLogger) GetEmailStatistics(days int) ([]EmailStatistics, error) {
	query := `
		SELECT date, total_sent, total_failed, total_bounced, 
		       avg_response_time_ms, unique_recipients
		FROM email_statistics
		WHERE date >= DATE_SUB(CURDATE(), INTERVAL ? DAY)
		ORDER BY date DESC
	`

	rows, err := el.db.Query(query, days)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stats []EmailStatistics
	for rows.Next() {
		var stat EmailStatistics
		var avgResponseTimeMs sql.NullFloat64
		err := rows.Scan(
			&stat.Date, &stat.TotalSent, &stat.TotalFailed, &stat.TotalBounced,
			&avgResponseTimeMs, &stat.UniqueRecipients,
		)
		if err != nil {
			return nil, err
		}

		// NULL değerleri kontrol et
		if avgResponseTimeMs.Valid {
			stat.AvgResponseTimeMs = &avgResponseTimeMs.Float64
		}

		stats = append(stats, stat)
	}

	return stats, nil
}

// UpdateDailyStatistics günlük istatistikleri güncelle
func (el *EmailLogger) UpdateDailyStatistics(date time.Time) error {
	query := `
		INSERT INTO email_statistics (date, total_sent, total_failed, total_bounced, avg_response_time_ms, unique_recipients)
		SELECT 
			? as date,
			COUNT(CASE WHEN status = 'sent' THEN 1 END) as total_sent,
			COUNT(CASE WHEN status = 'failed' THEN 1 END) as total_failed,
			COUNT(CASE WHEN status = 'bounced' THEN 1 END) as total_bounced,
			AVG(CASE WHEN response_time_ms IS NOT NULL THEN response_time_ms END) as avg_response_time_ms,
			COUNT(DISTINCT recipient_email) as unique_recipients
		FROM email_logs
		WHERE DATE(created_at) = ?
		ON DUPLICATE KEY UPDATE
			total_sent = VALUES(total_sent),
			total_failed = VALUES(total_failed),
			total_bounced = VALUES(total_bounced),
			avg_response_time_ms = VALUES(avg_response_time_ms),
			unique_recipients = VALUES(unique_recipients),
			updated_at = NOW()
	`

	_, err := el.db.Exec(query, date.Format("2006-01-02"), date.Format("2006-01-02"))
	return err
}

// LogTemplateUsage şablon kullanımını logla
func (el *EmailLogger) LogTemplateUsage(templateType, templateName string) error {
	query := `
		INSERT INTO email_templates_log (template_type, template_name, usage_count, last_used_at)
		VALUES (?, ?, 1, NOW())
		ON DUPLICATE KEY UPDATE
			usage_count = usage_count + 1,
			last_used_at = NOW(),
			updated_at = NOW()
	`

	_, err := el.db.Exec(query, templateType, templateName)
	return err
}

// GetTemplateUsageStats şablon kullanım istatistiklerini getir
func (el *EmailLogger) GetTemplateUsageStats() ([]EmailTemplateLog, error) {
	query := `
		SELECT id, template_type, template_name, usage_count, last_used_at, created_at, updated_at
		FROM email_templates_log
		ORDER BY usage_count DESC, last_used_at DESC
	`

	rows, err := el.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var templates []EmailTemplateLog
	for rows.Next() {
		var template EmailTemplateLog
		err := rows.Scan(
			&template.ID, &template.TemplateType, &template.TemplateName,
			&template.UsageCount, &template.LastUsedAt, &template.CreatedAt, &template.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		templates = append(templates, template)
	}

	return templates, nil
}

// GetEmailLogsCount toplam email log sayısını getir
func (el *EmailLogger) GetEmailLogsCount(status, recipient string) (int, error) {
	query := "SELECT COUNT(*) FROM email_logs WHERE 1=1"
	args := []interface{}{}

	if status != "" {
		query += " AND status = ?"
		args = append(args, status)
	}

	if recipient != "" {
		query += " AND recipient_email LIKE ?"
		args = append(args, "%"+recipient+"%")
	}

	var count int
	err := el.db.QueryRow(query, args...).Scan(&count)
	return count, err
}

// GetRecentEmailLogs son email loglarını getir
func (el *EmailLogger) GetRecentEmailLogs(limit int) ([]EmailLog, error) {
	return el.GetEmailLogs(limit, 0, "", "")
}

// GetFailedEmailLogs başarısız email loglarını getir
func (el *EmailLogger) GetFailedEmailLogs(limit int) ([]EmailLog, error) {
	return el.GetEmailLogs(limit, 0, "failed", "")
}

// GetEmailLogsByRecipient alıcıya göre email loglarını getir
func (el *EmailLogger) GetEmailLogsByRecipient(email string, limit int) ([]EmailLog, error) {
	return el.GetEmailLogs(limit, 0, "", email)
}
