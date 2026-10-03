package notifications

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"
)

// CooldownManager cooldown yöneticisi
type CooldownManager struct {
	cooldowns map[string]time.Time
	mu        sync.RWMutex
	ctx       context.Context
	cancel    context.CancelFunc
}

// NewCooldownManager yeni cooldown yöneticisi oluşturur
func NewCooldownManager() *CooldownManager {
	ctx, cancel := context.WithCancel(context.Background())

	return &CooldownManager{
		cooldowns: make(map[string]time.Time),
		ctx:       ctx,
		cancel:    cancel,
	}
}

// Start cooldown manager'ı başlatır
func (cm *CooldownManager) Start(ctx context.Context) {
	log.Println("⏰ Cooldown manager başlatılıyor...")

	// Periyodik olarak eski cooldown'ları temizle
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			cm.cleanupExpiredCooldowns()
		case <-ctx.Done():
			log.Println("⏰ Cooldown manager durduruldu")
			return
		}
	}
}

// IsInCooldown cooldown'da olup olmadığını kontrol eder
func (cm *CooldownManager) IsInCooldown(userID int, targetID int, notificationType NotificationType) bool {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	key := cm.generateKey(userID, targetID, notificationType)
	lastTriggered, exists := cm.cooldowns[key]

	if !exists {
		return false
	}

	// Cooldown süresi (varsayılan 5 dakika)
	cooldownDuration := 5 * time.Minute

	return time.Since(lastTriggered) < cooldownDuration
}

// UpdateCooldown cooldown'u günceller
func (cm *CooldownManager) UpdateCooldown(userID int, targetID int, notificationType NotificationType) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	key := cm.generateKey(userID, targetID, notificationType)
	cm.cooldowns[key] = time.Now()
}

// SetCooldownDuration cooldown süresini ayarlar
func (cm *CooldownManager) SetCooldownDuration(userID int, targetID int, notificationType NotificationType, duration time.Duration) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	key := cm.generateKey(userID, targetID, notificationType)
	cm.cooldowns[key] = time.Now().Add(-duration) // Süreyi geçmişe ayarla
}

// GetCooldownRemaining kalan cooldown süresini getirir
func (cm *CooldownManager) GetCooldownRemaining(userID int, targetID int, notificationType NotificationType) time.Duration {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	key := cm.generateKey(userID, targetID, notificationType)
	lastTriggered, exists := cm.cooldowns[key]

	if !exists {
		return 0
	}

	cooldownDuration := 5 * time.Minute
	remaining := cooldownDuration - time.Since(lastTriggered)

	if remaining < 0 {
		return 0
	}

	return remaining
}

// generateKey cooldown anahtarı oluşturur
func (cm *CooldownManager) generateKey(userID int, targetID int, notificationType NotificationType) string {
	return fmt.Sprintf("%d_%d_%s", userID, targetID, notificationType)
}

// cleanupExpiredCooldowns süresi dolmuş cooldown'ları temizler
func (cm *CooldownManager) cleanupExpiredCooldowns() {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	now := time.Now()
	cooldownDuration := 5 * time.Minute

	for key, lastTriggered := range cm.cooldowns {
		if now.Sub(lastTriggered) > cooldownDuration {
			delete(cm.cooldowns, key)
		}
	}

	log.Printf("🧹 %d cooldown temizlendi", len(cm.cooldowns))
}

// GetCooldownStats cooldown istatistiklerini getirir
func (cm *CooldownManager) GetCooldownStats() map[string]interface{} {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	return map[string]interface{}{
		"active_cooldowns": len(cm.cooldowns),
		"cooldowns":        cm.cooldowns,
	}
}

// ClearCooldown cooldown'u temizler
func (cm *CooldownManager) ClearCooldown(userID int, targetID int, notificationType NotificationType) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	key := cm.generateKey(userID, targetID, notificationType)
	delete(cm.cooldowns, key)
}

// ClearAllCooldowns tüm cooldown'ları temizler
func (cm *CooldownManager) ClearAllCooldowns() {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	cm.cooldowns = make(map[string]time.Time)
	log.Println("🧹 Tüm cooldown'lar temizlendi")
}
