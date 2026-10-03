package handlers

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

// ── Bulgu İstisnaları (false-positive yönetimi) ──────────────────────────────
// Bir bulgu "istisna" olarak işaretlendiğinde görünür kalır ama risk skoruna
// dahil edilmez (bkz. CrackFinding.Excepted, applyCrackScanExceptions).

type CrackScanExceptionItem struct {
	ID            int    `json:"id"`
	InventoryID   *int   `json:"inventory_id"`
	SignatureText string `json:"signature_text"`
	Reason        string `json:"reason"`
}

type CrackScanExceptionCreateRequest struct {
	Category string `json:"category"`
	Title    string `json:"title"`
	Evidence string `json:"evidence"`
	Reason   string `json:"reason"`
}

// GetCrackScanExceptions belirli bir envantere özel + global istisnaları döner.
func GetCrackScanExceptions(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		inventoryID, err := strconv.Atoi(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Geçersiz envanter ID"})
			return
		}

		rows, err := db.Query(`
			SELECT id, inventory_id, signature_text, reason
			FROM crack_scan_exceptions
			WHERE inventory_id = ? OR inventory_id IS NULL
			ORDER BY id DESC
		`, inventoryID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "İstisna listesi okunamadı"})
			return
		}
		defer rows.Close()

		items := make([]CrackScanExceptionItem, 0)
		for rows.Next() {
			var item CrackScanExceptionItem
			var invID sql.NullInt64
			if err := rows.Scan(&item.ID, &invID, &item.SignatureText, &item.Reason); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "İstisna listesi okunamadı"})
				return
			}
			if invID.Valid {
				v := int(invID.Int64)
				item.InventoryID = &v
			}
			items = append(items, item)
		}
		c.JSON(http.StatusOK, gin.H{"items": items})
	}
}

// CreateCrackScanException, category+title+evidence'tan türetilen imzayı bu
// envantere özel bir istisna olarak kaydeder (aynı imza başka makinelerde tekrar
// görünürse etkilenmez).
func CreateCrackScanException(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		inventoryID, err := strconv.Atoi(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Geçersiz envanter ID"})
			return
		}
		var req CrackScanExceptionCreateRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Geçersiz istek"})
			return
		}

		sig := findingSignature(CrackFinding{Category: req.Category, Title: req.Title, Evidence: req.Evidence})
		if sig == "" || sig == "||" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Bulgu bilgisi eksik (category/title/evidence)"})
			return
		}

		var createdBy *int
		if uid, ok := c.Get("user_id"); ok {
			if id, ok := uid.(int); ok {
				createdBy = &id
			}
		}

		_, err = db.Exec(`
			INSERT INTO crack_scan_exceptions (inventory_id, signature_hash, signature_text, reason, created_by)
			VALUES (?, ?, ?, ?, ?)
		`, inventoryID, sha256Hex(sig), sig, strings.TrimSpace(req.Reason), createdBy)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "İstisna eklenemedi"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "İstisna eklendi"})
	}
}

func DeleteCrackScanException(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		exceptionID := c.Param("exceptionId")
		if exceptionID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "ID gerekli"})
			return
		}
		if _, err := db.Exec(`DELETE FROM crack_scan_exceptions WHERE id = ?`, exceptionID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "İstisna silinemedi"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "İstisna silindi"})
	}
}

// applyCrackScanExceptions, kaydedilmiş istisnalara (bu envantere özel veya global)
// göre bulguları Excepted=true olarak işaretler ve risk seviyesini buna göre yeniden
// hesaplar. Bulgu listede görünür kalır, sadece risk skoruna dahil edilmez.
// Tek bir envanter kaydı için kullanılır (detay görünümü, tarama sonucu); çok sayıda
// kayıt render edilirken (envanter listesi) bunun yerine loadCrackExceptionSets +
// annotateExceptedFindings kullanılır, aksi halde her satır için bir sorgu gerekir.
func applyCrackScanExceptions(db *sql.DB, inventoryID int, findings []CrackFinding) ([]CrackFinding, string) {
	if db == nil || len(findings) == 0 {
		return findings, calcRiskLevel(findings)
	}

	rows, err := db.Query(`
		SELECT signature_hash FROM crack_scan_exceptions
		WHERE inventory_id = ? OR inventory_id IS NULL
	`, inventoryID)
	if err != nil {
		return findings, calcRiskLevel(findings)
	}
	defer rows.Close()

	hashes := map[string]bool{}
	for rows.Next() {
		var h string
		if rows.Scan(&h) == nil {
			hashes[h] = true
		}
	}
	return annotateExceptedFindings(findings, hashes)
}

// loadCrackExceptionSets, crack_scan_exceptions tablosunun tamamını tek sorguda okur:
// global istisnalar (inventory_id IS NULL, her kayda uygulanır) ve envanter-özel
// istisnalar. Envanter listesi gibi çok satırlı görünümlerde satır sayısı kadar sorgu
// atmamak için kullanılır.
func loadCrackExceptionSets(db *sql.DB) (global map[string]bool, perItem map[int]map[string]bool) {
	global = map[string]bool{}
	perItem = map[int]map[string]bool{}
	if db == nil {
		return
	}
	rows, err := db.Query(`SELECT inventory_id, signature_hash FROM crack_scan_exceptions`)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var invID sql.NullInt64
		var hash string
		if rows.Scan(&invID, &hash) != nil {
			continue
		}
		if !invID.Valid {
			global[hash] = true
			continue
		}
		id := int(invID.Int64)
		if perItem[id] == nil {
			perItem[id] = map[string]bool{}
		}
		perItem[id][hash] = true
	}
	return
}

// annotateExceptedFindings bulguları verilen hash kümesine göre Excepted=true ile
// işaretler ve risk seviyesini yeniden hesaplar. DB'ye dokunmaz.
func annotateExceptedFindings(findings []CrackFinding, hashes map[string]bool) ([]CrackFinding, string) {
	if len(findings) == 0 || len(hashes) == 0 {
		return findings, calcRiskLevel(findings)
	}
	out := make([]CrackFinding, len(findings))
	for i, f := range findings {
		f.Excepted = hashes[sha256Hex(findingSignature(f))]
		out[i] = f
	}
	return out, calcRiskLevel(out)
}

func sha256Hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}
