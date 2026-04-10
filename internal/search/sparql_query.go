package search

import (
	"fmt"
	"strings"
)

// SparqlQueryProvider implements full-text search via SPARQL FILTER(CONTAINS(...)).
// Used as a fallback when a native FTS provider returns no results, or as a primary
// provider for endpoints with no FTS plugin (plain SPARQL 1.1 only).
type SparqlQueryProvider struct{}

// Name returns the provider identifier
func (s *SparqlQueryProvider) Name() string {
	return "sparql-query"
}

// BuildQuery constructs a SPARQL FILTER(CONTAINS(...)) query from search parameters
func (s *SparqlQueryProvider) BuildQuery(params SearchParams) (string, error) {
	if params.Query == "" {
		return "", fmt.Errorf("search query cannot be empty")
	}

	limit := params.Limit
	if limit <= 0 {
		limit = 50
	}

	var parts []string

	// SELECT columns: include ?property only when no specific property is pinned
	if params.Property != "" {
		parts = append(parts, "SELECT ?type ?subject ?matchedText WHERE {")
	} else {
		parts = append(parts, "SELECT ?type ?subject ?property ?matchedText WHERE {")
	}

	// Optional class filter or OPTIONAL class lookup
	if params.Class != "" {
		parts = append(parts, fmt.Sprintf("  ?subject a <%s> .", params.Class))
		parts = append(parts, fmt.Sprintf("  BIND(<%s> AS ?type)", params.Class))
	} else {
		parts = append(parts, "  OPTIONAL { ?subject a ?type . }")
	}

	// Property triple
	if params.Property != "" {
		parts = append(parts, fmt.Sprintf("  ?subject <%s> ?matchedText .", params.Property))
	} else {
		parts = append(parts, "  ?subject ?property ?matchedText .")
	}

	// CONTAINS filter — case-insensitive substring match on literals only
	parts = append(parts, fmt.Sprintf(
		`  FILTER(isLiteral(?matchedText) && CONTAINS(LCASE(STR(?matchedText)), LCASE("%s")))`,
		escapeString(params.Query),
	))

	parts = append(parts, "}")
	parts = append(parts, fmt.Sprintf("LIMIT %d", limit))

	return strings.Join(parts, "\n"), nil
}

func init() {
	RegisterProvider(&SparqlQueryProvider{})
}
