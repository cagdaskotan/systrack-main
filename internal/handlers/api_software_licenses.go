package handlers

import (
	"database/sql"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

// SoftwareLicense represents a software license / purchase record
type SoftwareLicense struct {
	ID                int        `json:"id"`
	InventoryID       int        `json:"inventory_id"`
	SoftwareName      string     `json:"software_name"`
	PurchaseCost      *float64   `json:"purchase_cost"`
	CostCurrency      string     `json:"cost_currency"`
	PurchaseDate      *string    `json:"purchase_date"`
	LicenseExpiryDate *string    `json:"license_expiry_date"`
	Notes             *string    `json:"notes"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

// SoftwareLicenseRequest is the request body for create/update
type SoftwareLicenseRequest struct {
	SoftwareName      string   `json:"software_name" binding:"required"`
	PurchaseCost      *float64 `json:"purchase_cost"`
	CostCurrency      string   `json:"cost_currency"`
	PurchaseDate      *string  `json:"purchase_date"`
	LicenseExpiryDate *string  `json:"license_expiry_date"`
	Notes             *string  `json:"notes"`
}

// GetSoftwareLicenses returns all license records for an inventory item
func GetSoftwareLicenses(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		inventoryID, err := strconv.Atoi(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Geçersiz envanter ID"})
			return
		}

		rows, err := db.QueryContext(c, `
			SELECT id, inventory_id, software_name, purchase_cost, cost_currency,
			       purchase_date, license_expiry_date, notes, created_at, updated_at
			FROM software_licenses
			WHERE inventory_id = ?
			ORDER BY software_name ASC
		`, inventoryID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Lisanslar alınamadı"})
			return
		}
		defer rows.Close()

		licenses := []SoftwareLicense{}
		for rows.Next() {
			var l SoftwareLicense
			var purchaseDate, expiryDate sql.NullString
			if err := rows.Scan(
				&l.ID, &l.InventoryID, &l.SoftwareName,
				&l.PurchaseCost, &l.CostCurrency,
				&purchaseDate, &expiryDate,
				&l.Notes, &l.CreatedAt, &l.UpdatedAt,
			); err != nil {
				continue
			}
			if purchaseDate.Valid {
				l.PurchaseDate = &purchaseDate.String
			}
			if expiryDate.Valid {
				l.LicenseExpiryDate = &expiryDate.String
			}
			licenses = append(licenses, l)
		}
		c.JSON(http.StatusOK, licenses)
	}
}

// UpsertSoftwareLicense creates or updates a license record for a software name
func UpsertSoftwareLicense(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		inventoryID, err := strconv.Atoi(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Geçersiz envanter ID"})
			return
		}

		var req SoftwareLicenseRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Geçersiz istek: " + err.Error()})
			return
		}

		if req.CostCurrency == "" {
			req.CostCurrency = "TRY"
		}

		// Upsert: INSERT ... ON DUPLICATE KEY UPDATE
		result, err := db.ExecContext(c, `
			INSERT INTO software_licenses
				(inventory_id, software_name, purchase_cost, cost_currency, purchase_date, license_expiry_date, notes)
			VALUES
				(?, ?, ?, ?, ?, ?, ?)
			ON DUPLICATE KEY UPDATE
				purchase_cost       = VALUES(purchase_cost),
				cost_currency       = VALUES(cost_currency),
				purchase_date       = VALUES(purchase_date),
				license_expiry_date = VALUES(license_expiry_date),
				notes               = VALUES(notes),
				updated_at          = CURRENT_TIMESTAMP
		`,
			inventoryID,
			req.SoftwareName,
			req.PurchaseCost,
			req.CostCurrency,
			req.PurchaseDate,
			req.LicenseExpiryDate,
			req.Notes,
		)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Kayıt oluşturulamadı: " + err.Error()})
			return
		}

		// Fetch the upserted record to return it
		var lid int64
		lastID, _ := result.LastInsertId()
		if lastID > 0 {
			lid = lastID
		} else {
			// ON DUPLICATE KEY UPDATE — fetch by composite key
			db.QueryRowContext(c,
				"SELECT id FROM software_licenses WHERE inventory_id=? AND software_name=?",
				inventoryID, req.SoftwareName,
			).Scan(&lid)
		}

		var l SoftwareLicense
		var purchaseDate, expiryDate sql.NullString
		db.QueryRowContext(c, `
			SELECT id, inventory_id, software_name, purchase_cost, cost_currency,
			       purchase_date, license_expiry_date, notes, created_at, updated_at
			FROM software_licenses WHERE id = ?
		`, lid).Scan(
			&l.ID, &l.InventoryID, &l.SoftwareName,
			&l.PurchaseCost, &l.CostCurrency,
			&purchaseDate, &expiryDate,
			&l.Notes, &l.CreatedAt, &l.UpdatedAt,
		)
		if purchaseDate.Valid {
			l.PurchaseDate = &purchaseDate.String
		}
		if expiryDate.Valid {
			l.LicenseExpiryDate = &expiryDate.String
		}

		c.JSON(http.StatusOK, l)
	}
}

// DeleteSoftwareLicense removes a license record by its ID
func DeleteSoftwareLicense(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		inventoryID, err := strconv.Atoi(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Geçersiz envanter ID"})
			return
		}
		licenseID, err := strconv.Atoi(c.Param("lid"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Geçersiz lisans ID"})
			return
		}

		res, err := db.ExecContext(c,
			"DELETE FROM software_licenses WHERE id = ? AND inventory_id = ?",
			licenseID, inventoryID,
		)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Silinemedi"})
			return
		}
		affected, _ := res.RowsAffected()
		if affected == 0 {
			c.JSON(http.StatusNotFound, gin.H{"error": "Kayıt bulunamadı"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "Lisans kaydı silindi"})
	}
}
