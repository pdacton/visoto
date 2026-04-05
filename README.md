## Visoto

Visoto is a Go web application for browsing and visualizing RDF linked data resources via SPARQL endpoints. It renders resource pages using type-specific templates, supports full-text search, and exposes an MCP server for AI assistant integration.

Visoto is still in development and not all features work properly.

### Features

- **Resource browser** — fetch any RDF resource by IRI; templates are resolved automatically from the resource's `rdf:type`
- **Full-text search** — search across linked data resources with class and property filters
- **Multi-endpoint support** — switch between named SPARQL endpoints (e.g. LINDAS prod/int/test) via a sidebar menu
- **Graph Explorer** — interactive RDF graph visualization powered by Graph Explorer (Ontodia fork)
- **AI chat** — ask questions about linked data resources, powered by Google Gemini
- **Endpoint monitoring** — track SPARQL endpoint availability and response times over time
- **MCP server** — built-in Model Context Protocol server at `/mcp` for AI tool integration
- **Dark mode** — toggle between light and dark themes
- **Responsive UI** — built with Tabler (Bootstrap 5) and Tabulator for data tables

### Prerequisites

- Go 1.21+
- A SPARQL endpoint (default: [LINDAS](https://ld.admin.ch/query/))
- Optional: Google Gemini API key (for the chat feature)

### Setup

1. **Clone the repo and install dependencies:**
   ```sh
   go mod download
   ```

2. **Create your config file:**
   ```sh
   cp visoto.config.example visoto.config
   ```
   Edit `visoto.config` to set your SPARQL endpoint, port, and optional Gemini API key.

3. **Run the server:**
   ```sh
   go run ./cmd/visoto/
   ```

4. **Open the app:**
   - Web UI: `http://localhost:8060`
   - Health check: `http://localhost:8060/ping`
   - MCP server: `http://localhost:8060/mcp`

**Alternative: run with Docker:**
```sh
docker build -t visoto .
docker run -e GIN_MODE=release -p 8060:8060 \
  -v ./visoto.config:/app/visoto.config:ro \
  visoto
```
See [docs/deployment.md](docs/deployment.md) for full production Docker + Caddy setup.

### Configuration

Configuration is loaded from `visoto.config` (TOML format). See [`visoto.config.example`](visoto.config.example) for all options.

Key settings:

| Setting | Description | Default |
|---|---|---|
| `application.port` | HTTP port (code default is `8080`; example config uses `8060` — set explicitly) | `8080` |
| `application.sparqlEndpoint` | Default SPARQL endpoint URL | — |
| `application.sparqlEndpoints` | Named endpoints for the switcher menu | — |
| `application.timeout` | SPARQL query timeout (seconds) | `30` |
| `application.gemini_api_key` | Google Gemini API key for chat | — |
| `rdf.prefixes` | RDF prefix declarations (SPARQL or Turtle notation) | — |
| `rdf.type_priority` | Priority order for template resolution when a resource has multiple types | — |
| `logging.level` | Log level: `DEBUG`, `INFO`, `WARN`, `ERROR` | `INFO` |
| `mcp.port` | MCP port (unused when embedded in main server) | `8070` |
| `GIN_MODE` (env var) | Gin framework mode: `debug`, `release`, `test` | `debug` |

### Documentation

| Document | Audience |
|---|---|
| [Getting Started](docs/getting-started.md) | Run locally for the first time |
| [Architecture Overview](docs/architecture.md) | Understand how the system fits together |
| [Configuration Reference](docs/configuration.md) | All `visoto.config` fields explained |
| [Template Authoring Guide](docs/templating.md) | Create custom templates for your RDF types |
| [Deployment Guide](docs/deployment.md) | Docker + Caddy production setup |

### Project Structure

```
cmd/visoto/        — main entry point
internal/
  chat/            — Gemini AI chat handler
  config/          — TOML config loading
  logger/          — structured slog logger
  mcp/             — MCP server (AI tool integration)
  monitor/         — simple SPARQL endpoint health monitoring
  parser/          — SPARQL preprocessor (template → query → result)
  resource/        — RDF resource resolution and template matching
  search/          — full-text search over SPARQL endpoints
  sparql/          — SPARQL query execution
  templates/       — Go template loader
templates/
  layout/          — shared layout templates (base, sidebar, header, footer)
  pages/           — static page templates (home, search, monitoring, …)
  classes/         — class-level RDF templates
  instances/       — instance-level RDF templates
static/            — CSS, JS, images
```

### Routes

| Method | Path | Description |
|---|---|---|
| `GET` | `/` | Home page |
| `GET` | `/search` | Full-text search |
| `GET` | `/resource/*path` | Resource page (IRI-based) |
| `GET` | `/monitoring` | Endpoint monitoring dashboard |
| `GET` | `/api/monitoring/status` | Monitoring status (JSON) |
| `POST` | `api/monitoring/toggle` | Enable/disable monitoring |
| `GET` | `/api/monitoring/data` | Historical monitoring data (JSON) |
| `GET` | `/api/metric/:id` | Lazy-load metric counts (HTMX) |
| `POST` | `/api/chat` | Gemini AI chat |
| `ANY` | `/mcp` | MCP server endpoint |
| `GET` | `/ping` | Health check |
