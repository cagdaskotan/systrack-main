/**
 * Device Services Management
 * Cihaz içi servis izleme (WinRM/SSH)
 */

if (!window.showToast) {
    window.showToast = function(message, type = 'info') {
        if (window.app && typeof window.app.showToast === 'function') {
            window.app.showToast(type, message);
            return;
        }
        if (window.Toast && typeof window.Toast.show === 'function') {
            window.Toast.show({ type: type, title: '', message: message, timeout: 3000 });
            return;
        }
        console.log('[' + type + '] ' + message);
    };
}

let currentTargetId = null;
let currentTargetName = '';

// Modal'ı aç
window.showDeviceServicesModal = function(targetId, targetName) {
    currentTargetId = targetId;
    currentTargetName = targetName;

    const modal = document.getElementById('device-services-modal');
    const modalTargetName = document.getElementById('device-services-target-name');

    if (modalTargetName) {
        modalTargetName.textContent = targetName;
    }

    if (modal) {
        modal.classList.remove('hidden');

        // Reset filter tabs to "Tümü"
        document.querySelectorAll('.service-filter-tab').forEach(tab => {
            tab.classList.remove('active');
        });
        const allTab = document.querySelector('.service-filter-tab[data-filter="all"]');
        if (allTab) {
            allTab.classList.add('active');
        }

        // Credential ve servisleri yükle
        loadTargetCredentials(targetId);
        loadDeviceServices(targetId, 'all');
    }
};

// Modal'ı kapat
window.closeDeviceServicesModal = function() {
    const modal = document.getElementById('device-services-modal');
    if (modal) {
        modal.classList.add('hidden');
    }
    currentTargetId = null;
    currentTargetName = '';
};

// Credential form'unu göster/gizle
window.toggleCredentialForm = function(show) {
    const form = document.getElementById('credential-form-section');
    const infoSection = document.getElementById('credential-info-section');

    if (form && infoSection) {
        if (show) {
            form.classList.remove('hidden');
            infoSection.classList.add('hidden');
        } else {
            form.classList.add('hidden');
            infoSection.classList.remove('hidden');
        }
    }
};

// Credential bilgilerini yükle
async function loadTargetCredentials(targetId) {
    console.log('Loading credentials for target:', targetId);

    try {
        const response = await fetch(`/api/targets/${targetId}/credentials`, {
            headers: {
                'Authorization': `Bearer ${localStorage.getItem('token')}`
            }
        });

        console.log('Credentials API response status:', response.status);

        const credInfoSection = document.getElementById('credential-info-section');
        const credFormSection = document.getElementById('credential-form-section');
        const noCredSection = document.getElementById('no-credential-section');
        const servicesSection = document.getElementById('services-section');

        if (response.status === 404) {
            // Credential yok
            console.log('No credentials found for target', targetId);
            if (noCredSection) noCredSection.classList.remove('hidden');
            if (credInfoSection) credInfoSection.classList.add('hidden');
            if (credFormSection) credFormSection.classList.add('hidden');
            if (servicesSection) servicesSection.classList.add('hidden');
            return;
        }

        if (!response.ok) {
            const errorText = await response.text();
            console.error('API Error:', errorText);
            throw new Error('Failed to load credentials');
        }

        const data = await response.json();

        if (data && data.exists === false) {
            console.log('No credentials found for target', targetId);
            if (noCredSection) noCredSection.classList.remove('hidden');
            if (credInfoSection) credInfoSection.classList.add('hidden');
            if (credFormSection) credFormSection.classList.add('hidden');
            if (servicesSection) servicesSection.classList.add('hidden');
            return;
        }

        // Credential var, bilgileri göster
        if (noCredSection) noCredSection.classList.add('hidden');
        if (credInfoSection) credInfoSection.classList.remove('hidden');
        if (servicesSection) servicesSection.classList.remove('hidden');

        // Bilgileri doldur
        const osTypeEl = document.getElementById('device-cred-os-type');
        const protocolEl = document.getElementById('device-cred-protocol');
        const usernameEl = document.getElementById('device-cred-username');
        const lastTestEl = document.getElementById('device-cred-last-test');

        if (osTypeEl) osTypeEl.textContent = data.os_type === 'windows' ? 'Windows' : 'Linux';
        if (protocolEl) protocolEl.textContent = data.protocol === 'winrm' ? 'WinRM' : 'SSH';
        if (usernameEl) usernameEl.textContent = data.username;

        if (lastTestEl) {
            if (data.last_test_at) {
                const testDate = new Date(data.last_test_at);
                const testStatus = data.last_test_success ?
                    '<span class="text-green-400">✓ Successful</span>' :
                    '<span class="text-red-400">✗ Failed</span>';
                lastTestEl.innerHTML = `${testDate.toLocaleString('en-US')} - ${testStatus}`;
            } else {
                lastTestEl.textContent = 'Not tested yet';
            }
        }

        console.log('Credentials loaded:', data);

    } catch (error) {
        console.error('Error loading credentials:', error);
        window.showToast('Credentials could not be loaded', 'error');
    }
}

// Credential kaydet
window.saveDeviceCredentials = async function() {
    const osType = document.getElementById('cred-os-type-input').value;
    const username = document.getElementById('cred-username-input').value;
    const password = document.getElementById('cred-password-input').value;
    const domain = document.getElementById('cred-domain-input').value;
    const port = document.getElementById('cred-port-input').value;

    if (!username || !password) {
        window.showToast('Username and password are required', 'error');
        return;
    }

    try {
        const payload = {
            os_type: osType,
            username: username,
            password: password
        };

        if (domain) payload.domain = domain;
        if (port) payload.port = parseInt(port);

        const response = await fetch(`/api/targets/${currentTargetId}/credentials`, {
            method: 'POST',
            headers: {
                'Authorization': `Bearer ${localStorage.getItem('token')}`,
                'Content-Type': 'application/json'
            },
            body: JSON.stringify(payload)
        });

        if (!response.ok) {
            const errorData = await response.json();
            throw new Error(errorData.error || 'Failed to save credentials');
        }

        window.showToast('Credentials saved', 'success');
        toggleCredentialForm(false);
        loadTargetCredentials(currentTargetId);

        // Form'u temizle
        document.getElementById('cred-password-input').value = '';

    } catch (error) {
        console.error('Error saving credentials:', error);
        window.showToast('Credentials could not be saved: ' + error.message, 'error');
    }
};

// Bağlantı testi
window.testDeviceConnection = async function() {
    const testButton = document.getElementById('test-connection-btn');
    const testResult = document.getElementById('test-connection-result');

    if (testButton) testButton.disabled = true;
    if (testResult) testResult.textContent = 'Testing...';

    try {
        const response = await fetch(`/api/targets/${currentTargetId}/credentials/test`, {
            method: 'POST',
            headers: {
                'Authorization': `Bearer ${localStorage.getItem('token')}`
            }
        });

        const data = await response.json();

        if (data.success) {
            if (testResult) {
                testResult.innerHTML = '<span class="text-green-600">✓ Connection successful</span>';
            }
            window.showToast('Connection successful', 'success');
        } else {
            // Translate error codes to user-friendly Turkish messages
            const errorMessages = {
                'authentication_failed': 'Authentication failed. Check user permissions.',
                'invalid_credentials': 'Invalid username or password.',
                'winrm_not_configured': 'WinRM service is not configured or not running.',
                'host_unreachable': 'Target server unreachable. Check network connection.',
                'rpc_unavailable': 'RPC service unavailable. Check Windows firewall settings.',
                'connection_failed': 'Connection failed. Is the server reachable and are services running?',
                'trust_configuration_required': 'TrustedHosts configuration required. Add server to the trusted list.',
                'wmi_service_error': 'WMI service error. Is the service running and are permissions correct?',
                'connection_timeout': 'Connection timed out. Network latency or firewall may be blocking.',
                'network_error': 'Network error. Is the target IP/hostname correct?'
            };

            const friendlyError = errorMessages[data.error] || `Connection error: ${data.error}`;

            if (testResult) {
                testResult.innerHTML = `<span class="text-red-600">✗ ${friendlyError}</span>`;
            }
            window.showToast(friendlyError, 'error');
        }

    } catch (error) {
        console.error('Error testing connection:', error);
        if (testResult) {
            testResult.innerHTML = '<span class="text-red-600">✗ Test failed</span>';
        }
        window.showToast('Connection test failed', 'error');
    } finally {
        if (testButton) testButton.disabled = false;
    }
};

// Servisleri yükle
async function loadDeviceServices(targetId, filter = 'all') {
    const tbody = document.getElementById('device-services-tbody');
    const emptyState = document.getElementById('services-empty-state');
    const loadingState = document.getElementById('services-loading-state');

    if (!tbody) return;

    // Loading göster
    tbody.innerHTML = '';
    if (emptyState) emptyState.classList.add('hidden');
    if (loadingState) loadingState.classList.remove('hidden');

    try {
        let url = `/api/targets/${targetId}/device-services`;

        // Filter parametrelerini ayarla
        const params = new URLSearchParams();
        if (filter === 'running' || filter === 'stopped') {
            params.append('status', filter);
        } else if (filter === 'monitored') {
            params.append('monitored', 'true');
        }

        if (params.toString()) {
            url += `?${params.toString()}`;
        }

        const response = await fetch(url, {
            headers: {
                'Authorization': `Bearer ${localStorage.getItem('token')}`
            }
        });

        if (loadingState) loadingState.classList.add('hidden');

        if (!response.ok) {
            throw new Error('Failed to load services');
        }

        const data = await response.json();
        const services = data.services || [];
        window.currentDeviceServices = services;

        if (services.length === 0) {
            if (emptyState) emptyState.classList.remove('hidden');
            updateServiceBulkToolbar();
            return;
        }

        // Servisleri render et
        tbody.innerHTML = services.map(service => {
            const statusClass = service.status === 'running' ?
                'bg-green-100 dark:bg-green-900/20 text-green-800 dark:text-green-400' :
                service.status === 'stopped' ?
                'bg-red-100 dark:bg-red-900/20 text-red-800 dark:text-red-400' :
                'bg-gray-100 dark:bg-gray-700 text-gray-600 dark:text-gray-400';

            const statusText = service.status === 'running' ? 'Running' :
                              service.status === 'stopped' ? 'Stopped' : 'Unknown';

            const lastChecked = service.last_checked ?
                new Date(service.last_checked).toLocaleString('en-US') :
                'Not checked yet';

            const monitorToggleLabel = getMonitorToggleLabel(service.is_monitored);

            return `
                <tr class="hover:bg-gray-50 dark:hover:bg-gray-700/50" data-service-id="${service.id}">
                    <td class="px-4 py-3 text-center">
                        <input type="checkbox"
                               class="service-select-checkbox h-4 w-4 rounded border-gray-600 bg-gray-700 text-purple-500 focus:ring-purple-500"
                               value="${service.id}"
                               onchange="toggleServiceSelection()">
                    </td>
                    <td class="px-4 py-3 text-sm text-gray-900 dark:text-gray-100">
                        <span>${escapeHtml(service.service_name)}</span>
                        ${service.display_name ? `<div class="text-xs text-gray-500 dark:text-gray-400">${escapeHtml(service.display_name)}</div>` : ''}
                    </td>
                    <td class="px-4 py-3 overflow-visible">
                        <span class="px-2 py-1 text-xs font-medium rounded-full whitespace-nowrap ${statusClass}">
                            ${statusText}
                        </span>
                    </td>
                    <td class="px-4 py-3 text-sm text-gray-600 dark:text-gray-400">
                        ${service.startup_type || '-'}
                    </td>
                    <td class="px-4 py-3 text-xs text-gray-500 dark:text-gray-400">
                        ${lastChecked}
                    </td>
                    <td class="px-4 py-3 text-center">
                        <button
                            onclick="toggleServiceMonitoring(${service.id})"
                            class="service-toggle relative inline-flex h-7 w-12 items-center rounded-full border border-white/10 shadow-inner transition-all duration-300 ease-out focus:outline-none focus:ring-2 focus:ring-purple-500/40 ${service.is_monitored ? 'bg-gradient-to-r from-purple-500 to-indigo-500 shadow-purple-500/40' : 'bg-gray-700/50'}"
                            title="${monitorToggleLabel}"
                            aria-label="${monitorToggleLabel}"
                            data-role="monitor-toggle"
                            data-service-id="${service.id}"
                            data-monitored="${service.is_monitored}"
                        >
                            <span class="sr-only">${monitorToggleLabel}</span>
                            <span class="service-toggle-knob inline-flex h-5 w-5 transform items-center justify-center rounded-full bg-white shadow-md ring-1 ring-black/5 transition-transform duration-300 ${service.is_monitored ? 'translate-x-[22px]' : 'translate-x-[2px]'}">
                                <i class="service-toggle-icon-on fas fa-check text-[10px] text-emerald-500 ${service.is_monitored ? '' : 'hidden'}"></i>
                                <i class="service-toggle-icon-off fas fa-minus text-[10px] text-gray-400 ${service.is_monitored ? 'hidden' : ''}"></i>
                            </span>
                        </button>
                    </td>
                </tr>
            `;
        }).join('');

        updateServiceBulkToolbar();

    } catch (error) {
        console.error('Error loading services:', error);
        if (loadingState) loadingState.classList.add('hidden');
        if (emptyState) {
            emptyState.classList.remove('hidden');
            emptyState.innerHTML = '<p class="text-red-600">Services could not be loaded</p>';
        }
    }
}

// Manuel tarama yap
window.scanDeviceServicesNow = async function() {
    const scanButton = document.getElementById('scan-services-btn');
    if (scanButton) {
        scanButton.disabled = true;
        scanButton.innerHTML = '<i class="fas fa-spinner fa-spin mr-1"></i> Taranıyor...';
    }

    try {
        const response = await fetch(`/api/targets/${currentTargetId}/device-services/scan`, {
            method: 'POST',
            headers: {
                'Authorization': `Bearer ${localStorage.getItem('token')}`
            }
        });

        if (!response.ok) {
            const errorData = await response.json();
            throw new Error(errorData.message || 'Scan failed');
        }

        window.showToast('Services scanned', 'success');

        // Aktif filtreyi al ve o filtre ile yeniden yükle
        const activeTab = document.querySelector('.service-filter-tab.active');
        const currentFilter = activeTab ? activeTab.getAttribute('data-filter') : 'all';
        loadDeviceServices(currentTargetId, currentFilter);

    } catch (error) {
        console.error('Error scanning services:', error);
        window.showToast('Scan failed: ' + error.message, 'error');
    } finally {
        if (scanButton) {
            scanButton.disabled = false;
            scanButton.innerHTML = '<i class="fas fa-sync mr-1"></i> Şimdi Tara';
        }
    }
};

// Servis monitör durumunu değiştir
window.toggleServiceMonitoring = async function(serviceId, isMonitored) {
    const toggleButton = document.querySelector(`button[data-role="monitor-toggle"][data-service-id="${serviceId}"]`);
    const previousState = toggleButton ? toggleButton.getAttribute('data-monitored') === 'true' : false;

    if (typeof isMonitored === 'undefined') {
        isMonitored = !previousState;
    }

    const options = arguments.length > 2 ? arguments[2] : {};

    if (toggleButton) {
        toggleButton.disabled = true;
    }

    setServiceToggleState(serviceId, isMonitored);

    try {
        const response = await fetch(`/api/targets/${currentTargetId}/device-services/${serviceId}`, {
            method: 'PATCH',
            headers: {
                'Authorization': `Bearer ${localStorage.getItem('token')}`,
                'Content-Type': 'application/json'
            },
            body: JSON.stringify({ is_monitored: isMonitored })
        });

        if (!response.ok) {
            throw new Error('Failed to update monitoring status');
        }

        if (!options.silent) {
            window.showToast(isMonitored ? 'Service added to monitoring' : 'Service removed from monitoring', 'success');
        }

        updateServiceMonitoredState(serviceId, isMonitored);

        return true;

    } catch (error) {
        console.error('Error updating monitoring status:', error);
        if (!options.silent) {
            window.showToast('Update failed', 'error');
        }
        setServiceToggleState(serviceId, previousState);
        return false;
    } finally {
        if (toggleButton) {
            toggleButton.disabled = false;
        }
    }
};

// Filtre değiştiğinde
window.filterDeviceServices = function(filter) {
    if (currentTargetId) {
        // Tüm tab butonlarından active sınıfını kaldır
        document.querySelectorAll('.service-filter-tab').forEach(tab => {
            tab.classList.remove('active');
        });

        // Tıklanan butona active sınıfı ekle
        const clickedTab = document.querySelector(`.service-filter-tab[data-filter="${filter}"]`);
        if (clickedTab) {
            clickedTab.classList.add('active');
        }

        // Servisleri filtrele
        loadDeviceServices(currentTargetId, filter);
    }
};

// Servis menüsünü aç/kapat
window.toggleServiceMenu = function(event, serviceId) {
    event.stopPropagation();

    // Tüm açık menüleri kapat
    document.querySelectorAll('[id^="service-menu-"]').forEach(menu => {
        if (menu.id !== `service-menu-${serviceId}`) {
            menu.classList.add('hidden');
        }
    });

    // Bu menüyü aç/kapat
    const menu = document.getElementById(`service-menu-${serviceId}`);
    if (menu) {
        menu.classList.toggle('hidden');
    }
};

// Sayfa herhangi bir yerine tıklandığında menüleri kapat
document.addEventListener('click', function(event) {
    if (!event.target.closest('[id^="service-menu-"]') && !event.target.closest('button[onclick^="toggleServiceMenu"]')) {
        document.querySelectorAll('[id^="service-menu-"]').forEach(menu => {
            menu.classList.add('hidden');
        });
    }
});

// Credential sil
window.deleteDeviceCredentials = async function() {
    if (!confirm('Are you sure you want to delete credentials? Service monitoring will stop.')) {
        return;
    }

    try {
        const response = await fetch(`/api/targets/${currentTargetId}/credentials`, {
            method: 'DELETE',
            headers: {
                'Authorization': `Bearer ${localStorage.getItem('token')}`
            }
        });

        if (!response.ok) {
            throw new Error('Failed to delete credentials');
        }

        window.showToast('Credentials deleted', 'success');
        loadTargetCredentials(currentTargetId);

        // Servisleri gizle
        const servicesSection = document.getElementById('services-section');
        if (servicesSection) servicesSection.classList.add('hidden');

    } catch (error) {
        console.error('Error deleting credentials:', error);
        window.showToast('Delete failed', 'error');
    }
};

// Helper: escapeHtml
function escapeHtml(text) {
    if (!text) return '';
    const div = document.createElement('div');
    div.textContent = text;
    return div.innerHTML;
}

function getMonitorToggleLabel(isMonitored) {
    return isMonitored ? 'Remove from monitoring' : 'Add to monitoring';
}

function updateServiceMonitoredState(serviceId, isMonitored) {
    if (Array.isArray(window.currentDeviceServices)) {
        const service = window.currentDeviceServices.find(item => item.id === serviceId);
        if (service) {
            service.is_monitored = isMonitored;
        }
    }
}

function setServiceToggleState(serviceId, isMonitored) {
    const row = document.querySelector(`tr[data-service-id="${serviceId}"]`);
    if (!row) return;

    const button = row.querySelector('button[data-role="monitor-toggle"]');
    const knob = row.querySelector('.service-toggle-knob');
    const onIcon = row.querySelector('.service-toggle-icon-on');
    const offIcon = row.querySelector('.service-toggle-icon-off');
    if (!button || !knob) return;

    const label = getMonitorToggleLabel(isMonitored);
    const baseButtonClasses = 'service-toggle relative inline-flex h-7 w-12 items-center rounded-full border border-white/10 shadow-inner transition-all duration-300 ease-out focus:outline-none focus:ring-2 focus:ring-purple-500/40';
    const onButtonClasses = ' bg-gradient-to-r from-purple-500 to-indigo-500 shadow-purple-500/40';
    const offButtonClasses = ' bg-gray-700/50';
    button.className = baseButtonClasses + (isMonitored ? onButtonClasses : offButtonClasses);
    button.title = label;
    button.setAttribute('aria-label', label);
    button.setAttribute('data-monitored', String(isMonitored));

    const baseKnobClasses = 'service-toggle-knob inline-flex h-5 w-5 transform items-center justify-center rounded-full bg-white shadow-md ring-1 ring-black/5 transition-transform duration-300';
    knob.className = baseKnobClasses + (isMonitored ? ' translate-x-[22px]' : ' translate-x-[2px]');

    if (onIcon && offIcon) {
        onIcon.classList.toggle('hidden', !isMonitored);
        offIcon.classList.toggle('hidden', isMonitored);
    }
}


function getSelectedServiceIds() {
    return Array.from(document.querySelectorAll('.service-select-checkbox:checked'))
        .map(checkbox => parseInt(checkbox.value, 10))
        .filter(Number.isFinite);
}

function clearServiceSelections() {
    document.querySelectorAll('.service-select-checkbox').forEach(checkbox => {
        checkbox.checked = false;
    });
    updateServiceBulkToolbar();
}

function updateServiceBulkToolbar() {
    const toolbar = document.getElementById('services-bulk-toolbar');
    const countEl = document.getElementById('services-selected-count');
    const selectAll = document.getElementById('services-select-all');
    const checkboxes = Array.from(document.querySelectorAll('.service-select-checkbox'));
    const selectedCount = checkboxes.filter(checkbox => checkbox.checked).length;

    if (countEl) {
        countEl.textContent = String(selectedCount);
    }

    if (toolbar) {
        toolbar.classList.toggle('hidden', selectedCount === 0);
    }

    if (selectAll) {
        if (checkboxes.length === 0) {
            selectAll.checked = false;
            selectAll.indeterminate = false;
        } else {
            selectAll.checked = selectedCount > 0 && selectedCount === checkboxes.length;
            selectAll.indeterminate = selectedCount > 0 && selectedCount < checkboxes.length;
        }
    }
}

window.toggleSelectAllServices = function(checked) {
    document.querySelectorAll('.service-select-checkbox').forEach(checkbox => {
        checkbox.checked = checked;
    });
    updateServiceBulkToolbar();
};

window.toggleServiceSelection = function() {
    updateServiceBulkToolbar();
};

window.bulkMonitorSelectedServices = async function(enableMonitoring = true) {
    if (!currentTargetId) return;

    const addButton = document.getElementById('bulk-monitor-add-btn');
    const removeButton = document.getElementById('bulk-monitor-remove-btn');
    const selectedIds = getSelectedServiceIds();
    if (selectedIds.length === 0) return;

    const originalAddLabel = addButton ? addButton.innerHTML : '';
    const originalRemoveLabel = removeButton ? removeButton.innerHTML : '';
    if (addButton) addButton.disabled = true;
    if (removeButton) removeButton.disabled = true;
    if (enableMonitoring && addButton) {
        addButton.innerHTML = '<i class="fas fa-spinner fa-spin mr-2"></i> Monitoring...';
    }
    if (!enableMonitoring && removeButton) {
        removeButton.innerHTML = '<i class="fas fa-spinner fa-spin mr-2"></i> Unmonitoring...';
    }

    let successCount = 0;
    let skippedCount = 0;
    let failedCount = 0;

    for (const serviceId of selectedIds) {
        const service = Array.isArray(window.currentDeviceServices)
            ? window.currentDeviceServices.find(item => item.id === serviceId)
            : null;

        if (enableMonitoring && service && service.is_monitored) {
            skippedCount += 1;
            continue;
        }
        if (!enableMonitoring && service && !service.is_monitored) {
            skippedCount += 1;
            continue;
        }

        const ok = await window.toggleServiceMonitoring(serviceId, enableMonitoring, { silent: true });
        if (ok) {
            successCount += 1;
        } else {
            failedCount += 1;
        }
    }

    if (addButton) {
        addButton.disabled = false;
        addButton.innerHTML = originalAddLabel || '<i class="fas fa-eye mr-2"></i> Bulk Monitor';
    }
    if (removeButton) {
        removeButton.disabled = false;
        removeButton.innerHTML = originalRemoveLabel || '<i class="fas fa-eye-slash mr-2"></i> Bulk Unmonitor';
    }

    clearServiceSelections();

    if (successCount > 0) {
        const summary = enableMonitoring
            ? `${successCount} services added to monitoring${skippedCount > 0 ? `, ${skippedCount} already monitored` : ''}`
            : `${successCount} services removed from monitoring${skippedCount > 0 ? `, ${skippedCount} already unmonitored` : ''}`;
        window.showToast(summary, 'success');
    }

    if (failedCount > 0) {
        const errorMessage = enableMonitoring
            ? `${failedCount} services could not be monitored`
            : `${failedCount} services could not be unmonitored`;
        window.showToast(errorMessage, 'error');
    }
};

// ==========================================
// Service History Functions
// ==========================================

let historyPanelOpen = false;
let historyCurrentPage = 1;
let historyTotalPages = 1;
let historyLimit = 20;

// Toggle history panel - replaces services table
window.toggleServiceHistory = function() {
    const historyPanel = document.getElementById('service-history-panel');
    const tableContainer = document.getElementById('services-table-container');
    const toggleBtn = document.getElementById('toggle-history-btn');

    if (!historyPanel || !tableContainer) return;

    historyPanelOpen = !historyPanelOpen;

    if (historyPanelOpen) {
        // Show history, hide table
        tableContainer.classList.add('hidden');
        historyPanel.classList.remove('hidden');
        toggleBtn.classList.add('ring-2', 'ring-indigo-400/50');

        // Update button text
        toggleBtn.innerHTML = '<i class="fas fa-list text-xs"></i><span>Services</span>';

        // Load history data
        historyCurrentPage = 1;
        loadServiceHistory();
    } else {
        // Show table, hide history
        historyPanel.classList.add('hidden');
        tableContainer.classList.remove('hidden');
        toggleBtn.classList.remove('ring-2', 'ring-indigo-400/50');

        // Update button text
        toggleBtn.innerHTML = '<i class="fas fa-history text-xs"></i><span>History</span>';
    }
};

// Load service history from API
async function loadServiceHistory() {
    if (!currentTargetId) return;

    const content = document.getElementById('history-content');
    const loadingState = document.getElementById('history-loading-state');
    const emptyState = document.getElementById('history-empty-state');
    const pagination = document.getElementById('history-pagination');
    const totalCount = document.getElementById('history-total-count');

    if (!content) return;

    // Show loading
    content.innerHTML = '';
    if (loadingState) loadingState.classList.remove('hidden');
    if (emptyState) emptyState.classList.add('hidden');
    if (pagination) pagination.classList.add('hidden');

    try {
        const response = await fetch(`/api/targets/${currentTargetId}/device-services/history?page=${historyCurrentPage}&limit=${historyLimit}`, {
            headers: {
                'Authorization': `Bearer ${localStorage.getItem('token')}`
            }
        });

        if (loadingState) loadingState.classList.add('hidden');

        if (!response.ok) {
            throw new Error('Failed to load history');
        }

        const data = await response.json();
        const history = data.history || [];
        const total = data.total || 0;

        historyTotalPages = Math.ceil(total / historyLimit);

        // Update total count
        if (totalCount) {
            totalCount.textContent = `Total ${total} records`;
        }

        if (history.length === 0) {
            if (emptyState) emptyState.classList.remove('hidden');
            return;
        }

        // Render history items
        content.innerHTML = history.map(item => {
            const oldStatusBadge = getStatusBadge(item.old_status);
            const newStatusBadge = getStatusBadge(item.new_status);
            const changedAt = new Date(item.changed_at).toLocaleString('en-US');
            const serviceName = item.display_name || item.service_name;

            return `
                <div class="flex items-center gap-4 p-3 rounded-lg bg-gray-800/40 hover:bg-gray-800/60 transition-colors mb-2 border border-gray-700/30">
                    <div class="flex-shrink-0 w-8 h-8 rounded-full bg-indigo-500/20 flex items-center justify-center">
                        <i class="fas fa-exchange-alt text-indigo-400 text-xs"></i>
                    </div>
                    <div class="flex-1 min-w-0">
                        <div class="flex items-center gap-2 flex-wrap">
                            <span class="font-medium text-white text-sm truncate">${escapeHtml(serviceName)}</span>
                            ${item.service_name !== serviceName ? `<span class="text-xs text-gray-500">(${escapeHtml(item.service_name)})</span>` : ''}
                        </div>
                        <div class="flex items-center gap-2 mt-1 text-xs">
                            ${oldStatusBadge}
                            <i class="fas fa-arrow-right text-gray-500"></i>
                            ${newStatusBadge}
                        </div>
                    </div>
                    <div class="flex-shrink-0 text-right">
                        <div class="text-xs text-gray-400">${changedAt}</div>
                    </div>
                </div>
            `;
        }).join('');

        // Update pagination
        updateHistoryPagination(total);

    } catch (error) {
        console.error('Error loading service history:', error);
        if (loadingState) loadingState.classList.add('hidden');
        content.innerHTML = '<div class="text-center text-red-400 py-4"><i class="fas fa-exclamation-circle mr-2"></i>History could not be loaded</div>';
    }
}

// Get status badge HTML
function getStatusBadge(status) {
    if (!status) {
        return '<span class="px-2 py-0.5 rounded-full text-xs bg-gray-600/50 text-gray-300">-</span>';
    }

    const badges = {
        'running': '<span class="px-2 py-0.5 rounded-full text-xs bg-green-500/20 text-green-400 border border-green-500/30"><i class="fas fa-circle text-[8px] mr-1"></i>Running</span>',
        'stopped': '<span class="px-2 py-0.5 rounded-full text-xs bg-red-500/20 text-red-400 border border-red-500/30"><i class="fas fa-circle text-[8px] mr-1"></i>Stopped</span>',
        'unknown': '<span class="px-2 py-0.5 rounded-full text-xs bg-gray-500/20 text-gray-400 border border-gray-500/30"><i class="fas fa-question text-[8px] mr-1"></i>Unknown</span>'
    };

    return badges[status] || badges['unknown'];
}

// Update pagination UI
function updateHistoryPagination(total) {
    const pagination = document.getElementById('history-pagination');
    const pageInfo = document.getElementById('history-page-info');
    const prevBtn = document.getElementById('history-prev-btn');
    const nextBtn = document.getElementById('history-next-btn');

    if (!pagination) return;

    if (total > historyLimit) {
        pagination.classList.remove('hidden');

        const start = (historyCurrentPage - 1) * historyLimit + 1;
        const end = Math.min(historyCurrentPage * historyLimit, total);

        if (pageInfo) {
            pageInfo.textContent = `${start}-${end} / ${total} records (Page ${historyCurrentPage}/${historyTotalPages})`;
        }

        if (prevBtn) {
            prevBtn.disabled = historyCurrentPage <= 1;
        }
        if (nextBtn) {
            nextBtn.disabled = historyCurrentPage >= historyTotalPages;
        }
    } else {
        pagination.classList.add('hidden');
    }
}

// Load history page
window.loadServiceHistoryPage = function(direction) {
    if (direction === 'prev' && historyCurrentPage > 1) {
        historyCurrentPage--;
    } else if (direction === 'next' && historyCurrentPage < historyTotalPages) {
        historyCurrentPage++;
    }
    loadServiceHistory();
};

// Refresh history
window.refreshServiceHistory = function() {
    if (historyPanelOpen) {
        loadServiceHistory();
    }
};

// Reset history panel when modal closes
const originalCloseDeviceServicesModal = window.closeDeviceServicesModal;
window.closeDeviceServicesModal = function() {
    // Reset history state
    historyPanelOpen = false;
    historyCurrentPage = 1;

    const historyPanel = document.getElementById('service-history-panel');
    const tableContainer = document.getElementById('services-table-container');
    const toggleBtn = document.getElementById('toggle-history-btn');

    // Reset to show table, hide history
    if (historyPanel) {
        historyPanel.classList.add('hidden');
    }
    if (tableContainer) {
        tableContainer.classList.remove('hidden');
    }
    if (toggleBtn) {
        toggleBtn.classList.remove('ring-2', 'ring-indigo-400/50');
        toggleBtn.innerHTML = '<i class="fas fa-history text-xs"></i><span>History</span>';
    }

    // Call original function
    if (originalCloseDeviceServicesModal) {
        originalCloseDeviceServicesModal();
    } else {
        const modal = document.getElementById('device-services-modal');
        if (modal) {
            modal.classList.add('hidden');
            modal.classList.remove('flex');
            modal.style.display = '';
        }
        currentTargetId = null;
        currentTargetName = '';
    }
};

console.log('✅ target_services.js loaded');






