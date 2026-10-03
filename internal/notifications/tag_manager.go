package notifications

import (
	"database/sql"
	"fmt"
	"log"
	"strings"
	"time"
)

// TagManager tag yöneticisi
type TagManager struct {
	db *sql.DB
}

// NewTagManager yeni tag yöneticisi oluşturur
func NewTagManager(db *sql.DB) *TagManager {
	return &TagManager{db: db}
}

// Tag tag yapısı
type Tag struct {
	ID          int       `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Color       string    `json:"color"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// GetTags tüm tag'leri getirir
func (tm *TagManager) GetTags() ([]*Tag, error) {
	query := `
		SELECT id, name, description, color, created_at, updated_at
		FROM tags
		ORDER BY name
	`

	rows, err := tm.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("tag'ler sorgulanamadı: %w", err)
	}
	defer rows.Close()

	var tags []*Tag
	for rows.Next() {
		var tag Tag

		err := rows.Scan(
			&tag.ID,
			&tag.Name,
			&tag.Description,
			&tag.Color,
			&tag.CreatedAt,
			&tag.UpdatedAt,
		)
		if err != nil {
			log.Printf("Tag scan edilemedi: %v", err)
			continue
		}

		tags = append(tags, &tag)
	}

	return tags, nil
}

// CreateTag yeni tag oluşturur
func (tm *TagManager) CreateTag(tag *Tag) error {
	query := `
		INSERT INTO tags (name, description, color, created_at, updated_at)
		VALUES (?, ?, ?, NOW(), NOW())
	`

	result, err := tm.db.Exec(query, tag.Name, tag.Description, tag.Color)
	if err != nil {
		return fmt.Errorf("tag oluşturulamadı: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("tag ID alınamadı: %w", err)
	}

	tag.ID = int(id)
	return nil
}

// UpdateTag tag'i günceller
func (tm *TagManager) UpdateTag(tag *Tag) error {
	query := `
		UPDATE tags 
		SET name = ?, description = ?, color = ?, updated_at = NOW()
		WHERE id = ?
	`

	_, err := tm.db.Exec(query, tag.Name, tag.Description, tag.Color, tag.ID)
	if err != nil {
		return fmt.Errorf("tag güncellenemedi: %w", err)
	}

	return nil
}

// DeleteTag tag'i siler
func (tm *TagManager) DeleteTag(id int) error {
	query := `DELETE FROM tags WHERE id = ?`

	_, err := tm.db.Exec(query, id)
	if err != nil {
		return fmt.Errorf("tag silinemedi: %w", err)
	}

	return nil
}

// GetTargetTags hedefin tag'lerini getirir
func (tm *TagManager) GetTargetTags(targetID int) ([]string, error) {
	query := `SELECT tags FROM targets WHERE id = ?`

	var tagsJSON string
	err := tm.db.QueryRow(query, targetID).Scan(&tagsJSON)
	if err != nil {
		return nil, fmt.Errorf("hedef tag'leri alınamadı: %w", err)
	}

	// Basit parsing (virgülle ayrılmış)
	if tagsJSON == "" {
		return []string{}, nil
	}

	tags := strings.Split(tagsJSON, ",")
	for i, tag := range tags {
		tags[i] = strings.TrimSpace(tag)
	}

	return tags, nil
}

// SetTargetTags hedefin tag'lerini ayarlar
func (tm *TagManager) SetTargetTags(targetID int, tags []string) error {
	tagsJSON := strings.Join(tags, ",")

	query := `UPDATE targets SET tags = ? WHERE id = ?`

	_, err := tm.db.Exec(query, tagsJSON, targetID)
	if err != nil {
		return fmt.Errorf("hedef tag'leri ayarlanamadı: %w", err)
	}

	return nil
}

// GetTargetsByTag tag'e göre hedefleri getirir
func (tm *TagManager) GetTargetsByTag(tag string) ([]int, error) {
	query := `SELECT id FROM targets WHERE FIND_IN_SET(?, tags) > 0`

	rows, err := tm.db.Query(query, tag)
	if err != nil {
		return nil, fmt.Errorf("tag'e göre hedefler sorgulanamadı: %w", err)
	}
	defer rows.Close()

	var targetIDs []int
	for rows.Next() {
		var targetID int

		err := rows.Scan(&targetID)
		if err != nil {
			log.Printf("Target ID scan edilemedi: %v", err)
			continue
		}

		targetIDs = append(targetIDs, targetID)
	}

	return targetIDs, nil
}

// CreateDefaultTags varsayılan tag'leri oluşturur
func (tm *TagManager) CreateDefaultTags() error {
	defaultTags := []Tag{
		{
			Name:        "production",
			Description: "Production ortamı",
			Color:       "#dc3545", // Kırmızı
		},
		{
			Name:        "staging",
			Description: "Staging ortamı",
			Color:       "#fd7e14", // Turuncu
		},
		{
			Name:        "development",
			Description: "Development ortamı",
			Color:       "#ffc107", // Sarı
		},
		{
			Name:        "critical",
			Description: "Kritik sistemler",
			Color:       "#6f42c1", // Mor
		},
		{
			Name:        "monitoring",
			Description: "Monitoring sistemleri",
			Color:       "#20c997", // Yeşil
		},
		{
			Name:        "database",
			Description: "Veritabanı sistemleri",
			Color:       "#17a2b8", // Mavi
		},
		{
			Name:        "web",
			Description: "Web servisleri",
			Color:       "#28a745", // Yeşil
		},
		{
			Name:        "api",
			Description: "API servisleri",
			Color:       "#6c757d", // Gri
		},
	}

	for _, tag := range defaultTags {
		query := `
			INSERT INTO tags (name, description, color, created_at, updated_at)
			VALUES (?, ?, ?, NOW(), NOW())
			ON DUPLICATE KEY UPDATE
			description = VALUES(description),
			color = VALUES(color),
			updated_at = NOW()
		`

		_, err := tm.db.Exec(query, tag.Name, tag.Description, tag.Color)
		if err != nil {
			log.Printf("Varsayılan tag oluşturulamadı %s: %v", tag.Name, err)
		}
	}

	return nil
}

// GetTagStats tag istatistiklerini getirir
func (tm *TagManager) GetTagStats() (map[string]int, error) {
	query := `
		SELECT t.name, COUNT(t2.id) as target_count
		FROM tags t
		LEFT JOIN targets t2 ON FIND_IN_SET(t.name, t2.tags) > 0
		GROUP BY t.id, t.name
		ORDER BY target_count DESC
	`

	rows, err := tm.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("tag istatistikleri sorgulanamadı: %w", err)
	}
	defer rows.Close()

	stats := make(map[string]int)
	for rows.Next() {
		var tagName string
		var targetCount int

		err := rows.Scan(&tagName, &targetCount)
		if err != nil {
			log.Printf("Tag istatistik scan edilemedi: %v", err)
			continue
		}

		stats[tagName] = targetCount
	}

	return stats, nil
}
