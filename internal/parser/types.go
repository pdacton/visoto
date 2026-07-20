package parser

import (
	"hutzli.org/visoto/internal/config"
	"hutzli.org/visoto/internal/sparql"
)

// NamedGraph is a named graph containing the resource (as subject)
type NamedGraph struct {
	IRI   string // full graph IRI (used by the copy button)
	Short string // prefixed form for display, falls back to full IRI
}

// TemplateData wraps all data retrieved for a resource
type TemplateData struct {
	ResourceIRI          string                        // The IRI of the resource being rendered
	ShortIRI             string                        // Prefixed IRI (e.g. schema:Person), empty if no prefix match
	NamedGraphs          []NamedGraph                  // Named graphs containing the resource as subject
	TemplateName         string                        // Name of the template used to render this page (for debugging)
	QueryResults         map[string]sparql.QueryResult // Results indexed by query ID defined in template
	SparqlEndpoints      []config.SparqlEndpoint       // SPARQL endpoints for menu (no sensitive data)
	SelectedEndpointName string                        // Name of the active endpoint, so the topbar can server-render <option selected> (don't rely on JS/cookie timing)
	SelectedEndpointSlug string                        // Slug of the active endpoint — the only identifier used on the wire (?endpoint=, cookie)
	EndpointTag          string                        // Tag of the currently selected endpoint (e.g. "lindas", "stadtzuerich")
	EndpointURL          string                        // Resolved URL of the currently selected endpoint (for client-side use, e.g. Graph Explorer)
	BaseURL              string                        // Public base URL of this instance derived from the request (e.g. https://visoto.example.org)
}
