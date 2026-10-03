# SysTrack Fuar Videosu — Yapım Brief'i

**Süre:** 90 saniye · **Format:** 16:9 (1920×1080) · **Dil:** Türkçe
**Gösterim:** Fuar standı, **sesi kapalı**, döngüde

---

## 0. Bu video nasıl yapılacak — en önemli bölüm

Bu bir stok görüntü videosu **değil**. Bu, SysTrack arayüzünün **gerçek
kullanım kaydı gibi** görünen bir ürün demosu.

Yöntem: **arayüzü HTML/CSS ile yeniden inşa et ve içinde gezin.** Ekranda
gördüğümüz her şey — sol menü, kartlar, tablolar, grafikler, modal
pencereler — gerçek uygulamanın birebir kopyası olmalı. Üzerinde **bir fare
imleci** dolaşmalı, butonlara tıklamalı, sayfalar geçmeli, veriler akmalı.

Bunu uydurmana gerek yok: **uygulamanın gerçek arayüz kodu bu depoda.**

| Dosya | Ne verir |
|---|---|
| `static/admin/index.html` | Sol menü, üst bar, dashboard yerleşimi, **Tailwind config** |
| `static/admin/sensors.html` | Ortam İzleme sayfasının tüm bileşenleri |
| `static/admin/ip_scanner.html` | IP tarayıcı ekranı |
| `static/admin/inventory.html` | Envanter yönetimi ekranı |
| `static/admin/css/custom.css` | Özel stiller |
| `fuar-medya/arayuz/*.png` | Gerçek ekran görüntüleri — doğrulama için |

### Arayüzün teknik kimliği

- **Tailwind CSS**, `darkMode: 'class'` — video **koyu temada** çekilecek
- **primary** renk ölçeği mavi: `500 #3b82f6`, `600 #2563eb`, `400 #60a5fa`
- Zemin koyu lacivert-antrasit, kartlar bir ton açık, ince kenarlık
- İkonlar **Font Awesome**
- Durum renkleri: yeşil (online/başarılı), kırmızı (offline/hata),
  amber (uyarı), mor (istatistik)

### Arayüzün gerçek yapısı

**Üst bar (soldan sağa):** `SYSTRACK by Z` logosu · canlı sistem çipleri
`CPU %` · `RAM 0.5/7.8 GB` · `Disk 6/58 GB` · `Sıcaklık 48.0°C` · `Lisans`
rozeti · dil seçici (TR bayrağı) · tema düğmesi · bildirim zili ·
`AD admin@systrack.l…` avatarı

**Sol menü (yukarıdan aşağı, birebir bu sırayla):**

```
Dashboard
Hedefler
IP Tarayıcı
Envanter Yönetimi
Server Durumu
Ortam İzleme
Bildirimler  ▾
    Bildirim Geçmişi
    Bildirim Ayarları
    Bildirim Şablonları
Raporlama
Kullanıcılar  ▾
    Kullanıcı Yönetimi
    Modül İzinleri
Ağ Ayarları
Yedekleme
──────────────
Dokümantasyon
DomainTrack          ← mor degrade düğme
```

Aktif menü öğesi mavi vurgulu kapsül içinde gösterilir.

---

## 1. Gösterim koşulu — sert kısıtlar

Video fuar standında **sesi kapalı**, döngüde oynayacak. Ziyaretçi ortalama
**3 saniye** bakıp ya duracak ya geçecek.

- Anlatım tamamen **ekrandaki metinle**; seslendirmeye bağlı olmasın
- Alt başlık şeridindeki yazılar **birkaç metre uzaktan** okunabilsin
- Aynı anda **tek cümle**
- **Logoyla açma** — logo sona saklanır
- Döngü **dikişsiz**: son kare ilk kareye doğal dönmeli
- Arayüz **büyük görünsün** — gerektiğinde ilgili panele zoom yap, tüm
  ekranı küçük küçük gösterip detayı kaybetme

---

## 2. İmleç koreografisi — videonun kalite farkı burada

Fare imleci bu videonun **ana oyuncusu**. Kuralları:

- Hareket **eğrisel ve yumuşak**, asla düz çizgi ve asla ani sıçrama
  (ease-in-out, 400–700 ms)
- Tıklamadan önce hedefin üstünde **kısa bir duraklama** (~200 ms) ve
  butonun **hover durumu** aktifleşsin
- Tıklama anında: imleç **hafifçe küçülür**, tıklanan yerden dışa doğru
  **ince bir dalga halkası** yayılır, buton **basılı** görünür
- Tıklamadan sonra arayüz **gecikmeyle** tepki versin — gerçek yazılım gibi.
  Yükleniyor durumları, iskelet kartlar, dönen ikonlar göster.
- Tablolarda satırların üstünden geçerken **satır vurgusu** oluşsun
- Form alanlarına yazarken **karakter karakter** yazılsın, imleç yanıp sönsün
- Sayfa geçişlerinde içerik **yumuşak belirsin**, sert kesme olmasın

---

## 3. Yapı

```
00:00 ─ 00:08   AÇILIŞ        Sinematik: karanlıkta ana ünite uyanıyor
00:08 ─ 00:20   DASHBOARD     Panel yükleniyor, canlı veriler doluyor
00:20 ─ 00:38   IP TARAYICI   Ağ taraması — cihazlar akarak buluyor
00:38 ─ 00:54   ENVANTER      Varlık ve yazılım takibi, lisans uyarısı
00:54 ─ 01:14   ORTAM İZLEME  Sensörler, eşik aşımı, sıvı teması
01:14 ─ 01:22   BİLDİRİM      Uyarı telefona düşüyor
01:22 ─ 01:30   KAPANIŞ       Dört ürün, logo, slogan
```

---

## 4. Sahne sahne

### 00:00 – 00:08 · AÇILIŞ *(sinematik)*

Karanlık. Koyu lacivert-siyah zemin, arkada ince tel-kafes dünya küresi.

**Ana ünite** ortada: mat beyaz, kare, yuvarlatılmış köşeli kutu. Üst
yüzeyinde dairesel havalandırma ızgarası ve kabartma `SYSTRACK by Z` logosu.
Ön yüzünde RJ45 port ve beyaz ürün etiketi. *(Birebir görünüm:
`fuar-medya/referans/afis-pleksi.png`)*

Ethernet portundaki LED iki kez yanıp sönüyor. Izgaradan yukarı doğru ince
bir mavi ışık halkası yayılıyor. Altında ayna yansıması.

Kamera yavaşça yaklaşıyor, görüntü ünitenin ışığında **beyazlaşıp** bir
tarayıcı penceresine dönüşüyor.

> `Ağınızda olan biteni gerçekten görüyor musunuz?`

### 00:08 – 00:20 · DASHBOARD

Arayüz açılıyor. Önce **iskelet kartlar**, sonra veriler doluyor.

Beş istatistik kartı sayarak doluyor — sayılar 0'dan hedefe yükseliyor:

```
Toplam Hedef 147 │ Online 142 │ Offline 5 │ Açık Uyarı 3 │ Ortalama Uptime %99.8
```

**Sistem Sağlığı** panelinde beşgen radar grafiği (HTTP, HTTPS, SysTrack
Aktifliği, Bildirimler, PING) çizilerek beliriyor; yanında yeşil halka
`%98 Sistem Sağlığı` — altında `Mükemmel`.

Sağda **Hedef Durumları** halka grafiği dolarak çiziliyor.

Üst bardaki çipler canlı: `CPU %12` · `RAM 2.1/7.8 GB` · `Sıcaklık 48.0°C`
— sayılar hafifçe oynuyor.

İmleç ekranın ortasına geliyor, kısa bir an duruyor.

> `Tek ekranda tüm altyapınız.`

### 00:20 – 00:38 · IP TARAYICI

İmleç sol menüde **IP Tarayıcı**'ya gidiyor. Hover → tıklama dalgası →
sayfa geçiyor.

IP aralığı alanına **karakter karakter** yazılıyor: `192.168.1.1 - 192.168.1.254`

İmleç **Taramayı Başlat** butonuna gidip tıklıyor. Buton yükleniyor
durumuna geçiyor, ilerleme çubuğu dolmaya başlıyor.

Tablo **satır satır akarak** doluyor — her satır alttan yumuşakça kayarak
girsin:

```
IP ADRESİ        MAC ADRESİ          ÜRETİCİ          HOSTNAME         DURUM
192.168.1.1      a4:2b:b0:…          TP-Link          gateway          ● Online
192.168.1.24     00:1b:78:…          Dell Inc.        MUHASEBE-PC      ● Online
192.168.1.37     3c:2a:f4:…          HP Inc.          HP-LaserJet      ● Online
192.168.1.52     b8:27:eb:…          Raspberry Pi     systrack         ● Online
```

Köşedeki sayaç artıyor: `0 → 147 cihaz bulundu`

Sonra bir satır **amber** vurguyla beliriyor ve tablo ona kayıyor:

```
192.168.1.88     ff:1a:9c:…          Bilinmeyen       —                ⚠ Yeni cihaz
```

İmleç o satırın üstünde duruyor, satır vurgulanıyor.

> `Ağınıza bağlanan her cihazı bulur.`
> *(amber satır belirince:)* `Tanımadığını size bildirir.`

### 00:38 – 00:54 · ENVANTER YÖNETİMİ

İmleç sol menüde **Envanter Yönetimi**'ne tıklıyor.

Varlık kartları/tablosu geliyor: marka, model, seri numarası, lokasyon,
sorumlu kişi, durum rozeti. Üstte özet sayaçlar.

İmleç bir satıra tıklıyor → **detay paneli** sağdan kayarak açılıyor:

```
Dell OptiPlex 7090
Seri No: 7XK2F93        Lokasyon: Kat 2 · Muhasebe
Sorumlu: A. Yılmaz      Durum: Aktif

[ Donanım ]  [ Yazılım ]  [ Lisanslar ]
```

İmleç **Yazılım** sekmesine tıklıyor. Kurulu yazılım listesi **satır satır
akarak** yazılıyor, yanlarında yeşil onay işaretleri.

Sonra bir satırın rozeti **kırmızıya** dönüyor ve o satır vurgulanıyor:

```
⚠  Lisans uyumsuzluğu tespit edildi
```

> `Hangi cihaz kimde, hangi yazılım lisanslı — hepsi kayıt altında.`

### 00:54 – 01:14 · ORTAM İZLEME

İmleç sol menüde **Ortam İzleme**'ye tıklıyor.

Üstte üç sensör yuvası: `Hava` · `Sıvı` · `Kamera` — her biri bağlı
durumda, yeşil nokta ile.

**Dört canlı değer kartı** sayarak doluyor:

```
Sıcaklık Değerleri   22.4 °C
Nem Değeri           %48
Atmosfer Basıncı     1012 hPa
Hava Kalitesi        İyi
```

Altında **24 saatlik sıcaklık grafiği** soldan sağa çiziliyor — düz seyreden
bir çizgi.

**Gerilim başlıyor:** Sıcaklık kartındaki sayı yükselmeye başlıyor —
`22.4 → 26.1 → 29.3 → 31.2 °C`. Kart kenarlığı yeşilden **ambere**, sonra
**kırmızıya** dönüyor. Grafiğin ucu keskin yukarı kıvrılıyor, eşik çizgisi
(28 °C) kesiliyor ve kesişim noktası kırmızı işaretleniyor.

İmleç aşağı kayıyor. **Sıvı Temas Sensörü** kartı:

```
Son Durum:  Islaklık —           →   ⚠ TEMAS ALGILANDI
IP Adresi:  192.168.1.91
Son Kayıt:  03:19
```

Kart kırmızı yanıp sönüyor. **Olay Günlüğü**'ne yeni satır düşüyor.

İmleç **Kamera Arşivi**'ne tıklıyor — o ana ait kare açılıyor, üstünde
zaman damgası `03:19`.

> `Sıcaklık, nem, basınç, hava kalitesi.`
> *(sıcaklık yükselirken:)* `Eşik aşıldığında fark eder.`
> *(sıvı teması anında:)* `Ve zemindeki suyu da.`

### 01:14 – 01:22 · BİLDİRİM

İmleç üst bardaki **bildirim ziline** gidiyor — zilin üstünde kırmızı rozet
`2` belirmiş. Tıklıyor, açılır panel iniyor:

```
⚠  Sunucu Odası — Sıcaklık 31.2 °C  (eşik 28 °C)      az önce
⚠  Kabinet Altı — Sıvı teması algılandı                az önce
```

Sahne hafifçe geri çekiliyor; ekranın yanında bir **telefon** beliriyor ve
aynı iki bildirim arka arkaya ekranına düşüyor.

> `Siz uyurken telefonunuz çaldı.`

### 01:22 – 01:30 · KAPANIŞ

Tarayıcı penceresi mavi ışıkta eriyor. Sahne afiş kompozisyonuna dönüşüyor:
parlayan tel-kafes dünya küresinin önünde **dört ürün birlikte**, altlarında
ayna yansımaları.

```
beyaz ana ünite (ortada)  ·  mavi hava sensörü (sağda)
siyah sıvı sensörü + kablolu prob (solda önde)  ·  beyaz dome kamera (solda arkada)
```

Altlarında dört ikon ve etiket beliriyor:

```
Ağ İzleme  ·  Envanter ve Yazılım Takibi  ·  Sistem Yönetimi  ·  Ortam İzleme
```

Sonra logo ve slogan:

> **SYSTRACK by Z**
> **HER ŞEY KONTROL ALTINDA**

---

## 5. Metin şeridi

Ekrandaki anlatım cümleleri alt bölgede, **yarı saydam koyu bir şerit**
üzerinde dursun. Kalın, geniş, büyük puntolu sans-serif. Yumuşak belirip
yumuşak kaybolsun. Aynı anda tek cümle. Arayüzün kritik bölgesini
kapatmasın.

---

## 6. Kaçınılacaklar

- Logoyla açmak
- Aynı anda birden fazla cümle
- Arayüzü **uydurmak** — menü isimleri, kart başlıkları, renkler depodaki
  gerçek HTML ile birebir aynı olmalı
- Açık tema (uygulama videoda koyu temada)
- Işınlanan, sıçrayan veya hiç olmayan fare imleci
- Tıklamaya **anında** tepki veren arayüz — gerçekçi değil
- Kod, terminal penceresi, teknik jargon
- Gerçek müşteri adı, gerçek IP veya seri numarası
- Jenerik "siber güvenlik" klişeleri — kilit ikonu, kukuletalı hacker,
  Matrix yağmuru, devre kartı
- Cihazları afişteki görünümden farklı çizmek

---

## 7. Varlıklar

| Klasör / dosya | İçerik |
|---|---|
| `static/admin/*.html` | **Arayüzün gerçek kodu — birincil kaynak** |
| `fuar-medya/referans/afis-pleksi.png` | Ürünlerin gerçek render'ları |
| `fuar-medya/referans/afis-rollup.png` | Sahne kompozisyonu ve ışık dili |
| `fuar-medya/arayuz/` | Gerçek arayüz ekran görüntüleri |
| `fuar-medya/urun/` | Cihaz ve sensör fotoğrafları |
| `fuar-medya/marka/` | SYSTRACK by Z logosu |

Ürünün ne yaptığının ayrıntısı için → [`README.md`](README.md)
