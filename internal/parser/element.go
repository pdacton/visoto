package parser

// ExtractedElement represents any SPARQL custom element found in a template
type ExtractedElement struct {
	TagName    string            // "sparql-query", "sparql-table", etc.
	ID         string            // Required id attribute
	Content    string            // Text content between tags
	Attributes map[string]string // All HTML attributes
}
