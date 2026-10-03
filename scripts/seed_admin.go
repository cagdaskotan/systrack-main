package main

import (
	"fmt"
	"log"

	"systrack/internal/auth"
	"systrack/internal/config"
	"systrack/internal/db"

	_ "github.com/go-sql-driver/mysql"
)

func main() {
	// Konfigürasyonu yükle
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Veritabanı bağlantısını kur
	database, err := db.NewConnection(cfg.Database.DSN)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer database.Close()

	// Admin kullanıcısının var olup olmadığını kontrol et
	exists, err := auth.UserExists(database, cfg.Auth.AdminEmail)
	if err != nil {
		log.Fatalf("Failed to check user existence: %v", err)
	}

	if exists {
		fmt.Printf("Admin user %s already exists. Skipping creation.\n", cfg.Auth.AdminEmail)
		return
	}

	// Admin kullanıcısını oluştur
	user, err := auth.CreateUser(
		database,
		cfg.Auth.AdminEmail,
		cfg.Auth.AdminPassword,
		"admin",
		0,   // maxTargets; 0 bırakınca fonksiyon admin için varsayılan 1000 değerini atıyor
		0,   // ipQueryLimit; 0 bırakınca yine rol bazlı varsayılan atanıyor
		nil, // licenseExpiry; şimdilik sınırsız
	)
	if err != nil {
		log.Fatalf("Failed to create admin user: %v", err)
	}

	fmt.Printf("Admin user created successfully!\n")
	fmt.Printf("Email: %s\n", user.Email)
	fmt.Printf("Role: %s\n", user.Role)
	fmt.Printf("Password: %s\n", cfg.Auth.AdminPassword)
	fmt.Printf("\nPlease change the default password after first login.\n")
}
