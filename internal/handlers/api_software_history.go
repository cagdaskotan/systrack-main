package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// ── Tipler ────────────────────────────────────────────────────────────────────

// softwareItem, software_json array'indeki tek bir yazılımı temsil eder.
type softwareItem struct {
	Name        string `json:"DisplayName"`
	Version     string `json:"DisplayVersion"`
	Publisher   string `json:"Publisher"`
	InstallDate string `json:"InstallDate"`
}

// SoftwareChange, bir tarama sırasında tespit edilen tek bir değişikliği temsil eder.
type SoftwareChange struct {
	ID           int       `json:"id"`
	InventoryID  int       `json:"inventory_id"`
	ScannedAt    time.Time `json:"scanned_at"`
	SoftwareName string    `json:"software_name"`
	Publisher    string    `json:"publisher,omitempty"`
	ChangeType   string    `json:"change_type"` // added | removed | version_changed
	NewVersion   string    `json:"new_version,omitempty"`
	PrevVersion  string    `json:"prev_version,omitempty"`
}

// ScanSettings, bir cihaza ait otomatik tarama ayarlarını temsil eder.
type ScanSettings struct {
	AutoSoftwareScan bool      `json:"auto_software_scan"`
	ScanHour         int       `json:"scan_hour"`
	LastAutoScanAt   *time.Time `json:"last_auto_scan_at,omitempty"`
}

// ── Yazılım karşılaştırma ve history kayıt ────────────────────────────────────

// parseSoftwareJSON, software_json LONGTEXT alanını []softwareItem'a dönüştürür.
func parseSoftwareJSON(raw string) []softwareItem {
	if raw == "" {
		return nil
	}
	var items []softwareItem
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		return nil
	}
	return items
}

// RecordSoftwareChanges, yeni ve önceki yazılım listelerini karşılaştırıp
// farklılıkları software_scan_history tablosuna yazar.
// Hiç kayıt yoksa (ilk tarama) tüm yazılımlar "added" olarak eklenir.
func RecordSoftwareChanges(db *sql.DB, inventoryID int, newJSON, prevJSON string) error {
	newItems := parseSoftwareJSON(newJSON)
	prevItems := parseSoftwareJSON(prevJSON)

	if len(newItems) == 0 {
		return nil
	}

	// Önceki listeyi isim → versiyon map'ine dönüştür
	prevMap := make(map[string]softwareItem, len(prevItems))
	for _, s := range prevItems {
		key := strings.ToLower(strings.TrimSpace(s.Name))
		if key != "" {
			prevMap[key] = s
		}
	}

	// Yeni listeyi isim → versiyon map'ine dönüştür
	newMap := make(map[string]softwareItem, len(newItems))
	for _, s := range newItems {
		key := strings.ToLower(strings.TrimSpace(s.Name))
		if key != "" {
			newMap[key] = s
		}
	}

	now := time.Now()
	isFirstScan := len(prevItems) == 0

	type row struct {
		name       string
		publisher  string
		changeType string
		newVer     string
		prevVer    string
	}
	var changes []row

	// Eklenen veya versiyonu değişen yazılımlar
	for key, newSw := range newMap {
		if prevSw, exists := prevMap[key]; !exists {
			// İlk taramada "added" gürültüsü oluşturma — sadece gerçek değişimleri kaydet.
			if !isFirstScan {
				changes = append(changes, row{newSw.Name, newSw.Publisher, "added", newSw.Version, ""})
			}
		} else {
			// Versiyon değişimi
			if strings.TrimSpace(newSw.Version) != strings.TrimSpace(prevSw.Version) &&
				strings.TrimSpace(newSw.Version) != "" {
				changes = append(changes, row{newSw.Name, newSw.Publisher, "version_changed", newSw.Version, prevSw.Version})
			}
		}
	}

	// Kaldırılan yazılımlar
	for key, prevSw := range prevMap {
		if _, exists := newMap[key]; !exists {
			if !isFirstScan {
				changes = append(changes, row{prevSw.Name, prevSw.Publisher, "removed", "", prevSw.Version})
			}
		}
	}

	if len(changes) == 0 {
		return nil
	}

	stmt, err := db.Prepare(`
		INSERT INTO software_scan_history
			(inventory_id, scanned_at, software_name, publisher, change_type, new_version, prev_version)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, c := range changes {
		if _, err := stmt.Exec(inventoryID, now, c.name, c.publisher, c.changeType,
			nullableString(c.newVer), nullableString(c.prevVer)); err != nil {
			return err
		}
	}
	return nil
}

// ── HTTP Handler'ları ─────────────────────────────────────────────────────────

// GetSoftwareChanges, bir cihazın son N gündeki yazılım değişim geçmişini döner.
func GetSoftwareChanges(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		invID, err := strconv.Atoi(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "geçersiz id"})
			return
		}
		days := 30
		if d, err := strconv.Atoi(c.DefaultQuery("days", "30")); err == nil && d > 0 && d <= 365 {
			days = d
		}

		rows, err := db.Query(`
			SELECT id, inventory_id, scanned_at, software_name,
			       COALESCE(publisher,''), change_type,
			       COALESCE(new_version,''), COALESCE(prev_version,'')
			FROM software_scan_history
			WHERE inventory_id = ?
			  AND scanned_at >= DATE_SUB(NOW(), INTERVAL ? DAY)
			ORDER BY scanned_at DESC, id DESC
			LIMIT 500
		`, invID, days)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		defer rows.Close()

		var result []SoftwareChange
		for rows.Next() {
			var ch SoftwareChange
			if err := rows.Scan(&ch.ID, &ch.InventoryID, &ch.ScannedAt, &ch.SoftwareName,
				&ch.Publisher, &ch.ChangeType, &ch.NewVersion, &ch.PrevVersion); err != nil {
				continue
			}
			result = append(result, ch)
		}
		if result == nil {
			result = []SoftwareChange{}
		}
		c.JSON(http.StatusOK, result)
	}
}

// GetScanSettings, bir cihazın otomatik tarama ayarlarını döner.
func GetScanSettings(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		invID, err := strconv.Atoi(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "geçersiz id"})
			return
		}

		var settings ScanSettings
		var lastAt sql.NullTime
		err = db.QueryRow(`
			SELECT auto_software_scan, scan_hour, last_auto_scan_at
			FROM inventory_scan_settings
			WHERE inventory_id = ?
		`, invID).Scan(&settings.AutoSoftwareScan, &settings.ScanHour, &lastAt)

		if err == sql.ErrNoRows {
			// Henüz ayar girilmemiş — varsayılan dön
			c.JSON(http.StatusOK, ScanSettings{AutoSoftwareScan: false, ScanHour: 3})
			return
		}
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if lastAt.Valid {
			settings.LastAutoScanAt = &lastAt.Time
		}
		c.JSON(http.StatusOK, settings)
	}
}

// UpdateScanSettings, bir cihazın otomatik tarama ayarlarını günceller (UPSERT).
func UpdateScanSettings(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		invID, err := strconv.Atoi(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "geçersiz id"})
			return
		}

		var req ScanSettings
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if req.ScanHour < 0 || req.ScanHour > 23 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "scan_hour 0-23 arasında olmalı"})
			return
		}

		_, err = db.Exec(`
			INSERT INTO inventory_scan_settings (inventory_id, auto_software_scan, scan_hour)
			VALUES (?, ?, ?)
			ON DUPLICATE KEY UPDATE
				auto_software_scan = VALUES(auto_software_scan),
				scan_hour          = VALUES(scan_hour)
		`, invID, req.AutoSoftwareScan, req.ScanHour)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true})
	}
}

// GetReportUsers, envanterde kayıtlı olan benzersiz sahibi/kullanıcı listesini döner.
// Kullanıcı bazlı rapor oluştururken dropdown'da göstermek için.
func GetReportUsers(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		rows, err := db.Query(`
			SELECT DISTINCT assigned_to
			FROM inventory
			WHERE assigned_to IS NOT NULL AND assigned_to != ''
			ORDER BY assigned_to
		`)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		defer rows.Close()

		var users []string
		for rows.Next() {
			var u string
			if err := rows.Scan(&u); err == nil && u != "" {
				users = append(users, u)
			}
		}
		if users == nil {
			users = []string{}
		}
		c.JSON(http.StatusOK, users)
	}
}

// GetSoftwareSnapshot, belirli bir cihazın güncel yazılım listesini döner;
// her yazılıma son taramadan bu yana olan değişim bilgisini (changeType, prevVersion) ekler.
func GetSoftwareSnapshot(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		invID, err := strconv.Atoi(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "geçersiz id"})
			return
		}

		// Güncel software_json
		var rawJSON sql.NullString
		var scanAt sql.NullTime
		err = db.QueryRow(`SELECT software_json, software_scan_at FROM inventory WHERE id = ?`, invID).
			Scan(&rawJSON, &scanAt)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		items := parseSoftwareJSON(rawJSON.String)

		// Son taramaya ait değişimler (o taramada eklenen/güncellenen yazılımlar)
		type changeMeta struct {
			ChangeType  string `json:"change_type,omitempty"`
			PrevVersion string `json:"prev_version,omitempty"`
		}
		changeMap := make(map[string]changeMeta)

		if scanAt.Valid {
			crows, err := db.Query(`
				SELECT software_name, change_type, COALESCE(prev_version,'')
				FROM software_scan_history
				WHERE inventory_id = ?
				  AND scanned_at >= DATE_SUB(?, INTERVAL 2 HOUR)
			`, invID, scanAt.Time)
			if err == nil {
				defer crows.Close()
				for crows.Next() {
					var name, ct, pv string
					if crows.Scan(&name, &ct, &pv) == nil {
						changeMap[strings.ToLower(name)] = changeMeta{ct, pv}
					}
				}
			}
		}

		// Kaldırılan yazılımları da dahil et (son taramada "removed" olanlar)
		type enrichedItem struct {
			softwareItem
			ChangeType  string `json:"change_type,omitempty"`
			PrevVersion string `json:"prev_version,omitempty"`
		}

		result := make([]enrichedItem, 0, len(items))
		for _, sw := range items {
			ei := enrichedItem{softwareItem: sw}
			if m, ok := changeMap[strings.ToLower(sw.Name)]; ok {
				ei.ChangeType = m.ChangeType
				ei.PrevVersion = m.PrevVersion
			}
			result = append(result, ei)
		}

		// "removed" olanları da ekle (artık software_json'da yok ama geçmişte var)
		if scanAt.Valid {
			rrows, err := db.Query(`
				SELECT software_name, COALESCE(publisher,''), COALESCE(prev_version,'')
				FROM software_scan_history
				WHERE inventory_id = ?
				  AND change_type = 'removed'
				  AND scanned_at >= DATE_SUB(?, INTERVAL 2 HOUR)
			`, invID, scanAt.Time)
			if err == nil {
				defer rrows.Close()
				for rrows.Next() {
					var name, pub, pv string
					if rrows.Scan(&name, &pub, &pv) == nil {
						result = append(result, enrichedItem{
							softwareItem: softwareItem{Name: name, Publisher: pub, Version: pv},
							ChangeType:   "removed",
						})
					}
				}
			}
		}

		c.JSON(http.StatusOK, gin.H{
			"items":       result,
			"scanned_at":  scanAt,
			"total":       len(result),
		})
	}
}
