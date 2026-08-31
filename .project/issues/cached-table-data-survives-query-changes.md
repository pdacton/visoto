# Cached table data survives a query change (stale column shape)

**Status:** Open
**Component:** async/working-set tables, faceted search
**Routes:** `/api/async-table-data/:id`, `/api/faceted-table/:id`, `/api/facet-values/:id/:var`

## Problem

The cacheable table routes are keyed purely on their URL (`id`, `iri`, `endpoint`,
facet params). The declared SPARQL query is *not* part of the key. So editing a
`<sparql-async>` query — in particular renaming a SELECT variable — changes the
shape of the response while the URL stays byte-identical.

Every client holding a cached copy keeps receiving the OLD column set for the full
`max-age=21600` (6 hours), and the shared Souin/CDN copy serves it to new clients too.

## Observed symptom

Renaming `?additionalType` → `?rechtsform` in
`templates/classes/schch%3AZefixOrganisation.html` produced a table whose columns
were still `additionalType`, combined with freshly-scanned facet specs naming
`rechtsform`. The result was a console warning and a facet with no header control:

```
facet "rechtsform" has no matching column — add ?rechtsform to the base query
to attach its header control.
```

The server was correct throughout — `/api/async-table-data` returned
`["organization","companyUID","rechtsform","addressLocality","description"]`. Only
the browser's cached copy was stale. Clearing the HTTP cache fixed it immediately.

This is easy to misdiagnose as a template or scan-cache bug: the JS is current
(it's a document subresource, so a hard reload refreshes it) while the data is
hours old.

## Partial mitigation already in place

`static/js/faceted-table.js` exposes `fetchOptions()`, which sets
`cache: "reload"` when `performance.getEntriesByType("navigation")[0].type ===
"reload"`. A reload (F5 or Ctrl-Shift-R) now refetches table data, faceted results
and facet values. Normal navigation still uses the cache fully.

That only helps the user who reloads. It does nothing for other users, and nothing
for the shared cache.

## Proposed fix

Fold a hash of the finalized declared query into the cache key, so a changed query
yields a different URL and stale entries are simply never consulted.

Sketch: compute a short hash (e.g. first 8 hex chars of SHA-256) of the declared
query at scan time in `scanTemplateElements` (`cmd/visoto/template_scan.go`),
expose it alongside the query from `findAsyncQuery`, emit it as a `&v=<hash>` param
on the fragment URL built in `templates/partials/sparql-async-table.html`, and have
the frontend propagate it to the data/facet fetches. The handlers can ignore the
value entirely — its only job is to participate in the cache key.

Alternative (weaker): lower `max-age` on these routes. Blunt, and it trades away
shared-cache efficiency for every user to fix an authoring-time problem.

## Notes

- Only affects deployments where templates change without a cache purge. Production
  already has a purge path (Settings → "Clear cache" / `POST /api/cache/purge`),
  so this mainly bites during development and immediately after a deploy.
- The 6h cache is deliberate and worth keeping: these are expensive SPARQL queries.
  The defect is the missing query dimension in the key, not the caching itself.
