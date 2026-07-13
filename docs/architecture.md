# Architecture Overview

## Overview

Visoto is a stateless Go web server that sits between a browser and a remote SPARQL triple store. Given an RDF resource IRI, it selects the best-matching Go HTML template, executes the SPARQL queries embedded in that template server-side (in parallel), and returns rendered HTML. There is no local database — all data lives in the configured SPARQL endpoint.

## System Diagram

```
                     ┌─────────────────────────────┐
                     │           Browser           │
                     └──────────────┬──────────────┘
                                    │ HTTPS
                     ┌──────────────▼──────────────┐
                     │     Caddy (reverse proxy)   │
                     │  :80 → redirect to HTTPS    │
                     │  :443 → auto Let's Encrypt  │
                     └──────────────┬──────────────┘
                                    │ HTTP
                     ┌──────────────▼──────────────┐
                     │        Visoto (Go)          │
                     │        port 8060            │
                     └──────────────┬──────────────┘
                                    │ HTTPS (SPARQL over HTTP)
                     ┌──────────────▼──────────────┐
                     │   SPARQL Endpoint (remote)  │
                     │  e.g. ld.admin.ch/query/    │
                     └─────────────────────────────┘
```

Caddy is for production deployments and can be ommitted for local development — you can hit Visoto directly on port 8060.

## Request Lifecycle

This is what happens when a browser requests `/resource?iri=<IRI>`:

1. **Gin router** receives the request and decodes the IRI from the `iri` query parameter. (Legacy `/resource/<IRI>` path URLs are 301-redirected to this form.)

2. **`resource.New()`** normalizes the IRI, computes its shortened (prefixed) form using the configured RDF prefixes (e.g., `http://www.w3.org/2004/02/skos/core#Concept` → `skos:Concept`).

3. **`resource.ResolveTemplate()`** walks a 5-tier lookup to find the best template:
   - Direct IRI match in `templates/classes/` (short then full IRI)
   - Direct IRI match in `templates/instances/` (short then full IRI)
   - Query SPARQL for `rdf:type`; sorted by `type_priority`; match each type in `templates/instances/`
   - Detect class vs. instance; use `templates/classes/default.html` or `templates/instances/default.html`
   - Hard fallback: `templates/pages/resource.html`

4. **`preprocessor.ProcessTemplateFile()`** reads the selected template file, extracts all `<sparql-query>` custom elements, executes their SPARQL queries in parallel against the configured endpoint, and stores results in a `TemplateData.QueryResults` map keyed by query `id`.

5. **`c.HTML()`** renders the Go template with the populated `TemplateData` struct and returns the HTML response.

For non-resource pages (search, monitoring, home), the lifecycle is simpler: the Gin handler directly renders a page template with minimal or no SPARQL queries.

## Key Packages

| Package | Description |
|---|---|
| `cmd/visoto` | Main entry point: config loading, Gin router setup, all HTTP handler functions |
| `internal/config` | TOML config parsing; defines `Config` and `SparqlEndpoint` structs |
| `internal/resource` | IRI normalization, template resolution (5-tier lookup), icon resolution |
| `internal/parser` | Template file parsing, `<sparql-query>` element extraction, parallel SPARQL pre-execution; defines the `TemplateData` struct |
| `internal/sparql` | SPARQL query execution over HTTP; defines `QueryResult` and `Binding` types |
| `internal/templates` | Go template loading, layout+component composition at startup, custom template func map |
| `internal/search` | Full-text search handler (issues SPARQL queries to the endpoint) |
| `internal/monitor` | SPARQL endpoint health polling, response-time time-series storage in `./data/` |
| `internal/chat` | Google Gemini AI chat handler |
| `internal/mcp` | Model Context Protocol server (embedded at `/mcp` on the main port) |
| `internal/logger` | Structured `slog` logger wrapper |

## Template System

Templates live in the `templates/` directory. Each template is a Go HTML template file that may embed SPARQL queries using `<sparql-query id="...">` custom elements. The `??` token inside a query is substituted with the current resource IRI before execution. Query results are available in the template as `.QueryResults.<id>`.

The template loader (`internal/templates`) composes each page template with the shared layout files at startup. Templates are loaded once and cached in memory; a server restart is required to pick up changes.

See [docs/templating.md](templating.md) for the full authoring guide.

## Data Persistence

Persistence happens at two levels:

**Server-side (`./data/`):** SPARQL endpoint monitoring data is written as JSON time-series files. This directory is created automatically on startup. Everything else on the server is stateless — all RDF data lives in the remote SPARQL endpoint.

**Browser-side:** Several UI preferences are stored locally in the browser so they survive page navigation and reloads.

*Cookie (also read server-side):*

| Key | Description |
|---|---|
| `selectedEndpoint` | Name of the currently selected SPARQL endpoint. Read by the server on every request to set `.EndpointTag` in templates. |

*`localStorage` (client-side only):*

| Key | Description |
|---|---|
| `bs-theme` | Dark/light mode preference (`"dark"` or `"light"`). |
| `visoto-bookmarks` | Bookmarked resource IRIs with labels, reorderable in the sidebar. |
| `visoto-sidebar-tab` | Last active sidebar tab. |
| `rightSidebarOpen` | Whether the right sidebar panel is open. |
| `rightSidebarWidth` | Saved pixel width of the right sidebar after resizing. |
| `visoto-chat-<IRI>` | Chat history for a specific resource, keyed by IRI. |
| `tabulator-groupby-<id>` | Last selected group-by column for each `sparqlTable` instance. |
| `mermaid-height-<id>` | Saved height for each `sparqlMermaidFlow` diagram card. |
| `mermaid-direction-<id>` | Saved flow direction (`"LR"`/`"TD"`) for each Mermaid diagram. |

## Authentication and Security

Visoto has no built-in authentication. It is designed to be placed behind a reverse proxy (Caddy in production) that handles TLS termination. Rate limiting is applied at the Caddy layer.

If a SPARQL endpoint requires credentials, embed them in the endpoint URL (e.g., `https://user:pass@endpoint/query`) or handle authentication at the proxy layer.

## Configuration Flow

```
visoto.config (TOML)
       │
       ▼
internal/config.Load()
       │
       ▼
Config struct (passed to all handlers at startup)
       │
       ├── HTTP handlers (Gin)
       ├── SPARQL preprocessor
       ├── Template resolver
       └── Endpoint monitor
```

Configuration is loaded once at startup. There is no runtime config reload — restart the server to apply changes.

See [docs/configuration.md](configuration.md) for the full field reference.
