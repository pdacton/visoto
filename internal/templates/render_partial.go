package templates

// This file renders individual partials to standalone HTML fragments for async
// (HTMX) responses. The normal render path bundles every page with the full
// base.html document (see Load), so it cannot emit a bare partial; handlers that
// answer HTMX swaps use the helpers here instead.
//
// Like the page sets in Load, each partial is parsed once and then cloned per
// UI language with its i18n functions overridden, so a fragment swapped into a
// French page comes back in French. InitPartials must run at startup, before the
// first request; the sets are read-only afterwards, which is what replaced the
// sync.Once-per-partial lazy parse this file used to do.

import (
	"bytes"
	"fmt"
	"html/template"

	"hutzli.org/visoto/internal/i18n"
	"hutzli.org/visoto/internal/lang"
)

// sparqlTablePartial is the path to the reusable table partial. It stands alone:
// it uses only funcMap functions (dict, toJSON, toJSONRaw, iconNames, render,
// groupByValue) and no {{ template }} includes.
const sparqlTablePartial = "templates/partials/sparql-table.html"

// sparqlMetricPartial is the path to the metric partial. Like the table partial
// it stands alone (only dict/toJSON from funcMap, no {{ template }} includes),
// and it holds two defines: "sparqlMetric" (the card, rendered inline on pages)
// and "sparqlMetricValue" (the HTMX response body rendered here).
const sparqlMetricPartial = "templates/partials/sparql-metric.html"

// Per-language clones of each standalone partial, keyed by lang.Key(code).
// Populated once by InitPartials.
var (
	sparqlTableSets  map[string]*template.Template
	sparqlMetricSets map[string]*template.Template
)

// InitPartials parses the standalone partials and clones them per language.
// Returns an error rather than panicking so main can report a broken partial the
// same way it reports a broken config.
func InitPartials(cats *i18n.Catalogs, langs *lang.Set) error {
	codes := langs.Codes()
	if len(codes) == 0 {
		codes = []string{i18n.BaseCode}
	}
	var err error
	if sparqlTableSets, err = parsePartialPerLang(sparqlTablePartial, cats, langs, codes); err != nil {
		return err
	}
	if sparqlMetricSets, err = parsePartialPerLang(sparqlMetricPartial, cats, langs, codes); err != nil {
		return err
	}
	return nil
}

// parsePartialPerLang parses one standalone partial file and returns one clone
// per language, each with its own bound i18n functions.
func parsePartialPerLang(path string, cats *i18n.Catalogs, langs *lang.Set, codes []string) (map[string]*template.Template, error) {
	base, err := parseSet([]string{path})
	if err != nil {
		return nil, fmt.Errorf("parse partial %s: %w", path, err)
	}
	sets := make(map[string]*template.Template, len(codes))
	for _, code := range codes {
		variant, err := base.Clone()
		if err != nil {
			return nil, fmt.Errorf("clone partial %s for %q: %w", path, code, err)
		}
		sets[lang.Key(code)] = variant.Funcs(i18nFuncs(cats, langs, code))
	}
	return sets, nil
}

// pick returns the template set for a language, falling back to the base
// language so an unconfigured code renders English rather than failing the swap.
func pick(sets map[string]*template.Template, code string) (*template.Template, error) {
	if sets == nil {
		return nil, fmt.Errorf("partials not initialized; call InitPartials at startup")
	}
	if t, ok := sets[lang.Key(code)]; ok {
		return t, nil
	}
	if t, ok := sets[lang.Key(i18n.BaseCode)]; ok {
		return t, nil
	}
	return nil, fmt.Errorf("no template set for language %q", code)
}

// RenderSparqlTable renders the sparqlTable partial to standalone HTML for async
// (HTMX) fragment responses, in the given UI language. params is the same dict
// the pre-render path passes via {{ template "sparqlTable" (dict ...) }} — e.g.
// "result", "id", "title", "icon", "iconVar", "badgeVar". For large classes the
// async-table handler adds the working-set params "workingSet", "iri", "keyVar",
// "total", "complete", "searchProp", "max" (see the partial's header comment).
func RenderSparqlTable(code string, params map[string]any) (template.HTML, error) {
	tmpl, err := pick(sparqlTableSets, code)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, "sparqlTable", params); err != nil {
		return "", fmt.Errorf("render sparqlTable: %w", err)
	}
	return template.HTML(buf.String()), nil
}

// RenderSparqlMetricValue renders the sparqlMetricValue partial in the given UI
// language: the body of the /api/metric/:id HTMX response. params carries
// "queryId", "count", "query" and "endpoint" — the count is swapped into the
// card's value slot, while the query and endpoint become the config island
// backing its "Execute on endpoint" action.
func RenderSparqlMetricValue(code string, params map[string]any) (template.HTML, error) {
	tmpl, err := pick(sparqlMetricSets, code)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, "sparqlMetricValue", params); err != nil {
		return "", fmt.Errorf("render sparqlMetricValue: %w", err)
	}
	return template.HTML(buf.String()), nil
}
