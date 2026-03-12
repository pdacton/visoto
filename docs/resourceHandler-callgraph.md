# `resourceHandler` Call Graph

```mermaid
flowchart TD
    A["GET /resource/*path\n(Gin router)"] --> B["resourceHandler(c)"]

    B --> C["getSelectedEndpoint(c)\n(reads cookie)"]
    B --> D["resource.New(iri, prefixes)\n→ expandPrefixedIRI / shortenIRI"]
    B --> E["createPreprocessorForRequest(c)"]
    E --> C
    E --> F["sparql.New(Config)\n→ &Preprocessor{}"]

    B --> G["r.ResolveTemplate(preprocessor, typePriority, prefixes)"]
    G --> G1["normalizeToFilename(iri)"]
    G --> G2["tryTemplate(dir, name)\n→ templateExists(path) → os.Stat"]
    G --> G3["preprocessor.QueryTypes(iri)"]
    G3 --> H1["querySparqlEndpoint(endpointURL, query)"]
    H1 --> H2["finalizeQuery(query)\n→ extractDeclaredPrefixes\n→ extractUsedPrefixes\n→ buildNeededPrefixBlock"]
    H1 --> H3["http.Client.Do(POST)\n→ SPARQL endpoint"]
    G3 --> G4["sortTypesByPriority(types, priority)"]
    G --> G5["Fallback: pages/resource.html"]

    B --> I["preprocessor.ProcessTemplateFile(path, iri, lang)"]
    I --> I1["os.ReadFile(templatePath)"]
    I --> I2["extractQueriesDOM(content)"]
    I2 --> I3["extractElements(content)\n→ html.Parse\n→ walk DOM\n→ parseElement\n→ extractTextContent"]
    I2 --> I4["elem.AsQuery()"]
    I --> I5["strings.ReplaceAll(?? → IRI)"]
    I --> I6["executeQueriesParallel(endpointURL, queries, timeout, lang)"]
    I6 --> I7["goroutine × N queries"]
    I7 --> I8["ExecuteQuery(query, resolveLabels, lang, endpoint)"]
    I8 --> I9["resolveEndpoint(endpoint)"]
    I8 --> H1
    I8 --> I10["simplifyBindings(sparqlResp)"]
    I8 --> I11["if resolveLabels:\n  initLabelCache\n  extractIRIs\n  fetchLabels\n  enrichWithLabels"]

    B --> J["c.HTML(200, templateName, r.Data)\n→ Gin render"]
```

## Key Stages

1. **Resource creation** — parse/validate IRI, expand/shorten prefix (`resource.New`)
2. **Template resolution** — filesystem lookup: `classes/` → `instances/` → RDF type query → fallback `pages/resource.html`
3. **Template processing** — read file → parse DOM → extract `<sparql-query>` elements → execute in parallel → optional label enrichment
4. **Render** — pass `TemplateData` to Gin's HTML renderer (`c.HTML`)

## Source Locations

| Function | File |
|---|---|
| `resourceHandler` | `cmd/visoto/main.go:93` |
| `createPreprocessorForRequest` | `cmd/visoto/main.go:60` |
| `resource.New` | `internal/resource/resource.go:32` |
| `ResolveTemplate` | `internal/resource/resource.go:80` |
| `ProcessTemplateFile` | `internal/sparql/template.go:26` |
| `extractQueriesDOM` | `internal/sparql/template.go:182` |
| `extractElements` | `internal/sparql/template.go:101` |
| `executeQueriesParallel` | `internal/sparql/query.go:282` |
| `ExecuteQuery` | `internal/sparql/query.go:337` |
| `querySparqlEndpoint` | `internal/sparql/query.go:140` |
| `finalizeQuery` | `internal/sparql/query.go:110` |
| `QueryTypes` | `internal/sparql/query.go:254` |
