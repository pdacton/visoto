package parser

import (
	"hutzli.org/visoto/internal/config"
	"hutzli.org/visoto/internal/sparql"
)

// TemplateData wraps all data retrieved for a resource
type TemplateData struct {
	ResourceIRI     string                  // The IRI of the resource being rendered
	ShortIRI        string                  // Prefixed IRI (e.g. schema:Person), empty if no prefix match
	QueryResults    map[string]sparql.QueryResult // Results indexed by query ID defined in template
	SparqlEndpoints []config.SparqlEndpoint // SPARQL endpoints for menu (no sensitive data)
}
