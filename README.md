# SysTrack — Fuar Sunum Deposu

Bu depo, **SysTrack** akıllı izleme cihazının fuar tanıtım videosu için hazırlanmıştır.
İçinde cihaz üzerinde çalışan ana uygulamanın (`systrack-main`) kaynak kodu, arayüz
ekran görüntüleri, ürün fotoğrafları ve videonun senaryo brief'i bulunur.

> **Video senaryosu için:** [`VIDEO-BRIEF.md`](VIDEO-BRIEF.md)

---

## SysTrack nedir?

Müşterinin yerel ağına Ethernet ile bağlanan, Raspberry Pi 5 tabanlı bir **izleme
cihazıdır**. Ubuntu Server üzerinde Docker ile çalışır. Ağdaki sunucuları,
bilgisayarları, ağ cihazlarını, servisleri, yazılımları ve **fiziksel ortam
koşullarını** tek bir panelden takip eder.

Kullanıcı cihaza kendi ağından `http://<CIHAZ_IP>:8080` adresiyle erişir.

### Ana yetenekler

| Modül | Ne yapar |
|---|---|
| **Ortam İzleme** | Hava sensörü (sıcaklık, nem, basınç, hava kalitesi) ve sıvı temas sensörü ile fiziksel ortamı anlık izler; kamera arşivine erişir |
| **Hedef İzleme** | ICMP / TCP / HTTP(S) ile erişilebilirlik ve yanıt süresi takibi, SLA hesabı |
| **IP Tarayıcı** | Ağdaki cihazları ARP / NetBIOS / mDNS / LLMNR ile keşfeder, MAC üreticisini çözer |
| **Envanter** | Active Directory, SNMP, WinRM ve SSH ile cihaz ve donanım envanterini otomatik toplar |
| **Yazılım & Lisans Uyumluluğu** | Kurulu yazılımları listeler, lisans durumunu ve şüpheli yazılımları tespit eder |
| **Servis İzleme** | Uzak sunuculardaki Windows/Linux servislerinin durum değişimlerini izler |
| **Bildirimler** | Kullanıcı tanımlı kurallara göre e-posta, Telegram ve webhook ile anlık uyarı |
| **Raporlama** | CSV / HTML / PDF rapor üretimi |

---

## Ortam İzleme — videonun ana konusu

SysTrack yalnızca ağ ve envanter için değil, cihazın bulunduğu **fiziksel alanın**
takibi için de kullanılır. Üç harici bileşen desteklenir:

### Hava Sensörü
Sunucu odası, sistem kabini veya takip edilmesi gereken herhangi bir alana
yerleştirilir. Ölçtükleri:

- **Sıcaklık** (°C)
- **Nem** (%)
- **Atmosfer basıncı** (hPa)
- **Hava kalitesi / gaz**

Değerler cihaz arayüzünde anlık gösterilir, ayrıca **son 24 saat saat-saat**
grafiklenir (cihazın kendi CPU sıcaklığı da aynı grafikte izlenebilir).

### Sıvı Temas Sensörü
Kablolu algılama ucu sayesinde su kaçağı veya sıvı temasını tespit eder.
Sunucu kabini altı, klima altı, zemin seviyesi gibi risk bölgelerinde erken uyarı
sağlar. Her temas olayı **Sıvı Temas Geçmişi** olarak kayıt altına alınır.

### Kamera
Fiziksel alanın görüntülü izlenmesini destekler. Çektiği kareler
**Kamera Arşivi** sekmesinden görüntülenir; bildirim kuralı tetiklendiğinde
ilgili kare uyarı mesajına **fotoğraf olarak** eklenebilir.

### Nasıl çalışır
Sensörler merkezi SysTrack yönetim sunucusunda MQTT üzerinden keşfedilir ve
cihazın seri numarasıyla eşleştirilir. Kullanıcı cihaz arayüzündeki **Ortam İzleme**
sayfasından sensörün seri numarasını girip bağlar. Cihaz, bağlı sensörlerinin
canlı verisini sunucudan **SSE akışı** ile alır ve arayüze WebSocket üzerinden
anlık yansıtır — sayfa yenilemeye gerek yoktur.

Kritik eşikler için bildirim kuralı tanımlanır: *"sıcaklık 30 °C üstüne çıkarsa",
"nem %70'i geçerse", "sıvı teması algılanırsa"* → e-posta veya Telegram ile
anında uyarı gider.

---

## Depo yapısı

```
├── VIDEO-BRIEF.md        ← Video senaryosu, anlatım tonu, sahne planı
├── fuar-medya/
│   ├── arayuz/           ← Gerçek arayüz ekran görüntüleri
│   ├── urun/             ← Cihaz ve sensör fotoğrafları
│   └── marka/            ← Logo ve marka görselleri
├── cmd/systrack/         ← Uygulama giriş noktası
├── internal/
│   ├── services/         ← Sensör (MQTT/SSE), metrik toplayıcı, keşif servisleri
│   ├── notifications/    ← Bildirim kural motoru ve kanallar
│   ├── handlers/         ← HTTP API uçları
│   ├── ping/             ← Erişilebilirlik izleme motoru
│   ├── license/          ← Cihaz aktivasyon ve lisans yönetimi
│   └── network/scanner/  ← Ağ keşif tarayıcısı
├── static/admin/         ← Web arayüzü (TR / EN / DE)
└── migrations/           ← Veritabanı şema geçişleri
```

---

## Önemli not — bu depo tanıtım amaçlıdır

Bu depodaki yapılandırma dosyalarında **gerçek parola, API anahtarı veya parola
özeti bulunmaz**; hepsi `CHANGE_ME_...` ve `REPLACE_WITH_...` gibi yer tutucularla
değiştirilmiştir. Depo bu haliyle **üretim kurulumu için kullanılamaz**; gerçek
kurulum, ekibin kendi yapılandırma dosyalarıyla yapılır.

---

<p align="center"><strong>HER ŞEY KONTROL ALTINDA</strong></p>
