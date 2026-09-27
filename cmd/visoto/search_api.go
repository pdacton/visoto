package main

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"hutzli.org/visoto/internal/search"
)

// searchAPIMaxLimit caps ?limit= on /api/search: the response feeds a dropdown.
const searchAPIMaxLimit = 50

// searchHit is one row of /api/search.
type searchHit struct {
	IRI   string `json:"iri"`
	Label string `json:"label"`
	Type  string `json:"type,omitempty"`
}

// searchAPIHandler serves GET /api/search?q=&endpoint=&lang=&limit= — the label
// search behind the /search page (full-text provider of the endpoint, CONTAINS
// fallback), as JSON. The graph's "Add resource" field uses it (GL-47): GE's own
// lookup is a regex scan that does not finish on LINDAS.
//
// Like every /api route it is a pure function of the URL — endpoint and
// language come from epFromURL / langFromURL, never a cookie — so the shared
// cache may keep it.
func searchAPIHandler(c *gin.Context) {
	params := search.ParseParams(c)
	params.Query = strings.TrimSpace(params.Query)
	if params.Query == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing q"})
		return
	}
	if params.Limit <= 0 || params.Limit > searchAPIMaxLimit {
		params.Limit = search.DefaultLimit
	}

	providerName := "stardog"
	if ep := activeEndpoint(c); ep != nil && ep.SearchProvider != "" {
		providerName = ep.SearchProvider
	}
	searcher := search.New(prepareQueryInputs(c).SparqlPreprocessor(), providerName, activeEndpointURL(c))
	result := searcher.Execute(c.Request.Context(), params, queryLang(c))
	if result.Results.Error != "" {
		c.JSON(http.StatusBadGateway, gin.H{"error": result.Results.Error})
		return
	}

	hits := make([]searchHit, 0, len(result.Results.Bindings))
	seen := map[string]bool{}
	for _, row := range result.Results.Bindings {
		subject := row["subject"]
		if subject.Type != "uri" || seen[subject.Value] {
			continue
		}
		seen[subject.Value] = true
		label := row["matchedText"].Value
		if label == "" {
			label = subject.DisplayText
		}
		typ := row["type"].DisplayText
		if typ == "" {
			typ = row["type"].Value
		}
		hits = append(hits, searchHit{IRI: subject.Value, Label: label, Type: typ})
	}
	c.JSON(http.StatusOK, hits)
}
