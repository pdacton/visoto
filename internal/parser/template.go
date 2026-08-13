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

	// Validate required ID, for the elements that have one.
	if elem.ID == "" && requiresID(elem.TagName) {
		return elem, fmt.Errorf("missing required id attribute")
	}

	// Extract text content
	elem.Content = extractTextContent(n)

	return elem, nil
}

// requiresID reports whether a custom element is identified by its id. The column
// vocabulary is not: a <sparql-column> is identified by the query it decorates
// (its own for=, or its enclosing <sparql-columns>) plus its var=, and the
// <sparql-facet> it replaces was the same.
func requiresID(tag string) bool {
	switch tag {
	case "sparql-facet", "sparql-columns", "sparql-column":
		return false
	}
	return true
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

// ExtractSyncElements returns all <sparql-query> elements in a template — the
// queries executed server-side during the page render, as opposed to the
// <sparql-async> ones fetched afterwards.
//
// The async index uses this to know which ids exist, so a <sparql-column> may
// decorate a synchronously-rendered table too. Only the ids are of interest
// there: the query text is executed by the page pipeline, not by a fragment
// handler.
func ExtractSyncElements(content string) ([]ExtractedElement, error) {
	all, err := extractElements(content)
	if err != nil {
		return nil, err
	}
	var out []ExtractedElement
	for _, el := range all {
		if el.TagName == "sparql-query" {
			out = append(out, el)
		}
	}
	return out, nil
}

// ExtractFacetElements returns all <sparql-facet> elements in a template.
//
// The element was replaced by <sparql-column>, which describes the whole column
// rather than only its filter. This extractor survives for one purpose: letting
// startup FAIL on a leftover declaration. Nothing reads sparql-facet any more, so
// without this check a missed rename would silently drop a table's filters instead
// of announcing itself.
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

// ExtractColumnElements returns all <sparql-column> declarations in a template, in
// document order, with the base query id resolved.
//
// A column names the query it decorates with for=. Inside a <sparql-columns for="…">
// container it inherits that id instead, so a table writes it once and its columns
// stay one line each. An explicit for= on the column still wins, which keeps a lone
// declaration (no container) working.
//
// This walks the DOM itself rather than going through extractElements: inheritance
// needs the parent chain, which a flat tag-name scan has already discarded.
func ExtractColumnElements(content string) ([]ExtractedElement, error) {
	doc, err := html.Parse(strings.NewReader(content))
	if err != nil {
		return nil, fmt.Errorf("failed to parse HTML: %w", err)
	}

	var out []ExtractedElement
	var walk func(n *html.Node, inherited string)
	walk = func(n *html.Node, inherited string) {
		if n.Type == html.ElementNode {
			switch n.Data {
			case "sparql-columns":
				if v := attrValue(n, "for"); v != "" {
					inherited = v
				}
			case "sparql-column":
				if elem, err := parseElement(n); err == nil {
					if elem.Attributes["for"] == "" && inherited != "" {
						elem.Attributes["for"] = inherited
					}
					out = append(out, elem)
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c, inherited)
		}
	}
	walk(doc, "")
	return out, nil
}

// ExtractColumnContainers returns all <sparql-columns> elements. Only the indexer
// uses them, to reject a container that carries column attributes — the one-letter
// difference between the container and its children makes that typo easy and its
// symptom (a column that silently never appears) hard to read.
func ExtractColumnContainers(content string) ([]ExtractedElement, error) {
	doc, err := html.Parse(strings.NewReader(content))
	if err != nil {
		return nil, fmt.Errorf("failed to parse HTML: %w", err)
	}

	var out []ExtractedElement
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "sparql-columns" {
			if elem, err := parseElement(n); err == nil {
				out = append(out, elem)
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return out, nil
}

// attrValue returns one attribute of a node, or "".
func attrValue(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return strings.TrimSpace(a.Val)
		}
	}
	return ""
}
