package search

// GraphDB Lucene connector search.
//
// This provider does not know any index names. It asks the endpoint which
// connectors exist, reads their configuration to learn which RDF properties each
// one indexes, and picks the connector that covers the property being searched.
//
// That indirection is the point. A knowledge-base article documenting LINDAS
// named an index "lindas_name" that no longer exists — querying it returns zero
// rows, silently. Index names rot; asking the endpoint does not.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"hutzli.org/visoto/internal/cache"
	"hutzli.org/visoto/internal/logger"
	"hutzli.org/visoto/internal/sparql"
)

const (
	// luceneConnectorNS is the GraphDB Lucene connector predicate namespace.
	luceneConnectorNS = "http://www.ontotext.com/connectors/lucene#"

	// luceneDiscoveryTimeout bounds the probe so a slow endpoint delays a search
	// briefly rather than hanging it. Measured cost on LINDAS is ~0.1s.
	luceneDiscoveryTimeout = 3 * time.Second

	// A positive result describes endpoint *configuration*, which an admin can
	// change; a negative describes what *kind* of endpoint this is, which
	// effectively never changes — Virtuoso and QLever will not sprout GraphDB
	// connectors. Hence the longer negative TTL. Neither is infinite: a newly
	// added GraphDB endpoint must become discoverable without a restart.
	luceneTTLPositive = 2 * time.Hour
	luceneTTLNegative = 12 * time.Hour
)

// errNoLuceneIndex means this search cannot be served by a Lucene connector —
// none exist, discovery failed, or no index covers the requested property. The
// caller falls back to the CONTAINS scan.
var errNoLuceneIndex = errors.New("no usable lucene connector index")

// ── Discovered model ─────────────────────────────────────────────────────────

// connectorOptions is the JSON GraphDB returns from conn:listOptionValues. Only
// the fields this provider uses are modelled; GraphDB emits several more.
type connectorOptions struct {
	Languages []string         `json:"languages"`
	Types     []string         `json:"types"`
	Fields    []connectorField `json:"fields"`
}

type connectorField struct {
	PropertyChain []string `json:"propertyChain"`
	FieldName     string   `json:"fieldName"`
}

// luceneIndex is one discovered connector, reduced to what query building needs.
type luceneIndex struct {
	IRI    string              // ...lucene/instance#lindas_label
	Name   string              // "lindas_label", for logs
	Groups map[string][]string // group name ("label") -> property IRIs it indexes
}

// luceneDiscovery is the cached per-endpoint result.
type luceneDiscovery struct {
	Indexes []luceneIndex
}

// fieldGroup returns the searchable group a connector field belongs to.
//
// GraphDB names fields "<group>$<property>", e.g. "label$rdfslabel". Only the
// GROUP is addressable in a query: measured against LINDAS, "label:bern" returns
// hits while "label$schemaname:bern" returns zero rows. Do not "improve" this
// into per-property targeting — it silently breaks search.
func fieldGroup(fieldName string) string {
	if group, _, found := strings.Cut(fieldName, "$"); found {
		return group
	}
	return fieldName
}

// parseConnectorOptions turns the options JSON into the group -> properties map.
//
// Every element of a propertyChain is indexed, not just the last. A chain like
// [schema:address, schema:streetAddress] indexes the street address, but a user
// picking a property in the UI would pick the first. Over-matching costs a
// slightly-too-broad index choice; under-matching silently degrades to CONTAINS.
func parseConnectorOptions(raw string) (map[string][]string, error) {
	var opts connectorOptions
	if err := json.Unmarshal([]byte(raw), &opts); err != nil {
		return nil, fmt.Errorf("parsing connector options: %w", err)
	}

	groups := make(map[string][]string)
	for _, f := range opts.Fields {
		group := fieldGroup(f.FieldName)
		if group == "" {
			continue
		}
		for _, prop := range f.PropertyChain {
			if prop != "" {
				groups[group] = append(groups[group], prop)
			}
		}
	}
	return groups, nil
}

// localName returns the fragment after '#', for logging.
func localName(iri string) string {
	if i := strings.LastIndex(iri, "#"); i >= 0 && i+1 < len(iri) {
		return iri[i+1:]
	}
	return iri
}

// ── Selection ────────────────────────────────────────────────────────────────

// selectIndex picks the connector and field group to query for a property.
//
// Selection must be deterministic: SPARQL result order is not guaranteed, so two
// identical requests must not pick different indexes. Callers sort Indexes by
// IRI before this runs, and ties below break on explicit keys.
func selectIndex(indexes []luceneIndex, property string) (luceneIndex, string, bool) {
	type candidate struct {
		index luceneIndex
		group string
		size  int // properties in the group; fewer == more specific
	}
	var candidates []candidate

	for _, idx := range indexes {
		for group, props := range idx.Groups {
			if property == "" {
				// "Any property" wants the broadest reach, so size is negated:
				// the group indexing the most properties sorts first.
				candidates = append(candidates, candidate{idx, group, -len(props)})
				continue
			}
			for _, p := range props {
				if p == property {
					candidates = append(candidates, candidate{idx, group, len(props)})
					break
				}
			}
		}
	}

	if len(candidates) == 0 {
		return luceneIndex{}, "", false
	}

	sort.Slice(candidates, func(i, j int) bool {
		a, b := candidates[i], candidates[j]
		if a.size != b.size {
			return a.size < b.size
		}
		if a.index.IRI != b.index.IRI {
			return a.index.IRI < b.index.IRI
		}
		return a.group < b.group
	})

	best := candidates[0]
	return best.index, best.group, true
}

// ── Discovery ────────────────────────────────────────────────────────────────

// discoverer owns the caches. It is separate from the provider so tests can hold
// an isolated instance, and so the provider itself stays field-free (it is a
// shared singleton in the global registry).
type discoverer struct {
	positive *cache.TTL[luceneDiscovery]
	negative *cache.TTL[struct{}]
}

func newDiscoverer() *discoverer {
	return &discoverer{
		positive: cache.New[luceneDiscovery](luceneTTLPositive),
		negative: cache.New[struct{}](luceneTTLNegative),
	}
}

var defaultDiscoverer = newDiscoverer()

// discover returns the endpoint's Lucene connectors, from cache when possible.
//
// Expired entries are dropped on read by the cache, so re-probing happens lazily
// on the next search after a TTL lapses — no timer, nothing to shut down.
func (d *discoverer) discover(ctx context.Context, endpointURL string, exec ExecuteFunc) (luceneDiscovery, error) {
	log := logger.Get()

	// An empty endpoint URL means the caller could not identify the endpoint.
	// Probe fresh rather than caching under "": a cache entry keyed on "" would
	// serve one endpoint's connectors for a different endpoint, which is a silent
	// wrong-data bug. An uncached probe is merely slow.
	cacheable := endpointURL != ""

	if cacheable {
		if _, found := d.negative.Get(endpointURL); found {
			return luceneDiscovery{}, errNoLuceneIndex
		}
		if hit, found := d.positive.Get(endpointURL); found {
			return hit, nil
		}
	}

	ctx, cancel := context.WithTimeout(ctx, luceneDiscoveryTimeout)
	defer cancel()

	iris, err := d.listConnectors(ctx, exec)
	if err != nil {
		// An endpoint that is down or erroring says nothing about whether it has
		// connectors, so this is not cached — unlike a clean empty answer.
		log.Warn("lucene connector discovery failed",
			slog.String("endpoint", endpointURL),
			slog.String("error", err.Error()))
		return luceneDiscovery{}, errNoLuceneIndex
	}

	if len(iris) == 0 {
		// The steady state for every non-GraphDB endpoint, which answer this
		// query with an empty result rather than an error. Debug, not Warn: it
		// would otherwise be logged on every search against most endpoints.
		log.Debug("endpoint exposes no lucene connectors", slog.String("endpoint", endpointURL))
		if cacheable {
			d.negative.Set(endpointURL, struct{}{})
		}
		return luceneDiscovery{}, errNoLuceneIndex
	}

	indexes := d.fetchOptions(ctx, iris, exec)
	if len(indexes) == 0 {
		if cacheable {
			d.negative.Set(endpointURL, struct{}{})
		}
		return luceneDiscovery{}, errNoLuceneIndex
	}

	// Sort by IRI so selection is deterministic regardless of result-set order.
	sort.Slice(indexes, func(i, j int) bool { return indexes[i].IRI < indexes[j].IRI })

	result := luceneDiscovery{Indexes: indexes}
	if cacheable {
		d.positive.Set(endpointURL, result)
	}

	names := make([]string, len(indexes))
	for i, idx := range indexes {
		names[i] = idx.Name
	}
	log.Debug("discovered lucene connectors",
		slog.String("endpoint", endpointURL),
		slog.String("connectors", strings.Join(names, ", ")))

	return result, nil
}

// listConnectors asks the endpoint which Lucene connector instances exist.
func (d *discoverer) listConnectors(ctx context.Context, exec ExecuteFunc) ([]string, error) {
	query := fmt.Sprintf(`PREFIX conn: <%s>

SELECT ?inst WHERE { ?inst conn:listConnectors ?name }`, luceneConnectorNS)

	res, err := exec(ctx, query)
	if err != nil {
		return nil, err
	}
	if res.Error != "" {
		return nil, errors.New(res.Error)
	}

	var iris []string
	for _, b := range res.Bindings {
		if v, ok := b["inst"]; ok && v.Value != "" {
			iris = append(iris, v.Value)
		}
	}
	return iris, nil
}

// fetchOptions reads each connector's configuration, concurrently — LINDAS has
// two, so this is one round-trip instead of two inside a 3s budget.
func (d *discoverer) fetchOptions(ctx context.Context, iris []string, exec ExecuteFunc) []luceneIndex {
	log := logger.Get()

	var (
		mu      sync.Mutex
		indexes []luceneIndex
		wg      sync.WaitGroup
	)

	for _, iri := range iris {
		// The IRI comes from the endpoint's own response, but it is still spliced
		// into query text, so it is validated like any other untrusted term.
		term, err := sparql.IRITerm(iri)
		if err != nil {
			log.Warn("skipping connector with invalid IRI",
				slog.String("iri", iri), slog.String("error", err.Error()))
			continue
		}

		wg.Add(1)
		go func(iri, term string) {
			defer wg.Done()

			query := fmt.Sprintf(`PREFIX conn: <%s>

SELECT ?options WHERE { %s conn:listOptionValues ?options }`, luceneConnectorNS, term)

			res, err := exec(ctx, query)
			if err != nil || res.Error != "" || len(res.Bindings) == 0 {
				log.Warn("could not read connector options", slog.String("connector", iri))
				return
			}

			raw, ok := res.Bindings[0]["options"]
			if !ok || raw.Value == "" {
				return
			}

			groups, err := parseConnectorOptions(raw.Value)
			if err != nil || len(groups) == 0 {
				log.Warn("could not parse connector options",
					slog.String("connector", iri))
				return
			}

			mu.Lock()
			indexes = append(indexes, luceneIndex{IRI: iri, Name: localName(iri), Groups: groups})
			mu.Unlock()
		}(iri, term)
	}

	wg.Wait()
	return indexes
}

// ── Query building ───────────────────────────────────────────────────────────

// luceneQueryText prepares the string handed to the connector.
//
// Lucene syntax is deliberately passed through untouched, so wildcards, boolean
// operators and explicit field selectors all reach the connector. Only the SPARQL
// string literal is escaped, which is a separate concern: `label:"foo bar"`
// becomes `label:\"foo bar\"` in the query text and arrives intact.
//
// The group prefix is added only when the user wrote no field selector of their
// own. Testing for ':' is imprecise — searching for a URL contains one and skips
// the prefix — but that case is arguably raw-syntax intent anyway, and there is
// no cheap way to tell a field selector from a literal colon without parsing
// Lucene. A bare term works regardless (verified: "bern" and "label:bern" return
// the same rows), so the prefix is scoping, not a requirement.
func luceneQueryText(userQuery, group string) string {
	if strings.Contains(userQuery, ":") {
		return userQuery
	}
	return group + ":" + userQuery
}

// buildLuceneQuery renders the SPARQL for a discovered index.
func buildLuceneQuery(index luceneIndex, group string, params SearchParams) (string, error) {
	limit := params.Limit
	if limit <= 0 {
		limit = DefaultLimit
	}

	indexTerm, err := sparql.IRITerm(index.IRI)
	if err != nil {
		return "", err
	}

	text := escapeString(luceneQueryText(params.Query, group))

	var b strings.Builder
	fmt.Fprintf(&b, "PREFIX luc: <%s>\n", luceneConnectorNS)
	b.WriteString("PREFIX rdf: <http://www.w3.org/1999/02/22-rdf-syntax-ns#>\n\n")

	if params.Class == "" {
		// ?type is OPTIONAL because the connector indexes untyped resources too
		// ($untyped), and a required join would drop them entirely. It must also
		// keep being selected: the search page declares ?type as its row-icon
		// column, so dropping it silently removes the icons.
		b.WriteString("SELECT ?type ?subject ?score WHERE {\n")
		fmt.Fprintf(&b, "  ?search a %s ;\n", indexTerm)
		fmt.Fprintf(&b, "          luc:query \"%s\" ;\n", text)
		b.WriteString("          luc:entities ?subject .\n")
		b.WriteString("  ?subject luc:score ?score .\n")
		b.WriteString("  OPTIONAL { ?subject rdf:type ?type . }\n")
		b.WriteString("}\n")
		fmt.Fprintf(&b, "ORDER BY DESC(?score)\nLIMIT %d", limit)
		return b.String(), nil
	}

	classTerm, err := sparql.IRITerm(params.Class)
	if err != nil {
		return "", err
	}

	// Cap the Lucene hit set in a subquery, then join the type. Filtering by type
	// inside the connector pattern measured 4.1s against 0.7s for this shape.
	//
	// The trade-off is that the class filter applies only within the top innerCap
	// hits, so a class-filtered search returns a best-effort subset rather than an
	// exhaustive one. The alternative is the 4.1s query.
	innerCap := limit * 10
	if innerCap < 500 {
		innerCap = 500
	}
	if innerCap > 2000 {
		innerCap = 2000
	}

	b.WriteString("SELECT ?type ?subject ?score WHERE {\n")
	b.WriteString("  {\n    SELECT ?subject ?score WHERE {\n")
	fmt.Fprintf(&b, "      ?search a %s ;\n", indexTerm)
	fmt.Fprintf(&b, "              luc:query \"%s\" ;\n", text)
	b.WriteString("              luc:entities ?subject .\n")
	b.WriteString("      ?subject luc:score ?score .\n")
	b.WriteString("    }\n")
	fmt.Fprintf(&b, "    ORDER BY DESC(?score)\n    LIMIT %d\n  }\n", innerCap)
	fmt.Fprintf(&b, "  ?subject rdf:type %s .\n", classTerm)
	fmt.Fprintf(&b, "  BIND(%s AS ?type)\n", classTerm)
	b.WriteString("}\n")
	fmt.Fprintf(&b, "ORDER BY DESC(?score)\nLIMIT %d", limit)

	return b.String(), nil
}

// ── Provider ─────────────────────────────────────────────────────────────────

// GraphDBLuceneProvider searches via GraphDB Lucene connectors, discovering the
// available indexes at runtime.
//
// The struct is deliberately field-free: RegisterProvider stores one shared
// instance that every concurrent request uses, so per-request state would race.
// Everything it needs arrives in SearchContext; the caches live on discoverer.
type GraphDBLuceneProvider struct{}

func (g *GraphDBLuceneProvider) Name() string { return "graphdb-lucene" }

// BuildQuery satisfies Provider. This provider cannot build a query without
// first asking the endpoint what it supports, so the context-free path errors —
// which routes the search to the CONTAINS fallback rather than failing the page.
func (g *GraphDBLuceneProvider) BuildQuery(params SearchParams) (string, error) {
	return "", errNoLuceneIndex
}

// BuildQueryWithContext discovers the endpoint's connectors, picks the index
// covering the requested property, and renders the query.
func (g *GraphDBLuceneProvider) BuildQueryWithContext(sc SearchContext) (string, error) {
	return defaultDiscoverer.buildQuery(sc)
}

func (d *discoverer) buildQuery(sc SearchContext) (string, error) {
	if sc.Params.Query == "" {
		return "", fmt.Errorf("search query cannot be empty")
	}
	if sc.Execute == nil {
		return "", errNoLuceneIndex
	}

	ctx := sc.Ctx
	if ctx == nil {
		ctx = context.Background()
	}

	discovered, err := d.discover(ctx, sc.EndpointURL, sc.Execute)
	if err != nil {
		return "", err
	}

	index, group, ok := selectIndex(discovered.Indexes, sc.Params.Property)
	if !ok {
		// Discovery succeeded; only the selection failed. Not cached negatively —
		// a different property may well be covered. This log line is the primary
		// diagnostic when a search unexpectedly falls back.
		logger.Get().Debug("no lucene index covers the searched property",
			slog.String("property", sc.Params.Property),
			slog.String("endpoint", sc.EndpointURL))
		return "", errNoLuceneIndex
	}

	logger.Get().Debug("using lucene connector",
		slog.String("connector", index.Name),
		slog.String("field_group", group),
		slog.String("property", sc.Params.Property))

	return buildLuceneQuery(index, group, sc.Params)
}

func init() {
	RegisterProvider(&GraphDBLuceneProvider{})
}
