package mcp

// Pre-built SPARQL query templates used by MCP tools.
// Prefix injection is handled automatically by the sparql.Preprocessor.

const queryDiscoverClasses = `
SELECT ?type (COUNT(?s) AS ?count)
WHERE { ?s a ?type }
GROUP BY ?type
ORDER BY DESC(?count)
LIMIT %d
`

const queryDiscoverProperties = `
SELECT ?p (COUNT(*) AS ?uses)
WHERE { ?s ?p ?o }
GROUP BY ?p
ORDER BY DESC(?uses)
LIMIT %d
`

const queryGetResource = `
SELECT ?p ?o
WHERE { <%s> ?p ?o }
ORDER BY ?p
`

const querySearchByLabel = `
SELECT DISTINCT ?s ?label ?type
WHERE {
  ?s rdfs:label|skos:prefLabel ?label .
  FILTER(CONTAINS(LCASE(STR(?label)), LCASE("%s")))
  OPTIONAL { ?s a ?type }
}
LIMIT %d
`

const querySearchByLabelWithType = `
SELECT DISTINCT ?s ?label
WHERE {
  ?s rdfs:label|skos:prefLabel ?label .
  FILTER(CONTAINS(LCASE(STR(?label)), LCASE("%s")))
  ?s a <%s> .
}
LIMIT %d
`

const queryCountInstances = `
SELECT ?type (COUNT(?s) AS ?count)
WHERE { ?s a ?type }
GROUP BY ?type
ORDER BY DESC(?count)
LIMIT 100
`

const queryCountInstancesForClass = `
SELECT (COUNT(?s) AS ?count)
WHERE { ?s a <%s> }
`

const queryCheckEndpoint = `ASK {}`

// GraphDB system predicate: lists all named graphs in ~0.3s on LINDAS.
// Non-GraphDB stores match nothing (0 rows) — the handler falls back to
// queryListNamedGraphsPortable. Full IRI on purpose — avoids depending on
// a configured onto: prefix.
const queryListNamedGraphsGraphDB = `
SELECT ?graph
WHERE { ?s <http://www.ontotext.com/graphs> ?graph }
LIMIT %d
`

// Portable fallback. The rdf:type restriction is load-bearing: an
// unrestricted { ?s ?p ?o } scan times out on large stores.
const queryListNamedGraphsPortable = `
SELECT DISTINCT ?graph
WHERE { GRAPH ?graph { ?s a ?t } }
LIMIT %d
`

// GraphDB statistics-based triple count for one named graph (~0.16s).
// Approximate (index statistics); GraphDB-only — other stores return wrong
// results (QLever: 0) or errors (Virtuoso: HTTP 400), so never run it on
// the fallback path.
const queryGraphTripleCountGraphDB = `
SELECT (COUNT(*) AS ?triples)
FROM <http://www.ontotext.com/owlim/system#statistics>
WHERE { GRAPH <%s> { ?s ?p ?o } }
`

// Graph-scoped variant of queryDiscoverClasses (<1s even on huge graphs).
const queryDiscoverClassesInGraph = `
SELECT ?type (COUNT(?s) AS ?count)
WHERE { GRAPH <%s> { ?s a ?type } }
GROUP BY ?type
ORDER BY DESC(?count)
LIMIT %d
`
