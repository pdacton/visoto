package main

// Set-scoped index of the async SPARQL declarations embedded in the templates.
//
// <sparql-async> and <sparql-column> elements are inert at render time. They exist
// so an /api fragment handler can look a query up later, when HTMX or a Tabulator
// fetch asks for it by id. That id used to be resolved against a global scan of
// five template directories, first match wins in os.ReadDir order — so two
// templates that happened to reuse an id resolved by filesystem accident, and
// per-query column declarations silently merged across unrelated pages.
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

	"hutzli.org/visoto/internal/column"
	"hutzli.org/visoto/internal/parser"
	"hutzli.org/visoto/internal/templates"
)

// asyncIndex answers "which query does this id mean, for a request that came
// from this template set".
type asyncIndex struct {
	queries map[string]map[string]string       // set name → query id → query text
	columns map[string]map[string]column.Table // set name → base query id → declared columns, in document order
}

// asyncIdx is the process-wide index. Non-nil from the start so a lookup before
// initAsyncIndex (a handler test that skips startup) misses cleanly instead of
// panicking — indexing a nil map is legal and yields the zero value.
var asyncIdx = &asyncIndex{}

// fileDecls are one template file's async declarations. Parsed once and reused
// by every set that includes the file: layout/base.html alone is in all of them.
type fileDecls struct {
	async      []parser.ExtractedElement
	columns    []parser.ExtractedElement
	containers []parser.ExtractedElement
	facets     []parser.ExtractedElement // legacy <sparql-facet>, kept only to reject
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
		columns: make(map[string]map[string]column.Table, len(sets)),
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

		// Columns are collected after every query in the set is known, so a column
		// may sit in a different file from the query it decorates.
		columns := make(map[string]column.Table)
		for _, path := range files {
			if err := rejectLegacyFacets(path, parsed[path].facets); err != nil {
				return err
			}
			if err := rejectMisplacedColumnAttrs(path, parsed[path].containers); err != nil {
				return err
			}
			for _, el := range parsed[path].columns {
				spec := column.FromAttributes(el.Attributes)
				base := el.Attributes["for"]
				if base == "" {
					// Not a declaration. Extraction reads the raw file as a DOM, so a
					// bare tag name written in prose or a {{/* … */}} comment — which
					// is not an HTML comment and so is not skipped — arrives here with
					// no attributes at all. Documentation is allowed to name the
					// element; configuration without a base query is a genuine mistake.
					if spec.Var != "" {
						return fmt.Errorf(`%s: <sparql-column var=%q> names no base query — give it a for="…" or nest it in <sparql-columns for="…">`,
							path, spec.Var)
					}
					continue
				}
				if _, ok := queries[base]; !ok {
					return fmt.Errorf(`%s: <sparql-column for=%q var=%q> names no <sparql-async> in template set %s`,
						path, base, spec.Var, set)
				}
				if err := spec.Validate(); err != nil {
					return fmt.Errorf("%s: %w", path, err)
				}
				columns[base] = append(columns[base], spec)
			}
		}

		idx.queries[set] = queries
		if len(columns) > 0 {
			idx.columns[set] = columns
		}
	}

	asyncIdx = idx
	return nil
}

// parseDecls extracts one file's <sparql-async> and column declarations.
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
	columns, err := parser.ExtractColumnElements(string(content))
	if err != nil {
		return fileDecls{}, fmt.Errorf("%s: extract column elements: %w", path, err)
	}
	containers, err := parser.ExtractColumnContainers(string(content))
	if err != nil {
		return fileDecls{}, fmt.Errorf("%s: extract column containers: %w", path, err)
	}
	facets, err := parser.ExtractFacetElements(string(content))
	if err != nil {
		return fileDecls{}, fmt.Errorf("%s: extract facet elements: %w", path, err)
	}
	return fileDecls{async: async, columns: columns, containers: containers, facets: facets}, nil
}

// rejectLegacyFacets fails startup on a <sparql-facet> that still carries
// configuration. Nothing reads the element any more, so a missed rename would
// otherwise cost a table its filters with no symptom but their absence. Prose that
// merely names the tag (documentation, a Go comment — which is not an HTML comment
// and so is not skipped by extraction) carries no attributes and is left alone.
func rejectLegacyFacets(path string, els []parser.ExtractedElement) error {
	for _, el := range els {
		if el.Attributes["for"] == "" && el.Attributes["var"] == "" &&
			el.Attributes["path"] == "" && el.Attributes["control"] == "" {
			continue
		}
		return fmt.Errorf(`%s: <sparql-facet var=%q> was replaced by <sparql-column>, which declares the whole column `+
			`(label, tip, rendering, filter) — see docs/templating.md; control= is now filter=`,
			path, el.Attributes["var"])
	}
	return nil
}

// rejectMisplacedColumnAttrs fails startup on a <sparql-columns> container carrying
// attributes that belong on a <sparql-column> child. The two names differ by one
// letter, and the symptom of the typo — a column that simply never appears — reads
// as a data problem rather than a markup one.
func rejectMisplacedColumnAttrs(path string, els []parser.ExtractedElement) error {
	for _, el := range els {
		for _, attr := range []string{"var", "label", "tip", "filter", "type", "path", "root"} {
			if _, ok := el.Attributes[attr]; ok {
				return fmt.Errorf(`%s: <sparql-columns %s=…> carries a column attribute — the container takes only for=, `+
					`the attributes belong on a nested <sparql-column>`, path, attr)
			}
		}
	}
	return nil
}

// findAsyncQuery returns the text of the <sparql-async id=id> element visible to
// template set src — the ?src= the requesting page sent. An id declared by
// another set is not found, which is the whole point: ids are scoped, so the same
// name may mean different queries on different pages.
func findAsyncQuery(src, id string) (string, bool) {
	q, ok := asyncIdx.queries[src][id]
	return q, ok
}

// findColumns returns the columns declared for a base query id within one
// template set, in document order.
func findColumns(src, baseID string) column.Table {
	return asyncIdx.columns[src][baseID]
}

// findColumn returns the single column declared as (baseID, varName), if any.
func findColumn(src, baseID, varName string) (column.Spec, bool) {
	return findColumns(src, baseID).Find(varName)
}

// applyColumnParams folds a table's declared columns into the presentation params
// the sparqlTable fragment takes, so a table that declares its columns does not
// also repeat the same variable names in the partial call.
//
// A declaration wins over the dict param it replaces; the param stays supported
// because most tables declare no columns at all and a single "iconVar" key is the
// shorter way to say it. facetFor is derived rather than declared: a table is
// faceted exactly when one of its columns carries a filter.
func applyColumnParams(params map[string]any, src, id string) {
	cols := findColumns(src, id)
	if len(cols) == 0 {
		return
	}
	if v := cols.IconVar(); v != "" {
		params["iconVar"] = v
	}
	if v := cols.BadgeVar(); v != "" {
		params["badgeVar"] = v
	}
	if v := cols.GroupVar(); v != "" {
		params["groupBy"] = v
	}
	if cols.Filterable() {
		params["facetFor"] = id
	}
}
