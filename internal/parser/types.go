package parser

import (
	"hutzli.org/visoto/internal/config"
	"hutzli.org/visoto/internal/sparql"
)

// TemplateData wraps all data retrieved for a resource
type TemplateData struct {
	ResourceIRI     string                        // The IRI of the resource being rendered
	ShortIRI        string                        // Prefixed IRI (e.g. schema:Person), empty if no prefix match
	TemplateName    string                        // Name of the template used to render this page (for debugging)
	QueryResults    map[string]sparql.QueryResult // Results indexed by query ID defined in template
	SparqlEndpoints []config.SparqlEndpoint       // SPARQL endpoints for menu (no sensitive data)
	EndpointTag     string                        // Tag of the currently selected endpoint (e.g. "lindas", "stadtzuerich")
	EndpointURL     string                        // Resolved URL of the currently selected endpoint (for client-side use, e.g. Graph Explorer)
}
