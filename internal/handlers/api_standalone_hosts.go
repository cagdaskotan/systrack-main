package handlers

import (
	"database/sql"
	"net/http"
	"strconv"
	"time"

	"systrack/internal/auth"

	"github.com/gin-gonic/gin"
)

// StandaloneHost, domain dışı bir Windows cihazı temsil eder.
type StandaloneHost struct {
	ID          int        `json:"id"`
	IPAddress   string     `json:"ip_address"`
	Label       string     `json:"label,omitempty"`
	Username    string     `json:"username"`
	Password    string     `json:"password,omitempty"` // yalnızca yazma; okumada boş döner
	Port        int        `json:"port"`
	UseSSL      bool       `json:"use_ssl"`
	IsEnabled   bool       `json:"is_enabled"`
	LastSeenAt  *time.Time `json:"last_seen_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}

// GetStandaloneHosts, kayıtlı tüm standalone WinRM host'larını döner.
// Şifreler hiçbir zaman istemciye gönderilmez.
func GetStandaloneHosts(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		rows, err := db.Query(`
			SELECT id, ip_address, COALESCE(label,''), username, port, use_ssl, is_enabled,
			       last_seen_at, created_at
			FROM winrm_standalone_hosts
			ORDER BY created_at DESC
		`)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		defer rows.Close()

		var hosts []StandaloneHost
		for rows.Next() {
			var h StandaloneHost
			var lastSeen sql.NullTime
			if err := rows.Scan(&h.ID, &h.IPAddress, &h.Label, &h.Username,
				&h.Port, &h.UseSSL, &h.IsEnabled, &lastSeen, &h.CreatedAt); err != nil {
				continue
			}
			if lastSeen.Valid {
				h.LastSeenAt = &lastSeen.Time
			}
			hosts = append(hosts, h)
		}
		if hosts == nil {
			hosts = []StandaloneHost{}
		}
		c.JSON(http.StatusOK, hosts)
	}
}

// CreateStandaloneHost, yeni bir standalone WinRM host kaydeder.
func CreateStandaloneHost(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req StandaloneHost
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if req.IPAddress == "" || req.Username == "" || req.Password == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "ip_address, username ve password zorunludur"})
			return
		}
		if req.Port == 0 {
			req.Port = 5985
		}

		encPass, err := auth.EncryptPassword(req.Password)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Şifre şifrelenemedi"})
			return
		}

		res, err := db.Exec(`
			INSERT INTO winrm_standalone_hosts (ip_address, label, username, password_encrypted, port, use_ssl, is_enabled)
			VALUES (?, ?, ?, ?, ?, ?, ?)
		`, req.IPAddress, nullableString(req.Label), req.Username, encPass, req.Port, req.UseSSL, req.IsEnabled)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		id, _ := res.LastInsertId()
		c.JSON(http.StatusOK, gin.H{"id": id, "ok": true})
	}
}

// UpdateStandaloneHost, mevcut bir standalone host'u günceller.
func UpdateStandaloneHost(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := strconv.Atoi(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "geçersiz id"})
			return
		}

		var req StandaloneHost
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if req.Port == 0 {
			req.Port = 5985
		}

		// Şifre gönderildiyse şifrele; gönderilmediyse mevcut şifreyi koru
		if req.Password != "" && req.Password != "********" {
			encPass, err := auth.EncryptPassword(req.Password)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Şifre şifrelenemedi"})
				return
			}
			_, err = db.Exec(`
				UPDATE winrm_standalone_hosts
				SET ip_address=?, label=?, username=?, password_encrypted=?, port=?, use_ssl=?, is_enabled=?
				WHERE id=?
			`, req.IPAddress, nullableString(req.Label), req.Username, encPass, req.Port, req.UseSSL, req.IsEnabled, id)
		} else {
			_, err = db.Exec(`
				UPDATE winrm_standalone_hosts
				SET ip_address=?, label=?, username=?, port=?, use_ssl=?, is_enabled=?
				WHERE id=?
			`, req.IPAddress, nullableString(req.Label), req.Username, req.Port, req.UseSSL, req.IsEnabled, id)
		}
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true})
	}
}

// DeleteStandaloneHost, bir standalone host kaydını siler.
func DeleteStandaloneHost(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := strconv.Atoi(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "geçersiz id"})
			return
		}
		if _, err := db.Exec(`DELETE FROM winrm_standalone_hosts WHERE id = ?`, id); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true})
	}
}

