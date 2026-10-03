function modulePermissionsPage() {
    return {
        // State
        user: null,
        users: [],
        selectedUserId: '',
        selectedUserRole: '',
        availableModules: [],
        modulePermissions: {},
        loading: false,
        isDark: false,

        // Initialize
        async init() {
            this.loadTheme();
            await this.loadUserInfo();

            if (!this.user || this.user.role !== 'admin') {
                return;
            }

            await this.loadUsers();
            await this.loadAvailableModules();

            if (this.selectedUserId) {
                await this.loadUserPermissions(this.selectedUserId);
            }
        },

        // Theme management
        loadTheme() {
            this.isDark = localStorage.getItem('theme') === 'dark' || 
                         (!localStorage.getItem('theme') && window.matchMedia('(prefers-color-scheme: dark)').matches);
            this.applyTheme();
        },

        toggleTheme() {
            this.isDark = !this.isDark;
            this.applyTheme();
        },

        applyTheme() {
            if (this.isDark) {
                document.documentElement.classList.add('dark');
                localStorage.setItem('theme', 'dark');
            } else {
                document.documentElement.classList.remove('dark');
                localStorage.setItem('theme', 'light');
            }
        },

        // User info
        async loadUserInfo() {
            const token = localStorage.getItem('token');
            if (!token) {
                this.redirectToLogin();
                return;
            }

            if (this.isTokenExpired(token)) {
                this.logout();
                return;
            }

            try {
                const response = await fetch('/api/auth/me', {
                    headers: {
                        'Authorization': `Bearer ${token}`
                    }
                });

                if (response.status === 401) {
                    this.redirectToLogin();
                    return;
                }

                if (response.ok) {
                    const data = await response.json();
                    this.user = data; // API returns user object directly, not wrapped in 'user' property
                    
                    // Check if user has permission to view this page (only admin can access)
                    if (!this.user || this.user.role !== 'admin') {
                        this.showToast('error', 'Sie haben keine Berechtigung, auf diese Seite zuzugreifen.');
                        setTimeout(() => {
                            window.location.href = '/';
                        }, 3000);
                        return;
                    }
                } else {
                    this.showToast('error', 'Benutzerinformationen konnten nicht abgerufen werden.');
                    setTimeout(() => {
                        window.location.href = '/';
                    }, 3000);
                    return;
                }
            } catch (error) {
                console.error('Error loading user info:', error);
                this.showToast('error', 'Benutzerinformationen konnten nicht geladen werden.');
                setTimeout(() => {
                    window.location.href = '/';
                }, 3000);
            }
        },

        // Load users
        async loadUsers() {
            try {
                const response = await fetch('/api/users', {
                    headers: {
                        'Authorization': `Bearer ${localStorage.getItem('token')}`
                    }
                });

                if (response.ok) {
                    const data = await response.json();
                    this.users = data.users || [];
                }
            } catch (error) {
                console.error('Error loading users:', error);
                this.showToast('error', 'Benutzer konnten nicht geladen werden.');
            }
        },

        // Load available modules
        async loadAvailableModules() {
            try {
                const response = await fetch('/api/module-permissions/modules', {
                    headers: {
                        'Authorization': `Bearer ${localStorage.getItem('token')}`
                    }
                });

                if (response.ok) {
                    const data = await response.json();
                    this.availableModules = data.modules || [];
                }
            } catch (error) {
                console.error('Error loading modules:', error);
                this.showToast('error', 'Module konnten nicht geladen werden.');
            }
        },

        // Load user permissions
        async loadUserPermissions() {
            if (!this.selectedUserId) {
                this.modulePermissions = {};
                this.selectedUserRole = '';
                return;
            }

            // Get selected user's role
            const selectedUser = this.users.find(u => u.id == this.selectedUserId);
            this.selectedUserRole = selectedUser ? selectedUser.role : '';

            try {
                const response = await fetch(`/api/module-permissions/user/${this.selectedUserId}`, {
                    headers: {
                        'Authorization': `Bearer ${localStorage.getItem('token')}`
                    }
                });

                if (response.ok) {
                    const data = await response.json();
                    console.log('Module permissions API response:', data); // Debug log
                    this.modulePermissions = {};

                    // Convert array to object for easier access
                    data.permissions.forEach(perm => {
                        if (!this.modulePermissions[perm.module_name]) {
                            this.modulePermissions[perm.module_name] = {};
                        }
                        this.modulePermissions[perm.module_name].view = perm.can_view;
                        this.modulePermissions[perm.module_name].edit = perm.can_edit;
                    });
                    console.log('Module permissions object:', this.modulePermissions); // Debug log
                } else {
                    console.error('Failed to load user permissions:', response.status, response.statusText);
                }
            } catch (error) {
                console.error('Error loading user permissions:', error);
                this.showToast('error', 'Benutzerberechtigungen konnten nicht geladen werden.');
            }
        },

        // Get module permission
        getModulePermission(moduleName, permissionType) {
            console.log(`Getting permission for ${moduleName}.${permissionType}:`, this.modulePermissions[moduleName]); // Debug log
            if (!this.modulePermissions[moduleName]) {
                return false;
            }
            return this.modulePermissions[moduleName][permissionType] || false;
        },

        // Update module permission
        updateModulePermission(moduleName, permissionType, value) {
            if (!this.modulePermissions[moduleName]) {
                this.modulePermissions[moduleName] = {};
            }
            this.modulePermissions[moduleName][permissionType] = value;
        },

        // Save permissions
        async savePermissions() {
            if (!this.selectedUserId) {
                this.showToast('error', 'Bitte wählen Sie einen Benutzer aus.');
                return;
            }

            this.loading = true;

            try {
                // Convert permissions to array format
                const permissions = this.availableModules.map(module => ({
                    module_name: module.name,
                    can_view: this.getModulePermission(module.name, 'view'),
                    can_edit: this.getModulePermission(module.name, 'edit')
                }));

                const response = await fetch(`/api/module-permissions/user/${this.selectedUserId}`, {
                    method: 'PUT',
                    headers: {
                        'Authorization': `Bearer ${localStorage.getItem('token')}`,
                        'Content-Type': 'application/json'
                    },
                    body: JSON.stringify({ permissions })
                });

                if (response.ok) {
                    this.showToast('success', 'Modulberechtigungen wurden erfolgreich aktualisiert.');
                } else {
                    const error = await response.json();
                    throw new Error(error.error || 'Berechtigungen konnten nicht aktualisiert werden.');
                }
            } catch (error) {
                console.error('Error saving permissions:', error);
                this.showToast('error', error.message || 'Berechtigungen konnten nicht gespeichert werden.');
            } finally {
                this.loading = false;
            }
        },

        // Utility functions
        isTokenExpired(token) {
            try {
                const payloadPart = String(token || '').split('.')[1];
                if (!payloadPart) return true;

                // Do not rely on browser clock for exp. Only ensure token is parseable.
                const base64 = payloadPart.replace(/-/g, '+').replace(/_/g, '/');
                const padded = base64 + '='.repeat((4 - (base64.length % 4)) % 4);
                JSON.parse(atob(padded));
                return false;
            } catch {
                return true;
            }
        },

        hasPermission(permission) {
            if (!this.user) return false;
            
            // Admin has all permissions
            if (this.user.role === 'admin') return true;
            
            const rolePermissions = {
                'user': ['target.view', 'target.ping', 'alert.view', 'sla.view'],
                'viewer': ['target.view', 'alert.view', 'sla.view']
            };
            
            return rolePermissions[this.user.role]?.includes(permission) || false;
        },

        logout() {
            localStorage.removeItem('token');
            localStorage.removeItem('user');
            window.location.href = '/login.html';
        },

        redirectToLogin() {
            window.location.href = '/login.html';
        },

        showToast(type, message) {
            if (type === 'error') {
                Toast.error(message);
            } else {
                Toast.show({ type, message });
            }
        },

        // Check if module is locked (cannot be changed)
        isModuleLocked(moduleName) {
            // Admin: All modules are locked (cannot be changed)
            if (this.selectedUserRole === 'admin') {
                return true;
            }

            // User/Viewer: Dashboard is always locked
            if (moduleName === 'dashboard') {
                return true;
            }

            return false;
        },

        // Check if checkbox should be disabled
        isCheckboxDisabled(moduleName, permissionType) {
            // Admin: All checkboxes are disabled
            if (this.selectedUserRole === 'admin') {
                return true;
            }

            // Dashboard is always locked for user/viewer
            if (moduleName === 'dashboard') {
                return true;
            }

            // Viewer: Edit checkboxes are always disabled
            if (this.selectedUserRole === 'viewer' && permissionType === 'edit') {
                return true;
            }

            return false;
        },

        // Get role text for display
        getRoleText(role) {
            const roleMap = {
                'admin': 'Administrator',
                'user': 'Benutzer',
                'viewer': 'Betrachter'
            };
            return roleMap[role] || role;
        }
    };
}
