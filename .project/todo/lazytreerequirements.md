# Lazy Tree — Requirements

Status: **draft for review**. No code written yet; this collects what a lazy tree
has to do before we pick an implementation. Requirement IDs (LT-n) are stable —
cite them in issues and PRs.

---

## 1. Why

`templates/partials/sparql-tree.html` (the `sparqlTree` partial) is **eager**: one
`<sparql-query>` returns the entire hierarchy as flat `?node ?parent` rows, the
page render blocks on it, every row is serialised into a `<template>` JSON island,
and `static/js/sparql-tree.js` rebuilds the tree client-side for Wunderbaum.

That is right for the hierarchies it was written for (the SharedDimensionTerm tree
is 462 terms) and should stay for them. It has three hard limits:

| Limit | Consequence |
|---|---|
| The whole hierarchy is one query | A large thesaurus (a full geo or NACE hierarchy) times out, and the timeout takes the whole page with it. |
| The whole hierarchy is in the HTML | Page weight grows linearly with the hierarchy; 50k nodes is megabytes of JSON before first paint. |
| The query is synchronous | Time to first paint is bounded by the slowest hierarchy query on the page, unlike `sparqlAsyncTable`/`sparqlMetric`. |

A lazy tree fetches **one level at a time**, so cost scales with what the user
actually opens rather than with the size of the hierarchy.

---

## 2. Query roles

Five roles, of which the first two are required.

| Role | Req'd | Bound input | Output | Notes |
|---|---|---|---|---|
| `roots` | ✔ | none | `?node`, `?label`, `?hasChildren` | Top level. |
| `children` | ✔ | `?parent` | `?node`, `?label`, `?hasChildren` | One level below `?parent`. |
| `parents` | — | `?node` (set) | `?node`, `?parent` | **Single-level**, applied recursively to walk up to the roots. Required for focus and for hierarchical search hits. |
| `search` | — | `?__token__` | `?node`, `?label`, `?score`, `?hasChildren` | Hits anywhere in the hierarchy; ancestors resolved with `parents`. |
| `node-data` | — | `?node` (set) | anything | Batch extra bindings for visible nodes. Out of scope for v1 (§5). |

`?label` and `?hasChildren` are optional: without `?label` the existing label
enrichment resolves the display value; without `?hasChildren` the tree is
optimistic (LT-20).

Two constraints apply to every role query:

- **No cross products.** A node with two `rdfs:label` values multiplies rows —
  keep node and label in a distinct association.
- **Ordering.** Without an explicit `ORDER BY`, results are sorted by display
  label. With `ORDER BY` *and* a limit, project `?label` explicitly rather than
  relying on label enrichment (see LT-16).

### Reserved variables

**LT-1** — Reserved variables are `?parent` (children), `?node` (parents and
node-data) and `?__token__` (search). They are **bound by the server** with a
prepended `VALUES` clause, never string-substituted into the query body — the
posture `sparql.MembershipBody` already takes.

**LT-2** — `??` keeps its current meaning (the page's resource IRI) in all role
queries, so a children query can be scoped to the concept scheme or shared
dimension the page is about.

---

## 3. Declaration

**LT-3** — Queries are **declared, not passed in**. The result does not exist at
page-render time, so the `sparqlTree` pattern of passing `.QueryResults.foo` does
not apply. Declaration mirrors the `<sparql-columns for="…">` container pattern:

```html
<sparql-tree-queries for="conceptTree">
  <sparql-tree-query role="roots">
    SELECT ?node ?label ?code ?hasChildren WHERE {
      ?node skos:topConceptOf ?? ; skos:prefLabel ?label .
      OPTIONAL { ?node skos:notation ?code }
      BIND(EXISTS { ?c skos:broader ?node } AS ?hasChildren)
    } ORDER BY ?label
  </sparql-tree-query>
  <sparql-tree-query role="children">
    SELECT ?node ?label ?code ?hasChildren WHERE {
      ?node skos:broader ?parent ; skos:prefLabel ?label .
      OPTIONAL { ?node skos:notation ?code }
      BIND(EXISTS { ?c skos:broader ?node } AS ?hasChildren)
    } ORDER BY ?label
  </sparql-tree-query>
  <sparql-tree-query role="parents">
    SELECT ?node ?parent WHERE { ?node skos:broader ?parent }
  </sparql-tree-query>
  <sparql-tree-query role="search">
    SELECT ?node ?label WHERE {
      ?node skos:inScheme ?? ; skos:prefLabel ?label .
      FILTER(CONTAINS(LCASE(?label), LCASE(?__token__)))
    } LIMIT 200
  </sparql-tree-query>
</sparql-tree-queries>
```

The `parents` query is **single-level**, not a `skos:broader*` path; it is applied
recursively until it reaches a root. That keeps it cheap on endpoints that handle
transitive paths badly, which includes some of ours.

**LT-4** — Registered in the startup async index (`cmd/visoto/async_index.go`:
`parseDecls`, `fileDecls`) with the same guarantees `<sparql-async>` has:
set-scoped ids, duplicate ids fail startup naming both files, ids resolved per
template set via `?src=`.

**LT-5** — Startup must reject a block missing `roots` or `children`, with a
duplicate role, or naming an unknown role. Fail at boot, not at runtime.

**LT-6** — Startup must reject focus (`ResourceIRI`) usage without a `parents`
role, and a `search` role without a `parents` role *unless* flat search is the
only mode enabled (LT-22).

---

## 4. Partial API

**LT-7** — New partial `sparqlLazyTree` in
`templates/partials/sparql-lazy-tree.html`. `sparqlTree` stays for small, bounded
hierarchies; the two are siblings, not a replacement.

| Parameter | Type | Default | Description |
|---|---|---|---|
| `queryId` | string | required | Matches `<sparql-tree-queries for="…">`. |
| `iri` | string | — | Page resource IRI, substituted for `??`. |
| `title` / `icon` | string | — | Card heading and Lucide icon, as every other partial. |
| `collapsed` | bool | `false` | Card starts collapsed. |
| `ResourceIRI` | string | — | Node to expand to and focus. Requires a `parents` role. |
| `limit` | int | see LT-15 | Level page size or hard cap. |
| `minSearchLength` | int | `3` | Below this, no server search is issued. |
| `flatSearch` | bool | `false` | Show search hits as a flat list by default. |
| `flatSearchToggle` | bool | `false` | Offer the hierarchical/flat toggle. |
| `placeholder` | string | — | Search box placeholder. |
| `restoreState` | bool | `true` | Restore expanded branches across navigation (LT-30). |
| `breadcrumb` | bool | `false` | Render the ancestor breadcrumb above the tree (LT-31). |

**LT-8** — No `height` parameter. The tree **fills its container**: bounded by
default, and an author who wants a taller tree wraps it in a sized element. That
is what a full-height sidebar column needs anyway.

**LT-9** — The partial emits markup, data attributes and small JSON *config*
islands only. Behaviour lives in `static/js/sparql-lazy-tree.js`, following the
split `sparql-tree.js` and `faceted-table.js` already use. No inline `<script>`
with interpolated template values.

**LT-10** — The rendered page must **not** contain a node payload. Page HTML size
must be independent of hierarchy size.

---

## 5. Server endpoints

**LT-11** — Routes in the existing `/api` fragment tier:

```
GET /api/lazy-tree/:id/roots
GET /api/lazy-tree/:id/children?parent=<iri>
GET /api/lazy-tree/:id/parents?node=<iri>[&node=<iri>…]
GET /api/lazy-tree/:id/search?q=<term>
GET /api/lazy-tree/:id/focus?node=<iri>
GET /api/lazy-tree/:id/node-data?node=<iri>[&node=<iri>…]
```

All take `?src=` (template set), `?iri=` (page resource), `?endpoint=` and
`?lang=`, registered with `epFromURL, langFromURL` like
`/api/async-table-data/:id`. `parents` and `node-data` accept **multiple** `node`
values in one request — both feed a `VALUES` set, and batching is what makes
recursive ancestor resolution affordable.

**LT-11a (focus in one round trip).** `/focus` resolves the ancestor chain of
`node` *and* returns every level along that path in a single response:

```json
{ "path": ["<root>", "<mid>", "<node>"],
  "levels": { "": {…roots…}, "<root>": {…}, "<mid>": {…} } }
```

Each entry in `levels` is an ordinary level envelope (LT-12). Without it the
client walks `parents` recursively and then fetches each level in turn — 4–8
sequential round trips on a deep hierarchy, paid on every navigation (LT-29).
It is the same work the server would do anyway, done in one request instead of a
chain of them. Requires the `parents` role.

**LT-12** — Responses are JSON in Wunderbaum source shape, so the `render`/column
logic in `sparql-tree.js` is lifted rather than rewritten:

```json
{ "nodes": [ { "key": "<iri>", "title": "…", "lazy": true, "code": {…}, … } ],
  "total": 1234, "complete": false }
```

`complete: false` means the level hit `limit`.

**LT-13** — `parent`, `node` and `iri` must be validated as IRIs and rejected with
400 otherwise. `q` is bound as a SPARQL string literal via `sparql.StringLiteral`.
No request input may reach a query as a *variable name* — the reserved names are
fixed in code, the way `DeriveKeyVar` keeps `keyVar` server-derived. Note that the
search providers (`internal/search/graphdb_lucene.go`, `fuseki.go`, `stardog.go`)
have their own escaping if we ever route `search` through them.

**LT-14** — Pure-URL responses (`markURLPure`): no ETag, no `Vary`, long
`max-age`, cacheable by Souin. Malformed requests are `no-store`. Any per-process
cache for hot levels goes through `sweepExpired`/`maxCacheEntries` — the key
includes a caller-controlled IRI, so an unbounded map is an unbounded-growth
lever.

**LT-15** — `limit` has two modes: with `ORDER BY` in the level query it is a
**page size** and the level loads incrementally (default 200); without, it is a
**hard cap** per node (default 10000). The server reports which applied so the
client knows whether "load more" is meaningful.

**LT-16 (ordering trap).** Labels are resolved in a *second* batch query after the
level query returns (`enrichWithLabels`), so a level query using `ORDER BY ?label`
+ `LIMIT` without projecting `?label` will page over the wrong rows and then
relabel them. The docs must say: **when you use `limit` with `ORDER BY`, project
`?label`.** Consider warning at startup when a role query has `ORDER BY` on a
variable it does not project.

---

## 6. Client behaviour

**LT-17** — Roots load after first paint; the card shows the existing skeleton
until they arrive.

**LT-18** — Expanding a node fetches its children through Wunderbaum's `lazyLoad`
event, which accepts `{url, params}` or a Promise. Loaded levels are kept —
collapsing and re-expanding must not refetch.

**LT-19** — A loading level shows Wunderbaum's loading state on that node. A
failed level shows an error **on that node**; it must not blank the tree or the
page.

**LT-20 (expanders).** Default to optimistic: assume every node has children,
render the expander, and drop it when the level comes back empty. `?hasChildren`,
when projected, overrides that. One wasted request per leaf the user opens, in
exchange for no extra work in the common query.

**LT-21 (search).** Wunderbaum's client-side filter only sees loaded nodes, which
on a lazy tree is misleading. So:
- below `minSearchLength` (default 3), filter locally over loaded nodes only, and
  label the box as such;
- at or above it, call `/api/lazy-tree/:id/search`, debounced 300ms;
- rank by `?score` when projected, else by label;
- highlight the matched substring via Wunderbaum's `titleWithHighlight`;
- with no `search` role declared, keep local filtering and say so.

**LT-22 (flat vs hierarchical hits).** Reconstructing ancestors for every hit
costs one recursive `parents` walk per hit. Support both: `flatSearch` shows hits
as a flat list (cheap, often what the user wants); otherwise reveal each hit in
place. `flatSearchToggle` exposes the switch in the header.

**LT-23 (focus / deep link).** `ResourceIRI` must expand to and focus that node.
Because `makeVisible()` can only expand nodes already in the tree, this needs the
`parents` role: walk up recursively (batched, LT-11), load those levels, then
reveal. In a polyhierarchy **only one branch expands** — acceptable, and must be
documented rather than papered over.

**LT-24 (external focus).** Expose a documented way for other page code to say
"focus this tree on this IRI". A DOM `CustomEvent` on the tree root is the
idiomatic fit. Useful for a page that focuses the tree from elsewhere in its own
UI without a navigation.

**LT-25 (`limit` paging).** When a level reports `complete: false` under page-size
semantics, offer "load more" on that level; under hard-cap semantics, show
"showing N of M" and do not offer to load more. Never truncate silently.

---

## 7. Navigation and page integration

The tree is the navigation surface: label, optional code, a few extra columns,
each node a link to `/resource?iri=…`. Clicking a node is ordinary page
navigation, and the target page is rendered by the usual instance template.

A master–detail layout needs no special support: an instance template puts the
tree in one column and the resource's own content in the other, and the tree
re-renders there with `ResourceIRI` set (LT-28). The right pane is then the real
instance template, with every partial it already has, and the URL is the resource
URL — so deep-linking, history, middle-click and open-in-new-tab work because
they are not being simulated. The cost is a page load per click, which LT-29 and
LT-30 bring down to roughly a paint.

**LT-26** — Node titles are `<a href="/resource?iri=…">`: real links, so browser
history, middle-click and open-in-new-tab all work.

**LT-27** — Click-to-expand on the node body conflicts with LT-26: a click cannot
both follow a link and toggle expansion. Expansion is the **toggle only**; the
body is the link.

**LT-28** — On the target page the tree re-renders with `ResourceIRI` set, so the
user lands with their position in the hierarchy expanded and focused (LT-23).

**LT-29 (re-mount cost).** Because the tree re-mounts on every navigation, the
focus path must be restored in **one** request via `/focus` (LT-11a), not a
recursive `parents` walk followed by per-level fetches. Level responses are
already pure-URL and cacheable (LT-14), so repeat visits are served from cache.

**LT-30 (expansion state).** Re-mounting otherwise loses every expanded branch
except the focused path — the one real regression against an in-page split view.
The tree persists the set of expanded IRIs (`sessionStorage`, keyed by tree id +
endpoint + page IRI) and re-expands them on mount, served from the LT-14 cache.
Scroll position is restored with it. `restoreState=false` opts out.

**LT-31 (breadcrumb).** The ancestor chain from `/focus` is available for free,
so the partial can render it above the tree with `breadcrumb=true`. Useful when
the tree is collapsed or the focused node is deep.

**LT-32 (branch listings).** To show *the members of a branch* rather than one
resource — "the 340 things under this node" — the instance template adds a
`sparqlAsyncTable` scoped by `??` with the depth policy the author wants
(`skos:broader` vs `skos:broader+`). The tree partial encodes no depth policy and
no list pane of its own.

**LT-33** — Keyboard: arrows move through the tree, Enter/Space follows the
focused node's link. Focus is restored to the corresponding node after
navigation, not reset to the top of the tree.

---

## 8. Non-functional requirements

**LT-34 Performance.** Initial page render must not wait on any hierarchy query.
Target p50 < 500ms per level fetch; show a spinner only after 150ms so fast levels
do not flicker.

**LT-35 Security.** The client sends an *id*, never SPARQL. Query text is resolved
server-side from (template set, id, role). See LT-13 for input handling.

**LT-36 i18n.** Every string via `{{ t "key" "English" }}` in templates and
`vsT('js.…', 'English')` in JS, per `docs/templating.md`. New JS strings go in
`locales/en.toml` under `js.*` — the i18n test fails on a key used but not
defined.

**LT-37 Theming.** Reuses `static/css/wunderbaum_overrides.css`; light/dark via the
existing `data-bs-theme` handling. Row height stays in sync with
`--wb-header-height`, as the current partial notes.

**LT-38 Accessibility.** Keep Wunderbaum's ARIA tree semantics; the link in the
title cell stays focusable; loading and error states are announced, not just
coloured.

**LT-39 States.** Empty hierarchy, failed root query, failed level, and empty
search each get a distinct translated state, in the `alert-danger`/`alert-info`
idiom `sparqlTree` already uses.

**LT-40 Docs.** `docs/templating.md` gains a `sparqlLazyTree` section beside
`sparqlTree`, including when to choose which, and the LT-16 ordering warning.

**LT-41 Tests.** Go tests for: role extraction from `<sparql-tree-queries>`;
missing/duplicate/unknown role rejection; the LT-6 dependency checks; `VALUES`
binding of reserved variables; IRI-validation rejection; multi-`node` batching;
the JSON envelope shape; and the `/focus` response (LT-11a) — path order, one
level entry per ancestor, and rejection when no `parents` role is declared.

---

## 9. Out of scope

- **`node-data` role.** Worth having later — it batch-fetches extra columns for
  visible nodes instead of making every level query project them — but the level
  queries can carry those columns in v1.
- **Per-node HTML templates** for nodes, the search box and header actions.
  Visoto's equivalent is the `render` callback plus treegrid columns; a per-node
  templating language is a much larger surface.
- **An in-page split view** — a selection mode that swaps a list pane instead of
  navigating (open decision 2). A two-column instance template is the same view
  without a new render path; revisit only if page loads prove slow in practice.
- Editing the hierarchy (drag-drop re-parenting, add/remove).
- Multi-select and checkbox trees.
- Polyhierarchy display beyond what falls out of the data (see LT-23).
- Auto-switching eager↔lazy by size, the way `sparqlAsyncTable` auto-detects
  working-set mode. Worth doing later; needs a cheap size probe first.

---

## 10. Open decisions

| # | Question | Recommendation |
|---|---|---|
| 1 | New partial or extend `sparqlTree`? | **New.** The data contract differs (declared queries vs a passed-in result), and the eager tree is correct for small hierarchies. |
| 2 | An in-page split view (select a node, swap a list pane) instead of navigation? | **No.** An instance template with the tree in one column is the same view, with no new render path, no URL state to sync, and real links (§7). Revisit only if per-click page loads prove slow after LT-29/LT-30. |
| 3 | Ship the `node-data` role in v1? | **No** (§9), unless a real template needs a column the level query cannot cheaply project. |
| 4 | Does the two-column instance template need a scaffold? | Probably an `instanceTemplate`-skill variant, once the partial exists. |
| 5 | Persist expansion state in `sessionStorage` or the URL? | **`sessionStorage`** (LT-30) — the URL already carries the focused node, and a list of expanded IRIs would bloat it. |

---

## 11. Suggested phasing

| Phase | Delivers | Requirements |
|---|---|---|
| 1 | Declaration + index + `roots`/`children` + `sparqlLazyTree` as a navigator | LT-1…LT-5, LT-7…LT-20, LT-25…LT-28, LT-33…LT-39 |
| 1b | Two-column instance template using it (tree + resource content) | §7 |
| 2 | `parents` role: focus, deep link, external focus command | LT-6, LT-23, LT-24 |
| 3 | `search` role, flat/hierarchical hits | LT-21, LT-22 |
| 4 | Re-mount ergonomics: `/focus`, state restore, breadcrumb | LT-11a, LT-29…LT-32 |
| 5 | Docs, tests, a first real class template using it | LT-40, LT-41 |

Phase 1 alone removes the timeout and page-weight problems, which is the point of
the exercise; 2–4 are the ergonomics.
