package ping

import (
	"database/sql"
	"log"
	"time"
)

const (
	// Momentary tablosundaki verilerin ne kadar süre tutulacağı
	MOMENTARY_RETENTION_MINUTES = 10
	// Archive tablosundaki verilerin ne kadar süre tutulacağı (3 ay = 90 gün)
	ARCHIVE_RETENTION_DAYS = 90
	// Cleanup job'unun çalışma sıklığı
	CLEANUP_INTERVAL_MINUTES = 5
)

// Cleaner manages periodic cleanup of momentary ping data
type Cleaner struct {
	db       *sql.DB
	running  bool
	stopChan chan struct{}
}

// NewCleaner creates a new ping cleaner
func NewCleaner(db *sql.DB) *Cleaner {
	return &Cleaner{
		db:       db,
		stopChan: make(chan struct{}),
	}
}

// Start begins the cleaner job
func (c *Cleaner) Start() error {
	if c.running {
		return nil
	}

	c.running = true
	log.Printf("🧹 Starting ping cleaner job (runs every %d minutes)", CLEANUP_INTERVAL_MINUTES)

	// Raporlama için günlük özet tablosunu hazırla ve geçmişi arka planda doldur.
	if err := EnsurePingDailyStats(c.db); err != nil {
		log.Printf("⚠️ ping_daily_stats hazırlanamadı: %v", err)
	}
	go c.backfill(ARCHIVE_RETENTION_DAYS)

	go c.run()
	return nil
}

// Stop stops the cleaner job
func (c *Cleaner) Stop() {
	if !c.running {
		return
	}

	log.Printf("🛑 Stopping ping cleaner job")
	c.running = false
	close(c.stopChan)
	log.Printf("✅ Ping cleaner job stopped")
}

// run is the main cleaner loop
func (c *Cleaner) run() {
	// İlk çalıştırmayı hemen yap
	c.cleanup()

	ticker := time.NewTicker(time.Duration(CLEANUP_INTERVAL_MINUTES) * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.cleanup()
		case <-c.stopChan:
			return
		}
	}
}

// cleanup removes old data from pings_raw_momentary and pings_raw
func (c *Cleaner) cleanup() {
	c.cleanupMomentary()
	c.cleanupArchive()
	// Rapor için günlük özetleri güncel tut (bugün + dün).
	c.rollupRecent()
}

// cleanupMomentary removes old data from pings_raw_momentary (older than 10 minutes)
func (c *Cleaner) cleanupMomentary() {
	start := time.Now()

	// 10 dakikadan eski kayıtları sil
	query := `DELETE FROM pings_raw_momentary
	          WHERE created_at < DATE_SUB(NOW(), INTERVAL ? MINUTE)`

	result, err := c.db.Exec(query, MOMENTARY_RETENTION_MINUTES)
	if err != nil {
		log.Printf("❌ Momentary cleanup failed: %v", err)
		return
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		log.Printf("⚠️ Could not get momentary rows affected: %v", err)
		return
	}

	duration := time.Since(start)

	if rowsAffected > 0 {
		log.Printf("🧹 Momentary cleanup completed: deleted %d old records from pings_raw_momentary (took %v)",
			rowsAffected, duration)
	}
}

// cleanupArchive removes old data from pings_raw (older than 90 days)
func (c *Cleaner) cleanupArchive() {
	start := time.Now()

	// 90 günden (3 ay) eski kayıtları sil
	query := `DELETE FROM pings_raw
	          WHERE created_at < DATE_SUB(NOW(), INTERVAL ? DAY)
	          LIMIT 10000`

	result, err := c.db.Exec(query, ARCHIVE_RETENTION_DAYS)
	if err != nil {
		log.Printf("❌ Archive cleanup failed: %v", err)
		return
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		log.Printf("⚠️ Could not get archive rows affected: %v", err)
		return
	}

	duration := time.Since(start)

	if rowsAffected > 0 {
		log.Printf("🗄️ Archive cleanup completed: deleted %d old records from pings_raw (took %v)",
			rowsAffected, duration)
	}
}

// CleanupNow forces an immediate cleanup (for testing or manual trigger)
func (c *Cleaner) CleanupNow() {
	log.Printf("🧹 Manual cleanup triggered")
	c.cleanup()
}
