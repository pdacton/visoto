package search

import (
	"fmt"
	"strings"
)

// GraphDBProvider implements full-text search using GraphDB Simple FTS (onto:fts).
// No Lucene connector is required; this uses the built-in GraphDB FTS plugin.
// Note: Simple FTS does not expose a relevance score, so results are not ranked.
type GraphDBProvider struct{}

// Name returns the provider identifier
func (g *GraphDBProvider) Name() string {
	return "graphdb"
}

// BuildQuery constructs a GraphDB Simple FTS query from search parameters
func (g *GraphDBProvider) BuildQuery(params SearchParams) (string, error) {
	if params.Query == "" {
		return "", fmt.Errorf("search query cannot be empty")
	}

	limit := params.Limit
	if limit <= 0 {
		limit = 50
	}

	var parts []string

	parts = append(parts, "PREFIX onto: <http://www.ontotext.com/plugins/ontotextfts/>")
	parts = append(parts, "")

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

	// Property triple + FTS filter
	if params.Property != "" {
		parts = append(parts, fmt.Sprintf("  ?subject <%s> ?matchedText .", params.Property))
	} else {
		parts = append(parts, "  ?subject ?property ?matchedText .")
	}
	parts = append(parts, fmt.Sprintf(`  ?matchedText onto:fts ("%s" %d) .`, escapeString(params.Query), limit))

	parts = append(parts, "}")
	parts = append(parts, fmt.Sprintf("LIMIT %d", limit))

	return strings.Join(parts, "\n"), nil
}

func init() {
	RegisterProvider(&GraphDBProvider{})
}
