package sparql

// define custom functions for templates

import (
	"html/template"
	"net/url"
)

// Binding represents a SPARQL query result binding
type Binding struct {
	Type  string
	Value string
	Lol   string // label or literal
}

// RenderHTML returns an HTML link if the type is "uri", otherwise returns plain text
func (b Binding) RenderHTML() template.HTML {
	// return link if uri type
	if b.Type == "uri" {
		return template.HTML(`<a href="/resource/` + url.QueryEscape(b.Value) + `">` + template.HTMLEscapeString(b.Lol) + `</a>`)
	}

	// return plain text otherwise
	return template.HTML(template.HTMLEscapeString(b.Lol))
}
