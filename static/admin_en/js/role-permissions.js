function rolePermissionsPage() {
    return {
        // State
        isDark: localStorage.getItem('theme') === 'dark',
        userEmail: '',
        userInitials: '',
        selectedRole: '',
        rolePermissions: [],
        allPermissions: [],
        permissionCategories: {},
        roleDescription: '',
        roleSummary: null,
        loading: false,

        // Initialize
        init() {
            this.loadUserInfo();
            this.setupEventListeners();
            this.loadPermissionCategories();
            this.loadRoleSummary();
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

        // Open role modal
        openRoleModal() {
            this.showToast('info', 'Rol oluşturma özelliği yakında eklenecek');
        },

        // Load permission categories
        loadPermissionCategories() {
            const token = localStorage.getItem('token');
            if (!token) return;

            fetch('/api/permissions/categories', {
                headers: {
                    'Authorization': `Bearer ${token}`
                }
            })
            .then(response => response.json())
            .then(data => {
                if (data.categories) {
                    this.permissionCategories = data.categories;
                }
            })
            .catch(error => {
                console.error('Error loading permission categories:', error);
            });
        },

        // Load role summary
        loadRoleSummary() {
            const token = localStorage.getItem('token');
            if (!token) return;

            fetch('/api/permissions/summary', {
                headers: {
                    'Authorization': `Bearer ${token}`
                }
            })
            .then(response => response.json())
            .then(data => {
                if (data.role_summary) {
                    this.roleSummary = data.role_summary;
                }
            })
            .catch(error => {
                console.error('Error loading role summary:', error);
            });
        },

        // Load role details
        loadRoleDetails() {
            if (!this.selectedRole) {
                this.roleDescription = '';
                this.rolePermissions = [];
                return;
            }

            const token = localStorage.getItem('token');
            if (!token) return;

            // Load role description
            fetch(`/api/permissions/role/${this.selectedRole}/description`, {
                headers: {
                    'Authorization': `Bearer ${token}`
                }
            })
            .then(response => response.json())
            .then(data => {
                if (data.description) {
                    this.roleDescription = data.description;
                }
            })
            .catch(error => {
                console.error('Error loading role description:', error);
            });

            // Load role permissions
            fetch(`/api/permissions/role/${this.selectedRole}`, {
                headers: {
                    'Authorization': `Bearer ${token}`
                }
            })
            .then(response => response.json())
            .then(data => {
                if (data.permissions) {
                    this.rolePermissions = data.permissions;
                }
            })
            .catch(error => {
                console.error('Error loading role permissions:', error);
            });
        },

        // Get role display name
        getRoleDisplayName(role) {
            const names = {
                'admin': 'Admin',
                'user': 'Kullanıcı',
                'viewer': 'Görüntüleyici'
            };
            return names[role] || role;
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
            // Add any specific event listeners here
        }
    };
}

// Initialize when DOM is loaded
document.addEventListener('DOMContentLoaded', function() {
    // Make rolePermissionsPage globally available
    window.rolePermissionsPage = rolePermissionsPage();
    window.rolePermissionsPage.init();
});
