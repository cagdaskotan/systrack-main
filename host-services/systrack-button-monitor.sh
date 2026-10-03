#!/bin/bash
#
# SysTrack Button Monitor
# Power butonuna 5 saniye icinde 3 kez basilirsa ag ayarlarini sifirlar ve reboot yapar.
# ARM64 (64-bit) input_event yapisi 24 byte'tir.
#

set -uo pipefail

BUTTON_DEVICE="/dev/input/event0"
NETPLAN_FILE="/etc/netplan/99-systrack.yaml"
MANAGED_MARKER="/etc/systrack/network-managed"
FALLBACK_IP="192.168.100.5"
FALLBACK_PREFIX="24"
NETWORK_INTERFACE="eth0"
PRESS_COUNT=3
TIME_WINDOW=5
PRESS_TIMES_FILE="/tmp/systrack_button_presses"
LOG_TAG="systrack-button-monitor"

log_info() {
    echo "[INFO] $1"
    logger -t "$LOG_TAG" "[INFO] $1"
}

log_warn() {
    echo "[WARN] $1"
    logger -t "$LOG_TAG" "[WARN] $1"
}

log_error() {
    echo "[ERROR] $1"
    logger -t "$LOG_TAG" "[ERROR] $1"
}

reset_network() {
    log_info "=== AG SIFIRLAMA BASLADI ==="
    log_info "Ag ayarlari DHCP moduna sifirlaniyor"

    # Marker silinince network-watcher reboot sonrasinda DHCP-first akisini tekrar uygular.
    rm -f "$MANAGED_MARKER"
    log_info "Managed marker silindi"

    # Once DHCP'ye don. DHCP basarisiz olursa network-watcher boot sirasinda
    # 192.168.100.5/24 fallback'ini son care olarak uygulayacak.
    mkdir -p "$(dirname "$NETPLAN_FILE")"
    cat > "${NETPLAN_FILE}.tmp" <<EOF
network:
  version: 2
  renderer: networkd
  ethernets:
    ${NETWORK_INTERFACE}:
      dhcp4: true
      dhcp-identifier: mac
      dhcp4-overrides:
        send-hostname: true
EOF
    mv "${NETPLAN_FILE}.tmp" "$NETPLAN_FILE"
    chmod 600 "$NETPLAN_FILE"
    log_info "DHCP netplan yazildi: $NETPLAN_FILE"

    if netplan apply 2>&1 | logger -t "$LOG_TAG"; then
        log_info "DHCP netplan uygulandi"
    else
        log_error "Netplan apply basarisiz, reboot yapiliyor"
    fi

    log_info "Sistem 3 saniye icinde yeniden baslatiliyor..."
    sleep 3
    reboot
}

count_valid_presses() {
    local now="$1"
    local count=0
    local tmpfile
    tmpfile=$(mktemp)

    if [[ -f "$PRESS_TIMES_FILE" ]]; then
        while IFS= read -r t; do
            if [[ -n "$t" ]] && (( now - t <= TIME_WINDOW )); then
                echo "$t" >> "$tmpfile"
                ((count++)) || true
            fi
        done < "$PRESS_TIMES_FILE"
    fi

    mv "$tmpfile" "$PRESS_TIMES_FILE"
    echo "$count"
}

monitor_button() {
    > "$PRESS_TIMES_FILE"

    log_info "Power butonu izleniyor: $BUTTON_DEVICE"
    log_info "${PRESS_COUNT} basin / ${TIME_WINDOW} saniye = ag sifirlama + reboot"

    if [[ ! -c "$BUTTON_DEVICE" ]]; then
        log_error "Buton cihazi bulunamadi: $BUTTON_DEVICE"
        exit 1
    fi

    while true; do
        hex=$(dd if="$BUTTON_DEVICE" bs=24 count=1 2>/dev/null | od -An -v -tx1 | tr -d ' \n')

        [[ ${#hex} -ne 48 ]] && continue

        type_hex="${hex:32:4}"
        code_hex="${hex:36:4}"
        value_hex="${hex:40:8}"

        if [[ "$type_hex" == "0100" && "$code_hex" == "7400" && "$value_hex" == "01000000" ]]; then
            now=$(date +%s)
            echo "$now" >> "$PRESS_TIMES_FILE"

            count=$(count_valid_presses "$now")
            log_info "Buton basisi: ${count}/${PRESS_COUNT} (son ${TIME_WINDOW}s icinde)"

            if (( count >= PRESS_COUNT )); then
                > "$PRESS_TIMES_FILE"
                reset_network
            fi
        fi
    done
}

monitor_button
