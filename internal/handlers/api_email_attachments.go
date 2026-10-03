package handlers

import (
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"systrack/internal/notifications"

	"github.com/gin-gonic/gin"
)

// EmailAttachmentHandler email ek dosya işleyicisi
type EmailAttachmentHandler struct {
	db *sql.DB
}

// NewEmailAttachmentHandler yeni email ek dosya handler oluştur
func NewEmailAttachmentHandler(db *sql.DB) *EmailAttachmentHandler {
	return &EmailAttachmentHandler{
		db: db,
	}
}

// UploadAttachment ek dosya yükle
func (h *EmailAttachmentHandler) UploadAttachment(c *gin.Context) {
	// Multipart form parse
	err := c.Request.ParseMultipartForm(32 << 20) // 32MB limit
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Failed to parse multipart form"})
		return
	}

	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "No file uploaded"})
		return
	}
	defer file.Close()

	// Dosya bilgilerini oku
	filename := header.Filename
	if filename == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Filename is required"})
		return
	}

	// Dosya boyutunu kontrol et (10MB limit)
	maxSize := int64(10 * 1024 * 1024)
	if header.Size > maxSize {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "File size exceeds 10MB limit"})
		return
	}

	// Dosya içeriğini oku
	data, err := io.ReadAll(file)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to read file"})
		return
	}

	// Content-Type belirle
	contentType := header.Header.Get("Content-Type")
	if contentType == "" {
		if strings.HasSuffix(filename, ".pdf") {
			contentType = "application/pdf"
		} else if strings.HasSuffix(filename, ".jpg") || strings.HasSuffix(filename, ".jpeg") {
			contentType = "image/jpeg"
		} else if strings.HasSuffix(filename, ".png") {
			contentType = "image/png"
		} else if strings.HasSuffix(filename, ".txt") {
			contentType = "text/plain"
		} else if strings.HasSuffix(filename, ".csv") {
			contentType = "text/csv"
		} else {
			contentType = "application/octet-stream"
		}
	}

	// Attachment oluştur
	attachment := notifications.EmailAttachment{
		Filename:    filename,
		ContentType: contentType,
		Size:        header.Size,
		Data:        data,
		CreatedAt:   time.Now(),
	}

	// Veritabanına kaydet
	query := `
		INSERT INTO email_attachments (filename, content_type, size, data, created_at)
		VALUES (?, ?, ?, ?, ?)
	`

	result, err := h.db.Exec(query,
		attachment.Filename,
		attachment.ContentType,
		attachment.Size,
		attachment.Data,
		attachment.CreatedAt,
	)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to save attachment"})
		return
	}

	id, err := result.LastInsertId()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to get attachment ID"})
		return
	}

	attachment.ID = int(id)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    attachment,
		"message": "Attachment uploaded successfully",
	})
}

// GetAttachment ek dosya getir
func (h *EmailAttachmentHandler) GetAttachment(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid attachment ID"})
		return
	}

	var attachment notifications.EmailAttachment
	query := `SELECT id, filename, content_type, size, created_at FROM email_attachments WHERE id = ?`

	err = h.db.QueryRow(query, id).Scan(
		&attachment.ID,
		&attachment.Filename,
		&attachment.ContentType,
		&attachment.Size,
		&attachment.CreatedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Attachment not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to get attachment"})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    attachment,
	})
}

// DownloadAttachment ek dosya indir
func (h *EmailAttachmentHandler) DownloadAttachment(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid attachment ID"})
		return
	}

	var attachment notifications.EmailAttachment
	query := `SELECT filename, content_type, size, data FROM email_attachments WHERE id = ?`

	err = h.db.QueryRow(query, id).Scan(
		&attachment.Filename,
		&attachment.ContentType,
		&attachment.Size,
		&attachment.Data,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Attachment not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to get attachment"})
		}
		return
	}

	// Header'ları ayarla
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", attachment.Filename))
	c.Header("Content-Type", attachment.ContentType)
	c.Header("Content-Length", fmt.Sprintf("%d", attachment.Size))

	// Dosyayı gönder
	c.Data(http.StatusOK, attachment.ContentType, attachment.Data)
}

// DeleteAttachment ek dosya sil
func (h *EmailAttachmentHandler) DeleteAttachment(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid attachment ID"})
		return
	}

	// Attachment var mı kontrol et
	var exists bool
	query := `SELECT EXISTS(SELECT 1 FROM email_attachments WHERE id = ?)`
	err = h.db.QueryRow(query, id).Scan(&exists)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to check attachment"})
		return
	}

	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Attachment not found"})
		return
	}

	// Sil
	query = `DELETE FROM email_attachments WHERE id = ?`
	_, err = h.db.Exec(query, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to delete attachment"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Attachment deleted successfully",
	})
}

// ListAttachments ek dosyaları listele
func (h *EmailAttachmentHandler) ListAttachments(c *gin.Context) {
	limit := 20
	offset := 0

	if limitStr := c.Query("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil {
			limit = l
		}
	}

	if offsetStr := c.Query("offset"); offsetStr != "" {
		if o, err := strconv.Atoi(offsetStr); err == nil {
			offset = o
		}
	}

	query := `
		SELECT id, filename, content_type, size, created_at
		FROM email_attachments
		ORDER BY created_at DESC
		LIMIT ? OFFSET ?
	`

	rows, err := h.db.Query(query, limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to get attachments"})
		return
	}
	defer rows.Close()

	var attachments []notifications.EmailAttachment
	for rows.Next() {
		var attachment notifications.EmailAttachment
		err := rows.Scan(
			&attachment.ID,
			&attachment.Filename,
			&attachment.ContentType,
			&attachment.Size,
			&attachment.CreatedAt,
		)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to scan attachment"})
			return
		}
		attachments = append(attachments, attachment)
	}

	// Toplam sayıyı al
	var total int
	countQuery := `SELECT COUNT(*) FROM email_attachments`
	err = h.db.QueryRow(countQuery).Scan(&total)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to count attachments"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"attachments": attachments,
			"total":       total,
			"limit":       limit,
			"offset":      offset,
		},
	})
}
