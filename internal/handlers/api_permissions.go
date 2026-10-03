package handlers

import (
	"database/sql"
	"net/http"

	"systrack/internal/auth"

	"github.com/gin-gonic/gin"
)

// GetPermissions returns all available permissions
func GetPermissions(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		permissions := auth.GetAllPermissions()
		c.JSON(http.StatusOK, gin.H{"permissions": permissions})
	}
}

// GetRolePermissions returns permissions for a specific role
func GetRolePermissions(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		role := c.Param("role")
		permissions := auth.GetRolePermissions(role)
		c.JSON(http.StatusOK, gin.H{"role": role, "permissions": permissions})
	}
}

// GetUserPermissions returns permissions for the current user
func GetUserPermissions(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, _ := c.Get("user_id")
		id := userID.(int)

		permissions, err := auth.GetUserPermissions(db, id)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get user permissions"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"permissions": permissions})
	}
}

// CheckPermission checks if the current user has a specific permission
func CheckPermission(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		permission := c.Param("permission")
		userID, _ := c.Get("user_id")
		id := userID.(int)

		hasPermission, err := auth.CheckUserPermission(db, id, permission)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to check permission"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"permission":     permission,
			"has_permission": hasPermission,
		})
	}
}

// GetPermissionsByResource returns permissions for a specific resource
func GetPermissionsByResource(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		resource := c.Param("resource")
		permissions := auth.GetPermissionsByResource(resource)
		c.JSON(http.StatusOK, gin.H{"resource": resource, "permissions": permissions})
	}
}

// GetPermissionsByAction returns permissions for a specific action
func GetPermissionsByAction(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		action := c.Param("action")
		permissions := auth.GetPermissionsByAction(action)
		c.JSON(http.StatusOK, gin.H{"action": action, "permissions": permissions})
	}
}

// GetRoleDescription returns description for a role
func GetRoleDescription(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		role := c.Param("role")
		description := auth.GetRoleDescription(role)
		c.JSON(http.StatusOK, gin.H{"role": role, "description": description})
	}
}

// GetPermissionCategories returns permissions grouped by category
func GetPermissionCategories(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		categories := auth.GetPermissionCategories()
		c.JSON(http.StatusOK, gin.H{"categories": categories})
	}
}

// GetRoleSummary returns a summary of what each role can do
func GetRoleSummary(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		summary := auth.GetRoleSummary()
		c.JSON(http.StatusOK, gin.H{"role_summary": summary})
	}
}
