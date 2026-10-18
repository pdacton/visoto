package main

import (
	"context"
	"fmt"
	"hash/crc32"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"hutzli.org/visoto/internal/bnodetree"
	"hutzli.org/visoto/internal/config"
	"hutzli.org/visoto/internal/templates"
)

// bnodeTreeFormats maps ?format= to the download's content type and extension.
var bnodeTreeFormats = map[string]struct{ mime, ext string }{
	"ttl":    {"text/turtle; charset=utf-8", "ttl"},
	"nt":     {"application/n-triples; charset=utf-8", "nt"},
	"jsonld": {"application/ld+json; charset=utf-8", "jsonld"},
}

// bnodeTreeHandler serves GET /api/bnode-tree — the outgoing blank-node
// structure of ?iri= as the bnodeTreeCard fragment, or with ?format=ttl|nt|jsonld
// as a download of the same triples.
//
// Optional: ?property= restricts the first hop, ?via= follows one predicate to
// IRI roots first (both CURIE or full IRI), ?collapsed= starts nodes closed,
// ?title=/?icon=/?anchor= dress the card.
//
// Like every /api route it is a pure function of the URL — endpoint and language
// come from epFromURL / langFromURL — so the shared cache may keep it. A resource
// without blank nodes gets an empty 200: the placeholder swaps itself out
// (hx-swap="outerHTML") and nothing is shown.
func bnodeTreeHandler(c *gin.Context) {
	endpoint := activeEndpointURL(c)
	prefixes := cfg.PrefixesFor(endpoint)
	req := bnodetree.Request{
		IRI:      strings.TrimSpace(c.Query("iri")),
		Property: expandParam(c.Query("property"), prefixes),
		Via:      expandParam(c.Query("via"), prefixes),
	}
	format := c.Query("format")
	if _, ok := bnodeTreeFormats[format]; format != "" && !ok {
		c.String(http.StatusBadRequest, "unknown format")
		return
	}
	// The IRIs are request input spliced into the query: validated first, so a
	// crafted one cannot close the <...> term and inject graph patterns.
	if err := req.Validate(); err != nil {
		c.Header("Cache-Control", "no-store")
		c.String(http.StatusBadRequest, "invalid iri, property or via")
		return
	}

	dataLang := queryLang(c)
	preprocessor := prepareQueryInputs(c)
	query := req.Query()
	ctx, cancel := context.WithTimeout(c.Request.Context(), cfg.GetTimeout())
	defer cancel()
	result, err := preprocessor.ExecuteQueryWithContext(ctx, query, true, dataLang, "")

	params := map[string]any{
		"id":        bnodeTreeID(c.Request.URL.RawQuery),
		"title":     c.DefaultQuery("title", "Nested structure"),
		"icon":      c.DefaultQuery("icon", "brackets"),
		"anchor":    c.Query("anchor"),
		"rowCap":    bnodetree.RowCap,
		"query":     preprocessor.FinalizeQuery(query, dataLang, endpoint),
		"endpoint":  endpoint,
		"downloads": bnodeTreeDownloads(c),
	}

	if err != nil {
		if format != "" {
			c.Header("Cache-Control", "no-store")
			c.String(http.StatusBadGateway, "query failed: %v", err)
			return
		}
		// 200 with the error inside the card: htmx swaps nothing on an error
		// status, which would leave the skeleton spinning forever.
		params["error"] = err.Error()
		renderBnodeTree(c, params, false)
		return
	}

	graph := bnodetree.NewGraph(bnodetree.RowsFrom(result))
	if format != "" {
		writeBnodeTreeDownload(c, graph, format, prefixes)
		return
	}
	if graph.Empty() {
		markCacheable(c)
		c.Header("Content-Type", "text/html")
		c.String(http.StatusOK, "")
		return
	}
	params["tree"] = bnodetree.Build(graph, bnodetree.Options{
		Lang:      dataLang,
		Prefixes:  prefixes,
		Collapsed: c.Query("collapsed") != "",
	})
	renderBnodeTree(c, params, true)
}

// expandParam turns a CURIE into a full IRI with the endpoint's prefixes;
// full IRIs and empty values pass through. Validation happens afterwards.
func expandParam(v string, prefixes []config.Prefix) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	return config.ExpandCURIE(v, prefixes)
}

// bnodeTreeID derives a stable element id from the request, so two cards on
// one page (the Data view's and a template's) never share ids.
func bnodeTreeID(rawQuery string) string {
	return fmt.Sprintf("bnt-%08x", crc32.ChecksumIEEE([]byte(rawQuery)))
}

// bnodeTreeDownloads builds the download links from the request's own URL, so
// they carry the same iri, endpoint and lang and stay cacheable by URL.
func bnodeTreeDownloads(c *gin.Context) map[string]string {
	out := make(map[string]string, len(bnodeTreeFormats))
	for f := range bnodeTreeFormats {
		q := c.Request.URL.Query()
		for _, k := range []string{"title", "icon", "anchor", "collapsed", "_"} {
			q.Del(k)
		}
		q.Set("format", f)
		out[f] = c.Request.URL.Path + "?" + q.Encode()
	}
	return out
}

func writeBnodeTreeDownload(c *gin.Context, g *bnodetree.Graph, format string, prefixes []config.Prefix) {
	var body []byte
	switch format {
	case "ttl":
		body = []byte(g.Turtle(prefixes))
	case "nt":
		body = []byte(g.NTriples())
	case "jsonld":
		b, err := g.JSONLD()
		if err != nil {
			c.String(http.StatusInternalServerError, "failed to serialize")
			return
		}
		body = b
	}
	f := bnodeTreeFormats[format]
	markCacheable(c)
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="blank-nodes.%s"`, f.ext))
	c.Data(http.StatusOK, f.mime, body)
}

// renderBnodeTree writes the card fragment; cacheable is false for errors,
// which must never be served stale.
func renderBnodeTree(c *gin.Context, params map[string]any, cacheable bool) {
	html, err := templates.RenderBnodeTree(queryLang(c), params)
	if err != nil {
		c.String(http.StatusInternalServerError, "failed to render blank-node tree")
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
