package mw

import (
	"database/sql"
	"net"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

func AccessAllowlist(db *sql.DB, isActivationMode func() bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		if isActivationMode != nil && isActivationMode() {
			c.Next()
			return
		}

		enabled := getSettingBool(db, "access_allowlist_enabled")
		if !enabled {
			c.Next()
			return
		}

		clientIP := strings.TrimSpace(c.ClientIP())
		if clientIP == "" {
			denyAccess(c)
			return
		}

		parsed := net.ParseIP(clientIP)
		if parsed != nil && parsed.IsLoopback() {
			c.Next()
			return
		}

		var exists bool
		err := db.QueryRow(`SELECT EXISTS(SELECT 1 FROM access_allowlist WHERE ip_address = ? AND is_enabled = 1)`, clientIP).Scan(&exists)
		if err != nil || !exists {
			denyAccess(c)
			return
		}

		c.Next()
	}
}

func denyAccess(c *gin.Context) {
	if strings.HasPrefix(c.Request.URL.Path, "/api/") {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
	} else {
		c.String(http.StatusForbidden, "Access denied")
	}
	c.Abort()
}

func getSettingBool(db *sql.DB, key string) bool {
	var value string
	if err := db.QueryRow(`SELECT v FROM settings WHERE k = ?`, key).Scan(&value); err != nil {
		return false
	}
	value = strings.TrimSpace(strings.ToLower(value))
	return value == "1" || value == "true" || value == "yes" || value == "on"
}
