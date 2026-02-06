// deals with our custom html elements
package sparql

import (
	"fmt"
	"log/slog"
	"strings"

	"hutzli.org/visoto/internal/logger"
)

// ExtractedElement represents any SPARQL custom element found in template
type ExtractedElement struct {
	TagName    string            // "sparql-query", "sparql-table", etc.
	ID         string            // Required id attribute
	Content    string            // Text content between tags
	Attributes map[string]string // All HTML attributes
}

// AsQuery converts ExtractedElement to extractedQuery
func (e ExtractedElement) AsQuery() (extractedQuery, error) {
	if e.TagName != "sparql-query" {
		log := logger.Get()
		log.Warn("invalid element type for AsQuery conversion",
			slog.String("tag_name", e.TagName),
			slog.String("expected", "sparql-query"))
		return extractedQuery{}, fmt.Errorf("not a sparql-query element: %s", e.TagName)
	}

	// Parse resolve-labels attribute with default true
	resolveLabels := true
	if val, exists := e.Attributes["resolve-labels"]; exists {
		resolveLabels = val != "false"
	}

	// Parse optional endpoint attribute
	endpoint := ""
	if val, exists := e.Attributes["endpoint"]; exists {
		endpoint = strings.TrimSpace(val)
	}

	return extractedQuery{
		ID:            e.ID,
		Query:         e.Content,
		ResolveLabels: resolveLabels,
		Endpoint:      endpoint,
	}, nil
}
