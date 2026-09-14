# `search.js` calls a bare `bootstrap` global that does not exist

**Status:** Open
**Component:** search page (`static/js/search.js`)
**Routes:** `/search`

## Problem

[search.js:57](static/js/search.js#L57) constructs a collapse via the bare global:

```js
const bsCollapse = new bootstrap.Collapse(filterContent, { toggle: true });
```

There is no `window.bootstrap` on this site. Tabler bundles Bootstrap and exposes
it as `window.tabler.bootstrap` — which is exactly what the rest of the codebase
already does ([page-init.js:24](static/js/page-init.js#L24),
[darkmode.js:40](static/js/darkmode.js#L40)), and what the comments in
[sparql-graph.js:458](static/js/sparql-graph.js#L458) and
[schema-graph.js:550](static/js/schema-graph.js#L550) explicitly warn about:
"NOT window.bootstrap — that global does not exist."

Verified in the browser:

```
typeof window.bootstrap                  -> "undefined"
new bootstrap.Collapse(el, {...})        -> ReferenceError: bootstrap is not defined
window.tabler.bootstrap.Collapse         -> present
```

## Reachability

Narrow. The branch runs only on `/search` when the page loads with a **filter
already selected** (`class-filter` or `property-filter` non-empty) — i.e. arriving
at a search URL that carries a `class=` or `property=` value matching an option in
the select. On that path the handler throws a `ReferenceError` and the advanced-filter
accordion does not auto-expand: the user sees a filter applied to their results with
the filter panel collapsed, giving no visual indication of why the result set is
narrowed. Everything else on the page is unaffected, since the throw is at the end of
the `DOMContentLoaded` handler.

## Not caused by the Tabler 1.5.1 bump

Pre-existing since 4d8af01 (2026-01-16). `window.bootstrap` was already absent under
Tabler 1.4 — 1.5 bundling Bootstrap does not change this. Found while checking the
codebase against the Tabler 1.5 upgrade guide, which calls out
`window.bootstrap.X` → `window.tabler.X` as a migration step.

## Fix

One line, matching the existing convention:

```js
const bsCollapse = new window.tabler.bootstrap.Collapse(filterContent, { toggle: true });
```

Guard it the way `darkmode.js` does if the defensive style is preferred. Worth
testing by loading `/search` with a `class=` value that exists in the dropdown and
confirming the accordion expands.

Note the upgrade guide also offers `window.tabler.Collapse` (unnamespaced) as the
1.5 form. This codebase consistently uses `window.tabler.bootstrap.*`, which works
in both 1.4 and 1.5 — prefer it for consistency.
