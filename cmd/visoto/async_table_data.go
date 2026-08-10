package main

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"hutzli.org/visoto/internal/parser"
	"hutzli.org/visoto/internal/sparql"
)

// defaultMaxWorkingSet is the default cap on rows loaded into a single working
// set (the working-set table model). The frontend holds this many rows locally
// and pages/sorts/searches over them without further round-trips. Also the
// threshold above which asyncTableHandler switches a class table from inline to
// working-set mode. Authors may override per table via the "max" query param.
const defaultMaxWorkingSet = 20000

// asyncTableDataHandler serves GET /api/async-table-data/:id — the JSON backend
// that loads one bounded working set for a class-instance table. It returns the
// whole class when it fits under max, or the first max keys (or the first max
// search matches) otherwise; the frontend then pages/sorts/searches locally with
// no further round-trips. Unlike a remote pager it never uses a deep OFFSET, so
// query cost stays flat across the session.
//
// The query-building is delegated to internal/sparql (BuildWorkingSetQuery,
// MembershipBody, …); this handler is the HTTP glue that resolves the declared
// query, runs it, and shapes the JSON envelope.
//
// Query params:
//
//	iri        class IRI (the ?? substitution value)
//	keyVar     the class-membership key variable (e.g. "taxonName"); required
//	max        optional working-set cap (default/clamped to defaultMaxWorkingSet)
//	search     optional full-class search term (rebuilds the working set)
//	searchProp optional name property IRI to search (default rdfs:label)
func asyncTableDataHandler(c *gin.Context) {
	id := c.Param("id")
	dataLang := queryLang(c)

	declared, found := findAsyncQuery(c.Query("src"), id)
	if !found {
		c.JSON(http.StatusNotFound, gin.H{"data": []any{}, "total": 0, "complete": true})
		return
	}

	classIRI := c.Query("iri")
	keyVar := strings.TrimPrefix(c.Query("keyVar"), "?")
	if keyVar == "" {
		keyVar = sparql.DeriveKeyVar(declared) // fall back to deriving it server-side
	}
	if classIRI == "" || keyVar == "" {
		c.JSON(http.StatusBadRequest, gin.H{"data": []any{}, "total": 0, "complete": true,
			"error": "iri and a derivable key variable are required for a working set"})
		return
	}

	max, _ := strconv.Atoi(c.DefaultQuery("max", strconv.Itoa(defaultMaxWorkingSet)))
	if max < 1 || max > 50000 {
		max = defaultMaxWorkingSet
	}

	preprocessor := prepareQueryInputs(c)
	endpoint := activeEndpointURL(c)
	ctx, cancel := context.WithTimeout(c.Request.Context(), cfg.GetTimeout())
	defer cancel()

	term := c.Query("search")
	searchProp := c.DefaultQuery("searchProp", "http://www.w3.org/2000/01/rdf-schema#label")

	// One capped query loads the working set: browse mode returns the first max
	// keys of the class; search mode restricts to instances whose name property or
	// key IRI CONTAINS the term. Building search into the instance query (rather
	// than the separate search subsystem, which returns subject/matchedText rows)
	// keeps the result columns identical to the browsed table.
	wsQuery, err := sparql.BuildWorkingSetQuery(declared, classIRI, keyVar, term, searchProp, max)
	if err != nil {
		// Malformed iri/keyVar/searchProp — reject rather than sending a query
		// built from unvalidated input. Never cached.
		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusBadRequest, gin.H{"data": []any{}, "total": 0, "complete": true,
			"error": err.Error()})
		return
	}
	result, err := preprocessor.ExecuteQueryWithContext(ctx, wsQuery, true, dataLang, "")
	if err != nil {
		// A transient SPARQL failure must never be cached as if it were real data.
		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusOK, gin.H{"data": []any{}, "total": 0, "complete": true,
			"error": err.Error()})
		return
	}

	distinct := sparql.DistinctKeyCount(result, keyVar)

	// Determine total + completeness. Browse mode: a class-only COUNT (cheap, ~1s,
	// cached) gives the exact total, and the set is complete iff it wasn't capped.
	// Search mode skips the COUNT (a full-scan search count is as expensive as the
	// search itself); it's complete iff the match page didn't hit the cap.
	var total int
	var complete bool
	if term == "" {
		total = cachedInstanceCount(ctx, preprocessor, id, endpoint, classIRI, keyVar, "", searchProp, dataLang)
		complete = distinct < max
		if complete {
			total = distinct // COUNT and load agree when uncapped; prefer the loaded count
		}
	} else {
		total = distinct
		complete = distinct < max
	}
	writeWorkingSet(c, result, total, complete)
}

// writeWorkingSet emits the JSON envelope the working-set sparqlTable path expects:
// the full column list, every row of the working set, and whether that set is the
// entire in-scope population (complete) or a capped subset (search to load more).
func writeWorkingSet(c *gin.Context, result sparql.QueryResult, total int, complete bool) {
	markCacheable(c)
	c.JSON(http.StatusOK, gin.H{
		"vars":     result.Vars,
		"data":     result.Bindings,
		"total":    total,
		"complete": complete,
		"error":    result.Error,
	})
}

// ---- instance count cache ----

type countEntry struct {
	value   int
	expires time.Time
}

func (e countEntry) expiredAt(now time.Time) bool { return now.After(e.expires) }

var (
	countCacheMu sync.Mutex
	countCache   = map[string]countEntry{}
)

const countCacheTTL = 5 * time.Minute

// instanceCountKey builds the count cache key. Extracted so the "two languages
// must not collide" property is unit-testable without a live preprocessor.
func instanceCountKey(endpoint, id, classIRI, term, dataLang string) string {
	return endpoint + "|" + id + "|" + classIRI + "|" + term + "|" + dataLang
}

// cachedInstanceCount returns the number of instances in scope — the whole class
// in browse mode, or just the search matches when a term is given. Computed from
// the membership scope alone (no OPTIONALs, cheap: ~1s class-only). Cached per
// (endpoint+id+class+term+lang) for a short TTL. The endpoint is part of the key
// so the LINDAS prod/int/test switcher never serves a stale cross-endpoint count;
// the language is part of it because a membership pattern may use
// visoto:dispLang, which resolves into the query being counted.
func cachedInstanceCount(ctx context.Context, preprocessor *parser.Preprocessor, id, endpoint, classIRI, keyVar, term, searchProp, dataLang string) int {
	key := instanceCountKey(endpoint, id, classIRI, term, dataLang)
	countCacheMu.Lock()
	if e, ok := countCache[key]; ok && time.Now().Before(e.expires) {
		countCacheMu.Unlock()
		return e.value
	}
	countCacheMu.Unlock()

	body, err := sparql.MembershipBody(classIRI, keyVar, term, searchProp)
	if err != nil {
		// Invalid class IRI / key var / search property: treat as "no count" (the
		// caller falls back to inline-local) rather than querying with bad input.
		return 0
	}
	countQuery := fmt.Sprintf("SELECT (COUNT(*) AS ?count) WHERE { %s }", body)
	result, err := preprocessor.ExecuteQueryWithContext(ctx, countQuery, false, dataLang, "")
	if err != nil {
		return 0 // don't cache errors; caller falls back to inline-local
	}
	total := 0
	if len(result.Bindings) > 0 {
		if b, ok := result.Bindings[0]["count"]; ok {
			total, _ = strconv.Atoi(b.Value)
		}
	}

	// Cache all successful counts, including zero — an empty class must not
	// re-issue the COUNT on every load. Bounded: the key embeds request input
	// (class IRI, search term), so it is swept and capped rather than grown.
	countCacheMu.Lock()
	if sweepExpired(countCache) {
		countCache[key] = countEntry{value: total, expires: time.Now().Add(countCacheTTL)}
	}
	countCacheMu.Unlock()
	return total
}
