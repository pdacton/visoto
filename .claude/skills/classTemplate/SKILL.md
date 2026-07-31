---
name: classTemplate
description: Generate Visoto template files for RDF classes showing instances and class hierarchy
---

# Class Template Generator

Generate HTML template files for RDF classes in Visoto. Class templates display instances of the class, along with class hierarchy information and schema details.

## Usage

### Interactive Mode (No Arguments)
```bash
/classTemplate
```
You'll be prompted for:
- Class URI
- Template complexity level
- Breadcrumb hierarchy (optional)

### Argument Mode
```bash
/classTemplate <uri> [options]
```

**Examples:**
```bash
# Basic class template
/classTemplate schch:Municipality

# With full URI
/classTemplate https://schema.ld.admin.ch/Canton

# Basic template (minimal sections)
/classTemplate schch:Municipality --basic

# Standard template with breadcrumbs
/classTemplate schch:Municipality --breadcrumbs "schema:Country,schch:Canton"

# Advanced template with visualizations
/classTemplate skos:Concept --advanced

# Update existing template (creates backup)
/classTemplate schch:Municipality --update

# Preview without writing
/classTemplate schch:Municipality --preview
```

## Arguments

- **uri** (required): RDF class URI (short form like `schch:Canton` or full URI)
- **-b, --basic**: Generate minimal template (instances only)
- **-s, --standard**: Generate standard template with common sections (default)
- **-a, --advanced**: Include custom visualizations and advanced features
- **--breadcrumbs "uri1,uri2,..."**: Comma-separated list of breadcrumb URIs
- **-u, --update**: Update existing template (creates `.backup` file first)
- **-p, --preview**: Show template content without writing file

## Template Structure

### Basic Template (`--basic`)
- Title, subtitle, and metadata queries
- **Instances list** (primary focus) — rendered with `sparqlAsyncTable`
- Subclasses section, collapsed
- Superclasses section, collapsed

### Standard Template ('--standard', default)
- Title, subtitle, and metadata queries
- **Instances list** with relevant properties — rendered with `sparqlAsyncTable`
- Subclasses section
- Superclasses section

> **Instances table:** always use the `sparqlAsyncTable` partial for the instances
> list. It lazy-loads via HTMX and uses the auto-detected working-set model (one
> bounded fetch, all interactions local) so the page stays fast even for very large
> classes (e.g. `dwcFP:TaxonName`, ~918k instances). A plain `sparqlTable` would
> try to load every instance and choke on the big graphs. All existing class
> templates in `templates/classes/` use `sparqlAsyncTable`.

### Advanced Template (`--advanced`)
- All standard sections
- Schema information (domain/range, equivalentClass, etc.)
- SHACL constraints
- Custom visualization placeholders

## Features

- **SPARQL Introspection**: Automatically queries the SPARQL endpoint to discover class properties, using the visoto MCP server with endpoint configuration from `visoto.config`, use curl as fallback only
- **URL Encoding**: Automatically encodes class URI for filename (e.g., `schch:Canton` → `schch%3ACanton.html`)
- **Backup on Update**: Creates `.backup` file before overwriting existing templates
- **Example Comments**: Includes helpful comments showing how to customize queries
- **Partial Integration**: Uses the Visoto partials — `sparqlAsyncTable` (lazy HTMX working-set table; the default for the instances list), `sparqlTable`, `sparqlGrid`, `sparqlTree`, `sparqlMetric`, `sparqlMermaidFlow`, and the graph-embedding partials `sparqlGraph` / `schemaGraph`
- **Component Integration**: uses the pre-built, ready to use building blocks (`pageHeader`, `literals`, `relationships`, `classHierarchy`, `ontologyShacl`)

## UI strings must be translated

Every user-visible string the generated template emits — card titles, headings,
button labels, `placeholder`, `alt`, `title` — must go through the message
catalog, never be written as literal English:

```html
{{ template "sparqlTable" (dict "result" $r "title" (t "card.instances")) }}
<h3 class="card-title">{{ t "card.attributes" }}</h3>
```

So for each string the template needs:

1. Reuse an existing key if one fits. `card.attributes`,
   `card.incomingRelationships`, `card.outgoingRelationships`,
   `card.instancesOfThisClass`, `card.subclasses`, `card.superclasses`,
   `card.hierarchy` and most common headings already exist — check
   `locales/en.toml` before inventing a key.
2. Otherwise add a new key to `locales/en.toml` with the English text. Keys are
   quoted and flat (`"type.municipality.postalCodes" = "Postal Codes"`); use the
   `card.*` namespace for a heading reusable across templates, and
   `type.<typeName>.*` for copy specific to this one type.
3. Optionally translate it in `locales/de.toml`, `fr.toml`, `it.toml`. Skipping
   this is fine — untranslated keys fall back to the English text.

`go test ./internal/i18n/` fails if a template uses a key that is not defined, or
defines a key nothing uses, so a generated template with a missing key is caught
immediately. See `docs/templating.md#ui-strings`.


## Configuration

The skill reads SPARQL endpoint configuration from `visoto.config`:
```toml
[application]
sparqlEndpoint = "https://ld.admin.ch/query/"
```

## Output Location

Templates are created in:
```
/templates/classes/{encoded-uri}.html
```

## SPARQL Query Introspection

### Preparation: analyze a sample instance

Before generating the template, **fetch and analyze a real instance of the class**
using the visoto MCP server (curl as fallback only):

1. Use `search_by_label` (with `type_filter` set to the class IRI) or a small
   `run_sparql_query` (`SELECT ?s WHERE { ?s a <class> } LIMIT 1`) to find one
   instance IRI.
2. Call `get_resource` on that IRI to see its actual properties, values, and
   relationships. This shows you what the instances really look like — which
   attributes are populated, which carry a human-readable label, an identifier,
   a status, a description, etc.

Use this sample to inform the column choices below, rather than guessing from the
class IRI alone.

The skill also runs SPARQL queries to:
1. **Discover properties**: Find common properties used by instances of the class
2. **Count instances**: Estimate the number of instances
3. **Check hierarchy**: Detect subclasses and superclasses
4. **Suggest columns**: Recommend relevant columns for the instances table

### Choosing columns for the instances table

From the sample instance and the discovered properties, **pick up to seven
attributes that best describe an instance** for the `sparqlAsyncTable`. Favor the
attributes a person would use to recognize and tell apart instances, typically:

- a **label / name** (almost always include this as the first column),
- a **description** (e.g. `visoto:description`),
- an **identifier / ID** (registry number, code, IRI fragment),
- a **status** or lifecycle state (active/dissolved, valid/invalid, …),
- a **type / category** that subdivides the class,
- a **date** (created, valid-from, modified),
- one or two other **high-signal, well-populated** attributes.

Prefer properties that are populated across most instances (a per-predicate
`COUNT` is a good proxy for how useful a column is) and skip sparse or purely
technical ones. Cap the table at **seven columns** for readability — additional
attributes belong on the instance page, not the class table.

This information is displayed before creating the template and used to generate sensible default queries.

## Tips

- Use short form URIs (e.g., `schch:Canton`) when the prefix is defined in `visoto.config`
- The `--preview` flag is useful for testing before committing to file creation
- Breadcrumbs should be ordered from general to specific (e.g., Country → Canton → District), use use rdf:subClassOf relationships for automatic breadcrumb generation
- The skill will warn if the URI doesn't return any data from the endpoint

## Workflow

1. **Analyze a sample instance** — fetch one instance of the class via the visoto
   MCP server (`get_resource`) to see its real properties and values
2. **Query SPARQL endpoint** to gather information about the class (property
   frequency, instance count, hierarchy)
3. **Choose instance-table columns** — pick up to seven attributes that best
   describe an instance (label, description, ID, status, …)
4. **Display findings** (instance count, chosen columns, hierarchy)
5. **Generate template** with appropriate SPARQL queries and partials
6. **Create/update file** with backup if updating
7. **Show success message** with file path and next steps

---

When invoked, the skill will use the parameters provided or enter interactive mode to gather necessary information, then generate an appropriate class template file.
