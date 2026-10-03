package handlers

import (
	"database/sql"
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// ModulePermission represents a module permission
type ModulePermission struct {
	ID         int    `json:"id"`
	UserID     int    `json:"user_id"`
	ModuleName string `json:"module_name"`
	CanView    bool   `json:"can_view"`
	CanEdit    bool   `json:"can_edit"`
}

// GetUserModulePermissions returns module permissions for a specific user
func GetUserModulePermissions(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		userIDStr := c.Param("id")
		userID, err := strconv.Atoi(userIDStr)
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

		// Users can only get their own permissions, unless they're admin
		currentUserIDInt, ok := currentUserID.(int)
		if !ok {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Invalid user ID format"})
			return
		}

		// Check if user is admin or getting their own permissions
		var currentUserRole string
		err = db.QueryRow("SELECT role FROM users WHERE id = ?", currentUserIDInt).Scan(&currentUserRole)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get user role"})
			return
		}

		// DEBUG: Log permission check details
		fmt.Printf("[DEBUG] Permission check - CurrentUserID: %d, RequestedUserID: %d, Role: %s, Match: %v\n",
			currentUserIDInt, userID, currentUserRole, currentUserIDInt == userID)

		if currentUserRole != "admin" && currentUserIDInt != userID {
			fmt.Printf("[DEBUG] 403 Forbidden - CurrentUserID (%d) != RequestedUserID (%d) and Role (%s) != admin\n",
				currentUserIDInt, userID, currentUserRole)
			c.JSON(http.StatusForbidden, gin.H{"error": "You can only get your own permissions"})
			return
		}

		query := `SELECT id, user_id, module_name, can_view, can_edit 
				  FROM module_permissions 
				  WHERE user_id = ? 
				  ORDER BY module_name`

		rows, err := db.Query(query, userID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch module permissions"})
			return
		}
		defer rows.Close()

		var permissions []ModulePermission
		for rows.Next() {
			var perm ModulePermission
			err := rows.Scan(&perm.ID, &perm.UserID, &perm.ModuleName, &perm.CanView, &perm.CanEdit)
			if err != nil {
				continue
			}
			permissions = append(permissions, perm)
		}

		c.JSON(http.StatusOK, gin.H{"permissions": permissions})
	}
}

// UpdateUserModulePermissions updates module permissions for a user
func UpdateUserModulePermissions(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		userIDStr := c.Param("id")
		userID, err := strconv.Atoi(userIDStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID"})
			return
		}

		var request struct {
			Permissions []struct {
				ModuleName string `json:"module_name"`
				CanView    bool   `json:"can_view"`
				CanEdit    bool   `json:"can_edit"`
			} `json:"permissions"`
		}

		if err := c.ShouldBindJSON(&request); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request format"})
			return
		}

		// Start transaction
		tx, err := db.Begin()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to start transaction"})
			return
		}
		defer tx.Rollback()

		// Update each permission
		for _, perm := range request.Permissions {
			query := `INSERT INTO module_permissions (user_id, module_name, can_view, can_edit) 
					  VALUES (?, ?, ?, ?) 
					  ON DUPLICATE KEY UPDATE 
					  can_view = VALUES(can_view), 
					  can_edit = VALUES(can_edit)`

			_, err := tx.Exec(query, userID, perm.ModuleName, perm.CanView, perm.CanEdit)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update permission"})
				return
			}
		}

		// Commit transaction
		if err := tx.Commit(); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to commit changes"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"message": "Module permissions updated successfully"})
	}
}

// GetAvailableModules returns list of available modules
func GetAvailableModules() gin.HandlerFunc {
	return func(c *gin.Context) {
		modules := []struct {
			Name        string `json:"name"`
			DisplayName string `json:"display_name"`
			Description string `json:"description"`
		}{
			{"dashboard", "Dashboard", "Ana kontrol paneli ve genel bakış"},
			{"targets", "Hedefler", "Ağ hedeflerini izleme ve yönetme"},
			{"ip_scanner", "IP Tarayıcı", "IP adreslerini tarama ve analiz etme"},
			{"notifications", "Bildirimler", "Sistem bildirimlerini görüntüleme"},
			{"notification_settings", "Bildirim Ayarları", "Bildirim kanallarını yapılandırma"},
			{"reporting", "Raporlama", "Detaylı sistem raporları oluşturma"},
            {"network_settings", "Ağ Ayarları", "Cihazın ağ yapılandırmasını yönetme"},
			{"backup", "Yedekleme", "Sistem yedekleme ve geri yükleme"},
		}

		c.JSON(http.StatusOK, gin.H{"modules": modules})
	}
}

// CheckModulePermission checks if user has permission for a specific module
func CheckModulePermission(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		userIDStr := c.Param("user_id")
		moduleName := c.Param("module")
		permissionType := c.Query("type") // "view" or "edit"

		userID, err := strconv.Atoi(userIDStr)
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

		// Users can only check their own permissions, unless they're admin
		currentUserIDInt, ok := currentUserID.(int)
		if !ok {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Invalid user ID format"})
			return
		}

		// Check if user is admin or checking their own permissions
		var currentUserRole string
		err = db.QueryRow("SELECT role FROM users WHERE id = ?", currentUserIDInt).Scan(&currentUserRole)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get user role"})
			return
		}

		if currentUserRole != "admin" && currentUserIDInt != userID {
			c.JSON(http.StatusForbidden, gin.H{"error": "You can only check your own permissions"})
			return
		}

		var query string
		var args []interface{}

		if permissionType == "edit" {
			query = `SELECT can_edit FROM module_permissions WHERE user_id = ? AND module_name = ?`
		} else {
			query = `SELECT can_view FROM module_permissions WHERE user_id = ? AND module_name = ?`
		}

		args = []interface{}{userID, moduleName}

		var hasPermission bool
		err = db.QueryRow(query, args...).Scan(&hasPermission)
		if err != nil {
			if err == sql.ErrNoRows {
				// No permission record found, default to false
				hasPermission = false
			} else {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to check permission"})
				return
			}
		}

		c.JSON(http.StatusOK, gin.H{"has_permission": hasPermission})
	}
}
