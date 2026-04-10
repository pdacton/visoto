package search

import (
	"fmt"
	"strings"
)

// StardogProvider implements full-text search for Stardog databases
type StardogProvider struct{}

// Name returns the provider identifier
func (s *StardogProvider) Name() string {
	return "stardog"
}

// BuildQuery constructs a Stardog FTS query from search parameters
func (s *StardogProvider) BuildQuery(params SearchParams) (string, error) {
	if params.Query == "" {
		return "", fmt.Errorf("search query cannot be empty")
	}

	var queryParts []string

	// Add PREFIX for Stardog FTS
	queryParts = append(queryParts, "PREFIX fts: <tag:stardog:api:search:>")
	queryParts = append(queryParts, "")
	// Start SELECT clause, we're excluding highlight from the results, maybe add later for highlighting
	queryParts = append(queryParts, "SELECT ?type ?subject ?property ?matchedText ?score WHERE {")

	// Bind search text
	queryParts = append(queryParts, fmt.Sprintf(`  BIND ("%s" AS ?textSearch)`, escapeString(params.Query)))

	// Bind class filter (or unbound if not specified)
	if params.Class != "" {
		queryParts = append(queryParts, fmt.Sprintf(`  BIND (<%s> AS ?type)`, params.Class))
	} else {
		queryParts = append(queryParts, `  BIND (?undef AS ?type)`)
	}

	// Bind property filter (or unbound if not specified)
	if params.Property != "" {
		queryParts = append(queryParts, fmt.Sprintf(`  BIND (<%s> AS ?property)`, params.Property))
	} else {
		queryParts = append(queryParts, `  BIND (?undef AS ?property)`)
	}

	// Add empty line for readability
	queryParts = append(queryParts, "")

	// Triple pattern for subject, class, and property
	queryParts = append(queryParts, `  ?subject a ?type ;`)
	queryParts = append(queryParts, `     ?property ?matchedText .`)
	queryParts = append(queryParts, "")

	// Stardog FTS service
	queryParts = append(queryParts, `  service fts:textMatch {`)
	queryParts = append(queryParts, `      [] fts:query ?textSearch ;`)
	queryParts = append(queryParts, `         fts:score ?score ;`)
	queryParts = append(queryParts, `         fts:result ?matchedText ;`)
	queryParts = append(queryParts, `         fts:highlight ?highlight ;`)
	queryParts = append(queryParts, `  }`)

	// Close WHERE clause
	queryParts = append(queryParts, `}`)

	// Order by relevance score
	queryParts = append(queryParts, `ORDER BY DESC(?score)`)

	// Add limit
	if params.Limit > 0 {
		queryParts = append(queryParts, fmt.Sprintf(`LIMIT %d`, params.Limit))
	}

	return strings.Join(queryParts, "\n"), nil
}

// escapeString escapes special characters in SPARQL string literals
func escapeString(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\"", "\\\"")
	s = strings.ReplaceAll(s, "\n", "\\n")
	s = strings.ReplaceAll(s, "\r", "\\r")
	s = strings.ReplaceAll(s, "\t", "\\t")
	return s
}

func init() {
	RegisterProvider(&StardogProvider{})
}
