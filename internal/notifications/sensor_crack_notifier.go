package notifications

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"systrack/internal/services"
)

// CameraManagementURL is set from main.go so camera image fetching knows the management server.
var CameraManagementURL string

// NotifyCrackDetected çağrısı api_crack_scan.go'dan gelir; crack yazılımı tespit edildiğinde.
func NotifyCrackDetected(inventoryID int, deviceName, deviceIP, riskLevel string, findingCount int) {
	if defaultSimpleRunner == nil {
		return
	}
	go defaultSimpleRunner.checkCrackRules(inventoryID, deviceName, deviceIP, riskLevel, findingCount)
}

// NotifySensorReading çağrısı services.SensorNotifyHook üzerinden gelir; her yeni sensör verisinde.
func NotifySensorReading(reading services.SensorReading) {
	if defaultSimpleRunner == nil {
		return
	}
	go defaultSimpleRunner.checkSensorRules(reading)
}

// NotifyLiquidContact çağrısı api_liquid_sensor.go'dan gelir; sıvı teması yeni algılandığında.
func NotifyLiquidContact(serial string) {
	if defaultSimpleRunner == nil {
		return
	}
	go defaultSimpleRunner.checkLiquidContactRules(serial)
}

// ─── Crack Detection ──────────────────────────────────────────────────────────

func (r *SimpleRuleRunner) checkCrackRules(inventoryID int, deviceName, deviceIP, riskLevel string, findingCount int) {
	rows, err := r.db.Query(`
		SELECT id, name, channel, schedule_interval_minutes, recipients, template_id,
		       conditions, last_sent_at, inventory_id
		FROM new_notification_rules
		WHERE entity_type = 'crack_detection' AND is_active = 1
	`)
	if err != nil {
		log.Printf("checkCrackRules: DB query: %v", err)
		return
	}
	defer rows.Close()

	now := time.Now()
	for rows.Next() {
		var rr newRuleRow
		var recipJSON, condJSON sql.NullString
		var tplID sql.NullInt64
		var lastSent sql.NullTime
		var ruleInvID sql.NullInt64

		if err := rows.Scan(&rr.ID, &rr.Name, &rr.Channel, &rr.IntervalMin,
			&recipJSON, &tplID, &condJSON, &lastSent, &ruleInvID); err != nil {
			continue
		}
		if recipJSON.Valid {
			_ = json.Unmarshal([]byte(recipJSON.String), &rr.Recipients)
		}
		if tplID.Valid {
			id := int(tplID.Int64)
			rr.TemplateID = &id
		}
		var conds []ruleCondition
		if condJSON.Valid {
			_ = json.Unmarshal([]byte(condJSON.String), &conds)
		}
		if lastSent.Valid {
			rr.LastSent = &lastSent.Time
		}

		// inventory_ids koşulu: JSON conditions içinde saklanır
		allowedIDs := sensorParseInventoryIDs(conds)
		if len(allowedIDs) > 0 {
			found := false
			for _, id := range allowedIDs {
				if id == inventoryID {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		} else if ruleInvID.Valid && ruleInvID.Int64 > 0 && int(ruleInvID.Int64) != inventoryID {
			// backward compat: single inventory_id column
			continue
		}

		// min_severity koşulunu kontrol et
		minSeverity := "any"
		for _, c := range conds {
			if c.Field == "min_severity" {
				if s, ok := c.Value.(string); ok {
					minSeverity = strings.ToLower(s)
				}
			}
		}
		if !crackSeverityMeets(riskLevel, minSeverity) {
			continue
		}

		// Cooldown
		if rr.LastSent != nil && rr.IntervalMin > 0 {
			if now.Sub(*rr.LastSent) < time.Duration(rr.IntervalMin)*time.Minute {
				continue
			}
		}

		ts := now.Format("02.01.2006 15:04:05")
		subject := fmt.Sprintf("[SysTrack] Crack Tespit Uyarısı: %s", deviceName)
		body := fmt.Sprintf(
			"Cihaz: %s (%s)\nRisk Seviyesi: %s\nTespit Sayısı: %d\nZaman: %s",
			deviceName, deviceIP, riskLevel, findingCount, ts,
		)
		payload := messagePayload{
			Subject:    subject,
			Body:       body,
			Text:       subject + "\n\n" + body,
			TemplateID: rr.TemplateID,
		}
		if dispErr := r.dispatchChannel(rr, 0, payload); dispErr == nil {
			_, _ = r.db.Exec(`UPDATE new_notification_rules SET last_sent_at = NOW() WHERE id = ?`, rr.ID)
		}
	}
}

// sensorParseInventoryIDs extracts inventory_ids array from conditions.
func sensorParseInventoryIDs(conds []ruleCondition) []int {
	for _, c := range conds {
		if c.Field == "inventory_ids" {
			return parseIDsFromValue(c.Value)
		}
	}
	return nil
}

// ─── Hava Sensörü Eşiği ───────────────────────────────────────────────────────

func (r *SimpleRuleRunner) checkSensorRules(reading services.SensorReading) {
	rows, err := r.db.Query(`
		SELECT id, name, channel, schedule_interval_minutes, recipients, template_id,
		       conditions, last_sent_at, sensor_serial
		FROM new_notification_rules
		WHERE entity_type = 'sensor' AND is_active = 1
	`)
	if err != nil {
		log.Printf("checkSensorRules: DB query: %v", err)
		return
	}
	defer rows.Close()

	now := time.Now()
	for rows.Next() {
		var rr newRuleRow
		var recipJSON, condJSON sql.NullString
		var tplID sql.NullInt64
		var lastSent sql.NullTime
		var ruleSerial sql.NullString

		if err := rows.Scan(&rr.ID, &rr.Name, &rr.Channel, &rr.IntervalMin,
			&recipJSON, &tplID, &condJSON, &lastSent, &ruleSerial); err != nil {
			continue
		}
		if recipJSON.Valid {
			_ = json.Unmarshal([]byte(recipJSON.String), &rr.Recipients)
		}
		if tplID.Valid {
			id := int(tplID.Int64)
			rr.TemplateID = &id
		}
		var conds []ruleCondition
		if condJSON.Valid {
			_ = json.Unmarshal([]byte(condJSON.String), &conds)
		}
		if lastSent.Valid {
			rr.LastSent = &lastSent.Time
		}

		// camera kuralları bu path'den tetiklenmez
		if parseSensorSubtype(conds) == "camera" {
			continue
		}

		// sensor_serial filtresi
		if ruleSerial.Valid && ruleSerial.String != "" && ruleSerial.String != reading.Serial {
			continue
		}

		// Okuma tazelik kontrolü: SSE yeniden bağlanma gibi durumlarda eski önbellek
		// verileri tekrar gönderilir. 5 dakikadan eski okumalar bildirim tetiklemez.
		if !reading.ReceivedAt.IsZero() && now.Sub(reading.ReceivedAt) > 5*time.Minute {
			continue
		}

		// Eşik koşulları — meta-alanlar atlanır
		allPass := true
		hasThreshold := false
		for _, c := range conds {
			if c.Field == "sensor_subtype" || c.Field == "camera_serials" || c.Field == "liquid_contact" || c.Field == "inventory_ids" {
				continue
			}
			hasThreshold = true
			fieldVal, ok := sensorFieldValue(reading, c.Field)
			if !ok {
				allPass = false
				break
			}
			if !evalSensorCondition(fieldVal, c.Operator, toFloat64Notif(c.Value)) {
				allPass = false
				break
			}
		}
		if !hasThreshold || !allPass {
			continue
		}

		// Cooldown
		if rr.LastSent != nil && rr.IntervalMin > 0 {
			if now.Sub(*rr.LastSent) < time.Duration(rr.IntervalMin)*time.Minute {
				continue
			}
		}

		condDesc := buildSensorCondDesc(conds, reading)
		ts := now.Format("02.01.2006 15:04:05")
		subject := fmt.Sprintf("[SysTrack] Sensör Eşik Uyarısı: %s", rr.Name)
		body := fmt.Sprintf("Sensör: %s\nKoşul: %s\nZaman: %s", reading.Serial, condDesc, ts)
		payload := messagePayload{
			Subject:    subject,
			Body:       body,
			Text:       subject + "\n\n" + body,
			TemplateID: rr.TemplateID,
		}
		if dispErr := r.dispatchChannel(rr, 0, payload); dispErr == nil {
			_, _ = r.db.Exec(`UPDATE new_notification_rules SET last_sent_at = NOW() WHERE id = ?`, rr.ID)
		}
	}
}

// ─── Kamera ───────────────────────────────────────────────────────────────────

// CheckCameraRules periodically called by the rule loop to look for new camera images.
func (r *SimpleRuleRunner) CheckCameraRules() {
	if CameraManagementURL == "" {
		return
	}

	rows, err := r.db.Query(`
		SELECT id, name, channel, schedule_interval_minutes, recipients, template_id,
		       conditions, last_sent_at
		FROM new_notification_rules
		WHERE entity_type = 'sensor' AND is_active = 1
	`)
	if err != nil {
		return
	}
	defer rows.Close()

	now := time.Now()
	for rows.Next() {
		var rr newRuleRow
		var recipJSON, condJSON sql.NullString
		var tplID sql.NullInt64
		var lastSent sql.NullTime

		if err := rows.Scan(&rr.ID, &rr.Name, &rr.Channel, &rr.IntervalMin,
			&recipJSON, &tplID, &condJSON, &lastSent); err != nil {
			continue
		}
		if recipJSON.Valid {
			_ = json.Unmarshal([]byte(recipJSON.String), &rr.Recipients)
		}
		if tplID.Valid {
			id := int(tplID.Int64)
			rr.TemplateID = &id
		}
		var conds []ruleCondition
		if condJSON.Valid {
			_ = json.Unmarshal([]byte(condJSON.String), &conds)
		}
		if lastSent.Valid {
			rr.LastSent = &lastSent.Time
		}

		if parseSensorSubtype(conds) != "camera" {
			continue
		}

		cameraSerials := parseCameraSerials(conds)
		if len(cameraSerials) == 0 {
			continue
		}

		// Cooldown
		if rr.LastSent != nil && rr.IntervalMin > 0 {
			if now.Sub(*rr.LastSent) < time.Duration(rr.IntervalMin)*time.Minute {
				continue
			}
		}

		// Görüntü tazelik kesme noktası: en az (now - interval) kadar yeni olmalı.
		// last_sent daha yeniyse onu kullan (zaten cooldown'dan geçti).
		intervalDur := time.Duration(rr.IntervalMin) * time.Minute
		if intervalDur <= 0 {
			intervalDur = time.Minute
		}
		cutoff := now.Add(-intervalDur)
		if rr.LastSent != nil && rr.LastSent.After(cutoff) {
			cutoff = *rr.LastSent
		}

		r.dispatchCameraNotification(rr, cameraSerials, now, &cutoff)
	}
}

func (r *SimpleRuleRunner) dispatchCameraNotification(rr newRuleRow, serials []string, now time.Time, cutoff *time.Time) {
	ts := now.Format("02.01.2006 15:04:05")

	for _, serial := range serials {
		imgURL, imgData, imgName := fetchLatestCameraImage(CameraManagementURL, serial, cutoff)
		if imgURL == "" && len(imgData) == 0 {
			continue
		}

		subject := fmt.Sprintf("[SysTrack] Kamera Görüntüsü: %s", serial)
		body := fmt.Sprintf("Kamera: %s\nZaman: %s", serial, ts)

		channel := strings.TrimSpace(strings.ToLower(rr.Channel))
		switch channel {
		case "telegram":
			cfg, err := r.loadTelegramConfig()
			if err != nil || !cfg.Enabled || cfg.BotToken == "" {
				break
			}
			rawSvc := NewTelegramService(r.db)
			if err := rawSvc.SetConfig(cfg); err != nil {
				break
			}
			tgSvc, ok := rawSvc.(*TelegramServiceImpl)
			if !ok {
				break
			}
			ctx, cancel := context.WithTimeout(r.ctx, 30*time.Second)
			caption := fmt.Sprintf("<b>%s</b>\n%s", html.EscapeString(subject), html.EscapeString(body))
			var sendErr error
			for _, chatID := range rr.Recipients {
				chatID = strings.TrimSpace(chatID)
				if chatID == "" {
					continue
				}
				if len(imgData) > 0 {
					sendErr = tgSvc.SendPhotoBytes(ctx, chatID, imgName, imgData, caption)
				} else {
					sendErr = tgSvc.SendPhotoByURL(ctx, chatID, imgURL, caption)
				}
			}
			cancel()
			r.logNotification(rr.ID, 0, channel, rr.Recipients, subject, body, rr.TemplateID, sendErr)
			if sendErr == nil {
				_, _ = r.db.Exec(`UPDATE new_notification_rules SET last_sent_at = NOW() WHERE id = ?`, rr.ID)
			}

		case "email":
			emailCfg, err := r.loadEmailConfig()
			if err != nil || !emailCfg.Enabled {
				break
			}
			logger := NewEmailLogger(r.db)
			rawSvc := NewEmailServiceWithDB(emailCfg, logger, r.db)
			emailSvc, ok := rawSvc.(*emailService)
			if !ok {
				break
			}
			ctx, cancel := context.WithTimeout(r.ctx, 20*time.Second)
			var sendErr error
			if len(imgData) > 0 {
				htmlBody := fmt.Sprintf(
					"<p><strong>%s</strong></p><p>Kamera: %s<br>Zaman: %s</p><img src=\"cid:cam_image\" style=\"max-width:800px;\">",
					html.EscapeString(subject), html.EscapeString(serial), html.EscapeString(ts),
				)
				att := EmailAttachment{
					Filename:    imgName,
					ContentType: "image/jpeg",
					Data:        imgData,
					Size:        int64(len(imgData)),
				}
				sendErr = emailSvc.SendHTMLEmailWithAttachments(ctx, rr.Recipients, subject, htmlBody, []EmailAttachment{att})
			} else {
				fullBody := body
				if imgURL != "" {
					fullBody += "\nGörüntü URL: " + imgURL
				}
				sendErr = emailSvc.SendEmail(ctx, rr.Recipients, subject, fullBody)
			}
			cancel()
			r.logNotification(rr.ID, 0, "email", rr.Recipients, subject, body, rr.TemplateID, sendErr)
			if sendErr == nil {
				_, _ = r.db.Exec(`UPDATE new_notification_rules SET last_sent_at = NOW() WHERE id = ?`, rr.ID)
			}
		}
		break // tek başarılı kamera gönderimi yeter
	}
}

// fetchLatestCameraImage management API'sinden son görüntüyü çeker.
func fetchLatestCameraImage(mgmtURL, serial string, sinceTime *time.Time) (imgURL string, imgData []byte, imgName string) {
	listURL := fmt.Sprintf("%s/api/v1/cameras/%s/images?limit=1", strings.TrimRight(mgmtURL, "/"), serial)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", listURL, nil)
	if err != nil {
		return
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("fetchLatestCameraImage: list error for %s: %v", serial, err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return
	}

	var listPayload struct {
		Images []struct {
			RelativePath string `json:"relative_path"`
			CapturedAt   string `json:"captured_at"`
			CreatedAt    string `json:"created_at"`
		} `json:"images"`
	}
	rawBody, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(rawBody, &listPayload); err != nil || len(listPayload.Images) == 0 {
		return
	}

	img := listPayload.Images[0]
	relPath := img.RelativePath

	// Zaman filtresi: sinceTime her zaman set edilir (asla nil gelmez).
	// capturedStr boşsa veya parse edilemiyorsa görüntü tarihi bilinmiyor — atla.
	if sinceTime != nil {
		capturedStr := img.CapturedAt
		if capturedStr == "" {
			capturedStr = img.CreatedAt
		}
		if capturedStr == "" {
			return // tarih bilinmiyor, eski görüntü olabilir — gönderme
		}
		var captured time.Time
		for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05", "2006-01-02 15:04:05"} {
			if t, parseErr := time.Parse(layout, capturedStr); parseErr == nil {
				captured = t
				break
			}
		}
		if captured.IsZero() || !captured.After(*sinceTime) {
			return // parse başarısız ya da sinceTime'dan eski — gönderme
		}
	}

	if relPath == "" {
		return
	}

	imgURL = fmt.Sprintf("%s/api/v1/cameras/%s/image/%s", strings.TrimRight(mgmtURL, "/"), serial, relPath)
	parts := strings.Split(relPath, "/")
	imgName = parts[len(parts)-1]
	if imgName == "" {
		imgName = "camera_" + serial + ".jpg"
	}

	dlCtx, dlCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer dlCancel()

	dlReq, err := http.NewRequestWithContext(dlCtx, "GET", imgURL, nil)
	if err != nil {
		return imgURL, nil, imgName
	}
	dlResp, err := http.DefaultClient.Do(dlReq)
	if err != nil {
		return imgURL, nil, imgName
	}
	defer dlResp.Body.Close()

	if dlResp.StatusCode < 200 || dlResp.StatusCode >= 300 {
		return imgURL, nil, imgName
	}

	imgData, _ = io.ReadAll(io.LimitReader(dlResp.Body, 10*1024*1024))
	return
}

// ─── Sıvı Teması ──────────────────────────────────────────────────────────────

func (r *SimpleRuleRunner) checkLiquidContactRules(serial string) {
	rows, err := r.db.Query(`
		SELECT id, name, channel, schedule_interval_minutes, recipients, template_id,
		       conditions, last_sent_at, sensor_serial
		FROM new_notification_rules
		WHERE entity_type = 'sensor' AND is_active = 1
	`)
	if err != nil {
		return
	}
	defer rows.Close()

	now := time.Now()
	for rows.Next() {
		var rr newRuleRow
		var recipJSON, condJSON sql.NullString
		var tplID sql.NullInt64
		var lastSent sql.NullTime
		var ruleSerial sql.NullString

		if err := rows.Scan(&rr.ID, &rr.Name, &rr.Channel, &rr.IntervalMin,
			&recipJSON, &tplID, &condJSON, &lastSent, &ruleSerial); err != nil {
			continue
		}
		if recipJSON.Valid {
			_ = json.Unmarshal([]byte(recipJSON.String), &rr.Recipients)
		}
		if tplID.Valid {
			id := int(tplID.Int64)
			rr.TemplateID = &id
		}
		var conds []ruleCondition
		if condJSON.Valid {
			_ = json.Unmarshal([]byte(condJSON.String), &conds)
		}
		if lastSent.Valid {
			rr.LastSent = &lastSent.Time
		}

		if ruleSerial.Valid && ruleSerial.String != "" && ruleSerial.String != serial {
			continue
		}

		hasLC := false
		for _, c := range conds {
			if c.Field == "liquid_contact" {
				hasLC = true
				break
			}
		}
		if !hasLC {
			continue
		}

		// Cooldown
		if rr.LastSent != nil && rr.IntervalMin > 0 {
			if now.Sub(*rr.LastSent) < time.Duration(rr.IntervalMin)*time.Minute {
				continue
			}
		}

		ts := now.Format("02.01.2006 15:04:05")
		subject := fmt.Sprintf("[SysTrack] Sıvı Teması Tespit Edildi: %s", serial)
		body := fmt.Sprintf("Sensör: %s\nDurum: Sıvı teması algılandı\nZaman: %s", serial, ts)
		payload := messagePayload{
			Subject:    subject,
			Body:       body,
			Text:       subject + "\n\n" + body,
			TemplateID: rr.TemplateID,
		}
		if dispErr := r.dispatchChannel(rr, 0, payload); dispErr == nil {
			_, _ = r.db.Exec(`UPDATE new_notification_rules SET last_sent_at = NOW() WHERE id = ?`, rr.ID)
		}
	}
}

// ─── Yardımcı fonksiyonlar ────────────────────────────────────────────────────

func parseSensorSubtype(conds []ruleCondition) string {
	for _, c := range conds {
		if c.Field == "sensor_subtype" {
			if s, ok := c.Value.(string); ok {
				return strings.ToLower(s)
			}
		}
	}
	return "air"
}

func parseCameraSerials(conds []ruleCondition) []string {
	for _, c := range conds {
		if c.Field == "camera_serials" {
			return toStringSliceNotif(c.Value)
		}
	}
	return nil
}

func toStringSliceNotif(v interface{}) []string {
	switch arr := v.(type) {
	case []interface{}:
		var out []string
		for _, item := range arr {
			if s, ok := item.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	case []string:
		return arr
	case string:
		if arr != "" {
			return []string{arr}
		}
	}
	return nil
}

func crackSeverityMeets(riskLevel, minSeverity string) bool {
	if minSeverity == "any" || minSeverity == "" {
		return true
	}
	order := map[string]int{"low": 1, "medium": 2, "high": 3}
	return order[strings.ToLower(riskLevel)] >= order[minSeverity]
}

func sensorFieldValue(r services.SensorReading, field string) (float64, bool) {
	switch field {
	case "temperature", "sicaklik":
		if r.Temperature != nil {
			return *r.Temperature, true
		}
	case "humidity", "nem":
		if r.Humidity != nil {
			return *r.Humidity, true
		}
	case "pressure", "basinc":
		if r.Pressure != nil {
			return *r.Pressure, true
		}
	case "iaq", "hava_kalitesi":
		if r.IAQ != nil {
			return *r.IAQ, true
		}
	}
	return 0, false
}

func sensorFieldLabel(field string) string {
	switch field {
	case "temperature", "sicaklik":
		return "Sıcaklık (°C)"
	case "humidity", "nem":
		return "Nem (%)"
	case "pressure", "basinc":
		return "Basınç (hPa)"
	case "iaq", "hava_kalitesi":
		return "Hava Kalitesi (IAQ)"
	}
	return field
}

func buildSensorCondDesc(conds []ruleCondition, reading services.SensorReading) string {
	parts := make([]string, 0, len(conds))
	for _, c := range conds {
		if c.Field == "liquid_contact" || c.Field == "sensor_subtype" || c.Field == "camera_serials" {
			continue
		}
		label := sensorFieldLabel(c.Field)
		threshold := toFloat64Notif(c.Value)
		if val, ok := sensorFieldValue(reading, c.Field); ok {
			parts = append(parts, fmt.Sprintf("%s = %.2f (%s %.2f)", label, val, c.Operator, threshold))
		} else {
			parts = append(parts, fmt.Sprintf("%s %s %.2f", label, c.Operator, threshold))
		}
	}
	if len(parts) == 0 {
		return "Eşik değeri aşıldı"
	}
	return strings.Join(parts, ", ")
}

func evalSensorCondition(actual float64, op string, threshold float64) bool {
	switch op {
	case "<":
		return actual < threshold
	case "<=":
		return actual <= threshold
	case ">":
		return actual > threshold
	case ">=":
		return actual >= threshold
	case "=", "==":
		return actual == threshold
	case "!=":
		return actual != threshold
	}
	return false
}

func toFloat64Notif(v interface{}) float64 {
	switch val := v.(type) {
	case float64:
		return val
	case float32:
		return float64(val)
	case int:
		return float64(val)
	case int64:
		return float64(val)
	case string:
		var f float64
		fmt.Sscanf(val, "%f", &f)
		return f
	}
	return 0
}

// ─── Telegram foto gönderimi ─────────────────────────────────────────────────

// SendPhotoBytes Telegram botuna binary veri ile fotoğraf gönderir (multipart upload).
func (s *TelegramServiceImpl) SendPhotoBytes(ctx context.Context, chatID, filename string, data []byte, caption string) error {
	if !s.config.Enabled || s.config.BotToken == "" {
		return fmt.Errorf("telegram not configured")
	}
	if chatID == "" {
		chatID = s.config.DefaultChat
	}

	boundary := "SysTrackBoundary7f3d9c"
	var buf bytes.Buffer
	writeField := func(name, value string) {
		fmt.Fprintf(&buf, "--%s\r\nContent-Disposition: form-data; name=%q\r\n\r\n%s\r\n", boundary, name, value)
	}

	writeField("chat_id", chatID)
	if caption != "" {
		writeField("caption", caption)
		writeField("parse_mode", "HTML")
	}
	fmt.Fprintf(&buf, "--%s\r\nContent-Disposition: form-data; name=\"photo\"; filename=%q\r\nContent-Type: image/jpeg\r\n\r\n", boundary, filename)
	buf.Write(data)
	buf.WriteString("\r\n--" + boundary + "--\r\n")

	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/sendPhoto", s.config.BotToken)
	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, &buf)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "multipart/form-data; boundary="+boundary)

	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("telegram sendPhoto HTTP %d: %s", resp.StatusCode, string(b))
	}
	return nil
}

// SendPhotoByURL Telegram botuna URL ile fotoğraf gönderir.
func (s *TelegramServiceImpl) SendPhotoByURL(ctx context.Context, chatID, photoURL, caption string) error {
	if !s.config.Enabled || s.config.BotToken == "" {
		return fmt.Errorf("telegram not configured")
	}
	if chatID == "" {
		chatID = s.config.DefaultChat
	}
	msg := map[string]interface{}{
		"chat_id": chatID,
		"photo":   photoURL,
	}
	if caption != "" {
		msg["caption"] = caption
		msg["parse_mode"] = "HTML"
	}
	msgData, _ := json.Marshal(msg)
	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/sendPhoto", s.config.BotToken)
	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewReader(msgData))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("telegram sendPhoto HTTP %d: %s", resp.StatusCode, string(b))
	}
	return nil
}
