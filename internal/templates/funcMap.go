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
	"toJSON":       toJSON,
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

// toJSON converts any value to pretty-printed JSON
// Usage in templates: {{ toJSON . }}
func toJSON(v interface{}) (string, error) {
	jsonBytes, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "", err
	}
	return string(jsonBytes), nil
}
