# SysTrack Fuar Videosu — Senaryo Brief'i

**Konu:** SysTrack Ortam İzleme
**Süre:** 75 saniye
**Format:** 16:9, döngüde oynatılacak
**Gösterim koşulu:** Fuar standında, **sesi kapalı**, insanlar yanından geçerken

---

## Temel kural: sessiz izlenecek

Bu video bir standda, sesi kapalı, döngüde oynayacak. Ziyaretçi ona ortalama
**3 saniye** bakıp devam edecek ya da durup izleyecek. Bu yüzden:

- **Anlatım tamamen ekrandaki metinle kurulmalı.** Seslendirme varsa bile hikâye
  seslendirme olmadan da anlaşılmalı.
- İlk 3 saniyede gerilim kurulmalı — logo ile açılmamalı, logo sona saklanmalı.
- Ekrandaki yazılar **uzaktan okunacak kadar büyük** olmalı, aynı anda en fazla
  bir cümle.
- Döngü dikişsiz olmalı: son kare ilk kareye doğal dönmeli.

---

## Hikâye

Gece yarısı, boş bir sunucu odası. İki ayrı felaket aynı anda başlıyor ve kimse
orada değil. SysTrack ikisini de yakalıyor. Sabah geldiklerinde hiçbir şey olmamış.

**Vermek istediğimiz his:** *"Ben uyurken birisi nöbetteydi."*

---

## Sahne planı

### 00:00 – 00:06 — Sessizlik
Karanlık sunucu odası. Sadece rack LED'lerinin nabız gibi yanıp sönmesi.
Köşede küçük bir saat: **03:17**.

> **Ekranda:** `Saat 03:17. Ofiste kimse yok.`

### 00:06 – 00:14 — İlk tehdit
Klima ünitesine yakın plan — fan yavaşlıyor ve duruyor. Odanın rengi
usulca maviden sıcak kırmızıya kayıyor. Termometre benzeri bir gösterge
yükselmeye başlıyor: `22°C → 26°C → 31°C`

> **Ekranda:** `Klima durdu. Sıcaklık yükseliyor.`

### 00:14 – 00:22 — Cihaz uyanıyor
Kabinin içindeki **SysTrack ana ünitesi** (beyaz kutu) ve yanındaki
**mavi hava sensörü** aydınlanıyor. Sensörden ana üniteye doğru bir veri
dalgası akıyor.

Arayüzde dört değer canlı yükseliyor:
`SICAKLIK 31°C` · `NEM %64` · `BASINÇ 1012 hPa` · `HAVA KALİTESİ`

> **Ekranda:** `SysTrack fark etti.`

### 00:22 – 00:30 — İkinci tehdit
Kamera zemine iniyor. Kabinin altında ince bir su sızıntısı yayılıyor,
**sıvı sensörünün** algılama ucuna değiyor. Sensör kırmızı yanıp sönüyor.

> **Ekranda:** `Aynı anda, zeminde su.`

### 00:30 – 00:40 — Uyarı gidiyor
Cihazdan yukarı doğru çıkan ışık izi. Bir telefon ekranında arka arkaya
iki bildirim beliriyor:

```
⚠  Sunucu Odası — Sıcaklık 31°C (eşik: 28°C)
⚠  Kabinet Altı — Sıvı teması algılandı
```

> **Ekranda:** `Siz uyurken, telefonunuz çaldı.`

### 00:40 – 00:52 — Kontrol paneli
Gerçek arayüze geçiş (`fuar-medya/arayuz/`). Sakin, kontrollü bir tur:
- canlı sensör kartları
- son 24 saatlik sıcaklık grafiği — yükselişin tepe noktası net görünüyor
- Sıvı Temas Geçmişi listesi
- Kamera Arşivi'nden odanın o anki karesi

> **Ekranda:** `Ne olduğunu, ne zaman olduğunu görün.`

### 00:52 – 01:04 — Sabah
Oda tekrar mavi ve serin. Işıklar yanıyor, gösterge normale dönmüş: `22°C`.
Zemin kuru.

> **Ekranda:** `Sabah geldiğinizde, sorun çoktan çözülmüştü.`

### 01:04 – 01:15 — Kapanış
Dört ürün birlikte beliriyor — **ana ünite, hava sensörü, sıvı sensörü, kamera**
(fotoğraflar `fuar-medya/urun/` içinde). Altlarında dört kısa yetenek:

`Ortam İzleme` · `Ağ İzleme` · `Envanter Takibi` · `Sistem Yönetimi`

Sonra logo ve slogan:

> **SYSTRACK by Z**
> **HER ŞEY KONTROL ALTINDA**

---

## Görsel dil

Afişlerdeki dili birebir sürdür:

- **Arka plan:** çok koyu lacivert → siyah geçişli, derinlikli
- **Vurgu rengi:** elektrik mavisi / camgöbeği, parlayan ince çizgiler
- **Tehlike rengi:** sadece tehdit anlarında turuncu-kırmızı — az ve etkili
- **Motif:** ince ışık çizgileriyle örülmüş ağ / grid dokusu, hafif parıltı
- **Tipografi:** kalın, geniş, sans-serif, büyük harf başlıklar
- **Ürünler:** beyaz ana ünite, mavi hava sensörü, siyah sıvı sensörü, beyaz dome kamera

Hareket **ağır ve kontrollü** olmalı — yavaş kaydırmalar, yumuşak geçişler.
Hızlı kesme ve sarsıntılı kamera yok; bu bir güven ürünü, bir aksiyon filmi değil.

---

## Kaçınılacaklar

- Logoyla açmak (ilk 3 saniye hikâyeye ait)
- Aynı anda birden fazla cümle göstermek
- Kodun, terminalin veya teknik jargonun ekranda görünmesi
- Gerçek müşteri adı, IP adresi veya seri numarası
- Afiş paletinin dışına çıkan renkler
- Sadece seslendirmeyle anlaşılan bir anlatım

---

## Kullanılacak varlıklar

| Klasör | İçerik |
|---|---|
| `fuar-medya/urun/` | Cihaz ve sensör fotoğrafları |
| `fuar-medya/arayuz/` | Gerçek arayüz ekran görüntüleri |
| `fuar-medya/marka/` | SYSTRACK by Z logosu |

Ürünün ne yaptığının ayrıntısı için → [`README.md`](README.md)
