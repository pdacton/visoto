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
	"hutzli.org/visoto/internal/facet"
	"hutzli.org/visoto/internal/parser"
	"hutzli.org/visoto/internal/sparql"
)

// facetedResultCap bounds an inline faceted result. Faceting exists to narrow a
// class to a manageable set; if a weak filter still exceeds this, we return the
// capped rows and the frontend notes the result was truncated. (Working-set-aware
// faceting — pushing constraints into the bounded /api/async-table-data loader —
// is a v2 follow-up; the current working-set loader rebuilds from the unfiltered
// declared query and would ignore facet constraints.)
const facetedResultCap = defaultMaxWorkingSet

// findFacetSpecs collects the <sparql-facet for=baseID> declarations across the
// async template directories, in document order. Mirrors findAsyncQuery.
//
// NOTE: "for" is a GLOBAL id namespace — specs are matched across every async
// template dir, not just the page being rendered. Two pages that reuse one base
// query id would therefore silently merge each other's facets. Keep base query
// ids unique across templates.
//
// The scan is memoized (see templateScanCache) because both this and
// collectConstraints call it per request on a cacheable hot path.
func findFacetSpecs(baseID string) []facet.FacetSpec {
	var specs []facet.FacetSpec
	for _, el := range scanTemplateElements(parser.ExtractFacetElements, "facet") {
		if el.Attributes["for"] != baseID {
			continue
		}
		specs = append(specs, facet.FacetSpec{
			Var:     strings.TrimPrefix(el.Attributes["var"], "?"),
			Root:    el.Attributes["root"],
			Path:    el.Attributes["path"],
			Type:    el.Attributes["type"],
			Control: el.Attributes["control"],
			Label:   el.Attributes["label"],
		})
	}
	return specs
}

// findFacetSpec returns the single facet declared as (baseID, varName), if any.
func findFacetSpec(baseID, varName string) (facet.FacetSpec, bool) {
	for _, s := range findFacetSpecs(baseID) {
		if s.Var == varName {
			return s, true
		}
	}
	return facet.FacetSpec{}, false
}

// ---- Phase A: facet value enumeration ----

type facetValue struct {
	Value string `json:"value"`
	Label string `json:"label"`
	Type  string `json:"type"`
	Count int    `json:"count"`
}

type facetValuesEntry struct {
	values  []facetValue
	expires time.Time
}

func (e facetValuesEntry) expiredAt(now time.Time) bool { return now.After(e.expires) }

var (
	facetValuesMu    sync.Mutex
	facetValuesCache = map[string]facetValuesEntry{}
)

const facetValuesTTL = 5 * time.Minute

// facetValuesHandler serves GET /api/facet-values/:id/:var — the distinct values
// of one select facet (with per-value member counts) used to populate its
// dropdown. Enumeration is bounded (LIMIT) and cached in-process + at Souin, as
// it is the most expensive, most reused call in the feature.
func facetValuesHandler(c *gin.Context) {
	id := c.Param("id")
	varName := strings.TrimPrefix(c.Param("var"), "?")
	dataLang := queryLang(c)

	declared, found := findAsyncQuery(id)
	if !found {
		c.JSON(http.StatusNotFound, gin.H{"values": []facetValue{}})
		return
	}
	spec, ok := findFacetSpec(id, varName)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"values": []facetValue{}})
		return
	}
	// Every mode needs the IRI (the declared query's ?? placeholder must be
	// resolved, and class/instance mode anchor on it). A class-membership key is
	// required only by class mode — see enumerationQuery.
	classIRI := c.Query("iri")
	if classIRI == "" {
		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusBadRequest, gin.H{"values": []facetValue{},
			"error": "iri is required to enumerate facet values"})
		return
	}

	endpoint := activeEndpointURL(c)
	// Value labels are resolved per language (?lang= on the request), so the
	// language must be part of the cache key — otherwise the first requester's
	// language would be served to everyone for the TTL. A bare code keeps this
	// key space bounded to the configured languages; the raw Accept-Language it
	// replaced was effectively unbounded.
	cacheKey := endpoint + "|" + id + "|" + varName + "|" + classIRI + "|" + dataLang
	facetValuesMu.Lock()
	if e, ok := facetValuesCache[cacheKey]; ok && time.Now().Before(e.expires) {
		facetValuesMu.Unlock()
		writeFacetValues(c, e.values)
		return
	}
	facetValuesMu.Unlock()

	query, err := enumerationQuery(declared, classIRI, spec)
	if err != nil {
		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusOK, gin.H{"values": []facetValue{}, "error": err.Error()})
		return
	}

	preprocessor := prepareQueryInputs(c)
	ctx, cancel := context.WithTimeout(c.Request.Context(), cfg.GetTimeout())
	defer cancel()
	result, err := preprocessor.ExecuteQueryWithContext(ctx, query, true, dataLang, "")
	if err != nil {
		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusOK, gin.H{"values": []facetValue{}, "error": err.Error()})
		return
	}

	values := make([]facetValue, 0, len(result.Bindings))
	for _, b := range result.Bindings {
		vb, ok := b[varName]
		if !ok {
			continue
		}
		label := vb.DisplayText
		if label == "" {
			label = vb.Value
		}
		count := 0
		if cb, ok := b["count"]; ok {
			count, _ = strconv.Atoi(cb.Value)
		}
		values = append(values, facetValue{Value: vb.Value, Label: label, Type: vb.Type, Count: count})
	}

	// Bounded: the key embeds request input (class IRI, Accept-Language), so it is
	// swept of expired entries and capped rather than allowed to grow unbounded.
	// Over the cap we simply skip caching — serving the fresh result is always
	// correct, just not cached.
	facetValuesMu.Lock()
	if sweepExpired(facetValuesCache) {
		facetValuesCache[cacheKey] = facetValuesEntry{values: values, expires: time.Now().Add(facetValuesTTL)}
	}
	facetValuesMu.Unlock()

	writeFacetValues(c, values)
}

// enumerationQuery builds the value-enumeration query for a spec, dispatching on
// its declared mode (see facet.FacetSpec):
//
//	column   — wrap the declared query and group by the projected variable
//	instance — walk the path from the one fixed resource
//	class    — walk the path from each member of the class (the cheap shape)
//
// declared still carries the ?? placeholder; classIRI is the resolved page IRI.
func enumerationQuery(declared, classIRI string, spec facet.FacetSpec) (string, error) {
	if err := spec.Validate(); err != nil {
		return "", err
	}
	root := strings.TrimSpace(spec.Root)

	if strings.TrimSpace(spec.Path) == "" {
		full, err := sparql.SubstituteEntity(declared, classIRI)
		if err != nil {
			return "", err
		}
		return facet.BuildColumnValuesQuery(full, spec, facet.DefaultEnumerateLimit)
	}
	if root == facet.InstanceRoot {
		return facet.BuildInstanceValuesQuery(classIRI, spec, facet.DefaultEnumerateLimit)
	}

	// Class mode: an explicit root wins over the sniffed key var, which is what
	// lets a query hide its membership triple behind a BIND and still enumerate.
	keyVar := strings.TrimPrefix(root, "?")
	if keyVar == "" {
		keyVar = sparql.DeriveKeyVar(declared)
	}
	if keyVar == "" {
		return "", fmt.Errorf("facet %q: a class-membership key is required — add root=\"?var\" naming the entity variable, or drop path to filter the column directly", spec.Var)
	}
	return facet.Default().EnumerateQuery(classIRI, keyVar, spec, facet.DefaultEnumerateLimit)
}

func writeFacetValues(c *gin.Context, values []facetValue) {
	markCacheable(c)
	c.JSON(http.StatusOK, gin.H{"values": values})
}

// ---- Phase B: filtered result table ----

// facetedTableHandler serves GET /api/faceted-table/:id — it rewrites the base
// <sparql-async> query with the active facet selections (read from f.<var> query
// params) and returns a rendered sparqlTable fragment. Free-text facets make the
// URL key space unbounded, so a response that applied one is not cached.
func facetedTableHandler(c *gin.Context) {
	id := c.Param("id")
	dataLang := queryLang(c)

	// The only /api route whose body depends on a request header: wantsJSON
	// picks the JSON envelope or the HTML fragment off Accept, and both share one
	// URL. Everything else on this tier is URL-pure and sends no Vary at all (see
	// etag.go), so this must be declared explicitly or a shared cache could hand
	// the JSON envelope to an HTML caller. Set before any return path.
	c.Header("Vary", "Accept")

	declared, found := findAsyncQuery(id)
	if !found {
		c.String(http.StatusNotFound, "unknown async query id")
		return
	}
	// A faceted table is always class-scoped, so the class IRI is required (and
	// validated) rather than optional: without it the ?? placeholder would survive
	// into the query and the endpoint would reject it after a wasted round-trip.
	// Mirrors the guard in facetValuesHandler.
	classIRI := c.Query("iri")
	fullQuery, err := sparql.SubstituteEntity(declared, classIRI)
	if err != nil {
		c.Header("Cache-Control", "no-store")
		if wantsJSON(c) {
			writeFacetedEnvelope(c, sparql.QueryResult{Error: err.Error()}, 0, true, false)
			return
		}
		c.String(http.StatusBadRequest, "invalid iri")
		return
	}

	keyVar := sparql.DeriveKeyVar(declared)
	constraints, hasText := collectConstraints(c, id)

	faceted, err := facet.BuildFacetedQuery(fullQuery, keyVar, constraints, facet.Default())
	params := map[string]any{
		"id":       id,
		"title":    c.Query("title"),
		"icon":     c.Query("icon"),
		"iconVar":  c.Query("iconVar"),
		"badgeVar": c.Query("badgeVar"),
		"groupBy":  c.Query("groupBy"),
		// See asyncTableHandler: carried into the fragment so the second-stage
		// fetches can pass ?lang= on. Unconditional — "" is a real code.
		"lang": dataLang,
	}
	if err != nil {
		// Invalid selection (e.g. injection attempt) — surface it, never cache.
		if wantsJSON(c) {
			writeFacetedEnvelope(c, sparql.QueryResult{Error: err.Error()}, 0, true, false)
			return
		}
		params["result"] = sparql.QueryResult{Error: err.Error()}
		renderTableFragment(c, params, false)
		return
	}

	// Cap the inline result so a weak filter can't materialize an unbounded set.
	// Authors may lower the cap via ?max= (e.g. for a huge class whose unfiltered
	// initial view would otherwise pull a very large fragment); it is clamped to
	// facetedResultCap and never raised above it.
	limit := facetedResultCap
	if m, mErr := strconv.Atoi(c.Query("max")); mErr == nil && m > 0 && m < facetedResultCap {
		limit = m
	}
	faceted = sparql.StripTrailingLimitOffset(faceted) + "\nLIMIT " + strconv.Itoa(limit)

	preprocessor := prepareQueryInputs(c)
	ctx, cancel := context.WithTimeout(c.Request.Context(), cfg.GetTimeout())
	defer cancel()
	result, execErr := preprocessor.ExecuteQueryWithContext(ctx, faceted, true, dataLang, "")
	if execErr != nil {
		result.Error = execErr.Error()
	}

	// Cache carve-out: enum/range selections are a bounded, shareable key space;
	// a free-text facet is not, so never persist those in the shared cache.
	cacheable := execErr == nil && !hasText

	// Content negotiation: JSON callers (the header-facet Tabulator, which setData()s
	// the filtered result under the one working-set table instance) get the same
	// {vars,data,total,complete} envelope the working-set loader returns, so the
	// frontend can reuse its banner/completeness logic. `complete` is true iff the
	// filtered result wasn't capped — mirrors asyncTableDataHandler's search branch.
	if wantsJSON(c) {
		// Without a key variable there is no entity to count distinctly — the row
		// IS the unit — so fall back to the row count rather than reporting 0.
		distinct := sparql.DistinctKeyCount(result, keyVar)
		if keyVar == "" {
			distinct = len(result.Bindings)
		}
		writeFacetedEnvelope(c, result, distinct, distinct < limit, cacheable)
		return
	}

	params["result"] = result
	renderTableFragment(c, params, cacheable)
}

// wantsJSON reports whether the caller prefers the JSON envelope over an HTML
// fragment, based on the Accept header.
func wantsJSON(c *gin.Context) bool {
	return strings.Contains(c.GetHeader("Accept"), "application/json")
}

// writeFacetedEnvelope emits the working-set JSON envelope for a faceted result,
// honoring the free-text cache carve-out (writeWorkingSet always marks the response
// public, so the no-store case is emitted here instead).
func writeFacetedEnvelope(c *gin.Context, result sparql.QueryResult, total int, complete, cacheable bool) {
	if !cacheable {
		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusOK, gin.H{
			"vars":     result.Vars,
			"data":     result.Bindings,
			"total":    total,
			"complete": complete,
			"error":    result.Error,
		})
		return
	}
	writeWorkingSet(c, result, total, complete)
}

// collectConstraints reads the active facet selections for base query id from the
// request. Multi-value select facets repeat f.<var>; range facets pass f.<var>.min
// / f.<var>.max; text facets pass a single f.<var>. Any facet may also request
// members lacking a value: select carries facet.NoValueSentinel as a repeated
// f.<var> value (from the checkbox list), while range/text carry it out-of-band as
// f.<var>.novalue=1 so their positional Values stay clean. Returns the constraints
// with at least one active selection, and whether any active facet is free-text.
func collectConstraints(c *gin.Context, id string) (constraints []facet.FacetConstraint, hasText bool) {
	for _, spec := range findFacetSpecs(id) {
		key := "f." + spec.Var
		var values []string
		noValue := false
		switch spec.Control {
		case facet.ControlRange:
			values = []string{c.Query(key + ".min"), c.Query(key + ".max")}
			noValue = isTruthy(c.Query(key + ".novalue"))
			if values[0] == "" && values[1] == "" && !noValue {
				continue
			}
		case facet.ControlText:
			term := c.Query(key)
			noValue = isTruthy(c.Query(key + ".novalue"))
			if strings.TrimSpace(term) == "" && !noValue {
				continue
			}
			values = []string{term}
			if strings.TrimSpace(term) != "" {
				hasText = true
			}
		default: // select
			values = c.QueryArray(key)
			if len(values) == 0 {
				continue
			}
		}
		constraints = append(constraints, facet.FacetConstraint{Spec: spec, Values: values, NoValue: noValue})
	}
	return constraints, hasText
}

// isTruthy reports whether a query-param string represents an enabled flag.
func isTruthy(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}
