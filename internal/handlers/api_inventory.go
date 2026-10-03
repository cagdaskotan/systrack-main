package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// InventoryItem represents an inventory record
type InventoryItem struct {
	ID               int       `json:"id"`
	IPAddress        *string   `json:"ip_address"`
	MACAddress       *string   `json:"mac_address"`
	Hostname         *string   `json:"hostname"`
	Vendor           *string   `json:"vendor"`
	AssetName        string    `json:"asset_name"`
	AssetTag         *string   `json:"asset_tag"`
	AssetType        string    `json:"asset_type"`
	Brand            *string   `json:"brand"`
	Model            *string   `json:"model"`
	SerialNumber     *string   `json:"serial_number"`
	Location         *string   `json:"location"`
	Department       *string   `json:"department"`
	AssignedTo       *string   `json:"assigned_to"`
	Status           string    `json:"status"`
	PurchaseDate     *string   `json:"purchase_date"`
	WarrantyExpiry   *string   `json:"warranty_expiry"`
	PurchaseCost     *float64  `json:"purchase_cost"`
	Notes            *string   `json:"notes"`
	Source           string    `json:"source"`
	TargetID         *int      `json:"target_id"`
	ResolvedTargetID *int      `json:"resolved_target_id"`
	TargetLinkState  string    `json:"target_link_state"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
	// AD fields - Core
	ADOSName        *string                `json:"ad_os_name"`
	ADOSVersion     *string                `json:"ad_os_version"`
	ADOSServicePack *string                `json:"ad_os_service_pack"`
	ADDescription   *string                `json:"ad_description"`
	ADComment       *string                `json:"ad_comment"`
	ADLocation      *string                `json:"ad_location"`
	ADManagedBy     *string                `json:"ad_managed_by"`
	ADOUPath        *string                `json:"ad_ou_path"`
	OwnerJSON       map[string]interface{} `json:"owner_json,omitempty"`
	WinRMJSON       map[string]interface{} `json:"winrm_json,omitempty"`

	// AD fields - Timestamps
	ADLastLogon   *time.Time `json:"ad_last_logon"`
	ADWhenChanged *time.Time `json:"ad_when_changed"`
	ADPwdLastSet  *time.Time `json:"ad_pwd_last_set"`

	// AD fields - Meta
	DiscoverySource *string    `json:"discovery_source"`
	LastADSyncAt    *time.Time `json:"last_ad_sync_at"`

	// Yeni alanlar: yazılım, uptime, güç
	SoftwareJSON       []interface{}  `json:"software_json,omitempty"`
	SoftwareScanAt     *time.Time     `json:"software_scan_at"`
	LastBootTime       *string        `json:"last_boot_time"`
	PowerEstimateWatts *int           `json:"power_estimate_watts"`
	CrackScanAt        *time.Time     `json:"crack_scan_at"`
	CrackRiskLevel     *string        `json:"crack_risk_level"`
	CrackFindings      []CrackFinding `json:"crack_findings,omitempty"`

	// Computed fields
	IsMonitored  bool    `json:"is_monitored"`
	TargetStatus *string `json:"target_status,omitempty"`
}

// InventoryStats represents inventory statistics
type InventoryStats struct {
	Total            int `json:"total"`
	Active           int `json:"active"`
	Maintenance      int `json:"maintenance"`
	Storage          int `json:"storage"`
	Faulty           int `json:"faulty"`
	Retired          int `json:"retired"`
	Monitored        int `json:"monitored"`
	WarrantyExpiring int `json:"warranty_expiring"` // 30 gün içinde bitecek
}

// CreateInventoryRequest represents the request body for creating inventory
type CreateInventoryRequest struct {
	IPAddress      *string  `json:"ip_address"`
	MACAddress     *string  `json:"mac_address"`
	Hostname       *string  `json:"hostname"`
	Vendor         *string  `json:"vendor"`
	AssetName      string   `json:"asset_name" binding:"required"`
	AssetTag       *string  `json:"asset_tag"`
	AssetType      string   `json:"asset_type" binding:"required"`
	Brand          *string  `json:"brand"`
	Model          *string  `json:"model"`
	SerialNumber   *string  `json:"serial_number"`
	Location       *string  `json:"location"`
	Department     *string  `json:"department"`
	AssignedTo     *string  `json:"assigned_to"`
	Status         string   `json:"status"`
	PurchaseDate   *string  `json:"purchase_date"`
	WarrantyExpiry *string  `json:"warranty_expiry"`
	PurchaseCost   *float64 `json:"purchase_cost"`
	Notes          *string  `json:"notes"`
	Source         string   `json:"source"`
}

// BulkCreateInventoryRequest represents bulk create from IP Scanner
type BulkCreateInventoryRequest struct {
	Devices []struct {
		IPAddress  string  `json:"ip_address"`
		MACAddress *string `json:"mac_address"`
		Hostname   *string `json:"hostname"`
		Vendor     *string `json:"vendor"`
	} `json:"devices"`
	AssetType  string  `json:"asset_type"`
	Location   *string `json:"location"`
	Department *string `json:"department"`
}

func buildInventoryTag(id int64) string {
	return fmt.Sprintf("INV-%05d", id)
}

// GetInventoryList returns paginated inventory list with filters
func GetInventoryList(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Query params
		search := strings.TrimSpace(c.Query("search"))
		assetType := strings.TrimSpace(c.Query("type"))
		status := strings.TrimSpace(c.Query("status"))
		location := strings.TrimSpace(c.Query("location"))
		limitStr := c.DefaultQuery("limit", "100")
		offsetStr := c.DefaultQuery("offset", "0")

		limit, _ := strconv.Atoi(limitStr)
		offset, _ := strconv.Atoi(offsetStr)
		if limit <= 0 || limit > 500 {
			limit = 100
		}
		if offset < 0 {
			offset = 0
		}

		// Build query
		baseFrom := `
			FROM inventory i
			LEFT JOIN (
				SELECT address, MAX(id) AS id
				FROM targets
				GROUP BY address
			) t ON t.address = i.ip_address OR t.address = i.hostname
			WHERE 1=1
		`
		query := `
			SELECT
				i.id, i.ip_address, i.mac_address, i.hostname, i.vendor,
				i.asset_name, i.asset_tag, i.asset_type, i.brand, i.model,
				i.serial_number, i.location, i.department, i.assigned_to,
				i.status, i.purchase_date, i.warranty_expiry, i.purchase_cost,
				i.notes, i.source, i.target_id,
				i.ad_os_name, i.ad_os_version, i.ad_os_service_pack,
				i.ad_description, i.ad_comment, i.ad_location, i.ad_managed_by,
				i.ad_ou_path, i.owner_json, i.winrm_json,
				i.ad_last_logon, i.ad_when_changed, i.ad_pwd_last_set,
				i.discovery_source, i.last_ad_sync_at,
				i.last_boot_time, i.power_estimate_watts,
				i.crack_scan_at, i.crack_risk_level, i.crack_findings_json,
				COALESCE(i.target_id, t.id) as resolved_target_id,
				CASE
					WHEN i.target_id IS NOT NULL THEN 'linked'
					WHEN t.id IS NOT NULL THEN 'matched'
					ELSE 'none'
				END as target_link_state,
				i.created_at, i.updated_at,
				CASE WHEN COALESCE(i.target_id, t.id) IS NOT NULL THEN true ELSE false END as is_monitored
		` + baseFrom
		var args []interface{}
		filterSQL := ""

		if search != "" {
			filterSQL += ` AND (i.asset_name LIKE ? OR i.ip_address LIKE ? OR i.mac_address LIKE ? OR i.hostname LIKE ? OR i.asset_tag LIKE ? OR i.serial_number LIKE ?)`
			searchPattern := "%" + search + "%"
			args = append(args, searchPattern, searchPattern, searchPattern, searchPattern, searchPattern, searchPattern)
		}

		if assetType != "" && assetType != "all" {
			filterSQL += ` AND i.asset_type = ?`
			args = append(args, assetType)
		}

		if status != "" && status != "all" {
			filterSQL += ` AND i.status = ?`
			args = append(args, status)
		}

		if location != "" && location != "all" {
			filterSQL += ` AND i.location LIKE ?`
			args = append(args, "%"+location+"%")
		}

		if assignedTo := strings.TrimSpace(c.Query("assigned_to")); assignedTo != "" {
			filterSQL += ` AND i.assigned_to = ?`
			args = append(args, assignedTo)
		}

		// Count total
		query += filterSQL
		countQuery := "SELECT COUNT(*) " + baseFrom + filterSQL

		var total int
		if err := db.QueryRow(countQuery, args...).Scan(&total); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Sayım hatası"})
			return
		}

		// Add order and limit
		query += ` ORDER BY i.updated_at DESC LIMIT ? OFFSET ?`
		args = append(args, limit, offset)

		rows, err := db.Query(query, args...)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Veritabanı hatası"})
			return
		}
		defer rows.Close()

		globalExceptions, perItemExceptions := loadCrackExceptionSets(db)
		var items []InventoryItem
		for rows.Next() {
			var item InventoryItem
			var purchaseDate, warrantyExpiry sql.NullString
			var purchaseCost sql.NullFloat64
			var resolvedTargetID sql.NullInt64
			var adOSName, adOSVersion, adOSServicePack, adDescription, adComment, adLocation, adManagedBy, adOUPath, ownerJSON, winrmJSON, discoverySource sql.NullString
			var adLastLogon, adWhenChanged, adPwdLastSet, lastADSyncAt sql.NullTime
			var lastBootTime sql.NullString
			var powerEstimateWatts sql.NullInt64
			var crackScanAt sql.NullTime
			var crackRiskLevel, crackFindingsJSON sql.NullString

			err := rows.Scan(
				&item.ID, &item.IPAddress, &item.MACAddress, &item.Hostname, &item.Vendor,
				&item.AssetName, &item.AssetTag, &item.AssetType, &item.Brand, &item.Model,
				&item.SerialNumber, &item.Location, &item.Department, &item.AssignedTo,
				&item.Status, &purchaseDate, &warrantyExpiry, &purchaseCost,
				&item.Notes, &item.Source, &item.TargetID,
				&adOSName, &adOSVersion, &adOSServicePack,
				&adDescription, &adComment, &adLocation, &adManagedBy,
				&adOUPath, &ownerJSON, &winrmJSON,
				&adLastLogon, &adWhenChanged, &adPwdLastSet,
				&discoverySource, &lastADSyncAt,
				&lastBootTime, &powerEstimateWatts,
				&crackScanAt, &crackRiskLevel, &crackFindingsJSON,
				&resolvedTargetID,
				&item.TargetLinkState, &item.CreatedAt, &item.UpdatedAt,
				&item.IsMonitored,
			)
			if err != nil {
				continue
			}

			if purchaseDate.Valid {
				item.PurchaseDate = &purchaseDate.String
			}
			if warrantyExpiry.Valid {
				item.WarrantyExpiry = &warrantyExpiry.String
			}
			if purchaseCost.Valid {
				item.PurchaseCost = &purchaseCost.Float64
			}
			if resolvedTargetID.Valid {
				idVal := int(resolvedTargetID.Int64)
				item.ResolvedTargetID = &idVal
			}
			// Handle AD fields
			if adOSName.Valid {
				item.ADOSName = &adOSName.String
			}
			if adOSVersion.Valid {
				item.ADOSVersion = &adOSVersion.String
			}
			if adOSServicePack.Valid {
				item.ADOSServicePack = &adOSServicePack.String
			}
			if adDescription.Valid {
				item.ADDescription = &adDescription.String
			}
			if adComment.Valid {
				item.ADComment = &adComment.String
			}
			if adLocation.Valid {
				item.ADLocation = &adLocation.String
			}
			if adManagedBy.Valid {
				item.ADManagedBy = &adManagedBy.String
			}
			if adOUPath.Valid {
				item.ADOUPath = &adOUPath.String
			}
			if ownerJSON.Valid && strings.TrimSpace(ownerJSON.String) != "" {
				var ownerData map[string]interface{}
				if json.Unmarshal([]byte(ownerJSON.String), &ownerData) == nil {
					item.OwnerJSON = ownerData
				}
			}
			if winrmJSON.Valid && strings.TrimSpace(winrmJSON.String) != "" {
				var winrmData map[string]interface{}
				if json.Unmarshal([]byte(winrmJSON.String), &winrmData) == nil {
					item.WinRMJSON = winrmData
				}
			}
			if adLastLogon.Valid {
				item.ADLastLogon = &adLastLogon.Time
			}
			if adWhenChanged.Valid {
				item.ADWhenChanged = &adWhenChanged.Time
			}
			if adPwdLastSet.Valid {
				item.ADPwdLastSet = &adPwdLastSet.Time
			}
			if discoverySource.Valid {
				item.DiscoverySource = &discoverySource.String
			}
			if lastADSyncAt.Valid {
				item.LastADSyncAt = &lastADSyncAt.Time
			}
			if lastBootTime.Valid {
				item.LastBootTime = &lastBootTime.String
			}
			if powerEstimateWatts.Valid {
				v := int(powerEstimateWatts.Int64)
				item.PowerEstimateWatts = &v
			}
			if crackScanAt.Valid {
				item.CrackScanAt = &crackScanAt.Time
			}
			if crackRiskLevel.Valid {
				item.CrackRiskLevel = &crackRiskLevel.String
			}
			if crackFindingsJSON.Valid && strings.TrimSpace(crackFindingsJSON.String) != "" {
				var findings []CrackFinding
				if json.Unmarshal([]byte(crackFindingsJSON.String), &findings) == nil {
					hashes := globalExceptions
					if perItem := perItemExceptions[item.ID]; len(perItem) > 0 {
						hashes = make(map[string]bool, len(globalExceptions)+len(perItem))
						for h := range globalExceptions {
							hashes[h] = true
						}
						for h := range perItem {
							hashes[h] = true
						}
					}
					annotated, adjustedRisk := annotateExceptedFindings(findings, hashes)
					item.CrackFindings = annotated
					item.CrackRiskLevel = &adjustedRisk
				}
			}
			items = append(items, item)
		}

		c.JSON(http.StatusOK, gin.H{
			"items":  items,
			"total":  total,
			"limit":  limit,
			"offset": offset,
		})
	}
}

// GetInventoryStats returns inventory statistics
func GetInventoryStats(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var stats InventoryStats

		// Total and status counts
		err := db.QueryRow(`
			SELECT
				COUNT(*) as total,
				SUM(CASE WHEN status = 'active' THEN 1 ELSE 0 END) as active,
				SUM(CASE WHEN status = 'maintenance' THEN 1 ELSE 0 END) as maintenance,
				SUM(CASE WHEN status = 'storage' THEN 1 ELSE 0 END) as storage,
				SUM(CASE WHEN status = 'faulty' THEN 1 ELSE 0 END) as faulty,
				SUM(CASE WHEN status = 'retired' THEN 1 ELSE 0 END) as retired,
				SUM(CASE WHEN target_id IS NOT NULL THEN 1 ELSE 0 END) as monitored,
				SUM(CASE WHEN warranty_expiry IS NOT NULL AND warranty_expiry <= DATE_ADD(CURDATE(), INTERVAL 30 DAY) AND warranty_expiry >= CURDATE() THEN 1 ELSE 0 END) as warranty_expiring
			FROM inventory
		`).Scan(
			&stats.Total, &stats.Active, &stats.Maintenance, &stats.Storage,
			&stats.Faulty, &stats.Retired, &stats.Monitored, &stats.WarrantyExpiring,
		)

		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "İstatistik hatası"})
			return
		}

		c.JSON(http.StatusOK, stats)
	}
}

// GetInventoryItem returns a single inventory item
func GetInventoryItem(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := strconv.Atoi(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Geçersiz ID"})
			return
		}

		var item InventoryItem
		var purchaseDate, warrantyExpiry sql.NullString
		var purchaseCost sql.NullFloat64

		var swJSON, lbtTime, crackRiskLevel, crackFindingsJSON sql.NullString
		var swScanAt, crackScanAt sql.NullTime
		var pwrWatts sql.NullInt64

		err = db.QueryRow(`
			SELECT
				id, ip_address, mac_address, hostname, vendor,
				asset_name, asset_tag, asset_type, brand, model,
				serial_number, location, department, assigned_to,
				status, purchase_date, warranty_expiry, purchase_cost,
				notes, source, target_id, created_at, updated_at,
				software_json, software_scan_at, last_boot_time, power_estimate_watts,
				crack_scan_at, crack_risk_level, crack_findings_json
			FROM inventory WHERE id = ?
		`, id).Scan(
			&item.ID, &item.IPAddress, &item.MACAddress, &item.Hostname, &item.Vendor,
			&item.AssetName, &item.AssetTag, &item.AssetType, &item.Brand, &item.Model,
			&item.SerialNumber, &item.Location, &item.Department, &item.AssignedTo,
			&item.Status, &purchaseDate, &warrantyExpiry, &purchaseCost,
			&item.Notes, &item.Source, &item.TargetID, &item.CreatedAt, &item.UpdatedAt,
			&swJSON, &swScanAt, &lbtTime, &pwrWatts,
			&crackScanAt, &crackRiskLevel, &crackFindingsJSON,
		)

		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "Kayıt bulunamadı"})
			return
		} else if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Veritabanı hatası: " + err.Error()})
			return
		}

		if purchaseDate.Valid {
			item.PurchaseDate = &purchaseDate.String
		}
		if warrantyExpiry.Valid {
			item.WarrantyExpiry = &warrantyExpiry.String
		}
		if purchaseCost.Valid {
			item.PurchaseCost = &purchaseCost.Float64
		}
		if swJSON.Valid && swJSON.String != "" {
			var swList []interface{}
			if json.Unmarshal([]byte(swJSON.String), &swList) == nil {
				item.SoftwareJSON = swList
			}
		}
		if swScanAt.Valid {
			item.SoftwareScanAt = &swScanAt.Time
		}
		if lbtTime.Valid {
			item.LastBootTime = &lbtTime.String
		}
		if pwrWatts.Valid {
			v := int(pwrWatts.Int64)
			item.PowerEstimateWatts = &v
		}
		if crackScanAt.Valid {
			item.CrackScanAt = &crackScanAt.Time
		}
		if crackRiskLevel.Valid {
			item.CrackRiskLevel = &crackRiskLevel.String
		}
		if crackFindingsJSON.Valid && strings.TrimSpace(crackFindingsJSON.String) != "" {
			var findings []CrackFinding
			if json.Unmarshal([]byte(crackFindingsJSON.String), &findings) == nil {
				annotated, adjustedRisk := applyCrackScanExceptions(db, item.ID, findings)
				item.CrackFindings = annotated
				item.CrackRiskLevel = &adjustedRisk
			}
		}

		item.IsMonitored = item.TargetID != nil

		c.JSON(http.StatusOK, item)
	}
}

// CreateInventoryItem creates a new inventory item
func CreateInventoryItem(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req CreateInventoryRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Geçersiz istek"})
			return
		}

		// Default values
		if req.Status == "" {
			req.Status = "active"
		}
		if req.Source == "" {
			req.Source = "manual"
		}

		result, err := db.Exec(`
			INSERT INTO inventory (
				ip_address, mac_address, hostname, vendor,
				asset_name, asset_tag, asset_type, brand, model,
				serial_number, location, department, assigned_to,
				status, purchase_date, warranty_expiry, purchase_cost,
				notes, source
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`,
			req.IPAddress, req.MACAddress, req.Hostname, req.Vendor,
			req.AssetName, req.AssetTag, req.AssetType, req.Brand, req.Model,
			req.SerialNumber, req.Location, req.Department, req.AssignedTo,
			req.Status, req.PurchaseDate, req.WarrantyExpiry, req.PurchaseCost,
			req.Notes, req.Source,
		)

		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Kayıt oluşturulamadı: " + err.Error()})
			return
		}

		id, _ := result.LastInsertId()
		if req.AssetTag == nil || strings.TrimSpace(*req.AssetTag) == "" {
			tag := buildInventoryTag(id)
			_, _ = db.Exec(`UPDATE inventory SET asset_tag = ? WHERE id = ?`, tag, id)
		}

		c.JSON(http.StatusCreated, gin.H{"id": id, "message": "Envanter kaydı oluşturuldu"})
	}
}

// UpdateInventoryItem updates an existing inventory item
func UpdateInventoryItem(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := strconv.Atoi(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Geçersiz ID"})
			return
		}

		var req CreateInventoryRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Geçersiz istek"})
			return
		}

		if req.AssetTag == nil || strings.TrimSpace(*req.AssetTag) == "" {
			tag := buildInventoryTag(int64(id))
			req.AssetTag = &tag
		}

		_, err = db.Exec(`
			UPDATE inventory SET
				ip_address = ?, mac_address = ?, hostname = ?, vendor = ?,
				asset_name = ?, asset_tag = ?, asset_type = ?, brand = ?, model = ?,
				serial_number = ?, location = ?, department = ?, assigned_to = ?,
				status = ?, purchase_date = ?, warranty_expiry = ?, purchase_cost = ?,
				notes = ?
			WHERE id = ?
		`,
			req.IPAddress, req.MACAddress, req.Hostname, req.Vendor,
			req.AssetName, req.AssetTag, req.AssetType, req.Brand, req.Model,
			req.SerialNumber, req.Location, req.Department, req.AssignedTo,
			req.Status, req.PurchaseDate, req.WarrantyExpiry, req.PurchaseCost,
			req.Notes, id,
		)

		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Güncelleme hatası"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"message": "Envanter kaydı güncellendi"})
	}
}

// DeleteInventoryItem deletes an inventory item
func DeleteInventoryItem(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := strconv.Atoi(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Geçersiz ID"})
			return
		}

		_, err = db.Exec(`DELETE FROM inventory WHERE id = ?`, id)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Silme hatası"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"message": "Envanter kaydı silindi"})
	}
}

// BulkCreateFromScanner creates multiple inventory items from IP Scanner
func BulkCreateFromScanner(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req BulkCreateInventoryRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Geçersiz istek"})
			return
		}

		if len(req.Devices) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "En az bir cihaz seçmelisiniz"})
			return
		}

		if req.AssetType == "" {
			req.AssetType = "other"
		}

		created := 0
		duplicates := 0
		failed := 0

		for _, device := range req.Devices {
			// Check if already exists by MAC or IP
			var exists int
			if device.MACAddress != nil && *device.MACAddress != "" {
				db.QueryRow(`SELECT COUNT(*) FROM inventory WHERE mac_address = ?`, *device.MACAddress).Scan(&exists)
			}
			if exists == 0 && device.IPAddress != "" {
				db.QueryRow(`SELECT COUNT(*) FROM inventory WHERE ip_address = ?`, device.IPAddress).Scan(&exists)
			}

			if exists > 0 {
				duplicates++
				continue
			}

			// Generate asset name
			assetName := device.IPAddress
			if device.Hostname != nil && *device.Hostname != "" {
				assetName = *device.Hostname
			}

			_, err := db.Exec(`
				INSERT INTO inventory (ip_address, mac_address, hostname, vendor, asset_name, asset_type, location, department, source)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'ip_scanner')
			`, device.IPAddress, device.MACAddress, device.Hostname, device.Vendor, assetName, req.AssetType, req.Location, req.Department)

			if err != nil {
				failed++
			} else {
				created++
			}
		}

		c.JSON(http.StatusOK, gin.H{
			"created":    created,
			"duplicates": duplicates,
			"failed":     failed,
		})
	}
}

// LinkInventoryToTarget links an inventory item to a target
func LinkInventoryToTarget(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := strconv.Atoi(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Geçersiz ID"})
			return
		}

		var req struct {
			TargetID int `json:"target_id"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Geçersiz istek"})
			return
		}

		_, err = db.Exec(`UPDATE inventory SET target_id = ? WHERE id = ?`, req.TargetID, id)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Bağlantı hatası"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"message": "Hedef bağlantısı oluşturuldu"})
	}
}

// UnlinkInventoryFromTarget removes target link from inventory
func UnlinkInventoryFromTarget(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := strconv.Atoi(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Geçersiz ID"})
			return
		}

		_, err = db.Exec(`UPDATE inventory SET target_id = NULL WHERE id = ?`, id)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Bağlantı kaldırma hatası"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"message": "Hedef bağlantısı kaldırıldı"})
	}
}

// GetInventoryLocations returns distinct locations for filtering
func GetInventoryLocations(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		rows, err := db.Query(`SELECT DISTINCT location FROM inventory WHERE location IS NOT NULL AND location != '' ORDER BY location`)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Veritabanı hatası"})
			return
		}
		defer rows.Close()

		var locations []string
		for rows.Next() {
			var loc string
			if err := rows.Scan(&loc); err == nil {
				locations = append(locations, loc)
			}
		}

		c.JSON(http.StatusOK, locations)
	}
}
