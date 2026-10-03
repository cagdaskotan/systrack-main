package handlers

import (
	"crypto/tls"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"systrack/internal/auth"
	"systrack/internal/services"

	"github.com/gin-gonic/gin"
	"github.com/go-ldap/ldap/v3"
)

// ADSettings represents Active Directory connection settings
type ADSettings struct {
	ID                    int        `json:"id"`
	DomainController      string     `json:"domain_controller"`
	Port                  int        `json:"port"`
	UseSSL                bool       `json:"use_ssl"`
	BaseDN                string     `json:"base_dn"`
	BindUsername          string     `json:"bind_username"`
	BindPassword          string     `json:"bind_password,omitempty"` // Never returned in GET
	HasPassword           bool       `json:"has_password"`             // Indicates if password is set in DB
	SearchFilter          string     `json:"search_filter"`
	SearchScope           string     `json:"search_scope"`
	OUFilter              *string    `json:"ou_filter"`
	LocationFromOU        bool       `json:"location_from_ou"`
	OULocationMap         *string    `json:"ou_location_map"`
	IsEnabled             bool       `json:"is_enabled"`
	LastTestAt            *time.Time `json:"last_test_at"`
	LastTestResult        *string    `json:"last_test_result"`
	LastTestError         *string    `json:"last_test_error"`
	LastTestComputerCount *int       `json:"last_test_computer_count,omitempty"`
}

// ADSettingsUpdateRequest represents the request body for updating AD settings
type ADSettingsUpdateRequest struct {
	DomainController string  `json:"domain_controller"`
	Port             int     `json:"port"`
	UseSSL           bool    `json:"use_ssl"`
	BaseDN           string  `json:"base_dn"`
	BindUsername     string  `json:"bind_username"`
	BindPassword     string  `json:"bind_password"`
	SearchFilter     string  `json:"search_filter"`
	SearchScope      string  `json:"search_scope"`
	OUFilter         *string `json:"ou_filter"`
	LocationFromOU   bool    `json:"location_from_ou"`
	OULocationMap    *string `json:"ou_location_map"`
	IsEnabled        bool    `json:"is_enabled"`
}

// ADTestResult represents the result of an AD connection test
type ADTestResult struct {
	Success        bool   `json:"success"`
	Message        string `json:"message"`
	ComputerCount  int    `json:"computer_count,omitempty"`
	ResponseTimeMs int64  `json:"response_time_ms,omitempty"`
	ErrorDetail    string `json:"error_detail,omitempty"`
}

// GetADSettings returns the current AD settings (id=1)
func GetADSettings(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var settings ADSettings
		var lastTestAt sql.NullTime
		var lastTestResult, lastTestError, ouFilter, ouLocationMap sql.NullString
		var encryptedPassword sql.NullString

		err := db.QueryRow(`
			SELECT
				id, domain_controller, port, use_ssl, base_dn,
				bind_username, bind_password_encrypted, search_filter, search_scope, ou_filter,
				location_from_ou, ou_location_map, is_enabled,
				last_test_at, last_test_result, last_test_error
			FROM ad_settings WHERE id = 1
		`).Scan(
			&settings.ID, &settings.DomainController, &settings.Port, &settings.UseSSL, &settings.BaseDN,
			&settings.BindUsername, &encryptedPassword, &settings.SearchFilter, &settings.SearchScope, &ouFilter,
			&settings.LocationFromOU, &ouLocationMap, &settings.IsEnabled,
			&lastTestAt, &lastTestResult, &lastTestError,
		)

		if err == sql.ErrNoRows {
			// Return empty settings if not found
			c.JSON(http.StatusOK, ADSettings{
				ID:           1,
				Port:         389,
				SearchFilter: "(objectClass=computer)",
				SearchScope:  "sub",
			})
			return
		} else if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Ayarlar okunamadı: " + err.Error()})
			return
		}

		// Handle nullable fields
		if lastTestAt.Valid {
			settings.LastTestAt = &lastTestAt.Time
		}
		if lastTestResult.Valid {
			settings.LastTestResult = &lastTestResult.String
		}
		if lastTestError.Valid {
			settings.LastTestError = &lastTestError.String
		}
		if ouFilter.Valid {
			settings.OUFilter = &ouFilter.String
		}
		if ouLocationMap.Valid {
			settings.OULocationMap = &ouLocationMap.String
		}

		// Check if password exists (don't return actual password)
		settings.HasPassword = encryptedPassword.Valid && encryptedPassword.String != ""
		settings.BindPassword = ""

		c.JSON(http.StatusOK, settings)
	}
}

// UpdateADSettings updates the AD settings (id=1)
func UpdateADSettings(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req ADSettingsUpdateRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Geçersiz istek: " + err.Error()})
			return
		}

		// Set defaults
		if req.Port == 0 {
			req.Port = 389
		}
		if req.SearchFilter == "" {
			req.SearchFilter = "(objectClass=computer)"
		}
		if req.SearchScope == "" {
			req.SearchScope = "sub"
		}

		// Check if password should be updated
		var updatePassword bool
		var currentPassword string

		if req.BindPassword != "" && req.BindPassword != "********" {
			updatePassword = true
		} else {
			// Keep existing password
			db.QueryRow(`SELECT bind_password_encrypted FROM ad_settings WHERE id = 1`).Scan(&currentPassword)
		}

		var err error
		if updatePassword {
			// Encrypt password before storing (AES-256-GCM)
			encryptedPassword, encErr := auth.EncryptPassword(req.BindPassword)
			if encErr != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Şifre şifrelenemedi: " + encErr.Error()})
				return
			}

			_, err = db.Exec(`
				UPDATE ad_settings SET
					domain_controller = ?,
					port = ?,
					use_ssl = ?,
					base_dn = ?,
					bind_username = ?,
					bind_password_encrypted = ?,
					search_filter = ?,
					search_scope = ?,
					ou_filter = ?,
					location_from_ou = ?,
					ou_location_map = ?,
					is_enabled = ?,
					updated_at = NOW()
				WHERE id = 1
			`,
				req.DomainController, req.Port, req.UseSSL, req.BaseDN,
				req.BindUsername, encryptedPassword, req.SearchFilter, req.SearchScope,
				req.OUFilter, req.LocationFromOU, req.OULocationMap, req.IsEnabled,
			)
		} else {
			_, err = db.Exec(`
				UPDATE ad_settings SET
					domain_controller = ?,
					port = ?,
					use_ssl = ?,
					base_dn = ?,
					bind_username = ?,
					search_filter = ?,
					search_scope = ?,
					ou_filter = ?,
					location_from_ou = ?,
					ou_location_map = ?,
					is_enabled = ?,
					updated_at = NOW()
				WHERE id = 1
			`,
				req.DomainController, req.Port, req.UseSSL, req.BaseDN,
				req.BindUsername, req.SearchFilter, req.SearchScope,
				req.OUFilter, req.LocationFromOU, req.OULocationMap, req.IsEnabled,
			)
		}

		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Ayarlar kaydedilemedi: " + err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{"message": "AD ayarları kaydedildi"})
	}
}

// TestADConnection tests the AD/LDAP connection
func TestADConnection(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Start timing
		startTime := time.Now()

		// Get current settings
		var dc, baseDN, username, password, searchFilter string
		var port int
		var useSSL bool

		var encryptedPassword string
		err := db.QueryRow(`
			SELECT domain_controller, port, use_ssl, base_dn, bind_username, bind_password_encrypted, search_filter
			FROM ad_settings WHERE id = 1
		`).Scan(&dc, &port, &useSSL, &baseDN, &username, &encryptedPassword, &searchFilter)

		if err != nil {
			c.JSON(http.StatusOK, ADTestResult{
				Success:        false,
				Message:        "AD ayarları bulunamadı",
				ErrorDetail:    err.Error(),
				ResponseTimeMs: time.Since(startTime).Milliseconds(),
			})
			return
		}

		// Decrypt password
		password, err = auth.DecryptPassword(encryptedPassword)
		if err != nil {
			c.JSON(http.StatusOK, ADTestResult{
				Success:        false,
				Message:        "Şifre çözümlenemedi",
				ErrorDetail:    err.Error(),
				ResponseTimeMs: time.Since(startTime).Milliseconds(),
			})
			return
		}

		// Validate required fields
		if dc == "" {
			c.JSON(http.StatusOK, ADTestResult{
				Success:        false,
				Message:        "Domain Controller adresi boş",
				ResponseTimeMs: time.Since(startTime).Milliseconds(),
			})
			return
		}

		if username == "" || password == "" {
			c.JSON(http.StatusOK, ADTestResult{
				Success:        false,
				Message:        "Kullanıcı adı veya şifre boş",
				ResponseTimeMs: time.Since(startTime).Milliseconds(),
			})
			return
		}

		// Build connection with timeout and TLS support
		var conn *ldap.Conn
		connTimeout := 10 * time.Second
		address := fmt.Sprintf("%s:%d", dc, port)

		if useSSL {
			// LDAPS connection with TLS
			tlsConfig := &tls.Config{
				ServerName:         dc,
				InsecureSkipVerify: false, // Production should validate certs
			}

			// Dial with timeout
			netConn, dialErr := net.DialTimeout("tcp", address, connTimeout)
			if dialErr != nil {
				elapsedMs := time.Since(startTime).Milliseconds()
				updateTestResult(db, false, "Bağlantı hatası: "+dialErr.Error(), 0)
				c.JSON(http.StatusOK, ADTestResult{
					Success:        false,
					Message:        "LDAP sunucusuna bağlanılamadı",
					ErrorDetail:    dialErr.Error(),
					ResponseTimeMs: elapsedMs,
				})
				return
			}

			// Wrap with TLS
			tlsConn := tls.Client(netConn, tlsConfig)
			if err = tlsConn.Handshake(); err != nil {
				netConn.Close()
				elapsedMs := time.Since(startTime).Milliseconds()
				updateTestResult(db, false, "TLS handshake hatası: "+err.Error(), 0)
				c.JSON(http.StatusOK, ADTestResult{
					Success:        false,
					Message:        "TLS bağlantısı kurulamadı",
					ErrorDetail:    err.Error(),
					ResponseTimeMs: elapsedMs,
				})
				return
			}

			conn = ldap.NewConn(tlsConn, true)
			conn.Start()
		} else {
			// Plain LDAP connection with timeout
			netConn, dialErr := net.DialTimeout("tcp", address, connTimeout)
			if dialErr != nil {
				elapsedMs := time.Since(startTime).Milliseconds()
				updateTestResult(db, false, "Bağlantı hatası: "+dialErr.Error(), 0)
				c.JSON(http.StatusOK, ADTestResult{
					Success:        false,
					Message:        "LDAP sunucusuna bağlanılamadı",
					ErrorDetail:    dialErr.Error(),
					ResponseTimeMs: elapsedMs,
				})
				return
			}

			conn = ldap.NewConn(netConn, false)
			conn.Start()
		}
		defer conn.Close()

		// Try to bind (authenticate)
		err = conn.Bind(username, password)
		if err != nil {
			elapsedMs := time.Since(startTime).Milliseconds()
			updateTestResult(db, false, "Kimlik doğrulama hatası: "+err.Error(), 0)
			c.JSON(http.StatusOK, ADTestResult{
				Success:        false,
				Message:        "Kimlik doğrulama başarısız",
				ErrorDetail:    err.Error(),
				ResponseTimeMs: elapsedMs,
			})
			return
		}

		// Auto-detect Base DN if empty
		if baseDN == "" {
			detectedBaseDN, detectErr := autoDetectBaseDN(conn)
			if detectErr != nil {
				elapsedMs := time.Since(startTime).Milliseconds()
				c.JSON(http.StatusOK, ADTestResult{
					Success:        false,
					Message:        "Base DN otomatik tespit edilemedi",
					ErrorDetail:    detectErr.Error(),
					ResponseTimeMs: elapsedMs,
				})
				return
			}
			baseDN = detectedBaseDN
			log.Printf("Base DN otomatik tespit edildi: %s", baseDN)
		}

		// Search for computers
		if searchFilter == "" {
			searchFilter = "(objectClass=computer)"
		}

		searchRequest := ldap.NewSearchRequest(
			baseDN,
			ldap.ScopeWholeSubtree,
			ldap.NeverDerefAliases,
			0,    // No size limit for counting
			30,   // 30 second timeout
			false,
			searchFilter,
			[]string{"dn"}, // Only request DN to count
			nil,
		)

		result, err := conn.Search(searchRequest)
		if err != nil {
			elapsedMs := time.Since(startTime).Milliseconds()
			updateTestResult(db, false, "Arama hatası: "+err.Error(), 0)
			c.JSON(http.StatusOK, ADTestResult{
				Success:        false,
				Message:        "LDAP sorgusu başarısız",
				ErrorDetail:    err.Error(),
				ResponseTimeMs: elapsedMs,
			})
			return
		}

		computerCount := len(result.Entries)
		elapsedMs := time.Since(startTime).Milliseconds()

		// Update test result in database
		updateTestResult(db, true, "", computerCount)

		c.JSON(http.StatusOK, ADTestResult{
			Success:        true,
			Message:        fmt.Sprintf("Bağlantı başarılı! %d bilgisayar bulundu.", computerCount),
			ComputerCount:  computerCount,
			ResponseTimeMs: elapsedMs,
		})
	}
}

// updateTestResult saves test result to database
func updateTestResult(db *sql.DB, success bool, errorMsg string, computerCount int) {
	result := "success"
	if !success {
		result = "failed"
	}

	var errPtr *string
	if errorMsg != "" {
		errPtr = &errorMsg
	}

	var countPtr *int
	if computerCount > 0 {
		countPtr = &computerCount
	}

	_, err := db.Exec(`
		UPDATE ad_settings SET
			last_test_at = NOW(),
			last_test_result = ?,
			last_test_error = ?,
			last_test_computer_count = ?
		WHERE id = 1
	`, result, errPtr, countPtr)
	if err != nil {
		log.Printf("Failed to update AD test result: %v", err)
	}
}

// WinRMSettings represents WinRM connection settings
type WinRMSettings struct {
	ID              int        `json:"id"`
	Username        string     `json:"username"`
	Password        string     `json:"password,omitempty"`
	Port            int        `json:"port"`
	UseSSL          bool       `json:"use_ssl"`
	TimeoutSeconds  int        `json:"timeout_seconds"`
	ConcurrentLimit int        `json:"concurrent_limit"`
	RetryCount      int        `json:"retry_count"`
	IsEnabled       bool       `json:"is_enabled"`
	LastTestAt      *time.Time `json:"last_test_at"`
	LastTestResult  *string    `json:"last_test_result"`
	LastTestError   *string    `json:"last_test_error"`
}

// GetWinRMSettings returns the current WinRM settings (id=1)
func GetWinRMSettings(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var settings WinRMSettings
		var lastTestAt sql.NullTime
		var lastTestResult, lastTestError sql.NullString

		err := db.QueryRow(`
			SELECT
				id, username, port, use_ssl, timeout_seconds,
				concurrent_limit, retry_count, is_enabled,
				last_test_at, last_test_result, last_test_error
			FROM winrm_settings WHERE id = 1
		`).Scan(
			&settings.ID, &settings.Username, &settings.Port, &settings.UseSSL, &settings.TimeoutSeconds,
			&settings.ConcurrentLimit, &settings.RetryCount, &settings.IsEnabled,
			&lastTestAt, &lastTestResult, &lastTestError,
		)

		if err == sql.ErrNoRows {
			c.JSON(http.StatusOK, WinRMSettings{
				ID:              1,
				Port:            5985,
				TimeoutSeconds:  30,
				ConcurrentLimit: 5,
				RetryCount:      2,
			})
			return
		} else if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Ayarlar okunamadı"})
			return
		}

		if lastTestAt.Valid {
			settings.LastTestAt = &lastTestAt.Time
		}
		if lastTestResult.Valid {
			settings.LastTestResult = &lastTestResult.String
		}
		if lastTestError.Valid {
			settings.LastTestError = &lastTestError.String
		}

		settings.Password = ""

		c.JSON(http.StatusOK, settings)
	}
}

// WinRMSettingsUpdateRequest represents the request body for updating WinRM settings
type WinRMSettingsUpdateRequest struct {
	Username        string `json:"username"`
	Password        string `json:"password"`
	Port            int    `json:"port"`
	UseSSL          bool   `json:"use_ssl"`
	TimeoutSeconds  int    `json:"timeout_seconds"`
	ConcurrentLimit int    `json:"concurrent_limit"`
	RetryCount      int    `json:"retry_count"`
	IsEnabled       bool   `json:"is_enabled"`
}

// WinRMTestResult represents the result of a WinRM connection test
type WinRMTestResult struct {
	Success        bool   `json:"success"`
	Message        string `json:"message"`
	ResponseTimeMs int64  `json:"response_time_ms,omitempty"`
	ErrorDetail    string `json:"error_detail,omitempty"`
	ErrorCategory  string `json:"error_category,omitempty"`
	ErrorHint      string `json:"error_hint,omitempty"`
	OSInfo         string `json:"os_info,omitempty"`
}

// UpdateWinRMSettings updates the WinRM settings (id=1)
func UpdateWinRMSettings(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req WinRMSettingsUpdateRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Geçersiz istek: " + err.Error()})
			return
		}

		// Set defaults
		if req.Port == 0 {
			req.Port = 5985
		}
		if req.TimeoutSeconds == 0 {
			req.TimeoutSeconds = 30
		}
		if req.ConcurrentLimit == 0 {
			req.ConcurrentLimit = 5
		}
		if req.RetryCount == 0 {
			req.RetryCount = 2
		}

		// Check if password should be updated
		var updatePassword bool
		if req.Password != "" && req.Password != "********" {
			updatePassword = true
		}

		var err error
		if updatePassword {
			// Encrypt password before storing (AES-256-GCM)
			encryptedPassword, encErr := auth.EncryptPassword(req.Password)
			if encErr != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Şifre şifrelenemedi: " + encErr.Error()})
				return
			}

			_, err = db.Exec(`
				UPDATE winrm_settings SET
					username = ?,
					password_encrypted = ?,
					port = ?,
					use_ssl = ?,
					timeout_seconds = ?,
					concurrent_limit = ?,
					retry_count = ?,
					is_enabled = ?,
					updated_at = NOW()
				WHERE id = 1
			`,
				req.Username, encryptedPassword, req.Port, req.UseSSL,
				req.TimeoutSeconds, req.ConcurrentLimit, req.RetryCount, req.IsEnabled,
			)
		} else {
			_, err = db.Exec(`
				UPDATE winrm_settings SET
					username = ?,
					port = ?,
					use_ssl = ?,
					timeout_seconds = ?,
					concurrent_limit = ?,
					retry_count = ?,
					is_enabled = ?,
					updated_at = NOW()
				WHERE id = 1
			`,
				req.Username, req.Port, req.UseSSL,
				req.TimeoutSeconds, req.ConcurrentLimit, req.RetryCount, req.IsEnabled,
			)
		}

		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Ayarlar kaydedilemedi: " + err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{"message": "WinRM ayarları kaydedildi"})
	}
}

// TestWinRMConnection tests the WinRM connection to a target IP
func TestWinRMConnection(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		startTime := time.Now()

		// Get target IP from query parameter
		targetIP := c.Query("target_ip")
		if targetIP == "" {
			c.JSON(http.StatusOK, WinRMTestResult{
				Success:        false,
				Message:        "Test için hedef IP gerekli",
				ResponseTimeMs: time.Since(startTime).Milliseconds(),
			})
			return
		}

		// Get current settings
		var username, encryptedPassword string
		var port, timeoutSeconds int
		var useSSL bool

		err := db.QueryRow(`
			SELECT username, password_encrypted, port, use_ssl, timeout_seconds
			FROM winrm_settings WHERE id = 1
		`).Scan(&username, &encryptedPassword, &port, &useSSL, &timeoutSeconds)

		if err != nil {
			c.JSON(http.StatusOK, WinRMTestResult{
				Success:        false,
				Message:        "WinRM ayarları bulunamadı",
				ErrorDetail:    err.Error(),
				ResponseTimeMs: time.Since(startTime).Milliseconds(),
			})
			return
		}

		// Decrypt password
		password, err := auth.DecryptPassword(encryptedPassword)
		if err != nil {
			c.JSON(http.StatusOK, WinRMTestResult{
				Success:        false,
				Message:        "Şifre çözümlenemedi",
				ErrorDetail:    err.Error(),
				ResponseTimeMs: time.Since(startTime).Milliseconds(),
			})
			return
		}

		// Validate required fields
		if username == "" || password == "" {
			c.JSON(http.StatusOK, WinRMTestResult{
				Success:        false,
				Message:        "Kullanıcı adı veya şifre boş",
				ResponseTimeMs: time.Since(startTime).Milliseconds(),
			})
			return
		}

		// Create WinRM client and test connection
		client := services.NewWinRMClient(username, password, port, useSSL, timeoutSeconds)

		// First do a quick TCP reachability check
		if err := client.CheckWinRMReachable(targetIP); err != nil {
			elapsedMs := time.Since(startTime).Milliseconds()
			winrmErr := services.ClassifyError(err)
			updateWinRMTestResult(db, false, err.Error())
			c.JSON(http.StatusOK, WinRMTestResult{
				Success:        false,
				Message:        "WinRM portu erişilemiyor",
				ErrorDetail:    err.Error(),
				ErrorCategory:  winrmErr.Category,
				ErrorHint:      getWinRMErrorHint(winrmErr.Category),
				ResponseTimeMs: elapsedMs,
			})
			return
		}

		info, err := client.TestConnection(targetIP)
		if err != nil {
			elapsedMs := time.Since(startTime).Milliseconds()
			winrmErr := services.ClassifyError(err)
			updateWinRMTestResult(db, false, err.Error())
			c.JSON(http.StatusOK, WinRMTestResult{
				Success:        false,
				Message:        winrmErr.Message,
				ErrorDetail:    err.Error(),
				ErrorCategory:  winrmErr.Category,
				ErrorHint:      getWinRMErrorHint(winrmErr.Category),
				ResponseTimeMs: elapsedMs,
			})
			return
		}

		// Success - update test result
		updateWinRMTestResult(db, true, "")

		c.JSON(http.StatusOK, WinRMTestResult{
			Success:        true,
			Message:        fmt.Sprintf("WinRM bağlantısı başarılı! Hostname: %s", info.OSName),
			OSInfo:         info.OSName,
			ResponseTimeMs: info.CollectionMs,
		})
	}
}

// getWinRMErrorHint returns troubleshooting hints for error categories
func getWinRMErrorHint(category string) string {
	hints := map[string]string{
		services.ErrCategoryDNS:           "Hedef makinenin DNS adını kontrol edin veya IP adresi kullanın",
		services.ErrCategoryTimeout:       "Hedef makine erişilebilir mi? Firewall kurallarını kontrol edin",
		services.ErrCategoryAuthFailed:    "Kullanıcı adı/şifre doğru mu? Domain\\username formatını deneyin",
		services.ErrCategoryWinRMDisabled: "Hedef makinede WinRM servisi etkin mi? 'winrm quickconfig' çalıştırın",
		services.ErrCategorySSLError:      "SSL/TLS sertifikası geçerli mi? HTTPS yerine HTTP deneyin",
		services.ErrCategoryConnection:    "Port açık mı? Firewall kurallarını kontrol edin (5985/5986)",
		services.ErrCategoryUnknown:       "Detaylı hata mesajını inceleyin",
	}
	if hint, ok := hints[category]; ok {
		return hint
	}
	return ""
}

// updateWinRMTestResult saves WinRM test result to database
func updateWinRMTestResult(db *sql.DB, success bool, errorMsg string) {
	result := "success"
	if !success {
		result = "failed"
	}

	var errPtr *string
	if errorMsg != "" {
		errPtr = &errorMsg
	}

	_, err := db.Exec(`
		UPDATE winrm_settings SET
			last_test_at = NOW(),
			last_test_result = ?,
			last_test_error = ?
		WHERE id = 1
	`, result, errPtr)
	if err != nil {
		log.Printf("Failed to update WinRM test result: %v", err)
	}
}

// EnrichInventoryRequest represents the request body for enriching inventory items
type EnrichInventoryRequest struct {
	InventoryIDs []int `json:"inventory_ids"`
}

// EnrichInventoryResult represents the result of enriching a single inventory item
type EnrichInventoryResult struct {
	InventoryID   int    `json:"inventory_id"`
	IPAddress     string `json:"ip_address,omitempty"`
	AssetName     string `json:"asset_name,omitempty"`
	Success       bool   `json:"success"`
	Message       string `json:"message"`
	ErrorDetail   string `json:"error_detail,omitempty"`
	ErrorCategory string `json:"error_category,omitempty"`
	DurationMs    int64  `json:"duration_ms,omitempty"`
}

// EnrichInventoryResponse represents the response for the enrichment endpoint
type EnrichInventoryResponse struct {
	TotalRequested  int                     `json:"total_requested"`
	TotalSuccess    int                     `json:"total_success"`
	TotalFailed     int                     `json:"total_failed"`
	TotalSkipped    int                     `json:"total_skipped"`
	TotalDurationMs int64                   `json:"total_duration_ms"`
	Results         []EnrichInventoryResult `json:"results"`
}

// EnrichInventoryWithWinRM enriches inventory items with hardware info via WinRM
func EnrichInventoryWithWinRM(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		startTime := time.Now()

		var req EnrichInventoryRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Geçersiz istek: " + err.Error()})
			return
		}

		if len(req.InventoryIDs) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "En az bir envanter ID'si gerekli"})
			return
		}

		// Get WinRM settings
		var username, encryptedPassword string
		var port, timeoutSeconds, concurrentLimit int
		var useSSL, isEnabled bool

		err := db.QueryRow(`
			SELECT username, password_encrypted, port, use_ssl, timeout_seconds, concurrent_limit, is_enabled
			FROM winrm_settings WHERE id = 1
		`).Scan(&username, &encryptedPassword, &port, &useSSL, &timeoutSeconds, &concurrentLimit, &isEnabled)

		if err != nil || !isEnabled {
			c.JSON(http.StatusBadRequest, gin.H{"error": "WinRM ayarları yapılandırılmamış veya etkin değil"})
			return
		}

		// Decrypt password
		password, err := auth.DecryptPassword(encryptedPassword)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Şifre çözümlenemedi: " + err.Error()})
			return
		}

		if username == "" || password == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "WinRM kimlik bilgileri eksik"})
			return
		}

		// Build inventory map: ID -> (IP, AssetName)
		type inventoryItem struct {
			ID        int
			IPAddress string
			AssetName string
		}
		items := make([]inventoryItem, 0, len(req.InventoryIDs))
		ipToInvID := make(map[string]int)

		response := EnrichInventoryResponse{
			TotalRequested: len(req.InventoryIDs),
			Results:        []EnrichInventoryResult{},
		}

		// First pass: get all inventory items and validate
		for _, invID := range req.InventoryIDs {
			var ipAddress sql.NullString
			var assetName string
			err := db.QueryRow(`SELECT ip_address, asset_name FROM inventory WHERE id = ?`, invID).Scan(&ipAddress, &assetName)

			if err != nil {
				response.Results = append(response.Results, EnrichInventoryResult{
					InventoryID: invID,
					Success:     false,
					Message:     "Envanter kaydı bulunamadı",
					ErrorDetail: err.Error(),
				})
				response.TotalSkipped++
				continue
			}

			if !ipAddress.Valid || ipAddress.String == "" {
				response.Results = append(response.Results, EnrichInventoryResult{
					InventoryID: invID,
					AssetName:   assetName,
					Success:     false,
					Message:     "IP adresi yok",
				})
				response.TotalSkipped++
				continue
			}

			items = append(items, inventoryItem{
				ID:        invID,
				IPAddress: ipAddress.String,
				AssetName: assetName,
			})
			ipToInvID[ipAddress.String] = invID
		}

		// If no valid items to process, return early
		if len(items) == 0 {
			response.TotalDurationMs = time.Since(startTime).Milliseconds()
			c.JSON(http.StatusOK, response)
			return
		}

		// Create WinRM client with concurrency limit
		client := services.NewWinRMClient(username, password, port, useSSL, timeoutSeconds)
		client.ConcurrentLimit = concurrentLimit

		// Collect IPs for batch enrichment
		ips := make([]string, 0, len(items))
		for _, item := range items {
			ips = append(ips, item.IPAddress)
		}

		// Perform concurrent batch enrichment
		batchResult := client.EnrichInventoryItems(ips)

		// Process results and update database
		for _, item := range items {
			enrichResult, ok := batchResult.Results[item.IPAddress]
			if !ok {
				response.Results = append(response.Results, EnrichInventoryResult{
					InventoryID: item.ID,
					IPAddress:   item.IPAddress,
					AssetName:   item.AssetName,
					Success:     false,
					Message:     "Zenginleştirme sonucu bulunamadı",
				})
				response.TotalFailed++
				continue
			}

			result := EnrichInventoryResult{
				InventoryID: item.ID,
				IPAddress:   item.IPAddress,
				AssetName:   item.AssetName,
				DurationMs:  enrichResult.DurationMs,
			}

			if !enrichResult.Success {
				result.Success = false
				result.Message = "WinRM bağlantı hatası"
				if enrichResult.Error != nil {
					result.Message = enrichResult.Error.Message
					result.ErrorDetail = enrichResult.Error.Detail
					result.ErrorCategory = enrichResult.Error.Category
				}
				response.TotalFailed++

				// Update inventory with categorized error (safe JSON generation)
				errorData := map[string]string{
					"category":  result.ErrorCategory,
					"message":   result.Message,
					"detail":    result.ErrorDetail,
					"timestamp": time.Now().Format(time.RFC3339),
				}
				errorJSON, _ := json.Marshal(errorData)
				_, dbErr := db.Exec(`UPDATE inventory SET last_winrm_enrich_at = NOW(), enrichment_errors = ? WHERE id = ?`,
					string(errorJSON), item.ID)
				if dbErr != nil {
					log.Printf("Failed to update inventory enrichment error (ID %d): %v", item.ID, dbErr)
				}

				response.Results = append(response.Results, result)
				continue
			}

			// Success - update inventory with hardware info
			hwInfo := enrichResult.HardwareInfo
			_, err = db.Exec(`
				UPDATE inventory SET
					brand = COALESCE(NULLIF(?, ''), brand),
					model = COALESCE(NULLIF(?, ''), model),
					serial_number = COALESCE(NULLIF(?, ''), serial_number),
					vendor = COALESCE(NULLIF(?, ''), vendor),
					hardware_json = ?,
					last_winrm_enrich_at = NOW(),
					enrichment_errors = NULL
				WHERE id = ?
			`,
				hwInfo.Manufacturer,
				hwInfo.Model,
				services.ParseSerialFromBIOS(hwInfo.SerialNumber),
				hwInfo.Manufacturer,
				hwInfo.ToJSON(),
				item.ID,
			)

			if err != nil {
				result.Success = false
				result.Message = "Veritabanı güncelleme hatası"
				result.ErrorDetail = err.Error()
				response.TotalFailed++
			} else {
				result.Success = true
				result.Message = fmt.Sprintf("%s %s", hwInfo.Manufacturer, hwInfo.Model)
				if hwInfo.CPUName != "" {
					result.Message += fmt.Sprintf(" | CPU: %s", hwInfo.CPUName)
				}
				if hwInfo.TotalRAMGB > 0 {
					result.Message += fmt.Sprintf(" | RAM: %.0fGB", hwInfo.TotalRAMGB)
				}
				response.TotalSuccess++
			}

			response.Results = append(response.Results, result)
		}

		response.TotalDurationMs = time.Since(startTime).Milliseconds()
		c.JSON(http.StatusOK, response)
	}
}

// SNMPSettings represents SNMP connection settings
type SNMPSettings struct {
	ID               int    `json:"id"`
	Version          string `json:"version"`
	CommunityStrings string `json:"community_strings"`
	Port             int    `json:"port"`
	TimeoutSeconds   int    `json:"timeout_seconds"`
	RetryCount       int    `json:"retry_count"`
	ConcurrentLimit  int    `json:"concurrent_limit"`
	IsEnabled        bool   `json:"is_enabled"`
	ScanTargets      string `json:"scan_targets"` // IP ranges for discovery (CIDR or range format)
}

// GetSNMPSettings returns the current SNMP settings (id=1)
func GetSNMPSettings(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var settings SNMPSettings

		err := db.QueryRow(`
			SELECT
				id, version, community_strings, port, timeout_seconds,
				retry_count, concurrent_limit, is_enabled, scan_targets
			FROM snmp_settings WHERE id = 1
		`).Scan(
			&settings.ID, &settings.Version, &settings.CommunityStrings, &settings.Port,
			&settings.TimeoutSeconds, &settings.RetryCount, &settings.ConcurrentLimit, &settings.IsEnabled,
			&settings.ScanTargets,
		)

		if err == sql.ErrNoRows {
			c.JSON(http.StatusOK, SNMPSettings{
				ID:               1,
				Version:          "v2c",
				CommunityStrings: `["public"]`,
				Port:             161,
				TimeoutSeconds:   5,
				RetryCount:       2,
				ConcurrentLimit:  10,
			})
			return
		} else if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Ayarlar okunamadı"})
			return
		}

		c.JSON(http.StatusOK, settings)
	}
}

// SNMPSettingsUpdateRequest represents the request body for updating SNMP settings
type SNMPSettingsUpdateRequest struct {
	Version          string `json:"version"`
	CommunityStrings string `json:"community_strings"`
	Port             int    `json:"port"`
	TimeoutSeconds   int    `json:"timeout_seconds"`
	RetryCount       int    `json:"retry_count"`
	ConcurrentLimit  int    `json:"concurrent_limit"`
	IsEnabled        bool   `json:"is_enabled"`
}

// SNMPTestResult represents the result of an SNMP connection test
type SNMPTestResult struct {
	Success        bool   `json:"success"`
	Message        string `json:"message"`
	ResponseTimeMs int64  `json:"response_time_ms,omitempty"`
	ErrorDetail    string `json:"error_detail,omitempty"`
	ErrorCategory  string `json:"error_category,omitempty"`
	ErrorHint      string `json:"error_hint,omitempty"`
	DeviceInfo     string `json:"device_info,omitempty"`
	CommunityUsed  string `json:"community_used,omitempty"`
}

// UpdateSNMPSettings updates the SNMP settings (id=1)
func UpdateSNMPSettings(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req SNMPSettingsUpdateRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Geçersiz istek: " + err.Error()})
			return
		}

		// Set defaults
		if req.Version == "" {
			req.Version = "v2c"
		}
		if req.Port == 0 {
			req.Port = 161
		}
		if req.TimeoutSeconds == 0 {
			req.TimeoutSeconds = 5
		}
		if req.RetryCount == 0 {
			req.RetryCount = 2
		}
		if req.ConcurrentLimit == 0 {
			req.ConcurrentLimit = 10
		}
		if req.CommunityStrings == "" {
			req.CommunityStrings = `["public"]`
		}

		_, err := db.Exec(`
			UPDATE snmp_settings SET
				version = ?,
				community_strings = ?,
				port = ?,
				timeout_seconds = ?,
				retry_count = ?,
				concurrent_limit = ?,
				is_enabled = ?,
				updated_at = NOW()
			WHERE id = 1
		`,
			req.Version, req.CommunityStrings, req.Port,
			req.TimeoutSeconds, req.RetryCount, req.ConcurrentLimit, req.IsEnabled,
		)

		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Ayarlar kaydedilemedi: " + err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{"message": "SNMP ayarları kaydedildi"})
	}
}

// TestSNMPConnection tests the SNMP connection to a target IP
func TestSNMPConnection(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		startTime := time.Now()

		targetIP := c.Query("target_ip")
		if targetIP == "" {
			c.JSON(http.StatusOK, SNMPTestResult{
				Success:        false,
				Message:        "Test için hedef IP gerekli",
				ResponseTimeMs: time.Since(startTime).Milliseconds(),
			})
			return
		}

		// Get current settings
		var version, communityStrings string
		var port, timeoutSeconds, retryCount int

		err := db.QueryRow(`
			SELECT version, community_strings, port, timeout_seconds, retry_count
			FROM snmp_settings WHERE id = 1
		`).Scan(&version, &communityStrings, &port, &timeoutSeconds, &retryCount)

		if err != nil {
			c.JSON(http.StatusOK, SNMPTestResult{
				Success:        false,
				Message:        "SNMP ayarları bulunamadı",
				ErrorDetail:    err.Error(),
				ResponseTimeMs: time.Since(startTime).Milliseconds(),
			})
			return
		}

		// Parse community strings
		var communities []string
		if err := json.Unmarshal([]byte(communityStrings), &communities); err != nil {
			communities = []string{"public"}
		}

		if len(communities) == 0 {
			c.JSON(http.StatusOK, SNMPTestResult{
				Success:        false,
				Message:        "Community string tanımlı değil",
				ResponseTimeMs: time.Since(startTime).Milliseconds(),
			})
			return
		}

		// Create SNMP client and test connection
		client := services.NewSNMPClient(version, communities, port, timeoutSeconds, retryCount, 1)

		info, err := client.TestConnection(targetIP)
		if err != nil {
			elapsedMs := time.Since(startTime).Milliseconds()
			snmpErr := services.ClassifySNMPError(err)
			c.JSON(http.StatusOK, SNMPTestResult{
				Success:        false,
				Message:        snmpErr.Message,
				ErrorDetail:    err.Error(),
				ErrorCategory:  snmpErr.Category,
				ErrorHint:      getSNMPErrorHint(snmpErr.Category),
				ResponseTimeMs: elapsedMs,
			})
			return
		}

		c.JSON(http.StatusOK, SNMPTestResult{
			Success:        true,
			Message:        fmt.Sprintf("SNMP bağlantısı başarılı! %s - %s", info.Vendor, info.DeviceType),
			DeviceInfo:     info.SysDescr,
			CommunityUsed:  info.CommunityUsed,
			ResponseTimeMs: info.CollectionMs,
		})
	}
}

// getSNMPErrorHint returns troubleshooting hints for SNMP error categories
func getSNMPErrorHint(category string) string {
	hints := map[string]string{
		services.SNMPErrCategoryTimeout:     "Hedef cihaz erişilebilir mi? Firewall kurallarını kontrol edin (UDP 161)",
		services.SNMPErrCategoryUnreachable: "Hedef IP adresi doğru mu? Ağ bağlantısını kontrol edin",
		services.SNMPErrCategoryNoResponse:  "Community string doğru mu? Cihazda SNMP etkin mi?",
		services.SNMPErrCategoryUnknown:     "Detaylı hata mesajını inceleyin",
	}
	if hint, ok := hints[category]; ok {
		return hint
	}
	return ""
}

// SNMPEnrichInventoryRequest represents the request body for SNMP enrichment
type SNMPEnrichInventoryRequest struct {
	InventoryIDs []int `json:"inventory_ids"`
}

// SNMPEnrichInventoryResult represents the result of enriching a single inventory item via SNMP
type SNMPEnrichInventoryResult struct {
	InventoryID   int    `json:"inventory_id"`
	IPAddress     string `json:"ip_address,omitempty"`
	AssetName     string `json:"asset_name,omitempty"`
	Success       bool   `json:"success"`
	Message       string `json:"message"`
	ErrorDetail   string `json:"error_detail,omitempty"`
	ErrorCategory string `json:"error_category,omitempty"`
	DurationMs    int64  `json:"duration_ms,omitempty"`
}

// SNMPEnrichInventoryResponse represents the response for SNMP enrichment
type SNMPEnrichInventoryResponse struct {
	TotalRequested  int                         `json:"total_requested"`
	TotalSuccess    int                         `json:"total_success"`
	TotalFailed     int                         `json:"total_failed"`
	TotalSkipped    int                         `json:"total_skipped"`
	TotalDurationMs int64                       `json:"total_duration_ms"`
	Results         []SNMPEnrichInventoryResult `json:"results"`
}

// EnrichInventoryWithSNMP enriches inventory items with SNMP info
func EnrichInventoryWithSNMP(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		startTime := time.Now()

		var req SNMPEnrichInventoryRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Geçersiz istek: " + err.Error()})
			return
		}

		if len(req.InventoryIDs) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "En az bir envanter ID'si gerekli"})
			return
		}

		// Get SNMP settings
		var version, communityStrings string
		var port, timeoutSeconds, retryCount, concurrentLimit int
		var isEnabled bool

		err := db.QueryRow(`
			SELECT version, community_strings, port, timeout_seconds, retry_count, concurrent_limit, is_enabled
			FROM snmp_settings WHERE id = 1
		`).Scan(&version, &communityStrings, &port, &timeoutSeconds, &retryCount, &concurrentLimit, &isEnabled)

		if err != nil || !isEnabled {
			c.JSON(http.StatusBadRequest, gin.H{"error": "SNMP ayarları yapılandırılmamış veya etkin değil"})
			return
		}

		// Parse community strings
		var communities []string
		if err := json.Unmarshal([]byte(communityStrings), &communities); err != nil {
			communities = []string{"public"}
		}

		if len(communities) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Community string tanımlı değil"})
			return
		}

		// Build inventory map with cached community hints
		type inventoryItem struct {
			ID             int
			IPAddress      string
			AssetName      string
			CachedCommunity string
			CachedIndex    int
		}
		items := make([]inventoryItem, 0, len(req.InventoryIDs))

		response := SNMPEnrichInventoryResponse{
			TotalRequested: len(req.InventoryIDs),
			Results:        []SNMPEnrichInventoryResult{},
		}

		// First pass: get all inventory items and validate, including cached SNMP info
		for _, invID := range req.InventoryIDs {
			var ipAddress, snmpJSON sql.NullString
			var assetName string
			err := db.QueryRow(`SELECT ip_address, asset_name, snmp_json FROM inventory WHERE id = ?`, invID).Scan(&ipAddress, &assetName, &snmpJSON)

			if err != nil {
				response.Results = append(response.Results, SNMPEnrichInventoryResult{
					InventoryID: invID,
					Success:     false,
					Message:     "Envanter kaydı bulunamadı",
					ErrorDetail: err.Error(),
				})
				response.TotalSkipped++
				continue
			}

			if !ipAddress.Valid || ipAddress.String == "" {
				response.Results = append(response.Results, SNMPEnrichInventoryResult{
					InventoryID: invID,
					AssetName:   assetName,
					Success:     false,
					Message:     "IP adresi yok",
				})
				response.TotalSkipped++
				continue
			}

			// Extract cached community from previous snmp_json if available
			cachedCommunity := ""
			cachedIndex := -1
			if snmpJSON.Valid && snmpJSON.String != "" {
				var cached struct {
					CommunityUsed  string `json:"community_used"`
					CommunityIndex int    `json:"community_index"`
				}
				if json.Unmarshal([]byte(snmpJSON.String), &cached) == nil {
					cachedCommunity = cached.CommunityUsed
					cachedIndex = cached.CommunityIndex
				}
			}

			items = append(items, inventoryItem{
				ID:              invID,
				IPAddress:       ipAddress.String,
				AssetName:       assetName,
				CachedCommunity: cachedCommunity,
				CachedIndex:     cachedIndex,
			})
		}

		if len(items) == 0 {
			response.TotalDurationMs = time.Since(startTime).Milliseconds()
			c.JSON(http.StatusOK, response)
			return
		}

		// Create SNMP client
		client := services.NewSNMPClient(version, communities, port, timeoutSeconds, retryCount, concurrentLimit)

		// Collect IPs and build hints map for batch enrichment
		ips := make([]string, 0, len(items))
		hints := make(map[string]services.CommunityHint)
		for _, item := range items {
			ips = append(ips, item.IPAddress)
			// Add cached community hint if available
			if item.CachedCommunity != "" && item.CachedIndex >= 0 {
				hints[item.IPAddress] = services.CommunityHint{
					Community: item.CachedCommunity,
					Index:     item.CachedIndex,
				}
			}
		}

		// Perform concurrent batch enrichment with hints
		batchResult := client.EnrichInventoryItemsWithHints(ips, hints)

		// Process results and update database
		for _, item := range items {
			enrichResult, ok := batchResult.Results[item.IPAddress]
			if !ok {
				response.Results = append(response.Results, SNMPEnrichInventoryResult{
					InventoryID: item.ID,
					IPAddress:   item.IPAddress,
					AssetName:   item.AssetName,
					Success:     false,
					Message:     "Zenginleştirme sonucu bulunamadı",
				})
				response.TotalFailed++
				continue
			}

			result := SNMPEnrichInventoryResult{
				InventoryID: item.ID,
				IPAddress:   item.IPAddress,
				AssetName:   item.AssetName,
				DurationMs:  enrichResult.DurationMs,
			}

			if !enrichResult.Success {
				result.Success = false
				result.Message = "SNMP bağlantı hatası"
				if enrichResult.Error != nil {
					result.Message = enrichResult.Error.Message
					result.ErrorDetail = enrichResult.Error.Detail
					result.ErrorCategory = enrichResult.Error.Category
				}
				response.TotalFailed++

				// Update inventory with categorized error (safe JSON generation)
				errorData := map[string]string{
					"category":  result.ErrorCategory,
					"message":   result.Message,
					"detail":    result.ErrorDetail,
					"timestamp": time.Now().Format(time.RFC3339),
				}
				errorJSON, _ := json.Marshal(errorData)
				_, dbErr := db.Exec(`UPDATE inventory SET last_snmp_enrich_at = NOW(), enrichment_errors = ? WHERE id = ?`,
					string(errorJSON), item.ID)
				if dbErr != nil {
					log.Printf("Failed to update SNMP enrichment error (ID %d): %v", item.ID, dbErr)
				}

				response.Results = append(response.Results, result)
				continue
			}

			// Success - update inventory with SNMP info
			deviceInfo := enrichResult.DeviceInfo
			_, err = db.Exec(`
				UPDATE inventory SET
					vendor = COALESCE(NULLIF(?, ''), vendor),
					hostname = COALESCE(NULLIF(?, ''), hostname),
					snmp_json = ?,
					last_snmp_enrich_at = NOW(),
					enrichment_errors = NULL
				WHERE id = ?
			`,
				deviceInfo.Vendor,
				deviceInfo.SysName,
				deviceInfo.ToJSON(),
				item.ID,
			)

			if err != nil {
				result.Success = false
				result.Message = "Veritabanı güncelleme hatası"
				result.ErrorDetail = err.Error()
				response.TotalFailed++
			} else {
				result.Success = true
				result.Message = fmt.Sprintf("%s | %s | %s", deviceInfo.Vendor, deviceInfo.DeviceType, deviceInfo.SysName)
				response.TotalSuccess++
			}

			response.Results = append(response.Results, result)
		}

		response.TotalDurationMs = time.Since(startTime).Milliseconds()
		c.JSON(http.StatusOK, response)
	}
}

// GetAllDiscoverySettings returns all discovery settings (AD, WinRM, SNMP)
func GetAllDiscoverySettings(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		// This is a convenience endpoint that returns all settings at once
		// Useful for the settings UI

		type AllSettings struct {
			AD    interface{} `json:"ad"`
			WinRM interface{} `json:"winrm"`
			SNMP  interface{} `json:"snmp"`
		}

		// We'll call individual handlers internally
		// For simplicity, just return minimal info for now
		c.JSON(http.StatusOK, gin.H{
			"message": "Use individual endpoints: /ad-settings, /winrm-settings, /snmp-settings",
		})
	}
}

// =====================================================
// AD DISCOVERY - Fetch computers from Active Directory
// =====================================================

// ADDiscoveryRequest represents the request to discover computers from AD
type ADDiscoveryRequest struct {
	DryRun bool `json:"dry_run"` // If true, only list computers without adding to inventory
}

// ADDiscoveryResponse represents the response from AD discovery
type ADDiscoveryResponse struct {
	Success          bool                  `json:"success"`
	Message          string                `json:"message"`
	TotalFound       int                   `json:"total_found"`
	NewAdded         int                   `json:"new_added"`
	Updated          int                   `json:"updated"`
	Skipped          int                   `json:"skipped"`
	Errors           int                   `json:"errors"`
	DurationMs       int64                 `json:"duration_ms"`
	Computers        []ADComputerInfo      `json:"computers,omitempty"`
	ErrorDetails     []string              `json:"error_details,omitempty"`
}

// ADComputerInfo represents a computer discovered from AD
type ADComputerInfo struct {
	// Core Identifiers
	ObjectGUID         string    `json:"object_guid"`
	DistinguishedName  string    `json:"distinguished_name"`
	ComputerName       string    `json:"computer_name"`
	CNName             string    `json:"cn_name"`
	DNSHostName        string    `json:"dns_hostname"`

	// OS Information
	OSName             string    `json:"os_name"`
	OSVersion          string    `json:"os_version"`
	OSServicePack      string    `json:"os_service_pack"`

	// Organizational
	Description        string    `json:"description"`
	Comment            string    `json:"comment"`
	Location           string    `json:"location"`
	ManagedBy          string    `json:"managed_by"`
	OUPath             string    `json:"ou_path"`

	// Timestamps
	LastLogon          *time.Time `json:"last_logon,omitempty"`
	WhenCreated        *time.Time `json:"when_created,omitempty"`
	WhenChanged        *time.Time `json:"when_changed,omitempty"`
	PwdLastSet         *time.Time `json:"pwd_last_set,omitempty"`

	// Network & Status
	IPv4Address        string    `json:"ipv4_address,omitempty"`
	ServicePrincipalNames []string `json:"service_principal_names,omitempty"`
	Enabled            bool      `json:"enabled"`
}

// DiscoverFromAD discovers computers from Active Directory and optionally adds them to inventory
func DiscoverFromAD(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		startTime := time.Now()

		var req ADDiscoveryRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			// Default: not dry run
			req.DryRun = false
		}

		response := ADDiscoveryResponse{
			Success:      false,
			Computers:    []ADComputerInfo{},
			ErrorDetails: []string{},
		}

		// Get AD settings
		var dc, baseDN, username, encryptedPassword, searchFilter, searchScope string
		var port int
		var useSSL bool
		var ouFilter sql.NullString

		err := db.QueryRow(`
			SELECT domain_controller, port, use_ssl, base_dn, bind_username,
			       bind_password_encrypted, search_filter, search_scope, ou_filter
			FROM ad_settings WHERE id = 1
		`).Scan(&dc, &port, &useSSL, &baseDN, &username, &encryptedPassword,
			&searchFilter, &searchScope, &ouFilter)

		if err != nil {
			response.Message = "AD ayarları bulunamadı"
			response.ErrorDetails = append(response.ErrorDetails, err.Error())
			c.JSON(http.StatusBadRequest, response)
			return
		}

		// Decrypt password
		password, err := auth.DecryptPassword(encryptedPassword)
		if err != nil {
			response.Message = "Şifre çözümlenemedi"
			response.ErrorDetails = append(response.ErrorDetails, err.Error())
			c.JSON(http.StatusInternalServerError, response)
			return
		}

		// Validate
		if dc == "" || username == "" || password == "" {
			response.Message = "AD ayarları eksik (DC, kullanıcı adı veya şifre boş)"
			c.JSON(http.StatusBadRequest, response)
			return
		}

		// Connect to AD
		conn, err := connectToAD(dc, port, useSSL, username, password)
		if err != nil {
			response.Message = "AD'ye bağlanılamadı"
			response.ErrorDetails = append(response.ErrorDetails, err.Error())
			c.JSON(http.StatusInternalServerError, response)
			return
		}
		defer conn.Close()

		// Auto-detect Base DN if empty
		if baseDN == "" {
			detectedBaseDN, detectErr := autoDetectBaseDN(conn)
			if detectErr != nil {
				response.Message = "Base DN otomatik tespit edilemedi"
				response.ErrorDetails = append(response.ErrorDetails, detectErr.Error())
				c.JSON(http.StatusInternalServerError, response)
				return
			}
			baseDN = detectedBaseDN
			log.Printf("Base DN otomatik tespit edildi: %s", baseDN)
		}

		// Parse OU filter if present
		var ouFilterList []string
		if ouFilter.Valid && ouFilter.String != "" {
			if err := json.Unmarshal([]byte(ouFilter.String), &ouFilterList); err != nil {
				log.Printf("Failed to parse OU filter: %v", err)
			}
		}

		// Search for computers
		if searchFilter == "" {
			searchFilter = "(objectClass=computer)"
		}

		// Map search scope
		scope := ldap.ScopeWholeSubtree
		switch searchScope {
		case "base":
			scope = ldap.ScopeBaseObject
		case "one":
			scope = ldap.ScopeSingleLevel
		case "sub":
			scope = ldap.ScopeWholeSubtree
		}

		searchRequest := ldap.NewSearchRequest(
			baseDN,
			scope,
			ldap.NeverDerefAliases,
			0,    // No size limit
			0,    // No time limit
			false, // TypesOnly
			searchFilter,
			[]string{
				// Core identifiers
				"objectGUID",
				"distinguishedName",
				"sAMAccountName",
				"cn",
				"dNSHostName",

				// OS Information
				"operatingSystem",
				"operatingSystemVersion",
				"operatingSystemServicePack",

				// Organizational
				"description",
				"comment",
				"location",
				"managedBy",

				// Timestamps
				"whenCreated",
				"whenChanged",
				"lastLogonTimestamp",
				"pwdLastSet",

				// Status
				"userAccountControl",

				// Network (if populated)
				"servicePrincipalName",
			},
			nil,
		)

		sr, err := conn.Search(searchRequest)
		if err != nil {
			response.Message = "AD sorgusu başarısız"
			response.ErrorDetails = append(response.ErrorDetails, err.Error())
			c.JSON(http.StatusInternalServerError, response)
			return
		}

		response.TotalFound = len(sr.Entries)

		// Process each computer
		for _, entry := range sr.Entries {
			computer := parseADComputer(entry, ouFilterList)

			// Skip disabled computers
			if !computer.Enabled {
				response.Skipped++
				continue
			}

			// Check if OU filtering applies
			if len(ouFilterList) > 0 && !isOUAllowed(computer.DistinguishedName, ouFilterList) {
				response.Skipped++
				continue
			}

			response.Computers = append(response.Computers, computer)

			// If not dry run, add/update in inventory
			if !req.DryRun {
				// Check BEFORE upsert to count correctly
				var existsBefore int
				db.QueryRow("SELECT COUNT(*) FROM inventory WHERE ad_object_guid = ?",
					computer.ObjectGUID).Scan(&existsBefore)

				if err := upsertInventoryFromAD(db, computer); err != nil {
					response.Errors++
					response.ErrorDetails = append(response.ErrorDetails,
						fmt.Sprintf("%s: %v", computer.ComputerName, err))
					log.Printf("Failed to upsert inventory for %s: %v", computer.ComputerName, err)
				} else {
					if existsBefore > 0 {
						response.Updated++
					} else {
						response.NewAdded++
					}
				}
			}
		}

		response.Success = true
		response.DurationMs = time.Since(startTime).Milliseconds()

		if req.DryRun {
			response.Message = fmt.Sprintf("AD'den %d bilgisayar bulundu (test modu)", response.TotalFound)
		} else {
			response.Message = fmt.Sprintf("AD keşfi tamamlandı: %d yeni, %d güncellendi",
				response.NewAdded, response.Updated)

			// Log scan to inventory_scans table
			logADScan(db, response)
		}

		c.JSON(http.StatusOK, response)
	}
}

// connectToAD creates a secure connection to Active Directory
func connectToAD(dc string, port int, useSSL bool, username, password string) (*ldap.Conn, error) {
	connTimeout := 10 * time.Second
	address := fmt.Sprintf("%s:%d", dc, port)

	var conn *ldap.Conn
	var err error

	if useSSL {
		tlsConfig := &tls.Config{
			ServerName:         dc,
			InsecureSkipVerify: false,
		}

		netConn, dialErr := net.DialTimeout("tcp", address, connTimeout)
		if dialErr != nil {
			return nil, dialErr
		}

		tlsConn := tls.Client(netConn, tlsConfig)
		if err = tlsConn.Handshake(); err != nil {
			netConn.Close()
			return nil, fmt.Errorf("TLS handshake failed: %v", err)
		}

		conn = ldap.NewConn(tlsConn, true)
		conn.Start()
	} else {
		netConn, dialErr := net.DialTimeout("tcp", address, connTimeout)
		if dialErr != nil {
			return nil, dialErr
		}

		conn = ldap.NewConn(netConn, false)
		conn.Start()
	}

	// Bind (authenticate)
	err = conn.Bind(username, password)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("bind failed: %v", err)
	}

	return conn, nil
}

// autoDetectBaseDN queries rootDSE to get the default naming context (Base DN)
func autoDetectBaseDN(conn *ldap.Conn) (string, error) {
	// Search rootDSE (empty base DN, base scope)
	searchRequest := ldap.NewSearchRequest(
		"",                         // Empty base DN = rootDSE
		ldap.ScopeBaseObject,       // Base scope
		ldap.NeverDerefAliases,
		0, 0, false,
		"(objectClass=*)",          // Match anything
		[]string{"defaultNamingContext"}, // Attribute we want
		nil,
	)

	result, err := conn.Search(searchRequest)
	if err != nil {
		return "", fmt.Errorf("rootDSE query failed: %v", err)
	}

	if len(result.Entries) == 0 {
		return "", fmt.Errorf("rootDSE query returned no entries")
	}

	baseDN := result.Entries[0].GetAttributeValue("defaultNamingContext")
	if baseDN == "" {
		return "", fmt.Errorf("defaultNamingContext not found in rootDSE")
	}

	return baseDN, nil
}

// parseADComputer parses an LDAP entry into ADComputerInfo
func parseADComputer(entry *ldap.Entry, ouFilterList []string) ADComputerInfo {
	computer := ADComputerInfo{}

	// Parse objectGUID (binary to string)
	guidBytes := entry.GetRawAttributeValue("objectGUID")
	if len(guidBytes) == 16 {
		computer.ObjectGUID = fmt.Sprintf("%x-%x-%x-%x-%x",
			guidBytes[0:4], guidBytes[4:6], guidBytes[6:8], guidBytes[8:10], guidBytes[10:16])
	}

	computer.DistinguishedName = entry.GetAttributeValue("distinguishedName")

	// sAMAccountName has '$' suffix for computers (e.g. "COMPUTER1$"), strip it
	samName := entry.GetAttributeValue("sAMAccountName")
	computer.ComputerName = strings.TrimSuffix(samName, "$")

	// CN (Common Name)
	computer.CNName = entry.GetAttributeValue("cn")

	// Network
	computer.DNSHostName = entry.GetAttributeValue("dNSHostName")

	// OS Information
	computer.OSName = entry.GetAttributeValue("operatingSystem")
	computer.OSVersion = entry.GetAttributeValue("operatingSystemVersion")
	computer.OSServicePack = entry.GetAttributeValue("operatingSystemServicePack")

	// Organizational
	computer.Description = entry.GetAttributeValue("description")
	computer.Comment = entry.GetAttributeValue("comment")
	computer.Location = entry.GetAttributeValue("location")
	computer.ManagedBy = entry.GetAttributeValue("managedBy")

	// Service Principal Names (multi-valued)
	computer.ServicePrincipalNames = entry.GetAttributeValues("servicePrincipalName")

	// Extract OU path from DN
	computer.OUPath = extractOUPath(computer.DistinguishedName)

	// Parse lastLogonTimestamp (Windows filetime)
	if lastLogonStr := entry.GetAttributeValue("lastLogonTimestamp"); lastLogonStr != "" {
		if t := parseWindowsFiletime(lastLogonStr); t != nil {
			computer.LastLogon = t
		}
	}

	// Parse whenCreated
	if whenCreatedStr := entry.GetAttributeValue("whenCreated"); whenCreatedStr != "" {
		if t, err := time.Parse("20060102150405.0Z", whenCreatedStr); err == nil {
			computer.WhenCreated = &t
		}
	}

	// Parse whenChanged
	if whenChangedStr := entry.GetAttributeValue("whenChanged"); whenChangedStr != "" {
		if t, err := time.Parse("20060102150405.0Z", whenChangedStr); err == nil {
			computer.WhenChanged = &t
		}
	}

	// Parse pwdLastSet (Windows filetime)
	if pwdLastSetStr := entry.GetAttributeValue("pwdLastSet"); pwdLastSetStr != "" {
		if t := parseWindowsFiletime(pwdLastSetStr); t != nil {
			computer.PwdLastSet = t
		}
	}

	// Check if account is enabled (userAccountControl bit 2 = disabled)
	if uacStr := entry.GetAttributeValue("userAccountControl"); uacStr != "" {
		var uac int
		fmt.Sscanf(uacStr, "%d", &uac)
		computer.Enabled = (uac & 0x2) == 0
	}

	return computer
}

// extractOUPath extracts the OU path from a distinguished name
func extractOUPath(dn string) string {
	// Example: CN=COMPUTER1,OU=Workstations,OU=Headquarters,DC=company,DC=local
	// Returns: OU=Workstations,OU=Headquarters
	parsedDN, err := ldap.ParseDN(dn)
	if err != nil {
		return ""
	}

	parts := []string{}
	for _, part := range parsedDN.RDNs {
		for _, attr := range part.Attributes {
			if attr.Type == "OU" {
				parts = append(parts, fmt.Sprintf("OU=%s", attr.Value))
			}
		}
	}

	if len(parts) == 0 {
		return ""
	}

	result := parts[0]
	for i := 1; i < len(parts); i++ {
		result += "," + parts[i]
	}
	return result
}

// isOUAllowed checks if a computer's DN matches the OU filter
func isOUAllowed(dn string, ouFilterList []string) bool {
	if len(ouFilterList) == 0 {
		return true
	}

	dnLower := strings.ToLower(dn)
	for _, allowedOU := range ouFilterList {
		if strings.Contains(dnLower, strings.ToLower(allowedOU)) {
			return true
		}
	}
	return false
}

// parseWindowsFiletime converts Windows filetime to Go time
func parseWindowsFiletime(filetimeStr string) *time.Time {
	var filetime int64
	if _, err := fmt.Sscanf(filetimeStr, "%d", &filetime); err != nil {
		return nil
	}

	// Windows filetime is 100-nanosecond intervals since 1601-01-01
	// Unix epoch is 1970-01-01, difference is 116444736000000000
	if filetime == 0 {
		return nil
	}

	unixTime := (filetime - 116444736000000000) / 10000000
	t := time.Unix(unixTime, 0)
	return &t
}

// resolveIP tries to resolve a hostname to an IPv4 address
func resolveIP(hostname string) string {
	if hostname == "" {
		return ""
	}

	ips, err := net.LookupHost(hostname)
	if err != nil || len(ips) == 0 {
		return ""
	}

	// Return the first IPv4 address
	for _, ip := range ips {
		if net.ParseIP(ip) != nil && strings.Contains(ip, ".") {
			return ip
		}
	}
	return ips[0]
}

// upsertInventoryFromAD inserts or updates an inventory record from AD data
func upsertInventoryFromAD(db *sql.DB, computer ADComputerInfo) error {
	// Try to resolve actual IP from DNS hostname
	resolvedIP := resolveIP(computer.DNSHostName)

	// Serialize ServicePrincipalNames to JSON
	spnJSON := ""
	if len(computer.ServicePrincipalNames) > 0 {
		if spnBytes, err := json.Marshal(computer.ServicePrincipalNames); err == nil {
			spnJSON = string(spnBytes)
		}
	}

	// Use Location from AD if populated, otherwise derive from OU
	location := computer.Location
	if location == "" && computer.OUPath != "" {
		// Extract last OU component as location fallback
		parts := strings.Split(computer.OUPath, ",")
		if len(parts) > 0 {
			location = strings.TrimPrefix(parts[0], "OU=")
		}
	}

	// Check if exists
	var existingID int
	err := db.QueryRow("SELECT id FROM inventory WHERE ad_object_guid = ?",
		computer.ObjectGUID).Scan(&existingID)

	if err == sql.ErrNoRows {
		// Insert new
		_, err = db.Exec(`
			INSERT INTO inventory (
				ad_object_guid, ad_distinguished_name, ad_computer_name, ad_cn_name,
				ad_description, ad_comment, ad_location, ad_managed_by, ad_ou_path,
				ad_os_name, ad_os_version, ad_os_service_pack,
				ad_last_logon, ad_when_created, ad_when_changed, ad_pwd_last_set,
				ad_service_principal_names,
				asset_name, hostname, ip_address, location, source, discovery_source,
				last_ad_sync_at, created_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'ad_ldap', 'ad_ldap', NOW(), NOW())
		`,
			computer.ObjectGUID, computer.DistinguishedName, computer.ComputerName, computer.CNName,
			computer.Description, computer.Comment, computer.Location, computer.ManagedBy, computer.OUPath,
			computer.OSName, computer.OSVersion, computer.OSServicePack,
			computer.LastLogon, computer.WhenCreated, computer.WhenChanged, computer.PwdLastSet,
			spnJSON,
			computer.ComputerName, computer.DNSHostName, resolvedIP, location,
		)
		return err
	} else if err != nil {
		return err
	}

	// Update existing
	_, err = db.Exec(`
		UPDATE inventory SET
			ad_distinguished_name = ?,
			ad_computer_name = ?,
			ad_cn_name = ?,
			ad_description = ?,
			ad_comment = ?,
			ad_location = ?,
			ad_managed_by = ?,
			ad_ou_path = ?,
			ad_os_name = ?,
			ad_os_version = ?,
			ad_os_service_pack = ?,
			ad_last_logon = ?,
			ad_when_created = ?,
			ad_when_changed = ?,
			ad_pwd_last_set = ?,
			ad_service_principal_names = ?,
			hostname = COALESCE(NULLIF(?, ''), hostname),
			ip_address = COALESCE(NULLIF(?, ''), ip_address),
			location = COALESCE(NULLIF(?, ''), location),
			last_ad_sync_at = NOW(),
			updated_at = NOW()
		WHERE id = ?
	`,
		computer.DistinguishedName, computer.ComputerName, computer.CNName,
		computer.Description, computer.Comment, computer.Location, computer.ManagedBy,
		computer.OUPath, computer.OSName, computer.OSVersion, computer.OSServicePack,
		computer.LastLogon, computer.WhenCreated, computer.WhenChanged, computer.PwdLastSet,
		spnJSON,
		computer.DNSHostName, resolvedIP, location,
		existingID,
	)
	return err
}

// logADScan logs the AD discovery scan to inventory_scans table
func logADScan(db *sql.DB, response ADDiscoveryResponse) {
	errorsJSON, _ := json.Marshal(response.ErrorDetails)

	_, err := db.Exec(`
		INSERT INTO inventory_scans (
			scan_type, status, total_targets, processed, success_count, error_count,
			new_assets, updated_assets, errors_json, started_at, completed_at
		) VALUES (
			'ad_discovery', 'completed', ?, ?, ?, ?, ?, ?, ?, NOW(), NOW()
		)
	`,
		response.TotalFound,
		response.TotalFound - response.Skipped,
		response.NewAdded + response.Updated,
		response.Errors,
		response.NewAdded,
		response.Updated,
		string(errorsJSON),
	)

	if err != nil {
		log.Printf("Failed to log AD scan: %v", err)
	}
}

// ============================================
// SSH Settings Handlers
// ============================================

// SSHSettings represents the SSH settings from the database
type SSHSettings struct {
	ID                int    `json:"id"`
	Username          string `json:"username"`
	PasswordEncrypted string `json:"password_encrypted,omitempty"` // Don't send to frontend
	Port              int    `json:"port"`
	TimeoutSeconds    int    `json:"timeout"`
	IsEnabled         bool   `json:"is_enabled"`
}

// SSHSettingsUpdateRequest represents the request body for updating SSH settings
type SSHSettingsUpdateRequest struct {
	Username string `json:"username"`
	Password string `json:"password"` // Plain text from frontend
	Port     int    `json:"port"`
	Timeout  int    `json:"timeout"`
	IsEnabled bool  `json:"is_enabled"`
}

// GetSSHSettings retrieves SSH settings from the database
func GetSSHSettings(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var settings SSHSettings

		err := db.QueryRow(`
			SELECT id, username, port, timeout_seconds, is_enabled
			FROM ssh_settings WHERE id = 1
		`).Scan(
			&settings.ID, &settings.Username, &settings.Port,
			&settings.TimeoutSeconds, &settings.IsEnabled,
		)

		if err == sql.ErrNoRows {
			c.JSON(http.StatusOK, SSHSettings{
				ID:             1,
				Username:       "root",
				Port:           22,
				TimeoutSeconds: 10,
				IsEnabled:      false,
			})
			return
		} else if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "SSH ayarları okunamadı"})
			return
		}

		c.JSON(http.StatusOK, settings)
	}
}

// UpdateSSHSettings updates SSH settings in the database
func UpdateSSHSettings(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req SSHSettingsUpdateRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Geçersiz istek: " + err.Error()})
			return
		}

		// Set defaults
		if req.Username == "" {
			req.Username = "root"
		}
		if req.Port == 0 {
			req.Port = 22
		}
		if req.Timeout == 0 {
			req.Timeout = 10
		}

		// Encrypt password if provided and not placeholder
		var encryptedPassword string
		if req.Password != "" && req.Password != "********" {
			encrypted, err := auth.EncryptPassword(req.Password)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Şifre şifrelenemedi"})
				return
			}
			encryptedPassword = encrypted
		}

		// Update query
		var err error
		if encryptedPassword != "" {
			_, err = db.Exec(`
				UPDATE ssh_settings SET
					username = ?,
					password_encrypted = ?,
					port = ?,
					timeout_seconds = ?,
					is_enabled = ?,
					updated_at = NOW()
				WHERE id = 1
			`,
				req.Username, encryptedPassword, req.Port, req.Timeout, req.IsEnabled,
			)
		} else {
			// Password not changed, update other fields only
			_, err = db.Exec(`
				UPDATE ssh_settings SET
					username = ?,
					port = ?,
					timeout_seconds = ?,
					is_enabled = ?,
					updated_at = NOW()
				WHERE id = 1
			`,
				req.Username, req.Port, req.Timeout, req.IsEnabled,
			)
		}

		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "SSH ayarları kaydedilemedi: " + err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{"message": "SSH ayarları kaydedildi"})
	}
}
