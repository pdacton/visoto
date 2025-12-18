package sparql

import (
	"time"
)

// Config holds configuration for the SPARQL preprocessor
type Config struct {
	EndpointURL string        // SPARQL endpoint URL
	Timeout     time.Duration // Global timeout for all queries
}

// QueryResult stores simplified SPARQL results for template consumption
type QueryResult struct {
	Vars     []string              // Variable names from head.vars
	Bindings []map[string]struct { // bindings: value and type
		Type  string
		Value string
		Lol   string // Lol: Label or Literal (not laughing out loud)
	}
	Error string // Error message if query failed
}

// PageData wraps all data passed to templates with embedded queries
type PageData struct {
	QueryResults map[string]QueryResult // Results indexed by query ID
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
