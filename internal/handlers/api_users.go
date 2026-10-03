package handlers

import (
	"database/sql"
	"net/http"
	"strconv"
	"time"

	"systrack/internal/auth"
	"systrack/internal/config"

	"github.com/gin-gonic/gin"
)

type LoginRequest struct {
	Email    string `json:"email" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type LoginResponse struct {
	Token     string          `json:"token"`
	ExpiresAt int64           `json:"expires_at"`
	User      auth.PublicUser `json:"user"`
}

type CreateUserRequest struct {
	Email         string `json:"email" binding:"required,email"`
	Password      string `json:"password" binding:"required,min=6"`
	Role          string `json:"role" binding:"required,oneof=admin user viewer"`
	MaxTargets    int    `json:"max_targets"`
	IPQueryLimit  int    `json:"ip_query_limit"`
	LicenseExpiry string `json:"license_expiry"` // Format: "2025-10-01T00:00:00Z"
}

type UpdateUserRequest struct {
	Email         string `json:"email" binding:"required,email"`
	Role          string `json:"role" binding:"required,oneof=admin user viewer"`
	MaxTargets    int    `json:"max_targets"`
	IPQueryLimit  int    `json:"ip_query_limit"`
	IsActive      bool   `json:"is_active"`
	LicenseExpiry string `json:"license_expiry"` // Format: "2025-10-01T00:00:00Z"
}

type ResetPasswordRequest struct {
	NewPassword string `json:"new_password" binding:"required,min=6"`
}

func Login(db *sql.DB, cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req LoginRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		user, err := auth.AuthenticateUser(db, req.Email, req.Password)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid credentials"})
			return
		}

		tokenResp, err := auth.GenerateToken(user.ID, user.Email, user.Role, cfg.Auth.JWTSecret)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate token"})
			return
		}

		c.JSON(http.StatusOK, LoginResponse{
			Token:     tokenResp.Token,
			ExpiresAt: tokenResp.ExpiresAt,
			User: auth.PublicUser{
				ID:    user.ID,
				Email: user.Email,
				Role:  user.Role,
			},
		})
	}
}

func GetMe(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, _ := c.Get("user_id")
		id := userID.(int)

		user, err := auth.GetUserByID(db, id)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
			return
		}

		c.JSON(http.StatusOK, auth.PublicUser{
			ID:            user.ID,
			Email:         user.Email,
			Role:          user.Role,
			MaxTargets:    user.MaxTargets,
			IsActive:      user.IsActive,
			LicenseExpiry: user.LicenseExpiry,
			LastLoginAt:   user.LastLoginAt,
			CreatedAt:     user.CreatedAt,
			UpdatedAt:     user.UpdatedAt,
		})
	}
}

func GetUsers(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		users, err := auth.GetAllUsers(db)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch users"})
			return
		}

		// Convert to PublicUser format
		var publicUsers []auth.PublicUser
		for _, user := range users {
			publicUsers = append(publicUsers, auth.PublicUser{
				ID:            user.ID,
				Email:         user.Email,
				Role:          user.Role,
				MaxTargets:    user.MaxTargets,
				IsActive:      user.IsActive,
				LicenseExpiry: user.LicenseExpiry,
				LastLoginAt:   user.LastLoginAt,
				CreatedAt:     user.CreatedAt,
				UpdatedAt:     user.UpdatedAt,
			})
		}

		c.JSON(http.StatusOK, gin.H{"users": publicUsers})
	}
}

func CreateUser(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req CreateUserRequest

		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		exists, err := auth.UserExists(db, req.Email)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to check user existence"})
			return
		}
		if exists {
			c.JSON(http.StatusConflict, gin.H{"error": "User already exists"})
			return
		}

		// Parse license expiry date
		var licenseExpiry *time.Time
		if req.LicenseExpiry != "" {
			parsedTime, err := time.Parse(time.RFC3339, req.LicenseExpiry)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid license expiry date format. Use RFC3339 format (e.g., 2025-10-01T00:00:00Z)"})
				return
			}
			licenseExpiry = &parsedTime
		}

		user, err := auth.CreateUser(db, req.Email, req.Password, req.Role, req.MaxTargets, req.IPQueryLimit, licenseExpiry)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create user"})
			return
		}

		// Initialize module permissions for the new user based on their role
		if err := initializeModulePermissions(db, user.ID, user.Role); err != nil {
			// Log error but don't fail user creation
			// The admin can manually set permissions later
			c.JSON(http.StatusCreated, gin.H{
				"message": "User created but module permissions initialization failed",
				"user": auth.PublicUser{
					ID:            user.ID,
					Email:         user.Email,
					Role:          user.Role,
					MaxTargets:    user.MaxTargets,
					IsActive:      user.IsActive,
					LicenseExpiry: user.LicenseExpiry,
					LastLoginAt:   user.LastLoginAt,
					CreatedAt:     user.CreatedAt,
					UpdatedAt:     user.UpdatedAt,
				},
			})
			return
		}

		c.JSON(http.StatusCreated, auth.PublicUser{
			ID:            user.ID,
			Email:         user.Email,
			Role:          user.Role,
			MaxTargets:    user.MaxTargets,
			IsActive:      user.IsActive,
			LicenseExpiry: user.LicenseExpiry,
			LastLoginAt:   user.LastLoginAt,
			CreatedAt:     user.CreatedAt,
			UpdatedAt:     user.UpdatedAt,
		})
	}
}

func UpdateUser(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		idStr := c.Param("id")
		id, err := strconv.Atoi(idStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID"})
			return
		}

		var req UpdateUserRequest

		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		// Email güncelleniyorsa kontrol et
		exists, err := auth.UserExists(db, req.Email)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to check user existence"})
			return
		}
		if exists {
			// Check if it's the same user
			user, err := auth.GetUserByID(db, id)
			if err != nil || user.Email != req.Email {
				c.JSON(http.StatusConflict, gin.H{"error": "Email already exists"})
				return
			}
		}

		// Parse license expiry date
		var licenseExpiry *time.Time
		if req.LicenseExpiry != "" {
			parsedTime, err := time.Parse(time.RFC3339, req.LicenseExpiry)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid license expiry date format. Use RFC3339 format (e.g., 2025-10-01T00:00:00Z)"})
				return
			}
			licenseExpiry = &parsedTime
		}

		// Kullanıcıyı güncelle
		err = auth.UpdateUser(db, id, req.Email, req.Role, req.MaxTargets, req.IPQueryLimit, req.IsActive, licenseExpiry)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update user"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"message": "User updated successfully"})
	}
}

func DeleteUser(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		idStr := c.Param("id")
		id, err := strconv.Atoi(idStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID"})
			return
		}

		// Kendini silmeyi engelle
		userID, _ := c.Get("user_id")
		if userID.(int) == id {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Cannot delete yourself"})
			return
		}

		err = auth.DeleteUser(db, id)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete user"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"message": "User deleted successfully"})
	}
}

// ResetUserPassword resets a user's password
func ResetUserPassword(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		idStr := c.Param("id")
		id, err := strconv.Atoi(idStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID"})
			return
		}

		var req ResetPasswordRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		// Check if user exists
		_, err = auth.GetUserByID(db, id)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
			return
		}

		// Hash new password
		hashedPassword, err := auth.HashPassword(req.NewPassword)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to hash password"})
			return
		}

		// Update password
		now := time.Now()
		query := `UPDATE users SET pass_hash = ?, updated_at = ? WHERE id = ?`
		_, err = db.Exec(query, hashedPassword, now, id)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to reset password"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"message": "Password reset successfully"})
	}
}

// GetIPQueryLimit returns the user's IP query limit information
func GetIPQueryLimit(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		idStr := c.Param("id")
		id, err := strconv.Atoi(idStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID"})
			return
		}

		// Get current user ID from JWT token
		currentUserID, exists := c.Get("user_id")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
			return
		}

		currentUserIDInt, ok := currentUserID.(int)
		if !ok {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Invalid user ID format"})
			return
		}

		// Users can only get their own limit info, unless they're admin
		var currentUserRole string
		err = db.QueryRow("SELECT role FROM users WHERE id = ?", currentUserIDInt).Scan(&currentUserRole)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get user role"})
			return
		}

		if currentUserRole != "admin" && currentUserIDInt != id {
			c.JSON(http.StatusForbidden, gin.H{"error": "You can only get your own limit info"})
			return
		}

		// Get user's IP query limit
		var userQueryLimit int
		err = db.QueryRow("SELECT ip_query_limit FROM users WHERE id = ?", id).Scan(&userQueryLimit)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get user query limit"})
			return
		}

		// Get today's query count
		var todayQueryCount int
		err = db.QueryRow(`
			SELECT COUNT(*) FROM ip_query_logs 
			WHERE user_id = ? AND query_date = CURDATE()
		`, id).Scan(&todayQueryCount)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get query count"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"limit": userQueryLimit,
			"used":  todayQueryCount,
		})
	}
}

// initializeModulePermissions creates default module permissions for a new user based on their role
func initializeModulePermissions(db *sql.DB, userID int, role string) error {
	// Define default permissions for each role
	type ModulePermission struct {
		ModuleName string
		CanView    bool
		CanEdit    bool
	}

	var permissions []ModulePermission

	switch role {
	case "admin":
		// Admin has full access to all modules (view + edit)
		permissions = []ModulePermission{
			{"dashboard", true, true},
			{"targets", true, true},
			{"ip_scanner", true, true},
			{"inventory", true, true},
			{"notifications", true, true},
			{"notification_settings", true, true},
			{"notification_templates", true, true},
			{"reporting", true, true},
			{"server_metrics", true, true},
			{"user_management", true, true},
			{"module_permissions", true, true},
			{"backup", true, true},
            {"network_settings", true, true},
		}

	case "user":
		// User has default access to operational modules
		permissions = []ModulePermission{
			{"dashboard", true, false},           // View only (locked)
			{"targets", true, true},              // View + Edit
			{"ip_scanner", true, true},           // View + Edit
			{"inventory", true, true},            // View + Edit
			{"notifications", true, false},       // View only
			{"notification_settings", true, false}, // View only
			{"notification_templates", true, false},
			{"reporting", true, false},           // View only
			{"server_metrics", true, false},
			{"user_management", false, false},    // No access (admin can grant)
			{"module_permissions", false, false}, // No access (admin can grant)
			{"backup", false, false},             // No access (admin can grant)
            {"network_settings", false, false},    // No access (admin can grant)
		}

	case "viewer":
		// Viewer has read-only access to operational modules
		permissions = []ModulePermission{
			{"dashboard", true, false},           // View only (locked)
			{"targets", true, false},             // View only
			{"ip_scanner", true, false},          // View only
			{"inventory", true, false},           // View only
			{"notifications", true, false},       // View only
			{"notification_settings", true, false}, // View only
			{"notification_templates", true, false},
			{"reporting", true, false},           // View only
			{"server_metrics", true, false},
			{"user_management", false, false},    // No access (admin can grant)
			{"module_permissions", false, false}, // No access (admin can grant)
			{"backup", false, false},             // No access (admin can grant)
            {"network_settings", false, false},    // No access (admin can grant)
		}

	default:
		return nil // Unknown role, no permissions
	}

	// Insert all permissions in a transaction
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	query := `INSERT INTO module_permissions (user_id, module_name, can_view, can_edit)
			  VALUES (?, ?, ?, ?)
			  ON DUPLICATE KEY UPDATE
			  can_view = VALUES(can_view),
			  can_edit = VALUES(can_edit)`

	for _, perm := range permissions {
		_, err := tx.Exec(query, userID, perm.ModuleName, perm.CanView, perm.CanEdit)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}
