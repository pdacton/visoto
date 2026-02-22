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

// RenderHTML returns an HTML link if the type is "uri", raw HTML if the type is "html",
// otherwise returns escaped plain text
func (b Binding) RenderHTML() template.HTML {
	switch b.Type {
	case "uri":
		return template.HTML(`<a href="/resource/` + url.QueryEscape(b.Value) + `">` + template.HTMLEscapeString(b.Lol) + `</a>`)
	case "html":
		return template.HTML(b.Lol)
	default:
		return template.HTML(template.HTMLEscapeString(b.Lol))
	}
}
