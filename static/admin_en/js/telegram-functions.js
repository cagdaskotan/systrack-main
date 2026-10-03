// Telegram ve Webhook fonksiyonları
window.loadTelegramConfig = async function() {
    try {
        const response = await fetch('/api/telegram/config', {
            headers: {
                'Authorization': 'Bearer ' + localStorage.getItem('token')
            }
        });
        
        if (response.ok) {
            const data = await response.json();
            if (data.success) {
                document.getElementById('telegramBotToken').value = data.data.bot_token || '';
                document.getElementById('telegramChatId').value = data.data.default_chat || '';
                document.getElementById('telegramEnabled').checked = data.data.enabled || false;
            }
        }
    } catch (error) {
        console.error('Telegram config load error:', error);
    }
};

window.saveTelegramConfig = async function() {
    const config = {
        bot_token: document.getElementById('telegramBotToken').value,
        default_chat: document.getElementById('telegramChatId').value,
        enabled: document.getElementById('telegramEnabled').checked
    };

    try {
        const response = await fetch('/api/telegram/config', {
            method: 'POST',
            headers: {
                'Authorization': 'Bearer ' + localStorage.getItem('token'),
                'Content-Type': 'application/json'
            },
            body: JSON.stringify(config)
        });

        const data = await response.json();
        if (data.success) {
            alert('Telegram ayarları başarıyla kaydedildi!');
        } else {
            alert('Hata: ' + (data.error || 'Bilinmeyen hata'));
        }
    } catch (error) {
        console.error('Error:', error);
        alert('Hata: ' + error.message);
    }
};

window.testTelegramConnection = async function() {
    const config = {
        bot_token: document.getElementById('telegramBotToken').value,
        default_chat: document.getElementById('telegramChatId').value
    };

    try {
        const response = await fetch('/api/telegram/test-connection', {
            method: 'POST',
            headers: {
                'Authorization': 'Bearer ' + localStorage.getItem('token'),
                'Content-Type': 'application/json'
            },
            body: JSON.stringify(config)
        });

        const data = await response.json();
        if (data.success) {
            alert('Telegram bağlantı testi başarılı!');
        } else {
            alert('Bağlantı testi başarısız: ' + (data.error || 'Bilinmeyen hata'));
        }
    } catch (error) {
        console.error('Error:', error);
        alert('Test hatası: ' + error.message);
    }
};

window.sendTelegramTestMessage = async function() {
    const config = {
        bot_token: document.getElementById('telegramBotToken').value,
        chat_id: document.getElementById('telegramChatId').value
    };

    try {
        const response = await fetch('/api/telegram/test-message', {
            method: 'POST',
            headers: {
                'Authorization': 'Bearer ' + localStorage.getItem('token'),
                'Content-Type': 'application/json'
            },
            body: JSON.stringify(config)
        });

        const data = await response.json();
        if (data.success) {
            alert('Test mesajı başarıyla gönderildi!');
        } else {
            alert('Test mesajı gönderilemedi: ' + (data.error || 'Bilinmeyen hata'));
        }
    } catch (error) {
        console.error('Error:', error);
        alert('Test hatası: ' + error.message);
    }
};

window.copyWebhookUrl = function() {
    const webhookUrl = document.getElementById('webhookUrl').textContent;
    navigator.clipboard.writeText(webhookUrl).then(() => {
        alert('Webhook URL kopyalandı!');
    }).catch(err => {
        console.error('Kopyalama hatası:', err);
        alert('Kopyalama başarısız!');
    });
};

window.setupWebhook = async function() {
    const botToken = document.getElementById('telegramBotToken').value;
    if (!botToken) {
        alert('Önce Bot Token girin!');
        return;
    }

    const webhookUrl = window.location.origin + '/webhook/telegram';
    
    try {
        const response = await fetch(`https://api.telegram.org/bot${botToken}/setWebhook`, {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json'
            },
            body: JSON.stringify({
                url: webhookUrl
            })
        });

        const data = await response.json();
        if (data.ok) {
            alert('Webhook başarıyla kuruldu! Artık bot Chat ID\'leri otomatik gönderebilir.');
        } else {
            alert('Webhook kurulumu başarısız: ' + data.description);
        }
    } catch (error) {
        console.error('Webhook kurulum hatası:', error);
        alert('Webhook kurulumu sırasında hata oluştu!');
    }
};

window.testWebhook = async function() {
    const botToken = document.getElementById('telegramBotToken').value;
    if (!botToken) {
        alert('Önce Bot Token girin!');
        return;
    }

    try {
        const response = await fetch(`https://api.telegram.org/bot${botToken}/getWebhookInfo`);
        const data = await response.json();
        
        if (data.ok) {
            const info = data.result;
            let message = `Webhook Durumu:\n`;
            message += `URL: ${info.url || 'Kurulmamış'}\n`;
            message += `Son Hata: ${info.last_error_message || 'Yok'}\n`;
            message += `Son Güncelleme: ${info.last_error_date ? new Date(info.last_error_date * 1000).toLocaleString() : 'Yok'}`;
            alert(message);
        } else {
            alert('Webhook bilgisi alınamadı: ' + data.description);
        }
    } catch (error) {
        console.error('Webhook test hatası:', error);
        alert('Webhook testi sırasında hata oluştu!');
    }
};

console.log('Telegram functions loaded successfully!');
