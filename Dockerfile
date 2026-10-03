# Multi-stage build for Raspberry Pi 5 (ARM64)
FROM golang:1.24-alpine AS builder

# Gerekli paketleri yükle
RUN apk add --no-cache \
    git \
    ca-certificates \
    tzdata \
    iputils \
    nmap-nping \
    libcap

# Çalışma dizinini ayarla
WORKDIR /app

# Kaynak kodları kopyala
COPY . .

# Bağımlılıkları indir ve derle
RUN go mod tidy && CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build \
    -ldflags="-w -s" \
    -o systrack ./cmd/systrack

# Final stage
FROM alpine:3.18

# Gerekli paketleri yükle (network tarama + SSH tunnel + DB backup için)
RUN apk add --no-cache \
    openssh-client \
    sshpass \
    fping \
    iputils \
    nmap-nping \
    ca-certificates \
    tzdata \
    libcap \
    mariadb-client

# Çalışma kullanıcısı oluştur
RUN adduser -D -s /bin/sh systrack

# Çalışma dizinini ayarla
WORKDIR /app

# Binary'yi kopyala
COPY --from=builder /app/systrack .

# Static dosyaları kopyala
COPY --from=builder /app/static ./static

# OUI database'i kopyala (MAC vendor lookup için)
COPY --from=builder /app/data ./data

# Log dizini oluştur
RUN mkdir -p /app/logs && chown -R systrack:systrack /app

# Binary'ye network capabilities ver (root olmadan ping/ARP için)
RUN setcap cap_net_raw,cap_net_admin=+eip /app/systrack
RUN setcap cap_net_raw=+ep /usr/sbin/fping || true
# arping binary'ye de capability ver
RUN setcap cap_net_raw=+ep /usr/sbin/arping

# Kullanıcıyı değiştir (artık non-root ama network yetkili)
USER systrack

# Port'u expose et
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/healthz || exit 1

# Uygulamayı başlat
CMD ["./systrack"]
