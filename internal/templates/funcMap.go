package templates

import (
	"encoding/json"
	"fmt"
	"html/template"

	"hutzli.org/visoto/internal/resource"
	"hutzli.org/visoto/internal/sparql"
)

// funcMap defines custom template functions available in all templates
var funcMap = template.FuncMap{
	"render":       sparql.Binding.RenderHTML,
	"dict":         makeDict,
	"resourceIcon": resource.GetIconForResource,
	"iconNames":    resource.GetIconNames,
	"toJSON":       toJSON,
	"toJSONPretty": toJSONPretty,
	"firstValue":   firstValue,
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
