---
name: instanceTemplate
description: Generate Visoto template files for RDF instances showing attributes, relationships, and connections
---

# Instance Template Generator

Generate HTML template files for RDF instances in Visoto. Instance templates display attributes (literals), incoming/outgoing relationships, and custom sections specific to the instance type.

## Usage

### Interactive Mode (No Arguments)
```bash
/instanceTemplate
```
You'll be prompted for:
- Instance URI or class type
- Template complexity level
- Breadcrumb hierarchy (optional)
- Custom sections to include

### Argument Mode
```bash
/instanceTemplate <uri> [options]
```

**Examples:**
```bash
# Basic instance template
/instanceTemplate schch:Municipality

# With full URI
/instanceTemplate https://schema.ld.admin.ch/Canton

# Basic template (attributes and relationships only)
/instanceTemplate schch:Municipality --basic

# Standard template with breadcrumbs
/instanceTemplate schch:Municipality --breadcrumbs "schema:Country,schch:Canton,schch:District"

# Advanced template with custom sections
/instanceTemplate skos:Concept --advanced

# Update existing template (creates backup)
/instanceTemplate schch:Municipality --update

# Preview without writing
/instanceTemplate schch:Municipality --preview
```

## Arguments

- **uri** (required): RDF instance URI or class type (short form like `schch:Canton` or full URI)
- **-b, --basic**: Generate minimal template (attributes and relationships only)
- **-s, --standard**: Generate standard template with common sections (default)
- **-a, --advanced**: Include custom visualizations and advanced features
- **--breadcrumbs "uri1,uri2,..."**: Comma-separated list of breadcrumb URIs
- **-u, --update**: Update existing template (creates `.backup` file first)
- **-p, --preview**: Show template content without writing file

## Template Structure

### Basic Template (`--basic`)
- Title, subtitle, and metadata queries
- **Attributes** (all literal values using `sparqlGrid`)
- **Incoming relationships** (resources linking to this instance)
- **Outgoing relationships** (resources this instance links to)

### Standard Template (default)
- All basic sections
- Custom sections based on common patterns detected
- Example: postal codes, stations, organizations for municipalities

### Advanced Template (`--advanced`)
- All standard sections
- Mermaid flow diagrams (version flow, hierarchies)
- Custom icons/images (e.g., Wikidata integration)
- Interactive visualizations

## Features

- **SPARQL Introspection**: Automatically queries the SPARQL endpoint to discover instance properties and relationships
- **URL Encoding**: Automatically encodes instance URI for filename (e.g., `schch:Canton` → `schch%3ACanton.html`)
- **Backup on Update**: Creates `.backup` file before overwriting existing templates
- **Example Comments**: Includes helpful comments showing how to customize queries
- **Partial Integration**: Uses the more basic Visoto partials (`sparqlTable`, `sparqlGrid`, `sparqlTree`, `sparqlMetric`, `sparqlMermaidFlow`)
- **Component Integration**: uses the pre-built, ready to use building blocks (`pageHeader`, `literals`, `relationships`, `classHierarchy`, `ontologyShacl`)
- **Smart Filtering**: Excludes class/subclass relationships (no `rdf:type`, `rdfs:subClassOf`)

## Configuration

The skill reads SPARQL endpoint configuration from `visoto.config`:
```toml
[application]
sparqlEndpoint = "https://ld.admin.ch/query/"
```

## Output Location

Templates are created in:
```
/templates/instances/{encoded-uri}.html
```

## SPARQL Query Introspection

The skill automatically runs SPARQL queries to:
1. **Discover attributes**: Find all literal properties of the instance
2. **Analyze relationships**: Identify common incoming/outgoing relationship patterns
3. **Detect patterns**: Look for domain-specific patterns (geographic, organizational, etc.)
4. **Suggest sections**: Recommend relevant custom sections based on detected patterns

This information is displayed before creating the template and used to generate sensible default queries.

## Instance vs Class Templates

**Instance templates do NOT include:**
- Instances section (that's for class templates)
- Subclasses/Superclasses sections
- Schema information (domain/range)
- Class hierarchy visualizations

**Instance templates focus on:**
- The specific resource's attributes
- Connections to other resources
- Domain-specific sections (stations, organizations, etc.)
- Version/history information if applicable

## Tips

- Use short form URIs (e.g., `schch:Municipality`) when the prefix is defined in `visoto.config`
- The `--preview` flag is useful for testing before committing to file creation
- Breadcrumbs should show the hierarchical path to this instance (Country → Canton → District → Municipality)
- The skill will warn if the URI doesn't return any data from the endpoint
- For geographic entities, consider adding map visualizations
- For versioned entities, consider adding Mermaid flow diagrams

## Common Custom Sections

Based on instance type, the skill may suggest:
- **Geographic**: Maps, contained places, postal codes
- **Organizations**: Related organizations, legal forms, registrations
- **Concepts**: Broader/narrower concepts, related schemes
- **Versioned**: Version flow diagrams, change events, predecessors/successors

## Workflow

1. **Query SPARQL endpoint** to gather information about the instance
2. **Display findings** (attributes, relationships, detected patterns)
3. **Generate template** with appropriate SPARQL queries and partials
4. **Create/update file** with backup if updating
5. **Show success message** with file path and next steps

---

When invoked, the skill will use the parameters provided or enter interactive mode to gather necessary information, then generate an appropriate instance template file.
