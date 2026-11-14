# Agent Instructions for this Project

Visoto is a Go server that renders RDF resources from SPARQL endpoints as HTML
pages. Every page is built from SPARQL queries embedded in its template; there
is no database for RDF data. (The only local store is a SQLite file under
`./data/` holding endpoint-monitoring history.)

## Build & run

- JS tests: `node --test tests/js/` (dependency-free; covers `static/js/graph-layout.js`)
- Run: `go run ./cmd/visoto/` (port from `visoto.config`, currently 8060).
  `PORT=8061 go run ./cmd/visoto/` overrides it — use a different port than the
  one the user is already running on.
- `templates/`, `static/` and `locales/` load from disk at startup, not
  `go:embed`. **Restart the server after adding or changing a template.**
- A new runtime asset directory must be added to both `Dockerfile` and
  `deploy.sh`, or production crash-loops.

## Templates

A template does not "extend" a layout — it **defines blocks** (`pageTitle`,
`pageSubtitle`, `pageIcon`, `pageContent`, …) that the base layout renders.
`{{ template "pageHeader" . }}` fills the whole header from SPARQL.

**Read `docs/templating.md` before writing or editing a template.** It is the
authoritative authoring guide (page shell, custom tags, partial parameters).

### Custom SPARQL tags

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

Use Bootstrap/Tabler classes rather than custom CSS; overrides go in `static/css/*_overrides.css`.

Templates emit markup and `<template>` config islands only — **no inline JS**.
Behaviour lives in `static/js/`, keyed off a `data-<name>` marker.

JS never hard-codes a vocabulary namespace: write the term as a CURIE and
expand it through `VisotoPrefixes.fromPage()` (`static/js/prefixes.js`), which
carries the active endpoint's prefixes (an endpoint may redeclare e.g. `schema:`).

## Go backend

- Prefer methods on structs over standalone functions.
- Entry point `cmd/visoto/` is a thin wrapper around `app.Main`; the server
  (config, router, HTTP handlers) lives in the public `app/` package, everything
  else under `internal/`.

## Data layer

SPARQL endpoints are configured in `visoto.config` under
`[[application.sparqlEndpoints]]`; each has a **slug**, which is the only
identifier on the wire (`/resource?iri=<IRI>&endpoint=<slug>`). Default is
LINDAS prod cached (`https://cached.lindas.admin.ch/query`).

Every `/api/*` route (plus `/metrics`, `/mcp`, `/health`, `/ping`) is documented
in `static/openapi.yaml`, rendered with Swagger UI at `/apiDocs.html`. Adding,
removing or changing a route means updating the spec; `TestOpenAPICoversRoutes`
fails on any route/spec mismatch.

Cacheable routes (`/resource`, `/api/*`) must stay pure functions of the URL —
never read the endpoint cookie in them.

Register routes only through the route table in `app/server.go` (`t.GET(path,
access.Browse, …)`), never on the router directly: the declared `access.Action`
puts an authorization guard in front of the handler, and
`TestEveryRouteDeclaresAccess` fails otherwise. Server-rendered pages must be the
same for every visitor (they are cached); personal UI belongs in client-side
code. See "Extension points" in `docs/architecture.md`.

Build resource links with `sparql.ResourceHref` (Go) or `visotoResourceHref`
(JS), never by hand.

Endpoint **URLs are server-side only** — the browser never receives one. All
browser-side SPARQL (Graph Explorer, the schema diagram) posts to the
same-origin proxy `POST /api/sparql?endpoint=<slug>` (`internal/sparqlproxy`),
which attaches endpoint credentials server-side and is what makes the
Docker-private QLever endpoint usable at all. Templates
get the proxy path as `.GraphQueryURL`; there is no `.EndpointURL`.

Note: LINDAS instance counts drift between calls — query counts live with
`<sparql-async>`, never hard-code them in prose.

## Graph Explorer

Embedding, the shared adapter layer and styling: see the `graph-explorer` skill.
GE queries go through the `/api/sparql` proxy and **must stay POST**: a GET puts
the whole query in the URL and long link queries blow past the ~8 KB limit,
silently losing every edge.

## RDF tooling (Python)

`rdflib` and `pyshacl` live in a persistent venv at `~/.venvs/rdf` (system pip
is PEP 668-locked, no sudo). Call it directly, no activation needed:
`~/.venvs/rdf/bin/python -I -c "import rdflib; ..."` to parse or inspect Turtle,
`~/.venvs/rdf/bin/pyshacl -s shapes.ttl data.ttl` to validate.

## Debugging

Playwright MCP is configured in `.mcp.json` (gitignored); start the server
first. Chromium runs headless under WSL. It silently caches `static/js` and
`static/css` — hard-clear the browser cache before verifying frontend changes.

## graphify

Knowledge graph at `graphify-out/`. **Scope: `.go`, `.js`, `.sh` only** —
`templates/` is not indexed, so use grep/Read for template questions.

- Go/JS questions: run `graphify query "<question>"` first; `graphify path
  "<A>" "<B>"` for relationships, `graphify explain "<concept>"` for concepts.
- `graphify-out/wiki/index.md` for broad navigation;
  `graphify-out/GRAPH_REPORT.md` only for architecture review.
- After changing code, run `graphify update .`.
