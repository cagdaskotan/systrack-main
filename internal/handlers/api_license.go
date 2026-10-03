package handlers

import (
	"database/sql"
	"net/http"
	"strconv"
	"time"

	"systrack/internal/auth"

	"github.com/gin-gonic/gin"
)

type ExtendLicenseRequest struct {
	ExpiryDate string `json:"expiry_date" binding:"required"` // Format: "2025-10-01T00:00:00Z"
}

// ExtendUserLicense extends a user's license expiry date
func ExtendUserLicense(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		idStr := c.Param("id")
		id, err := strconv.Atoi(idStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID"})
			return
		}

		var req ExtendLicenseRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		// Parse expiry date
		expiryDate, err := time.Parse(time.RFC3339, req.ExpiryDate)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid expiry date format. Use RFC3339 format (e.g., 2025-10-01T00:00:00Z)"})
			return
		}

		// Extend license
		err = auth.ExtendLicense(db, id, expiryDate)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to extend license"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"message": "License extended successfully"})
	}
}

// CheckUserLicense checks if a user's license has expired
func CheckUserLicense(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		idStr := c.Param("id")
		id, err := strconv.Atoi(idStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID"})
			return
		}

		expired, err := auth.CheckLicenseExpiry(db, id)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to check license"})
			return
		}

		user, err := auth.GetUserByID(db, id)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"expired":        expired,
			"license_expiry": user.LicenseExpiry,
			"is_active":      user.IsActive,
		})
	}
}

// GetLicenseStatus returns license status for all users
func GetLicenseStatus(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		users, err := auth.GetAllUsers(db)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch users"})
			return
		}

		var licenseStatus []gin.H
		now := time.Now()

		for _, user := range users {
			expired := false
			daysUntilExpiry := 0

			if user.LicenseExpiry != nil {
				expired = user.LicenseExpiry.Before(now)
				if !expired {
					daysUntilExpiry = int(user.LicenseExpiry.Sub(now).Hours() / 24)
				}
			}

			licenseStatus = append(licenseStatus, gin.H{
				"user_id":           user.ID,
				"email":             user.Email,
				"license_expiry":    user.LicenseExpiry,
				"expired":           expired,
				"days_until_expiry": daysUntilExpiry,
				"is_active":         user.IsActive,
			})
		}

		c.JSON(http.StatusOK, gin.H{"license_status": licenseStatus})
	}
}

// CheckUpdateNotification checks if there's a pending update notification to show
func CheckUpdateNotification(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var showNotification bool
		var updateVersion sql.NullString

		err := db.QueryRow(`
			SELECT show_update_notification, last_update_version
			FROM device_license
			LIMIT 1
		`).Scan(&showNotification, &updateVersion)

		if err != nil {
			if err == sql.ErrNoRows {
				c.JSON(http.StatusOK, gin.H{
					"show_notification": false,
					"update_version":    nil,
				})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to check notification"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"show_notification": showNotification,
			"update_version":    updateVersion.String,
		})
	}
}

// DismissUpdateNotification marks the update notification as seen
func DismissUpdateNotification(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		_, err := db.Exec(`
			UPDATE device_license
			SET show_update_notification = FALSE
		`)

		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to dismiss notification"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"message": "Notification dismissed"})
	}
}
