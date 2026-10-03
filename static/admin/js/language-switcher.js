/**
 * SysTrack Language Switcher
 * Handles language switching between Turkish, English and German
 * Stores user preference in localStorage
 */

(function() {
    'use strict';

    // Language configuration
    const LANGUAGES = {
        tr: {
            code: 'tr',
            name: 'Türkçe',
            flagClass: 'fi fi-tr',
            basePath: '/static/admin/'
        },
        en: {
            code: 'en',
            name: 'English',
            flagClass: 'fi fi-gb',
            basePath: '/static/admin_en/'
        },
        de: {
            code: 'de',
            name: 'Deutsch',
            flagClass: 'fi fi-de',
            basePath: '/static/admin_de/'
        }
    };

    const STORAGE_KEY = 'systrack_language';
    const DEFAULT_LANGUAGE = 'tr';

    /**
     * Get current language from localStorage
     */
    function getCurrentLanguage() {
        return localStorage.getItem(STORAGE_KEY) || DEFAULT_LANGUAGE;
    }

    /**
     * Set language preference
     */
    function setLanguage(langCode) {
        if (!LANGUAGES[langCode]) {
            console.error('Invalid language code:', langCode);
            return;
        }
        localStorage.setItem(STORAGE_KEY, langCode);
    }

    /**
     * Get current page name from URL
     */
    function getCurrentPageName() {
        const path = window.location.pathname;
        const filename = path.split('/').pop() || 'index.html';
        return filename;
    }

    /**
     * Detect which admin folder we're currently in
     */
    function getCurrentFolder() {
        const path = window.location.pathname;
        if (path.includes('/admin_en/')) {
            return 'en';
        }
        if (path.includes('/admin_de/')) {
            return 'de';
        }
        return 'tr';
    }

    /**
     * Switch to specified language
     */
    function switchLanguage(langCode) {
        if (!LANGUAGES[langCode]) {
            console.error('Invalid language code:', langCode);
            return;
        }

        // Save preference
        setLanguage(langCode);

        // Get current page
        const currentPage = getCurrentPageName();

        // Get current hash (route) if exists, or use last saved route
        let currentHash = window.location.hash || '';
        if (!currentHash) {
            const lastRoute = localStorage.getItem('systrack_last_route');
            if (lastRoute && lastRoute !== 'dashboard') {
                currentHash = '#' + lastRoute;
            }
        }

        // Build new URL with hash preserved
        const newPath = LANGUAGES[langCode].basePath + currentPage + currentHash;

        console.log('Switching language:', {
            from: getCurrentFolder(),
            to: langCode,
            currentPage: currentPage,
            currentHash: currentHash,
            newPath: newPath
        });

        // Redirect to new language version
        window.location.href = newPath;
    }

    /**
     * Update UI to show current language
     */
    function updateLanguageUI() {
        const currentLang = getCurrentFolder();
        const langData = LANGUAGES[currentLang];

        // Update flag display
        const flagElement = document.getElementById('current-flag');
        const langTextElement = document.getElementById('current-lang-text');

        if (flagElement) {
            flagElement.className = langData.flagClass + ' rounded shadow-sm';
            flagElement.style.width = '24px';
            flagElement.style.height = '18px';
            flagElement.style.display = 'inline-block';
        }

        if (langTextElement) {
            langTextElement.textContent = langData.code.toUpperCase();
        }
    }

    /**
     * Initialize language switcher on page load
     */
    function initLanguageSwitcher() {
        const savedLang = getCurrentLanguage();
        const currentFolder = getCurrentFolder();

        // If saved language doesn't match current folder, redirect
        if (savedLang !== currentFolder) {
            const currentPage = getCurrentPageName();
            const currentHash = window.location.hash || '';
            const correctPath = LANGUAGES[savedLang].basePath + currentPage + currentHash;
            window.location.href = correctPath;
            return;
        }

        // Update UI to reflect current language
        updateLanguageUI();

        console.log('Language Switcher initialized:', {
            saved: savedLang,
            current: currentFolder
        });
    }

    // Expose switchLanguage to global scope for onclick handlers
    window.switchLanguage = switchLanguage;

    // Initialize when DOM is ready
    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', initLanguageSwitcher);
    } else {
        initLanguageSwitcher();
    }
})();
