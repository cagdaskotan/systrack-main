# Bulk Notification Kullanım Kılavuzu

## 📋 Genel Bakış

Bulk Notification özelliği, birden fazla hedef için tek bir bildirimde toplu mail gönderimini sağlar. Bu sayede:
- ✅ 20 hedef için 20 ayrı mail yerine **1 tek mail** gönderilir
- ✅ Mail kutusu spam'lenmez
- ✅ Tüm hedeflerin durumu **tek bir tabloda** görülür
- ✅ SMTP sunucu yükü %95 oranında azalır

---

## 🏗️ Mimari

### Backend Değişiklikleri

#### 1. Yeni Struct'lar (`internal/notifications/types.go`)

```go
// BulkNotificationRecipients bulk notification için recipients yapısı
type BulkNotificationRecipients struct {
    Emails  []string                  `json:"emails"`
    Targets []TargetNotificationData  `json:"targets,omitempty"`
}

// TargetNotificationData tek bir hedefin bildirim verisi
type TargetNotificationData struct {
    ID            int      `json:"id"`
    Name          string   `json:"name"`
    Address       string   `json:"address"`
    Status        string   `json:"status"`
    StatusClass   string   `json:"status_class"`   // "online", "offline", "warning"
    UptimePercent *float64 `json:"uptime_percent,omitempty"`
    ResponseTime  *float64 `json:"response_time,omitempty"`
}
```

#### 2. Bulk Template'ler (`internal/notifications/templates.go`)

- `getBulkTargetStatusTemplate()`: Toplu hedef durum değişikliği
- `getBulkSLAViolationTemplate()`: Toplu SLA ihlali

#### 3. Email Service (`internal/notifications/email.go`)

- `SendBulk()`: Bulk mail gönderimi
- `generateBulkHTMLBody()`: Bulk HTML body oluşturma
- `prepareTargetsData()`: Hedef verilerini hazırlama

#### 4. Notification Service (`internal/notifications/notification_service.go`)

- `SendNotification()`: Recipients parse ederek bulk/single otomatik ayırımı yapar
- `sendBulkNotification()`: Bulk notification gönderim logici

---

## 🚀 Kullanım

### Örnek 1: API Üzerinden Bulk Notification Gönderimi

```bash
curl -X POST http://localhost:8080/api/notifications/send \
  -H "Content-Type: application/json" \
  -d '{
    "type": "target_status_change",
    "channel": "email",
    "priority": "high",
    "title": "Toplu Hedef Durum Bildirimi",
    "message": "Aşağıdaki hedefler offline duruma geçti",
    "recipients": [
      "{\"emails\":[\"admin@firma.com\"],\"targets\":[{\"id\":1,\"name\":\"Ana Ofis Router\",\"address\":\"192.168.1.1\",\"status\":\"Offline\",\"status_class\":\"offline\",\"uptime_percent\":98.5,\"response_time\":250},{\"id\":2,\"name\":\"Şube 1 Switch\",\"address\":\"192.168.2.1\",\"status\":\"Offline\",\"status_class\":\"offline\",\"uptime_percent\":99.2,\"response_time\":180},{\"id\":3,\"name\":\"DMZ Firewall\",\"address\":\"10.0.0.1\",\"status\":\"Offline\",\"status_class\":\"offline\",\"uptime_percent\":97.8,\"response_time\":320}]}"
    ]
  }'
```

### Örnek 2: Go Kod İçinden Bulk Notification

```go
package main

import (
    "context"
    "systrack/internal/notifications"
)

func sendBulkNotification(service notifications.Service) error {
    // Hedef verilerini hazırla
    targets := []notifications.TargetNotificationData{
        {
            ID:            1,
            Name:          "Ana Ofis Router",
            Address:       "192.168.1.1",
            Status:        "Offline",
            StatusClass:   "offline",
            UptimePercent: floatPtr(98.5),
            ResponseTime:  floatPtr(250),
        },
        {
            ID:            2,
            Name:          "Şube 1 Switch",
            Address:       "192.168.2.1",
            Status:        "Offline",
            StatusClass:   "offline",
            UptimePercent: floatPtr(99.2),
            ResponseTime:  floatPtr(180),
        },
    }

    // Bulk recipients oluştur
    bulkRecipients := notifications.BulkNotificationRecipients{
        Emails:  []string{"admin@firma.com"},
        Targets: targets,
    }

    // JSON'a çevir
    recipientsJSON, _ := json.Marshal(bulkRecipients)

    // Notification oluştur
    notification := &notifications.Notification{
        Type:       notifications.TargetStatusChange,
        Channel:    notifications.Email,
        Priority:   notifications.High,
        Title:      "Toplu Hedef Durum Bildirimi",
        Message:    "Aşağıdaki hedefler offline duruma geçti",
        Recipients: []string{string(recipientsJSON)},
    }

    // Gönder
    return service.SendNotification(context.Background(), notification)
}

func floatPtr(f float64) *float64 {
    return &f
}
```

### Örnek 3: Frontend (JavaScript) İle Kullanım

```javascript
async function sendBulkNotification(targetIds) {
    // 1. Seçilen hedeflerin bilgilerini al
    const targets = [];
    for (const targetId of targetIds) {
        const response = await fetch(`/api/targets/${targetId}`);
        const target = await response.json();

        targets.push({
            id: target.id,
            name: target.name,
            address: target.address,
            status: target.status,
            status_class: getStatusClass(target.status),
            uptime_percent: target.uptime_percent,
            response_time: target.response_time
        });
    }

    // 2. Bulk recipients oluştur
    const bulkRecipients = {
        emails: ["admin@firma.com"],
        targets: targets
    };

    // 3. Notification payload
    const payload = {
        type: "target_status_change",
        channel: "email",
        priority: "high",
        title: `Toplu Bildirim - ${targetIds.length} Hedef`,
        message: "Hedef durum değişikliği bildirimi",
        recipients: [JSON.stringify(bulkRecipients)]
    };

    // 4. API'ye gönder
    const response = await fetch('/api/notifications/send', {
        method: 'POST',
        headers: {'Content-Type': 'application/json'},
        body: JSON.stringify(payload)
    });

    return response.json();
}

function getStatusClass(status) {
    switch (status.toLowerCase()) {
        case 'online':
        case 'success':
            return 'online';
        case 'offline':
        case 'failed':
            return 'offline';
        case 'warning':
            return 'warning';
        default:
            return 'unknown';
    }
}

// Kullanım
const selectedTargetIds = [1, 2, 3, 4, 5];
sendBulkNotification(selectedTargetIds);
```

---

## 📧 Email Template Çıktısı

### Bulk Target Status Email Örneği

```
┌────────────────────────────────────────────────────────┐
│ 📊 Toplu Hedef Durum Bildirimi                         │
│ SysTrack İzleme Sistemi                                │
└────────────────────────────────────────────────────────┘

Toplam Hedef Sayısı: 3
02.12.2025 14:30:15

┌───────────────────────────────────────────────────────────────────┐
│ Hedef Adı          │ Adres         │ Durum    │ Uptime  │ Yanıt  │
├───────────────────────────────────────────────────────────────────┤
│ Ana Ofis Router    │ 192.168.1.1   │ OFFLINE  │ 98.5%   │ 250ms  │
│ Şube 1 Switch      │ 192.168.2.1   │ OFFLINE  │ 99.2%   │ 180ms  │
│ DMZ Firewall       │ 10.0.0.1      │ OFFLINE  │ 97.8%   │ 320ms  │
└───────────────────────────────────────────────────────────────────┘

💡 Bilgi: Bu bildirim 3 hedef için toplu olarak oluşturulmuştur.
```

---

## 🔍 Bulk vs Single Notification Karşılaştırması

### Single Notification (Mevcut Sistem)

```json
{
  "recipients": ["admin@firma.com"],
  "target_id": 1
}
```

**Sonuç:** 1 hedef = 1 mail

---

### Bulk Notification (Yeni Sistem)

```json
{
  "recipients": [
    "{\"emails\":[\"admin@firma.com\"],\"targets\":[{\"id\":1,...},{\"id\":2,...},{\"id\":3,...}]}"
  ]
}
```

**Sonuç:** 3 hedef = 1 mail (tablo formatında)

---

## ⚙️ Otomatik Tespit Mekanizması

`SendNotification()` fonksiyonu otomatik olarak bulk/single ayırımı yapar:

```go
// Recipients içinde "targets" key'i var mı?
// targets array'inde 2+ eleman var mı?
// → EVET: sendBulkNotification() çağır
// → HAYIR: Normal Send() çağır
```

**Geriye Dönük Uyumluluk:** Mevcut single notification kodları çalışmaya devam eder!

---

## 🎯 Status Class Değerleri

| Status | StatusClass | CSS Class | Renk |
|--------|-------------|-----------|------|
| Online, Success, Çevrimiçi | `online` | `.status-online` | 🟢 Yeşil |
| Offline, Failed, Çevrimdışı | `offline` | `.status-offline` | 🔴 Kırmızı |
| Warning, Uyarı | `warning` | `.status-warning` | 🟡 Sarı |
| Unknown | `unknown` | `.status-unknown` | ⚫ Gri |

---

## 🧪 Test Senaryoları

### Test 1: 2 Hedef ile Bulk Notification

```bash
curl -X POST http://localhost:8080/api/notifications/send \
  -H "Content-Type: application/json" \
  -d @test_bulk_2_targets.json
```

**Beklenen Sonuç:**
- ✅ 1 email gönderilir
- ✅ Email'de 2 satırlık tablo görülür
- ✅ `notifications` tablosunda 1 kayıt oluşur
- ✅ `email_logs` tablosunda 1 kayıt oluşur

### Test 2: 1 Hedef ile Single Notification

```json
{
  "recipients": ["admin@firma.com"],
  "target_id": 1
}
```

**Beklenen Sonuç:**
- ✅ Normal single notification template kullanılır
- ✅ Bulk template DEĞİL, single template render edilir

---

## 📊 Performans İyileştirmesi

| Senaryo | Eski Sistem | Yeni Sistem | İyileştirme |
|---------|-------------|-------------|-------------|
| 5 hedef | 5 mail | 1 mail | **80% azalma** |
| 10 hedef | 10 mail | 1 mail | **90% azalma** |
| 20 hedef | 20 mail | 1 mail | **95% azalma** |
| 50 hedef | 50 mail | 1 mail | **98% azalma** |

**SMTP Bağlantı Sayısı:** N → 1 (N: hedef sayısı)

---

## ⚠️ Dikkat Edilmesi Gerekenler

1. **Recipients Format:** `recipients` array'inin ilk elemanı JSON string olmalı
2. **Targets Minimum:** En az 2 hedef olmalı (1 hedef = normal notification)
3. **StatusClass Zorunlu:** Her target için `status_class` alanı doldurulmalı
4. **Optional Alanlar:** `uptime_percent` ve `response_time` opsiyoneldir (pointer)
5. **Email Only:** Şu anda sadece `email` kanalı bulk destekliyor

---

## 🛠️ Troubleshooting

### Hata: "invalid bulk recipients format"

**Neden:** `recipients` JSON'u parse edilemiyor.

**Çözüm:** JSON formatını kontrol edin:
```json
{
  "emails": ["admin@firma.com"],
  "targets": [...]
}
```

### Hata: "no targets provided for bulk notification"

**Neden:** `targets` array'i boş.

**Çözüm:** En az 1 hedef ekleyin.

### Email gelmiyor ama hata yok

**Neden:** SMTP ayarları yanlış veya email logger kapalı.

**Çözüm:**
1. `email_logs` tablosunu kontrol edin
2. SMTP host/port/credentials doğru mu?
3. `config.Email.Enabled = true` mi?

---

## 📝 Veritabanı Etkisi

### `notifications` Tablosu

- **Değişiklik YOK:** Mevcut yapı korundu
- **recipients (JSON):** Artık hem string array hem de object içerebilir

**Örnek (Bulk):**
```json
[
  "{\"emails\":[\"admin@firma.com\"],\"targets\":[...]}"
]
```

**Örnek (Single - eski):**
```json
["admin@firma.com", "user@firma.com"]
```

### Geriye Dönük Uyumluluk

✅ **%100 Uyumlu:** Eski notification kayıtları çalışmaya devam eder!

---

## 🎉 Özet

- ✅ Veritabanı değişikliği **YOK**
- ✅ Mevcut kod **ÇALIŞMAYA DEVAM** eder
- ✅ Bulk notification **OTOMATİK** tespit edilir
- ✅ Email template'leri **DİNAMİK** olarak render edilir
- ✅ Performance **%95'e varan** iyileşme

**Başarılı implementasyon! 🚀**
