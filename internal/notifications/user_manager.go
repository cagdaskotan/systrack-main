package notifications

import (
	"database/sql"
	"fmt"
	"log"
	"strings"
	"time"
)

// UserManager kullanıcı yöneticisi
type UserManager struct {
	db *sql.DB
}

// NewUserManager yeni kullanıcı yöneticisi oluşturur
func NewUserManager(db *sql.DB) *UserManager {
	return &UserManager{db: db}
}

// UserNotificationSettings kullanıcı bildirim ayarları
type UserNotificationSettings struct {
	ID               int                   `json:"id"`
	UserID           int                   `json:"user_id"`
	TargetID         *int                  `json:"target_id"`
	Tags             []string              `json:"tags"`
	Channels         []NotificationChannel `json:"channels"`
	Events           []NotificationType    `json:"events"`
	Enabled          bool                  `json:"enabled"`
	CooldownMinutes  int                   `json:"cooldown_minutes"`
	QuietHoursStart  *time.Time            `json:"quiet_hours_start"`
	QuietHoursEnd    *time.Time            `json:"quiet_hours_end"`
	PreferredChannel NotificationChannel   `json:"preferred_channel"`
	Email            string                `json:"email"`
	TelegramChatID   string                `json:"telegram_chat_id"`
	CreatedAt        time.Time             `json:"created_at"`
	UpdatedAt        time.Time             `json:"updated_at"`
}

// GetUsersForTarget hedef için bildirim alacak kullanıcıları getirir
func (um *UserManager) GetUsersForTarget(targetID int, targetTags []string) ([]*UserNotificationSettings, error) {
	query := `
		SELECT DISTINCT
			uns.id, uns.user_id, uns.target_id, uns.tags, uns.channels, uns.events,
			uns.enabled, uns.cooldown_minutes, uns.quiet_hours_start, uns.quiet_hours_end,
			uns.preferred_channel, u.email, uns.telegram_chat_id,
			uns.created_at, uns.updated_at
		FROM user_notification_settings uns
		JOIN users u ON uns.user_id = u.id
		WHERE uns.enabled = true
		AND (
			uns.target_id = ? OR
			uns.target_id IS NULL OR
			JSON_OVERLAPS(uns.tags, ?)
		)
		AND JSON_CONTAINS(uns.events, ?)
	`

	// Target tags'ı JSON'a çevir
	targetTagsJSON := fmt.Sprintf(`["%s"]`, strings.Join(targetTags, `","`))
	targetStatusChangeJSON := `"target_status_change"`

	rows, err := um.db.Query(query, targetID, targetTagsJSON, targetStatusChangeJSON)
	if err != nil {
		return nil, fmt.Errorf("kullanıcılar sorgulanamadı: %w", err)
	}
	defer rows.Close()

	var users []*UserNotificationSettings
	for rows.Next() {
		user, err := um.scanUserSettings(rows)
		if err != nil {
			log.Printf("Kullanıcı scan edilemedi: %v", err)
			continue
		}

		// Sessiz saatler kontrolü
		if um.isInQuietHours(user) {
			continue
		}

		users = append(users, user)
	}

	return users, nil
}

// GetUsersForSLANotifications SLA bildirimleri için kullanıcıları getirir
func (um *UserManager) GetUsersForSLANotifications(targetTags []string) ([]*UserNotificationSettings, error) {
	query := `
		SELECT DISTINCT
			uns.id, uns.user_id, uns.target_id, uns.tags, uns.channels, uns.events,
			uns.enabled, uns.cooldown_minutes, uns.quiet_hours_start, uns.quiet_hours_end,
			uns.preferred_channel, u.email, uns.telegram_chat_id,
			uns.created_at, uns.updated_at
		FROM user_notification_settings uns
		JOIN users u ON uns.user_id = u.id
		WHERE uns.enabled = true
		AND (
			uns.target_id IS NULL OR
			JSON_OVERLAPS(uns.tags, ?)
		)
		AND JSON_CONTAINS(uns.events, ?)
	`

	// Target tags'ı JSON'a çevir
	targetTagsJSON := fmt.Sprintf(`["%s"]`, strings.Join(targetTags, `","`))
	slaViolationJSON := `"sla_violation"`

	rows, err := um.db.Query(query, targetTagsJSON, slaViolationJSON)
	if err != nil {
		return nil, fmt.Errorf("SLA kullanıcıları sorgulanamadı: %w", err)
	}
	defer rows.Close()

	var users []*UserNotificationSettings
	for rows.Next() {
		user, err := um.scanUserSettings(rows)
		if err != nil {
			log.Printf("SLA kullanıcı scan edilemedi: %v", err)
			continue
		}

		// Sessiz saatler kontrolü
		if um.isInQuietHours(user) {
			continue
		}

		users = append(users, user)
	}

	return users, nil
}

// GetUserSettings kullanıcı ayarlarını getirir
func (um *UserManager) GetUserSettings(userID int) ([]*UserNotificationSettings, error) {
	query := `
		SELECT id, user_id, target_id, tags, channels, events, enabled, cooldown_minutes, 
		       quiet_hours_start, quiet_hours_end, preferred_channel, email, telegram_chat_id,
		       created_at, updated_at
		FROM user_notification_settings
		WHERE user_id = ?
		ORDER BY target_id, created_at
	`

	rows, err := um.db.Query(query, userID)
	if err != nil {
		return nil, fmt.Errorf("kullanıcı ayarları sorgulanamadı: %w", err)
	}
	defer rows.Close()

	var settings []*UserNotificationSettings
	for rows.Next() {
		setting, err := um.scanUserSettings(rows)
		if err != nil {
			log.Printf("Kullanıcı ayarı scan edilemedi: %v", err)
			continue
		}

		settings = append(settings, setting)
	}

	return settings, nil
}

// scanUserSettings kullanıcı ayarlarını scan eder
func (um *UserManager) scanUserSettings(rows *sql.Rows) (*UserNotificationSettings, error) {
	var user UserNotificationSettings
	var tagsJSON, channelsJSON, eventsJSON string

	err := rows.Scan(
		&user.ID,
		&user.UserID,
		&user.TargetID,
		&tagsJSON,
		&channelsJSON,
		&eventsJSON,
		&user.Enabled,
		&user.CooldownMinutes,
		&user.QuietHoursStart,
		&user.QuietHoursEnd,
		&user.PreferredChannel,
		&user.Email,
		&user.TelegramChatID,
		&user.CreatedAt,
		&user.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	// JSON'ları parse et
	// TODO: JSON parsing implement et
	// Şimdilik basit parsing

	return &user, nil
}

// isInQuietHours sessiz saatlerde olup olmadığını kontrol eder
func (um *UserManager) isInQuietHours(user *UserNotificationSettings) bool {
	if user.QuietHoursStart == nil || user.QuietHoursEnd == nil {
		return false
	}

	now := time.Now()
	currentTime := time.Date(0, 1, 1, now.Hour(), now.Minute(), 0, 0, time.UTC)
	startTime := time.Date(0, 1, 1, user.QuietHoursStart.Hour(), user.QuietHoursStart.Minute(), 0, 0, time.UTC)
	endTime := time.Date(0, 1, 1, user.QuietHoursEnd.Hour(), user.QuietHoursEnd.Minute(), 0, 0, time.UTC)

	// Gece yarısını geçen durumları kontrol et
	if startTime.After(endTime) {
		return currentTime.After(startTime) || currentTime.Before(endTime)
	}

	return currentTime.After(startTime) && currentTime.Before(endTime)
}

// CreateDefaultUserSettings varsayılan kullanıcı ayarları oluşturur
func (um *UserManager) CreateDefaultUserSettings(userID int, email string) error {
	query := `
		INSERT INTO user_notification_settings (
			user_id, target_id, tags, channels, events, enabled, 
			cooldown_minutes, preferred_channel, email, created_at, updated_at
		) VALUES (?, NULL, '[]', '["email"]', '["target_status_change", "sla_violation"]', 
			true, 5, 'email', ?, NOW(), NOW())
		ON DUPLICATE KEY UPDATE
		email = VALUES(email),
		updated_at = NOW()
	`

	_, err := um.db.Exec(query, userID, email)
	if err != nil {
		return fmt.Errorf("varsayılan kullanıcı ayarları oluşturulamadı: %w", err)
	}

	return nil
}

// UpdateUserSettings kullanıcı ayarlarını günceller
func (um *UserManager) UpdateUserSettings(settings *UserNotificationSettings) error {
	// JSON'ları hazırla
	tagsJSON := fmt.Sprintf(`["%s"]`, strings.Join(settings.Tags, `","`))
	channelsJSON := fmt.Sprintf(`["%s"]`, strings.Join(convertChannelsToStrings(settings.Channels), `","`))
	eventsJSON := fmt.Sprintf(`["%s"]`, strings.Join(convertTypesToStrings(settings.Events), `","`))

	query := `
		INSERT INTO user_notification_settings (
			user_id, target_id, tags, channels, events, enabled,
			cooldown_minutes, quiet_hours_start, quiet_hours_end,
			preferred_channel, email, telegram_chat_id, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NOW(), NOW())
		ON DUPLICATE KEY UPDATE
		target_id = VALUES(target_id),
		tags = VALUES(tags),
		channels = VALUES(channels),
		events = VALUES(events),
		enabled = VALUES(enabled),
		cooldown_minutes = VALUES(cooldown_minutes),
		quiet_hours_start = VALUES(quiet_hours_start),
		quiet_hours_end = VALUES(quiet_hours_end),
		preferred_channel = VALUES(preferred_channel),
		email = VALUES(email),
		telegram_chat_id = VALUES(telegram_chat_id),
		updated_at = NOW()
	`

	_, err := um.db.Exec(query,
		settings.UserID,
		settings.TargetID,
		tagsJSON,
		channelsJSON,
		eventsJSON,
		settings.Enabled,
		settings.CooldownMinutes,
		settings.QuietHoursStart,
		settings.QuietHoursEnd,
		settings.PreferredChannel,
		settings.Email,
		settings.TelegramChatID,
	)

	if err != nil {
		return fmt.Errorf("kullanıcı ayarları güncellenemedi: %w", err)
	}

	return nil
}

// convertChannelsToStrings kanalları string slice'a çevirir
func convertChannelsToStrings(channels []NotificationChannel) []string {
	result := make([]string, len(channels))
	for i, channel := range channels {
		result[i] = string(channel)
	}
	return result
}

// convertTypesToStrings türleri string slice'a çevirir
func convertTypesToStrings(types []NotificationType) []string {
	result := make([]string, len(types))
	for i, notificationType := range types {
		result[i] = string(notificationType)
	}
	return result
}
