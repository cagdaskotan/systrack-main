package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"systrack/internal/notifications"

	"github.com/gin-gonic/gin"
)

type liquidHistoryEntry struct {
	ID              int64     `json:"id"`
	SensorSerial    string    `json:"sensor_serial"`
	StatusText      string    `json:"status_text"`
	AnalogValue     *int      `json:"analog_value"`
	ContactDetected bool      `json:"contact_detected"`
	RecordedAt      time.Time `json:"recorded_at"`
	RawPayload      *string   `json:"raw_payload,omitempty"`
}

// LiquidSensorDelete DELETE /api/liquid-sensors/:serial — removes all history for a serial
func LiquidSensorDelete(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		serial := strings.TrimSpace(c.Param("serial"))
		if serial == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "serial gerekli"})
			return
		}
		_, err := db.Exec(`DELETE FROM liquid_sensor_history WHERE sensor_serial = ?`, serial)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "silinemedi"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true})
	}
}

// LiquidSensorList GET /api/liquid-sensors — distinct serials from history
func LiquidSensorList(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		rows, err := db.Query(`SELECT DISTINCT sensor_serial FROM liquid_sensor_history ORDER BY sensor_serial`)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "liquid sensors listelenemedi"})
			return
		}
		defer rows.Close()
		serials := make([]string, 0)
		for rows.Next() {
			var serial string
			if rows.Scan(&serial) == nil && serial != "" {
				serials = append(serials, serial)
			}
		}
		c.JSON(http.StatusOK, gin.H{"liquid_sensors": serials})
	}
}

func LiquidSensorStateProxy(managementURL string, db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		serial := strings.TrimSpace(c.Param("serial"))
		url := fmt.Sprintf("%s/api/v1/sensors/state/%s", strings.TrimRight(managementURL, "/"), serial)

		resp, err := http.Get(url) //nolint:gosec
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": "Yonetim sunucusuna baglanilamadi"})
			return
		}
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			_ = recordLiquidState(db, serial, body)
		}

		c.Data(resp.StatusCode, "application/json; charset=utf-8", body)
	}
}

func GetLiquidSensorHistory(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		serial := strings.TrimSpace(c.Param("serial"))
		if serial == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "sensor serial gerekli"})
			return
		}

		limit := 100
		if raw := strings.TrimSpace(c.DefaultQuery("limit", "100")); raw != "" {
			var parsed int
			if _, err := fmt.Sscanf(raw, "%d", &parsed); err == nil && parsed > 0 {
				if parsed > 500 {
					parsed = 500
				}
				limit = parsed
			}
		}

		rows, err := db.Query(`
			SELECT id, sensor_serial, status_text, analog_value, contact_detected, recorded_at
			FROM liquid_sensor_history
			WHERE sensor_serial = ?
			ORDER BY recorded_at DESC, id DESC
			LIMIT ?
		`, serial, limit)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "liquid history okunamadi"})
			return
		}
		defer rows.Close()

		history := make([]liquidHistoryEntry, 0)
		for rows.Next() {
			var item liquidHistoryEntry
			var analog sql.NullInt64
			var contactInt int
			if err := rows.Scan(&item.ID, &item.SensorSerial, &item.StatusText, &analog, &contactInt, &item.RecordedAt); err != nil {
				continue
			}
			if analog.Valid {
				value := int(analog.Int64)
				item.AnalogValue = &value
			}
			item.ContactDetected = contactInt == 1
			history = append(history, item)
		}

		c.JSON(http.StatusOK, gin.H{"history": history})
	}
}

func recordLiquidState(db *sql.DB, serial string, body []byte) error {
	if db == nil || serial == "" || len(body) == 0 {
		return nil
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil
	}

	statusText, analogValue, ok := extractLiquidFields(payload)
	if !ok {
		return nil
	}
	contactDetected := isLiquidContactDetected(statusText)

	var lastStatus string
	var lastAnalog sql.NullInt64
	err := db.QueryRow(`
		SELECT status_text, analog_value
		FROM liquid_sensor_history
		WHERE sensor_serial = ?
		ORDER BY recorded_at DESC, id DESC
		LIMIT 1
	`, serial).Scan(&lastStatus, &lastAnalog)
	if err != nil && err != sql.ErrNoRows {
		return err
	}

	if err == nil {
		lastAnalogMatches := (!lastAnalog.Valid && analogValue == nil) ||
			(lastAnalog.Valid && analogValue != nil && int(lastAnalog.Int64) == *analogValue)
		if strings.EqualFold(strings.TrimSpace(lastStatus), strings.TrimSpace(statusText)) && lastAnalogMatches {
			return nil
		}
	}

	rawPayload := strings.TrimSpace(string(body))
	_, err = db.Exec(`
		INSERT INTO liquid_sensor_history (sensor_serial, status_text, analog_value, contact_detected, raw_payload)
		VALUES (?, ?, ?, ?, ?)
	`, serial, statusText, analogValue, boolToTinyInt(contactDetected), rawPayload)
	if err == nil && contactDetected {
		notifications.NotifyLiquidContact(serial)
	}
	return err
}

func extractLiquidFields(payload map[string]interface{}) (string, *int, bool) {
	if payload == nil {
		return "", nil, false
	}

	statusText := firstNonEmptyString(
		payload["su_analog_durum"],
		payload["water_status"],
		payload["liquid_status"],
		payload["su_durumu"],
	)

	analogValue := firstIntValue(
		payload["su_analog"],
		payload["water_analog"],
		payload["liquid_analog"],
	)

	if statusText == "" && analogValue == nil {
		return "", nil, false
	}
	if statusText == "" {
		statusText = "bilinmiyor"
	}

	return statusText, analogValue, true
}

func firstNonEmptyString(values ...interface{}) string {
	for _, value := range values {
		text := strings.TrimSpace(fmt.Sprint(value))
		if text == "" || text == "<nil>" {
			continue
		}
		return text
	}
	return ""
}

func firstIntValue(values ...interface{}) *int {
	for _, value := range values {
		switch v := value.(type) {
		case float64:
			out := int(v)
			return &out
		case float32:
			out := int(v)
			return &out
		case int:
			out := v
			return &out
		case int64:
			out := int(v)
			return &out
		case json.Number:
			if i64, err := v.Int64(); err == nil {
				out := int(i64)
				return &out
			}
		case string:
			var parsed int
			if _, err := fmt.Sscanf(strings.TrimSpace(v), "%d", &parsed); err == nil {
				return &parsed
			}
		}
	}
	return nil
}

func isLiquidContactDetected(status string) bool {
	s := strings.ToLower(strings.TrimSpace(status))
	if s == "" {
		return false
	}
	if strings.Contains(s, "kuru") || strings.Contains(s, "dry") || strings.Contains(s, "normal") {
		return false
	}
	return true
}

func boolToTinyInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
