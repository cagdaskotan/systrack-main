// Bildirim Yönetimi JavaScript Fonksiyonları
// Bu dosya tüm bildirim yönetimi sayfalarında kullanılabilir

class NotificationManager {
    constructor() {
        this.baseURL = '/api/notifications';
        this.templates = [];
        this.rules = [];
        this.notifications = [];
        this.stats = {};
    }

    // API çağrıları
    async apiCall(endpoint, method = 'GET', data = null) {
        try {
            const options = {
                method,
                headers: {
                    'Content-Type': 'application/json',
                }
            };

            if (data) {
                options.body = JSON.stringify(data);
            }

            const response = await fetch(`${this.baseURL}${endpoint}`, options);
            const result = await response.json();

            if (!result.success) {
                throw new Error(result.error || 'API çağrısında hata oluştu');
            }

            return result;
        } catch (error) {
            console.error('API çağrısı hatası:', error);
            throw error;
        }
    }

    // Şablon yönetimi
    async loadTemplates() {
        try {
            const result = await this.apiCall('/templates');
            this.templates = result.data;
            return this.templates;
        } catch (error) {
            this.showToast('Şablonlar yüklenirken hata oluştu: ' + error.message, 'error');
            return [];
        }
    }

    async createTemplate(templateData) {
        try {
            const result = await this.apiCall('/templates', 'POST', templateData);
            this.showToast('Şablon başarıyla oluşturuldu', 'success');
            return result.data;
        } catch (error) {
            this.showToast('Şablon oluşturulurken hata oluştu: ' + error.message, 'error');
            throw error;
        }
    }

    async updateTemplate(id, templateData) {
        try {
            const result = await this.apiCall(`/templates/${id}`, 'PUT', templateData);
            this.showToast('Şablon başarıyla güncellendi', 'success');
            return result.data;
        } catch (error) {
            this.showToast('Şablon güncellenirken hata oluştu: ' + error.message, 'error');
            throw error;
        }
    }

    async deleteTemplate(id) {
        try {
            await this.apiCall(`/templates/${id}`, 'DELETE');
            this.showToast('Şablon başarıyla silindi', 'success');
            return true;
        } catch (error) {
            this.showToast('Şablon silinirken hata oluştu: ' + error.message, 'error');
            throw error;
        }
    }

    // Kural yönetimi
    async loadRules() {
        try {
            const result = await this.apiCall('/rules');
            this.rules = result.data;
            return this.rules;
        } catch (error) {
            this.showToast('Kurallar yüklenirken hata oluştu: ' + error.message, 'error');
            return [];
        }
    }

    async createRule(ruleData) {
        try {
            const result = await this.apiCall('/rules', 'POST', ruleData);
            this.showToast('Kural başarıyla oluşturuldu', 'success');
            return result.data;
        } catch (error) {
            this.showToast('Kural oluşturulurken hata oluştu: ' + error.message, 'error');
            throw error;
        }
    }

    async updateRule(id, ruleData) {
        try {
            const result = await this.apiCall(`/rules/${id}`, 'PUT', ruleData);
            this.showToast('Kural başarıyla güncellendi', 'success');
            return result.data;
        } catch (error) {
            this.showToast('Kural güncellenirken hata oluştu: ' + error.message, 'error');
            throw error;
        }
    }

    async deleteRule(id) {
        try {
            await this.apiCall(`/rules/${id}`, 'DELETE');
            this.showToast('Kural başarıyla silindi', 'success');
            return true;
        } catch (error) {
            this.showToast('Kural silinirken hata oluştu: ' + error.message, 'error');
            throw error;
        }
    }

    async toggleRule(id, isActive) {
        try {
            const rule = this.rules.find(r => r.id === id);
            if (!rule) throw new Error('Kural bulunamadı');

            const result = await this.apiCall(`/rules/${id}`, 'PUT', {
                ...rule,
                is_active: isActive
            });
            
            this.showToast(`Kural ${isActive ? 'aktif' : 'pasif'} edildi`, 'success');
            return result.data;
        } catch (error) {
            this.showToast('Kural durumu değiştirilirken hata oluştu: ' + error.message, 'error');
            throw error;
        }
    }

    // Bildirim geçmişi
    async loadNotifications(limit = 50, offset = 0) {
        try {
            const result = await this.apiCall(`/history?limit=${limit}&offset=${offset}`);
            this.notifications = result.data;
            return this.notifications;
        } catch (error) {
            this.showToast('Bildirimler yüklenirken hata oluştu: ' + error.message, 'error');
            return [];
        }
    }

    async retryNotification(id) {
        try {
            const result = await this.apiCall(`/${id}/retry`, 'POST');
            this.showToast('Bildirim tekrar gönderildi', 'success');
            return result.data;
        } catch (error) {
            this.showToast('Bildirim tekrar gönderilirken hata oluştu: ' + error.message, 'error');
            throw error;
        }
    }

    // İstatistikler
    async loadStats() {
        try {
            const result = await this.apiCall('/stats');
            this.stats = result.data;
            return this.stats;
        } catch (error) {
            this.showToast('İstatistikler yüklenirken hata oluştu: ' + error.message, 'error');
            return {};
        }
    }

    // Test bildirimi
    async sendTestNotification(testData) {
        try {
            const result = await this.apiCall('/test', 'POST', testData);
            this.showToast('Test bildirimi başarıyla gönderildi', 'success');
            return result.data;
        } catch (error) {
            this.showToast('Test bildirimi gönderilirken hata oluştu: ' + error.message, 'error');
            throw error;
        }
    }

    // Tag yönetimi
    async loadTags() {
        try {
            const result = await this.apiCall('/tags');
            return result.data;
        } catch (error) {
            this.showToast('Tag\'ler yüklenirken hata oluştu: ' + error.message, 'error');
            return [];
        }
    }

    async createTag(tagData) {
        try {
            const result = await this.apiCall('/tags', 'POST', tagData);
            this.showToast('Tag başarıyla oluşturuldu', 'success');
            return result.data;
        } catch (error) {
            this.showToast('Tag oluşturulurken hata oluştu: ' + error.message, 'error');
            throw error;
        }
    }

    async updateTag(id, tagData) {
        try {
            const result = await this.apiCall(`/tags/${id}`, 'PUT', tagData);
            this.showToast('Tag başarıyla güncellendi', 'success');
            return result.data;
        } catch (error) {
            this.showToast('Tag güncellenirken hata oluştu: ' + error.message, 'error');
            throw error;
        }
    }

    async deleteTag(id) {
        try {
            await this.apiCall(`/tags/${id}`, 'DELETE');
            this.showToast('Tag başarıyla silindi', 'success');
            return true;
        } catch (error) {
            this.showToast('Tag silinirken hata oluştu: ' + error.message, 'error');
            throw error;
        }
    }

    // Kullanıcı ayarları
    async loadUserSettings() {
        try {
            const result = await this.apiCall('/settings');
            return result.data;
        } catch (error) {
            this.showToast('Kullanıcı ayarları yüklenirken hata oluştu: ' + error.message, 'error');
            return [];
        }
    }

    async updateUserSettings(settingsData) {
        try {
            const result = await this.apiCall('/settings', 'PUT', settingsData);
            this.showToast('Ayarlar başarıyla güncellendi', 'success');
            return result.data;
        } catch (error) {
            this.showToast('Ayarlar güncellenirken hata oluştu: ' + error.message, 'error');
            throw error;
        }
    }

    // Yardımcı fonksiyonlar
    showToast(message, type = 'info') {
        if (window.Toast && typeof window.Toast.show === 'function') {
            window.Toast.show({ type, title: '', message });
            return;
        }

        const container = document.getElementById('toastContainer') || this.createToastContainer();
        const toast = document.createElement('div');
        
        const colors = {
            success: 'bg-green-500',
            error: 'bg-red-500',
            warning: 'bg-yellow-500',
            info: 'bg-blue-500'
        };
        
        const icons = {
            success: 'fas fa-check-circle',
            error: 'fas fa-exclamation-circle',
            warning: 'fas fa-exclamation-triangle',
            info: 'fas fa-info-circle'
        };
        
        toast.className = `${colors[type]} text-white px-6 py-3 rounded-lg shadow-lg mb-2 flex items-center`;
        toast.innerHTML = `
            <i class="${icons[type]} mr-2"></i>
            <span>${message}</span>
        `;
        
        container.appendChild(toast);
        
        // Auto remove after 5 seconds
        setTimeout(() => {
            toast.remove();
        }, 5000);
    }

    createToastContainer() {
        const container = document.createElement('div');
        container.id = 'toastContainer';
        container.className = 'fixed top-4 right-4 z-50';
        document.body.appendChild(container);
        return container;
    }

    // Filtreleme fonksiyonları
    filterTemplates(filters) {
        let filtered = [...this.templates];
        
        if (filters.type) {
            filtered = filtered.filter(t => t.type === filters.type);
        }
        
        if (filters.channel) {
            filtered = filtered.filter(t => t.channel === filters.channel);
        }
        
        if (filters.status) {
            if (filters.status === 'active') {
                filtered = filtered.filter(t => t.is_active);
            } else if (filters.status === 'inactive') {
                filtered = filtered.filter(t => !t.is_active);
            } else if (filters.status === 'default') {
                filtered = filtered.filter(t => t.is_default);
            }
        }
        
        return filtered;
    }

    filterRules(filters) {
        let filtered = [...this.rules];
        
        if (filters.type) {
            filtered = filtered.filter(r => r.type === filters.type);
        }
        
        if (filters.channel) {
            filtered = filtered.filter(r => r.channel === filters.channel);
        }
        
        if (filters.priority) {
            filtered = filtered.filter(r => r.priority === filters.priority);
        }
        
        if (filters.status) {
            if (filters.status === 'active') {
                filtered = filtered.filter(r => r.is_active);
            } else if (filters.status === 'inactive') {
                filtered = filtered.filter(r => !r.is_active);
            }
        }
        
        return filtered;
    }

    filterNotifications(filters) {
        let filtered = [...this.notifications];
        
        if (filters.type) {
            filtered = filtered.filter(n => n.type === filters.type);
        }
        
        if (filters.channel) {
            filtered = filtered.filter(n => n.channel === filters.channel);
        }
        
        if (filters.status) {
            filtered = filtered.filter(n => n.status === filters.status);
        }
        
        if (filters.date) {
            const filterDate = new Date(filters.date);
            filtered = filtered.filter(n => {
                const notificationDate = new Date(n.created_at);
                return notificationDate.toDateString() === filterDate.toDateString();
            });
        }
        
        return filtered;
    }

    // Export fonksiyonları
    exportToCSV(data, filename) {
        const csvContent = this.generateCSV(data);
        const blob = new Blob([csvContent], { type: 'text/csv;charset=utf-8;' });
        const link = document.createElement('a');
        const url = URL.createObjectURL(blob);
        link.setAttribute('href', url);
        link.setAttribute('download', filename);
        link.style.visibility = 'hidden';
        document.body.appendChild(link);
        link.click();
        document.body.removeChild(link);
    }

    generateCSV(data) {
        if (data.length === 0) return '';
        
        const headers = Object.keys(data[0]);
        const rows = data.map(item => headers.map(header => item[header]));
        
        return [headers, ...rows].map(row => 
            row.map(field => `"${field}"`).join(',')
        ).join('\n');
    }

    // Utility fonksiyonları
    formatDate(dateString) {
        return new Date(dateString).toLocaleString('tr-TR');
    }

    formatDateShort(dateString) {
        return new Date(dateString).toLocaleDateString('tr-TR');
    }

    getTypeLabel(type) {
        const labels = {
            'target_status_change': 'Hedef Durum Değişikliği',
            'sla_violation': 'SLA İhlali',
            'alert_opened': 'Uyarı Açılma',
            'alert_closed': 'Uyarı Kapanma',
            'system_health': 'Sistem Sağlığı',
            'daily_report': 'Günlük Rapor'
        };
        return labels[type] || type;
    }

    getChannelLabel(channel) {
        const labels = {
            'email': 'Email',
            'telegram': 'Telegram',
            'whatsapp': 'WhatsApp',
            'webhook': 'Webhook'
        };
        return labels[channel] || channel;
    }

    getChannelIcon(channel) {
        const icons = {
            'email': 'fa-envelope',
            'telegram': 'fa-paper-plane',
            'whatsapp': 'fa-whatsapp',
            'webhook': 'fa-link'
        };
        return icons[channel] || 'fa-question';
    }

    getChannelColor(channel) {
        const colors = {
            'email': 'bg-blue-100 text-blue-800',
            'telegram': 'bg-cyan-100 text-cyan-800',
            'whatsapp': 'bg-green-100 text-green-800',
            'webhook': 'bg-purple-100 text-purple-800'
        };
        return colors[channel] || 'bg-gray-100 text-gray-800';
    }

    getPriorityLabel(priority) {
        const labels = {
            'low': 'Düşük',
            'medium': 'Orta',
            'high': 'Yüksek',
            'critical': 'Kritik'
        };
        return labels[priority] || priority;
    }

    getPriorityColor(priority) {
        const colors = {
            'low': 'bg-gray-100 text-gray-800',
            'medium': 'bg-yellow-100 text-yellow-800',
            'high': 'bg-orange-100 text-orange-800',
            'critical': 'bg-red-100 text-red-800'
        };
        return colors[priority] || 'bg-gray-100 text-gray-800';
    }

    getStatusLabel(status) {
        const labels = {
            'pending': 'Bekleyen',
            'sent': 'Gönderilen',
            'failed': 'Başarısız',
            'delivered': 'Teslim Edilen'
        };
        return labels[status] || status;
    }

    getStatusColor(status) {
        const colors = {
            'pending': 'bg-yellow-100 text-yellow-800',
            'sent': 'bg-green-100 text-green-800',
            'failed': 'bg-red-100 text-red-800',
            'delivered': 'bg-blue-100 text-blue-800'
        };
        return colors[status] || 'bg-gray-100 text-gray-800';
    }

    getStatusIcon(status) {
        const icons = {
            'pending': 'fa-clock',
            'sent': 'fa-check',
            'failed': 'fa-times',
            'delivered': 'fa-check-double'
        };
        return icons[status] || 'fa-question';
    }

    // Modal yönetimi
    openModal(modalId) {
        const modal = document.getElementById(modalId);
        if (modal) {
            modal.classList.remove('hidden');
        }
    }

    closeModal(modalId) {
        const modal = document.getElementById(modalId);
        if (modal) {
            modal.classList.add('hidden');
        }
    }

    // Form validasyonu
    validateForm(formId) {
        const form = document.getElementById(formId);
        if (!form) return false;

        const requiredFields = form.querySelectorAll('[required]');
        let isValid = true;

        requiredFields.forEach(field => {
            if (!field.value.trim()) {
                field.classList.add('border-red-500');
                isValid = false;
            } else {
                field.classList.remove('border-red-500');
            }
        });

        return isValid;
    }

    // Loading state yönetimi
    showLoading(elementId, show = true) {
        const element = document.getElementById(elementId);
        if (!element) return;

        if (show) {
            element.innerHTML = `
                <div class="text-center py-8">
                    <div class="inline-flex items-center">
                        <div class="animate-spin rounded-full h-6 w-6 border-b-2 border-blue-600 mr-2"></div>
                        <span class="text-gray-500">Yükleniyor...</span>
                    </div>
                </div>
            `;
        }
    }

    // Empty state yönetimi
    showEmptyState(elementId, message = 'Veri bulunamadı', icon = 'fa-inbox') {
        const element = document.getElementById(elementId);
        if (!element) return;

        element.innerHTML = `
            <div class="text-center py-12">
                <i class="fas ${icon} text-gray-300 text-6xl mb-4"></i>
                <h3 class="text-lg font-medium text-gray-900 mb-2">${message}</h3>
                <p class="text-gray-500">Henüz veri bulunmuyor</p>
            </div>
        `;
    }
}

// Global instance
const notificationManager = new NotificationManager();

// Utility fonksiyonları global olarak erişilebilir hale getir
window.showToast = (message, type) => notificationManager.showToast(message, type);
window.formatDate = (dateString) => notificationManager.formatDate(dateString);
window.formatDateShort = (dateString) => notificationManager.formatDateShort(dateString);
window.getTypeLabel = (type) => notificationManager.getTypeLabel(type);
window.getChannelLabel = (channel) => notificationManager.getChannelLabel(channel);
window.getChannelIcon = (channel) => notificationManager.getChannelIcon(channel);
window.getChannelColor = (channel) => notificationManager.getChannelColor(channel);
window.getPriorityLabel = (priority) => notificationManager.getPriorityLabel(priority);
window.getPriorityColor = (priority) => notificationManager.getPriorityColor(priority);
window.getStatusLabel = (status) => notificationManager.getStatusLabel(status);
window.getStatusColor = (status) => notificationManager.getStatusColor(status);
window.getStatusIcon = (status) => notificationManager.getStatusIcon(status);
