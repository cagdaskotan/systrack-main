package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	App       AppConfig
	Database  DatabaseConfig
	Auth      AuthConfig
	Ping      PingConfig
	Retention RetentionConfig
	Sensor    SensorConfig
}

type AppConfig struct {
	Bind string
	Port string
}

type DatabaseConfig struct {
	DSN string
}

type AuthConfig struct {
	AdminEmail    string
	AdminPassword string
	JWTSecret     string
}

type PingConfig struct {
	Engine           string
	BatchSize        int
	PacingMs         int
	TimeoutMs        int
	Retry            int
	BatchIntervalSec int
	FailOpen         int
	SuccessClose     int
}

type RetentionConfig struct {
	RawRetentionHours int
}

type SensorConfig struct {
	MQTTBrokerURL string
	MQTTUsername  string
	MQTTPassword  string
	ManagementURL string
}

func Load() (*Config, error) {
	// .env dosyasını yükle (varsa)
	if err := godotenv.Load(); err != nil {
		// .env dosyası yoksa hata verme, sadece logla
		fmt.Println("Warning: .env file not found, using environment variables")
	}

	config := &Config{
		App: AppConfig{
			Bind: getEnv("APP_BIND", "0.0.0.0"),
			Port: getEnv("APP_PORT", "8081"),
		},
		Database: DatabaseConfig{
			DSN: getEnv("DB_DSN", "root:CHANGE_ME_ROOT_PASS@tcp(localhost:3306)/systrack?parseTime=true&charset=utf8mb4&collation=utf8mb4_unicode_ci"),
		},
		Auth: AuthConfig{
			AdminEmail:    getEnv("ADMIN_EMAIL", "admin@systrack.local"),
			AdminPassword: getEnv("ADMIN_PASSWORD", "ChangeMeNow!"),
			JWTSecret:     getEnv("JWT_SECRET", "change_this_secret_key_for_production_use"),
		},
		Ping: PingConfig{
			Engine:           getEnv("PING_ENGINE", "auto"),
			BatchSize:        getEnvAsInt("PING_BATCH_SIZE", 256),
			PacingMs:         getEnvAsInt("PING_PACING_MS", 5),
			TimeoutMs:        getEnvAsInt("PING_TIMEOUT_MS", 5000),
			Retry:            getEnvAsInt("PING_RETRY", 0),
			BatchIntervalSec: getEnvAsInt("PING_BATCH_INTERVAL_SEC", 30),
			FailOpen:         getEnvAsInt("FAIL_OPEN", 3),
			SuccessClose:     getEnvAsInt("SUCCESS_CLOSE", 2),
		},
		Retention: RetentionConfig{
			RawRetentionHours: getEnvAsInt("RAW_RETENTION_HOURS", 48),
		},
		Sensor: SensorConfig{
			MQTTBrokerURL: getEnv("MQTT_BROKER_URL", ""),
			MQTTUsername:  getEnv("MQTT_USERNAME", ""),
			MQTTPassword:  getEnv("MQTT_PASSWORD", ""),
			ManagementURL: getEnv("MANAGEMENT_SERVER_URL", ""),
		},
	}

	// Kritik konfigürasyonları kontrol et
	if config.Auth.JWTSecret == "change_this_secret_key_for_production_use" {
		fmt.Println("Warning: Using default JWT secret. Change JWT_SECRET in production!")
	}

	return config, nil
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvAsInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

func (c *Config) GetServerAddr() string {
	return fmt.Sprintf("%s:%s", c.App.Bind, c.App.Port)
}

func (c *Config) GetPingInterval() time.Duration {
	return time.Duration(c.Ping.BatchIntervalSec) * time.Second
}

func (c *Config) GetPingTimeout() time.Duration {
	return time.Duration(c.Ping.TimeoutMs) * time.Millisecond
}
