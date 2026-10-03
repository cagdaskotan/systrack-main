// Filter Options Loader for Targets Page
(function () {
    const MAX_RETRY = 15;

    function clearSelectOptions(selectEl) {
        for (let i = selectEl.options.length - 1; i >= 1; i--) {
            selectEl.remove(i);
        }
    }

    function populateOptions(selectEl, values, textFormatter = (val) => val) {
        values.forEach((value) => {
            const option = document.createElement('option');
            option.value = value;
            option.textContent = textFormatter(value);
            selectEl.appendChild(option);
        });
    }

    async function fetchFilterOptions(attempt = 0) {
        const typeFilter = document.getElementById('type-filter');
        const statusFilter = document.getElementById('status-filter');
        const tagFilter = document.getElementById('tag-filter');

        if (!typeFilter || !statusFilter || !tagFilter) {
            if (attempt < MAX_RETRY) {
                setTimeout(() => fetchFilterOptions(attempt + 1), 200);
            }
            return;
        }

        try {
            const response = await fetch('/api/targets/filter-options');
            if (!response.ok) {
                throw new Error(`Failed to load filter options: ${response.status}`);
            }

            const data = await response.json();
            if (!(data.success && data.data)) {
                console.error('Filter options API returned unexpected payload:', data);
                return;
            }

            clearSelectOptions(typeFilter);
            clearSelectOptions(statusFilter);
            clearSelectOptions(tagFilter);

            if (Array.isArray(data.data.types)) {
                populateOptions(typeFilter, data.data.types, (t) => t.toUpperCase());
            }
            if (Array.isArray(data.data.statuses)) {
                populateOptions(statusFilter, data.data.statuses, (status) =>
                    status === 'online' ? 'Online' : 'Offline'
                );
            }
            if (Array.isArray(data.data.tags)) {
                populateOptions(tagFilter, data.data.tags);
            }
        } catch (error) {
            console.error('Error loading filter options:', error);
            if (attempt < MAX_RETRY) {
                setTimeout(() => fetchFilterOptions(attempt + 1), 500);
            }
        }
    }

    window.loadTargetFilterOptions = function () {
        fetchFilterOptions(0);
    };
    // Backward compatibility for older references
    window.loadFilterOptions = window.loadTargetFilterOptions;

    // Automatically load when navigating directly to /admin/targets
    document.addEventListener('DOMContentLoaded', () => {
        if (window.location.pathname.includes('/admin/targets') || window.location.hash === '#targets') {
            window.loadTargetFilterOptions();
        }
    });

    window.addEventListener('hashchange', () => {
        if (window.location.hash === '#targets') {
            setTimeout(() => window.loadTargetFilterOptions(), 100);
        }
    });
})();
