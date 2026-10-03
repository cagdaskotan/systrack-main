// Modal.js - Modal system
class Modal {
    static open(content, options = {}) {
        const modal = document.createElement('div');
        modal.className = 'modal fixed inset-0 z-50 overflow-y-auto';
        modal.innerHTML = `
            <div class="flex items-center justify-center min-h-screen pt-4 px-4 pb-20 text-center sm:block sm:p-0">
                <div class="modal-backdrop fixed inset-0 bg-gray-500 bg-opacity-75 transition-opacity"></div>
                <span class="hidden sm:inline-block sm:align-middle sm:h-screen">&#8203;</span>
                <div class="modal-content inline-block align-bottom bg-white dark:bg-gray-800 rounded-lg text-left overflow-hidden shadow-xl transform transition-all sm:my-8 sm:align-middle sm:max-w-lg sm:w-full">
                    ${content}
                </div>
            </div>
        `;

        // Add to container
        const container = document.getElementById('modal-root');
        container.appendChild(modal);

        // Animate in
        requestAnimationFrame(() => {
            modal.classList.add('opacity-100');
            const content = modal.querySelector('.modal-content');
            content.classList.add('sm:scale-100');
        });

        // Close handlers
        const backdrop = modal.querySelector('.modal-backdrop');
        backdrop.addEventListener('click', () => {
            this.close(modal);
        });

        // ESC key handler
        const escHandler = (e) => {
            if (e.key === 'Escape') {
                this.close(modal);
                document.removeEventListener('keydown', escHandler);
            }
        };
        document.addEventListener('keydown', escHandler);

        // Store esc handler for cleanup
        modal._escHandler = escHandler;

        return modal;
    }

    static close(modal) {
        if (!modal || !modal.parentNode) return;

        // Remove esc handler
        if (modal._escHandler) {
            document.removeEventListener('keydown', modal._escHandler);
        }

        // Animate out
        modal.classList.add('opacity-0');
        const content = modal.querySelector('.modal-content');
        content.classList.add('sm:scale-95');

        setTimeout(() => {
            if (modal.parentNode) {
                modal.parentNode.removeChild(modal);
            }
        }, 300);
    }

    static confirm(message, title = 'Onay', options = {}) {
        return new Promise((resolve) => {
            const content = `
                <div class="bg-white dark:bg-gray-800 px-4 pt-5 pb-4 sm:p-6 sm:pb-4">
                    <div class="sm:flex sm:items-start">
                        <div class="mx-auto flex-shrink-0 flex items-center justify-center h-12 w-12 rounded-full bg-red-100 dark:bg-red-900/20 sm:mx-0 sm:h-10 sm:w-10">
                            <svg class="h-6 w-6 text-red-600 dark:text-red-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-2.5L13.732 4c-.77-.833-1.964-.833-2.732 0L3.732 16.5c-.77.833.192 2.5 1.732 2.5z"></path>
                            </svg>
                        </div>
                        <div class="mt-3 text-center sm:mt-0 sm:ml-4 sm:text-left">
                            <h3 class="text-lg leading-6 font-medium text-gray-900 dark:text-gray-100">${title}</h3>
                            <div class="mt-2">
                                <p class="text-sm text-gray-500 dark:text-gray-400">${message}</p>
                            </div>
                        </div>
                    </div>
                </div>
                <div class="bg-gray-50 dark:bg-gray-700 px-4 py-3 sm:px-6 sm:flex sm:flex-row-reverse">
                    <button type="button" class="modal-confirm w-full inline-flex justify-center rounded-md border border-transparent shadow-sm px-4 py-2 bg-red-600 text-base font-medium text-white hover:bg-red-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-red-500 sm:ml-3 sm:w-auto sm:text-sm">
                        Onayla
                    </button>
                    <button type="button" class="modal-cancel mt-3 w-full inline-flex justify-center rounded-md border border-gray-300 dark:border-gray-600 shadow-sm px-4 py-2 bg-white dark:bg-gray-800 text-base font-medium text-gray-700 dark:text-gray-300 hover:bg-gray-50 dark:hover:bg-gray-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-primary-500 sm:mt-0 sm:ml-3 sm:w-auto sm:text-sm">
                        İptal
                    </button>
                </div>
            `;

            const modal = this.open(content);

            // Event handlers
            modal.querySelector('.modal-confirm').addEventListener('click', () => {
                this.close(modal);
                resolve(true);
            });

            modal.querySelector('.modal-cancel').addEventListener('click', () => {
                this.close(modal);
                resolve(false);
            });
        });
    }

    static alert(message, title = 'Bilgi', options = {}) {
        return new Promise((resolve) => {
            const content = `
                <div class="bg-white dark:bg-gray-800 px-4 pt-5 pb-4 sm:p-6 sm:pb-4">
                    <div class="sm:flex sm:items-start">
                        <div class="mx-auto flex-shrink-0 flex items-center justify-center h-12 w-12 rounded-full bg-blue-100 dark:bg-blue-900/20 sm:mx-0 sm:h-10 sm:w-10">
                            <svg class="h-6 w-6 text-blue-600 dark:text-blue-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"></path>
                            </svg>
                        </div>
                        <div class="mt-3 text-center sm:mt-0 sm:ml-4 sm:text-left">
                            <h3 class="text-lg leading-6 font-medium text-gray-900 dark:text-gray-100">${title}</h3>
                            <div class="mt-2">
                                <p class="text-sm text-gray-500 dark:text-gray-400">${message}</p>
                            </div>
                        </div>
                    </div>
                </div>
                <div class="bg-gray-50 dark:bg-gray-700 px-4 py-3 sm:px-6 sm:flex sm:flex-row-reverse">
                    <button type="button" class="modal-ok w-full inline-flex justify-center rounded-md border border-transparent shadow-sm px-4 py-2 bg-primary-600 text-base font-medium text-white hover:bg-primary-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-primary-500 sm:w-auto sm:text-sm">
                        Tamam
                    </button>
                </div>
            `;

            const modal = this.open(content);

            modal.querySelector('.modal-ok').addEventListener('click', () => {
                this.close(modal);
                resolve();
            });
        });
    }
}

// Make Modal available globally
window.Modal = Modal;
