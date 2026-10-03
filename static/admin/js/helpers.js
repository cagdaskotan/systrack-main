// Global debug flag (default: off)
window.SYSTRACK_DEBUG = window.SYSTRACK_DEBUG ?? false;

if (!window.SYSTRACK_DEBUG) {
    console.log = () => {};
    console.info = () => {};
    console.warn = () => {};
}
// console.error intentionally left enabled

// Helpers.js - Utility functions
class Helpers {
    // Query string utilities
    static getQueryParam(name) {
        const urlParams = new URLSearchParams(window.location.search);
        return urlParams.get(name);
    }

    static setQueryParam(name, value) {
        const url = new URL(window.location);
        url.searchParams.set(name, value);
        window.history.replaceState({}, '', url);
    }

    static removeQueryParam(name) {
        const url = new URL(window.location);
        url.searchParams.delete(name);
        window.history.replaceState({}, '', url);
    }

    // Debounce function
    static debounce(func, wait, immediate) {
        let timeout;
        return function executedFunction(...args) {
            const later = () => {
                timeout = null;
                if (!immediate) func(...args);
            };
            const callNow = immediate && !timeout;
            clearTimeout(timeout);
            timeout = setTimeout(later, wait);
            if (callNow) func(...args);
        };
    }

    // Throttle function
    static throttle(func, limit) {
        let inThrottle;
        return function(...args) {
            if (!inThrottle) {
                func.apply(this, args);
                inThrottle = true;
                setTimeout(() => inThrottle = false, limit);
            }
        };
    }

    // Format date/time
    static formatDate(date, options = {}) {
        const defaultOptions = {
            year: 'numeric',
            month: 'short',
            day: 'numeric',
            hour: '2-digit',
            minute: '2-digit'
        };
        return new Intl.DateTimeFormat('tr-TR', { ...defaultOptions, ...options }).format(new Date(date));
    }

    static formatRelativeTime(date) {
        const now = new Date();
        const diff = now - new Date(date);
        const seconds = Math.floor(diff / 1000);
        const minutes = Math.floor(seconds / 60);
        const hours = Math.floor(minutes / 60);
        const days = Math.floor(hours / 24);

        if (days > 0) return `${days} gün önce`;
        if (hours > 0) return `${hours} saat önce`;
        if (minutes > 0) return `${minutes} dakika önce`;
        return 'Az önce';
    }

    // Format numbers
    static formatNumber(num, decimals = 0) {
        return new Intl.NumberFormat('tr-TR', {
            minimumFractionDigits: decimals,
            maximumFractionDigits: decimals
        }).format(num);
    }

    static formatBytes(bytes, decimals = 2) {
        if (bytes === 0) return '0 Bytes';
        const k = 1024;
        const dm = decimals < 0 ? 0 : decimals;
        const sizes = ['Bytes', 'KB', 'MB', 'GB', 'TB', 'PB', 'EB', 'ZB', 'YB'];
        const i = Math.floor(Math.log(bytes) / Math.log(k));
        return parseFloat((bytes / Math.pow(k, i)).toFixed(dm)) + ' ' + sizes[i];
    }

    // DOM utilities
    static $(selector) {
        return document.querySelector(selector);
    }

    static $$(selector) {
        return document.querySelectorAll(selector);
    }

    static createElement(tag, className = '', innerHTML = '') {
        const element = document.createElement(tag);
        if (className) element.className = className;
        if (innerHTML) element.innerHTML = innerHTML;
        return element;
    }

    // Local storage utilities
    static setStorage(key, value) {
        try {
            localStorage.setItem(key, JSON.stringify(value));
        } catch (e) {
            console.error('Failed to save to localStorage:', e);
        }
    }

    static getStorage(key, defaultValue = null) {
        try {
            const item = localStorage.getItem(key);
            return item ? JSON.parse(item) : defaultValue;
        } catch (e) {
            console.error('Failed to read from localStorage:', e);
            return defaultValue;
        }
    }

    static removeStorage(key) {
        try {
            localStorage.removeItem(key);
        } catch (e) {
            console.error('Failed to remove from localStorage:', e);
        }
    }

    // HTTP utilities
    static async fetchJSON(url, options = {}) {
        const defaultOptions = {
            headers: {
                'Content-Type': 'application/json',
                'Authorization': `Bearer ${localStorage.getItem('token')}`
            }
        };

        const response = await fetch(url, { ...defaultOptions, ...options });
        
        if (!response.ok) {
            throw new Error(`HTTP error! status: ${response.status}`);
        }

        return await response.json();
    }

    static async postJSON(url, data) {
        return this.fetchJSON(url, {
            method: 'POST',
            body: JSON.stringify(data)
        });
    }

    static async putJSON(url, data) {
        return this.fetchJSON(url, {
            method: 'PUT',
            body: JSON.stringify(data)
        });
    }

    static async deleteJSON(url) {
        return this.fetchJSON(url, {
            method: 'DELETE'
        });
    }

    // Validation utilities
    static isValidEmail(email) {
        const re = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;
        return re.test(email);
    }

    static isValidURL(string) {
        try {
            new URL(string);
            return true;
        } catch (_) {
            return false;
        }
    }

    static isValidIP(ip) {
        const re = /^(?:(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.){3}(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)$/;
        return re.test(ip);
    }

    // Color utilities
    static getStatusColor(status) {
        const colors = {
            'online': 'text-green-500',
            'offline': 'text-red-500',
            'unknown': 'text-gray-500',
            'warning': 'text-yellow-500',
            'error': 'text-red-500',
            'success': 'text-green-500',
            'info': 'text-blue-500'
        };
        return colors[status] || 'text-gray-500';
    }

    static getStatusBgColor(status) {
        const colors = {
            'online': 'bg-green-100 dark:bg-green-900/20',
            'offline': 'bg-red-100 dark:bg-red-900/20',
            'unknown': 'bg-gray-100 dark:bg-gray-900/20',
            'warning': 'bg-yellow-100 dark:bg-yellow-900/20',
            'error': 'bg-red-100 dark:bg-red-900/20',
            'success': 'bg-green-100 dark:bg-green-900/20',
            'info': 'bg-blue-100 dark:bg-blue-900/20'
        };
        return colors[status] || 'bg-gray-100 dark:bg-gray-900/20';
    }

    // Animation utilities
    static fadeIn(element, duration = 300) {
        element.style.opacity = '0';
        element.style.display = 'block';
        
        let start = performance.now();
        
        function animate(time) {
            let elapsed = time - start;
            let progress = Math.min(elapsed / duration, 1);
            
            element.style.opacity = progress;
            
            if (progress < 1) {
                requestAnimationFrame(animate);
            }
        }
        
        requestAnimationFrame(animate);
    }

    static fadeOut(element, duration = 300) {
        let start = performance.now();
        let initialOpacity = parseFloat(getComputedStyle(element).opacity);
        
        function animate(time) {
            let elapsed = time - start;
            let progress = Math.min(elapsed / duration, 1);
            
            element.style.opacity = initialOpacity * (1 - progress);
            
            if (progress < 1) {
                requestAnimationFrame(animate);
            } else {
                element.style.display = 'none';
            }
        }
        
        requestAnimationFrame(animate);
    }

    // Copy to clipboard
    static async copyToClipboard(text) {
        try {
            await navigator.clipboard.writeText(text);
            Toast.success('Panoya kopyalandı');
        } catch (err) {
            // Fallback for older browsers
            const textArea = document.createElement('textarea');
            textArea.value = text;
            document.body.appendChild(textArea);
            textArea.focus();
            textArea.select();
            try {
                document.execCommand('copy');
                Toast.success('Panoya kopyalandı');
            } catch (err) {
                Toast.error('Kopyalama başarısız');
            }
            document.body.removeChild(textArea);
        }
    }
}

// Make Helpers available globally
window.Helpers = Helpers;
