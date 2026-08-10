package main

// Set-scoped index of the async SPARQL declarations embedded in the templates.
//
// <sparql-async> and <sparql-facet> elements are inert at render time. They exist
// so an /api fragment handler can look a query up later, when HTMX or a Tabulator
// fetch asks for it by id. That id used to be resolved against a global scan of
// five template directories, first match wins in os.ReadDir order — so two
// templates that happened to reuse an id resolved by filesystem accident, and
// <sparql-facet for=…> duplicates silently merged facets across unrelated pages.
//
// The scope is now the template set: one page (or class/instance) template plus
// every layout, partial and referenced component it is parsed with. That is the
// grouping templates.SetFiles describes and multitemplate registers under a single
// render name, so the async namespace is now exactly the Go template namespace of
// the same files — an id only has to be unique within the set that declares it.
// Requests name their set in ?src=, which the frontend attaches from the
// {{ templateSet }} value base.html renders into <head>.
//
// The index is built once at startup and is read-only afterwards, so it needs no
// lock. It replaced a per-request directory walk that stat'ed all 84 template
// files on every /api call — on the order of 7000 syscalls per page load — with
// two map lookups. The one cost is that editing a query body now needs a restart,
// like every other template edit already did.

import (
	"fmt"
	"os"
	"strings"

	"hutzli.org/visoto/internal/facet"
	"hutzli.org/visoto/internal/parser"
	"hutzli.org/visoto/internal/templates"
)

// asyncIndex answers "which query does this id mean, for a request that came
// from this template set".
type asyncIndex struct {
	queries map[string]map[string]string            // set name → query id → query text
	facets  map[string]map[string][]facet.FacetSpec // set name → base query id → specs, in document order
}

// asyncIdx is the process-wide index. Non-nil from the start so a lookup before
// initAsyncIndex (a handler test that skips startup) misses cleanly instead of
// panicking — indexing a nil map is legal and yields the zero value.
var asyncIdx = &asyncIndex{}

// fileDecls are one template file's async declarations. Parsed once and reused
// by every set that includes the file: layout/base.html alone is in all of them.
type fileDecls struct {
	async  []parser.ExtractedElement
	facets []parser.ExtractedElement
}

// initAsyncIndex builds the index from the template sets under templatesDir and
// installs it. It fails — and so fails startup — on any declaration that could
// not be resolved unambiguously at request time, rather than letting a duplicate
// id decide by directory order.
func initAsyncIndex(templatesDir string) error {
	sets, err := templates.SetFiles(templatesDir)
	if err != nil {
		return fmt.Errorf("template sets: %w", err)
	}

	// The index and the Go template namespace are derived from the same mapping,
	// so validate that mapping once here rather than in a separate startup step.
	if err := templates.ValidateIncludes(sets); err != nil {
		return err
	}

	parsed := make(map[string]fileDecls)
	for _, files := range sets {
		for _, path := range files {
			if _, done := parsed[path]; done {
				continue
			}
			decls, err := parseDecls(path)
			if err != nil {
				return err
			}
			parsed[path] = decls
		}
	}

	idx := &asyncIndex{
		queries: make(map[string]map[string]string, len(sets)),
		facets:  make(map[string]map[string][]facet.FacetSpec, len(sets)),
	}
	for set, files := range sets {
		queries := make(map[string]string)
		declaredIn := make(map[string]string) // id → file, to name both sides of a clash
		for _, path := range files {
			for _, el := range parsed[path].async {
				if prev, dup := declaredIn[el.ID]; dup {
					return fmt.Errorf("duplicate <sparql-async id=%q> in template set %s: declared in %s and %s",
						el.ID, set, prev, path)
				}
				declaredIn[el.ID] = path
				queries[el.ID] = el.Content
			}
		}

		// Facets are collected after every query in the set is known, so a facet
		// may sit in a different file from the query it decorates.
		facets := make(map[string][]facet.FacetSpec)
		for _, path := range files {
			for _, el := range parsed[path].facets {
				base := el.Attributes["for"]
				if base == "" {
					// Not a declaration. Extraction reads the raw file as a DOM, so a
					// bare tag name written in prose or a {{/* … */}} comment — which
					// is not an HTML comment and so is not skipped — arrives here with
					// no attributes at all. Documentation is allowed to name the
					// element; configuration without a for= is a genuine mistake.
					if el.Attributes["var"] != "" || el.Attributes["path"] != "" || el.Attributes["control"] != "" {
						return fmt.Errorf(`%s: <sparql-facet var=%q> has no for="…" naming its base query`,
							path, el.Attributes["var"])
					}
					continue
				}
				if _, ok := queries[base]; !ok {
					return fmt.Errorf(`%s: <sparql-facet for=%q var=%q> names no <sparql-async> in template set %s`,
						path, base, el.Attributes["var"], set)
				}
				facets[base] = append(facets[base], facetSpecFrom(el))
			}
		}

		idx.queries[set] = queries
		if len(facets) > 0 {
			idx.facets[set] = facets
		}
	}

	asyncIdx = idx
	return nil
}

// parseDecls extracts one file's <sparql-async> and <sparql-facet> elements.
// Unlike the scan it replaces, an unparseable file is an error rather than a
// silently skipped one: at startup that is a bug worth stopping for.
func parseDecls(path string) (fileDecls, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return fileDecls{}, fmt.Errorf("read %s: %w", path, err)
	}
	async, err := parser.ExtractAsyncElements(string(content))
	if err != nil {
		return fileDecls{}, fmt.Errorf("%s: extract async elements: %w", path, err)
	}
	facets, err := parser.ExtractFacetElements(string(content))
	if err != nil {
		return fileDecls{}, fmt.Errorf("%s: extract facet elements: %w", path, err)
	}
	return fileDecls{async: async, facets: facets}, nil
}

// facetSpecFrom reads one <sparql-facet> element's configuration attributes.
func facetSpecFrom(el parser.ExtractedElement) facet.FacetSpec {
	return facet.FacetSpec{
		Var:     strings.TrimPrefix(el.Attributes["var"], "?"),
		Root:    el.Attributes["root"],
		Path:    el.Attributes["path"],
		Type:    el.Attributes["type"],
		Control: el.Attributes["control"],
		Label:   el.Attributes["label"],
	}
}

// findAsyncQuery returns the text of the <sparql-async id=id> element visible to
// template set src — the ?src= the requesting page sent. An id declared by
// another set is not found, which is the whole point: ids are scoped, so the same
// name may mean different queries on different pages.
func findAsyncQuery(src, id string) (string, bool) {
	q, ok := asyncIdx.queries[src][id]
	return q, ok
}

// findFacetSpecs returns the facets declared for a base query id within one
// template set, in document order.
func findFacetSpecs(src, baseID string) []facet.FacetSpec {
	return asyncIdx.facets[src][baseID]
}

// findFacetSpec returns the single facet declared as (baseID, varName), if any.
func findFacetSpec(src, baseID, varName string) (facet.FacetSpec, bool) {
	for _, s := range findFacetSpecs(src, baseID) {
		if s.Var == varName {
			return s, true
		}
	}
	return facet.FacetSpec{}, false
}
