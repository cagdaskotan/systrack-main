# SysTrack Raspberry Pi 5 Kurulum Rehberi

Bu rehber, SysTrack uygulamasını Raspberry Pi 5 üzerinde kurmak için farklı yöntemleri açıklar.

## 🚀 Hızlı Kurulum (Otomatik Script)

### Gereksinimler
- Raspberry Pi 5 (ARM64)
- Raspberry Pi OS (Debian tabanlı)
- İnternet bağlantısı
- sudo yetkileri

### Kurulum Adımları

1. **Projeyi klonlayın:**
```bash
git clone https://github.com/your-repo/systrack.git
cd systrack
```

2. **Kurulum script'ini çalıştırın:**
```bash
chmod +x scripts/deploy.sh
./scripts/deploy.sh
```

3. **Kurulum tamamlandıktan sonra:**
- Web arayüzü: `http://RASPBERRY_PI_IP:8080`
- Admin email: `admin@systrack.local`
- Admin şifre: `CHANGE_ME_DB_PASS`

## 🐳 Docker ile Kurulum

### Gereksinimler
- Docker
- Docker Compose

### Kurulum Adımları

1. **Docker kurulumu (Raspberry Pi OS'da):**
```bash
curl -fsSL https://get.docker.com -o get-docker.sh
sudo sh get-docker.sh
sudo usermod -aG docker pi
```

2. **Docker Compose ile başlatın:**
```bash
docker-compose up -d
```

3. **Servis durumunu kontrol edin:**
```bash
docker-compose ps
```

4. **Logları görüntüleyin:**
```bash
docker-compose logs -f systrack
```

## 🤖 Ansible ile Kurulum

### Gereksinimler
- Ansible
- SSH erişimi (Raspberry Pi'ye)

### Kurulum Adımları

1. **Ansible kurulumu:**
```bash
sudo apt install ansible
```

2. **Inventory dosyası oluşturun:**
```ini
# ansible/inventory.ini
[raspberry_pi]
192.168.1.100 ansible_user=pi ansible_ssh_private_key_file=~/.ssh/id_rsa
```

3. **Playbook'u çalıştırın:**
```bash
ansible-playbook -i ansible/inventory.ini ansible/systrack-deploy.yml
```

## 🔧 Manuel Kurulum

### 1. Sistem Güncellemesi
```bash
sudo apt update && sudo apt upgrade -y
```

### 2. Gerekli Paketleri Yükleme
```bash
sudo apt install -y mariadb-server fping golang-go git curl wget unzip build-essential pkg-config
```

### 3. MariaDB Kurulumu
```bash
sudo systemctl start mariadb
sudo systemctl enable mariadb
sudo mysql_secure_installation
```

### 4. Veritabanı Oluşturma
```sql
CREATE DATABASE systrack CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
CREATE USER 'systrack'@'localhost' IDENTIFIED BY 'CHANGE_ME_DB_PASS';
GRANT ALL PRIVILEGES ON systrack.* TO 'systrack'@'localhost';
FLUSH PRIVILEGES;
```

### 5. Uygulama Derleme
```bash
go mod download
go build -o systrack ./cmd/systrack
```

### 6. Konfigürasyon
```bash
cp env.example .env
# .env dosyasını düzenleyin
```

### 7. Veritabanı Migrasyonları
```bash
go install -tags 'mysql' github.com/golang-migrate/migrate/v4/cmd/migrate@latest
~/go/bin/migrate -path internal/db/migrations -database "mysql://systrack:CHANGE_ME_DB_PASS@tcp(localhost:3306)/systrack" up
```

### 8. Admin Kullanıcısı Oluşturma
```bash
go run ./scripts/seed_admin.go
```

### 9. Systemd Servisi
```bash
sudo cp systemd/systrack.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable systrack
sudo systemctl start systrack
```

## 🔒 Güvenlik Ayarları

### Firewall Kurulumu
```bash
sudo ufw enable
sudo ufw allow ssh
sudo ufw allow 8080/tcp
```

### SSL/TLS (Opsiyonel)
Nginx reverse proxy ile SSL kurulumu:
```bash
sudo apt install nginx certbot python3-certbot-nginx
sudo certbot --nginx -d your-domain.com
```

## 📊 Monitoring ve Bakım

### Servis Durumu
```bash
sudo systemctl status systrack
```

### Logları Görüntüleme
```bash
sudo journalctl -u systrack -f
```

### Performans İzleme
```bash
htop
iostat -x 1
```

### Veritabanı Bakımı
```bash
mysql -u systrack -p systrack
SHOW PROCESSLIST;
SHOW TABLE STATUS;
```

## 🔄 Güncelleme

### Otomatik Güncelleme
```bash
git pull
make build
sudo systemctl restart systrack
```

### Docker Güncelleme
```bash
docker-compose pull
docker-compose up -d
```

## 🆘 Sorun Giderme

### Servis Başlamıyor
1. Logları kontrol edin: `sudo journalctl -u systrack -f`
2. Port çakışması: `sudo netstat -tlnp | grep 8080`
3. Veritabanı bağlantısı: `mysql -u systrack -p`

### Performans Sorunları
1. CPU kullanımı: `htop`
2. Bellek kullanımı: `free -h`
3. Disk kullanımı: `df -h`

### Ağ Sorunları
1. Ping testi: `fping -c 1 google.com`
2. Port erişimi: `telnet localhost 8080`

## 📞 Destek

Sorunlar için:
- GitHub Issues: [Repository Issues](https://github.com/your-repo/systrack/issues)
- Email: support@beyzsystem.com
- Dokümantasyon: [Wiki](https://github.com/your-repo/systrack/wiki)
