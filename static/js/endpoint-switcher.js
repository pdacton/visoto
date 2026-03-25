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
            switchEndpoint(e.target.value);
        });
    }
});
