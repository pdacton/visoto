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
