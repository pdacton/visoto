# Lazy Tree — Implementation Plan

Companion to `lazytreerequirements.md`. Requirement IDs (LT-n) cite that document.
Written after reading the existing machinery; the notes below record what the code
actually does, since several requirements assume a shape it does not have yet.

---

## 0. What the code already gives us

Verified, not assumed:

| Need | Exists | Where |
|---|---|---|
| Set-scoped declaration index | ✔ | `cmd/visoto/async_index.go` — `initAsyncIndex`, `parseDecls`, `findAsyncQuery` |
| Duplicate-id startup failure naming both files | ✔ | `async_index.go` ~L95 |
| IRI validation before splicing | ✔ | `sparql.ValidateIRI` / `IRITerm` (`internal/sparql/terms.go`) |
| `??` substitution, validated | ✔ | `sparql.SubstituteEntity` |
| Safe string literal for search terms | ✔ | `sparql.StringLiteral` (`paging.go:101`) |
| Variable-name validation | ✔ | `sparql.ValidateVarName` |
| Pure-URL cache headers | ✔ | `markURLPure` / `markCacheable` (`cmd/visoto/etag.go`) |
| Bounded per-process cache | ✔ | `sweepExpired`, `maxCacheEntries` (`cmd/visoto/cache_bound.go`) |
| Route middleware for endpoint + lang | ✔ | `epFromURL`, `langFromURL` (`main.go:870-875`) |
| Query execution + label enrichment | ✔ | `Preprocessor.ExecuteQueryWithContext` (`query.go:398`) |
| Wunderbaum render/columns/link logic | ✔ | `static/js/sparql-tree.js` — lift, don't rewrite |

**Two gaps the requirements did not anticipate:**

1. **The parser cannot see the new elements.** `extractElements`
   (`internal/parser/template.go:28`) matches a hardcoded tag list —
   `sparql-query`, `sparql-table`, `sparql-tree`, `sparql-async`, `sparql-facet`.
   `<sparql-tree-queries>` and `<sparql-tree-query>` are invisible to it. Worse,
   `parseElement` calls `extractTextContent(n)`, which would flatten a container's
   children into one blob. So this needs a **dedicated extractor** that walks
   containers and reads their element children individually — the shape
   `ExtractColumnContainers` already uses, not the `extractElements` path.

2. **`?parent` is a *reserved* name, and nothing enforces that today.** The
   children query is written with `?parent` free, to be bound by a prepended
   `VALUES`. Startup must reject a role query that *also* binds `?parent` in a
   pattern, or the `VALUES` silently intersects instead of parameterising.

One thing to preserve: `sparql-tree` is already in the tag whitelist and four
instance templates use the `sparqlTree` partial
(`skos%3AConcept`, `skos%3AConceptScheme`, `meta%3AHierarchy`,
`…%23SharedDimensionTerm`). None of them change.

---

## 1. Phase 1 — Declaration, index, roots/children, navigator

Delivers LT-1…LT-5, LT-7…LT-20, LT-25…LT-28, LT-33…LT-39. This is the phase that
removes the timeout and page-weight problems; everything after it is ergonomics.

### 1.1 Parser — `internal/parser/tree.go` (new)

```go
type TreeQueries struct {
    ID    string            // the for= value
    Roles map[string]string // role name → query text
    Path  string            // source file, for error messages
}

func ExtractTreeQueries(content string) ([]TreeQueries, error)
```

Walks for `sparql-tree-queries`, then iterates *element children* named
`sparql-tree-query`, reading `role=` and that child's own text content. Does not
touch `extractElements` — adding the tags there would break container parsing, as
above.

Rejects at extraction: a `<sparql-tree-query>` with no `role`, a container with no
`for`, a role child outside a container. Prose that merely names the tag carries no
attributes and is skipped, following `rejectLegacyFacets`.

**Tests** (`internal/parser/tree_test.go`): nesting, multiple containers per file,
role text preserved verbatim (whitespace and all — it is SPARQL), a Go template
comment naming the tag ignored, malformed cases.

### 1.2 Validation — `internal/tree/validate.go` (new)

Pure functions, no HTTP, no template deps, so they test cheaply:

- `ValidateRoles(TreeQueries) error` — `roots` and `children` required; unknown
  role rejected; duplicate role rejected (LT-5).
- `ValidateReservedVars(role, query string) error` — the gap found above:
  `children` must leave `?parent` free, `parents`/`node-data` must leave `?node`
  free, `search` must reference `?__token__`. Reject a query that binds a reserved
  name in a pattern.
- `WarnOrderByUnprojected(query string) []string` — LT-16: `ORDER BY ?x` where `?x`
  is not in the projection. A **warning at startup**, not a failure — labels are
  enriched in a second pass (`enrichWithLabels`, `labels.go:356`), so this pages
  over the wrong rows silently. Log it loudly; do not break existing templates.

Role constant strings live here, exported, so the handler cannot typo one.

### 1.3 Index — extend `cmd/visoto/async_index.go`

- `fileDecls` gains `trees []parser.TreeQueries`.
- `parseDecls` calls `ExtractTreeQueries`.
- `initAsyncIndex` builds `idx.trees map[string]map[string]parser.TreeQueries`
  (set → tree id → roles), with the same duplicate-id-names-both-files error
  (LT-4), running §1.2's validators per declaration (LT-5).
- `findTreeQueries(src, id string) (parser.TreeQueries, bool)`, mirroring
  `findAsyncQuery`.

**Deliberately a separate namespace from `queries`.** A tree id and an async id may
coincide without ambiguity — they are addressed by different routes. This differs
from the `<sparql-column for=>` case, which had to reject collisions because a
column had to pick one table.

**Tests** (`cmd/visoto/async_index_test.go`, alongside the existing ones):
duplicate tree id across two files in one set names both; a tree declared in set A
is invisible to set B; missing `children` fails startup.

### 1.4 Query building — `internal/tree/query.go` (new)

One function per role, each returning finished query text:

```go
func RootsQuery(decl, pageIRI string, limit int) (string, error)
func ChildrenQuery(decl, pageIRI, parent string, limit int) (string, error)
func ParentsQuery(decl, pageIRI string, nodes []string) (string, error)
func SearchQuery(decl, pageIRI, term string, limit int) (string, error)
```

Each: `sparql.SubstituteEntity` for `??` (LT-2, validated), then prepend a `VALUES`
clause binding the reserved variable to `sparql.IRITerm`-checked values (LT-1) —
never string substitution into the body. `SearchQuery` binds `?__token__` via
`sparql.StringLiteral`. The `VALUES` goes **inside** the outer `WHERE {`, which
means locating it; `sparql.MembershipTriplePattern` shows the established approach
to that kind of surgery.

`limit` follows LT-15's two modes — detect a trailing `ORDER BY` to decide page-size
vs hard-cap, reusing `sparql.StripTrailingLimitOffset`.

**Tests** are the bulk of the Go test surface (LT-41): reserved-variable binding
produces `VALUES`, not substitution; a hostile IRI (`>` , whitespace, relative) is
rejected; a term with quotes and backslashes is contained; multi-node batching
emits one `VALUES` with N terms.

### 1.5 Routes — `cmd/visoto/lazy_tree.go` (new)

```go
router.GET("/api/lazy-tree/:id/:role", epFromURL, langFromURL, lazyTreeHandler)
```

One handler, role from the path, dispatching per §1.4 — rather than five near
identical handlers. Modelled closely on `asyncTableDataHandler`
(`async_table_data.go:40`): resolve declaration from `?src=`, 404 if unknown,
400 + `Cache-Control: no-store` on malformed input (LT-13), `markURLPure` on
success (LT-14), `cfg.GetTimeout()` context.

Envelope per LT-12:

```json
{ "nodes": [ { "key": "<iri>", "title": "…", "lazy": true, … } ],
  "total": 1234, "complete": false, "limitMode": "page" | "cap" }
```

`limitMode` is what lets the client decide between "load more" and "showing N of M"
(LT-25) — the requirement says the server must report which applied but does not
name the field.

Node shaping mirrors `arrayToTree`'s per-node object (`sparql-tree.js:47-58`) so
extra vars land as `node[varName] = {value, label, type}` and the existing `render`
callback works unchanged.

`lazy: true` comes from `?hasChildren` when projected, else `true` — LT-20's
optimistic default.

### 1.6 Partial — `templates/partials/sparql-lazy-tree.html` (new)

Card chrome copied from `sparql-tree.html` (header, collapse, search input, expand/
collapse buttons, `alert-danger`/`alert-info` states per LT-39). Differences:

- No `{{ toJSON $result.Bindings }}` island — that is the whole point (LT-10).
- One small config island: tree id, query id, page IRI, `src`, limit,
  `minSearchLength`, `restoreState`, `breadcrumb`, `ResourceIRI`. `<template>` +
  `JSON.parse`, per the html/template escaping rules.
- No `treeElem.style.height` computation — the tree fills its container (LT-8).
  Height moves to CSS.
- `data-sparql-lazy-tree` marker, `<script src="/static/js/sparql-lazy-tree.js" defer>`.

`sparqlTree` is untouched (LT-7).

### 1.7 Client — `static/js/sparql-lazy-tree.js` (new)

Lifts `render`, the column builder and the expand/collapse handlers from
`sparql-tree.js` verbatim; replaces `source: treeData` with a lazy source.

- Roots after first paint (LT-17), skeleton until they land.
- `lazyLoad` returning `{url, params}` for `/children` (LT-18); Wunderbaum keeps
  loaded levels, so collapse/re-expand does not refetch.
- Per-node loading and error state (LT-19) — never blank the tree.
- Drop the expander when an optimistic level returns empty (LT-20).
- Local filter below `minSearchLength`, labelled as such (LT-21 first bullet). The
  server search arrives in phase 3; until then the box says so.
- "Load more" / "showing N of M" from `limitMode` (LT-25).
- Keyboard: Enter/Space follows the focused node's link (LT-33).
- Same `boot()` convention as `sparql-tree.js`: per-element guard, no module-level
  latch, `readyState` either/or.

New JS strings go in `locales/en.toml` under `js.*` — the i18n test fails on a key
used but not defined (LT-36).

### 1.8 CSS

Reuse `static/css/wunderbaum_overrides.css` (LT-37). Add only the container-fill
rule LT-8 needs. Row height stays pinned to `--wb-header-height`.

### Phase 1 exit criteria

A page declares `<sparql-tree-queries>` with `roots` + `children`, renders
`sparqlLazyTree`, and the tree loads level by level. Page HTML contains no node
payload. Startup fails on a missing/duplicate/unknown role, and on a reserved
variable bound in a pattern. `go build ./...` and `go test ./...` pass.

### Phase 1b — first two-column instance template

Prove §7 of the requirements with a real page before building anything on top of
it: an instance template with the tree in one column and the resource's own content
in the other. `skos:Concept` is the natural candidate — it already uses the eager
tree, so it is a direct before/after. **Keep the eager version until phase 2 lands**,
since without `parents` the tree cannot restore its position on navigation, which
is exactly what that template needs.

---

## 2. Phase 2 — `parents`, focus, deep link

LT-6, LT-11a, LT-23, LT-24, LT-29, LT-30, LT-31.

**Do LT-11a first.** `/focus` in one round trip is what makes the navigator usable;
a recursive client-side walk would be built and then immediately thrown away.

- `internal/tree/focus.go` — `ResolveFocus(decl, pageIRI, node)` walks `parents`
  recursively server-side (batched `VALUES` per level, so it is one query per depth,
  not per node), then fetches each level along the path. Returns
  `{path: [...], levels: {parentIRI: envelope}}`.
- Guard against cycles: a malformed hierarchy with `A broader B broader A` must
  terminate. Cap depth (say 64) and stop on a repeat.
- `/api/lazy-tree/:id/focus?node=` route, same handler dispatch.
- LT-6 startup checks: focus/`ResourceIRI` without `parents` fails; `search`
  without `parents` fails unless flat-only.
- Client: `ResourceIRI` triggers one `/focus`, seeds the levels, reveals, focuses
  (LT-23). Document the polyhierarchy single-branch behaviour in the partial header.
- LT-30 expansion restore: `sessionStorage`, keyed tree id + endpoint + page IRI,
  storing expanded IRIs and scroll position. Re-expand on mount from the LT-14
  cache. `restoreState=false` opts out. **Cap the stored set** (a few hundred IRIs)
  — a user who expands aggressively should not blow the quota.
- LT-24 external focus: `CustomEvent` on the tree root, documented.
- LT-31 breadcrumb: render the `/focus` path above the tree when `breadcrumb=true`.

Then flip `skos:Concept` (or the chosen template) to the lazy tree and delete its
eager declaration.

---

## 3. Phase 3 — `search`

LT-21, LT-22.

- `/api/lazy-tree/:id/search?q=`, term via `StringLiteral`, debounced 300ms client
  side, `minSearchLength` gate.
- Rank by `?score` when projected, else label.
- Highlight via Wunderbaum's `titleWithHighlight` — already used in
  `sparql-tree.js:150`.
- Flat hits by default (`flatSearch`); hierarchical reveal costs one `/focus` per
  hit, so **batch it** rather than firing N requests. `flatSearchToggle` exposes the
  switch.

Note for later: `internal/search/graphdb_lucene.go` has its own escaping if we ever
route this through the search providers instead of a declared query. Out of scope
now.

---

## 4. Phase 4 — docs and hardening

LT-40, LT-41.

- `docs/templating.md` gains a `sparqlLazyTree` section beside `sparqlTree`
  (currently at line 379), including **when to choose which** and the LT-16
  ordering warning. Add it to the partial list at line 65.
- Fill out the Go test surface: role extraction, the LT-5/LT-6 rejections,
  `VALUES` binding, IRI rejection, multi-node batching, envelope shape, `/focus`
  path order and its no-`parents` rejection.
- Consider a `classTemplate`/`instanceTemplate` skill variant for the two-column
  layout (open decision 4) — only once a real template has proven the shape.

---

## 5. Risks

| Risk | Mitigation |
|---|---|
| `VALUES` injection point is fragile — finding the outer `WHERE {` by pattern breaks on nested/`SELECT`-in-`WHERE` queries | Test against every declared query; fail loudly at startup rather than producing a malformed query at request time. Consider requiring roles be simple `SELECT … WHERE { … }` in v1. |
| Endpoints differ on recursive `parents` cost | Already why `parents` is single-level (§3 of requirements). Cap depth; measure on LINDAS before phase 2 ships. |
| `sessionStorage` restore re-expands a hundred levels and floods the endpoint | Cap the stored set; rely on LT-14 caching; restore serially with a ceiling. |
| The LT-16 ordering trap silently pages the wrong rows | Startup warning in §1.2, plus the docs note. Not a failure — it would break templates that are merely imprecise. |
| Deep hierarchies still feel slow because each navigation re-mounts | The point of LT-11a + LT-30. If it still drags after phase 2, open decision 2 (in-page split view) is the escape hatch — reopen it with measurements, not before. |

---

## 6. File inventory

New:
```
internal/parser/tree.go              + tree_test.go
internal/tree/validate.go            + validate_test.go
internal/tree/query.go               + query_test.go
internal/tree/focus.go               + focus_test.go        (phase 2)
cmd/visoto/lazy_tree.go              + lazy_tree_test.go
templates/partials/sparql-lazy-tree.html
static/js/sparql-lazy-tree.js
```

Modified:
```
cmd/visoto/async_index.go     fileDecls.trees, parseDecls, initAsyncIndex, findTreeQueries
cmd/visoto/main.go            one route registration
locales/en.toml               js.* strings (+ other locales)
docs/templating.md            sparqlLazyTree section
static/css/wunderbaum_overrides.css   container-fill rule
```

Untouched: `templates/partials/sparql-tree.html`, `static/js/sparql-tree.js`, and
the four instance templates using them.
