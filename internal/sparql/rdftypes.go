package sparql

// rdftypes.go resolves the rdf:type of the IRIs a query result mentions, so a
// table can show the icon of an entity's *class* rather than only of the entity
// itself.
//
// It deliberately mirrors labels.go — same batching, same cache shape, same
// best-effort contract — but runs as a SEPARATE query rather than as an extra
// clause on the label query. That keeps both queries trivial for the endpoint
// planner (this one is a bound-subject triple pattern; the label query is a
// nested aggregate), keeps a failure in one from taking out the other, and lets
// this cache be keyed by IRI alone: a resource's types do not depend on the
// visitor's language, so a German visit warms the cache for a French one.

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"hutzli.org/visoto/internal/cache"
	"hutzli.org/visoto/internal/icon"
	"hutzli.org/visoto/internal/logger"
)

// typeCache holds rdf:type IRIs per (endpoint, IRI), and is held far longer
// than labels: a resource's class is essentially static, while its label can be
// re-translated.
//
// Keyed by endpoint as well as IRI, like the label cache (bestCacheKey): the
// same IRI can be described differently on two endpoints, and the answer one
// gave must not stand in for the other's. Not keyed by language — types do not
// vary by it, so a German visit still warms the cache for a French one.
var typeCache = cache.New[[]string](6 * time.Hour)

func typeCacheKey(endpointURL, iri string) string { return endpointURL + "\x00" + iri }

func getCachedTypes(endpointURL, iri string) ([]string, bool) {
	return typeCache.Get(typeCacheKey(endpointURL, iri))
}

func setCachedTypes(endpointURL, iri string, types []string) {
	typeCache.Set(typeCacheKey(endpointURL, iri), types)
}

// buildTypeQuery constructs the SPARQL query fetching types for the given IRIs.
//
// Note what is NOT here. No OPTIONAL: an untyped IRI simply produces no rows,
// and "absent" already means "no icon". No GROUP BY or GROUP_CONCAT: a
// multi-typed IRI comes back as several rows, which is exactly what
// icon.Resolve wants — it scans the whole list for an exact match before
// accepting a fallback — so there is nothing to pack and unpack.
func buildTypeQuery(iris []string) string {
	if len(iris) == 0 {
		return ""
	}

	var values strings.Builder
	values.WriteString("VALUES ?iri { ")
	for _, iri := range iris {
		fmt.Fprintf(&values, "<%s> ", iri)
	}
	values.WriteString("}")

	// The same rdf:type|owl:type alternation the resource page header uses
	// (templates/components/pageHeader.html), so header and tables agree on what
	// counts as a type.
	return fmt.Sprintf(`
PREFIX rdf: <http://www.w3.org/1999/02/22-rdf-syntax-ns#>
PREFIX owl: <http://www.w3.org/2002/07/owl#>

SELECT ?iri ?type WHERE {
  %s
  ?iri (rdf:type|owl:type) ?type .
}`, values.String())
}

// fetchTypesBatch queries one batch of IRIs (≤ labelQueryBatchSize) and merges
// the result into typeMap.
//
// Every IRI in the batch is cached, including those the endpoint returned
// nothing for: "this IRI has no usable type" is an answer worth remembering, or
// every page view would re-ask for the same untyped IRIs. On a transport or
// parse failure nothing is cached, so the next request retries.
func fetchTypesBatch(p *Preprocessor, endpointURL string, iris []string, typeMap map[string][]string) {
	query := p.finalizeQuery(buildTypeQuery(iris), "", endpointURL)
	response, err := p.querySparqlEndpoint(context.Background(), endpointURL, query)
	if err != nil {
		logger.Get().Warn("type query failed, icons will be unresolved",
			slog.String("error", err.Error()),
			slog.Int("iri_count", len(iris)))
		return
	}

	var resp sparqlResponse
	if err := json.Unmarshal(response, &resp); err != nil {
		logger.Get().Warn("type response parse failed",
			slog.String("error", err.Error()))
		return
	}

	for _, binding := range resp.Results.Bindings {
		iri := binding["iri"].Value
		typ := binding["type"].Value
		if iri == "" || typ == "" {
			continue
		}
		typeMap[iri] = append(typeMap[iri], typ)
	}

	for _, iri := range iris {
		setCachedTypes(endpointURL, iri, typeMap[iri]) // nil for an untyped IRI — still an answer
	}
}

// fetchTypes returns IRI → rdf:type IRIs, reading through the cache and
// splitting the remainder into parallel batches, exactly as fetchLabels does.
func fetchTypes(p *Preprocessor, endpointURL string, iris []string) map[string][]string {
	typeMap := make(map[string][]string)
	uncached := make([]string, 0, len(iris))

	for _, iri := range iris {
		if types, found := getCachedTypes(endpointURL, iri); found {
			if len(types) > 0 {
				typeMap[iri] = types
			}
		} else {
			uncached = append(uncached, iri)
		}
	}
	if len(uncached) == 0 {
		return typeMap
	}

	batches := chunk(uncached, labelQueryBatchSize)
	results := make([]map[string][]string, len(batches))
	runParallel(len(batches), func(i int) {
		results[i] = make(map[string][]string)
		fetchTypesBatch(p, endpointURL, batches[i], results[i])
	})
	for _, batch := range results {
		for k, v := range batch {
			typeMap[k] = v
		}
	}

	return typeMap
}

// IconProperty is the magic property naming a resource's own icon
// ([rdf.magic_properties] icon = "schema:thumbnail"). Unset, nothing is queried.
const IconProperty = "icon"

// fetchIconOverrides returns IRI → the resource's own icon value, through the
// same best-value machinery as labels (fetchBest): one batched query, cached per
// endpoint and IRI, the earlier property alternative winning. No requested
// languages — an icon does not depend on the visitor's, so every visitor shares
// one cache entry. Without an "icon" magic property nothing is sent or cached.
func (p *Preprocessor) fetchIconOverrides(endpointURL string, iris []string) map[string]string {
	// Checked here rather than left to fetchBest, which would still cache an
	// empty answer for every IRI of every table.
	if p.config.MagicProperties[IconProperty] == "" {
		return nil
	}
	return p.fetchBest(IconProperty, endpointURL, iris, nil, false)
}

// ResolveIconOverride is fetchIconOverrides for one resource on the
// preprocessor's endpoint, for the page header. "" when it has none.
func (p *Preprocessor) ResolveIconOverride(iri string) string {
	return p.fetchIconOverrides(p.config.EndpointURL, []string{iri})[iri]
}

// enrichWithIcons resolves an icon path for every IRI the result mentions and
// stores them on the result as a side map. overrides (IRI → the resource's own
// icon value) take precedence over the type-based rule; see icon.Resolve.
//
// A side map rather than a field on Binding: Binding is serialized for every
// cell of every row, and a working set runs to 20 000 rows, while the icons are
// one entry per *distinct* IRI. Unresolved IRIs are omitted entirely, so a
// result whose IRIs have no icons carries no map at all.
func enrichWithIcons(result *QueryResult, typeMap map[string][]string, overrides map[string]string) {
	icons := make(map[string]string)
	for _, iri := range extractIRIs(*result) {
		s := icon.Subject{IRI: iri, Types: typeMap[iri], Override: overrides[iri]}
		if path := icon.Resolve(s, icon.Cell); path != "" {
			icons[iri] = path
		}
	}
	if len(icons) > 0 {
		result.Icons = icons
	}
}

// ---- small helpers shared with the label path ----

// chunk splits items into slices of at most size elements.
func chunk(items []string, size int) [][]string {
	var out [][]string
	for i := 0; i < len(items); i += size {
		end := min(i+size, len(items))
		out = append(out, items[i:end])
	}
	return out
}

// runParallel runs fn(0..n-1) concurrently and waits for all of them. Each call
// must write only to its own index, so no locking is needed.
func runParallel(n int, fn func(i int)) {
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			fn(i)
		}(i)
	}
	wg.Wait()
}
