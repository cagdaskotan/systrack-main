package handlers

import (
	"database/sql"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

func HealthCheck(db *sql.DB, scheduler interface{}) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Database health check
		start := time.Now()
		err := db.Ping()
		dbDuration := time.Since(start)

		if err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"status": "unhealthy",
				"error":  "Database connection failed",
			})
			return
		}

		// Get scheduler status if available
		var schedulerStatus map[string]interface{}
		if scheduler != nil {
			if statusMethod, ok := scheduler.(interface{ GetStatus() map[string]interface{} }); ok {
				schedulerStatus = statusMethod.GetStatus()
			}
		}

		response := gin.H{
			"status": "healthy",
			"checks": map[string]interface{}{
				"database": "ok",
				"db_ms":    dbDuration.Milliseconds(),
			},
		}

		// Add scheduler info if available
		if schedulerStatus != nil {
			response["engine"] = schedulerStatus["engine"]
			response["scheduler"] = schedulerStatus
		}

		c.JSON(http.StatusOK, response)
	}
}
