package parser

import (
	"fmt"
	"strings"

	"golang.org/x/net/html"
	"hutzli.org/visoto/internal/sparql"
)

// ----- Helper functions for element extraction (DOM parsing) -----

// extractElements parses template as HTML DOM and extracts embedded SPARQL custom elements
// calls parseElement for each element to extract attributes and content
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
			case "sparql-query", "sparql-table", "sparql-tree", "sparql-async", "sparql-facet":
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
// calls extractTextContent to get the text content of the element
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

	// Validate required ID. <sparql-facet> is the exception: it carries no id and
	// is identified by its "for" (base query id) + "var" attributes instead.
	if elem.ID == "" && elem.TagName != "sparql-facet" {
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

// ----- Query extraction -----

// extractQueriesDOM extracts SPARQL queries using HTML DOM parser for regular <sparql-query> elements.
// these are server-side queries
func extractQueriesDOM(templateContent string) ([]sparql.ExtractedQuery, error) {
	elements, err := extractElements(templateContent)
	if err != nil {
		return nil, err
	}

	queries := make([]sparql.ExtractedQuery, 0, len(elements))
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

		// Parse resolve-labels attribute (default true)
		resolveLabels := true
		if val, exists := elem.Attributes["resolve-labels"]; exists {
			resolveLabels = val != "false"
		}

		// Parse optional endpoint attribute
		endpoint := strings.TrimSpace(elem.Attributes["endpoint"])

		queries = append(queries, sparql.ExtractedQuery{
			ID:            elem.ID,
			Query:         elem.Content,
			ResolveLabels: resolveLabels,
			Endpoint:      endpoint,
		})
	}

	return queries, nil
}

// ExtractAsyncElements extracts SPARQL queries and returns all <sparql-async> elements.
// These are client-side async queries
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

// ExtractFacetElements returns all <sparql-facet> elements in a template. Each
// declares one facet of a base <sparql-async> query, identified by its "for"
// attribute; the facet's configuration lives in its other attributes (var, path,
// type, control, label).
func ExtractFacetElements(content string) ([]ExtractedElement, error) {
	all, err := extractElements(content)
	if err != nil {
		return nil, err
	}
	var out []ExtractedElement
	for _, el := range all {
		if el.TagName == "sparql-facet" {
			out = append(out, el)
		}
	}
	return out, nil
}
