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
# ... RDF prefix declarations, type priority, magic properties

[[ontologies]]
# ... one block per importable ontology (repeatable)

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
| `allow_private_upload_urls` | boolean | `false` | Allows the URL mode of `/api/upload` to fetch from loopback, private and link-local hosts. Off by default as an SSRF guard — enable only in a local test environment where you need to upload from e.g. `http://localhost`. |
| `default_language` | string | `"en"` | Language used when a request expresses no usable preference. Must be one of the codes in `[[application.languages]]`. See [UI languages](#ui-languages). |

---

## UI languages {#ui-languages}

The interface is translated per request. `[[application.languages]]` is the
closed set the site offers, in the order the topbar picker lists them:

```toml
[application]
default_language = "en"

[[application.languages]]
code  = "de"
label = "Deutsch"

[[application.languages]]
code  = ""
label = "No language"
```

| Field | Description |
|---|---|
| `code` | The value carried by the `site-lang` cookie and the `X-Site-Lang` header, and the name of the `locales/<code>.toml` catalog. Each non-empty code needs that catalog, or startup fails. |
| `label` | The text the picker displays. A literal, not a message key — one label per code, the same in every locale. Conventionally the language's own endonym ("Deutsch", not "German"), so the list reads identically whichever language the page is rendered in. That is what a reader scanning for their own language needs. |

The empty `code` is a legal, deliberate member: it is the **"no language"**
choice in the picker. For UI strings it renders the base (English) catalog; the
semantics reserved for it in RDF literal filtering are "untagged literals only".

> **TOML ordering.** `[[application.languages]]` is an array of tables, so every
> bare key written after it belongs to that table rather than to `[application]`.
> Keep `port`, `timeout`, `gemini_api_key`, `default_language` and friends
> *above* the language blocks — otherwise they silently move, with no parse
> error. `TestExampleConfigParses` guards the shipped example against exactly
> this.

### How a request's language is resolved

In order, first usable wins:

1. the `X-Site-Lang` request header,
2. the `site-lang` cookie,
3. `Accept-Language`, matched against `languages` with CLDR-aware matching,
4. `default_language`.

Every branch is validated against `languages`, so an unconfigured code can never
reach the renderer — it falls back to `default_language` instead.

In production Caddy collapses the cookie and `Accept-Language` into a single
normalized `X-Site-Lang` before the shared cache and folds that header into the
cache key, so only step 1 runs. Steps 2 and 3 are what make the same code behave
correctly in development, where there is no cache in front.

### Keeping the Caddyfile in sync

The `languages` list is duplicated in the `Caddyfile`, which cannot read this
file. The duplication is safe — visoto re-validates `X-Site-Lang` against the
configured set, so a Caddyfile listing a code visoto does not know only causes a
fallback to `default_language`. But **adding or removing a language means editing
both files**, plus adding the catalog.

Changing the list also changes every shared-cache key, so a deploy that touches
it should be followed by a cache purge (Settings → "Clear cache").

### Response caching

Because the language now comes from a cookie rather than the URL, every response
carries `Cache-Control: public, max-age=0, must-revalidate` and an
`ETag: "<lang>-v<hash>"`, and `Vary` names `X-Site-Lang` when that header was
present and `Accept-Language` otherwise. Browsers therefore revalidate on every
navigation — which is what stops a language switch from re-serving the previous
language out of the private cache — and get a cheap `304`. Responses a handler
marks cacheable additionally carry `s-maxage=21600`, so Souin still serves them
for six hours and answers those revalidations itself.

---

## `[[application.sparqlEndpoints]]` — Named Endpoints {#named-endpoints}

Each `[[application.sparqlEndpoints]]` block defines one entry in the endpoint-switcher sidebar menu. You can define as many as you need.

| Field | Type | Default | Description |
|---|---|---|---|
| `name` | string | required | Display name shown in the sidebar menu (e.g., `"LINDAS prod"`). |
| `url` | string | required | Full SPARQL endpoint URL. |
| `slug` | string | required | Unique, URL-safe identifier for this endpoint. See [Slugs](#slugs) below — startup fails if slugs are missing or duplicated. |
| `default` | boolean | `false` | Marks this endpoint as pre-selected when the app starts. At most one endpoint should have `default = true`. Also used as the fallback when resolving `.EndpointTag` in templates. |
| `monitor` | boolean | `false` | Enables health monitoring for this endpoint. Monitored endpoints appear on the `/monitoring` dashboard with response-time history stored in `./data/`. |
| `tag` | string | `""` | A logical group label (e.g., `"lindas"`, `"stadtzuerich"`). The tag of the currently selected endpoint is exposed to templates as `.EndpointTag`, allowing templates to conditionally show endpoint-specific content. |
| `search_provider` | string | `"stardog"` | Full-text search backend for this endpoint: `"stardog"`, `"graphdb"`, `"fuseki"` or `"sparql-query"`. Different triple stores expose FTS through different vendor predicates. |
| `export_provider` | string | auto | Overrides how named-graph export is performed: `"graphdb"`, `"gsp"` (Graph Store Protocol) or `"construct"`. Autodetected when omitted. |
| `access_token` | string | — | Bearer token for write operations (upload, graph deletion). Takes precedence over `username`/`password`. |
| `username` / `password` | string | — | Basic-auth credentials for write operations, used only when `access_token` is absent. |

### Slugs {#slugs}

The `slug` is the **only** endpoint identifier that travels over the wire. It appears
in shareable links as `?endpoint=<slug>`, in the `selectedEndpoint` cookie, and in
`/api/*` request parameters. Endpoint URLs and display names are never exposed as
identifiers.

Slugs must be unique (compared case-insensitively); `config.Load()` returns an error at startup
if any endpoint is missing a slug or two endpoints share one. A config parse error
also skips the `PORT` override (see [Environment Variables](#environment-variables)),
so check for `config loaded successfully` in the startup log if the server binds an
unexpected port.

### Example: minimal endpoint

```toml
[[application.sparqlEndpoints]]
name = "My endpoint"
url = "https://example.org/sparql"
slug = "my-endpoint"
default = true
monitor = false
tag = "myendpoint"
```

### Example: multiple endpoints with monitoring

```toml
[[application.sparqlEndpoints]]
name = "Prod"
url = "https://example.org/sparql"
slug = "prod"
default = true
monitor = true
tag = "myendpoint"

[[application.sparqlEndpoints]]
name = "Staging"
url = "https://staging.example.org/sparql"
slug = "staging"
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
| `magic_properties` | table of strings | — | Maps a `visoto:<key>` token to a property path, expanded in template queries. See [Magic properties](#magic-properties) below. |

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

### Magic properties {#magic-properties}

Different vocabularies express the same idea with different predicates. Rather than
repeating a long alternation in every template query, map a name once:

```toml
[rdf.magic_properties]
description = "rdfs:comment|schema:description|dct:description|dc:description"
```

Template queries can then write `visoto:description`, which is expanded to the
configured path wrapped in parentheses before the query is sent. Any prefixes used in
the path are declared automatically.

```sparql
SELECT ?description WHERE { ?? visoto:description ?description }
```

The key `dispLang` is reserved and cannot be used as a magic property name.

---

## `[[ontologies]]`

Each block defines a well-known ontology offered for one-click import in the Upload
dialog. Fetching one loads it into the given named graph on the active endpoint.

| Field | Type | Description |
|---|---|---|
| `name` | string | Short display label (e.g. `"SKOS"`). |
| `url` | string | Canonical ontology URL to fetch. |
| `graph` | string | Target named graph URI to load it into. |

```toml
[[ontologies]]
name  = "SKOS"
url   = "http://www.w3.org/2004/02/skos/core#"
graph = "urn:ontology:w3/skos"
```

Writing to a graph requires the active endpoint to have `access_token` or
`username`/`password` configured.

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

## Environment Variables {#environment-variables}

Two environment variables are recognized at runtime:

| Variable | Values | Description |
|---|---|---|
| `PORT` | `1`–`65535` | Overrides `application.port`. Lets a second instance run on a different port without editing the config. An out-of-range or non-numeric value is a startup error. |
| `GIN_MODE` | `debug`, `release`, `test` | Controls Gin framework verbosity. Set to `release` in production to suppress per-request debug output. The Docker Compose file sets this automatically. |

Example:
```sh
PORT=8061 GIN_MODE=release go run ./cmd/visoto/
```

> **Note:** the `PORT` override is applied at the *end* of `config.Load()`. If the
> config file fails to parse, `Load` returns before reaching it and the server falls
> back to the code default port — silently ignoring `PORT`. If a `PORT=…` run binds
> an unexpected port, look for `config loaded successfully` in the startup log.
