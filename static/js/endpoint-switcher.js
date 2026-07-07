// Endpoint switcher for multi-endpoint SPARQL support
// Manages endpoint selection using cookies

/**
 * Gets the currently selected endpoint name from cookie
 * @returns {string} The selected endpoint name, or empty string if none selected
 */
function getSelectedEndpoint() {
    const match = document.cookie.match(/selectedEndpoint=([^;]+)/);
    return match ? match[1] : '';
}

/**
 * Sets the selected endpoint name in a cookie
 * @param {string} name - The endpoint name to select, or empty/null to clear
 */
function setSelectedEndpoint(name) {
    if (name) {
        // Set cookie for 1 year with Lax SameSite policy. encodeURIComponent ensures non-ASCII
        // endpoint names (e.g. "Stadt Zürich") are safely stored and matched on the server side.
        document.cookie = `selectedEndpoint=${encodeURIComponent(name)}; path=/; max-age=31536000; SameSite=Lax`;
    } else {
        // Clear cookie
        document.cookie = 'selectedEndpoint=; path=/; max-age=0';
    }
}

/**
 * Switches to a new endpoint and reloads the page
 * @param {string} name - The endpoint name to switch to
 */
function switchEndpoint(name) {
    setSelectedEndpoint(name);
    location.reload();  // Reload to apply new endpoint
}

/**
 * Keeps the visible URL's ?endpoint= param in sync with the active endpoint's slug,
 * via history.replaceState (no navigation), so the address bar is always an accurate,
 * shareable snapshot even when the active endpoint came from a cookie/default rather
 * than an explicit query param. Removes a stale param if the active endpoint has no slug.
 * @param {HTMLSelectElement} endpointSelect - The endpoint <select> element
 */
function syncEndpointUrlParam(endpointSelect) {
    const option = endpointSelect.selectedOptions[0];
    const slug = option && option.dataset.slug;
    const url = new URL(window.location.href);
    if (slug) {
        if (url.searchParams.get('endpoint') === slug) return;
        url.searchParams.set('endpoint', slug);
    } else {
        if (!url.searchParams.has('endpoint')) return;
        url.searchParams.delete('endpoint');
    }
    history.replaceState(null, '', url);
}

// Update UI on page load
document.addEventListener('DOMContentLoaded', function() {
    const selected = getSelectedEndpoint();

    // Set the select element's value to match the cookie (decode since cookie is URI-encoded)
    const endpointSelect = document.getElementById('endpoint-selector');
    if (endpointSelect && selected) {
        endpointSelect.value = decodeURIComponent(selected);
    }

    // Add change event listener to trigger endpoint switch
    if (endpointSelect) {
        endpointSelect.addEventListener('change', function(e) {
            // Sync the URL to the new selection *before* reloading: otherwise the
            // reload re-requests the pre-switch ?endpoint= (still in the address bar
            // from the previous sync), which would take precedence over the
            // freshly-set cookie server-side and silently revert this switch.
            syncEndpointUrlParam(e.target);
            switchEndpoint(e.target.value);
        });

        syncEndpointUrlParam(endpointSelect);
    }
});
