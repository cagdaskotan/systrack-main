// Filter Options Loader
// This script loads dynamic filter options for the targets page

function loadFilterOptions() {
    // Wait for elements to be available
    const typeFilter = document.getElementById('type-filter');
    const statusFilter = document.getElementById('status-filter');
    const tagFilter = document.getElementById('tag-filter');
    
    if (!typeFilter || !statusFilter || !tagFilter) {
        // Retry after 100ms if elements not found
        setTimeout(loadFilterOptions, 100);
        return;
    }
    
    console.log('Loading filter options...');
    
    fetch('/api/targets/filter-options')
    .then(response => {
        console.log('Filter options response status:', response.status);
        if (!response.ok) {
            throw new Error('Failed to load filter options: ' + response.status);
        }
        return response.json();
    })
    .then(data => {
        console.log('Filter options data:', data);
        if (data.success) {
            // Load types
            if (data.data.types) {
                data.data.types.forEach(type => {
                    const option = document.createElement('option');
                    option.value = type;
                    option.textContent = type.toUpperCase();
                    typeFilter.appendChild(option);
                });
            }
            
            // Load statuses
            if (data.data.statuses) {
                data.data.statuses.forEach(status => {
                    const option = document.createElement('option');
                    option.value = status;
                    option.textContent = status === 'online' ? 'Online' : 'Offline';
                    statusFilter.appendChild(option);
                });
            }
            
            // Load tags
            if (data.data.tags) {
                data.data.tags.forEach(tag => {
                    const option = document.createElement('option');
                    option.value = tag;
                    option.textContent = tag;
                    tagFilter.appendChild(option);
                });
            }
        }
    })
    .catch(error => {
        console.error('Error loading filter options:', error);
    });
}

// Start loading when DOM is ready
if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', loadFilterOptions);
} else {
    loadFilterOptions();
}