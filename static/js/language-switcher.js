// UI language switcher.
//
// Invariant (mirrors the server): the language reaches the server two different
// ways, because the two kinds of route resolve it differently.
//
// Pages take it from the site-lang cookie, which the client owns and the server
// never sets. Caddy normalizes that cookie and Accept-Language into the
// X-Site-Lang request header before the shared cache and folds it into the cache
// key there, so for pages the frontend's whole job is to write the cookie and
// reload. That reload is only correct because every page response carries
// `Cache-Control: max-age=0, must-revalidate` plus an ETag (see cmd/visoto/etag.go):
// a browser cannot see X-Site-Lang, so `Vary: X-Site-Lang` means nothing to its
// private cache, and without forced revalidation the reload would happily
// re-serve the previous language's page.
//
// /api/* fragment requests instead carry the code on the URL as `lang=`, exactly
// like the endpoint slug (see endpoint-switcher.js), because those routes sit in
// the shared cache and must be pure functions of their URL — reading the cookie
// there would serve one user's language to everyone. Attaching it is this file's
// other job: the htmx hook below covers fragment requests, while the plain
// fetch()es that htmx cannot see get it from the server-rendered config island
// (sparql-table.js) or from activeSiteLang() directly (faceted-table.js).

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
 * Resolves the language to send with /api/* requests: the URL param first (so a
 * hand-built or shared API URL is self-describing), else the server-rendered
 * <select>, whose value is the language this page was actually rendered in.
 *
 * Returns null — not '' — when nothing resolves, because '' is a real configured
 * code (the "no language" choice). Callers must test `!== null`, never
 * truthiness, or picking "None" would silently send no param at all and the
 * server would fall back to its default, which is a *different* language.
 *
 * Deliberately no page-URL counterpart to endpoint-switcher.js's replaceState
 * sync: pages resolve their language from the cookie, so a ?lang= in a page URL
 * would be a second source of truth the server ignores.
 * @returns {string|null} The active language code, or null
 */
function activeSiteLang() {
    const fromUrl = knownSiteLang(new URL(window.location.href).searchParams.get('lang'));
    if (fromUrl !== null) return fromUrl;
    const sel = siteLangSelectEl();
    return sel ? sel.value : null;
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

    // The visible control is a Tabler dropdown; the <select> is its hidden state
    // holder (see topbar.html). Clicking an item is what a change event would
    // have been, so it goes through the same switchSiteLang path.
    document.querySelectorAll('.site-lang-item').forEach(function (item) {
        item.addEventListener('click', function (e) {
            e.preventDefault();
            switchSiteLang(item.getAttribute('data-lang-code'));
        });
    });

    // Kept for the hidden <select>: nothing in the UI fires it today, but
    // activeSiteLang() treats the select as the source of truth, so anything that
    // sets its value programmatically should still take effect.
    sel.addEventListener('change', function (e) {
        switchSiteLang(e.target.value);
    });
}

// Fold the active language into every lazily-loaded fragment request, so the
// SPARQL data comes back in the picked language rather than the browser's. The
// server on these routes reads only this param — not the cookie, not
// Accept-Language — which is what lets the response be cached by URL alone.
//
// A separate listener from endpoint-switcher.js's rather than an edit to it:
// htmx fires every registered listener, so keeping each param with its own
// switcher avoids depending on which script tag parses first.
//
// The path test matches /api/async-table-data/ by prefix too, and covers the
// hand-written hx-get attributes in the theme pages without touching them.
document.addEventListener('htmx:configRequest', function (evt) {
    const path = evt.detail.path || '';
    if (!/^\/api\/(metric|async-table)\//.test(path)) return;
    const code = activeSiteLang();
    // `!== null`, not truthiness: '' is the real "no language" code and must be
    // sent as `lang=`, since an absent param means "use the server default".
    if (code !== null && evt.detail.parameters.lang === undefined) {
        evt.detail.parameters.lang = code;
    }
});

// Not latched at module level: HTMX swaps can re-run page scripts, and the tag
// sits above the CDN bundles, so both readiness paths have to be covered.
if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', boot);
} else {
    boot();
}
