package main

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"hutzli.org/visoto/internal/config"
	"hutzli.org/visoto/internal/icon"
	"hutzli.org/visoto/internal/sparql"
)

// resourceIconAPIHandler serves GET /api/resource-icon?iri=&endpoint= — the icon
// the resource page header would show for ?iri=, as {"icon": "<path>"}.
//
// It exists for "My Links": a bookmark is often created from a link with no type
// icon beside it (the page title, a breadcrumb, a relationship card), so the
// sidebar asks here instead of scraping the page. The rule is icon.Resolve over
// the resource's rdf:types, the same one the header uses.
//
// A miss is {"icon": ""}, not the header's default.svg: what a miss looks like is
// the caller's choice. Like every /api route it is a pure function of the URL.
func resourceIconAPIHandler(c *gin.Context) {
	endpoint := activeEndpointURL(c)
	iri := config.ExpandCURIE(strings.TrimSpace(c.Query("iri")), cfg.PrefixesFor(endpoint))
	// QueryTypes splices the IRI into the query: validated first.
	if err := sparql.ValidateIRI(iri); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid iri"})
		return
	}

	types, err := prepareQueryInputs(c).QueryTypes(iri)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	markCacheable(c)
	c.JSON(http.StatusOK, gin.H{"icon": icon.Resolve(iri, types)})
}
