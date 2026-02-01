package sparql

import (
	"time"

	"hutzli.org/visoto/internal/config"
)

// Config holds configuration for the SPARQL preprocessor
type Config struct {
	EndpointURL string          // SPARQL endpoint URL
	Timeout     time.Duration   // Global timeout for all queries
	Prefixes    []config.Prefix // RDF prefix declarations
}

// QueryResult stores simplified SPARQL results for template consumption
type QueryResult struct {
	Vars     []string             // Variable names from head.vars
	Bindings []map[string]Binding // bindings: value and type
	Error    string               // Error message if query failed
	Query    string               // The finalized SPARQL query sent to endpoint (with PREFIXes)
}

// PageData wraps all data retrieved for a resource
type TemplateData struct {
	ResourceIRI  string                 // The IRI of the resource being rendered
	QueryResults map[string]QueryResult // Results indexed by query ID defined in template
}

// extractedQuery represents a SPARQL query found in template (internal use)
type extractedQuery struct {
	ID            string // Query identifier
	Query         string // SPARQL query text
	ResolveLabels bool   // Whether to perform IRI label enrichment
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
