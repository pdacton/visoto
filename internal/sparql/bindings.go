package sparql

// define custom functions for templates

import (
	"html/template"
)

// Binding represents a SPARQL query result binding
type Binding struct {
	Type  string
	Value string
	Lol   string
}

// RenderHTML returns an HTML link if the type is "uri", otherwise returns plain text
// TODO: think about link generation, if /embedded/ is the best choice
func (b Binding) RenderHTML() template.HTML {
	// return link if uri type
	if b.Type == "uri" {
		return template.HTML(`<a href="/embedded/` + template.HTMLEscapeString(b.Value) + `">` + template.HTMLEscapeString(b.Lol) + `</a>`)
	}

	// return plain text otherwise
	return template.HTML(template.HTMLEscapeString(b.Lol))
}
