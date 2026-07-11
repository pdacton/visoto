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

var (
	sparqlTableTmpl     *template.Template
	sparqlTableTmplErr  error
	sparqlTableTmplOnce sync.Once
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
