#!/bin/bash
#
# SysTrack Network Watcher
# Container'dan gelen ag yapilandirma isteklerini isler
#

set -uo pipefail

WATCH_DIR="/home/systrack/network-pending"
REQUEST_FILE="${WATCH_DIR}/request.json"
RESULT_FILE="${WATCH_DIR}/result.json"
NETPLAN_FILE="/etc/netplan/99-systrack.yaml"
LOG_TAG="systrack-network-watcher"
STATE_DIR="/etc/systrack"
MANAGED_MARKER="${STATE_DIR}/network-managed"
AUTO_DEFAULT_IP="192.168.100.5"  # Factory default IP - DHCP yoksa bu IP ile acilir
AUTO_DEFAULT_PREFIX="24"
NETWORK_INTERFACE="eth0"

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

ensure_state_dir() {
    if [[ ! -d "$STATE_DIR" ]]; then
        mkdir -p "$STATE_DIR"
        chmod 755 "$STATE_DIR"
    fi
}

write_result() {
    local success="$1"
    local message="$2"
    local timestamp
    timestamp=$(date +%s)

    cat > "${RESULT_FILE}.tmp" <<EOF
{
  "success": ${success},
  "message": "${message}",
  "timestamp": ${timestamp}
}
EOF
    mv "${RESULT_FILE}.tmp" "${RESULT_FILE}"
    chown systrack:systrack "${RESULT_FILE}" 2>/dev/null || true
}

validate_ip() {
    local ip="$1"
    if [[ $ip =~ ^([0-9]{1,3}\.){3}[0-9]{1,3}$ ]]; then
        IFS='.' read -ra octets <<< "$ip"
        for octet in "${octets[@]}"; do
            if (( octet > 255 )); then
                return 1
            fi
        done
        return 0
    fi
    return 1
}

validate_prefix() {
    local prefix="$1"
    if [[ $prefix =~ ^[0-9]+$ ]] && (( prefix >= 1 && prefix <= 32 )); then
        return 0
    fi
    return 1
}

write_netplan_dhcp() {
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
}

is_loopback_dns() {
    local addr="$1"
    [[ "$addr" =~ ^127\. ]] && return 0
    return 1
}

# Gecersiz/loopback DNS adreslerini filtrele, bossa fallback kullan
# Sonuc: SANITIZED_DNS1 ve SANITIZED_DNS2 global degiskenlerine yazar
sanitize_dns() {
    local raw_dns1="${1:-}"
    local raw_dns2="${2:-}"

    SANITIZED_DNS1=""
    SANITIZED_DNS2=""

    # Loopback olmayan gecerli DNS'leri topla
    if [[ -n "$raw_dns1" ]] && ! is_loopback_dns "$raw_dns1"; then
        SANITIZED_DNS1="$raw_dns1"
    fi
    if [[ -n "$raw_dns2" ]] && ! is_loopback_dns "$raw_dns2"; then
        SANITIZED_DNS2="$raw_dns2"
    fi

    # Hicbir gecerli DNS yoksa fallback kullan
    if [[ -z "$SANITIZED_DNS1" ]] && [[ -z "$SANITIZED_DNS2" ]]; then
        log_warn "No valid DNS provided (loopback addresses filtered). Using fallback DNS: 8.8.8.8, 1.1.1.1"
        SANITIZED_DNS1="8.8.8.8"
        SANITIZED_DNS2="1.1.1.1"
    fi
}

write_netplan_static() {
    local ip="$1"
    local prefix="$2"
    local gateway="${3:-}"
    local raw_dns1="${4:-}"
    local raw_dns2="${5:-}"

    # DNS adreslerini dogrula/filtrele
    sanitize_dns "$raw_dns1" "$raw_dns2"
    local dns1="$SANITIZED_DNS1"
    local dns2="$SANITIZED_DNS2"

    # Hostname bilgisini al
    local hostname
    hostname=$(hostname 2>/dev/null || echo "systrack")

    cat > "${NETPLAN_FILE}.tmp" <<EOF
network:
  version: 2
  renderer: networkd
  ethernets:
    ${NETWORK_INTERFACE}:
      dhcp4: false
      addresses:
        - ${ip}/${prefix}
EOF

    if [[ -n "$gateway" ]]; then
        cat >> "${NETPLAN_FILE}.tmp" <<EOF
      routes:
        - to: default
          via: ${gateway}
EOF
    fi

    # Statik IP'de DNS her zaman yazilir (bos birakilamaz)
    echo "      nameservers:" >> "${NETPLAN_FILE}.tmp"
    echo "        addresses:" >> "${NETPLAN_FILE}.tmp"
    [[ -n "$dns1" ]] && echo "          - ${dns1}" >> "${NETPLAN_FILE}.tmp"
    [[ -n "$dns2" ]] && echo "          - ${dns2}" >> "${NETPLAN_FILE}.tmp"
}

apply_netplan() {
    mv "${NETPLAN_FILE}.tmp" "$NETPLAN_FILE"
    chmod 600 "$NETPLAN_FILE"

    log_info "Applying netplan configuration..."
    local output
    if output=$(netplan apply 2>&1); then
        log_info "Network configuration applied successfully"
        return 0
    else
        log_error "Netplan apply failed: $output"
        return 1
    fi
}

is_ip_taken() {
    local ip="$1"

    if ! command -v arping >/dev/null 2>&1; then
        log_warn "arping not installed; skipping IP conflict check"
        return 1
    fi

    if arping -D -c 2 -w 2 -I "${NETWORK_INTERFACE}" "$ip" >/dev/null 2>&1; then
        return 1
    fi

    return 0
}

# Gateway'e erisilebilir mi kontrol et
check_gateway_reachable() {
    local gateway="$1"

    if [[ -z "$gateway" ]]; then
        log_warn "No gateway specified for health check"
        return 1
    fi

    log_info "Checking gateway reachability: $gateway"

    # 3 deneme, toplam 5 saniye timeout
    local count=0
    local max_attempts=3

    while (( count < max_attempts )); do
        if ping -c 1 -W 2 "$gateway" >/dev/null 2>&1; then
            log_info "Gateway $gateway is reachable"
            return 0
        fi
        ((count++))
        sleep 1
    done

    log_warn "Gateway $gateway is NOT reachable after $max_attempts attempts"
    return 1
}

# Netplan dosyasindan gateway bilgisini oku
get_gateway_from_netplan() {
    if [[ ! -f "$NETPLAN_FILE" ]]; then
        echo ""
        return 1
    fi

    # routes: - to: default via: X.X.X.X satirini ara
    local gateway
    gateway=$(grep "via:" "$NETPLAN_FILE" | awk '{print $2}' | head -n 1)

    echo "$gateway"
}

# Netplan'da statik IP yapilandirmasi var mi?
is_static_mode_configured() {
    if [[ ! -f "$NETPLAN_FILE" ]]; then
        return 1
    fi

    # dhcp4: false satirini ara
    if grep -q "dhcp4: false" "$NETPLAN_FILE"; then
        return 0
    fi

    return 1
}

# DHCP'den IP alindi mi kontrol et (max timeout saniye bekler)
wait_for_dhcp_ip() {
    local timeout="${1:-30}"
    local count=0

    log_info "Waiting for DHCP IP (up to ${timeout}s)..."
    while (( count < timeout )); do
        local ip
        ip=$(ip addr show "${NETWORK_INTERFACE}" 2>/dev/null | grep "inet " | awk '{print $2}' | cut -d/ -f1 | grep -v "^169\.254\." | head -1)
        if [[ -n "$ip" ]]; then
            log_info "Got IP via DHCP: $ip"
            return 0
        fi
        sleep 1
        ((count++))
    done

    log_warn "No IP obtained from DHCP within ${timeout} seconds"
    return 1
}

# Gecici DHCP moduna gec (kullanici ayarlarini koruyarak)
apply_temporary_dhcp() {
    log_warn "Applying temporary DHCP due to gateway unreachable"

    # Gecici DHCP netplan yaz
    write_netplan_dhcp

    if apply_netplan; then
        log_info "Temporary DHCP applied successfully"
        # NOT: MANAGED_MARKER dosyasini SILME - kullanici ayarlarini koru
        # Kullanici daha sonra Web UI'dan statik IP'yi tekrar aktif edebilir
        return 0
    fi

    log_error "Failed to apply temporary DHCP"
    return 1
}

# Boot sirasinda IP sagligini kontrol et, gerekirse fallback uygula
check_and_fallback_if_needed() {
    log_info "Checking network IP health..."

    # DHCP modunda ve kullanici tarafindan yonetiliyorsa - IP alindi mi kontrol et
    if ! is_static_mode_configured; then
        if [[ -f "$MANAGED_MARKER" ]]; then
            if ! wait_for_dhcp_ip 30; then
                log_warn "DHCP mode but no IP obtained, applying factory default IP as last resort"
                write_netplan_static "$AUTO_DEFAULT_IP" "$AUTO_DEFAULT_PREFIX" "" "" ""
                apply_netplan || true
            fi
        fi
        log_info "No static IP configured, skipping gateway health check"
        return 0
    fi

    # Gateway bilgisini al
    local gateway
    gateway=$(get_gateway_from_netplan)

    if [[ -z "$gateway" ]]; then
        log_warn "Static IP configured but no gateway found, skipping health check"
        return 0
    fi

    # Gateway'e erisilebilir mi?
    if check_gateway_reachable "$gateway"; then
        log_info "Gateway health check passed, keeping static IP configuration"
        return 0
    fi

    # Gateway erisilemez - gecici DHCP'ye gec
    log_warn "Gateway unreachable, falling back to temporary DHCP"
    apply_temporary_dhcp || true

    # DHCP'den IP alamazsak son care olarak 192.168.100.5 uygula
    if ! wait_for_dhcp_ip 30; then
        log_warn "DHCP failed to obtain IP, applying factory default IP (${AUTO_DEFAULT_IP}) as last resort"
        write_netplan_static "$AUTO_DEFAULT_IP" "$AUTO_DEFAULT_PREFIX" "" "" ""
        apply_netplan || true
    fi

    return 0
}

apply_auto_default() {
    if [[ -z "$AUTO_DEFAULT_IP" ]]; then
        log_info "Auto IP disabled: keeping DHCP configuration"
        return 0
    fi

    if [[ -f "$MANAGED_MARKER" ]]; then
        log_info "Auto IP skipped: user-managed network settings detected"
        return 0
    fi

    if [[ -f "$REQUEST_FILE" ]]; then
        log_info "Auto IP skipped: pending request exists"
        return 0
    fi

    log_info "Trying DHCP first; will fall back to factory default IP if no IP obtained in 60s"

    write_netplan_dhcp
    if apply_netplan; then
        if ! wait_for_dhcp_ip 60; then
            log_warn "DHCP failed, falling back to factory default IP (${AUTO_DEFAULT_IP})"
            write_netplan_static "$AUTO_DEFAULT_IP" "$AUTO_DEFAULT_PREFIX" "" "" ""
            apply_netplan || true
        fi
    else
        log_error "Failed to apply DHCP configuration, falling back to factory default IP"
        write_netplan_static "$AUTO_DEFAULT_IP" "$AUTO_DEFAULT_PREFIX" "" "" ""
        apply_netplan || true
    fi
}

process_request() {
    log_info "Processing network configuration request"

    if [[ ! -f "$REQUEST_FILE" ]]; then
        log_error "Request file not found"
        write_result "false" "Request file not found"
        return 1
    fi

    local mode ip prefix gateway dns1 dns2
    mode=$(jq -r '.mode // "dhcp"' "$REQUEST_FILE" 2>/dev/null)

    if [[ "$mode" != "dhcp" && "$mode" != "static" ]]; then
        log_error "Invalid mode: $mode"
        write_result "false" "Invalid mode. Use dhcp or static."
        return 1
    fi

    if [[ "$mode" == "dhcp" ]]; then
        log_info "Configuring DHCP mode"
        write_netplan_dhcp
    else
        ip=$(jq -r '.ip // ""' "$REQUEST_FILE")
        prefix=$(jq -r '.prefix // ""' "$REQUEST_FILE")
        gateway=$(jq -r '.gateway // ""' "$REQUEST_FILE")
        dns1=$(jq -r '.dns[0] // ""' "$REQUEST_FILE")
        dns2=$(jq -r '.dns[1] // ""' "$REQUEST_FILE")

        log_info "Configuring static IP: $ip/$prefix"

        if ! validate_ip "$ip"; then
            log_error "Invalid IP address: $ip"
            write_result "false" "Invalid IP address"
            return 1
        fi

        if ! validate_prefix "$prefix"; then
            log_error "Invalid prefix: $prefix"
            write_result "false" "Invalid subnet prefix"
            return 1
        fi

        if [[ -n "$gateway" ]] && ! validate_ip "$gateway"; then
            log_error "Invalid gateway: $gateway"
            write_result "false" "Invalid gateway address"
            return 1
        fi

        write_netplan_static "$ip" "$prefix" "$gateway" "$dns1" "$dns2"
    fi

    if apply_netplan; then
        log_info "Network configuration applied successfully (mode: $mode)"
        touch "$MANAGED_MARKER"
        write_result "true" "Network configuration applied successfully"
        return 0
    fi

    write_result "false" "Netplan apply failed"
    return 1
}

main() {
    log_info "Starting SysTrack Network Watcher"

    ensure_state_dir

    # Dizin yoksa olustur
    if [[ ! -d "$WATCH_DIR" ]]; then
        mkdir -p "$WATCH_DIR"
        chown systrack:systrack "$WATCH_DIR"
    fi

    apply_auto_default || true

    # Gateway health check - statik IP varsa gateway kontrolu yap
    check_and_fallback_if_needed || true

    log_info "Watching directory: $WATCH_DIR"

    # inotifywait ile izle - monitor modunda surekli calisir
    inotifywait -m -e close_write -e moved_to "$WATCH_DIR" 2>/dev/null | while read -r directory event filename; do
        if [[ "$filename" == "request.json" ]]; then
            log_info "Detected change: $event $filename"
            sleep 0.3
            process_request || true
        fi
    done

    log_error "inotifywait exited unexpectedly"
}

main "$@"
