# Remove `badgeVar` / `iconVar` from `sparqlTable` and `sparqlAsyncTable`

**Status:** Resolved (2026-08-17)
**Component:** `templates/partials/sparql-table.html`, `templates/partials/sparql-async-table.html`,
`cmd/visoto/main.go`, `cmd/visoto/async_index.go`, `cmd/visoto/faceted_table.go`,
`internal/column`, `internal/templates`, `static/js/sparql-table.js`, `static/js/faceted-table.js`
**Type:** cleanup / API narrowing

## Problem

Two ways exist to say the same thing about a column.

`<sparql-column var="…" icon>` and `<sparql-column var="…" badge>` fully cover
what the old `iconVar=` / `badgeVar=` dict params expressed, and the declaration
form is strictly better: it lives next to the column it describes, participates in
`order=`, and shares the scanner/validation path with `label`, `tip`, `filter`,
`path`, `type`, `group`, `hidden`, `width`.

The old params survive only as a shorthand for tables that declare no columns at
all. That shorthand costs a parallel plumbing path that has to be kept in sync:

- Template layer: `sparqlAsyncTable` appends `&badgeVar=` to the fragment URL;
  `sparqlTable` accepts `.iconVar` / `.badgeVar` in its dict.
- Handler layer: `/api/async-table`, `/api/faceted-table` read `badgeVar` from the
  query string and *also* fold `Table.IconVar()` / `Table.BadgeVar()` in from the
  declarations — with precedence rules ("the declaration wins") documented in prose
  in the partial header.
- Frontend: `sparql-table.js` reads `#<id>-icon-var` / `#<id>-badge-var` islands;
  `faceted-table.js` carries them in its `passthrough` set.
- `iconVar` is already *not* an authoring parameter — the partial header says so
  explicitly ("NOT an authoring parameter — declare it with `<sparql-column icon>`
  instead"), yet it still exists as a dict key, a URL param in `queryOptions`, and
  a `columnIconVar` fallback lookup.

So `iconVar` is pure internal wiring pretending to be a parameter, and `badgeVar`
is a duplicate authoring surface kept alive for eight call sites.

## Current call sites

`badgeVar=` is passed by hand in 8 templates:

| Template | Value |
|---|---|
| `templates/classes/fabio%3AJournalArticle.html:44` | `journal` |
| `templates/classes/dwcFP%3ATaxonConcept.html:49` | `rank` |
| `templates/classes/dwc%3AMaterialCitation.html:49` | `typeStatus` |
| `templates/classes/skos%3AConceptScheme.html:63` | `status` |
| `templates/classes/…dwcFP%23TaxonName.html:50` | `rank` |
| `templates/instances/…dwcFP%23TaxonName.html:37` | `rank` |
| `templates/instances/cube%3ACube.html:183` | `status` |
| `templates/instances/…vocab%23SharedDimensionTerm.html:162` | `status,superseded` |

No template passes `iconVar=` — it is entirely handler-internal already.

## Proposed change

1. Convert the 8 templates above to `<sparql-column var="…" badge>` inside their
   existing (or a new) `<sparql-columns for="…">` container. The two-value case
   (`"status,superseded"`) becomes two `badge` columns, which
   `Table.BadgeVar()` already supports.
2. Drop `.badgeVar` / `.iconVar` from both partials' dicts and headers, and stop
   emitting `&badgeVar=` in the async fragment URL.
3. Drop the `badgeVar` query-param reads in `cmd/visoto/main.go` and
   `cmd/visoto/faceted_table.go`, and the param-map writes in
   `cmd/visoto/async_index.go` — the handlers keep only the declaration-derived
   values.
4. Keep `Table.IconVar()` / `Table.BadgeVar()` in `internal/column`: they are the
   declaration accessors and stay the single source. Consider renaming them to
   `IconVars()` / `BadgeVars()` since both already return comma-separated lists,
   and the `…Var` name is what invites confusion with the removed params.
5. Keep the `#<id>-icon-var` / `#<id>-badge-var` template islands as the
   template→JS transport (they are not the authoring surface), but simplify
   `sparql-table.js`'s comment about "a hand-passed `iconVar=`/`badgeVar=` param",
   which no longer describes anything possible.
6. Simplify `columnIconVar` in `internal/templates`: with `.iconVar` gone, the
   sync path's `{{ if not $iconVar }}` fallback in `sparql-table.html:46-47`
   becomes the only path.

## Resolution

Done as proposed, with one addition the issue did not anticipate.

Two of the call sites are **sync** `sparqlTable` calls, and sync tables had no
badge equivalent of the `columnIconVar` lookup — the SharedDimensionTerm template
carried a comment saying exactly that, and passed `badgeVar` by hand *because* the
declarations it already had could not be read. Removing the param therefore
required adding that lookup, not just deleting plumbing. Rather than a near-copy
of `columnIconVar`, both roles now go through one `lookupVars(set, id, role)`
helper exposed as `{{ columnIconVars }}` / `{{ columnBadgeVars }}`, so a future
role is one line.

Two further call sites (`cube:Constraint`, `meta:SharedDimension`) appeared in
commits made after this issue was written, bringing the total to 10. Both already
declared `badge` on every column, so their params were pure duplication.

`Table.IconVar()`/`BadgeVar()` were renamed to `IconVars()`/`BadgeVars()`
(step 4), the config-island keys and the JS facet passthrough dropped both roles
(step 5), and `TestNoIconVarParamsRemain` — which despite its name only checked
`queryOptions` — became a real template scan that fails on any reintroduced
`"iconVar"`/`"badgeVar"` dict key, verified by reintroducing one.

## Out of scope

`groupBy` is the third shorthand of the same family (superseded by
`<sparql-column group>`) and has the same argument for removal, but the user's
request named only `badgeVar` / `iconVar`. Worth deciding together — removing
two of three leaves the remaining one looking arbitrary.

## Notes

- `internal/column/column_test.go` already pins the multi-value badge behaviour
  (`TestBadgeVarListsEveryBadgeColumn`, `TestBadgeVarEmptyWithoutBadges`) and
  `internal/templates/columns_test.go` pins `columnIconVar`; both should survive
  the change, adjusted for any rename.
- `cmd/visoto/icon_columns_test.go` covers the icon fold-in and is the regression
  guard for step 3.
- `docs/templating.md:360, 408, 415` documents both params and the
  shorthand-vs-declaration precedence rule; those rows and the paragraph go away
  with the params.
