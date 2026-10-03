#!/bin/bash
#
# SysTrack Guncelleme Scripti
# Mevcut cihazlara yeni ozellikleri AG'A DOKUNMADAN ekler
#
# Kullanim: sudo ./update.sh
#

set -euo pipefail

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

log_info() { echo -e "${GREEN}[INFO]${NC} $1"; }
log_warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
log_error() { echo -e "${RED}[ERROR]${NC} $1"; }

if [[ $EUID -ne 0 ]]; then
    log_error "Bu script root olarak calistirilmali: sudo ./update.sh"
    exit 1
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

log_info "SysTrack guncelleme basliyor..."
log_info "NOT: Ag ayarlarina dokunulmayacak, mevcut IP korunacak"
echo ""

# 1. network-watcher.sh guncelle
log_info "[1/5] Network watcher guncelleniyor..."
cp "${SCRIPT_DIR}/host-services/network-watcher.sh" /opt/systrack/
chmod +x /opt/systrack/network-watcher.sh
systemctl restart systrack-network-watcher.service
log_info "Network watcher guncellendi ve yeniden baslatildi"

# 2. Button monitor kur (yeni ozellik)
log_info "[2/5] Button monitor kuruluyor..."
cp "${SCRIPT_DIR}/host-services/systrack-button-monitor.sh" /opt/systrack/
chmod +x /opt/systrack/systrack-button-monitor.sh
cp "${SCRIPT_DIR}/host-services/systrack-button-monitor.service" /etc/systemd/system/
systemctl daemon-reload
systemctl enable systrack-button-monitor.service
systemctl restart systrack-button-monitor.service

if systemctl is-active --quiet systrack-button-monitor.service; then
    log_info "Button monitor aktif"
else
    log_warn "Button monitor baslamadi (power butonu olmayabilir)"
fi

# 3. Cloud-init ag yonetimini devre disi birak
log_info "[3/5] Cloud-init ag yonetimi devre disi birakiliyor..."
mkdir -p /etc/cloud/cloud.cfg.d
echo "network: {config: disabled}" > /etc/cloud/cloud.cfg.d/99-disable-network-config.cfg
log_info "Cloud-init ag yonetimi devre disi birakildi (bir sonraki reboot'ta aktif)"

# 4. Docker guncelle (varsa)
log_info "[4/5] Docker container'lar guncelleniyor..."
if [[ -f "${SCRIPT_DIR}/docker-compose.yml" ]]; then
    cd "$SCRIPT_DIR"
    docker compose pull 2>/dev/null || true
    docker compose up -d --build
    log_info "Docker container'lar guncellendi"
else
    log_warn "docker-compose.yml bulunamadi, Docker atlandi"
fi

# 5. Static dosyalari guncelle
log_info "[5/5] Static dosyalar guncelleniyor..."
if [[ -d "${SCRIPT_DIR}/static" ]]; then
    log_info "Static dosyalar Docker volume uzerinden otomatik guncellendi"
fi

echo ""
log_info "=========================================="
log_info "Guncelleme tamamlandi!"
log_info "=========================================="
echo ""
log_info "Mevcut IP korundu: $(ip -4 addr show eth0 2>/dev/null | grep -oP '(?<=inet\s)\d+(\.\d+){3}' | head -1 || echo 'kontrol edin')"
log_info "Factory default IP: 192.168.100.5/24"
log_info "Ag sifirlama: Power butonuna 5 saniye icinde 3 kez basin"
echo ""
log_info "Servis durumlari:"
systemctl is-active systrack-network-watcher.service && log_info "  network-watcher: aktif" || log_warn "  network-watcher: aktif degil"
systemctl is-active systrack-button-monitor.service && log_info "  button-monitor:  aktif" || log_warn "  button-monitor:  aktif degil"
