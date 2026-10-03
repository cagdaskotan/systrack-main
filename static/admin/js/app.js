// Dashboard translations - Turkish
const dashboardTranslations = {
    healthLabels: {
        excellent: "Mükemmel",
        very_good: "Çok İyi",
        healthy: "Sağlıklı",
        needs_improvement: "İyileştirilmeli",
        critical: "Kritik",
        unknown: "Bilinmiyor"
    },
    healthDescriptions: {
        excellent: "Tüm sistemler sorunsuz çalışıyor.",
        very_good: "Sistemler oldukça iyi durumda.",
        healthy: "Sistemler normal çalışıyor.",
        needs_improvement: "Bazı sistemlerde sorunlar var.",
        critical: "Sistemlerde kritik sorunlar mevcut.",
        unknown: "Sistem durumu belirlenemiyor."
    },
    chartLabels: {
        http: "HTTP",
        https: "HTTPS",
        services: "Servisler",
        notifications: "Bildirimler",
        ping: "PING"
    }
};

// API Error translations - Turkish
const errorTranslations = {
    duplicate_target: "Bu IP adresi ve monitoring tipi kombinasyonu zaten mevcut. Lütfen farklı bir IP adresi veya monitoring tipi seçin.",
    target_not_found: "Hedef bulunamadı",
    invalid_target_id: "Geçersiz hedef ID",
    failed_to_fetch_target: "Hedef bilgileri getirilemedi",
    failed_to_validate_uniqueness: "Benzersizlik kontrolü başarısız",
    failed_to_create_target: "Hedef oluşturulamadı",
    failed_to_update_target: "Hedef güncellenemedi",
    failed_to_delete_target: "Hedef silinemedi",
    failed_to_fetch_targets: "Hedefler getirilemedi",
    no_fields_to_update: "Güncellenecek alan yok",
    invalid_request_format: "Geçersiz istek formatı",
    no_targets_selected: "Hedef seçilmedi",
    operation_required: "İşlem gerekli",
    no_file_uploaded: "Dosya yüklenmedi"
};

// Helper function to translate API errors
function translateError(errorKey) {
    // If error is already translated or is a sentence, return as-is
    if (!errorKey || errorKey.includes(' ') || errorKey.length > 50) {
        return errorKey;
    }
    // Convert to snake_case if needed (e.g., "Invalid target ID" -> "invalid_target_id")
    const snakeKey = errorKey.toLowerCase().replace(/\s+/g, '_');
    // Return translation or original
    return errorTranslations[snakeKey] || errorTranslations[errorKey] || errorKey;
}

// Global tag suggestions variables
let availableTags = [];
let tagSuggestionsVisible = false;

// Helper function for Turkish time formatting
function formatTurkishTime(timestamp) {
    return new Date(timestamp).toLocaleString('tr-TR', {
        day: '2-digit',
        month: '2-digit',
        hour: '2-digit',
        minute: '2-digit',
        second: '2-digit',
        timeZone: 'Europe/Istanbul'
    });
}

// Helper function for Turkish date formatting
function formatTurkishDate(timestamp) {
    return new Date(timestamp).toLocaleString('tr-TR', {
        timeZone: 'Europe/Istanbul'
    });
}

// Server metrics Alpine component
function serverMetricsApp() {
    return {
        loading: true,
        metrics: [],
        totalServers: 0,
        healthyServers: 0,
        criticalServers: 0,
        lastUpdateText: '',
        error: null,
        wsListener: null,
        destroyListener: null,
        guideModalOpen: false,
        guideTab: 'windows',

        init() {
            this.fetchMetrics();
            this.setupRealtimeListener();
            window.addEventListener('beforeunload', () => {
                this.cleanupRealtimeListener();
                this.cleanupGuideModalState();
            });

            this.destroyListener = (event) => {
                if (event.target === this.$el) {
                    this.cleanupRealtimeListener();
                    this.cleanupGuideModalState();
                    document.removeEventListener('alpine:destroy', this.destroyListener);
                }
            };
            document.addEventListener('alpine:destroy', this.destroyListener);
        },

        setupRealtimeListener() {
            if (typeof window === 'undefined') {
                return;
            }

            this.cleanupRealtimeListener();
            this.wsListener = (event) => this.handleRealtimeMetric(event.detail);
            window.addEventListener('device-metrics', this.wsListener);
        },

        cleanupRealtimeListener() {
            if (this.wsListener) {
                window.removeEventListener('device-metrics', this.wsListener);
                this.wsListener = null;
            }
        },

        openGuideModal(tab = 'windows') {
            this.guideTab = tab;
            this.guideModalOpen = true;
            document.body.classList.add('overflow-hidden');
        },

        closeGuideModal() {
            this.cleanupGuideModalState();
        },

        setGuideTab(tab) {
            this.guideTab = tab;
        },

        cleanupGuideModalState() {
            this.guideModalOpen = false;
            document.body.classList.remove('overflow-hidden');
        },

        async fetchMetrics() {
            this.loading = true;
            this.error = null;

            try {
                const token = localStorage.getItem('token') || '';

                // Fast load: Just get target list (no SNMP queries)
                const response = await fetch('/api/device-metrics/targets', {
                    headers: { 'Authorization': 'Bearer ' + token }
                });

                if (!response.ok) {
                    throw new Error('Hedef listesi yüklenemedi');
                }

                const targets = await response.json();

                // Initialize metrics array with pending status
                this.metrics = Array.isArray(targets) ? targets.map(t => ({
                    target_id: t.target_id,
                    target_name: t.target_name,
                    target_address: t.target_address,
                    status: 'pending',
                    cpu_percent: null,
                    cpu_cores: null,
                    ram_total_mb: null,
                    ram_used_mb: null,
                    ram_percent: null,
                    disk_total_gb: null,
                    disk_used_gb: null,
                    disk_percent: null,
                    uptime_seconds: null,
                    temperature_c: null
                })) : [];

                this.updateStats();

                // Immediately trigger live collection for instant results (like IP Scanner)
                console.log(`[ServerMetrics] Loaded ${this.metrics.length} targets, triggering immediate SNMP collection...`);
                if (this.metrics.length > 0) {
                    this.triggerLiveCollection();
                }

            } catch (err) {
                console.error('[ServerMetrics] Fetch error:', err);
                this.metrics = [];
                this.totalServers = 0;
                this.healthyServers = 0;
                this.criticalServers = 0;
                this.lastUpdateText = '';
                this.error = err?.message || 'Hedef listesi yüklenemedi';
            } finally {
                this.loading = false;
            }
        },

        async triggerLiveCollection() {
            // Trigger immediate SNMP collection via dedicated endpoint
            try {
                const token = localStorage.getItem('token') || '';
                const response = await fetch('/api/device-metrics/collect-now', {
                    method: 'POST',
                    headers: {
                        'Authorization': 'Bearer ' + token,
                        'Content-Type': 'application/json'
                    }
                });

                if (response.ok) {
                    console.log('[ServerMetrics] Live collection triggered successfully');
                } else {
                    console.warn('[ServerMetrics] Failed to trigger live collection');
                }
            } catch (err) {
                console.error('[ServerMetrics] Error triggering live collection:', err);
            }
        },

        handleRealtimeMetric(payload) {
            if (!payload) {
                return;
            }

            const updates = Array.isArray(payload) ? payload : [payload];
            let changed = false;

            updates.forEach(item => {
                const metric = this.normalizeMetric(item);
                if (!metric?.target_id) {
                    return;
                }

                const index = this.metrics.findIndex(existing => existing.target_id === metric.target_id);
                if (index !== -1) {
                    this.metrics.splice(index, 1, { ...this.metrics[index], ...metric });
                } else {
                    this.metrics.push(metric);
                }
                changed = true;
            });

            if (changed) {
                this.sortMetrics();
                this.updateStats();
            }
        },

        normalizeMetric(metric) {
            if (!metric) {
                return null;
            }

            const normalized = { ...metric };
            if (!normalized.collected_at && normalized.collectedAt) {
                normalized.collected_at = normalized.collectedAt;
            }
            if (!normalized.target_name && normalized.target?.name) {
                normalized.target_name = normalized.target.name;
            }
            if (!normalized.target_address && normalized.target?.address) {
                normalized.target_address = normalized.target.address;
            }
            if (!normalized.status) {
                normalized.status = 'snmp_error';
            }

            ['cpu_percent', 'ram_percent', 'disk_percent', 'cpu_cores', 'ram_total_mb', 'ram_used_mb', 'disk_total_gb', 'disk_used_gb', 'uptime_seconds', 'temperature_c'].forEach(key => {
                if (normalized[key] !== undefined && normalized[key] !== null) {
                    normalized[key] = Number(normalized[key]);
                }
            });

            return normalized;
        },

        updateStats() {
            this.totalServers = this.metrics.length;
            this.healthyServers = this.metrics.filter(item => item.status === 'success').length;
            // Only count non-pending items as critical
            const nonPending = this.metrics.filter(item => item.status !== 'pending').length;
            this.criticalServers = nonPending - this.healthyServers;

            const latestMetric = this.metrics.reduce((latest, current) => {
                if (!current?.collected_at) {
                    return latest;
                }
                const currentTime = Date.parse(current.collected_at);
                if (!Number.isFinite(currentTime)) {
                    return latest;
                }
                if (!latest) {
                    return { time: currentTime };
                }
                return currentTime > latest.time ? { time: currentTime } : latest;
            }, null);

            if (latestMetric?.time) {
                this.lastUpdateText = formatTurkishTime(latestMetric.time);
            } else {
                this.lastUpdateText = '';
            }
        },

        formatPercent(value) {
            if (value === null || value === undefined) {
                return '...';
            }
            const num = Number(value);
            if (!Number.isFinite(num)) {
                return 'N/A';
            }
            return num.toFixed(1) + '%';
        },

        formatCPUCores(cores) {
            if (cores === null || cores === undefined) {
                return '...';
            }
            const num = Number(cores);
            if (!Number.isFinite(num) || num <= 0) {
                return 'N/A';
            }
            return num + (num === 1 ? ' Core' : ' Cores');
        },

        formatRAM(usedMB, totalMB) {
            if (usedMB === null || usedMB === undefined || totalMB === null || totalMB === undefined) {
                return '...';
            }
            const used = Number(usedMB);
            const total = Number(totalMB);

            if (!Number.isFinite(used) || !Number.isFinite(total) || total <= 0) {
                return 'N/A';
            }

            const usedGB = (used / 1024).toFixed(1);
            const totalGB = (total / 1024).toFixed(1);

            return `${usedGB}/${totalGB} GB`;
        },

        formatDisk(usedGB, totalGB) {
            if (usedGB === null || usedGB === undefined || totalGB === null || totalGB === undefined) {
                return '...';
            }
            const used = Number(usedGB);
            const total = Number(totalGB);

            if (!Number.isFinite(used) || !Number.isFinite(total) || total <= 0) {
                return 'N/A';
            }

            return `${used.toFixed(0)}/${total.toFixed(0)} GB`;
        },

        formatTemperature(value) {
            if (value === null || value === undefined) {
                return '...';
            }
            const num = Number(value);
            if (!Number.isFinite(num)) {
                return 'N/A';
            }
            return `${num.toFixed(1)} C`;
        },

        formatUptime(seconds) {
            if (seconds === null || seconds === undefined) {
                return '...';
            }
            const totalSeconds = Number(seconds);
            if (!Number.isFinite(totalSeconds) || totalSeconds <= 0) {
                return 'N/A';
            }
            const days = Math.floor(totalSeconds / 86400);
            const hours = Math.floor((totalSeconds % 86400) / 3600);
            const minutes = Math.floor((totalSeconds % 3600) / 60);

            if (days > 0) {
                return `${days}g ${hours}s`;
            }
            if (hours > 0) {
                return `${hours}s ${minutes}dk`;
            }
            return `${minutes}dk`;
        },

        statusLabel(status) {
            const labels = {
                success: 'Başarılı',
                pending: 'Bekliyor...',
                timeout: 'Zaman Aşımı',
                snmp_error: 'Başarısız',
                unreachable: 'Erişilemez'
            };
            return labels[status] || status || 'Bilinmiyor';
        },

        statusClass(status) {
            if (status === 'success') {
                return 'text-green-600 dark:text-green-300';
            }
            if (status === 'pending') {
                return 'text-gray-500 dark:text-gray-400 animate-pulse';
            }
            if (status === 'timeout') {
                return 'text-yellow-600 dark:text-yellow-300';
            }
            return 'text-red-600 dark:text-red-300';
        }
    };
}

function ipScannerApp() {
    const defaultWorkers = (typeof navigator !== 'undefined' && navigator.hardwareConcurrency)
        ? Math.min(64, Math.max(4, navigator.hardwareConcurrency * 2))
        : 8;
    return {
        interfaces: [],
        selectedInterface: null,
        subnet: '',
        scanMode: 'cidr',
        rangeStart: '',
        rangeEnd: '',
        options: {
            timeoutMs: 600,
            workerCount: Math.max(1, defaultWorkers),
            maxHosts: 512,
            includeUnreachable: false,
            ports: ''
        },
        loadingDefaults: false,
        loading: false,
        devices: [],
        stats: null,
        error: null,
        selectedDevices: [],
        showAddTargetModal: false,
        showSettingsModal: false,
        hasSavedSettings: false,
        targetForm: {
            interval: 120,  // Sabit 2 dakika - kullanıcı değiştiremez
            timeout: 30000, // Sabit 30 saniye
            tags: ''
        },
        targetFormError: null,
        submittingTargets: false,
        settingsForm: {
            mode: 'range',
            subnet: '',
            rangeStart: '',
            rangeEnd: '',
            timeoutMs: 600,
            workerCount: Math.max(1, defaultWorkers),
            maxHosts: 512,
            includeUnreachable: false,
            ports: ''
        },
        settingsFormError: null,
        showSnmpTestModal: false,
        snmpTestResults: [],
        snmpTestInProgress: false,
        snmpTestProgress: 0,
        snmpTestTotal: 0,
        snmpCommunity: 'public',
        snmpVersion: 'v2c',
        selectedSnmpResults: [],

        // Inventory modal
        showAddInventoryModal: false,
        inventoryForm: {
            assetType: 'other',
            location: '',
            department: ''
        },
        inventoryFormError: null,
        submittingInventory: false,

        async init() {
            await this.fetchDefaults();
        },

        async fetchDefaults() {
            this.loadingDefaults = true;
            this.error = null;
            try {
                const response = await fetch('/api/network/ip-scanner/defaults', {
                    headers: {
                        'Authorization': 'Bearer ' + (localStorage.getItem('token') || '')
                    }
                });
                const data = await response.json();
                if (!response.ok) {
                    throw new Error(data.error || 'Arayüz bilgileri alınmadı');
                }
                if (data.interfaces) {
                    this.interfaces = data.interfaces;
                }
                if (data.default_subnet) {
                    this.subnet = data.default_subnet;
                }
                if (!this.selectedInterface && this.interfaces.length) {
                    const preferred = this.interfaces.find(iface => iface.default) || this.interfaces[0];
                    this.selectedInterface = preferred;
                    this.subnet = preferred?.cidr || this.subnet;
                }
                this.syncSettingsFormFromState();
            } catch (error) {
                this.error = error.message;
            } finally {
                this.loadingDefaults = false;
            }
        },

        async runScan() {
            if (!this.subnet && this.scanMode === 'cidr') {
                await this.fetchDefaults();
            }

            if (this.scanMode === 'cidr') {
                if (!this.subnet) {
                    this.error = 'Subnet otomatik alınmadı. Tekrar deneyin.';
                    return;
                }
            } else if (!this.rangeStart || !this.rangeEnd) {
                this.error = 'Başlangıç ve bitiş IP adresleri gerekli.';
                return;
            }

            this.loading = true;
            this.error = null;

            try {
                const payload = {
                    subnet: this.scanMode === 'cidr' ? this.subnet : '',
                    range_start: this.scanMode === 'range' ? this.rangeStart : '',
                    range_end: this.scanMode === 'range' ? this.rangeEnd : '',
                    timeout_ms: this.options.timeoutMs,
                    worker_count: this.options.workerCount,
                    max_hosts: this.options.maxHosts,
                    include_unreachable: this.options.includeUnreachable,
                    ports: this.options.ports
                };

                const response = await fetch('/api/network/ip-scanner/scan', {
                    method: 'POST',
                    headers: {
                        'Content-Type': 'application/json',
                        'Authorization': 'Bearer ' + (localStorage.getItem('token') || '')
                    },
                    body: JSON.stringify(payload)
                });

                const data = await response.json();
                if (!response.ok) {
                    throw new Error(data.error || 'Tarama başarısız oldu');
                }

                this.devices = data.devices || [];
                this.stats = {
                    subnet: data.subnet,
                    hostCount: data.host_count,
                    reachable: data.reachable_count,
                    durationMs: data.duration_ms,
                    generatedAt: data.generated_at
                };
                this.selectedDevices = [];
                this.syncSettingsFormFromState();

                if (window.Toast) {
                    Toast.show({
                        type: 'success',
                        title: 'Tarama tamamlandı',
                        message: `${this.devices.length} kayıt güncellendi.`,
                        timeout: 3000
                    });
                }
            } catch (error) {
                this.error = error.message;
            } finally {
                this.loading = false;
            }
        },

        successRate() {
            if (!this.stats || !this.stats.hostCount) {
                return 0;
            }
            return Math.round((this.stats.reachable / this.stats.hostCount) * 100);
        },

        formattedGeneratedAt() {
            if (!this.stats?.generatedAt) {
                return '-';
            }
            return formatTurkishDate(this.stats.generatedAt);
        },

        formatCheckedAt(value) {
            if (!value) {
                return '-';
            }
            return formatTurkishDate(value);
        },

        downloadJSON() {
            if (!this.stats) {
                return;
            }
            const blob = new Blob([JSON.stringify({
                stats: this.stats,
                devices: this.devices
            }, null, 2)], { type: 'application/json' });

            const url = URL.createObjectURL(blob);
            const link = document.createElement('a');
            link.href = url;
            link.download = `ip-scan-${Date.now()}.json`;
            link.click();
            URL.revokeObjectURL(url);
        },

        interfaceLabel() {
            if (!this.selectedInterface) {
                return this.interfaces.length ? 'Otomatik seçiliyor...' : 'Arayüz bulunamadı';
            }
            return `${this.selectedInterface.name} (${this.selectedInterface.ip})`;
        },

        scanProfile() {
            return `${this.options.workerCount} worker | ${this.options.timeoutMs} ms timeout | ${this.options.maxHosts} IP limiti`;
        },

        currentScopeLabel() {
            if (this.scanMode === 'range' && this.rangeStart && this.rangeEnd) {
                return `${this.rangeStart} - ${this.rangeEnd}`;
            }
            return this.subnet || '-';
        },

        isDeviceSelected(ip) {
            return this.selectedDevices.some(device => device.ip === ip);
        },

        toggleDevice(device) {
            const index = this.selectedDevices.findIndex(item => item.ip === device.ip);
            if (index !== -1) {
                this.selectedDevices.splice(index, 1);
            } else {
                this.selectedDevices.push({
                    ip: device.ip,
                    hostname: device.hostname || ''
                });
            }
        },

        toggleSelectAll(event) {
            if (!this.devices.length) {
                this.selectedDevices = [];
                return;
            }

            if (event.target.checked) {
                this.selectedDevices = this.devices.map(device => ({
                    ip: device.ip,
                    hostname: device.hostname || ''
                }));
            } else {
                this.selectedDevices = [];
            }
        },

        canCreateTargets() {
            return typeof window.app?.hasPermission === 'function'
                && window.app.hasPermission('target.create');
        },

        openAddTargetsModal() {
            if (!this.selectedDevices.length || !this.canCreateTargets()) {
                return;
            }
            this.targetFormError = null;
            this.showAddTargetModal = true;
        },

        closeAddTargetsModal() {
            if (this.submittingTargets) {
                return;
            }
            this.showAddTargetModal = false;
            this.targetFormError = null;
        },

        validateTargetForm() {
            // Artık interval ve timeout sabit olduğu için validasyon gerekmiyor
            return null;
        },

        async submitTargets() {
            const validationError = this.validateTargetForm();
            if (validationError) {
                this.targetFormError = validationError;
                return;
            }

            this.submittingTargets = true;
            this.targetFormError = null;

            try {
                const payload = {
                    devices: this.selectedDevices.map(device => ({
                        ip: device.ip,
                        hostname: device.hostname
                    })),
                    interval_sec: 120,  // Sabit 2 dakika
                    timeout_ms: 30000,  // Sabit 30 saniye
                    tags: this.targetForm.tags,
                    metrics_enabled: true,
                    snmp_community: 'public',
                    snmp_version: 'v2c'
                };

                const response = await fetch('/api/network/ip-scanner/targets', {
                    method: 'POST',
                    headers: {
                        'Content-Type': 'application/json',
                        'Authorization': 'Bearer ' + (localStorage.getItem('token') || '')
                    },
                    body: JSON.stringify(payload)
                });

                const data = await response.json().catch(() => ({}));
                if (!response.ok) {
                    throw new Error(data.error || 'Hedefler eklenemedi');
                }

                if (window.Toast) {
                    Toast.show({
                        type: 'success',
                        title: 'Hedefler eklendi',
                        message: `${data.created_count || 0} hedef oluşturuldu`,
                        timeout: 3000
                    });
                }

                this.submittingTargets = false;
                this.closeAddTargetsModal();
                this.selectedDevices = [];
            } catch (error) {
                this.targetFormError = error.message;
                this.submittingTargets = false;
            }
        },

        // Inventory functions
        openAddInventoryModal() {
            if (!this.selectedDevices.length) {
                return;
            }
            this.inventoryFormError = null;
            this.inventoryForm = {
                assetType: 'other',
                location: '',
                department: ''
            };
            this.showAddInventoryModal = true;
        },

        closeAddInventoryModal() {
            if (this.submittingInventory) {
                return;
            }
            this.showAddInventoryModal = false;
            this.inventoryFormError = null;
        },

        async submitInventory() {
            this.submittingInventory = true;
            this.inventoryFormError = null;

            try {
                const payload = {
                    devices: this.selectedDevices.map(device => {
                        const fullDevice = this.devices.find(d => d.ip === device.ip) || {};
                        return {
                            ip_address: device.ip,
                            hostname: device.hostname || null,
                            mac_address: fullDevice.mac_address || null,
                            vendor: fullDevice.vendor || null
                        };
                    }),
                    asset_type: this.inventoryForm.assetType,
                    location: this.inventoryForm.location.trim() || null,
                    department: this.inventoryForm.department.trim() || null
                };

                const response = await fetch('/api/inventory/bulk', {
                    method: 'POST',
                    headers: {
                        'Content-Type': 'application/json',
                        'Authorization': 'Bearer ' + (localStorage.getItem('token') || '')
                    },
                    body: JSON.stringify(payload)
                });

                const data = await response.json().catch(() => ({}));
                if (!response.ok) {
                    throw new Error(data.error || 'Envantere eklenemedi');
                }

                let message = `${data.created || 0} varlık oluşturuldu`;
                if (data.duplicates > 0) {
                    message += `, ${data.duplicates} zaten mevcut`;
                }

                if (window.Toast) {
                    Toast.show({
                        type: 'success',
                        title: 'Envantere eklendi',
                        message: message,
                        timeout: 3000
                    });
                }

                this.submittingInventory = false;
                this.closeAddInventoryModal();
                this.selectedDevices = [];
            } catch (error) {
                this.inventoryFormError = error.message;
                this.submittingInventory = false;
            }
        },

        syncSettingsFormFromState() {
            this.settingsForm = {
                mode: this.hasSavedSettings ? this.scanMode : (this.settingsForm.mode || 'range'),
                subnet: this.subnet,
                rangeStart: this.rangeStart,
                rangeEnd: this.rangeEnd,
                timeoutMs: this.options.timeoutMs,
                workerCount: this.options.workerCount,
                maxHosts: this.options.maxHosts,
                includeUnreachable: this.options.includeUnreachable,
                ports: this.options.ports || ''
            };
        },

        openSettingsModal() {
            this.settingsFormError = null;
            if (this.hasSavedSettings) {
                this.syncSettingsFormFromState();
            }
            this.showSettingsModal = true;
            if (typeof document !== 'undefined') {
                document.body.classList.add('overflow-hidden');
            }
        },

        closeSettingsModal() {
            this.showSettingsModal = false;
            this.settingsFormError = null;
            if (typeof document !== 'undefined') {
                document.body.classList.remove('overflow-hidden');
            }
        },

        validateSettingsForm() {
            if (this.settingsForm.mode === 'cidr') {
                const subnet = (this.settingsForm.subnet || '').trim();
                if (!subnet) {
                    return 'Subnet zorunlu';
                }
            } else {
                if (!this.isValidIPv4(this.settingsForm.rangeStart)) {
                    return 'Başlangıç IP adresi geçerli değil';
                }
                if (!this.isValidIPv4(this.settingsForm.rangeEnd)) {
                    return 'Bitiş IP adresi geçerli değil';
                }
            }
            if (this.settingsForm.timeoutMs < 200 || this.settingsForm.timeoutMs > 5000) {
                return 'Timeout 200-5000 ms arasında olmalıdır';
            }
            if (this.settingsForm.workerCount < 1 || this.settingsForm.workerCount > 128) {
                return 'Worker 1-128 arasında olmalıdır';
            }
            if (this.settingsForm.maxHosts < 16 || this.settingsForm.maxHosts > 4096) {
                return 'IP limiti 16-4096 arasında olmalıdır';
            }
            return null;
        },

        saveSettings() {
            const error = this.validateSettingsForm();
            if (error) {
                this.settingsFormError = error;
                return;
            }

            this.scanMode = this.settingsForm.mode;
            if (this.scanMode === 'cidr') {
                this.subnet = this.settingsForm.subnet.trim();
                this.rangeStart = '';
                this.rangeEnd = '';
            } else {
                this.rangeStart = this.settingsForm.rangeStart.trim();
                this.rangeEnd = this.settingsForm.rangeEnd.trim();
            }

            this.options.timeoutMs = Math.min(5000, Math.max(200, this.settingsForm.timeoutMs));
            this.options.workerCount = Math.min(128, Math.max(1, this.settingsForm.workerCount));
            this.options.maxHosts = Math.min(4096, Math.max(16, this.settingsForm.maxHosts));
            this.options.includeUnreachable = !!this.settingsForm.includeUnreachable;
            this.options.ports = (this.settingsForm.ports || '').trim();
            this.hasSavedSettings = true;
            this.closeSettingsModal();

            if (window.Toast) {
                Toast.show({
                    type: 'success',
                    title: 'Ayarlar güncellendi',
                    message: 'Yeni tarama profili kaydedildi.',
                    timeout: 2500
                });
            }
        },

        openSnmpTestModal() {
            if (!this.selectedDevices.length) {
                Toast.error('Lütfen en az bir cihaz seçin');
                return;
            }
            this.showSnmpTestModal = true;
            this.snmpTestResults = [];
            this.selectedSnmpResults = [];
        },

        closeSnmpTestModal() {
            this.showSnmpTestModal = false;
        },

        async runSnmpTest() {
            if (!this.selectedDevices.length) {
                Toast.error('Lütfen en az bir cihaz seçin');
                return;
            }

            this.snmpTestInProgress = true;
            this.snmpTestProgress = 0;
            this.snmpTestTotal = this.selectedDevices.length;
            this.snmpTestResults = [];
            this.selectedSnmpResults = [];

            try {
                const response = await fetch('/api/snmp/test', {
                    method: 'POST',
                    headers: {
                        'Content-Type': 'application/json',
                        'Authorization': 'Bearer ' + (localStorage.getItem('token') || '')
                    },
                    body: JSON.stringify({
                        devices: this.selectedDevices.map(d => ({
                            ip: d.ip,
                            hostname: d.hostname || d.ip
                        })),
                        community: this.snmpCommunity,
                        version: this.snmpVersion
                    })
                });

                const data = await response.json();

                if (!response.ok) {
                    throw new Error(data.error || 'SNMP testi başarısız');
                }

                this.snmpTestResults = data.results || [];
                this.snmpTestProgress = data.tested_count;

                Toast.show({
                    type: 'success',
                    title: 'SNMP Test Tamamlandı',
                    message: `${data.accessible_count} / ${data.tested_count} cihaz erişilebilir`,
                    timeout: 3000
                });

            } catch (error) {
                Toast.error('SNMP testi hatası: ' + error.message);
                console.error('SNMP test error:', error);
            } finally {
                this.snmpTestInProgress = false;
            }
        },

        toggleSelectAllSnmpResults(event) {
            const checked = event.target.checked;
            if (checked) {
                this.selectedSnmpResults = this.snmpTestResults
                    .filter(r => r.accessible)
                    .map(r => r.target_id);
            } else {
                this.selectedSnmpResults = [];
            }
        },

        async enableMetricsForSelected() {
            if (!this.selectedSnmpResults.length) {
                Toast.error('Lütfen en az bir cihaz seçin');
                return;
            }

            try {
                // Get selected devices details
                const selectedDevicesDetails = this.snmpTestResults.filter(r =>
                    this.selectedSnmpResults.includes(r.target_id) && r.accessible
                );

                // First, create targets for devices that don't have IDs yet (negative IDs)
                const devicesNeedingTargets = selectedDevicesDetails.filter(d => d.target_id < 0);
                let createdTargetIds = [];

                if (devicesNeedingTargets.length > 0) {
                    const createResponse = await fetch('/api/network/ip-scanner/create-targets', {
                        method: 'POST',
                        headers: {
                            'Content-Type': 'application/json',
                            'Authorization': 'Bearer ' + (localStorage.getItem('token') || '')
                        },
                        body: JSON.stringify({
                            devices: devicesNeedingTargets.map(d => ({
                                ip: d.ip,
                                hostname: d.name || d.ip
                            })),
                            interval_sec: 120,
                            timeout_ms: 30000,
                            tags: ''
                        })
                    });

                    const createData = await createResponse.json();
                    if (!createResponse.ok) {
                        throw new Error(createData.error || 'Target oluşturma başarısız');
                    }

                    createdTargetIds = createData.target_ids || [];
                }

                // Combine existing target IDs and newly created target IDs
                const existingTargetIds = selectedDevicesDetails
                    .filter(d => d.target_id > 0)
                    .map(d => d.target_id);
                const allTargetIds = [...existingTargetIds, ...createdTargetIds];

                // Now enable metrics for all targets
                const response = await fetch('/api/targets/enable-metrics', {
                    method: 'POST',
                    headers: {
                        'Content-Type': 'application/json',
                        'Authorization': 'Bearer ' + (localStorage.getItem('token') || '')
                    },
                    body: JSON.stringify({
                        target_ids: allTargetIds,
                        community: this.snmpCommunity,
                        version: this.snmpVersion
                    })
                });

                const data = await response.json();

                if (!response.ok) {
                    throw new Error(data.error || 'Metrik aktifleştirme başarısız');
                }

                Toast.show({
                    type: 'success',
                    title: 'Başarılı',
                    message: data.message || `${data.updated_count} cihaz için SNMP aktifleştirildi`,
                    timeout: 3000
                });

                this.closeSnmpTestModal();

            } catch (error) {
                Toast.error('Aktifleştirme hatası: ' + error.message);
                console.error('Enable metrics error:', error);
            }
        },

        isValidIPv4(ip) {
            if (!ip) {
                return false;
            }
            const parts = ip.trim().split('.');
            if (parts.length !== 4) {
                return false;
            }
            return parts.every(part => {
                if (!/^\d{1,3}$/.test(part)) {
                    return false;
                }
                const num = Number(part);
                return num >= 0 && num <= 255;
            });
        }
    };
}
function inventoryApp() {
    return {
        items: [],
        stats: {},
        locations: [],
        total: 0,
        limit: 50,
        offset: 0,
        loading: false,
        error: null,

        filters: {
            search: '',
            type: 'all',
            status: 'all',
            location: 'all'
        },

        showFormModal: false,
        editingItem: null,
        submitting: false,
        formError: null,
        form: {
            asset_name: '',
            asset_tag: '',
            asset_type: 'other',
            status: 'active',
            ip_address: '',
            mac_address: '',
            hostname: '',
            vendor: '',
            brand: '',
            model: '',
            serial_number: '',
            location: '',
            department: '',
            assigned_to: '',
            purchase_date: '',
            warranty_expiry: '',
            purchase_cost: '',
            notes: ''
        },

        showDeleteModal: false,
        deletingItem: null,
        deleting: false,
        showDetailModal: false,
        detailItem: null,
        detailTab: 'overview',
        targetDetails: null,
        targetLoading: false,
        targetError: null,
        // Yazılım lisans yönetimi
        softwareLicenses: [],
        licenseOpenIdx: null,
        crackFindingsOpenIdx: null,
        licenseFormSoftware: '',
        licenseFormLicenseId: null,
        licenseFormSaving: false,
        complianceScanning: false,
        licenseFormData: { purchase_cost: '', cost_currency: 'TRY', purchase_date: '', license_expiry_date: '', notes: '' },
        showGuideModal: false,
        guideTab: 'windows',
        guideCopied: null,
        winrmSetupScript: [
            "# Ag profilini Private'a cevir (Public profil WinRM'i kisitlar)",
            "Get-NetConnectionProfile | Where-Object NetworkCategory -eq 'Public' | Set-NetConnectionProfile -NetworkCategory Private",
            "",
            "# WinRM'i etkinlestir ve yapilandir",
            "winrm quickconfig -force",
            'winrm set winrm/config/service "@{AllowUnencrypted=`"true`"}"',
            'winrm set winrm/config/service/auth "@{Basic=`"true`"}"',
            "Set-Item WSMan:\\localhost\\Client\\TrustedHosts -Value '*' -Force",
            "",
            "# Guvenlik duvari kurali (zaten varsa hata vermez)",
            "New-NetFirewallRule -DisplayName 'SysTrack-WinRM' -Direction Inbound -Protocol TCP -LocalPort 5985 -Action Allow -Profile Any -ErrorAction SilentlyContinue",
            "",
            "Restart-Service WinRM -Force",
            "Write-Host 'Hazir! SysTrack artik bu cihazi tarayabilir.' -ForegroundColor Green"
        ].join('\n'),
        sshSetupScript: [
            "# SSH servis durumunu kontrol et",
            "systemctl status ssh || systemctl status sshd",
            "",
            "# Kurulu degilse kur (Ubuntu/Debian)",
            "sudo apt-get install -y openssh-server",
            "",
            "# Kurulu degilse kur (RHEL/CentOS/Rocky)",
            "sudo yum install -y openssh-server",
            "",
            "# Servisi baslat ve otomatik baslatmaya al",
            "sudo systemctl enable --now ssh",
            "",
            "# Guvenlik duvari (ufw varsa)",
            "sudo ufw allow 22/tcp"
        ].join('\n'),

        copyGuideScript(key) {
            const text = key === 'winrm' ? this.winrmSetupScript : this.sshSetupScript;
            navigator.clipboard.writeText(text).then(() => {
                this.guideCopied = key;
                setTimeout(() => { this.guideCopied = null; }, 2000);
            });
        },

        showReportModal: false,
        reportGenerating: false,
        inventoryReportForm: {
            scope: 'filtered', // filtered | all | by_user | risk | changes
            reportType: 'inventory', // inventory | by_user | risk | changes
            selectedUser: '',
            changeDays: 30,
        },
        reportUsers: [],
        // Yazılım değişim geçmişi
        softwareChanges: [],       // son taramadan bu yana değişimler (by name key)
        softwareChangesMap: {},    // { 'software name lowercase': change_type }
        scanSettings: { auto_software_scan: false, scan_hour: 3, last_auto_scan_at: null },
        scanSettingsSaving: false,

        async init() {
            await Promise.all([
                this.loadStats(),
                this.loadLocations(),
                this.loadItems()
            ]);
        },

        openReportModal() {
            // Seçili varlık varsa scope'u otomatik olarak 'selected' yap
            const hasSelection = this.selectedItems && this.selectedItems.length > 0;
            this.inventoryReportForm = {
                scope: hasSelection ? 'selected' : 'filtered',
                reportType: 'inventory',
                selectedUser: '',
                userScope: 'all',   // 'all' | 'single'
                changeDays: 30,
            };
            this.showReportModal = true;
            this.loadReportUsers();
        },

        closeReportModal() {
            if (this.reportGenerating) return;
            this.showReportModal = false;
        },

        async fetchInventoryForReport(reportType) {
            const token = localStorage.getItem('token');
            const scope = this.inventoryReportForm.scope;

            // Seçili varlıklar bazında rapor — zaten listede var, direkt kullan
            if (scope === 'selected' && this.selectedItems?.length > 0) {
                return this.items.filter(i => this.selectedItems.includes(i.id));
            }

            const allItems = [];
            const limit = 500;
            let offset = 0;
            let total = 0;

            do {
                const params = new URLSearchParams({ limit: String(limit), offset: String(offset) });

                if (scope === 'filtered') {
                    if (this.filters.search) params.append('search', this.filters.search);
                    if (this.filters.type !== 'all') params.append('type', this.filters.type);
                    if (this.filters.status !== 'all') params.append('status', this.filters.status);
                    if (this.filters.location !== 'all') params.append('location', this.filters.location);
                }

                // Kullanıcı bazlı rapor: belirli kullanıcı seçildiyse filtrele
                if (reportType === 'by_user' && this.inventoryReportForm.userScope === 'single' && this.inventoryReportForm.selectedUser) {
                    params.append('assigned_to', this.inventoryReportForm.selectedUser);
                }

                const res = await fetch('/api/inventory?' + params.toString(), {
                    headers: { 'Authorization': 'Bearer ' + token }
                });
                if (!res.ok) throw new Error('Rapor için envanter verisi alınamadı');

                const data = await res.json();
                const items = Array.isArray(data.items) ? data.items : [];
                total = Number(data.total || 0);
                allItems.push(...items);
                offset += items.length;
                if (items.length === 0) break;
            } while (offset < total);

            return allItems;
        },

        escapeHtml(value) {
            return String(value ?? '')
                .replace(/&/g, '&amp;')
                .replace(/</g, '&lt;')
                .replace(/>/g, '&gt;')
                .replace(/"/g, '&quot;')
                .replace(/'/g, '&#39;');
        },

        getReportOwner(item) {
            return item?.owner_json?.display_name || item?.assigned_to || '-';
        },

        getReportOS(item) {
            return item?.ad_os_name || item?.winrm_json?.os?.Caption || item?.ssh_json?.os_pretty_name || item?.snmp_json?.sysDescr || '-';
        },

        getReportCPU(item) {
            return item?.winrm_json?.cpu?.Name || item?.ssh_json?.cpu_model || item?.snmp_json?.cpu_load_avg_percent || '-';
        },

        getReportRAM(item) {
            return item?.winrm_json?.total_ram_gb || item?.ssh_json?.total_ram_gb || item?.snmp_json?.total_ram_gb || '-';
        },

        getReportDisk(item) {
            if (item?.winrm_json) return this.getWinRMDiskTotalLabel(item.winrm_json);
            if (item?.ssh_json?.disk_total_gb) return this.formatDiskTotalGB(Number(item.ssh_json.disk_total_gb));
            if (item?.snmp_json?.disk_total_gb) return item.snmp_json.disk_total_gb;
            return '-';
        },

        getReportLastBoot(item) {
            if (item?.winrm_json) return this.getWinRMLastBootLabel(item.winrm_json);
            return item?.ssh_json?.last_boot_time || '-';
        },

        getReportGpu(item) {
            if (item?.winrm_json) return this.getWinRMDevicePreview(item.winrm_json, 'video_controllers', 2);
            return '-';
        },

        getReportPeripheralSummary(item) {
            const winrm = item?.winrm_json;
            if (!winrm) return '-';
            const monitor = this.getWinRMDeviceCount(winrm, 'monitor_devices') || this.getWinRMDeviceCount(winrm, 'monitors');
            const keyboard = this.getWinRMDeviceCount(winrm, 'keyboards');
            const mouse = this.getWinRMDeviceCount(winrm, 'mice');
            const bt = this.getWinRMDeviceCount(winrm, 'bluetooth_devices');
            const total = monitor + keyboard + mouse + bt;
            if (!total) return '-';
            return `Monitör:${monitor} Klavye:${keyboard} Fare:${mouse} BT:${bt}`;
        },

        buildInventoryReportSummary(items) {
            const byStatus = { active: 0, maintenance: 0, storage: 0, faulty: 0, retired: 0 };
            for (const item of items) {
                if (byStatus[item.status] !== undefined) byStatus[item.status] += 1;
            }
            return {
                total: items.length,
                byStatus
            };
        },

        downloadInventoryCSV(items) {
            const headers = [
                'Varlik Adi', 'Durum', 'Tur', 'IP', 'MAC', 'Hostname',
                'Uretici', 'Model', 'Seri No', 'Sorumlu', 'Departman', 'Lokasyon',
                'Isletim Sistemi', 'CPU', 'RAM', 'Disk Toplam', 'Kaynak'
            ];
            const rows = items.map((item) => [
                item.asset_name || '',
                this.getStatusLabel(item.status || ''),
                this.getAssetTypeLabel(item.asset_type || ''),
                item.ip_address || '',
                item.mac_address || '',
                item.hostname || '',
                item.vendor || item.brand || '',
                item.model || '',
                item.serial_number || '',
                this.getReportOwner(item),
                item.department || '',
                item.location || '',
                this.getReportOS(item),
                this.getReportCPU(item),
                this.getReportRAM(item),
                this.getReportDisk(item),
                this.getSourceLabel(item.source || 'manual')
            ]);

            const toCell = (v) => `"${String(v ?? '').replace(/"/g, '""')}"`;
            const content = [headers, ...rows].map((r) => r.map(toCell).join(';')).join('\n');
            const blob = new Blob(['\uFEFF' + content], { type: 'text/csv;charset=utf-8;' });
            const url = URL.createObjectURL(blob);
            const a = document.createElement('a');
            a.href = url;
            a.download = `envanter_raporu_${new Date().toISOString().slice(0, 10)}.csv`;
            document.body.appendChild(a);
            a.click();
            document.body.removeChild(a);
            URL.revokeObjectURL(url);
        },

        downloadInventoryJSON(items, summary) {
            const payload = {
                generated_at: new Date().toISOString(),
                scope: this.inventoryReportForm.scope,
                filters: { ...this.filters },
                summary,
                items
            };
            const blob = new Blob([JSON.stringify(payload, null, 2)], { type: 'application/json;charset=utf-8' });
            const url = URL.createObjectURL(blob);
            const a = document.createElement('a');
            a.href = url;
            a.download = `envanter_raporu_${new Date().toISOString().slice(0, 10)}.json`;
            document.body.appendChild(a);
            a.click();
            document.body.removeChild(a);
            URL.revokeObjectURL(url);
        },

        openPrintableInventoryReport(items, summary, reportType) {
            const hasValue = (value) => {
                if (value === null || value === undefined) return false;
                const text = String(value).trim();
                return text !== '' && text !== '-';
            };
            const line = (label, value) => hasValue(value)
                ? `<p><b>${this.escapeHtml(label)}:</b> ${this.escapeHtml(value)}</p>`
                : '';
            const renderGroup = (title, lines) => {
                const content = lines.filter(Boolean).join('');
                if (!content) return '';
                return `<div class="group"><h4>${this.escapeHtml(title)}</h4>${content}</div>`;
            };

            const renderCard = (item) => {
                const vendorModel = [item.vendor || item.brand, item.model].filter(Boolean).join(' / ');
                const ownerName = this.getReportOwner(item);
                const ownerMail = item?.owner_json?.mail;
                const ownerDepartment = item?.owner_json?.department || item.department;
                const ownerTitle = item?.owner_json?.title;
                const cardMeta = [this.getAssetTypeLabel(item.asset_type || ''), item.asset_tag].filter(hasValue).join(' · ');
                const identityGroup = renderGroup('Kimlik ve Ağ', [
                    line('IP', item.ip_address), line('MAC', item.mac_address),
                    line('Hostname', item.hostname), line('Seri No', item.serial_number),
                    line('Üretici / Model', vendorModel), line('Lokasyon', item.location)
                ]);
                const orgGroup = renderGroup('Sorumlu ve Organizasyon', [
                    line('Sorumlu', ownerName), line('E-posta', ownerMail),
                    line('Departman', ownerDepartment), line('Ünvan', ownerTitle)
                ]);
                const hwGroup = renderGroup('Donanım Özeti', [
                    line('İşletim Sistemi', this.getReportOS(item)),
                    line('CPU', this.getReportCPU(item)), line('RAM', this.getReportRAM(item)),
                    line('Disk', this.getReportDisk(item)), line('GPU', this.getReportGpu(item)),
                    line('Çevre Birimleri', this.getReportPeripheralSummary(item)),
                    line('Son Yeniden Başlatma', this.getReportLastBoot(item))
                ]);
                return `<article class="asset-card">
                    <header class="asset-head">
                        <div>
                            <h3>${this.escapeHtml(item.asset_name || 'Varlık')}</h3>
                            ${hasValue(cardMeta) ? `<p>${this.escapeHtml(cardMeta)}</p>` : ''}
                        </div>
                        <div class="badge-row">
                            <span class="badge status-${this.escapeHtml(item.status || 'active')}">${this.escapeHtml(this.getStatusLabel(item.status || ''))}</span>
                        </div>
                    </header>
                    <div class="asset-grid">${identityGroup}${orgGroup}${hwGroup}</div>
                </article>`;
            };

            // Kullanıcı bazlı + tüm kullanıcılar → kişiye göre gruplandır
            let cards = '';
            let reportTitle = 'Genel Envanter Raporu';
            let reportMeta = `Oluşturulma: ${new Date().toLocaleString('tr-TR')}`;

            if (reportType === 'by_user' && this.inventoryReportForm.userScope === 'all') {
                reportTitle = 'Kullanıcı Bazlı Envanter Raporu';
                reportMeta = `Tüm kullanıcılar · ${new Date().toLocaleString('tr-TR')}`;
                // Kullanıcıya göre grupla
                const grouped = {};
                const noUser = [];
                for (const item of items) {
                    const key = this.getReportOwner(item);
                    if (key === '-' || !key) { noUser.push(item); continue; }
                    if (!grouped[key]) grouped[key] = [];
                    grouped[key].push(item);
                }
                for (const [user, uItems] of Object.entries(grouped).sort()) {
                    cards += `<section class="user-section">
                        <h2 class="user-heading"><span class="user-icon">👤</span>${this.escapeHtml(user)} <span class="user-count">${uItems.length} varlık</span></h2>
                        ${uItems.map(renderCard).join('')}
                    </section>`;
                }
                if (noUser.length) {
                    cards += `<section class="user-section">
                        <h2 class="user-heading" style="color:#94a3b8"><span class="user-icon">—</span>Atanmamış <span class="user-count">${noUser.length} varlık</span></h2>
                        ${noUser.map(renderCard).join('')}
                    </section>`;
                }
            } else if (reportType === 'by_user' && this.inventoryReportForm.userScope === 'single') {
                reportTitle = 'Kullanıcı Bazlı Envanter Raporu';
                const uName = this.inventoryReportForm.selectedUser;
                reportMeta = `Kullanıcı: ${this.escapeHtml(uName)} · ${new Date().toLocaleString('tr-TR')}`;
                cards = items.map(renderCard).join('');
            } else {
                const scopeLabel = { selected: 'Seçili varlıklar', filtered: 'Aktif filtre', all: 'Tüm envanter' }[this.inventoryReportForm.scope] || '';
                reportMeta = `${scopeLabel} · ${new Date().toLocaleString('tr-TR')}`;
                cards = items.map(renderCard).join('');
            }

            const html = `<!doctype html>
<html lang="tr"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><title>Envanter Raporu</title>
<style>
:root{--ink:#0f172a;--muted:#64748b;--line:#dbe4ef;--bg:#f2f6fb;--card:#ffffff;--brand:#0f4c81;--brand-soft:#e6f0fb}
*{box-sizing:border-box} body{font-family:Segoe UI,Arial,sans-serif;margin:0;color:var(--ink);background:var(--bg)}
.wrap{max-width:1200px;margin:24px auto;padding:0 18px}
.pdf-fab{position:fixed;right:18px;bottom:18px;z-index:50;display:inline-flex;align-items:center;gap:8px;background:#0f4c81;color:#fff;border:none;border-radius:999px;padding:10px 16px;font-size:13px;font-weight:700;box-shadow:0 10px 24px rgba(15,76,129,.28);cursor:pointer}
.pdf-fab:hover{background:#0b3b64}
.top{background:linear-gradient(140deg,#edf4ff,#ffffff 48%,#eefbf6);border:1px solid var(--line);border-radius:16px;padding:20px 22px}
.title{font-size:28px;font-weight:800;letter-spacing:.2px;margin:0 0 6px}.meta{color:var(--muted);font-size:12px}
.sum{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:10px;margin-top:14px}
.sum .k{background:#fff;border:1px solid var(--line);border-radius:12px;padding:10px 12px;display:flex;align-items:center;justify-content:space-between;gap:12px;box-shadow:0 2px 10px rgba(15,23,42,.04)}.sum .l{font-size:11px;color:var(--muted);text-transform:uppercase}
.sum .v{font-size:24px;font-weight:800;margin-top:0;line-height:1;text-align:right}
.asset-card{margin-top:14px;background:var(--card);border:1px solid var(--line);border-radius:14px;overflow:hidden;page-break-inside:avoid}
.asset-head{display:flex;justify-content:space-between;gap:12px;padding:14px 16px;background:#ffffff;border-bottom:1px solid var(--line)}
.asset-head h3{margin:0;font-size:18px}.asset-head p{margin:4px 0 0;color:var(--muted);font-size:12px}
.badge-row{display:flex;gap:8px;flex-wrap:wrap;align-content:flex-start}
.badge{display:inline-block;border:1px solid var(--line);border-radius:999px;padding:4px 10px;font-size:11px;background:#fff;font-weight:700}
.status-active{background:#dcfce7;border-color:#86efac}.status-maintenance{background:#fef3c7;border-color:#fcd34d}
.status-storage{background:#dbeafe;border-color:#93c5fd}.status-faulty{background:#fee2e2;border-color:#fca5a5}
.status-retired{background:#e5e7eb;border-color:#cbd5e1}
.asset-grid{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:10px;padding:12px}
.group{border:1px solid var(--line);border-radius:10px;padding:10px;background:#fff}.group h4{margin:0 0 8px;font-size:12px;text-transform:uppercase;color:var(--brand);letter-spacing:.2px}
.group p{margin:4px 0;font-size:12px;line-height:1.45;word-break:break-word}.group b{color:#334155}
@media (max-width:980px){
  .sum{grid-template-columns:repeat(2,minmax(0,1fr))}
  .asset-grid{grid-template-columns:1fr}
}
@media (max-width:640px){
  .wrap{margin:14px auto;padding:0 12px}
  .top{padding:14px}
  .title{font-size:22px}
  .asset-head{flex-direction:column;align-items:flex-start}
  .sum{grid-template-columns:1fr}
}
.user-section{margin-top:28px}
.user-heading{display:flex;align-items:center;gap:10px;font-size:16px;font-weight:700;color:#1e3a5f;margin:0 0 10px;padding:10px 14px;background:linear-gradient(90deg,#e8f0fe,#f5f8ff);border-left:4px solid #4f7dd8;border-radius:0 10px 10px 0}
.user-icon{font-size:18px}.user-count{margin-left:auto;font-size:12px;font-weight:500;color:#64748b;background:#fff;border:1px solid #dbe4ef;border-radius:999px;padding:2px 10px}
@media print{
  body{background:#fff}.wrap{max-width:none;margin:0;padding:0}
  .asset-grid{grid-template-columns:1fr 1fr 1fr}
  .top{border-radius:0;border-left:none;border-right:none}
  .asset-card{break-inside:avoid-page}
  .user-section{break-before:page}
  .pdf-fab{display:none !important}
}
</style></head><body>
<button class="pdf-fab" onclick="window.print()">📄 PDF Oluştur</button>
<main class="wrap">
  <section class="top">
    <h1 class="title">${reportTitle}</h1>
    <div class="meta">${reportMeta}</div>
    <div class="sum">
      <div class="k"><div class="l">Toplam</div><div class="v">${summary.total}</div></div>
      <div class="k"><div class="l">Aktif</div><div class="v">${summary.byStatus.active}</div></div>
      <div class="k"><div class="l">Bakımda</div><div class="v">${summary.byStatus.maintenance}</div></div>
      <div class="k"><div class="l">Depoda</div><div class="v">${summary.byStatus.storage}</div></div>
      <div class="k"><div class="l">Arızalı</div><div class="v">${summary.byStatus.faulty}</div></div>
      <div class="k"><div class="l">Emekli</div><div class="v">${summary.byStatus.retired}</div></div>
    </div>
  </section>
  ${cards}
</main>
</body></html>`;

            const win = window.open('', '_blank');
            if (!win) throw new Error('Tarayıcı rapor penceresini engelledi');
            win.document.open();
            win.document.write(html);
            win.document.close();
            win.focus();
        },

        async generateInventoryReport() {
            this.reportGenerating = true;
            try {
                const type = this.inventoryReportForm.reportType || 'inventory';

                if (type === 'inventory' || type === 'by_user') {
                    const items = await this.fetchInventoryForReport(type);
                    if (!items.length) throw new Error('Seçili kapsam için kayıt bulunamadı.');
                    const summary = this.buildInventoryReportSummary(items);
                    this.openPrintableInventoryReport(items, summary, type);
                    window.SYS?.showToast?.(`Rapor hazır — ${items.length} varlık.`, 'success');

                } else if (type === 'risk') {
                    const items = await this.fetchInventoryForReport('risk');
                    this.openRiskReport(items);
                    window.SYS?.showToast?.(`Risk raporu hazır — ${items.length} varlık.`, 'success');

                } else if (type === 'changes') {
                    const days = this.inventoryReportForm.changeDays || 30;
                    const data = await this.fetchSoftwareChangesReport(days);
                    this.openChangesReport(data, days);
                    const total = data.reduce((s, d) => s + d.changes.length, 0);
                    window.SYS?.showToast?.(`Değişim raporu hazır — ${total} kayıt.`, 'success');
                }
            } catch (error) {
                console.error('Inventory report error:', error);
                window.SYS?.showToast?.(error.message || 'Rapor oluşturulamadı', 'error');
            } finally {
                this.reportGenerating = false;
            }
        },

        async fetchSoftwareChangesReport(days) {
            const token = localStorage.getItem('token');
            // Önce tüm cihazları çek (inventory listesi)
            const items = await this.fetchInventoryForReport('all');
            // Her cihaz için değişimleri paralel çek (max 10 eş zamanlı)
            const results = [];
            const batch = 10;
            for (let i = 0; i < items.length; i += batch) {
                const chunk = items.slice(i, i + batch);
                const fetched = await Promise.all(chunk.map(async (item) => {
                    try {
                        const r = await fetch(`/api/inventory/${item.id}/software-changes?days=${days}`, {
                            headers: { 'Authorization': 'Bearer ' + token }
                        });
                        if (!r.ok) return null;
                        const changes = await r.json();
                        if (!changes.length) return null;
                        return { item, changes };
                    } catch { return null; }
                }));
                results.push(...fetched.filter(Boolean));
            }
            return results;
        },

        openRiskReport(items) {
            const esc = (v) => this.escapeHtml(v);
            const riskItems = items.filter(it => it.crack_risk_level && it.crack_risk_level !== 'clean' && it.crack_findings?.length);
            const now = new Date().toLocaleString('tr-TR');

            const cards = riskItems.map(item => {
                const riskLabel = { high: 'Yüksek', medium: 'Orta', low: 'Düşük' }[item.crack_risk_level] || item.crack_risk_level;
                const riskColor = { high: '#dc2626', medium: '#d97706', low: '#059669' }[item.crack_risk_level] || '#6b7280';
                const findings = (item.crack_findings || []).map(f => `
                    <tr>
                        <td style="padding:6px 8px;border-bottom:1px solid #f0f0f0">${esc(f.title)}</td>
                        <td style="padding:6px 8px;border-bottom:1px solid #f0f0f0;color:#6b7280;font-size:11px">${esc(f.description || '')}</td>
                        <td style="padding:6px 8px;border-bottom:1px solid #f0f0f0;text-align:center;font-size:11px">${f.confidence ? f.confidence + '%' : '-'}</td>
                    </tr>`).join('');
                return `<div class="card">
                    <div style="display:flex;align-items:center;justify-content:space-between;margin-bottom:10px">
                        <strong style="font-size:15px">${esc(item.asset_name || item.hostname || item.ip_address || 'Bilinmiyor')}</strong>
                        <span style="background:${riskColor};color:white;border-radius:6px;padding:2px 10px;font-size:12px;font-weight:700">${riskLabel} Risk</span>
                    </div>
                    <div style="font-size:12px;color:#6b7280;margin-bottom:8px">IP: ${esc(item.ip_address || '-')} &bull; ${esc(item.department || item.location || '-')} &bull; ${esc(this.getReportOwner(item))}</div>
                    <table style="width:100%;border-collapse:collapse;font-size:12px">
                        <thead><tr style="background:#fef2f2"><th style="padding:6px 8px;text-align:left;font-weight:600">Tespit</th><th style="padding:6px 8px;text-align:left;font-weight:600">Açıklama</th><th style="padding:6px 8px;text-align:center;font-weight:600">Kesinlik</th></tr></thead>
                        <tbody>${findings}</tbody>
                    </table>
                </div>`;
            }).join('');

            const cleanCount = items.filter(it => it.crack_risk_level === 'clean').length;
            const html = `<!DOCTYPE html><html lang="tr"><head><meta charset="utf-8"><title>Risk ve Uyumluluk Raporu</title>
<style>*{box-sizing:border-box;margin:0;padding:0}body{font-family:system-ui,sans-serif;background:#fff;padding:24px;color:#111}
.wrap{max-width:900px;margin:0 auto}.title{font-size:22px;font-weight:700;color:#111;margin-bottom:4px}
.meta{font-size:12px;color:#6b7280;margin-bottom:18px}.sum{display:flex;gap:12px;margin-bottom:24px;flex-wrap:wrap}
.k{background:#f9fafb;border:1px solid #e5e7eb;border-radius:8px;padding:10px 16px;min-width:100px}
.l{font-size:11px;color:#6b7280;font-weight:500}.v{font-size:20px;font-weight:700;color:#111}
.card{border:1px solid #e5e7eb;border-radius:12px;padding:16px;margin-bottom:16px;break-inside:avoid}
@media print{.no-print{display:none}body{padding:0}.wrap{max-width:100%}}</style></head><body>
<div class="wrap">
    <div class="no-print" style="margin-bottom:16px"><button onclick="window.print()" style="background:#4f46e5;color:#fff;border:none;padding:8px 18px;border-radius:8px;cursor:pointer;font-size:13px">PDF Oluştur</button></div>
    <h1 class="title">Risk ve Uyumluluk Raporu</h1>
    <div class="meta">Oluşturulma: ${now}</div>
    <div class="sum">
        <div class="k"><div class="l">Toplam Cihaz</div><div class="v">${items.length}</div></div>
        <div class="k"><div class="l">Riskli Cihaz</div><div class="v" style="color:#dc2626">${riskItems.length}</div></div>
        <div class="k"><div class="l">Temiz</div><div class="v" style="color:#059669">${cleanCount}</div></div>
        <div class="k"><div class="l">Taranmamış</div><div class="v" style="color:#6b7280">${items.length - cleanCount - riskItems.length}</div></div>
    </div>
    ${riskItems.length ? cards : '<div style="text-align:center;padding:40px 0;color:#059669;font-size:16px">Tespit edilen risk bulunmamaktadır.</div>'}
</div></body></html>`;
            const win = window.open('', '_blank');
            if (!win) throw new Error('Tarayıcı rapor penceresini engelledi');
            win.document.open(); win.document.write(html); win.document.close(); win.focus();
        },

        openChangesReport(data, days) {
            const esc = (v) => this.escapeHtml(v);
            const now = new Date().toLocaleString('tr-TR');
            const changeTypeLabel = { added: 'Kuruldu', removed: 'Kaldırıldı', version_changed: 'Güncellendi' };
            const changeTypeColor = { added: '#059669', removed: '#dc2626', version_changed: '#d97706' };

            const cards = data.map(({ item, changes }) => {
                const rows = changes.map(c => {
                    const label = changeTypeLabel[c.change_type] || c.change_type;
                    const color = changeTypeColor[c.change_type] || '#6b7280';
                    const verInfo = c.change_type === 'version_changed' ? ` (${esc(c.prev_version || '?')} → ${esc(c.new_version || '?')})` :
                                    c.change_type === 'added' ? (c.new_version ? ` v${esc(c.new_version)}` : '') : '';
                    const date = c.scanned_at ? new Date(c.scanned_at).toLocaleDateString('tr-TR') : '-';
                    return `<tr>
                        <td style="padding:5px 8px;border-bottom:1px solid #f0f0f0">${esc(c.software_name)}</td>
                        <td style="padding:5px 8px;border-bottom:1px solid #f0f0f0;color:${color};font-weight:600;font-size:11px">${label}${verInfo}</td>
                        <td style="padding:5px 8px;border-bottom:1px solid #f0f0f0;color:#6b7280;font-size:11px">${esc(c.publisher || '-')}</td>
                        <td style="padding:5px 8px;border-bottom:1px solid #f0f0f0;color:#6b7280;font-size:11px">${date}</td>
                    </tr>`;
                }).join('');
                return `<div class="card">
                    <div style="display:flex;align-items:center;justify-content:space-between;margin-bottom:8px">
                        <strong style="font-size:14px">${esc(item.asset_name || item.hostname || item.ip_address || 'Bilinmiyor')}</strong>
                        <span style="font-size:11px;color:#6b7280">${changes.length} değişim</span>
                    </div>
                    <div style="font-size:11px;color:#6b7280;margin-bottom:8px">${esc(item.ip_address || '-')} &bull; ${esc(item.department || item.location || '-')}</div>
                    <table style="width:100%;border-collapse:collapse;font-size:12px">
                        <thead><tr style="background:#fffbeb"><th style="padding:5px 8px;text-align:left">Yazılım</th><th style="padding:5px 8px;text-align:left">Durum</th><th style="padding:5px 8px;text-align:left">Üretici</th><th style="padding:5px 8px;text-align:left">Tarih</th></tr></thead>
                        <tbody>${rows}</tbody>
                    </table>
                </div>`;
            }).join('');

            const totalChanges = data.reduce((s, d) => s + d.changes.length, 0);
            const addedCount = data.reduce((s, d) => s + d.changes.filter(c => c.change_type === 'added').length, 0);
            const removedCount = data.reduce((s, d) => s + d.changes.filter(c => c.change_type === 'removed').length, 0);
            const updatedCount = data.reduce((s, d) => s + d.changes.filter(c => c.change_type === 'version_changed').length, 0);

            const html = `<!DOCTYPE html><html lang="tr"><head><meta charset="utf-8"><title>Yazılım Değişim Raporu</title>
<style>*{box-sizing:border-box;margin:0;padding:0}body{font-family:system-ui,sans-serif;background:#fff;padding:24px;color:#111}
.wrap{max-width:960px;margin:0 auto}.title{font-size:22px;font-weight:700;color:#111;margin-bottom:4px}
.meta{font-size:12px;color:#6b7280;margin-bottom:18px}.sum{display:flex;gap:12px;margin-bottom:24px;flex-wrap:wrap}
.k{background:#f9fafb;border:1px solid #e5e7eb;border-radius:8px;padding:10px 16px;min-width:100px}
.l{font-size:11px;color:#6b7280;font-weight:500}.v{font-size:20px;font-weight:700;color:#111}
.card{border:1px solid #e5e7eb;border-radius:12px;padding:16px;margin-bottom:16px;break-inside:avoid}
@media print{.no-print{display:none}body{padding:0}}</style></head><body>
<div class="wrap">
    <div class="no-print" style="margin-bottom:16px"><button onclick="window.print()" style="background:#4f46e5;color:#fff;border:none;padding:8px 18px;border-radius:8px;cursor:pointer;font-size:13px">PDF Oluştur</button></div>
    <h1 class="title">Yazılım Değişim Raporu</h1>
    <div class="meta">Son ${days} gün &bull; Oluşturulma: ${now}</div>
    <div class="sum">
        <div class="k"><div class="l">Etkilenen Cihaz</div><div class="v">${data.length}</div></div>
        <div class="k"><div class="l">Toplam Değişim</div><div class="v">${totalChanges}</div></div>
        <div class="k"><div class="l">Kuruldu</div><div class="v" style="color:#059669">${addedCount}</div></div>
        <div class="k"><div class="l">Kaldırıldı</div><div class="v" style="color:#dc2626">${removedCount}</div></div>
        <div class="k"><div class="l">Güncellendi</div><div class="v" style="color:#d97706">${updatedCount}</div></div>
    </div>
    ${data.length ? cards : '<div style="text-align:center;padding:40px 0;color:#6b7280;font-size:16px">Bu dönemde yazılım değişimi tespit edilmemiştir.</div>'}
</div></body></html>`;
            const win = window.open('', '_blank');
            if (!win) throw new Error('Tarayıcı rapor penceresini engelledi');
            win.document.open(); win.document.write(html); win.document.close(); win.focus();
        },

        async loadStats() {
            try {
                const res = await fetch('/api/inventory/stats', {
                    headers: { 'Authorization': 'Bearer ' + localStorage.getItem('token') }
                });
                if (res.ok) {
                    this.stats = await res.json();
                }
            } catch (e) {
                console.error('Stats error:', e);
            }
        },

        async loadLocations() {
            try {
                const res = await fetch('/api/inventory/locations', {
                    headers: { 'Authorization': 'Bearer ' + localStorage.getItem('token') }
                });
                if (res.ok) {
                    this.locations = await res.json() || [];
                }
            } catch (e) {
                console.error('Locations error:', e);
            }
        },

        async loadItems() {
            this.loading = true;
            this.error = null;
            try {
                const params = new URLSearchParams({
                    limit: this.limit,
                    offset: this.offset
                });
                if (this.filters.search) params.append('search', this.filters.search);
                if (this.filters.type !== 'all') params.append('type', this.filters.type);
                if (this.filters.status !== 'all') params.append('status', this.filters.status);
                if (this.filters.location !== 'all') params.append('location', this.filters.location);

                const res = await fetch('/api/inventory?' + params.toString(), {
                    headers: { 'Authorization': 'Bearer ' + localStorage.getItem('token') }
                });
                if (res.ok) {
                    const data = await res.json();
                    this.items = data.items || [];
                    this.total = data.total || 0;
                } else {
                    this.error = 'Veri yüklenemedi';
                }
            } catch (e) {
                this.error = 'Bağlantı hatası';
                console.error('Load items error:', e);
            } finally {
                this.loading = false;
            }
        },

        resetFilters() {
            this.filters = { search: '', type: 'all', status: 'all', location: 'all' };
            this.offset = 0;
            this.loadItems();
        },

        prevPage() {
            if (this.offset > 0) {
                this.offset = Math.max(0, this.offset - this.limit);
                this.loadItems();
            }
        },

        nextPage() {
            if (this.offset + this.limit < this.total) {
                this.offset += this.limit;
                this.loadItems();
            }
        },

        openCreateModal() {
            this.editingItem = null;
            this.resetForm();
            this.showFormModal = true;
        },

        openEditModal(item) {
            this.editingItem = item;
            this.form = {
                asset_name: item.asset_name || '',
                asset_tag: item.asset_tag || '',
                asset_type: item.asset_type || 'other',
                status: item.status || 'active',
                ip_address: item.ip_address || '',
                mac_address: item.mac_address || '',
                hostname: item.hostname || '',
                vendor: item.vendor || '',
                brand: item.brand || '',
                model: item.model || '',
                serial_number: item.serial_number || '',
                location: item.location || '',
                department: item.department || '',
                assigned_to: item.assigned_to || '',
                purchase_date: this.normalizeDate(item.purchase_date),
                warranty_expiry: this.normalizeDate(item.warranty_expiry),
                purchase_cost: item.purchase_cost || '',
                notes: item.notes || ''
            };
            this.formError = null;
            this.showFormModal = true;
        },

        normalizeDate(value) {
            if (!value) return '';
            const text = String(value);
            if (text.includes('T')) {
                return text.split('T')[0];
            }
            if (text.includes(' ')) {
                return text.split(' ')[0];
            }
            return text;
        },

        openDetailModal(item) {
            this.detailItem = item;
            this.detailTab = 'overview';
            this.targetDetails = null;
            this.targetError = null;
            this.softwareLicenses = [];
            this.licenseOpenIdx = null;
            this.crackFindingsOpenIdx = null;
            this.softwareChanges = [];
            this.softwareChangesMap = {};
            this.scanSettings = { auto_software_scan: false, scan_hour: 3, last_auto_scan_at: null };
            this.showDetailModal = true;
            this.loadTargetDetails();
            this.loadFullInventoryItem(item.id);
            this.loadSoftwareLicenses(item.id);
            this.loadSoftwareChanges(item.id);
            this.loadScanSettings(item.id);
        },

        async loadFullInventoryItem(id) {
            if (!id) return;
            try {
                const res = await fetch(`/api/inventory/${id}`, {
                    headers: { 'Authorization': 'Bearer ' + localStorage.getItem('token') }
                });
                if (res.ok && this.showDetailModal && this.detailItem?.id === id) {
                    const full = await res.json();
                    // software_json ve software_scan_at ekle — diğer alanlar liste verisinden zaten doğru
                    this.detailItem = {
                        ...this.detailItem,
                        software_json: full.software_json || null,
                        software_scan_at: full.software_scan_at || null,
                        crack_scan_at: full.crack_scan_at || null,
                        crack_risk_level: full.crack_risk_level || null,
                        crack_findings: full.crack_findings || [],
                    };
                }
            } catch (_) {
                // sessiz hata — modal zaten açık, sadece yazılımlar sekmesi boş kalır
            }
        },

        closeDetailModal() {
            this.showDetailModal = false;
            this.detailItem = null;
            this.targetDetails = null;
            this.targetError = null;
            this.targetLoading = false;
            this.softwareLicenses = [];
            this.licenseOpenIdx = null;
            this.crackFindingsOpenIdx = null;
            this.softwareChanges = [];
            this.softwareChangesMap = {};
        },

        async loadSoftwareChanges(id) {
            if (!id) return;
            try {
                const token = localStorage.getItem('token');
                const res = await fetch(`/api/inventory/${id}/software-changes?days=7`, {
                    headers: { 'Authorization': 'Bearer ' + token }
                });
                if (res.ok) {
                    const changes = await res.json();
                    this.softwareChanges = changes || [];
                    const map = {};
                    for (const c of this.softwareChanges) {
                        map[c.software_name.toLowerCase()] = c;
                    }
                    this.softwareChangesMap = map;
                }
            } catch (_) {}
        },

        softwareChangeFor(sw) {
            const name = (sw?.DisplayName || sw?.name || '').toLowerCase();
            return this.softwareChangesMap[name] || null;
        },

        async loadScanSettings(id) {
            if (!id) return;
            try {
                const token = localStorage.getItem('token');
                const res = await fetch(`/api/inventory/${id}/scan-settings`, {
                    headers: { 'Authorization': 'Bearer ' + token }
                });
                if (res.ok) this.scanSettings = await res.json();
            } catch (_) {}
        },

        async saveScanSettings(id) {
            if (!id) return;
            this.scanSettingsSaving = true;
            try {
                const token = localStorage.getItem('token');
                await fetch(`/api/inventory/${id}/scan-settings`, {
                    method: 'PUT',
                    headers: { 'Authorization': 'Bearer ' + token, 'Content-Type': 'application/json' },
                    body: JSON.stringify(this.scanSettings)
                });
            } catch (_) {}
            this.scanSettingsSaving = false;
        },

        async loadReportUsers() {
            try {
                const token = localStorage.getItem('token');
                const res = await fetch('/api/inventory/report-users', {
                    headers: { 'Authorization': 'Bearer ' + token }
                });
                if (res.ok) this.reportUsers = await res.json();
            } catch (_) { this.reportUsers = []; }
        },

        async loadTargetDetails() {
            if (!this.detailItem?.resolved_target_id) {
                return;
            }
            this.targetLoading = true;
            this.targetError = null;
            try {
                const res = await fetch(`/api/targets/${this.detailItem.resolved_target_id}`, {
                    headers: { 'Authorization': 'Bearer ' + localStorage.getItem('token') }
                });
                if (res.ok) {
                    this.targetDetails = await res.json();
                } else {
                    this.targetError = 'Hedef bilgisi alınamadı';
                }
            } catch (e) {
                this.targetError = 'Bağlantı hatası';
            } finally {
                this.targetLoading = false;
            }
        },

        closeFormModal() {
            this.showFormModal = false;
            this.editingItem = null;
            this.formError = null;
        },

        resetForm() {
            this.form = {
                asset_name: '',
                asset_tag: '',
                asset_type: 'other',
                status: 'active',
                ip_address: '',
                mac_address: '',
                hostname: '',
                vendor: '',
                brand: '',
                model: '',
                serial_number: '',
                location: '',
                department: '',
                assigned_to: '',
                purchase_date: '',
                warranty_expiry: '',
                purchase_cost: '',
                notes: ''
            };
            this.formError = null;
        },

        async submitForm() {
            if (!this.form.asset_name.trim()) {
                this.formError = 'Varlık adı zorunludur';
                return;
            }

            this.submitting = true;
            this.formError = null;

            try {
                const payload = {
                    asset_name: this.form.asset_name.trim(),
                    asset_tag: this.form.asset_tag.trim() || null,
                    asset_type: this.form.asset_type,
                    status: this.form.status,
                    ip_address: this.form.ip_address.trim() || null,
                    mac_address: this.form.mac_address.trim() || null,
                    hostname: this.form.hostname.trim() || null,
                    vendor: this.form.vendor.trim() || null,
                    brand: this.form.brand.trim() || null,
                    model: this.form.model.trim() || null,
                    serial_number: this.form.serial_number.trim() || null,
                    location: this.form.location.trim() || null,
                    department: this.form.department.trim() || null,
                    assigned_to: this.form.assigned_to.trim() || null,
                    purchase_date: this.form.purchase_date || null,
                    warranty_expiry: this.form.warranty_expiry || null,
                    purchase_cost: this.form.purchase_cost ? parseFloat(this.form.purchase_cost) : null,
                    notes: this.form.notes.trim() || null
                };

                const url = this.editingItem ? `/api/inventory/${this.editingItem.id}` : '/api/inventory';
                const method = this.editingItem ? 'PUT' : 'POST';

                const res = await fetch(url, {
                    method,
                    headers: {
                        'Content-Type': 'application/json',
                        'Authorization': 'Bearer ' + localStorage.getItem('token')
                    },
                    body: JSON.stringify(payload)
                });

                if (res.ok) {
                    this.closeFormModal();
                    await Promise.all([this.loadStats(), this.loadLocations(), this.loadItems()]);
                    if (window.SYS && window.SYS.showToast) {
                        window.SYS.showToast(this.editingItem ? 'Varlık güncellendi' : 'Varlık oluşturuldu', 'success');
                    }
                } else {
                    const data = await res.json();
                    this.formError = data.error || 'İşlem başarısız';
                }
            } catch (e) {
                this.formError = 'Bağlantı hatası';
                console.error('Submit error:', e);
            } finally {
                this.submitting = false;
            }
        },

        confirmDelete(item) {
            this.deletingItem = item;
            this.showDeleteModal = true;
        },

        async deleteItem() {
            if (!this.deletingItem) return;

            this.deleting = true;
            try {
                const res = await fetch(`/api/inventory/${this.deletingItem.id}`, {
                    method: 'DELETE',
                    headers: { 'Authorization': 'Bearer ' + localStorage.getItem('token') }
                });

                if (res.ok) {
                    this.showDeleteModal = false;
                    this.deletingItem = null;
                    await Promise.all([this.loadStats(), this.loadItems()]);
                    if (window.SYS && window.SYS.showToast) {
                        window.SYS.showToast('Varlık silindi', 'success');
                    }
                } else {
                    const data = await res.json();
                    alert(data.error || 'Silme başarısız');
                }
            } catch (e) {
                alert('Bağlantı hatası');
                console.error('Delete error:', e);
            } finally {
                this.deleting = false;
            }
        },

        async linkToTarget(item) {
            if (!item || !item.resolved_target_id) {
                return;
            }
            try {
                const res = await fetch(`/api/inventory/${item.id}/link`, {
                    method: 'POST',
                    headers: {
                        'Content-Type': 'application/json',
                        'Authorization': 'Bearer ' + localStorage.getItem('token')
                    },
                    body: JSON.stringify({ target_id: item.resolved_target_id })
                });
                if (res.ok) {
                    await Promise.all([this.loadStats(), this.loadItems()]);
                    if (window.SYS && window.SYS.showToast) {
                        window.SYS.showToast('Hedef bağlantısı oluşturuldu', 'success');
                    }
                } else {
                    const data = await res.json().catch(() => ({}));
                    if (window.SYS && window.SYS.showToast) {
                        window.SYS.showToast(data.error || 'Bağlama başarısız', 'error');
                    }
                }
            } catch (e) {
                console.error('Link target error:', e);
                if (window.SYS && window.SYS.showToast) {
                    window.SYS.showToast('Bağlantı hatası', 'error');
                }
            }
        },

        goToTargets(item = null) {
            const searchValue = item?.ip_address || item?.hostname || item?.asset_name || '';
            if (window.app && typeof window.app.navigateTo === 'function') {
                window.app.navigateTo('targets', { search: searchValue });
            } else {
                if (searchValue) {
                    localStorage.setItem('systrack_targets_search', searchValue);
                }
                window.location.hash = '#targets';
            }
        },

        goToPage(page) {
            if (window.app && typeof window.app.navigateTo === 'function') {
                window.app.navigateTo(page);
            } else {
                window.location.hash = '#' + page;
            }
        },

        statusPercent(value) {
            const total = Number(this.stats?.total || 0);
            if (!total) return 0;
            return Math.max(0, Math.min(100, Math.round((Number(value || 0) / total) * 100)));
        },

        getSourceLabel(source) {
            const labels = {
                manual: 'Manuel',
                ip_scanner: 'IP Tarayıcı',
                csv_import: 'CSV'
            };
            return labels[source] || 'Manuel';
        },

        getSourceClass(source) {
            const classes = {
                manual: 'bg-gray-100 text-gray-700 dark:bg-gray-700 dark:text-gray-200',
                ip_scanner: 'bg-blue-50 text-blue-700 dark:bg-blue-500/10 dark:text-blue-300',
                csv_import: 'bg-amber-50 text-amber-700 dark:bg-amber-500/10 dark:text-amber-300'
            };
            return classes[source] || classes.manual;
        },

        getAssetTypeLabel(type) {
            const labels = {
                pc: 'Bilgisayar',
                laptop: 'Laptop',
                printer: 'Yazıcı',
                switch: 'Switch',
                router: 'Router',
                access_point: 'Access Point',
                pos: 'POS',
                tv: 'TV',
                phone: 'Telefon',
                camera: 'Kamera',
                server: 'Sunucu',
                tablet: 'Tablet',
                other: 'Diğer'
            };
            return labels[type] || type;
        },

        getStatusLabel(status) {
            const labels = {
                active: 'Aktif',
                maintenance: 'Bakımda',
                storage: 'Depoda',
                faulty: 'Arızalı',
                retired: 'Emekli'
            };
            return labels[status] || status;
        },

        getStatusClass(status) {
            const classes = {
                active: 'bg-emerald-100 text-emerald-700 border border-emerald-200 dark:bg-emerald-500/20 dark:text-emerald-300 dark:border-emerald-500/30',
                maintenance: 'bg-amber-100 text-amber-700 border border-amber-200 dark:bg-amber-500/20 dark:text-amber-300 dark:border-amber-500/30',
                storage: 'bg-blue-100 text-blue-700 border border-blue-200 dark:bg-blue-500/20 dark:text-blue-300 dark:border-blue-500/30',
                faulty: 'bg-red-100 text-red-700 border border-red-200 dark:bg-red-500/20 dark:text-red-300 dark:border-red-500/30',
                retired: 'bg-gray-100 text-gray-700 border border-gray-200 dark:bg-gray-700 dark:text-gray-200 dark:border-gray-600'
            };
            return classes[status] || 'bg-gray-100 text-gray-700 border border-gray-200 dark:bg-gray-700 dark:text-gray-200 dark:border-gray-600';
        },

        getCompleteness(item) {
            if (!item) return 0;
            const fields = [
                item.ip_address,
                item.mac_address,
                item.brand || item.vendor,
                item.model,
                item.serial_number,
                item.location
            ];
            const filled = fields.filter(v => v !== null && v !== undefined && String(v).trim() !== '').length;
            return Math.round((filled / fields.length) * 100);
        },

        getCompletenessClass(item) {
            const value = this.getCompleteness(item);
            if (value >= 85) return 'bg-emerald-100 text-emerald-700 dark:bg-emerald-500/20 dark:text-emerald-300';
            if (value >= 60) return 'bg-amber-100 text-amber-700 dark:bg-amber-500/20 dark:text-amber-300';
            return 'bg-red-100 text-red-700 dark:bg-red-500/20 dark:text-red-300';
        },

        normalizeDeviceList(value) {
            if (!value) return [];
            return Array.isArray(value) ? value : [value];
        },

        parseByteValue(value) {
            if (value === null || value === undefined) return 0;
            if (typeof value === 'number') return value;
            const parsed = Number(String(value).trim());
            return Number.isFinite(parsed) ? parsed : 0;
        },

        formatDiskTotalGB(gb) {
            if (!gb || !Number.isFinite(gb)) return '-';
            if (gb >= 1024) return (gb / 1024).toFixed(2) + ' TB';
            return gb.toFixed(2) + ' GB';
        },

        getWinRMDiskTotalLabel(winrmData) {
            if (!winrmData) return '-';
            const directGB = Number(winrmData.disk_total_gb || 0);
            if (Number.isFinite(directGB) && directGB > 0) {
                return this.formatDiskTotalGB(directGB);
            }

            const physical = this.normalizeDeviceList(winrmData.physical_disks);
            const physicalTotal = physical.reduce((sum, disk) => sum + this.parseByteValue(disk?.Size), 0);
            if (physicalTotal > 0) {
                return this.formatDiskTotalGB(physicalTotal / 1024 / 1024 / 1024);
            }

            const logical = this.normalizeDeviceList(winrmData.disks);
            const logicalTotal = logical.reduce((sum, disk) => sum + this.parseByteValue(disk?.Size), 0);
            if (logicalTotal > 0) {
                return this.formatDiskTotalGB(logicalTotal / 1024 / 1024 / 1024);
            }

            return '-';
        },

        formatWinRMDateTime(rawValue) {
            if (!rawValue) return '-';
            const raw = String(rawValue).trim();
            if (!raw) return '-';

            // WMI format: yyyyMMddHHmmss.xxxxxx+zzz
            const wmiMatch = raw.match(/^(\d{4})(\d{2})(\d{2})(\d{2})(\d{2})(\d{2})/);
            if (wmiMatch) {
                const [, y, m, d, hh, mm, ss] = wmiMatch;
                const parsed = new Date(`${y}-${m}-${d}T${hh}:${mm}:${ss}`);
                if (!Number.isNaN(parsed.getTime())) {
                    return parsed.toLocaleString('tr-TR', {
                        day: '2-digit',
                        month: '2-digit',
                        year: 'numeric',
                        hour: '2-digit',
                        minute: '2-digit'
                    });
                }
            }

            const date = new Date(raw);
            if (Number.isNaN(date.getTime())) return '-';
            return date.toLocaleString('tr-TR', {
                day: '2-digit',
                month: '2-digit',
                year: 'numeric',
                hour: '2-digit',
                minute: '2-digit'
            });
        },

        getWinRMLastBootLabel(winrmData) {
            if (!winrmData) return '-';
            const raw = winrmData?.os?.LastBootUpTime || winrmData?.last_boot_time || '';
            return this.formatWinRMDateTime(raw);
        },

        formatUptime(lastBootTime) {
            if (!lastBootTime) return '-';
            try {
                const boot = new Date(lastBootTime.replace(' ', 'T'));
                if (isNaN(boot.getTime())) return '-';
                const diffMs = Date.now() - boot.getTime();
                if (diffMs < 0) return '-';
                const days = Math.floor(diffMs / 86400000);
                const hours = Math.floor((diffMs % 86400000) / 3600000);
                const mins = Math.floor((diffMs % 3600000) / 60000);
                if (days > 0) return `${days} gün ${hours} saat`;
                if (hours > 0) return `${hours} saat ${mins} dk`;
                return `${mins} dakika`;
            } catch { return '-'; }
        },

        filteredSoftware() {
            return Array.isArray(this.detailItem?.software_json) ? this.detailItem.software_json : [];
        },

        formatInstallDate(raw) {
            if (!raw) return '-';
            const s = String(raw).trim();
            // YYYYMMDD → GG.AA.YYYY
            if (/^\d{8}$/.test(s)) {
                return s.slice(6, 8) + '.' + s.slice(4, 6) + '.' + s.slice(0, 4);
            }
            // ISO / datetime → tr-TR tarih formatı
            try {
                const d = new Date(s);
                if (!isNaN(d.getTime())) return d.toLocaleDateString('tr-TR');
            } catch (_) {}
            return s;
        },

        // ── Yazılım Lisans Yönetimi ──────────────────────────────────────
        async loadSoftwareLicenses(inventoryId) {
            if (!inventoryId) return;
            try {
                const res = await fetch(`/api/inventory/${inventoryId}/software-licenses`, {
                    headers: { 'Authorization': 'Bearer ' + localStorage.getItem('token') }
                });
                if (res.ok) this.softwareLicenses = await res.json();
            } catch (_) {}
        },

        getLicenseFor(softwareName) {
            return this.softwareLicenses.find(l => l.software_name === softwareName) || null;
        },

        softwareName(sw) {
            return sw?.DisplayName || sw?.name || '';
        },

        normalizeSoftwareText(value) {
            return String(value || '')
                .toLowerCase()
                .replace(/\s+/g, ' ')
                .trim();
        },

        crackToolPattern() {
            return /(kmspico|kmsauto|autokms|auto\s*kms|aact|re-loader|reloader|microsoft toolkit|kms tools|kmseldi|ratiborus|hwidgen|massgrave|office toolkit)/i;
        },

        crackToolTokens(value) {
            const text = this.normalizeSoftwareText(value);
            const tokens = [];
            [
                ['kmspico', /kms\s*pico|kmspico/],
                ['kmsauto', /kms\s*auto|kmsauto/],
                ['autokms', /auto\s*kms|autokms/],
                ['aact', /\baact\b/],
                ['reloader', /re-loader|reloader/],
                ['microsoft-toolkit', /microsoft toolkit|office toolkit/],
                ['kms-tools', /kms tools|kmseldi|ratiborus/],
                ['hwidgen', /hwidgen|massgrave/],
            ].forEach(([token, rx]) => {
                if (rx.test(text)) tokens.push(token);
            });
            return tokens;
        },

        softwareFindings(sw, contextItem = null) {
            const item = contextItem || this.detailItem || {};
            const findings = Array.isArray(item.crack_findings) ? item.crack_findings : [];
            return findings.filter(f => this.softwareMatchesFinding(sw, f));
        },

        // Belirli bir kurulu yazılımla eşleşmeyen bulgular (KMS host, hosts dosyası
        // tahrifatı, event log izleri vb. — Windows'un kendisiyle ilgili ama "kurulu
        // yazılım" listesinde bir satır olarak görünmeyen sistem-düzeyi sinyaller).
        unmatchedCrackFindings(contextItem = null) {
            const item = contextItem || this.detailItem || {};
            const findings = Array.isArray(item.crack_findings) ? item.crack_findings : [];
            const swList = this.softwareListForItem(item);
            return findings.filter(f => !swList.some(sw => this.softwareMatchesFinding(sw, f)));
        },

        toggleCrackFindings(key) {
            this.crackFindingsOpenIdx = (this.crackFindingsOpenIdx === key) ? null : key;
        },

        softwareMatchesFinding(sw, finding) {
            const name = this.normalizeSoftwareText(this.softwareName(sw));
            if (!name || !finding) return false;
            const haystack = this.normalizeSoftwareText([finding.title, finding.description, finding.evidence, finding.category].join(' '));
            if (haystack.includes(name)) return true;
            if ((name.includes('office') || name.includes('microsoft 365') || name.includes('word')) &&
                (haystack.includes('office') || haystack.includes('winword') || haystack.includes('license'))) {
                return true;
            }
            // Not: genel bir "windows" substring kuralı kasıtlı olarak yok — Windows işletim
            // sisteminin kendisi normalde kurulu yazılım listesinde bir satır olarak
            // görünmez, bu yüzden böyle bir kural "Update for ... Windows ..." gibi KB
            // güncellemelerini veya "Windows Media Player" gibi rastgele bileşenleri
            // Windows aktivasyon/KMS bulgularıyla yanlışlıkla eşleştirir. Bu tür sistem
            // düzeyi bulgular kasıtlı olarak eşleşmeden bırakılır (bkz. unmatchedCrackFindings).
            const nameTokens = this.crackToolTokens(name);
            if (nameTokens.length > 0) {
                const findingTokens = this.crackToolTokens(haystack);
                if (findingTokens.some(token => nameTokens.includes(token))) return true;
            }
            return false;
        },

        softwareComplianceStatus(sw, contextItem = null) {
            const findings = this.softwareFindings(sw, contextItem);
            if (findings.some(f => ['high', 'medium'].includes(f.severity))) {
                return { label: 'Geçersiz Lisans', tone: 'invalid', icon: 'fa-circle-exclamation' };
            }
            const license = this.getLicenseFor(this.softwareName(sw));
            if (!license) {
                return { label: 'Lisans Kaydı Yok', tone: 'missing', icon: 'fa-circle-question' };
            }
            const expiry = this.licenseExpiryStatus(license);
            if (expiry === 'expired') {
                return { label: 'Süresi Dolmuş', tone: 'expired', icon: 'fa-triangle-exclamation' };
            }
            if (expiry === 'expiring') {
                return { label: 'Süresi Yaklaşıyor', tone: 'expiring', icon: 'fa-clock' };
            }
            return { label: 'Lisanslı', tone: 'licensed', icon: 'fa-circle-check' };
        },

        softwareComplianceClass(sw, contextItem = null) {
            const tone = this.softwareComplianceStatus(sw, contextItem).tone;
            return {
                invalid: 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-300',
                expired: 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-300',
                expiring: 'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300',
                missing: 'bg-gray-100 text-gray-600 dark:bg-gray-700 dark:text-gray-300',
                licensed: 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300',
            }[tone] || 'bg-gray-100 text-gray-600 dark:bg-gray-700 dark:text-gray-300';
        },

        findingSeverityClass(severity) {
            return {
                high: 'border-red-300 bg-red-50 text-red-800 dark:border-red-700/70 dark:bg-red-950/30 dark:text-red-200',
                medium: 'border-orange-300 bg-orange-50 text-orange-800 dark:border-orange-700/70 dark:bg-orange-950/30 dark:text-orange-200',
                low: 'border-amber-300 bg-amber-50 text-amber-800 dark:border-amber-700/70 dark:bg-amber-950/30 dark:text-amber-200',
            }[severity] || 'border-gray-300 bg-gray-50 text-gray-700 dark:border-gray-700 dark:bg-gray-900/40 dark:text-gray-300';
        },

        softwareListForItem(item) {
            return item?.software_json || item?.winrm_data?.software_list || item?.ssh_data?.software_list || [];
        },

        async runComplianceScan() {
            if (!this.detailItem?.id || this.complianceScanning) return;
            this.complianceScanning = true;
            try {
                const res = await fetch(`/api/inventory/${this.detailItem.id}/crack-scan`, {
                    method: 'POST',
                    headers: { 'Authorization': 'Bearer ' + localStorage.getItem('token') },
                });
                const data = await res.json().catch(() => ({}));
                if (!res.ok) {
                    window.showToast && window.showToast(data.error || 'Lisans taraması başarısız', 'error');
                    return;
                }
                this.detailItem = {
                    ...this.detailItem,
                    crack_scan_at: new Date().toISOString(),
                    crack_risk_level: data.risk_level,
                    crack_findings: data.findings || [],
                };
                await this.loadItems();
                window.showToast && window.showToast(`Lisans taraması tamamlandı: ${this.crackRiskLabel(data.risk_level)}`, data.risk_level === 'high' ? 'warning' : 'success');
            } catch (e) {
                console.error('Compliance scan error:', e);
                window.showToast && window.showToast('Lisans taraması sırasında hata oluştu', 'error');
            } finally {
                this.complianceScanning = false;
            }
        },

        async markFindingException(finding) {
            if (!this.detailItem?.id || finding?.excepted) return;
            const reason = prompt('Bu bulguyu istisna olarak işaretleme nedeni (IT notu):', '');
            if (reason === null) return;
            try {
                const res = await fetch(`/api/inventory/${this.detailItem.id}/crack-scan/exceptions`, {
                    method: 'POST',
                    headers: {
                        'Content-Type': 'application/json',
                        'Authorization': 'Bearer ' + localStorage.getItem('token'),
                    },
                    body: JSON.stringify({
                        category: finding.category,
                        title: finding.title,
                        evidence: finding.evidence,
                        reason: reason,
                    }),
                });
                if (res.ok) {
                    await this.loadFullInventoryItem(this.detailItem.id);
                    await this.loadItems();
                    window.showToast && window.showToast('Bulgu istisna olarak işaretlendi', 'success');
                } else {
                    const err = await res.json().catch(() => ({}));
                    window.showToast && window.showToast(err.error || 'İstisna eklenemedi', 'error');
                }
            } catch (_) {
                window.showToast && window.showToast('Bağlantı hatası', 'error');
            }
        },

        licenseExpiryStatus(license) {
            if (!license?.license_expiry_date) return null;
            const expiry = new Date(license.license_expiry_date);
            const now = new Date();
            const diff = (expiry - now) / (1000 * 60 * 60 * 24);
            if (diff < 0) return 'expired';
            if (diff <= 30) return 'expiring';
            return 'ok';
        },

        formatCost(cost, currency) {
            if (cost == null || cost === '') return '-';
            const c = parseFloat(cost);
            if (isNaN(c)) return '-';
            const sym = { TRY: '₺', EUR: '€', USD: '$' }[currency] || currency || '';
            return sym + c.toLocaleString('tr-TR', { minimumFractionDigits: 0, maximumFractionDigits: 2 });
        },

        openLicenseForm(sw, idx) {
            // Aynı satıra tekrar tıklanırsa kapat (toggle)
            if (this.licenseOpenIdx === idx) {
                this.licenseOpenIdx = null;
                return;
            }
            const name = sw.DisplayName || sw.name || '';
            const existing = this.getLicenseFor(name);
            this.licenseFormSoftware = name;
            this.licenseFormLicenseId = existing?.id || null;
            this.licenseFormData = {
                purchase_cost:       existing?.purchase_cost != null ? String(existing.purchase_cost) : '',
                cost_currency:       existing?.cost_currency || 'TRY',
                purchase_date:       existing?.purchase_date ? existing.purchase_date.slice(0, 10) : '',
                license_expiry_date: existing?.license_expiry_date ? existing.license_expiry_date.slice(0, 10) : '',
                notes:               existing?.notes || '',
            };
            this.licenseOpenIdx = idx;
        },

        closeLicenseForm() {
            this.licenseOpenIdx = null;
        },

        async saveLicenseForm() {
            if (!this.detailItem?.id || !this.licenseFormSoftware) return;
            this.licenseFormSaving = true;
            try {
                const payload = {
                    software_name:       this.licenseFormSoftware,
                    purchase_cost:       this.licenseFormData.purchase_cost !== '' ? parseFloat(this.licenseFormData.purchase_cost) : null,
                    cost_currency:       this.licenseFormData.cost_currency || 'TRY',
                    purchase_date:       this.licenseFormData.purchase_date || null,
                    license_expiry_date: this.licenseFormData.license_expiry_date || null,
                    notes:               this.licenseFormData.notes || null,
                };
                const res = await fetch(`/api/inventory/${this.detailItem.id}/software-licenses`, {
                    method: 'POST',
                    headers: {
                        'Content-Type': 'application/json',
                        'Authorization': 'Bearer ' + localStorage.getItem('token'),
                    },
                    body: JSON.stringify(payload),
                });
                if (res.ok) {
                    const updated = await res.json();
                    const i = this.softwareLicenses.findIndex(l => l.software_name === this.licenseFormSoftware);
                    if (i >= 0) this.softwareLicenses[i] = updated;
                    else this.softwareLicenses.push(updated);
                    this.licenseOpenIdx = null;
                } else {
                    const err = await res.json();
                    alert('Kayıt hatası: ' + (err.error || 'Bilinmeyen hata'));
                }
            } catch (e) {
                alert('Bağlantı hatası');
            } finally {
                this.licenseFormSaving = false;
            }
        },

        async deleteLicenseFromForm() {
            if (!this.detailItem?.id || !this.licenseFormLicenseId) return;
            if (!confirm('Bu lisans kaydı silinsin mi?')) return;
            try {
                const res = await fetch(`/api/inventory/${this.detailItem.id}/software-licenses/${this.licenseFormLicenseId}`, {
                    method: 'DELETE',
                    headers: { 'Authorization': 'Bearer ' + localStorage.getItem('token') },
                });
                if (res.ok) {
                    this.softwareLicenses = this.softwareLicenses.filter(l => l.id !== this.licenseFormLicenseId);
                    this.licenseOpenIdx = null;
                }
            } catch (_) {}
        },
        // ── /Yazılım Lisans Yönetimi ─────────────────────────────────────

        crackRiskColor(level) {
            return { 'clean': 'emerald', 'low': 'yellow', 'medium': 'orange', 'high': 'red' }[level] || 'gray';
        },

        crackRiskLabel(level) {
            return { clean: 'Temiz', low: 'Düşük Risk', medium: 'Orta Risk', high: 'Yüksek Risk' }[level] || '—';
        },

        crackSeverityColor(severity) {
            return { high: 'red', medium: 'orange', low: 'yellow' }[severity] || 'gray';
        },

        getWinRMPhysicalDiskLabel(winrmData, maxItems = 2) {
            const disks = this.normalizeDeviceList(winrmData?.physical_disks);
            if (!disks.length) return '-';

            const labels = disks
                .slice(0, maxItems)
                .map((disk) => {
                    const model = disk?.Model || disk?.Name || disk?.DeviceID || 'Disk';
                    const mediaType = String(disk?.MediaType || '').trim();
                    const interfaceType = String(disk?.InterfaceType || '').trim();
                    const normalizedMedia = mediaType.toLowerCase();
                    let diskType = '';
                    if (normalizedMedia.includes('ssd')) diskType = 'SSD';
                    else if (normalizedMedia.includes('hdd') || normalizedMedia.includes('hard')) diskType = 'HDD';
                    else if (interfaceType) diskType = interfaceType.toUpperCase();

                    const sizeBytes = this.parseByteValue(disk?.Size);
                    const sizeLabel = sizeBytes > 0 ? this.formatDiskTotalGB(sizeBytes / 1024 / 1024 / 1024) : '';
                    const parts = [model];
                    if (diskType) parts.push(diskType);
                    if (sizeLabel) parts.push(sizeLabel);
                    return parts.join(' - ');
                });

            const remaining = disks.length - maxItems;
            if (remaining > 0) {
                labels.push(`... +${remaining}`);
            }
            return labels.join(', ');
        },

        getWinRMDeviceCount(winrmData, key) {
            return this.normalizeDeviceList(winrmData?.[key]).length;
        },

        getWinRMDevicePreview(winrmData, key, maxItems = 3) {
            const items = this.normalizeDeviceList(winrmData?.[key]);
            if (!items.length) return '-';

            return items
                .slice(0, maxItems)
                .map((item) => {
                    const name = item?.Name || item?.Model || item?.UserFriendlyName || item?.Description || '-';
                    const manufacturer = item?.Manufacturer || item?.MonitorManufacturer || '';
                    return manufacturer ? `${name} (${manufacturer})` : name;
                })
                .join(', ');
        },

        formatDate(dateStr) {
            if (!dateStr) return '-';
            try {
                const date = new Date(dateStr);
                if (isNaN(date.getTime())) return '-';
                return date.toLocaleDateString('tr-TR', {
                    day: '2-digit',
                    month: '2-digit',
                    year: 'numeric'
                });
            } catch (e) {
                return '-';
            }
        },

        // ========================================
        // AD Settings & Unified Discovery
        // ========================================
        showADSettingsModal: false,
        discoverySettingsTab: 'settings',
        adSettingsLoading: false,
        settingsSaving: false,
        adTesting: false,
        discovering: false,
        importing: false,
        adTestResult: null,

        // Discovery Methods (toggles)
        discoveryMethods: {
            ldap: false, // Optional, now with checkbox
            dns: true,   // Recommended, default on
            snmp: false, // Optional, default off
            winrm: true, // Recommended for Windows enrichment (controlled by ldap)
            ssh: false   // Optional, for Linux/macOS
        },
        crackDetectionEnabled: false, // Lisans/crack uyumluluk taraması (sadece WinRM açıkken etkili)
        snmpCollapsed: true,
        sshCollapsed: true,

        // Discovery Results
        discoveryResults: [],
        selectedScanResults: [],
        scanResultFilters: {
            showExisting: true  // Default: tüm sonuçları göster
        },

        adSettings: {
            domain_controller: '',
            port: 389,
            use_ssl: false,
            base_dn: '',
            bind_username: '',
            bind_password: '',
            search_filter: '(objectClass=computer)',
            search_scope: 'sub',
            ou_filter: '',
            location_from_ou: true,
            ou_location_map: '',
            is_enabled: false,
            exclude_disabled: true, // Default: exclude disabled computers
            last_test_at: null,
            last_test_result: null,
            last_test_error: null
        },

        // JSON validation helper
        isValidJson(str) {
            if (!str || str.trim() === '') return true;
            try {
                JSON.parse(str);
                return true;
            } catch (e) {
                return false;
            }
        },

        // Validate OU filter JSON
        validateOuFilter() {
            if (this.adSettings.ou_filter && !this.isValidJson(this.adSettings.ou_filter)) {
                window.showToast && window.showToast('OU filtresi geçersiz JSON formatında', 'error');
            }
        },

        // Update search filter based on exclude_disabled setting
        updateSearchFilter() {
            const baseFilter = '(objectClass=computer)';
            const disabledFilter = '(&(objectClass=computer)(!(userAccountControl:1.2.840.113556.1.4.803:=2)))';

            // Only auto-update if using default filter
            if (this.adSettings.search_filter === baseFilter || this.adSettings.search_filter === disabledFilter) {
                this.adSettings.search_filter = this.adSettings.exclude_disabled ? disabledFilter : baseFilter;
            }
        },

        openADSettingsModal() {
            this.showADSettingsModal = true;
            this.adTestResult = null;
            this.winrmTestResult = null;
            this.snmpTestResult = null;
            this.loadADSettings();
            this.loadWinRMSettings();
            this.loadSNMPSettings();
            this.loadSSHSettings();
        },

        closeADSettingsModal() {
            this.showADSettingsModal = false;
            this.adTestResult = null;
        },

        async loadADSettings() {
            this.adSettingsLoading = true;
            try {
                const res = await fetch('/api/inventory/ad-settings', {
                    headers: { 'Authorization': 'Bearer ' + localStorage.getItem('token') }
                });
                if (res.ok) {
                    const data = await res.json();
                    const searchFilter = data.search_filter || '(objectClass=computer)';
                    // Detect if disabled computers are excluded based on filter
                    const excludeDisabled = searchFilter.includes('userAccountControl');

                    this.adSettings = {
                        domain_controller: data.domain_controller || '',
                        port: data.port || 389,
                        use_ssl: data.use_ssl || false,
                        base_dn: data.base_dn || '',
                        bind_username: data.bind_username || '',
                        bind_password: data.has_password ? '********' : '', // Show placeholder if password exists
                        has_password: data.has_password || false,
                        search_filter: searchFilter,
                        search_scope: data.search_scope || 'sub',
                        ou_filter: data.ou_filter || '',
                        location_from_ou: data.location_from_ou !== false,
                        ou_location_map: data.ou_location_map || '',
                        is_enabled: data.is_enabled || false,
                        exclude_disabled: excludeDisabled,
                        last_test_at: data.last_test_at,
                        last_test_result: data.last_test_result,
                        last_test_error: data.last_test_error
                    };
                }
            } catch (e) {
                console.error('AD settings load error:', e);
                window.showToast && window.showToast('AD ayarları yüklenemedi', 'error');
            } finally {
                this.adSettingsLoading = false;
            }
        },

        async saveADSettings() {
            this.adSaving = true;
            try {
                const payload = {
                    domain_controller: this.adSettings.domain_controller,
                    port: parseInt(this.adSettings.port) || 389,
                    use_ssl: this.adSettings.use_ssl,
                    base_dn: this.adSettings.base_dn,
                    bind_username: this.adSettings.bind_username,
                    bind_password: this.adSettings.bind_password || '********',
                    search_filter: this.adSettings.search_filter || '(objectClass=computer)',
                    search_scope: this.adSettings.search_scope || 'sub',
                    ou_filter: this.adSettings.ou_filter || null,
                    location_from_ou: this.adSettings.location_from_ou,
                    ou_location_map: this.adSettings.ou_location_map || null,
                    is_enabled: this.adSettings.is_enabled
                };

                const res = await fetch('/api/inventory/ad-settings', {
                    method: 'POST',
                    headers: {
                        'Authorization': 'Bearer ' + localStorage.getItem('token'),
                        'Content-Type': 'application/json'
                    },
                    body: JSON.stringify(payload)
                });

                if (res.ok) {
                    window.showToast && window.showToast('AD ayarları kaydedildi', 'success');
                } else {
                    const data = await res.json();
                    window.showToast && window.showToast(data.error || 'Kaydetme hatası', 'error');
                }
            } catch (e) {
                console.error('AD settings save error:', e);
                window.showToast && window.showToast('Bağlantı hatası', 'error');
            } finally {
                this.adSaving = false;
            }
        },

        async testADConnection() {
            this.adTesting = true;
            this.adTestResult = null;
            try {
                // First save the settings
                await this.saveADSettings();

                // Then test
                const res = await fetch('/api/inventory/ad-settings/test', {
                    method: 'POST',
                    headers: { 'Authorization': 'Bearer ' + localStorage.getItem('token') }
                });

                if (res.ok) {
                    this.adTestResult = await res.json();
                    if (this.adTestResult.success) {
                        window.showToast && window.showToast(this.adTestResult.message, 'success');
                    } else {
                        window.showToast && window.showToast(this.adTestResult.message, 'error');
                    }
                    // Reload settings to get updated test time
                    await this.loadADSettings();
                } else {
                    const data = await res.json();
                    this.adTestResult = { success: false, message: data.error || 'Test hatası' };
                    window.showToast && window.showToast(this.adTestResult.message, 'error');
                }
            } catch (e) {
                console.error('AD test error:', e);
                this.adTestResult = { success: false, message: 'Bağlantı hatası' };
                window.showToast && window.showToast('Bağlantı hatası', 'error');
            } finally {
                this.adTesting = false;
            }
        },

        async discoverFromAD() {
            // Check required settings
            if (!this.adSettings.domain_controller) {
                window.showToast && window.showToast('Önce Domain Controller adresini girin ve kaydedin', 'error');
                return;
            }
            if (!this.adSettings.bind_username) {
                window.showToast && window.showToast('Önce kullanıcı adı ve şifre girin ve kaydedin', 'error');
                return;
            }

            if (!confirm('AD\'den bilgisayarları keşfet ve envantere ekle?\n\nBase DN boşsa otomatik tespit edilecek.\n\nBu işlem birkaç dakika sürebilir.')) {
                return;
            }

            this.adDiscovering = true;
            try {
                const res = await fetch('/api/inventory/ad-discovery', {
                    method: 'POST',
                    headers: {
                        'Authorization': 'Bearer ' + localStorage.getItem('token'),
                        'Content-Type': 'application/json'
                    },
                    body: JSON.stringify({ dry_run: false })
                });

                if (res.ok) {
                    const result = await res.json();
                    if (result.success) {
                        const msg = `AD Keşfi Tamamlandı!\n\nBulunan: ${result.total_found}\nYeni: ${result.new_added}\nGüncellenen: ${result.updated}\nAtlanan: ${result.skipped}\nHata: ${result.errors}`;
                        window.showToast && window.showToast(msg, 'success');
                        // Reload inventory list
                        await this.loadItems();
                    } else {
                        window.showToast && window.showToast(result.message || 'Keşif hatası', 'error');
                        if (result.error_details && result.error_details.length > 0) {
                            console.error('AD Discovery errors:', result.error_details);
                        }
                    }
                } else {
                    const data = await res.json();
                    window.showToast && window.showToast(data.error || 'Keşif hatası', 'error');
                }
            } catch (e) {
                console.error('AD discovery error:', e);
                window.showToast && window.showToast('Bağlantı hatası', 'error');
            } finally {
                this.adDiscovering = false;
            }
        },

        // ========================================
        // Unified Discovery Functions (NEW)
        // ========================================

        async autoDetectIPRange(type) {
            // PRIMARY: Use IP Scanner's proven endpoint (same as IP Scanner page)
            try {
                const response = await fetch('/api/network/ip-scanner/defaults', {
                    headers: { 'Authorization': `Bearer ${localStorage.getItem('token')}` }
                });

                if (response.ok) {
                    const data = await response.json();
                    // IP Scanner returns interfaces array with cidr
                    if (data && data.default_subnet) {
                        const cidr = data.default_subnet;
                        if (type === 'ssh') {
                            this.sshSettings.scan_targets = cidr;
                        } else if (type === 'snmp') {
                            this.snmpSettings.scan_targets = cidr;
                        }
                        window.showToast && window.showToast(`IP aralığı tespit edildi: ${cidr}`, 'success');
                        return;
                    }
                    // Or try first interface
                    if (data && data.interfaces && data.interfaces.length > 0) {
                        const preferred = data.interfaces.find(iface => iface.default) || data.interfaces[0];
                        const cidr = preferred.cidr;
                        if (cidr) {
                            if (type === 'ssh') {
                                this.sshSettings.scan_targets = cidr;
                            } else if (type === 'snmp') {
                                this.snmpSettings.scan_targets = cidr;
                            }
                            window.showToast && window.showToast(`IP aralığı tespit edildi: ${cidr}`, 'success');
                            return;
                        }
                    }
                }
            } catch (e) {
                console.log('IP Scanner defaults failed, trying server-status:', e);
            }

            // FALLBACK 1: Try server status API
            try {
                const response = await fetch('/api/admin/server-status', {
                    headers: { 'Authorization': `Bearer ${localStorage.getItem('token')}` }
                });

                if (response.ok) {
                    const data = await response.json();
                    if (data && data.network_info && data.network_info.cidr) {
                        const cidr = data.network_info.cidr;
                        if (type === 'ssh') {
                            this.sshSettings.scan_targets = cidr;
                        } else if (type === 'snmp') {
                            this.snmpSettings.scan_targets = cidr;
                        }
                        window.showToast && window.showToast(`IP aralığı tespit edildi: ${cidr}`, 'success');
                        return;
                    }
                }
            } catch (e) {
                console.log('Server status detection failed, trying network-info:', e);
            }

            // FALLBACK 2: Try the dedicated network-info endpoint
            try {
                const response = await fetch('/api/admin/inventory/network-info', {
                    headers: { 'Authorization': `Bearer ${localStorage.getItem('token')}` }
                });

                if (response.ok) {
                    const data = await response.json();
                    if (data && data.cidr) {
                        if (type === 'ssh') {
                            this.sshSettings.scan_targets = data.cidr;
                        } else if (type === 'snmp') {
                            this.snmpSettings.scan_targets = data.cidr;
                        }
                        window.showToast && window.showToast(`IP aralığı tespit edildi: ${data.cidr}`, 'success');
                        return;
                    }
                }
            } catch (e) {
                console.error('Auto-detect failed:', e);
                window.showToast && window.showToast('IP aralığı tespit edilemedi', 'error');
            }
        },

        async saveUnifiedSettings() {
            this.settingsSaving = true;
            try {
                // Save AD settings
                await this.saveADSettings();
                // Save SNMP settings if enabled
                if (this.discoveryMethods.snmp) {
                    await this.saveSNMPSettings();
                }
                // Save WinRM settings if enabled
                if (this.discoveryMethods.winrm) {
                    await this.saveWinRMSettings();
                }
                // Save SSH settings if enabled
                if (this.discoveryMethods.ssh) {
                    await this.saveSSHSettings();
                }
                window.showToast && window.showToast('Tüm ayarlar kaydedildi', 'success');
            } catch (e) {
                console.error('Settings save error:', e);
                window.showToast && window.showToast('Ayarlar kaydedilemedi', 'error');
            } finally {
                this.settingsSaving = false;
            }
        },

        async startPreviewDiscovery() {
            // Build methods array first to check what's enabled
            const methods = [];
            if (this.discoveryMethods.ldap) methods.push('ldap');
            methods.push('dns');  // DNS always active (no longer optional)
            if (this.discoveryMethods.snmp) methods.push('snmp');
            if (this.discoveryMethods.winrm) methods.push('winrm');
            if (this.discoveryMethods.ssh) methods.push('ssh');

            // Validation: Only require LDAP credentials if LDAP is enabled
            if (this.discoveryMethods.ldap) {
                if (!this.adSettings.domain_controller) {
                    window.showToast && window.showToast('LDAP için Domain Controller adresini girin', 'error');
                    return;
                }
                if (!this.adSettings.bind_username) {
                    window.showToast && window.showToast('LDAP için kullanıcı adı girin', 'error');
                    return;
                }
            }

            // Check if at least one method is enabled (besides DNS which is always on)
            if (methods.length === 1 && methods[0] === 'dns') {
                window.showToast && window.showToast('En az bir keşif yöntemi seçin (LDAP, SSH veya SNMP)', 'warning');
                return;
            }

            this.discovering = true;
            this.discoveryResults = [];
            this.selectedScanResults = [];

            try {

                const payload = {
                    methods: methods,
                    ad_settings: this.adSettings,
                    snmp_settings: this.discoveryMethods.snmp ? this.snmpSettings : null,
                    winrm_settings: this.discoveryMethods.winrm ? this.winrmSettings : null,
                    ssh_settings: this.discoveryMethods.ssh ? this.sshSettings : null,
                    crack_detection_enabled: this.crackDetectionEnabled
                };

                const res = await fetch('/api/inventory/discover-preview', {
                    method: 'POST',
                    headers: {
                        'Authorization': 'Bearer ' + localStorage.getItem('token'),
                        'Content-Type': 'application/json'
                    },
                    body: JSON.stringify(payload)
                });

                if (res.ok) {
                    const result = await res.json();
                    this.discoveryResults = result.items || [];
                    // Auto-select new items
                    this.selectedScanResults = this.discoveryResults
                        .filter(i => !i.already_exists)
                        .map(i => i.temp_id);
                    // Switch to results tab with animation
                    setTimeout(() => {
                        this.discoverySettingsTab = 'results';
                    }, 300);
                    window.showToast && window.showToast(
                        `Keşif tamamlandı! ${result.new_items} yeni, ${result.existing} mevcut varlık bulundu`,
                        'success'
                    );
                } else {
                    const data = await res.json();
                    window.showToast && window.showToast(data.error || 'Keşif hatası', 'error');
                }
            } catch (e) {
                console.error('Discovery error:', e);
                window.showToast && window.showToast('Keşif sırasında hata oluştu', 'error');
            } finally {
                this.discovering = false;
            }
        },

        async importSelectedItems() {
            if (this.selectedScanResults.length === 0) {
                window.showToast && window.showToast('Lütfen en az bir varlık seçin', 'warning');
                return;
            }

            const newCount = this.discoveryResults.filter(i => this.selectedScanResults.includes(i.temp_id) && !i.already_exists).length;
            const updateCount = this.discoveryResults.filter(i => this.selectedScanResults.includes(i.temp_id) && i.already_exists).length;
            let confirmMsg = '';
            if (newCount > 0 && updateCount > 0) {
                confirmMsg = `${newCount} yeni varlık eklenecek, ${updateCount} mevcut varlık güncellenecek. Onaylıyor musunuz?`;
            } else if (updateCount > 0) {
                confirmMsg = `${updateCount} mevcut varlık güncellenecek. Onaylıyor musunuz?`;
            } else {
                confirmMsg = `${newCount} varlık envantere eklenecek. Onaylıyor musunuz?`;
            }
            if (!confirm(confirmMsg)) {
                return;
            }

            this.importing = true;
            try {
                const selectedItems = this.discoveryResults.filter(
                    i => this.selectedScanResults.includes(i.temp_id)
                );

                const res = await fetch('/api/inventory/import-scanned', {
                    method: 'POST',
                    headers: {
                        'Authorization': 'Bearer ' + localStorage.getItem('token'),
                        'Content-Type': 'application/json'
                    },
                    body: JSON.stringify({ items: selectedItems })
                });

                if (res.ok) {
                    const result = await res.json();
                    console.log('[IMPORT] Response:', JSON.stringify(result));

                    // Hata varsa göster
                    if (result.errors && result.errors.length > 0) {
                        console.error('[IMPORT] Errors:', result.errors);
                        const parts = [];
                        if (result.imported > 0) parts.push(`${result.imported} eklendi`);
                        if (result.updated > 0) parts.push(`${result.updated} güncellendi`);
                        parts.push(`${result.errors.length} hata: ${result.errors[0]}`);
                        window.showToast && window.showToast(
                            parts.join(', '),
                            (result.imported > 0 || result.updated > 0) ? 'warning' : 'error'
                        );
                    } else {
                        const parts = [];
                        if (result.imported > 0) parts.push(`${result.imported} eklendi`);
                        if (result.updated > 0) parts.push(`${result.updated} güncellendi`);
                        if (result.skipped > 0) parts.push(`${result.skipped} atlandı`);
                        window.showToast && window.showToast(
                            `Başarılı! ${parts.join(', ')}`,
                            (result.imported > 0 || result.updated > 0) ? 'success' : 'warning'
                        );
                    }

                    if (result.imported > 0 || result.updated > 0) {
                        // Close modal and reload inventory
                        this.showADSettingsModal = false;
                        await this.loadItems();
                        // Reset
                        this.discoveryResults = [];
                        this.selectedScanResults = [];
                        this.discoverySettingsTab = 'settings';
                    }
                } else {
                    const data = await res.json();
                    window.showToast && window.showToast(data.error || 'İçe aktarma hatası', 'error');
                    if (data.errors && data.errors.length > 0) {
                        console.error('[IMPORT] Errors:', data.errors);
                    }
                }
            } catch (e) {
                console.error('Import error:', e);
                window.showToast && window.showToast('İçe aktarma sırasında hata oluştu', 'error');
            } finally {
                this.importing = false;
            }
        },

        // ========================================
        // WinRM Settings
        // ========================================
        winrmSettings: {
            username: '',
            password: '',
            port: 5985,
            use_ssl: false,
            timeout_seconds: 30,
            concurrent_limit: 5,
            retry_count: 2,
            is_enabled: false,
            last_test_at: null,
            last_test_result: null,
            last_test_error: null
        },
        winrmSaving: false,
        winrmTesting: false,
        winrmTestResult: null,
        winrmTestIP: '',

        // ========================================
        // SSH Settings (Linux/macOS)
        // ========================================
        sshSettings: {
            username: 'root',
            password: '',
            port: 22,
            timeout: 10
        },

        async loadWinRMSettings() {
            try {
                const res = await fetch('/api/inventory/winrm-settings', {
                    headers: { 'Authorization': 'Bearer ' + localStorage.getItem('token') }
                });
                if (res.ok) {
                    const data = await res.json();
                    this.winrmSettings = {
                        username: data.username || '',
                        password: '', // Never returned from server
                        port: data.port || 5985,
                        use_ssl: data.use_ssl || false,
                        timeout_seconds: data.timeout_seconds || 30,
                        concurrent_limit: data.concurrent_limit || 5,
                        retry_count: data.retry_count || 2,
                        is_enabled: data.is_enabled || false,
                        last_test_at: data.last_test_at,
                        last_test_result: data.last_test_result,
                        last_test_error: data.last_test_error
                    };
                }
            } catch (e) {
                console.error('WinRM settings load error:', e);
            }
        },

        async saveWinRMSettings() {
            this.winrmSaving = true;
            try {
                const payload = {
                    username: this.winrmSettings.username,
                    password: this.winrmSettings.password || '********',
                    port: parseInt(this.winrmSettings.port) || 5985,
                    use_ssl: this.winrmSettings.use_ssl,
                    timeout_seconds: parseInt(this.winrmSettings.timeout_seconds) || 30,
                    concurrent_limit: parseInt(this.winrmSettings.concurrent_limit) || 5,
                    retry_count: parseInt(this.winrmSettings.retry_count) || 2,
                    is_enabled: this.winrmSettings.is_enabled
                };

                const res = await fetch('/api/inventory/winrm-settings', {
                    method: 'POST',
                    headers: {
                        'Authorization': 'Bearer ' + localStorage.getItem('token'),
                        'Content-Type': 'application/json'
                    },
                    body: JSON.stringify(payload)
                });

                if (res.ok) {
                    window.showToast && window.showToast('WinRM ayarları kaydedildi', 'success');
                } else {
                    const data = await res.json();
                    window.showToast && window.showToast(data.error || 'Kaydetme hatası', 'error');
                }
            } catch (e) {
                console.error('WinRM settings save error:', e);
                window.showToast && window.showToast('Bağlantı hatası', 'error');
            } finally {
                this.winrmSaving = false;
            }
        },

        async testWinRMConnection() {
            if (!this.winrmTestIP) {
                window.showToast && window.showToast('Test için hedef IP gerekli', 'error');
                return;
            }

            this.winrmTesting = true;
            this.winrmTestResult = null;
            try {
                // First save the settings
                await this.saveWinRMSettings();

                // Then test
                const res = await fetch('/api/inventory/winrm-settings/test?target_ip=' + encodeURIComponent(this.winrmTestIP), {
                    method: 'POST',
                    headers: { 'Authorization': 'Bearer ' + localStorage.getItem('token') }
                });

                if (res.ok) {
                    this.winrmTestResult = await res.json();
                    if (this.winrmTestResult.success) {
                        window.showToast && window.showToast(this.winrmTestResult.message, 'success');
                    } else {
                        window.showToast && window.showToast(this.winrmTestResult.message, 'error');
                    }
                    await this.loadWinRMSettings();
                } else {
                    const data = await res.json();
                    this.winrmTestResult = { success: false, message: data.error || 'Test hatası' };
                    window.showToast && window.showToast(this.winrmTestResult.message, 'error');
                }
            } catch (e) {
                console.error('WinRM test error:', e);
                this.winrmTestResult = { success: false, message: 'Bağlantı hatası' };
                window.showToast && window.showToast('Bağlantı hatası', 'error');
            } finally {
                this.winrmTesting = false;
            }
        },

        async loadSSHSettings() {
            try {
                const res = await fetch('/api/inventory/ssh-settings', {
                    headers: { 'Authorization': 'Bearer ' + localStorage.getItem('token') }
                });
                if (res.ok) {
                    const data = await res.json();
                    this.sshSettings = {
                        username: data.username || 'root',
                        password: data.has_password ? '********' : '', // Show placeholder if password exists
                        port: data.port || 22,
                        timeout: data.timeout || 10,
                        scan_targets: data.scan_targets || '',
                        is_enabled: data.is_enabled || false
                    };
                }
            } catch (e) {
                console.error('SSH settings load error:', e);
            }
        },

        async saveSSHSettings() {
            this.sshSaving = true;
            try {
                const payload = {
                    username: this.sshSettings.username,
                    password: this.sshSettings.password || '********',
                    port: parseInt(this.sshSettings.port) || 22,
                    timeout: parseInt(this.sshSettings.timeout) || 10,
                    is_enabled: this.sshSettings.is_enabled
                };

                const res = await fetch('/api/inventory/ssh-settings', {
                    method: 'POST',
                    headers: {
                        'Authorization': 'Bearer ' + localStorage.getItem('token'),
                        'Content-Type': 'application/json'
                    },
                    body: JSON.stringify(payload)
                });

                if (res.ok) {
                    window.showToast && window.showToast('SSH ayarları kaydedildi', 'success');
                } else {
                    const data = await res.json();
                    window.showToast && window.showToast(data.error || 'Kaydetme hatası', 'error');
                }
            } catch (e) {
                console.error('SSH settings save error:', e);
                window.showToast && window.showToast('Bağlantı hatası', 'error');
            } finally {
                this.sshSaving = false;
            }
        },

        formatDateTime(dateStr) {
            if (!dateStr) return '-';
            try {
                const date = new Date(dateStr);
                if (isNaN(date.getTime())) return '-';
                return date.toLocaleString('tr-TR', {
                    day: '2-digit',
                    month: '2-digit',
                    year: 'numeric',
                    hour: '2-digit',
                    minute: '2-digit'
                });
            } catch (e) {
                return '-';
            }
        },

        // ========================================
        // WinRM Enrichment (Hardware Collection)
        // ========================================
        selectedItems: [],
        selectAll: false,
        bulkDeleting: false,
        showEnrichModal: false,
        enriching: false,
        enrichmentProgress: 0,
        enrichmentResults: null,

        toggleSelectAll() {
            if (this.selectAll) {
                // Select all items
                this.selectedItems = this.items.map(item => item.id);
            } else {
                this.selectedItems = [];
            }
        },

        toggleItemSelection(itemId) {
            const idx = this.selectedItems.indexOf(itemId);
            if (idx > -1) {
                this.selectedItems.splice(idx, 1);
            } else {
                this.selectedItems.push(itemId);
            }
            this.selectAll = this.selectedItems.length === this.items.filter(i => i.ip_address).length;
        },

        isSelected(itemId) {
            return this.selectedItems.includes(itemId);
        },

        getSelectedWithIP() {
            return this.items.filter(item =>
                this.selectedItems.includes(item.id) && item.ip_address
            );
        },

        async bulkDeleteSelected() {
            if (!this.selectedItems.length || this.bulkDeleting) return;

            const approved = window.confirm(`${this.selectedItems.length} seçili varlığı silmek istediğinize emin misiniz?`);
            if (!approved) return;

            this.bulkDeleting = true;
            let successCount = 0;
            let failCount = 0;

            try {
                for (const id of this.selectedItems) {
                    try {
                        const res = await fetch(`/api/inventory/${id}`, {
                            method: 'DELETE',
                            headers: { 'Authorization': 'Bearer ' + localStorage.getItem('token') }
                        });
                        if (res.ok) {
                            successCount++;
                        } else {
                            failCount++;
                        }
                    } catch (_) {
                        failCount++;
                    }
                }

                this.selectedItems = [];
                this.selectAll = false;
                await Promise.all([this.loadStats(), this.loadItems()]);

                if (window.SYS && window.SYS.showToast) {
                    if (failCount === 0) {
                        window.SYS.showToast(`${successCount} varlık silindi`, 'success');
                    } else {
                        window.SYS.showToast(`${successCount} silindi, ${failCount} silinemedi`, 'warning');
                    }
                }
            } finally {
                this.bulkDeleting = false;
            }
        },

        async startEnrichment() {
            const selectedWithIP = this.getSelectedWithIP();
            if (selectedWithIP.length === 0) {
                window.showToast && window.showToast('IP adresi olan en az bir varlık seçin', 'error');
                return;
            }

            this.showEnrichModal = true;
            this.enriching = true;
            this.enrichmentProgress = 0;
            this.enrichmentResults = null;

            try {
                const res = await fetch('/api/inventory/enrich-winrm', {
                    method: 'POST',
                    headers: {
                        'Authorization': 'Bearer ' + localStorage.getItem('token'),
                        'Content-Type': 'application/json'
                    },
                    body: JSON.stringify({
                        inventory_ids: this.selectedItems
                    })
                });

                if (res.ok) {
                    this.enrichmentResults = await res.json();
                    this.enrichmentProgress = 100;

                    const msg = `${this.enrichmentResults.total_success} başarılı, ${this.enrichmentResults.total_failed} başarısız`;
                    if (this.enrichmentResults.total_success > 0) {
                        window.showToast && window.showToast('Donanım bilgileri toplandı: ' + msg, 'success');
                    } else {
                        window.showToast && window.showToast('Zenginleştirme tamamlandı: ' + msg, 'warning');
                    }

                    // Refresh items
                    await this.loadItems();
                } else {
                    const data = await res.json();
                    this.enrichmentResults = { error: data.error || 'Zenginleştirme hatası' };
                    window.showToast && window.showToast(data.error || 'Zenginleştirme hatası', 'error');
                }
            } catch (e) {
                console.error('Enrichment error:', e);
                this.enrichmentResults = { error: 'Bağlantı hatası' };
                window.showToast && window.showToast('Bağlantı hatası', 'error');
            } finally {
                this.enriching = false;
            }
        },

        closeEnrichModal() {
            this.showEnrichModal = false;
            this.enrichmentResults = null;
            this.selectedItems = [];
            this.selectAll = false;
        },

        getErrorCategoryLabel(category) {
            const labels = {
                'dns_error': 'DNS Hatası',
                'timeout': 'Zaman Aşımı',
                'auth_failed': 'Kimlik Doğrulama',
                'winrm_disabled': 'WinRM Kapalı',
                'ssl_error': 'SSL/TLS Hatası',
                'connection_refused': 'Bağlantı Reddedildi',
                'unknown_error': 'Bilinmeyen Hata'
            };
            return labels[category] || category;
        },

        getErrorCategoryClass(category) {
            const classes = {
                'dns_error': 'bg-purple-100 text-purple-700 dark:bg-purple-500/20 dark:text-purple-300',
                'timeout': 'bg-amber-100 text-amber-700 dark:bg-amber-500/20 dark:text-amber-300',
                'auth_failed': 'bg-red-100 text-red-700 dark:bg-red-500/20 dark:text-red-300',
                'winrm_disabled': 'bg-orange-100 text-orange-700 dark:bg-orange-500/20 dark:text-orange-300',
                'ssl_error': 'bg-pink-100 text-pink-700 dark:bg-pink-500/20 dark:text-pink-300',
                'connection_refused': 'bg-gray-100 text-gray-700 dark:bg-gray-500/20 dark:text-gray-300',
                'unknown_error': 'bg-gray-100 text-gray-700 dark:bg-gray-500/20 dark:text-gray-300',
                'no_response': 'bg-amber-100 text-amber-700 dark:bg-amber-500/20 dark:text-amber-300',
                'unreachable': 'bg-red-100 text-red-700 dark:bg-red-500/20 dark:text-red-300'
            };
            return classes[category] || 'bg-gray-100 text-gray-700';
        },

        // ========================================
        // SNMP Settings
        // ========================================
        snmpSettings: {
            version: 'v2c',
            community_strings: '["public"]',
            port: 161,
            timeout_seconds: 5,
            retry_count: 2,
            concurrent_limit: 10,
            is_enabled: false,
            scan_targets: ''
        },
        snmpSaving: false,
        snmpTesting: false,
        snmpTestResult: null,
        snmpTestIP: '',

        async loadSNMPSettings() {
            try {
                const res = await fetch('/api/inventory/snmp-settings', {
                    headers: { 'Authorization': 'Bearer ' + localStorage.getItem('token') }
                });
                if (res.ok) {
                    const data = await res.json();
                    this.snmpSettings = {
                        version: data.version || 'v2c',
                        community_strings: data.community_strings || '["public"]',
                        port: data.port || 161,
                        timeout_seconds: data.timeout_seconds || 5,
                        retry_count: data.retry_count || 2,
                        concurrent_limit: data.concurrent_limit || 10,
                        is_enabled: data.is_enabled || false,
                        scan_targets: data.scan_targets || this.snmpSettings.scan_targets || '' // Preserve existing value
                    };
                }
            } catch (e) {
                console.error('SNMP settings load error:', e);
            }
        },

        async saveSNMPSettings() {
            this.snmpSaving = true;
            try {
                const payload = {
                    version: this.snmpSettings.version,
                    community_strings: this.snmpSettings.community_strings,
                    port: parseInt(this.snmpSettings.port) || 161,
                    timeout_seconds: parseInt(this.snmpSettings.timeout_seconds) || 5,
                    retry_count: parseInt(this.snmpSettings.retry_count) || 2,
                    concurrent_limit: parseInt(this.snmpSettings.concurrent_limit) || 10,
                    is_enabled: this.snmpSettings.is_enabled
                };

                const res = await fetch('/api/inventory/snmp-settings', {
                    method: 'POST',
                    headers: {
                        'Authorization': 'Bearer ' + localStorage.getItem('token'),
                        'Content-Type': 'application/json'
                    },
                    body: JSON.stringify(payload)
                });

                if (res.ok) {
                    window.showToast && window.showToast('SNMP ayarları kaydedildi', 'success');
                } else {
                    const data = await res.json();
                    window.showToast && window.showToast(data.error || 'Kaydetme hatası', 'error');
                }
            } catch (e) {
                console.error('SNMP settings save error:', e);
                window.showToast && window.showToast('Bağlantı hatası', 'error');
            } finally {
                this.snmpSaving = false;
            }
        },

        async testSNMPConnection() {
            if (!this.snmpTestIP) {
                window.showToast && window.showToast('Test için hedef IP gerekli', 'error');
                return;
            }

            this.snmpTesting = true;
            this.snmpTestResult = null;
            try {
                // First save the settings
                await this.saveSNMPSettings();

                // Then test
                const res = await fetch('/api/inventory/snmp-settings/test?target_ip=' + encodeURIComponent(this.snmpTestIP), {
                    method: 'POST',
                    headers: { 'Authorization': 'Bearer ' + localStorage.getItem('token') }
                });

                if (res.ok) {
                    this.snmpTestResult = await res.json();
                    if (this.snmpTestResult.success) {
                        window.showToast && window.showToast(this.snmpTestResult.message, 'success');
                    } else {
                        window.showToast && window.showToast(this.snmpTestResult.message, 'error');
                    }
                } else {
                    const data = await res.json();
                    this.snmpTestResult = { success: false, message: data.error || 'Test hatası' };
                    window.showToast && window.showToast(this.snmpTestResult.message, 'error');
                }
            } catch (e) {
                console.error('SNMP test error:', e);
                this.snmpTestResult = { success: false, message: 'Bağlantı hatası' };
                window.showToast && window.showToast('Bağlantı hatası', 'error');
            } finally {
                this.snmpTesting = false;
            }
        },

        // SNMP Batch Enrichment
        async startSNMPEnrichment() {
            const selectedWithIP = this.getSelectedWithIP();
            if (selectedWithIP.length === 0) {
                window.showToast && window.showToast('IP adresi olan en az bir varlık seçin', 'error');
                return;
            }

            this.showEnrichModal = true;
            this.enriching = true;
            this.enrichmentProgress = 0;
            this.enrichmentResults = null;

            try {
                const res = await fetch('/api/inventory/enrich-snmp', {
                    method: 'POST',
                    headers: {
                        'Authorization': 'Bearer ' + localStorage.getItem('token'),
                        'Content-Type': 'application/json'
                    },
                    body: JSON.stringify({
                        inventory_ids: this.selectedItems
                    })
                });

                if (res.ok) {
                    this.enrichmentResults = await res.json();
                    this.enrichmentProgress = 100;

                    const msg = `${this.enrichmentResults.total_success} başarılı, ${this.enrichmentResults.total_failed} başarısız`;
                    if (this.enrichmentResults.total_success > 0) {
                        window.showToast && window.showToast('SNMP bilgileri toplandı: ' + msg, 'success');
                    } else {
                        window.showToast && window.showToast('SNMP taraması tamamlandı: ' + msg, 'warning');
                    }

                    // Refresh items
                    await this.loadItems();
                } else {
                    const data = await res.json();
                    this.enrichmentResults = { error: data.error || 'SNMP zenginleştirme hatası' };
                    window.showToast && window.showToast(data.error || 'SNMP zenginleştirme hatası', 'error');
                }
            } catch (e) {
                console.error('SNMP Enrichment error:', e);
                this.enrichmentResults = { error: 'Bağlantı hatası' };
                window.showToast && window.showToast('Bağlantı hatası', 'error');
            } finally {
                this.enriching = false;
            }
        }
    };
}
// Email logs Alpine.js component
function emailLogsApp() {
    return {
        loading: true,
        error: null,
        logs: [],
        recentLogs: [],
        summary: {
            total_sent: 0,
            total_failed: 0,
            success_rate: 0
        },
        filters: {
            status: '',
            recipient: '',
            limit: 50
        },
        pagination: {
            total: 0,
            limit: 50,
            offset: 0,
            has_more: false
        },

        async init() {
            await this.loadDashboardData();
            await this.loadLogs();
        },

        async loadDashboardData() {
            try {
                const response = await fetch('/api/email-logs/dashboard', {
                    headers: {
                        'Authorization': 'Bearer ' + localStorage.getItem('token')
                    }
                });
                
                if (response.ok) {
                    const data = await response.json();
                    this.summary = data.summary;
                    this.recentLogs = data.recent_logs;
                }
            } catch (error) {
                console.error('Dashboard data load error:', error);
            }
        },

        async loadLogs() {
            this.loading = true;
            this.error = null;
            
            try {
                const params = new URLSearchParams({
                    limit: this.filters.limit,
                    offset: this.pagination.offset
                });
                
                if (this.filters.status) {
                    params.append('status', this.filters.status);
                }
                
                if (this.filters.recipient) {
                    params.append('recipient', this.filters.recipient);
                }

                const response = await fetch(`/api/email-logs?${params}`, {
                    headers: {
                        'Authorization': 'Bearer ' + localStorage.getItem('token')
                    }
                });

                if (response.ok) {
                    const data = await response.json();
                    this.logs = data.logs;
                    this.pagination = data.pagination;
                } else {
                    const errorData = await response.json();
                    this.error = errorData.error || 'Email logları yüklenemedi';
                }
            } catch (error) {
                this.error = 'Bağlantı hatası: ' + error.message;
            } finally {
                this.loading = false;
            }
        },

        async applyFilters() {
            this.pagination.offset = 0;
            await this.loadLogs();
        },

        clearFilters() {
            this.filters = {
                status: '',
                recipient: '',
                limit: 50
            };

            this.pagination.offset = 0;
            this.loadLogs();
        },

        async refreshLogs() {
            await this.loadDashboardData();
            await this.loadLogs();
        },

        async nextPage() {
            if (this.pagination.has_more) {
                this.pagination.offset += this.pagination.limit;
                await this.loadLogs();
            }
        },

        async previousPage() {
            if (this.pagination.offset > 0) {
                this.pagination.offset = Math.max(0, this.pagination.offset - this.pagination.limit);
                await this.loadLogs();
            }
        },

        async updateStatistics() {
            try {
                const response = await fetch('/api/email-logs/update-statistics', {
                    method: 'POST',
                    headers: {
                        'Authorization': 'Bearer ' + localStorage.getItem('token')
                    }
                });

                if (response.ok) {
                    Toast.success('İstatistikler güncellendi');
                    await this.refreshLogs();
                } else {
                    Toast.error('İstatistik güncelleme hatası');
                }
            } catch (error) {
                Toast.error('Bağlantı hatası');
            }
        },

        exportLogs() {
            // İstatistik sayfasına yönlendir
            window.app.navigateTo('email-statistics');
        }
    }
}

// Make emailLogsApp globally available
window.emailLogsApp = emailLogsApp;

// Global tag suggestion functions
function loadAvailableTags() {
    fetch('/api/targets/tags/available', {
        headers: {
            'Authorization': 'Bearer ' + localStorage.getItem('token')
        }
    })
    .then(response => response.json())
    .then(data => {
        // Convert tag objects to tag names for backward compatibility
        availableTags = (data.tags || []).map(tagObj => tagObj.name || tagObj);
        populateTagSuggestions();
    })
    .catch(error => {
        console.error('Error loading tags:', error);
    });
}

function populateTagSuggestions() {
    const suggestionsList = document.getElementById('tag-suggestions-list');
    if (!suggestionsList) return;

    suggestionsList.innerHTML = '';
    
    if (availableTags.length === 0) {
        suggestionsList.innerHTML = '<div class="px-3 py-2 text-sm text-gray-500 dark:text-gray-400">Henüz etiket bulunmuyor</div>';
        return;
    }

    availableTags.forEach(tagName => {
        const tagElement = document.createElement('div');
        tagElement.className = 'px-3 py-2 text-sm text-gray-700 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-600 cursor-pointer';
        tagElement.textContent = tagName;
        tagElement.onclick = () => selectTag(tagName);
        suggestionsList.appendChild(tagElement);
    });
}

function selectTag(tag) {
    const tagsInput = document.getElementById('target-tags');
    const currentValue = tagsInput.value.trim();
    
    if (currentValue === '') {
        tagsInput.value = tag;
    } else {
        // Check if tag already exists
        const existingTags = currentValue.split(',').map(t => t.trim());
        if (!existingTags.includes(tag)) {
            tagsInput.value = currentValue + ',' + tag;
        }
    }
    
    hideTagSuggestions();
    tagsInput.focus();
}

function showTagSuggestions() {
    const suggestions = document.getElementById('tag-suggestions');
    if (suggestions) {
        suggestions.classList.remove('hidden');
        tagSuggestionsVisible = true;
        loadAvailableTags();
    }
}

function hideTagSuggestions() {
    // Delay hiding to allow click events to fire
    setTimeout(() => {
        const suggestions = document.getElementById('tag-suggestions');
        if (suggestions) {
            suggestions.classList.add('hidden');
            tagSuggestionsVisible = false;
        }
    }, 200);
}

function toggleTagSuggestions() {
    if (tagSuggestionsVisible) {
        hideTagSuggestions();
    } else {
        showTagSuggestions();
    }
}

function filterTagSuggestions() {
    const tagsInput = document.getElementById('target-tags');
    const filter = tagsInput.value.toLowerCase();
    const suggestionsList = document.getElementById('tag-suggestions-list');
    
    if (!suggestionsList) return;

    const tagElements = suggestionsList.children;
    for (let i = 0; i < tagElements.length; i++) {
        const tag = tagElements[i].textContent.toLowerCase();
        if (tag.includes(filter)) {
            tagElements[i].style.display = 'block';
        } else {
            tagElements[i].style.display = 'none';
        }
    }
}

// Global target search and filter functionality - now uses server-side filtering
window.filterTargets = function() {
    const searchInput = document.getElementById('target-search');
    const typeFilter = document.getElementById('type-filter');
    const statusFilter = document.getElementById('status-filter');
    const tagFilter = document.getElementById('tag-filter');

    const searchFilter = searchInput ? searchInput.value.toLowerCase() : '';
    const typeFilterValue = typeFilter ? typeFilter.value : '';
    const statusFilterValue = statusFilter ? statusFilter.value : '';
    const tagFilterValue = tagFilter ? tagFilter.value : '';

    // Save current filter state
    currentFilters.search = searchFilter;
    currentFilters.type = typeFilterValue;
    currentFilters.status = statusFilterValue;
    currentFilters.tag = tagFilterValue;

    console.log('Filter changed, reloading targets with filters:', currentFilters);

    // Reset pagination to first page when filters change
    if (typeof targetsPagination !== 'undefined') {
        targetsPagination.offset = 0;
    }

    // Reload targets from server with filters
    if (window.loadTargets) {
        window.loadTargets();
    }
}

// Check if a row matches current filters
function checkRowMatchesFilters(row) {
    const cells = row.getElementsByTagName('td');

    // Expecting: [0]=checkbox, [1]=name, [2]=address, [3]=type, [4]=status, [5]=tags
    if (cells.length < 6) return true;

    // Search filter (name, address, type, status, tags)
    if (currentFilters.search !== '') {
        let searchMatch = false;
        for (let j = 0; j < cells.length - 1; j++) { // -1 to exclude actions column
            const cellText = cells[j].textContent.toLowerCase();
            if (cellText.includes(currentFilters.search)) {
                searchMatch = true;
                break;
            }
        }
        if (!searchMatch) return false;
    }

    // Type filter (column index 3)
    if (currentFilters.type !== '') {
        const typeCell = cells[3];
        const typeText = typeCell.textContent.toLowerCase().trim();
        if (typeText !== currentFilters.type.toLowerCase()) {
            return false;
        }
    }

    // Status filter (column index 4)
    if (currentFilters.status !== '') {
        const statusCell = cells[4];
        const statusText = statusCell.textContent.toLowerCase();
        if (!statusText.includes(currentFilters.status)) {
            return false;
        }
    }

    // Tag filter (column index 5)
    if (currentFilters.tag !== '') {
        const tagCell = cells[5];
        const tagText = tagCell.textContent.toLowerCase();
        if (!tagText.includes(currentFilters.tag.toLowerCase())) {
            return false;
        }
    }

    return true;
}

// Clear all filters
window.clearFilters = function() {
    const searchInput = document.getElementById('target-search');
    const typeFilter = document.getElementById('type-filter');
    const statusFilter = document.getElementById('status-filter');
    const tagFilter = document.getElementById('tag-filter');

    if (searchInput) searchInput.value = '';
    if (typeFilter) typeFilter.value = '';
    if (statusFilter) statusFilter.value = '';
    if (tagFilter) tagFilter.value = '';

    window.filterTargets();
}

// App.js - Ana uygulama mantığı
function app() {
    return {
        // State
        sidebarOpen: false,
        currentRoute: 'dashboard',
        navigationParams: {}, // Store params for navigation (e.g., filters)
        isDark: false,
        latestSystemHealthData: null,
        serverStats: {
            cpu: 0,
            ram: { usedGb: 0, totalGb: 0, percent: 0 },
            disk: { usedGb: 0, totalGb: 0, percent: 0 },
            temperature: 0
        },
        searchQuery: '',
        userEmail: '',
        userRole: '',
        userLicenseExpiry: null,
        licenseExpired: false,
        licenseExpiresSoon: false,
        licenseExpiryText: '',
        userModulePermissions: {},
        alertCount: 0,

        ipAlertsUnreadCount: 0,
        ipAlerts: [],
        ipAlertsOpen: false,
        ipAlertsLoading: false,
        ipAlertsPoller: null,
        targetsCache: [], // Cache for targets to check if IP exists

        // Service Alerts
        serviceAlertsCount: 0,
        serviceAlerts: [],
        serviceAlertsLoading: false,
        serviceAlertsPoller: null,

        // Modals
        notificationsModalOpen: false,
        serviceAlertsModalOpen: false,

        ws: null,
        reconnectAttempts: 0,
        maxReconnectAttempts: 5,
        
        // Chart instances
        targetHistoryChart: null,
        responseTimeTrendChart: null,
        performanceDistributionChart: null,
        performanceComparisonChart: null,

        // IP Blacklist data
        ipBlacklist: [],
        ipBlacklistSummary: {
            totalIPs: 0,
            highRiskIPs: 0,
            torIPs: 0,
            whitelistedIPs: 0
        },
        canEditIPBlacklist: false,
        showIPCheckModal: false,
        showIPDetailModal: false,
        ipToCheck: '',
        ipCheckResult: null,
        ipDetail: null,
        checkingIP: false,
        ipQueryLimitInfo: {
            used: 0,
            limit: 0
        },

        // Reporting data
        reportingSummary: {
            totalReports: 0,
            completedReports: 0,
            generatingReports: 0,
            lastGeneratedAt: null
        },
        reports: [],
        generatingReport: false,
        reportForm: {
            type: 'system_overview',
            format: 'pdf',
            date_range: {
                start: '',
                end: ''
            },
            targets: []
        },
        reportPreview: {
            loading: false,
            data: null,
            error: null
        },
        showReportPreviewModal: false,
        
        ipDetectionsModalOpen: false,

        openNotificationsModal() {
            this.notificationsModalOpen = true;
            document.documentElement.classList.add('overflow-hidden');
            document.body.classList.add('overflow-hidden');
        },

        closeNotificationsModal() {
            this.notificationsModalOpen = false;
            document.documentElement.classList.remove('overflow-hidden');
            document.body.classList.remove('overflow-hidden');
        },

        openIPDetectionsModal() {
            this.ipDetectionsModalOpen = true;
            this.refreshIPAlerts?.();
            document.documentElement.classList.add('overflow-hidden');
            document.body.classList.add('overflow-hidden');
        },

        closeIPDetectionsModal() {
            this.ipDetectionsModalOpen = false;
            document.documentElement.classList.remove('overflow-hidden');
            document.body.classList.remove('overflow-hidden');
        },

        openServiceAlertsModal() {
            this.serviceAlertsModalOpen = true;
            this.refreshServiceAlerts();
            document.documentElement.classList.add('overflow-hidden');
            document.body.classList.add('overflow-hidden');
        },

        closeServiceAlertsModal() {
            this.serviceAlertsModalOpen = false;
            document.documentElement.classList.remove('overflow-hidden');
            document.body.classList.remove('overflow-hidden');
        },

        // Initialize
        async init() {
            this.loadTheme();
            // CRITICAL: Wait for user info and permissions to load before navigation
            await this.loadUserInfo();
            this.initWebSocket();
            this.setupHTMXListeners();
            this.startIPAlertsPolling();
            this.startServiceAlertsPolling();

            // Make app instance globally available
            window.app = this;

            // Make permission helper functions globally available for page-level controls
            window.canViewModule = (moduleName) => this.canViewModule(moduleName);
            window.canEditModule = (moduleName) => this.canEditModule(moduleName);

            // Persist current page on refresh via hash routing
            window.addEventListener('hashchange', () => { this.handleRouteChange(); });
            this.handleRouteChange();

            // Make toggleHTTPFields globally available
            window.toggleHTTPFields = this.toggleHTTPFields;
        },

        startIPAlertsPolling() {
            this.stopIPAlertsPolling();
            this.fetchIPAlertsUnreadCount();
            this.ipAlertsPoller = setInterval(() => {
                this.fetchIPAlertsUnreadCount();
            }, 30000);
        },

        stopIPAlertsPolling() {
            if (this.ipAlertsPoller) {
                clearInterval(this.ipAlertsPoller);
                this.ipAlertsPoller = null;
            }
        },

        toggleIPAlertsDropdown() {
            this.ipAlertsOpen = !this.ipAlertsOpen;
            if (this.ipAlertsOpen) {
                this.refreshIPAlerts();
            }
        },

        async fetchIPAlertsUnreadCount() {
            try {
                const token = localStorage.getItem('token') || '';
                if (!token) return;

                const response = await fetch('/api/network/ip-alerts/unread-count', {
                    headers: { 'Authorization': 'Bearer ' + token }
                });
                if (!response.ok) return;

                const data = await response.json();
                this.ipAlertsUnreadCount = Number(data?.count || 0);
            } catch (e) {
                // ignore
            }
        },

        async refreshIPAlerts() {
            this.ipAlertsLoading = true;
            try {
                const token = localStorage.getItem('token') || '';
                if (!token) return;

                // Load IP alerts
                const response = await fetch('/api/network/ip-alerts?unacked=1&limit=50', {
                    headers: { 'Authorization': 'Bearer ' + token }
                });
                if (!response.ok) return;

                const data = await response.json();
                this.ipAlerts = Array.isArray(data) ? data : [];
                this.ipAlertsUnreadCount = this.ipAlerts.length;

                // Load targets cache for checking if IP exists
                await this.loadTargetsCache();
            } catch (e) {
                this.ipAlerts = [];
            } finally {
                this.ipAlertsLoading = false;
            }
        },

        async ackAllIPAlerts() {
            try {
                const token = localStorage.getItem('token') || '';
                if (!token) return;

                const response = await fetch('/api/network/ip-alerts/ack-all', {
                    method: 'POST',
                    headers: { 'Authorization': 'Bearer ' + token }
                });
                if (!response.ok) return;

                this.ipAlertsUnreadCount = 0;
                this.ipAlerts = [];
                this.ipAlertsOpen = false;
            } catch (e) {
                // ignore
            }
        },

        async ackIPAlert(id) {
            try {
                const token = localStorage.getItem('token') || '';
                if (!token) return;

                const response = await fetch(`/api/network/ip-alerts/${id}/ack`, {
                    method: 'POST',
                    headers: { 'Authorization': 'Bearer ' + token }
                });
                if (!response.ok) return;

                this.ipAlerts = this.ipAlerts.filter(a => a.id !== id);
                this.ipAlertsUnreadCount = Math.max(0, this.ipAlertsUnreadCount - 1);
            } catch (e) {
                // ignore
            }
        },

        // Service Alerts Functions
        startServiceAlertsPolling() {
            this.stopServiceAlertsPolling();
            this.fetchServiceAlertsCount();
            this.serviceAlertsPoller = setInterval(() => {
                this.fetchServiceAlertsCount();
            }, 30000); // Poll every 30 seconds
        },

        stopServiceAlertsPolling() {
            if (this.serviceAlertsPoller) {
                clearInterval(this.serviceAlertsPoller);
                this.serviceAlertsPoller = null;
            }
        },

        async fetchServiceAlertsCount() {
            try {
                const token = localStorage.getItem('token') || '';
                if (!token) return;

                const response = await fetch('/api/service-alerts/count', {
                    headers: { 'Authorization': 'Bearer ' + token }
                });
                if (!response.ok) return;

                const data = await response.json();
                this.serviceAlertsCount = Number(data?.count || 0);
            } catch (e) {
                // ignore
            }
        },

        async refreshServiceAlerts() {
            this.serviceAlertsLoading = true;
            try {
                const token = localStorage.getItem('token') || '';
                if (!token) return;

                const response = await fetch('/api/service-alerts?limit=100', {
                    headers: { 'Authorization': 'Bearer ' + token }
                });
                if (!response.ok) return;

                const data = await response.json();
                this.serviceAlerts = Array.isArray(data?.alerts) ? data.alerts : [];
                this.serviceAlertsCount = this.serviceAlerts.length;
            } catch (e) {
                this.serviceAlerts = [];
            } finally {
                this.serviceAlertsLoading = false;
            }
        },

        formatDateTime(dateString) {
            if (!dateString) return '-';
            const date = new Date(dateString);
            return date.toLocaleString('tr-TR', {
                year: 'numeric',
                month: '2-digit',
                day: '2-digit',
                hour: '2-digit',
                minute: '2-digit'
            });
        },

        // Load targets to check if an IP is already a target
        async loadTargetsCache() {
            try {
                const token = localStorage.getItem('token') || '';
                if (!token) return;

                const response = await fetch('/api/targets?limit=10000', {
                    headers: { 'Authorization': 'Bearer ' + token }
                });
                if (!response.ok) return;

                const data = await response.json();
                this.targetsCache = Array.isArray(data?.targets) ? data.targets : [];
            } catch (e) {
                this.targetsCache = [];
            }
        },

        // Check if an IP is already in the targets list
        isIPInTargets(ip) {
            if (!ip || !Array.isArray(this.targetsCache)) return false;
            return this.targetsCache.some(t => t.address === ip);
        },

        // Create a target from IP alert
        async createTargetFromIPAlert(alert) {
            try {
                const token = localStorage.getItem('token') || '';
                if (!token) {
                    this.showToast('error', 'Oturum bulunamadı');
                    return;
                }

                // Prepare target data in the correct format
                const targetData = {
                    devices: [{
                        ip: alert.ip,
                        hostname: alert.ip // Use IP as hostname (will be used as name if empty)
                    }],
                    interval_sec: 120,  // Default ping interval: 120 seconds
                    timeout_ms: 5000,   // Default timeout: 5 seconds
                    tags: ''            // No automatic tag
                };

                const response = await fetch('/api/network/ip-scanner/targets', {
                    method: 'POST',
                    headers: {
                        'Authorization': 'Bearer ' + token,
                        'Content-Type': 'application/json'
                    },
                    body: JSON.stringify(targetData)
                });

                if (!response.ok) {
                    const errorData = await response.json().catch(() => ({}));
                    this.showToast('error', errorData.error || errorData.message || 'Hedef oluşturulamadı');
                    return;
                }

                const result = await response.json();

                // Check if there were duplicates
                if (result.duplicates && result.duplicates.length > 0) {
                    this.showToast('warning', `${alert.ip} zaten hedefler listesinde mevcut`);
                    // Still refresh and acknowledge
                    await this.loadTargetsCache();
                    await this.ackIPAlert(alert.id);
                    return;
                }

                // Success - show toast
                this.showToast('success', `${alert.ip} hedefler listesine eklendi`);

                // Refresh targets cache
                await this.loadTargetsCache();

                // Acknowledge the alert
                await this.ackIPAlert(alert.id);

            } catch (e) {
                console.error('Error creating target from IP alert:', e);
                this.showToast('error', 'Hedef oluşturulurken bir hata oluştu');
            }
        },

        // Navigation
        navigateTo(route, params = {}) {
            // Check permissions before navigation
            if (route === 'users' && !this.canViewModule('user_management')) {
                this.showToast('error', 'Bu sayfaya erişim yetkiniz bulunmuyor');
                return;
            }
            if (route === 'module-permissions' && !this.canViewModule('module_permissions')) {
                this.showToast('error', 'Bu sayfaya erişim yetkiniz bulunmuyor');
                return;
            }
            if (route === 'targets' && !this.canViewModule('targets')) {
                this.showToast('error', 'Bu sayfaya erişim yetkiniz bulunmuyor');
                return;
            }
            if (route === 'ip-scanner' && !this.canViewModule('ip_scanner')) {
                this.showToast('error', 'Bu sayfaya erişim yetkiniz bulunmuyor');
                return;
            }
            if (route === 'inventory' && !this.canViewModule('inventory')) {
                this.showToast('error', 'Bu sayfaya erişim yetkiniz bulunmuyor');
                return;
            }
            if (route === 'notifications' && !this.canViewModule('notifications')) {
                this.showToast('error', 'Bu sayfaya erişim yetkiniz bulunmuyor');
                return;
            }
            if (route === 'notification-settings' && !this.canViewModule('notification_settings')) {
                this.showToast('error', 'Bu sayfaya erişim yetkiniz bulunmuyor');
                return;
            }
            if (route === 'new-notification-templates' && !this.canViewModule('notification_settings')) {
                this.showToast('error', 'Bu sayfaya erişim yetkiniz bulunmuyor');
                return;
            }
            if (route === 'reporting' && !this.canViewModule('reporting')) {
                this.showToast('error', 'Bu sayfaya erişim yetkiniz bulunmuyor');
                return;
            }
            if (route === 'network-settings' && !this.canViewModule('network_settings')) {
                this.showToast('error', 'Bu sayfaya eri?im yetkiniz bulunmuyor');
                return;
            }
            if (route === 'backup' && !this.canViewModule('backup')) {
                this.showToast('error', 'Bu sayfaya erişim yetkiniz bulunmuyor');
                return;
            }

            // Store params for use after page load
            this.navigationParams = params;

            // Save last route for language switching
            localStorage.setItem('systrack_last_route', route);

            const newHash = '#' + route;
            if (window.location.hash !== newHash) { window.location.hash = newHash; return; }
            this.currentRoute = route;
            this.loadPageContent(route);
        },

        // Load current route from hash on initial load or hash changes
        handleRouteChange() {
            const route = (window.location.hash || '#dashboard').slice(1) || 'dashboard';
            this.currentRoute = route;
            this.loadPageContent(route);
        },

        async loadPageContent(route) {
            const token = localStorage.getItem('token');
            if (!token) {
                this.redirectToLogin();
                return;
            }

            // Check if token is expired
            if (this.isTokenExpired(token)) {
                this.logout();
                return;
            }

            // Map route to module name for permission checking
            const routeToModuleMap = {
                'dashboard': 'dashboard',
                'targets': 'targets',
                'sensors': 'sensors',
                'ip-scanner': 'ip_scanner',
                'inventory': 'inventory',
                'notification-history': 'notifications',
                'notification-settings': 'notification_settings',
                'reporting': 'reporting',
                'users': 'user_management',
                'module-permissions': 'module_permissions',
                'backup': 'backup',
                'network-settings': 'network_settings'
            };

            const moduleName = routeToModuleMap[route];

            // Check module permission (skip for dashboard as it's always accessible)
            if (moduleName && moduleName !== 'dashboard') {
                const hasPermission = this.canViewModule(moduleName);
                console.log(`[loadPageContent] Route: ${route}, Module: ${moduleName}, HasPermission: ${hasPermission}, UserRole: ${this.userRole}, Permissions:`, this.userModulePermissions);

                if (!hasPermission) {
                    this.showToast('error', 'Bu sayfaya erişim yetkiniz bulunmuyor');
                    // Redirect to dashboard
                    window.location.hash = '#dashboard';
                    return;
                }
            }

            // Get current language from localStorage
            const currentLanguage = localStorage.getItem('systrack_language') || 'tr';

            let endpoint = '';
            switch (route) {
                case 'dashboard':
                    endpoint = '/admin/dashboard';
                    break;
                case 'targets':
                    endpoint = '/admin/targets';
                    break;
                case 'reporting':
                    endpoint = '/admin/reporting';
                    break;
                case 'new-notification-templates':
                    endpoint = '/admin/new-notification-templates';
                    break;
                case 'notification-settings':
                    endpoint = '/admin/notification-settings';
                    break;
                case 'users':
                    endpoint = '/admin/users';
                    break;
                case 'module-permissions':
                    endpoint = '/admin/module-permissions';
                    break;
                case 'notification-history':
                    endpoint = '/admin/notifications';
                    break;
                case 'backup':
                    endpoint = '/admin/backup';
                    break;
                case 'network-settings':
                    endpoint = '/admin/network-settings';
                    break;
                case 'ip-scanner':
                    endpoint = '/admin/ip-scanner';
                    break;
                case 'inventory':
                    endpoint = '/admin/inventory';
                    break;
                case 'sensors':
                    endpoint = '/admin/sensors';
                    break;
                case 'sensors':
                    endpoint = '/admin/sensors';
                    break;
                case 'server-metrics':
                    endpoint = '/admin/server-metrics.html';
                    break;
                default:
                    endpoint = '/admin/dashboard';
            }

            // Add language parameter
            endpoint += `?lang=${currentLanguage}`;

            try {
                const response = await fetch(endpoint, {
                    headers: {
                        'Authorization': `Bearer ${token}`,
                        'Content-Type': 'text/html'
                    }
                });

                if (response.status === 401) {
                    // Clear invalid/expired token to avoid redirect loops.
                    this.logout();
                    return;
                }

                if (response.ok) {
                    const html = await response.text();
                    const viewRoot = document.getElementById('view-root');
                    viewRoot.innerHTML = html;

                    if (window.Alpine && typeof window.Alpine.initTree === 'function') {
                        window.Alpine.initTree(viewRoot);
                    }
                   
                   // Route-specific initialization
                   if (route === 'targets') {
                       // Reset pagination state when loading targets page
                       if (typeof targetsPagination !== 'undefined') {
                           targetsPagination.offset = 0;
                           targetsPagination.limit = 25;
                           targetsPagination.total = 0;
                           targetsPagination.hasMore = false;
                           targetsPagination.loading = false;
                       }

                       setTimeout(() => {
                           // Set the page size select to default value
                           const pageSizeSelect = document.getElementById('page-size-select');
                           if (pageSizeSelect) {
                               pageSizeSelect.value = '25';
                           }

                           if (window.loadTargetFilterOptions) {
                               window.loadTargetFilterOptions();
                           }

                           const navParams = this.navigationParams || {};
                           this.navigationParams = {}; // Clear params first
                           const searchValue = navParams.search || localStorage.getItem('systrack_targets_search') || '';
                           if (searchValue) {
                               localStorage.removeItem('systrack_targets_search');
                           }

                           // Apply navigation params (e.g., status filter from dashboard)
                           if (navParams.status || searchValue) {
                               // Wait a bit longer for DOM and variables to be fully ready
                               setTimeout(() => {
                                   if (navParams.status) {
                                       const statusFilter = document.getElementById('status-filter');
                                       if (statusFilter) {
                                           statusFilter.value = navParams.status;
                                           console.log('Set status filter dropdown to:', navParams.status);
                                       }
                                   }

                                   if (searchValue) {
                                       const searchInput = document.getElementById('target-search');
                                       if (searchInput) {
                                           searchInput.value = searchValue;
                                           console.log('Set targets search to:', searchValue);
                                       }
                                   }

                                   // Trigger filter with a small delay to ensure DOM is updated
                                   setTimeout(() => {
                                       if (window.filterTargets) {
                                           console.log('Calling filterTargets() with params');
                                           window.filterTargets();
                                       }
                                   }, 50);
                               }, 100);
                           } else {
                               // Load targets with pagination after page content is loaded (only if no filter applied)
                               if (window.loadTargets) {
                                   console.log('Calling loadTargets() after page reload');
                                   window.loadTargets();
                               }
                           }
                       }, 50);
                   } else if (route === 'dashboard') {
                       // Dashboard: içeriği yüklendikten sonra istatistikleri ve grafikleri başlat
                       setTimeout(() => {
                           this.initCharts();
                           // Kartları ve online/offline tablolarını hemen doldur
                           this.updateDashboardStats();
                       }, 100);
                   } else if (route === 'sensors') {
                       setTimeout(() => {
                           if (window.initSensorsPage) {
                               window.initSensorsPage();
                           }
                       }, 60);
                   } else if (route === 'sla-reports') {
                       // SLA Reports yüklendikten sonra verileri yükle
                       setTimeout(() => {
                           window.loadSLAReportsData();
                       }, 100);
                   } else if (route === 'performance-trends') {
                       // Performance Trends yüklendikten sonra verileri yükle
                       setTimeout(() => {
                           this.loadPerformanceTrendsData();
                       }, 100);
        } else if (route === 'ip-blacklist') {
            // IP Blacklist sayfası yüklendi, Alpine.js otomatik olarak init() çağıracak
            console.log('IP Blacklist page loaded');
            await this.loadIPBlacklist();
        } else if (route === 'whois-domain') {
            // WHOIS Domain sayfası yüklendi
            console.log('WHOIS Domain page loaded');
            // Load query stats when page loads
            setTimeout(() => {
                if (window.loadQueryStats) {
                    window.loadQueryStats();
                }
            }, 100);
        } else if (route === 'domain-registry') {
            // Domain Registry sayfası yüklendi
            console.log('Domain Registry page loaded');
            // Load domain registry data
            setTimeout(() => {
                this.loadDomainRegistry();
            }, 100);
        } else if (route === 'reporting') {
            // Reporting sayfası yüklendi
            console.log('Reporting page loaded');
            await this.loadReportingData();
        } else if (route === 'notification-settings') {
            console.log('Notification Settings page loaded');
            setTimeout(() => {
                if (window.initNotificationSettingsPage) {
                    window.initNotificationSettingsPage();
                }
            }, 100);
        } else if (route === 'new-notification-templates') {
            console.log('Notification Templates page loaded');
            setTimeout(() => {
                if (window.initNotificationTemplatesPage) {
                    window.initNotificationTemplatesPage();
                }
            }, 100);
        } else if (route === 'email-logs') {
            // Email Logs sayfası yüklendi
            console.log('Email Logs page loaded');
            // Initialize email logs component
            setTimeout(() => {
                if (window.emailLogsApp) {
                    // Component is already defined
                    console.log('Email logs component initialized');
                } else {
                    console.error('Email logs component not found');
                }
            }, 100);
        } else if (route === 'backup') {
            console.log('Backup page loaded');
            setTimeout(() => {
                if (window.initBackupPage) {
                    window.initBackupPage();
                }
            }, 100);
        }
        else if (route === 'network-settings') {
            console.log('Network Settings page loaded');
            setTimeout(() => {
                if (window.initNetworkSettingsPage) {
                    window.initNetworkSettingsPage();
                }
            }, 100);
        }
                } else {
                    throw new Error('Failed to load page content');
                }
            } catch (error) {
                console.error('Page load error:', error);
                Toast.show({
                    type: 'error',
                    title: 'Hata',
                    message: 'Sayfa yüklenemedi.',
                    timeout: 5000
                });
            }
        },

        // Load dashboard content
        async loadDashboard() {
            await this.loadPageContent('dashboard');
            // Dashboard yüklendikten sonra grafikleri başlat
            setTimeout(() => {
                this.initCharts();
            }, 100);
        },

        formatNotificationTimestamp(notification) {
            const timestamp = notification.sent_at || notification.created_at;
            if (!timestamp) {
                return '';
            }
            const date = new Date(timestamp);
            if (Number.isNaN(date.getTime())) {
                return '';
            }
            return date.toLocaleString('tr-TR', {
                day: '2-digit',
                month: '2-digit',
                hour: '2-digit',
                minute: '2-digit'
            });
        },

        humanizeIdentifier(value) {
            if (!value) {
                return 'Bilinmiyor';
            }
            return String(value)
                .split(/[_\s]+/)
                .filter(Boolean)
                .map(part => part.charAt(0).toUpperCase() + part.slice(1))
                .join(' ');
        },

        // Chart.js grafikleri
        initCharts() {
            this.initSystemHealthChart();
            this.initStatusChart();
            this.initSLATrendsChart();
            this.initSLAComplianceChart();
            this.initSLABreachChart();
            this.initSLAComparisonChart();

            // Dashboard chart güncelleme timer'larını başlat
            this.startDashboardChartUpdates();
        },

        // Dashboard chart güncellemelerini başlat
        startDashboardChartUpdates() {
            // Sistem Sağlığı: Her 10 dakikada bir güncelle (API son 10 dakikalık veri döndürüyor)
            this.systemHealthUpdateInterval = setInterval(() => {
                if (this.currentRoute === 'dashboard') {
                    this.refreshSystemHealthChart();
                }
            }, 10 * 60 * 1000); // 10 dakika

            console.log(' Dashboard chart update timers started');
            console.log('   - System Health Chart: Every 10 minutes (10-min data window)');
            console.log('   - Status Chart: Updates with dashboard_stats WebSocket (30 sec)');
        },

        // Dashboard timer'larını durdur (sayfa değiştiğinde)
        stopDashboardChartUpdates() {
            if (this.systemHealthUpdateInterval) {
                clearInterval(this.systemHealthUpdateInterval);
                this.systemHealthUpdateInterval = null;
            }
            console.log('Dashboard chart update timers stopped');
        },

        // Sistem Sağlık Chart'ını yenile (10 dakikalık interval)
        async refreshSystemHealthChart() {
            if (!this.systemHealthChartInstance?.data?.datasets?.[0]) return;

            try {
                const data = await this.loadSystemHealthData();
                this.updateSystemGraphMeta(data);
                this.latestSystemHealthData = data;
                this.updateSystemHealthSummary(data);

                const axes = (data && Array.isArray(data.axes)) ? data.axes : [];
                if (!axes.length) return;

                const datasetValues = axes.map(axis => {
                    if (typeof axis.score === 'number') return axis.score;
                    if (typeof axis.success_rate === 'number') return Math.round(axis.success_rate * 1000) / 10;
                    return 0;
                });

                // Chart datasını array yerine tek tek güncelle
                const dataset = this.systemHealthChartInstance.data.datasets[0];
                datasetValues.forEach((value, index) => {
                    if (dataset.data[index] !== undefined) {
                        dataset.data[index] = value;
                    }
                });

                // Manuel render tetikle (update() yerine)
                if (typeof this.systemHealthChartInstance.render === 'function') {
                    this.systemHealthChartInstance.render();
                }

                console.log(' System Health Chart updated');
            } catch (e) {
                // Sessizce geç
            }
        },

        // Status Chart'ın hazır olmasını bekle
        waitForStatusChart(maxWaitMs = 3000) {
            return new Promise((resolve, reject) => {
                if (this.statusChartInstance && this.statusChartInstance.data) {
                    resolve();
                    return;
                }

                const startTime = Date.now();
                const checkInterval = setInterval(() => {
                    if (this.statusChartInstance && this.statusChartInstance.data) {
                        clearInterval(checkInterval);
                        resolve();
                    } else if (Date.now() - startTime > maxWaitMs) {
                        clearInterval(checkInterval);
                        reject(new Error('Timeout waiting for Status Chart'));
                    }
                }, 100);
            });
        },

        // Status Chart'ı güncelle (dashboard_stats WebSocket mesajından)
        updateStatusChartFromStats(stats) {
            if (!this.statusChartInstance?.data?.datasets?.[0]) return;

            try {
                const online = stats.onlineTargets || 0;
                const offline = stats.offlineTargets || 0;

                // Chart datasını direkt güncelle
                const dataset = this.statusChartInstance.data.datasets[0];
                dataset.data = [online, offline];

                // Manuel render tetikle (update() yerine)
                if (typeof this.statusChartInstance.update === 'function') {
                    this.statusChartInstance.update();
                } else if (typeof this.statusChartInstance.render === 'function') {
                    this.statusChartInstance.render();
                }
                if (typeof this.statusChartInstance.resize === 'function') {
                    this.statusChartInstance.resize();
                }

                console.log(` Status Chart: ${online} online, ${offline} offline`);
            } catch (e) {
                // Sessizce geç
            }
        },
        // Güçlü Status Chart güncelleme (canvas değiştiyse yeniden oluşturur)
        forceUpdateStatusChartFromStats(stats) {
            try {
                const canvas = document.getElementById('statusChart');
                if (!canvas) return;
                const online = stats.onlineTargets || 0;
                const offline = stats.offlineTargets || 0;
                if (this.statusChartInstance?.destroy) {
                    try { this.statusChartInstance.destroy(); } catch (_) {}
                }
                this.statusChartInstance = new Chart(canvas, {
                    type: 'doughnut',
                    data: { labels: ['Online', 'Offline'], datasets: [{ data: [online, offline], backgroundColor: ['#10b981', '#ef4444'], borderWidth: 0 }] },
                    options: { responsive: true, maintainAspectRatio: false, animation: { duration: 900, easing: 'easeInOutCubic' }, plugins: { legend: { position: 'bottom' } } }
                });
            } catch (_) { /* no-op */ }
        },

        // Sistem Sağlığı Radar Chart'ını yeniden kur (destroy + new)
        forceRecreateSystemHealthChartFromData(data) {
            try {
                const ctx = document.getElementById('systemHealthChart');
                if (!ctx) return;

                const axes = (data && Array.isArray(data.axes)) ? data.axes : [];
                if (!axes.length) return;

                const labels = axes.map(axis => {
                    const key = axis.label || axis.key || '-';
                    return dashboardTranslations.chartLabels[key] || key;
                });
                const datasetValues = axes.map(axis => {
                    if (typeof axis.score === 'number') return axis.score;
                    if (typeof axis.success_rate === 'number') return Math.round(axis.success_rate * 1000) / 10;
                    return 0;
                });

                const chartColors = this.isDark
                    ? { border: 'rgba(96, 165, 250, 0.9)', background: 'rgba(59, 130, 246, 0.15)', point: '#93c5fd', grid: 'rgba(156, 163, 175, 0.25)', label: '#f9fafb' }
                    : { border: 'rgba(37, 99, 235, 0.9)', background: 'rgba(59, 130, 246, 0.12)', point: '#2563eb', grid: 'rgba(209, 213, 219, 0.6)', label: '#111827' };

                const axisPalettes = datasetValues.map(value => this.getHealthPalette(value));
                const axisPointColors = axisPalettes.map(palette => palette.chart);
                const axisLabelColors = axisPalettes.map(palette => palette.labelColor);

                if (this.systemHealthChartInstance?.destroy) {
                    try { this.systemHealthChartInstance.destroy(); } catch (_) {}
                }

                this.systemHealthChartInstance = new Chart(ctx, {
                    type: 'radar',
                    data: {
                        labels,
                        datasets: [{
                            label: 'Başarı Oranı',
                            data: datasetValues,
                            backgroundColor: chartColors.background,
                            borderColor: chartColors.border,
                            pointBackgroundColor: axisPointColors,
                            pointBorderColor: axisPointColors,
                            pointHoverBackgroundColor: '#ffffff',
                            pointHoverBorderColor: axisPointColors,
                            borderWidth: 2,
                            pointRadius: 4,
                            pointHoverRadius: 7,
                            pointHitRadius: 10,
                            fill: true,
                            tension: 0.2
                        }]
                    },
                    options: {
                        responsive: true,
                            maintainAspectRatio: false,
                            interaction: { mode: 'nearest', intersect: false },
                        animation: { duration: 1000, easing: 'easeInOutCubic' },
                        scales: {
                            r: {
                                suggestedMin: 0,
                                suggestedMax: 100,
                                ticks: { display: false },
                                angleLines: { color: chartColors.grid },
                                grid: { color: chartColors.grid },
                                pointLabels: { color: (ctx) => axisLabelColors[ctx.index] || chartColors.label, font: { size: 12, weight: '500' } }
                            }
                        },
                        plugins: { 
                            legend: { display: false }, 
                            tooltip: { 
                                callbacks: { 
                                    label: (ctx) => {
                                        try {
                                            const idx = ctx.dataIndex ?? 0;
                                            const axis = (Array.isArray(axes) && axes[idx]) ? axes[idx] : {};
                                            let successRate = Number.isFinite(axis?.success_rate) 
                                                ? Math.round(axis.success_rate * 1000) / 10 
                                                : (Number.isFinite(axis?.score) ? axis.score : (Number.isFinite(ctx.parsed?.r) ? Math.round(ctx.parsed.r * 10) / 10 : parseFloat(ctx.formattedValue)));
                                            if (!Number.isFinite(successRate)) successRate = 0;
                                            const failureRate = Math.max(0, Math.round((100 - successRate) * 10) / 10);
                                            const successCount = (axis && (axis.success_count ?? axis.successCount));
                                            const failureCount = (axis && (axis.failure_count ?? axis.failureCount));

                                            const lines = [];
                                            lines.push(`Başarı: ${successRate.toFixed(1)}%${Number.isFinite(successCount) ? ` (${successCount})` : ''}`);
                                            lines.push(`Başarısız: ${failureRate.toFixed(1)}%${Number.isFinite(failureCount) ? ` (${failureCount})` : ''}`);
                                            return lines;
                                        } catch (_) {
                                            return `${ctx.formattedValue}%`;
                                        }
                                    }
                                } 
                            } 
                        }
                    }
                });
            } catch (_) { /* no-op */ }
        },

        // WebSocket ile sistem sağlığı verisini tazele ve grafikleri yeniden kur
        async refreshSystemHealthFromWebSocket() {
            if (this.currentRoute !== 'dashboard') return;
            try {
                const data = await this.loadSystemHealthData();
                this.updateSystemGraphMeta(data);
                this.latestSystemHealthData = data;
                this.updateSystemHealthSummary(data);
                this.forceRecreateSystemHealthChartFromData(data);
            } catch (_) { /* ignore */ }
        },

        // Debounced redraw helpers
        scheduleStatusChartRedraw(stats) {
            try {
                if (this._statusChartTimer) clearTimeout(this._statusChartTimer);
                this._lastStatusStats = stats;
                this._statusChartTimer = setTimeout(() => {
                    this.forceUpdateStatusChartFromStats(this._lastStatusStats || {});
                }, 750);
            } catch (_) { /* no-op */ }
        },

        scheduleSystemHealthRedraw() {
            try {
                if (this._systemHealthTimer) clearTimeout(this._systemHealthTimer);
                this._systemHealthTimer = setTimeout(() => {
                    this.refreshSystemHealthFromWebSocket();
                }, 1000);
            } catch (_) { /* no-op */ }
        },


        initSystemHealthChart() {
            const ctx = document.getElementById('systemHealthChart');
            if (!ctx) return;

            this.loadSystemHealthData()
                .then(data => {
                    this.updateSystemGraphMeta(data);
                    this.latestSystemHealthData = data;
                    this.updateSystemHealthSummary(data);

                    const axes = (data && Array.isArray(data.axes)) ? data.axes : [];
                    if (!axes.length) {
                        return;
                    }

                    const labels = axes.map(axis => {
                        const key = axis.label || axis.key || '-';
                        return dashboardTranslations.chartLabels[key] || key;
                    });
                    const datasetValues = axes.map(axis => {
                        if (typeof axis.score === 'number') {
                            return axis.score;
                        }
                        if (typeof axis.success_rate === 'number') {
                            return Math.round(axis.success_rate * 1000) / 10;
                        }
                        return 0;
                    });

                    const chartColors = this.isDark
                        ? {
                            border: 'rgba(96, 165, 250, 0.9)',
                            background: 'rgba(59, 130, 246, 0.15)',
                            point: '#93c5fd',
                            grid: 'rgba(156, 163, 175, 0.25)',
                            label: '#f9fafb'
                        }
                        : {
                            border: 'rgba(37, 99, 235, 0.9)',
                            background: 'rgba(59, 130, 246, 0.12)',
                            point: '#2563eb',
                            grid: 'rgba(209, 213, 219, 0.6)',
                            label: '#111827'
                        };

                    const axisPalettes = datasetValues.map(value => this.getHealthPalette(value));
                    const axisPointColors = axisPalettes.map(palette => palette.chart);
                    const axisLabelColors = axisPalettes.map(palette => palette.labelColor);
                    const tooltipData = axes;

                    // Chart instance'ı sakla (güncelleme için)
                    this.systemHealthChartInstance = new Chart(ctx, {
                        type: 'radar',
                        data: {
                            labels,
                            datasets: [{
                                label: 'Başarı Oranı',
                                data: datasetValues,
                                backgroundColor: chartColors.background,
                                borderColor: chartColors.border,
                                pointBackgroundColor: axisPointColors,
                                pointBorderColor: axisPointColors,
                                pointHoverBackgroundColor: '#ffffff',
                                pointHoverBorderColor: axisPointColors,
                                borderWidth: 2,
                                pointRadius: 4,
                                pointHoverRadius: 6,
                                fill: true,
                                tension: 0.2
                            }]
                        },
                    options: {
                        responsive: true,
                        maintainAspectRatio: false,
                        animation: { duration: 1000, easing: 'easeInOutCubic' },
                        scales: {
                            r: {
                                suggestedMin: 0,
                                suggestedMax: 100,
                                ticks: {
                                        display: false
                                    },
                                    angleLines: {
                                        color: chartColors.grid
                                    },
                                    grid: {
                                        color: chartColors.grid
                                    },
                                    pointLabels: {
                                        color: (context) => axisLabelColors[context.index] || chartColors.label,
                                        font: { size: 12, weight: '500' }
                                    }
                                }
                            },
                            plugins: {
                                legend: {
                                    display: false
                                },
                                tooltip: {
                                    callbacks: {
                                        title(items) {
                                            const item = items && items.length ? items[0] : null;
                                            return item ? item.label : '';
                                        },
                                        label: (context) => {
                                            const axis = tooltipData[context.dataIndex];
                                            if (!axis) {
                                                return `${context.formattedValue}% başarı`;
                                            }

                                            const success = axis.success_count ?? 0;
                                            const failure = axis.failure_count ?? 0;
                                            const total = axis.total_count ?? success + failure;
                                            const successRate = typeof axis.success_rate === 'number'
                                                ? (axis.success_rate * 100).toFixed(1)
                                                : context.formattedValue;
                                            const failureRate = typeof axis.failure_rate === 'number'
                                                ? (axis.failure_rate * 100).toFixed(1)
                                                : (100 - Number(successRate)).toFixed(1);

                                            const rows = [
                                                `Başarı: ${success} (${successRate}%)`,
                                                `Başarısız: ${failure} (${failureRate}%)`
                                            ];

                                            if (axis.static) {
                                                rows.push('Statik Veri');
                                            }

                                            return rows;
                                        }
                                    }
                                }
                            }
                        }
                    });
                })
                .catch(error => {
                    console.error('System health chart init failed:', error);
                    this.updateSystemHealthSummary();
                });
        },

        updateSystemGraphMeta(data) {
            const rangeEl = document.querySelector('[data-system-graph-range]');
            if (rangeEl && data && data.time_range_hours) {
                rangeEl.textContent = `Son ${data.time_range_hours} saat`;
            }

            const updatedEl = document.querySelector('[data-system-graph-updated]');
            if (updatedEl && data && data.generated_at) {
                const generatedAt = new Date(data.generated_at);
                if (!Number.isNaN(generatedAt.getTime())) {
                    updatedEl.textContent = generatedAt.toLocaleString('tr-TR', {
                        day: '2-digit',
                        month: '2-digit',
                        hour: '2-digit',
                        minute: '2-digit'
                    });
                }
            }
        },

        updateSystemHealthSummary(data = {}) {
            const axes = (data && Array.isArray(data.axes)) ? data.axes : [];
            const numericScore = Number.isFinite(data?.overall_score)
                ? data.overall_score
                : this.calculateAverageScore(axes);
            const normalizedScore = this.normalizeScore(numericScore);
            const palette = this.getHealthPalette(normalizedScore);
            const displayScore = Number.isFinite(normalizedScore)
                ? Math.round(normalizedScore)
                : null;
            const overallLabelKey = data?.overall_label || palette.label;
            const overallLabel = dashboardTranslations.healthLabels[overallLabelKey] || overallLabelKey;

            const scoreEl = document.querySelector('[data-overall-health-score]');
            if (scoreEl) {
                scoreEl.textContent = displayScore !== null ? `${displayScore}%` : '--%';
                scoreEl.style.color = palette.textColor;
            }

            const labelEl = document.querySelector('[data-overall-health-label]');
            if (labelEl) {
                labelEl.textContent = overallLabel;
                labelEl.style.color = palette.textColor;
            }

            const descriptionEl = document.querySelector('[data-overall-health-description]');
            if (descriptionEl) {
                if (displayScore !== null) {
                    const description = dashboardTranslations.healthDescriptions[overallLabelKey] || 'Sistem durumu belirleniyor...';
                    descriptionEl.textContent = description;
                } else {
                    descriptionEl.textContent = 'Sistem verileri yükleniyor...';
                }
                descriptionEl.style.color = palette.mutedColor;
            }

            const ringEl = document.querySelector('[data-overall-health-ring]');
            if (ringEl) {
                const percent = Number.isFinite(normalizedScore) ? normalizedScore : 0;

                if (ringEl.tagName && ringEl.tagName.toLowerCase() === 'circle') {
                    let circumference = parseFloat(ringEl.getAttribute('data-ring-circumference') || '0');
                    if (!Number.isFinite(circumference) || circumference <= 0) {
                        circumference = 502;
                    }
                    const offset = circumference * (1 - (percent / 100));

                    ringEl.style.strokeDasharray = `${circumference}`;
                    ringEl.style.strokeDashoffset = `${offset}`;
                    if (!ringEl.style.transition) {
                        ringEl.style.transition = 'stroke-dashoffset 900ms ease-in-out, stroke 900ms ease-in-out';
                    }
                    ringEl.style.stroke = palette.chart;
                } else {
                    if (!ringEl.style.transition) {
                        ringEl.style.transition = 'background 900ms ease-in-out';
                    }
                    ringEl.style.background = `conic-gradient(${palette.chart} ${percent}%, ${palette.ringTrack} ${percent}% 100%)`;
                }
            }

            const metricRows = document.querySelectorAll('[data-health-metric]');
            if (metricRows.length) {
                const axisMap = axes.reduce((acc, axis) => {
                    if (axis && axis.key) {
                        acc[String(axis.key).toLowerCase()] = axis;
                    }
                    return acc;
                }, {});

                metricRows.forEach(row => {
                    const key = (row.getAttribute('data-health-metric') || '').toLowerCase();
                    const axis = axisMap[key];
                    const score = this.extractAxisScore(axis);
                    const metricPalette = this.getHealthPalette(score);
                    const valueEl = row.querySelector('[data-health-metric-value]');
                    if (valueEl) {
                        valueEl.textContent = this.formatScoreLabel(score);
                        valueEl.style.color = metricPalette.textColor;
                    }
                    const dotEl = row.querySelector('[data-health-metric-dot]');
                    if (dotEl) {
                        dotEl.style.backgroundColor = metricPalette.chart;
                    }
                });
            }
        },

        getHealthPalette(score) {
            const normalized = this.normalizeScore(score);
            let tone = 'neutral';
            if (Number.isFinite(normalized)) {
                if (normalized >= 95) {
                    tone = 'perfect';
                } else if (normalized >= 70) {
                    tone = 'healthy';
                } else if (normalized >= 55) {
                    tone = 'caution';
                } else if (normalized > 0) {
                    tone = 'critical';
                }
            }

            const palettes = {
                perfect: { label: 'Mükemmel', chart: '#22c55e', text: '#166534', lightText: '#bbf7d0' },
                healthy: { label: 'Sağlıklı', chart: '#84cc16', text: '#4d7c0f', lightText: '#d9f99d' },
                caution: { label: 'İyileştirilmeli', chart: '#f97316', text: '#c2410c', lightText: '#fdba74' },
                critical: { label: 'Kritik', chart: '#ef4444', text: '#b91c1c', lightText: '#fecaca' },
                neutral: { label: 'Bilinmiyor', chart: '#6b7280', text: '#4b5563', lightText: '#d1d5db' }
            };

            const palette = palettes[tone];
            const textColor = this.isDark ? palette.lightText : palette.text;

            return {
                tone,
                label: palette.label,
                chart: palette.chart,
                textColor,
                labelColor: textColor,
                mutedColor: this.isDark ? '#9ca3af' : '#6b7280',
                ringTrack: this.isDark ? '#0f172a' : '#e5e7eb'
            };
        },

        normalizeScore(value) {
            if (!Number.isFinite(value)) {
                return null;
            }
            const clamped = Math.max(0, Math.min(100, value));
            return Math.round(clamped * 10) / 10;
        },

        extractAxisScore(axis) {
            if (!axis) {
                return null;
            }
            if (typeof axis.score === 'number') {
                return this.normalizeScore(axis.score);
            }
            if (typeof axis.success_rate === 'number') {
                return this.normalizeScore(axis.success_rate * 100);
            }
            return null;
        },

        calculateAverageScore(axes) {
            if (!axes || !axes.length) {
                return null;
            }
            let total = 0;
            let counted = 0;
            axes.forEach(axis => {
                const score = this.extractAxisScore(axis);
                if (Number.isFinite(score)) {
                    total += score;
                    counted += 1;
                }
            });
            if (!counted) {
                return null;
            }
            return total / counted;
        },

        formatScoreLabel(value) {
            if (!Number.isFinite(value)) {
                return '--%';
            }
            const normalized = this.normalizeScore(value);
            if (!Number.isFinite(normalized)) {
                return '--%';
            }
            return Number.isInteger(normalized)
                ? `${normalized.toFixed(0)}%`
                : `${normalized.toFixed(1)}%`;
        },

        async loadSystemHealthData() {
            try {
                const response = await fetch('/api/dashboard/system-graph', {
                    headers: {
                        'Authorization': `Bearer ${localStorage.getItem('token')}`
                    }
                });

                if (!response.ok) {
                    throw new Error('Failed to load system graph data');
                }

                return await response.json();
            } catch (error) {
                console.error('Error loading system graph data:', error);
                return { axes: [] };
            }
        },
        // Hedef durumları grafiği
        initStatusChart() {
            const ctx = document.getElementById('statusChart');
            if (!ctx) return;

            // Hedef durumlarını getir
            this.loadStatusData().then(data => {
                // Chart instance sakla
                this.statusChartInstance = new Chart(ctx, {
                    type: 'doughnut',
                    data: {
                        labels: ['Online', 'Offline'],
                        datasets: [{
                            data: [data.online, data.offline],
                            backgroundColor: ['#10b981', '#ef4444'],
                            borderWidth: 0
                        }]
                    },
                    options: {
                        responsive: true,
                        maintainAspectRatio: false,
                        animation: { duration: 900, easing: 'easeInOutCubic' },
                        plugins: {
                            legend: {
                                position: 'bottom',
                                labels: {
                                    color: this.isDark ? '#d1d5db' : '#374151',
                                    padding: 20
                                }
                            }
                        }
                    }
                });
            });
        },
        // Durum verilerini yükle
        async loadStatusData() {
            try {
                const response = await fetch('/api/dashboard/stats', {
                    headers: {
                        'Authorization': `Bearer ${localStorage.getItem('token')}`
                    }
                });
                
                if (!response.ok) {
                    throw new Error('Failed to load status data');
                }
                
                const stats = await response.json();
                
                // API response'unu kontrol et - targets array'i içinde geliyor
                const online = (stats && typeof stats.onlineTargets === 'number') ? stats.onlineTargets : 0;
                const offline = (stats && typeof stats.offlineTargets === 'number')
                    ? stats.offlineTargets
                    : Math.max(0, (stats && typeof stats.totalTargets === 'number' ? stats.totalTargets : 0) - online);
                
                return { online, offline };
            } catch (error) {
                console.error('Error loading status data:', error);
                return { online: 0, offline: 0 };
            }
        },

        // SLA Trends Chart
        initSLATrendsChart() {
            const ctx = document.getElementById('slaTrendsChart');
            if (!ctx) return;

            // SLA trend verilerini getir
            this.loadSLATrendsData().then(data => {
                new Chart(ctx, {
                    type: 'line',
                    data: {
                        labels: data.labels,
                        datasets: [{
                            label: 'Ortalama Uptime (%)',
                            data: data.uptimeData,
                            borderColor: '#8b5cf6',
                            backgroundColor: 'rgba(139, 92, 246, 0.1)',
                            borderWidth: 2,
                            fill: true,
                            tension: 0.4
                        }, {
                            label: 'SLA Hedefi (99.9%)',
                            data: data.slaTarget,
                            borderColor: '#ef4444',
                            backgroundColor: 'rgba(239, 68, 68, 0.1)',
                            borderWidth: 2,
                            borderDash: [5, 5],
                            fill: false
                        }]
                    },
                    options: {
                        responsive: true,
                        maintainAspectRatio: false,
                        plugins: {
                            legend: {
                                labels: {
                                    color: this.isDark ? '#d1d5db' : '#374151'
                                }
                            }
                        },
                        scales: {
                            x: {
                                ticks: {
                                    color: this.isDark ? '#d1d5db' : '#374151'
                                },
                                grid: {
                                    color: this.isDark ? '#374151' : '#e5e7eb'
                                }
                            },
                            y: {
                                min: 90,
                                max: 100,
                                ticks: {
                                    color: this.isDark ? '#d1d5db' : '#374151',
                                    callback: function(value) {
                                        return value + '%';
                                    },
                                    stepSize: 1
                                },
                                grid: {
                                    color: this.isDark ? '#374151' : '#e5e7eb'
                                },
                                afterBuildTicks: function(scale) {
                                    scale.ticks = [
                                        {value: 90, label: '90%'},
                                        {value: 91, label: '91%'},
                                        {value: 92, label: '92%'},
                                        {value: 93, label: '93%'},
                                        {value: 94, label: '94%'},
                                        {value: 95, label: '95%'},
                                        {value: 96, label: '96%'},
                                        {value: 97, label: '97%'},
                                        {value: 98, label: '98%'},
                                        {value: 99, label: '99%'},
                                        {value: 100, label: '100%'}
                                    ];
                                }
                            }
                        }
                    }
                });
            });
        },

        // SLA trend verilerini yükle
        async loadSLATrendsData() {
            try {
                const response = await fetch('/api/analytics/sla/dashboard', {
                    headers: {
                        'Authorization': `Bearer ${localStorage.getItem('token')}`
                    }
                });
                
                if (!response.ok) {
                    throw new Error('Failed to load SLA trends data');
                }
                
                const data = await response.json();
                
                // Son 7 günlük veri için mock data
                const labels = [];
                const uptimeData = [];
                const slaTarget = [];
                
                for (let i = 6; i >= 0; i--) {
                    const date = new Date();
                    date.setDate(date.getDate() - i);
                    labels.push(date.toLocaleDateString('tr-TR', { month: 'short', day: 'numeric' }));
                    
                    // Mock uptime data (gerçek veri geldiğinde değiştirilecek)
                    const baseUptime = data.data?.statistics?.avg_uptime || 95;
                    const variation = (Math.random() - 0.5) * 4;
                    uptimeData.push(Math.max(90, Math.min(100, baseUptime + variation)));
                    
                    slaTarget.push(99.9);
                }
                
                return { labels, uptimeData, slaTarget };
            } catch (error) {
                console.error('Error loading SLA trends data:', error);
                // Fallback data
                const labels = ['Pzt', 'Sal', 'Çar', 'Per', 'Cum', 'Cmt', 'Paz'];
                const uptimeData = [99.5, 99.2, 99.8, 99.1, 99.7, 99.9, 99.6];
                const slaTarget = [99.9, 99.9, 99.9, 99.9, 99.9, 99.9, 99.9];
                return { labels, uptimeData, slaTarget };
            }
        },

        // SLA Compliance Chart
        initSLAComplianceChart() {
            const ctx = document.getElementById('slaComplianceChart');
            if (!ctx) return;

            // SLA compliance verilerini getir
            this.loadSLAComplianceData().then(data => {
                new Chart(ctx, {
                    type: 'doughnut',
                    data: {
                        labels: ['SLA Uyumlu', 'SLA İhlali'],
                        datasets: [{
                            data: [data.compliant, data.breached],
                            backgroundColor: ['#10b981', '#ef4444'],
                            borderWidth: 0
                        }]
                    },
                    options: {
                        responsive: true,
                        maintainAspectRatio: false,
                        plugins: {
                            legend: {
                                position: 'bottom',
                                labels: {
                                    color: this.isDark ? '#d1d5db' : '#374151',
                                    padding: 20
                                }
                            }
                        }
                    }
                });
            });
        },

        // SLA Breach Timeline Chart
        initSLABreachChart() {
            const ctx = document.getElementById('slaBreachChart');
            if (!ctx) return;

            // SLA breach verilerini getir
            this.loadSLABreachData().then(data => {
                new Chart(ctx, {
                    type: 'bar',
                    data: {
                        labels: data.labels,
                        datasets: [{
                            label: 'SLA İhlal Sayısı',
                            data: data.breachData,
                            backgroundColor: '#ef4444',
                            borderColor: '#dc2626',
                            borderWidth: 1
                        }]
                    },
                    options: {
                        responsive: true,
                        maintainAspectRatio: false,
                        plugins: {
                            legend: {
                                labels: {
                                    color: this.isDark ? '#d1d5db' : '#374151'
                                }
                            }
                        },
                        scales: {
                            x: {
                                ticks: {
                                    color: this.isDark ? '#d1d5db' : '#374151'
                                },
                                grid: {
                                    color: this.isDark ? '#374151' : '#e5e7eb'
                                }
                            },
                            y: {
                                beginAtZero: true,
                                ticks: {
                                    color: this.isDark ? '#d1d5db' : '#374151'
                                },
                                grid: {
                                    color: this.isDark ? '#374151' : '#e5e7eb'
                                }
                            }
                        }
                    }
                });
            });
        },

        // SLA compliance verilerini yükle
        async loadSLAComplianceData() {
            try {
                const response = await fetch('/api/analytics/sla/summary', {
                    headers: {
                        'Authorization': `Bearer ${localStorage.getItem('token')}`
                    }
                });
                
                if (!response.ok) {
                    throw new Error('Failed to load SLA compliance data');
                }
                
                const data = await response.json();
                
                // Mock data - gerçek veri geldiğinde değiştirilecek
                const totalTargets = data.data?.total_targets || 10;
                const breachedTargets = data.data?.breached_targets || 2;
                const compliantTargets = totalTargets - breachedTargets;
                
                return { compliant: compliantTargets, breached: breachedTargets };
            } catch (error) {
                console.error('Error loading SLA compliance data:', error);
                return { compliant: 8, breached: 2 };
            }
        },

        // SLA breach verilerini yükle
        async loadSLABreachData() {
            try {
                // Mock data - gerçek API endpoint'i eklenecek
                const labels = ['Pzt', 'Sal', 'Çar', 'Per', 'Cum', 'Cmt', 'Paz'];
                const breachData = [0, 1, 0, 2, 1, 0, 1];
                
                return { labels, breachData };
            } catch (error) {
                console.error('Error loading SLA breach data:', error);
                const labels = ['Pzt', 'Sal', 'Çar', 'Per', 'Cum', 'Cmt', 'Paz'];
                const breachData = [0, 1, 0, 2, 1, 0, 1];
                return { labels, breachData };
            }
        },

        // SLA Comparison Chart
        initSLAComparisonChart() {
            const ctx = document.getElementById('slaComparisonChart');
            if (!ctx) return;

            // SLA comparison verilerini getir
            this.loadSLAComparisonData().then(data => {
                new Chart(ctx, {
                    type: 'bar',
                    data: {
                        labels: data.labels,
                        datasets: [{
                            label: 'Uptime (%)',
                            data: data.uptimeData,
                            backgroundColor: data.colors,
                            borderColor: data.borderColors,
                            borderWidth: 1
                        }]
                    },
                    options: {
                        responsive: true,
                        maintainAspectRatio: false,
                        plugins: {
                            legend: {
                                labels: {
                                    color: this.isDark ? '#d1d5db' : '#374151'
                                }
                            }
                        },
                        scales: {
                            x: {
                                ticks: {
                                    color: this.isDark ? '#d1d5db' : '#374151',
                                    maxRotation: 45
                                },
                                grid: {
                                    color: this.isDark ? '#374151' : '#e5e7eb'
                                }
                            },
                            y: {
                                min: 90,
                                max: 100,
                                ticks: {
                                    color: this.isDark ? '#d1d5db' : '#374151',
                                    callback: function(value) {
                                        return value + '%';
                                    },
                                    stepSize: 1
                                },
                                grid: {
                                    color: this.isDark ? '#374151' : '#e5e7eb'
                                },
                                afterBuildTicks: function(scale) {
                                    scale.ticks = [
                                        {value: 90, label: '90%'},
                                        {value: 91, label: '91%'},
                                        {value: 92, label: '92%'},
                                        {value: 93, label: '93%'},
                                        {value: 94, label: '94%'},
                                        {value: 95, label: '95%'},
                                        {value: 96, label: '96%'},
                                        {value: 97, label: '97%'},
                                        {value: 98, label: '98%'},
                                        {value: 99, label: '99%'},
                                        {value: 100, label: '100%'}
                                    ];
                                }
                            }
                        }
                    }
                });
            });
        },

        // SLA comparison verilerini yükle
        async loadSLAComparisonData() {
            try {
                const response = await fetch('/api/targets', {
                    headers: {
                        'Authorization': `Bearer ${localStorage.getItem('token')}`
                    }
                });
                
                if (!response.ok) {
                    throw new Error('Failed to load targets data');
                }
                
                const data = await response.json();
                const targets = data.targets || [];
                
                // İlk 10 target'ı al ve mock SLA data oluştur
                const labels = targets.slice(0, 10).map(t => t.name);
                const uptimeData = [];
                const colors = [];
                const borderColors = [];
                
                targets.slice(0, 10).forEach((target, index) => {
                    // Mock uptime data
                    const uptime = 95 + Math.random() * 5; // 95-100% arası
                    uptimeData.push(uptime);
                    
                    // Renk belirleme (SLA'ya göre)
                    if (uptime >= 99.9) {
                        colors.push('rgba(16, 185, 129, 0.8)'); // Green
                        borderColors.push('#10b981');
                    } else if (uptime >= 99.0) {
                        colors.push('rgba(245, 158, 11, 0.8)'); // Yellow
                        borderColors.push('#f59e0b');
                    } else {
                        colors.push('rgba(239, 68, 68, 0.8)'); // Red
                        borderColors.push('#ef4444');
                    }
                });
                
                return { labels, uptimeData, colors, borderColors };
            } catch (error) {
                console.error('Error loading SLA comparison data:', error);
                // Fallback data
                const labels = ['Target 1', 'Target 2', 'Target 3', 'Target 4', 'Target 5'];
                const uptimeData = [99.5, 98.2, 99.8, 97.1, 99.7];
                const colors = ['rgba(16, 185, 129, 0.8)', 'rgba(245, 158, 11, 0.8)', 'rgba(16, 185, 129, 0.8)', 'rgba(239, 68, 68, 0.8)', 'rgba(16, 185, 129, 0.8)'];
                const borderColors = ['#10b981', '#f59e0b', '#10b981', '#ef4444', '#10b981'];
                return { labels, uptimeData, colors, borderColors };
            }
        },

        // SLA Reports functions
        async loadSLAReportsData() {
            try {
                const response = await fetch('/api/analytics/sla/summary', {
                    headers: {
                        'Authorization': `Bearer ${localStorage.getItem('token')}`
                    }
                });
                
                if (!response.ok) {
                    throw new Error('Failed to load SLA reports data');
                }
                
                const data = await response.json();
                
                // Update summary cards
                document.getElementById('total-targets').textContent = data.data?.total_targets || 0;
                document.getElementById('sla-compliant').textContent = data.data?.compliant_targets || 0;
                document.getElementById('sla-breached').textContent = data.data?.breached_targets || 0;
                document.getElementById('avg-uptime').textContent = (data.data?.avg_uptime || 0).toFixed(1) + '%';
                
                // Load targets for filter
                await this.loadTargetsForFilter();
                
                // Load SLA reports table
                await this.loadSLAReportsTable();
                
            } catch (error) {
                console.error('Error loading SLA reports data:', error);
            }
        },

        async loadTargetsForFilter() {
            try {
                const response = await fetch('/api/targets', {
                    headers: {
                        'Authorization': `Bearer ${localStorage.getItem('token')}`
                    }
                });
                
                if (!response.ok) {
                    throw new Error('Failed to load targets');
                }
                
                const data = await response.json();
                const targets = data.targets || [];
                
                const targetFilter = document.getElementById('targetFilter');
                if (targetFilter) {
                    // Clear existing options except "all"
                    targetFilter.innerHTML = '<option value="all">Tüm Target\'lar</option>';
                    
                    targets.forEach(target => {
                        const option = document.createElement('option');
                        option.value = target.id;
                        option.textContent = target.name;
                        targetFilter.appendChild(option);
                    });
                }
                
            } catch (error) {
                console.error('Error loading targets for filter:', error);
            }
        },

        async loadSLAReportsTable() {
            try {
                // Get filter values
                const targetFilter = document.getElementById('targetFilter')?.value || 'all';
                const dateRange = document.getElementById('dateRange')?.value || '30';
                const slaThreshold = document.getElementById('slaThreshold')?.value || '95.0';
                
                const tableBody = document.getElementById('sla-reports-table-body');
                
                if (tableBody) {
                    tableBody.innerHTML = '<tr><td colspan="6" class="px-6 py-4 text-center text-gray-500">Yükleniyor...</td></tr>';
                    
                    // Get targets for filtering
                    const targetsResponse = await fetch('/api/targets', {
                        headers: {
                            'Authorization': `Bearer ${localStorage.getItem('token')}`
                        }
                    });
                    const targetsData = await targetsResponse.json();
                    const targets = targetsData.targets || [];
                    
                    // Filter targets based on selection
                    let filteredTargets = targets;
                    if (targetFilter !== 'all') {
                        filteredTargets = targets.filter(target => target && target.id && target.id.toString() === targetFilter);
                    }
                    
                    tableBody.innerHTML = '';
                    
                    // Load SLA summary data (more efficient than individual calls)
                    const slaSummaryResponse = await fetch('/api/analytics/sla/summary', {
                        headers: {
                            'Authorization': `Bearer ${localStorage.getItem('token')}`
                        }
                    });
                    
                    if (!slaSummaryResponse.ok) {
                        throw new Error('Failed to fetch SLA data');
                    }
                    
                    const slaData = await slaSummaryResponse.json();
                    const slaReports = slaData.data.summary || [];
                    
                    // Filter targets if needed
                    let filteredReports = slaReports;
                    if (targetFilter !== 'all') {
                        filteredReports = slaReports.filter(report => report.target_id && report.target_id.toString() === targetFilter);
                    }
                    
                    // Populate table with data
                    if (filteredReports.length === 0) {
                        tableBody.innerHTML = '<tr><td colspan="6" class="px-6 py-4 text-center text-gray-500">Veri bulunamadı</td></tr>';
                        return;
                    }
                    
                    for (const report of filteredReports) {
                        const threshold = parseFloat(slaThreshold);
                        const isCompliant = report.current_uptime >= threshold;
                        
                        const row = document.createElement('tr');
                        row.innerHTML = `
                            <td class="px-6 py-4 whitespace-nowrap text-sm font-medium text-gray-900 dark:text-gray-100">
                                ${report.target_name}
                            </td>
                            <td class="px-6 py-4 whitespace-nowrap text-sm text-gray-500 dark:text-gray-400">
                                ${report.current_uptime.toFixed(1)}%
                            </td>
                            <td class="px-6 py-4 whitespace-nowrap">
                                <span class="px-2 py-1 text-xs font-medium rounded-full ${isCompliant ? 'bg-green-100 text-green-800 dark:bg-green-900/20 dark:text-green-400' : 'bg-red-100 text-red-800 dark:bg-red-900/20 dark:text-red-400'}">
                                    ${isCompliant ? 'Uyumlu' : 'İhlal'}
                                </span>
                            </td>
                            <td class="px-6 py-4 whitespace-nowrap text-sm text-gray-500 dark:text-gray-400">
                                ${isCompliant ? 0 : 1}
                            </td>
                            <td class="px-6 py-4 whitespace-nowrap text-sm text-gray-500 dark:text-gray-400">
                                ${isCompliant ? 'Hiç ihlal yok' : 'Bugün'}
                            </td>
                            <td class="px-6 py-4 whitespace-nowrap text-sm text-gray-500 dark:text-gray-400">
                                <button onclick="viewSLAReport(${report.target_id})" class="text-primary-600 hover:text-primary-900 dark:text-primary-400 dark:hover:text-primary-300">
                                    Detay
                                </button>
                            </td>
                        `;
                        tableBody.appendChild(row);
                    }
                }
                
            } catch (error) {
                console.error('Error loading SLA reports table:', error);
                const tableBody = document.getElementById('sla-reports-table-body');
                if (tableBody) {
                    tableBody.innerHTML = '<tr><td colspan="6" class="px-6 py-4 text-center text-red-500">Veri yüklenirken hata oluştu.</td></tr>';
                }
            }
        },

        // Theme management
        loadTheme() {
            const savedTheme = localStorage.getItem('theme');
            this.isDark = savedTheme === 'dark' || (!savedTheme && window.matchMedia('(prefers-color-scheme: dark)').matches);
            this.applyTheme();
        },

        toggleTheme() {
            this.isDark = !this.isDark;
            this.applyTheme();
            this.updateSystemHealthSummary(this.latestSystemHealthData);
            localStorage.setItem('theme', this.isDark ? 'dark' : 'light');
        },

        applyTheme() {
            if (this.isDark) {
                document.documentElement.classList.add('dark');
            } else {
                document.documentElement.classList.remove('dark');
            }
        },

        // User management
        async loadUserInfo() {
            const token = localStorage.getItem('token');
            if (!token) {
                this.redirectToLogin();
                return;
            }

            // Check if token is expired
            if (this.isTokenExpired(token)) {
                this.logout();
                return;
            }

            // Decode JWT token to get user info
            try {
                const payload = this.decodeJwtPayload(token);
                if (!payload) throw new Error('invalid token payload');
                this.userEmail = payload.email;
                this.userRole = payload.role;
            } catch (e) {
                console.error('Failed to decode token:', e);
                this.redirectToLogin();
            }

            // Load detailed user info including license
            this.loadUserDetails();
            // Load module permissions (MUST complete before navigation)
            await this.loadUserModulePermissions();
        },

        loadUserDetails() {
            const token = localStorage.getItem('token');
            if (!token) return;

            fetch('/api/auth/me', {
                headers: {
                    'Authorization': `Bearer ${token}`
                }
            })
            .then(response => response.json())
            .then(data => {
                if (data.license_expiry) {
                    this.userLicenseExpiry = new Date(data.license_expiry);
                    this.updateLicenseStatus();
                }
            })
            .catch(error => {
                console.error('Failed to load user details:', error);
            });
        },

        updateLicenseStatus() {
            if (!this.userLicenseExpiry) {
                this.licenseExpiryText = 'Sınırsız';
                this.licenseExpired = false;
                this.licenseExpiresSoon = false;
                return;
            }

            const now = new Date();
            const diffTime = this.userLicenseExpiry - now;
            const diffDays = Math.ceil(diffTime / (1000 * 60 * 60 * 24));

            this.licenseExpired = diffTime < 0;
            this.licenseExpiresSoon = diffDays <= 7 && diffDays > 0;

            if (this.licenseExpired) {
                this.licenseExpiryText = 'Süresi Dolmuş';
            } else if (this.licenseExpiresSoon) {
                this.licenseExpiryText = `${diffDays} gün kaldı`;
            } else {
                this.licenseExpiryText = this.userLicenseExpiry.toLocaleDateString('tr-TR');
            }
        },

        get userInitials() {
            return this.userEmail.split('@')[0].substring(0, 2).toUpperCase();
        },

        logout() {
            this.stopIPAlertsPolling();
            localStorage.removeItem('token');
            localStorage.removeItem('user');
            this.redirectToLogin();
        },

        decodeJwtPayload(token) {
            try {
                const payloadPart = String(token || '').split('.')[1];
                if (!payloadPart) return null;

                // JWT uses base64url.
                const base64 = payloadPart.replace(/-/g, '+').replace(/_/g, '/');
                const padded = base64 + '='.repeat((4 - (base64.length % 4)) % 4);

                return JSON.parse(atob(padded));
            } catch (error) {
                return null;
            }
        },

        // We intentionally do NOT enforce exp using browser time here.
        // Some deployments run without NTP/internet and device clock drift causes login loops.
        isTokenExpired(token) {
            return !this.decodeJwtPayload(token);
        },

        // Check if user has permission
        hasPermission(permission) {
            // Role-based permission mapping
            const rolePermissions = {
                'admin': [
                    'dashboard.view', 'target.view', 'target.create', 'target.update', 'target.delete', 'target.ping',
                    'user.view', 'user.create', 'user.update', 'user.delete',
                    'alert.view', 'alert.close', 'alert.create',
                    'report.view', 'report.create', 'report.export', 'report.delete',
                    'settings.view', 'settings.update',
                    'notification.view', 'notification.update', 'notification.test',
                    'analytics.view', 'sla.view', 'network.scan'
                ],
                'user': [
                    'dashboard.view', 'target.view', 'target.create', 'target.update', 'target.ping',
                    'alert.view', 'alert.close',
                    'report.view', 'analytics.view', 'sla.view'
                ],
                'viewer': [
                    'dashboard.view', 'target.view', 'alert.view', 'report.view', 'analytics.view', 'sla.view'
                ]
            };

            const userRole = this.userRole;
            const permissions = rolePermissions[userRole] || [];
            return permissions.includes(permission);
        },

        // Load user module permissions
        async loadUserModulePermissions() {
            const token = localStorage.getItem('token');
            if (!token) return;

            try {
                // Get user ID from token
                const payload = this.decodeJwtPayload(token);
                const userId = payload?.user_id;
                if (!userId) throw new Error('missing user_id in token');
                console.log('[DEBUG] JWT Token Payload:', payload); // Debug log
                console.log('[DEBUG] Loading module permissions for user ID:', userId); // Debug log

                const response = await fetch(`/api/module-permissions/user/${userId}`, {
                    headers: {
                        'Authorization': `Bearer ${token}`
                    }
                });

                if (response.ok) {
                    const data = await response.json();
                    console.log('Module permissions API response:', data); // Debug log
                    this.userModulePermissions = {};

                    // Convert array to object for easier access (check if data.permissions exists)
                    if (data.permissions && Array.isArray(data.permissions)) {
                        data.permissions.forEach(perm => {
                            if (!this.userModulePermissions[perm.module_name]) {
                                this.userModulePermissions[perm.module_name] = {};
                            }
                            this.userModulePermissions[perm.module_name].view = perm.can_view;
                            this.userModulePermissions[perm.module_name].edit = perm.can_edit;
                        });
                    }
                    console.log('User module permissions object:', this.userModulePermissions); // Debug log
                } else {
                    console.error('Failed to load module permissions:', response.status, response.statusText);
                }
            } catch (error) {
                console.error('Error loading module permissions:', error);
            }
        },

        // Check if user can view a module
        canViewModule(moduleName) {
            console.log(`Checking permission for ${moduleName}:`, this.userModulePermissions[moduleName]); // Debug log

            // Admin has access to all modules
            if (this.userRole === 'admin') {
                return true;
            }

            if (!this.userModulePermissions[moduleName]) {
                console.log(`No specific permission found for ${moduleName}, using default for role: ${this.userRole}`); // Debug log
                // Default permissions based on role and new module structure
                const defaultPermissions = {
                    'user': {
                        'dashboard': { view: true, edit: false },
                        'targets': { view: true, edit: true },
                        'ip_scanner': { view: true, edit: true },
                        'inventory': { view: true, edit: true },
                        'notifications': { view: true, edit: false },
                        'notification_settings': { view: true, edit: false },
                        'reporting': { view: true, edit: false },
                        'user_management': { view: false, edit: false },
                        'module_permissions': { view: false, edit: false },
                        'backup': { view: false, edit: false }
                    },
                    'viewer': {
                        'dashboard': { view: true, edit: false },
                        'targets': { view: true, edit: false },
                        'ip_scanner': { view: true, edit: false },
                        'inventory': { view: true, edit: false },
                        'notifications': { view: true, edit: false },
                        'notification_settings': { view: true, edit: false },
                        'reporting': { view: true, edit: false },
                        'user_management': { view: false, edit: false },
                        'module_permissions': { view: false, edit: false },
                        'backup': { view: false, edit: false }
                    }
                };

                const result = defaultPermissions[this.userRole]?.[moduleName]?.view || false;
                console.log(`Default permission for ${moduleName} (${this.userRole}):`, result); // Debug log
                return result;
            }

            const result = this.userModulePermissions[moduleName].view || false;
            console.log(`Specific permission for ${moduleName}:`, result); // Debug log
            return result;
        },

        // Check if user can edit a module
        canEditModule(moduleName) {
            // Admin has edit access to all modules
            if (this.userRole === 'admin') {
                return true;
            }

            // Viewer never has edit access
            if (this.userRole === 'viewer') {
                return false;
            }

            if (!this.userModulePermissions[moduleName]) {
                // Default permissions for user role
                const defaultPermissions = {
                    'user': {
                        'dashboard': { edit: false },
                        'targets': { edit: true },
                        'ip_scanner': { edit: true },
                        'inventory': { edit: true },
                        'notifications': { edit: false },
                        'notification_settings': { edit: false },
                        'reporting': { edit: false },
                        'user_management': { edit: false },
                        'module_permissions': { edit: false },
                        'backup': { edit: false }
                    }
                };

                return defaultPermissions[this.userRole]?.[moduleName]?.edit || false;
            }

            return this.userModulePermissions[moduleName].edit || false;
        },

        redirectToLogin() {
            window.location.href = '/login.html';
        },

        // Show toast notification
        showToast(type, message) {
            if (typeof Toast !== 'undefined') {
                if (type === 'error') {
                    Toast.error(message);
                } else {
                    Toast.show({ type, message });
                }
            } else {
                console.log(`${type.toUpperCase()}: ${message}`);
            }
        },


        // WebSocket
        initWebSocket() {
            const token = localStorage.getItem('token');
            if (!token) return;

            const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
            const wsUrl = `${protocol}//${window.location.host}/ws?token=${token}`;
            
            this.ws = new WebSocket(wsUrl);
            
            this.ws.onopen = () => {
                console.log('WebSocket connected');
                this.reconnectAttempts = 0;
            };

            this.ws.onmessage = (event) => {
                try {
                    // Handle multiple JSON messages in one event
                    const data = event.data;
                    if (typeof data === 'string') {
                        // Split by newlines in case multiple messages are concatenated
                        const messages = data.split('\n').filter(msg => msg.trim());
                        for (const messageStr of messages) {
                            try {
                                const message = JSON.parse(messageStr);
                                this.handleWebSocketMessage(message);
                            } catch (parseError) {
                                console.warn('Failed to parse individual WebSocket message:', parseError, messageStr);
                            }
                        }
                    } else {
                        const message = JSON.parse(data);
                        this.handleWebSocketMessage(message);
                    }
                } catch (e) {
                    console.error('Failed to parse WebSocket message:', e, event.data);
                }
            };

            this.ws.onclose = () => {
                console.log('WebSocket disconnected');
                this.reconnectWebSocket();
            };

            this.ws.onerror = (error) => {
                console.error('WebSocket error:', error);
            };
        },

        reconnectWebSocket() {
            if (this.reconnectAttempts < this.maxReconnectAttempts) {
                this.reconnectAttempts++;
                const delay = Math.min(1000 * Math.pow(2, this.reconnectAttempts), 30000);
                
                setTimeout(() => {
                    console.log(`Attempting to reconnect WebSocket (${this.reconnectAttempts}/${this.maxReconnectAttempts})`);
                    this.initWebSocket();
                }, delay);
            }
        },

        handleWebSocketMessage(message) {
            console.log('WebSocket message received:', message);

            switch (message.type) {
                case 'status_update':
                    console.log(`Status update for target ${message.targetId}: ${message.data.status}`);
                    this.updateTargetStatus(message.targetId, message.data.status);

                    // Dashboard tablolarını anlık güncelle
                    if (this.currentRoute === 'dashboard') {
                        this.updateDashboardOnStatusChange(message.targetId, message.data.status);
                    }
                    break;
                case 'alert_open':
                    this.showAlert(message);
                    this.alertCount++;
                    this.updateDashboardStats(); // Dashboard istatistiklerini güncelle
                    break;
                case 'alert_close':
                    this.hideAlert(message);
                    if (this.alertCount > 0) this.alertCount--;
                    this.updateDashboardStats(); // Dashboard istatistiklerini güncelle
                    break;
                case 'dashboard_stats':
                    console.log('Dashboard stats update received');
                    this.updateDashboardStatsFromData(message.data);

                    // Dashboard stats mesajı geldiğinde tabloları ve grafikleri güncelle
                    if (this.currentRoute === 'dashboard') {
                        console.log('Updating dashboard tables and charts due to stats update');
                        this.updateDashboardTargetTables();

                        // Debounced redraw to slow down rapid updates
                        this.scheduleStatusChartRedraw(message.data);
                        this.scheduleSystemHealthRedraw();
                    }
                    break;
                case 'server_stats':
                    // Sunucu sistem istatistiklerini güncelle (CPU, RAM, Disk)
                    this.serverStats = message.data;
                    window.dispatchEvent(new CustomEvent('server-stats', { detail: message.data }));
                    break;
                case 'device_metrics':
                    // Device metrics güncellemesini broadcast et
                    window.dispatchEvent(new CustomEvent('device-metrics', {
                        detail: message.data
                    }));
                    break;
                case 'sensor_data':
                    // ESP32 ortam sensörü verisini sensörler sayfasına ilet
                    window.dispatchEvent(new CustomEvent('sensor-data', {
                        detail: message.data
                    }));
                    break;
            }
        },

        updateTargetStatus(targetId, status) {
            // Update target status in the current view
            const statusElement = document.querySelector(`[data-target-id="${targetId}"] .status-indicator`);
            if (statusElement) {
                statusElement.className = `status-indicator ${status === 'online' ? 'text-green-500' : 'text-red-500'}`;
                statusElement.textContent = status === 'online' ? 'Online' : 'Offline';
            }

            // Update targets table row if exists (without reloading entire page)
            const tbody = document.getElementById('targets-table-body');
            if (tbody) {
                const rows = tbody.getElementsByTagName('tr');
                for (let row of rows) {
                    const checkbox = row.querySelector('.target-checkbox');
                    if (checkbox && parseInt(checkbox.value) === targetId) {
                        // Update status badge in this row
                        const statusCell = row.cells[4]; // Status is in column 4
                        if (statusCell) {
                            const isOnline = status === 'online';
                            const statusClass = isOnline ?
                                'bg-green-100 dark:bg-green-900/20 text-green-800 dark:text-green-400' :
                                'bg-red-100 dark:bg-red-900/20 text-red-800 dark:text-red-400';
                            const statusText = isOnline ? 'Online' : 'Offline';
                            statusCell.innerHTML = `<span class="px-2 py-1 text-xs font-medium rounded-full ${statusClass}">${statusText}</span>`;
                        }

                        // Apply current filters to the updated row
                        if (typeof checkRowMatchesFilters === 'function') {
                            const matches = checkRowMatchesFilters(row);
                            row.style.display = matches ? '' : 'none';
                        }

                        break;
                    }
                }
            }
        },

        // Dashboard sayfasında hedef durum değişikliğinde anlık güncelleme
        async updateDashboardOnStatusChange(targetId, newStatus) {
            console.log(`Updating dashboard for target ${targetId} -> ${newStatus}`);

            try {
                // Hedef bilgisini API'den al
                const response = await fetch(`/api/targets/${targetId}`, {
                    headers: {
                        'Authorization': `Bearer ${localStorage.getItem('token')}`
                    }
                });

                if (!response.ok) {
                    console.error('Failed to fetch target info');
                    // Fetch başarısız olursa tüm tabloları yenile
                    await this.updateDashboardTargetTables();
                    return;
                }

                const targetData = await response.json();

                // Mevcut elemandan hedefi kaldır (her iki tablodan da)
                this.removeTargetFromDashboardTables(targetId);

                // Yeni duruma göre hedefe ekle
                if (newStatus === 'online') {
                    this.addTargetToOnlineTable(targetData);
                } else {
                    this.addTargetToOfflineTable(targetData);
                }

                // İstatistikleri güncelle
                await this.updateDashboardStats();

            } catch (error) {
                console.error('Error updating dashboard:', error);
                // Hata durumunda tüm tabloları yenile
                await this.updateDashboardTargetTables();
            }
        },

        // Dashboard tablolarından hedefi kaldır
        removeTargetFromDashboardTables(targetId) {
            // Online tablosundan kaldır
            const onlineContainer = document.getElementById('online-targets-list');
            if (onlineContainer) {
                const onlineTarget = onlineContainer.querySelector(`[data-target-id="${targetId}"]`);
                if (onlineTarget) {
                    onlineTarget.remove();
                    console.log(` Removed target ${targetId} from online table`);
                }
            }

            // Offline tablosundan kaldır
            const offlineContainer = document.getElementById('offline-targets-list');
            if (offlineContainer) {
                const offlineTarget = offlineContainer.querySelector(`[data-target-id="${targetId}"]`);
                if (offlineTarget) {
                    offlineTarget.remove();
                    console.log(` Removed target ${targetId} from offline table`);
                }
            }
        },

        // Online tablosuna hedef ekle
        addTargetToOnlineTable(target) {
            const container = document.getElementById('online-targets-list');
            if (!container) return;

            // Eğer "Henüz hedef yok" mesajı varsa, kaldır
            const emptyMessage = container.querySelector('p');
            if (emptyMessage) {
                emptyMessage.remove();
            }

            // Eğer container'da space-y-3 div yoksa oluştur
            let listDiv = container.querySelector('.space-y-3');
            if (!listDiv) {
                listDiv = document.createElement('div');
                listDiv.className = 'space-y-3';
                container.appendChild(listDiv);
            }

            const monitoringType = target.monitoring_type || target.type || 'PING';
            const lastCheckTime = new Date().toLocaleString('tr-TR');

            const targetHTML = `
                <div class="flex items-center justify-between p-3 bg-gray-50 dark:bg-gray-700 rounded-lg hover:bg-gray-100 dark:hover:bg-gray-600 transition-colors" data-target-id="${target.id}">
                    <div class="flex items-center space-x-3">
                        <span class="h-3 w-3 rounded-full bg-green-400 shadow-lg shadow-green-500/40 animate-pulse"></span>
                        <div>
                            <p class="text-sm font-medium text-gray-900 dark:text-gray-100">${target.name}</p>
                            <p class="text-xs text-gray-500 dark:text-gray-400">${target.address}</p>
                            <p class="text-xs text-gray-400 dark:text-gray-500 mt-1">${monitoringType.toUpperCase()}</p>
                        </div>
                    </div>
                    <div class="text-right">
                        <span class="px-2 py-1 text-xs font-medium rounded-full bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-300">Online</span>
                        <p class="text-xs text-gray-500 dark:text-gray-400 mt-1">${lastCheckTime}</p>
                    </div>
                </div>
            `;

            // En üste ekle (prepend)
            listDiv.insertAdjacentHTML('afterbegin', targetHTML);
            console.log(` Added target ${target.id} (${target.name}) to online table`);

            // Animasyon efekti
            const newElement = listDiv.firstElementChild;
            newElement.style.opacity = '0';
            newElement.style.transform = 'translateY(-10px)';
            setTimeout(() => {
                newElement.style.transition = 'all 0.3s ease';
                newElement.style.opacity = '1';
                newElement.style.transform = 'translateY(0)';
            }, 10);
        },

        // Offline tablosuna hedef ekle
        addTargetToOfflineTable(target) {
            const container = document.getElementById('offline-targets-list');
            if (!container) return;

            // Eğer "Henüz hedef yok" mesajı varsa, kaldır
            const emptyMessage = container.querySelector('p');
            if (emptyMessage) {
                emptyMessage.remove();
            }

            // Eğer container'da space-y-3 div yoksa oluştur
            let listDiv = container.querySelector('.space-y-3');
            if (!listDiv) {
                listDiv = document.createElement('div');
                listDiv.className = 'space-y-3';
                container.appendChild(listDiv);
            }

            const monitoringType = target.monitoring_type || target.type || 'PING';
            const lastCheckTime = new Date().toLocaleString('tr-TR');

            const targetHTML = `
                <div class="flex items-center justify-between p-3 bg-gray-50 dark:bg-gray-700 rounded-lg hover:bg-gray-100 dark:hover:bg-gray-600 transition-colors" data-target-id="${target.id}">
                    <div class="flex items-center space-x-3">
                        <span class="h-3 w-3 rounded-full bg-red-400 shadow-lg shadow-red-500/40 animate-pulse"></span>
                        <div>
                            <p class="text-sm font-medium text-gray-900 dark:text-gray-100">${target.name}</p>
                            <p class="text-xs text-gray-500 dark:text-gray-400">${target.address}</p>
                            <p class="text-xs text-gray-400 dark:text-gray-500 mt-1">${monitoringType.toUpperCase()}</p>
                        </div>
                    </div>
                    <div class="text-right">
                        <span class="px-2 py-1 text-xs font-medium rounded-full bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-300">Offline</span>
                        <p class="text-xs text-gray-500 dark:text-gray-400 mt-1">${lastCheckTime}</p>
                    </div>
                </div>
            `;

            // En üste ekle (prepend)
            listDiv.insertAdjacentHTML('afterbegin', targetHTML);
            console.log(` Added target ${target.id} (${target.name}) to offline table`);

            // Animasyon efekti
            const newElement = listDiv.firstElementChild;
            newElement.style.opacity = '0';
            newElement.style.transform = 'translateY(-10px)';
            setTimeout(() => {
                newElement.style.transition = 'all 0.3s ease';
                newElement.style.opacity = '1';
                newElement.style.transform = 'translateY(0)';
            }, 10);
        },

        // Real-time dashboard güncellemeleri
        async updateDashboardStats() {
            if (this.currentRoute !== 'dashboard') return;

            try {
                // Dashboard istatistiklerini API'den al
                const response = await fetch('/api/dashboard/stats', {
                    headers: {
                        'Authorization': `Bearer ${localStorage.getItem('token')}`
                    }
                });

                if (response.ok) {
                    const stats = await response.json();
                    this.updateDashboardStatsFromData(stats);
                    // Grafik redraw'unu debounced şekilde planla
                    this.scheduleStatusChartRedraw(stats);
                }

                // Online ve Offline hedef tablolarını güncelle
                await this.updateDashboardTargetTables();
            } catch (error) {
                console.error('Failed to update dashboard stats:', error);
            }
        },

        updateDashboardStatsFromData(stats) {
            // İstatistik kartlarını güncelle
            this.updateStatCard('total-targets', stats.totalTargets || 0);
            this.updateStatCard('online-targets', stats.onlineTargets || 0);
            this.updateStatCard('offline-targets', stats.offlineTargets || 0);
            this.updateStatCard('open-alerts', stats.openAlerts || 0);
            this.updateStatCard('avg-uptime', `${(stats.avgUptime || 0).toFixed(1)}%`);
            this.updateStatCard('sla-breach', stats.slaBreachCount || 0);

            // Açık Uyarı tooltip detaylarını güncelle
            const ipDetail = document.querySelector('[data-alert-detail="ip"]');
            if (ipDetail) ipDetail.textContent = stats.ipAlertsCount || 0;
            const serviceDetail = document.querySelector('[data-alert-detail="service"]');
            if (serviceDetail) serviceDetail.textContent = stats.serviceAlertsCount || 0;
        },

        // Dashboard Online/Offline hedef tablolarını güncelle
        async updateDashboardTargetTables() {
            if (this.currentRoute !== 'dashboard') return;

            try {
                // Online hedefleri güncelle
                const onlineResponse = await fetch('/api/dashboard/online-targets', {
                    headers: {
                        'Authorization': `Bearer ${localStorage.getItem('token')}`
                    }
                });

                if (onlineResponse.ok) {
                    const onlineData = await onlineResponse.json();
                    this.updateOnlineTargetsTable(onlineData);
                }

                // Offline hedefleri güncelle
                const offlineResponse = await fetch('/api/dashboard/offline-targets', {
                    headers: {
                        'Authorization': `Bearer ${localStorage.getItem('token')}`
                    }
                });

                if (offlineResponse.ok) {
                    const offlineData = await offlineResponse.json();
                    this.updateOfflineTargetsTable(offlineData);
                }
            } catch (error) {
                console.error('Failed to update dashboard target tables:', error);
            }
        },

        // Online hedefler tablosunu güncelle
        updateOnlineTargetsTable(data) {
            const container = document.getElementById('online-targets-list');
            if (!container) return;

            if (!data.targets || data.targets.length === 0) {
                container.innerHTML = '<p class="text-gray-500 dark:text-gray-400 text-center py-4">Henüz online hedef bulunmuyor.</p>';
                return;
            }

            let html = '<div class="space-y-3">';
            data.targets.forEach(target => {
                const monitoringType = target.monitoring_type || target.type || 'PING';
                const lastCheckTime = target.last_check ? new Date(target.last_check).toLocaleString('tr-TR') : '';

                html += `
                    <div class="flex items-center justify-between p-3 bg-gray-50 dark:bg-gray-700 rounded-lg hover:bg-gray-100 dark:hover:bg-gray-600 transition-colors" data-target-id="${target.id}">
                        <div class="flex items-center space-x-3">
                            <span class="h-3 w-3 rounded-full bg-green-400 shadow-lg shadow-green-500/40 animate-pulse"></span>
                            <div>
                                <p class="text-sm font-medium text-gray-900 dark:text-gray-100">${target.name}</p>
                                <p class="text-xs text-gray-500 dark:text-gray-400">${target.address}</p>
                                <p class="text-xs text-gray-400 dark:text-gray-500 mt-1">${monitoringType.toUpperCase()}</p>
                            </div>
                        </div>
                        <div class="text-right">
                            <span class="px-2 py-1 text-xs font-medium rounded-full bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-300">Online</span>
                            <p class="text-xs text-gray-500 dark:text-gray-400 mt-1">${lastCheckTime}</p>
                        </div>
                    </div>
                `;
            });
            html += '</div>';

            container.innerHTML = html;

            // Toplam sayıyı güncelle
            this.updateTargetCountLabel('online', data.count || data.targets.length);
        },

        // Offline hedefler tablosunu güncelle
        updateOfflineTargetsTable(data) {
            const container = document.getElementById('offline-targets-list');
            if (!container) return;

            if (!data.targets || data.targets.length === 0) {
                container.innerHTML = '<p class="text-gray-500 dark:text-gray-400 text-center py-4">Henüz offline hedef bulunmuyor.</p>';

                // Toplam sayıyı güncelle (0 hedef)
                this.updateTargetCountLabel('offline', 0);
                return;
            }

            let html = '<div class="space-y-3">';
            data.targets.forEach(target => {
                const monitoringType = target.monitoring_type || target.type || 'PING';
                const lastCheckTime = target.last_check ? new Date(target.last_check).toLocaleString('tr-TR') : '';

                html += `
                    <div class="flex items-center justify-between p-3 bg-gray-50 dark:bg-gray-700 rounded-lg hover:bg-gray-100 dark:hover:bg-gray-600 transition-colors" data-target-id="${target.id}">
                        <div class="flex items-center space-x-3">
                            <span class="h-3 w-3 rounded-full bg-red-400 shadow-lg shadow-red-500/40 animate-pulse"></span>
                            <div>
                                <p class="text-sm font-medium text-gray-900 dark:text-gray-100">${target.name}</p>
                                <p class="text-xs text-gray-500 dark:text-gray-400">${target.address}</p>
                                <p class="text-xs text-gray-400 dark:text-gray-500 mt-1">${monitoringType.toUpperCase()}</p>
                            </div>
                        </div>
                        <div class="text-right">
                            <span class="px-2 py-1 text-xs font-medium rounded-full bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-300">Offline</span>
                            <p class="text-xs text-gray-500 dark:text-gray-400 mt-1">${lastCheckTime}</p>
                        </div>
                    </div>
                `;
            });
            html += '</div>';

            container.innerHTML = html;

            // Toplam sayıyı güncelle
            this.updateTargetCountLabel('offline', data.count || data.targets.length);
        },

        // Hedef toplam sayı label'ını güncelle
        updateTargetCountLabel(type, count) {
            const containerId = type === 'online' ? 'online-targets-container' : 'offline-targets-container';
            const container = document.getElementById(containerId);
            if (!container) return;

            // Header kısmındaki "Toplam: X hedef" yazısını bul ve güncelle
            const header = container.closest('.bg-white, .rounded-lg');
            if (header) {
                const countElement = header.querySelector('.text-xs.text-gray-500');
                if (countElement) {
                    const oldText = countElement.textContent;
                    countElement.textContent = `Toplam: ${count} hedef`;

                    // Sayı değiştiyse animasyon ekle
                    if (oldText !== countElement.textContent) {
                        countElement.style.transition = 'all 0.3s ease';
                        countElement.style.color = type === 'online' ? '#10b981' : '#ef4444';
                        setTimeout(() => {
                            countElement.style.color = '';
                        }, 500);
                    }

                    console.log(` Updated ${type} target count: ${count}`);
                }
            }
        },

        updateStatCard(cardId, value) {
            const cardElement = document.querySelector(`[data-stat-card="${cardId}"] .stat-value`);
            if (cardElement) {
                // Smooth transition efekti
                cardElement.style.transition = 'all 0.3s ease';
                cardElement.style.transform = 'scale(1.1)';
                cardElement.textContent = value;
                
                setTimeout(() => {
                    cardElement.style.transform = 'scale(1)';
                }, 300);
            }
        },
        showAlert(message) {
            Toast.show({
                type: 'warning',
                title: 'Yeni Uyarı',
                message: message.data.message || 'Bir hedef offline duruma geçti',
                timeout: 0
            });
        },

        hideAlert(message) {
            // Handle alert close if needed
        },

        // Search
        performSearch() {
            if (this.searchQuery.length < 2) return;
            
            // Implement search logic based on current route
            const searchEvent = new CustomEvent('search', {
                detail: { query: this.searchQuery }
            });
            document.dispatchEvent(searchEvent);
        },

        // Target Modal Functions
        toggleHTTPFields() {
            const monitoringType = document.getElementById('target-monitoring-type').value;
            const httpFields = document.getElementById('http-fields');
            
            if (monitoringType === 'http' || monitoringType === 'https') {
                httpFields.style.display = 'block';
            } else {
                httpFields.style.display = 'none';
            }
        },

        sortMetrics() {
            const priority = { success: 0, timeout: 1, snmp_error: 2, unreachable: 2 };
            this.metrics.sort((a, b) => {
                const pa = priority[a.status] ?? 9;
                const pb = priority[b.status] ?? 9;
                if (pa === pb) {
                    const nameA = (a.target_name || '').toLowerCase();
                    const nameB = (b.target_name || '').toLowerCase();
                    return nameA.localeCompare(nameB, 'tr');
                }
                return pa - pb;
            });
        },

        toggleMetricsFields(forceState = null) {
            const toggle = document.getElementById('target-metrics-enabled');
            const fields = document.getElementById('metrics-fields');
            if (!toggle || !fields) {
                return;
            }

            if (forceState !== null) {
                toggle.checked = forceState;
            }

            const isEnabled = toggle.checked;
            const inputs = fields.querySelectorAll('input, select');
            inputs.forEach(input => {
                input.disabled = !isEnabled;
            });

            fields.classList.toggle('expanded', isEnabled);
            fields.classList.toggle('collapsed', !isEnabled);
        },

        openTargetModal(targetId = null) {
            const url = '/static/admin/partials/target_form.html';
            fetch(url, {
                headers: {
                    'Authorization': `Bearer ${localStorage.getItem('token')}`,
                    'Content-Type': 'text/html'
                }
            })
            .then(response => response.text())
            .then(html => {
                document.body.insertAdjacentHTML('beforeend', html);
                this.setupTargetForm();
                if (!targetId) {
                    const metricsToggle = document.getElementById('target-metrics-enabled');
                    if (metricsToggle) {
                        metricsToggle.checked = true;
                    }
                    this.toggleMetricsFields(true);
                }
                
                const titleEl = document.getElementById('target-modal-title');
                if (titleEl) {
                    titleEl.textContent = targetId ? 'Hedef Düzenle' : 'Yeni Hedef Ekle';
                }
                
                // Eğer düzenleme modundaysa, mevcut verileri yükle
                if (targetId) {
                    this.loadTargetData(targetId);
                }
            })
            .catch(error => {
                console.error('Modal load error:', error);
                Toast.show({
                    type: 'error',
                    title: 'Hata',
                    message: 'Modal yüklenemedi.',
                    timeout: 5000
                });
            });
        },

        closeTargetModal() {
            const modal = document.getElementById('target-modal');
            if (modal) {
                modal.remove();
            }
        },

        setupTargetForm() {
            const form = document.getElementById('target-form');
            if (form) {
                form.addEventListener('submit', (e) => {
                    e.preventDefault();
                    this.submitTargetForm();
                });
            }

            const metricsToggle = document.getElementById('target-metrics-enabled');
            if (metricsToggle) {
                metricsToggle.addEventListener('change', () => this.toggleMetricsFields());
            }

            // Ensure stateful sections reflect defaults on open
            this.toggleHTTPFields();
            this.toggleMetricsFields();
        },

        mapTargetType(frontendType) {
            const typeMap = {
                'ping': 'icmp',
                'http': 'http',
                'https': 'https', // HTTPS'i ayrı tut
                'tcp': 'tcp'
            };
            return typeMap[frontendType] || 'icmp';
        },

        async submitTargetForm() {
            const formData = {
                name: document.getElementById('target-name').value,
                address: document.getElementById('target-address').value,
                type: this.mapTargetType(document.getElementById('target-monitoring-type').value),
                monitoring_type: document.getElementById('target-monitoring-type').value,
                interval_sec: 120, // Sabit 2 dakika - kullanıcı tarafından değiştirilemez
                timeout_ms: 30000, // Sabit 30 saniye
                enabled: true,
                tags: document.getElementById('target-tags').value || null
            };

            const metricsToggle = document.getElementById('target-metrics-enabled');
            const snmpCommunityInput = document.getElementById('target-snmp-community');
            const snmpVersionSelect = document.getElementById('target-snmp-version');

            formData.metrics_enabled = metricsToggle ? metricsToggle.checked : false;
            formData.snmp_community = (snmpCommunityInput?.value || 'public').trim() || 'public';
            formData.snmp_version = snmpVersionSelect?.value || 'v2c';

            const duplicateWarning = document.getElementById('target-duplicate-warning');
            if (duplicateWarning) {
                duplicateWarning.classList.add('hidden');
            }

            // Add HTTP-specific fields if monitoring type is HTTP/HTTPS
            const monitoringType = document.getElementById('target-monitoring-type').value;
            if (monitoringType === 'http' || monitoringType === 'https') {
                formData.http_method = document.getElementById('target-http-method').value;
                formData.http_path = document.getElementById('target-http-path').value;
                formData.expected_status_code = parseInt(document.getElementById('target-expected-status').value);
                formData.timeout_sec = 30; // Sabit 30 saniye timeout
                formData.ssl_check = document.getElementById('target-ssl-check').checked;
                formData.follow_redirects = document.getElementById('target-follow-redirects').checked;
                
                const expectedContent = document.getElementById('target-expected-content').value;
                if (expectedContent) {
                    formData.expected_content = expectedContent;
                }
                
                const httpHeaders = document.getElementById('target-http-headers').value;
                if (httpHeaders) {
                    formData.http_headers = httpHeaders;
                }
            }

            const targetIdInput = document.getElementById('target-id');
            const targetId = targetIdInput ? targetIdInput.value : '';
            const url = targetId ? `/api/targets/${targetId}` : '/api/targets';
            const method = targetId ? 'PUT' : 'POST';

            try {
                const response = await fetch(url, {
                    method: method,
                    headers: {
                        'Authorization': `Bearer ${localStorage.getItem('token')}`,
                        'Content-Type': 'application/json'
                    },
                    body: JSON.stringify(formData)
                });

                const data = await response.json().catch(() => ({}));
                if (!response.ok || data.error) {
                    const rawErrorKey = data.code || data.error;
                    const translatedMessage = translateError(rawErrorKey || 'Hedef kaydedilemedi.');
                    const error = new Error(translatedMessage);

                    if (rawErrorKey && !rawErrorKey.includes(' ') && rawErrorKey.length <= 50) {
                        error.code = rawErrorKey;
                    }

                    throw error;
                }
                
                Toast.show({
                    type: 'success',
                    title: 'Başarılı',
                    message: targetId ? 'Hedef güncellendi!' : 'Hedef eklendi!',
                    timeout: 3000
                });
                
                this.closeTargetModal();
                this.loadPageContent('targets'); // Refresh targets list
            } catch (error) {
                console.error('Target save error:', error);

                if (duplicateWarning && (error.code === 'duplicate_target' || (error.message && error.message.includes('hedef zaten')))) {
                    duplicateWarning.textContent = error.message || 'Bu hedeften zaten var';
                    duplicateWarning.classList.remove('hidden');
                }

                Toast.show({
                    type: 'error',
                    title: 'Hata',
                    message: error.message || 'Hedef kaydedilemedi.',
                    timeout: 5000
                });
            }
        },

                loadTargetData(targetId) {
            fetch(`/api/targets/${targetId}`, {
                headers: {
                    'Authorization': `Bearer ${localStorage.getItem('token')}`
                }
            })
            .then(response => response.json())
            .then(data => {
                if (data.error) {
                    throw new Error(data.error);
                }
                
                // Modal'ın DOM'a eklenmesini bekle
                setTimeout(() => {
                    // CRITICAL: Set target ID for edit mode
                    const targetIdField = document.getElementById('target-id');
                    if (targetIdField) targetIdField.value = targetId;

                    // Form alanlarını doldur
                    const nameField = document.getElementById('target-name');
                    const addressField = document.getElementById('target-address');
                    const monitoringTypeField = document.getElementById('target-monitoring-type');
                    const tagsField = document.getElementById('target-tags');

                    if (nameField) nameField.value = data.name || '';
                    if (addressField) addressField.value = data.address || '';
                    if (tagsField) tagsField.value = data.tags || '';
                    const metricsToggle = document.getElementById('target-metrics-enabled');
                    const snmpCommunityField = document.getElementById('target-snmp-community');
                    const snmpVersionField = document.getElementById('target-snmp-version');
                    if (metricsToggle) metricsToggle.checked = !!data.metrics_enabled;
                    if (snmpCommunityField) snmpCommunityField.value = data.snmp_community || 'public';
                    if (snmpVersionField) snmpVersionField.value = data.snmp_version || 'v2c';
                    
                    // Monitoring type mapping
                    const frontendType = this.mapBackendTypeToFrontend(data.type);
                    if (monitoringTypeField) {
                        monitoringTypeField.value = frontendType;
                        // HTTP alanlarını göster/gizle
                        this.toggleHTTPFields();
                    }
                    
                    // HTTP alanlarını doldur
                    if (data.monitoring_type === 'http' || data.monitoring_type === 'https') {
                        const httpMethodField = document.getElementById('target-http-method');
                        const httpPathField = document.getElementById('target-http-path');
                        const expectedStatusField = document.getElementById('target-expected-status');
                        const timeoutSecField = document.getElementById('target-timeout-sec');
                        const expectedContentField = document.getElementById('target-expected-content');
                        const httpHeadersField = document.getElementById('target-http-headers');
                        const sslCheckField = document.getElementById('target-ssl-check');
                        const followRedirectsField = document.getElementById('target-follow-redirects');
                        
                        if (httpMethodField) httpMethodField.value = data.http_method || 'GET';
                        if (httpPathField) httpPathField.value = data.http_path || '/';
                        if (expectedStatusField) expectedStatusField.value = data.expected_status_code || 200;
                        if (timeoutSecField) timeoutSecField.value = data.timeout_sec || 10;
                        if (expectedContentField) expectedContentField.value = data.expected_content || '';
                        if (httpHeadersField) httpHeadersField.value = data.http_headers || '';
                        if (sslCheckField) sslCheckField.checked = data.ssl_check || false;
                        if (followRedirectsField) followRedirectsField.checked = data.follow_redirects !== false;
                    }
                    
                    // Genel alanlar
                    const intervalField = document.getElementById('target-interval');
                    const timeoutField = document.getElementById('target-timeout');
                    
                    if (intervalField) intervalField.value = data.interval_sec || 30;
                    if (timeoutField) timeoutField.value = data.timeout_ms || 1000;
                    this.toggleMetricsFields();
                }, 100); // 100ms bekle
            })
            .catch(error => {
                console.error('Target data load error:', error);
                Toast.show({
                    type: 'error',
                    title: 'Hata',
                    message: 'Hedef verileri yüklenemedi.',
                    timeout: 5000
                });
            });
        },

        mapBackendTypeToFrontend(backendType) {
            const typeMap = {
                'icmp': 'ping',
                'http': 'http',
                'https': 'https',
                'tcp': 'tcp'
            };
            return typeMap[backendType] || 'ping';
        },

        editTarget(targetId) {
            this.openTargetModal(targetId);
        },

        deleteTarget(targetId) {
            if (confirm('Bu hedefi silmek istediğinizden emin misiniz?')) {
                fetch(`/api/targets/${targetId}`, {
                    method: 'DELETE',
                    headers: {
                        'Authorization': `Bearer ${localStorage.getItem('token')}`
                    }
                })
                .then(response => response.json())
                .then(data => {
                    if (data.error) {
                        throw new Error(data.error);
                    }
                    
                    Toast.show({
                        type: 'success',
                        title: 'Başarılı',
                        message: 'Hedef silindi!',
                        timeout: 3000
                    });
                    
                    this.loadPageContent('targets'); // Refresh targets list
                })
                .catch(error => {
                    console.error('Target delete error:', error);
                    Toast.show({
                        type: 'error',
                        title: 'Hata',
                        message: error.message || 'Hedef silinemedi.',
                        timeout: 5000
                    });
                });
            }
        },

        pingTarget(targetId) {
            fetch(`/api/targets/${targetId}/ping-now`, {
                method: 'POST',
                headers: {
                    'Authorization': `Bearer ${localStorage.getItem('token')}`
                }
            })
            .then(response => response.json())
            .then(data => {
                if (data.error) {
                    throw new Error(data.error);
                }
                
                Toast.show({
                    type: 'success',
                    title: 'Başarılı',
                    message: `Ping başlatıldı: ${data.target}`,
                    timeout: 3000
                });
                
                // Birkaç saniye sonra listeyi yenile
                setTimeout(() => {
                    this.loadPageContent('targets');
                }, 2000);
            })
            .catch(error => {
                console.error('Ping error:', error);
                Toast.show({
                    type: 'error',
                    title: 'Hata',
                    message: error.message || 'Ping başlatılamadı.',
                    timeout: 5000
                });
            });
        },

        showTargetHistory(targetId) {
            // Store target ID globally for period changes
            window.currentTargetId = targetId;
            this.currentTargetId = targetId;
            
            console.log('Opening history for target:', targetId);
            
            // Show loading
            this.showLoading();
            
            // Fetch target history data
            fetch(`/api/target-history/${targetId}?period=1d`, {
                headers: {
                    'Authorization': `Bearer ${localStorage.getItem('token')}`,
                    'Content-Type': 'application/json'
                }
            })
            .then(response => {
                console.log('Target history response status:', response.status);
                console.log('Target history response headers:', response.headers.get('content-type'));
                
                if (!response.ok) {
                    throw new Error(`HTTP ${response.status}: ${response.statusText}`);
                }
                
                return response.json();
            })
            .then(data => {
                console.log('Target history data:', data);
                
                if (data.error) {
                    throw new Error(data.error);
                }
                
                this.renderTargetHistoryModal(data);
            })
            .catch(error => {
                console.error('Target history error:', error);
                Toast.show({
                    type: 'error',
                    title: 'Hata',
                    message: 'Hedef geçmişi yüklenemedi: ' + error.message,
                    timeout: 5000
                });
            })
            .finally(() => {
                this.hideLoading();
            });
        },

        renderTargetHistoryModal(data) {
            // Target monitoring type'ını kontrol et
            const isHTTPMonitoring = data.data.some(ping => ping.status_code !== undefined);
            const historyTitle = isHTTPMonitoring ? 'HTTP Geçmişi' : 'Ping Geçmişi';
            const chartTitle = isHTTPMonitoring ? 'HTTP Yanıt Süreleri' : 'Ping Yanıt Süreleri';
            const recordsTitle = isHTTPMonitoring ? 'Son HTTP Kayıtları' : 'Son Ping Kayıtları';
            
            const modal = document.createElement('div');
            modal.id = 'target-history-modal';
            modal.className = 'fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50 p-4';
            modal.innerHTML = `
                <div class="bg-white dark:bg-gray-800 rounded-xl shadow-2xl w-full max-w-7xl h-[95vh] flex flex-col">
                    <!-- Header -->
                    <div class="flex items-center justify-between p-6 border-b border-gray-200 dark:border-gray-700 flex-shrink-0">
                        <div>
                            <h2 class="text-2xl font-bold text-gray-900 dark:text-gray-100">${data.target_name}</h2>
                            <p class="text-gray-600 dark:text-gray-400">${data.target_addr} - ${historyTitle}</p>
                        </div>
                        <button onclick="closeTargetHistory()" class="text-gray-400 hover:text-gray-600 dark:hover:text-gray-300 p-2 rounded-lg hover:bg-gray-100 dark:hover:bg-gray-700">
                            <svg class="w-6 h-6" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"></path>
                            </svg>
                        </button>
                    </div>
                    
                    <!-- Content -->
                    <div class="flex-1 overflow-y-auto flex flex-col">
                        <!-- Period Filter -->
                        <div class="p-6 pb-4 flex-shrink-0">
                            <div class="flex space-x-3">
                                <button onclick="changeHistoryPeriod('1h')" class="px-6 py-3 text-sm font-medium rounded-lg transition-colors ${data.period === '1h' ? 'bg-blue-600 text-white shadow-lg' : 'bg-gray-200 dark:bg-gray-700 text-gray-700 dark:text-gray-300 hover:bg-gray-300 dark:hover:bg-gray-600'}">1 Saat</button>
                                <button onclick="changeHistoryPeriod('1d')" class="px-6 py-3 text-sm font-medium rounded-lg transition-colors ${data.period === '1d' ? 'bg-blue-600 text-white shadow-lg' : 'bg-gray-200 dark:bg-gray-700 text-gray-700 dark:text-gray-300 hover:bg-gray-300 dark:hover:bg-gray-600'}">1 Gün</button>
                                <button onclick="changeHistoryPeriod('7d')" class="px-6 py-3 text-sm font-medium rounded-lg transition-colors ${data.period === '7d' ? 'bg-blue-600 text-white shadow-lg' : 'bg-gray-200 dark:bg-gray-700 text-gray-700 dark:text-gray-300 hover:bg-gray-300 dark:hover:bg-gray-600'}">7 Gün</button>
                                <button onclick="changeHistoryPeriod('30d')" class="px-6 py-3 text-sm font-medium rounded-lg transition-colors ${data.period === '30d' ? 'bg-blue-600 text-white shadow-lg' : 'bg-gray-200 dark:bg-gray-700 text-gray-700 dark:text-gray-300 hover:bg-gray-300 dark:hover:bg-gray-600'}">30 Gün</button>
                            </div>
                        </div>
                        
                        <!-- Statistics Cards -->
                        <div class="px-6 pb-4 flex-shrink-0">
                            <div class="grid grid-cols-2 md:grid-cols-6 gap-4">
                                <div class="bg-gradient-to-br from-blue-50 to-blue-100 dark:from-blue-900/20 dark:to-blue-800/20 rounded-lg p-4 border border-blue-200 dark:border-blue-800">
                                    <div class="text-sm text-blue-600 dark:text-blue-400 font-medium">Toplam</div>
                                    <div class="text-2xl font-bold text-blue-900 dark:text-blue-100">${data.stats.total_pings}</div>
                                </div>
                                <div class="bg-gradient-to-br from-green-50 to-green-100 dark:from-green-900/20 dark:to-green-800/20 rounded-lg p-4 border border-green-200 dark:border-green-800">
                                    <div class="text-sm text-green-600 dark:text-green-400 font-medium">Başarılı</div>
                                    <div class="text-2xl font-bold text-green-900 dark:text-green-100">${data.stats.success_pings}</div>
                                </div>
                                <div class="bg-gradient-to-br from-red-50 to-red-100 dark:from-red-900/20 dark:to-red-800/20 rounded-lg p-4 border border-red-200 dark:border-red-800">
                                    <div class="text-sm text-red-600 dark:text-red-400 font-medium">Başarısız</div>
                                    <div class="text-2xl font-bold text-red-900 dark:text-red-100">${data.stats.failed_pings}</div>
                                </div>
                                <div class="bg-gradient-to-br from-purple-50 to-purple-100 dark:from-purple-900/20 dark:to-purple-800/20 rounded-lg p-4 border border-purple-200 dark:border-purple-800">
                                    <div class="text-sm text-purple-600 dark:text-purple-400 font-medium">Uptime</div>
                                    <div class="text-2xl font-bold text-purple-900 dark:text-purple-100">${data.stats.uptime_percent.toFixed(1)}%</div>
                                </div>
                                <div class="bg-gradient-to-br from-orange-50 to-orange-100 dark:from-orange-900/20 dark:to-orange-800/20 rounded-lg p-4 border border-orange-200 dark:border-orange-800">
                                    <div class="text-sm text-orange-600 dark:text-orange-400 font-medium">Ortalama</div>
                                    <div class="text-xl font-bold text-orange-900 dark:text-orange-100">${data.stats.avg_duration.toFixed(1)}ms</div>
                                </div>
                                <div class="bg-gradient-to-br from-indigo-50 to-indigo-100 dark:from-indigo-900/20 dark:to-indigo-800/20 rounded-lg p-4 border border-indigo-200 dark:border-indigo-800">
                                    <div class="text-sm text-indigo-600 dark:text-indigo-400 font-medium">Min/Max</div>
                                    <div class="text-lg font-bold text-indigo-900 dark:text-indigo-100">${data.stats.min_duration.toFixed(0)}/${data.stats.max_duration.toFixed(0)}ms</div>
                                </div>
                            </div>
                            
                            <!-- SSL Expiry Card - Only for HTTPS targets -->
                            ${isHTTPMonitoring && data.data.some(ping => ping.ssl_expiry_date && ping.ssl_expiry_date !== '-') ? `
                            <div class="mt-4">
                                <div class="bg-gradient-to-br from-teal-50 to-teal-100 dark:from-teal-900/20 dark:to-teal-800/20 rounded-lg p-4 border border-teal-200 dark:border-teal-800">
                                    <div class="flex items-center justify-between">
                                        <div>
                                            <div class="text-sm text-teal-600 dark:text-teal-400 font-medium flex items-center">
                                                <svg class="w-4 h-4 mr-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 15v2m-6 4h12a2 2 0 002-2v-6a2 2 0 00-2-2H6a2 2 0 00-2 2v6a2 2 0 002 2zm10-10V7a4 4 0 00-8 0v4h8z"></path>
                                                </svg>
                                                SSL Sertifika Geçerlilik
                                            </div>
                                            <div class="text-lg font-bold text-teal-900 dark:text-teal-100 mt-1">
                                                ${this.getLatestSSLExpiry(data.data)}
                                            </div>
                                        </div>
                                        <div class="text-right">
                                            <div class="text-sm text-teal-600 dark:text-teal-400 font-medium">Kalan Süre</div>
                                            <div class="text-lg font-bold text-teal-900 dark:text-teal-100">
                                                ${this.getSSLExpiryDays(data.data)}
                                            </div>
                                        </div>
                                    </div>
                                </div>
                            </div>
                            ` : ''}
                        </div>
                        
                        <!-- Chart Container - Above Records -->
                        <div class="p-6 pt-0 flex-shrink-0">
                            <div class="bg-gray-50 dark:bg-gray-700 rounded-xl p-4 border border-gray-200 dark:border-gray-600">
                                <h3 class="text-lg font-semibold text-gray-900 dark:text-gray-100 mb-3 flex items-center">
                                    <svg class="w-5 h-5 mr-2 text-blue-600" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 19v-6a2 2 0 00-2-2H5a2 2 0 00-2 2v6a2 2 0 002 2h2a2 2 0 002-2zm0 0V9a2 2 0 012-2h2a2 2 0 012 2v10m-6 0a2 2 0 002 2h2a2 2 0 002-2m0 0V5a2 2 0 012-2h2a2 2 0 012 2v14a2 2 0 01-2 2h-2a2 2 0 01-2-2z"></path>
                                    </svg>
                                    ${chartTitle}
                                </h3>
                                <div class="relative" style="height: 300px;">
                                    <canvas id="targetHistoryChart"></canvas>
                                </div>
                            </div>
                        </div>
                        
                        <!-- Recent Pings Table - Scrollable Below Chart -->
                        <div class="p-6 pt-0 flex-shrink-0">
                            <div class="bg-gray-50 dark:bg-gray-700 rounded-xl p-4 border border-gray-200 dark:border-gray-600">
                                <h3 class="text-lg font-semibold text-gray-900 dark:text-gray-100 mb-3 flex items-center">
                                    <svg class="w-5 h-5 mr-2 text-green-600" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z"></path>
                                    </svg>
                                    ${recordsTitle}
                                </h3>
                                <div class="overflow-y-auto" style="height: 400px;">
                                    <table class="min-w-full">
                                        <thead class="sticky top-0 bg-gray-50 dark:bg-gray-700 z-10">
                                            <tr class="border-b border-gray-200 dark:border-gray-600">
                                                <th class="text-left py-2 px-3 text-sm font-medium text-gray-600 dark:text-gray-400">Zaman</th>
                                                <th class="text-left py-2 px-3 text-sm font-medium text-gray-600 dark:text-gray-400">Durum</th>
                                                <th class="text-left py-2 px-3 text-sm font-medium text-gray-600 dark:text-gray-400">Süre</th>
                                                ${isHTTPMonitoring ? '<th class="text-left py-2 px-3 text-sm font-medium text-gray-600 dark:text-gray-400">Status Code</th>' : ''}
                                                ${isHTTPMonitoring ? '<th class="text-left py-2 px-3 text-sm font-medium text-gray-600 dark:text-gray-400">SSL Expiry</th>' : ''}
                                                <th class="text-left py-2 px-3 text-sm font-medium text-gray-600 dark:text-gray-400">Hata</th>
                                            </tr>
                                        </thead>
                                        <tbody class="bg-white dark:bg-gray-800">
                                            ${data.data.slice(-30).reverse().map(ping => `
                                                <tr class="border-b border-gray-200 dark:border-gray-600 hover:bg-gray-100 dark:hover:bg-gray-600">
                                                    <td class="py-2 px-3 text-sm text-gray-900 dark:text-gray-100">${formatTurkishTime(ping.timestamp)}</td>
                                                    <td class="py-2 px-3">
                                                        <span class="px-2 py-1 text-xs font-medium rounded-full ${ping.success ? 'bg-green-100 dark:bg-green-900/20 text-green-800 dark:text-green-400' : 'bg-red-100 dark:bg-red-900/20 text-red-800 dark:text-red-400'}">
                                                            ${ping.success ? '✅' : '❌'}
                                                        </span>
                                                    </td>
                                                    <td class="py-2 px-3 text-sm text-gray-900 dark:text-gray-100 font-medium">${ping.duration_ms.toFixed(1)}ms</td>
                                                    ${isHTTPMonitoring ? `<td class="py-2 px-3 text-sm text-gray-900 dark:text-gray-100">${ping.status_code || '-'}</td>` : ''}
                                                    ${isHTTPMonitoring ? `<td class="py-2 px-3 text-sm text-gray-900 dark:text-gray-100">${ping.ssl_expiry_date || '-'}</td>` : ''}
                                                    <td class="py-2 px-3 text-sm text-gray-500 dark:text-gray-400">${ping.error_msg || '-'}</td>
                                                </tr>
                                            `).join('')}
                                        </tbody>
                                    </table>
                                </div>
                            </div>
                        </div>
                    </div>
                </div>
            `;
            
            document.body.appendChild(modal);
            
            // Initialize chart
            setTimeout(() => {
                this.initTargetHistoryChart(data);
            }, 100);
        },

        initTargetHistoryChart(data) {
            const ctx = document.getElementById('targetHistoryChart');
            if (!ctx) return;
            
            // Destroy existing chart if it exists
            if (this.targetHistoryChart) {
                this.targetHistoryChart.destroy();
            }
            
            // Prepare data with better time formatting
            const labels = data.data.map(item => {
                const date = new Date(item.timestamp);
                return date.toLocaleString('tr-TR', { 
                    month: 'short', 
                    day: '2-digit', 
                    hour: '2-digit', 
                    minute: '2-digit',
                    timeZone: 'Europe/Istanbul'
                });
            });
            
            const successData = data.data.map(item => item.success ? item.duration_ms : null);
            const failureData = data.data.map(item => !item.success ? item.duration_ms : null);
            
            this.targetHistoryChart = new Chart(ctx, {
                type: 'line',
                data: {
                    labels: labels,
                    datasets: [
                        {
                            label: 'Başarılı',
                            data: successData,
                            borderColor: '#22c55e',
                            backgroundColor: 'rgba(34, 197, 94, 0.2)',
                            borderWidth: 2,
                            fill: true,
                            tension: 0.1,
                            pointRadius: 2,
                            pointHoverRadius: 4,
                            pointBackgroundColor: '#22c55e',
                            pointBorderColor: '#22c55e',
                            spanGaps: false
                        },
                        {
                            label: 'Başarısız',
                            data: failureData,
                            borderColor: '#ef4444',
                            backgroundColor: 'rgba(239, 68, 68, 0.2)',
                            borderWidth: 2,
                            fill: true,
                            tension: 0.1,
                            pointRadius: 2,
                            pointHoverRadius: 4,
                            pointBackgroundColor: '#ef4444',
                            pointBorderColor: '#ef4444',
                            spanGaps: false
                        }
                    ]
                },
                options: {
                    responsive: true,
                    maintainAspectRatio: false,
                    interaction: {
                        intersect: false,
                        mode: 'index'
                    },
                    plugins: {
                        legend: {
                            display: true,
                            position: 'top',
                            labels: {
                                color: this.isDark ? '#e5e7eb' : '#374151',
                                usePointStyle: true,
                                padding: 15,
                                font: {
                                    size: 11,
                                    weight: '500'
                                }
                            }
                        },
                        tooltip: {
                            backgroundColor: this.isDark ? '#1f2937' : '#ffffff',
                            titleColor: this.isDark ? '#f9fafb' : '#111827',
                            bodyColor: this.isDark ? '#d1d5db' : '#374151',
                            borderColor: this.isDark ? '#374151' : '#e5e7eb',
                            borderWidth: 1,
                            cornerRadius: 6,
                            displayColors: true,
                            callbacks: {
                                title: function(context) {
                                    const index = context[0].dataIndex;
                                    const timestamp = data.data[index].timestamp;
                                    return formatTurkishTime(timestamp);
                                },
                                label: function(context) {
                                    const index = context.dataIndex;
                                    const item = data.data[index];
                                    if (item.success) {
                                        return `Ping: ${item.duration_ms.toFixed(1)}ms`;
                                    } else {
                                        return `Hata: ${item.error_msg || 'Bağlantı başarısız'}`;
                                    }
                                }
                            }
                        }
                    },
                    scales: {
                        x: {
                            display: true,
                            title: {
                                display: true,
                                text: 'Zaman',
                                color: this.isDark ? '#e5e7eb' : '#374151',
                                font: {
                                    size: 11,
                                    weight: '600'
                                }
                            },
                            ticks: {
                                color: this.isDark ? '#9ca3af' : '#6b7280',
                                maxTicksLimit: 10,
                                font: {
                                    size: 10,
                                    weight: '500'
                                }
                            },
                            grid: {
                                color: this.isDark ? '#374151' : '#e5e7eb',
                                drawBorder: false,
                                lineWidth: 1
                            }
                        },
                        y: {
                            display: true,
                            title: {
                                display: true,
                                text: 'Ping Süresi (ms)',
                                color: this.isDark ? '#e5e7eb' : '#374151',
                                font: {
                                    size: 11,
                                    weight: '600'
                                }
                            },
                            beginAtZero: true,
                            ticks: {
                                color: this.isDark ? '#9ca3af' : '#6b7280',
                                font: {
                                    size: 10,
                                    weight: '500'
                                },
                                callback: function(value) {
                                    return value + 'ms';
                                }
                            },
                            grid: {
                                color: this.isDark ? '#374151' : '#e5e7eb',
                                drawBorder: false,
                                lineWidth: 1
                            }
                        }
                    }
                }
            });
        },

        changeHistoryPeriod(period) {
            // Use global target ID
            const targetId = window.currentTargetId || this.currentTargetId;
            console.log('Changing period to:', period, 'for target:', targetId);
            
            if (!targetId) {
                console.error('No current target ID found');
                Toast.show({
                    type: 'error',
                    title: 'Hata',
                    message: 'Hedef ID bulunamadı',
                    timeout: 3000
                });
                return;
            }
            
            this.showLoading();
            
            fetch(`/api/target-history/${targetId}?period=${period}`, {
                headers: {
                    'Authorization': `Bearer ${localStorage.getItem('token')}`,
                    'Content-Type': 'application/json'
                }
            })
            .then(response => {
                console.log('Period change response status:', response.status);
                
                if (!response.ok) {
                    throw new Error(`HTTP ${response.status}: ${response.statusText}`);
                }
                
                return response.json();
            })
            .then(data => {
                console.log('Period change data:', data);
                
                if (data.error) {
                    throw new Error(data.error);
                }
                
                // Update modal content
                document.getElementById('target-history-modal').remove();
                this.renderTargetHistoryModal(data);
            })
            .catch(error => {
                console.error('Period change error:', error);
                Toast.show({
                    type: 'error',
                    title: 'Hata',
                    message: 'Veri yüklenemedi: ' + error.message,
                    timeout: 5000
                });
            })
            .finally(() => {
                this.hideLoading();
            });
        },

        closeTargetHistory() {
            const modal = document.getElementById('target-history-modal');
            if (modal) {
                modal.remove();
            }
            this.currentTargetId = null;
            window.currentTargetId = null;
        },

        // HTMX Event Listeners
        setupHTMXListeners() {
            // Global HTMX event listeners
            document.body.addEventListener('htmx:configRequest', (event) => {
                // JWT token'ı header'a ekle
                const token = localStorage.getItem('token');
                console.log('HTMX Request - Token:', token ? 'Present' : 'Missing');
                if (token) {
                    event.detail.headers['Authorization'] = `Bearer ${token}`;
                    console.log('HTMX Request - Added Authorization header');
                } else {
                    console.log('HTMX Request - No token found, request will fail');
                }
            });

            document.body.addEventListener('htmx:beforeRequest', (event) => {
                this.showLoading();
            });

            document.body.addEventListener('htmx:afterRequest', (event) => {
                this.hideLoading();
                console.log('HTMX Response - Status:', event.detail.xhr.status);
            });

            document.body.addEventListener('htmx:responseError', (event) => {
                console.log('HTMX Error - Status:', event.detail.xhr.status);
                if (event.detail.xhr.status === 401) {
                    console.log('HTMX Error - 401 Unauthorized, but ignoring redirect');
                    // HTMX 401 hatalarını görmezden gel, JavaScript zaten dashboard'ı yükleyecek
                    return;
                }
                
                Toast.show({
                    type: 'error',
                    title: 'Hata',
                    message: 'Bir hata oluştu. Lütfen tekrar deneyin.',
                    timeout: 5000
                });
            });
        },

        showLoading() {
            document.getElementById('loading-overlay').classList.remove('hidden');
        },

        hideLoading() {
            document.getElementById('loading-overlay').classList.add('hidden');
        },
        
        // SSL Expiry helper functions
        getLatestSSLExpiry(data) {
            const sslData = data.filter(ping => ping.ssl_expiry_date && ping.ssl_expiry_date !== '-');
            if (sslData.length === 0) return 'Bilinmiyor';
            
            // En son SSL expiry date'i al
            const latestSSL = sslData[0].ssl_expiry_date;
            return latestSSL;
        },
        
        getSSLExpiryDays(data) {
            const sslData = data.filter(ping => ping.ssl_expiry_date && ping.ssl_expiry_date !== '-');
            if (sslData.length === 0) return 'Bilinmiyor';
            
            const expiryDate = new Date(sslData[0].ssl_expiry_date);
            const now = new Date();
            const diffTime = expiryDate - now;
            const diffDays = Math.ceil(diffTime / (1000 * 60 * 60 * 24));
            
            if (diffDays < 0) {
                return `${Math.abs(diffDays)} gün önce`;
            } else if (diffDays === 0) {
                return 'Bugün';
            } else if (diffDays < 30) {
                return `${diffDays} gün`;
            } else {
                const months = Math.floor(diffDays / 30);
                return `${months} ay`;
            }
        },

        // Performance Trends functions
        async loadPerformanceTrendsData() {
            try {
                // Load summary cards
                await this.loadPerformanceSummary();
                
                // Load targets for filter
                await this.loadPerformanceTargetsForFilter();
                
                // Load performance alerts
                await this.loadPerformanceAlerts();
                
                // Initialize charts
                setTimeout(() => {
                    this.initResponseTimeTrendChart();
                    this.initPerformanceDistributionChart();
                    this.initPerformanceComparisonChart();
                }, 100);
                
            } catch (error) {
                console.error('Error loading performance trends data:', error);
            }
        },

        // Domain Registry functions
        loadQueryStats() {
            // Only load query stats on WHOIS Domain page
            if (!window.location.pathname.includes('whois-domain')) {
                return;
            }

            fetch('/api/domains/stats', {
                headers: {
                    'Authorization': `Bearer ${localStorage.getItem('token')}`
                }
            })
            .then(response => response.json())
            .then(data => {
                if (data.success) {
                    const statsDiv = document.getElementById('query-stats');
                    if (statsDiv) {
                        statsDiv.innerHTML = `
                        <div class="bg-blue-50 dark:bg-blue-900/20 rounded-lg p-4">
                            <div class="text-sm text-blue-600 dark:text-blue-400">Günlük Limit</div>
                            <div class="text-2xl font-bold text-blue-900 dark:text-blue-100">${data.daily_limit}</div>
                        </div>
                        <div class="bg-yellow-50 dark:bg-yellow-900/20 rounded-lg p-4">
                            <div class="text-sm text-yellow-600 dark:text-yellow-400">Kullanılan</div>
                            <div class="text-2xl font-bold text-yellow-900 dark:text-yellow-100">${data.used_today}</div>
                        </div>
                        <div class="bg-green-50 dark:bg-green-900/20 rounded-lg p-4">
                            <div class="text-sm text-green-600 dark:text-green-400">Kalan</div>
                            <div class="text-2xl font-bold text-green-900 dark:text-green-100">${data.remaining}</div>
                        </div>
                    `;
                    } else {
                        console.error('query-stats element not found');
                    }
                }
            })
            .catch(error => {
                console.error('Error loading query stats:', error);
            });
        },

        saveDomainToRegistry() {
            const domainInfo = window.currentDomainInfo;
            if (!domainInfo) {
                this.showToast('Önce bir domain sorgulayın', 'error');
                return;
            }

            fetch('/api/domains/registry', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                    'Authorization': `Bearer ${localStorage.getItem('token')}`
                },
                body: JSON.stringify({
                    domain: domainInfo.domain,
                    domain_info: domainInfo
                })
            })
            .then(response => response.json())
            .then(data => {
                if (data.success) {
                    this.showToast('Domain başarıyla kayıt defterine eklendi', 'success');
                    this.loadQueryStats(); // Refresh stats
                } else {
                    this.showToast(data.message || 'Domain kaydedilirken hata oluştu', 'error');
                }
            })
            .catch(error => {
                console.error('Error saving domain:', error);
                this.showToast('Domain kaydedilirken hata oluştu', 'error');
            });
        },

        viewDomainRegistry() {
            this.navigateTo('domain-registry');
        },

        loadDomainRegistry() {
            // Show loading state
            const refreshBtn = document.getElementById('refresh-btn');
            const refreshText = document.getElementById('refresh-text');
            const refreshSpinner = document.getElementById('refresh-spinner');
            
            if (refreshBtn && refreshText && refreshSpinner) {
                refreshBtn.disabled = true;
                refreshText.classList.add('hidden');
                refreshSpinner.classList.remove('hidden');
            }

            fetch('/api/domains/registry', {
                headers: {
                    'Authorization': `Bearer ${localStorage.getItem('token')}`
                }
            })
            .then(response => response.json())
            .then(data => {
                if (data.success) {
                    this.displayDomainRegistry(data.data);
                    this.showToast('Domain kayıt defteri yenilendi', 'success');
                } else {
                    this.showToast(data.message || 'Domain kayıt defteri yüklenemedi', 'error');
                }
            })
            .catch(error => {
                console.error('Error loading domain registry:', error);
                this.showToast('Domain kayıt defteri yüklenirken hata oluştu', 'error');
            })
            .finally(() => {
                // Hide loading state
                if (refreshBtn && refreshText && refreshSpinner) {
                    refreshBtn.disabled = false;
                    refreshText.classList.remove('hidden');
                    refreshSpinner.classList.add('hidden');
                }
            });
        },

        displayDomainRegistry(domains) {
            const container = document.getElementById('domain-registry-list');
            if (!container) return;

            if (!domains || domains.length === 0) {
                container.innerHTML = `
                    <div class="text-center py-8">
                        <div class="text-gray-500 dark:text-gray-400">
                            <svg class="mx-auto h-12 w-12 mb-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z"></path>
                            </svg>
                            <p class="text-lg font-medium">Henüz kayıtlı domain yok</p>
                            <p class="text-sm">WHOIS Domain sayfasından domain sorgulayıp kayıt defterine ekleyebilirsiniz</p>
                        </div>
                    </div>
                `;
                return;
            }

            const html = domains.map(domain => {
                const expireDate = new Date(domain.expire_date);
                const now = new Date();
                const daysUntilExpiry = Math.ceil((expireDate - now) / (1000 * 60 * 60 * 24));
                
                let expiryStatus = '';
                let expiryClass = '';
                
                if (daysUntilExpiry < 0) {
                    expiryStatus = 'Süresi Dolmuş';
                    expiryClass = 'bg-red-100 text-red-800 dark:bg-red-900 dark:text-red-200';
                } else if (daysUntilExpiry <= 30) {
                    expiryStatus = `${daysUntilExpiry} gün kaldı`;
                    expiryClass = 'bg-yellow-100 text-yellow-800 dark:bg-yellow-900 dark:text-yellow-200';
                } else {
                    expiryStatus = `${daysUntilExpiry} gün kaldı`;
                    expiryClass = 'bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-200';
                }

                return `
                    <div class="bg-white dark:bg-gray-800 rounded-lg shadow p-6">
                        <div class="flex justify-between items-start mb-4">
                            <div>
                                <h3 class="text-lg font-semibold text-gray-900 dark:text-gray-100">${domain.domain}</h3>
                                <p class="text-sm text-gray-600 dark:text-gray-400">${domain.registrar_name || 'Bilinmeyen Kayıt Şirketi'}</p>
                            </div>
                            <div class="flex space-x-2">
                                <span class="px-3 py-1 text-sm font-medium rounded-full ${expiryClass}">
                                    ${expiryStatus}
                                </span>
                                <button onclick="deleteDomainFromRegistry(${domain.id})" 
                                        class="px-3 py-1 text-sm text-red-600 hover:text-red-800 dark:text-red-400 dark:hover:text-red-300">
                                    Sil
                                </button>
                            </div>
                        </div>
                        
                        <div class="grid grid-cols-1 md:grid-cols-3 gap-4 text-sm">
                            <div>
                                <span class="text-gray-600 dark:text-gray-400">Oluşturulma:</span>
                                <span class="text-gray-900 dark:text-gray-100 ml-2">${this.formatDate(domain.create_date)}</span>
                            </div>
                            <div>
                                <span class="text-gray-600 dark:text-gray-400">Son Kullanma:</span>
                                <span class="text-gray-900 dark:text-gray-100 ml-2">${this.formatDate(domain.expire_date)}</span>
                            </div>
                            <div>
                                <span class="text-gray-600 dark:text-gray-400">Domain Yaşı:</span>
                                <span class="text-gray-900 dark:text-gray-100 ml-2">${domain.domain_age || 0} gün</span>
                            </div>
                        </div>
                    </div>
                `;
            }).join('');

            container.innerHTML = html;
        },

        deleteDomainFromRegistry(domainId) {
            if (!confirm('Bu domain\'i kayıt defterinden silmek istediğinizden emin misiniz?')) {
                return;
            }

            fetch(`/api/domains/registry/${domainId}`, {
                method: 'DELETE',
                headers: {
                    'Authorization': `Bearer ${localStorage.getItem('token')}`
                }
            })
            .then(response => response.json())
            .then(data => {
                if (data.success) {
                    this.showToast('Domain başarıyla silindi', 'success');
                    this.loadDomainRegistry(); // Refresh list
                } else {
                    this.showToast(data.message || 'Domain silinirken hata oluştu', 'error');
                }
            })
            .catch(error => {
                console.error('Error deleting domain:', error);
                this.showToast('Domain silinirken hata oluştu', 'error');
            });
        },

        // WHOIS Domain functions
        queryDomain() {
            const domainInput = document.getElementById('domain-input');
            const domain = domainInput.value.trim();
            
            if (!domain) {
                this.showToast('Lütfen bir domain adı girin', 'error');
                return;
            }

            // Show loading indicator
            document.getElementById('loading-indicator').classList.remove('hidden');
            document.getElementById('domain-info').classList.add('hidden');

            // Make API request
            fetch('/api/whois/domain', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                    'Authorization': `Bearer ${localStorage.getItem('token')}`
                },
                body: JSON.stringify({ domain: domain })
            })
            .then(response => response.json())
            .then(data => {
                document.getElementById('loading-indicator').classList.add('hidden');
                
                if (data.success) {
                    window.currentDomainInfo = data.data; // Store globally
                    this.displayDomainInfo(data.data);
                    document.getElementById('domain-actions').classList.remove('hidden');
                    this.loadQueryStats(); // Refresh query stats
                } else {
                    this.showToast(data.message || 'Domain bilgileri alınamadı', 'error');
                }
            })
            .catch(error => {
                document.getElementById('loading-indicator').classList.add('hidden');
                console.error('Domain query error:', error);
                this.showToast('Domain sorgulanırken bir hata oluştu', 'error');
            });
        },

        displayDomainInfo(domainInfo) {
            const domainInfoDiv = document.getElementById('domain-info');
            
            const html = `
                <div class="p-6">
                    <div class="flex justify-between items-center mb-6">
                        <h3 class="text-xl font-semibold text-gray-900 dark:text-gray-100">${domainInfo.domain}</h3>
                        <span class="px-3 py-1 text-sm font-medium rounded-full ${domainInfo.status === 'registered' || domainInfo.domain_id ? 'bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-200' : 'bg-red-100 text-red-800 dark:bg-red-900 dark:text-red-200'}">
                            ${domainInfo.status === 'registered' || domainInfo.domain_id ? 'Kayıtlı' : 'Kayıtsız'}
                        </span>
                    </div>

                    <div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
                        <!-- Domain Details -->
                        <div class="space-y-4">
                            <h4 class="text-lg font-medium text-gray-900 dark:text-gray-100">Domain Bilgileri</h4>
                            <div class="space-y-2">
                                <div class="flex justify-between">
                                    <span class="text-sm text-gray-600 dark:text-gray-400">Domain ID:</span>
                                    <span class="text-sm text-gray-900 dark:text-gray-100">${domainInfo.domain_id || 'N/A'}</span>
                                </div>
                                <div class="flex justify-between">
                                    <span class="text-sm text-gray-600 dark:text-gray-400">Domain Yaşı:</span>
                                    <span class="text-sm text-gray-900 dark:text-gray-100">${domainInfo.domain_age ? domainInfo.domain_age + ' gün' : 'N/A'}</span>
                                </div>
                                <div class="flex justify-between">
                                    <span class="text-sm text-gray-600 dark:text-gray-400">WHOIS Server:</span>
                                    <span class="text-sm text-gray-900 dark:text-gray-100">${domainInfo.whois_server || 'N/A'}</span>
                                </div>
                            </div>
                        </div>

                        <!-- Dates -->
                        <div class="space-y-4">
                            <h4 class="text-lg font-medium text-gray-900 dark:text-gray-100">Tarihler</h4>
                            <div class="space-y-2">
                                <div class="flex justify-between">
                                    <span class="text-sm text-gray-600 dark:text-gray-400">Oluşturulma:</span>
                                    <span class="text-sm text-gray-900 dark:text-gray-100">${this.formatDate(domainInfo.create_date)}</span>
                                </div>
                                <div class="flex justify-between">
                                    <span class="text-sm text-gray-600 dark:text-gray-400">Güncellenme:</span>
                                    <span class="text-sm text-gray-900 dark:text-gray-100">${this.formatDate(domainInfo.update_date, true)}</span>
                                </div>
                                <div class="flex justify-between">
                                    <span class="text-sm text-gray-600 dark:text-gray-400">Son Kullanma:</span>
                                    <span class="text-sm text-gray-900 dark:text-gray-100">${this.formatDate(domainInfo.expire_date)}</span>
                                </div>
                            </div>
                        </div>

                        <!-- Registrar -->
                        <div class="space-y-4">
                            <h4 class="text-lg font-medium text-gray-900 dark:text-gray-100">Kayıt Şirketi</h4>
                            <div class="space-y-2">
                                <div class="flex justify-between">
                                    <span class="text-sm text-gray-600 dark:text-gray-400">Ad:</span>
                                    <span class="text-sm text-gray-900 dark:text-gray-100">${domainInfo.registrar.name || 'N/A'}</span>
                                </div>
                                <div class="flex justify-between">
                                    <span class="text-sm text-gray-600 dark:text-gray-400">IANA ID:</span>
                                    <span class="text-sm text-gray-900 dark:text-gray-100">${domainInfo.registrar.iana_id || 'N/A'}</span>
                                </div>
                                ${domainInfo.registrar.url ? `
                                <div class="flex justify-between">
                                    <span class="text-sm text-gray-600 dark:text-gray-400">Website:</span>
                                    <a href="${domainInfo.registrar.url}" target="_blank" class="text-sm text-primary-600 hover:text-primary-800 dark:text-primary-400 dark:hover:text-primary-300">Ziyaret Et</a>
                                </div>
                                ` : ''}
                            </div>
                        </div>
                    </div>

                    <!-- Contact Information -->
                    <div class="mt-8 grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-6">
                        <!-- Registrant -->
                        <div class="space-y-4">
                            <h4 class="text-lg font-medium text-gray-900 dark:text-gray-100">Kayıt Sahibi</h4>
                            <div class="space-y-2">
                                ${domainInfo.registrant && domainInfo.registrant.name ? `<div class="text-sm"><span class="text-gray-600 dark:text-gray-400">Ad:</span> <span class="text-gray-900 dark:text-gray-100">${domainInfo.registrant.name}</span></div>` : '<div class="text-sm text-gray-500 dark:text-gray-400">Bilgi mevcut değil</div>'}
                                ${domainInfo.registrant && domainInfo.registrant.organization ? `<div class="text-sm"><span class="text-gray-600 dark:text-gray-400">Organizasyon:</span> <span class="text-gray-900 dark:text-gray-100">${domainInfo.registrant.organization}</span></div>` : ''}
                                ${domainInfo.registrant && domainInfo.registrant.email ? `<div class="text-sm"><span class="text-gray-600 dark:text-gray-400">Email:</span> <span class="text-gray-900 dark:text-gray-100">${domainInfo.registrant.email}</span></div>` : ''}
                                ${domainInfo.registrant && domainInfo.registrant.phone ? `<div class="text-sm"><span class="text-gray-600 dark:text-gray-400">Telefon:</span> <span class="text-gray-900 dark:text-gray-100">${domainInfo.registrant.phone}</span></div>` : ''}
                                ${domainInfo.registrant && domainInfo.registrant.country ? `<div class="text-sm"><span class="text-gray-600 dark:text-gray-400">Ãœlke:</span> <span class="text-gray-900 dark:text-gray-100">${domainInfo.registrant.country}</span></div>` : ''}
                            </div>
                        </div>

                        <!-- Admin -->
                        <div class="space-y-4">
                            <h4 class="text-lg font-medium text-gray-900 dark:text-gray-100">Yönetici</h4>
                            <div class="space-y-2">
                                ${domainInfo.admin && domainInfo.admin.name ? `<div class="text-sm"><span class="text-gray-600 dark:text-gray-400">Ad:</span> <span class="text-gray-900 dark:text-gray-100">${domainInfo.admin.name}</span></div>` : '<div class="text-sm text-gray-500 dark:text-gray-400">Bilgi mevcut değil</div>'}
                                ${domainInfo.admin && domainInfo.admin.organization ? `<div class="text-sm"><span class="text-gray-600 dark:text-gray-400">Organizasyon:</span> <span class="text-gray-900 dark:text-gray-100">${domainInfo.admin.organization}</span></div>` : ''}
                                ${domainInfo.admin && domainInfo.admin.email ? `<div class="text-sm"><span class="text-gray-600 dark:text-gray-400">Email:</span> <span class="text-gray-900 dark:text-gray-100">${domainInfo.admin.email}</span></div>` : ''}
                                ${domainInfo.admin && domainInfo.admin.phone ? `<div class="text-sm"><span class="text-gray-600 dark:text-gray-400">Telefon:</span> <span class="text-gray-900 dark:text-gray-100">${domainInfo.admin.phone}</span></div>` : ''}
                            </div>
                        </div>

                        <!-- Tech -->
                        <div class="space-y-4">
                            <h4 class="text-lg font-medium text-gray-900 dark:text-gray-100">Teknik</h4>
                            <div class="space-y-2">
                                ${domainInfo.tech && domainInfo.tech.name ? `<div class="text-sm"><span class="text-gray-600 dark:text-gray-400">Ad:</span> <span class="text-gray-900 dark:text-gray-100">${domainInfo.tech.name}</span></div>` : '<div class="text-sm text-gray-500 dark:text-gray-400">Bilgi mevcut değil</div>'}
                                ${domainInfo.tech && domainInfo.tech.organization ? `<div class="text-sm"><span class="text-gray-600 dark:text-gray-400">Organizasyon:</span> <span class="text-gray-900 dark:text-gray-100">${domainInfo.tech.organization}</span></div>` : ''}
                                ${domainInfo.tech && domainInfo.tech.email ? `<div class="text-sm"><span class="text-gray-600 dark:text-gray-400">Email:</span> <span class="text-gray-900 dark:text-gray-100">${domainInfo.tech.email}</span></div>` : ''}
                                ${domainInfo.tech && domainInfo.tech.phone ? `<div class="text-sm"><span class="text-gray-600 dark:text-gray-400">Telefon:</span> <span class="text-gray-900 dark:text-gray-100">${domainInfo.tech.phone}</span></div>` : ''}
                            </div>
                        </div>

                        <!-- Billing -->
                        <div class="space-y-4">
                            <h4 class="text-lg font-medium text-gray-900 dark:text-gray-100">Faturalama</h4>
                            <div class="space-y-2">
                                ${domainInfo.billing && domainInfo.billing.name ? `<div class="text-sm"><span class="text-gray-600 dark:text-gray-400">Ad:</span> <span class="text-gray-900 dark:text-gray-100">${domainInfo.billing.name}</span></div>` : '<div class="text-sm text-gray-500 dark:text-gray-400">Bilgi mevcut değil</div>'}
                                ${domainInfo.billing && domainInfo.billing.organization ? `<div class="text-sm"><span class="text-gray-600 dark:text-gray-400">Organizasyon:</span> <span class="text-gray-900 dark:text-gray-100">${domainInfo.billing.organization}</span></div>` : ''}
                                ${domainInfo.billing && domainInfo.billing.email ? `<div class="text-sm"><span class="text-gray-600 dark:text-gray-400">Email:</span> <span class="text-gray-900 dark:text-gray-100">${domainInfo.billing.email}</span></div>` : ''}
                                ${domainInfo.billing && domainInfo.billing.phone ? `<div class="text-sm"><span class="text-gray-600 dark:text-gray-400">Telefon:</span> <span class="text-gray-900 dark:text-gray-100">${domainInfo.billing.phone}</span></div>` : ''}
                            </div>
                        </div>
                    </div>

                    <!-- Name Servers -->
                    ${domainInfo.nameservers && domainInfo.nameservers.length > 0 ? `
                    <div class="mt-8">
                        <h4 class="text-lg font-medium text-gray-900 dark:text-gray-100 mb-4">Name Server'lar</h4>
                        <div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-2">
                            ${domainInfo.nameservers.map(ns => `
                                <div class="bg-gray-50 dark:bg-gray-700 rounded-lg p-3">
                                    <span class="text-sm text-gray-900 dark:text-gray-100 font-mono">${ns}</span>
                                </div>
                            `).join('')}
                        </div>
                    </div>
                    ` : ''}
                </div>
            `;
            
            domainInfoDiv.innerHTML = html;
            domainInfoDiv.classList.remove('hidden');
        },

        formatDate(dateString, isUpdateDate = false) {
            if (!dateString) return 'N/A';
            try {
                const date = new Date(dateString);
                const now = new Date();
                
                // Eğer güncellenme tarihi gelecekteyse, muhtemelen yanlış veri
                if (isUpdateDate && date > now) {
                    return 'N/A';
                }
                
                return date.toLocaleDateString('tr-TR', {
                    year: 'numeric',
                    month: 'long',
                    day: 'numeric',
                    hour: '2-digit',
                    minute: '2-digit'
                });
            } catch (error) {
                return dateString;
            }
        },

        async loadPerformanceSummary() {
            try {
                // Get filter values
                const targetFilter = document.getElementById('perfTargetFilter')?.value || 'all';
                
                const response = await fetch('/api/pings/recent', {
                    headers: {
                        'Authorization': `Bearer ${localStorage.getItem('token')}`
                    }
                });
                
                if (!response.ok) {
                    throw new Error('Failed to load performance summary');
                }
                
                const data = await response.json();
                console.log('Performance summary data:', data);
                
                // Filter targets based on selection
                let filteredTargets = data.targets;
                if (targetFilter !== 'all') {
                    filteredTargets = data.targets.filter(target => target && target.target_id && target.target_id.toString() === targetFilter);
                }
                
                // Calculate summary statistics
                let totalResponseTime = 0;
                let responseTimeCount = 0;
                let minResponseTime = Infinity;
                let maxResponseTime = 0;
                let errorCount = 0;
                let totalPings = 0;
                
                console.log('Filtered targets:', filteredTargets);
                
                filteredTargets.forEach(target => {
                    console.log('Processing target:', target.name, 'pings:', target.pings);
                    target.pings.forEach(ping => {
                        totalPings++;
                        if (ping.duration_ms) {
                            totalResponseTime += ping.duration_ms;
                            responseTimeCount++;
                            minResponseTime = Math.min(minResponseTime, ping.duration_ms);
                            maxResponseTime = Math.max(maxResponseTime, ping.duration_ms);
                        }
                        if (!ping.success) {
                            errorCount++;
                        }
                    });
                });
                
                const avgResponseTime = responseTimeCount > 0 ? totalResponseTime / responseTimeCount : 0;
                const errorRate = totalPings > 0 ? (errorCount / totalPings) * 100 : 0;
                
                // Update summary cards
                document.getElementById('avg-response-time').textContent = avgResponseTime.toFixed(1) + 'ms';
                document.getElementById('min-response-time').textContent = minResponseTime === Infinity ? '-' : minResponseTime.toFixed(1) + 'ms';
                document.getElementById('max-response-time').textContent = maxResponseTime.toFixed(1) + 'ms';
                document.getElementById('error-rate').textContent = errorRate.toFixed(1) + '%';
                
            } catch (error) {
                console.error('Error loading performance summary:', error);
                // Set default values
                document.getElementById('avg-response-time').textContent = '-';
                document.getElementById('min-response-time').textContent = '-';
                document.getElementById('max-response-time').textContent = '-';
                document.getElementById('error-rate').textContent = '-';
            }
        },

        async loadPerformanceTargetsForFilter() {
            try {
                const response = await fetch('/api/targets', {
                    headers: {
                        'Authorization': `Bearer ${localStorage.getItem('token')}`
                    }
                });
                
                if (!response.ok) {
                    throw new Error('Failed to load targets');
                }
                
                const data = await response.json();
                const targets = data.targets || [];
                
                const targetFilter = document.getElementById('perfTargetFilter');
                if (targetFilter) {
                    // Clear existing options except "all"
                    targetFilter.innerHTML = '<option value="all">Tüm Target\'lar</option>';
                    
                    targets.forEach(target => {
                        const option = document.createElement('option');
                        option.value = target.id;
                        option.textContent = target.name;
                        targetFilter.appendChild(option);
                    });
                }
                
            } catch (error) {
                console.error('Error loading targets for performance filter:', error);
            }
        },

        async loadPerformanceAlerts() {
            try {
                const response = await fetch('/api/alerts', {
                    headers: {
                        'Authorization': `Bearer ${localStorage.getItem('token')}`
                    }
                });
                
                if (!response.ok) {
                    throw new Error('Failed to load performance alerts');
                }
                
                const data = await response.json();
                const alerts = data.alerts || [];
                const alertsList = document.getElementById('performance-alerts-list');
                
                if (alertsList) {
                    if (alerts.length === 0) {
                        alertsList.innerHTML = '<p class="text-gray-500 dark:text-gray-400 text-center py-4">Henüz performans uyarısı bulunmuyor.</p>';
                        return;
                    }
                    
                    // Filter performance-related alerts (response time, uptime, etc.)
                    const performanceAlerts = alerts.filter(alert => 
                        alert.message && (
                            alert.message.toLowerCase().includes('yanıt süresi') ||
                            alert.message.toLowerCase().includes('response time') ||
                            alert.message.toLowerCase().includes('uptime') ||
                            alert.message.toLowerCase().includes('performans') ||
                            alert.message.toLowerCase().includes('performance') ||
                            alert.message.toLowerCase().includes('sla')
                        )
                    );
                    
                    if (performanceAlerts.length === 0) {
                        alertsList.innerHTML = '<p class="text-gray-500 dark:text-gray-400 text-center py-4">Henüz performans uyarısı bulunmuyor.</p>';
                        return;
                    }
                    
                    alertsList.innerHTML = '<div class="space-y-3">';
                    
                    performanceAlerts.slice(0, 10).forEach(alert => {
                        const levelClass = alert.level === 'major' ? 
                            'bg-red-100 dark:bg-red-900/20 text-red-800 dark:text-red-400' : 
                            'bg-yellow-100 dark:bg-yellow-900/20 text-yellow-800 dark:text-yellow-400';
                        
                        const statusClass = alert.status === 'open' ? 
                            'bg-red-100 dark:bg-red-900/20 text-red-800 dark:text-red-400' : 
                            'bg-green-100 dark:bg-green-900/20 text-green-800 dark:text-green-400';
                        
                        const statusText = alert.status === 'open' ? 'Açık' : 'Kapalı';
                        
                        const alertDate = formatTurkishDate(alert.created_at);
                        
                        alertsList.innerHTML += `
                            <div class="flex items-center justify-between p-3 bg-gray-50 dark:bg-gray-700 rounded-lg">
                                <div class="flex items-center space-x-3">
                                    <span class="px-2 py-1 text-xs font-medium rounded-full ${levelClass}">${alert.level}</span>
                                    <div>
                                        <p class="text-sm font-medium text-gray-900 dark:text-gray-100">${alert.target_name}</p>
                                        <p class="text-xs text-gray-500 dark:text-gray-400">${alert.message}</p>
                                    </div>
                                </div>
                                <div class="flex items-center space-x-2">
                                    <span class="px-2 py-1 text-xs font-medium rounded-full ${statusClass}">${statusText}</span>
                                    <span class="text-xs text-gray-500 dark:text-gray-400">${alertDate}</span>
                                </div>
                            </div>
                        `;
                    });
                    
                    alertsList.innerHTML += '</div>';
                }
                
            } catch (error) {
                console.error('Error loading performance alerts:', error);
                const alertsList = document.getElementById('performance-alerts-list');
                if (alertsList) {
                    alertsList.innerHTML = '<p class="text-red-500 text-center py-4">Uyarılar yüklenirken hata oluştu.</p>';
                }
            }
        },

        // Performance Trends Charts
        initResponseTimeTrendChart() {
            const ctx = document.getElementById('responseTimeTrendChart');
            if (!ctx) return;

            // Destroy existing chart if it exists
            if (this.responseTimeTrendChart) {
                this.responseTimeTrendChart.destroy();
                this.responseTimeTrendChart = null;
            }

            // Also destroy any chart with this canvas ID from global registry
            Chart.getChart(ctx)?.destroy();

            // Response time trend verilerini getir
            this.loadResponseTimeTrendData().then(data => {
                this.responseTimeTrendChart = new Chart(ctx, {
                    type: 'line',
                    data: {
                        labels: data.labels,
                        datasets: [{
                            label: 'Ortalama Yanıt Süresi (ms)',
                            data: data.responseTimes,
                            borderColor: '#3b82f6',
                            backgroundColor: 'rgba(59, 130, 246, 0.1)',
                            borderWidth: 2,
                            fill: true,
                            tension: 0.4
                        }, {
                            label: 'Maksimum Yanıt Süresi (ms)',
                            data: data.maxResponseTimes,
                            borderColor: '#ef4444',
                            backgroundColor: 'rgba(239, 68, 68, 0.1)',
                            borderWidth: 2,
                            fill: false,
                            tension: 0.4
                        }]
                    },
                    options: {
                        responsive: true,
                        maintainAspectRatio: false,
                        plugins: {
                            legend: {
                                labels: {
                                    color: this.isDark ? '#d1d5db' : '#374151'
                                }
                            }
                        },
                        scales: {
                            x: {
                                ticks: {
                                    color: this.isDark ? '#d1d5db' : '#374151'
                                },
                                grid: {
                                    color: this.isDark ? '#374151' : '#e5e7eb'
                                }
                            },
                            y: {
                                ticks: {
                                    color: this.isDark ? '#d1d5db' : '#374151',
                                    callback: function(value) {
                                        return value + 'ms';
                                    }
                                },
                                grid: {
                                    color: this.isDark ? '#374151' : '#e5e7eb'
                                }
                            }
                        }
                    }
                });
            });
        },

        initPerformanceDistributionChart() {
            const ctx = document.getElementById('performanceDistributionChart');
            if (!ctx) return;

            // Destroy existing chart if it exists
            if (this.performanceDistributionChart) {
                this.performanceDistributionChart.destroy();
                this.performanceDistributionChart = null;
            }

            // Also destroy any chart with this canvas ID from global registry
            Chart.getChart(ctx)?.destroy();

            // Performance distribution verilerini getir
            this.loadPerformanceDistributionData().then(data => {
                this.performanceDistributionChart = new Chart(ctx, {
                    type: 'doughnut',
                    data: {
                        labels: data.labels,
                        datasets: [{
                            data: data.values,
                            backgroundColor: [
                                '#10b981', // Green - Excellent
                                '#3b82f6', // Blue - Good
                                '#f59e0b', // Yellow - Fair
                                '#ef4444'  // Red - Poor
                            ],
                            borderColor: [
                                '#059669',
                                '#2563eb',
                                '#d97706',
                                '#dc2626'
                            ],
                            borderWidth: 2
                        }]
                    },
                    options: {
                        responsive: true,
                        maintainAspectRatio: false,
                        plugins: {
                            legend: {
                                position: 'bottom',
                                labels: {
                                    color: this.isDark ? '#d1d5db' : '#374151',
                                    padding: 20
                                }
                            }
                        }
                    }
                });
            });
        },

        initPerformanceComparisonChart() {
            const ctx = document.getElementById('performanceComparisonChart');
            if (!ctx) return;

            // Destroy existing chart if it exists
            if (this.performanceComparisonChart) {
                this.performanceComparisonChart.destroy();
                this.performanceComparisonChart = null;
            }

            // Also destroy any chart with this canvas ID from global registry
            Chart.getChart(ctx)?.destroy();

            // Performance comparison verilerini getir
            this.loadPerformanceComparisonData().then(data => {
                this.performanceComparisonChart = new Chart(ctx, {
                    type: 'bar',
                    data: {
                        labels: data.labels,
                        datasets: [{
                            label: 'Ortalama Yanıt Süresi (ms)',
                            data: data.avgResponseTimes,
                            backgroundColor: '#3b82f6',
                            borderColor: '#2563eb',
                            borderWidth: 1
                        }, {
                            label: 'Maksimum Yanıt Süresi (ms)',
                            data: data.maxResponseTimes,
                            backgroundColor: '#ef4444',
                            borderColor: '#dc2626',
                            borderWidth: 1
                        }]
                    },
                    options: {
                        responsive: true,
                        maintainAspectRatio: false,
                        plugins: {
                            legend: {
                                labels: {
                                    color: this.isDark ? '#d1d5db' : '#374151'
                                }
                            }
                        },
                        scales: {
                            x: {
                                ticks: {
                                    color: this.isDark ? '#d1d5db' : '#374151'
                                },
                                grid: {
                                    color: this.isDark ? '#374151' : '#e5e7eb'
                                }
                            },
                            y: {
                                ticks: {
                                    color: this.isDark ? '#d1d5db' : '#374151',
                                    callback: function(value) {
                                        return value + 'ms';
                                    }
                                },
                                grid: {
                                    color: this.isDark ? '#374151' : '#e5e7eb'
                                }
                            }
                        }
                    }
                });
            });
        },

        // Performance data loading functions
        async loadResponseTimeTrendData() {
            try {
                // Get filter values
                const targetFilter = document.getElementById('perfTargetFilter')?.value || 'all';
                const dateRange = document.getElementById('perfDateRange')?.value || '24h';
                
                const response = await fetch('/api/pings/recent', {
                    headers: {
                        'Authorization': `Bearer ${localStorage.getItem('token')}`
                    }
                });
                
                if (!response.ok) {
                    throw new Error('Failed to load response time trend data');
                }
                
                const data = await response.json();
                
                // Filter data based on target selection
                let filteredTargets = data.targets;
                if (targetFilter !== 'all') {
                    filteredTargets = data.targets.filter(target => target && target.target_id && target.target_id.toString() === targetFilter);
                }
                
                // Son 24 saatlik veri için mock data oluştur
                const labels = [];
                const responseTimes = [];
                const maxResponseTimes = [];
                
                for (let i = 23; i >= 0; i--) {
                    const time = new Date();
                    time.setHours(time.getHours() - i);
                    labels.push(time.toLocaleTimeString('tr-TR', { hour: '2-digit', minute: '2-digit' }));
                    
                    // Mock response time data - vary based on target count
                    const baseTime = filteredTargets.length > 0 ? 50 + Math.random() * 100 : 0;
                    responseTimes.push(baseTime);
                    maxResponseTimes.push(baseTime + Math.random() * 50);
                }
                
                return { labels, responseTimes, maxResponseTimes };
            } catch (error) {
                console.error('Error loading response time trend data:', error);
                // Fallback data
                const labels = [];
                const responseTimes = [];
                const maxResponseTimes = [];
                
                for (let i = 23; i >= 0; i--) {
                    const time = new Date();
                    time.setHours(time.getHours() - i);
                    labels.push(time.toLocaleTimeString('tr-TR', { hour: '2-digit', minute: '2-digit' }));
                    responseTimes.push(80 + Math.random() * 40);
                    maxResponseTimes.push(120 + Math.random() * 60);
                }
                
                return { labels, responseTimes, maxResponseTimes };
            }
        },

        async loadPerformanceDistributionData() {
            try {
                // Mock data for performance distribution
                const labels = ['Mükemmel (<50ms)', 'İyi (50-100ms)', 'Orta (100-200ms)', 'Zayıf (>200ms)'];
                const values = [25, 45, 20, 10]; // Percentage distribution
                
                return { labels, values };
            } catch (error) {
                console.error('Error loading performance distribution data:', error);
                return { labels: ['Mükemmel', 'İyi', 'Orta', 'Zayıf'], values: [30, 40, 20, 10] };
            }
        },

        async loadPerformanceComparisonData() {
            try {
                // Get filter values
                const targetFilter = document.getElementById('perfTargetFilter')?.value || 'all';
                
                const response = await fetch('/api/targets', {
                    headers: {
                        'Authorization': `Bearer ${localStorage.getItem('token')}`
                    }
                });
                
                if (!response.ok) {
                    throw new Error('Failed to load targets data');
                }
                
                const data = await response.json();
                let targets = data.targets || [];
                
                // Filter targets based on selection
                if (targetFilter !== 'all') {
                    targets = targets.filter(target => target && target.target_id && target.target_id.toString() === targetFilter);
                }
                
                // İlk 8 target'ı al ve mock performance data oluştur
                const labels = targets.slice(0, 8).map(t => t.name);
                const avgResponseTimes = [];
                const maxResponseTimes = [];
                
                targets.slice(0, 8).forEach((target, index) => {
                    // Mock performance data
                    const avgTime = 60 + Math.random() * 80; // 60-140ms
                    const maxTime = avgTime + 20 + Math.random() * 40; // Max is higher
                    
                    avgResponseTimes.push(avgTime);
                    maxResponseTimes.push(maxTime);
                });
                
                return { labels, avgResponseTimes, maxResponseTimes };
            } catch (error) {
                console.error('Error loading performance comparison data:', error);
                // Fallback data
                const labels = ['Target 1', 'Target 2', 'Target 3', 'Target 4', 'Target 5'];
                const avgResponseTimes = [85, 120, 95, 150, 110];
                const maxResponseTimes = [120, 180, 140, 220, 160];
                return { labels, avgResponseTimes, maxResponseTimes };
            }
        },

        // IP Blacklist functions
        async loadIPBlacklist() {
            try {
                const response = await fetch('/api/ip-blacklist', {
                    headers: {
                        'Authorization': `Bearer ${localStorage.getItem('token')}`
                    }
                });

                if (!response.ok) {
                    throw new Error('Failed to load IP blacklist');
                }

                const data = await response.json();
                this.ipBlacklist = data.data || [];
                this.calculateIPBlacklistSummary();
                
                // Check edit permission for IP Blacklist
                this.canEditIPBlacklist = this.canViewModule('ip_blacklist') && this.userModulePermissions['ip_blacklist']?.edit;
                
                // Load IP query limit info
                await this.loadIPQueryLimitInfo();
            } catch (error) {
                console.error('Error loading IP blacklist:', error);
                Toast.error('IP blacklist yüklenirken hata oluştu');
            }
        },

        calculateIPBlacklistSummary() {
            if (!this.ipBlacklist || !Array.isArray(this.ipBlacklist)) {
                this.ipBlacklistSummary.totalIPs = 0;
                this.ipBlacklistSummary.highRiskIPs = 0;
                this.ipBlacklistSummary.torIPs = 0;
                this.ipBlacklistSummary.whitelistedIPs = 0;
                return;
            }
            
            this.ipBlacklistSummary.totalIPs = this.ipBlacklist.length;
            this.ipBlacklistSummary.highRiskIPs = this.ipBlacklist.filter(ip => ip.abuse_confidence >= 75).length;
            this.ipBlacklistSummary.torIPs = this.ipBlacklist.filter(ip => ip.is_tor).length;
            this.ipBlacklistSummary.whitelistedIPs = this.ipBlacklist.filter(ip => ip.is_whitelisted).length;
        },

        async loadIPQueryLimitInfo() {
            try {
                const token = localStorage.getItem('token');
                if (!token) return;

                // Get user ID from token
                const payload = this.decodeJwtPayload(token);
                const userId = payload?.user_id;
                if (!userId) return;

                const response = await fetch(`/api/users/${userId}/ip-query-limit`, {
                    headers: {
                        'Authorization': `Bearer ${token}`
                    }
                });

                if (response.ok) {
                    const data = await response.json();
                    this.ipQueryLimitInfo = {
                        used: data.used || 0,
                        limit: data.limit || 0
                    };
                }
            } catch (error) {
                console.error('Error loading IP query limit info:', error);
            }
        },

        // CSV Import/Export functions
        openCSVImportModal() {
            const modal = document.getElementById('csv-import-modal');
            const modalRoot = document.getElementById('modal-root');

            if (modal && modalRoot && !modalRoot.contains(modal)) {
                modalRoot.appendChild(modal);
            }

            if (modal) {
                modal.classList.remove('hidden');
            }

            // Reset form
            const fileInput = document.getElementById('csv-file-input');
            const progress = document.getElementById('import-progress');
            const results = document.getElementById('import-results');

            if (fileInput) {
                fileInput.value = '';
            }
            if (progress) {
                progress.classList.add('hidden');
            }
            if (results) {
                results.classList.add('hidden');
            }
        },

        closeCSVImportModal() {
            const modal = document.getElementById('csv-import-modal');
            if (modal) {
                modal.classList.add('hidden');
            }
        },

        downloadCSVTemplate() {
            // Create CSV template content
            const csvContent = [
                'name,address,type,port,path,interval_sec,timeout_ms,enabled,tags',
                'Google DNS,8.8.8.8,ping,,,30,5000,true,DNS',
                'Cloudflare DNS,1.1.1.1,ping,,,30,5000,true,DNS',
                'Google HTTPS,google.com,https,443,,30,5000,true,Web',
                'GitHub HTTPS,github.com,https,443,,30,5000,true,Web',
                'Local Server,192.168.1.100,http,80,/status,60,3000,true,Local',
                'API Endpoint,api.example.com,https,443,/health,30,5000,true,API'
            ].join('\n');

            // Create blob and download
            const blob = new Blob([csvContent], { type: 'text/csv;charset=utf-8;' });
            const link = document.createElement('a');
            const url = URL.createObjectURL(blob);
            link.setAttribute('href', url);
            link.setAttribute('download', 'targets_template.csv');
            link.style.visibility = 'hidden';
            document.body.appendChild(link);
            link.click();
            document.body.removeChild(link);
            URL.revokeObjectURL(url);

            // Show success message
            Toast.show({
                type: 'success',
                title: 'Şablon İndirildi',
                message: 'Örnek CSV şablonu indirildi. Dosyayı düzenleyip tekrar yükleyebilirsiniz.',
                timeout: 3000
            });
        },

        async importCSVFile() {
            const fileInput = document.getElementById('csv-file-input');
            const file = fileInput.files[0];
            
            if (!file) {
                Toast.error('Lütfen bir CSV dosyası seçin');
                return;
            }

            if (!file.name.toLowerCase().endsWith('.csv')) {
                Toast.error('Lütfen geçerli bir CSV dosyası seçin');
                return;
            }

            const formData = new FormData();
            formData.append('file', file);

            // Show progress
            document.getElementById('import-progress').classList.remove('hidden');
            document.getElementById('import-results').classList.add('hidden');

            try {
                const response = await fetch('/api/targets/import', {
                    method: 'POST',
                    headers: {
                        'Authorization': `Bearer ${localStorage.getItem('token')}`
                    },
                    body: formData
                });

                const result = await response.json();

                // Hide progress, show results
                document.getElementById('import-progress').classList.add('hidden');
                document.getElementById('import-results').classList.remove('hidden');

                // Update result display
                document.getElementById('total-count').textContent = result.total || 0;
                document.getElementById('success-count').textContent = result.success || 0;
                document.getElementById('failed-count').textContent = result.failed || 0;

                // Show errors if any
                const errorList = document.getElementById('error-list');
                errorList.innerHTML = '';
                if (result.errors && result.errors.length > 0) {
                    result.errors.forEach(error => {
                        const errorDiv = document.createElement('div');
                        errorDiv.className = 'text-xs text-red-600 dark:text-red-400 mb-1';
                        errorDiv.textContent = error;
                        errorList.appendChild(errorDiv);
                    });
                }

                if (result.success > 0) {
                    Toast.success(`${result.success} hedef başarıyla içe aktarıldı`);
                    // Reload targets if on targets page
                    if (this.currentRoute === 'targets') {
                        this.loadPageContent('targets');
                    }
                } else if (result.failed > 0) {
                    Toast.error(`${result.failed} hedef içe aktarılamadı`);
                }

            } catch (error) {
                console.error('CSV import error:', error);
                Toast.error('CSV içe aktarma sırasında hata oluştu');
                document.getElementById('import-progress').classList.add('hidden');
            }
        },

        async exportTargetsCSV() {
            try {
                const response = await fetch('/api/targets/export', {
                    headers: {
                        'Authorization': `Bearer ${localStorage.getItem('token')}`
                    }
                });

                if (response.ok) {
                    // Create download link
                    const blob = await response.blob();
                    const url = window.URL.createObjectURL(blob);
                    const a = document.createElement('a');
                    a.href = url;
                    a.download = `targets_export_${new Date().toISOString().split('T')[0]}.csv`;
                    document.body.appendChild(a);
                    a.click();
                    document.body.removeChild(a);
                    window.URL.revokeObjectURL(url);

                    Toast.success('Hedefler CSV olarak dışa aktarıldı');
                } else {
                    Toast.error('CSV dışa aktarma sırasında hata oluştu');
                }
            } catch (error) {
                console.error('CSV export error:', error);
                Toast.error('CSV dışa aktarma sırasında hata oluştu');
            }
        },

        async checkIP() {
            if (!this.ipToCheck) {
                Toast.error('Lütfen bir IP adresi girin');
                return;
            }

            // Check if limit is exceeded
            if (this.ipQueryLimitInfo.used >= this.ipQueryLimitInfo.limit) {
                Toast.error(`Günlük IP sorgulama limitiniz aşıldı! (${this.ipQueryLimitInfo.used}/${this.ipQueryLimitInfo.limit})`);
                return;
            }

            this.checkingIP = true;
            try {
                const response = await fetch('/api/ip-blacklist/check', {
                    method: 'POST',
                    headers: {
                        'Content-Type': 'application/json',
                        'Authorization': `Bearer ${localStorage.getItem('token')}`
                    },
                    body: JSON.stringify({
                        ip_address: this.ipToCheck
                    })
                });

                if (!response.ok) {
                    if (response.status === 429) {
                        // IP query limit exceeded
                        const errorData = await response.json();
                        const message = `Günlük IP sorgulama limitiniz aşıldı! (${errorData.used}/${errorData.limit})`;
                        Toast.error(message);
                        return;
                    }
                    throw new Error('Failed to check IP');
                }

                const data = await response.json();
                this.ipCheckResult = data.data;
                
                // Update query limit info after successful query
                await this.loadIPQueryLimitInfo();
            } catch (error) {
                console.error('Error checking IP:', error);
                Toast.error('IP sorgulanırken hata oluştu');
            } finally {
                this.checkingIP = false;
            }
        },

        async saveIP() {
            if (!this.ipCheckResult) {
                return;
            }

            try {
                const response = await fetch('/api/ip-blacklist/save', {
                    method: 'POST',
                    headers: {
                        'Content-Type': 'application/json',
                        'Authorization': `Bearer ${localStorage.getItem('token')}`
                    },
                    body: JSON.stringify(this.ipCheckResult)
                });

                if (!response.ok) {
                    throw new Error('Failed to save IP');
                }

                Toast.success('IP adresi başarıyla kaydedildi');
                this.showIPCheckModal = false;
                this.ipCheckResult = null;
                this.ipToCheck = '';
                await this.loadIPBlacklist();
            } catch (error) {
                console.error('Error saving IP:', error);
                Toast.error('IP kaydedilirken hata oluştu');
            }
        },

        async viewIPDetail(id) {
            try {
                const response = await fetch(`/api/ip-blacklist/${id}`, {
                    headers: {
                        'Authorization': `Bearer ${localStorage.getItem('token')}`
                    }
                });

                if (!response.ok) {
                    throw new Error('Failed to get IP detail');
                }

                const data = await response.json();
                this.ipDetail = data.data;
                this.showIPDetailModal = true;
            } catch (error) {
                console.error('Error getting IP detail:', error);
                Toast.error('IP detayları alınırken hata oluştu');
            }
        },

        async deleteIP(id) {
            if (!confirm('Bu IP adresini silmek istediğinizden emin misiniz?')) {
                return;
            }

            try {
                const response = await fetch(`/api/ip-blacklist/${id}`, {
                    method: 'DELETE',
                    headers: {
                        'Authorization': `Bearer ${localStorage.getItem('token')}`
                    }
                });

                if (!response.ok) {
                    throw new Error('Failed to delete IP');
                }

                Toast.success('IP adresi başarıyla silindi');
                await this.loadIPBlacklist();
            } catch (error) {
                console.error('Error deleting IP:', error);
                Toast.error('IP silinirken hata oluştu');
            }
        },

        // Reporting functions
        async loadReportingData() {
            try {
                if (!this.reportForm.date_range.start || !this.reportForm.date_range.end) {
                    this.setDefaultReportDateRange();
                }
                await Promise.all([
                    this.loadReports(),
                    this.loadReportingSummary()
                ]);
            } catch (error) {
                console.error('Error loading reporting data:', error);
                Toast.error('Raporlama verileri yüklenirken hata oluştu');
            }
        },

        async loadReports() {
            try {
                const response = await fetch('/api/reporting/reports?limit=10', {
                    headers: {
                        'Authorization': `Bearer ${localStorage.getItem('token')}`
                    }
                });

                if (!response.ok) {
                    throw new Error('Failed to load reports');
                }

                const data = await response.json();
                this.reports = data.data.reports || [];
                this.renderReportsTable();
            } catch (error) {
                console.error('Error loading reports:', error);
                Toast.error('Raporlar yüklenirken hata oluştu');
            }
        },

        async loadReportingSummary() {
            try {
                const response = await fetch('/api/reporting/reports', {
                    headers: {
                        'Authorization': `Bearer ${localStorage.getItem('token')}`
                    }
                });

                if (!response.ok) {
                    throw new Error('Failed to load reporting summary');
                }

                const data = await response.json();
                const reports = data.data.reports || [];

                this.reportingSummary.totalReports = reports.length;
                this.reportingSummary.completedReports = reports.filter(r => r.status === 'completed').length;
                this.reportingSummary.generatingReports = reports.filter(r => r.status === 'generating').length;
                this.reportingSummary.lastGeneratedAt = reports.length > 0 ? reports[0].created_at : null;
            } catch (error) {
                console.error('Error loading reporting summary:', error);
            }
        },

        renderReportsTable() {
            const tbody = document.getElementById('reports-table-body');
            const mobileList = document.getElementById('reports-mobile-list');

            if (!tbody && !mobileList) return;

            // Desktop tablo render
            if (tbody) {
                if (this.reports.length === 0) {
                    tbody.innerHTML = `
                        <tr>
                            <td colspan="5" class="px-6 py-4 text-center text-gray-500 dark:text-gray-400">
                                Henüz rapor oluşturulmadı
                            </td>
                        </tr>
                    `;
                } else {
                    tbody.innerHTML = this.reports.map(report => `
                        <tr class="hover:bg-gray-50 dark:hover:bg-gray-700">
                            <td class="px-6 py-4 whitespace-nowrap">
                                <div class="text-sm font-medium text-gray-900 dark:text-gray-100">${report.title}</div>
                            </td>
                            <td class="px-6 py-4 whitespace-nowrap text-sm text-gray-500 dark:text-gray-400">
                                ${(report.format || '').toUpperCase()}
                            </td>
                            <td class="px-6 py-4 whitespace-nowrap">
                                <span class="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium ${this.getStatusBadgeClass(report.status)}">
                                    ${this.getStatusLabel(report.status)}
                                </span>
                            </td>
                            <td class="px-6 py-4 whitespace-nowrap text-sm text-gray-500 dark:text-gray-400">
                                ${this.formatReportDate(report.created_at)}
                            </td>
                            <td class="px-6 py-4 whitespace-nowrap text-sm font-medium text-right">
                                <div class="flex justify-end space-x-2">
                                    ${report.status === 'completed' ? `
                                        <button onclick="downloadReport(${report.id})" class="text-primary-600 hover:text-primary-900 dark:text-primary-400 dark:hover:text-primary-300">
                                            İndir
                                        </button>
                                    ` : ''}
                                    <button onclick="deleteReport(${report.id})" class="text-red-600 hover:text-red-900 dark:text-red-400 dark:hover:text-red-300">
                                        Sil
                                    </button>
                                </div>
                            </td>
                        </tr>
                    `).join('');
                }
            }

            // Mobil liste render
            if (mobileList) {
                if (this.reports.length === 0) {
                    mobileList.innerHTML = `
                        <div class="py-8 text-center text-sm text-gray-500 dark:text-gray-400">
                            Henüz rapor oluşturulmadı
                        </div>
                    `;
                } else {
                    mobileList.innerHTML = `
                        <div class="space-y-3">
                            ${this.reports.map(report => `
                                <div class="bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-700 rounded-lg p-4">
                                    <div class="flex items-start justify-between gap-3 mb-3">
                                        <div class="flex-1">
                                            <h3 class="text-sm font-medium text-gray-900 dark:text-gray-100 mb-1">${report.title}</h3>
                                            <p class="text-xs text-gray-500 dark:text-gray-400">${this.formatReportDate(report.created_at)}</p>
                                        </div>
                                        <span class="inline-flex items-center px-2 py-1 rounded-full text-xs font-medium ${this.getStatusBadgeClass(report.status)}">
                                            ${this.getStatusLabel(report.status)}
                                        </span>
                                    </div>
                                    <div class="space-y-2 mb-3">
                                        <div class="flex items-center justify-between text-xs">
                                            <span class="text-gray-500 dark:text-gray-400">Format:</span>
                                            <span class="font-medium text-gray-900 dark:text-gray-100">${(report.format || '').toUpperCase()}</span>
                                        </div>
                                    </div>
                                    <div class="flex items-center justify-end gap-3 pt-3 border-t border-gray-100 dark:border-gray-700">
                                        ${report.status === 'completed' ? `
                                            <button onclick="downloadReport(${report.id})" class="text-xs font-medium text-primary-600 hover:text-primary-700 dark:text-primary-400 dark:hover:text-primary-300">
                                                İndir
                                            </button>
                                        ` : ''}
                                        <button onclick="deleteReport(${report.id})" class="text-xs font-medium text-red-600 hover:text-red-700 dark:text-red-400 dark:hover:text-red-300">
                                            Sil
                                        </button>
                                    </div>
                                </div>
                            `).join('')}
                        </div>
                    `;
                }
            }
        },

        getStatusBadgeClass(status) {
            const classes = {
                'generating': 'bg-yellow-100 text-yellow-800 dark:bg-yellow-900 dark:text-yellow-200',
                'completed': 'bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-200',
                'failed': 'bg-red-100 text-red-800 dark:bg-red-900 dark:text-red-200'
            };
            return classes[status] || 'bg-gray-100 text-gray-800 dark:bg-gray-900 dark:text-gray-200';
        },

        getStatusLabel(status) {
            const labels = {
                'generating': 'Hazırlanıyor',
                'completed': 'Tamamlandı',
                'failed': 'Başarısız'
            };
            return labels[status] || status;
        },

        formatReportDate(value) {
            if (!value) {
                return '-';
            }
            const date = new Date(value);
            if (Number.isNaN(date.getTime())) {
                return value;
            }
            return date.toLocaleDateString('tr-TR');
        },

        setDefaultReportDateRange() {
            const end = new Date();
            const start = new Date();
            start.setDate(end.getDate() - 29);
            this.reportForm.date_range.start = start.toISOString().slice(0, 10);
            this.reportForm.date_range.end = end.toISOString().slice(0, 10);
        },

        validateReportDateRange(showErrors = true) {
            const { start, end } = this.reportForm.date_range;
            if (!start || !end) {
                if (showErrors) {
                    Toast.warning('Lütfen tarih aralığı seçin');
                }
                return false;
            }

            const startDate = new Date(start);
            const endDate = new Date(end);
            if (startDate > endDate) {
                if (showErrors) {
                    Toast.warning('Başlangıç tarihi bitişten büyük olamaz');
                }
                return false;
            }
            return true;
        },

        async openReportPreview() {
            if (!this.validateReportDateRange()) {
                return;
            }
            this.showReportPreviewModal = true;
            await this.refreshReportPreview(false);
        },

        closeReportPreview() {
            this.showReportPreviewModal = false;
        },

        async refreshReportPreview(showErrors = true) {
            if (!this.validateReportDateRange(showErrors)) {
                return;
            }

            this.reportPreview.loading = true;
            this.reportPreview.error = null;

            try {
                const params = new URLSearchParams({
                    start: this.reportForm.date_range.start,
                    end: this.reportForm.date_range.end
                });

                const response = await fetch(`/api/reporting/data?${params.toString()}`, {
                    headers: {
                        'Authorization': `Bearer ${localStorage.getItem('token')}`
                    }
                });

                if (!response.ok) {
                    throw new Error('Failed to load report preview');
                }

                const data = await response.json();
                this.reportPreview.data = data.data || null;
            } catch (error) {
                console.error('Error loading report preview:', error);
                this.reportPreview.error = 'Ön izleme alınmada başarısız';
                this.reportPreview.data = null;
                if (showErrors) {
                    Toast.error('Ön izleme alınırken hata oluştu');
                }
            } finally {
                this.reportPreview.loading = false;
            }
        },

        resetReportForm() {
            this.reportForm.format = 'pdf';
            this.reportForm.targets = [];
            this.setDefaultReportDateRange();
        },

        getPreviewRangeLabel() {
            if (!this.reportPreview.data || !this.reportPreview.data.date_range) {
                return '';
            }
            const start = this.reportPreview.data.date_range.start;
            const end = this.reportPreview.data.date_range.end;
            return `${this.formatReportDate(start)} - ${this.formatReportDate(end)}`;
        },

        // Rapor hazırlanırken ekranı bloklayan ilerleme modalı gösterir.
        // Geçen süreyi sayar ve İptal ile isteği durdurmayı sağlar.
        _showReportProgress(onCancel) {
            const existing = document.getElementById('report-progress-overlay');
            if (existing) existing.remove();

            const overlay = document.createElement('div');
            overlay.id = 'report-progress-overlay';
            overlay.className = 'fixed inset-0 z-[9999] flex items-center justify-center bg-slate-900/60 backdrop-blur-sm p-4';
            overlay.innerHTML = `
                <div class="w-full max-w-md rounded-2xl bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-700 shadow-2xl p-6">
                    <div class="flex items-start gap-4">
                        <svg class="animate-spin h-6 w-6 text-primary-600 dark:text-primary-400 shrink-0" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
                            <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                            <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8v4a4 4 0 00-4 4H4z"></path>
                        </svg>
                        <div class="min-w-0 flex-1">
                            <h3 class="text-base font-semibold text-gray-900 dark:text-gray-100">Rapor hazırlanıyor</h3>
                            <p class="text-sm text-gray-600 dark:text-gray-300 mt-1">
                                Seçilen tarih aralığındaki veriler işleniyor. Aralık genişse bu işlem biraz sürebilir.
                            </p>
                            <p class="text-xs text-gray-500 dark:text-gray-400 mt-2">
                                Geçen süre: <span id="report-progress-elapsed">0</span> sn
                            </p>
                        </div>
                    </div>
                    <div class="mt-5 flex justify-end">
                        <button id="report-progress-cancel"
                                class="px-4 py-2 rounded-lg text-sm font-medium text-gray-700 dark:text-gray-200 bg-gray-100 dark:bg-gray-700 hover:bg-gray-200 dark:hover:bg-gray-600 transition-colors">
                            İptal
                        </button>
                    </div>
                </div>
            `;
            document.body.appendChild(overlay);
            document.body.classList.add('overflow-hidden');

            const startedAt = Date.now();
            const elapsedEl = overlay.querySelector('#report-progress-elapsed');
            const timer = setInterval(() => {
                if (elapsedEl) elapsedEl.textContent = String(Math.floor((Date.now() - startedAt) / 1000));
            }, 1000);

            const cancelBtn = overlay.querySelector('#report-progress-cancel');
            if (cancelBtn && typeof onCancel === 'function') {
                cancelBtn.addEventListener('click', () => {
                    cancelBtn.disabled = true;
                    cancelBtn.textContent = 'İptal ediliyor...';
                    onCancel();
                });
            }

            return {
                close() {
                    clearInterval(timer);
                    overlay.remove();
                    document.body.classList.remove('overflow-hidden');
                }
            };
        },

        async downloadReport(id) {
            const controller = new AbortController();
            const progress = this._showReportProgress(() => controller.abort());

            try {
                const response = await fetch(`/api/reporting/reports/${id}/download`, {
                    headers: {
                        'Authorization': `Bearer ${localStorage.getItem('token')}`
                    },
                    signal: controller.signal
                });

                if (!response.ok) {
                    // Hata mesajını parse et
                    const contentType = response.headers.get('Content-Type');
                    if (contentType && contentType.includes('application/json')) {
                        const errorData = await response.json();
                        Toast.error(errorData.message || errorData.error || 'Rapor indirilirken hata oluştu');
                        return;
                    }
                    throw new Error('Failed to download report');
                }

                const contentType = response.headers.get('Content-Type');

                // PDF için HTML olarak geliyorsa, yeni sekmede aç (print dialog otomatik açılacak)
                if (contentType && contentType.includes('text/html')) {
                    const html = await response.text();
                    const newWindow = window.open('', '_blank');
                    if (newWindow) {
                        newWindow.document.write(html);
                        newWindow.document.close();
                        Toast.success('Rapor yeni sekmede açıldı. PDF olarak kaydetmek için yazdırma penceresini kullanın.');
                    } else {
                        // Pop-up engellendiyse rapor kaybolmasın: dosya olarak indir.
                        this._saveBlob(new Blob([html], { type: 'text/html;charset=utf-8' }), `report_${id}.html`);
                        Toast.warning('Pop-up engellendiği için rapor HTML dosyası olarak indirildi. Dosyayı açıp yazdırarak PDF alabilirsiniz.');
                    }
                    return;
                }

                // Diğer formatlar için (CSV, HTML dosyası) normal indirme
                const contentDisposition = response.headers.get('Content-Disposition');
                let filename = `report_${id}`;
                if (contentDisposition) {
                    const match = contentDisposition.match(/filename="?([^";\s]+)"?/);
                    if (match) {
                        filename = match[1].trim();
                    }
                }

                this._saveBlob(await response.blob(), filename);
                Toast.success('Rapor başarıyla indirildi');
            } catch (error) {
                if (error && error.name === 'AbortError') {
                    Toast.warning('Rapor indirme iptal edildi');
                    return;
                }
                console.error('Error downloading report:', error);
                Toast.error('Rapor indirilirken hata oluştu');
            } finally {
                progress.close();
            }
        },

        _saveBlob(blob, filename) {
            const url = window.URL.createObjectURL(blob);
            const a = document.createElement('a');
            a.href = url;
            a.download = filename;
            document.body.appendChild(a);
            a.click();
            document.body.removeChild(a);
            window.URL.revokeObjectURL(url);
        },

        async deleteReport(id) {
            if (!confirm('Bu raporu silmek istediğinizden emin misiniz?')) {
                return;
            }

            try {
                const response = await fetch(`/api/reporting/reports/${id}`, {
                    method: 'DELETE',
                    headers: {
                        'Authorization': `Bearer ${localStorage.getItem('token')}`
                    }
                });

                if (!response.ok) {
                    throw new Error('Failed to delete report');
                }

                Toast.success('Rapor başarıyla silindi');
                await this.loadReports();
                await this.loadReportingSummary();
            } catch (error) {
                console.error('Error deleting report:', error);
                Toast.error('Rapor silinirken hata oluştu');
            }
        },

        async submitReportGeneration() {
            if (!this.validateReportDateRange()) {
                return;
            }

            const payload = {
                type: this.reportForm.type || 'system_overview',
                format: this.reportForm.format,
                date_range: {
                    start: this.reportForm.date_range.start,
                    end: this.reportForm.date_range.end
                },
                targets: this.reportForm.targets || []
            };

            this.generatingReport = true;
            try {
                const response = await fetch('/api/reporting/generate', {
                    method: 'POST',
                    headers: {
                        'Content-Type': 'application/json',
                        'Authorization': `Bearer ${localStorage.getItem('token')}`
                    },
                    body: JSON.stringify(payload)
                });

                if (!response.ok) {
                    throw new Error('Failed to generate report');
                }

                Toast.success('Rapor başarıyla oluşturuldu');
                await this.loadReports();
                await this.loadReportingSummary();
            } catch (error) {
                console.error('Error generating report:', error);
                Toast.error('Rapor oluşturulurken bir hata oluştu');
            } finally {
                this.generatingReport = false;
            }
        },    };
}


// Global functions
window.openTargetModal = () => window.app.openTargetModal();
window.closeTargetModal = () => window.app.closeTargetModal();
window.editTarget = (targetId) => window.app.editTarget(targetId);
window.deleteTarget = (targetId) => window.app.deleteTarget(targetId);

// ── Bildirim "Test Gönder" butonları ─────────────────────────────────────────
// SPA, sayfa partial'lerini innerHTML ile enjekte ediyor; bu durumda partial'in
// kendi <script>'i ÇALIŞMAZ. Bu yüzden bildirim ayarları sayfasındaki test
// butonlarını her zaman yüklenen app.js içinden, document seviyesinde event
// delegation ile yakalıyoruz (yükleniyor + kilit + cooldown ile).
(function () {
  const COOLDOWN_MS = 6000;
  async function runNotifTest(button, channelSelId, recipientsInputId, resultElId) {
    const resultEl = document.getElementById(resultElId);
    const setMsg = (t, cls) => { if (resultEl) { resultEl.textContent = t; resultEl.className = 'text-xs ' + cls; } };
    if (button.dataset.busy === '1') return;
    const last = Number(button.dataset.lastTest || 0);
    const waitMs = COOLDOWN_MS - (Date.now() - last);
    if (last && waitMs > 0) { setMsg('Çok sık denediniz. ' + Math.ceil(waitMs / 1000) + ' sn sonra tekrar deneyin.', 'text-amber-600 dark:text-amber-400'); return; }
    const channel = (document.getElementById(channelSelId)?.value || 'email').trim();
    const raw = (document.getElementById(recipientsInputId)?.value || '').trim();
    const first = raw.split(',').map(s => s.trim()).filter(Boolean)[0];
    if (!first) { setMsg(channel === 'telegram' ? 'Önce bir chat ID girin.' : 'Önce bir alıcı e-posta girin.', 'text-amber-600 dark:text-amber-400'); return; }
    button.dataset.busy = '1'; button.disabled = true; button.dataset.orig = button.innerHTML;
    button.innerHTML = '<i class="fas fa-spinner fa-spin text-[11px]"></i> Gönderiliyor…';
    setMsg('Gönderiliyor…', 'text-gray-500 dark:text-gray-400');
    try {
      const url = channel === 'telegram' ? '/api/new-notifications/test-telegram' : '/api/new-notifications/test-email';
      const body = channel === 'telegram' ? { chat_id: first } : { recipient: first };
      const res = await fetch(url, { method: 'POST', headers: { 'Content-Type': 'application/json', 'Authorization': 'Bearer ' + (localStorage.getItem('token') || '') }, body: JSON.stringify(body) });
      const data = await res.json().catch(() => ({}));
      if (res.ok && data.success !== false) {
        setMsg(channel === 'telegram' ? '✓ Test bildirimi gönderildi — Telegram\'ı kontrol edin.' : '✓ Test bildirimi gönderildi — gelen kutunuzu kontrol edin.', 'text-green-600 dark:text-green-400');
      } else {
        setMsg('✗ ' + (data.error || 'Test gönderilemedi'), 'text-red-600 dark:text-red-400');
      }
    } catch (e) { setMsg('✗ İstek başarısız: ' + e.message, 'text-red-600 dark:text-red-400'); }
    finally { button.dataset.busy = '0'; button.dataset.lastTest = String(Date.now()); button.disabled = false; if (button.dataset.orig) button.innerHTML = button.dataset.orig; }
  }
  document.addEventListener('click', function (e) {
    if (!e.target || !e.target.closest) return;
    const c = e.target.closest('#btn-test-create-recipients');
    if (c) { e.preventDefault(); runNotifTest(c, 'rule-channel', 'rule-recipients', 'test-rule-result'); return; }
    const ed = e.target.closest('#btn-test-edit-recipients');
    if (ed) { e.preventDefault(); runNotifTest(ed, 'edit-rule-channel', 'edit-rule-recipients', 'test-edit-result'); return; }
  });
})();
window.pingTarget = (targetId) => window.app.pingTarget(targetId);
window.showTargetHistory = (targetId) => window.app.showTargetHistory(targetId);
window.closeTargetHistory = () => window.app.closeTargetHistory();
window.changeHistoryPeriod = (period) => window.app.changeHistoryPeriod(period);
window.toggleMetricsFields = (forceState = null) => {
    if (window.app && typeof window.app.toggleMetricsFields === 'function') {
        window.app.toggleMetricsFields(forceState);
    }
};

// IP Blacklist global functions
window.checkIP = () => window.app.checkIP();
window.saveIP = () => window.app.saveIP();
window.viewIPDetail = (id) => window.app.viewIPDetail(id);
window.deleteIP = (id) => window.app.deleteIP(id);

// Reporting global functions
window.downloadReport = (id) => window.app.downloadReport(id);
window.deleteReport = (id) => window.app.deleteReport(id);

// SLA Reports global functions
// Load targets for SLA filter
window.loadTargetsForFilter = async () => {
    try {
        const response = await fetch('/api/targets', {
            headers: {
                'Authorization': `Bearer ${localStorage.getItem('token')}`
            }
        });
        
        if (!response.ok) {
            throw new Error('Failed to load targets');
        }
        
        const data = await response.json();
        const targets = data.targets || [];
        
        const targetFilter = document.getElementById('targetFilter');
        if (targetFilter) {
            // Clear existing options except "all"
            targetFilter.innerHTML = '<option value="all">Tüm Target\'lar</option>';
            
            targets.forEach(target => {
                const option = document.createElement('option');
                option.value = target.id;
                option.textContent = target.name;
                targetFilter.appendChild(option);
            });
        }
        
    } catch (error) {
        console.error('Error loading targets for filter:', error);
    }
};

// Notification logs Alpine.js component
function notificationLogsApp() {
    return {
        loading: false,
        error: null,
        logs: [],
        summary: {
            total: 0,
            sent: 0,
            failed: 0,
            email: 0,
            telegram: 0,
            last_sent_at: null
        },
        filters: {
            search: '',
            channel: '',
            status: '',
            start: '',
            end: '',
            limit: '50'
        },
        pagination: {
            total: 0,
            limit: 50,
            offset: 0,
            has_more: false
        },
        selectedLog: null,
        showDetails: false,

        init() {
            this.fetchLogs();
        },

        async fetchLogs() {
            this.loading = true;
            this.error = null;

            const limitValue = parseInt(this.filters.limit, 10) || 50;
            this.pagination.limit = limitValue;

            const params = new URLSearchParams();
            params.set('limit', limitValue);
            params.set('offset', this.pagination.offset);

            if (this.filters.channel) params.set('channel', this.filters.channel);
            if (this.filters.status) params.set('status', this.filters.status);
            if (this.filters.search) params.set('search', this.filters.search.trim());
            if (this.filters.start) params.set('start', this.filters.start);
            if (this.filters.end) params.set('end', this.filters.end);

            const headers = {};
            const token = localStorage.getItem('token');
            if (token) {
                headers.Authorization = `Bearer ${token}`;
            }

            try {
                const response = await fetch(`/api/new-notifications/logs?${params.toString()}`, { headers });
                const result = await response.json();

                if (!response.ok || !result.success) {
                    throw new Error(result.error || result.message || 'Kayıtlar yüklenemedi');
                }

                this.logs = Array.isArray(result.data?.logs) ? result.data.logs : [];
                this.summary = Object.assign({
                    total: 0,
                    sent: 0,
                    failed: 0,
                    email: 0,
                    telegram: 0,
                    last_sent_at: null
                }, result.data?.summary || {});

                const pagination = Object.assign({}, this.pagination, result.data?.pagination || {});
                this.pagination.total = pagination.total ?? this.pagination.total;
                this.pagination.limit = pagination.limit ?? limitValue;
                this.pagination.offset = pagination.offset ?? this.pagination.offset;
                this.pagination.has_more = Boolean(pagination.has_more);
                this.filters.limit = String(this.pagination.limit);
            } catch (err) {
                console.error('notificationLogsApp fetch error:', err);
                this.error = err.message || 'Beklenmeyen bir hata oluştu';
            } finally {
                this.loading = false;
            }
        },

        applyFilters() {
            this.pagination.offset = 0;
            this.fetchLogs();
        },

        resetFilters() {
            this.filters = {
                search: '',
                channel: '',
                status: '',
                start: '',
                end: '',
                limit: '50'
            };
            this.pagination.offset = 0;
            this.fetchLogs();
        },

        updateLimit(value) {
            this.filters.limit = value;
            this.pagination.limit = parseInt(value, 10) || 50;
            this.pagination.offset = 0;
            this.fetchLogs();
        },

        prevPage() {
            if (this.pagination.offset >= this.pagination.limit) {
                this.pagination.offset -= this.pagination.limit;
                this.fetchLogs();
            }
        },

        nextPage() {
            if (this.pagination.has_more) {
                this.pagination.offset += this.pagination.limit;
                this.fetchLogs();
            }
        },

        currentPage() {
            if (!this.pagination.limit) return 1;
            return Math.floor(this.pagination.offset / this.pagination.limit) + 1;
        },

        totalPages() {
            if (!this.pagination.limit) return 1;
            return Math.max(1, Math.ceil((this.pagination.total || 0) / this.pagination.limit));
        },

        successRate() {
            if (!this.summary.total) return 0;
            return Math.round((this.summary.sent / this.summary.total) * 100);
        },

        formatDate(value) {
            return formatTurkishTime(value);
        },

        channelLabel(channel) {
            const labels = { email: 'Email', telegram: 'Telegram' };
            return labels[channel] || (channel || '-');
        },

        channelIcon(channel) {
            const icons = {
                email: 'fas fa-envelope',
                telegram: 'fas fa-paper-plane'
            };
            return icons[channel] || 'fas fa-question-circle';
        },

        channelBadgeClasses(channel) {
            const badges = {
                email: 'bg-blue-50 dark:bg-blue-500/10 text-blue-600 dark:text-blue-200 border-blue-100 dark:border-blue-400/40',
                telegram: 'bg-cyan-50 dark:bg-cyan-500/10 text-cyan-600 dark:text-cyan-200 border-cyan-100 dark:border-cyan-400/40'
            };
            return badges[channel] || 'bg-gray-100 dark:bg-gray-800 text-gray-600 dark:text-gray-300 border-gray-200 dark:border-gray-600';
        },

        channelTextColor(channel) {
            const colors = {
                email: 'text-blue-600 dark:text-blue-300',
                telegram: 'text-cyan-600 dark:text-cyan-300'
            };
            return colors[channel] || 'text-gray-700 dark:text-gray-200';
        },

        statusLabel(status) {
            const labels = { sent: 'Gönderildi', failed: 'Başarısız' };
            return labels[status] || (status || '-');
        },

        statusIcon(status) {
            const icons = {
                sent: 'fas fa-check-circle',
                failed: 'fas fa-times-circle'
            };
            return icons[status] || 'fas fa-circle-info';
        },

        statusBadgeClasses(status) {
            const badges = {
                sent: 'bg-emerald-50 dark:bg-emerald-500/10 text-emerald-600 dark:text-emerald-300 border-emerald-100 dark:border-emerald-400/40',
                failed: 'bg-red-50 dark:bg-red-500/10 text-red-600 dark:text-red-300 border-red-100 dark:border-red-400/40'
            };
            return badges[status] || 'bg-gray-100 dark:bg-gray-800 text-gray-600 dark:text-gray-300 border-gray-200 dark:border-gray-600';
        },

        preview(text, length = 120) {
            if (!text) return '-';
            if (text.length <= length) return text;
            return text.slice(0, length) + '...';
        },

        openDetails(log) {
            // Dispatch event for modal outside Alpine scope
            window.dispatchEvent(new CustomEvent('open-notification-modal', { detail: log }));
        },

        closeDetails() {
            this.showDetails = false;
            this.selectedLog = null;
            document.body.classList.remove('overflow-hidden');
        }
    };
}

// SLA Reports initialization function
window.loadSLAReportsData = () => {
    // Load summary cards
    window.loadSLASummaryCards();
    // Load targets for filter
    window.loadTargetsForFilter();
    // Load SLA reports table
    setTimeout(() => {
        if (window.app && window.app.loadSLAReportsTable) {
            window.app.loadSLAReportsTable();
        }
    }, 200);
};

window.applySLAFilters = () => {
    const dateRange = document.getElementById('dateRange')?.value;
    const slaThreshold = document.getElementById('slaThreshold')?.value;
    const targetFilter = document.getElementById('targetFilter')?.value;
    
    // SLA Threshold validation
    if (!slaThreshold || isNaN(slaThreshold)) {
        Toast.show({
            type: 'error',
            title: 'Geçersiz Threshold',
            message: 'SLA Threshold geçerli bir sayı olmalıdır (0.1-100.0 arası).',
            timeout: 5000
        });
        return;
    }
    
    const threshold = parseFloat(slaThreshold);
    if (threshold < 0.1 || threshold > 100.0) {
        Toast.show({
            type: 'error',
            title: 'Geçersiz Threshold',
            message: 'SLA Threshold 0.1 ile 100.0 arasında olmalıdır.',
            timeout: 5000
        });
        return;
    }
    
    console.log('Applying SLA filters:', { dateRange, slaThreshold: threshold, targetFilter });
    
    // Reload SLA summary cards with filters
    window.loadSLASummaryCards();
    // Reload SLA reports table with filters
    if (window.app && window.app.loadSLAReportsTable) {
        window.app.loadSLAReportsTable();
    }
    
    Toast.show({
        type: 'success',
        title: 'Filtreler Uygulandı',
        message: `SLA raporları güncellendi (Threshold: ${threshold}%).`,
        timeout: 3000
    });
};

// SLA summary cards function
window.loadSLASummaryCards = async () => {
    try {
        // Get current filter values
        const slaThreshold = document.getElementById('slaThreshold')?.value || '99.9';
        const targetFilter = document.getElementById('targetFilter')?.value || 'all';
        
        const response = await fetch('/api/analytics/sla/summary', {
            headers: {
                'Authorization': `Bearer ${localStorage.getItem('token')}`
            }
        });
        
        if (response.ok) {
            const result = await response.json();
            const data = result.data || {};
            const summary = data.summary || [];
            
            // Filter data based on current filters
            let filteredSummary = summary;
            if (targetFilter !== 'all') {
                filteredSummary = summary.filter(item => item.target_id && item.target_id.toString() === targetFilter);
            }
            
            const threshold = parseFloat(slaThreshold);
            
            // Calculate filtered statistics
            const totalTargets = filteredSummary.length;
            let compliantCount = 0;
            let totalUptime = 0;
            
            filteredSummary.forEach(item => {
                if (item.current_uptime >= threshold) {
                    compliantCount++;
                }
                totalUptime += item.current_uptime || 0;
            });
            
            const violatedCount = totalTargets - compliantCount;
            const avgUptime = totalTargets > 0 ? totalUptime / totalTargets : 0;
            
            // Update summary cards with calculated data
            const totalTargetsEl = document.getElementById('total-targets');
            const compliantEl = document.getElementById('sla-compliant');
            const violatedEl = document.getElementById('sla-breached');
            const avgUptimeEl = document.getElementById('avg-uptime');
            
            if (totalTargetsEl) totalTargetsEl.textContent = totalTargets;
            if (compliantEl) compliantEl.textContent = compliantCount;
            if (violatedEl) violatedEl.textContent = violatedCount;
            if (avgUptimeEl) avgUptimeEl.textContent = avgUptime.toFixed(1) + '%';
        }
    } catch (error) {
        console.error('Error loading SLA summary cards:', error);
    }
};

// SLA Reports Export functions
window.exportSLAReport = (format) => {
    console.log('Exporting SLA report as:', format);
    
    const dateRange = document.getElementById('dateRange')?.value || '30';
    const slaThresholdValue = document.getElementById('slaThreshold')?.value || '95.0';
    const targetFilter = document.getElementById('targetFilter')?.value || 'all';
    
    // Validate threshold before export
    const slaThreshold = parseFloat(slaThresholdValue) || 95.0;
    if (slaThreshold < 0.1 || slaThreshold > 100.0) {
        Toast.show({
            type: 'error',
            title: 'Export Hatası',
            message: 'SLA Threshold geçerli bir değer olmalı (0.1-100.0 arası).',
            timeout: 5000
        });
        return;
    }
    
    // Export URL
    const exportUrl = `/api/analytics/sla/export?format=${format}&days=${dateRange}&threshold=${slaThreshold}&target=${targetFilter}`;
    
    Toast.show({
        type: 'info',
        title: 'Export Başlatıldı',
        message: `${format.toUpperCase()} formatında rapor hazırlanıyor...`,
        timeout: 3000
    });
    
    // Download file with authentication using fetch
    const token = localStorage.getItem('token');
    
    fetch(exportUrl, {
        method: 'GET',
        headers: {
            'Authorization': `Bearer ${token}`,
            'Content-Type': 'application/json'
        }
    })
    .then(response => {
        if (!response.ok) {
            throw new Error(`HTTP error! status: ${response.status}`);
        }
        return response.blob();
    })
    .then(blob => {
        // Create download link
        const url = window.URL.createObjectURL(blob);
        const fileName = `sla-report-${format}-${new Date().toISOString().split('T')[0]}.${format}`;
        const link = document.createElement('a');
        link.href = url;
        link.download = fileName;
        document.body.appendChild(link);
        link.click();
        document.body.removeChild(link);
        window.URL.revokeObjectURL(url);
    })
    .catch(error => {
        console.error('Export error:', error);
        Toast.show({
            type: 'error',
            title: 'Export Hatası',
            message: 'Rapor indirilemedi. Lütfen tekrar deneyin.',
            timeout: 5000
        });
    });
    
    setTimeout(() => {
        Toast.show({
            type: 'success',
            title: 'Export Tamamlandı',
            message: `${format.toUpperCase()} raporu indirildi.`,
            timeout: 3000
        });
    }, 2000);
};

// Email notification functions
window.loadNotificationSettings = () => {
    fetch('/api/notifications/config', {
        method: 'GET',
        headers: {
            'Authorization': 'Bearer ' + localStorage.getItem('token')
        }
    })
    .then(response => response.json())
    .then(data => {
        console.log('Loaded config:', data);
        if (data.data && data.data.email) {
            const emailConfig = data.data.email;
            
            // Form alanlarını doldur
            const smtpHost = document.getElementById('smtpHost');
            const smtpPort = document.getElementById('smtpPort');
            const username = document.getElementById('username');
            const password = document.getElementById('password');
            const fromEmail = document.getElementById('fromEmail');
            const fromName = document.getElementById('fromName');
            const notificationEmail = document.getElementById('notificationEmail');
            const useTLS = document.getElementById('useTLS');
            
            if (smtpHost) smtpHost.value = emailConfig.smtp_host || 'smtp.gmail.com';
            if (smtpPort) smtpPort.value = emailConfig.smtp_port || '587';
            if (username) username.value = emailConfig.username || '';
            if (password) password.value = emailConfig.password || '';
            if (fromEmail) fromEmail.value = emailConfig.from_email || '';
            if (fromName) fromName.value = emailConfig.from_name || 'SysTrack';
            if (notificationEmail) notificationEmail.value = emailConfig.notification_email || '';
            if (useTLS) useTLS.checked = emailConfig.use_tls !== false;
        } else {
            // Varsayılan değerleri ayarla
            const smtpHost = document.getElementById('smtpHost');
            const smtpPort = document.getElementById('smtpPort');
            const useTLS = document.getElementById('useTLS');
            
            if (smtpHost) smtpHost.value = 'smtp.gmail.com';
            if (smtpPort) smtpPort.value = '587';
            if (useTLS) useTLS.checked = true;
        }
    })
    .catch(error => {
        console.error('Error loading settings:', error);
        // Varsayılan değerleri ayarla
        const smtpHost = document.getElementById('smtpHost');
        const smtpPort = document.getElementById('smtpPort');
        const useTLS = document.getElementById('useTLS');
        
        if (smtpHost) smtpHost.value = 'smtp.gmail.com';
        if (smtpPort) smtpPort.value = '587';
        if (useTLS) useTLS.checked = true;
    });
};

window.saveEmailSettings = () => {
    const smtpHost = document.getElementById('smtpHost');
    const smtpPort = document.getElementById('smtpPort');
    const username = document.getElementById('username');
    const password = document.getElementById('password');
    const fromEmail = document.getElementById('fromEmail');
    const fromName = document.getElementById('fromName');
    const notificationEmail = document.getElementById('notificationEmail');
    const useTLS = document.getElementById('useTLS');
    
    if (!smtpHost || !smtpPort || !username || !password || !fromEmail || !notificationEmail) {
        Toast.show({
            type: 'warning',
            title: 'Uyarı',
            message: 'Lütfen tüm gerekli alanları doldurun!',
            timeout: 3000
        });
        return;
    }
    
    const settings = {
        host: smtpHost.value,
        port: smtpPort.value,
        username: username.value,
        password: password.value,
        fromEmail: fromEmail.value,
        fromName: fromName.value,
        notificationEmail: notificationEmail.value,
        useTLS: useTLS.checked
    };

    // Validation
    if (!settings.host || !settings.port || !settings.username || !settings.password || !settings.fromEmail || !settings.notificationEmail) {
        Toast.show({
            type: 'warning',
            title: 'Uyarı',
            message: 'Lütfen tüm gerekli alanları doldurun!',
            timeout: 3000
        });
        return;
    }

    const port = parseInt(settings.port);
    if (isNaN(port) || port <= 0 || port > 65535) {
        Toast.show({
            type: 'warning',
            title: 'Uyarı',
            message: 'Geçerli bir port numarası girin!',
            timeout: 3000
        });
        return;
    }

    fetch('/api/notifications/config', {
        method: 'POST',
        headers: {
            'Content-Type': 'application/json',
            'Authorization': 'Bearer ' + localStorage.getItem('token')
        },
        body: JSON.stringify({
            email: {
                enabled: true,
                smtp_host: settings.host,
                smtp_port: parseInt(settings.port),
                username: settings.username,
                password: settings.password,
                from_email: settings.fromEmail,
                from_name: settings.fromName,
                use_tls: settings.useTLS,
                notification_email: settings.notificationEmail
            },
            telegram: { enabled: false, bot_token: '', default_chat: '', webhook_url: '' },
            whatsapp: { enabled: false, access_token: '', phone_id: '', webhook_url: '' },
            webhook: { enabled: false, url: '', secret: '' }
        })
    })
    .then(response => response.json())
    .then(data => {
        console.log('Config response:', data);
        if (data.success) {
            Toast.show({
                type: 'success',
                title: 'Başarılı',
                message: 'Email ayarları kaydedildi!',
                timeout: 3000
            });
        } else {
            Toast.show({
                type: 'error',
                title: 'Hata',
                message: data.error || 'Bilinmeyen hata',
                timeout: 5000
            });
        }
    })
    .catch(error => {
        console.error('Error:', error);
        Toast.show({
            type: 'error',
            title: 'Hata',
            message: error.message,
            timeout: 5000
        });
    });
};

// -------------------------
// Backup page helpers
// -------------------------
(function () {
    const DEFAULT_LOADING_TEXT = 'Yükleniyor...';
    let selectedFile = null;

    function toggleLoading(show, message) {
        const overlay = document.getElementById('loading-overlay');
        if (!overlay) { return; }
        const label = overlay.querySelector('span');
        if (show) {
            if (label && message) {
                label.textContent = message;
            }
            overlay.classList.remove('hidden');
        } else {
            if (label) {
                label.textContent = DEFAULT_LOADING_TEXT;
            }
            overlay.classList.add('hidden');
        }
    }

    function showToast(type, message, title = '') {
        if (window.Toast && typeof window.Toast.show === 'function') {
            Toast.show({ type, title, message });
        } else {
            console[type === 'error' ? 'error' : 'log'](message);
        }
    }

    async function exportBackup() {
        toggleLoading(true, 'Yedekleme dosyası hazırlanıyor...');

        try {
            const response = await fetch('/api/backup/export', {
                method: 'GET',
                headers: {
                    'Authorization': 'Bearer ' + localStorage.getItem('token')
                }
            });

            if (!response.ok) {
                throw new Error('Yedekleme indirilemedi');
            }

            const blob = await response.blob();
            const url = window.URL.createObjectURL(blob);
            const anchor = document.createElement('a');
            anchor.href = url;
            anchor.download = `systrack_backup_${new Date().toISOString().slice(0, 19).replace(/:/g, '-')}.json`;
            document.body.appendChild(anchor);
            anchor.click();
            window.URL.revokeObjectURL(url);
            anchor.remove();

            showToast('success', 'Yedekleme dosyası başarıyla indirildi.', 'İşlem tamamlandı');
            loadBackupHistory();
        } catch (error) {
            console.error('Backup export error:', error);
            showToast('error', error.message || 'Yedekleme indirilemedi.');
        } finally {
            toggleLoading(false);
        }
    }

    async function importBackup() {
        if (!selectedFile) {
            showToast('warning', 'Lütfen önce bir JSON yedeği seçin.');
            return;
        }

        toggleLoading(true, 'Yedekleme dosyası yükleniyor...');

        try {
            const formData = new FormData();
            formData.append('file', selectedFile);

            const response = await fetch('/api/backup/import', {
                method: 'POST',
                headers: {
                    'Authorization': 'Bearer ' + localStorage.getItem('token')
                },
                body: formData
            });

            const result = await response.json().catch(() => ({}));

            if (!response.ok) {
                throw new Error(result.error || 'Yedekleme yüklenemedi');
            }

            const info = result?.data || {};
            showToast(
                'success',
                `Hedefler: ${info.targets_count ?? 0}, Bildirim Kuralları: ${info.notification_rules_count ?? 0}`,
                'Yedekleme başarıyla yüklendi'
            );

            resetFileSelection();
            loadBackupHistory();
        } catch (error) {
            console.error('Backup import error:', error);
            showToast('error', error.message || 'Yedekleme yüklenemedi.');
        } finally {
            toggleLoading(false);
        }
    }

    function resetFileSelection() {
        const fileInput = document.getElementById('backup-file-input');
        const info = document.getElementById('backup-selected-file');
        const importBtn = document.getElementById('backup-import-button');

        selectedFile = null;
        if (fileInput) {
            fileInput.value = '';
        }
        if (info) {
            info.classList.add('hidden');
            info.textContent = '';
        }
        if (importBtn) {
            importBtn.disabled = true;
            importBtn.classList.add('bg-gray-300', 'dark:bg-gray-700', 'text-gray-500', 'dark:text-gray-400', 'cursor-not-allowed');
            importBtn.classList.remove('bg-emerald-600', 'hover:bg-emerald-700', 'text-white', 'cursor-pointer');
        }
    }

    function handleFileChange(event) {
        const file = event.target.files?.[0];
        const info = document.getElementById('backup-selected-file');
        const importBtn = document.getElementById('backup-import-button');

        if (file) {
            selectedFile = file;
            if (info) {
                info.textContent = `Seçilen dosya: ${file.name}`;
                info.classList.remove('hidden');
            }
            if (importBtn) {
                importBtn.disabled = false;
                importBtn.classList.remove('bg-gray-300', 'dark:bg-gray-700', 'text-gray-500', 'dark:text-gray-400', 'cursor-not-allowed');
                importBtn.classList.add('bg-emerald-600', 'hover:bg-emerald-700', 'text-white', 'cursor-pointer');
            }
        } else {
            resetFileSelection();
        }
    }

    async function loadBackupHistory() {
        const history = document.getElementById('backup-history');
        const historyMobile = document.getElementById('backup-history-mobile');
        if (!history && !historyMobile) { return; }

        const loadingHtml = `
      <div class="flex items-center justify-center gap-3 py-10 text-sm text-gray-500 dark:text-gray-400">
        <span class="h-3 w-3 rounded-full border-2 border-primary-200 border-t-primary-500 animate-spin"></span>
        <span>Yedekleme geçmişi yükleniyor...</span>
      </div>
    `;

        if (history) history.innerHTML = loadingHtml;
        if (historyMobile) historyMobile.innerHTML = loadingHtml;

        try {
            const response = await fetch('/api/backup/logs', {
                headers: {
                    'Authorization': 'Bearer ' + localStorage.getItem('token'),
                    'Content-Type': 'application/json'
                }
            });
            if (!response.ok) {
                throw new Error(`HTTP ${response.status}`);
            }

            const payload = await response.json();
            const logs = Array.isArray(payload?.logs) ? payload.logs : [];

            if (!logs.length) {
                const emptyHtml = `
          <div class="text-center text-sm text-gray-500 dark:text-gray-400 space-y-2">
            <i class="fas fa-database text-2xl text-gray-400"></i>
            <p>Henüz kayıt yok.</p>
            <p class="text-xs">Yeni bir yedekleme işlemi gerçekleştirerek burada geçmişi görebilirsiniz.</p>
          </div>
        `;
                if (history) history.innerHTML = emptyHtml;
                if (historyMobile) historyMobile.innerHTML = emptyHtml;
                return;
            }

            if (history) history.innerHTML = renderBackupHistory(logs);
            if (historyMobile) historyMobile.innerHTML = renderBackupHistoryMobile(logs);
        } catch (error) {
            console.error('Backup history error:', error);
            const errorHtml = `
          <div class="text-center text-sm text-red-500 dark:text-red-400 space-y-2">
            <i class="fas fa-triangle-exclamation text-xl"></i>
            <p>Yedekleme geçmişi yüklenemedi.</p>
            <p class="text-xs">Lütfen daha sonra tekrar deneyin.</p>
          </div>
        `;
            if (history) history.innerHTML = errorHtml;
            if (historyMobile) historyMobile.innerHTML = errorHtml;
            showToast('error', 'Yedekleme geçmişi yüklenemedi.');
        }
    }

    const escapeHtmlMap = {
        '&': '&amp;',
        '<': '&lt;',
        '>': '&gt;',
        '"': '&quot;',
        "'": '&#39;',
        '`': '&#96;'
    };

    function escapeHtml(value) {
        if (value === null || value === undefined) {
            return '';
        }
        return String(value).replace(/[&<>"'`]/g, char => escapeHtmlMap[char] || char);
    }

function renderBackupHistory(logs) {
  const rows = logs.map(renderBackupHistoryRow).join('');
  return `
    <div class="overflow-x-auto">
      <table class="min-w-full divide-y divide-gray-200 dark:divide-gray-700 text-sm">
        <thead class="bg-gray-50 dark:bg-gray-900/40">
          <tr>
            <th class="px-4 py-3 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">Tarih</th>
            <th class="px-4 py-3 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">Kullanıcı</th>
            <th class="px-4 py-3 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">İşlem</th>
            <th class="px-4 py-3 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">Durum</th>
            <th class="px-4 py-3 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">Detay</th>
          </tr>
        </thead>
        <tbody class="bg-white dark:bg-gray-900 divide-y divide-gray-200 dark:divide-gray-700">
          ${rows}
        </tbody>
      </table>
    </div>
  `;
}

function renderBackupHistoryMobile(logs) {
  const cards = logs.map(renderBackupHistoryMobileCard).join('');
  return `
    <div class="space-y-3">
      ${cards}
    </div>
  `;
}

function renderBackupHistoryMobileCard(log) {
  const createdAt = formatBackupDateTime(log.created_at);
  const userLabel = log.user_email ? escapeHtml(String(log.user_email))
                  : (log.user_id ? escapeHtml(`Kullanıcı #${log.user_id}`) : 'Bilinmeyen kullanıcı');
  const ipInfo = log.ip_address ? escapeHtml(String(log.ip_address)) : '';
  const actionBadge = renderBackupActionBadge(log.action);
  const statusBadge = renderBackupStatusBadge(log.status);
  const metadataText = formatBackupMetadata(log.metadata);
  const messageText = log.message ? escapeHtml(String(log.message)) : 'Detay yok';
  const fileName = log.file_name ? escapeHtml(String(log.file_name)) : '';

  return `
    <div class="bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-700 rounded-lg p-4">
      <div class="flex items-start justify-between gap-3 mb-3">
        <div class="flex-1">
          <p class="text-xs text-gray-500 dark:text-gray-400 mb-1">${escapeHtml(createdAt)}</p>
          <p class="text-sm font-medium text-gray-900 dark:text-gray-100">${userLabel}</p>
          ${ipInfo ? `<p class="text-xs text-gray-400 dark:text-gray-500 mt-0.5">IP: ${ipInfo}</p>` : ''}
        </div>
        <div class="flex flex-col gap-1 items-end">
          ${actionBadge}
          ${statusBadge}
        </div>
      </div>

      <div class="space-y-2 pt-3 border-t border-gray-100 dark:border-gray-700">
        <div>
          <p class="text-xs font-medium text-gray-500 dark:text-gray-400 mb-1">Detay:</p>
          <p class="text-sm text-gray-700 dark:text-gray-200">${messageText}</p>
          ${metadataText ? `<p class="text-xs text-gray-500 dark:text-gray-400 mt-1">${escapeHtml(metadataText)}</p>` : ''}
          ${fileName ? `<p class="text-xs text-gray-400 dark:text-gray-500 mt-1">Dosya: ${fileName}</p>` : ''}
        </div>
      </div>
    </div>
  `;
}

function renderBackupHistoryRow(log) {
  const createdAt = formatBackupDateTime(log.created_at);
  const userLabel = log.user_email ? escapeHtml(String(log.user_email))
                  : (log.user_id ? escapeHtml(`Kullanıcı #${log.user_id}`) : 'Bilinmeyen kullanıcı');
  const ipInfo = log.ip_address ? `<div class="text-xs text-gray-400 dark:text-gray-500">IP: ${escapeHtml(String(log.ip_address))}</div>` : '';
  const actionBadge = renderBackupActionBadge(log.action);
  const statusBadge = renderBackupStatusBadge(log.status);
  const metadataText = formatBackupMetadata(log.metadata);
  const messageText = log.message ? escapeHtml(String(log.message)) : 'Detay yok';
  const metadataHtml = metadataText ? `<div class="text-xs text-gray-500 dark:text-gray-400 mt-1">${escapeHtml(metadataText)}</div>` : '';
  const fileHtml = log.file_name ? `<div class="text-xs text-gray-400 dark:text-gray-500 mt-1">Dosya: ${escapeHtml(String(log.file_name))}</div>` : '';

  return `
    <tr class="hover:bg-gray-50 dark:hover:bg-gray-900/60 transition">
      <!-- Tarih -->
      <td class="px-4 py-3 whitespace-nowrap text-sm text-gray-700 dark:text-gray-200 align-middle">
        ${escapeHtml(createdAt)}
      </td>

      <!-- Kullanıcı -->
      <td class="px-4 py-3 align-middle">
        <div class="text-sm font-medium text-gray-900 dark:text-gray-100">${userLabel}</div>
        ${ipInfo}
      </td>

      <!-- İşlem (badge ortalı) -->
      <td class="px-4 py-3 align-middle">
        <div class="flex items-center gap-2">${actionBadge}</div>
      </td>

      <!-- Durum (badge ortalı) -->
      <td class="px-4 py-3 align-middle">
        <div class="flex items-center gap-2">${statusBadge}</div>
      </td>

      <!-- Detay -->
      <td class="px-4 py-3 align-middle">
        <div class="text-sm text-gray-700 dark:text-gray-200">${messageText}</div>
        ${metadataHtml}
        ${fileHtml}
      </td>
    </tr>
  `;
}


    function renderBackupActionBadge(action) {
        if (action === 'import') {
            return `<span class="inline-flex items-center gap-2 px-3 py-1 rounded-full text-xs font-medium bg-emerald-50 text-emerald-600 dark:bg-emerald-500/10 dark:text-emerald-300"><i class="fas fa-upload"></i> Yükleme</span>`;
        }
        return `<span class="inline-flex items-center gap-2 px-3 py-1 rounded-full text-xs font-medium bg-blue-50 text-blue-600 dark:bg-blue-500/10 dark:text-blue-300"><i class="fas fa-download"></i> İndirme</span>`;
    }

    function renderBackupStatusBadge(status) {
        if (status === 'success') {
            return `<span class="inline-flex items-center gap-2 px-3 py-1 rounded-full text-xs font-semibold border bg-emerald-50 text-emerald-600 border-emerald-200 dark:bg-emerald-500/10 dark:text-emerald-300 dark:border-emerald-400/40"><span class="inline-block w-2 h-2 rounded-full bg-emerald-500"></span> Başarılı</span>`;
        }
        return `<span class="inline-flex items-center gap-2 px-3 py-1 rounded-full text-xs font-semibold border bg-red-50 text-red-600 border-red-200 dark:bg-red-500/10 dark:text-red-300 dark:border-red-400/40"><span class="inline-block w-2 h-2 rounded-full bg-red-500"></span> Hata</span>`;
    }

    function formatBackupMetadata(meta) {

        if (!meta || typeof meta !== 'object') { return ''; }

        const fragments = [];



        const formatNumber = (value) => {

            const numeric = Number(value);

            return Number.isFinite(numeric) ? numeric : value;

        };



        if (meta.targets_count !== undefined && meta.targets_count !== null) {

            fragments.push(`Hedef: ${formatNumber(meta.targets_count)}`);

        }

        if (meta.ip_blacklist_count !== undefined && meta.ip_blacklist_count !== null) {

            fragments.push(`IP: ${formatNumber(meta.ip_blacklist_count)}`);

        }

        if (meta.domains_count !== undefined && meta.domains_count !== null) {

            fragments.push(`Domain: ${formatNumber(meta.domains_count)}`);

        }

        if (meta.error) {

            fragments.push(`Hata: ${meta.error}`);

        }

        if (meta.raw) {

            fragments.push(String(meta.raw));

        }

        return fragments.join(' - ');

    }



function formatBackupDateTime(value) {
        if (!value) { return '-'; }
        const primary = new Date(value);
        if (!Number.isNaN(primary.getTime())) {
            return primary.toLocaleString('tr-TR', { day: '2-digit', month: '2-digit', year: 'numeric', hour: '2-digit', minute: '2-digit' });
        }
        const fallback = new Date(String(value).replace(' ', 'T') + 'Z');
        if (!Number.isNaN(fallback.getTime())) {
            return fallback.toLocaleString('tr-TR', { day: '2-digit', month: '2-digit', year: 'numeric', hour: '2-digit', minute: '2-digit' });
        }
        return String(value);
    }

    function bindIfNeeded(element, handler, event = 'click') {
        if (!element || element.dataset.bound === 'true') { return; }
        element.addEventListener(event, handler);
        element.dataset.bound = 'true';
    }

    window.initBackupPage = function initBackupPage() {
        const exportBtn = document.getElementById('backup-export-button');
        const importBtn = document.getElementById('backup-import-button');
        const selectBtn = document.getElementById('backup-file-select');
        const fileInput = document.getElementById('backup-file-input');
        const refreshBtn = document.getElementById('backup-refresh-history');

        bindIfNeeded(exportBtn, exportBackup);
        bindIfNeeded(importBtn, importBackup);
        bindIfNeeded(selectBtn, () => fileInput && fileInput.click());
        bindIfNeeded(refreshBtn, loadBackupHistory);
        bindIfNeeded(fileInput, handleFileChange, 'change');

        resetFileSelection();
        loadBackupHistory();
    };
})();

// -------------------------
// Network settings helpers
// -------------------------
(function () {
    const DEFAULT_LOADING_TEXT = 'Yükleniyor...';
    let isEditMode = false;
    let originalValues = {};
    let allowlistItems = [];

    function toggleLoading(show, message) {
        const overlay = document.getElementById('loading-overlay');
        if (!overlay) { return; }
        const label = overlay.querySelector('span');
        if (show) {
            if (label && message) { label.textContent = message; }
            overlay.classList.remove('hidden');
        } else {
            if (label) { label.textContent = DEFAULT_LOADING_TEXT; }
            overlay.classList.add('hidden');
        }
    }

    function showToast(type, message, title = '') {
        if (window.Toast && typeof window.Toast.show === 'function') {
            Toast.show({ type, title, message });
        } else {
            console[type === 'error' ? 'error' : 'log'](message);
        }
    }

    function getInputIds() {
        return ['network-ip', 'network-subnet', 'network-gateway', 'network-dns1', 'network-dns2'];
    }

    function setFieldsDisabled(disabled) {
        getInputIds().forEach(id => {
            const el = document.getElementById(id);
            if (el) { el.disabled = disabled; }
        });
    }

    function setFormValues(payload) {
        const ipInput = document.getElementById('network-ip');
        const subnetInput = document.getElementById('network-subnet');
        const gatewayInput = document.getElementById('network-gateway');
        const dns1Input = document.getElementById('network-dns1');
        const dns2Input = document.getElementById('network-dns2');

        if (ipInput) ipInput.value = payload.ip || '';
        if (subnetInput) subnetInput.value = payload.subnet_mask || '';
        if (gatewayInput) gatewayInput.value = payload.gateway || '';
        if (dns1Input) dns1Input.value = payload.dns && payload.dns[0] ? payload.dns[0] : '';
        if (dns2Input) dns2Input.value = payload.dns && payload.dns[1] ? payload.dns[1] : '';
    }

    function getFormValues() {
        return {
            mode: 'static',
            ip: document.getElementById('network-ip')?.value || '',
            subnet_mask: document.getElementById('network-subnet')?.value || '',
            gateway: document.getElementById('network-gateway')?.value || '',
            dns: [
                document.getElementById('network-dns1')?.value || '',
                document.getElementById('network-dns2')?.value || ''
            ]
        };
    }

    function saveOriginalValues() {
        originalValues = getFormValues();
    }

    function restoreOriginalValues() {
        setFormValues(originalValues);
    }

    function enterEditMode() {
        isEditMode = true;
        saveOriginalValues();
        setFieldsDisabled(false);

        const editBtn = document.getElementById('network-edit-btn');
        const actionsDiv = document.getElementById('network-actions');

        if (editBtn) { editBtn.classList.add('hidden'); }
        if (actionsDiv) { actionsDiv.classList.remove('hidden'); }
    }

    function exitEditMode() {
        isEditMode = false;
        setFieldsDisabled(true);

        const editBtn = document.getElementById('network-edit-btn');
        const actionsDiv = document.getElementById('network-actions');

        if (editBtn) { editBtn.classList.remove('hidden'); }
        if (actionsDiv) { actionsDiv.classList.add('hidden'); }
    }

    function cancelEdit() {
        restoreOriginalValues();
        exitEditMode();
    }

    async function loadNetworkSettings() {
        toggleLoading(true, 'Ağ bilgileri alınıyor...');
        try {
            const response = await fetch('/api/network/settings', {
                headers: {
                    'Authorization': 'Bearer ' + localStorage.getItem('token'),
                    'Content-Type': 'application/json'
                }
            });
            const payload = await response.json().catch(() => ({}));
            if (!response.ok) {
                throw new Error(payload.error || 'Ağ bilgileri alınamadı.');
            }
            setFormValues(payload);
            exitEditMode();
        } catch (error) {
            console.error('Network settings load error:', error);
            showToast('error', error.message || 'Ağ bilgileri alınamadı.');
        } finally {
            toggleLoading(false);
        }
    }

    let pendingNetworkPayload = null;

    async function applyNetworkSettings() {
        const ip = document.getElementById('network-ip')?.value?.trim() || '';
        const subnet = document.getElementById('network-subnet')?.value?.trim() || '';

        if (!ip) {
            showToast('error', 'IP adresi zorunludur.');
            return;
        }
        if (!subnet) {
            showToast('error', 'Subnet mask seçimi zorunludur.');
            return;
        }

        pendingNetworkPayload = {
            mode: 'static',
            ip,
            subnet_mask: subnet,
            gateway: document.getElementById('network-gateway')?.value?.trim() || '',
            dns: [
                document.getElementById('network-dns1')?.value?.trim() || '',
                document.getElementById('network-dns2')?.value?.trim() || ''
            ].filter(Boolean)
        };

        const confirmModal = document.getElementById('network-confirm-modal');
        const confirmIp = document.getElementById('network-confirm-ip');
        if (confirmIp) { confirmIp.textContent = ip; }

        const noGatewayWarn = document.getElementById('network-confirm-no-gateway');
        if (noGatewayWarn) {
            const gw = document.getElementById('network-gateway')?.value?.trim() || '';
            noGatewayWarn.classList.toggle('hidden', !!gw);
        }

        if (confirmModal) { confirmModal.classList.remove('hidden'); }
    }

    async function confirmAndApplyNetwork() {
        if (!pendingNetworkPayload) { return; }
        const payload = pendingNetworkPayload;
        pendingNetworkPayload = null;

        const confirmModal = document.getElementById('network-confirm-modal');
        if (confirmModal) { confirmModal.classList.add('hidden'); }

        toggleLoading(true, 'Ağ ayarları uygulanıyor...');
        try {
            const response = await fetch('/api/network/settings', {
                method: 'POST',
                headers: {
                    'Authorization': 'Bearer ' + localStorage.getItem('token'),
                    'Content-Type': 'application/json'
                },
                body: JSON.stringify(payload)
            });
            const result = await response.json().catch(() => ({}));
            if (!response.ok) {
                throw new Error(result.error || 'Ağ ayarları uygulanamadı.');
            }
            showToast('success', 'Ağ ayarları başarıyla uygulandı.');
            exitEditMode();
            await loadNetworkSettings();
        } catch (error) {
            console.error('Network settings apply error:', error);
            showToast('error', error.message || 'Ağ ayarları uygulanamadı.');
        } finally {
            toggleLoading(false);
        }
    }

    function bindIfNeeded(element, handler, event = 'click') {
        if (!element || element.dataset.bound === 'true') { return; }
        element.addEventListener(event, handler);
        element.dataset.bound = 'true';
    }

    function setActiveTab(tab) {
        const configTab = document.getElementById('network-tab-config');
        const allowlistTab = document.getElementById('network-tab-allowlist');
        const configBtn = document.getElementById('network-tab-btn-config');
        const allowlistBtn = document.getElementById('network-tab-btn-allowlist');
        const descriptionEl = document.getElementById('network-page-description');

        if (configTab) configTab.classList.toggle('hidden', tab !== 'config');
        if (allowlistTab) allowlistTab.classList.toggle('hidden', tab !== 'allowlist');

        if (configBtn) {
            configBtn.classList.toggle('bg-primary-600', tab === 'config');
            configBtn.classList.toggle('text-white', tab === 'config');
            configBtn.classList.toggle('bg-gray-100', tab !== 'config');
            configBtn.classList.toggle('dark:bg-gray-700', tab !== 'config');
            configBtn.classList.toggle('text-gray-700', tab !== 'config');
            configBtn.classList.toggle('dark:text-gray-100', tab !== 'config');
        }
        if (allowlistBtn) {
            allowlistBtn.classList.toggle('bg-primary-600', tab === 'allowlist');
            allowlistBtn.classList.toggle('text-white', tab === 'allowlist');
            allowlistBtn.classList.toggle('bg-gray-100', tab !== 'allowlist');
            allowlistBtn.classList.toggle('dark:bg-gray-700', tab !== 'allowlist');
            allowlistBtn.classList.toggle('text-gray-700', tab !== 'allowlist');
            allowlistBtn.classList.toggle('dark:text-gray-100', tab !== 'allowlist');
        }

        if (descriptionEl) {
            if (tab === 'allowlist') {
                descriptionEl.innerHTML = 'Sadece izin verdiğiniz IP adreslerinden erişimi sınırlandırın.';
            } else {
                descriptionEl.innerHTML = 'Cihazın <span class="font-mono font-semibold">eth0</span> arayüzü için IP yapılandırması';
            }
        }
    }

    function renderAllowlist() {
        const listEl = document.getElementById('allowlist-list');
        if (!listEl) { return; }
        listEl.innerHTML = '';

        if (!allowlistItems || allowlistItems.length === 0) {
            const empty = document.createElement('div');
            empty.className = 'text-sm text-gray-500 dark:text-gray-400';
            empty.textContent = 'Henüz IP eklenmedi.';
            listEl.appendChild(empty);
            return;
        }

        allowlistItems.forEach(item => {
            const isCurrent = String(item.label || '').trim().toLowerCase() === 'current ip';
            const row = document.createElement('div');
            row.className = 'flex items-center justify-between gap-3 p-3 sm:p-4 rounded-xl border border-gray-200 dark:border-gray-700 bg-white/70 dark:bg-gray-900/40 shadow-sm hover:shadow-md transition';

            const left = document.createElement('div');
            left.className = 'flex items-center gap-3';

            const badge = document.createElement('div');
            badge.className = 'h-10 w-10 rounded-xl bg-emerald-50 dark:bg-emerald-500/10 text-emerald-600 dark:text-emerald-300 flex items-center justify-center';
            badge.innerHTML = '<i class="fas fa-network-wired"></i>';

            const textWrap = document.createElement('div');
            textWrap.className = 'space-y-1';
            const ip = document.createElement('div');
            ip.className = 'text-sm font-semibold text-gray-900 dark:text-gray-100';
            ip.textContent = item.ip_address;
            const label = document.createElement('div');
            label.className = 'text-xs text-gray-500 dark:text-gray-400';
            if (isCurrent) {
                label.textContent = 'Mevcut IP (Otomatik)';
            } else {
                label.textContent = item.label || 'Etiket yok';
            }

            textWrap.appendChild(ip);
            textWrap.appendChild(label);
            left.appendChild(badge);
            left.appendChild(textWrap);

            let actionEl;
            if (isCurrent) {
                actionEl = document.createElement('div');
                actionEl.className = 'h-9 w-9 inline-flex items-center justify-center rounded-lg text-gray-400 bg-gray-100/70 dark:bg-gray-800/60';
                actionEl.innerHTML = '<i class="fas fa-lock"></i>';
                actionEl.setAttribute('title', 'Bu kayıt silinemez');
            } else {
                const btn = document.createElement('button');
                btn.className = 'h-9 w-9 inline-flex items-center justify-center rounded-lg text-red-600 hover:text-red-700 hover:bg-red-50 dark:hover:bg-red-500/10 transition';
                btn.setAttribute('aria-label', 'IP sil');
                btn.innerHTML = '<i class="fas fa-trash"></i>';
                btn.dataset.id = item.id;
                btn.addEventListener('click', async () => {
                    openDeleteModal(item.id, item.ip_address);
                });
                actionEl = btn;
            }

            row.appendChild(left);
            row.appendChild(actionEl);
            listEl.appendChild(row);
        });
    }

    let pendingDeleteId = null;

    function openDeleteModal(id, ip) {
        pendingDeleteId = id;
        const modal = document.getElementById('allowlist-delete-modal');
        const ipEl = document.getElementById('allowlist-delete-ip');
        if (ipEl) { ipEl.textContent = ip || '-'; }
        if (modal) { modal.classList.remove('hidden'); }
    }

    function closeDeleteModal() {
        pendingDeleteId = null;
        const modal = document.getElementById('allowlist-delete-modal');
        if (modal) { modal.classList.add('hidden'); }
    }

    async function confirmDelete() {
        if (!pendingDeleteId) { return; }
        const id = pendingDeleteId;
        closeDeleteModal();
        await deleteAllowlistItem(id);
    }

    async function loadAllowlist() {
        try {
            const response = await fetch('/api/network/allowlist', {
                headers: {
                    'Authorization': 'Bearer ' + localStorage.getItem('token'),
                    'Content-Type': 'application/json'
                }
            });
            const payload = await response.json().catch(() => ({}));
            if (!response.ok) {
                throw new Error(payload.error || 'Allowlist yüklenemedi.');
            }
            allowlistItems = payload.items || [];
            const toggle = document.getElementById('allowlist-enabled');
            if (toggle) { toggle.checked = !!payload.enabled; }
            renderAllowlist();
        } catch (error) {
            console.error('Allowlist load error:', error);
            showToast('error', error.message || 'Allowlist yüklenemedi.');
        }
    }

    async function toggleAllowlist(enabled) {
        try {
            const response = await fetch('/api/network/allowlist/toggle', {
                method: 'POST',
                headers: {
                    'Authorization': 'Bearer ' + localStorage.getItem('token'),
                    'Content-Type': 'application/json'
                },
                body: JSON.stringify({ enabled })
            });
            const payload = await response.json().catch(() => ({}));
            if (!response.ok) {
                throw new Error(payload.error || 'Ayar güncellenemedi.');
            }
            showToast('success', 'Ayar güncellendi.');
            await loadAllowlist();
        } catch (error) {
            console.error('Allowlist toggle error:', error);
            showToast('error', error.message || 'Ayar güncellenemedi.');
        }
    }

    async function addAllowlistItem() {
        const ipInput = document.getElementById('allowlist-ip');
        const labelInput = document.getElementById('allowlist-label');
        const ip = ipInput?.value?.trim() || '';
        const label = labelInput?.value?.trim() || '';
        if (!ip) {
            showToast('error', 'IP adresi gerekli.');
            return;
        }

        try {
            const response = await fetch('/api/network/allowlist', {
                method: 'POST',
                headers: {
                    'Authorization': 'Bearer ' + localStorage.getItem('token'),
                    'Content-Type': 'application/json'
                },
                body: JSON.stringify({ ip_address: ip, label })
            });
            const payload = await response.json().catch(() => ({}));
            if (!response.ok) {
                throw new Error(payload.error || 'IP eklenemedi.');
            }
            if (ipInput) ipInput.value = '';
            if (labelInput) labelInput.value = '';
            showToast('success', payload.message || 'IP eklendi.');
            await loadAllowlist();
        } catch (error) {
            console.error('Allowlist add error:', error);
            showToast('error', error.message || 'IP eklenemedi.');
        }
    }

    async function deleteAllowlistItem(id) {
        try {
            const response = await fetch(`/api/network/allowlist/${id}`, {
                method: 'DELETE',
                headers: {
                    'Authorization': 'Bearer ' + localStorage.getItem('token'),
                    'Content-Type': 'application/json'
                }
            });
            const payload = await response.json().catch(() => ({}));
            if (!response.ok) {
                throw new Error(payload.error || 'IP silinemedi.');
            }
            showToast('success', payload.message || 'IP silindi.');
            await loadAllowlist();
        } catch (error) {
            console.error('Allowlist delete error:', error);
            showToast('error', error.message || 'IP silinemedi.');
        }
    }

    window.initNetworkSettingsPage = function initNetworkSettingsPage() {
        const refreshBtn = document.getElementById('network-refresh-btn');
        const editBtn = document.getElementById('network-edit-btn');
        const cancelBtn = document.getElementById('network-cancel-btn');
        const applyBtn = document.getElementById('network-apply-btn');
        const tabConfigBtn = document.getElementById('network-tab-btn-config');
        const tabAllowlistBtn = document.getElementById('network-tab-btn-allowlist');
        const allowlistToggle = document.getElementById('allowlist-enabled');
        const allowlistAddBtn = document.getElementById('allowlist-add-btn');
        const deleteModal = document.getElementById('allowlist-delete-modal');
        const deleteBackdrop = document.getElementById('allowlist-delete-backdrop');
        const deleteCancelBtn = document.getElementById('allowlist-delete-cancel');
        const deleteConfirmBtn = document.getElementById('allowlist-delete-confirm');

        const confirmOkBtn = document.getElementById('network-confirm-ok');
        const confirmCancelBtn = document.getElementById('network-confirm-cancel');

        bindIfNeeded(refreshBtn, loadNetworkSettings);
        bindIfNeeded(editBtn, enterEditMode);
        bindIfNeeded(cancelBtn, cancelEdit);
        bindIfNeeded(applyBtn, applyNetworkSettings);
        bindIfNeeded(confirmOkBtn, confirmAndApplyNetwork);
        bindIfNeeded(confirmCancelBtn, () => {
            pendingNetworkPayload = null;
            const modal = document.getElementById('network-confirm-modal');
            if (modal) { modal.classList.add('hidden'); }
        });
        bindIfNeeded(tabConfigBtn, () => setActiveTab('config'));
        bindIfNeeded(tabAllowlistBtn, () => setActiveTab('allowlist'));
        bindIfNeeded(allowlistAddBtn, addAllowlistItem);
        bindIfNeeded(allowlistToggle, (event) => {
            const enabled = event?.target?.checked === true;
            toggleAllowlist(enabled);
        }, 'change');
        bindIfNeeded(deleteCancelBtn, closeDeleteModal);
        bindIfNeeded(deleteConfirmBtn, confirmDelete);
        bindIfNeeded(deleteBackdrop, closeDeleteModal);

        loadNetworkSettings();
        loadAllowlist();
        setActiveTab('config');
    };
})();

// Notification Settings page init (executed after partial load)
window.initNotificationSettingsPage = function() {
    const MAX_NOTIFICATION_TARGETS = 50;
    // Bind buttons if present
    const saveBtn = document.getElementById('btn-save-settings');
    const testOpenBtn = document.getElementById('btn-test-email-open');
    const testSendBtn = document.getElementById('btn-test-email-send');
    const testCloseBtn = document.getElementById('btn-close-test-email');
    const testCancelBtn = document.getElementById('btn-cancel-test-email');
    const openBtn = document.getElementById('btn-open-create');
    const closeBtn = document.getElementById('btn-close-create');
    const cancelBtn = document.getElementById('btn-cancel-create');
    const submitBtn = document.getElementById('btn-submit-create');
    const templatesLink = document.getElementById('lnk-templates');
    const btnSaveSMTP = document.getElementById('btn-save-smtp');
    const targetPicker = createTargetPicker();
    const serviceModeAll = document.getElementById('service-mode-all');
    const serviceModeMonitored = document.getElementById('service-mode-monitored');
    const serviceListEl = document.getElementById('rule-service-list');
    const serviceHelperEl = document.getElementById('rule-service-helper');
    const condStatusWrap = document.getElementById('cond-status-wrap');
    const condServicesWrap = document.getElementById('cond-services-wrap');
    const TARGET_PICKER_BATCH_SIZE = 250;
    const TARGET_PICKER_MAX_PAGES = 500;
    const targetPickerDataState = {
        options: [],
        loadingPromise: null,
        loaded: false
    };

    const ESCAPE_HTML_MAP = {
        '&': '&amp;',
        '<': '&lt;',
        '>': '&gt;',
        '"': '&quot;',
        "'": '&#39;',
        '`': '&#96;'
    };

    function escapeHtml(value) {
        if (value === null || value === undefined) {
            return '';
        }
        return String(value).replace(/[&<>"'`]/g, char => ESCAPE_HTML_MAP[char] || char);
    }

    function createTargetPicker(){
        const wrapper = document.getElementById('rule-target-picker');
        const filterInput = document.getElementById('rule-target-filter');
        const filterWrap = document.getElementById('rule-target-filter-wrap');
        if (!wrapper) {
            return {
                setOptions(){},
                setSelected(){},
                clearSelection(){},
                getSelected(){ return []; },
                setDisabled(){},
                closePanel(){},
                setMaxSelectable(){},
                describeTarget(){ return ''; },
                isEmpty(){ return true; },
                setListMessage(){},
                setFilter(){},
                clearFilter(){},
                setOnChange(){}
            };
        }

        const toggleBtn = document.getElementById('rule-target-toggle');
        const panel = document.getElementById('rule-target-panel');
        const labelEl = document.getElementById('rule-target-label');
        const listEl = document.getElementById('rule-target-list');
        const selectAllEl = document.getElementById('rule-target-select-all');
        const counterEl = document.getElementById('rule-target-counter');
        const helperEl = document.getElementById('rule-target-helper');

        const state = {
            options: [],
            selected: new Set(),
            expandedTags: new Set(),
            maxSelectable: Infinity,
            disabled: false,
            listMessage: 'Hedefler yükleniyor...',
            filterText: '',
            onChange: null
        };

        function notifyChange(){
            if (typeof state.onChange === 'function') {
                state.onChange(Array.from(state.selected));
            }
        }

        function normalizeTagList(value){
            if (!value) return [];
            if (Array.isArray(value)) {
                return value.map(tag => String(tag || '').trim()).filter(Boolean);
            }
            if (typeof value === 'string') {
                return value.split(',').map(tag => tag.trim()).filter(Boolean);
            }
            return [];
        }

        function matchesFilter(target, tag, filter){
            if (!filter) return true;
            const haystack = [
                tag,
                target.name,
                target.address,
                ...(target.tags || [])
            ].map(value => String(value || '').toLowerCase());
            return haystack.some(value => value.includes(filter));
        }

        function getTagGroups(){
            const groups = new Map();
            const filter = state.filterText.trim().toLowerCase();
            state.options.forEach(opt => {
                const tags = opt.tags && opt.tags.length ? opt.tags : ['Etiketsiz'];
                tags.forEach(tag => {
                    const key = String(tag || '').trim() || 'Etiketsiz';
                    if (!matchesFilter(opt, key, filter)) return;
                    if (!groups.has(key)) groups.set(key, []);
                    groups.get(key).push(opt);
                });
            });
            return Array.from(groups.entries())
                .map(([tag, targets]) => ({
                    tag,
                    targets: targets.slice().sort((a, b) => (a.name || '').localeCompare(b.name || '', 'tr'))
                }))
                .sort((a, b) => a.tag.localeCompare(b.tag, 'tr'));
        }

        function getVisibleOptions(){
            const groups = getTagGroups();
            const visible = [];
            groups.forEach(group => {
                if (state.expandedTags.has(group.tag)) {
                    group.targets.forEach(target => visible.push(target));
                }
            });
            return visible;
        }

        function getSelectableVisibleOptions(){
            const seen = new Set();
            return getVisibleOptions().filter(opt => {
                const id = String(opt.id);
                if (seen.has(id)) return false;
                seen.add(id);
                return true;
            });
        }

        function getTagSelectionState(targets){
            const total = targets.length;
            const selected = targets.filter(target => state.selected.has(String(target.id))).length;
            return {
                checked: total > 0 && selected === total,
                indeterminate: selected > 0 && selected < total,
                selected,
                total
            };
        }

        function canAddCount(count){
            if (!Number.isFinite(state.maxSelectable)) return count;
            return Math.max(0, state.maxSelectable - state.selected.size);
        }

        function selectTargets(targets, shouldSelect){
            const uniqueTargets = [];
            const seen = new Set();
            targets.forEach(target => {
                const id = String(target.id);
                if (seen.has(id)) return;
                seen.add(id);
                uniqueTargets.push(target);
            });
            if (shouldSelect) {
                let available = canAddCount(uniqueTargets.filter(target => !state.selected.has(String(target.id))).length);
                uniqueTargets.forEach(target => {
                    const id = String(target.id);
                    if (state.selected.has(id)) return;
                    if (available <= 0) return;
                    state.selected.add(id);
                    available -= 1;
                });
            } else {
                uniqueTargets.forEach(target => state.selected.delete(String(target.id)));
            }
            renderList();
            notifyChange();
        }

        function buildLabel(){
            if (state.selected.size === 0) return 'Hedef seçin';
            if (state.selected.size === state.options.length && state.options.length > 0) {
                return 'Tüm hedefler seçili';
            }
            if (state.selected.size === 1) {
                const id = Array.from(state.selected)[0];
                const target = state.options.find(opt => String(opt.id) === String(id));
                if (target) {
                    return target.name || `Hedef #${target.id}`;
                }
            }
            return `${state.selected.size} hedef seçildi`;
        }

        function updateSelectAllState(visible, selectedVisibleCount){
            if (!selectAllEl) return;
            const visibleTargets = visible || getSelectableVisibleOptions();
            const selectedVisible = typeof selectedVisibleCount === 'number'
                ? selectedVisibleCount
                : visibleTargets.filter(opt => state.selected.has(String(opt.id))).length;

            selectAllEl.disabled = state.disabled || visibleTargets.length === 0;
            if (visibleTargets.length === 0) {
                selectAllEl.checked = false;
                selectAllEl.indeterminate = false;
                return;
            }
            if (selectedVisible === visibleTargets.length) {
                selectAllEl.checked = true;
                selectAllEl.indeterminate = false;
            } else if (selectedVisible === 0) {
                selectAllEl.checked = false;
                selectAllEl.indeterminate = false;
            } else {
                selectAllEl.checked = false;
                selectAllEl.indeterminate = true;
            }
        }

        function updateSummary(visible, selectedVisibleCount){
            const safeVisible = visible || getSelectableVisibleOptions();
            const selectedVisible = typeof selectedVisibleCount === 'number'
                ? selectedVisibleCount
                : safeVisible.filter(opt => state.selected.has(String(opt.id))).length;
            if (labelEl) {
                labelEl.textContent = buildLabel();
            }
            if (counterEl) {
                counterEl.textContent = `${state.selected.size}/${state.options.length}`;
            }
            if (helperEl) {
                if (state.disabled) {
                    helperEl.textContent = 'Hedef seçimi devre dışı.';
                } else if (state.filterText.trim() && safeVisible.length === 0) {
                    helperEl.textContent = 'Aramaya uyan açık hedef yok. Etiketi açarak veya aramayı temizleyerek devam edebilirsiniz.';
                } else if (safeVisible.length > 0) {
                    helperEl.textContent = `${safeVisible.length} görünen hedef, toplam ${state.selected.size} seçili.`;
                } else if (state.selected.size > 0) {
                    helperEl.textContent = `${state.selected.size} hedef seçili. Bir etiketi açarak hedefleri görüntüleyebilirsiniz.`;
                } else {
                    helperEl.textContent = 'Etiketi seçerek toplu seçim yapabilir veya etiketi açarak hedefleri tek tek seçebilirsiniz.';
                }
            }
            updateSelectAllState(safeVisible, selectedVisible);
        }

        function closePanel(){
            if (panel) {
                panel.classList.add('hidden');
            }
            if (toggleBtn) {
                toggleBtn.setAttribute('aria-expanded', 'false');
            }
        }

        function openPanel(){
            if (state.disabled || !panel) return;
            panel.classList.remove('hidden');
            if (toggleBtn) {
                toggleBtn.setAttribute('aria-expanded', 'true');
            }
        }

        function toggleValue(id, shouldSelect){
            if (shouldSelect) {
                if (state.maxSelectable === 1) {
                    state.selected.clear();
                }
                if (!Number.isFinite(state.maxSelectable) || state.selected.has(id) || state.selected.size < state.maxSelectable) {
                    state.selected.add(id);
                }
            } else {
                state.selected.delete(id);
            }
            renderList();
            notifyChange();
        }

        function renderList(){
            if (!listEl) return;
            listEl.innerHTML = '';
            const groups = getTagGroups();
            if (groups.length === 0) {
                const msg = document.createElement('div');
                msg.className = 'px-3 py-2 text-xs text-gray-500 dark:text-gray-400';
                msg.textContent = state.filterText.trim() ? 'Aramaya uyan hedef veya etiket bulunamadı.' : (state.listMessage || 'Hedef bulunamadı.');
                listEl.appendChild(msg);
                updateSummary([], 0);
                return;
            }
            groups.forEach(group => {
                const tagState = getTagSelectionState(group.targets);
                const tagRow = document.createElement('div');
                tagRow.className = 'flex items-center gap-3 px-3 py-2 hover:bg-gray-50 dark:hover:bg-gray-700/40';

                const tagCb = document.createElement('input');
                tagCb.type = 'checkbox';
                tagCb.className = 'rounded text-blue-600';
                tagCb.checked = tagState.checked;
                tagCb.indeterminate = tagState.indeterminate;
                tagCb.addEventListener('click', event => {
                    event.stopPropagation();
                    selectTargets(group.targets, tagState.selected === 0);
                });

                const expandBtn = document.createElement('button');
                expandBtn.type = 'button';
                expandBtn.className = 'flex min-w-0 flex-1 items-center justify-between gap-3 text-left';
                expandBtn.setAttribute('aria-expanded', state.expandedTags.has(group.tag) ? 'true' : 'false');
                expandBtn.addEventListener('click', event => {
                    event.stopPropagation();
                    if (state.expandedTags.has(group.tag)) {
                        state.expandedTags.delete(group.tag);
                    } else {
                        state.expandedTags.add(group.tag);
                    }
                    renderList();
                });

                const tagInfo = document.createElement('div');
                tagInfo.className = 'min-w-0';
                const tagTitle = document.createElement('span');
                tagTitle.className = 'block truncate text-sm font-semibold text-gray-800 dark:text-gray-100';
                tagTitle.textContent = group.tag;
                const tagSub = document.createElement('span');
                tagSub.className = 'block text-xs text-gray-500 dark:text-gray-400';
                tagSub.textContent = `${tagState.selected}/${tagState.total} hedef seçili`;
                tagInfo.appendChild(tagTitle);
                tagInfo.appendChild(tagSub);

                const chevron = document.createElement('span');
                chevron.className = 'text-xs text-gray-500 dark:text-gray-400';
                chevron.textContent = state.expandedTags.has(group.tag) ? '▲' : '▼';
                expandBtn.appendChild(tagInfo);
                expandBtn.appendChild(chevron);

                tagRow.appendChild(tagCb);
                tagRow.appendChild(expandBtn);
                listEl.appendChild(tagRow);

                if (!state.expandedTags.has(group.tag)) return;

                group.targets.forEach(target => {
                const row = document.createElement('label');
                row.className = 'ml-8 flex items-center gap-3 px-3 py-2 cursor-pointer hover:bg-gray-50 dark:hover:bg-gray-700/40';
                const cb = document.createElement('input');
                cb.type = 'checkbox';
                cb.className = 'rounded text-blue-600';
                cb.checked = state.selected.has(String(target.id));
                cb.addEventListener('click', event => {
                    event.stopPropagation();
                    toggleValue(String(target.id), !state.selected.has(String(target.id)));
                });
                const body = document.createElement('div');
                body.className = 'flex min-w-0 flex-1 items-center justify-between gap-3';
                const info = document.createElement('div');
                info.className = 'min-w-0 flex-1';
                const title = document.createElement('span');
                title.className = 'block truncate text-sm font-medium text-gray-800 dark:text-gray-100';
                title.textContent = target.name || `Hedef #${target.id}`;
                const subtitle = document.createElement('span');
                subtitle.className = 'block truncate text-xs text-gray-500 dark:text-gray-400';
                subtitle.textContent = target.address || '-';
                info.appendChild(title);
                info.appendChild(subtitle);
                body.appendChild(info);
                if (target.tags && target.tags.length > 0) {
                    const tagsWrap = document.createElement('div');
                    tagsWrap.className = 'ml-auto flex max-w-[50%] flex-wrap justify-end gap-1';
                    target.tags.forEach(tag => {
                        const badge = document.createElement('span');
                        badge.className = 'px-2 py-0.5 rounded-full text-[11px] font-medium bg-blue-50 text-blue-700 dark:bg-blue-500/10 dark:text-blue-200';
                        badge.textContent = tag;
                        tagsWrap.appendChild(badge);
                    });
                    body.appendChild(tagsWrap);
                }
                row.appendChild(cb);
                row.appendChild(body);
                row.addEventListener('click', event => {
                    event.stopPropagation();
                });
                listEl.appendChild(row);
                });
            });
            updateSummary(getSelectableVisibleOptions());
        }

        if (toggleBtn) {
            toggleBtn.addEventListener('click', event => {
                event.stopPropagation();
                if (panel?.classList.contains('hidden')) openPanel();
                else closePanel();
            });
        }

        if (panel) {
            panel.addEventListener('click', event => {
                event.stopPropagation();
            });
        }

        if (selectAllEl) {
            selectAllEl.addEventListener('click', event => {
                event.stopPropagation();
                if (selectAllEl.disabled) return;
                const visibleTargets = getSelectableVisibleOptions();
                const selectedVisible = visibleTargets.filter(opt => state.selected.has(String(opt.id))).length;
                selectTargets(visibleTargets, selectedVisible === 0);
            });
        }

        if (window.__targetPickerDocHandler) {
            document.removeEventListener('click', window.__targetPickerDocHandler);
        }
        const docHandler = (event) => {
            const insidePicker = wrapper.contains(event.target);
            const insideFilter = filterWrap ? filterWrap.contains(event.target) : false;
            if (!insidePicker && !insideFilter) {
                closePanel();
            }
        };
        window.__targetPickerDocHandler = docHandler;
        document.addEventListener('click', docHandler);

        if (window.__targetPickerKeyHandler) {
            document.removeEventListener('keydown', window.__targetPickerKeyHandler);
        }
        const keyHandler = (event) => {
            if (event.key === 'Escape') {
                closePanel();
            }
        };
        window.__targetPickerKeyHandler = keyHandler;
        document.addEventListener('keydown', keyHandler);

        function applyFilter(value, syncInput){
            state.filterText = value || '';
            if (syncInput && filterInput && filterInput.value !== value) {
                filterInput.value = value;
            }
            if (state.filterText.trim()) {
                getTagGroups().forEach(group => state.expandedTags.add(group.tag));
            }
            renderList();
        }

        if (filterInput) {
            filterInput.addEventListener('click', event => {
                event.stopPropagation();
                openPanel();
            });
            filterInput.addEventListener('input', event => {
                applyFilter(event.target.value || '', false);
                openPanel();
            });
        }

        renderList();

        return {
            setOptions(opts){
                const formatted = Array.isArray(opts)
                    ? opts.filter(Boolean).map(opt => {
                        const tags = normalizeTagList(opt.tags ?? opt.Tags ?? opt.tags_list);
                        return {
                            id: String(opt.id ?? opt.ID ?? ''),
                            name: opt.name ?? opt.Name ?? '',
                            address: opt.address ?? opt.Address ?? '',
                            tags,
                            tagsLower: tags.map(tag => tag.toLowerCase())
                        };
                    }).filter(opt => opt.id)
                    : [];
                const preserved = new Set();
                formatted.forEach(opt => {
                    if (state.selected.has(String(opt.id))) {
                        preserved.add(String(opt.id));
                    }
                });
                state.options = formatted;
                state.selected = preserved;
                state.expandedTags = new Set(Array.from(state.expandedTags).filter(tag =>
                    formatted.some(opt => (opt.tags && opt.tags.includes(tag)) || (tag === 'Etiketsiz' && (!opt.tags || opt.tags.length === 0)))
                ));
                state.listMessage = formatted.length ? '' : (state.listMessage || 'Hedef bulunamadı.');
                renderList();
            },
            setSelected(ids){
                const next = new Set();
                (Array.isArray(ids) ? ids : []).forEach(id => {
                    const sid = String(id);
                    if (state.options.some(opt => String(opt.id) === sid)) {
                        next.add(sid);
                    }
                });
                state.selected = next;
                renderList();
                notifyChange();
            },
            clearSelection(){
                state.selected.clear();
                renderList();
                notifyChange();
            },
            getSelected(){
                return Array.from(state.selected);
            },
            setDisabled(disabled){
                state.disabled = !!disabled;
                if (toggleBtn) toggleBtn.disabled = state.disabled;
                if (filterInput) filterInput.disabled = state.disabled;
                if (state.disabled) {
                    closePanel();
                }
                updateSummary();
            },
            closePanel,
            setMaxSelectable(limit){
                if (typeof limit !== 'number' || limit <= 0) {
                    state.maxSelectable = Infinity;
                } else {
                    state.maxSelectable = limit;
                    if (limit === 1 && state.selected.size > 1) {
                        const first = Array.from(state.selected)[0];
                        state.selected = first ? new Set([first]) : new Set();
                    }
                }
                renderList();
            },
            describeTarget(id){
                const target = state.options.find(opt => String(opt.id) === String(id));
                if (!target) return `Hedef #${id}`;
                return target.name || `Hedef #${target.id}`;
            },
            isEmpty(){
                return state.options.length === 0;
            },
                setListMessage(msg){
                    state.listMessage = msg || '';
                    if (state.options.length === 0) {
                        renderList();
                    }
                },
                setFilter(value){
                    const tag = String(value || '').trim();
                    if (tag) state.expandedTags.add(tag);
                    renderList();
                },
            clearFilter(){
                state.filterText = '';
                if (filterInput) filterInput.value = '';
                state.expandedTags.clear();
                renderList();
            },
            setOnChange(callback){
                state.onChange = typeof callback === 'function' ? callback : null;
            }
        };
    }

    // Load all targets into dropdown (tüm hedefleri yükle)
    async function loadTargetsSelect(){
        if (targetPickerDataState.loaded && targetPickerDataState.options.length > 0) {
            targetPicker.setOptions(targetPickerDataState.options);
            return targetPickerDataState.options;
        }

        if (targetPickerDataState.loadingPromise) {
            return targetPickerDataState.loadingPromise;
        }

        if (targetPicker.isEmpty()) {
            targetPicker.setListMessage('Hedefler yükleniyor...');
        }

        targetPickerDataState.loadingPromise = (async () => {
            try {
                const allTargets = [];
                const seenIds = new Set();
                let offset = 0;
                let page = 0;
                let total = null;

                while (page < TARGET_PICKER_MAX_PAGES) {
                    const res = await fetch(`/api/targets?limit=${TARGET_PICKER_BATCH_SIZE}&offset=${offset}`, { headers: authHeaders() });
                    const data = await res.json().catch(() => ({}));
                    if (!res.ok) {
                        throw new Error(data.error || 'Hedef listesi alınamadı');
                    }

                    const targets = Array.isArray(data.targets) ? data.targets : (Array.isArray(data.data) ? data.data : []);
                    const pagination = data.pagination || {};
                    if (typeof pagination.total === 'number') {
                        total = pagination.total;
                    }

                    for (const t of targets) {
                        const id = t?.id ?? t?.ID;
                        const sid = String(id || '').trim();
                        if (!sid || seenIds.has(sid)) continue;
                        seenIds.add(sid);
                        allTargets.push({
                            id,
                            name: t.name ?? t.Name ?? '',
                            address: t.address ?? t.Address ?? '',
                            tags: t.tags ?? t.Tags ?? t.tags_list ?? ''
                        });
                    }

                    // İlk sayfayı hızlıca göster, uzun listelerde kullanıcıyı bekletme.
                    if (page === 0 && allTargets.length > 0) {
                        targetPicker.setOptions(allTargets);
                    }

                    const hasMoreByBackend = Boolean(pagination.has_more);
                    const hasMoreByTotal = typeof total === 'number' ? allTargets.length < total : false;
                    const effectiveLimit = typeof pagination.limit === 'number' && pagination.limit > 0
                        ? pagination.limit
                        : TARGET_PICKER_BATCH_SIZE;
                    const hasMoreByBatch = targets.length === effectiveLimit;
                    if (!hasMoreByBackend && !hasMoreByTotal && !hasMoreByBatch) {
                        break;
                    }
                    if (targets.length === 0) {
                        break;
                    }

                    offset += targets.length;
                    page += 1;

                    if (page % 4 === 0) {
                        // Büyük veri setlerinde main thread'i serbest bırak.
                        await new Promise(resolve => setTimeout(resolve, 0));
                    }
                }

                targetPickerDataState.options = allTargets;
                targetPickerDataState.loaded = true;
                targetPicker.setOptions(allTargets);
                return allTargets;
            } catch (e) {
                // Daha önce başarılı yükleme varsa onu koru.
                if (targetPickerDataState.options.length > 0) {
                    targetPicker.setOptions(targetPickerDataState.options);
                } else {
                    targetPicker.setOptions([]);
                    targetPicker.setListMessage('Hedef listesi alınamadı');
                }
                throw e;
            } finally {
                targetPickerDataState.loadingPromise = null;
            }
        })();

        return targetPickerDataState.loadingPromise;
    }


    const condFieldSelect = document.getElementById('rule-cond-field');
    if (condFieldSelect) {
        condFieldSelect.addEventListener('change', syncConditionControls);
        syncConditionControls();
    }
    const ruleTypeSelect = document.getElementById('rule-type');
    if (ruleTypeSelect) {
        ruleTypeSelect.addEventListener('change', syncEntityTypeUI);
    }
    const sensorFieldSelect = document.getElementById('sensor-field-select');
    if (sensorFieldSelect) {
        sensorFieldSelect.addEventListener('change', syncEntityTypeUI);
    }
    if (serviceModeAll) {
        serviceModeAll.addEventListener('change', () => {
            if (serviceModeAll.checked) {
                if (serviceModeMonitored) serviceModeMonitored.checked = false;
                setServiceMode('all', false);
                renderServiceList();
            } else if (!serviceModeMonitored?.checked) {
                setServiceMode('selected', true);
                renderServiceList();
            }
        });
    }
    if (serviceModeMonitored) {
        serviceModeMonitored.addEventListener('change', () => {
            if (serviceModeMonitored.checked) {
                if (serviceModeAll) serviceModeAll.checked = false;
                setServiceMode('monitored', false);
                renderServiceList();
            } else if (!serviceModeAll?.checked) {
                setServiceMode('selected', true);
                renderServiceList();
            }
        });
    }
    if (typeof targetPicker.setOnChange === 'function') {
        targetPicker.setOnChange((selected) => {
            const field = document.getElementById('rule-cond-field')?.value || '';
            if (field !== 'services') {
                return;
            }
            const targetId = Array.isArray(selected) && selected.length === 1 ? Number(selected[0]) : null;
            loadServicesForTarget(targetId);
        });
    }

    const channelSelectEl = document.getElementById('rule-channel');
    if (channelSelectEl) {
        channelSelectEl.addEventListener('change', () => {
            syncRecipientUI();
        });
        syncRecipientUI();
    }

    function authHeaders(){
        return { 'Authorization': 'Bearer ' + localStorage.getItem('token'), 'Content-Type': 'application/json' };
    }

    function extractConditionArray(raw){
        if (!raw) return [];
        if (Array.isArray(raw)) return raw;
        if (Array.isArray(raw?.conditions)) return raw.conditions;
        if (Array.isArray(raw?.data)) return raw.data;
        if (typeof raw === 'object') {
            if (raw.field) return [raw];
            const keys = Object.keys(raw || {}).filter(k => !Number.isNaN(Number(k)));
            if (keys.length > 0) {
                return keys.sort().map(k => raw[k]).filter(Boolean);
            }
        }
        return [];
    }

    const serviceState = {
        targetId: null,
        options: [],
        selected: new Set(),
        mode: 'monitored',
        loading: false,
        pendingIds: null,
        listMessage: 'Servis listesi yükleniyor...'
    };

    function normalizeServiceIds(ids){
        if (!Array.isArray(ids)) return [];
        return ids.map(id => Number(id)).filter(id => Number.isFinite(id) && id > 0);
    }

    function getServiceMode(){
        if (serviceModeAll?.checked) return 'all';
        if (serviceModeMonitored?.checked) return 'monitored';
        return 'selected';
    }

    function setServiceMode(mode, preserveSelection){
        serviceState.mode = mode || 'selected';
        if (serviceModeAll) serviceModeAll.checked = serviceState.mode === 'all';
        if (serviceModeMonitored) serviceModeMonitored.checked = serviceState.mode === 'monitored';
        if (!preserveSelection) {
            applyServiceModeSelection();
        }
    }

    function applyServiceModeSelection(){
        if (serviceState.mode === 'all') {
            serviceState.selected = new Set(serviceState.options.map(svc => String(svc.id)));
        } else if (serviceState.mode === 'monitored') {
            serviceState.selected = new Set(serviceState.options.filter(svc => svc.is_monitored).map(svc => String(svc.id)));
        }
    }

    function updateServiceHelper(){
        if (!serviceHelperEl) return;
        if (!serviceState.targetId) {
            serviceHelperEl.textContent = 'Servis listesi için tek hedef seçin.';
            return;
        }
        const total = serviceState.options.length;
        const selected = serviceState.selected.size;
        if (serviceState.mode === 'all') {
            serviceHelperEl.textContent = `Tüm servisler seçili (${selected}/${total}).`;
            return;
        }
        if (serviceState.mode === 'monitored') {
            serviceHelperEl.textContent = `Takipte olan servisler seçili (${selected}/${total}).`;
            return;
        }
        serviceHelperEl.textContent = `Seçilen servis sayısı: ${selected}/${total}.`;
    }

    function setServiceListMessage(message){
        serviceState.listMessage = message || 'Servis bulunamadı.';
        renderServiceList();
    }

    function renderServiceList(){
        if (!serviceListEl) return;
        serviceListEl.innerHTML = '';
        if (serviceState.loading) {
            const row = document.createElement('div');
            row.className = 'px-3 py-2 text-xs text-gray-500 dark:text-gray-400';
            row.textContent = serviceState.listMessage || 'Servis listesi yükleniyor...';
            serviceListEl.appendChild(row);
            return;
        }
        if (!serviceState.options.length) {
            const row = document.createElement('div');
            row.className = 'px-3 py-2 text-xs text-gray-500 dark:text-gray-400';
            row.textContent = serviceState.listMessage || 'Servis bulunamadı.';
            serviceListEl.appendChild(row);
            return;
        }
        serviceState.options.forEach(svc => {
            const row = document.createElement('label');
            row.className = 'flex items-start gap-3 px-3 py-2 cursor-pointer hover:bg-gray-50 dark:hover:bg-gray-800/60';
            const cb = document.createElement('input');
            cb.type = 'checkbox';
            cb.className = 'mt-1 rounded text-blue-600';
            cb.checked = serviceState.selected.has(String(svc.id));
            cb.addEventListener('change', () => {
                if (serviceState.mode !== 'selected') {
                    setServiceMode('selected', true);
                }
                if (cb.checked) {
                    serviceState.selected.add(String(svc.id));
                } else {
                    serviceState.selected.delete(String(svc.id));
                }
                updateServiceHelper();
            });
            const body = document.createElement('div');
            body.className = 'flex flex-col gap-1';
            const title = document.createElement('div');
            title.className = 'text-sm font-medium text-gray-800 dark:text-gray-100';
            title.textContent = svc.display_name || svc.service_name || `Servis #${svc.id}`;
            const subtitle = document.createElement('div');
            subtitle.className = 'flex flex-wrap items-center gap-2 text-xs text-gray-500 dark:text-gray-400';
            const code = document.createElement('span');
            code.textContent = svc.service_name || '-';
            subtitle.appendChild(code);
            const status = document.createElement('span');
            status.className = svc.status === 'running'
                ? 'px-2 py-0.5 rounded-full text-[11px] bg-emerald-50 text-emerald-700 dark:bg-emerald-500/10 dark:text-emerald-200'
                : (svc.status === 'stopped'
                    ? 'px-2 py-0.5 rounded-full text-[11px] bg-rose-50 text-rose-700 dark:bg-rose-500/10 dark:text-rose-200'
                    : 'px-2 py-0.5 rounded-full text-[11px] bg-gray-100 text-gray-600 dark:bg-gray-700/60 dark:text-gray-200');
            status.textContent = svc.status || 'unknown';
            subtitle.appendChild(status);
            if (svc.is_monitored) {
                const badge = document.createElement('span');
                badge.className = 'px-2 py-0.5 rounded-full text-[11px] bg-blue-50 text-blue-700 dark:bg-blue-500/10 dark:text-blue-200';
                badge.textContent = 'Takipte';
                subtitle.appendChild(badge);
            }
            body.appendChild(title);
            body.appendChild(subtitle);
            row.appendChild(cb);
            row.appendChild(body);
            serviceListEl.appendChild(row);
        });
        updateServiceHelper();
    }

    async function loadServicesForTarget(targetId){
        if (!serviceListEl) return;
        if (!targetId || !Number.isFinite(targetId) || targetId <= 0) {
            serviceState.targetId = null;
            serviceState.options = [];
            serviceState.selected = new Set();
            serviceState.loading = false;
            setServiceListMessage('Servis listesi için tek hedef seçin.');
            return;
        }
        if (serviceState.targetId !== targetId) {
            serviceState.selected = new Set();
            serviceState.pendingIds = null;
        }
        serviceState.targetId = targetId;
        serviceState.loading = true;
        setServiceListMessage('Servis listesi yükleniyor...');
        try {
            const res = await fetch(`/api/targets/${targetId}/device-services`, { headers: authHeaders() });
            const data = await res.json().catch(() => ({}));
            const list = Array.isArray(data?.services) ? data.services : [];
            serviceState.options = list.map(item => ({
                id: item.id,
                service_name: item.service_name || '',
                display_name: item.display_name || '',
                status: item.status || '',
                is_monitored: !!item.is_monitored
            }));
            serviceState.loading = false;
            if (serviceState.pendingIds) {
                const wanted = new Set(serviceState.pendingIds.map(String));
                serviceState.selected = new Set(serviceState.options.filter(svc => wanted.has(String(svc.id))).map(svc => String(svc.id)));
                serviceState.pendingIds = null;
            } else {
                applyServiceModeSelection();
            }
            renderServiceList();
        } catch (err) {
            serviceState.loading = false;
            serviceState.options = [];
            serviceState.selected = new Set();
            setServiceListMessage('Servis listesi alınamadı.');
        }
    }

    function parseServiceConditions(conds){
        const info = { ok: false, mode: '', ids: [] };
        const list = Array.isArray(conds) ? conds : [];
        list.forEach(c => {
            const field = (c?.field || c?.Field || '').toString().toLowerCase();
            if (field === 'service_mode' || field === 'service_filter') {
                info.mode = (c?.value ?? c?.Value ?? '').toString();
                info.ok = true;
            } else if (field === 'service_ids') {
                info.ids = normalizeServiceIds(c?.value ?? c?.Value ?? []);
                info.ok = true;
            } else if (field === 'services') {
                const val = c?.value ?? c?.Value;
                if (val && typeof val === 'object') {
                    info.mode = (val.mode || info.mode || '').toString();
                    info.ids = normalizeServiceIds(val.ids || info.ids || []);
                    info.ok = true;
                }
            }
        });
        if (!info.mode) {
            info.mode = info.ids.length ? 'selected' : '';
        }
        return info;
    }

    function getSelectedServiceIds(){
        return Array.from(serviceState.selected)
            .map(id => Number(id))
            .filter(id => Number.isFinite(id) && id > 0);
    }
    // --- Simple multi-select picker (camera serials, inventory) ---
    function createSimplePicker(prefix, defaultLabel) {
        const wrapperEl = document.getElementById(prefix);
        const toggleEl  = document.getElementById(prefix + '-toggle');
        const panelEl   = document.getElementById(prefix + '-panel');
        const labelEl   = document.getElementById(prefix + '-label');
        const listEl    = document.getElementById(prefix + '-list');
        const filterEl  = document.getElementById(prefix + '-filter');
        const counterEl = document.getElementById(prefix + '-counter');
        if (!wrapperEl || !toggleEl || !panelEl || !labelEl || !listEl) {
            return { setOptions(){}, getSelected(){ return []; }, autoSelectIfSingle(){}, clearSelection(){}, setMessage(){} };
        }
        const state = { options: [], selected: new Set(), message: null };
        let open = false;

        function render() {
            if (state.options.length === 0 && state.message) {
                listEl.innerHTML = `<div class="px-3 py-2 text-xs text-gray-400 dark:text-gray-500">${escapeHtml(state.message)}</div>`;
                return;
            }
            const q = filterEl ? filterEl.value.toLowerCase() : '';
            const visible = state.options.filter(o => !q || String(o.label).toLowerCase().includes(q));
            if (visible.length === 0) {
                listEl.innerHTML = `<div class="px-3 py-2 text-xs text-gray-400 dark:text-gray-500">${state.options.length === 0 ? 'Kayıt bulunamadı' : 'Sonuç yok'}</div>`;
            } else {
                listEl.innerHTML = visible.map(o => {
                    const sid = String(o.id);
                    const chk = state.selected.has(sid) ? ' checked' : '';
                    return `<label class="flex items-center gap-2 px-3 py-2 cursor-pointer hover:bg-gray-50 dark:hover:bg-gray-700/50 text-sm text-gray-800 dark:text-gray-200 select-none">
                        <input type="checkbox" class="rounded border-gray-300 dark:border-gray-600 text-blue-600 focus:ring-blue-500"${chk} data-pid="${escapeHtml(sid)}">
                        <span class="truncate">${escapeHtml(o.label)}</span>
                    </label>`;
                }).join('');
                listEl.querySelectorAll('input[type=checkbox]').forEach(cb => {
                    cb.addEventListener('change', () => {
                        if (cb.checked) state.selected.add(cb.dataset.pid);
                        else state.selected.delete(cb.dataset.pid);
                        updateLabel();
                    });
                });
            }
        }

        function updateLabel() {
            const sel = state.options.filter(o => state.selected.has(String(o.id)));
            if (sel.length === 0) {
                labelEl.textContent = defaultLabel;
                labelEl.classList.add('text-gray-400');
                labelEl.classList.remove('text-gray-900');
            } else {
                labelEl.textContent = sel.map(o => o.label).join(', ');
                labelEl.classList.remove('text-gray-400');
                labelEl.classList.add('text-gray-900');
            }
            if (counterEl) counterEl.textContent = state.selected.size > 0 ? `${state.selected.size} seçili` : '';
        }

        function openPanel() { panelEl.classList.remove('hidden'); open = true; render(); }
        function closePanel() { panelEl.classList.add('hidden'); open = false; }

        toggleEl.addEventListener('click', e => { e.stopPropagation(); open ? closePanel() : openPanel(); });
        document.addEventListener('click', e => { if (open && !wrapperEl.contains(e.target)) closePanel(); });
        if (filterEl) filterEl.addEventListener('input', render);

        return {
            setOptions(items) {
                state.options = items;
                state.selected.clear();
                state.message = null;
                updateLabel();
                if (open) render();
            },
            getSelected() {
                return state.options.filter(o => state.selected.has(String(o.id))).map(o => o.id);
            },
            autoSelectIfSingle() {
                if (state.options.length === 1) {
                    state.selected.add(String(state.options[0].id));
                    updateLabel();
                }
            },
            clearSelection() { state.selected.clear(); updateLabel(); },
            setSelected(ids) {
                state.selected.clear();
                ids.forEach(function(id) { state.selected.add(String(id)); });
                updateLabel();
                if (open) render();
            },
            setMessage(msg) {
                state.message = msg;
                listEl.innerHTML = `<div class="px-3 py-2 text-xs text-gray-400 dark:text-gray-500">${escapeHtml(msg)}</div>`;
            }
        };
    }

    let _cameraPicker = null;
    let _inventoryPicker = null;

    function ensurePickers() {
        if (!_cameraPicker) _cameraPicker = createSimplePicker('camera-picker', 'Kamera seçin...');
        if (!_inventoryPicker) _inventoryPicker = createSimplePicker('inventory-picker', 'Tüm cihazlar (seçim yapılmadı)');
    }

    async function loadSensorSerials() {
        const sel = document.getElementById('sensor-serial-select');
        if (!sel) return;
        try {
            const r = await fetch('/api/sensors', { headers: authHeaders() });
            if (!r.ok) throw new Error('HTTP ' + r.status);
            const data = await r.json();
            const sensors = data.sensors || [];
            if (sensors.length === 0) {
                sel.innerHTML = '<option value="">— Tüm sensörler —</option>';
                const warn = document.getElementById('sensor-no-serial-warn');
                if (warn) warn.classList.remove('hidden');
            } else {
                const warn = document.getElementById('sensor-no-serial-warn');
                if (warn) warn.classList.add('hidden');
                sel.innerHTML = '<option value="">— Tüm sensörler —</option>' +
                    sensors.map(s => `<option value="${escapeHtml(s.serial || '')}">${escapeHtml(s.serial || '')}</option>`).join('');
            }
        } catch {
            const s2 = document.getElementById('sensor-serial-select');
            if (s2) s2.innerHTML = '<option value="">— Tüm sensörler —</option>';
        }
    }

    async function loadLiquidSensorSerials() {
        const sel = document.getElementById('liquid-serial-select');
        if (!sel) return;
        try {
            const r = await fetch('/api/liquid-sensors', { headers: authHeaders() });
            if (!r.ok) throw new Error('HTTP ' + r.status);
            const data = await r.json();
            const serials = data.liquid_sensors || [];
            sel.innerHTML = '<option value="">— Tümü —</option>' +
                serials.map(s => `<option value="${escapeHtml(s)}">${escapeHtml(s)}</option>`).join('');
        } catch {
            const s2 = document.getElementById('liquid-serial-select');
            if (s2) s2.innerHTML = '<option value="">— Tümü —</option>';
        }
    }

    async function loadCameraSerials() {
        ensurePickers();
        try {
            const r = await fetch('/api/cameras', { headers: authHeaders() });
            if (!r.ok) throw new Error('HTTP ' + r.status);
            const data = await r.json();
            const cameras = data.cameras || [];
            if (cameras.length === 0) {
                _cameraPicker.setOptions([]);
                _cameraPicker.setMessage('Bu cihaza kayıtlı kamera bulunamadı');
                return;
            }
            _cameraPicker.setOptions(cameras.map(c => ({ id: c.serial || c.id || '', label: c.serial || c.name || c.id || '' })));
            _cameraPicker.autoSelectIfSingle();
        } catch (err) {
            if (_cameraPicker) _cameraPicker.setMessage('Kameralar yüklenemedi');
        }
    }

    async function loadInventoryForCrack() {
        ensurePickers();
        try {
            const r = await fetch('/api/inventory?limit=500', { headers: authHeaders() });
            if (!r.ok) throw new Error('HTTP ' + r.status);
            const data = await r.json();
            const items = data.items || data.inventory || data.data || [];
            if (items.length === 0) {
                _inventoryPicker.setOptions([]);
                _inventoryPicker.setMessage('Kayıtlı cihaz bulunamadı');
                return;
            }
            _inventoryPicker.setOptions(items.map(it => ({
                id: it.id,
                label: it.asset_name || it.hostname || it.ip_address || ('#' + it.id)
            })));
        } catch (err) {
            if (_inventoryPicker) _inventoryPicker.setMessage('Cihazlar yüklenemedi');
        }
    }

    function syncEntityTypeUI(){
        const typeEl = document.getElementById('rule-type');
        const entityType = typeEl ? typeEl.value : 'target';
        const isTarget = entityType === 'target';
        const isAirSensor = entityType === 'air_sensor';
        const isLiquidSensor = entityType === 'liquid_sensor';
        const isCamera = entityType === 'camera';
        const isCrack = entityType === 'crack_detection';
        const targetSection = document.getElementById('create-target-section');
        const condTargetFields = document.getElementById('cond-target-fields');
        const sensorSection = document.getElementById('create-sensor-section');
        const liquidSection = document.getElementById('create-liquid-section');
        const cameraSection = document.getElementById('create-camera-section');
        const crackSection = document.getElementById('create-crack-section');

        if (targetSection) targetSection.style.display = isTarget ? '' : 'none';
        if (condTargetFields) condTargetFields.style.display = isTarget ? '' : 'none';
        if (sensorSection) sensorSection.classList.toggle('hidden', !isAirSensor);
        if (liquidSection) liquidSection.classList.toggle('hidden', !isLiquidSensor);
        if (cameraSection) cameraSection.classList.toggle('hidden', !isCamera);
        if (crackSection) crackSection.classList.toggle('hidden', !isCrack);
    }
    function syncConditionControls(){
        const fieldEl = document.getElementById('rule-cond-field');
        const statusWrap = document.getElementById('cond-status-wrap');
        const servicesWrap = document.getElementById('cond-services-wrap');
        const rtWrap = document.getElementById('cond-rt-wrap');
        const rtHelp = document.getElementById('cond-rt-help');
        const metricNote = document.getElementById('cond-metric-note');
        const valueLabel = document.getElementById('cond-value-label');
        const opEl = document.getElementById('rule-cond-operator');
        const valEl = document.getElementById('rule-cond-value');
        if (!fieldEl) return;
        const field = fieldEl.value;
        const isMetricField = field === 'cpu_percent' || field === 'ram_gb' || field === 'disk_gb' || field === 'temperature_c';
        const needsValue = field === 'response_time_ms' || isMetricField;
        if (statusWrap) statusWrap.classList.toggle('is-active', field === 'status');
        if (servicesWrap) servicesWrap.classList.toggle('is-active', field === 'services');
        if (rtWrap) rtWrap.classList.toggle('is-active', needsValue);
        if (rtHelp) rtHelp.classList.toggle('is-active', needsValue);
        if (metricNote) metricNote.classList.toggle('is-active', isMetricField);
        if (valueLabel) {
            if (field === 'response_time_ms') {
                valueLabel.textContent = 'Değer (ms)';
            } else if (field === 'cpu_percent') {
                valueLabel.textContent = 'Değer (%)';
            } else if (field === 'ram_gb') {
                valueLabel.textContent = 'Değer (GB)';
            } else if (field === 'disk_gb') {
                valueLabel.textContent = 'Değer (GB)';
            } else if (field === 'temperature_c') {
                valueLabel.textContent = 'Değer (°C)';
            } else {
                valueLabel.textContent = 'Değer';
            }
        }
        if (rtHelp) {
            if (field === 'response_time_ms') {
                rtHelp.textContent = 'Örnek: Yanıt süresi (ms) < 200';
            } else if (field === 'cpu_percent') {
                rtHelp.textContent = 'Örnek: CPU (%) > 85';
            } else if (field === 'ram_gb') {
                rtHelp.textContent = 'Örnek: RAM (GB) > 8';
            } else if (field === 'disk_gb') {
                rtHelp.textContent = 'Örnek: Disk (GB) > 200';
            } else if (field === 'temperature_c') {
                rtHelp.textContent = 'Örnek: Sıcaklık (°C) > 70';
            } else {
                rtHelp.textContent = '';
            }
        }
        const enableRT = needsValue;
        if (opEl) opEl.disabled = !enableRT;
        if (valEl) {
            valEl.disabled = !enableRT;
            if (field === 'response_time_ms') {
                valEl.placeholder = '200';
            } else if (field === 'cpu_percent') {
                valEl.placeholder = '85';
            } else if (field === 'ram_gb') {
                valEl.placeholder = '8';
            } else if (field === 'disk_gb') {
                valEl.placeholder = '200';
            } else if (field === 'temperature_c') {
                valEl.placeholder = '70';
            } else {
                valEl.placeholder = '';
            }
        }
        if (field === 'services') {
            if (serviceModeMonitored && serviceModeAll && !serviceModeMonitored.checked && !serviceModeAll.checked) {
                setServiceMode('monitored', false);
            }
            targetPicker.setMaxSelectable(1);
            const selected = targetPicker.getSelected();
            if (selected.length > 1) {
                targetPicker.setSelected([selected[0]]);
            }
            const targetId = selected.length === 1 ? Number(selected[0]) : null;
            loadServicesForTarget(targetId);
        } else {
            targetPicker.setMaxSelectable(Infinity);
        }
    }

    function syncRecipientUI(){
        const channelSel = document.getElementById('rule-channel');
        const helper = document.getElementById('rule-recipients-help');
        const input = document.getElementById('rule-recipients');
        if (!channelSel || !helper || !input) return;
        const channel = (channelSel.value || 'email').toLowerCase();
        if (channel === 'telegram') {
            helper.textContent = 'Telegram için chat_id değerlerini virgülle ayırın.';
            input.placeholder = '123456789, 987654321';
        } else {
            helper.textContent = 'E-posta için adresleri virgülle ayırın.';
            input.placeholder = 'user@example.com, second@example.com';
        }
    }

    async function loadSettings(){
        // This function is deprecated - settings are now managed via new_notification_config table
        // No longer needed as we use per-channel configuration
        console.log('loadSettings() is deprecated and no longer used');
    }
    function setMsg(t){
        const message = String(t || '').trim();
        if (!message) return;

        const m = document.getElementById('settings-message');
        if (m) m.textContent = message;

        const lower = message.toLowerCase();
        let type = 'info';
        if (
            lower.includes('hata') ||
            lower.includes('oluşmadı') ||
            lower.includes('olmadı') ||
            lower.includes('silinemedi') ||
            lower.includes('kaydedilemedi') ||
            lower.includes('güncellenemedi')
        ) {
            type = 'error';
        } else if (
            lower.includes('lütfen') ||
            lower.includes('gerekli') ||
            lower.includes('seçin') ||
            lower.includes('uyarı')
        ) {
            type = 'warning';
        } else if (
            lower.includes('başarı') ||
            lower.includes('oluşturuldu') ||
            lower.includes('kaydedildi') ||
            lower.includes('güncellendi') ||
            lower.includes('silindi')
        ) {
            type = 'success';
        }

        if (window.Toast && typeof window.Toast.show === 'function') {
            window.Toast.show({ type, message });
            return;
        }
        if (window.app && typeof window.app.showToast === 'function') {
            window.app.showToast(type, message);
        }
    }

    function collectConditions(){
        const typeEl = document.getElementById('rule-type');
        const entityType = typeEl ? typeEl.value : 'target';

        if (entityType === 'air_sensor') {
            const sensorField = document.getElementById('sensor-field-select')?.value || 'temperature';
            const op = document.getElementById('sensor-op-select')?.value || '<';
            const valStr = document.getElementById('sensor-val-input')?.value ?? '';
            const val = Number(valStr);
            if (valStr === '' || isNaN(val)) {
                setMsg('Sensör eşik değeri için geçerli bir sayı girin.');
                return null;
            }
            return [
                { field: 'sensor_subtype', operator: '=', value: 'air' },
                { field: sensorField, operator: op, value: val }
            ];
        }

        if (entityType === 'liquid_sensor') {
            return [{ field: 'liquid_contact', operator: '=', value: true }];
        }

        if (entityType === 'camera') {
            const selected = _cameraPicker ? _cameraPicker.getSelected().map(String).filter(Boolean) : [];
            if (selected.length === 0) { setMsg('En az bir kamera seçin.'); return null; }
            return [
                { field: 'sensor_subtype', operator: '=', value: 'camera' },
                { field: 'camera_serials', operator: 'in', value: selected }
            ];
        }

        if (entityType === 'crack_detection') {
            const severity = document.getElementById('crack-severity-select')?.value || 'any';
            const invIDs = _inventoryPicker ? _inventoryPicker.getSelected().map(n => parseInt(n, 10)).filter(n => !isNaN(n) && n > 0) : [];
            const conditions = [{ field: 'min_severity', operator: '=', value: severity }];
            if (invIDs.length > 0) conditions.push({ field: 'inventory_ids', operator: 'in', value: invIDs });
            return conditions;
        }

        const fieldEl = document.getElementById('rule-cond-field');
        const field = fieldEl ? fieldEl.value : '';
        if (field === 'response_time_ms' || field === 'cpu_percent' || field === 'ram_gb' || field === 'disk_gb' || field === 'temperature_c') {
            const opEl = document.getElementById('rule-cond-operator');
            const valEl = document.getElementById('rule-cond-value');
            const op = opEl?.value || '<';
            const valStr = valEl?.value ?? '';
            const val = Number(valStr);
            if (Number.isNaN(val)) {
                const msg = field === 'response_time_ms'
                    ? 'Yanıt süresi için geçerli bir sayı girin.'
                    : 'Değer için geçerli bir sayı girin.';
                setMsg(msg);
                if (valEl) valEl.focus();
                return null;
            }
            return [{ field: field, operator: op, value: val }];
        }
        if (field === 'status') {
            // Return empty conditions array for automatic online/offline notifications
            return [];
        }
        if (field === 'services') {
            const selectedTargets = targetPicker.getSelected();
            if (!selectedTargets || selectedTargets.length !== 1) {
                setMsg('Servis bildirimi için tek hedef seçmelisiniz.');
                return null;
            }
            const mode = getServiceMode();
            const ids = getSelectedServiceIds();
            if (mode === 'selected' && ids.length === 0) {
                setMsg('Lütfen en az bir servis seçin.');
                return null;
            }
            return [
                { field: 'service_mode', operator: '=', value: mode },
                { field: 'service_ids', operator: 'in', value: ids }
            ];
        }
        return [];
    }
    async function saveSettings(){
        // This function is deprecated - settings are now managed via SMTP configuration
        // and per-rule configuration in notification_settings.html
        console.log('saveSettings() is deprecated - use SMTP config or per-rule settings');
        setMsg('Bu ayar artık kullanılmıyor. SMTP yapılandırması veya bildirim kurallarını kullanın.');
    }
    async function sendTest(){
        const recipient = (document.getElementById('test-recipient')?.value || '').trim();
        if (!recipient) { setMsg('Test için bir alıcı e-posta girin.'); return; }
        try {
            const res = await fetch('/api/new-notifications/test-email', { method:'POST', headers: authHeaders(), body: JSON.stringify({ recipient })});
            const data = await res.json().catch(()=>({}));
            setMsg(res.ok && (data.success === undefined || data.success) ? 'Test eâ€‘posta gönderildi.' : (data.error || 'Test gönderilemedi'));
        } catch(e){ setMsg('Test gönderilemedi: ' + e.message); }
    }
    function openModal(){
        const m = document.getElementById('create-modal');
        if (m){
            m.classList.remove('hidden');
            m.classList.add('flex');
        }
        // Set default title/button for create
        const titleEl = document.querySelector('#create-modal h3');
        
        if (titleEl) titleEl.textContent = 'Bildirim Oluştur';
        const submitEl = document.getElementById('btn-submit-create');
        if (submitEl) { submitEl.textContent = 'Oluştur'; submitEl.onclick = submitRule; }
        // Ensure dropdown-only target selection is active on open
        if (typeof loadTargetsSelect === 'function') loadTargetsSelect();
        const nm = document.getElementById('rule-name'); if (nm) nm.value = '';
        const ruleTypeEl = document.getElementById('rule-type'); if (ruleTypeEl) ruleTypeEl.value = 'target';
        const ch = document.getElementById('rule-channel'); if (ch) ch.value = 'email';
        const iv = document.getElementById('rule-interval'); if (iv) iv.value = '10';
        const tp = document.getElementById('rule-template'); if (tp) tp.value = '';
        const rc = document.getElementById('rule-recipients'); if (rc) rc.value = '';
        const condField = document.getElementById('rule-cond-field');
        const condStatus = document.getElementById('rule-cond-status');
        const condOp = document.getElementById('rule-cond-operator');
        const condVal = document.getElementById('rule-cond-value');
        if (condField) condField.value = 'status';
        if (condStatus) condStatus.value = 'fail';
        if (condOp) { condOp.value = '<'; condOp.disabled = true; }
        if (condVal) { condVal.value = ''; condVal.disabled = true; }
        if (serviceModeAll) serviceModeAll.checked = false;
        if (serviceModeMonitored) serviceModeMonitored.checked = true;
        serviceState.mode = 'monitored';
        serviceState.options = [];
        serviceState.selected = new Set();
        serviceState.targetId = null;
        serviceState.pendingIds = null;
        serviceState.loading = false;
        setServiceListMessage('Servis listesi için tek hedef seçin.');
        syncConditionControls();
        syncRecipientUI();
        syncEntityTypeUI();
        // Sensör/kamera/envanter picker'larını yükle
        loadSensorSerials();
        loadLiquidSensorSerials();
        loadCameraSerials();
        loadInventoryForCrack();
        targetPicker.setDisabled(false);
        targetPicker.setMaxSelectable(MAX_NOTIFICATION_TARGETS);
        targetPicker.clearFilter();
        targetPicker.clearSelection();
        targetPicker.closePanel();
    }
    function closeModal(){
        const m = document.getElementById('create-modal');
        if (m){ m.classList.add('hidden'); m.classList.remove('flex'); }
        // Reset modal to default create state
        const titleEl = document.querySelector('#create-modal h3');
        
        if (titleEl) titleEl.textContent = 'Bildirim Oluştur';
        const submitEl = document.getElementById('btn-submit-create');
        if (submitEl){ submitEl.textContent = 'Oluştur'; submitEl.onclick = submitRule; }

        targetPicker.setDisabled(false);
        targetPicker.setMaxSelectable(MAX_NOTIFICATION_TARGETS);
        targetPicker.clearFilter();
        targetPicker.clearSelection();
        targetPicker.closePanel();
    }
    async function submitRule(){
        const name = (document.getElementById('rule-name')?.value || '').trim();
        const selectedTargets = targetPicker.getSelected();
        const entityType = document.getElementById('rule-type')?.value || 'target';
        const channel = (document.getElementById('rule-channel')?.value || 'email').toLowerCase();
        const interval = parseInt(document.getElementById('rule-interval')?.value || '10', 10);
        const templateVal = (document.getElementById('rule-template')?.value || '').trim();
        const recipientsStr = document.getElementById('rule-recipients')?.value || '';
        const recipients = recipientsStr.split(',').map(s => s.trim()).filter(Boolean);
        if (!name){ setMsg('Lütfen bildirim adı girin'); return; }
        if (recipients.length === 0){ setMsg(channel === 'telegram' ? 'Lütfen en az bir chat_id girin' : 'Lütfen alıcıları girin'); return; }

        const needsTarget = entityType === 'target';
        if (needsTarget && selectedTargets.length === 0){ setMsg('Lütfen en az bir hedef seçin'); return; }
        if (needsTarget && selectedTargets.length > MAX_NOTIFICATION_TARGETS){ setMsg(`En fazla ${MAX_NOTIFICATION_TARGETS} hedef seçebilirsiniz.`); return; }
        // Kamera, hava ve sıvı sensörü: entity_type DB'de 'sensor' olarak saklanır
        const dbEntityType = (entityType === 'camera' || entityType === 'air_sensor' || entityType === 'liquid_sensor') ? 'sensor' : entityType;

        const conditions = collectConditions();
        if (!conditions) return;

        let templateId = null;
        if (templateVal && templateVal !== '') {
            const parsed = parseInt(templateVal, 10);
            if (!isNaN(parsed) && parsed > 0) {
                templateId = parsed;
            }
        }

        const targetIds = needsTarget
            ? selectedTargets.map(id => Number(id)).filter(id => Number.isFinite(id) && id > 0)
            : [];

        const payload = {
            name,
            entity_type: dbEntityType,
            channel,
            schedule_interval_minutes: interval,
            recipients,
            template_id: templateId,
            is_active: true,
            conditions,
            target_ids: targetIds
        };
        if (targetIds.length === 1) {
            payload.target_id = targetIds[0];
        }

        // Sensör tipi: seri numarasını ekle
        if (entityType === 'air_sensor') {
            const serialSel = document.getElementById('sensor-serial-select');
            const serial = (serialSel ? serialSel.value : '') || '';
            if (serial) payload.sensor_serial = serial;
        }
        if (entityType === 'liquid_sensor') {
            const serialSel = document.getElementById('liquid-serial-select');
            const serial = (serialSel ? serialSel.value : '') || '';
            if (serial) payload.sensor_serial = serial;
        }

        try {
            const res = await fetch('/api/new-notifications/rules', {
                method: 'POST',
                headers: authHeaders(),
                body: JSON.stringify(payload)
            });
            const data = await res.json().catch(() => ({}));
            if (res.ok && (data.success === undefined || data.success)) {
                setMsg('Bildirim kuralı oluşturuldu');
                closeModal();
                if (typeof loadRules === 'function') loadRules();
            } else {
                setMsg(data.error || 'Bildirim oluşturulamadı');
            }
        } catch (error) {
            setMsg(error.message || 'Bildirim oluşturulamadı');
        }
    }
function ruleChannelBadge(ch) {
  const c = String(ch || '').toLowerCase();
  if (c === 'email') {
    return '<span class="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-xs font-medium bg-blue-50 text-blue-700 dark:bg-blue-500/10 dark:text-blue-300">'
         + '<i class="fas fa-envelope"></i> E-mail'
         + '</span>';
  }
  if (c === 'telegram') {
    return '<span class="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-xs font-medium bg-sky-50 text-sky-700 dark:bg-sky-500/10 dark:text-sky-300">'
         + '<i class="fab fa-telegram-plane"></i> Telegram'
         + '</span>';
  }
  if (c === 'webpush' || c === 'push') {
    return '<span class="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-xs font-medium bg-violet-50 text-violet-700 dark:bg-violet-500/10 dark:text-violet-300">'
         + '<i class="fas fa-bell"></i> Push'
         + '</span>';
  }
  return '<span class="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-xs font-medium bg-gray-100 text-gray-700 dark:bg-gray-700/60 dark:text-gray-200">'
       + '<i class="fas fa-circle"></i> ' + escapeHtml(c || '-') + '</span>';
}

function ruleIntervalBadge(mins) {
  const n = Number(mins || 0);
  const text = isNaN(n) ? '-' : (n + ' dk');
  return '<span class="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-xs font-medium bg-amber-50 text-amber-700 dark:bg-amber-500/10 dark:text-amber-300">'
       + '<i class="fas fa-clock"></i> ' + text + '</span>';
}

function ruleActiveBadge(flag) {
  return flag
    ? '<span class="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-xs font-semibold bg-emerald-50 text-emerald-700 dark:bg-emerald-500/10 dark:text-emerald-300">'
        + '<span class="w-2 h-2 rounded-full bg-emerald-500 inline-block"></span> Aktif</span>'
    : '<span class="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-xs font-semibold bg-rose-50 text-rose-700 dark:bg-rose-500/10 dark:text-rose-300">'
        + '<span class="w-2 h-2 rounded-full bg-rose-500 inline-block"></span> Pasif</span>';
}

function ruleRecipientsBadge(count) {
  const n = Number(count || 0);
  return '<span class="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-xs font-medium bg-indigo-50 text-indigo-700 dark:bg-indigo-500/10 dark:text-indigo-300">'
       + '<i class="fas fa-user-friends"></i> ' + (isNaN(n) ? '0' : String(n)) + '</span>';
}

const targetPopoverTexts = {
  title: 'Seçili Hedefler',
  loading: 'Hedefler yükleniyor...',
  empty: 'Hedef bilgisi bulunamadı.',
  error: 'Hedefler getirilemedi.',
  openLabel: 'Hedefleri görüntüle'
};

const servicePopoverTexts = {
  title: 'Servisler',
  loading: 'Servisler yükleniyor...',
  empty: 'Servis bilgisi bulunamadı.',
  error: 'Servisler getirilemedi.',
  openLabel: 'Servisleri görüntüle'
};

const targetDetailsCache = new Map();
let activeTargetsPopover = null;
let activeTargetsPopoverAnchor = null;
let activeTargetsPopoverHandler = null;

const serviceDetailsCache = new Map();
let activeServicesPopover = null;
let activeServicesPopoverAnchor = null;
let activeServicesPopoverHandler = null;

function parseTargetIds(value) {
  if (Array.isArray(value)) {
    return value
      .map(function (val) { return Number(val); })
      .filter(function (num) { return Number.isFinite(num) && num > 0; });
  }
  if (typeof value === 'string') {
    return value
      .split(',')
      .map(function (part) { return Number(part.trim()); })
      .filter(function (num) { return Number.isFinite(num) && num > 0; });
  }
  if (typeof value === 'number' && Number.isFinite(value) && value > 0) {
    return [value];
  }
  return [];
}

function closeTargetsPopover() {
  if (activeTargetsPopover) {
    activeTargetsPopover.remove();
    activeTargetsPopover = null;
  }
  if (activeTargetsPopoverHandler) {
    document.removeEventListener('click', activeTargetsPopoverHandler, true);
    activeTargetsPopoverHandler = null;
  }
  activeTargetsPopoverAnchor = null;
}

function positionTargetsPopover(popover, anchor) {
  if (!popover || !anchor) return;
  const rect = anchor.getBoundingClientRect();
  const top = window.scrollY + rect.bottom + 8;
  const initialLeft = window.scrollX + rect.left;
  const width = popover.offsetWidth || 0;
  let left = initialLeft;
  const viewportRight = window.scrollX + window.innerWidth;
  if (left + width > viewportRight - 12) {
    left = viewportRight - width - 12;
  }
  if (left < 12) {
    left = 12;
  }
  popover.style.top = top + 'px';
  popover.style.left = left + 'px';
}

async function fetchTargetsByIds(ids) {
  const normalized = Array.from(new Set(parseTargetIds(ids)));
  if (normalized.length === 0) {
    return [];
  }
  const missing = normalized.filter(function (id) { return !targetDetailsCache.has(id); });
  if (missing.length) {
    await Promise.all(missing.map(async function (id) {
      try {
        const res = await fetch('/api/targets/' + id, { headers: authHeaders() });
        if (!res.ok) throw new Error('Failed to fetch target');
        const data = await res.json().catch(() => ({}));
        const targetData = data?.data || data?.target || data || {};
        const info = {
          id: id,
          name: targetData.name || `Hedef #${id}`,
          address: targetData.address || ''
        };
        targetDetailsCache.set(id, info);
        return info;
      } catch (err) {
        const fallback = { id: id, name: `Hedef #${id}`, address: '' };
        targetDetailsCache.set(id, fallback);
        return fallback;
      }
    }));
  }
  return normalized.map(function (id) {
    return targetDetailsCache.get(id) || { id: id, name: `Hedef #${id}`, address: '' };
  });
}

function buildTargetsListHTML(targets) {
  if (!Array.isArray(targets) || targets.length === 0) {
    return '<p class="text-sm text-gray-500 dark:text-gray-400">' + targetPopoverTexts.empty + '</p>';
  }
  // max-h-80 = yaklaşık 4 hedefin yüksekliği, scroll ile kaydırılabilir
  return '<div class="space-y-2 max-h-80 overflow-y-auto pr-1">'
    + targets.map(function (target) {
      const safeName = escapeHtml(target.name || `Hedef #${target.id}`);
      const subtitleParts = [];
      if (target.address) {
        subtitleParts.push(escapeHtml(target.address));
      }
      return '<div class="border border-gray-100 dark:border-gray-700 rounded-md px-3 py-2 bg-gray-50 dark:bg-gray-800/60">'
        + '<div class="text-sm font-medium text-gray-900 dark:text-gray-100">' + safeName + '</div>'
        + '<div class="text-xs text-gray-500 dark:text-gray-400">' + subtitleParts.join(' • ') + '</div>'
        + '</div>';
    }).join('')
    + '</div>';
}

function showTargetsPopover(anchor, ids) {
  if (!anchor) return;
  closeTargetsPopover();
  const popover = document.createElement('div');
  popover.className = 'target-popover absolute z-50 w-72 max-w-sm rounded-lg shadow-xl border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800';
  popover.innerHTML = ''
    + '<div class="flex items-center justify-between px-4 py-2 border-b border-gray-100 dark:border-gray-700">'
    + '<span class="text-sm font-semibold text-gray-900 dark:text-gray-100">' + targetPopoverTexts.title + '</span>'
    + '<button type="button" class="text-gray-400 hover:text-gray-600 dark:text-gray-400 dark:hover:text-gray-200 focus:outline-none" data-popover-close="1">'
    + '<i class="fas fa-times"></i>'
    + '</button>'
    + '</div>'
    + '<div class="target-popover-body px-4 py-3 text-sm text-gray-600 dark:text-gray-300">'
    + targetPopoverTexts.loading
    + '</div>';
  document.body.appendChild(popover);
  positionTargetsPopover(popover, anchor);
  activeTargetsPopover = popover;
  activeTargetsPopoverAnchor = anchor;

  const closeBtn = popover.querySelector('[data-popover-close]');
  if (closeBtn) {
    closeBtn.addEventListener('click', function (event) {
      event.preventDefault();
      closeTargetsPopover();
    });
  }

  const outsideHandler = function (event) {
    if (popover.contains(event.target) || anchor.contains(event.target)) {
      return;
    }
    closeTargetsPopover();
  };
  setTimeout(function () {
    document.addEventListener('click', outsideHandler, true);
  }, 0);
  activeTargetsPopoverHandler = outsideHandler;

  fetchTargetsByIds(ids).then(function (details) {
    if (!activeTargetsPopover || activeTargetsPopover !== popover) return;
    const body = popover.querySelector('.target-popover-body');
    if (!body) return;
    body.innerHTML = buildTargetsListHTML(details);
    positionTargetsPopover(popover, anchor);
  }).catch(function () {
    if (!activeTargetsPopover || activeTargetsPopover !== popover) return;
    const body = popover.querySelector('.target-popover-body');
    if (body) {
      body.innerHTML = '<p class="text-sm text-red-500">' + targetPopoverTexts.error + '</p>';
    }
  });
}

function wrapTargetChipForPopover(html, ids) {
  const normalized = parseTargetIds(ids);
  if (normalized.length === 0) return html;
  const dataAttr = normalized.join(',');
  return '<button type="button" class="target-chip-trigger inline-flex items-center gap-1 focus:outline-none"'
    + ' data-target-ids="' + dataAttr + '"'
    + ' aria-label="' + targetPopoverTexts.openLabel + '"'
    + ' title="' + targetPopoverTexts.openLabel + '"'
    + ' style="background:transparent;border:none;padding:0;margin:0;">'
    + html
    + '<i class="fas fa-chevron-down text-[10px] text-gray-500 dark:text-gray-400 ml-1"></i>'
    + '</button>';
}

function bindTargetChipEvents(container) {
  if (!container) return;
  container.querySelectorAll('.target-chip-trigger').forEach(function (btn) {
    btn.addEventListener('click', function (event) {
      event.preventDefault();
      event.stopPropagation();
      const ids = parseTargetIds(btn.dataset.targetIds || '');
      if (ids.length === 0) return;
      if (activeTargetsPopoverAnchor === btn) {
        closeTargetsPopover();
        return;
      }
      showTargetsPopover(btn, ids);
    });
  });
}

function closeServicesPopover() {
  if (activeServicesPopover) {
    activeServicesPopover.remove();
    activeServicesPopover = null;
  }
  if (activeServicesPopoverHandler) {
    document.removeEventListener('click', activeServicesPopoverHandler, true);
    activeServicesPopoverHandler = null;
  }
  activeServicesPopoverAnchor = null;
}

function positionServicesPopover(popover, anchor) {
  if (!popover || !anchor) return;
  const rect = anchor.getBoundingClientRect();
  const top = window.scrollY + rect.bottom + 8;
  const initialLeft = window.scrollX + rect.left;
  const width = popover.offsetWidth || 0;
  let left = initialLeft;
  const viewportRight = window.scrollX + window.innerWidth;
  if (left + width > viewportRight - 12) {
    left = viewportRight - width - 12;
  }
  if (left < 12) {
    left = 12;
  }
  popover.style.top = top + 'px';
  popover.style.left = left + 'px';
}

function parseServiceIds(value) {
  if (Array.isArray(value)) {
    return value
      .map(function (val) { return Number(val); })
      .filter(function (num) { return Number.isFinite(num) && num > 0; });
  }
  if (typeof value === 'string') {
    return value
      .split(',')
      .map(function (part) { return Number(part.trim()); })
      .filter(function (num) { return Number.isFinite(num) && num > 0; });
  }
  if (typeof value === 'number' && Number.isFinite(value) && value > 0) {
    return [value];
  }
  return [];
}

async function fetchServicesByTarget(targetId) {
  const id = Number(targetId || 0);
  if (!Number.isFinite(id) || id <= 0) return [];
  if (serviceDetailsCache.has(id)) {
    return serviceDetailsCache.get(id) || [];
  }
  try {
    const res = await fetch('/api/targets/' + id + '/device-services', { headers: authHeaders() });
    if (!res.ok) throw new Error('Failed to fetch services');
    const data = await res.json().catch(() => ({}));
    const list = Array.isArray(data?.services) ? data.services : [];
    const normalized = list.map(function (svc) {
      return {
        id: svc.id,
        display_name: svc.display_name || '',
        service_name: svc.service_name || '',
        status: svc.status || '',
        is_monitored: !!svc.is_monitored
      };
    });
    serviceDetailsCache.set(id, normalized);
    return normalized;
  } catch (err) {
    return [];
  }
}

function filterServices(list, mode, ids) {
  const safeList = Array.isArray(list) ? list : [];
  const normalizedMode = (mode || '').toLowerCase();
  if (normalizedMode === 'all') return safeList;
  if (normalizedMode === 'monitored') {
    return safeList.filter(function (svc) { return svc.is_monitored; });
  }
  const idSet = new Set(parseServiceIds(ids).map(String));
  if (idSet.size === 0) return [];
  return safeList.filter(function (svc) { return idSet.has(String(svc.id)); });
}

function buildServicesListHTML(services) {
  if (!Array.isArray(services) || services.length === 0) {
    return '<p class="text-sm text-gray-500 dark:text-gray-400">' + servicePopoverTexts.empty + '</p>';
  }
  return '<div class="space-y-2 max-h-80 overflow-y-auto pr-1">'
    + services.map(function (svc) {
      const name = escapeHtml(svc.display_name || svc.service_name || 'Servis');
      const code = escapeHtml(svc.service_name || '-');
      const statusClass = svc.status === 'running'
        ? 'bg-emerald-50 text-emerald-700 dark:bg-emerald-500/10 dark:text-emerald-200'
        : (svc.status === 'stopped'
          ? 'bg-rose-50 text-rose-700 dark:bg-rose-500/10 dark:text-rose-200'
          : 'bg-gray-100 text-gray-600 dark:bg-gray-700/60 dark:text-gray-200');
      const statusText = escapeHtml(svc.status || 'unknown');
      const monitoredBadge = svc.is_monitored
        ? '<span class="px-2 py-0.5 rounded-full text-[11px] bg-blue-50 text-blue-700 dark:bg-blue-500/10 dark:text-blue-200">Takipte</span>'
        : '';
      return '<div class="border border-gray-100 dark:border-gray-700 rounded-md px-3 py-2 bg-gray-50 dark:bg-gray-800/60">'
        + '<div class="text-sm font-medium text-gray-900 dark:text-gray-100">' + name + '</div>'
        + '<div class="flex flex-wrap items-center gap-2 text-xs text-gray-500 dark:text-gray-400">'
        + '<span>' + code + '</span>'
        + '<span class="px-2 py-0.5 rounded-full text-[11px] ' + statusClass + '">' + statusText + '</span>'
        + monitoredBadge
        + '</div>'
        + '</div>';
    }).join('')
    + '</div>';
}

function showServicesPopover(anchor, options) {
  if (!anchor) return;
  const targetId = Number(options?.targetId || 0);
  if (!Number.isFinite(targetId) || targetId <= 0) return;
  closeServicesPopover();
  const mode = options?.mode || 'selected';
  const ids = options?.ids || [];
  const targetName = options?.targetName ? escapeHtml(options.targetName) : '';
  const title = targetName ? (servicePopoverTexts.title + ' - ' + targetName) : servicePopoverTexts.title;
  const popover = document.createElement('div');
  popover.className = 'service-popover absolute z-50 w-80 max-w-sm rounded-lg shadow-xl border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800';
  popover.innerHTML = ''
    + '<div class="flex items-center justify-between px-4 py-2 border-b border-gray-100 dark:border-gray-700">'
    + '<span class="text-sm font-semibold text-gray-900 dark:text-gray-100">' + title + '</span>'
    + '<button type="button" class="text-gray-400 hover:text-gray-600 dark:text-gray-400 dark:hover:text-gray-200 focus:outline-none" data-popover-close="1">'
    + '<i class="fas fa-times"></i>'
    + '</button>'
    + '</div>'
    + '<div class="service-popover-body px-4 py-3 text-sm text-gray-600 dark:text-gray-300">'
    + servicePopoverTexts.loading
    + '</div>';
  document.body.appendChild(popover);
  positionServicesPopover(popover, anchor);
  activeServicesPopover = popover;
  activeServicesPopoverAnchor = anchor;

  const closeBtn = popover.querySelector('[data-popover-close]');
  if (closeBtn) {
    closeBtn.addEventListener('click', function (event) {
      event.preventDefault();
      closeServicesPopover();
    });
  }

  const outsideHandler = function (event) {
    if (popover.contains(event.target) || anchor.contains(event.target)) {
      return;
    }
    closeServicesPopover();
  };
  setTimeout(function () {
    document.addEventListener('click', outsideHandler, true);
  }, 0);
  activeServicesPopoverHandler = outsideHandler;

  fetchServicesByTarget(targetId).then(function (services) {
    if (!activeServicesPopover || activeServicesPopover !== popover) return;
    const body = popover.querySelector('.service-popover-body');
    if (!body) return;
    const filtered = filterServices(services, mode, ids);
    body.innerHTML = buildServicesListHTML(filtered);
    positionServicesPopover(popover, anchor);
  }).catch(function () {
    if (!activeServicesPopover || activeServicesPopover !== popover) return;
    const body = popover.querySelector('.service-popover-body');
    if (body) {
      body.innerHTML = '<p class="text-sm text-red-500">' + servicePopoverTexts.error + '</p>';
    }
  });
}

function wrapServiceChipForPopover(html, options) {
  const targetId = Number(options?.targetId || 0);
  if (!Number.isFinite(targetId) || targetId <= 0) return html;
  const mode = options?.mode || 'selected';
  const idsAttr = parseServiceIds(options?.ids).join(',');
  const targetName = options?.targetName || '';
  return '<button type="button" class="service-chip-trigger inline-flex items-center gap-1 focus:outline-none"'
    + ' data-target-id="' + targetId + '"'
    + ' data-service-mode="' + mode + '"'
    + ' data-service-ids="' + idsAttr + '"'
    + ' data-target-name="' + escapeHtml(targetName) + '"'
    + ' aria-label="' + servicePopoverTexts.openLabel + '"'
    + ' title="' + servicePopoverTexts.openLabel + '"'
    + ' style="background:transparent;border:none;padding:0;margin:0;">'
    + html
    + '<i class="fas fa-chevron-down text-[10px] text-gray-500 dark:text-gray-400 ml-1"></i>'
    + '</button>';
}

function bindServiceChipEvents(container) {
  if (!container) return;
  container.querySelectorAll('.service-chip-trigger').forEach(function (btn) {
    btn.addEventListener('click', function (event) {
      event.preventDefault();
      event.stopPropagation();
      const targetId = Number(btn.dataset.targetId || 0);
      if (!Number.isFinite(targetId) || targetId <= 0) return;
      if (activeServicesPopoverAnchor === btn) {
        closeServicesPopover();
        return;
      }
      showServicesPopover(btn, {
        targetId: targetId,
        mode: btn.dataset.serviceMode || 'selected',
        ids: parseServiceIds(btn.dataset.serviceIds || ''),
        targetName: btn.dataset.targetName || ''
      });
    });
  });
}

function renderRuleTargetDisplay(targetInfo, targetIds, serviceInfo) {
  const chip = ruleTargetChip(targetInfo);
  if (serviceInfo && serviceInfo.ok && targetInfo && !targetInfo.multiple) {
    const targetId = targetInfo.id || (Array.isArray(targetIds) && targetIds.length ? targetIds[0] : null);
    return wrapServiceChipForPopover(chip, {
      targetId: targetId,
      mode: serviceInfo.mode || 'selected',
      ids: Array.isArray(serviceInfo.ids) ? serviceInfo.ids : [],
      targetName: targetInfo.name || ''
    });
  }
  if (targetInfo && targetInfo.multiple && Array.isArray(targetIds) && targetIds.length > 1) {
    return wrapTargetChipForPopover(chip, targetIds);
  }
  return chip;
}

function ruleTargetChip(target) {
  let label = 'Belirtilmedi';
  if (target) {
    if (target.multiple) {
      label = target.name || `${target.count || 0} hedef`;
    } else if (target.name && target.name.trim()) {
      label = target.name.trim();
    } else if (target.id) {
      label = `Hedef #${target.id}`;
    }
  }
  return '<span class="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-md text-xs font-medium bg-gray-100 text-gray-700 dark:bg-gray-700/60 dark:text-gray-200">'
       + '<i class="fas fa-bullseye"></i> ' + escapeHtml(label) + '</span>';
}

function renderSensorTargetDisplay(rule, conds) {
  if (rule.entity_type === 'crack_detection') {
    var invCond = conds.find(function(c) { return (c.field||c.Field||'').toLowerCase() === 'inventory_ids'; });
    var invIds = invCond && Array.isArray(invCond.value||invCond.Value) ? (invCond.value||invCond.Value) : [];
    var label = invIds.length > 0 ? ('Crack • ' + invIds.length + ' cihaz') : 'Crack Tespiti • Tüm cihazlar';
    return '<span class="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-md text-xs font-medium bg-red-50 text-red-700 dark:bg-red-900/30 dark:text-red-300">'
         + '<i class="fas fa-exclamation-triangle"></i> ' + escapeHtml(label) + '</span>';
  }
  var hasCameraSubtype = conds.some(function(c) { return (c.field||c.Field||'').toLowerCase() === 'sensor_subtype' && (c.value||c.Value||'') === 'camera'; });
  var hasLiquidContact = conds.some(function(c) { return (c.field||c.Field||'').toLowerCase() === 'liquid_contact'; });
  if (hasCameraSubtype) {
    var cameraCond = conds.find(function(c) { return (c.field||c.Field||'').toLowerCase() === 'camera_serials'; });
    var serials = cameraCond && Array.isArray(cameraCond.value||cameraCond.Value) ? (cameraCond.value||cameraCond.Value) : [];
    var camLabel = serials.length > 0 ? ('Kamera • ' + serials.join(', ')) : 'Kamera';
    return '<span class="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-md text-xs font-medium bg-purple-50 text-purple-700 dark:bg-purple-900/30 dark:text-purple-300">'
         + '<i class="fas fa-camera"></i> ' + escapeHtml(camLabel) + '</span>';
  }
  if (hasLiquidContact) {
    var liqSerial = rule.sensor_serial || '';
    var liqLabel = liqSerial ? ('Sıvı Sensörü • ' + liqSerial) : 'Sıvı Sensörü';
    return '<span class="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-md text-xs font-medium bg-cyan-50 text-cyan-700 dark:bg-cyan-900/30 dark:text-cyan-300">'
         + '<i class="fas fa-tint"></i> ' + escapeHtml(liqLabel) + '</span>';
  }
  var airSerial = rule.sensor_serial || '';
  var airLabel = airSerial ? ('Hava Sensörü • ' + airSerial) : 'Hava Sensörü';
  return '<span class="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-md text-xs font-medium bg-sky-50 text-sky-700 dark:bg-sky-900/30 dark:text-sky-300">'
       + '<i class="fas fa-wind"></i> ' + escapeHtml(airLabel) + '</span>';
}
function renderRuleMobileCard(rule, canEdit) {
  const mobileConds = extractConditionArray(rule.conditions);
  let targetDisplay;
  if (rule.entity_type === 'sensor' || rule.entity_type === 'crack_detection') {
    targetDisplay = renderSensorTargetDisplay(rule, mobileConds);
  } else {
    const serviceInfo = parseServiceConditions(mobileConds);
    let targetInfo = { name: 'Tüm hedefler' };
    if (Array.isArray(rule.target_ids) && rule.target_ids.length > 1) {
      targetInfo = { name: `${rule.target_ids.length} hedef`, multiple: true, count: rule.target_ids.length };
    } else if (rule.target_id) {
      targetInfo = { id: rule.target_id, name: rule.target_name };
    }
    targetDisplay = renderRuleTargetDisplay(targetInfo, rule.target_ids, serviceInfo);
  }

  // Son gönderim formatlaması
  let last = '-';
  if (rule.last_sent_at) {
    const parts = String(rule.last_sent_at).split(' ');
    if (parts.length === 2) {
      const dateParts = parts[0].split('-');
      if (dateParts.length === 3) {
        const year = dateParts[0], month = dateParts[1], day = dateParts[2];
        last = day + '.' + month + '.' + year + ' ' + parts[1];
      } else {
        last = rule.last_sent_at;
      }
    } else {
      last = rule.last_sent_at;
    }
  } else if (rule.conditions) {
    if (rule.conditions.last_sent_text) {
      last = formatYMDHMSasTR(rule.conditions.last_sent_text);
    } else if (rule.conditions.last_sent_at) {
      last = formatTurkishTime(rule.conditions.last_sent_at);
    }
  }

  const recipients = Array.isArray(rule.recipients) ? rule.recipients.join(', ') : '-';
  const recipientCount = Array.isArray(rule.recipients) ? rule.recipients.length : 0;

  return ''
    + '<div class="bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-700 rounded-lg p-4">'
      + '<div class="flex items-start justify-between gap-3 mb-3">'
        + '<div class="flex-1">'
          + '<h4 class="text-sm font-semibold text-gray-900 dark:text-gray-100 mb-1">' + escapeHtml(rule.name || 'Adsız bildirim') + '</h4>'
          + '<div class="text-xs text-gray-500 dark:text-gray-400">' + targetDisplay + '</div>'
        + '</div>'
        + ruleActiveBadge(!!rule.is_active)
      + '</div>'
      + '<div class="space-y-2 mb-3">'
        + '<div class="flex items-center justify-between text-xs">'
          + '<span class="text-gray-500 dark:text-gray-400">Kanal:</span>'
          + '<span>' + ruleChannelBadge(rule.channel) + '</span>'
        + '</div>'
        + '<div class="flex items-center justify-between text-xs">'
          + '<span class="text-gray-500 dark:text-gray-400">Alıcılar:</span>'
          + '<span class="text-gray-700 dark:text-gray-200 truncate ml-2">' + escapeHtml(recipients) + ' (' + recipientCount + ')</span>'
        + '</div>'
        + '<div class="flex items-center justify-between text-xs">'
          + '<span class="text-gray-500 dark:text-gray-400">Sıklık:</span>'
          + '<span>' + ruleIntervalBadge(rule.schedule_interval_minutes) + '</span>'
        + '</div>'
        + '<div class="flex items-center justify-between text-xs">'
          + '<span class="text-gray-500 dark:text-gray-400">Son Gönderim:</span>'
          + '<span class="text-gray-700 dark:text-gray-200">' + escapeHtml(last) + '</span>'
        + '</div>'
      + '</div>'
      + (canEdit ? (
          '<div class="flex items-center justify-end gap-2 pt-3 border-t border-gray-100 dark:border-gray-700">'
            + '<button data-id="' + rule.id + '" class="btn-edit text-xs font-medium text-blue-600 hover:text-blue-700 dark:text-blue-400 dark:hover:text-blue-300">Düzenle</button>'
            + '<button data-id="' + rule.id + '" class="btn-del text-xs font-medium text-red-600 hover:text-red-700 dark:text-red-400 dark:hover:text-red-300">Sil</button>'
          + '</div>'
        ) : '')
    + '</div>';
}

    // Load rules list
// Tüm tabloyu yükler ve eventleri bağlar
async function loadRules() {
  closeTargetsPopover();
  closeServicesPopover();
  const root = document.getElementById('rules-list');
  const mobileRoot = document.getElementById('rules-list-mobile');
  if (!root) return;

  const canEdit = window.canEditModule && window.canEditModule('notification_settings');

  root.innerHTML = '<p class="text-sm text-gray-500 dark:text-gray-400">Yükleniyor...</p>';
  if (mobileRoot) {
    mobileRoot.innerHTML = '<p class="text-sm text-gray-500 dark:text-gray-400">Yükleniyor...</p>';
  }

  try {
    const res = await fetch('/api/new-notifications/rules', { headers: authHeaders() });
    const data = await res.json().catch(() => ({}));

    // Beklenen yapı yoksa boş mesaj
    if (!res.ok || (data.success === false) || !Array.isArray(data.data) || data.data.length === 0) {
      root.innerHTML = '<p class="text-sm text-gray-500 dark:text-gray-400">Kayıtlı bildirim bulunamadı.</p>';
      if (mobileRoot) {
        mobileRoot.innerHTML = '<p class="text-sm text-gray-500 dark:text-gray-400">Kayıtlı bildirim bulunamadı.</p>';
      }
      return;
    }

    let rows = '';
    try {
      rows = data.data.map(function (r) {
        // Son gönderim formatlaması (senin mantığını koruyorum)
        var last = '-';
        if (r.last_sent_at) {
          const parts = String(r.last_sent_at).split(' ');
          if (parts.length === 2) {
            const dateParts = parts[0].split('-');
            if (dateParts.length === 3) {
              const year = dateParts[0], month = dateParts[1], day = dateParts[2];
              last = day + '.' + month + '.' + year + ' ' + parts[1];
            } else {
              last = r.last_sent_at;
            }
          } else {
            last = r.last_sent_at;
          }
        } else if (r.conditions) {
          if (r.conditions.last_sent_text) {
            last = formatYMDHMSasTR(r.conditions.last_sent_text);
          } else if (r.conditions.last_sent_at) {
            last = formatTurkishTime(r.conditions.last_sent_at);
          }
        }

        const existingConds = extractConditionArray(r.conditions);
        let ruleTargetHtml;
        if (r.entity_type === 'sensor' || r.entity_type === 'crack_detection') {
          ruleTargetHtml = renderSensorTargetDisplay(r, existingConds);
        } else {
          const serviceInfo = parseServiceConditions(existingConds);
          let targetInfo = null;
          if (Array.isArray(r.target_ids) && r.target_ids.length > 1) {
            targetInfo = { name: `${r.target_ids.length} hedef`, multiple: true, count: r.target_ids.length };
          } else if (r.target_id) {
            targetInfo = { id: r.target_id, name: r.target_name };
          }
          ruleTargetHtml = renderRuleTargetDisplay(targetInfo, r.target_ids, serviceInfo);
        }

        // Satır HTML (align-middle + rozetler)
        return ''
          + '<tr class="divide-x divide-gray-200 dark:divide-gray-800 bg-white dark:bg-gray-900 hover:bg-gray-50/70 dark:hover:bg-gray-800/60 transition">'
            + '<td class="px-4 py-3 text-sm align-middle text-center">'
            + (canEdit ? '<input type="checkbox" class="rule-checkbox rounded border-gray-300 dark:border-gray-600 text-blue-600 focus:ring-blue-500" data-rule-id="' + r.id + '">' : '')
            + '</td>'

            + '<td class="px-4 py-3 text-sm text-gray-900 dark:text-white align-middle">'
              + '<div class="font-medium">' + escapeHtml(r.name || '-') + '</div>'
            + '</td>'

            + '<td class="px-4 py-3 text-sm align-middle">'
              + '<div class="flex items-center gap-2">' + ruleTargetHtml + '</div>'
            + '</td>'

            + '<td class="px-4 py-3 text-sm align-middle">'
              + '<div class="flex items-center gap-2">' + ruleChannelBadge(r.channel) + '</div>'
            + '</td>'

            + '<td class="px-4 py-3 text-sm align-middle">'
              + '<div class="flex items-center gap-2">' + ruleIntervalBadge(r.schedule_interval_minutes) + '</div>'
            + '</td>'

            + '<td class="px-4 py-3 text-sm align-middle">'
              + '<div class="flex items-center gap-2">' + ruleRecipientsBadge((r.recipients || []).length) + '</div>'
            + '</td>'

            + '<td class="px-4 py-3 text-sm text-gray-900 dark:text-white align-middle whitespace-nowrap">'
              + escapeHtml(last)
            + '</td>'

            + '<td class="px-4 py-3 text-sm align-middle">'
              + '<div class="flex items-center gap-2">' + ruleActiveBadge(!!r.is_active) + '</div>'
            + '</td>'

            + (canEdit ? (
                '<td class="px-4 py-3 text-sm align-middle">'
                    + '<div class="flex items-center gap-3">'
                        + '<button data-id="' + r.id + '" class="btn-edit inline-flex items-center gap-1.5 px-2.5 py-1.5 rounded-md border border-gray-200 dark:border-gray-700 hover:bg-gray-50 dark:hover:bg-gray-700 text-blue-600 dark:text-blue-400" title="Düzenle">'
                        + '<i class="fas fa-pen"></i><span class="hidden sm:inline">Düzenle</span>'
                        + '</button>'
                        + '<button data-id="' + r.id + '" class="btn-del inline-flex items-center gap-1.5 px-2.5 py-1.5 rounded-md border border-gray-200 dark:border-gray-700 hover:bg-gray-50 dark:hover:bg-gray-700 text-red-600 dark:text-red-400" title="Sil">'
                        + '<i class="fas fa-trash"></i><span class="hidden sm:inline">Sil</span>'
                        + '</button>'
                    + '</div>'
                + '</td>'
            ) : '')
          + '</tr>';
      }).join('');
    } catch (mapErr) {
      console.error('rules map error:', mapErr, data);
      rows = '';
    }

    const html =
      '<div class="overflow-x-auto rounded-xl border border-gray-200 dark:border-gray-700">'
        + '<table class="min-w-[900px] w-full divide-y divide-gray-200 dark:divide-gray-700 text-sm">'
          + '<thead class="sticky top-0 z-10 bg-gray-50/90 dark:bg-gray-800/90 backdrop-blur">'
            + '<tr class="divide-x divide-gray-200 dark:divide-gray-700 text-gray-700 dark:text-gray-200">'
              + (canEdit
                ? '<th class="px-4 py-3 text-center text-xs font-semibold uppercase tracking-wider w-12">'
                    + '<input type="checkbox" id="select-all-rules" class="rounded border-gray-300 dark:border-gray-600 text-blue-600 focus:ring-blue-500">'
                  + '</th>'
                : '<th class="px-4 py-3 text-center text-xs font-semibold uppercase tracking-wider w-12"></th>'
              )
              + '<th class="px-4 py-3 text-left text-xs font-semibold uppercase tracking-wider">Ad</th>'
              + '<th class="px-4 py-3 text-left text-xs font-semibold uppercase tracking-wider">Hedef</th>'
              + '<th class="px-4 py-3 text-left text-xs font-semibold uppercase tracking-wider">Kanal</th>'
              + '<th class="px-4 py-3 text-left text-xs font-semibold uppercase tracking-wider">Sıklık</th>'
              + '<th class="px-4 py-3 text-left text-xs font-semibold uppercase tracking-wider">Alıcı</th>'
              + '<th class="px-4 py-3 text-left text-xs font-semibold uppercase tracking-wider">Son Gönderim</th>'
              + '<th class="px-4 py-3 text-left text-xs font-semibold uppercase tracking-wider">Durum</th>'
              + (canEdit
                ? '<th class="px-4 py-3 text-left text-xs font-semibold uppercase tracking-wider">İşlemler</th>'
                : ''
              )
            + '</tr>'
          + '</thead>'
            + '<tbody class="bg-white dark:bg-gray-950 divide-y divide-gray-200 dark:divide-gray-800">'
              + rows +
            '</tbody>'
        + '</table>'
      + '</div>';

    root.innerHTML = html;
    bindTargetChipEvents(root);
    bindServiceChipEvents(root);

    // Mobil liste render
    if (mobileRoot) {
      const mobileCards = data.data.map(function(r) {
        return renderRuleMobileCard(r, canEdit);
      }).join('');
      mobileRoot.innerHTML = '<div class="space-y-3">' + mobileCards + '</div>';
      bindTargetChipEvents(mobileRoot);
      bindServiceChipEvents(mobileRoot);
    }

    // --- Event bind (ikon/span tıklamalarında da ID gelsin diye currentTarget) ---
    // Desktop ve mobil için ayrı ayrı event handler'lar ekle
    const attachDeleteHandlers = function(container) {
      if (!container) return;
      container.querySelectorAll('.btn-del').forEach(function (btn) {
        btn.addEventListener('click', async function (e) {
          const id = e.currentTarget.dataset.id;
          if (!id) return;
          if (!confirm('Silmek istediğinize emin misiniz?')) return;
          const resDel = await fetch('/api/new-notifications/rules/' + id, { method: 'DELETE', headers: authHeaders() });
          const d = await resDel.json().catch(() => ({}));
          setMsg(resDel.ok && (d.success === undefined || d.success) ? 'Silindi' : (d.error || 'Silinemedi'));
          loadRules();
        });
      });
    };

    const attachEditHandlers = function(container) {
      if (!container) return;
      container.querySelectorAll('.btn-edit').forEach(function (btn) {
        btn.addEventListener('click', async function (e) {
          const id = e.currentTarget.dataset.id;
          if (!id) return;

          const rule = data.data.find(function (x) { return String(x.id) === String(id); });
          if (!rule) return;

          openModal();
          const headerEl = document.querySelector('#create-modal h3'); if (headerEl) headerEl.textContent = 'Bildirim Düzenle';
          const sbEl = document.getElementById('btn-submit-create'); if (sbEl) sbEl.textContent = 'Kaydet';
          document.getElementById('rule-name').value = rule.name || '';
          document.getElementById('rule-channel').value = rule.channel || 'email';
          syncRecipientUI();
          document.getElementById('rule-interval').value = String(rule.schedule_interval_minutes || 10);

          const recInput = document.getElementById('rule-recipients');
          recInput.value = (rule.recipients || []).join(', ');

          const templateSelect = document.getElementById('rule-template');
          if (templateSelect) {
            const selectedTemplate = rule.template_id != null ? String(rule.template_id) : '';
            if (selectedTemplate) {
              const hasOption = Array.from(templateSelect.options || []).some(function(opt) {
                return String(opt.value) === selectedTemplate;
              });
              if (!hasOption) {
                const fallbackOption = document.createElement('option');
                fallbackOption.value = selectedTemplate;
                fallbackOption.dataset.dynamic = '1';
                fallbackOption.textContent = 'Şablon #' + selectedTemplate;
                templateSelect.appendChild(fallbackOption);
              }
              templateSelect.value = selectedTemplate;
            } else {
              templateSelect.value = '';
            }
          }

          // Determine UI entity type from DB entity_type + conditions
          const editConds = extractConditionArray(rule.conditions);
          let uiEntityType = 'target';
          if (rule.entity_type === 'sensor') {
            const hasCameraSubtype = editConds.some(function(c) {
              return (c.field || c.Field || '').toLowerCase() === 'sensor_subtype' && (c.value || c.Value || '') === 'camera';
            });
            const hasLiquidContact = editConds.some(function(c) {
              return (c.field || c.Field || '').toLowerCase() === 'liquid_contact';
            });
            if (hasCameraSubtype) uiEntityType = 'camera';
            else if (hasLiquidContact) uiEntityType = 'liquid_sensor';
            else uiEntityType = 'air_sensor';
          } else if (rule.entity_type === 'crack_detection') {
            uiEntityType = 'crack_detection';
          }

          const ruleTypeSelEl = document.getElementById('rule-type');
          if (ruleTypeSelEl) ruleTypeSelEl.value = uiEntityType;
          syncEntityTypeUI();

          if (uiEntityType === 'target') {
            await loadTargetsSelect();
            targetPicker.setMaxSelectable(MAX_NOTIFICATION_TARGETS);
            targetPicker.setDisabled(false);
            targetPicker.clearFilter();
            if (Array.isArray(rule.target_ids) && rule.target_ids.length) {
              targetPicker.setSelected(rule.target_ids.map(function(tid){ return String(tid); }));
            } else if (rule.target_id) {
              targetPicker.setSelected([String(rule.target_id)]);
            } else {
              targetPicker.clearSelection();
            }
            targetPicker.closePanel();

            const condFieldEl = document.getElementById('rule-cond-field');
            const condStatusEl = document.getElementById('rule-cond-status');
            const condOpEl = document.getElementById('rule-cond-operator');
            const condValEl = document.getElementById('rule-cond-value');
            if (condFieldEl) {
              if (editConds.length > 0) {
                const serviceInfo = parseServiceConditions(editConds);
                if (serviceInfo.ok) {
                  condFieldEl.value = 'services';
                  setServiceMode(serviceInfo.mode || 'selected', true);
                  serviceState.pendingIds = Array.isArray(serviceInfo.ids) ? serviceInfo.ids : [];
                  syncConditionControls();
                } else {
                  const c0 = editConds[0] || {};
                  const field = (c0.field || c0.Field || '').toString().toLowerCase();
                  if (field === 'response_time_ms' || field === 'rtt_ms' || field === 'cpu_percent' || field === 'ram_gb' || field === 'disk_gb' || field === 'temperature_c') {
                    const normalizedField = field === 'rtt_ms' ? 'response_time_ms' : field;
                    condFieldEl.value = normalizedField;
                    syncConditionControls();
                    if (condOpEl) { condOpEl.disabled = false; condOpEl.value = (c0.operator || c0.Operator || '<'); }
                    if (condValEl) { condValEl.disabled = false; condValEl.value = (c0.value != null ? c0.value : (c0.Value != null ? c0.Value : '')); }
                    if (condStatusEl) condStatusEl.value = 'fail';
                  } else {
                    condFieldEl.value = 'status';
                    syncConditionControls();
                    if (condStatusEl) condStatusEl.value = (c0.value || c0.Value || 'fail');
                  }
                }
              } else {
                condFieldEl.value = 'status';
                if (condStatusEl) condStatusEl.value = 'fail';
                syncConditionControls();
              }
            } else {
              syncConditionControls();
            }
          } else if (uiEntityType === 'air_sensor') {
            const threshCond = editConds.find(function(c) {
              return (c.field || c.Field || '').toLowerCase() !== 'sensor_subtype';
            });
            if (threshCond) {
              const fieldSel = document.getElementById('sensor-field-select');
              const opSel = document.getElementById('sensor-op-select');
              const valInput = document.getElementById('sensor-val-input');
              if (fieldSel) fieldSel.value = (threshCond.field || threshCond.Field || 'temperature');
              if (opSel) opSel.value = (threshCond.operator || threshCond.Operator || '<');
              if (valInput) valInput.value = (threshCond.value != null ? threshCond.value : (threshCond.Value != null ? threshCond.Value : ''));
            }
            await loadSensorSerials();
            if (rule.sensor_serial) {
              const serialSel = document.getElementById('sensor-serial-select');
              if (serialSel) serialSel.value = rule.sensor_serial;
            }
          } else if (uiEntityType === 'liquid_sensor') {
            await loadLiquidSensorSerials();
            if (rule.sensor_serial) {
              const serialSel = document.getElementById('liquid-serial-select');
              if (serialSel) serialSel.value = rule.sensor_serial;
            }
          } else if (uiEntityType === 'camera') {
            const cameraCond = editConds.find(function(c) {
              return (c.field || c.Field || '').toLowerCase() === 'camera_serials';
            });
            const preselected = cameraCond && Array.isArray(cameraCond.value || cameraCond.Value)
              ? (cameraCond.value || cameraCond.Value) : [];
            await loadCameraSerials();
            if (preselected.length > 0 && _cameraPicker) {
              _cameraPicker.setSelected(preselected.map(String));
            }
          } else if (uiEntityType === 'crack_detection') {
            const severityCond = editConds.find(function(c) {
              return (c.field || c.Field || '').toLowerCase() === 'min_severity';
            });
            if (severityCond) {
              const sevSel = document.getElementById('crack-severity-select');
              if (sevSel) sevSel.value = (severityCond.value || severityCond.Value || 'any');
            }
            const invCond = editConds.find(function(c) {
              return (c.field || c.Field || '').toLowerCase() === 'inventory_ids';
            });
            await loadInventoryForCrack();
            if (invCond && Array.isArray(invCond.value || invCond.Value) && _inventoryPicker) {
              _inventoryPicker.setSelected((invCond.value || invCond.Value).map(String));
            }
          }

          const submitBtn = document.getElementById('btn-submit-create');
          if (submitBtn) {
            const original = submitRule;
            submitBtn.onclick = async function () {
              const channelValue = (document.getElementById('rule-channel')?.value || 'email').toLowerCase();
              const recipients = recInput.value.split(',').map(function (s) { return s.trim(); }).filter(Boolean);
              if (recipients.length === 0) {
                setMsg(channelValue === 'telegram' ? 'Lütfen en az bir chat_id girin' : 'Lütfen alıcıları girin');
                return;
              }

              const currentEntityType = document.getElementById('rule-type')?.value || 'target';
              const dbEntityType = (currentEntityType === 'camera' || currentEntityType === 'air_sensor' || currentEntityType === 'liquid_sensor') ? 'sensor' : currentEntityType;
              const needsTarget = currentEntityType === 'target';

              const conditions = collectConditions();
              if (!conditions) return;

              const templateField = document.getElementById('rule-template');
              const templateValue = (templateField?.value || '').trim();
              let templateId = null;
              if (templateValue !== '') {
                const parsedTemplateId = parseInt(templateValue, 10);
                templateId = (!Number.isNaN(parsedTemplateId) && parsedTemplateId > 0) ? parsedTemplateId : null;
              }

              const payload = {
                name: document.getElementById('rule-name').value,
                entity_type: dbEntityType,
                channel: channelValue,
                schedule_interval_minutes: Number(document.getElementById('rule-interval').value || 10),
                recipients: recipients,
                template_id: templateId,
                is_active: true,
                conditions: conditions
              };

              if (needsTarget) {
                const selectedForEdit = targetPicker.getSelected();
                const editTargetIds = selectedForEdit
                  .map(function(val){ return Number(val); })
                  .filter(function(num){ return Number.isFinite(num) && num > 0; });
                if (editTargetIds.length === 0) { setMsg('Lütfen bir hedef seçin'); return; }
                if (editTargetIds.length > MAX_NOTIFICATION_TARGETS) {
                  setMsg('En fazla ' + MAX_NOTIFICATION_TARGETS + ' hedef seçebilirsiniz.');
                  return;
                }
                payload.target_ids = editTargetIds;
                payload.target_id = editTargetIds.length === 1 ? editTargetIds[0] : null;
              } else {
                payload.target_ids = [];
                payload.target_id = null;
              }

              if (currentEntityType === 'air_sensor') {
                const serialSel = document.getElementById('sensor-serial-select');
                const serial = (serialSel ? serialSel.value : '') || '';
                if (serial) payload.sensor_serial = serial;
              }
              if (currentEntityType === 'liquid_sensor') {
                const serialSel = document.getElementById('liquid-serial-select');
                const serial = (serialSel ? serialSel.value : '') || '';
                if (serial) payload.sensor_serial = serial;
              }

              const res2 = await fetch('/api/new-notifications/rules/' + id, {
                method: 'PUT',
                headers: authHeaders(),
                body: JSON.stringify(payload)
              });
              const d2 = await res2.json().catch(() => ({}));
              setMsg(res2.ok && (d2.success === undefined || d2.success) ? 'Güncellendi' : (d2.error || 'Güncellenemedi'));
              closeModal();
              submitBtn.onclick = original;
              loadRules();
            };
          }
        });
      });
    };

    // Hem desktop hem mobil için handler'ları ekle
    attachDeleteHandlers(root);
    attachDeleteHandlers(mobileRoot);
    attachEditHandlers(root);
    attachEditHandlers(mobileRoot);

    // Checkbox event listeners (sadece desktop için)
    const selectAllCheckbox = document.getElementById('select-all-rules');
    const ruleCheckboxes = root.querySelectorAll('.rule-checkbox');

    function updateBulkDeleteButton() {
      const checkedBoxes = root.querySelectorAll('.rule-checkbox:checked');
      const bulkDeleteBtn = document.getElementById('btn-bulk-delete');
      const bulkDeleteText = document.getElementById('bulk-delete-text');
      if (bulkDeleteBtn) {
        if (checkedBoxes.length > 0) {
          bulkDeleteBtn.classList.remove('hidden');
          bulkDeleteBtn.classList.add('inline-flex');
          if (bulkDeleteText) {
            bulkDeleteText.textContent = 'Toplu Sil (' + checkedBoxes.length + ')';
          }
        } else {
          bulkDeleteBtn.classList.add('hidden');
          bulkDeleteBtn.classList.remove('inline-flex');
        }
      }
    }

    if (selectAllCheckbox) {
      selectAllCheckbox.addEventListener('change', function() {
        ruleCheckboxes.forEach(function(cb) {
          cb.checked = selectAllCheckbox.checked;
        });
        updateBulkDeleteButton();
      });
    }

    ruleCheckboxes.forEach(function(cb) {
      cb.addEventListener('change', function() {
        const allChecked = Array.from(ruleCheckboxes).every(function(checkbox) { return checkbox.checked; });
        const someChecked = Array.from(ruleCheckboxes).some(function(checkbox) { return checkbox.checked; });

        if (selectAllCheckbox) {
          selectAllCheckbox.checked = allChecked;
          selectAllCheckbox.indeterminate = someChecked && !allChecked;
        }

        updateBulkDeleteButton();
      });
    });

  } catch (e) {
    console.error('loadRules error:', e);
    root.innerHTML = '<p class="text-sm text-red-600">Kayıtlar yüklenemedi</p>';
  }
}

    const btnRefresh = document.getElementById('btn-refresh-rules'); if (btnRefresh) btnRefresh.onclick = loadRules;
    const btnBulkDelete = document.getElementById('btn-bulk-delete');
    if (btnBulkDelete) {
        btnBulkDelete.onclick = async function() {
            const selectedCheckboxes = document.querySelectorAll('.rule-checkbox:checked');
            const ids = Array.from(selectedCheckboxes).map(function(cb) { return cb.dataset.ruleId; }).filter(Boolean);

            if (ids.length === 0) return;

            if (!confirm(ids.length + ' bildirimi silmek istediğinize emin misiniz? Bu işlem geri alınamaz.')) return;

            try {
                const results = await Promise.all(
                    ids.map(function(id) {
                        return fetch('/api/new-notifications/rules/' + id, {
                            method: 'DELETE',
                            headers: authHeaders()
                        }).then(function(r) { return r.json(); });
                    })
                );

                const successCount = results.filter(function(r) { return r.success; }).length;
                const failCount = results.length - successCount;

                if (failCount > 0) {
                    setMsg(successCount + ' bildirim silindi, ' + failCount + ' bildirim silinemedi');
                } else {
                    setMsg(successCount + ' bildirim başarıyla silindi');
                }

                // Hide bulk delete button
                btnBulkDelete.classList.add('hidden');
                btnBulkDelete.classList.remove('inline-flex');

                loadRules();
            } catch (err) {
                setMsg('Toplu silme hatası: ' + err.message);
            }
        };
    }
    loadRules();

    if (saveBtn) saveBtn.onclick = saveSettings;
    if (openBtn) openBtn.onclick = openModal;
    if (closeBtn) closeBtn.onclick = closeModal;
    if (cancelBtn) cancelBtn.onclick = closeModal;
    if (submitBtn) submitBtn.onclick = submitRule;
    if (templatesLink) templatesLink.onclick = (e)=>{ e.preventDefault(); if (window.app) window.app.navigateTo('new-notification-templates'); };
    if (btnSaveSMTP) btnSaveSMTP.onclick = async ()=>{
        const cfg = {
            smtp_host: document.getElementById('smtp-host')?.value || '',
            smtp_port: Number(document.getElementById('smtp-port')?.value || '587'),
            username: document.getElementById('smtp-user')?.value || '',
            from_email: document.getElementById('smtp-from')?.value || '',
            from_name: document.getElementById('smtp-from-name')?.value || 'SysTrack',
            use_tls: !!document.getElementById('smtp-tls')?.checked
        };
        const smtpPass = (document.getElementById('smtp-pass')?.value || '').trim();
        if (smtpPass && smtpPass !== '********') {
            cfg.password = smtpPass;
        }
        try {
            const res = await fetch('/api/new-notifications/config', { method:'POST', headers: authHeaders(), body: JSON.stringify({ channel:'email', is_enabled:true, config: cfg })});
            const data = await res.json().catch(()=>({}));
            setMsg(res.ok && (data.success === undefined || data.success) ? 'SMTP ayarları kaydedildi.' : (data.error || 'SMTP ayarları kaydedilemedi'));
        } catch(e){ setMsg('SMTP ayarları kaydedilemedi: ' + e.message); }
    };
    if (testOpenBtn) testOpenBtn.onclick = ()=>{ openModalById('test-email-modal'); };
    if (testSendBtn) testSendBtn.onclick = ()=>{ closeModalById('test-email-modal'); };
    if (testCloseBtn) testCloseBtn.onclick = ()=>{ closeModalById('test-email-modal'); };
    if (testCancelBtn) testCancelBtn.onclick = ()=>{ closeModalById('test-email-modal'); };
    // Load existing SMTP config
    (async ()=>{
        try {
            const res = await fetch('/api/new-notifications/config?channel=email', { headers: authHeaders() });
            const data = await res.json();
            const cfg = data?.data?.config || {};
            if (cfg.smtp_host) document.getElementById('smtp-host').value = cfg.smtp_host;
            if (cfg.smtp_port) document.getElementById('smtp-port').value = cfg.smtp_port;
            if (cfg.username) document.getElementById('smtp-user').value = cfg.username;
            const passEl = document.getElementById('smtp-pass');
            if (passEl) {
                // Never prefill stored password into the browser.
                passEl.value = '';
                passEl.placeholder = data?.data?.has_password ? '********' : '';
            }
            if (cfg.from_email) document.getElementById('smtp-from').value = cfg.from_email;
            if (cfg.from_name) document.getElementById('smtp-from-name').value = cfg.from_name;
            if (typeof cfg.use_tls !== 'undefined') document.getElementById('smtp-tls').checked = !!cfg.use_tls;
        } catch{}
    })();

    // preload templates into both create and edit selects
    fetch('/api/new-notifications/templates', { headers: authHeaders() }).then(r=>r.json()).then(d=>{
        if (d.success){
            // Load templates into create modal select
            const createSel = document.getElementById('rule-template');
            if (createSel){
                // Clear existing options except the first one (default option)
                while(createSel.options.length > 1) { createSel.remove(1); }
                d.data.forEach(t=>{
                    const opt=document.createElement('option');
                    opt.value=t.id;
                    opt.textContent=`${t.name} (${t.channel})`;
                    createSel.appendChild(opt);
                });
            }
            // Load templates into edit modal select
            const editSel = document.getElementById('edit-rule-template');
            if (editSel){
                // Clear existing options except the first one (default option)
                while(editSel.options.length > 1) { editSel.remove(1); }
                d.data.forEach(t=>{
                    const opt=document.createElement('option');
                    opt.value=t.id;
                    opt.textContent=`${t.name} (${t.channel})`;
                    editSel.appendChild(opt);
                });
            }
        }
    }).catch(()=>{});

    loadSettings();
    function openModalById(id){ const m=document.getElementById(id); if(m){ m.classList.remove('hidden'); m.classList.add('flex'); } }
    function closeModalById(id){ const m=document.getElementById(id); if(m){ m.classList.add('hidden'); m.classList.remove('flex'); } }
};

// Notification Templates page (TR)
(function() {
  function authHeaders() {
    return { 'Authorization': 'Bearer ' + localStorage.getItem('token'), 'Content-Type': 'application/json' };
  }

  const fallbackPlaceholders = [
    // Temel Bilgiler (Önerilen)
    { key: 'hedef_adi', label: 'Hedef Adı', description: 'İzlenen hedefin adı', example: 'Ana Ofis Router', recommended: true },
    { key: 'hedef_adresi', label: 'Hedef Adresi', description: 'IP adresi veya domain', example: '192.168.1.254', recommended: true },
    { key: 'durum', label: 'Durum', description: 'Hedefin anlık durum açıklaması', example: 'Cihaz offline (Hatalı ping)', recommended: true },
    { key: 'izleme_tipi', label: 'İzleme Tipi', description: 'Kullanılan izleme yöntemi', example: 'ICMP Ping' },
    { key: 'tarih_saat', label: 'Tarih & Saat', description: 'Bildirimin gönderilme zamanı', example: '27.11.2025 14:32:10', recommended: true },
    // Kural & Alıcı Bilgileri
    { key: 'kural_adi', label: 'Kural Adı', description: 'Bildirim kuralının adı', example: 'Kritik Cihaz İzleme' },
    { key: 'alicilar', label: 'Alıcılar', description: 'Bildirim alan kişiler', example: 'admin@firma.com, destek@firma.com' },
    // Yanıt Süresi Koşulları (Opsiyonel)
    { key: 'yanit_suresi', label: 'Yanıt Süresi', description: 'Ölçülen yanıt süresi (varsa)', example: '245 ms' },
    { key: 'kosul', label: 'Koşul Açıklaması', description: 'Tetiklenen koşulun açıklaması', example: 'Yanıt süresi > 200 ms' }
  ];
  const texts = {
    loading: 'Şablonlar yükleniyor...',
    empty: 'Henüz şablon oluşturulmadı.',
    placeholdersEmpty: 'Değişken bulunamadı',
    defaultSubject: 'Varsayılan konu kullanılacak',
    placeholderNone: 'Değişken listesi yok',
    refreshSuccess: 'Şablon listesi yenilendi.',
    loadError: 'Şablon listesi alınamadı',
    saveMissing: 'Lütfen şablon adı ve içerik alanını doldurun.',
    createSuccess: 'Şablon oluşturuldu.',
    updateSuccess: 'Şablon güncellendi.',
    saveError: 'Şablon kaydedilemedi',
    deleteConfirm: 'Şablonu silmek istediğinize emin misiniz?',
    deleteSuccess: 'Şablon silindi.',
    deleteError: 'Şablon silinemedi'
  };
  const state = { cache: [], placeholders: fallbackPlaceholders.slice(), editingId: null, activeInput: null, canEdit: true };

  function setTemplatesMessage(message, isError) {
    const el = document.getElementById('templates-message');
    if (!el) return;
    el.textContent = message || '';
    el.classList.toggle('text-red-500', !!isError);
    el.classList.toggle('dark:text-red-400', !!isError);
    if (!isError) {
      el.classList.remove('text-red-500', 'dark:text-red-400');
    }
  }

  async function loadTemplatePlaceholders() {
    try {
      const res = await fetch('/api/new-notifications/templates/placeholders', { headers: authHeaders() });
      const data = await res.json();
      if (!res.ok || data.success === false) {
        throw new Error(data.error || '');
      }
      state.placeholders = Array.isArray(data.data) && data.data.length ? data.data : fallbackPlaceholders.slice();
    } catch (err) {
      console.error('template placeholders err', err);
      state.placeholders = fallbackPlaceholders.slice();
    }
  }

  function renderTemplatePlaceholderList() {
    const container = document.getElementById('template-placeholder-list');
    if (!container) return;
    if (!state.placeholders.length) {
      container.innerHTML = `<span class="text-xs text-gray-500 dark:text-gray-400">${texts.placeholdersEmpty}</span>`;
      return;
    }
    container.innerHTML = state.placeholders.map((ph) => {
      const isRecommended = ph.recommended === true;
      const btnClass = isRecommended
        ? 'px-3 py-1 text-xs rounded-full bg-indigo-100 dark:bg-indigo-900/40 text-indigo-700 dark:text-indigo-300 border border-indigo-300 dark:border-indigo-700 hover:bg-indigo-200 dark:hover:bg-indigo-900/60 transition font-medium'
        : 'px-3 py-1 text-xs rounded-full bg-gray-100 dark:bg-gray-700 text-gray-700 dark:text-gray-100 hover:bg-gray-200 dark:hover:bg-gray-600 transition';
      const icon = isRecommended ? '<i class="fas fa-star text-[10px] mr-1"></i>' : '';
      return `
      <button type="button" class="${btnClass}" data-placeholder="${ph.key}" title="${escapeHtml(ph.description || ph.label || ph.key)}">
        ${icon}{${ph.key}}
      </button>
    `;
    }).join('');
    container.querySelectorAll('button[data-placeholder]').forEach((btn) => {
      btn.addEventListener('click', () => insertPlaceholderToken(btn.dataset.placeholder));
    });
  }

  function insertPlaceholderToken(key) {
    if (!key) return;
    const el = state.activeInput || document.getElementById('template-body');
    if (!el) return;
    const insertion = `{${key}}`;
    const start = el.selectionStart || 0;
    const end = el.selectionEnd || 0;
    const value = el.value || '';
    el.value = value.slice(0, start) + insertion + value.slice(end);
    const pos = start + insertion.length;
    el.focus();
    if (typeof el.selectionStart === 'number') {
      el.selectionStart = pos;
      el.selectionEnd = pos;
    }
    // Ön izlemeyi güncelle
    updatePreview();
  }

  // Akıllı uyarı fonksiyonu
  function checkTemplateWarnings(body, subject) {
    const warnings = [];
    const text = (subject || '') + ' ' + (body || '');

    // Hedef bilgisi kontrolü
    if (!text.includes('{hedef_adi}') && !text.includes('{hedef_adresi}')) {
      warnings.push('⚠️ Hangi hedefle ilgili olduğu anlaşılamayabilir. "{hedef_adi}" veya "{hedef_adresi}" eklemeyi düşünün.');
    }

    // Durum bilgisi kontrolü
    if (!text.includes('{durum}')) {
      warnings.push('⚠️ Durum bilgisi eksik. "{durum}" değişkenini eklemeyi düşünün.');
    }

    return warnings;
  }

  // Ön izleme güncelleme fonksiyonu
  function updatePreview() {
    const previewEl = document.getElementById('template-preview');
    if (!previewEl) return;

    const subject = (document.getElementById('template-subject').value || '').trim();
    const body = (document.getElementById('template-body').value || '').trim();

    if (!body) {
      previewEl.innerHTML = '<p class="text-sm text-gray-500 dark:text-gray-400 italic">Şablon içeriği girildiğinde ön izleme burada görünecek...</p>';
      return;
    }

    // Örnek verilerle render et
    const previewData = {
      hedef_adi: 'Ana Ofis Router',
      hedef_adresi: '192.168.1.254',
      durum: 'Cihaz offline (Hatalı ping)',
      izleme_tipi: 'ICMP Ping',
      tarih_saat: '27.11.2025 14:32:10',
      kural_adi: 'Kritik Cihaz İzleme',
      alicilar: 'admin@firma.com, destek@firma.com',
      yanit_suresi: '245 ms',
      kosul: 'Yanıt süresi > 200 ms'
    };

    let renderedSubject = subject;
    let renderedBody = body;

    // Değişkenleri değiştir
    Object.keys(previewData).forEach(key => {
      const regex = new RegExp(`\\{${key}\\}`, 'g');
      renderedSubject = renderedSubject.replace(regex, previewData[key]);
      renderedBody = renderedBody.replace(regex, previewData[key]);
    });

    // HTML oluştur
    let html = '<div class="space-y-3">';
    if (renderedSubject) {
      html += `<div><strong class="text-sm font-semibold text-gray-700 dark:text-gray-300">Konu:</strong> <span class="text-sm text-gray-900 dark:text-gray-100">${escapeHtml(renderedSubject)}</span></div>`;
    }
    html += `<div><strong class="text-sm font-semibold text-gray-700 dark:text-gray-300">İçerik:</strong><pre class="mt-2 p-3 bg-gray-50 dark:bg-gray-800 rounded text-xs text-gray-900 dark:text-gray-100 whitespace-pre-wrap font-mono">${escapeHtml(renderedBody)}</pre></div>`;

    // Akıllı uyarıları göster
    const warnings = checkTemplateWarnings(body, subject);
    if (warnings.length > 0) {
      html += '<div class="mt-3 p-3 bg-yellow-50 dark:bg-yellow-900/20 border border-yellow-200 dark:border-yellow-700 rounded space-y-1">';
      warnings.forEach(w => {
        html += `<p class="text-xs text-yellow-800 dark:text-yellow-200">${escapeHtml(w)}</p>`;
      });
      html += '</div>';
    }

    html += '</div>';
    previewEl.innerHTML = html;
  }

  function applyLoadingState() {
    const listEl = document.getElementById('template-list');
    if (!listEl) return;
    listEl.innerHTML = `
      <div class="col-span-full">
        <div class="rounded-xl bg-gray-50 dark:bg-gray-900/40 border border-dashed border-gray-200 dark:border-gray-700 px-6 py-8 text-center space-y-2">
          <i class="fas fa-layer-group text-2xl text-gray-400 dark:text-gray-500"></i>
          <p class="text-sm text-gray-500 dark:text-gray-400">${texts.loading}</p>
        </div>
      </div>`;
  }

  async function refreshTemplateList(showMessage) {
    applyLoadingState();
    try {
      const res = await fetch('/api/new-notifications/templates', { headers: authHeaders() });
      const data = await res.json();
      if (!res.ok || data.success === false) {
        throw new Error(data.error || texts.loadError);
      }
      state.cache = Array.isArray(data.data) ? data.data : [];
      renderTemplateCards(state.cache);
      if (showMessage) setTemplatesMessage(texts.refreshSuccess);
      else setTemplatesMessage('');
    } catch (err) {
      console.error('template fetch err', err);
      setTemplatesMessage(`${texts.loadError}: ${err && err.message ? err.message : ''}`, true);
    }
  }

  function renderTemplateCards(items) {
    const listEl = document.getElementById('template-list');
    if (!listEl) return;
    if (!items.length) {
      listEl.innerHTML = `
        <div class="col-span-full">
          <div class="rounded-xl border border-dashed border-gray-200 dark:border-gray-700 px-6 py-8 text-center space-y-2">
            <i class="fas fa-layer-group text-2xl text-gray-400 dark:text-gray-500"></i>
            <p class="text-sm text-gray-500 dark:text-gray-400">${texts.empty}</p>
          </div>
        </div>`;
      return;
    }

    listEl.innerHTML = items.map((tpl) => {
      const subject = tpl.subject ? escapeHtml(tpl.subject) : texts.defaultSubject;
      // İçerik ön izleme: sadece ilk satır, maksimum 120 karakter
      const bodyText = (tpl.body || '').replace(/\r?\n/g, ' ').trim();
      const preview = escapeHtml(bodyText.slice(0, 120));

      // Kanal bazlı tasarım
      let channelIcon, channelBg, channelText, channelBorder, channelLabel, channelBarBg;
      if (tpl.channel === 'telegram') {
        channelIcon = 'fa-paper-plane';
        channelBg = 'bg-cyan-50 dark:bg-cyan-950/30';
        channelText = 'text-cyan-700 dark:text-cyan-300';
        channelBorder = 'border-cyan-200 dark:border-cyan-800';
        channelBarBg = 'bg-cyan-500 dark:bg-cyan-400';
        channelLabel = 'Telegram';
      } else if (tpl.channel === 'email') {
        channelIcon = 'fa-envelope';
        channelBg = 'bg-blue-50 dark:bg-blue-950/30';
        channelText = 'text-blue-700 dark:text-blue-300';
        channelBorder = 'border-blue-200 dark:border-blue-800';
        channelBarBg = 'bg-blue-500 dark:bg-blue-400';
        channelLabel = 'E-posta';
      } else if (tpl.channel === 'all') {
        channelIcon = 'fa-globe';
        channelBg = 'bg-purple-50 dark:bg-purple-950/30';
        channelText = 'text-purple-700 dark:text-purple-300';
        channelBorder = 'border-purple-200 dark:border-purple-800';
        channelBarBg = 'bg-purple-500 dark:bg-purple-400';
        channelLabel = 'Tüm Kanallar';
      } else {
        channelIcon = 'fa-bell';
        channelBg = 'bg-gray-50 dark:bg-gray-800';
        channelText = 'text-gray-700 dark:text-gray-300';
        channelBorder = 'border-gray-200 dark:border-gray-700';
        channelBarBg = 'bg-gray-500 dark:bg-gray-400';
        channelLabel = tpl.channel;
      }

      // Şablon içeriğinde gerçekten kullanılan değişkenleri bul
      const contentToScan = (tpl.subject || '') + ' ' + (tpl.body || '');
      const variableRegex = /\{([a-zA-Z0-9_]+)\}/g;
      const foundVariables = new Set();
      let match;
      while ((match = variableRegex.exec(contentToScan)) !== null) {
        foundVariables.add(match[1]);
      }
      const usedVariables = Array.from(foundVariables);
      const placeholders = usedVariables.slice(0, 6).map(
        (key) => `<span class="inline-flex items-center px-2 py-0.5 text-[10px] font-medium rounded bg-indigo-50 dark:bg-indigo-950/40 text-indigo-700 dark:text-indigo-300 border border-indigo-200 dark:border-indigo-800">{${key}}</span>`
      ).join('');

      const actions = state.canEdit ? `
        <button class="inline-flex items-center gap-1.5 px-3 py-2 text-xs font-medium text-gray-700 dark:text-gray-200 bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-700 rounded-lg hover:bg-gray-50 dark:hover:bg-gray-700 transition-colors" data-role="edit-template" data-template-id="${tpl.id}">
          <i class="fas fa-edit text-xs"></i>
          <span>Düzenle</span>
        </button>
        <button class="inline-flex items-center gap-1.5 px-3 py-2 text-xs font-medium text-red-600 dark:text-red-400 bg-red-50 dark:bg-red-950/30 border border-red-200 dark:border-red-800 rounded-lg hover:bg-red-100 dark:hover:bg-red-950/50 transition-colors" data-role="delete-template" data-template-id="${tpl.id}">
          <i class="fas fa-trash-alt text-xs"></i>
          <span>Sil</span>
        </button>` : '';

      return `
        <div class="group relative overflow-hidden border border-gray-200 dark:border-gray-700 rounded-xl bg-white dark:bg-gray-900 hover:border-indigo-300 dark:hover:border-indigo-700 hover:shadow-lg transition-all duration-200">
          <!-- Sol taraf - Kanal göstergesi -->
          <div class="absolute left-0 top-0 bottom-0 w-1 ${channelBarBg}"></div>

          <div class="flex flex-col p-5 pl-6">
            <!-- Üst kısım: Başlık ve kanal badge -->
            <div class="flex items-start justify-between gap-3 mb-3">
              <h3 class="flex-1 text-base font-semibold text-gray-900 dark:text-white truncate">
                ${escapeHtml(tpl.name)}
              </h3>
              <div class="flex items-center gap-2 flex-shrink-0">
                <span class="inline-flex items-center gap-1.5 px-2.5 py-1 text-xs font-medium rounded-lg border ${channelBg} ${channelText} ${channelBorder}">
                  <i class="fas ${channelIcon}"></i>
                  ${channelLabel}
                </span>
                ${tpl.is_default ? '<span class="inline-flex items-center gap-1 px-2 py-0.5 text-[10px] font-semibold rounded-md bg-amber-100 dark:bg-amber-900/30 text-amber-700 dark:text-amber-300 border border-amber-300 dark:border-amber-700"><i class="fas fa-star"></i> Varsayılan</span>' : ''}
              </div>
            </div>

            <!-- Konu satırı -->
            <div class="mb-3 pb-3 border-b border-gray-100 dark:border-gray-800">
              <p class="text-xs text-gray-500 dark:text-gray-400 mb-1 font-medium">Konu Başlığı</p>
              <p class="text-sm text-gray-800 dark:text-gray-200 font-medium">${subject}</p>
            </div>

            <!-- İçerik ön izleme -->
            <div class="mb-4">
              <p class="text-xs text-gray-500 dark:text-gray-400 mb-1.5 font-medium">İçerik Ön İzleme</p>
              <div class="text-xs text-gray-700 dark:text-gray-300 bg-gray-50 dark:bg-gray-800/50 rounded-lg p-3 border border-gray-100 dark:border-gray-800 leading-relaxed truncate">
                ${preview}${bodyText.length > 120 ? '...' : ''}
              </div>
            </div>

            <!-- Değişkenler -->
            ${usedVariables.length > 0 ? `
              <div class="mb-4">
                <p class="text-xs text-gray-500 dark:text-gray-400 mb-2 font-medium">
                  <i class="fas fa-code text-[10px] mr-1"></i>
                  Kullanılan Değişkenler
                </p>
                <div class="flex flex-wrap gap-1.5">
                  ${placeholders}
                  ${usedVariables.length > 6 ? `<span class="inline-flex items-center px-2 py-0.5 text-[10px] font-medium text-gray-500 dark:text-gray-400">+${usedVariables.length - 6} daha</span>` : ''}
                </div>
              </div>
            ` : ''}

            <!-- Alt kısım: Aksiyon butonları -->
            ${actions ? `
              <div class="flex items-center justify-end gap-2 pt-3 mt-auto border-t border-gray-100 dark:border-gray-800">
                ${actions}
              </div>
            ` : ''}
          </div>
        </div>`;
    }).join('');

    if (state.canEdit) {
      listEl.querySelectorAll('[data-role="edit-template"]').forEach((btn) => {
        btn.addEventListener('click', () => {
          const id = Number(btn.dataset.templateId);
          if (!Number.isNaN(id)) {
            const tpl = state.cache.find((item) => item.id === id);
            if (tpl) openTemplateModal(tpl);
          }
        });
      });
      listEl.querySelectorAll('[data-role="delete-template"]').forEach((btn) => {
        btn.addEventListener('click', () => {
          const id = Number(btn.dataset.templateId);
          if (!Number.isNaN(id)) deleteTemplate(id);
        });
      });
    }
  }

  function openTemplateModal(template) {
    if (!state.canEdit) return;
    state.editingId = template ? template.id : null;
    const modal = document.getElementById('template-modal');
    if (!modal) return;
    document.getElementById('template-modal-title').textContent = template ? 'Şablon Düzenle' : 'Şablon Oluştur';
    document.getElementById('template-name').value = template ? (template.name || '') : '';
    document.getElementById('template-channel').value = template ? template.channel : 'all';
    document.getElementById('template-default').checked = false; // Varsayılan her zaman false
    document.getElementById('template-active').checked = true; // Aktif her zaman true
    document.getElementById('template-subject').value = template && template.subject ? template.subject : '';
    document.getElementById('template-body').value = template ? (template.body || '') : '';
    modal.classList.remove('hidden');
    modal.classList.add('flex');
    document.body.classList.add('overflow-hidden');
    // Ön izlemeyi hemen güncelle
    updatePreview();
  }

  function closeTemplateModal() {
    const modal = document.getElementById('template-modal');
    if (!modal) return;
    modal.classList.add('hidden');
    modal.classList.remove('flex');
    document.body.classList.remove('overflow-hidden');
    state.editingId = null;
  }

  async function saveTemplate() {
    const name = (document.getElementById('template-name').value || '').trim();
    const channel = document.getElementById('template-channel').value || 'email';
    const subject = (document.getElementById('template-subject').value || '').trim();
    const body = (document.getElementById('template-body').value || '').trim();
    const isDefault = document.getElementById('template-default').checked;
    const isActive = document.getElementById('template-active').checked;

    if (!name || !body) {
      setTemplatesMessage(texts.saveMissing, true);
      return;
    }

    const payload = {
      name,
      type: 'target',
      channel,
      subject: subject || null,
      body,
      variables: state.placeholders.map((ph) => ph.key),
      is_default: isDefault,
      is_active: isActive
    };

    const method = state.editingId ? 'PUT' : 'POST';
    const url = state.editingId ? `/api/new-notifications/templates/${state.editingId}` : '/api/new-notifications/templates';

    try {
      const res = await fetch(url, {
        method,
        headers: authHeaders(),
        body: JSON.stringify(payload)
      });
      const data = await res.json();
      if (!res.ok || data.success === false) {
        throw new Error(data.error || texts.saveError);
      }
      setTemplatesMessage(state.editingId ? texts.updateSuccess : texts.createSuccess);
      closeTemplateModal();
      refreshTemplateList();
    } catch (err) {
      setTemplatesMessage(`${texts.saveError}: ${err && err.message ? err.message : ''}`, true);
    }
  }

  async function deleteTemplate(templateId) {
    if (!state.canEdit) return;
    if (!window.confirm(texts.deleteConfirm)) {
      return;
    }
    try {
      const res = await fetch(`/api/new-notifications/templates/${templateId}`, {
        method: 'DELETE',
        headers: authHeaders()
      });
      const data = await res.json();
      if (!res.ok || data.success === false) {
        throw new Error(data.error || texts.deleteError);
      }
      setTemplatesMessage(texts.deleteSuccess);
      refreshTemplateList();
    } catch (err) {
      setTemplatesMessage(`${texts.deleteError}: ${err && err.message ? err.message : ''}`, true);
    }
  }

  function bindTemplateButtons() {
    const openBtn = document.getElementById('btn-open-template-modal');
    if (openBtn) {
      if (state.canEdit) openBtn.onclick = () => openTemplateModal();
      else openBtn.style.display = 'none';
    }
    const refreshBtn = document.getElementById('btn-refresh-templates');
    if (refreshBtn) refreshBtn.onclick = () => refreshTemplateList(true);
    const closeBtn = document.getElementById('btn-close-template-modal');
    if (closeBtn) closeBtn.onclick = closeTemplateModal;
    const cancelBtn = document.getElementById('btn-cancel-template-modal');
    if (cancelBtn) cancelBtn.onclick = closeTemplateModal;
    const saveBtn = document.getElementById('btn-save-template');
    if (saveBtn) saveBtn.onclick = () => {
      if (state.canEdit) saveTemplate();
    };
    ['template-subject', 'template-body'].forEach((id) => {
      const el = document.getElementById(id);
      if (el) {
        el.addEventListener('focus', () => { state.activeInput = el; });
        // Ön izleme için input event'i ekle
        el.addEventListener('input', () => { updatePreview(); });
      }
    });
  }

  window.initNotificationTemplatesPage = function() {
    state.canEdit = window.canEditModule ? window.canEditModule('notification_settings') : true;
    state.activeInput = null;
    bindTemplateButtons();
    loadTemplatePlaceholders().finally(renderTemplatePlaceholderList);
    refreshTemplateList();
  };
})();

window.testEmailSettings = () => {
    const smtpHost = document.getElementById('smtpHost');
    const smtpPort = document.getElementById('smtpPort');
    const username = document.getElementById('username');
    const password = document.getElementById('password');
    const fromEmail = document.getElementById('fromEmail');
    const fromName = document.getElementById('fromName');
    const notificationEmail = document.getElementById('notificationEmail');
    const useTLS = document.getElementById('useTLS');
    
    if (!smtpHost || !smtpPort || !username || !password || !fromEmail || !notificationEmail) {
        Toast.show({
            type: 'warning',
            title: 'Uyarı',
            message: 'Lütfen tüm gerekli alanları doldurun!',
            timeout: 3000
        });
        return;
    }
    
    const settings = {
        host: smtpHost.value,
        port: smtpPort.value,
        username: username.value,
        password: password.value,
        fromEmail: fromEmail.value,
        fromName: fromName.value,
        notificationEmail: notificationEmail.value,
        useTLS: useTLS.checked
    };

    console.log('Frontend settings:', settings);

    // Validation
    if (!settings.host || !settings.port || !settings.username || !settings.password || !settings.fromEmail || !settings.notificationEmail) {
        Toast.show({
            type: 'warning',
            title: 'Uyarı',
            message: 'Lütfen tüm gerekli alanları doldurun!',
            timeout: 3000
        });
        return;
    }

    const port = parseInt(settings.port);
    if (isNaN(port) || port <= 0 || port > 65535) {
        Toast.show({
            type: 'warning',
            title: 'Uyarı',
            message: 'Geçerli bir port numarası girin!',
            timeout: 3000
        });
        return;
    }

    fetch('/api/notifications/test/email', {
        method: 'POST',
        headers: {
            'Content-Type': 'application/json',
            'Authorization': 'Bearer ' + localStorage.getItem('token')
        },
        body: JSON.stringify({
            config: {
                enabled: true,
                smtp_host: settings.host,
                smtp_port: parseInt(settings.port),
                username: settings.username,
                password: settings.password,
                from_email: settings.fromEmail,
                from_name: settings.fromName,
                use_tls: settings.useTLS,
                notification_email: settings.notificationEmail
            }
        })
    })
    .then(response => response.json())
    .then(data => {
        console.log('Test response:', data);
        if (data.success) {
            Toast.show({
                type: 'success',
                title: 'Başarılı',
                message: 'Test başarılı! Email gönderildi.',
                timeout: 3000
            });
        } else {
            Toast.show({
                type: 'error',
                title: 'Hata',
                message: data.error || 'Bilinmeyen hata',
                timeout: 5000
            });
        }
    })
    .catch(error => {
        console.error('Error:', error);
        Toast.show({
            type: 'error',
            title: 'Hata',
            message: error.message,
            timeout: 5000
        });
    });
};

// Performance Trends global functions
window.applyPerformanceFilters = () => {
    const dateRange = document.getElementById('perfDateRange')?.value;
    const targetFilter = document.getElementById('perfTargetFilter')?.value;
    const metric = document.getElementById('perfMetric')?.value;
    
    console.log('Applying performance filters:', { dateRange, targetFilter, metric });
    
    // Reload performance trends data with filters
    app().loadPerformanceTrendsData();
    
    Toast.show({
        type: 'success',
        title: 'Filtreler Uygulandı',
        message: 'Performance trends güncellendi.',
        timeout: 3000
    });
};

// Email notification functions
window.loadNotificationSettings = () => {
    fetch('/api/notifications/config', {
        method: 'GET',
        headers: {
            'Authorization': 'Bearer ' + localStorage.getItem('token')
        }
    })
    .then(response => response.json())
    .then(data => {
        console.log('Loaded config:', data);
        if (data.data && data.data.email) {
            const emailConfig = data.data.email;
            
            // Form alanlarını doldur
            const smtpHost = document.getElementById('smtpHost');
            const smtpPort = document.getElementById('smtpPort');
            const username = document.getElementById('username');
            const password = document.getElementById('password');
            const fromEmail = document.getElementById('fromEmail');
            const fromName = document.getElementById('fromName');
            const notificationEmail = document.getElementById('notificationEmail');
            const useTLS = document.getElementById('useTLS');
            
            if (smtpHost) smtpHost.value = emailConfig.smtp_host || 'smtp.gmail.com';
            if (smtpPort) smtpPort.value = emailConfig.smtp_port || '587';
            if (username) username.value = emailConfig.username || '';
            if (password) password.value = emailConfig.password || '';
            if (fromEmail) fromEmail.value = emailConfig.from_email || '';
            if (fromName) fromName.value = emailConfig.from_name || 'SysTrack';
            if (notificationEmail) notificationEmail.value = emailConfig.notification_email || '';
            if (useTLS) useTLS.checked = emailConfig.use_tls !== false;
        } else {
            // Varsayılan değerleri ayarla
            const smtpHost = document.getElementById('smtpHost');
            const smtpPort = document.getElementById('smtpPort');
            const useTLS = document.getElementById('useTLS');
            
            if (smtpHost) smtpHost.value = 'smtp.gmail.com';
            if (smtpPort) smtpPort.value = '587';
            if (useTLS) useTLS.checked = true;
        }
    })
    .catch(error => {
        console.error('Error loading settings:', error);
        // Varsayılan değerleri ayarla
        const smtpHost = document.getElementById('smtpHost');
        const smtpPort = document.getElementById('smtpPort');
        const useTLS = document.getElementById('useTLS');
        
        if (smtpHost) smtpHost.value = 'smtp.gmail.com';
        if (smtpPort) smtpPort.value = '587';
        if (useTLS) useTLS.checked = true;
    });
};

window.saveEmailSettings = () => {
    const smtpHost = document.getElementById('smtpHost');
    const smtpPort = document.getElementById('smtpPort');
    const username = document.getElementById('username');
    const password = document.getElementById('password');
    const fromEmail = document.getElementById('fromEmail');
    const fromName = document.getElementById('fromName');
    const notificationEmail = document.getElementById('notificationEmail');
    const useTLS = document.getElementById('useTLS');
    
    if (!smtpHost || !smtpPort || !username || !password || !fromEmail || !notificationEmail) {
        Toast.show({
            type: 'warning',
            title: 'Uyarı',
            message: 'Lütfen tüm gerekli alanları doldurun!',
            timeout: 3000
        });
        return;
    }
    
    const settings = {
        host: smtpHost.value,
        port: smtpPort.value,
        username: username.value,
        password: password.value,
        fromEmail: fromEmail.value,
        fromName: fromName.value,
        notificationEmail: notificationEmail.value,
        useTLS: useTLS.checked
    };

    // Validation
    if (!settings.host || !settings.port || !settings.username || !settings.password || !settings.fromEmail || !settings.notificationEmail) {
        Toast.show({
            type: 'warning',
            title: 'Uyarı',
            message: 'Lütfen tüm gerekli alanları doldurun!',
            timeout: 3000
        });
        return;
    }

    const port = parseInt(settings.port);
    if (isNaN(port) || port <= 0 || port > 65535) {
        Toast.show({
            type: 'warning',
            title: 'Uyarı',
            message: 'Geçerli bir port numarası girin!',
            timeout: 3000
        });
        return;
    }

    fetch('/api/notifications/config', {
        method: 'POST',
        headers: {
            'Content-Type': 'application/json',
            'Authorization': 'Bearer ' + localStorage.getItem('token')
        },
        body: JSON.stringify({
            email: {
                enabled: true,
                smtp_host: settings.host,
                smtp_port: parseInt(settings.port),
                username: settings.username,
                password: settings.password,
                from_email: settings.fromEmail,
                from_name: settings.fromName,
                use_tls: settings.useTLS,
                notification_email: settings.notificationEmail
            },
            telegram: { enabled: false, bot_token: '', default_chat: '', webhook_url: '' },
            whatsapp: { enabled: false, access_token: '', phone_id: '', webhook_url: '' },
            webhook: { enabled: false, url: '', secret: '' }
        })
    })
    .then(response => response.json())
    .then(data => {
        console.log('Config response:', data);
        if (data.success) {
            Toast.show({
                type: 'success',
                title: 'Başarılı',
                message: 'Email ayarları kaydedildi!',
                timeout: 3000
            });
        } else {
            Toast.show({
                type: 'error',
                title: 'Hata',
                message: data.error || 'Bilinmeyen hata',
                timeout: 5000
            });
        }
    })
    .catch(error => {
        console.error('Error:', error);
        Toast.show({
            type: 'error',
            title: 'Hata',
            message: error.message,
            timeout: 5000
        });
    });
};

// Global functions for CSV import/export
function openCSVImportModal() {
    if (window.app && window.app.openCSVImportModal) {
        window.app.openCSVImportModal();
    }
}

function closeCSVImportModal() {
    if (window.app && window.app.closeCSVImportModal) {
        window.app.closeCSVImportModal();
    }
}

function importCSVFile() {
    if (window.app && window.app.importCSVFile) {
        window.app.importCSVFile();
    }
}

function exportTargetsCSV() {
    if (window.app && window.app.exportTargetsCSV) {
        window.app.exportTargetsCSV();
    }
}

function downloadCSVTemplate() {
    if (window.app && window.app.downloadCSVTemplate) {
        window.app.downloadCSVTemplate();
    }
}

// WHOIS Domain Functions
window.queryDomain = () => {
    if (window.app && window.app.queryDomain) {
        window.app.queryDomain();
    }
};

window.displayDomainInfo = (domainInfo) => {
    if (window.app && window.app.displayDomainInfo) {
        window.app.displayDomainInfo(domainInfo);
    }
};

window.formatDate = (dateString) => {
    if (window.app && window.app.formatDate) {
        return window.app.formatDate(dateString);
    }
    return 'N/A';
};

// Domain Registry Functions
window.loadQueryStats = () => {
    // Only load query stats on WHOIS Domain page
    if (!window.location.pathname.includes('whois-domain')) {
        return;
    }

    if (window.app && window.app.loadQueryStats) {
        window.app.loadQueryStats();
    }
};

window.saveDomainToRegistry = () => {
    if (window.app && window.app.saveDomainToRegistry) {
        window.app.saveDomainToRegistry();
    }
};

window.viewDomainRegistry = () => {
    if (window.app && window.app.viewDomainRegistry) {
        window.app.viewDomainRegistry();
    }
};

window.deleteDomainFromRegistry = (domainId) => {
    if (window.app && window.app.deleteDomainFromRegistry) {
        window.app.deleteDomainFromRegistry(domainId);
    }
};

window.loadDomainRegistry = () => {
    if (window.app && window.app.loadDomainRegistry) {
        window.app.loadDomainRegistry();
    }
};

// Global selection state for targets table
if (!window.selectedTargetIds) {
    window.selectedTargetIds = new Set();
}

async function fetchAllTargetIdsForCurrentFilters() {
    // Backend maksimum 100 limit uyguluyor, bu yüzden pagination ile tüm sayfaları çekelim
    const limit = 100;
    let offset = 0;
    let allIds = [];
    let hasMore = true;
    let pageCount = 0;

    const filters = window.currentFilters || currentFilters;

    console.log('[SELECT ALL] Tüm hedefler çekiliyor, filtreler:', filters);

    while (hasMore) {
        pageCount++;
        const params = new URLSearchParams();
        params.set('limit', String(limit));
        params.set('offset', String(offset));

        if (filters) {
            if (filters.search) params.append('search', filters.search);
            if (filters.type) params.append('type', filters.type);
            if (filters.status) params.append('status', filters.status);
            if (filters.tag) params.append('tag', filters.tag);
        }

        const url = `/api/targets?${params.toString()}`;
        console.log(`[SELECT ALL] Sayfa ${pageCount} çekiliyor: ${url}`);

        const response = await fetch(url, {
            headers: {
                'Authorization': 'Bearer ' + (localStorage.getItem('token') || '')
            }
        });

        if (!response.ok) {
            console.error('[SELECT ALL] API hatası:', response.status);
            throw new Error('Failed to load targets for selection');
        }

        const data = await response.json();
        console.log(`[SELECT ALL] Backend yanıtı:`, data);

        const targets = data.targets || data.data || [];
        const ids = targets
            .map(t => t.id)
            .filter(id => typeof id === 'number');

        console.log(`[SELECT ALL] Sayfa ${pageCount}: ${targets.length} hedef, ${ids.length} ID çekildi`);
        console.log(`[SELECT ALL] data.total: ${data.total}, data.has_more: ${data.has_more}`);

        allIds = allIds.concat(ids);

        // Backend'den gelen has_more bilgisine MUTLAK öncelik ver!
        const backendHasMore = data.has_more !== undefined ? data.has_more : (data.pagination && data.pagination.has_more);

        console.log(`[SELECT ALL] Karar: backendHasMore=${backendHasMore}, targets.length=${targets.length}, limit=${limit}`);

        // Eğer backend açıkça has_more bilgisi veriyorsa, SADECE ona göre karar ver
        if (backendHasMore === true) {
            // Backend daha fazla var diyor, devam et!
            offset += limit;
            hasMore = true;
            console.log(`[SELECT ALL] Backend has_more=TRUE, kesinlikle devam! Yeni offset: ${offset}`);
        } else if (backendHasMore === false) {
            // Backend daha fazla yok diyor, dur!
            hasMore = false;
            console.log(`[SELECT ALL] Backend has_more=FALSE, döngü bitiyor`);
        } else {
            // Backend has_more vermiyor, hedef sayısına bak (fallback)
            if (targets.length < limit) {
                hasMore = false;
                console.log(`[SELECT ALL] has_more yok, hedef sayısı az (${targets.length} < ${limit}), döngü bitiyor`);
            } else {
                offset += limit;
                hasMore = true;
                console.log(`[SELECT ALL] has_more yok ama hedef sayısı tam (${targets.length}), devam`);
            }
        }
    }

    console.log(`[SELECT ALL] TOPLAM: ${allIds.length} hedef seçildi, ${pageCount} sayfa çekildi`);
    return allIds;
}

// Bulk Operations Functions
window.toggleSelectAll = async () => {
    const selectAllCheckbox = document.getElementById('select-all');
    if (!selectAllCheckbox) {
        return;
    }

    if (!window.selectedTargetIds) {
        window.selectedTargetIds = new Set();
    }

    if (selectAllCheckbox.checked) {
        try {
            const allIds = await fetchAllTargetIdsForCurrentFilters();
            window.selectedTargetIds = new Set(allIds);
        } catch (error) {
            console.error('Failed to select all targets across pages:', error);
            // Fallback: only select currently visible rows
            const rows = document.querySelectorAll('#targets-table-body tr');
            rows.forEach(row => {
                if (row.style.display !== 'none') {
                    const checkbox = row.querySelector('.target-checkbox');
                    if (checkbox) {
                        checkbox.checked = true;
                        const id = parseInt(checkbox.value, 10);
                        if (!isNaN(id)) {
                            window.selectedTargetIds.add(id);
                        }
                    }
                }
            });
        }
    } else {
        // Deselect all targets
        window.selectedTargetIds.clear();
    }

    // Sync current page checkboxes with global selection state
    const rows = document.querySelectorAll('#targets-table-body tr');
    rows.forEach(row => {
        const checkbox = row.querySelector('.target-checkbox');
        if (checkbox) {
            const id = parseInt(checkbox.value, 10);
            checkbox.checked = !isNaN(id) && window.selectedTargetIds.has(id);
        }
    });

    updateSelection();
};

window.onTargetCheckboxChange = (checkbox) => {
    if (!checkbox) return;

    if (!window.selectedTargetIds) {
        window.selectedTargetIds = new Set();
    }

    const id = parseInt(checkbox.value, 10);
    if (isNaN(id)) return;

    if (checkbox.checked) {
        window.selectedTargetIds.add(id);
    } else {
        window.selectedTargetIds.delete(id);
    }

    updateSelection();
};

window.updateSelection = () => {
    // Hybrid sistem: Klasör modunda DOM-based, normal listede Set-based seçim
    const isInFolderMode = window.TargetFolders &&
                          window.TargetFolders.getState &&
                          window.TargetFolders.getState().currentFolder !== null;

    let selectedCount;

    if (isInFolderMode) {
        // Klasör modunda: Sadece görünen checkbox'ları say (pagination yok)
        selectedCount = document.querySelectorAll('.target-checkbox:checked').length;
    } else {
        // Normal liste modunda: Set-based seçim (pagination var, tüm sayfalardaki seçimler)
        selectedCount = window.selectedTargetIds ? window.selectedTargetIds.size : 0;
    }

    const selectedCountElement = document.getElementById('selected-count');
    const bulkToolbar = document.getElementById('bulk-toolbar');
    const selectAllCheckbox = document.getElementById('select-all');

    if (selectedCountElement) {
        selectedCountElement.textContent = selectedCount;
    }

    if (bulkToolbar) {
        if (selectedCount > 0) {
            bulkToolbar.classList.remove('hidden');
        } else {
            bulkToolbar.classList.add('hidden');
        }
    }

    // Update select all checkbox state
    if (selectAllCheckbox) {
        if (isInFolderMode) {
            // Klasör modunda: Görünen checkbox'lara göre
            const totalVisibleCheckboxes = document.querySelectorAll('.target-checkbox').length;

            if (selectedCount === 0) {
                selectAllCheckbox.indeterminate = false;
                selectAllCheckbox.checked = false;
            } else if (totalVisibleCheckboxes > 0 && selectedCount === totalVisibleCheckboxes) {
                selectAllCheckbox.indeterminate = false;
                selectAllCheckbox.checked = true;
            } else {
                selectAllCheckbox.indeterminate = true;
            }
        } else {
            // Normal liste modunda: Set içeriğine göre
            const totalVisibleCheckboxes = document.querySelectorAll('.target-checkbox').length;

            if (selectedCount === 0) {
                selectAllCheckbox.indeterminate = false;
                selectAllCheckbox.checked = false;
            } else if (totalVisibleCheckboxes > 0 && selectedCount === totalVisibleCheckboxes) {
                selectAllCheckbox.indeterminate = false;
                selectAllCheckbox.checked = true;
            } else {
                selectAllCheckbox.indeterminate = true;
            }
        }
    }
};

window.clearSelection = () => {
    const targetCheckboxes = document.querySelectorAll('.target-checkbox');
    const selectAllCheckbox = document.getElementById('select-all');
    
    targetCheckboxes.forEach(checkbox => {
        checkbox.checked = false;
    });

    if (window.selectedTargetIds) {
        window.selectedTargetIds.clear();
    }
    
    if (selectAllCheckbox) {
        selectAllCheckbox.checked = false;
        selectAllCheckbox.indeterminate = false;
    }
    
    updateSelection();
};

window.getSelectedTargets = () => {
    // Hybrid sistem: Klasör modunda DOM-based, normal listede Set-based seçim
    const isInFolderMode = window.TargetFolders &&
                          window.TargetFolders.getState &&
                          window.TargetFolders.getState().currentFolder !== null;

    if (isInFolderMode) {
        // Klasör modunda: Sadece görünen checked checkbox'ların ID'lerini al
        const checkedCheckboxes = document.querySelectorAll('.target-checkbox:checked');
        const ids = [];
        checkedCheckboxes.forEach(cb => {
            const id = parseInt(cb.value, 10);
            if (!isNaN(id)) {
                ids.push(id);
            }
        });
        return ids;
    } else {
        // Normal liste modunda: Set'ten tüm seçili ID'leri al (tüm sayfalar dahil)
        return window.selectedTargetIds ? Array.from(window.selectedTargetIds) : [];
    }
};

window.bulkDelete = () => {
    const selectedTargets = getSelectedTargets();
    
    if (selectedTargets.length === 0) {
        Toast.show({
            type: 'warning',
            title: 'Uyarı',
            message: 'Lütfen silinecek hedefleri seçin.',
            timeout: 3000
        });
        return;
    }
    
    if (!confirm(`${selectedTargets.length} hedefi silmek istediğinizden emin misiniz? Bu işlem geri alınamaz.`)) {
        return;
    }
    
    fetch('/api/targets/bulk/delete', {
        method: 'POST',
        headers: {
            'Content-Type': 'application/json',
            'Authorization': 'Bearer ' + localStorage.getItem('token')
        },
        body: JSON.stringify({
            target_ids: selectedTargets
        })
    })
    .then(response => response.json())
    .then(data => {
        if (data.error) {
            throw new Error(data.error);
        }
        
        Toast.show({
            type: 'success',
            title: 'Başarılı',
            message: data.message,
            timeout: 3000
        });
        
        clearSelection();
        if (window.loadTargets) {
            window.loadTargets();
        } else if (window.app && window.app.loadTargets) {
            window.app.loadTargets();
        } else {
            // Fallback: reload page
            window.location.reload();
        }
    })
    .catch(error => {
        Toast.show({
            type: 'error',
            title: 'Hata',
            message: error.message,
            timeout: 5000
        });
    });
};

// Email notification functions
window.loadNotificationSettings = () => {
    fetch('/api/notifications/config', {
        method: 'GET',
        headers: {
            'Authorization': 'Bearer ' + localStorage.getItem('token')
        }
    })
    .then(response => response.json())
    .then(data => {
        console.log('Loaded config:', data);
        if (data.data && data.data.email) {
            const emailConfig = data.data.email;
            
            // Form alanlarını doldur
            const smtpHost = document.getElementById('smtpHost');
            const smtpPort = document.getElementById('smtpPort');
            const username = document.getElementById('username');
            const password = document.getElementById('password');
            const fromEmail = document.getElementById('fromEmail');
            const fromName = document.getElementById('fromName');
            const notificationEmail = document.getElementById('notificationEmail');
            const useTLS = document.getElementById('useTLS');
            
            if (smtpHost) smtpHost.value = emailConfig.smtp_host || 'smtp.gmail.com';
            if (smtpPort) smtpPort.value = emailConfig.smtp_port || '587';
            if (username) username.value = emailConfig.username || '';
            if (password) password.value = emailConfig.password || '';
            if (fromEmail) fromEmail.value = emailConfig.from_email || '';
            if (fromName) fromName.value = emailConfig.from_name || 'SysTrack';
            if (notificationEmail) notificationEmail.value = emailConfig.notification_email || '';
            if (useTLS) useTLS.checked = emailConfig.use_tls !== false;
        } else {
            // Varsayılan değerleri ayarla
            const smtpHost = document.getElementById('smtpHost');
            const smtpPort = document.getElementById('smtpPort');
            const useTLS = document.getElementById('useTLS');
            
            if (smtpHost) smtpHost.value = 'smtp.gmail.com';
            if (smtpPort) smtpPort.value = '587';
            if (useTLS) useTLS.checked = true;
        }
    })
    .catch(error => {
        console.error('Error loading settings:', error);
        // Varsayılan değerleri ayarla
        const smtpHost = document.getElementById('smtpHost');
        const smtpPort = document.getElementById('smtpPort');
        const useTLS = document.getElementById('useTLS');
        
        if (smtpHost) smtpHost.value = 'smtp.gmail.com';
        if (smtpPort) smtpPort.value = '587';
        if (useTLS) useTLS.checked = true;
    });
};

window.saveEmailSettings = () => {
    const smtpHost = document.getElementById('smtpHost');
    const smtpPort = document.getElementById('smtpPort');
    const username = document.getElementById('username');
    const password = document.getElementById('password');
    const fromEmail = document.getElementById('fromEmail');
    const fromName = document.getElementById('fromName');
    const notificationEmail = document.getElementById('notificationEmail');
    const useTLS = document.getElementById('useTLS');
    
    if (!smtpHost || !smtpPort || !username || !password || !fromEmail || !notificationEmail) {
        Toast.show({
            type: 'warning',
            title: 'Uyarı',
            message: 'Lütfen tüm gerekli alanları doldurun!',
            timeout: 3000
        });
        return;
    }
    
    const settings = {
        host: smtpHost.value,
        port: smtpPort.value,
        username: username.value,
        password: password.value,
        fromEmail: fromEmail.value,
        fromName: fromName.value,
        notificationEmail: notificationEmail.value,
        useTLS: useTLS.checked
    };

    // Validation
    if (!settings.host || !settings.port || !settings.username || !settings.password || !settings.fromEmail || !settings.notificationEmail) {
        Toast.show({
            type: 'warning',
            title: 'Uyarı',
            message: 'Lütfen tüm gerekli alanları doldurun!',
            timeout: 3000
        });
        return;
    }

    const port = parseInt(settings.port);
    if (isNaN(port) || port <= 0 || port > 65535) {
        Toast.show({
            type: 'warning',
            title: 'Uyarı',
            message: 'Geçerli bir port numarası girin!',
            timeout: 3000
        });
        return;
    }

    fetch('/api/notifications/config', {
        method: 'POST',
        headers: {
            'Content-Type': 'application/json',
            'Authorization': 'Bearer ' + localStorage.getItem('token')
        },
        body: JSON.stringify({
            email: {
                enabled: true,
                smtp_host: settings.host,
                smtp_port: parseInt(settings.port),
                username: settings.username,
                password: settings.password,
                from_email: settings.fromEmail,
                from_name: settings.fromName,
                use_tls: settings.useTLS,
                notification_email: settings.notificationEmail
            },
            telegram: { enabled: false, bot_token: '', default_chat: '', webhook_url: '' },
            whatsapp: { enabled: false, access_token: '', phone_id: '', webhook_url: '' },
            webhook: { enabled: false, url: '', secret: '' }
        })
    })
    .then(response => response.json())
    .then(data => {
        console.log('Config response:', data);
        if (data.success) {
            Toast.show({
                type: 'success',
                title: 'Başarılı',
                message: 'Email ayarları kaydedildi!',
                timeout: 3000
            });
        } else {
            Toast.show({
                type: 'error',
                title: 'Hata',
                message: data.error || 'Bilinmeyen hata',
                timeout: 5000
            });
        }
    })
    .catch(error => {
        console.error('Error:', error);
        Toast.show({
            type: 'error',
            title: 'Hata',
            message: error.message,
            timeout: 5000
        });
    });
};

// Email notification functions
window.loadNotificationSettings = () => {
    fetch('/api/notifications/config', {
        method: 'GET',
        headers: {
            'Authorization': 'Bearer ' + localStorage.getItem('token')
        }
    })
    .then(response => response.json())
    .then(data => {
        console.log('Loaded config:', data);
        if (data.data && data.data.email) {
            const emailConfig = data.data.email;
            
            // Form alanlarını doldur
            const smtpHost = document.getElementById('smtpHost');
            const smtpPort = document.getElementById('smtpPort');
            const username = document.getElementById('username');
            const password = document.getElementById('password');
            const fromEmail = document.getElementById('fromEmail');
            const fromName = document.getElementById('fromName');
            const notificationEmail = document.getElementById('notificationEmail');
            const useTLS = document.getElementById('useTLS');
            
            if (smtpHost) smtpHost.value = emailConfig.smtp_host || 'smtp.gmail.com';
            if (smtpPort) smtpPort.value = emailConfig.smtp_port || '587';
            if (username) username.value = emailConfig.username || '';
            if (password) password.value = emailConfig.password || '';
            if (fromEmail) fromEmail.value = emailConfig.from_email || '';
            if (fromName) fromName.value = emailConfig.from_name || 'SysTrack';
            if (notificationEmail) notificationEmail.value = emailConfig.notification_email || '';
            if (useTLS) useTLS.checked = emailConfig.use_tls !== false;
        } else {
            // Varsayılan değerleri ayarla
            const smtpHost = document.getElementById('smtpHost');
            const smtpPort = document.getElementById('smtpPort');
            const useTLS = document.getElementById('useTLS');
            
            if (smtpHost) smtpHost.value = 'smtp.gmail.com';
            if (smtpPort) smtpPort.value = '587';
            if (useTLS) useTLS.checked = true;
        }
    })
    .catch(error => {
        console.error('Error loading settings:', error);
        // Varsayılan değerleri ayarla
        const smtpHost = document.getElementById('smtpHost');
        const smtpPort = document.getElementById('smtpPort');
        const useTLS = document.getElementById('useTLS');
        
        if (smtpHost) smtpHost.value = 'smtp.gmail.com';
        if (smtpPort) smtpPort.value = '587';
        if (useTLS) useTLS.checked = true;
    });
};

window.saveEmailSettings = () => {
    const smtpHost = document.getElementById('smtpHost');
    const smtpPort = document.getElementById('smtpPort');
    const username = document.getElementById('username');
    const password = document.getElementById('password');
    const fromEmail = document.getElementById('fromEmail');
    const fromName = document.getElementById('fromName');
    const notificationEmail = document.getElementById('notificationEmail');
    const useTLS = document.getElementById('useTLS');
    
    if (!smtpHost || !smtpPort || !username || !password || !fromEmail || !notificationEmail) {
        Toast.show({
            type: 'warning',
            title: 'Uyarı',
            message: 'Lütfen tüm gerekli alanları doldurun!',
            timeout: 3000
        });
        return;
    }
    
    const settings = {
        host: smtpHost.value,
        port: smtpPort.value,
        username: username.value,
        password: password.value,
        fromEmail: fromEmail.value,
        fromName: fromName.value,
        notificationEmail: notificationEmail.value,
        useTLS: useTLS.checked
    };

    // Validation
    if (!settings.host || !settings.port || !settings.username || !settings.password || !settings.fromEmail || !settings.notificationEmail) {
        Toast.show({
            type: 'warning',
            title: 'Uyarı',
            message: 'Lütfen tüm gerekli alanları doldurun!',
            timeout: 3000
        });
        return;
    }

    const port = parseInt(settings.port);
    if (isNaN(port) || port <= 0 || port > 65535) {
        Toast.show({
            type: 'warning',
            title: 'Uyarı',
            message: 'Geçerli bir port numarası girin!',
            timeout: 3000
        });
        return;
    }

    fetch('/api/notifications/config', {
        method: 'POST',
        headers: {
            'Content-Type': 'application/json',
            'Authorization': 'Bearer ' + localStorage.getItem('token')
        },
        body: JSON.stringify({
            email: {
                enabled: true,
                smtp_host: settings.host,
                smtp_port: parseInt(settings.port),
                username: settings.username,
                password: settings.password,
                from_email: settings.fromEmail,
                from_name: settings.fromName,
                use_tls: settings.useTLS,
                notification_email: settings.notificationEmail
            },
            telegram: { enabled: false, bot_token: '', default_chat: '', webhook_url: '' },
            whatsapp: { enabled: false, access_token: '', phone_id: '', webhook_url: '' },
            webhook: { enabled: false, url: '', secret: '' }
        })
    })
    .then(response => response.json())
    .then(data => {
        console.log('Config response:', data);
        if (data.success) {
            Toast.show({
                type: 'success',
                title: 'Başarılı',
                message: 'Email ayarları kaydedildi!',
                timeout: 3000
            });
        } else {
            Toast.show({
                type: 'error',
                title: 'Hata',
                message: data.error || 'Bilinmeyen hata',
                timeout: 5000
            });
        }
    })
    .catch(error => {
        console.error('Error:', error);
        Toast.show({
            type: 'error',
            title: 'Hata',
            message: error.message,
            timeout: 5000
        });
    });
};

// Email notification functions
window.loadNotificationSettings = () => {
    fetch('/api/notifications/config', {
        method: 'GET',
        headers: {
            'Authorization': 'Bearer ' + localStorage.getItem('token')
        }
    })
    .then(response => response.json())
    .then(data => {
        console.log('Loaded config:', data);
        if (data.data && data.data.email) {
            const emailConfig = data.data.email;
            
            // Form alanlarını doldur
            const smtpHost = document.getElementById('smtpHost');
            const smtpPort = document.getElementById('smtpPort');
            const username = document.getElementById('username');
            const password = document.getElementById('password');
            const fromEmail = document.getElementById('fromEmail');
            const fromName = document.getElementById('fromName');
            const notificationEmail = document.getElementById('notificationEmail');
            const useTLS = document.getElementById('useTLS');
            
            if (smtpHost) smtpHost.value = emailConfig.smtp_host || 'smtp.gmail.com';
            if (smtpPort) smtpPort.value = emailConfig.smtp_port || '587';
            if (username) username.value = emailConfig.username || '';
            if (password) password.value = emailConfig.password || '';
            if (fromEmail) fromEmail.value = emailConfig.from_email || '';
            if (fromName) fromName.value = emailConfig.from_name || 'SysTrack';
            if (notificationEmail) notificationEmail.value = emailConfig.notification_email || '';
            if (useTLS) useTLS.checked = emailConfig.use_tls !== false;
        } else {
            // Varsayılan değerleri ayarla
            const smtpHost = document.getElementById('smtpHost');
            const smtpPort = document.getElementById('smtpPort');
            const useTLS = document.getElementById('useTLS');
            
            if (smtpHost) smtpHost.value = 'smtp.gmail.com';
            if (smtpPort) smtpPort.value = '587';
            if (useTLS) useTLS.checked = true;
        }
    })
    .catch(error => {
        console.error('Error loading settings:', error);
        // Varsayılan değerleri ayarla
        const smtpHost = document.getElementById('smtpHost');
        const smtpPort = document.getElementById('smtpPort');
        const useTLS = document.getElementById('useTLS');
        
        if (smtpHost) smtpHost.value = 'smtp.gmail.com';
        if (smtpPort) smtpPort.value = '587';
        if (useTLS) useTLS.checked = true;
    });
};

window.saveEmailSettings = () => {
    const smtpHost = document.getElementById('smtpHost');
    const smtpPort = document.getElementById('smtpPort');
    const username = document.getElementById('username');
    const password = document.getElementById('password');
    const fromEmail = document.getElementById('fromEmail');
    const fromName = document.getElementById('fromName');
    const notificationEmail = document.getElementById('notificationEmail');
    const useTLS = document.getElementById('useTLS');
    
    if (!smtpHost || !smtpPort || !username || !password || !fromEmail || !notificationEmail) {
        Toast.show({
            type: 'warning',
            title: 'Uyarı',
            message: 'Lütfen tüm gerekli alanları doldurun!',
            timeout: 3000
        });
        return;
    }
    
    const settings = {
        host: smtpHost.value,
        port: smtpPort.value,
        username: username.value,
        password: password.value,
        fromEmail: fromEmail.value,
        fromName: fromName.value,
        notificationEmail: notificationEmail.value,
        useTLS: useTLS.checked
    };

    // Validation
    if (!settings.host || !settings.port || !settings.username || !settings.password || !settings.fromEmail || !settings.notificationEmail) {
        Toast.show({
            type: 'warning',
            title: 'Uyarı',
            message: 'Lütfen tüm gerekli alanları doldurun!',
            timeout: 3000
        });
        return;
    }

    const port = parseInt(settings.port);
    if (isNaN(port) || port <= 0 || port > 65535) {
        Toast.show({
            type: 'warning',
            title: 'Uyarı',
            message: 'Geçerli bir port numarası girin!',
            timeout: 3000
        });
        return;
    }

    fetch('/api/notifications/config', {
        method: 'POST',
        headers: {
            'Content-Type': 'application/json',
            'Authorization': 'Bearer ' + localStorage.getItem('token')
        },
        body: JSON.stringify({
            email: {
                enabled: true,
                smtp_host: settings.host,
                smtp_port: parseInt(settings.port),
                username: settings.username,
                password: settings.password,
                from_email: settings.fromEmail,
                from_name: settings.fromName,
                use_tls: settings.useTLS,
                notification_email: settings.notificationEmail
            },
            telegram: { enabled: false, bot_token: '', default_chat: '', webhook_url: '' },
            whatsapp: { enabled: false, access_token: '', phone_id: '', webhook_url: '' },
            webhook: { enabled: false, url: '', secret: '' }
        })
    })
    .then(response => response.json())
    .then(data => {
        console.log('Config response:', data);
        if (data.success) {
            Toast.show({
                type: 'success',
                title: 'Başarılı',
                message: 'Email ayarları kaydedildi!',
                timeout: 3000
            });
        } else {
            Toast.show({
                type: 'error',
                title: 'Hata',
                message: data.error || 'Bilinmeyen hata',
                timeout: 5000
            });
        }
    })
    .catch(error => {
        console.error('Error:', error);
        Toast.show({
            type: 'error',
            title: 'Hata',
            message: error.message,
            timeout: 5000
        });
    });
};

// Email notification functions
window.loadNotificationSettings = () => {
    fetch('/api/notifications/config', {
        method: 'GET',
        headers: {
            'Authorization': 'Bearer ' + localStorage.getItem('token')
        }
    })
    .then(response => response.json())
    .then(data => {
        console.log('Loaded config:', data);
        if (data.data && data.data.email) {
            const emailConfig = data.data.email;
            
            // Form alanlarını doldur
            const smtpHost = document.getElementById('smtpHost');
            const smtpPort = document.getElementById('smtpPort');
            const username = document.getElementById('username');
            const password = document.getElementById('password');
            const fromEmail = document.getElementById('fromEmail');
            const fromName = document.getElementById('fromName');
            const notificationEmail = document.getElementById('notificationEmail');
            const useTLS = document.getElementById('useTLS');
            
            if (smtpHost) smtpHost.value = emailConfig.smtp_host || 'smtp.gmail.com';
            if (smtpPort) smtpPort.value = emailConfig.smtp_port || '587';
            if (username) username.value = emailConfig.username || '';
            if (password) password.value = emailConfig.password || '';
            if (fromEmail) fromEmail.value = emailConfig.from_email || '';
            if (fromName) fromName.value = emailConfig.from_name || 'SysTrack';
            if (notificationEmail) notificationEmail.value = emailConfig.notification_email || '';
            if (useTLS) useTLS.checked = emailConfig.use_tls !== false;
        } else {
            // Varsayılan değerleri ayarla
            const smtpHost = document.getElementById('smtpHost');
            const smtpPort = document.getElementById('smtpPort');
            const useTLS = document.getElementById('useTLS');
            
            if (smtpHost) smtpHost.value = 'smtp.gmail.com';
            if (smtpPort) smtpPort.value = '587';
            if (useTLS) useTLS.checked = true;
        }
    })
    .catch(error => {
        console.error('Error loading settings:', error);
        // Varsayılan değerleri ayarla
        const smtpHost = document.getElementById('smtpHost');
        const smtpPort = document.getElementById('smtpPort');
        const useTLS = document.getElementById('useTLS');
        
        if (smtpHost) smtpHost.value = 'smtp.gmail.com';
        if (smtpPort) smtpPort.value = '587';
        if (useTLS) useTLS.checked = true;
    });
};

window.saveEmailSettings = () => {
    const smtpHost = document.getElementById('smtpHost');
    const smtpPort = document.getElementById('smtpPort');
    const username = document.getElementById('username');
    const password = document.getElementById('password');
    const fromEmail = document.getElementById('fromEmail');
    const fromName = document.getElementById('fromName');
    const notificationEmail = document.getElementById('notificationEmail');
    const useTLS = document.getElementById('useTLS');
    
    if (!smtpHost || !smtpPort || !username || !password || !fromEmail || !notificationEmail) {
        Toast.show({
            type: 'warning',
            title: 'Uyarı',
            message: 'Lütfen tüm gerekli alanları doldurun!',
            timeout: 3000
        });
        return;
    }
    
    const settings = {
        host: smtpHost.value,
        port: smtpPort.value,
        username: username.value,
        password: password.value,
        fromEmail: fromEmail.value,
        fromName: fromName.value,
        notificationEmail: notificationEmail.value,
        useTLS: useTLS.checked
    };

    // Validation
    if (!settings.host || !settings.port || !settings.username || !settings.password || !settings.fromEmail || !settings.notificationEmail) {
        Toast.show({
            type: 'warning',
            title: 'Uyarı',
            message: 'Lütfen tüm gerekli alanları doldurun!',
            timeout: 3000
        });
        return;
    }

    const port = parseInt(settings.port);
    if (isNaN(port) || port <= 0 || port > 65535) {
        Toast.show({
            type: 'warning',
            title: 'Uyarı',
            message: 'Geçerli bir port numarası girin!',
            timeout: 3000
        });
        return;
    }

    fetch('/api/notifications/config', {
        method: 'POST',
        headers: {
            'Content-Type': 'application/json',
            'Authorization': 'Bearer ' + localStorage.getItem('token')
        },
        body: JSON.stringify({
            email: {
                enabled: true,
                smtp_host: settings.host,
                smtp_port: parseInt(settings.port),
                username: settings.username,
                password: settings.password,
                from_email: settings.fromEmail,
                from_name: settings.fromName,
                use_tls: settings.useTLS,
                notification_email: settings.notificationEmail
            },
            telegram: { enabled: false, bot_token: '', default_chat: '', webhook_url: '' },
            whatsapp: { enabled: false, access_token: '', phone_id: '', webhook_url: '' },
            webhook: { enabled: false, url: '', secret: '' }
        })
    })
    .then(response => response.json())
    .then(data => {
        console.log('Config response:', data);
        if (data.success) {
            Toast.show({
                type: 'success',
                title: 'Başarılı',
                message: 'Email ayarları kaydedildi!',
                timeout: 3000
            });
        } else {
            Toast.show({
                type: 'error',
                title: 'Hata',
                message: data.error || 'Bilinmeyen hata',
                timeout: 5000
            });
        }
    })
    .catch(error => {
        console.error('Error:', error);
        Toast.show({
            type: 'error',
            title: 'Hata',
            message: error.message,
            timeout: 5000
        });
    });
};

// Email notification functions
window.loadNotificationSettings = () => {
    fetch('/api/notifications/config', {
        method: 'GET',
        headers: {
            'Authorization': 'Bearer ' + localStorage.getItem('token')
        }
    })
    .then(response => response.json())
    .then(data => {
        console.log('Loaded config:', data);
        if (data.data && data.data.email) {
            const emailConfig = data.data.email;
            
            // Form alanlarını doldur
            const smtpHost = document.getElementById('smtpHost');
            const smtpPort = document.getElementById('smtpPort');
            const username = document.getElementById('username');
            const password = document.getElementById('password');
            const fromEmail = document.getElementById('fromEmail');
            const fromName = document.getElementById('fromName');
            const notificationEmail = document.getElementById('notificationEmail');
            const useTLS = document.getElementById('useTLS');
            
            if (smtpHost) smtpHost.value = emailConfig.smtp_host || 'smtp.gmail.com';
            if (smtpPort) smtpPort.value = emailConfig.smtp_port || '587';
            if (username) username.value = emailConfig.username || '';
            if (password) password.value = emailConfig.password || '';
            if (fromEmail) fromEmail.value = emailConfig.from_email || '';
            if (fromName) fromName.value = emailConfig.from_name || 'SysTrack';
            if (notificationEmail) notificationEmail.value = emailConfig.notification_email || '';
            if (useTLS) useTLS.checked = emailConfig.use_tls !== false;
        } else {
            // Varsayılan değerleri ayarla
            const smtpHost = document.getElementById('smtpHost');
            const smtpPort = document.getElementById('smtpPort');
            const useTLS = document.getElementById('useTLS');
            
            if (smtpHost) smtpHost.value = 'smtp.gmail.com';
            if (smtpPort) smtpPort.value = '587';
            if (useTLS) useTLS.checked = true;
        }
    })
    .catch(error => {
        console.error('Error loading settings:', error);
        // Varsayılan değerleri ayarla
        const smtpHost = document.getElementById('smtpHost');
        const smtpPort = document.getElementById('smtpPort');
        const useTLS = document.getElementById('useTLS');
        
        if (smtpHost) smtpHost.value = 'smtp.gmail.com';
        if (smtpPort) smtpPort.value = '587';
        if (useTLS) useTLS.checked = true;
    });
};

window.saveEmailSettings = () => {
    const smtpHost = document.getElementById('smtpHost');
    const smtpPort = document.getElementById('smtpPort');
    const username = document.getElementById('username');
    const password = document.getElementById('password');
    const fromEmail = document.getElementById('fromEmail');
    const fromName = document.getElementById('fromName');
    const notificationEmail = document.getElementById('notificationEmail');
    const useTLS = document.getElementById('useTLS');
    
    if (!smtpHost || !smtpPort || !username || !password || !fromEmail || !notificationEmail) {
        Toast.show({
            type: 'warning',
            title: 'Uyarı',
            message: 'Lütfen tüm gerekli alanları doldurun!',
            timeout: 3000
        });
        return;
    }
    
    const settings = {
        host: smtpHost.value,
        port: smtpPort.value,
        username: username.value,
        password: password.value,
        fromEmail: fromEmail.value,
        fromName: fromName.value,
        notificationEmail: notificationEmail.value,
        useTLS: useTLS.checked
    };

    // Validation
    if (!settings.host || !settings.port || !settings.username || !settings.password || !settings.fromEmail || !settings.notificationEmail) {
        Toast.show({
            type: 'warning',
            title: 'Uyarı',
            message: 'Lütfen tüm gerekli alanları doldurun!',
            timeout: 3000
        });
        return;
    }

    const port = parseInt(settings.port);
    if (isNaN(port) || port <= 0 || port > 65535) {
        Toast.show({
            type: 'warning',
            title: 'Uyarı',
            message: 'Geçerli bir port numarası girin!',
            timeout: 3000
        });
        return;
    }

    fetch('/api/notifications/config', {
        method: 'POST',
        headers: {
            'Content-Type': 'application/json',
            'Authorization': 'Bearer ' + localStorage.getItem('token')
        },
        body: JSON.stringify({
            email: {
                enabled: true,
                smtp_host: settings.host,
                smtp_port: parseInt(settings.port),
                username: settings.username,
                password: settings.password,
                from_email: settings.fromEmail,
                from_name: settings.fromName,
                use_tls: settings.useTLS,
                notification_email: settings.notificationEmail
            },
            telegram: { enabled: false, bot_token: '', default_chat: '', webhook_url: '' },
            whatsapp: { enabled: false, access_token: '', phone_id: '', webhook_url: '' },
            webhook: { enabled: false, url: '', secret: '' }
        })
    })
    .then(response => response.json())
    .then(data => {
        console.log('Config response:', data);
        if (data.success) {
            Toast.show({
                type: 'success',
                title: 'Başarılı',
                message: 'Email ayarları kaydedildi!',
                timeout: 3000
            });
        } else {
            Toast.show({
                type: 'error',
                title: 'Hata',
                message: data.error || 'Bilinmeyen hata',
                timeout: 5000
            });
        }
    })
    .catch(error => {
        console.error('Error:', error);
        Toast.show({
            type: 'error',
            title: 'Hata',
            message: error.message,
            timeout: 5000
        });
    });
};

// Email notification functions
window.loadNotificationSettings = () => {
    fetch('/api/notifications/config', {
        method: 'GET',
        headers: {
            'Authorization': 'Bearer ' + localStorage.getItem('token')
        }
    })
    .then(response => response.json())
    .then(data => {
        console.log('Loaded config:', data);
        if (data.data && data.data.email) {
            const emailConfig = data.data.email;
            
            // Form alanlarını doldur
            const smtpHost = document.getElementById('smtpHost');
            const smtpPort = document.getElementById('smtpPort');
            const username = document.getElementById('username');
            const password = document.getElementById('password');
            const fromEmail = document.getElementById('fromEmail');
            const fromName = document.getElementById('fromName');
            const notificationEmail = document.getElementById('notificationEmail');
            const useTLS = document.getElementById('useTLS');
            
            if (smtpHost) smtpHost.value = emailConfig.smtp_host || 'smtp.gmail.com';
            if (smtpPort) smtpPort.value = emailConfig.smtp_port || '587';
            if (username) username.value = emailConfig.username || '';
            if (password) password.value = emailConfig.password || '';
            if (fromEmail) fromEmail.value = emailConfig.from_email || '';
            if (fromName) fromName.value = emailConfig.from_name || 'SysTrack';
            if (notificationEmail) notificationEmail.value = emailConfig.notification_email || '';
            if (useTLS) useTLS.checked = emailConfig.use_tls !== false;
        } else {
            // Varsayılan değerleri ayarla
            const smtpHost = document.getElementById('smtpHost');
            const smtpPort = document.getElementById('smtpPort');
            const useTLS = document.getElementById('useTLS');
            
            if (smtpHost) smtpHost.value = 'smtp.gmail.com';
            if (smtpPort) smtpPort.value = '587';
            if (useTLS) useTLS.checked = true;
        }
    })
    .catch(error => {
        console.error('Error loading settings:', error);
        // Varsayılan değerleri ayarla
        const smtpHost = document.getElementById('smtpHost');
        const smtpPort = document.getElementById('smtpPort');
        const useTLS = document.getElementById('useTLS');
        
        if (smtpHost) smtpHost.value = 'smtp.gmail.com';
        if (smtpPort) smtpPort.value = '587';
        if (useTLS) useTLS.checked = true;
    });
};

window.saveEmailSettings = () => {
    const smtpHost = document.getElementById('smtpHost');
    const smtpPort = document.getElementById('smtpPort');
    const username = document.getElementById('username');
    const password = document.getElementById('password');
    const fromEmail = document.getElementById('fromEmail');
    const fromName = document.getElementById('fromName');
    const notificationEmail = document.getElementById('notificationEmail');
    const useTLS = document.getElementById('useTLS');
    
    if (!smtpHost || !smtpPort || !username || !password || !fromEmail || !notificationEmail) {
        Toast.show({
            type: 'warning',
            title: 'Uyarı',
            message: 'Lütfen tüm gerekli alanları doldurun!',
            timeout: 3000
        });
        return;
    }
    
    const settings = {
        host: smtpHost.value,
        port: smtpPort.value,
        username: username.value,
        password: password.value,
        fromEmail: fromEmail.value,
        fromName: fromName.value,
        notificationEmail: notificationEmail.value,
        useTLS: useTLS.checked
    };

    // Validation
    if (!settings.host || !settings.port || !settings.username || !settings.password || !settings.fromEmail || !settings.notificationEmail) {
        Toast.show({
            type: 'warning',
            title: 'Uyarı',
            message: 'Lütfen tüm gerekli alanları doldurun!',
            timeout: 3000
        });
        return;
    }

    const port = parseInt(settings.port);
    if (isNaN(port) || port <= 0 || port > 65535) {
        Toast.show({
            type: 'warning',
            title: 'Uyarı',
            message: 'Geçerli bir port numarası girin!',
            timeout: 3000
        });
        return;
    }

    fetch('/api/notifications/config', {
        method: 'POST',
        headers: {
            'Content-Type': 'application/json',
            'Authorization': 'Bearer ' + localStorage.getItem('token')
        },
        body: JSON.stringify({
            email: {
                enabled: true,
                smtp_host: settings.host,
                smtp_port: parseInt(settings.port),
                username: settings.username,
                password: settings.password,
                from_email: settings.fromEmail,
                from_name: settings.fromName,
                use_tls: settings.useTLS,
                notification_email: settings.notificationEmail
            },
            telegram: { enabled: false, bot_token: '', default_chat: '', webhook_url: '' },
            whatsapp: { enabled: false, access_token: '', phone_id: '', webhook_url: '' },
            webhook: { enabled: false, url: '', secret: '' }
        })
    })
    .then(response => response.json())
    .then(data => {
        console.log('Config response:', data);
        if (data.success) {
            Toast.show({
                type: 'success',
                title: 'Başarılı',
                message: 'Email ayarları kaydedildi!',
                timeout: 3000
            });
        } else {
            Toast.show({
                type: 'error',
                title: 'Hata',
                message: data.error || 'Bilinmeyen hata',
                timeout: 5000
            });
        }
    })
    .catch(error => {
        console.error('Error:', error);
        Toast.show({
            type: 'error',
            title: 'Hata',
            message: error.message,
            timeout: 5000
        });
    });
};

// ===========================
// Targets Server-Side Pagination
// ===========================
let targetsPagination = {
    offset: 0,
    limit: 25,
    total: 0,
    hasMore: false,
    loading: false
};

// Store current filter state to preserve during WebSocket updates
let currentFilters = {
    search: '',
    type: '',
    status: '',
    tag: ''
};
window.currentFilters = currentFilters;

// Load targets from API with pagination and filters
window.loadTargets = async function(silent = false) {
    if (targetsPagination.loading) return;

    targetsPagination.loading = true;
    const tbody = document.getElementById('targets-table-body');

    // Only show loading message if not silent mode
    if (tbody && !silent) {
        tbody.innerHTML = '<tr><td colspan="7" class="px-6 py-8 text-center text-gray-500 dark:text-gray-400">Yükleniyor...</td></tr>';
    }

    try {
        // Build URL with pagination and filter parameters
        const params = new URLSearchParams({
            limit: targetsPagination.limit,
            offset: targetsPagination.offset
        });

        // Add filter parameters if they exist
        if (currentFilters.search) {
            params.append('search', currentFilters.search);
        }
        if (currentFilters.type) {
            params.append('type', currentFilters.type);
        }
        if (currentFilters.status) {
            params.append('status', currentFilters.status);
        }
        if (currentFilters.tag) {
            params.append('tag', currentFilters.tag);
        }

        const url = `/api/targets?${params.toString()}`;
        console.log('Loading targets from:', url, 'with filters:', currentFilters);
        const response = await fetch(url, {
            headers: {
                'Authorization': 'Bearer ' + (localStorage.getItem('token') || '')
            }
        });

        if (!response.ok) {
            throw new Error('Hedefler yüklenemedi');
        }

        const data = await response.json();
        console.log('Loaded targets:', data);
        const targets = data.targets || [];
        const pagination = data.pagination || {};

        // Update pagination state
        targetsPagination.total = pagination.total || 0;
        targetsPagination.offset = pagination.offset || 0;
        targetsPagination.limit = pagination.limit || 25;
        targetsPagination.hasMore = pagination.has_more || false;

        // Render targets (skip client-side filtering since it's done server-side)
        renderTargets(targets, true);

        // Update pagination UI
        updatePaginationUI();
    } catch (error) {
        console.error('Error loading targets:', error);
        if (tbody && !silent) {
            tbody.innerHTML = '<tr><td colspan="7" class="px-6 py-8 text-center text-red-500 dark:text-red-400">Hata: ' + error.message + '</td></tr>';
        }
    } finally {
        targetsPagination.loading = false;
    }
};

// Render single target as mobile card
function renderTargetMobileCard(target) {
    const tags = target.tags ? target.tags.split(',').map(t => t.trim()).filter(t => t) : [];
    const tagColors = target.tag_colors || {};

    const statusClass = target.is_online ?
        'bg-green-100 dark:bg-green-900/20 text-green-800 dark:text-green-400' :
        'bg-red-100 dark:bg-red-900/20 text-red-800 dark:text-red-400';
    const statusText = target.is_online ? 'Online' : 'Offline';

    const normalizedType = normalizeType(target.monitoring_type || target.type);
    const typeClass = getTypeBadgeClass(normalizedType);
    const displayType = normalizedType.toUpperCase();

    const tagsHTML = tags.map(tag => {
        const color = tagColors[tag] || '#3B82F6';
        return `<span class="inline-block px-2 py-1 text-xs font-medium rounded-full text-white mr-1 mb-1" style="background-color: ${color}">${escapeHtml(tag)}</span>`;
    }).join('');

    return `
        <div class="bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-700 rounded-lg p-4">
            <div class="flex items-start justify-between gap-3 mb-3">
                <div class="flex-1">
                    <h4 class="text-sm font-semibold text-gray-900 dark:text-gray-100 mb-1">${escapeHtml(target.name)}</h4>
                    <p class="text-xs text-gray-500 dark:text-gray-400">${escapeHtml(target.address)}</p>
                </div>
                <span class="px-2 py-1 text-xs font-medium rounded-full ${statusClass}">${statusText}</span>
            </div>

            <div class="space-y-2 mb-3">
                <div class="flex items-center justify-between text-xs">
                    <span class="text-gray-500 dark:text-gray-400">Tip:</span>
                    <span class="px-2 py-1 text-xs font-medium rounded-full ${typeClass}">${displayType}</span>
                </div>
                ${tags.length > 0 ? `
                    <div class="flex items-start justify-between text-xs">
                        <span class="text-gray-500 dark:text-gray-400">Etiketler:</span>
                        <div class="flex flex-wrap gap-1 justify-end">${tagsHTML}</div>
                    </div>
                ` : ''}
            </div>

            <div class="flex flex-wrap gap-2 pt-3 border-t border-gray-100 dark:border-gray-700">
                ${window.canEditModule && window.canEditModule('targets') ? `<button onclick="editTarget(${target.id})" class="text-xs font-medium text-blue-600 hover:text-blue-700 dark:text-blue-400 dark:hover:text-blue-300">Düzenle</button>` : ''}
                ${window.canEditModule && window.canEditModule('targets') ? `<button onclick="deleteTarget(${target.id})" class="text-xs font-medium text-red-600 hover:text-red-700 dark:text-red-400 dark:hover:text-red-300">Sil</button>` : ''}
                <button onclick="pingTarget(${target.id})" class="text-xs font-medium text-green-600 hover:text-green-700 dark:text-green-400 dark:hover:text-green-300">Ping</button>
                <button onclick="showDeviceServicesModal(${target.id}, '${escapeHtml(target.name)}')" class="text-xs font-medium text-purple-600 hover:text-purple-700 dark:text-purple-400 dark:hover:text-purple-300">Servisler</button>
                <button onclick="showTargetHistory(${target.id})" class="text-xs font-medium text-blue-600 hover:text-blue-700 dark:text-blue-400 dark:hover:text-blue-300">Geçmiş</button>
                <button onclick="openConnectDropdown(event)" data-target-id="${target.id}" data-target-address="${escapeHtml(target.address)}" class="text-xs font-medium text-gray-600 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-300">
                        Bağlan <i class="fas fa-chevron-down ml-1"></i>
                    </button>
                </div>
            </div>
        </div>
    `;
}

// Render targets in table
// skipClientFiltering: if true, don't apply client-side filters (used when server already filtered)
window.renderTargets = function(targets, skipClientFiltering = false) {
    const tbody = document.getElementById('targets-table-body');
    const mobileList = document.getElementById('targets-mobile-list');
    if (!tbody) return;

    console.log('renderTargets called with', targets ? targets.length : 0, 'targets', 'skipClientFiltering:', skipClientFiltering);

    if (!targets || targets.length === 0) {
        tbody.innerHTML = '<tr><td colspan="7" class="px-6 py-8 text-center text-gray-500 dark:text-gray-400">Hiç hedef bulunamadı</td></tr>';
        if (mobileList) {
            mobileList.innerHTML = '<div class="text-center py-8 text-gray-500 dark:text-gray-400">Hiç hedef bulunamadı</div>';
        }
        return;
    }

    tbody.innerHTML = targets.map(target => {
        const tags = target.tags ? target.tags.split(',').map(t => t.trim()).filter(t => t) : [];
        const tagColors = target.tag_colors || {};

        // Status
        const statusClass = target.is_online ?
            'bg-green-100 dark:bg-green-900/20 text-green-800 dark:text-green-400' :
            'bg-red-100 dark:bg-red-900/20 text-red-800 dark:text-red-400';
        const statusText = target.is_online ? 'Online' : 'Offline';

        // Type
        const normalizedType = normalizeType(target.monitoring_type || target.type);
        const typeClass = getTypeBadgeClass(normalizedType);
        const displayType = normalizedType.toUpperCase();

        // Tags HTML
        const tagsHTML = tags.map(tag => {
            const color = tagColors[tag] || '#3B82F6';
            return `<span class="inline-block px-2 py-1 text-xs font-medium rounded-full text-white mr-1 mb-1" style="background-color: ${color}">${escapeHtml(tag)}</span>`;
        }).join('');

        return `
            <tr>
                <td class="px-2 sm:px-3 md:px-6 py-2 sm:py-3 md:py-4 whitespace-nowrap">
                    ${window.canEditModule && window.canEditModule('targets') ? `<input type="checkbox" class="target-checkbox rounded border-gray-300 text-primary-600 focus:ring-primary-500" value="${target.id}" ${window.selectedTargetIds && window.selectedTargetIds.has && window.selectedTargetIds.has(target.id) ? 'checked' : ''} onchange="onTargetCheckboxChange(this)">` : ''}
                </td>
                <td class="px-2 sm:px-3 md:px-6 py-2 sm:py-3 md:py-4 whitespace-nowrap text-xs sm:text-sm font-medium text-gray-900 dark:text-gray-100">${escapeHtml(target.name)}</td>
                <td class="px-2 sm:px-3 md:px-6 py-2 sm:py-3 md:py-4 whitespace-nowrap text-xs sm:text-sm text-gray-500 dark:text-gray-400">${escapeHtml(target.address)}</td>
                <td class="hidden sm:table-cell px-2 sm:px-3 md:px-6 py-2 sm:py-3 md:py-4 whitespace-nowrap">
                    <span class="px-2 py-1 text-xs font-medium rounded-full ${typeClass}">${displayType}</span>
                </td>
                <td class="px-2 sm:px-3 md:px-6 py-2 sm:py-3 md:py-4 whitespace-nowrap">
                    <span class="px-2 py-1 text-xs font-medium rounded-full ${statusClass}">${statusText}</span>
                </td>
                <td class="hidden md:table-cell px-2 sm:px-3 md:px-6 py-2 sm:py-3 md:py-4 text-xs sm:text-sm text-gray-500 dark:text-gray-400">
                    ${tagsHTML}
                </td>
                <td class="px-2 sm:px-3 md:px-6 py-2 sm:py-3 md:py-4 text-xs sm:text-sm text-gray-500 dark:text-gray-400">
                    <div class="flex flex-wrap gap-1 sm:gap-2 items-center">
                        ${window.canEditModule && window.canEditModule('targets') ? `<button onclick="editTarget(${target.id})" class="text-primary-600 hover:text-primary-900 dark:text-primary-400 dark:hover:text-primary-300 whitespace-nowrap">Düzenle</button>` : ''}
                        ${window.canEditModule && window.canEditModule('targets') ? `<button onclick="deleteTarget(${target.id})" class="text-red-600 hover:text-red-900 dark:text-red-400 dark:hover:text-red-300 whitespace-nowrap">Sil</button>` : ''}
                        <button onclick="pingTarget(${target.id})" class="text-green-600 hover:text-green-900 dark:text-green-400 dark:hover:text-green-300 whitespace-nowrap">Ping</button>
                        <button onclick="showDeviceServicesModal(${target.id}, '${escapeHtml(target.name)}')" class="text-purple-600 hover:text-purple-900 dark:text-purple-400 dark:hover:text-purple-300 whitespace-nowrap">Servisler</button>
                        <button onclick="showTargetHistory(${target.id})" class="text-blue-600 hover:text-blue-900 dark:text-blue-400 dark:hover:text-blue-300 whitespace-nowrap hidden sm:inline">Geçmiş</button>
                        <button onclick="openConnectDropdown(event)" data-target-id="${target.id}" data-target-address="${escapeHtml(target.address)}" class="text-gray-600 hover:text-gray-900 dark:text-gray-400 dark:hover:text-gray-300 whitespace-nowrap">
                                Bağlan
                                <i class="fas fa-chevron-down ml-1"></i>
                            </button>
                        </div>
                    </div>
                </td>
            </tr>
        `;
    }).join('');

    // Render mobile list
    if (mobileList) {
        mobileList.innerHTML = targets.map(target => renderTargetMobileCard(target)).join('');
    }

    // Only apply client-side filters if server-side filtering was not used
    // This is for backward compatibility with old code that might not use loadTargets()
    // IMPORTANT: Don't reapply filters if we're already in folder mode (to prevent infinite loop)
    const isInFolderMode = window.TargetFolders &&
                          window.TargetFolders.getState &&
                          window.TargetFolders.getState().currentFolder !== null;

    if (!skipClientFiltering && !isInFolderMode) {
        console.log('Current filters:', currentFilters);
        const hasActiveFilters = currentFilters.search || currentFilters.type || currentFilters.status || currentFilters.tag;
        if (hasActiveFilters && typeof filterTargets === 'function') {
            console.log('Reapplying client-side filters after render');
            setTimeout(() => filterTargets(), 0);
        }
    } else {
        console.log('Skipping client-side filtering - server already filtered results or in folder mode');
    }
}



// Floating connect dropdown (escapes table overflow)
let connectDropdownState = null;

function ensureConnectDropdown() {
    if (connectDropdownState) return connectDropdownState;
    const menu = document.getElementById('connect-dropdown');
    if (!menu) return null;

    const sshLink = document.getElementById('connect-dropdown-ssh');
    const webLink = document.getElementById('connect-dropdown-web');

    const state = {
        menu,
        sshLink,
        webLink,
        isOpen: false,
        anchor: null
    };

    function close() {
        if (!state.isOpen) return;
        state.menu.classList.add('hidden');
        state.menu.style.visibility = '';
        state.menu.style.left = '';
        state.menu.style.top = '';
        state.isOpen = false;
        state.anchor = null;
    }

    state.close = close;

    state.menu.addEventListener('click', function(event) {
        event.stopPropagation();
    });

    document.addEventListener('click', close);
    window.addEventListener('resize', close);
    window.addEventListener('scroll', close, true);

    connectDropdownState = state;
    return state;
}

window.closeConnectDropdown = function() {
    if (connectDropdownState && connectDropdownState.close) {
        connectDropdownState.close();
    }
};

window.openConnectDropdown = function(event) {
    const state = ensureConnectDropdown();
    if (!state) return;

    event.stopPropagation();

    const button = event.currentTarget;

    if (state.isOpen && state.anchor === button) {
        state.close();
        return;
    }

    const targetId = button.dataset.targetId || '';
    const targetAddress = button.dataset.targetAddress || '';

    if (state.sshLink) {
        const params = new URLSearchParams({
            target_id: targetId,
            target_address: targetAddress
        });
        state.sshLink.href = `/admin/ssh-terminal?${params.toString()}`;
    }

    if (state.webLink) {
        state.webLink.href = `http://${targetAddress}`;
    }

    state.menu.classList.remove('hidden');
    state.menu.style.visibility = 'hidden';

    const rect = button.getBoundingClientRect();
    const menuRect = state.menu.getBoundingClientRect();
    const margin = 8;
    let left = rect.right - menuRect.width;
    left = Math.max(margin, Math.min(left, window.innerWidth - menuRect.width - margin));

    let top = rect.bottom + 0;
    if (top + menuRect.height > window.innerHeight - margin) {
        top = rect.top - menuRect.height - 0;
    }

    state.menu.style.left = `${left}px`;
    state.menu.style.top = `${top}px`;
    state.menu.style.visibility = 'visible';
    state.isOpen = true;
    state.anchor = button;
};

function normalizeType(type) {
    const t = (type || '').toLowerCase().trim();
    if (['icmp', 'ping', 'tcp'].includes(t)) return 'ping';
    if (t === 'https') return 'https';
    if (t === 'http') return 'http';
    return t || 'ping';
}

function getTypeBadgeClass(type) {
    switch (type) {
        case 'ping': return 'bg-green-100 dark:bg-green-900/20 text-green-800 dark:text-green-300';
        case 'https': return 'bg-indigo-100 dark:bg-indigo-900/20 text-indigo-800 dark:text-indigo-300';
        case 'http': return 'bg-purple-100 dark:bg-purple-900/20 text-purple-800 dark:text-purple-300';
        default: return 'bg-blue-100 dark:bg-blue-900/20 text-blue-800 dark:text-blue-400';
    }
}

function escapeHtml(text) {
    if (text == null) return '';
    const div = document.createElement('div');
    div.textContent = text;
    return div.innerHTML;
}

function updatePaginationUI() {
    const currentPage = Math.floor(targetsPagination.offset / targetsPagination.limit) + 1;
    const totalPages = Math.ceil(targetsPagination.total / targetsPagination.limit);

    console.log('Updating pagination UI:', {
        offset: targetsPagination.offset,
        limit: targetsPagination.limit,
        total: targetsPagination.total,
        currentPage,
        totalPages,
        hasMore: targetsPagination.hasMore
    });

    const paginationText = `${targetsPagination.total} kayıt | Sayfa ${currentPage} / ${Math.max(1, totalPages)}`;

    // Desktop pagination
    const infoEl = document.getElementById('pagination-info');
    if (infoEl) {
        infoEl.textContent = paginationText;
    }

    // Mobile pagination
    const infoElMobile = document.getElementById('pagination-info-mobile');
    if (infoElMobile) {
        infoElMobile.textContent = paginationText;
    }

    const prevBtn = document.getElementById('prev-page-btn');
    const nextBtn = document.getElementById('next-page-btn');
    const prevBtnMobile = document.getElementById('prev-page-btn-mobile');
    const nextBtnMobile = document.getElementById('next-page-btn-mobile');

    const shouldDisablePrev = targetsPagination.offset === 0;
    const shouldDisableNext = !targetsPagination.hasMore;

    // Desktop buttons
    if (prevBtn) {
        prevBtn.disabled = shouldDisablePrev;
        prevBtn.classList.toggle('opacity-40', shouldDisablePrev);
        prevBtn.classList.toggle('pointer-events-none', shouldDisablePrev);
    }

    if (nextBtn) {
        nextBtn.disabled = shouldDisableNext;
        nextBtn.classList.toggle('opacity-40', shouldDisableNext);
        nextBtn.classList.toggle('pointer-events-none', shouldDisableNext);
    }

    // Mobile buttons
    if (prevBtnMobile) {
        prevBtnMobile.disabled = shouldDisablePrev;
        prevBtnMobile.classList.toggle('opacity-40', shouldDisablePrev);
        prevBtnMobile.classList.toggle('pointer-events-none', shouldDisablePrev);
    }

    if (nextBtnMobile) {
        nextBtnMobile.disabled = shouldDisableNext;
        nextBtnMobile.classList.toggle('opacity-40', shouldDisableNext);
        nextBtnMobile.classList.toggle('pointer-events-none', shouldDisableNext);
    }
}

window.previousPage = function() {
    if (targetsPagination.offset > 0) {
        const newOffset = Math.max(0, targetsPagination.offset - targetsPagination.limit);
        console.log('Going to previous page:', targetsPagination.offset, '->', newOffset);
        targetsPagination.offset = newOffset;
        loadTargets();
    } else {
        console.log('Already on first page');
    }
};

window.nextPage = function() {
    if (targetsPagination.hasMore) {
        const newOffset = targetsPagination.offset + targetsPagination.limit;
        console.log('Going to next page:', targetsPagination.offset, '->', newOffset);
        targetsPagination.offset = newOffset;
        loadTargets();
    } else {
        console.log('No more pages available');
    }
};

window.changePageSize = function() {
    const select = document.getElementById('page-size-select');
    if (select) {
        const newLimit = parseInt(select.value) || 25;
        console.log('Changing page size from', targetsPagination.limit, 'to', newLimit);
        targetsPagination.limit = newLimit;
        targetsPagination.offset = 0; // Reset to first page when changing size
        loadTargets();
    }
};

// Initialize targets pagination on page load (for direct page access)
if (typeof document !== 'undefined') {
    document.addEventListener('DOMContentLoaded', function() {
        setTimeout(function() {
            const tbody = document.getElementById('targets-table-body');
            // Only initialize if we're on targets page AND loadTargets hasn't been called yet
            if (tbody && !targetsPagination.loading && targetsPagination.total === 0) {
                console.log('DOMContentLoaded: Initializing targets page with pagination');
                // Initialize pagination state
                targetsPagination.offset = 0;
                targetsPagination.limit = parseInt(document.getElementById('page-size-select')?.value || '25');

                // Load targets from API with pagination
                loadTargets();
            }
        }, 100);
    });
}

// ====================================
// Dashboard Online/Offline Targets
// ====================================

// Fetch and render online targets
async function updateOnlineTargets() {
    try {
        const response = await fetch('/api/dashboard/online-targets', {
            method: 'GET',
            headers: {
                'Authorization': `Bearer ${localStorage.getItem('token')}`,
                'Content-Type': 'application/json'
            }
        });

        if (!response.ok) {
            throw new Error('Online hedefler yüklenemedi');
        }

        const data = await response.json();
        renderOnlineTargets(data.targets, data.count);
    } catch (error) {
        console.error('Online hedefler güncellenemedi:', error);
    }
}

// Fetch and render offline targets
async function updateOfflineTargets() {
    try {
        const response = await fetch('/api/dashboard/offline-targets', {
            method: 'GET',
            headers: {
                'Authorization': `Bearer ${localStorage.getItem('token')}`,
                'Content-Type': 'application/json'
            }
        });

        if (!response.ok) {
            throw new Error('Offline hedefler yüklenemedi');
        }

        const data = await response.json();
        renderOfflineTargets(data.targets, data.count);
    } catch (error) {
        console.error('Offline hedefler güncellenemedi:', error);
    }
}

// Render online targets list
function renderOnlineTargets(targets, totalCount) {
    const container = document.getElementById('online-targets-list');
    if (!container) return;

    // Toplam sayıyı güncelle
    updateTargetCountDisplay('online-targets-container', totalCount || 0);

    if (!targets || targets.length === 0) {
        container.innerHTML = '<p class="text-gray-500 dark:text-gray-400 text-center py-4">Henüz online hedef bulunmuyor.</p>';
        return;
    }

    const html = targets.map(target => {
        const lastCheck = target.last_check ? formatTurkishTime(target.last_check) : 'Bilinmiyor';
        const monitoringLabel = formatMonitoringType(target.monitoring_type);

        return `
            <div class="flex items-center justify-between p-3 bg-gray-50 dark:bg-gray-700 rounded-lg hover:bg-gray-100 dark:hover:bg-gray-600 transition-colors">
                <div class="flex items-center space-x-3">
                    <span class="h-3 w-3 rounded-full bg-green-400 shadow-lg shadow-green-500/40 animate-pulse"></span>
                    <div>
                        <p class="text-sm font-medium text-gray-900 dark:text-gray-100">${escapeHtml(target.name)}</p>
                        <p class="text-xs text-gray-500 dark:text-gray-400">${escapeHtml(target.address)}</p>
                        <p class="text-xs text-gray-400 dark:text-gray-500 mt-1">${monitoringLabel}</p>
                    </div>
                </div>
                <div class="text-right">
                    <span class="px-2 py-1 text-xs font-medium rounded-full bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-300">Online</span>
                    <p class="text-xs text-gray-500 dark:text-gray-400 mt-1">${lastCheck}</p>
                </div>
            </div>
        `;
    }).join('');

    container.innerHTML = `<div class="space-y-3">${html}</div>`;
}

// Render offline targets list
function renderOfflineTargets(targets, totalCount) {
    const container = document.getElementById('offline-targets-list');
    if (!container) return;

    // Toplam sayıyı güncelle
    updateTargetCountDisplay('offline-targets-container', totalCount || 0);

    if (!targets || targets.length === 0) {
        container.innerHTML = '<p class="text-gray-500 dark:text-gray-400 text-center py-4">Henüz offline hedef bulunmuyor.</p>';
        return;
    }

    const html = targets.map(target => {
        const lastCheck = target.last_check ? formatTurkishTime(target.last_check) : 'Bilinmiyor';
        const monitoringLabel = formatMonitoringType(target.monitoring_type);

        return `
            <div class="flex items-center justify-between p-3 bg-gray-50 dark:bg-gray-700 rounded-lg hover:bg-gray-100 dark:hover:bg-gray-600 transition-colors">
                <div class="flex items-center space-x-3">
                    <span class="h-3 w-3 rounded-full bg-red-400 shadow-lg shadow-red-500/40 animate-pulse"></span>
                    <div>
                        <p class="text-sm font-medium text-gray-900 dark:text-gray-100">${escapeHtml(target.name)}</p>
                        <p class="text-xs text-gray-500 dark:text-gray-400">${escapeHtml(target.address)}</p>
                        <p class="text-xs text-gray-400 dark:text-gray-500 mt-1">${monitoringLabel}</p>
                    </div>
                </div>
                <div class="text-right">
                    <span class="px-2 py-1 text-xs font-medium rounded-full bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-300">Offline</span>
                    <p class="text-xs text-gray-500 dark:text-gray-400 mt-1">${lastCheck}</p>
                </div>
            </div>
        `;
    }).join('');

    container.innerHTML = `<div class="space-y-3">${html}</div>`;
}

// Update target count display in the header
function updateTargetCountDisplay(containerId, count) {
    const container = document.getElementById(containerId);
    if (!container) return;

    // Container'ın parent card'ını bul
    const card = container.closest('.bg-white, .dark\\:bg-gray-800');
    if (!card) return;

    // Header'daki count element'ini bul veya oluştur
    let countElement = card.querySelector('.target-count-display');
    if (!countElement) {
        // Eğer yoksa, header içinde oluştur
        const header = card.querySelector('h3');
        if (header && header.parentElement) {
            countElement = document.createElement('span');
            countElement.className = 'target-count-display text-xs text-gray-500 dark:text-gray-400 ml-2';
            header.parentElement.appendChild(countElement);
        }
    }

    if (countElement) {
        countElement.textContent = `Toplam: ${count} hedef`;
    }
}

// Format monitoring type for display
function formatMonitoringType(type) {
    const types = {
        'http': 'HTTP İzleme',
        'https': 'HTTPS İzleme',
        'ping': 'Ping İzleme',
        'icmp': 'Ping İzleme',
        'tcp': 'TCP İzleme'
    };
    return types[type?.toLowerCase()] || 'Genel İzleme';
}

// Escape HTML to prevent XSS
function escapeHtml(text) {
    const div = document.createElement('div');
    div.textContent = text;
    return div.innerHTML;
}

// Global WebSocket instance for dashboard
let dashboardWebSocket = null;

// Initialize dashboard targets on page load
document.addEventListener('DOMContentLoaded', function() {
    // Check if we're on the dashboard page
    const onlineContainer = document.getElementById('online-targets-container');
    const offlineContainer = document.getElementById('offline-targets-container');

    if (onlineContainer && offlineContainer) {
        console.log('Dashboard initialized - Loading online/offline targets');

        // Initial load
        updateOnlineTargets();
        updateOfflineTargets();

        // Polling: Update every 15 seconds
        setInterval(() => {
            updateOnlineTargets();
            updateOfflineTargets();
        }, 15000);

        // Initialize WebSocket for real-time updates
        initDashboardWebSocket();
    }
});

// Initialize WebSocket for dashboard
function initDashboardWebSocket() {
    const token = localStorage.getItem('token');
    if (!token) {
        console.warn('No auth token found, skipping WebSocket initialization');
        return;
    }

    // Construct WebSocket URL
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const wsUrl = `${protocol}//${window.location.host}/ws?token=${token}`;

    console.log('Initializing dashboard WebSocket:', wsUrl);

    dashboardWebSocket = new WSHelper(wsUrl, {
        onOpen: (event) => {
            console.log('Dashboard WebSocket connected');
        },
        onMessage: (message) => {
            console.log('[Dashboard WebSocket] Message received:', {
                type: message.type,
                targetId: message.targetId,
                data: message.data
            });

            // Handle different message types
            if (message.type === 'status_update' ||
                message.type === 'target_status_update' ||
                message.type === 'ping_result' ||
                message.type === 'dashboard_stats') {
                console.log('[Dashboard] Target/status update detected, refreshing tables...');
                // Refresh both lists when any target status changes
                updateOnlineTargets();
                updateOfflineTargets();
            } else {
                console.log('[Dashboard] Ignoring message type:', message.type);
            }
        },
        onClose: (event) => {
            console.log('Dashboard WebSocket closed:', event.code);
        },
        onError: (error) => {
            console.error('Dashboard WebSocket error:', error);
        },
        maxReconnectAttempts: 10,
        reconnectInterval: 2000
    });
}


// ==============================================
// TARGET SERVICES MANAGEMENT
// ==============================================

let currentTargetServices = {
    targetId: null,
    targetName: '',
    targetAddress: '',
    services: []
};

window.showTargetServices = async function(targetId, targetName, targetAddress) {
    currentTargetServices.targetId = targetId;
    currentTargetServices.targetName = targetName;
    currentTargetServices.targetAddress = targetAddress;

    // Also set global variable for credential management
    window.currentServiceTargetId = targetId;

    console.log('showTargetServices called with targetId:', targetId);

    // Open modal
    const modal = document.getElementById('target-services-modal');
    if (modal) {
        modal.classList.remove('hidden');
        document.body.classList.add('overflow-hidden');
    }

    // Update modal title
    const modalTitle = document.getElementById('services-modal-target-name');
    if (modalTitle) {
        modalTitle.textContent = targetName;
    }

    // Load services
    await loadTargetServices();
};

window.closeTargetServicesModal = function() {
    const modal = document.getElementById('target-services-modal');
    if (modal) {
        modal.classList.add('hidden');
        document.body.classList.remove('overflow-hidden');
    }
    currentTargetServices = { targetId: null, targetName: '', targetAddress: '', services: [] };
};

async function loadTargetServices() {
    const tbody = document.getElementById('services-table-body');
    if (!tbody) return;

    tbody.innerHTML = '<tr><td colspan="5" class="px-6 py-4 text-center text-gray-500">Yükleniyor...</td></tr>';

    try {
        const token = localStorage.getItem('token');
        const response = await fetch(`/api/targets/${currentTargetServices.targetId}/services`, {
            headers: { 'Authorization': 'Bearer ' + token }
        });

        if (!response.ok) {
            const errorText = await response.text();
            console.error('API Error:', response.status, errorText);
            throw new Error(`HTTP ${response.status}: ${errorText}`);
        }

        const services = await response.json();
        currentTargetServices.services = services || [];

        renderTargetServicesTable();
    } catch (error) {
        console.error('Error loading target services:', error);
        tbody.innerHTML = `<tr><td colspan="5" class="px-6 py-4 text-center text-red-500">Hata: ${error.message}<br><small>Console'u kontrol edin (F12)</small></td></tr>`;
    }
}

function renderTargetServicesTable() {
    const tbody = document.getElementById('services-table-body');
    if (!tbody) return;

    if (!currentTargetServices.services || currentTargetServices.services.length === 0) {
        tbody.innerHTML = '<tr><td colspan="5" class="px-6 py-4 text-center text-gray-500 dark:text-gray-400">Henüz servis eklenmemiş. "Servis Ekle" butonunu kullanarak servis ekleyebilirsiniz.</td></tr>';
        return;
    }

    tbody.innerHTML = currentTargetServices.services.map(service => {
        const statusClass = service.status === 'online'
            ? 'bg-green-100 dark:bg-green-900/20 text-green-800 dark:text-green-400'
            : service.status === 'offline'
            ? 'bg-red-100 dark:bg-red-900/20 text-red-800 dark:text-red-400'
            : 'bg-gray-100 dark:bg-gray-900/20 text-gray-800 dark:text-gray-400';

        const statusText = service.status === 'online' ? '🟢 Online' : service.status === 'offline' ? '🔴 Offline' : '⚪ Unknown';

        const lastChecked = service.last_checked
            ? new Date(service.last_checked).toLocaleString('tr-TR')
            : 'Henüz kontrol edilmedi';

        return `
            <tr class="hover:bg-gray-50 dark:hover:bg-gray-700/50">
                <td class="px-6 py-4 text-sm font-medium text-gray-900 dark:text-gray-100">${escapeHtml(service.service_name)}</td>
                <td class="px-6 py-4 text-sm text-gray-500 dark:text-gray-400">${service.port}</td>
                <td class="px-6 py-4 text-sm text-gray-500 dark:text-gray-400">${service.protocol.toUpperCase()}</td>
                <td class="px-6 py-4">
                    <span class="px-2 py-1 text-xs font-medium rounded-full ${statusClass}">${statusText}</span>
                    <div class="text-xs text-gray-500 dark:text-gray-400 mt-1">${lastChecked}</div>
                </td>
                <td class="px-6 py-4 text-sm text-gray-500 dark:text-gray-400">
                    <button onclick="deleteTargetService(${service.id})" class="text-red-600 hover:text-red-900 dark:text-red-400 dark:hover:text-red-300">
                        <i class="fas fa-trash"></i> Sil
                    </button>
                </td>
            </tr>
        `;
    }).join('');
}

window.showAddServiceForm = function() {
    const form = document.getElementById('add-service-form');
    if (form) {
        form.classList.remove('hidden');
    }
};

window.hideAddServiceForm = function() {
    const form = document.getElementById('add-service-form');
    if (form) {
        form.classList.add('hidden');
    }
    // Reset form
    document.getElementById('service-name').value = '';
    document.getElementById('service-port').value = '';
    document.getElementById('service-protocol').value = 'tcp';
};

window.submitAddService = async function() {
    const serviceName = document.getElementById('service-name').value.trim();
    const port = parseInt(document.getElementById('service-port').value);
    const protocol = document.getElementById('service-protocol').value;

    if (!serviceName || !port || port < 1 || port > 65535) {
        alert('Lütfen geçerli bir servis adı ve port numarası girin (1-65535)');
        return;
    }

    try {
        const token = localStorage.getItem('token');
        const response = await fetch(`/api/targets/${currentTargetServices.targetId}/services`, {
            method: 'POST',
            headers: {
                'Authorization': 'Bearer ' + token,
                'Content-Type': 'application/json'
            },
            body: JSON.stringify({
                service_name: serviceName,
                port: port,
                protocol: protocol
            })
        });

        if (!response.ok) {
            const error = await response.json();
            throw new Error(error.error || 'Servis eklenemedi');
        }

        // Success
        hideAddServiceForm();
        await loadTargetServices();

        if (window.app && window.app.showToast) {
            window.app.showToast('success', 'Servis başarıyla eklendi');
        }
    } catch (error) {
        console.error('Error adding service:', error);
        alert('Hata: ' + error.message);
    }
};

window.deleteTargetService = async function(serviceId) {
    if (!confirm('Bu servisi silmek istediğinizden emin misiniz?')) {
        return;
    }

    try {
        const token = localStorage.getItem('token');
        const response = await fetch(`/api/targets/${currentTargetServices.targetId}/services/${serviceId}`, {
            method: 'DELETE',
            headers: { 'Authorization': 'Bearer ' + token }
        });

        if (!response.ok) {
            throw new Error('Servis silinemedi');
        }

        await loadTargetServices();

        if (window.app && window.app.showToast) {
            window.app.showToast('success', 'Servis silindi');
        }
    } catch (error) {
        console.error('Error deleting service:', error);
        alert('Hata: ' + error.message);
    }
};

// Discover services automatically
window.discoverServices = async function() {
    if (!currentTargetServices.targetId) {
        alert('Hedef seçilmedi');
        return;
    }

    const tbody = document.getElementById('services-table-body');
    if (!tbody) return;

    tbody.innerHTML = '<tr><td colspan="5" class="px-6 py-4 text-center text-blue-500"><i class="fas fa-spinner fa-spin mr-2"></i>Servisler taranıyor... Bu işlem 30 saniye sürebilir.</td></tr>';

    try {
        const token = localStorage.getItem('token');
        const response = await fetch(`/api/targets/${currentTargetServices.targetId}/services/discover`, {
            method: 'POST',
            headers: { 'Authorization': 'Bearer ' + token }
        });

        if (!response.ok) {
            const errorData = await response.json().catch(() => ({}));
            throw new Error(errorData.error || 'Servis keşfi başarısız');
        }

        const discoveredServices = await response.json();

        if (!discoveredServices || discoveredServices.length === 0) {
            tbody.innerHTML = '<tr><td colspan="5" class="px-6 py-4 text-center text-gray-500">Açık port bulunamadı. Hedef cihaz çevrimiçi değil veya güvenlik duvarı etkin olabilir.</td></tr>';
            return;
        }

        // Show discovered services with "Add" button
        showDiscoveredServices(discoveredServices);

    } catch (error) {
        console.error('Error discovering services:', error);
        tbody.innerHTML = `<tr><td colspan="5" class="px-6 py-4 text-center text-red-500">Hata: ${error.message}</td></tr>`;
    }
};

function showDiscoveredServices(discoveredServices) {
    const tbody = document.getElementById('services-table-body');
    if (!tbody) return;

    tbody.innerHTML = `
        <tr>
            <td colspan="5" class="px-6 py-4 bg-blue-50 dark:bg-blue-900/20">
                <div class="flex items-center justify-between">
                    <div>
                        <h4 class="font-bold text-blue-900 dark:text-blue-100">
                            🎉 ${discoveredServices.length} adet açık port bulundu!
                        </h4>
                        <p class="text-sm text-blue-700 dark:text-blue-300 mt-1">
                            İzlemek istediğiniz servisleri seçin ve ekleyin.
                        </p>
                    </div>
                    <div class="flex gap-2">
                        <button onclick="addAllDiscoveredServices()" class="px-4 py-2 bg-green-600 hover:bg-green-700 text-white rounded-lg text-sm">
                            <i class="fas fa-check-double mr-1"></i> Hepsini Ekle
                        </button>
                        <button onclick="loadTargetServices()" class="px-4 py-2 bg-gray-500 hover:bg-gray-600 text-white rounded-lg text-sm">
                            İptal
                        </button>
                    </div>
                </div>
            </td>
        </tr>
    ` + discoveredServices.map(service => `
        <tr class="hover:bg-gray-50 dark:hover:bg-gray-700/50">
            <td class="px-6 py-4 text-sm font-medium text-gray-900 dark:text-gray-100">${escapeHtml(service.service_name)}</td>
            <td class="px-6 py-4 text-sm text-gray-500 dark:text-gray-400">${service.port}</td>
            <td class="px-6 py-4 text-sm text-gray-500 dark:text-gray-400">${service.protocol.toUpperCase()}</td>
            <td class="px-6 py-4">
                <span class="px-2 py-1 text-xs font-medium rounded-full bg-green-100 dark:bg-green-900/20 text-green-800 dark:text-green-400">
                    🟢 Açık (${service.method})
                </span>
            </td>
            <td class="px-6 py-4 text-sm text-gray-500 dark:text-gray-400">
                <button onclick='addDiscoveredService(${JSON.stringify(service)})' class="px-3 py-1.5 bg-purple-600 hover:bg-purple-700 text-white rounded-lg text-sm">
                    <i class="fas fa-plus mr-1"></i> İzlemeye Ekle
                </button>
            </td>
        </tr>
    `).join('');

    // Store discovered services globally
    window.currentDiscoveredServices = discoveredServices;
}

window.addDiscoveredService = async function(service) {
    try {
        const token = localStorage.getItem('token');
        const response = await fetch(`/api/targets/${currentTargetServices.targetId}/services`, {
            method: 'POST',
            headers: {
                'Authorization': 'Bearer ' + token,
                'Content-Type': 'application/json'
            },
            body: JSON.stringify({
                service_name: service.service_name,
                port: service.port,
                protocol: service.protocol
            })
        });

        if (!response.ok) {
            const error = await response.json();
            if (error.error === 'service_already_exists') {
                if (window.app && window.app.showToast) {
                    window.app.showToast('warning', `${service.service_name} zaten izleniyor`);
                }
                return;
            }
            throw new Error(error.error || 'Servis eklenemedi');
        }

        if (window.app && window.app.showToast) {
            window.app.showToast('success', `${service.service_name} izlemeye eklendi`);
        }

        // Remove from discovered list
        if (window.currentDiscoveredServices) {
            window.currentDiscoveredServices = window.currentDiscoveredServices.filter(
                s => !(s.port === service.port && s.protocol === service.protocol)
            );

            if (window.currentDiscoveredServices.length === 0) {
                // All added, show the services list
                await loadTargetServices();
            } else {
                // Update the view
                showDiscoveredServices(window.currentDiscoveredServices);
            }
        }

    } catch (error) {
        console.error('Error adding discovered service:', error);
        alert('Hata: ' + error.message);
    }
};

window.addAllDiscoveredServices = async function() {
    if (!window.currentDiscoveredServices || window.currentDiscoveredServices.length === 0) {
        return;
    }

    const services = [...window.currentDiscoveredServices];
    let added = 0;
    let skipped = 0;

    for (const service of services) {
        try {
            const token = localStorage.getItem('token');
            const response = await fetch(`/api/targets/${currentTargetServices.targetId}/services`, {
                method: 'POST',
                headers: {
                    'Authorization': 'Bearer ' + token,
                    'Content-Type': 'application/json'
                },
                body: JSON.stringify({
                    service_name: service.service_name,
                    port: service.port,
                    protocol: service.protocol
                })
            });

            if (response.ok) {
                added++;
            } else {
                const error = await response.json();
                if (error.error === 'service_already_exists') {
                    skipped++;
                }
            }
        } catch (error) {
            console.error('Error adding service:', error);
        }
    }

    if (window.app && window.app.showToast) {
        window.app.showToast('success', `${added} servis eklendi${skipped > 0 ? `, ${skipped} zaten mevcuttu` : ''}`);
    }

    // Reload services list
    await loadTargetServices();
};

// ============================================================================
// CREDENTIAL MANAGEMENT
// ============================================================================

// Show credentials modal
window.showCredentialsModal = function() {
    const modal = document.getElementById('credentials-modal');
    const targetName = document.getElementById('services-modal-target-name').textContent;
    document.getElementById('credentials-modal-target-name').textContent = targetName;

    // Load current credentials from target
    loadCurrentCredentials();

    modal.classList.remove('hidden');
};

// Close credentials modal
window.closeCredentialsModal = function() {
    const modal = document.getElementById('credentials-modal');
    modal.classList.add('hidden');

    // Clear form
    document.getElementById('cred-os-type').value = 'unknown';
    document.getElementById('cred-auth-type').value = 'none';
    document.getElementById('cred-username').value = '';
    document.getElementById('cred-password').value = '';
    document.getElementById('cred-domain').value = '';
    document.getElementById('test-result').textContent = '';
    document.getElementById('credential-fields').classList.add('hidden');
};

// Toggle credential fields visibility
window.toggleCredentialFields = function() {
    const authType = document.getElementById('cred-auth-type').value;
    const osType = document.getElementById('cred-os-type').value;
    const fieldsDiv = document.getElementById('credential-fields');
    const domainField = document.getElementById('domain-field');

    if (authType === 'password') {
        fieldsDiv.classList.remove('hidden');

        // Show domain field for Windows
        if (osType === 'windows') {
            domainField.classList.remove('hidden');
        } else {
            domainField.classList.add('hidden');
        }
    } else {
        fieldsDiv.classList.add('hidden');
    }
};

// Load current credentials for target
async function loadCurrentCredentials() {
    if (!window.currentServiceTargetId) return;

    try {
        const token = localStorage.getItem('token');
        const response = await fetch(`/api/targets/${window.currentServiceTargetId}`, {
            headers: {
                'Authorization': 'Bearer ' + (token || '')
            }
        });
        if (!response.ok) throw new Error('Failed to load target');

        const target = await response.json();

        // Set form values
        document.getElementById('cred-os-type').value = target.os_type || 'unknown';
        document.getElementById('cred-auth-type').value = target.auth_type || 'none';
        document.getElementById('cred-username').value = target.username || '';
        document.getElementById('cred-password').value = ''; // Don't show existing password
        document.getElementById('cred-domain').value = target.domain || '';

        // Trigger visibility toggle
        toggleCredentialFields();
    } catch (error) {
        console.error('Error loading credentials:', error);
    }
}

// Save credentials
window.saveCredentials = async function() {
    console.log('saveCredentials called, targetId:', window.currentServiceTargetId);

    if (!window.currentServiceTargetId) {
        console.error('No currentServiceTargetId set');
        if (window.app && window.app.showToast) {
            window.app.showToast('error', 'Hedef ID bulunamadı');
        }
        return;
    }

    const osType = document.getElementById('cred-os-type').value;
    const authType = document.getElementById('cred-auth-type').value;
    const username = document.getElementById('cred-username').value;
    const password = document.getElementById('cred-password').value;
    const domain = document.getElementById('cred-domain').value;

    console.log('Credentials:', { osType, authType, username: username ? '***' : '', domain });

    // Validation
    if (authType === 'password' && (!username || !password)) {
        if (window.app && window.app.showToast) {
            window.app.showToast('error', 'Kullanıcı adı ve şifre gerekli');
        }
        return;
    }

    try {
        const url = `/api/targets/${window.currentServiceTargetId}/credentials`;
        console.log('Sending PUT request to:', url);

        const token = localStorage.getItem('token');
        const response = await fetch(url, {
            method: 'PUT',
            headers: {
                'Content-Type': 'application/json',
                'Authorization': 'Bearer ' + (token || '')
            },
            body: JSON.stringify({
                os_type: osType,
                auth_type: authType,
                username: username,
                password: password,
                domain: domain
            })
        });

        console.log('Response status:', response.status);

        if (!response.ok) {
            const error = await response.json();
            console.error('API error:', error);
            throw new Error(error.error || 'Failed to save credentials');
        }

        if (window.app && window.app.showToast) {
            window.app.showToast('success', 'Kimlik bilgileri kaydedildi');
        }

        closeCredentialsModal();
    } catch (error) {
        console.error('Error saving credentials:', error);
        if (window.app && window.app.showToast) {
            window.app.showToast('error', `Hata: ${error.message}`);
        }
    }
};

// Test credentials
window.testCredentials = async function() {
    console.log('testCredentials called, targetId:', window.currentServiceTargetId);

    if (!window.currentServiceTargetId) {
        console.error('No currentServiceTargetId set');
        alert('Hedef ID bulunamadı. Lütfen modal\'ı kapatıp tekrar açın.');
        return;
    }

    const resultSpan = document.getElementById('test-result');
    resultSpan.textContent = 'Test ediliyor...';
    resultSpan.className = 'text-sm py-2 text-blue-600';

    try {
        const url = `/api/targets/${window.currentServiceTargetId}/credentials/test`;
        console.log('Sending POST request to:', url);

        const token = localStorage.getItem('token');
        const response = await fetch(url, {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json',
                'Authorization': 'Bearer ' + (token || '')
            }
        });

        console.log('Response status:', response.status);
        const result = await response.json();
        console.log('Test result:', result);

        if (result.success) {
            resultSpan.textContent = '✓ Bağlantı başarılı';
            resultSpan.className = 'text-sm py-2 text-green-600 dark:text-green-400';
        } else {
            resultSpan.textContent = '✗ Bağlantı başarısız: ' + (result.error || 'Bilinmeyen hata');
            resultSpan.className = 'text-sm py-2 text-red-600 dark:text-red-400';
        }
    } catch (error) {
        console.error('Error testing credentials:', error);
        resultSpan.textContent = '✗ Test hatası: ' + error.message;
        resultSpan.className = 'text-sm py-2 text-red-600 dark:text-red-400';
    }
};

// Update discoverServices to show better messages for WMI/SSH
const originalDiscoverServices = window.discoverServices;
window.discoverServices = async function() {
    const button = event.target.closest('button');
    const originalText = button.innerHTML;
    button.innerHTML = '<i class="fas fa-spinner fa-spin mr-1"></i> Taranıyor...';
    button.disabled = true;

    try {
        const token = localStorage.getItem('token');
        const response = await fetch(`/api/targets/${window.currentServiceTargetId}/services/discover`, {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json',
                'Authorization': 'Bearer ' + (token || '')
            }
        });

        if (!response.ok) {
            throw new Error('Keşif başarısız');
        }

        const services = await response.json();

        if (!services || services.length === 0) {
            if (window.app && window.app.showToast) {
                window.app.showToast('warning', 'Açık port bulunamadı. Gelişmiş tarama için "Kimlik Bilgileri" ile WMI/SSH yapılandırın.');
            }
        } else {
            // Show discovered services modal
            showDiscoveredServices(services);
        }
    } catch (error) {
        console.error('Error discovering services:', error);
        if (window.app && window.app.showToast) {
            window.app.showToast('error', `Keşif hatası: ${error.message}`);
        }
    } finally {
        button.innerHTML = originalText;
        button.disabled = false;
    }
};

// Add listener for OS type change to toggle domain field
document.addEventListener('DOMContentLoaded', function() {
    const osTypeSelect = document.getElementById('cred-os-type');
    if (osTypeSelect) {
        osTypeSelect.addEventListener('change', toggleCredentialFields);
    }

    // Check for update notifications
    checkUpdateNotification();
});

// Update notification system
async function checkUpdateNotification() {
    try {
        const token = localStorage.getItem('token');
        if (!token) return;

        const response = await fetch('/api/license/update-notification', {
            headers: {
                'Authorization': `Bearer ${token}`
            }
        });

        if (!response.ok) return;

        const data = await response.json();

        if (data.show_notification && data.update_version) {
            showUpdateNotificationModal(data.update_version);
        }
    } catch (error) {
        console.error('Update notification check failed:', error);
    }
}

function showUpdateNotificationModal(version) {
    // Create modal HTML
    const modalHTML = `
        <div id="update-notification-modal" class="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50" style="display: flex;">
            <div class="bg-white dark:bg-gray-800 rounded-lg shadow-xl max-w-md w-full mx-4 p-6">
                <div class="flex items-center mb-4">
                    <div class="flex-shrink-0">
                        <svg class="h-12 w-12 text-green-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z"></path>
                        </svg>
                    </div>
                    <div class="ml-4">
                        <h3 class="text-lg font-semibold text-gray-900 dark:text-gray-100">
                            Güncelleme Tamamlandı
                        </h3>
                        <p class="text-sm text-gray-600 dark:text-gray-400 mt-1">
                            Sistem başarıyla <span class="font-mono font-semibold text-primary-600">${version}</span> sürümüne güncellendi.
                        </p>
                    </div>
                </div>
                <div class="mt-6 flex justify-end">
                    <button onclick="dismissUpdateNotification()" class="px-4 py-2 bg-primary-600 text-white rounded-lg hover:bg-primary-700 transition-colors">
                        Tamam
                    </button>
                </div>
            </div>
        </div>
    `;

    // Add modal to page
    document.body.insertAdjacentHTML('beforeend', modalHTML);
}

async function dismissUpdateNotification() {
    try {
        const token = localStorage.getItem('token');
        if (!token) return;

        await fetch('/api/license/update-notification/dismiss', {
            method: 'POST',
            headers: {
                'Authorization': `Bearer ${token}`,
                'Content-Type': 'application/json'
            }
        });

        // Remove modal
        const modal = document.getElementById('update-notification-modal');
        if (modal) {
            modal.remove();
        }
    } catch (error) {
        console.error('Failed to dismiss notification:', error);
        // Still remove modal even if request fails
        const modal = document.getElementById('update-notification-modal');
        if (modal) {
            modal.remove();
        }
    }
}


