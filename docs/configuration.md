# Configuration Reference

Visoto is configured via a `visoto.config` file in TOML format. The file must be in the working directory where the server is started. Configuration is loaded once at startup — restart the server to apply changes.

To create your config file:
```sh
cp visoto.config.example visoto.config
```

## File Structure

```toml
[application]
# ... application settings

[[application.sparqlEndpoints]]
# ... one block per named endpoint (repeatable)

[rdf]
# ... RDF prefix declarations and type priority

[mcp]
# ... MCP server settings (currently unused)

[logging]
# ... log level, format, output
```

---

## `[application]`

| Field | Type | Default | Description |
|---|---|---|---|
| `port` | integer | `8080` | HTTP port to listen on. The example config uses `8060` — set this explicitly, as the code default differs. |
| `sparqlEndpoint` | string | — | Fallback SPARQL endpoint URL. Used when no named endpoints are configured or when the selected named endpoint cannot be resolved. |
| `timeout` | integer | `30` | Per-query timeout in seconds for all SPARQL requests. |
| `gemini_api_key` | string | — | Google Gemini API key. Required only for the AI chat feature at `/api/chat`. The rest of the app works without it. Get a key at [aistudio.google.com](https://aistudio.google.com/app/apikey). |

---

## `[[application.sparqlEndpoints]]` — Named Endpoints {#named-endpoints}

Each `[[application.sparqlEndpoints]]` block defines one entry in the endpoint-switcher sidebar menu. You can define as many as you need.

| Field | Type | Default | Description |
|---|---|---|---|
| `name` | string | required | Display name shown in the sidebar menu (e.g., `"LINDAS prod"`). |
| `url` | string | required | Full SPARQL endpoint URL. |
| `default` | boolean | `false` | Marks this endpoint as pre-selected when the app starts. At most one endpoint should have `default = true`. Also used as the fallback when resolving `.EndpointTag` in templates. |
| `monitor` | boolean | `false` | Enables health monitoring for this endpoint. Monitored endpoints appear on the `/monitoring` dashboard with response-time history stored in `./data/`. |
| `tag` | string | `""` | A logical group label (e.g., `"lindas"`, `"stadtzuerich"`). The tag of the currently selected endpoint is exposed to templates as `.EndpointTag`, allowing templates to conditionally show endpoint-specific content. |

### Example: minimal endpoint

```toml
[[application.sparqlEndpoints]]
name = "My endpoint"
url = "https://example.org/sparql"
default = true
monitor = false
tag = "myendpoint"
```

### Example: multiple endpoints with monitoring

```toml
[[application.sparqlEndpoints]]
name = "Prod"
url = "https://example.org/sparql"
default = true
monitor = true
tag = "myendpoint"

[[application.sparqlEndpoints]]
name = "Staging"
url = "https://staging.example.org/sparql"
monitor = true
tag = "myendpoint"
```

### Using `tag` in templates

The `.EndpointTag` value is available in every template. Use it to show endpoint-specific links or content:

```html
{{ if eq .EndpointTag "lindas" }}
  <a href="https://ld.admin.ch/...">Open in LINDAS</a>
{{ end }}
```

See [docs/templating.md](templating.md) for the full template data model.

---

## `[rdf]`

| Field | Type | Default | Description |
|---|---|---|---|
| `prefixes` | list of strings | — | RDF prefix declarations. Prepended to every SPARQL query and used to compute shortened IRIs in the UI. |
| `type_priority` | list of strings | — | Full IRIs of RDF types. When a resource has multiple `rdf:type` values, template resolution tries types in this order. |

### Prefix format

Three notations are accepted (all equivalent):

```toml
prefixes = [
  # SPARQL notation
  "PREFIX skos: <http://www.w3.org/2004/02/skos/core#>",
  # Turtle notation
  "@prefix dct: <http://purl.org/dc/terms/> .",
  # Short notation
  "schema: http://schema.org/",
]
```

### `type_priority`

When a resource has multiple `rdf:type` values, Visoto tries to find a template for each type in the order listed here. Types not in the list are tried after the prioritized ones, in their original order.

```toml
[rdf]
type_priority = [
  "https://schema.ld.admin.ch/ZefixOrganisation",
  "http://schema.org/Organization",
]
```

In this example, if a resource is both a `ZefixOrganisation` and an `Organization`, the `ZefixOrganisation` template wins (if it exists).

See [Template Resolution](templating.md#template-resolution) for the full lookup algorithm.

---

## `[mcp]`

| Field | Type | Default | Description |
|---|---|---|---|
| `port` | integer | `8070` | Currently unused. The MCP server is embedded in the main application and served at `/mcp` on the same port as the web UI. |

---

## `[logging]`

| Field | Type | Default | Description |
|---|---|---|---|
| `level` | string | `"INFO"` | Log verbosity: `DEBUG`, `INFO`, `WARN`, `ERROR`. Use `DEBUG` during development to see SPARQL queries and template resolution details. |
| `format` | string | `"text"` | Log format: `"text"` (human-readable) or `"json"` (structured, suitable for log aggregators like Loki or Datadog). |
| `output` | string | `"stdout"` | Log destination: `"stdout"`, `"stderr"`, or a file path (e.g., `"./logs/visoto.log"`). |

---

## Environment Variables

Only one environment variable is recognized at runtime:

| Variable | Values | Description |
|---|---|---|
| `GIN_MODE` | `debug`, `release`, `test` | Controls Gin framework verbosity. Set to `release` in production to suppress per-request debug output. The Docker Compose file sets this automatically. |

Example:
```sh
GIN_MODE=release go run ./cmd/visoto/
```
