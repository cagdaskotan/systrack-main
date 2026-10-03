// Toast.js - Toast notification system
class Toast {
    static show({ type = 'info', title = '', message = '', timeout = 5000 }) {
        const toast = document.createElement('div');
        toast.className = `toast toast-${type} transform transition-all duration-300 ease-in-out translate-x-full opacity-0`;
        
        const icons = {
            success: `<svg class="h-5 w-5 text-green-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z"></path>
                      </svg>`,
            error: `<svg class="h-5 w-5 text-red-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                     <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 8v4m0 4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"></path>
                   </svg>`,
            warning: `<svg class="h-5 w-5 text-yellow-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-2.5L13.732 4c-.77-.833-1.964-.833-2.732 0L3.732 16.5c-.77.833.192 2.5 1.732 2.5z"></path>
                      </svg>`,
            info: `<svg class="h-5 w-5 text-blue-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                     <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"></path>
                   </svg>`
        };

        toast.innerHTML = `
            <div class="bg-white dark:bg-gray-800 rounded-lg shadow-lg border border-gray-200 dark:border-gray-700 p-4 max-w-sm w-full">
                <div class="flex items-start">
                    <div class="flex-shrink-0">
                        ${icons[type] || icons.info}
                    </div>
                    <div class="ml-3 flex-1">
                        ${title ? `<h3 class="text-sm font-medium text-gray-900 dark:text-gray-100">${title}</h3>` : ''}
                        ${message ? `<p class="mt-1 text-sm text-gray-600 dark:text-gray-400">${message}</p>` : ''}
                    </div>
                    <div class="ml-4 flex-shrink-0">
                        <button class="toast-close bg-white dark:bg-gray-800 rounded-md inline-flex text-gray-400 hover:text-gray-500 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-primary-500">
                            <svg class="h-5 w-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"></path>
                            </svg>
                        </button>
                    </div>
                </div>
            </div>
        `;

        // Add to container
        let container = document.getElementById('toast-root');
        if (!container) {
            // Create toast container if it doesn't exist
            container = document.createElement('div');
            container.id = 'toast-root';
            container.className = 'fixed top-4 right-4 z-50 space-y-2';
            document.body.appendChild(container);
        }
        container.appendChild(toast);

        // Animate in
        requestAnimationFrame(() => {
            toast.classList.remove('translate-x-full', 'opacity-0');
        });

        // Close button
        const closeBtn = toast.querySelector('.toast-close');
        closeBtn.addEventListener('click', () => {
            this.close(toast);
        });

        // Auto close
        if (timeout > 0) {
            setTimeout(() => {
                this.close(toast);
            }, timeout);
        }

        return toast;
    }

    static close(toast) {
        toast.classList.add('translate-x-full', 'opacity-0');
        setTimeout(() => {
            if (toast.parentNode) {
                toast.parentNode.removeChild(toast);
            }
        }, 300);
    }

    static success(message, title = 'Başarılı') {
        return this.show({ type: 'success', title, message });
    }

    static error(message, title = 'Erişim Reddedildi') {
        return this.show({ type: 'error', title, message });
    }

    static warning(message, title = 'Uyarı') {
        return this.show({ type: 'warning', title, message });
    }

    static info(message, title = 'Bilgi') {
        return this.show({ type: 'info', title, message });
    }
}

// Make Toast available globally
window.Toast = Toast;
