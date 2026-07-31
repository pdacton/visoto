// UI language switcher.
//
// Invariant (mirrors the server): the site-lang cookie is the only thing the
// client owns. It never appears in URLs — unlike the endpoint slug, the language
// is normalized into the X-Site-Lang request header by Caddy before the shared
// cache, and folded into that cache's key there. So the frontend's whole job is
// to write the cookie and reload.
//
// That reload is only correct because every response carries
// `Cache-Control: max-age=0, must-revalidate` plus an ETag (see cmd/visoto/etag.go):
// a browser cannot see X-Site-Lang, so `Vary: X-Site-Lang` means nothing to its
// private cache, and without forced revalidation the reload would happily
// re-serve the previous language's page.

function siteLangSelectEl() {
    return document.getElementById('site-lang-selector');
}

/**
 * Validates a code against the options the server rendered, returning the
 * canonical value — so a stale cookie from a language that has since been
 * removed from visoto.config is never written back.
 * @param {string} code - Candidate language code ("" is a valid one)
 * @returns {string|null} The configured code, or null if unknown
 */
function knownSiteLang(code) {
    const sel = siteLangSelectEl();
    if (!sel || code === null || code === undefined) return null;
    const match = Array.prototype.find.call(sel.options, o => o.value === code);
    return match ? match.value : null;
}

/**
 * Reads the site-lang cookie.
 * @returns {string|null} The cookie value ("" when set but empty), or null when absent
 */
function getSiteLangCookie() {
    const match = document.cookie.match(/(?:^|;\s*)site-lang=([^;]*)/);
    return match ? decodeURIComponent(match[1]) : null;
}

/**
 * Stores the language preference. The empty string is a real choice ("no
 * language"), so it is written as an empty cookie rather than clearing it —
 * clearing would fall back to Accept-Language, which is a different outcome.
 * @param {string} code - The language code, or "" for the no-language choice
 */
function setSiteLangCookie(code) {
    document.cookie = `site-lang=${encodeURIComponent(code)}; path=/; max-age=31536000; SameSite=Lax`;
}

/**
 * Switches the UI language and reloads so the server re-renders in it.
 * @param {string} code - The language code to switch to
 */
function switchSiteLang(code) {
    setSiteLangCookie(code);
    location.reload();
}

function boot() {
    const sel = siteLangSelectEl();
    if (!sel) return;

    // Cookie upkeep: the server rendered the language it actually used, so adopt
    // that as the stored preference when the two disagree. This is what makes a
    // first visit (no cookie, language negotiated from Accept-Language) sticky,
    // and self-heals a cookie naming a language that no longer exists.
    const rendered = sel.value;
    const stored = getSiteLangCookie();
    if (stored === null || knownSiteLang(stored) === null) {
        setSiteLangCookie(rendered);
    }

    sel.addEventListener('change', function (e) {
        switchSiteLang(e.target.value);
    });
}

// Not latched at module level: HTMX swaps can re-run page scripts, and the tag
// sits above the CDN bundles, so both readiness paths have to be covered.
if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', boot);
} else {
    boot();
}
