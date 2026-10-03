package handlers

import (
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
)

// SensorHourlyPoint tek bir saatlik sensor okumasini temsil eder.
type SensorHourlyPoint struct {
	Hour           string   `json:"hour"`
	Temperature    *float64 `json:"temperature"`
	CPUTemperature *float64 `json:"cpu_temperature"`
	Humidity       *float64 `json:"humidity"`
	Pressure       *float64 `json:"pressure"`
	Gas            *float64 `json:"gas"`
}

// GetSensorHourly son 24 saatlik sensor verilerini dondurur.
// Secili sensor verilirse management'tan sensor bazli veriyi ceker.
// Fallback olarak yerel tablodan mevcut tek-kayitli yapiyi korur.
func GetSensorHourly(db *sql.DB, managementURL string) gin.HandlerFunc {
	return func(c *gin.Context) {
		serial := c.Query("serial")
		shouldUseLocalFallback := serial == ""

		if serial != "" && managementURL != "" {
			u := managementURL + "/api/v1/sensors/data/" + url.PathEscape(serial)
			resp, err := http.Get(u) //nolint:gosec
			if err == nil {
				defer resp.Body.Close()
				body, _ := io.ReadAll(resp.Body)
				contentType := strings.ToLower(resp.Header.Get("Content-Type"))
				trimmed := strings.TrimSpace(string(body))
				if resp.StatusCode >= 200 && resp.StatusCode < 300 &&
					(strings.Contains(contentType, "application/json") ||
						strings.HasPrefix(trimmed, "[") ||
						strings.HasPrefix(trimmed, "{")) {
					c.Data(resp.StatusCode, "application/json; charset=utf-8", body)
					return
				}
			}

			ownSerial, err := getOwnSerial(db)
			if err == nil {
				linkBody, _, ok := proxyJSONOrNil(managementURL + "/api/v1/device-sensor/" + url.PathEscape(ownSerial))
				if ok {
					var legacy struct {
						SensorSerial *string `json:"sensor_serial"`
					}
					if jsonErr := json.Unmarshal(linkBody, &legacy); jsonErr == nil &&
						legacy.SensorSerial != nil && *legacy.SensorSerial == serial {
						shouldUseLocalFallback = true
					}
				}
			}
		}

		if !shouldUseLocalFallback {
			c.JSON(http.StatusOK, []SensorHourlyPoint{})
			return
		}

		rows, err := db.QueryContext(c.Request.Context(), `
			SELECT DATE_FORMAT(hour_ts,'%H:00') AS hour,
			       temperature, cpu_temperature,
			       humidity, pressure, gas
			FROM sensor_hourly
			ORDER BY hour_ts ASC
			LIMIT 24
		`)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		defer rows.Close()

		var points []SensorHourlyPoint
		for rows.Next() {
			var p SensorHourlyPoint
			if err := rows.Scan(&p.Hour, &p.Temperature, &p.CPUTemperature,
				&p.Humidity, &p.Pressure, &p.Gas); err != nil {
				continue
			}
			points = append(points, p)
		}
		if points == nil {
			points = []SensorHourlyPoint{}
		}
		c.JSON(http.StatusOK, points)
	}
}
