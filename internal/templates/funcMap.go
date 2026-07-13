package templates

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"sort"
	"strings"

	"hutzli.org/visoto/internal/resource"
	"hutzli.org/visoto/internal/sparql"
)

// funcMap defines custom template functions available in all templates
var funcMap = template.FuncMap{
	"render":          sparql.Binding.RenderHTML,
	"resourceHref":    sparql.ResourceHref,
	"dict":            makeDict,
	"resourceIcon":    resource.GetIconForResource,
	"iconNames":       resource.GetIconNames,
	"toJSON":          toJSON,
	"toJSONRaw":       toJSONRaw,
	"toJSONPretty":    toJSONPretty,
	"firstValue":      firstValue,
	"lastPathSegment": lastPathSegment,
	"groupByValue":    groupByValue,
}

// makeDict creates a map from alternating key-value pairs
// Usage: {{ dict "key1" value1 "key2" value2 }}
func makeDict(values ...interface{}) (map[string]interface{}, error) {
	if len(values)%2 != 0 {
		return nil, fmt.Errorf("dict: odd number of arguments")
	}

	dict := make(map[string]interface{}, len(values)/2)
	for i := 0; i < len(values); i += 2 {
		key, ok := values[i].(string)
		if !ok {
			return nil, fmt.Errorf("dict: key at position %d is not a string", i)
		}
		dict[key] = values[i+1]
	}
	return dict, nil
}

// toJSON converts any value to JSON for embedding in script tags
// Returns template.HTML to prevent any escaping
// HTML escaping (< > &) is kept for safety when embedding in <script> tags
// Usage in templates: {{ toJSON . }}
func toJSON(v interface{}) (template.HTML, error) {
	jsonBytes, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return template.HTML(jsonBytes), nil
}

// toJSONRaw converts a value to JSON without HTML-escaping < > &
// Use this when the JSON value will be processed by JS (e.g. passed to encodeURIComponent)
// rather than rendered as HTML, so that URIs in SPARQL queries are preserved literally.
func toJSONRaw(v interface{}) (template.HTML, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return "", err
	}
	// json.Encoder appends a newline; trim it
	return template.HTML(bytes.TrimRight(buf.Bytes(), "\n")), nil
}

// toJSONPretty converts any value to indented JSON for display
// HTML escaping (< > &) is kept for safety when embedding in HTML
// Usage in templates: {{ toJSONPretty . }}
func toJSONPretty(v interface{}) (template.HTML, error) {
	jsonBytes, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "", err
	}
	return template.HTML(jsonBytes), nil
}

// lastPathSegment returns the last path segment of an IRI:
// the fragment after # takes priority, then the last path segment after /
func lastPathSegment(iri string) string {
	iri = strings.TrimRight(iri, "/")
	if idx := strings.LastIndex(iri, "#"); idx != -1 && idx < len(iri)-1 {
		return iri[idx+1:]
	}
	if idx := strings.LastIndex(iri, "/"); idx != -1 && idx < len(iri)-1 {
		return iri[idx+1:]
	}
	return iri
}

// groupByValue deduplicates SPARQL grid bindings by the "value" field.
// When multiple bindings share the same value, their "property" labels are
// merged into a single comma-separated property Binding so the value is shown
// only once in the datagrid. Insertion order (first occurrence) is preserved.
func groupByValue(bindings []map[string]sparql.Binding) []map[string]sparql.Binding {
	type entry struct {
		propHTMLs []string
		value     sparql.Binding
	}
	seen := make(map[string]*entry)
	order := []string{} // tracks first-seen order by value key

	for _, b := range bindings {
		val := b["value"]
		prop := b["property"]
		key := val.Value
		if e, ok := seen[key]; ok {
			e.propHTMLs = append(e.propHTMLs, string(prop.RenderHTML()))
		} else {
			seen[key] = &entry{propHTMLs: []string{string(prop.RenderHTML())}, value: val}
			order = append(order, key)
		}
	}

	result := make([]map[string]sparql.Binding, 0, len(order))
	for _, key := range order {
		e := seen[key]
		joinedHTML := strings.Join(e.propHTMLs, ", ")
		result = append(result, map[string]sparql.Binding{
			"property": {Type: "html", DisplayText: joinedHTML},
			"value":    e.value,
		})
	}

	// Sort: short values (< 200 chars) first, long values second.
	// SliceStable preserves relative order within each group.
	sort.SliceStable(result, func(i, j int) bool {
		iLong := len(result[i]["value"].Value) >= 200
		jLong := len(result[j]["value"].Value) >= 200
		return !iLong && jLong
	})

	// Mark long entries so the template can apply full-width styling.
	for i, row := range result {
		if len(row["value"].Value) >= 200 {
			result[i]["long"] = sparql.Binding{Type: "literal", DisplayText: "true"}
		}
	}
	return result
}

// firstValue extracts the first value from a QueryResult for a given variable name
// Returns empty string if no bindings exist or variable not found
// Usage: {{ firstValue .QueryResults.pageTitle "title" }}
func firstValue(result sparql.QueryResult, varName string) string {
	if len(result.Bindings) == 0 {
		return ""
	}
	if binding, ok := result.Bindings[0][varName]; ok {
		return binding.Value
	}
	return ""
}
