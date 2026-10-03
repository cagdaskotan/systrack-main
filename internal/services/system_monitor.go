package services

import (
	"log"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/host"
	"github.com/shirou/gopsutil/v3/mem"
)

// SystemStats sunucu sistem istatistiklerini tutar
type SystemStats struct {
	CPU         float64 `json:"cpu"`         // CPU kullanım yüzdesi
	RAM         RAMInfo `json:"ram"`         // RAM bilgileri
	Disk        DiskInfo `json:"disk"`        // Disk bilgileri
	Temperature float64 `json:"temperature"` // Cihaz sıcaklığı (°C)
}

// RAMInfo RAM kullanım bilgilerini tutar
type RAMInfo struct {
	UsedGB   float64 `json:"usedGb"`   // Kullanılan RAM (GB)
	TotalGB  float64 `json:"totalGb"`  // Toplam RAM (GB)
	Percent  float64 `json:"percent"`  // Kullanım yüzdesi
}

// DiskInfo disk kullanım bilgilerini tutar
type DiskInfo struct {
	UsedGB  float64 `json:"usedGb"`  // Kullanılan disk (GB)
	TotalGB float64 `json:"totalGb"` // Toplam disk (GB)
	Percent float64 `json:"percent"` // Kullanım yüzdesi
}

// SystemMonitor sistem kaynaklarını izler ve broadcast eder
type SystemMonitor struct {
	broadcastFunc func(SystemStats)
	stopChan      chan struct{}
}

// NewSystemMonitor yeni bir sistem monitörü oluşturur
func NewSystemMonitor(broadcastFunc func(SystemStats)) *SystemMonitor {
	return &SystemMonitor{
		broadcastFunc: broadcastFunc,
		stopChan:      make(chan struct{}),
	}
}

// Start sistem izlemeyi başlatır (5 saniyede bir güncelleme)
func (sm *SystemMonitor) Start() {
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		// İlk okuma hemen yapılsın
		sm.collectAndBroadcast()

		for {
			select {
			case <-ticker.C:
				sm.collectAndBroadcast()
			case <-sm.stopChan:
				log.Println("System monitor stopped")
				return
			}
		}
	}()
	log.Println("System monitor started (5 second interval)")
}

// Stop sistem izlemeyi durdurur
func (sm *SystemMonitor) Stop() {
	close(sm.stopChan)
}

// collectAndBroadcast sistem bilgilerini toplar ve broadcast eder
func (sm *SystemMonitor) collectAndBroadcast() {
	stats, err := sm.collectStats()
	if err != nil {
		log.Printf("Failed to collect system stats: %v", err)
		return
	}

	sm.broadcastFunc(stats)
}

// collectStats sistem istatistiklerini toplar
func (sm *SystemMonitor) collectStats() (SystemStats, error) {
	var stats SystemStats

	// CPU kullanımını al (1 saniye boyunca ölç)
	cpuPercent, err := cpu.Percent(time.Second, false)
	if err != nil {
		log.Printf("Failed to get CPU stats: %v", err)
		stats.CPU = 0
	} else if len(cpuPercent) > 0 {
		stats.CPU = cpuPercent[0]
	}

	// RAM bilgilerini al
	memInfo, err := mem.VirtualMemory()
	if err != nil {
		log.Printf("Failed to get RAM stats: %v", err)
		stats.RAM = RAMInfo{}
	} else {
		stats.RAM = RAMInfo{
			UsedGB:  float64(memInfo.Used) / (1024 * 1024 * 1024),
			TotalGB: float64(memInfo.Total) / (1024 * 1024 * 1024),
			Percent: memInfo.UsedPercent,
		}
	}

	// Disk bilgilerini al (root partition)
	// Windows'ta C:\, Linux'ta /
	diskPath := "/"
	diskInfo, err := disk.Usage(diskPath)
	if err != nil {
		// Windows için C:\ dene
		diskPath = "C:\\"
		diskInfo, err = disk.Usage(diskPath)
		if err != nil {
			log.Printf("Failed to get disk stats: %v", err)
			stats.Disk = DiskInfo{}
			return stats, nil
		}
	}

	stats.Disk = DiskInfo{
		UsedGB:  float64(diskInfo.Used) / (1024 * 1024 * 1024),
		TotalGB: float64(diskInfo.Total) / (1024 * 1024 * 1024),
		Percent: diskInfo.UsedPercent,
	}

	// Sıcaklık bilgisini al (platform-bağımsız)
	temperature, err := sm.getTemperature()
	if err != nil {
		log.Printf("❄️  Failed to get temperature: %v", err)
		stats.Temperature = 0
	} else {
		log.Printf("🌡️  Temperature read: %.1f°C", temperature)
		stats.Temperature = temperature
	}

	return stats, nil
}

// getTemperature cihaz sıcaklığını okur (platform-bağımsız)
func (sm *SystemMonitor) getTemperature() (float64, error) {
	switch runtime.GOOS {
	case "linux":
		// Linux (Raspberry Pi) için /sys/class/thermal/thermal_zone0/temp
		return sm.getLinuxTemperature()
	case "windows":
		// Windows için gopsutil'den sıcaklık bilgisi
		return sm.getWindowsTemperature()
	default:
		// Desteklenmeyen platform
		return 0, nil
	}
}

// getLinuxTemperature Linux sistemlerde sıcaklık okur
func (sm *SystemMonitor) getLinuxTemperature() (float64, error) {
	// /sys/class/thermal/thermal_zone0/temp dosyasından oku
	data, err := os.ReadFile("/sys/class/thermal/thermal_zone0/temp")
	if err != nil {
		return 0, err
	}

	// String'i temizle ve float'a çevir
	tempStr := strings.TrimSpace(string(data))
	tempMilliC, err := strconv.ParseFloat(tempStr, 64)
	if err != nil {
		return 0, err
	}

	// Milidereceden dereceye çevir (45200 -> 45.2°C)
	return tempMilliC / 1000.0, nil
}

// getWindowsTemperature Windows sistemlerde sıcaklık okur
func (sm *SystemMonitor) getWindowsTemperature() (float64, error) {
	// gopsutil/host kütüphanesinden sensör bilgilerini al
	sensors, err := host.SensorsTemperatures()
	if err != nil || len(sensors) == 0 {
		// Sensör bilgisi yoksa 0 döndür (hata değil)
		return 0, nil
	}

	// İlk bulduğumuz sıcaklık değerini döndür
	// Genellikle CPU veya sistem sıcaklığı olur
	for _, sensor := range sensors {
		if sensor.Temperature > 0 {
			return sensor.Temperature, nil
		}
	}

	return 0, nil
}
