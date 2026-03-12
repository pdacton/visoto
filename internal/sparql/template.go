package sparql

// TODO: refactor into the template package with methods

import (
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

// Preprocessor handles SPARQL query preprocessing for templates
type Preprocessor struct {
	config     Config
	httpClient *http.Client
}

// New creates a new Preprocessor with the given configuration
func New(config Config) *Preprocessor {
	return &Preprocessor{
		config:     config,
		httpClient: &http.Client{},
	}
}

// ProcessTemplateFile reads a template file, extracts and executes SPARQL queries,
// and returns the cleaned template content and query results
func (p *Preprocessor) ProcessTemplateFile(filepath string, iri string, acceptLanguage string) (TemplateData, error) {

	// Read template file
	content, err := os.ReadFile(filepath)
	if err != nil {
		return TemplateData{}, fmt.Errorf("failed to read template file: %w", err)
	}

	// Extract SPARQL queries from template using DOM parser
	queries, err := extractQueriesDOM(string(content))
	if err != nil {
		return TemplateData{}, fmt.Errorf("failed to extract queries: %w", err)
	}

	// Replace the entity placehoder `??` with the provided IRI in each query
	for i := range queries {
		queries[i].Query = strings.ReplaceAll(queries[i].Query, "??", fmt.Sprintf("<%s>", iri))
	}

	// Execute queries in parallel with language preference
	results := p.executeQueriesParallel(queries, p.config.Timeout, acceptLanguage)

	// Create TemplateData with results
	data := TemplateData{
		ResourceIRI:  iri,
		QueryResults: results,
	}

	return data, nil
}

// parse template content and extract all <sparql-query> tags
// TODO: OLD, REMOVE
func extractQueries(templateContent string) ([]extractedQuery, error) {
	re := regexp.MustCompile(`<sparql-query\s+id="([^"]+)"(?:\s+resolve-labels="(true|false)")?\s*>([\s\S]*?)</sparql-query>`)
	matches := re.FindAllStringSubmatch(templateContent, -1)

	queries := make([]extractedQuery, 0, len(matches))
	idMap := make(map[string]bool)

	for _, match := range matches {
		if len(match) != 4 {
			continue
		}

		id := strings.TrimSpace(match[1])
		resolveLabelsAttr := match[2] // Can be "", "true", or "false"
		query := strings.TrimSpace(match[3])

		if id == "" {
			return nil, fmt.Errorf("empty query ID found")
		}
		if query == "" {
			return nil, fmt.Errorf("empty query content for ID: %s", id)
		}
		if idMap[id] {
			return nil, fmt.Errorf("duplicate query ID: %s", id)
		}

		// Default to true (label resolution enabled by default)
		// Only false if explicitly set to "false"
		resolveLabels := resolveLabelsAttr != "false"

		idMap[id] = true
		queries = append(queries, extractedQuery{
			ID:            id,
			Query:         query,
			ResolveLabels: resolveLabels,
		})
	}

	return queries, nil
}

// extractElements parses template as HTML DOM and extracts SPARQL custom elements
func extractElements(templateContent string) ([]ExtractedElement, error) {
	doc, err := html.Parse(strings.NewReader(templateContent))
	if err != nil {
		return nil, fmt.Errorf("failed to parse HTML: %w", err)
	}

	var elements []ExtractedElement
	var walk func(*html.Node)

	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			// Check if this is a SPARQL custom element
			switch n.Data {
			case "sparql-query", "sparql-table", "sparql-tree", "sparql-async":
				elem, err := parseElement(n)
				if err == nil {
					elements = append(elements, elem)
				}
			}
		}

		// Recursively walk children
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}

	walk(doc)
	return elements, nil
}

// parseElement extracts data from an HTML node
func parseElement(n *html.Node) (ExtractedElement, error) {
	elem := ExtractedElement{
		TagName:    n.Data,
		Attributes: make(map[string]string),
	}

	// Extract all attributes
	for _, attr := range n.Attr {
		elem.Attributes[attr.Key] = attr.Val

		// Capture ID separately for convenience
		if attr.Key == "id" {
			elem.ID = strings.TrimSpace(attr.Val)
		}
	}

	// Validate required ID
	if elem.ID == "" {
		return elem, fmt.Errorf("missing required id attribute")
	}

	// Extract text content
	elem.Content = extractTextContent(n)

	return elem, nil
}

// extractTextContent gets all text nodes from an element
func extractTextContent(n *html.Node) string {
	var buf strings.Builder

	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.TextNode {
			buf.WriteString(node.Data)
		}
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}

	for c := n.FirstChild; c != nil; c = c.NextSibling {
		walk(c)
	}

	return strings.TrimSpace(buf.String())
}

// extractQueriesDOM extracts SPARQL queries using HTML DOM parser
func extractQueriesDOM(templateContent string) ([]extractedQuery, error) {
	elements, err := extractElements(templateContent)
	if err != nil {
		return nil, err
	}

	queries := make([]extractedQuery, 0, len(elements))
	idMap := make(map[string]bool)

	for _, elem := range elements {
		// Only process sparql-query elements
		if elem.TagName != "sparql-query" {
			continue
		}

		// Check for duplicate IDs
		if idMap[elem.ID] {
			return nil, fmt.Errorf("duplicate query ID: %s", elem.ID)
		}
		idMap[elem.ID] = true

		// Validate content
		if elem.Content == "" {
			return nil, fmt.Errorf("empty query content for ID: %s", elem.ID)
		}

		// Convert to extractedQuery
		query, err := elem.AsQuery()
		if err != nil {
			return nil, fmt.Errorf("failed to convert element %s: %w", elem.ID, err)
		}

		queries = append(queries, query)
	}

	return queries, nil
}

// ExtractAsyncElements parses HTML content and returns all <sparql-async> elements.
// These are client-side async SPARQL queries not executed during server-side rendering.
func ExtractAsyncElements(content string) ([]ExtractedElement, error) {
	all, err := extractElements(content)
	if err != nil {
		return nil, err
	}
	var out []ExtractedElement
	for _, el := range all {
		if el.TagName == "sparql-async" {
			out = append(out, el)
		}
	}
	return out, nil
}
