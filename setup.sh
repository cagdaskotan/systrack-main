#!/bin/bash
#
# SysTrack Kurulum Scripti
# Seri uretimde her cihazda 1 kez calistirilir
#
# Kullanim: sudo ./setup.sh
#

set -euo pipefail

# Renkli output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

log_info() { echo -e "${GREEN}[INFO]${NC} $1"; }
log_warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
log_error() { echo -e "${RED}[ERROR]${NC} $1"; }

# Root kontrolu
if [[ $EUID -ne 0 ]]; then
    log_error "Bu script root olarak calistirilmali: sudo ./setup.sh"
    exit 1
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SYSTRACK_USER="systrack"
DEVICE_HOSTNAME="systrack"
DEVICE_PRETTY_NAME="SysTrack"

log_info "SysTrack kurulumu basliyor..."

# 1. Gerekli paketleri kur
log_info "Gerekli paketler kuruluyor..."
apt-get update -qq
apt-get install -y -qq inotify-tools jq iputils-arping > /dev/null
log_info "Paketler kuruldu: inotify-tools, jq, iputils-arping"

# 2a. Cloud-init ag yonetimini devre disi birak (50-cloud-init.yaml cakismasini onle)
log_info "Cloud-init ag yonetimi devre disi birakiliyor..."
mkdir -p /etc/cloud/cloud.cfg.d
echo "network: {config: disabled}" > /etc/cloud/cloud.cfg.d/99-disable-network-config.cfg
log_info "Cloud-init ag yonetimi devre disi birakildi"

# 2. /opt/systrack dizinini olustur
log_info "Dizinler olusturuluyor..."
mkdir -p /opt/systrack
mkdir -p /home/${SYSTRACK_USER}/network-pending

# 3. network-watcher.sh kopyala
log_info "Network watcher kuruluyor..."
cp "${SCRIPT_DIR}/host-services/network-watcher.sh" /opt/systrack/
chmod +x /opt/systrack/network-watcher.sh

# 4. Systemd service'leri kopyala
cp "${SCRIPT_DIR}/host-services/systrack-network-watcher.service" /etc/systemd/system/

# Button monitor kopyala
log_info "Button monitor kuruluyor..."
cp "${SCRIPT_DIR}/host-services/systrack-button-monitor.sh" /opt/systrack/
chmod +x /opt/systrack/systrack-button-monitor.sh
cp "${SCRIPT_DIR}/host-services/systrack-button-monitor.service" /etc/systemd/system/

# 5. Dizin izinleri
chown -R ${SYSTRACK_USER}:${SYSTRACK_USER} /home/${SYSTRACK_USER}/network-pending
chmod 755 /home/${SYSTRACK_USER}/network-pending

# 6. Systemd reload ve servisleri baslat
log_info "Servisler aktif ediliyor..."
systemctl daemon-reload

systemctl enable systrack-network-watcher.service
systemctl start systrack-network-watcher.service

systemctl enable systrack-button-monitor.service
systemctl start systrack-button-monitor.service

# 7. Servis durumlarini kontrol et
if systemctl is-active --quiet systrack-network-watcher.service; then
    log_info "Network watcher servisi basariyla basladi"
else
    log_error "Network watcher baslatilamadi! Kontrol edin: journalctl -u systrack-network-watcher"
    exit 1
fi

if systemctl is-active --quiet systrack-button-monitor.service; then
    log_info "Button monitor servisi basariyla basladi"
else
    log_warn "Button monitor baslatilamadi (power butonu olmayabilir): journalctl -u systrack-button-monitor"
fi

# 8. Docker compose up (opsiyonel)
if [[ -f "${SCRIPT_DIR}/docker-compose.yml" ]]; then
    log_info "Docker containerlar baslatiliyor..."
    cd "$SCRIPT_DIR"
    docker compose up -d
    log_info "Docker containerlar baslatildi"
fi

# 9. Hostname ayari (cihazlarin agda "systrack" olarak gorunmesi icin)
log_info "Hostname ayarlaniyor..."
hostnamectl set-hostname "${DEVICE_HOSTNAME}"
hostnamectl set-hostname --pretty "${DEVICE_PRETTY_NAME}" || true
if grep -q "^127.0.1.1" /etc/hosts; then
    sed -i "s/^127.0.1.1.*/127.0.1.1\t${DEVICE_HOSTNAME}/" /etc/hosts
else
    echo -e "127.0.1.1\t${DEVICE_HOSTNAME}" >> /etc/hosts
fi

echo ""
log_info "=========================================="
log_info "SysTrack kurulumu tamamlandi!"
log_info "=========================================="
echo ""
log_info "Network watcher: systemctl status systrack-network-watcher"
log_info "Button monitor: systemctl status systrack-button-monitor"
log_info "Loglar: journalctl -u systrack-network-watcher -f"
log_info ""
log_info "Factory default IP: 192.168.100.5/24"
log_info "Ag sifirlama: Power butonuna 5 saniye icinde 3 kez basin"
echo ""
