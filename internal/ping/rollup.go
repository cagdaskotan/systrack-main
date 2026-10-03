package ping

import (
	"database/sql"
	"fmt"
	"log"
	"time"
)

// Günlük özet (rollup) tablosu, raporlamanın milyonlarca ham ping satırını
// her seferinde taramasını önler. pings_raw ~90 gün, ~1000 hedef ve hedef
// başına 2 dakikada bir ping = milyonlarca satır. Rapor bunun yerine gün
// bazında önceden hesaplanmış ~ (hedef sayısı × gün sayısı) satır okur.
//
// Bir günün özeti bir kez hesaplanır (geçmiş günler değişmez); bugünün özeti
// cleaner her çalıştığında (5 dk) yenilenir.

// EnsurePingDailyStats tabloyu oluşturur (idempotent).
func EnsurePingDailyStats(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS ping_daily_stats (
			target_id   INT      NOT NULL,
			day         DATE     NOT NULL,
			total       INT      NOT NULL DEFAULT 0,
			successful  INT      NOT NULL DEFAULT 0,
			failed      INT      NOT NULL DEFAULT 0,
			sum_rtt_ms  BIGINT   NOT NULL DEFAULT 0,
			rtt_count   INT      NOT NULL DEFAULT 0,
			min_rtt_ms  INT      DEFAULT NULL,
			max_rtt_ms  INT      DEFAULT NULL,
			incidents   INT      NOT NULL DEFAULT 0,
			downtime_ms BIGINT   NOT NULL DEFAULT 0,
			updated_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
			PRIMARY KEY (target_id, day),
			KEY idx_pds_day (day)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
	`)
	return err
}

// dayBoundsMs verilen günün yerel gece yarısı [start, next) sınırlarını
// epoch-millis olarak döndürür.
func dayBoundsMs(day time.Time) (int64, int64) {
	start := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, day.Location())
	next := start.AddDate(0, 0, 1)
	return start.UnixMilli(), next.UnixMilli()
}

// RollupPingDay tek bir günün özetini (tüm hedefler için) hesaplar ve upsert eder.
// Pencere fonksiyonları yalnızca o güne ait satırlar üzerinde çalışır; bu iş
// rapor yolunun DIŞINDA, arka planda ve günde bir kez yapıldığı için maliyeti
// önemsizdir.
func RollupPingDay(db *sql.DB, day time.Time) error {
	startMs, endMs := dayBoundsMs(day)
	dayStr := day.Format("2006-01-02")

	_, err := db.Exec(`
		INSERT INTO ping_daily_stats
			(target_id, day, total, successful, failed, sum_rtt_ms, rtt_count, min_rtt_ms, max_rtt_ms, incidents, downtime_ms)
		SELECT
			target_id,
			?,
			COUNT(*),
			SUM(CASE WHEN ok = 1 THEN 1 ELSE 0 END),
			SUM(CASE WHEN ok = 0 THEN 1 ELSE 0 END),
			SUM(CASE WHEN ok = 1 AND rtt_ms > 0 THEN rtt_ms ELSE 0 END),
			SUM(CASE WHEN ok = 1 AND rtt_ms > 0 THEN 1 ELSE 0 END),
			MIN(CASE WHEN ok = 1 AND rtt_ms > 0 THEN rtt_ms ELSE NULL END),
			MAX(CASE WHEN ok = 1 AND rtt_ms > 0 THEN rtt_ms ELSE NULL END),
			SUM(CASE WHEN ok = 0 AND (prev_ok IS NULL OR prev_ok = 1) THEN 1 ELSE 0 END),
			SUM(CASE WHEN ok = 0 AND next_ts IS NOT NULL THEN next_ts - ts_ms ELSE 0 END)
		FROM (
			SELECT
				target_id, ok, rtt_ms, ts_ms,
				LAG(ok)     OVER (PARTITION BY target_id ORDER BY ts_ms) AS prev_ok,
				LEAD(ts_ms) OVER (PARTITION BY target_id ORDER BY ts_ms) AS next_ts
			FROM pings_raw
			WHERE ts_ms >= ? AND ts_ms < ?
		) AS w
		GROUP BY target_id
		ON DUPLICATE KEY UPDATE
			total       = VALUES(total),
			successful  = VALUES(successful),
			failed      = VALUES(failed),
			sum_rtt_ms  = VALUES(sum_rtt_ms),
			rtt_count   = VALUES(rtt_count),
			min_rtt_ms  = VALUES(min_rtt_ms),
			max_rtt_ms  = VALUES(max_rtt_ms),
			incidents   = VALUES(incidents),
			downtime_ms = VALUES(downtime_ms)
	`, dayStr, startMs, endMs)
	return err
}

// dayHasRollup o gün için en az bir özet satırı olup olmadığını söyler.
func dayHasRollup(db *sql.DB, day time.Time) bool {
	var exists bool
	err := db.QueryRow(`SELECT EXISTS(SELECT 1 FROM ping_daily_stats WHERE day = ?)`, day.Format("2006-01-02")).Scan(&exists)
	return err == nil && exists
}

// EnsureRollupForRange rapor için gereken tarih aralığındaki eksik günleri
// hesaplar. Geçmiş günler değişmez; bir kez hesaplanınca tekrar dokunulmaz.
// Bugün her zaman yeniden hesaplanır (veri hâlâ akıyor). Rapor bu fonksiyonu
// çağırdığı için, arka plan görevi henüz doldurmamış olsa bile rapor doğru
// veriyle gelir (ilk sefer yavaş, sonrası anlık).
func EnsureRollupForRange(db *sql.DB, start, end time.Time) error {
	if err := EnsurePingDailyStats(db); err != nil {
		return err
	}

	today := time.Now()
	todayStr := today.Format("2006-01-02")

	d := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, start.Location())
	last := time.Date(end.Year(), end.Month(), end.Day(), 0, 0, 0, 0, end.Location())

	for !d.After(last) {
		isToday := d.Format("2006-01-02") == todayStr
		if isToday || !dayHasRollup(db, d) {
			if err := RollupPingDay(db, d); err != nil {
				return fmt.Errorf("rollup %s: %w", d.Format("2006-01-02"), err)
			}
		}
		d = d.AddDate(0, 0, 1)
	}
	return nil
}

// rollupRecent bugünü ve dünü yeniden hesaplar (gece yarısı geçişini ve geç
// gelen kayıtları yakalamak için). Cleaner her çalıştığında çağrılır.
func (c *Cleaner) rollupRecent() {
	if err := EnsurePingDailyStats(c.db); err != nil {
		log.Printf("❌ ping_daily_stats oluşturulamadı: %v", err)
		return
	}
	now := time.Now()
	for _, day := range []time.Time{now.AddDate(0, 0, -1), now} {
		if err := RollupPingDay(c.db, day); err != nil {
			log.Printf("❌ Günlük rollup başarısız (%s): %v", day.Format("2006-01-02"), err)
		}
	}
}

// backfill açılışta son N günün eksik özetlerini arka planda doldurur.
// Pi'yi boğmamak için günler arasında kısa bekleme koyar; sadece eksik
// (ve bugün olmayan) günleri hesaplar.
func (c *Cleaner) backfill(days int) {
	if err := EnsurePingDailyStats(c.db); err != nil {
		log.Printf("❌ ping_daily_stats oluşturulamadı (backfill): %v", err)
		return
	}
	log.Printf("🗂️ Ping günlük özet backfill başladı (son %d gün)", days)
	now := time.Now()
	filled := 0
	for i := 1; i <= days; i++ {
		day := now.AddDate(0, 0, -i)
		if dayHasRollup(c.db, day) {
			continue
		}
		if err := RollupPingDay(c.db, day); err != nil {
			log.Printf("⚠️ Backfill rollup başarısız (%s): %v", day.Format("2006-01-02"), err)
			continue
		}
		filled++
		time.Sleep(500 * time.Millisecond) // Pi'yi yormamak için
	}
	log.Printf("✅ Ping günlük özet backfill tamamlandı (%d gün dolduruldu)", filled)
}
