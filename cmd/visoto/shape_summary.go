package main

import (
	"context"
	"fmt"
	"hash/crc32"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"hutzli.org/visoto/internal/bnodetree"
	"hutzli.org/visoto/internal/shapeprose"
	"hutzli.org/visoto/internal/templates"
)

// shapeSummaryDepth is how many levels the summary query fetches: the shape
// (0), its property shapes (1), their nested shapes such as sh:or alternatives
// or a qualified value shape (2), and the lists hanging off those (3), e.g. a
// qualified sh:languageIn. One query, no deeper stage: the summary explains no
// more than that, and the depth-6 tree query exhausts QLever's query memory on
// the DCAT-AP shapes.
const shapeSummaryDepth = 3

// shapeSummaryHandler serves GET /api/shape-summary — the plain-language
// summary of the SHACL shape ?iri= as the shapeSummaryCard fragment.
//
// ?collapsed= starts the card closed, ?title=/?icon= dress it. Like every /api
// route it is a pure function of the URL. A resource that is no shape gets an
// empty 200, so the placeholder swaps itself out.
func shapeSummaryHandler(c *gin.Context) {
	endpoint := activeEndpointURL(c)
	prefixes := cfg.PrefixesFor(endpoint)
	iri := strings.TrimSpace(c.Query("iri"))
	req := bnodetree.Request{IRI: iri, RootTriples: true, WithProperties: true}
	// The IRI is spliced into the query: validated first.
	if err := req.Validate(); err != nil {
		c.Header("Cache-Control", "no-store")
		c.String(http.StatusBadRequest, "invalid iri")
		return
	}

	dataLang := queryLang(c)
	preprocessor := prepareQueryInputs(c)
	ctx, cancel := context.WithTimeout(c.Request.Context(), cfg.GetTimeout())
	defer cancel()
	result, err := preprocessor.ExecuteQueryWithContext(ctx, req.QueryTo(shapeSummaryDepth), true, dataLang, "")

	params := map[string]any{
		"id":        fmt.Sprintf("shs-%08x", crc32.ChecksumIEEE([]byte(c.Request.URL.RawQuery))),
		"title":     c.DefaultQuery("title", "Summary"),
		"icon":      c.DefaultQuery("icon", "book-open-text"),
		"collapsed": c.Query("collapsed") != "",
	}
	if err != nil {
		// 200 with the error inside the card: htmx swaps nothing on an error
		// status, which would leave the skeleton spinning forever.
		params["error"] = err.Error()
		renderShapeSummary(c, params, false)
		return
	}

	rows := bnodetree.RowsFrom(result)
	graph := bnodetree.NewGraph(rows)
	summary := shapeprose.Explain(graph, iri, shapeprose.Options{Lang: dataLang, Prefixes: prefixes})
	if summary.Empty() {
		markCacheable(c)
		c.Header("Content-Type", "text/html")
		c.String(http.StatusOK, "")
		return
	}
	summary.Cut = graph.Truncated || bnodetree.NeedsDeeper(rows, shapeSummaryDepth)
	params["summary"] = summary
	renderShapeSummary(c, params, true)
}

// renderShapeSummary writes the card fragment; cacheable is false for errors.
func renderShapeSummary(c *gin.Context, params map[string]any, cacheable bool) {
	html, err := templates.RenderShapeSummary(queryLang(c), params)
	if err != nil {
		c.String(http.StatusInternalServerError, "failed to render shape summary")
		return
	}
	if cacheable {
		markCacheable(c)
	} else {
		c.Header("Cache-Control", "no-store")
	}
	c.Header("Content-Type", "text/html")
	c.String(http.StatusOK, string(html))
}
