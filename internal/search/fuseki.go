package search

import (
	"fmt"
	"strings"
)

// FusekiProvider implements full-text search using Apache Jena Fuseki text:query.
// Requires the jena-text module and a configured Lucene text index on the Fuseki dataset.
type FusekiProvider struct{}

// Name returns the provider identifier
func (f *FusekiProvider) Name() string {
	return "fuseki"
}

// BuildQuery constructs a Fuseki text:query FTS query from search parameters
func (f *FusekiProvider) BuildQuery(params SearchParams) (string, error) {
	if params.Query == "" {
		return "", fmt.Errorf("search query cannot be empty")
	}

	limit := params.Limit
	if limit <= 0 {
		limit = 50
	}

	var parts []string

	parts = append(parts, "PREFIX text: <http://jena.apache.org/text#>")
	parts = append(parts, "")
	parts = append(parts, "SELECT ?type ?subject ?score ?matchedText WHERE {")

	// Optional class filter or OPTIONAL class lookup
	if params.Class != "" {
		parts = append(parts, fmt.Sprintf("  ?subject a <%s> .", params.Class))
		parts = append(parts, fmt.Sprintf("  BIND(<%s> AS ?type)", params.Class))
	} else {
		parts = append(parts, "  OPTIONAL { ?subject a ?type . }")
	}

	// text:query with optional property IRI as first argument
	var textQueryArg string
	if params.Property != "" {
		textQueryArg = fmt.Sprintf(`(<%s> "%s" %d)`, params.Property, escapeString(params.Query), limit)
	} else {
		textQueryArg = fmt.Sprintf(`("%s" %d)`, escapeString(params.Query), limit)
	}
	parts = append(parts, fmt.Sprintf("  (?subject ?score ?matchedText) text:query %s .", textQueryArg))

	parts = append(parts, "}")
	parts = append(parts, "ORDER BY DESC(?score)")
	parts = append(parts, fmt.Sprintf("LIMIT %d", limit))

	return strings.Join(parts, "\n"), nil
}

func init() {
	RegisterProvider(&FusekiProvider{})
}
