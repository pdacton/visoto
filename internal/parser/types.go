package parser

import (
	"strings"

	"hutzli.org/visoto/internal/config"
	"hutzli.org/visoto/internal/sparql"
)

// NamedGraph is a named graph containing the resource (as subject)
type NamedGraph struct {
	IRI   string // full graph IRI (used by the copy button)
	Short string // prefixed form for display, falls back to full IRI
}

// PagePrefix is one prefix declaration as the browser receives it (the
// vs-prefixes island in base.html, read by static/js/prefixes.js).
type PagePrefix struct {
	Name string `json:"name"`
	IRI  string `json:"iri"` // without angle brackets
}

// NewPagePrefixes converts config prefixes for the browser.
func NewPagePrefixes(prefixes []config.Prefix) []PagePrefix {
	out := make([]PagePrefix, len(prefixes))
	for i, p := range prefixes {
		out[i] = PagePrefix{Name: p.Name, IRI: strings.Trim(p.URI, "<>")}
	}
	return out
}

// TemplateData wraps all data retrieved for a resource
type TemplateData struct {
	ResourceIRI          string                        // The IRI of the resource being rendered
	Title                string                        // The resource's single best label (language first, then property priority; IRI last segment as fallback). Feeds the page title.
	Subtitle             string                        // The resource's single best description, "" if none. Feeds the page subtitle.
	ShortIRI             string                        // Prefixed IRI (e.g. schema:Person), empty if no prefix match
	NamedGraphs          []NamedGraph                  // Named graphs containing the resource as subject
	TemplateName         string                        // Name of the template used to render this page (for debugging)
	TemplateType         string                        // IRI of the class this page is rendered as; "" when the template matched on the resource's own IRI or nothing matched. The breadcrumb's default segment.
	QueryResults         map[string]sparql.QueryResult // Results indexed by query ID defined in template
	SparqlEndpoints      []config.SparqlEndpoint       // SPARQL endpoints for menu (no sensitive data)
	SelectedEndpointName string                        // Name of the active endpoint, so the topbar can server-render <option selected> (don't rely on JS/cookie timing)
	SelectedEndpointSlug string                        // Slug of the active endpoint — the only identifier used on the wire (?endpoint=, cookie)
	EndpointTag          string                        // Tag of the currently selected endpoint (e.g. "lindas", "stadtzuerich")
	SearchProvider       string                        // search_provider of the currently selected endpoint (e.g. "graphdb-lucene", "qlever") — doubles as the engine hint for engine-specific queries
	GraphQueryURL        string                        // Same-origin proxy path for browser-side SPARQL (Graph Explorer, schema diagram). NOT the endpoint URL: the upstream host and its credentials deliberately never reach the browser. See internal/sparqlproxy.
	LabelPath            string                        // The "label" magic property path (Graph Explorer's label property)
	EndpointPrefixes     []PagePrefix                  // Prefixes in effect for the active endpoint (global merged with its overrides); the only namespace source for browser JS
	BaseURL              string                        // Public base URL of this instance derived from the request (e.g. https://visoto.example.org)
}
