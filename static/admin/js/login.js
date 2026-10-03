function loginPage() {
    return {
        // State
        form: {
            email: '',
            password: '',
            remember: false
        },
        isLoading: false,
        errorMessage: '',
        isDark: localStorage.getItem('theme') === 'dark',

        // Initialize
        init() {
            this.checkExistingAuth();
            this.setupEventListeners();
        },

        // Check if user is already authenticated
        async checkExistingAuth() {
            const token = localStorage.getItem('token');
            if (!token) return;

            // Do not rely on browser clock for JWT expiry checks (device clock drift causes login loops).
            // Ask the server whether the token is accepted.
            try {
                const response = await fetch('/api/auth/me', {
                    headers: { 'Authorization': 'Bearer ' + token }
                });

                if (response.ok) {
                    window.location.href = '/';
                    return;
                }

                if (response.status === 401) {
                    localStorage.removeItem('token');
                    localStorage.removeItem('user');
                }
            } catch (error) {
                // Network errors: keep user on login page and allow manual login.
            }
        },

        // Check if token is expired
        isTokenExpired(token) {
            try {
                const payload = JSON.parse(atob(token.split('.')[1]));
                const currentTime = Math.floor(Date.now() / 1000);
                return payload.exp < currentTime;
            } catch (error) {
                return true;
            }
        },

        // Login function
        async login() {
            if (this.isLoading) return;

            this.isLoading = true;
            this.errorMessage = '';

            try {
                const response = await fetch('/api/auth/login', {
                    method: 'POST',
                    headers: {
                        'Content-Type': 'application/json',
                    },
                    body: JSON.stringify({
                        email: this.form.email,
                        password: this.form.password
                    })
                });

                const data = await response.json();

                if (response.ok) {
                    // Store token
                    localStorage.setItem('token', data.token);
                    
                    // Store user info
                    localStorage.setItem('user', JSON.stringify(data.user));

                    // Show success message
                    this.showToast('success', 'Başarıyla giriş yapıldı');

                    // Redirect to admin panel
                    setTimeout(() => {
                        window.location.href = '/';
                    }, 1000);
                } else {
                    this.errorMessage = data.error || 'Giriş yapılırken bir hata oluştu';
                }
            } catch (error) {
                console.error('Login error:', error);
                this.errorMessage = 'Bağlantı hatası. Lütfen tekrar deneyin.';
            } finally {
                this.isLoading = false;
            }
        },

        // Show toast notification
        showToast(type, message) {
            if (typeof Toast !== 'undefined') {
                Toast.show({ type, message });
            } else {
                console.log(`${type.toUpperCase()}: ${message}`);
            }
        },

        // Setup event listeners
        setupEventListeners() {
            // Enter key to submit form
            document.addEventListener('keydown', (e) => {
                if (e.key === 'Enter' && !this.isLoading) {
                    this.login();
                }
            });

            // Auto-fill demo credentials
            document.addEventListener('keydown', (e) => {
                if (e.ctrlKey && e.key === 'd') {
                    this.form.email = 'admin@systrack.local';
                    this.form.password = 'ChangeMeNow!';
                    this.showToast('info', 'Demo bilgileri dolduruldu');
                }
            });
        }
    };
}

// Initialize when DOM is loaded
document.addEventListener('DOMContentLoaded', function() {
    // Make loginPage globally available
    window.loginPage = loginPage();
    window.loginPage.init();
});
