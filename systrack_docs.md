# SYSTRACK - Ağ İzleme Sistemi

## Genel Bakış

SysTrack, kurumsal ağ altyapılarını 7/24 izlemek ve yönetmek için tasarlanmış tak-çalıştır bir ağ izleme cihazıdır. Ağınıza bağladığınızda tüm cihazlarınızı, servislerinizi ve sistemlerinizi merkezi bir web arayüzünden anlık olarak izlemenizi sağlar.

### Temel Konsept

SysTrack fiziksel bir cihaz olarak çalışır ve ağınıza bağlandığında:
- İzlenen cihazlara yazılım yüklemeye gerek kalmaz
- Tüm izleme işlemleri merkezi yapılır
- Web tarayıcısından erişim sağlanır
- Gerçek zamanlı durum bilgisi alınır

---

## Özellikler

### Cihaz İzleme

**ICMP/Ping İzleme**
- Ağdaki cihazların erişilebilirliğini kontrol eder
- Yanıt sürelerini ölçer
- Kesinti durumlarını tespit eder

**HTTP/HTTPS İzleme**
- Web servislerinin durumunu kontrol eder
- Sayfa yanıt sürelerini ölçer
- HTTP durum kodlarını doğrular

**SSL Sertifika Takibi**
- Sertifika son kullanma tarihlerini izler

### Servis İzleme

**Windows Sistemler**
- Servislerin durumunu okur
- Hangi servislerin çalıştığını gösterir
- Servis değişikliklerini takip eder

**Linux Sistemler**
- Systemd servislerini listeler
- Servis durumlarını izler
- Değişiklikleri kaydeder

**Özellikler**
- Otomatik servis keşfi
- Kritik servisleri işaretleme
- Durum değişikliği geçmişi

### IP Scanner

Ağınızdaki tüm cihazları otomatik keşfeder:
- Subnet taraması
- IP aralığı taraması
- MAC adres tespiti
- Cihaz üreticisi tanıma
- SNMP test desteği
- Bulunan cihazları sisteme ekleme

### SSH Terminal

Web tarayıcısından SSH bağlantısı:
- Cihazlara uzaktan erişim
- Terminal komutları çalıştırma

### Sunucu Performansı

Sunucularınızın performans verilerini izler:

**Windows ve Linux için:**
- CPU kullanımı
- Bellek kullanımı
- Disk kullanımı
- Ağ trafiği

**Görselleştirme:**
- Gerçek zamanlı grafikler
- Geçmiş verilerle karşılaştırma
- Eşik değer uyarıları

### Bildirim Sistemi

**Bildirim Kanalları:**
- E-posta
- Telegram

**Bildirim Türleri:**
- Cihaz çevrimiçi/çevrimdışı durumu
- Servis durum değişiklikleri
- SSL sertifikası uyarıları
- Performans eşik aşımları

### Raporlama ve Analitik

**Performans Analizi:**
- Yanıt süresi trendleri
- Performans dağılımları
- Cihaz karşılaştırmaları
- Zaman bazlı analizler

**İstatistikler:**
- Toplam uptime
- Kesinti süreleri
- Ortalama yanıt süreleri
- Olay sayıları

### Kullanıcı Yönetimi

**Rol Bazlı Erişim**

**Güvenlik:**
- Şifreli oturum yönetimi
- Modül bazlı izinler

### Web Arayüzü

**Özellikler:**
- Modern ve kullanıcı dostu tasarım
- Karanlık ve aydınlık tema
- Türkçe, İngilizce, Almanca dil desteği
- Mobil uyumlu tasarım
- Gerçek zamanlı güncellemeler

**Sayfalar:**
- Ana kontrol paneli
- Cihaz listesi ve yönetimi
- IP tarayıcı
- SSH terminal
- Sunucu Durum Takibi
- Raporlar
- Bildirim ayarları
- Kullanıcı yönetimi

---

## Kullanım Senaryoları

### Kurumsal Ağ İzleme

Merkez ofis ve şubelerdeki tüm cihazları tek bir noktadan izleyin:
- Sunucular
- Ağ cihazları (router, switch, firewall)
- Web servisleri
- Kritik uygulamalar

### Data Center

Barındırdığınız sunucuları ve sistemleri izleyin:
- Yeni cihaz keşfi
- SNMP ile cihaz bilgileri
- Performans takibi
- Hızlı müdahale

---

### Kurulum

**Hızlı Kurulum:**
Cihaz ağa bağlanır, otomatik kurulum scripti çalıştırılır ve dakikalar içinde kullanıma hazır hale gelir.

### Veritabanı

Tüm izleme verileri, ayarlar ve kullanıcı bilgileri yerel veritabanında saklanır:
- İzleme geçmişi
- Servis durumları
- Kullanıcı ayarları
- Bildirim kuralları
- Performans metrikleri

### Bağlantı Protokolleri

**İzleme:**
- ICMP (Ping)
- HTTP/HTTPS
- SNMP v1/v2c/v3

**Servis İzleme:**
- WinRM (Windows)
- SSH (Linux)

**Bildirimler:**
- E-posta
- Telegram

### Güvenlik

**Kimlik Doğrulama:**
- Güvenli şifre saklama
- Oturum yönetimi

**Yetkilendirme:**
- Rol bazlı erişim kontrolü
- Modül bazlı izinler
- Kullanıcı izinleri

**Veri Güvenliği:**
- Kimlik bilgilerinin şifrelenmesi
- Güvenli veritabanı bağlantısı
- HTTPS desteği

---

## Avantajlar

### Kolay Kullanım

- Grafik arayüzle yönetim
- Hızlı kurulum
- Anlaşılır raporlar
- Basit konfigürasyon

- Düşük enerji tüketimi
- Agent gerektirmez
- Tek cihazdan tüm ağı izleme

### Kapsamlı İzleme

- Cihaz erişilebilirliği
- Servis durumları
- SSL sertifikaları

### Hızlı Müdahale

- Anlık bildirimler
- Gerçek zamanlı görüntüleme
- Web tabanlı SSH erişimi

### Esneklik

- Özelleştirilebilir bildirimler
- Esnek raporlama
- Farklı protokol desteği
- Çoklu kullanıcı

---

## Kullanıcı Deneyimi

### İlk Kurulum

1. Cihaz ağa bağlanır
2. Kurulum scripti çalıştırılır
3. Web arayüzüne erişilir
4. İlk hedefler eklenir
5. Bildirimler yapılandırılır
6. İzleme başlar

### Günlük Kullanım

**Ana Kontrol Paneli:**
Sisteme giriş yapıldığında tüm cihazların durumu bir bakışta görülür. Çevrimiçi/çevrimdışı sayıları, aktif uyarılar ve son durumlar listelenir.

**Cihaz Yönetimi:**
Yeni cihaz eklemek için form doldurulur. İzleme tipi, adres ve kontrol sıklığı belirlenir. Cihazlar etiketlerle gruplandırılarak organize edilir.

**IP Tarayıcı:**
Ağdaki cihazları bulmak için subnet taranır. Bulunan cihazlar listelenir ve istenirse izlemeye eklenir.

**Servis İzleme:**
Windows veya Linux sunucuların kimlik bilgileri girilir. Servisler otomatik keşfedilir ve listeye eklenir. Kritik servisler işaretlenir.

**SSH Terminal:**
Sunuculara hızlı erişim için tarayıcıdan terminal açılır. Komutlar çalıştırılır, loglar incelenir.

**Bildirimler:**
E-posta ve Telegram ayarları yapılandırılır. Hangi durumlarda bildirim gönderileceği belirlenir. Bildirim kuralları oluşturulur.

**Raporlar:**
Belirli bir zaman aralığı için raporlar oluşturulur. SLA performansı, uptime yüzdeleri ve kesinti detayları görüntülenir.

### Uyarı Senaryosu

Örnek bir kesinti senaryosu:

1. Bir sunucu çevrimdışı olur
2. SysTrack bunu tespit eder
3. Kontrol panelinde durum kırmızı gösterilir
4. Telegram ve e-posta ile bildirim gönderilir
5. Sorun giderilir ise ve sunucu çevrimiçi olur
6. SysTrack yeşile döner ve bildirim gönderir
7. Kesinti süresi ve detayları kaydedilir

---


## Destek ve Dokümantasyon

**Kurulum Rehberi:**
Detaylı kurulum adımları ve sistem gereksinimleri

**Kullanım Kılavuzu:**
Her özellik için adım adım açıklamalar
YouTube kanalındaki öğretici videolar

**API Dokümantasyonu:**
Özel entegrasyonlar için API referansı

**Sorun Giderme:**
Yaygın sorunlar ve çözümleri

**İletişim:**
Teknik destek ve satış bilgileri

---

## Özet

SysTrack, ağ altyapınızı izlemek için ihtiyacınız olan her şeyi sağlayan kapsamlı bir çözümdür. Kolay kurulum, kullanıcı dostu arayüz ve güçlü özellikleri ile ağınızın kesintisiz çalışmasını garanti eder.

Tak-çalıştır mantığı ile dakikalar içinde devreye alınır ve anında izlemeye başlar. Agent gerektirmediği için mevcut sistemlerinizde hiçbir değişiklik yapmadan kullanabilirsiniz.

Gerçek zamanlı bildirimler sayesinde sorunları anında tespit eder ve hızlı müdahale etmenizi sağlar. Detaylı raporlar ve analizler ile ağınızın performansını sürekli takip edebilirsiniz.