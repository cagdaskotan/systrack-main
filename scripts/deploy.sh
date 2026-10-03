#!/bin/bash

# SysTrack Raspberry Pi 5 Otomatik Kurulum Script'i
# Beyz System - Network Monitoring Solution

set -e

# Renkli çıktı için
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Log fonksiyonu
log() {
    echo -e "${GREEN}[$(date +'%Y-%m-%d %H:%M:%S')] $1${NC}"
}

warn() {
    echo -e "${YELLOW}[WARNING] $1${NC}"
}

error() {
    echo -e "${RED}[ERROR] $1${NC}"
    exit 1
}

# Sistem bilgilerini kontrol et
check_system() {
    log "Sistem bilgileri kontrol ediliyor..."
    
    # Raspberry Pi kontrolü
    if ! grep -q "Raspberry Pi" /proc/cpuinfo; then
        warn "Bu script Raspberry Pi için optimize edilmiştir"
    fi
    
    # OS kontrolü
    if ! command -v apt &> /dev/null; then
        error "Bu script Debian/Ubuntu tabanlı sistemler için tasarlanmıştır"
    fi
    
    # Root kontrolü
    if [[ $EUID -eq 0 ]]; then
        error "Bu script root olarak çalıştırılmamalıdır. sudo kullanın."
    fi
    
    log "Sistem uyumlu ✓"
}

# Gerekli paketleri yükle
install_dependencies() {
    log "Gerekli paketler yükleniyor..."
    
    sudo apt update
    sudo apt install -y \
        mariadb-server \
        fping \
        golang-go \
        git \
        curl \
        wget \
        unzip \
        build-essential \
        pkg-config
    
    log "Paketler yüklendi ✓"
}

# MySQL/MariaDB kurulumu
setup_database() {
    log "Veritabanı kurulumu yapılıyor..."
    
    # MariaDB servisini başlat
    sudo systemctl start mariadb
    sudo systemctl enable mariadb
    
    # Güvenlik kurulumu
    sudo mysql_secure_installation <<EOF
n
y
CHANGE_ME_ROOT_PASS
CHANGE_ME_ROOT_PASS
y
y
y
y
EOF
    
    # Veritabanı ve kullanıcı oluştur
    sudo mysql -u root -pCHANGE_ME_ROOT_PASS <<EOF
CREATE DATABASE IF NOT EXISTS systrack CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
CREATE USER IF NOT EXISTS 'systrack'@'localhost' IDENTIFIED BY 'CHANGE_ME_DB_PASS';
GRANT ALL PRIVILEGES ON systrack.* TO 'systrack'@'localhost';
FLUSH PRIVILEGES;
EOF
    
    log "Veritabanı kurulumu tamamlandı ✓"
}

# Go kurulumu (gerekirse güncelle)
setup_go() {
    log "Go kurulumu kontrol ediliyor..."
    
    # Go versiyonunu kontrol et
    if command -v go &> /dev/null; then
        GO_VERSION=$(go version | cut -d' ' -f3 | sed 's/go//')
        log "Mevcut Go versiyonu: $GO_VERSION"
        
        # Go 1.21+ gerekli
        if [[ $(echo "$GO_VERSION >= 1.21" | bc -l) -eq 1 ]]; then
            log "Go versiyonu uygun ✓"
            return
        fi
    fi
    
    # Go'yu güncelle
    log "Go güncelleniyor..."
    wget https://go.dev/dl/go1.21.5.linux-arm64.tar.gz
    sudo rm -rf /usr/local/go
    sudo tar -C /usr/local -xzf go1.21.5.linux-arm64.tar.gz
    rm go1.21.5.linux-arm64.tar.gz
    
    # PATH'e ekle
    echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
    export PATH=$PATH:/usr/local/go/bin
    
    log "Go kurulumu tamamlandı ✓"
}

# SysTrack uygulamasını derle
build_application() {
    log "SysTrack uygulaması derleniyor..."
    
    # Proje dizinine git
    cd "$(dirname "$0")/.."
    
    # Go modülleri indir
    go mod download
    go mod tidy
    
    # Uygulamayı derle
    CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags="-w -s" -o systrack ./cmd/systrack
    
    log "Uygulama derlendi ✓"
}

# Konfigürasyon dosyasını oluştur
setup_config() {
    log "Konfigürasyon dosyası oluşturuluyor..."
    
    # .env dosyasını oluştur
    cat > .env <<EOF
# Database Configuration
DB_DSN=systrack:CHANGE_ME_DB_PASS@tcp(localhost:3306)/systrack?parseTime=true&charset=utf8mb4&collation=utf8mb4_unicode_ci

# Authentication
JWT_SECRET=$(openssl rand -base64 32)
ADMIN_EMAIL=admin@systrack.local
ADMIN_PASSWORD=CHANGE_ME_DB_PASS

# Server Configuration
BIND_ADDRESS=0.0.0.0
PORT=8080

# Monitoring Configuration
PING_ENGINE=auto
BATCH_SIZE=256
PACING_MS=5
TIMEOUT_MS=1000
RETRY=0
BATCH_INTERVAL_SEC=30
FAIL_OPEN=3
SUCCESS_CLOSE=2

# Data Retention
RAW_RETENTION_HOURS=48

# Logging
LOG_LEVEL=info
LOG_FORMAT=json
EOF
    
    log "Konfigürasyon dosyası oluşturuldu ✓"
}

# Veritabanı migrasyonlarını çalıştır
run_migrations() {
    log "Veritabanı migrasyonları çalıştırılıyor..."
    
    # migrate tool'unu yükle
    go install -tags 'mysql' github.com/golang-migrate/migrate/v4/cmd/migrate@latest
    
    # Migrasyonları çalıştır
    ~/go/bin/migrate -path internal/db/migrations -database "mysql://systrack:CHANGE_ME_DB_PASS@tcp(localhost:3306)/systrack" up
    
    log "Migrasyonlar tamamlandı ✓"
}

# Admin kullanıcısını oluştur
create_admin_user() {
    log "Admin kullanıcısı oluşturuluyor..."
    
    go run ./scripts/seed_admin.go
    
    log "Admin kullanıcısı oluşturuldu ✓"
}

# Systemd servisini kur
setup_systemd_service() {
    log "Systemd servisi kuruluyor..."
    
    # Servis dosyasını kopyala
    sudo cp systemd/systrack.service /etc/systemd/system/
    
    # Servis dosyasını güncelle
    sudo sed -i "s|/opt/systrack|$(pwd)|g" /etc/systemd/system/systrack.service
    
    # Servisi etkinleştir
    sudo systemctl daemon-reload
    sudo systemctl enable systrack
    
    log "Systemd servisi kuruldu ✓"
}

# Firewall kurallarını ayarla
setup_firewall() {
    log "Firewall kuralları ayarlanıyor..."
    
    # UFW kurulumu
    sudo apt install -y ufw
    
    # Temel kurallar
    sudo ufw default deny incoming
    sudo ufw default allow outgoing
    sudo ufw allow ssh
    sudo ufw allow 8080/tcp
    
    # Firewall'u etkinleştir
    sudo ufw --force enable
    
    log "Firewall kuralları ayarlandı ✓"
}

# Servisi başlat
start_service() {
    log "SysTrack servisi başlatılıyor..."
    
    sudo systemctl start systrack
    
    # Servis durumunu kontrol et
    sleep 3
    if sudo systemctl is-active --quiet systrack; then
        log "SysTrack servisi başarıyla başlatıldı ✓"
    else
        error "SysTrack servisi başlatılamadı!"
    fi
}

# Kurulum sonrası bilgileri göster
show_post_install_info() {
    log "Kurulum tamamlandı! 🎉"
    
    echo ""
    echo -e "${BLUE}========================================${NC}"
    echo -e "${BLUE}        SysTrack Kurulum Bilgileri${NC}"
    echo -e "${BLUE}========================================${NC}"
    echo ""
    echo -e "${GREEN}Web Arayüzü:${NC} http://$(hostname -I | awk '{print $1}'):8080"
    echo -e "${GREEN}Admin Email:${NC} admin@systrack.local"
    echo -e "${GREEN}Admin Şifre:${NC} CHANGE_ME_DB_PASS"
    echo ""
    echo -e "${YELLOW}Önemli Notlar:${NC}"
    echo "• İlk girişten sonra admin şifresini değiştirin"
    echo "• Servis durumu: sudo systemctl status systrack"
    echo "• Logları görüntüle: sudo journalctl -u systrack -f"
    echo "• Servisi yeniden başlat: sudo systemctl restart systrack"
    echo ""
    echo -e "${BLUE}========================================${NC}"
}

# Ana kurulum fonksiyonu
main() {
    echo -e "${BLUE}"
    echo "=========================================="
    echo "    SysTrack Raspberry Pi 5 Kurulumu"
    echo "         Beyz System - 2024"
    echo "=========================================="
    echo -e "${NC}"
    
    check_system
    install_dependencies
    setup_database
    setup_go
    build_application
    setup_config
    run_migrations
    create_admin_user
    setup_systemd_service
    setup_firewall
    start_service
    show_post_install_info
}

# Script'i çalıştır
main "$@"
