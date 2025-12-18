package templates

import (
	"html/template"

	"hutzli.org/visoto/internal/sparql"
)

// funcMap defines custom template functions available in all templates
var funcMap = template.FuncMap{
	"render": sparql.Binding.RenderHTML,
}
