// Template-set propagation — the scope an async SPARQL query id is resolved in.
//
// A <sparql-async id="…"> declaration is visible to exactly one template set:
// the page (or class/instance) template plus the layouts, partials and
// referenced components it is parsed with. Ids therefore only have to be unique
// within a set, and a fragment request has to say which set it came from or the
// server cannot tell two same-named queries apart.
//
// The set name is rendered into <head> by base.html from {{ templateSet }},
// which Load binds per set — so this value is fixed by the same thing that chose
// the markup, and cannot drift from it. It is a property of the PAGE, not of any
// fragment, which is why (unlike the endpoint slug and the language) it is never
// echoed through a fragment's config island: code running inside a swapped-in
// fragment reads the same page-level value.
//
// Two transports, matching how endpoint and language already travel:
//   - HTMX requests   → the htmx:configRequest hook below
//   - plain fetch()es → activeTemplateSet(), read directly by sparql-table.js
//                       and faceted-table.js when they build their URLs

/**
 * The template set this page was rendered from, e.g. "pages/plazi.html".
 * @returns {string} The set name, or '' if the page did not render the meta tag
 */
function activeTemplateSet() {
    const el = document.querySelector('meta[name="vs-template-set"]');
    return (el && el.getAttribute('content')) || '';
}

// Fold the set name into every cacheable HTMX fragment request. Registered on
// document, like the endpoint and language hooks, so it also covers the first
// load-triggered hx-get on the page.
//
// A hook rather than a template change on purpose: it reaches the hx-get
// attributes written by the partials, the ones hand-written in the theme pages,
// the ones embedded in translated strings in locales/*.toml, and the ones whose
// id is built at render time (/api/metric/{{ .metricId }}) — none of which a
// static rewrite could have covered.
//
// A separate listener from the other two rather than an edit to them: htmx fires
// every registered listener, so keeping each param with its own script avoids
// depending on which tag parses first.
document.addEventListener('htmx:configRequest', function (evt) {
    const path = evt.detail.path || '';
    if (!/^\/api\/(metric|async-table)\//.test(path)) return;
    if (evt.detail.parameters.src === undefined) {
        evt.detail.parameters.src = activeTemplateSet();
    }
});
