package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"systrack/internal/license"

	"github.com/gin-gonic/gin"
)

// GetDeviceInfo returns device information for activation form
func GetDeviceInfo(c *gin.Context) {
	db := c.MustGet("db").(*sql.DB)

	// Load license from database
	var (
		serialNumber string
		macAddress   string
		status       int
	)

	err := db.QueryRow(`
		SELECT serial_number, mac_address, status
		FROM device_license
		LIMIT 1
	`).Scan(&serialNumber, &macAddress, &status)

	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{
				"success": false,
				"error":   "No device license found",
			})
			return
		}

		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Database error",
		})
		return
	}

	// MAC address is set during manufacturing - don't auto-update
	// Production devices should have pre-configured MAC addresses in device_license table

	c.JSON(http.StatusOK, gin.H{
		"success":       true,
		"serial_number": serialNumber,
		"mac_address":   macAddress,
		"status":        status,
		"status_name":   license.GetStatusName(status),
		"management_url": license.ManagementServerURL(),
	})
}

// MarkActivationSubmitted marks that activation request was submitted
func MarkActivationSubmitted(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Activation request submitted successfully",
	})
}

// CheckActivationStatus checks if device has been activated
func CheckActivationStatus(c *gin.Context) {
	db := c.MustGet("db").(*sql.DB)

	var (
		status      int
		activatedAt sql.NullTime
	)

	err := db.QueryRow(`
		SELECT status, activated_at
		FROM device_license
		LIMIT 1
	`).Scan(&status, &activatedAt)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Database error",
		})
		return
	}

	isActivated := status == license.StatusActive ||
		status == license.StatusTrial ||
		activatedAt.Valid

	c.JSON(http.StatusOK, gin.H{
		"success":     true,
		"status":      status,
		"status_name": license.GetStatusName(status),
		"activated":   isActivated,
	})
}

// SyncActivationFromServer checks management server and updates local database if activated
func SyncActivationFromServer(c *gin.Context) {
	db := c.MustGet("db").(*sql.DB)

	// Get serial number from local database
	var serialNumber string
	err := db.QueryRow(`SELECT serial_number FROM device_license LIMIT 1`).Scan(&serialNumber)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to get serial number",
		})
		return
	}

	// Get management server URL
	managementURL := license.ManagementServerURL()

	// Check activation status on management server
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(fmt.Sprintf("%s/api/v1/activation/status/%s", managementURL, serialNumber))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"error":   "Failed to connect to management server",
			"synced":  false,
		})
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"error":   "Failed to read response",
			"synced":  false,
		})
		return
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"error":   "Failed to parse response",
			"synced":  false,
		})
		return
	}

	// Check if status is email_confirmed
	status, ok := result["status"].(string)
	if ok && status == "email_confirmed" {
		// Update local database
		_, err = db.Exec(`
			UPDATE device_license
			SET status = ?, activated_at = NOW()
			WHERE serial_number = ?
		`, license.StatusActive, serialNumber)

		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error":   "Failed to update local database",
				"synced":  false,
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"success":   true,
			"activated": true,
			"synced":    true,
			"message":   "Device activated successfully",
		})
		return
	}

	// Not activated yet
	c.JSON(http.StatusOK, gin.H{
		"success":   true,
		"activated": false,
		"synced":    false,
		"status":    status,
	})
}

// GetBlockStatus returns whether the device is currently blocked
// Used by blocked.html to check if block has been lifted
func GetBlockStatus(c *gin.Context) {
	db := c.MustGet("db").(*sql.DB)

	var status int
	err := db.QueryRow(`
		SELECT status FROM device_license LIMIT 1
	`).Scan(&status)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Database error",
		})
		return
	}

	isBlocked := status == license.StatusBlocked
	isExpired := status == license.StatusExpired

	c.JSON(http.StatusOK, gin.H{
		"success":     true,
		"blocked":     isBlocked,
		"expired":     isExpired,
		"status":      status,
		"status_name": license.GetStatusName(status),
	})
}
