package handlers

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	"systrack/internal/services"

	"github.com/gin-gonic/gin"
)

func getOwnSerial(db *sql.DB) (string, error) {
	var ownSerial string
	if err := db.QueryRow("SELECT serial_number FROM device_license LIMIT 1").Scan(&ownSerial); err != nil {
		return "", err
	}
	if ownSerial == "" {
		return "", sql.ErrNoRows
	}
	return ownSerial, nil
}

func proxyJSONOrNil(url string) ([]byte, int, bool) {
	resp, err := http.Get(url) //nolint:gosec
	if err != nil {
		return nil, 0, false
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	contentType := strings.ToLower(resp.Header.Get("Content-Type"))
	trimmed := strings.TrimSpace(string(body))

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, resp.StatusCode, false
	}
	if !strings.Contains(contentType, "application/json") {
		if !(strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[")) {
			return nil, resp.StatusCode, false
		}
	}
	return body, resp.StatusCode, true
}

// SensorList GET /api/sensors
// Management'tan bu cihaza bagli sensorleri listeler (proxy).
func SensorList(managementURL string, db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		ownSerial, err := getOwnSerial(db)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Cihaz seri numarasi alinamadi"})
			return
		}

		if body, status, ok := proxyJSONOrNil(fmt.Sprintf("%s/api/v1/device-sensors/%s", managementURL, ownSerial)); ok {
			c.Data(status, "application/json; charset=utf-8", body)
			return
		}

		if body, status, ok := proxyJSONOrNil(fmt.Sprintf("%s/api/v1/device-sensor/%s", managementURL, ownSerial)); ok {
			var legacy struct {
				SensorSerial *string `json:"sensor_serial"`
			}
			if err := json.Unmarshal(body, &legacy); err == nil && legacy.SensorSerial != nil && *legacy.SensorSerial != "" {
				c.JSON(status, gin.H{
					"sensors": []gin.H{
						{"serial": *legacy.SensorSerial, "systrack_serial": ownSerial},
					},
				})
				return
			}
			c.JSON(http.StatusOK, gin.H{"sensors": []gin.H{}})
			return
		}

		c.JSON(http.StatusOK, gin.H{"sensors": []gin.H{}})
	}
}

// SensorStateProxy GET /api/sensors/:serial/state
// Management'tan belirli bir sensorun anlik verisini ceker (proxy).
func SensorStateProxy(managementURL string) gin.HandlerFunc {
	return func(c *gin.Context) {
		serial := c.Param("serial")
		url := fmt.Sprintf("%s/api/v1/sensors/state/%s", managementURL, serial)
		resp, err := http.Get(url) //nolint:gosec
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": "Yonetim sunucusuna baglanilamadi"})
			return
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		c.Data(resp.StatusCode, "application/json; charset=utf-8", body)
	}
}

// SensorLink POST /api/sensor-link
// Kullanici arayuzunden girilen sensor seri numarasini
// management sunucusuna iletir ve sensor servisini gunceller.
func SensorLink(sensorService *services.SensorMQTTService, managementURL string, db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			SensorSerial string `json:"sensor_serial"`
		}
		if err := c.ShouldBindJSON(&req); err != nil || req.SensorSerial == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "sensor_serial gerekli"})
			return
		}

		ownSerial, err := getOwnSerial(db)
		if err != nil {
			log.Printf("SensorLink: device_license okunamadi: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Cihaz seri numarasi alinamadi"})
			return
		}

		body, _ := json.Marshal(map[string]string{
			"systrack_serial": ownSerial,
			"sensor_serial":   req.SensorSerial,
		})
		url := fmt.Sprintf("%s/api/v1/sensors/self-assign", managementURL)
		resp, err := http.Post(url, "application/json", bytes.NewReader(body)) //nolint:gosec
		if err != nil {
			log.Printf("SensorLink: management baglantisi hatasi: %v", err)
			c.JSON(http.StatusBadGateway, gin.H{"error": "Yonetim sunucusuna baglanilamadi"})
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "sensor_not_found"})
			return
		}
		if resp.StatusCode != http.StatusOK {
			b, _ := io.ReadAll(resp.Body)
			log.Printf("SensorLink: management %d: %s", resp.StatusCode, string(b))
			c.JSON(http.StatusBadGateway, gin.H{"error": "Sunucu hatasi"})
			return
		}

		sensorService.SetLinkedSerial(req.SensorSerial)

		log.Printf("SensorLink: cihaz=%s sensor=%s baglandi", ownSerial, req.SensorSerial)
		c.JSON(http.StatusOK, gin.H{"status": "ok", "sensor_serial": req.SensorSerial})
	}
}
