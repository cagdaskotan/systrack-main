package notifications

import (
	"bytes"
	"context"
	"crypto/tls"
	"database/sql"
	"encoding/base64"
	"fmt"
	"html/template"
	"log"
	"net/smtp"
	"strings"
	"time"
)

// emailService mail servisi implementasyonu
type emailService struct {
	config EmailConfig
	logger *EmailLogger
	db     *sql.DB
}

// NewEmailService yeni mail servisi oluştur
func NewEmailService(config EmailConfig, logger *EmailLogger) EmailService {
	return &emailService{
		config: config,
		logger: logger,
	}
}

// NewEmailServiceWithDB database ile yeni mail servisi oluştur
func NewEmailServiceWithDB(config EmailConfig, logger *EmailLogger, db *sql.DB) EmailService {
	return &emailService{
		config: config,
		logger: logger,
		db:     db,
	}
}

// Send mail gönder
func (s *emailService) Send(ctx context.Context, notification *Notification) error {
	if !s.config.Enabled {
		return fmt.Errorf("email notifications are disabled")
	}

	// HTML template oluştur
	htmlBody, err := s.generateHTMLBody(notification)
	if err != nil {
		// Template oluşturulamazsa basit text mesaj kullan
		log.Printf("⚠️ generateHTMLBody failed: %v, using fallback message", err)
		htmlBody = fmt.Sprintf(`<!DOCTYPE html>
<html>
<head><meta charset="UTF-8"></head>
<body>
<h2>%s</h2>
<p>%s</p>
<p><strong>Not:</strong> Hedef verileri alınamadı veya template oluşturulamadı.</p>
<p><em>Zaman: %s</em></p>
</body>
</html>`, notification.Title, notification.Message, notification.CreatedAt.Format("2006-01-02 15:04:05"))
	}

	// Her alıcı için ayrı log oluştur
	var lastError error
	for _, recipient := range notification.Recipients {
		// Ek dosya bilgilerini hesapla
		attachmentCount := len(notification.Attachments)
		var attachmentSize int64
		for _, att := range notification.Attachments {
			attachmentSize += att.Size
		}

		// Email log oluştur
		emailLog := &EmailLog{
			NotificationID:  &notification.ID,
			RecipientEmail:  recipient,
			Subject:         notification.Title,
			MessageType:     "html",
			Status:          "pending",
			SMTPHost:        s.config.SMTPHost,
			SMTPPort:        s.config.SMTPPort,
			UseTLS:          s.config.UseTLS,
			RetryCount:      notification.RetryCount,
			AttachmentCount: attachmentCount,
			AttachmentSize:  attachmentSize,
		}

		// Log kaydet
		var logID int
		if s.logger != nil {
			var err error
			logID, err = s.logger.LogEmailAttempt(emailLog)
			if err != nil {
				// Log hatası kritik değil, devam et
				fmt.Printf("Failed to log email attempt: %v\n", err)
			}
		}

		// Mail gönder
		err := s.SendHTMLEmailWithAttachments(ctx, []string{recipient}, notification.Title, htmlBody, notification.Attachments)
		if err != nil {
			lastError = err
			// Log durumunu güncelle
			if s.logger != nil && logID > 0 {
				errorMsg := err.Error()
				s.logger.UpdateEmailLogStatus(logID, "failed", &errorMsg, nil, nil)
			}
		} else {
			// Log durumunu güncelle
			if s.logger != nil && logID > 0 {
				s.logger.UpdateEmailLogStatus(logID, "sent", nil, nil, nil)
			}
		}

		// Template kullanımını logla
		if s.logger != nil {
			s.logger.LogTemplateUsage(string(notification.Type), notification.Title)
		}
	}

	return lastError
}

// SendEmail basit mail gönder
func (s *emailService) SendEmail(ctx context.Context, to []string, subject, body string) error {
	if !s.config.Enabled {
		return fmt.Errorf("email notifications are disabled")
	}

	// SMTP auth
	auth := smtp.PlainAuth("", s.config.Username, s.config.Password, s.config.SMTPHost)

	// Mail başlıkları
	headers := make(map[string]string)
	headers["From"] = fmt.Sprintf("%s <%s>", s.config.FromName, s.config.FromEmail)
	headers["To"] = strings.Join(to, ", ")
	headers["Subject"] = subject
	headers["MIME-Version"] = "1.0"
	headers["Content-Type"] = "text/plain; charset=UTF-8"

	// Mail içeriği
	var msg bytes.Buffer
	for k, v := range headers {
		msg.WriteString(fmt.Sprintf("%s: %s\r\n", k, v))
	}
	msg.WriteString("\r\n")
	msg.WriteString(body)

	// SMTP sunucusuna bağlan
	addr := fmt.Sprintf("%s:%d", s.config.SMTPHost, s.config.SMTPPort)

	if s.config.UseTLS {
		// STARTTLS ile bağlan (Gmail için uygun)
		client, err := smtp.Dial(addr)
		if err != nil {
			return fmt.Errorf("failed to connect to SMTP server: %w", err)
		}
		defer client.Quit()

		// STARTTLS başlat
		tlsConfig := &tls.Config{
			ServerName: s.config.SMTPHost,
		}
		if err := client.StartTLS(tlsConfig); err != nil {
			return fmt.Errorf("failed to start TLS: %w", err)
		}

		// Auth
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("failed to authenticate: %w", err)
		}

		// Mail gönder
		if err := client.Mail(s.config.FromEmail); err != nil {
			return fmt.Errorf("failed to set sender: %w", err)
		}

		for _, recipient := range to {
			if err := client.Rcpt(recipient); err != nil {
				return fmt.Errorf("failed to set recipient %s: %w", recipient, err)
			}
		}

		writer, err := client.Data()
		if err != nil {
			return fmt.Errorf("failed to get data writer: %w", err)
		}

		_, err = writer.Write(msg.Bytes())
		if err != nil {
			return fmt.Errorf("failed to write message: %w", err)
		}

		err = writer.Close()
		if err != nil {
			return fmt.Errorf("failed to close writer: %w", err)
		}

	} else {
		// Normal SMTP
		err := smtp.SendMail(addr, auth, s.config.FromEmail, to, msg.Bytes())
		if err != nil {
			return fmt.Errorf("failed to send email: %w", err)
		}
	}

	return nil
}

// SendHTMLEmail HTML mail gönder
func (s *emailService) SendHTMLEmail(ctx context.Context, to []string, subject, htmlBody string) error {
	return s.SendHTMLEmailWithAttachments(ctx, to, subject, htmlBody, nil)
}

// SendHTMLEmailWithAttachments HTML mail gönder (ek dosyalarla)
func (s *emailService) SendHTMLEmailWithAttachments(ctx context.Context, to []string, subject, htmlBody string, attachments []EmailAttachment) error {
	if !s.config.Enabled {
		return fmt.Errorf("email notifications are disabled")
	}

	// SMTP auth
	auth := smtp.PlainAuth("", s.config.Username, s.config.Password, s.config.SMTPHost)

	// Mail başlıkları
	headers := make(map[string]string)
	headers["From"] = fmt.Sprintf("%s <%s>", s.config.FromName, s.config.FromEmail)
	headers["To"] = strings.Join(to, ", ")
	headers["Subject"] = subject
	headers["MIME-Version"] = "1.0"

	// Mail içeriği
	var msg bytes.Buffer

	// MIME boundary oluştur
	boundary := fmt.Sprintf("boundary_%d", time.Now().UnixNano())

	if len(attachments) > 0 {
		// Multipart mail
		headers["Content-Type"] = fmt.Sprintf("multipart/mixed; boundary=%s", boundary)

		// Headers yaz
		for k, v := range headers {
			msg.WriteString(fmt.Sprintf("%s: %s\r\n", k, v))
		}
		msg.WriteString("\r\n")

		// HTML bölümü
		msg.WriteString(fmt.Sprintf("--%s\r\n", boundary))
		msg.WriteString("Content-Type: text/html; charset=UTF-8\r\n")
		msg.WriteString("Content-Transfer-Encoding: 7bit\r\n")
		msg.WriteString("\r\n")
		msg.WriteString(htmlBody)
		msg.WriteString("\r\n")

		// Ek dosyalar
		for _, attachment := range attachments {
			msg.WriteString(fmt.Sprintf("--%s\r\n", boundary))
			msg.WriteString(fmt.Sprintf("Content-Type: %s\r\n", attachment.ContentType))
			msg.WriteString("Content-Transfer-Encoding: base64\r\n")
			msg.WriteString(fmt.Sprintf("Content-Disposition: attachment; filename=\"%s\"\r\n", attachment.Filename))
			msg.WriteString("\r\n")

			// Base64 encode
			encoded := base64.StdEncoding.EncodeToString(attachment.Data)
			// 76 karakterde satır kır
			for i := 0; i < len(encoded); i += 76 {
				end := i + 76
				if end > len(encoded) {
					end = len(encoded)
				}
				msg.WriteString(encoded[i:end])
				msg.WriteString("\r\n")
			}
		}

		msg.WriteString(fmt.Sprintf("--%s--\r\n", boundary))
	} else {
		// Basit HTML mail
		headers["Content-Type"] = "text/html; charset=UTF-8"

		// Headers yaz
		for k, v := range headers {
			msg.WriteString(fmt.Sprintf("%s: %s\r\n", k, v))
		}
		msg.WriteString("\r\n")
		msg.WriteString(htmlBody)
	}

	// SMTP sunucusuna bağlan
	addr := fmt.Sprintf("%s:%d", s.config.SMTPHost, s.config.SMTPPort)

	if s.config.UseTLS {
		// STARTTLS ile bağlan (Gmail için uygun)
		client, err := smtp.Dial(addr)
		if err != nil {
			return fmt.Errorf("failed to connect to SMTP server: %w", err)
		}
		defer client.Quit()

		// STARTTLS başlat
		tlsConfig := &tls.Config{
			ServerName: s.config.SMTPHost,
		}
		if err := client.StartTLS(tlsConfig); err != nil {
			return fmt.Errorf("failed to start TLS: %w", err)
		}

		// Auth
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("failed to authenticate: %w", err)
		}

		// Mail gönder
		if err := client.Mail(s.config.FromEmail); err != nil {
			return fmt.Errorf("failed to set sender: %w", err)
		}

		for _, recipient := range to {
			if err := client.Rcpt(recipient); err != nil {
				return fmt.Errorf("failed to set recipient %s: %w", recipient, err)
			}
		}

		writer, err := client.Data()
		if err != nil {
			return fmt.Errorf("failed to get data writer: %w", err)
		}

		_, err = writer.Write(msg.Bytes())
		if err != nil {
			return fmt.Errorf("failed to write message: %w", err)
		}

		err = writer.Close()
		if err != nil {
			return fmt.Errorf("failed to close writer: %w", err)
		}

	} else {
		// Normal SMTP
		err := smtp.SendMail(addr, auth, s.config.FromEmail, to, msg.Bytes())
		if err != nil {
			return fmt.Errorf("failed to send email: %w", err)
		}
	}

	return nil
}

// ValidateConfig konfigürasyonu doğrula
func (s *emailService) ValidateConfig(config interface{}) error {
	emailConfig, ok := config.(EmailConfig)
	if !ok {
		return fmt.Errorf("invalid config type")
	}

	if emailConfig.SMTPHost == "" {
		return fmt.Errorf("SMTP host is required")
	}

	if emailConfig.SMTPPort <= 0 || emailConfig.SMTPPort > 65535 {
		return fmt.Errorf("invalid SMTP port")
	}

	if emailConfig.Username == "" {
		return fmt.Errorf("SMTP username is required")
	}

	if emailConfig.FromEmail == "" {
		return fmt.Errorf("from email is required")
	}

	return nil
}

// TestConnection bağlantıyı test et
func (s *emailService) TestConnection(ctx context.Context) error {
	if !s.config.Enabled {
		return fmt.Errorf("email notifications are disabled")
	}

	// Test mail gönder - notification email adresine gönder
	testRecipients := []string{s.config.NotificationEmail}
	testSubject := "SysTrack - Test Email"

	// Test için özel HTML template oluştur
	testHTML := `
<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <title>SysTrack Test Email</title>
    <style>
        body { font-family: 'Segoe UI', Tahoma, Geneva, Verdana, sans-serif; line-height: 1.6; color: #333; margin: 0; padding: 0; background-color: #f5f5f5; }
        .container { max-width: 600px; margin: 20px auto; background: white; border-radius: 12px; overflow: hidden; box-shadow: 0 4px 6px rgba(0,0,0,0.1); }
        .header { background: linear-gradient(135deg, #667eea 0%, #764ba2 100%); color: white; padding: 30px; text-align: center; }
        .header h1 { margin: 0; font-size: 24px; font-weight: 600; }
        .content { padding: 30px; }
        .success-badge { display: inline-block; padding: 12px 20px; border-radius: 25px; font-weight: bold; font-size: 16px; margin: 15px 0; background: #00b894; color: white; }
        .info-card { background: #e8f5e8; border-radius: 8px; padding: 20px; margin: 20px 0; border-left: 4px solid #00b894; }
        .footer { background: #f8f9fa; padding: 20px; text-align: center; color: #6c757d; font-size: 12px; }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1>✅ SysTrack Test Email</h1>
            <p>Email Bildirim Sistemi Test</p>
        </div>
        <div class="content">
            <h2>Test Başarılı!</h2>
            <p>SysTrack email bildirim sistemi başarıyla çalışıyor.</p>
            
            <div class="success-badge">
                🎉 EMAIL SİSTEMİ AKTİF
            </div>
            
            <div class="info-card">
                <h3>📧 Test Detayları</h3>
                <p><strong>SMTP Host:</strong> ` + s.config.SMTPHost + `</p>
                <p><strong>SMTP Port:</strong> ` + fmt.Sprintf("%d", s.config.SMTPPort) + `</p>
                <p><strong>TLS:</strong> ` + fmt.Sprintf("%t", s.config.UseTLS) + `</p>
                <p><strong>Test Zamanı:</strong> ` + time.Now().Format("2006-01-02 15:04:05") + `</p>
            </div>
            
            <div style="background: #d1ecf1; border: 1px solid #bee5eb; border-radius: 6px; padding: 15px; margin: 20px 0;">
                <strong>💡 Bilgi:</strong>
                <p>Bu test emaili, SysTrack monitoring sisteminin email bildirim özelliğinin düzgün çalıştığını doğrular.</p>
            </div>
        </div>
        <div class="footer">
            <p>Bu test emaili SysTrack monitoring sistemi tarafından otomatik olarak gönderilmiştir.</p>
            <p>Bildirim ayarlarınızı değiştirmek için admin panelini ziyaret edin.</p>
        </div>
    </div>
</body>
</html>`

	// Test email için log oluştur
	var logID int
	if s.logger != nil {
		emailLog := &EmailLog{
			RecipientEmail: s.config.NotificationEmail,
			Subject:        testSubject,
			MessageType:    "html",
			Status:         "pending",
			SMTPHost:       s.config.SMTPHost,
			SMTPPort:       s.config.SMTPPort,
			UseTLS:         s.config.UseTLS,
			RetryCount:     0,
		}

		var err error
		logID, err = s.logger.LogEmailAttempt(emailLog)
		if err != nil {
			fmt.Printf("Failed to log test email attempt: %v\n", err)
		}
	}

	// Test email gönder
	err := s.SendHTMLEmail(ctx, testRecipients, testSubject, testHTML)

	// Log durumunu güncelle
	if s.logger != nil && logID > 0 {
		if err != nil {
			errorMsg := err.Error()
			s.logger.UpdateEmailLogStatus(logID, "failed", &errorMsg, nil, nil)
		} else {
			s.logger.UpdateEmailLogStatus(logID, "sent", nil, nil, nil)
		}

		// Test template kullanımını logla
		s.logger.LogTemplateUsage("test_email", "Test Emaili")
	}

	return err
}

// generateHTMLBody HTML mail içeriği oluştur
func (s *emailService) generateHTMLBody(notification *Notification) (string, error) {
	// Template manager oluştur
	templateManager := NewEmailTemplateManager()

	// Gerçek verilerle NotificationData oluştur
	data := s.buildNotificationData(notification)

	// Template türüne göre HTML oluştur
	return templateManager.GenerateHTML(notification, data)
}

// buildNotificationData gerçek verilerle NotificationData oluştur
func (s *emailService) buildNotificationData(notification *Notification) *NotificationData {
	data := &NotificationData{
		TargetName:    "Bilinmeyen Hedef",
		TargetURL:     "N/A",
		Status:        notification.Status,
		ResponseTime:  0.0,
		UptimePercent: 0.0,
		AlertMessage:  notification.Message,
		AlertLevel:    string(notification.Priority),
		Timestamp:     notification.CreatedAt.Format("2006-01-02 15:04:05"),
		SLATarget:     99.9, // Varsayılan SLA hedefi
		SLABreach:     false,
	}

	// Target ID varsa gerçek verileri getir
	if notification.TargetID != nil && s.db != nil {
		s.loadTargetData(notification.TargetID, data)
	}

	return data
}

// loadTargetData target ile ilgili gerçek verileri yükle
func (s *emailService) loadTargetData(targetID *int, data *NotificationData) {
	if targetID == nil || s.db == nil {
		log.Printf("⚠️ loadTargetData: targetID or db is nil, using default values")
		return
	}

	// Target bilgilerini al
	targetQuery := `SELECT name, address FROM targets WHERE id = ?`
	var name, address string
	err := s.db.QueryRow(targetQuery, *targetID).Scan(&name, &address)
	if err == nil {
		data.TargetName = name
		data.TargetURL = address
	} else {
		log.Printf("⚠️ loadTargetData: Failed to load target info for ID %d: %v, using placeholder", *targetID, err)
		data.TargetName = fmt.Sprintf("Hedef #%d (Bilgi Alınamadı)", *targetID)
		data.TargetURL = "Bilgi mevcut değil"
	}

	// SLA analizi için son uptime verilerini al
	slaQuery := `
		SELECT AVG(CASE WHEN status = 'success' THEN 1 ELSE 0 END) * 100 as uptime,
		       AVG(response_time) as avg_response_time
		FROM pings
		WHERE target_id = ? AND created_at >= DATE_SUB(NOW(), INTERVAL 24 HOUR)
	`
	var uptimePercent, avgResponseTime sql.NullFloat64
	err = s.db.QueryRow(slaQuery, *targetID).Scan(&uptimePercent, &avgResponseTime)
	if err == nil {
		if uptimePercent.Valid {
			data.UptimePercent = uptimePercent.Float64
		}
		if avgResponseTime.Valid {
			data.ResponseTime = avgResponseTime.Float64
		}

		// SLA ihlali kontrol et
		data.SLABreach = data.UptimePercent < data.SLATarget
	} else {
		log.Printf("⚠️ loadTargetData: Failed to load SLA data for target %d: %v, using defaults", *targetID, err)
		// Default değerleri zaten data'da mevcut (0.0 vs.)
	}
}

// SendBulk bulk mail gönder (birden fazla hedef için tek mail)
func (s *emailService) SendBulk(ctx context.Context, notification *Notification, recipients *BulkNotificationRecipients) error {
	if !s.config.Enabled {
		return fmt.Errorf("email notifications are disabled")
	}

	if len(recipients.Targets) == 0 {
		return fmt.Errorf("no targets provided for bulk notification")
	}

	// HTML body oluştur
	htmlBody, err := s.generateBulkHTMLBody(notification, recipients)
	if err != nil {
		return fmt.Errorf("failed to generate bulk HTML body: %w", err)
	}

	// Subject oluştur
	subject := fmt.Sprintf("Toplu Bildirim - %d Hedef", len(recipients.Targets))
	if notification.Title != "" {
		subject = notification.Title
	}

	// Her alıcı için ayrı log oluştur
	var lastError error
	for _, email := range recipients.Emails {
		// Email log oluştur
		emailLog := &EmailLog{
			NotificationID: &notification.ID,
			RecipientEmail: email,
			Subject:        subject,
			MessageType:    "html",
			Status:         "pending",
			SMTPHost:       s.config.SMTPHost,
			SMTPPort:       s.config.SMTPPort,
			UseTLS:         s.config.UseTLS,
			RetryCount:     notification.RetryCount,
		}

		// Log kaydet
		var logID int
		if s.logger != nil {
			var err error
			logID, err = s.logger.LogEmailAttempt(emailLog)
			if err != nil {
				fmt.Printf("Failed to log bulk email attempt: %v\n", err)
			}
		}

		// Mail gönder
		err := s.SendHTMLEmail(ctx, []string{email}, subject, htmlBody)
		if err != nil {
			lastError = err
			// Log durumunu güncelle
			if s.logger != nil && logID > 0 {
				errorMsg := err.Error()
				s.logger.UpdateEmailLogStatus(logID, "failed", &errorMsg, nil, nil)
			}
		} else {
			// Log durumunu güncelle
			if s.logger != nil && logID > 0 {
				s.logger.UpdateEmailLogStatus(logID, "sent", nil, nil, nil)
			}
		}

		// Template kullanımını logla
		if s.logger != nil {
			s.logger.LogTemplateUsage("bulk_"+string(notification.Type), subject)
		}
	}

	return lastError
}

// generateBulkHTMLBody bulk notification için HTML body oluştur
func (s *emailService) generateBulkHTMLBody(notification *Notification, recipients *BulkNotificationRecipients) (string, error) {
	templateManager := NewEmailTemplateManager()

	// Template verisi hazırla
	templateData := map[string]interface{}{
		"Title":       notification.Title,
		"Message":     notification.Message,
		"Type":        notification.Type,
		"Priority":    notification.Priority,
		"Timestamp":   time.Now().Format("02.01.2006 15:04:05"),
		"TargetCount": len(recipients.Targets),
		"Targets":     s.prepareTargetsData(recipients.Targets),
	}

	// Bulk template al
	templateStr := templateManager.GetBulkTemplate(notification.Type)

	// Template'i parse et ve render et
	tmpl, err := template.New("bulk_email").Parse(templateStr)
	if err != nil {
		return "", fmt.Errorf("failed to parse bulk template: %w", err)
	}

	var buf bytes.Buffer
	err = tmpl.Execute(&buf, templateData)
	if err != nil {
		return "", fmt.Errorf("failed to execute bulk template: %w", err)
	}

	return buf.String(), nil
}

// prepareTargetsData hedef verilerini template için hazırla
func (s *emailService) prepareTargetsData(targets []TargetNotificationData) []map[string]interface{} {
	var result []map[string]interface{}

	for _, target := range targets {
		targetData := map[string]interface{}{
			"Name":        target.Name,
			"Address":     target.Address,
			"Status":      target.Status,
			"StatusClass": target.StatusClass,
		}

		// Optional alanlar
		if target.UptimePercent != nil {
			targetData["UptimePercent"] = *target.UptimePercent
		}
		if target.ResponseTime != nil {
			targetData["ResponseTime"] = *target.ResponseTime
		}

		result = append(result, targetData)
	}

	return result
}

// getStatusClass status'a göre CSS class döndür
func getStatusClass(status string) string {
	switch strings.ToLower(status) {
	case "online", "success", "çevrimiçi":
		return "online"
	case "offline", "failed", "çevrimdışı":
		return "offline"
	case "warning", "uyarı":
		return "warning"
	default:
		return "unknown"
	}
}
