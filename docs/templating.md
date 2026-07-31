# Template Authoring Guide

Visoto uses Go's `html/template` package for server-side rendering. When a user visits `/resource?iri=<IRI>`, Visoto selects the best-matching template file, executes all SPARQL queries embedded in that file in parallel, and renders the result as HTML.

Templates are loaded at startup and cached in memory. **Restart the server to pick up new or changed template files.**

---

## Page Layout

Every resource page is built from the same fixed shell. The diagram below shows how the layout pieces fit together:

```
┌───────────────────────────────────────────────────────────────────────────────┐
│  Topbar  (text search, dark/light toggle, endpoint switcher, settings menu)   │
├──────────────┬──────────────────────────────────┬─────────────────────────────┤
│              │  Header                          │                             │
│              │  ┌────────────────────────────┐  │                             │
│   Sidebar    │  │ breadcrumb                 │  │      Right sidebar          │
│              │  │ icon · pageTitle (h1)      │  │                             │
│  (menu,      │  │ pageClasses  (rdf:type)    │  │         (chat)              │
│  bookmarks)  │  │ pageSubtitle               │  │                             │
│              │  └────────────────────────────┘  │                             │
│              ├──────────────────────────────────┤                             │
│              │  Page Body                       │                             │
│              │  ┌────────────────────────────┐  │                             │
│              │  │  {{ pageContent }}         │  │                             │
│              │  │  (components + partials)   │  │                             │
│              │  └────────────────────────────┘  │                             │
│              ├──────────────────────────────────┤                             │
│              │  Footer                          │                             │
└──────────────┴──────────────────────────────────┴─────────────────────────────┘
```

As a template author, the areas you control are the **Header blocks** (`pageTitle`, `pageClasses`, `pageSubtitle`, `breadcrumb`, `pageIcon`) and the **Page Body** (`pageContent`). Everything else is rendered by the layout and is not overridden per template.

The `pageHeader` component fills all the header blocks automatically from SPARQL queries — include it and you only need to write `pageContent`.

### Page Body: components and partials

Inside `pageContent`, you compose the page from components (which carry their own SPARQL queries) and partials (which display data you pass in):

```
{{ define "pageContent" }}
│
├── {{ template "literals" . }}              ← component: all literal properties
│
├── <sparql-query id="myQuery"> ... </sparql-query>   ← inline query
│
├── {{ template "sparqlTable" (dict         ← partial: tabular display
│       "result" .QueryResults.myQuery
│       "id"     "myQuery"
│       "title"  "My Data") }}
│
├── {{ template "sparqlTree" (dict          ← partial: hierarchy tree
│       "result" .QueryResults.hierarchy
│       "id"     "hierarchy") }}
│
├── {{ template "relationships" . }}         ← component: outgoing + incoming links
│
└── {{ template "classHierarchy" . }}        ← component: subclass/superclass tables
{{ end }}
```

**Components** (`literals`, `relationships`, `classHierarchy`, `ontologyShacl`, `pageHeader`) — pass `.` (full context), they issue their own SPARQL queries internally.

**Partials** (`sparqlTable`, `sparqlAsyncTable`, `sparqlGrid`, `sparqlTree`, `sparqlMermaidFlow`, `sparqlGraph`, `schemaGraph`, `sparqlMetric`) — pass `(dict ...)` with the query result and display options.

---

## Directory Layout

```
templates/
  layout/      — the fundamental page structure (base, header, footer, sidebar)
  pages/       — static page templates (home, search, monitoring, …)
  partials/    — reusable display components; called with {{ template "name" (dict ...) }}
  components/  — reusable query+display bundles; called with {{ template "name" . }}
  classes/     — one file per RDF class type, for rendering the class node itself
  instances/   — one file per RDF instance type, for rendering instances of a class
```

**Partials vs. components:**
- **Partials** (`templates/partials/`) require you to pass data explicitly via `dict`. They have no embedded SPARQL queries.
- **Components** (`templates/components/`) contain their own `<sparql-query>` blocks and accept the full `.` context. Their queries are auto-discovered and executed alongside the page's own queries.

---

## Template Resolution {#template-resolution}

When a request arrives for IRI `X`, Visoto does the following:

1. Compute the **short (prefixed) IRI** from `X` using the configured `rdf.prefixes` (e.g., `http://www.w3.org/2004/02/skos/core#Concept` → `skos:Concept`).
2. **URL-encode** both the full IRI and the short IRI using Go's `url.QueryEscape` (`:` → `%3A`, `/` → `%2F`, `#` → `%23`).
3. Look for `templates/classes/<short-IRI-encoded>.html` — use it if found.
4. Look for `templates/classes/<full-IRI-encoded>.html` — use it if found.
5. Look for `templates/instances/<short-IRI-encoded>.html` — use it if found.
6. Look for `templates/instances/<full-IRI-encoded>.html` — use it if found.
7. Issue a SPARQL query for all `rdf:type` values of `X`. Sort the types by `rdf.type_priority` (configured types first, then the rest). For each type (short then full IRI form), look for a match in `templates/instances/`.
8. Detect whether `X` is a class (it has `rdfs:Class` or `owl:Class` as its type, or has `rdfs:subClassOf` triples). Use `templates/classes/default.html` for classes, `templates/instances/default.html` for instances.
9. Hard fallback: `templates/pages/resource.html`.

The first match wins. See [configuration.md](configuration.md) for how `rdf.prefixes` and `rdf.type_priority` are configured.

---

## Filename Convention

To create a template for an RDF type, URL-encode the prefixed IRI of that type and append `.html`.

| RDF type (prefixed) | Folder | Filename |
|---|---|---|
| `skos:Concept` | `instances/` | `skos%3AConcept.html` |
| `dcat:Dataset` | `instances/` | `dcat%3ADataset.html` |
| `owl:Class` | `classes/` | `owl%3AClass.html` |
| `schema:Person` | `instances/` | `schema%3APerson.html` |

For types not in the prefix table, encode the full IRI:

| Full IRI | Filename |
|---|---|
| `https://schema.ld.admin.ch/ZefixOrganisation` | `https%3A%2F%2Fschema.ld.admin.ch%2FZefixOrganisation.html` |

**Shell helper:**
```sh
python3 -c "import urllib.parse; print(urllib.parse.quote('skos:Concept', safe=''))"
# → skos%3AConcept
```

> Note: Go's `url.QueryEscape` encodes space as `+` (not `%20`). IRIs should not contain spaces, but if yours does, use `+` in the filename.

**Class vs. instance:** Put the file in `templates/classes/` if the template renders the class node itself (i.e., `X` is the class `skos:Concept`). Put it in `templates/instances/` if the template renders individual resources that *have* this type (i.e., `X` is an instance of `skos:Concept`).

---

## Template Data Model

Every template receives a `TemplateData` value as `.`:

| Field | Type | Description |
|---|---|---|
| `.ResourceIRI` | `string` | Full IRI of the resource being rendered. |
| `.ShortIRI` | `string` | Prefixed IRI (e.g., `skos:Concept`). Empty if no prefix matched. |
| `.TemplateName` | `string` | Relative path of the selected template (e.g., `instances/skos%3AConcept.html`). Useful for debugging. |
| `.QueryResults` | `map[string]QueryResult` | Results of all `<sparql-query>` elements, keyed by their `id` attribute. |
| `.SparqlEndpoints` | `[]SparqlEndpoint` | All configured named endpoints (for building custom endpoint UI if needed). |
| `.EndpointTag` | `string` | The `tag` value of the currently selected endpoint. Empty if not set. |

### `QueryResult` fields

Accessed as `.QueryResults.<id>`:

| Field | Type | Description |
|---|---|---|
| `.Vars` | `[]string` | Variable names from the SPARQL SELECT (column names). |
| `.Bindings` | `[]map[string]Binding` | Rows of results. Each row is a map from variable name to `Binding`. |
| `.Error` | `string` | Non-empty if the query failed. |
| `.Query` | `string` | The SPARQL query that was executed (after IRI substitution). Useful for debugging. |
| `.Endpoint` | `string` | The endpoint URL that was queried. |

### `Binding` fields

| Field | Type | Description |
|---|---|---|
| `.Value` | `string` | Raw value (IRI or literal string). |
| `.Type` | `string` | `"uri"`, `"literal"`, or `"html"`. |
| `.DisplayText` | `string` | Human-readable label. For URIs, this may be a resolved label; otherwise same as `.Value`. |

Use the `render` function to output a `Binding` as HTML — it renders URIs as clickable links and literals as plain text.

---

## Embedding SPARQL Queries

SPARQL queries are embedded in template files using `<sparql-query>` custom elements:

```html
<sparql-query id="title" class="d-none">
  SELECT ?title WHERE {
    BIND (?? AS ?s)
    ?s rdfs:label ?title .
    FILTER (lang(?title) = "en" || lang(?title) = "")
  }
</sparql-query>
```

**Rules:**
- The `id` attribute is the key under which results are stored: `.QueryResults.title`.
- `??` is replaced with the current resource IRI (wrapped in `<...>`) before execution.
- All prefix declarations from `rdf.prefixes` are prepended to the query automatically.
- `class="d-none"` hides the element from browser display (Bootstrap utility class).
- All queries in a template are executed **in parallel** before rendering. Queries cannot depend on each other's results.

**Special tokens:**
- `visoto:dispLang` — substituted with the value of the request's `Accept-Language` header. Use in FILTER clauses for multilingual data:
  ```sparql
  FILTER (lang(?label) = visoto:dispLang || lang(?label) = "en" || lang(?label) = "")
  ```
- **Magic properties** — any other `visoto:<key>` token is expanded to the property path configured under `[rdf.magic_properties]` in `visoto.config`, wrapped in parentheses. With
  ```toml
  [rdf.magic_properties]
  description = "rdfs:comment|schema:description|dct:description|dc:description"
  ```
  the query `?? visoto:description ?description` becomes `?? (rdfs:comment|schema:description|dct:description|dc:description) ?description`. Because the parentheses are added automatically, the token can be used anywhere a property path is valid, including inside longer paths (`visoto:description/rdfs:label`). Prefixes used inside the expansion are declared automatically. The key `dispLang` is reserved.

**Async queries (HTMX):** For heavy aggregate queries (e.g., counts), use `<sparql-async>` instead of `<sparql-query>`. The count is loaded after the page renders via HTMX. Pair with the `sparqlMetric` partial:
```html
<sparql-async id="count" class="d-none">
  SELECT (COUNT(?x) AS ?count) WHERE { ?x a ?? . }
</sparql-async>

{{ template "sparqlMetric" (dict "queryId" "count" "title" "Instances" "icon" "list") }}
```

---

## Available Template Functions

| Function | Description | Example |
|---|---|---|
| `render` | Renders a `Binding` as HTML: URI → clickable link, literal → text. | `{{ render (index $row "label") }}` |
| `dict` | Builds a map for passing named parameters to partials. | `{{ template "sparqlTable" (dict "result" .QueryResults.foo "id" "foo") }}` |
| `resourceIcon` | Returns the icon name for the current resource. | `{{ resourceIcon . }}` |
| `iconNames` | Returns a map of all available icon names. | `{{ iconNames \| toJSON }}` |
| `toJSON` | Serializes a value to HTML-safe JSON (for `<script>` tags). | `{{ toJSON .QueryResults.foo }}` |
| `toJSONRaw` | Same as `toJSON` but without HTML-escaping `<`, `>`, `&`. Use when the JSON is passed to `encodeURIComponent`. | `{{ toJSONRaw $query }}` |
| `toJSONPretty` | Indented JSON for display. | `{{ toJSONPretty . }}` |
| `firstValue` | Returns the first `.Value` for a given variable from a `QueryResult`. Returns `""` if no results. | `{{ firstValue .QueryResults.title "title" }}` |
| `lastPathSegment` | Returns the fragment after `#`, or the last `/`-separated segment of an IRI. | `{{ lastPathSegment .ResourceIRI }}` |
| `groupByValue` | Deduplicates bindings by `value`, merging `property` labels. Used internally by the `literals` component. | `{{ groupByValue $bindings }}` |
| `t` | Translated UI string for the page's language. See [UI strings](#ui-strings). | `{{ t "table.download" }}` |
| `tHTML` | Same as `t`, but the message may contain inline markup (a link inside a sentence). | `{{ tHTML "chat.ownAi" }}` |
| `tn` | Plural form of a message, with `{{.Count}}` pre-seeded. | `{{ tn "table.rowCount" $n }}` |
| `siteLang` | The active language code (`""` for the no-language choice). | `<html lang="{{ siteLang }}">` |
| `siteLanguages` | The language picker's options (`Code`, `Label`, `Selected`), from `[[application.languages]]` — not from a catalog. | `{{ range siteLanguages }}…{{ end }}` |
| `jsStrings` | The `js.*` catalog subset, for the JSON island `static/js` reads. | `{{ toJSON jsStrings }}` |

All content rendered by `render` and `firstValue` is already HTML-safe — no need to add `| safeHTML`.

---

## UI strings {#ui-strings}

**Never write user-visible English directly in a template.** Every label, heading,
button, placeholder, `alt` and `title` goes through the message catalog:

```html
<h3 class="card-title">{{ t "card.attributes" }}</h3>
<input placeholder="{{ t "topbar.search" }}">
{{ template "sparqlTable" (dict "result" $r "title" (t "card.instances")) }}
```

`locales/en.toml` is the source catalog — it defines which keys exist. The other
`locales/<code>.toml` files translate it, key by key, and anything they omit
falls back to the English text, so a locale that is behind renders English for
the gaps rather than blanks or raw keys.

### Adding a string

1. Add the key to `locales/en.toml` with the English text. Keys are **quoted and
   flat** (`"card.attributes" = "Attributes"`) — an unquoted dotted key would be
   read by TOML as a nested table.
2. Reference it with `{{ t "key" }}`.
3. Optionally translate it in `de/fr/it/rm.toml`. Skipping this is fine.

Namespaces in use: `topbar.*`, `nav.*`, `card.*` (table/card headings), `table.*`,
`view.*`, `header.*`, `footer.*`, `chat.*`, `upload.*`, `graphs.*`, `query.*`,
`page.<name>.*` (copy belonging to one page), `type.*` (copy on a class/instance
template) and `js.*`.

`go test ./internal/i18n/` fails on a key used but not defined, a key defined but
never used, and a translation defining a key the base catalog does not have — so
a rename that misses one side is caught before it reaches the UI.

### Strings containing markup

Use `tHTML` when the message deliberately holds inline markup, so a translator
can move the link inside the sentence instead of reassembling fragments:

```toml
"chat.ownAi" = "Prefer your own AI? <a href=\"/connect.html\">Connect Visoto as an MCP server</a>"
```

Reserve it for that case: catalog content is trusted and not escaped, so routing
ordinary strings through `tHTML` would quietly disable escaping. Note also that
`<i data-lucide=...>` icons stay in the template, never in the message.

### Strings used by JavaScript

Keys under `js.` are serialized into a JSON island in `<head>` and read through
`window.vsT` (see `static/js/i18n.js`):

```js
btn.textContent = vsT('js.table.searchAll', 'Search all');
msg = vsTf('js.table.showingOf', 'Showing {n} of {total}', { n: a, total: b });
```

Always pass the English text as the second argument: it keeps the call site
readable and means a missing key degrades to correct English. `vsTf` substitutes
`{named}` placeholders, which a translation is free to reorder.

### How it works

Template functions bind at parse time, so `t` has to know its language before
anything renders. `internal/templates` parses each page's file set **once**, then
clones it per configured language and overrides only the i18n functions on each
clone (`internal/i18n.Catalogs.FuncMap`). Handlers pick the variant by name via
`templates.Name(lang, "pages/home.html")` — which is why every `c.HTML` call goes
through `renderName`. Adding a language costs clones, not parses.

---

## Available Partial Templates

Partials are called with `{{ template "<name>" (dict ...) }}`. They render a card with a title, optional icon, and content.

### `sparqlTable`

Tabular display powered by Tabulator.js. Supports search, sorting, CSV/JSON/XLSX export, and an "Execute on endpoint" link.

| Parameter | Type | Required | Description |
|---|---|---|---|
| `result` | `QueryResult` | yes | The query result to display. |
| `id` | `string` | yes | Unique identifier for the table element. |
| `title` | `string` | no | Card heading. |
| `icon` | `string` | no | Lucide icon name (e.g., `"list"`, `"users"`). |
| `collapsed` | `bool` | no | Start collapsed. Auto-collapses if the result is empty. |
| `iconVar` | `string` | no | SPARQL variable name whose URI is used to look up a resource icon image for each row. |
| `badgeVar` | `string` | no | SPARQL variable name to render as a colored badge in each row. |

```html
{{ template "sparqlTable" (dict
  "result" .QueryResults.instances
  "id" "instances"
  "title" "Instances"
  "icon" "list"
) }}
```

### `sparqlGrid`

Two-column property/value datagrid. Best for displaying an attribute list (literal properties).

Parameters: `result`, `id`, `title`, `icon`, `collapsed`.

### `sparqlTree`

Hierarchical tree display powered by Wunderbaum. The SPARQL result **must** have `?node` and `?parent` columns. An optional `?code` column is shown alongside the label.

| Parameter | Type | Default | Description |
|---|---|---|---|
| `result` | `QueryResult` | required | Must have `?node` and `?parent` columns. |
| `id` | `string` | required | Unique identifier. |
| `title` | `string` | — | Card heading. |
| `treeExpanded` | `bool` | `true` | Whether all nodes start expanded. |
| `ResourceIRI` | `string` | — | IRI of the node to scroll to and highlight on load. |

### `sparqlMermaidFlow`

Mermaid flowchart. Accepts `direction` (`"LR"` or `"TD"`).

### `sparqlAsyncTable`

A `sparqlTable` that is fetched **after** the page renders, so a slow or large query
never blocks first paint. Renders a skeleton placeholder that HTMX swaps with a full
table fragment from `/api/async-table/:queryId`.

Unlike the other partials, the query is not passed in as a result — it is declared
separately in a `<sparql-async id="...">` element on the same page, and executed only
when the placeholder is triggered.

| Parameter | Type | Default | Description |
|---|---|---|---|
| `queryId` | `string` | required | Must match the `id` of a `<sparql-async>` element. |
| `iri` | `string` | — | Resource IRI, substituted for `??` in the query. |
| `title` / `icon` | `string` | — | Card heading and Lucide icon. |
| `iconVar` / `badgeVar` | `string` | — | As in `sparqlTable`. |
| `groupBy` | `string` | — | SPARQL variable the table is initially grouped by. |
| `collapsed` | `bool` | `false` | Start the loaded card collapsed. |
| `trigger` | `string` | `"load"` | HTMX trigger. Pass a custom event (e.g. `"showData"`) to defer until the view is first revealed. |
| `searchProp` | `string` | — | Name property IRI used by the working-set "Search all" rebuild. |
| `max` | `int` | `20000` | Working-set row cap for large classes. |
| `facetFor` | `string` | — | Set to this table's own `queryId` to enable faceted search. |

```html
<sparql-async id="myTable" class="d-none">
  SELECT ?s ?p WHERE { ?s a ?? ; ?p ?o }
</sparql-async>

{{ template "sparqlAsyncTable" (dict
  "queryId" "myTable"
  "iri"     .ResourceIRI
  "title"   "Instances"
  "icon"    "database"
) }}
```

**Working-set mode is auto-detected.** A class-instance query whose class exceeds the
size threshold is rendered as a bounded fetch with all-local interaction and a "Search
all" server rebuild; everything else renders inline. Authors do not choose the mode.

**Faceted search.** Setting `facetFor` to the table's own `queryId` hangs a filter
control off each facetable column header, declared with sibling
`<sparql-facet for="..." var="...">` elements. Every faceted `var` must also be
SELECTed as a column — a facet with no matching column has no header to attach to and
is skipped with a console warning. `<sparql-facet>` is a non-void custom element, so
it always needs an explicit closing tag.

### `sparqlGraph`

Embeds an interactive RDF graph (Graph Explorer, an Ontodia fork) that draws and lays
out a set of IRIs at initialization.

| Parameter | Type | Default | Description |
|---|---|---|---|
| `id` | `string` | `"sparql-graph"` | DOM id suffix; allows multiple graphs on one page. |
| `iris` | `[]string` | — | IRIs to draw at initialization. |
| `iri` | `string` | — | Convenience alias for a single starting IRI, appended to `iris`. |
| `endpointUrl` | `string` | fallback | SPARQL endpoint URL — pass `.EndpointURL`. |
| `height` | `string` | `"calc(100vh - 200px)"` | CSS height of the container. |
| `lazy` | `bool` | `false` | Defer initialization until a `graph:init` event fires on the `-root` element. |

The partial self-loads the Graph Explorer CDN bundle, guarded so it loads only once
per page. The `?iri=` URL parameter is honored as an additional starting IRI.

### `schemaGraph`

Reconstructs an informal schema from the data actually present around a resource and
renders it as a UML-style class diagram — class boxes with datatype-attribute rows,
object properties as labeled edges.

Takes the same `id` / `iri` / `endpointUrl` / `height` / `lazy` / `title` / `icon`
parameters as `sparqlGraph`. The mode is auto-detected client-side: for a class it
derives the schema from a sample of up to 50 instances; for an instance it derives it
from that one resource, anchored on a detected class.

```html
{{ template "schemaGraph" (dict "iri" .ResourceIRI "endpointUrl" .EndpointURL "lazy" true) }}
```

### `sparqlMetric`

Async metric card, loaded via HTMX after the page renders. Pairs with `<sparql-async>`.

| Parameter | Description |
|---|---|
| `queryId` | Must match the `id` of a `<sparql-async>` element. |
| `title` | Card label. |
| `href` | Optional link URL for the metric. |
| `icon` | Lucide icon name. |

---

## Available Component Templates

Components are called with `{{ template "<name>" . }}` — pass the full `.` context. They contain their own `<sparql-query>` blocks which are auto-discovered and executed alongside the page's queries.

| Component | Description |
|---|---|
| `pageHeader` | Standard page header: queries for title, subtitle, and `rdf:type` list. Defines the `pageTitle`, `metaTitle`, `metaIcon`, `pageSubtitle`, and `pageClasses` blocks. **Include this in almost every template.** |
| `literals` | Renders all literal (non-IRI) properties of the resource in a `sparqlGrid`. |
| `classHierarchy` | Renders superclass and subclass tables. Best suited for class templates. |
| `relationships` | Renders outgoing and incoming IRI-valued relationship tables (collapsed by default). |
| `ontologyShacl` | Renders SHACL shape constraints. Suited for ontology/class templates. |

---

## Layout Blocks

The base layout (`templates/layout/base.html`) provides named blocks that page templates can override with `{{ define "blockName" }}...{{ end }}`:

| Block | Description |
|---|---|
| `pageTitle` | Page heading (rendered as H1). |
| `metaTitle` | Browser tab title. |
| `metaIcon` | Resource icon shown in the header. |
| `pageSubtitle` | Subtitle displayed below the heading. |
| `pageClasses` | `rdf:type` list shown below the title. |
| `breadcrumb` | Breadcrumb trail (`<li>` elements). |
| `pageContent` | Main content area. **Required — every template must define this.** |

The `pageHeader` component defines `pageTitle`, `metaTitle`, `metaIcon`, `pageSubtitle`, and `pageClasses` automatically from SPARQL queries — include it and you only need to define `breadcrumb` and `pageContent`.

---

## Step-by-Step: Creating a Template for `schema:Person`

This walks through creating `templates/instances/schema%3APerson.html`.

### Step 1: Compute the filename

The RDF type is `schema:Person`. Assuming `schema:` is in `rdf.prefixes`:

```sh
python3 -c "import urllib.parse; print(urllib.parse.quote('schema:Person', safe=''))"
# → schema%3APerson
```

Filename: `schema%3APerson.html`. This is an instance template (for resources that *are* a `schema:Person`), so place it in `templates/instances/`.

### Step 2: Write the template file

```html
{{ template "pageHeader" . }}

{{ define "breadcrumb" }}
  <li class="breadcrumb-item">
    <a href="/resource?iri=schema%3APerson">Person</a>
  </li>
{{ end }}

<sparql-query id="colleagues" class="d-none">
  SELECT ?colleague ?name WHERE {
    ?? schema:colleague ?colleague .
    OPTIONAL { ?colleague schema:name ?name . }
  }
</sparql-query>

{{ define "pageContent" }}
  {{ template "literals" . }}

  {{ template "sparqlTable" (dict
    "result" .QueryResults.colleagues
    "id" "colleagues"
    "title" "Colleagues"
    "icon" "users")
  }}

  {{ template "relationships" . }}
{{ end }}
```

### Step 3: Restart the server

```sh
go run ./cmd/visoto/
```

Visit `/resource?iri=<IRI-of-a-person>` — the new template renders automatically.

### Step 4: Conditionally show endpoint-specific content

```html
{{ if eq .EndpointTag "lindas" }}
  <a href="https://ld.admin.ch/...">Open in LINDAS</a>
{{ end }}
```

See [configuration.md#named-endpoints](configuration.md#named-endpoints) for how `tag` is configured on endpoints.

---

## Tips

- Check `.QueryResults.<id>.Error` in your template to show a friendly message when a query fails.
- Use `{{ .TemplateName }}` in a comment or debug block to confirm which template is being rendered.
- Use `logging.level = "DEBUG"` in your config to see which template Visoto selected and why.
- The `default.html` templates in `classes/` and `instances/` are good starting points — copy and customize one.
