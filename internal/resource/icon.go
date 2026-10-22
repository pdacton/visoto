package resource

import (
	"hutzli.org/visoto/internal/icon"
	"hutzli.org/visoto/internal/parser"
)

// GetIconForResource returns the icon path for a resource page's header.
//
// The matching rule itself lives in internal/icon and is shared with the SPARQL
// tables; this function only supplies the two things that are specific to a
// resource page — where the RDF types come from (the pageClasses query declared
// in templates/components/pageHeader.html) and what a miss means (icon.Page: default.svg).
//
// Accepts parser.TemplateData for resource pages; anything else (search, home)
// gets "" because those pages have no single resource to describe.
func GetIconForResource(data interface{}) string {
	td, ok := data.(parser.TemplateData)
	if !ok {
		return ""
	}

	return icon.Resolve(icon.Subject{IRI: td.ResourceIRI, Types: pageClasses(td)}, icon.Page)
}

// pageClasses collects the RDF types the page header query found, in the order
// the endpoint returned them. Order only matters as a tie-break: icon.Resolve
// scans the whole list for an exact match before it accepts any fallback, which
// is what keeps a generic class (schema:DefinedTerm) from beating a specific one
// (schch:Canton) on the many LINDAS resources that carry both.
func pageClasses(td parser.TemplateData) []string {
	result, ok := td.QueryResults["pageClasses"]
	if !ok {
		return nil
	}
	classes := make([]string, 0, len(result.Bindings))
	for _, binding := range result.Bindings {
		if b, ok := binding["class"]; ok && b.Value != "" {
			classes = append(classes, b.Value)
		}
	}
	return classes
}
