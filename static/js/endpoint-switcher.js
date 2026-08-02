// Endpoint switcher — slug-only endpoint handling for multi-endpoint SPARQL support.
//
// Invariant (mirrors the server): the ?endpoint=<slug> URL param is the source
// of truth for the active endpoint. The selectedEndpoint cookie (also a slug)
// is only a saved preference that uncached entry pages (/, static pages,
// /search) honor server-side; routes in the shared Caddy/Souin cache
// (/resource, the /api fragments) are pure functions of the URL and never read
// it. The frontend's job is therefore to keep the slug in URLs: fragment
// requests (htmx hook below), in-app resource links (click rewriter +
// visotoResourceHref), and the address bar (replaceState sync).

function endpointSelectEl() {
    return document.getElementById('endpoint-selector');
}

/**
 * Validates a slug against the configured endpoints in the topbar select
 * (case-insensitive, like the server's GetEndpointBySlug) and returns the
 * canonical slug — so a stale bookmark's dead slug is never propagated into
 * fragment requests or the cookie.
 * @param {string} slug - Candidate slug
 * @returns {string} The canonical configured slug, or '' if unknown
 */
function knownEndpointSlug(slug) {
    const sel = endpointSelectEl();
    if (!slug || !sel) return '';
    const match = Array.prototype.find.call(sel.options,
        o => o.value.toLowerCase() === slug.toLowerCase());
    return match ? match.value : '';
}

/**
 * Resolves the active endpoint slug: URL param first (server truth for this
 * page), else the select (server-rendered `selected` on every page), else ''.
 * @returns {string} The active endpoint slug, or empty string
 */
function activeEndpointSlug() {
    const urlSlug = knownEndpointSlug(new URL(window.location.href).searchParams.get('endpoint'));
    if (urlSlug) return urlSlug;
    const sel = endpointSelectEl();
    return (sel && sel.value) || '';
}

function getEndpointCookie() {
    const match = document.cookie.match(/(?:^|;\s*)selectedEndpoint=([^;]+)/);
    return match ? match[1] : '';
}

/**
 * Stores the endpoint slug in the preference cookie (slugs are plain ASCII, so
 * no encoding is needed), or clears the cookie when passed a falsy value.
 * The cookie is client-managed only; the server never sets it.
 * @param {string} slug - The endpoint slug, or empty/null to clear
 */
function setEndpointCookie(slug) {
    if (slug) {
        document.cookie = `selectedEndpoint=${slug}; path=/; max-age=31536000; SameSite=Lax`;
    } else {
        document.cookie = 'selectedEndpoint=; path=/; max-age=0';
    }
}

/**
 * Builds a resource page URL carrying the active endpoint slug. Use this for
 * all JS-side navigation to /resource (graph views etc.); plain anchors are
 * covered by the delegated click rewriter below.
 * @param {string} iri - The resource IRI
 * @returns {string} /resource?iri=<escaped>&endpoint=<slug>
 */
function visotoResourceHref(iri) {
    let href = '/resource?iri=' + encodeURIComponent(iri);
    const slug = activeEndpointSlug();
    if (slug) href += '&endpoint=' + encodeURIComponent(slug);
    return href;
}

// Fold the active endpoint slug into every cacheable HTMX fragment request
// (/api/metric/*, /api/async-table/*) so the shared Caddy/Souin cache keys each
// endpoint's response separately — the server ignores the cookie on these
// routes. Registered on document so it covers even the first load-triggered
// hx-get on the page.
document.addEventListener('htmx:configRequest', function (evt) {
    const path = evt.detail.path || '';
    if (!/^\/api\/(metric|async-table)\//.test(path)) return;
    const slug = activeEndpointSlug();
    if (slug && evt.detail.parameters.endpoint === undefined) {
        evt.detail.parameters.endpoint = slug;
    }
});

// Rewrite in-app resource links to carry the active endpoint slug at click time
// (capture phase: the default action — including middle-/ctrl-click — reads the
// href after this runs). Server-rendered links are bare by design: /resource
// pages live in a shared URL-keyed cache, so their HTML cannot depend on who is
// viewing; the endpoint is attached client-side at navigation time instead.
document.addEventListener('click', function (evt) {
    const a = evt.target && evt.target.closest && evt.target.closest('a[href^="/resource?"]');
    if (!a) return;
    const url = new URL(a.getAttribute('href'), window.location.origin);
    if (url.searchParams.has('endpoint')) return;
    const slug = activeEndpointSlug();
    if (!slug) return;
    url.searchParams.set('endpoint', slug);
    a.setAttribute('href', url.pathname + url.search + url.hash);
}, true);

/**
 * Switches to a new endpoint and reloads the page
 * @param {string} slug - The endpoint slug to switch to
 */
function switchEndpoint(slug) {
    setEndpointCookie(slug);
    location.reload();  // Reload to apply new endpoint
}

/**
 * Keeps the visible URL's ?endpoint= param in sync with the select's slug via
 * history.replaceState (no navigation), so the address bar is always an
 * accurate, shareable snapshot of what the page is actually showing — this also
 * self-heals a stale bookmark's unknown slug to the actually-rendered endpoint.
 * @param {HTMLSelectElement} endpointSelect - The endpoint <select> element
 */
function syncEndpointUrlParam(endpointSelect) {
    const slug = endpointSelect.value;
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

document.addEventListener('DOMContentLoaded', function () {
    const endpointSelect = endpointSelectEl();
    if (!endpointSelect) return;

    // The server renders the correct <option selected> on every page, so no
    // client-side selection fixup is needed — only cookie upkeep: persist the
    // page's explicit URL slug as the preference (replacing the old server-side
    // Set-Cookie, which a shared cache must never emit), and drop stale or
    // legacy (pre-slug, name-valued) cookie values that match no endpoint.
    const urlSlug = knownEndpointSlug(new URL(window.location.href).searchParams.get('endpoint'));
    const cookieSlug = getEndpointCookie();
    if (urlSlug && urlSlug !== cookieSlug) {
        setEndpointCookie(urlSlug);
    } else if (cookieSlug && !knownEndpointSlug(cookieSlug)) {
        setEndpointCookie('');
    }

    // Narrow screens hide the <select> and show the same endpoints as entries in
    // the settings menu. Those entries are triggers only — they drive the select
    // and let its change handler below do the actual switching, so the select
    // stays the single source of truth for the active slug.
    document.querySelectorAll('.endpoint-menu-item').forEach(function (item) {
        item.addEventListener('click', function (e) {
            e.preventDefault();
            const slug = knownEndpointSlug(item.dataset.endpointSlug);
            if (!slug || slug === endpointSelect.value) return;
            endpointSelect.value = slug;
            endpointSelect.dispatchEvent(new Event('change'));
        });
    });

    endpointSelect.addEventListener('change', function (e) {
        // Sync the URL to the new selection *before* reloading: otherwise the
        // reload re-requests the pre-switch ?endpoint= (still in the address bar
        // from the previous sync), which would take precedence over the
        // freshly-set cookie server-side and silently revert this switch.
        syncEndpointUrlParam(e.target);
        switchEndpoint(e.target.value);
    });

    syncEndpointUrlParam(endpointSelect);
});
