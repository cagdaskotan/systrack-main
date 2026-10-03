package handlers

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
)

// CameraList GET /api/cameras
// Lists cameras assigned to this SysTrack device by proxying the management server.
func CameraList(managementURL string, db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		ownSerial, err := getOwnSerial(db)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Cihaz seri numarasi alinamadi"})
			return
		}

		url := fmt.Sprintf("%s/api/v1/device-cameras/%s", strings.TrimRight(managementURL, "/"), ownSerial)
		if body, status, ok := proxyJSONOrNil(url); ok {
			c.Data(status, "application/json; charset=utf-8", body)
			return
		}

		c.JSON(http.StatusOK, gin.H{"cameras": []gin.H{}})
	}
}

// CameraLink POST /api/camera-link
// Assigns a camera serial to this SysTrack device on the management server.
func CameraLink(managementURL string, db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			CameraSerial string `json:"camera_serial"`
		}
		if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.CameraSerial) == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "camera_serial gerekli"})
			return
		}

		ownSerial, err := getOwnSerial(db)
		if err != nil {
			log.Printf("CameraLink: device_license okunamadi: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Cihaz seri numarasi alinamadi"})
			return
		}

		cameraSerial := strings.ToUpper(strings.TrimSpace(req.CameraSerial))
		body, _ := json.Marshal(map[string]string{
			"systrack_serial": ownSerial,
			"camera_serial":   cameraSerial,
		})
		url := fmt.Sprintf("%s/api/v1/cameras/self-assign", strings.TrimRight(managementURL, "/"))
		resp, err := http.Post(url, "application/json", bytes.NewReader(body)) //nolint:gosec
		if err != nil {
			log.Printf("CameraLink: management baglantisi hatasi: %v", err)
			c.JSON(http.StatusBadGateway, gin.H{"error": "Yonetim sunucusuna baglanilamadi"})
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusNotFound {
			b, _ := io.ReadAll(resp.Body)
			if strings.Contains(string(b), "camera_not_found") {
				c.JSON(http.StatusNotFound, gin.H{"error": "camera_not_found"})
				return
			}
			log.Printf("CameraLink: management camera route missing or not deployed: %s", string(b))
			c.JSON(http.StatusBadGateway, gin.H{"error": "management_camera_route_missing"})
			return
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			b, _ := io.ReadAll(resp.Body)
			log.Printf("CameraLink: management %d: %s", resp.StatusCode, string(b))
			c.JSON(http.StatusBadGateway, gin.H{"error": "Sunucu hatasi"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"status": "ok", "camera_serial": cameraSerial})
	}
}

// CameraImagesProxy GET /api/cameras/:serial/images
func CameraImagesProxy(managementURL string) gin.HandlerFunc {
	return func(c *gin.Context) {
		serial := strings.TrimSpace(c.Param("serial"))
		limit := strings.TrimSpace(c.DefaultQuery("limit", "10"))
		requestURL := fmt.Sprintf("%s/api/v1/cameras/%s/images?limit=%s", strings.TrimRight(managementURL, "/"), url.PathEscape(serial), url.QueryEscape(limit))
		resp, err := http.Get(requestURL) //nolint:gosec
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": "Yonetim sunucusuna baglanilamadi"})
			return
		}
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			c.Data(resp.StatusCode, "application/json; charset=utf-8", body)
			return
		}

		var payload struct {
			Images []map[string]interface{} `json:"images"`
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			c.Data(resp.StatusCode, "application/json; charset=utf-8", body)
			return
		}
		for _, img := range payload.Images {
			if rel, ok := img["relative_path"].(string); ok && rel != "" {
				img["image_url"] = "/api/cameras/" + serial + "/image/" + rel
			}
		}
		c.JSON(http.StatusOK, payload)
	}
}

// CameraImageProxy GET /api/cameras/:serial/image/*path
func CameraImageProxy(managementURL string) gin.HandlerFunc {
	return func(c *gin.Context) {
		serial := strings.TrimSpace(c.Param("serial"))
		relPath := strings.TrimPrefix(c.Param("path"), "/")
		requestURL := fmt.Sprintf("%s/api/v1/cameras/%s/image/%s", strings.TrimRight(managementURL, "/"), url.PathEscape(serial), escapePathSegments(relPath))

		resp, err := http.Get(requestURL) //nolint:gosec
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": "Yonetim sunucusuna baglanilamadi"})
			return
		}
		defer resp.Body.Close()

		for key, values := range resp.Header {
			if strings.EqualFold(key, "Content-Length") {
				continue
			}
			for _, value := range values {
				c.Writer.Header().Add(key, value)
			}
		}
		c.Status(resp.StatusCode)
		_, _ = io.Copy(c.Writer, resp.Body)
	}
}

func escapePathSegments(path string) string {
	parts := strings.Split(strings.ReplaceAll(path, "\\", "/"), "/")
	for i, part := range parts {
		parts[i] = url.PathEscape(part)
	}
	return strings.Join(parts, "/")
}
