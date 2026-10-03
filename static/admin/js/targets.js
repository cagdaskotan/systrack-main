/**
 * Targets Tag Accordion Mode
 * Hedefleri etiket klasorleriyle alt alta accordion olarak gosterir.
 */
(function () {
    'use strict';

    const TARGETS_BATCH_SIZE = 100;
    const TARGETS_MAX_PAGES = 500;

    const state = {
        allTargets: [],
        filteredTargets: [],
        folders: [],
        openFolders: new Set(),
        loading: false,
        overridesInstalled: false,
        originals: {
            loadTargets: null,
            filterTargets: null,
            clearFilters: null,
            previousPage: null,
            nextPage: null,
            changePageSize: null
        }
    };

    function isTargetsMounted() {
        return !!document.getElementById('folder-list-container') && !!document.getElementById('target-search');
    }

    function initForCurrentMount() {
        if (!isTargetsMounted()) return;

        const folderView = document.getElementById('folder-view-container');
        if (folderView && folderView.dataset.accordionReady === '1') return;

        setupLayout();
        bindAccordionEvents();
        installOverrides();
        loadAndRenderTargets();

        if (folderView) {
            folderView.dataset.accordionReady = '1';
        }
    }

    function setupLayout() {
        const listView = document.getElementById('list-view-container');
        const folderView = document.getElementById('folder-view-container');
        const folderBreadcrumb = document.getElementById('folder-breadcrumb');
        const tagFilterWrapper = document.getElementById('tag-filter-wrapper');
        const pageSizeSelect = document.getElementById('page-size-select');

        if (listView) listView.classList.add('hidden');
        if (folderView) folderView.classList.remove('hidden');
        if (folderBreadcrumb) folderBreadcrumb.classList.add('hidden');

        if (tagFilterWrapper) tagFilterWrapper.classList.add('hidden');
        if (pageSizeSelect && pageSizeSelect.parentElement) {
            pageSizeSelect.parentElement.classList.add('hidden');
        }

        const toggleContainer = document.querySelector('.view-toggle-container');
        if (toggleContainer) {
            const wrapper = toggleContainer.closest('.w-full') || toggleContainer.parentElement;
            if (wrapper) wrapper.classList.add('hidden');
        }

        hidePaginationControls();
        updatePaginationText(0);
        syncFolderBulkToolbar();
    }

    function bindAccordionEvents() {
        const container = document.getElementById('folder-list-container');
        if (!container || container.dataset.bound === '1') return;

        container.addEventListener('click', (event) => {
            const toggleBtn = event.target.closest('.js-accordion-toggle');
            if (toggleBtn) {
                const item = toggleBtn.closest('.tag-accordion-item');
                if (!item) return;

                const folderName = item.dataset.folderName || '';
                const willOpen = !item.classList.contains('open');
                item.classList.toggle('open', willOpen);
                item.querySelectorAll('.js-accordion-toggle').forEach((btn) => {
                    btn.setAttribute('aria-expanded', willOpen ? 'true' : 'false');
                });

                if (willOpen) {
                    state.openFolders.add(folderName);
                } else {
                    state.openFolders.delete(folderName);
                }
                return;
            }

            const servicesBtn = event.target.closest('.js-services-btn');
            if (servicesBtn) {
                const targetId = Number(servicesBtn.dataset.targetId || 0);
                const targetName = servicesBtn.dataset.targetName || '';
                if (targetId > 0 && typeof window.showDeviceServicesModal === 'function') {
                    window.showDeviceServicesModal(targetId, targetName);
                }
                return;
            }

            const editFolderBtn = event.target.closest('.js-folder-edit');
            if (editFolderBtn) {
                const folderName = editFolderBtn.dataset.folderName || '';
                renameFolderTag(folderName);
                return;
            }

            const deleteFolderBtn = event.target.closest('.js-folder-delete');
            if (deleteFolderBtn) {
                const folderName = deleteFolderBtn.dataset.folderName || '';
                removeFolderTag(folderName);
            }
        });

        container.addEventListener('change', (event) => {
            const folderCheckbox = event.target.closest('.folder-header-checkbox');
            if (folderCheckbox) {
                const table = folderCheckbox.closest('.folder-target-table');
                if (!table) return;
                const shouldCheck = !!folderCheckbox.checked;
                if (!window.selectedTargetIds) window.selectedTargetIds = new Set();

                table.querySelectorAll('.target-checkbox').forEach((checkbox) => {
                    checkbox.checked = shouldCheck;
                    const id = Number(checkbox.value);
                    if (!Number.isNaN(id) && id > 0) {
                        if (shouldCheck) window.selectedTargetIds.add(id);
                        else window.selectedTargetIds.delete(id);
                    }
                });

                if (typeof window.updateSelection === 'function') {
                    window.updateSelection();
                }
                syncFolderBulkToolbar();
                return;
            }

            const checkbox = event.target.closest('.target-checkbox');
            if (!checkbox) return;
            if (typeof window.onTargetCheckboxChange === 'function') {
                window.onTargetCheckboxChange(checkbox);
            }
            syncFolderBulkToolbar();
        });

        container.dataset.bound = '1';
    }

    function bindFolderSelectionEvents() {
        const selectAll = document.getElementById('folder-select-all');
        if (!selectAll || selectAll.dataset.bound === '1') return;

        selectAll.addEventListener('change', () => {
            const shouldCheck = !!selectAll.checked;
            if (!window.selectedTargetIds) window.selectedTargetIds = new Set();

            document.querySelectorAll('#folder-list-container .target-checkbox').forEach((checkbox) => {
                checkbox.checked = shouldCheck;
                const id = Number(checkbox.value);
                if (!Number.isNaN(id) && id > 0) {
                    if (shouldCheck) {
                        window.selectedTargetIds.add(id);
                    } else {
                        window.selectedTargetIds.delete(id);
                    }
                }
            });

            if (typeof window.updateSelection === 'function') {
                window.updateSelection();
            }
            syncFolderBulkToolbar();
        });

        selectAll.dataset.bound = '1';
    }

    function bindTagActionModal() {
        const modal = document.getElementById('tag-action-modal');
        if (!modal || modal.dataset.bound === '1') return;

        const cancelBtn = document.getElementById('tag-modal-cancel');
        if (cancelBtn) {
            cancelBtn.addEventListener('click', () => {
                modal.classList.remove('show');
            });
        }

        modal.addEventListener('click', (event) => {
            if (event.target === modal) {
                modal.classList.remove('show');
            }
        });

        modal.dataset.bound = '1';
    }

    function installOverrides() {
        if (state.overridesInstalled) return;
        if (typeof window.loadTargets !== 'function' || typeof window.filterTargets !== 'function' || typeof window.clearFilters !== 'function') {
            return;
        }

        state.originals.loadTargets = window.loadTargets;
        state.originals.filterTargets = window.filterTargets;
        state.originals.clearFilters = window.clearFilters;
        state.originals.previousPage = window.previousPage;
        state.originals.nextPage = window.nextPage;
        state.originals.changePageSize = window.changePageSize;

        window.loadTargets = async function () {
            if (!isTargetsMounted()) {
                if (typeof state.originals.loadTargets === 'function') {
                    return state.originals.loadTargets();
                }
                return;
            }
            await loadAndRenderTargets();
        };

        window.filterTargets = function () {
            if (!isTargetsMounted()) {
                if (typeof state.originals.filterTargets === 'function') {
                    return state.originals.filterTargets();
                }
                return;
            }

            syncCurrentFilters();
            applyFiltersAndBuildFolders();
            renderAccordion();
        };

        window.clearFilters = function () {
            if (!isTargetsMounted()) {
                if (typeof state.originals.clearFilters === 'function') {
                    return state.originals.clearFilters();
                }
                return;
            }

            const searchInput = document.getElementById('target-search');
            const typeFilter = document.getElementById('type-filter');
            const statusFilter = document.getElementById('status-filter');
            const tagFilter = document.getElementById('tag-filter');

            if (searchInput) searchInput.value = '';
            if (typeFilter) typeFilter.value = '';
            if (statusFilter) statusFilter.value = '';
            if (tagFilter) tagFilter.value = '';

            syncCurrentFilters();
            applyFiltersAndBuildFolders();
            renderAccordion();
        };

        window.previousPage = function () {};
        window.nextPage = function () {};
        window.changePageSize = function () {};

        state.overridesInstalled = true;
    }

    function getFiltersFromDom() {
        const searchInput = document.getElementById('target-search');
        const typeFilter = document.getElementById('type-filter');
        const statusFilter = document.getElementById('status-filter');

        return {
            search: (searchInput?.value || '').trim().toLowerCase(),
            type: (typeFilter?.value || '').trim().toLowerCase(),
            status: (statusFilter?.value || '').trim().toLowerCase()
        };
    }

    function syncCurrentFilters() {
        const filters = getFiltersFromDom();
        if (!window.currentFilters) window.currentFilters = {};
        window.currentFilters.search = filters.search;
        window.currentFilters.type = filters.type;
        window.currentFilters.status = filters.status;
        window.currentFilters.tag = '';
    }

    async function loadAndRenderTargets() {
        if (state.loading) return;
        state.loading = true;

        showLoading();

        try {
            state.allTargets = await fetchAllTargetsPaged();

            syncCurrentFilters();
            applyFiltersAndBuildFolders();
            renderAccordion();
        } catch (error) {
            renderError(error.message || 'Hedefler yüklenirken hata oluştu.');
        } finally {
            state.loading = false;
        }
    }

    async function fetchAllTargetsPaged() {
        const allTargets = [];
        const seenIds = new Set();
        let offset = 0;
        let page = 0;
        let total = null;

        while (page < TARGETS_MAX_PAGES) {
            const url = `/api/targets?limit=${TARGETS_BATCH_SIZE}&offset=${offset}`;
            const response = await fetch(url, {
                method: 'GET',
                headers: {
                    Authorization: `Bearer ${localStorage.getItem('token') || ''}`,
                    'Content-Type': 'application/json'
                }
            });

            if (!response.ok) {
                throw new Error('Hedef listesi alınamadı.');
            }

            const data = await response.json();
            const targets = Array.isArray(data.targets) ? data.targets : [];
            const pagination = data.pagination || {};

            if (typeof pagination.total === 'number') {
                total = pagination.total;
            }

            for (const target of targets) {
                const key = Number(target?.id);
                if (Number.isFinite(key) && key > 0) {
                    if (seenIds.has(key)) continue;
                    seenIds.add(key);
                }
                allTargets.push(target);
            }

            // Keep UI responsive for large target sets.
            updatePaginationText(allTargets.length);

            const hasMoreByBackend = Boolean(pagination.has_more);
            const hasMoreByTotal = typeof total === 'number' ? allTargets.length < total : false;
            const shouldContinue = hasMoreByBackend || hasMoreByTotal;

            if (!shouldContinue || targets.length === 0) {
                break;
            }

            offset += TARGETS_BATCH_SIZE;
            page += 1;
        }

        return allTargets;
    }

    function applyFiltersAndBuildFolders() {
        const filters = getFiltersFromDom();

        state.filteredTargets = state.allTargets.filter((target) => {
            if (filters.search) {
                const text = [
                    target.name || '',
                    target.address || '',
                    target.type || '',
                    target.monitoring_type || '',
                    target.tags || ''
                ].join(' ').toLowerCase();

                if (!text.includes(filters.search)) return false;
            }

            if (filters.type) {
                const targetType = normalizeType(target.monitoring_type || target.type);
                const selectedType = normalizeType(filters.type);
                if (targetType !== selectedType) return false;
            }

            if (filters.status) {
                const isOnline = !!target.is_online;
                if (filters.status === 'online' && !isOnline) return false;
                if (filters.status === 'offline' && isOnline) return false;
            }

            return true;
        });

        const folderMap = new Map();

        state.filteredTargets.forEach((target) => {
            const tags = parseTags(target.tags);

            if (tags.length === 0) {
                addTargetToFolder(folderMap, 'Etiketsiz', target, true);
                return;
            }

            tags.forEach((tag) => {
                addTargetToFolder(folderMap, tag, target, false);
            });
        });

        const folders = Array.from(folderMap.values()).sort((a, b) => {
            if (a.isUntagged) return 1;
            if (b.isUntagged) return -1;
            return a.name.localeCompare(b.name, 'tr');
        });

        const validNames = new Set(folders.map((folder) => folder.name));
        state.openFolders = new Set(Array.from(state.openFolders).filter((name) => validNames.has(name)));
        state.folders = folders;
    }

    function addTargetToFolder(folderMap, folderName, target, isUntagged) {
        if (!folderMap.has(folderName)) {
            folderMap.set(folderName, {
                name: folderName,
                isUntagged,
                targets: []
            });
        }

        folderMap.get(folderName).targets.push(target);
    }

    function renderAccordion() {
        const container = document.getElementById('folder-list-container');
        if (!container) return;

        updateCounters();
        updatePaginationText(state.filteredTargets.length);
        hidePaginationControls();

        if (state.folders.length === 0) {
            container.innerHTML = `
                <div class="text-center py-10 border border-dashed border-gray-300 dark:border-gray-600 rounded-xl">
                    <p class="text-sm text-gray-500 dark:text-gray-400">Filtrelere uygun hedef bulunamadı.</p>
                </div>
            `;
            return;
        }

        container.innerHTML = state.folders.map((folder) => {
            const isOpen = state.openFolders.has(folder.name);
            const editableFolder = !folder.isUntagged && window.canEditModule && window.canEditModule('targets');
            return `
                <div class="tag-accordion-item ${isOpen ? 'open' : ''}" data-folder-name="${escapeHtml(folder.name)}">
                    <div class="tag-accordion-trigger">
                        <button type="button" class="tag-accordion-toggle js-accordion-toggle" aria-expanded="${isOpen ? 'true' : 'false'}">
                            <span class="tag-accordion-left">
                                <svg class="tag-accordion-icon" fill="currentColor" viewBox="0 0 20 20" aria-hidden="true">
                                    <path d="M2 6a2 2 0 012-2h5l2 2h5a2 2 0 012 2v6a2 2 0 01-2 2H4a2 2 0 01-2-2V6z"></path>
                                </svg>
                                <span class="tag-accordion-name">${escapeHtml(folder.name)}</span>
                            </span>
                            <span class="flex items-center gap-2">
                                <span class="tag-accordion-count">${folder.targets.length} hedef</span>
                            </span>
                        </button>
                        ${editableFolder ? `
                            <span class="tag-folder-actions">
                                <button type="button"
                                        class="tag-folder-action edit js-folder-edit"
                                        data-folder-name="${escapeHtml(folder.name)}"
                                        title="Etiketi düzenle"
                                        aria-label="Etiketi düzenle">
                                    <i class="fas fa-pen text-[10px]"></i>
                                </button>
                                <button type="button"
                                        class="tag-folder-action delete js-folder-delete"
                                        data-folder-name="${escapeHtml(folder.name)}"
                                        title="Etiketi sil"
                                        aria-label="Etiketi sil">
                                    <i class="fas fa-trash text-[10px]"></i>
                                </button>
                            </span>
                        ` : ''}
                        <button type="button" class="tag-accordion-chevron-btn js-accordion-toggle" aria-expanded="${isOpen ? 'true' : 'false'}">
                            <svg class="tag-accordion-chevron" fill="none" stroke="currentColor" viewBox="0 0 24 24" aria-hidden="true">
                                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7"></path>
                            </svg>
                        </button>
                    </div>
                    <div class="tag-accordion-content">
                        <div class="tag-accordion-content-inner">
                            ${renderFolderTable(folder.name, folder.targets)}
                        </div>
                    </div>
                </div>
            `;
        }).join('');

        bindFolderSelectionEvents();
        bindTagActionModal();
        syncFolderBulkToolbar();
    }

    function renderFolderTable(folderName, targets) {
        if (!Array.isArray(targets) || targets.length === 0) {
            return '<div class="py-5 text-sm text-gray-500 dark:text-gray-400">Bu etikette hedef yok.</div>';
        }

        const rows = targets.map((target) => renderTargetRow(folderName, target)).join('');
        return `
            <div class="overflow-x-auto">
                <table class="folder-target-table min-w-full text-sm" data-folder-name="${escapeHtml(folderName)}">
                    <colgroup>
                        <col style="width:5%">
                        <col style="width:16%">
                        <col style="width:14%">
                        <col style="width:9%">
                        <col style="width:10%">
                        <col style="width:12%">
                        <col style="width:34%">
                    </colgroup>
                    <thead>
                        <tr>
                            <th class="col-select px-3 py-2.5 text-xs font-medium text-gray-500 dark:text-gray-300 uppercase tracking-wider whitespace-nowrap">
                                <input type="checkbox"
                                       class="folder-header-checkbox folder-select-header-checkbox rounded border-gray-300 text-primary-600 focus:ring-primary-500"
                                       title="Bu etiketteki tüm hedefleri seç">
                            </th>
                            <th class="px-3 py-2.5 text-left text-xs font-medium text-gray-500 dark:text-gray-300 uppercase tracking-wider whitespace-nowrap">Ad</th>
                            <th class="px-3 py-2.5 text-left text-xs font-medium text-gray-500 dark:text-gray-300 uppercase tracking-wider whitespace-nowrap">Adres</th>
                            <th class="col-type px-3 py-2.5 text-xs font-medium text-gray-500 dark:text-gray-300 uppercase tracking-wider whitespace-nowrap">Tip</th>
                            <th class="col-status px-3 py-2.5 text-xs font-medium text-gray-500 dark:text-gray-300 uppercase tracking-wider whitespace-nowrap">Durum</th>
                            <th class="col-tags px-3 py-2.5 text-xs font-medium text-gray-500 dark:text-gray-300 uppercase tracking-wider whitespace-nowrap">Etiketler</th>
                            <th class="col-actions px-3 py-2.5 text-xs font-medium text-gray-500 dark:text-gray-300 uppercase tracking-wider whitespace-nowrap">İşlemler</th>
                        </tr>
                    </thead>
                    <tbody class="divide-y divide-gray-200 dark:divide-gray-700">
                        ${rows}
                    </tbody>
                </table>
            </div>
        `;
    }

    function renderTargetRow(folderName, target) {
        const tags = parseTags(target.tags);
        const statusClass = target.is_online
            ? 'bg-green-100 dark:bg-green-900/20 text-green-800 dark:text-green-300'
            : 'bg-red-100 dark:bg-red-900/20 text-red-800 dark:text-red-300';
        const statusText = target.is_online ? 'Online' : 'Offline';

        const type = normalizeType(target.monitoring_type || target.type);
        const typeClass = getTypeBadgeClass(type);

        const tagsHtml = tags.map((tag) => {
            const color = target.tag_colors && target.tag_colors[tag] ? target.tag_colors[tag] : '#3B82F6';
            return `<span class="inline-block px-2 py-1 text-xs font-medium rounded-full text-white mr-1 mb-1" style="background-color:${color}">${escapeHtml(tag)}</span>`;
        }).join('');

        return `
            <tr>
                <td class="col-select px-3 py-2.5">
                    ${window.canEditModule && window.canEditModule('targets') ? `
                        <input type="checkbox"
                               class="target-checkbox rounded border-gray-300 text-primary-600 focus:ring-primary-500"
                               value="${target.id}"
                               data-folder-name="${escapeHtml(folderName)}"
                               ${window.selectedTargetIds && window.selectedTargetIds.has(target.id) ? 'checked' : ''}>
                    ` : ''}
                </td>
                <td class="px-3 py-2.5 text-gray-900 dark:text-gray-100 whitespace-nowrap">${escapeHtml(target.name || '-')}</td>
                <td class="px-3 py-2.5 text-gray-600 dark:text-gray-300 whitespace-nowrap">${escapeHtml(target.address || '-')}</td>
                <td class="col-type px-3 py-2.5 whitespace-nowrap">
                    <span class="px-2 py-1 text-xs font-medium rounded-full ${typeClass}">${escapeHtml(type.toUpperCase())}</span>
                </td>
                <td class="col-status px-3 py-2.5 whitespace-nowrap">
                    <span class="px-2 py-1 text-xs font-medium rounded-full ${statusClass}">${statusText}</span>
                </td>
                <td class="col-tags px-3 py-2.5 text-gray-600 dark:text-gray-300">${tagsHtml}</td>
                <td class="col-actions px-3 py-2.5 whitespace-nowrap">
                    <div class="target-actions">
                        ${window.canEditModule && window.canEditModule('targets') ? `
                            <button onclick="editTarget(${target.id})"
                                    class="target-action-icon edit"
                                    title="Düzenle"
                                    aria-label="Düzenle">
                                <i class="fas fa-pen text-[11px]"></i>
                            </button>
                        ` : ''}
                        ${window.canEditModule && window.canEditModule('targets') ? `
                            <button onclick="deleteTarget(${target.id})"
                                    class="target-action-icon delete"
                                    title="Sil"
                                    aria-label="Sil">
                                <i class="fas fa-trash text-[11px]"></i>
                            </button>
                        ` : ''}
                        <button onclick="pingTarget(${target.id})" class="target-action-pill ping" title="Ping gönder">
                            <i class="fas fa-signal text-[10px]"></i> Ping
                        </button>
                        <button type="button"
                                class="js-services-btn target-action-pill services"
                                data-target-id="${target.id}"
                                data-target-name="${escapeHtml(target.name || '')}"
                                title="Servisleri Yönet">
                            <i class="fas fa-cogs text-[10px]"></i> Servisler
                        </button>
                        <button onclick="showTargetHistory(${target.id})" class="target-action-pill history" title="Geçmişi göster">
                            <i class="fas fa-history text-[10px]"></i> Geçmiş
                        </button>
                        <button onclick="openConnectDropdown(event)"
                                data-target-id="${target.id}"
                                data-target-address="${escapeHtml(target.address || '')}"
                                class="target-action-pill connect"
                                title="Bağlantı seçenekleri">
                            <i class="fas fa-plug text-[10px]"></i> Bağlan <i class="fas fa-chevron-down ml-1 text-[9px]"></i>
                        </button>
                    </div>
                </td>
            </tr>
        `;
    }

    function updateCounters() {
        const targetsTotal = document.getElementById('targets-total-count');
        const foldersTotal = document.getElementById('folders-total-count');

        if (targetsTotal) targetsTotal.textContent = String(state.filteredTargets.length);
        if (foldersTotal) foldersTotal.textContent = String(state.folders.length);
    }

    function syncFolderBulkToolbar() {
        const countEl = document.getElementById('folder-selected-count');
        const bulkDeleteBtn = document.getElementById('folder-bulk-delete-btn');
        const bulkEditBtn = document.getElementById('folder-bulk-edit-btn');
        const bulkEditCount = document.getElementById('folder-bulk-edit-count');
        const selectAll = document.getElementById('folder-select-all');
        if (!countEl || !bulkDeleteBtn) return;

        const selectedCount = document.querySelectorAll('#folder-list-container .target-checkbox:checked').length;
        countEl.textContent = String(selectedCount);
        bulkDeleteBtn.classList.toggle('hidden', selectedCount === 0);
        if (bulkEditBtn) bulkEditBtn.classList.toggle('hidden', selectedCount === 0);
        if (bulkEditCount) bulkEditCount.textContent = String(selectedCount);

        if (selectAll) {
            const allCheckboxes = document.querySelectorAll('#folder-list-container .target-checkbox');
            const totalVisible = allCheckboxes.length;
            const checkedVisible = document.querySelectorAll('#folder-list-container .target-checkbox:checked').length;
            if (checkedVisible === 0) {
                selectAll.checked = false;
                selectAll.indeterminate = false;
            } else if (totalVisible > 0 && checkedVisible === totalVisible) {
                selectAll.checked = true;
                selectAll.indeterminate = false;
            } else {
                selectAll.checked = false;
                selectAll.indeterminate = true;
            }
        }

        document.querySelectorAll('#folder-list-container .folder-target-table').forEach((table) => {
            const folderHeaderCheckbox = table.querySelector('.folder-header-checkbox');
            if (!folderHeaderCheckbox) return;

            const rowCheckboxes = table.querySelectorAll('.target-checkbox');
            const checkedCount = table.querySelectorAll('.target-checkbox:checked').length;

            if (checkedCount === 0) {
                folderHeaderCheckbox.checked = false;
                folderHeaderCheckbox.indeterminate = false;
            } else if (rowCheckboxes.length > 0 && checkedCount === rowCheckboxes.length) {
                folderHeaderCheckbox.checked = true;
                folderHeaderCheckbox.indeterminate = false;
            } else {
                folderHeaderCheckbox.checked = false;
                folderHeaderCheckbox.indeterminate = true;
            }
        });
    }

    async function showTagActionModal(options) {
        bindTagActionModal();

        const modal = document.getElementById('tag-action-modal');
        const titleEl = document.getElementById('tag-modal-title');
        const textEl = document.getElementById('tag-modal-text');
        const inputEl = document.getElementById('tag-modal-input');
        const noteEl = document.getElementById('tag-modal-note');
        const confirmBtn = document.getElementById('tag-modal-confirm');
        const cancelBtn = document.getElementById('tag-modal-cancel');

        if (!modal || !titleEl || !textEl || !inputEl || !noteEl || !confirmBtn || !cancelBtn) {
            return { confirmed: false, value: '' };
        }

        titleEl.textContent = options.title || 'Etiket İşlemi';
        textEl.textContent = options.message || '';
        const baseConfirmText = options.confirmText || 'Onayla';
        const loadingText = options.loadingText || 'İşleniyor...';
        confirmBtn.textContent = baseConfirmText;
        confirmBtn.classList.remove('primary', 'danger');
        confirmBtn.classList.add(options.confirmVariant === 'danger' ? 'danger' : 'primary');

        if (options.showInput) {
            inputEl.classList.remove('hidden');
            inputEl.value = options.defaultValue || '';
            setTimeout(() => inputEl.focus(), 30);
        } else {
            inputEl.classList.add('hidden');
            inputEl.value = '';
        }

        if (options.note) {
            noteEl.classList.remove('hidden');
            noteEl.textContent = options.note;
        } else {
            noteEl.classList.add('hidden');
            noteEl.textContent = '';
        }

        modal.classList.add('show');

        return new Promise((resolve) => {
            let busy = false;
            const setBusy = (value) => {
                busy = !!value;
                confirmBtn.disabled = busy;
                cancelBtn.disabled = busy;
                inputEl.disabled = busy;
                confirmBtn.classList.toggle('opacity-70', busy);
                confirmBtn.classList.toggle('cursor-not-allowed', busy);
                confirmBtn.textContent = busy ? loadingText : baseConfirmText;
            };
            const reportError = (error) => {
                if (typeof options.onError === 'function') {
                    options.onError(error);
                    return;
                }
                if (window.Toast && window.Toast.show) {
                    window.Toast.show({
                        type: 'error',
                        title: 'Hata',
                        message: error?.message || 'İşlem tamamlanamadı.',
                        timeout: 4000
                    });
                }
            };
            const onConfirm = async () => {
                if (busy) return;
                const value = inputEl.value.trim();

                if (typeof options.onConfirm === 'function') {
                    try {
                        setBusy(true);
                        await options.onConfirm(value);
                        done(true, value);
                    } catch (error) {
                        setBusy(false);
                        reportError(error);
                    }
                    return;
                }

                done(true, value);
            };
            const onCancel = () => {
                if (busy) return;
                done(false, inputEl.value.trim());
            };
            const onKeyDown = (event) => {
                if (busy) return;
                if (event.key === 'Escape') {
                    event.preventDefault();
                    done(false, inputEl.value.trim());
                } else if (event.key === 'Enter') {
                    event.preventDefault();
                    onConfirm();
                }
            };
            const done = (confirmed, value) => {
                cleanup();
                modal.classList.remove('show');
                resolve({ confirmed, value: value ?? inputEl.value.trim() });
            };
            const cleanup = () => {
                confirmBtn.removeEventListener('click', onConfirm);
                cancelBtn.removeEventListener('click', onCancel);
                document.removeEventListener('keydown', onKeyDown);
            };

            confirmBtn.addEventListener('click', onConfirm);
            cancelBtn.addEventListener('click', onCancel);
            document.addEventListener('keydown', onKeyDown);
        });
    }

    async function bulkUpdateTags(targetIds, operation, tags) {
        const response = await fetch('/api/targets/bulk/tags', {
            method: 'POST',
            headers: {
                'Authorization': 'Bearer ' + (localStorage.getItem('token') || ''),
                'Content-Type': 'application/json'
            },
            body: JSON.stringify({
                target_ids: targetIds,
                operation: operation,
                tags: tags
            })
        });
        if (!response.ok) {
            const data = await response.json().catch(() => ({}));
            throw new Error(data.error || 'Etiket güncellenemedi');
        }
    }

    function getTargetIdsByTag(tagName) {
        if (!tagName) return [];
        const ids = new Set();
        state.allTargets.forEach((target) => {
            const tags = parseTags(target.tags);
            if (!tags.includes(tagName)) return;
            const id = Number(target.id);
            if (!Number.isNaN(id) && id > 0) ids.add(id);
        });
        return Array.from(ids);
    }

    async function renameFolderTag(folderName) {
        if (!folderName || folderName === 'Etiketsiz') return;

        await showTagActionModal({
            title: 'Etiketi Düzenle',
            message: `"${folderName}" etiketi için yeni adı girin.`,
            confirmText: 'Kaydet',
            confirmVariant: 'primary',
            loadingText: 'Kaydediliyor...',
            showInput: true,
            defaultValue: folderName,
            onConfirm: async (value) => {
                const newTag = String(value || '').trim();
                if (!newTag) throw new Error('Etiket adı boş olamaz.');
                if (newTag.includes(',')) throw new Error('Etiket adında virgül kullanılamaz.');
                if (newTag === folderName) return;

                const targetIds = getTargetIdsByTag(folderName);
                if (targetIds.length === 0) throw new Error('Bu etikete ait hedef bulunamadı.');

                // Rename = add new tag + remove old tag.
                await bulkUpdateTags(targetIds, 'add', newTag);
                await bulkUpdateTags(targetIds, 'remove', folderName);
                await loadAndRenderTargets();

                if (window.Toast && window.Toast.show) {
                    window.Toast.show({
                        type: 'success',
                        title: 'Başarılı',
                        message: `"${folderName}" etiketi "${newTag}" olarak güncellendi.`,
                        timeout: 3000
                    });
                }
            },
            onError: (error) => {
                if (window.Toast && window.Toast.show) {
                    window.Toast.show({
                        type: 'error',
                        title: 'Hata',
                        message: error?.message || 'Etiket güncellenemedi.',
                        timeout: 4000
                    });
                }
            }
        });
    }

    async function removeFolderTag(folderName) {
        if (!folderName || folderName === 'Etiketsiz') return;

        const targetIds = getTargetIdsByTag(folderName);
        if (targetIds.length === 0) {
            if (window.Toast && window.Toast.show) {
                window.Toast.show({
                    type: 'warning',
                    title: 'Uyarı',
                    message: 'Bu etikete ait hedef bulunamadı.',
                    timeout: 3000
                });
            }
            return;
        }

        const choice = await showTagDeleteModal(folderName, targetIds.length);
        if (choice === 'cancel') return;

        try {
            if (choice === 'purge') {
                // Klasördeki tüm hedefleri kalıcı olarak sil.
                await bulkDeleteTargetsByIds(targetIds);
                if (window.selectedTargetIds) {
                    targetIds.forEach((id) => window.selectedTargetIds.delete(id));
                }
                await loadAndRenderTargets();
                if (window.Toast && window.Toast.show) {
                    window.Toast.show({
                        type: 'success',
                        title: 'Başarılı',
                        message: `"${folderName}" etiketi ve ${targetIds.length} hedef silindi.`,
                        timeout: 3500
                    });
                }
            } else {
                // Yalnızca etiketi kaldır; hedefler Etiketsiz'e taşınır.
                await bulkUpdateTags(targetIds, 'remove', folderName);
                await loadAndRenderTargets();
                if (window.Toast && window.Toast.show) {
                    window.Toast.show({
                        type: 'success',
                        title: 'Başarılı',
                        message: `"${folderName}" etiketi silindi, hedefler Etiketsiz klasörüne taşındı.`,
                        timeout: 3500
                    });
                }
            }
        } catch (error) {
            if (window.Toast && window.Toast.show) {
                window.Toast.show({
                    type: 'error',
                    title: 'Hata',
                    message: error?.message || 'İşlem tamamlanamadı.',
                    timeout: 4000
                });
            }
        }
    }

    async function bulkDeleteTargetsByIds(targetIds) {
        const response = await fetch('/api/targets/bulk/delete', {
            method: 'POST',
            headers: {
                'Authorization': 'Bearer ' + (localStorage.getItem('token') || ''),
                'Content-Type': 'application/json'
            },
            body: JSON.stringify({ target_ids: targetIds })
        });
        if (!response.ok) {
            const data = await response.json().catch(() => ({}));
            throw new Error(data.error || 'Hedefler silinemedi');
        }
    }

    function bindTagDeleteModal() {
        const modal = document.getElementById('tag-delete-modal');
        if (!modal || modal.dataset.bound === '1') return;

        modal.addEventListener('click', (event) => {
            if (event.target === modal) {
                modal.classList.remove('show');
            }
        });

        modal.dataset.bound = '1';
    }

    // Üç sonuçlu seçim modalı: 'move' | 'purge' | 'cancel'
    function showTagDeleteModal(folderName, targetCount) {
        bindTagDeleteModal();

        const modal = document.getElementById('tag-delete-modal');
        const titleEl = document.getElementById('tag-delete-title');
        const textEl = document.getElementById('tag-delete-text');
        const noteEl = document.getElementById('tag-delete-note');
        const cancelBtn = document.getElementById('tag-delete-cancel');
        const moveBtn = document.getElementById('tag-delete-move');
        const purgeBtn = document.getElementById('tag-delete-purge');

        if (!modal || !titleEl || !textEl || !cancelBtn || !moveBtn || !purgeBtn) {
            return Promise.resolve('cancel');
        }

        titleEl.textContent = `"${folderName}" etiketini sil`;
        textEl.textContent = `Bu etikette ${targetCount} hedef var. Etiketi silerken hedefleri ne yapmak istersiniz?`;
        if (noteEl) {
            noteEl.classList.remove('hidden');
            noteEl.textContent = '“Hedefleri de sil” seçeneği hedefleri kalıcı olarak kaldırır ve geri alınamaz. “Etiketsiz’e taşı” yalnızca etiketi kaldırır.';
        }

        modal.classList.add('show');

        return new Promise((resolve) => {
            const done = (result) => {
                cleanup();
                modal.classList.remove('show');
                resolve(result);
            };
            const onCancel = () => done('cancel');
            const onMove = () => done('move');
            const onPurge = () => done('purge');
            const onKeyDown = (event) => {
                if (event.key === 'Escape') {
                    event.preventDefault();
                    done('cancel');
                }
            };
            const cleanup = () => {
                cancelBtn.removeEventListener('click', onCancel);
                moveBtn.removeEventListener('click', onMove);
                purgeBtn.removeEventListener('click', onPurge);
                document.removeEventListener('keydown', onKeyDown);
            };

            cancelBtn.addEventListener('click', onCancel);
            moveBtn.addEventListener('click', onMove);
            purgeBtn.addEventListener('click', onPurge);
            document.addEventListener('keydown', onKeyDown);
        });
    }

    function updatePaginationText(total) {
        const text = `${total} kayıt`;
        const desktop = document.getElementById('pagination-info');
        const mobile = document.getElementById('pagination-info-mobile');
        if (desktop) desktop.textContent = text;
        if (mobile) mobile.textContent = text;
    }

    function hidePaginationControls() {
        ['prev-page-btn', 'next-page-btn', 'prev-page-btn-mobile', 'next-page-btn-mobile'].forEach((id) => {
            const btn = document.getElementById(id);
            if (btn) btn.classList.add('hidden');
        });
    }

    function showLoading() {
        const container = document.getElementById('folder-list-container');
        if (!container) return;
        container.innerHTML = '<div class="text-center py-10 text-sm text-gray-500 dark:text-gray-400">Hedefler yükleniyor...</div>';
    }

    function renderError(message) {
        const container = document.getElementById('folder-list-container');
        if (!container) return;
        container.innerHTML = `<div class="text-center py-10 text-sm text-red-600 dark:text-red-400">${escapeHtml(message)}</div>`;
        updateCounters();
        updatePaginationText(0);
    }

    function parseTags(value) {
        if (!value) return [];
        return String(value)
            .split(',')
            .map((tag) => tag.trim())
            .filter(Boolean);
    }

    function normalizeType(type) {
        const t = String(type || '').toLowerCase().trim();
        if (['icmp', 'ping', 'tcp'].includes(t)) return 'ping';
        if (t === 'https') return 'https';
        if (t === 'http') return 'http';
        return t || 'ping';
    }

    function getTypeBadgeClass(type) {
        switch (type) {
            case 'ping':
                return 'bg-green-100 dark:bg-green-900/20 text-green-800 dark:text-green-300';
            case 'https':
                return 'bg-indigo-100 dark:bg-indigo-900/20 text-indigo-800 dark:text-indigo-300';
            case 'http':
                return 'bg-purple-100 dark:bg-purple-900/20 text-purple-800 dark:text-purple-300';
            default:
                return 'bg-blue-100 dark:bg-blue-900/20 text-blue-800 dark:text-blue-300';
        }
    }

    function escapeHtml(text) {
        if (text == null) return '';
        const div = document.createElement('div');
        div.textContent = String(text);
        return div.innerHTML;
    }

    // ---- Toplu Düzenleme ----
    const bulkEditTagCache = { tags: [], loaded: false, loading: null };

    async function ensureBulkEditTagsLoaded() {
        if (bulkEditTagCache.loaded) return bulkEditTagCache.tags;
        if (bulkEditTagCache.loading) return bulkEditTagCache.loading;

        bulkEditTagCache.loading = (async () => {
            try {
                const response = await fetch('/api/targets/tags/available', {
                    headers: { 'Authorization': 'Bearer ' + (localStorage.getItem('token') || '') }
                });
                const data = await response.json().catch(() => ({}));
                const list = Array.isArray(data.tags) ? data.tags : [];
                bulkEditTagCache.tags = list
                    .map((t) => (t && typeof t === 'object' ? (t.name || '') : t))
                    .map((t) => String(t || '').trim())
                    .filter(Boolean);
                bulkEditTagCache.loaded = true;
            } catch (error) {
                bulkEditTagCache.tags = [];
            } finally {
                bulkEditTagCache.loading = null;
            }
            return bulkEditTagCache.tags;
        })();

        return bulkEditTagCache.loading;
    }

    function hideBulkEditSuggestions() {
        const box = document.getElementById('bulk-edit-tags-suggestions');
        if (box) box.classList.add('hidden');
    }

    function renderBulkEditSuggestions() {
        const input = document.getElementById('bulk-edit-tags');
        const box = document.getElementById('bulk-edit-tags-suggestions');
        if (!input || !box) return;
        if (input.disabled) { hideBulkEditSuggestions(); return; }

        const parts = String(input.value || '').split(',');
        const typing = parts[parts.length - 1].trim().toLowerCase();
        const committed = parts.slice(0, -1).map((t) => t.trim().toLowerCase()).filter(Boolean);

        const matches = bulkEditTagCache.tags.filter((tag) => {
            const tl = tag.toLowerCase();
            if (committed.includes(tl)) return false;
            return typing === '' ? true : tl.includes(typing);
        });

        if (matches.length === 0) {
            box.innerHTML = '<div class="bulk-edit-suggestion-empty">Kayıtlı etiket bulunamadı</div>';
            box.classList.remove('hidden');
            return;
        }

        box.innerHTML = matches
            .map((tag) => `<div class="bulk-edit-suggestion" data-tag="${escapeHtml(tag)}">${escapeHtml(tag)}</div>`)
            .join('');
        box.classList.remove('hidden');
    }

    function insertBulkEditTag(tag) {
        const input = document.getElementById('bulk-edit-tags');
        if (!input) return;
        const parts = String(input.value || '').split(',');
        const committed = parts.slice(0, -1).map((t) => t.trim()).filter(Boolean);
        committed.push(tag);
        input.value = committed.join(', ') + ', ';
        input.focus();
        renderBulkEditSuggestions();
    }

    function bindBulkEditTagAutocomplete() {
        const input = document.getElementById('bulk-edit-tags');
        const box = document.getElementById('bulk-edit-tags-suggestions');
        if (!input || !box || input.dataset.acBound === '1') return;

        input.addEventListener('focus', () => {
            if (input.disabled) return;
            ensureBulkEditTagsLoaded().then(() => renderBulkEditSuggestions());
        });
        input.addEventListener('input', () => {
            if (input.disabled) return;
            ensureBulkEditTagsLoaded().then(() => renderBulkEditSuggestions());
        });
        input.addEventListener('keydown', (event) => {
            if (event.key === 'Escape') {
                event.stopPropagation();
                hideBulkEditSuggestions();
            }
        });
        input.addEventListener('blur', () => {
            setTimeout(hideBulkEditSuggestions, 120);
        });
        // mousedown: blur'dan önce çalışsın ki seçim kaybolmasın.
        box.addEventListener('mousedown', (event) => {
            const item = event.target.closest('.bulk-edit-suggestion');
            if (!item) return;
            event.preventDefault();
            insertBulkEditTag(item.dataset.tag || '');
        });

        input.dataset.acBound = '1';
    }

    function bindBulkEditModal() {
        const modal = document.getElementById('bulk-edit-modal');
        if (!modal || modal.dataset.bound === '1') return;

        // Alan aç/kapa checkbox'ları ilgili kontrolü etkinleştirir/pasifleştirir.
        modal.querySelectorAll('.bulk-edit-enable').forEach((checkbox) => {
            checkbox.addEventListener('change', () => {
                const row = checkbox.closest('.bulk-edit-row');
                if (!row) return;
                const controls = row.querySelectorAll('.bulk-edit-control');
                controls.forEach((control) => { control.disabled = !checkbox.checked; });
                if (checkbox.checked && controls[0] && typeof controls[0].focus === 'function') {
                    controls[0].focus();
                } else if (!checkbox.checked && checkbox.dataset.field === 'tags') {
                    hideBulkEditSuggestions();
                }
            });
        });

        bindBulkEditTagAutocomplete();

        modal.addEventListener('click', (event) => {
            if (event.target === modal) {
                closeBulkEditModal();
            }
        });

        modal.dataset.bound = '1';
    }

    function resetBulkEditModal() {
        const modal = document.getElementById('bulk-edit-modal');
        if (!modal) return;
        modal.querySelectorAll('.bulk-edit-enable').forEach((checkbox) => {
            checkbox.checked = false;
        });
        modal.querySelectorAll('.bulk-edit-control').forEach((control) => {
            control.disabled = true;
        });
    }

    function openBulkEditModal() {
        bindBulkEditModal();
        const modal = document.getElementById('bulk-edit-modal');
        if (!modal) return;

        const ids = (typeof window.getSelectedTargets === 'function') ? window.getSelectedTargets() : [];
        if (!ids || ids.length === 0) {
            if (window.Toast && window.Toast.show) {
                window.Toast.show({
                    type: 'warning',
                    title: 'Uyarı',
                    message: 'Lütfen düzenlenecek hedefleri seçin.',
                    timeout: 3000
                });
            }
            return;
        }

        resetBulkEditModal();
        hideBulkEditSuggestions();
        ensureBulkEditTagsLoaded(); // öneriler için önden yükle
        const countEl = document.getElementById('bulk-edit-count');
        if (countEl) countEl.textContent = String(ids.length);
        modal.classList.add('show');
    }

    function closeBulkEditModal() {
        const modal = document.getElementById('bulk-edit-modal');
        if (modal) modal.classList.remove('show');
        hideBulkEditSuggestions();
    }

    async function submitBulkEdit() {
        const modal = document.getElementById('bulk-edit-modal');
        if (!modal) return;

        const ids = (typeof window.getSelectedTargets === 'function') ? window.getSelectedTargets() : [];
        if (!ids || ids.length === 0) {
            if (window.Toast && window.Toast.show) {
                window.Toast.show({ type: 'warning', title: 'Uyarı', message: 'Seçili hedef kalmadı.', timeout: 3000 });
            }
            return;
        }

        // Yinelenen ID'leri temizle (çoklu etiketli hedefler birden çok tabloda görünebilir).
        const uniqueIds = Array.from(new Set(ids));

        // SNMP/izleme alanları için bulk/update payload'ı, etiket için ayrı bulk/tags işlemi.
        const updatePayload = { target_ids: uniqueIds };
        let tagOp = null; // { operation, tags }

        modal.querySelectorAll('.bulk-edit-enable').forEach((checkbox) => {
            if (!checkbox.checked) return;
            const field = checkbox.dataset.field;
            const row = checkbox.closest('.bulk-edit-row');
            if (!field || !row) return;

            if (field === 'tags') {
                const opEl = row.querySelector('#bulk-edit-tag-operation');
                const tagsEl = row.querySelector('#bulk-edit-tags');
                tagOp = {
                    operation: opEl ? opEl.value : 'add',
                    tags: tagsEl ? tagsEl.value.trim() : ''
                };
                return;
            }

            const control = row.querySelector('.bulk-edit-control');
            if (!control) return;
            switch (field) {
                case 'metrics_enabled':
                    updatePayload.metrics_enabled = control.value === 'true';
                    break;
                case 'snmp_community':
                    updatePayload.snmp_community = control.value.trim();
                    break;
                case 'snmp_version':
                    updatePayload.snmp_version = control.value;
                    break;
                default:
                    break;
            }
        });

        const hasUpdateFields = Object.keys(updatePayload).some((key) => key !== 'target_ids');
        const hasTagOp = tagOp !== null;

        if (!hasUpdateFields && !hasTagOp) {
            if (window.Toast && window.Toast.show) {
                window.Toast.show({ type: 'warning', title: 'Uyarı', message: 'Değiştirmek için en az bir alan seçin.', timeout: 3000 });
            }
            return;
        }

        // Etiket işlemi doğrulaması: ekle/çıkar için etiket zorunlu (değiştir boş olabilir = tümünü temizle).
        if (hasTagOp && tagOp.operation !== 'replace' && !tagOp.tags) {
            if (window.Toast && window.Toast.show) {
                window.Toast.show({ type: 'warning', title: 'Uyarı', message: 'Etiket işlemi için bir etiket adı girin.', timeout: 3000 });
            }
            return;
        }

        const applyBtn = document.getElementById('bulk-edit-apply');
        const cancelBtn = document.getElementById('bulk-edit-cancel');
        const setBusy = (busy) => {
            if (applyBtn) {
                applyBtn.disabled = busy;
                applyBtn.textContent = busy ? 'Uygulanıyor...' : 'Uygula';
            }
            if (cancelBtn) cancelBtn.disabled = busy;
        };

        try {
            setBusy(true);

            if (hasUpdateFields) {
                const response = await fetch('/api/targets/bulk/update', {
                    method: 'POST',
                    headers: {
                        'Authorization': 'Bearer ' + (localStorage.getItem('token') || ''),
                        'Content-Type': 'application/json'
                    },
                    body: JSON.stringify(updatePayload)
                });
                const data = await response.json().catch(() => ({}));
                if (!response.ok) {
                    throw new Error(data.error || 'Hedefler güncellenemedi');
                }
            }

            if (hasTagOp) {
                await bulkUpdateTags(uniqueIds, tagOp.operation, tagOp.tags);
            }

            closeBulkEditModal();
            await loadAndRenderTargets();
            if (window.Toast && window.Toast.show) {
                window.Toast.show({
                    type: 'success',
                    title: 'Başarılı',
                    message: `${uniqueIds.length} hedef güncellendi.`,
                    timeout: 3000
                });
            }
        } catch (error) {
            if (window.Toast && window.Toast.show) {
                window.Toast.show({
                    type: 'error',
                    title: 'Hata',
                    message: error?.message || 'Hedefler güncellenemedi.',
                    timeout: 4000
                });
            }
        } finally {
            setBusy(false);
        }
    }

    window.bulkEditTargets = openBulkEditModal;
    window.closeBulkEditModal = closeBulkEditModal;
    window.submitBulkEdit = submitBulkEdit;

    window.TargetFolders = {
        init: initForCurrentMount,
        switchViewMode: function () {},
        openFolder: function (name) {
            if (!name) return;
            state.openFolders.add(name);
            renderAccordion();
        },
        goBackToFolders: function () {},
        getFilterableTargets: function () {
            return state.filteredTargets;
        },
        applyClientFilters: function (targets) {
            return Array.isArray(targets) ? targets : [];
        },
        resetBulkSelection: function () {},
        syncFolderBulkToolbar: syncFolderBulkToolbar,
        getState: function () {
            return {
                viewMode: 'folder',
                currentFolder: '__all_folders__',
                currentFolderTargets: null,
                allTargets: state.allTargets,
                folders: state.folders
            };
        }
    };

    // Route degisimi ile targets sayfasi tekrar yuklenebildigi icin periyodik mount kontrolu yapar.
    setInterval(initForCurrentMount, 300);

    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', initForCurrentMount);
    } else {
        setTimeout(initForCurrentMount, 50);
    }
})();
