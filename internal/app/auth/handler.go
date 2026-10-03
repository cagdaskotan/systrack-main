package auth

import (
	"database/sql"
	"net/http"

	domainAuth "systrack/internal/auth"
	"systrack/internal/config"

	"github.com/gin-gonic/gin"
)

// LoginRequest represents incoming credentials for mobile clients.
type LoginRequest struct {
	Email    string `json:"email" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// LoginResponse wraps the JWT token payload returned to the mobile client.
type LoginResponse struct {
	Token     string                `json:"token"`
	ExpiresAt int64                 `json:"expires_at"`
	User      domainAuth.PublicUser `json:"user"`
}

// Login handles POST /api/mobile/auth/login requests.
// It reuses the shared authentication helpers but returns a mobile-friendly JSON payload.
func Login(db *sql.DB, cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req LoginRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "invalid_request",
				"message": err.Error(),
			})
			return
		}

		user, err := domainAuth.AuthenticateUser(db, req.Email, req.Password)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error":   "invalid_credentials",
				"message": "E-posta veya kullanici adi ya da sifre hatali.",
			})
			return
		}

		tokenResp, err := domainAuth.GenerateToken(user.ID, user.Email, user.Role, cfg.Auth.JWTSecret)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "token_generation_failed",
				"message": "Oturum oluşturulamadı.",
			})
			return
		}

		response := LoginResponse{
			Token:     tokenResp.Token,
			ExpiresAt: tokenResp.ExpiresAt,
			User: domainAuth.PublicUser{
				ID:            user.ID,
				Email:         user.Email,
				Role:          user.Role,
				MaxTargets:    user.MaxTargets,
				IPQueryLimit:  user.IPQueryLimit,
				IsActive:      user.IsActive,
				LicenseExpiry: user.LicenseExpiry,
				LastLoginAt:   user.LastLoginAt,
				CreatedAt:     user.CreatedAt,
				UpdatedAt:     user.UpdatedAt,
			},
		}

		c.JSON(http.StatusOK, response)
	}
}

