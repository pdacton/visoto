package sparql

import (
	"html/template"
	"net/url"
	"time"

	"hutzli.org/visoto/internal/config"
)

// Binding represents a single SPARQL query result value
type Binding struct {
	Type        string
	Value       string
	DisplayText string // human-readable display string (resolved label for URIs, literal value otherwise)
}

// ResourceHref returns the canonical Visoto page URL for an IRI,
// using the query-param form: /resource?iri=<escaped-iri>.
func ResourceHref(iri string) string {
	return "/resource?iri=" + url.QueryEscape(iri)
}

// RenderHTML returns an HTML link if the type is "uri", raw HTML if the type is "html",
// otherwise returns escaped plain text
func (b Binding) RenderHTML() template.HTML {
	switch b.Type {
	case "uri":
		return template.HTML(`<a href="` + ResourceHref(b.Value) + `">` + template.HTMLEscapeString(b.DisplayText) + `</a>`)
	case "html":
		return template.HTML(b.DisplayText)
	default:
		return template.HTML(template.HTMLEscapeString(b.DisplayText))
	}
}

// QueryInput holds the inputs for SPARQL query processing:
// endpoint resolution, prefix substitution, and query execution.
type QueryInput struct {
	EndpointURL     string            // Default SPARQL endpoint URL
	Timeout         time.Duration     // Global timeout for all queries
	Prefixes        []config.Prefix   // RDF prefix declarations
	NamedEndpoints  map[string]string // Named endpoints: name -> URL mapping
	MagicProperties map[string]string // visoto:<key> tokens expanded to property paths
}

// QueryResult stores simplified SPARQL results for template consumption
type QueryResult struct {
	Vars     []string             // Variable names from head.vars
	Bindings []map[string]Binding // bindings: value and type
	Error    string               // Error message if query failed
	Query    string               // The finalized SPARQL query sent to endpoint (with PREFIXes)
	Endpoint string               // The endpoint URL used for this query
	// Icons maps an IRI to its resource icon path, for the results that asked for
	// type resolution (see WithTypes). It is a side map rather than a field on
	// Binding because Binding is serialized for every cell of every row, while
	// this holds one entry per *distinct* IRI. Nil when nothing resolved.
	Icons map[string]string `json:",omitempty"`
}

// ExtractedQuery represents a SPARQL query found in template
type ExtractedQuery struct {
	ID            string // Query identifier
	Query         string // SPARQL query text
	ResolveLabels bool   // Whether to perform IRI label enrichment
	Endpoint      string // Optional endpoint override from template attribute
	// ResolveTypes requests the rdf:type lookup that resolves row icons. Set from
	// the query's own <sparql-column icon> declarations, so the extra round trip
	// is paid only by a table that will actually render icons — the same gate
	// queryOptions applies on the async path.
	ResolveTypes bool
}

// queryExecutionResult holds the result of executing one query (internal use)
type queryExecutionResult struct {
	ID     string
	Result QueryResult
}

// sparqlResponse matches the JSON response structure from SPARQL endpoint
type sparqlResponse struct {
	Head struct {
		Vars []string `json:"vars"`
	} `json:"head"`
	Results struct {
		Bindings []map[string]struct {
			Type  string `json:"type"`
			Value string `json:"value"`
		} `json:"bindings"`
	} `json:"results"`
}
