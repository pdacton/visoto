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
	"hutzli.org/visoto/internal/sparql"
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
	sync       []parser.ExtractedElement // <sparql-query>: ids only, so columns can decorate a sync table
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

		// Sync <sparql-query> ids are collected so a <sparql-column> may decorate a
		// synchronously-rendered table too — only the ids matter here, the query
		// text is executed by the page pipeline rather than by a fragment handler.
		//
		// Deliberately NOT checked for duplicates the way async ids are: overriding
		// a shared component's query is an established pattern (a page redefining
		// pageHeader.html's pageSubtitle or title for itself, ~67 times across the
		// templates), and the page render already resolves that by last-one-wins.
		syncIDs := make(map[string]bool)
		for _, path := range files {
			for _, el := range parsed[path].sync {
				if el.ID != "" {
					syncIDs[el.ID] = true
				}
			}
			// <sparql-columns for="X" external> declares that the table with id X is
			// rendered from data the Go side supplies rather than from a query
			// declared in the templates — the search results table is the one such
			// case. Without this the for= would name nothing and fail the check
			// below, and that check is worth keeping strict for everything else: a
			// mistyped for= otherwise costs a column its configuration silently.
			for _, el := range parsed[path].containers {
				if _, ok := el.Attributes["external"]; ok && el.Attributes["for"] != "" {
					syncIDs[el.Attributes["for"]] = true
				}
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
				_, isAsync := queries[base]
				if !isAsync && !syncIDs[base] {
					return fmt.Errorf(`%s: <sparql-column for=%q var=%q> names no <sparql-async> or <sparql-query> in template set %s`,
						path, base, spec.Var, set)
				}
				// An id that names both kinds in one set leaves it undecided which
				// table the declaration decorates. Only worth rejecting when a column
				// actually references it — the collision alone is harmless, and a few
				// ids (outgoing, incoming, instances) exist in both forms in unrelated
				// sets.
				if isAsync && syncIDs[base] {
					return fmt.Errorf(`%s: <sparql-column for=%q var=%q> is ambiguous in template set %s — `+
						`that id names both a <sparql-async> and a <sparql-query>; rename one of them`,
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
	// Sync tables resolve their own declarations while rendering, since no handler
	// builds their params. The templates package cannot reach this index directly
	// (it is built here, from package main), so hand it a lookup.
	templates.SetColumnLookup(findColumns)
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
	sync, err := parser.ExtractSyncElements(string(content))
	if err != nil {
		return fileDecls{}, fmt.Errorf("%s: extract sync elements: %w", path, err)
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
	return fileDecls{async: async, sync: sync, columns: columns, containers: containers, facets: facets}, nil
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

// queryOptions returns the per-query switches a table result needs — currently
// only whether to resolve rdf:type into resource icons.
//
// This is what gates the type query: with no icon column nothing renders icons,
// so the extra (batched, cached, parallel) round trip would be pure cost. The
// fragment routes pass the folded params value; the working-set route, which
// builds no params, reads the declaration directly.
func queryOptions(iconVars string) []sparql.Option {
	if iconVars == "" {
		return nil
	}
	return []sparql.Option{sparql.WithTypes()}
}

// findColumn returns the single column declared as (baseID, varName), if any.
func findColumn(src, baseID, varName string) (column.Spec, bool) {
	return findColumns(src, baseID).Find(varName)
}

// applyColumnParams folds a table's declared columns into the presentation params
// the sparqlTable fragment takes, so a table that declares its columns does not
// also repeat the same variable names in the partial call.
//
// icon and badge come from the declarations and nowhere else — there is no dict
// param or query-string key left to lose to. groupBy still has one, and the
// declaration wins over it. facetFor is derived rather than declared: a table is
// faceted exactly when one of its columns carries a filter.
func applyColumnParams(params map[string]any, src, id string) {
	cols := findColumns(src, id)
	if len(cols) == 0 {
		return
	}
	params["iconVars"] = cols.IconVars()
	params["badgeVars"] = cols.BadgeVars()
	if v := cols.GroupVar(); v != "" {
		params["groupBy"] = v
	}
	if cols.Filterable() {
		params["facetFor"] = id
	}
}
