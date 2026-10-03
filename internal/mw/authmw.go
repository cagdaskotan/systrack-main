package mw

import (
	"database/sql"
	"net/http"
	"strings"

	"systrack/internal/auth"

	"github.com/gin-gonic/gin"
)

func AuthRequired(jwtSecret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Authorization header required"})
			c.Abort()
			return
		}

		// Bearer token formatını kontrol et
		tokenParts := strings.Split(authHeader, " ")
		if len(tokenParts) != 2 || tokenParts[0] != "Bearer" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid authorization header format"})
			c.Abort()
			return
		}

		tokenString := tokenParts[1]
		claims, err := auth.ValidateToken(tokenString, jwtSecret)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid token"})
			c.Abort()
			return
		}

		// Kullanıcı bilgilerini context'e ekle
		c.Set("user_id", claims.UserID)
		c.Set("user_email", claims.Email)
		c.Set("user_role", claims.Role)

		c.Next()
	}
}

func RequireRole(requiredRole string) gin.HandlerFunc {
	return func(c *gin.Context) {
		userRole, exists := c.Get("user_role")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "User role not found"})
			c.Abort()
			return
		}

		role := userRole.(string)

		// Role hierarchy: admin > user > viewer
		roleLevels := map[string]int{
			"viewer": 1,
			"user":   2,
			"admin":  3,
		}

		userLevel, userExists := roleLevels[role]
		requiredLevel, requiredExists := roleLevels[requiredRole]

		if !userExists || !requiredExists || userLevel < requiredLevel {
			c.JSON(http.StatusForbidden, gin.H{"error": "Insufficient permissions"})
			c.Abort()
			return
		}

		c.Next()
	}
}

func RequireAdmin() gin.HandlerFunc {
	return RequireRole("admin")
}

func RequireUser() gin.HandlerFunc {
	return RequireRole("user")
}

func RequireViewer() gin.HandlerFunc {
	return RequireRole("viewer")
}

// RequirePermission middleware for checking specific permissions
func RequirePermission(permission string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Get user role from context
		userRole, exists := c.Get("user_role")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "User role not found"})
			c.Abort()
			return
		}

		role := userRole.(string)
		hasPermission := auth.HasPermission(role, permission)

		if !hasPermission {
			c.JSON(http.StatusForbidden, gin.H{"error": "Insufficient permissions"})
			c.Abort()
			return
		}

		c.Next()
	}
}

// RequireModulePermission middleware for checking module-based permissions
// permissionType should be either "view" or "edit"
func RequireModulePermission(db *sql.DB, moduleName string, permissionType string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Get user ID and role from context
		userID, exists := c.Get("user_id")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
			c.Abort()
			return
		}

		userRole, exists := c.Get("user_role")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "User role not found"})
			c.Abort()
			return
		}

		role := userRole.(string)
		uid := userID.(int)

		// Admin has access to all modules
		if role == "admin" {
			c.Next()
			return
		}

		// Check module permission in database
		var hasPermission bool
		var query string

		if permissionType == "edit" {
			query = `SELECT can_edit FROM module_permissions WHERE user_id = ? AND module_name = ?`
		} else {
			query = `SELECT can_view FROM module_permissions WHERE user_id = ? AND module_name = ?`
		}

		err := db.QueryRow(query, uid, moduleName).Scan(&hasPermission)
		if err != nil {
			if err == sql.ErrNoRows {
				// No permission record found, deny access
				c.JSON(http.StatusForbidden, gin.H{"error": "Bu sayfaya erişim yetkiniz bulunmuyor"})
				c.Abort()
				return
			}
			// Database error
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to check permissions"})
			c.Abort()
			return
		}

		if !hasPermission {
			c.JSON(http.StatusForbidden, gin.H{"error": "Bu sayfaya erişim yetkiniz bulunmuyor"})
			c.Abort()
			return
		}

		c.Next()
	}
}
