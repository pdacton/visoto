// UI strings for the frontend: the JS-side counterpart of {{ t "key" }}.
//
// The active language's `js.*` catalog entries are rendered into a JSON island
// in <head> (see base.html and i18n.Catalogs.JSStrings). This file reads that
// island once and exposes window.vsT, so a script anywhere on the page can look
// a string up synchronously — no fetch, no async, no ordering rules beyond
// "loaded after <head>", which is every other script here.
(function () {
    'use strict';

    var strings = {};
    try {
        var el = document.getElementById('i18n-strings');
        if (el && el.textContent.trim()) {
            var parsed = JSON.parse(el.textContent);
            // html/template treats a <script> body as a JS context, so the JSON
            // the server rendered arrives escaped as a JSON *string* rather than
            // an object — the same double encoding the #resource-data island has
            // (see chat.js). Unwrap one level when that happened, but tolerate it
            // arriving as a plain object, so this keeps working either way.
            if (typeof parsed === 'string') {
                parsed = JSON.parse(parsed);
            }
            strings = parsed || {};
        }
    } catch (e) {
        // A malformed island must not take the page's JS down with it; every
        // lookup then falls back to the English default passed at the call site.
        console.warn('i18n: could not parse the string island', e);
    }

    /**
     * Looks up a UI string for the active language.
     *
     * `fallback` is required in spirit, and is always the English text: it keeps
     * each call site readable on its own, and means a key missing from the
     * catalog degrades to correct English rather than to a raw key.
     *
     * @param {string} key - Catalog key, including the "js." prefix
     * @param {string} [fallback] - English text to use when the key is absent
     * @returns {string} The translated string
     */
    window.vsT = function (key, fallback) {
        var v = strings[key];
        return (typeof v === 'string' && v !== '') ? v : (fallback !== undefined ? fallback : key);
    };

    /**
     * vsT with {placeholder} substitution, for the strings that interpolate a
     * count or a search term. Placeholders are named so a translation can move
     * them, which positional %s cannot express.
     *
     * @param {string} key - Catalog key
     * @param {string} fallback - English text, with the same placeholders
     * @param {Object} vars - Values keyed by placeholder name
     * @returns {string} The translated, interpolated string
     */
    window.vsTf = function (key, fallback, vars) {
        var s = window.vsT(key, fallback);
        return s.replace(/\{(\w+)\}/g, function (whole, name) {
            return Object.prototype.hasOwnProperty.call(vars || {}, name) ? vars[name] : whole;
        });
    };
})();
