package search

import (
	"fmt"
	"strings"
)

// QLeverProvider implements full-text search for QLever using the textSearch: SERVICE.
// Requires a QLever text index built with `qlever index --text-index`.
// See: https://docs.qlever.dev/text-search/
type QLeverProvider struct{}

// Name returns the provider identifier
func (q *QLeverProvider) Name() string {
	return "qlever"
}

// BuildQuery constructs a QLever textSearch SERVICE query from search parameters
func (q *QLeverProvider) BuildQuery(params SearchParams) (string, error) {
	if params.Query == "" {
		return "", fmt.Errorf("search query cannot be empty")
	}

	limit := params.Limit
	if limit <= 0 {
		limit = 50
	}

	var parts []string

	parts = append(parts, "PREFIX textSearch: <https://qlever.cs.uni-freiburg.de/textSearch/>")
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

	// Property triple
	if params.Property != "" {
		parts = append(parts, fmt.Sprintf("  ?subject <%s> ?matchedText .", params.Property))
	} else {
		parts = append(parts, "  ?subject ?property ?matchedText .")
	}

	// textSearch SERVICE block — append * for prefix matching
	parts = append(parts, "  SERVICE textSearch: {")
	parts = append(parts, "    ?matchedText textSearch:contains [")
	parts = append(parts, fmt.Sprintf(`      textSearch:word "%s*" ;`, escapeString(params.Query)))
	parts = append(parts, "      textSearch:score ?score")
	parts = append(parts, "    ] .")
	parts = append(parts, "  }")

	parts = append(parts, "}")
	parts = append(parts, "ORDER BY DESC(?score)")
	parts = append(parts, fmt.Sprintf("LIMIT %d", limit))

	return strings.Join(parts, "\n"), nil
}

func init() {
	RegisterProvider(&QLeverProvider{})
}
