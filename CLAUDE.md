# Agent Instructions for this Project

Visoto is a Go server that renders RDF resources from SPARQL endpoints as HTML
pages. Every page is built from SPARQL queries embedded in its template; there
is no database for RDF data. (The only local store is a SQLite file under
`./data/` holding endpoint-monitoring history.)

## Build & run

- Build: `go build ./...` — Test: `go test ./...`
- Run: `go run ./cmd/visoto/` (port from `visoto.config`, currently 8060).
  `PORT=8061 go run ./cmd/visoto/` overrides it — use a different port than the
  one the user is already running on.
- `templates/`, `static/` and `locales/` load from disk at startup, not
  `go:embed`. **Restart the server after adding or changing a template.**
- A new runtime asset directory must be added to both `Dockerfile` and
  `deploy.sh`, or production crash-loops.

## Templates

Templates live in `templates/`:

| Dir | Purpose |
|---|---|
| `layout/` | base, topbar, sidebar, header, footer |
| `partials/` | `sparqlTable`, `sparqlAsyncTable`, `sparqlGraph`, tree, metric, … |
| `components/` | `pageHeader`, `literals`, `relationships` |
| `pages/` | standalone pages (`/about.html`, `/politics.html`, …) |
| `classes/` | per-RDF-class pages |
| `instances/` | per-instance-type pages |

A template does not "extend" a layout — it **defines blocks** (`pageTitle`,
`pageSubtitle`, `pageIcon`, `pageContent`, …) that the base layout renders.
`{{ template "pageHeader" . }}` fills the whole header from SPARQL.

**Read `docs/templating.md` before writing or editing a template.** It is the
authoritative authoring guide (page shell, custom tags, partial parameters).

### Custom SPARQL tags

Queries are written directly in the markup and preprocessed at startup by
`internal/parser`; results land in `.QueryResults.<id>`:

- `<sparql-query id="x">` — query executed during page render
- `<sparql-async id="x">` — lazily loaded over HTMX after paint (use for slow
  or large queries)
- `<sparql-column>` / `<sparql-columns>` — declare table columns; a column
  carrying `filter` makes the table faceted
- `<sparql-tree-query>` / `<sparql-tree-queries>` — hierarchy trees

Go template actions (`{{ t }}`, `{{ if }}`) **must not** appear inside a query
body — the text is sent to the endpoint verbatim.

`visoto:<key>` in a query expands to a property path from
`[rdf.magic_properties]` in `visoto.config`. Any query selecting a label or
description needs the `visoto:dispLang` filter.

### i18n

Every user-facing string goes through `{{ t "key" "English default" }}` or
`{{ tHTML ... }}`. Catalogs live in `locales/*.toml` (de, en, fr, it, rm).
Pages resolve language from the `site-lang` cookie; `/api/*` reads `?lang=`
from the URL only.

### Frontend

Tabler 1.4 (Bootstrap 5) + Tabulator 6.5 + HTMX 2 + Lucide, loaded via CDN in
`templates/layout/base.html`. Use Bootstrap/Tabler classes rather than custom
CSS; overrides go in `static/css/*_overrides.css`.

Templates emit markup and `<template>` config islands only — **no inline JS**.
Behaviour lives in `static/js/`, keyed off a `data-<name>` marker.

## Go backend

- Idiomatic Go; prefer methods on structs over standalone functions.
- Entry point `cmd/visoto/`, everything else under `internal/`
  (`sparql`, `templates`, `parser`, `column`, `facet`, `search`, `i18n`,
  `lang`, `cache`, `mcp`, `resource`, `tree`, `upload`, `monitor`).
- Tests are standard `*_test.go` alongside the code.

## Data layer

SPARQL endpoints are configured in `visoto.config` under
`[[application.sparqlEndpoints]]`; each has a **slug**, which is the only
identifier on the wire (`/resource?iri=<IRI>&endpoint=<slug>`). Default is
LINDAS prod cached (`https://cached.lindas.admin.ch/query`).

Cacheable routes (`/resource`, `/api/*`) must stay pure functions of the URL —
never read the endpoint cookie in them.

Build resource links with `sparql.ResourceHref` (Go) or `visotoResourceHref`
(JS), never by hand.

Note: LINDAS instance counts drift between calls — query counts live with
`<sparql-async>`, never hard-code them in prose.

## Graph Explorer

RDF graph visualization via [Graph Explorer](https://github.com/zazuko/graph-explorer)
2.1.0 from CDN. Embed it with `{{ template "sparqlGraph" (dict ...) }}`
(`templates/partials/sparql-graph.html`); styling overrides in
`static/css/ontodia_overrides.css`. See the `graph-explorer` skill and
`docs/ontodia-graph-explorer-references.md`.

## Debugging

Playwright MCP is configured in `.mcp.json` (gitignored); start the server
first. Chromium runs headless under WSL. It silently caches `static/js` and
`static/css` — hard-clear the browser cache before verifying frontend changes.

## Skills

In `.claude/skills/`: **graph-explorer**, **branding**, **instanceTemplate**,
**classTemplate**, **iconGeneration**.

## Docs

`docs/templating.md` (template authoring), `docs/architecture.md`,
`docs/configuration.md`, `docs/deployment.md`, `docs/getting-started.md`.

## graphify

Knowledge graph at `graphify-out/`. **Scope: `.go`, `.js`, `.sh` only** —
`templates/` is not indexed, so use grep/Read for template questions.

- Go/JS questions: run `graphify query "<question>"` first; `graphify path
  "<A>" "<B>"` for relationships, `graphify explain "<concept>"` for concepts.
- `graphify-out/wiki/index.md` for broad navigation;
  `graphify-out/GRAPH_REPORT.md` only for architecture review.
- After changing code, run `graphify update .`.
