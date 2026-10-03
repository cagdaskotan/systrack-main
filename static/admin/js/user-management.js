function userManagementPage() {
    return {
        // State
        users: [],
        filteredUsers: [],
        searchQuery: '',
        roleFilter: '',
        sortBy: 'created_at',
        isDark: localStorage.getItem('theme') === 'dark',
        userEmail: '',
        userInitials: '',
        stats: {
            totalUsers: 0,
            adminUsers: 0,
            regularUsers: 0,
            viewerUsers: 0
        },

        listenersBound: false,
        // Initialize
        init() {
            if (!this.listenersBound) {
                this.setupEventListeners();
            }
            this.loadUserInfo();
            this.loadUsers();
            window.userManagementActions = this;
        },

        // Load current user info
        loadUserInfo() {
            const token = localStorage.getItem('token');
            if (!token) {
                window.location.href = '/login.html';
                return;
            }

            // Check if token is expired
            if (this.isTokenExpired(token)) {
                this.logout();
                return;
            }

            // Check if user has permission to access this page
            if (!this.hasPermission('user.view')) {
                this.showToast('error', 'Bu sayfaya erişim yetkiniz bulunmuyor');
                setTimeout(() => {
                    window.location.href = '/';
                }, 3000);
                return;
            }

            fetch('/api/auth/me', {
                headers: {
                    'Authorization': `Bearer ${token}`
                }
            })
            .then(response => {
                if (response.ok) {
                    return response.json();
                } else if (response.status === 401) {
                    // Token is invalid
                    this.logout();
                    return;
                } else {
                    throw new Error('Failed to load user info');
                }
            })
            .then(data => {
                if (data) {
                    this.userEmail = data.email;
                    this.userInitials = this.getInitials(data.email);
                }
            })
            .catch(error => {
                console.error('Error loading user info:', error);
                this.logout();
            });
        },

        // Load users from API
        loadUsers() {
            const token = localStorage.getItem('token');
            if (!token) {
                window.location.href = '/login.html';
                return;
            }

            // Check if token is expired
            if (this.isTokenExpired(token)) {
                this.logout();
                return;
            }

            this.showLoading();

            fetch('/api/users', {
                headers: {
                    'Authorization': `Bearer ${token}`
                }
            })
            .then(response => response.json())
            .then(data => {
                this.users = data.users || [];
                this.filteredUsers = [...this.users];
                this.updateStats();
                this.renderUsersTable();
                this.hideLoading();
            })
            .catch(error => {
                console.error('Error loading users:', error);
                this.hideLoading();
                this.showToast('error', 'Kullanıcılar yüklenirken hata oluştu');
            });
        },

        // Update statistics
        updateStats() {
            this.stats.totalUsers = this.users.length;
            this.stats.adminUsers = this.users.filter(u => u.role === 'admin').length;
            this.stats.regularUsers = this.users.filter(u => u.role === 'user').length;
            this.stats.viewerUsers = this.users.filter(u => u.role === 'viewer').length;
        },

        // Search users
        searchUsers() {
            this.filterUsers();
        },

        // Filter users
        filterUsers() {
            let filtered = [...this.users];

            // Search filter
            if (this.searchQuery) {
                const query = this.searchQuery.toLowerCase();
                filtered = filtered.filter(user => 
                    user.email.toLowerCase().includes(query)
                );
            }

            // Role filter
            if (this.roleFilter) {
                filtered = filtered.filter(user => user.role === this.roleFilter);
            }

            this.filteredUsers = filtered;
            this.sortUsers();
        },

        // Sort users
        sortUsers() {
            this.filteredUsers.sort((a, b) => {
                switch (this.sortBy) {
                    case 'email':
                        return a.email.localeCompare(b.email);
                    case 'role':
                        return a.role.localeCompare(b.role);
                    case 'created_at':
                    default:
                        return new Date(b.created_at) - new Date(a.created_at);
                }
            });
            this.renderUsersTable();
        },

        // Reset filters
        resetFilters() {
            this.searchQuery = '';
            this.roleFilter = '';
            this.sortBy = 'created_at';
            this.filterUsers();
        },

        // Render users table
        renderUsersTable() {
            const tbody = document.getElementById('users-table-body');
            const mobileList = document.getElementById('users-mobile-list');

            const emptyMessage = this.searchQuery || this.roleFilter ?
                'Filtrelere uygun kullanıcı bulunamadı.' :
                'Henüz kullanıcı bulunmuyor.';

            // Desktop tablo
            if (tbody) {
                if (this.filteredUsers.length === 0) {
                    tbody.innerHTML = `
                        <tr>
                            <td colspan="5" class="px-6 py-4 text-center text-gray-500 dark:text-gray-400">
                                ${emptyMessage}
                            </td>
                        </tr>
                    `;
                } else {
                    tbody.innerHTML = this.filteredUsers.map(user => this.renderUserRow(user)).join('');
                }
            }

            // Mobil liste
            if (mobileList) {
                if (this.filteredUsers.length === 0) {
                    mobileList.innerHTML = `
                        <div class="py-8 text-center text-sm text-gray-500 dark:text-gray-400">
                            ${emptyMessage}
                        </div>
                    `;
                } else {
                    mobileList.innerHTML = `
                        <div class="space-y-3">
                            ${this.filteredUsers.map(user => this.renderUserCard(user)).join('')}
                        </div>
                    `;
                }
            }
        },

        // Render user row
        renderUserRow(user) {
            const roleClass = this.getRoleClass(user.role);
            const roleText = this.getRoleText(user.role);
            const createdAt = this.formatDate(user.created_at);
            const isCurrentUser = user.email === this.userEmail;
            
            // Status badge
            const statusClass = user.is_active ? 
                'bg-green-100 dark:bg-green-900/20 text-green-800 dark:text-green-400' : 
                'bg-red-100 dark:bg-red-900/20 text-red-800 dark:text-red-400';
            const statusText = user.is_active ? 'Aktif' : 'Pasif';
            
            // License status
            let licenseStatus = '';
            if (user.license_expiry) {
                const expiryDate = new Date(user.license_expiry);
                const now = new Date();
                const diffTime = expiryDate - now;
                const diffDays = Math.ceil(diffTime / (1000 * 60 * 60 * 24));
                
                if (diffTime < 0) {
                    licenseStatus = '<span class="text-red-600 dark:text-red-400 text-xs">Süresi Dolmuş</span>';
                } else if (diffDays <= 7) {
                    licenseStatus = `<span class="text-yellow-600 dark:text-yellow-400 text-xs">${diffDays} gün kaldı</span>`;
                } else {
                    licenseStatus = `<span class="text-green-600 dark:text-green-400 text-xs">${expiryDate.toLocaleDateString('tr-TR')}</span>`;
                }
            } else {
                licenseStatus = '<span class="text-gray-500 dark:text-gray-400 text-xs">Sınırsız</span>';
            }

            return `
                <tr class="hover:bg-gray-50 dark:hover:bg-gray-700">
                    <td class="px-6 py-4 whitespace-nowrap">
                        <div class="flex items-center">
                            <div class="flex-shrink-0 h-10 w-10">
                                <div class="h-10 w-10 rounded-full bg-primary-500 flex items-center justify-center">
                                    <span class="text-sm font-medium text-white">${this.getInitials(user.email)}</span>
                                </div>
                            </div>
                            <div class="ml-4">
                                <div class="text-sm font-medium text-gray-900 dark:text-gray-100">${user.email}</div>
                                <div class="text-sm text-gray-500 dark:text-gray-400">ID: ${user.id} • Max: ${user.max_targets} hedef</div>
                                <div class="text-xs text-gray-400 dark:text-gray-500">Lisans: ${licenseStatus}</div>
                            </div>
                        </div>
                    </td>
                    <td class="px-6 py-4 whitespace-nowrap">
                        <span class="px-2 py-1 text-xs font-medium rounded-full ${roleClass}">${roleText}</span>
                    </td>
                    <td class="px-6 py-4 whitespace-nowrap text-sm text-gray-500 dark:text-gray-400">${createdAt}</td>
                    <td class="px-6 py-4 whitespace-nowrap">
                        <span class="px-2 py-1 text-xs font-medium rounded-full ${statusClass}">
                            ${statusText}
                        </span>
                    </td>
                    <td class="px-6 py-4 whitespace-nowrap text-sm font-medium">
                        <div class="flex items-center space-x-2">
                            <button onclick="userManagementActions.editUser(${user.id})" 
                                    class="text-primary-600 hover:text-primary-900 dark:text-primary-400 dark:hover:text-primary-300">
                                Düzenle
                            </button>
                            <span class="text-gray-300 dark:text-gray-600">|</span>
                            <button onclick="userManagementActions.resetPassword(${user.id})" 
                                    class="text-yellow-600 hover:text-yellow-900 dark:text-yellow-400 dark:hover:text-yellow-300">
                                Şifre Sıfırla
                            </button>
                            <span class="text-gray-300 dark:text-gray-600">|</span>
                            ${!isCurrentUser ? `
                                <button onclick="userManagementActions.deleteUser(${user.id})" 
                                        class="text-red-600 hover:text-red-900 dark:text-red-400 dark:hover:text-red-300">
                                    Sil
                                </button>
                            ` : `
                                <span class="text-gray-400 dark:text-gray-500">-</span>
                            `}
                        </div>
                    </td>
                </tr>
            `;
        },

        // Render user card for mobile
        renderUserCard(user) {
            const roleClass = this.getRoleClass(user.role);
            const roleText = this.getRoleText(user.role);
            const createdAt = this.formatDate(user.created_at);
            const isCurrentUser = user.email === this.userEmail;

            // Status badge
            const statusClass = user.is_active ?
                'bg-green-100 dark:bg-green-900/20 text-green-800 dark:text-green-400' :
                'bg-red-100 dark:bg-red-900/20 text-red-800 dark:text-red-400';
            const statusText = user.is_active ? 'Aktif' : 'Pasif';

            // License status
            let licenseStatus = '';
            if (user.license_expiry) {
                const expiryDate = new Date(user.license_expiry);
                const now = new Date();
                const diffTime = expiryDate - now;
                const diffDays = Math.ceil(diffTime / (1000 * 60 * 60 * 24));

                if (diffTime < 0) {
                    licenseStatus = '<span class="text-red-600 dark:text-red-400 text-xs">Süresi Dolmuş</span>';
                } else if (diffDays <= 7) {
                    licenseStatus = `<span class="text-yellow-600 dark:text-yellow-400 text-xs">${diffDays} gün kaldı</span>`;
                } else {
                    licenseStatus = `<span class="text-green-600 dark:text-green-400 text-xs">${expiryDate.toLocaleDateString('tr-TR')}</span>`;
                }
            } else {
                licenseStatus = '<span class="text-gray-500 dark:text-gray-400 text-xs">Sınırsız</span>';
            }

            return `
                <div class="bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-700 rounded-lg p-4">
                    <div class="flex items-start gap-3 mb-3">
                        <div class="flex-shrink-0 h-10 w-10">
                            <div class="h-10 w-10 rounded-full bg-primary-500 flex items-center justify-center">
                                <span class="text-sm font-medium text-white">${this.getInitials(user.email)}</span>
                            </div>
                        </div>
                        <div class="flex-1 min-w-0">
                            <p class="text-sm font-medium text-gray-900 dark:text-gray-100 truncate">${user.email}</p>
                            <p class="text-xs text-gray-500 dark:text-gray-400">ID: ${user.id}</p>
                            <p class="text-xs text-gray-500 dark:text-gray-400">${createdAt}</p>
                        </div>
                        <div class="flex flex-col gap-1 items-end">
                            <span class="px-2 py-1 text-xs font-medium rounded-full ${roleClass}">${roleText}</span>
                            <span class="px-2 py-1 text-xs font-medium rounded-full ${statusClass}">${statusText}</span>
                        </div>
                    </div>

                    <div class="space-y-2 pt-3 border-t border-gray-100 dark:border-gray-700">
                        <div class="flex items-center justify-between text-xs">
                            <span class="text-gray-500 dark:text-gray-400">Max Hedef:</span>
                            <span class="font-medium text-gray-900 dark:text-gray-100">${user.max_targets}</span>
                        </div>
                        <div class="flex items-center justify-between text-xs">
                            <span class="text-gray-500 dark:text-gray-400">Lisans:</span>
                            <span>${licenseStatus}</span>
                        </div>
                    </div>

                    <div class="flex items-center justify-end gap-3 pt-3 mt-3 border-t border-gray-100 dark:border-gray-700">
                        <button onclick="userManagementActions.editUser(${user.id})"
                                class="text-xs font-medium text-primary-600 hover:text-primary-700 dark:text-primary-400 dark:hover:text-primary-300">
                            Düzenle
                        </button>
                        <button onclick="userManagementActions.resetPassword(${user.id})"
                                class="text-xs font-medium text-yellow-600 hover:text-yellow-700 dark:text-yellow-400 dark:hover:text-yellow-300">
                            Şifre Sıfırla
                        </button>
                        ${!isCurrentUser ? `
                            <button onclick="userManagementActions.deleteUser(${user.id})"
                                    class="text-xs font-medium text-red-600 hover:text-red-700 dark:text-red-400 dark:hover:text-red-300">
                                Sil
                            </button>
                        ` : ''}
                    </div>
                </div>
            `;
        },

        // Get role class
        getRoleClass(role) {
            switch (role) {
                case 'admin':
                    return 'bg-red-100 dark:bg-red-900/20 text-red-800 dark:text-red-400';
                case 'user':
                    return 'bg-green-100 dark:bg-green-900/20 text-green-800 dark:text-green-400';
                case 'viewer':
                    return 'bg-yellow-100 dark:bg-yellow-900/20 text-yellow-800 dark:text-yellow-400';
                default:
                    return 'bg-gray-100 dark:bg-gray-900/20 text-gray-800 dark:text-gray-400';
            }
        },

        // Get role text
        getRoleText(role) {
            switch (role) {
                case 'admin':
                    return 'Admin';
                case 'user':
                    return 'Kullanıcı';
                case 'viewer':
                    return 'Görüntüleyici';
                default:
                    return 'Bilinmiyor';
            }
        },

        // Check if token is expired
        isTokenExpired(token) {
            try {
                const payloadPart = String(token || '').split('.')[1];
                if (!payloadPart) return true;

                // Do not rely on browser clock for exp. Only ensure token is parseable.
                const base64 = payloadPart.replace(/-/g, '+').replace(/_/g, '/');
                const padded = base64 + '='.repeat((4 - (base64.length % 4)) % 4);
                JSON.parse(atob(padded));
                return false;
            } catch (error) {
                return true;
            }
        },

        // Format date
        formatDate(dateString) {
            if (!dateString) return 'Bilinmiyor';
            
            // Handle different date formats
            let date;
            if (typeof dateString === 'string') {
                // Try to parse the date string
                date = new Date(dateString);
                // Check if date is valid
                if (isNaN(date.getTime())) {
                    // Try parsing as timestamp
                    const timestamp = parseInt(dateString);
                    if (!isNaN(timestamp)) {
                        date = new Date(timestamp * 1000); // Convert from seconds to milliseconds
                    } else {
                        return 'Geçersiz Tarih';
                    }
                }
            } else if (typeof dateString === 'number') {
                // Handle timestamp
                date = new Date(dateString * 1000);
            } else {
                date = new Date(dateString);
            }
            
            // Check if date is valid
            if (isNaN(date.getTime())) {
                return 'Geçersiz Tarih';
            }
            
            return date.toLocaleDateString('tr-TR', {
                year: 'numeric',
                month: 'long',
                day: 'numeric',
                hour: '2-digit',
                minute: '2-digit'
            });
        },

        // Format date for datetime-local input
        formatDateTimeLocal(dateString) {
            if (!dateString) return '';
            
            const date = new Date(dateString);
            if (isNaN(date.getTime())) return '';
            
            // Format as YYYY-MM-DDTHH:MM for datetime-local input
            const year = date.getFullYear();
            const month = String(date.getMonth() + 1).padStart(2, '0');
            const day = String(date.getDate()).padStart(2, '0');
            const hours = String(date.getHours()).padStart(2, '0');
            const minutes = String(date.getMinutes()).padStart(2, '0');
            
            return `${year}-${month}-${day}T${hours}:${minutes}`;
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
                    'analytics.view', 'sla.view'
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

            // Get user role from token
            const token = localStorage.getItem('token');
            if (!token) return false;
            
            try {
                const payload = JSON.parse(atob(token.split('.')[1]));
                const userRole = payload.role;
                const permissions = rolePermissions[userRole] || [];
                return permissions.includes(permission);
            } catch (error) {
                return false;
            }
        },

        // Get user initials
        getInitials(email) {
            return email.substring(0, 2).toUpperCase();
        },

        // Open user modal
        openUserModal(userId = null) {
            const modal = document.getElementById('modal-root');
            if (!modal) return;

            const isEdit = userId !== null;
            const user = isEdit ? this.users.find(u => u.id === userId) : null;

            modal.innerHTML = `
                <div class="fixed inset-0 bg-gray-600 bg-opacity-50 overflow-y-auto h-full w-full z-50" id="user-modal">
                    <div class="relative top-10 mx-auto p-5 border w-full max-w-md shadow-lg rounded-md bg-white dark:bg-gray-800">
                        <div class="mt-3">
                            <div class="flex items-center justify-between mb-4">
                                <h3 class="text-lg font-medium text-gray-900 dark:text-gray-100">
                                    ${isEdit ? 'Kullanıcı Düzenle' : 'Yeni Kullanıcı'}
                                </h3>
                                <button onclick="userManagementActions.closeUserModal()" class="text-gray-400 hover:text-gray-600 dark:hover:text-gray-300">
                                    <svg class="w-6 h-6" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"></path>
                                    </svg>
                                </button>
                            </div>
                            
                            <form id="user-form" class="space-y-4">
                                <input type="hidden" id="user-id" value="${userId || ''}">
                                
                                <div>
                                    <label class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">E-posta</label>
                                    <input type="email" id="user-email" required 
                                           class="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-lg bg-white dark:bg-gray-700 text-gray-900 dark:text-gray-100 focus:ring-2 focus:ring-primary-500 focus:border-primary-500"
                                           placeholder="kullanici@example.com" value="${user ? user.email : ''}">
                                </div>
                                
                                ${!isEdit ? `
                                    <div>
                                        <label class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">Şifre</label>
                                        <input type="password" id="user-password" required 
                                               class="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-lg bg-white dark:bg-gray-700 text-gray-900 dark:text-gray-100 focus:ring-2 focus:ring-primary-500 focus:border-primary-500"
                                               placeholder="En az 6 karakter">
                                    </div>
                                ` : ''}
                                
                                <div>
                                    <label class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">Rol</label>
                                    <select id="user-role" required 
                                            class="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-lg bg-white dark:bg-gray-700 text-gray-900 dark:text-gray-100 focus:ring-2 focus:ring-primary-500 focus:border-primary-500">
                                        <option value="viewer" ${user && user.role === 'viewer' ? 'selected' : ''}>Görüntüleyici</option>
                                        <option value="user" ${user && user.role === 'user' ? 'selected' : ''}>Kullanıcı</option>
                                        <option value="admin" ${user && user.role === 'admin' ? 'selected' : ''}>Admin</option>
                                    </select>
                                </div>
                                
                                <div>
                                    <label class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">Maksimum Hedef Sayısı</label>
                                    <input type="number" id="user-max-targets" min="1" max="1000"
                                           class="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-lg bg-white dark:bg-gray-700 text-gray-900 dark:text-gray-100 focus:ring-2 focus:ring-primary-500 focus:border-primary-500"
                                           placeholder="10" value="${user ? user.max_targets : ''}">
                                    <p class="text-xs text-gray-500 dark:text-gray-400 mt-1">Admin: 1000, Kullanıcı: 50, Görüntüleyici: 10</p>
                                </div>

                                <div>
                                    <label class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">Lisans Bitiş Tarihi</label>
                                    <input type="datetime-local" id="user-license-expiry" 
                                           class="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-lg bg-white dark:bg-gray-700 text-gray-900 dark:text-gray-100 focus:ring-2 focus:ring-primary-500 focus:border-primary-500"
                                           value="${user && user.license_expiry ? this.formatDateTimeLocal(user.license_expiry) : ''}">
                                    <p class="text-xs text-gray-500 dark:text-gray-400 mt-1">Kullanıcının lisans süresini belirleyin</p>
                                </div>
                                
                                ${isEdit ? `
                                    <div>
                                        <label class="flex items-center space-x-2">
                                            <input type="checkbox" id="user-is-active" 
                                                   class="rounded border-gray-300 text-primary-600 focus:ring-primary-500"
                                                   ${user && user.is_active ? 'checked' : ''}>
                                            <span class="text-sm font-medium text-gray-700 dark:text-gray-300">Hesap Aktif</span>
                                        </label>
                                        <p class="text-xs text-gray-500 dark:text-gray-400 mt-1">Kullanıcının sisteme giriş yapabilmesi için gerekli</p>
                                    </div>
                                ` : ''}
                                
                                <div class="flex justify-end space-x-3 pt-4">
                                    <button type="button" onclick="userManagementActions.closeUserModal()" 
                                            class="px-4 py-2 text-sm font-medium text-gray-700 dark:text-gray-300 bg-gray-100 dark:bg-gray-600 hover:bg-gray-200 dark:hover:bg-gray-500 rounded-lg transition-colors">
                                        İptal
                                    </button>
                                    <button type="submit" 
                                            class="px-4 py-2 text-sm font-medium text-white bg-primary-600 hover:bg-primary-700 rounded-lg transition-colors">
                                        ${isEdit ? 'Güncelle' : 'Ekle'}
                                    </button>
                                </div>
                            </form>
                        </div>
                    </div>
                </div>
            `;

            // Add form submit handler
            document.getElementById('user-form').addEventListener('submit', (e) => {
                e.preventDefault();
                if (isEdit) {
                    this.updateUser(userId);
                } else {
                    this.createUser();
                }
            });
        },

        // Close user modal
        closeUserModal() {
            const modal = document.getElementById('modal-root');
            if (modal) {
                modal.innerHTML = '';
            }
        },

        // Create user
        createUser() {
            const email = document.getElementById('user-email').value;
            const password = document.getElementById('user-password').value;
            const role = document.getElementById('user-role').value;
            const maxTargets = parseInt(document.getElementById('user-max-targets').value) || 0;
            const licenseExpiry = document.getElementById('user-license-expiry').value;

            const token = localStorage.getItem('token');
            if (!token) return;

            this.showLoading();

            const requestData = {
                email,
                password,
                role,
                max_targets: maxTargets
            };

            // Add license expiry if provided
            if (licenseExpiry) {
                requestData.license_expiry = new Date(licenseExpiry).toISOString();
            }

            fetch('/api/users', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                    'Authorization': `Bearer ${token}`
                },
                body: JSON.stringify(requestData)
            })
            .then(response => response.json())
            .then(data => {
                this.hideLoading();
                if (data.error) {
                    this.showToast('error', data.error);
                } else {
                    this.showToast('success', 'Kullanıcı başarıyla oluşturuldu');
                    this.closeUserModal();
                    this.loadUsers();
                }
            })
            .catch(error => {
                this.hideLoading();
                console.error('Error creating user:', error);
                this.showToast('error', 'Kullanıcı oluşturulurken hata oluştu');
            });
        },

        // Edit user
        editUser(userId) {
            this.openUserModal(userId);
        },

        // Update user
        updateUser(userId) {
            const email = document.getElementById('user-email').value;
            const role = document.getElementById('user-role').value;
            const maxTargets = parseInt(document.getElementById('user-max-targets').value) || 0;
            const licenseExpiry = document.getElementById('user-license-expiry').value;
            const isActive = document.getElementById('user-is-active').checked;

            const token = localStorage.getItem('token');
            if (!token) return;

            this.showLoading();

            const requestData = {
                email,
                role,
                max_targets: maxTargets,
                is_active: isActive
            };

            // Add license expiry if provided
            if (licenseExpiry) {
                requestData.license_expiry = new Date(licenseExpiry).toISOString();
            }

            fetch(`/api/users/${userId}`, {
                method: 'PUT',
                headers: {
                    'Content-Type': 'application/json',
                    'Authorization': `Bearer ${token}`
                },
                body: JSON.stringify(requestData)
            })
            .then(response => response.json())
            .then(data => {
                this.hideLoading();
                if (data.error) {
                    this.showToast('error', data.error);
                } else {
                    this.showToast('success', 'Kullanıcı başarıyla güncellendi');
                    this.closeUserModal();
                    this.loadUsers();
                }
            })
            .catch(error => {
                this.hideLoading();
                console.error('Error updating user:', error);
                this.showToast('error', 'Kullanıcı güncellenirken hata oluştu');
            });
        },

        // Delete user
        deleteUser(userId) {
            if (!confirm('Bu kullanıcıyı silmek istediğinizden emin misiniz?')) {
                return;
            }

            const token = localStorage.getItem('token');
            if (!token) return;

            this.showLoading();

            fetch(`/api/users/${userId}`, {
                method: 'DELETE',
                headers: {
                    'Authorization': `Bearer ${token}`
                }
            })
            .then(response => response.json())
            .then(data => {
                this.hideLoading();
                if (data.error) {
                    this.showToast('error', data.error);
                } else {
                    this.showToast('success', 'Kullanıcı başarıyla silindi');
                    this.loadUsers();
                }
            })
            .catch(error => {
                this.hideLoading();
                console.error('Error deleting user:', error);
                this.showToast('error', 'Kullanıcı silinirken hata oluştu');
            });
        },

        // Reset user password
        resetPassword(userId) {
            const user = this.users.find(u => u.id === userId);
            if (!user) return;

            const modal = document.getElementById('modal-root');
            if (!modal) return;

            modal.innerHTML = `
                <div class="fixed inset-0 bg-gray-600 bg-opacity-50 overflow-y-auto h-full w-full z-50" id="reset-password-modal">
                    <div class="relative top-10 mx-auto p-5 border w-full max-w-md shadow-lg rounded-md bg-white dark:bg-gray-800">
                        <div class="mt-3">
                            <div class="flex items-center justify-between mb-4">
                                <h3 class="text-lg font-medium text-gray-900 dark:text-gray-100">
                                    Şifre Sıfırla
                                </h3>
                                <button onclick="userManagementActions.closeResetPasswordModal()" class="text-gray-400 hover:text-gray-600 dark:hover:text-gray-300">
                                    <svg class="w-6 h-6" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"></path>
                                    </svg>
                                </button>
                            </div>
                            
                            <div class="mb-4 p-3 bg-blue-50 dark:bg-blue-900/20 rounded-lg">
                                <p class="text-sm text-blue-800 dark:text-blue-200">
                                    <strong>${user.email}</strong> kullanıcısının şifresini sıfırlıyorsunuz.
                                </p>
                            </div>
                            
                            <form id="reset-password-form" class="space-y-4">
                                <div>
                                    <label class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">Yeni Şifre</label>
                                    <input type="password" id="new-password" required minlength="6"
                                           class="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-lg bg-white dark:bg-gray-700 text-gray-900 dark:text-gray-100 focus:ring-2 focus:ring-primary-500 focus:border-primary-500"
                                           placeholder="En az 6 karakter">
                                </div>
                                
                                <div>
                                    <label class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">Şifre Tekrar</label>
                                    <input type="password" id="confirm-password" required minlength="6"
                                           class="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-lg bg-white dark:bg-gray-700 text-gray-900 dark:text-gray-100 focus:ring-2 focus:ring-primary-500 focus:border-primary-500"
                                           placeholder="Şifreyi tekrar girin">
                                </div>
                                
                                <div class="flex justify-end space-x-3 pt-4">
                                    <button type="button" onclick="userManagementActions.closeResetPasswordModal()" 
                                            class="px-4 py-2 text-sm font-medium text-gray-700 dark:text-gray-300 bg-gray-100 dark:bg-gray-600 hover:bg-gray-200 dark:hover:bg-gray-500 rounded-lg transition-colors">
                                        İptal
                                    </button>
                                    <button type="submit" 
                                            class="px-4 py-2 text-sm font-medium text-white bg-yellow-600 hover:bg-yellow-700 rounded-lg transition-colors">
                                        Şifre Sıfırla
                                    </button>
                                </div>
                            </form>
                        </div>
                    </div>
                </div>
            `;

            // Add form submit handler
            document.getElementById('reset-password-form').addEventListener('submit', (e) => {
                e.preventDefault();
                this.submitPasswordReset(userId);
            });
        },

        // Close reset password modal
        closeResetPasswordModal() {
            const modal = document.getElementById('modal-root');
            if (modal) {
                modal.innerHTML = '';
            }
        },

        // Submit password reset
        submitPasswordReset(userId) {
            const newPassword = document.getElementById('new-password').value;
            const confirmPassword = document.getElementById('confirm-password').value;

            if (newPassword !== confirmPassword) {
                this.showToast('error', 'Şifreler eşleşmiyor');
                return;
            }

            if (newPassword.length < 6) {
                this.showToast('error', 'Şifre en az 6 karakter olmalıdır');
                return;
            }

            const token = localStorage.getItem('token');
            if (!token) return;

            this.showLoading();

            fetch(`/api/users/${userId}/reset-password`, {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                    'Authorization': `Bearer ${token}`
                },
                body: JSON.stringify({ new_password: newPassword })
            })
            .then(response => response.json())
            .then(data => {
                this.hideLoading();
                if (data.error) {
                    this.showToast('error', data.error);
                } else {
                    this.showToast('success', 'Şifre başarıyla sıfırlandı');
                    this.closeResetPasswordModal();
                }
            })
            .catch(error => {
                this.hideLoading();
                this.showToast('error', 'Şifre sıfırlanırken hata oluştu');
                console.error('Error:', error);
            });
        },

        // Toggle theme
        toggleTheme() {
            this.isDark = !this.isDark;
            if (this.isDark) {
                document.documentElement.classList.add('dark');
                localStorage.setItem('theme', 'dark');
            } else {
                document.documentElement.classList.remove('dark');
                localStorage.setItem('theme', 'light');
            }
        },

        // Logout
        logout() {
            localStorage.removeItem('token');
            localStorage.removeItem('user');
            window.location.href = '/login.html';
        },

        // Show loading
        showLoading() {
            const overlay = document.getElementById('loading-overlay');
            if (overlay) {
                overlay.classList.remove('hidden');
            }
        },

        // Hide loading
        hideLoading() {
            const overlay = document.getElementById('loading-overlay');
            if (overlay) {
                overlay.classList.add('hidden');
            }
        },

        // Show toast
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

        // Setup event listeners
        setupEventListeners() {
            if (this.listenersBound) {
                return;
            }

            // Close modal on escape key
            document.addEventListener('keydown', (e) => {
                if (e.key === 'Escape') {
                    this.closeUserModal();
                }
            });

            // Close modal on backdrop click
            document.addEventListener('click', (e) => {
                if (e.target.id === 'user-modal') {
                    this.closeUserModal();
                }
            });

            this.listenersBound = true;
        }
    };
}


