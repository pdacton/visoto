package templates

// This file renders individual partials to standalone HTML fragments for async
// (HTMX) responses. The normal render path bundles every page with the full
// base.html document (see Load), so it cannot emit a bare partial; handlers that
// answer HTMX swaps use the helpers here instead.

import (
	"bytes"
	"fmt"
	"html/template"
	"sync"
)

// sparqlTablePartial is the path to the reusable table partial, parsed once and
// cached. It stands alone: it uses only funcMap functions (dict, toJSON,
// toJSONRaw, iconNames, render, groupByValue) and no {{ template }} includes.
const sparqlTablePartial = "templates/partials/sparql-table.html"

// sparqlMetricPartial is the path to the metric partial. Like the table partial
// it stands alone (only dict/toJSON from funcMap, no {{ template }} includes),
// and it holds two defines: "sparqlMetric" (the card, rendered inline on pages)
// and "sparqlMetricValue" (the HTMX response body rendered here).
const sparqlMetricPartial = "templates/partials/sparql-metric.html"

var (
	sparqlTableTmpl     *template.Template
	sparqlTableTmplErr  error
	sparqlTableTmplOnce sync.Once

	sparqlMetricTmpl     *template.Template
	sparqlMetricTmplErr  error
	sparqlMetricTmplOnce sync.Once
)

// RenderSparqlTable renders the sparqlTable partial to standalone HTML for async
// (HTMX) fragment responses. params is the same dict the pre-render path passes
// via {{ template "sparqlTable" (dict ...) }} — e.g. "result", "id", "title",
// "icon", "iconVar", "badgeVar". For large classes the async-table handler adds
// the working-set params "workingSet", "iri", "keyVar", "total", "complete",
// "searchProp", "max" (see the partial's header comment). The partial template is
// parsed once and cached; params are per-call, so caching is unaffected.
func RenderSparqlTable(params map[string]any) (template.HTML, error) {
	sparqlTableTmplOnce.Do(func() {
		sparqlTableTmpl, sparqlTableTmplErr = template.New("sparql-table.html").
			Funcs(funcMap).
			ParseFiles(sparqlTablePartial)
	})
	if sparqlTableTmplErr != nil {
		return "", fmt.Errorf("parse sparqlTable partial: %w", sparqlTableTmplErr)
	}

	var buf bytes.Buffer
	if err := sparqlTableTmpl.ExecuteTemplate(&buf, "sparqlTable", params); err != nil {
		return "", fmt.Errorf("render sparqlTable: %w", err)
	}
	return template.HTML(buf.String()), nil
}

// RenderSparqlMetricValue renders the sparqlMetricValue partial: the body of the
// /api/metric/:id HTMX response. params carries "queryId", "count", "query" and
// "endpoint" — the count is swapped into the card's value slot, while the query
// and endpoint become the config island backing its "Execute on endpoint" action.
// The partial template is parsed once and cached; params are per-call, so caching
// is unaffected.
func RenderSparqlMetricValue(params map[string]any) (template.HTML, error) {
	sparqlMetricTmplOnce.Do(func() {
		sparqlMetricTmpl, sparqlMetricTmplErr = template.New("sparql-metric.html").
			Funcs(funcMap).
			ParseFiles(sparqlMetricPartial)
	})
	if sparqlMetricTmplErr != nil {
		return "", fmt.Errorf("parse sparqlMetric partial: %w", sparqlMetricTmplErr)
	}

	var buf bytes.Buffer
	if err := sparqlMetricTmpl.ExecuteTemplate(&buf, "sparqlMetricValue", params); err != nil {
		return "", fmt.Errorf("render sparqlMetricValue: %w", err)
	}
	return template.HTML(buf.String()), nil
}
