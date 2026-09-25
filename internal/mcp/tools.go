package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	goMcp "github.com/mark3labs/mcp-go/mcp"
	"hutzli.org/visoto/internal/config"
	"hutzli.org/visoto/internal/sparql"
)

// toolResult is the standard JSON response shape returned by all MCP tools.
type toolResult struct {
	EndpointUsed  string           `json:"endpoint_used,omitempty"`
	QueryExecuted string           `json:"query_executed,omitempty"`
	VisotoLink    string           `json:"visoto_link,omitempty"`
	RowCount      int              `json:"row_count,omitempty"`
	Results       []map[string]any `json:"results,omitempty"`
	Hints         []string         `json:"hints,omitempty"`
	Error         string           `json:"error,omitempty"`
}

// toolContext bundles dependencies needed by all tool handlers.
type toolContext struct {
	preprocessor *sparql.Preprocessor
	cfg          *config.Config
}

// visotoLink returns the Visoto resource-page URL for a given IRI. The base URL
// is derived from the incoming request (stored in ctx by the HTTP transport);
// endpointURL, when it maps to a configured endpoint with a slug, is preserved
// as the canonical &endpoint=<slug> query param so the link opens against the
// same endpoint the query ran on.
func (tc *toolContext) visotoLink(ctx context.Context, iri, endpointURL string) string {
	link := baseURLFromContext(ctx, tc.cfg.Application.Port) + sparql.ResourceHref(iri)
	if ep := tc.cfg.Application.GetEndpointByURL(endpointURL); ep != nil && ep.Slug != "" {
		link += "&endpoint=" + url.QueryEscape(ep.Slug)
	}
	return link
}

// --- helper: execute one query and wrap in toolResult ---

func (tc *toolContext) run(ctx context.Context, query, endpoint string, resolveLabels bool) toolResult {
	result, _ := tc.preprocessor.ExecuteQuery(query, resolveLabels, "", endpoint)

	rows := make([]map[string]any, 0, len(result.Bindings))
	for _, binding := range result.Bindings {
		row := make(map[string]any, len(binding))
		for k, v := range binding {
			row[k] = v.DisplayText
			if v.Type == "uri" {
				row[k+"_visoto_link"] = tc.visotoLink(ctx, v.Value, result.Endpoint)
			}
		}
		rows = append(rows, row)
	}

	hints := buildHints(result, tc.cfg.Application.SparqlEndpoints, tc.cfg.RDF.ParsedPrefixes)

	return toolResult{
		EndpointUsed:  result.Endpoint,
		QueryExecuted: result.Query,
		RowCount:      len(result.Bindings),
		Results:       rows,
		Hints:         hints,
		Error:         result.Error,
	}
}

// --- helper: marshal toolResult to mcp text result ---

func toMCPResult(r toolResult) (*goMcp.CallToolResult, error) {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return goMcp.NewToolResultErrorf("failed to marshal result: %v", err), nil
	}
	if r.Error != "" {
		return &goMcp.CallToolResult{
			Content: []goMcp.Content{goMcp.NewTextContent(string(data))},
			IsError: true,
		}, nil
	}
	return goMcp.NewToolResultText(string(data)), nil
}

// --- helper: get optional string param ---

func getStringParam(request goMcp.CallToolRequest, name string) string {
	return strings.TrimSpace(request.GetString(name, ""))
}

func getIntParam(request goMcp.CallToolRequest, name string, defaultVal int) int {
	return request.GetInt(name, defaultVal)
}

func getBoolParam(request goMcp.CallToolRequest, name string, defaultVal bool) bool {
	return request.GetBool(name, defaultVal)
}

// --- Tool handlers ---

// handleListEndpoints returns all configured SPARQL endpoints.
func (tc *toolContext) handleListEndpoints(_ context.Context, _ goMcp.CallToolRequest) (*goMcp.CallToolResult, error) {
	type endpointInfo struct {
		Name    string `json:"name"`
		URL     string `json:"url"`
		Default bool   `json:"default"`
	}

	endpoints := make([]endpointInfo, 0, len(tc.cfg.Application.SparqlEndpoints))
	for _, ep := range tc.cfg.Application.SparqlEndpoints {
		endpoints = append(endpoints, endpointInfo{
			Name:    ep.Name,
			URL:     ep.URL,
			Default: ep.Default,
		})
	}

	data, err := json.MarshalIndent(map[string]any{
		"endpoints": endpoints,
		"default":   tc.cfg.Application.SparqlEndpoint,
	}, "", "  ")
	if err != nil {
		return goMcp.NewToolResultErrorf("marshal error: %v", err), nil
	}
	return goMcp.NewToolResultText(string(data)), nil
}

// handleListPrefixes returns all configured RDF prefix declarations.
func (tc *toolContext) handleListPrefixes(_ context.Context, _ goMcp.CallToolRequest) (*goMcp.CallToolResult, error) {
	type prefixInfo struct {
		Name string `json:"name"`
		URI  string `json:"uri"`
	}

	prefixes := make([]prefixInfo, 0, len(tc.cfg.RDF.ParsedPrefixes))
	for _, p := range tc.cfg.RDF.ParsedPrefixes {
		prefixes = append(prefixes, prefixInfo{
			Name: p.Name + ":",
			URI:  strings.Trim(p.URI, "<>"),
		})
	}

	data, err := json.MarshalIndent(map[string]any{"prefixes": prefixes}, "", "  ")
	if err != nil {
		return goMcp.NewToolResultErrorf("marshal error: %v", err), nil
	}
	return goMcp.NewToolResultText(string(data)), nil
}

// handleCheckEndpoint runs ASK {} to test endpoint reachability.
func (tc *toolContext) handleCheckEndpoint(_ context.Context, request goMcp.CallToolRequest) (*goMcp.CallToolResult, error) {
	endpoint := getStringParam(request, "endpoint")

	start := time.Now()
	result, _ := tc.preprocessor.ExecuteQuery(queryCheckEndpoint, false, "", endpoint)
	latency := time.Since(start)

	var status string
	if result.Error != "" {
		status = "unreachable"
	} else {
		status = "reachable"
	}

	hints := buildHints(result, tc.cfg.Application.SparqlEndpoints, tc.cfg.RDF.ParsedPrefixes)

	data, err := json.MarshalIndent(map[string]any{
		"endpoint":   result.Endpoint,
		"status":     status,
		"latency_ms": latency.Milliseconds(),
		"error":      result.Error,
		"hints":      hints,
	}, "", "  ")
	if err != nil {
		return goMcp.NewToolResultErrorf("marshal error: %v", err), nil
	}
	return goMcp.NewToolResultText(string(data)), nil
}

// handleRunSPARQLQuery executes a raw SPARQL SELECT query.
func (tc *toolContext) handleRunSPARQLQuery(ctx context.Context, request goMcp.CallToolRequest) (*goMcp.CallToolResult, error) {
	query := getStringParam(request, "query")
	if query == "" {
		return goMcp.NewToolResultError("parameter 'query' is required"), nil
	}
	endpoint := getStringParam(request, "endpoint")
	resolveLabels := getBoolParam(request, "resolve_labels", false)

	r := tc.run(ctx, query, endpoint, resolveLabels)
	return toMCPResult(r)
}

// handleDiscoverClasses lists distinct RDF types in the endpoint, optionally
// scoped to a single named graph.
func (tc *toolContext) handleDiscoverClasses(ctx context.Context, request goMcp.CallToolRequest) (*goMcp.CallToolResult, error) {
	limit := getIntParam(request, "limit", 100)
	endpoint := getStringParam(request, "endpoint")
	graph := getStringParam(request, "graph")

	if graph != "" {
		r := tc.run(ctx, fmt.Sprintf(queryDiscoverClassesInGraph, graph, limit), endpoint, false)
		return toMCPResult(r)
	}

	// A store-wide GROUP BY over every rdf:type triple times out on large
	// GraphDB stores (LINDAS: ~468M triples), while the same query scoped to one
	// graph is fast. On GraphDB, fan out per graph and add the counts up.
	graphs := tc.run(ctx, fmt.Sprintf(queryListNamedGraphsGraphDB, 1000), endpoint, false)
	if graphs.Error == "" && graphs.RowCount > 0 {
		return toMCPResult(tc.discoverClassesPerGraph(ctx, graphs, endpoint, limit))
	}

	r := tc.run(ctx, fmt.Sprintf(queryDiscoverClasses, limit), endpoint, false)
	return toMCPResult(r)
}

// discoverClassesPerGraphConcurrency caps the per-graph fan-out: each query
// scans one graph's rdf:type triples, and firing all ~60 of LINDAS's at once
// loads the store for no latency gain.
const discoverClassesPerGraphConcurrency = 10

// discoverClassesPerGraph runs queryDiscoverClassesInGraph once per named graph
// in parallel and merges the rows: one row per class, counts added up across
// graphs.
func (tc *toolContext) discoverClassesPerGraph(ctx context.Context, graphs toolResult, endpoint string, limit int) toolResult {
	queries := make([]sparql.ExtractedQuery, 0, len(graphs.Results))
	for _, row := range graphs.Results {
		if iri, ok := row["graph"].(string); ok && iri != "" {
			queries = append(queries, sparql.ExtractedQuery{
				ID:       iri,
				Query:    fmt.Sprintf(queryDiscoverClassesInGraph, iri, 100000),
				Endpoint: endpoint,
			})
		}
	}
	results := tc.preprocessor.ExecuteQueriesParallelN(queries, discoverClassesPerGraphConcurrency, tc.cfg.GetTimeout(), "")

	classes, failed := mergeClassCounts(results)
	if len(classes) > limit {
		classes = classes[:limit]
	}

	r := toolResult{
		EndpointUsed:  graphs.EndpointUsed,
		QueryExecuted: fmt.Sprintf("-- run once per named graph (%d graphs), counts added up:%s", len(queries), fmt.Sprintf(queryDiscoverClassesInGraph, "GRAPH_IRI", 100000)),
		RowCount:      len(classes),
		Results:       make([]map[string]any, 0, len(classes)),
	}
	for _, c := range classes {
		r.Results = append(r.Results, map[string]any{
			"type":             c.display,
			"type_visoto_link": tc.visotoLink(ctx, c.iri, graphs.EndpointUsed),
			"count":            strconv.Itoa(c.count),
		})
	}
	r.Hints = append(r.Hints, "Counts are added up across named graphs; an instance typed in several graphs is counted once per graph.")
	if len(failed) > 0 {
		sort.Strings(failed)
		r.Hints = append(r.Hints, fmt.Sprintf("Counts are partial: %d graph(s) failed or timed out: %s", len(failed), strings.Join(failed, ", ")))
	}
	if len(classes) == limit {
		r.Hints = append(r.Hints, fmt.Sprintf("Result may be truncated at limit=%d — raise the limit parameter to see more classes.", limit))
	}
	return r
}

// classCount is one merged row of the per-graph class discovery.
type classCount struct {
	iri     string
	display string
	count   int
}

// mergeClassCounts merges per-graph {type, count} results (keyed by graph IRI)
// into one row per class, sorted by count descending, then by IRI. Graphs whose
// query failed are returned in failed and contribute nothing; rows with a
// non-numeric count are skipped.
func mergeClassCounts(results map[string]sparql.QueryResult) (classes []classCount, failed []string) {
	byIRI := map[string]*classCount{}
	for graph, res := range results {
		if res.Error != "" {
			failed = append(failed, graph)
			continue
		}
		for _, b := range res.Bindings {
			t, ok := b["type"]
			if !ok || t.Value == "" {
				continue
			}
			n, err := strconv.Atoi(b["count"].Value)
			if err != nil {
				continue
			}
			c := byIRI[t.Value]
			if c == nil {
				display := t.DisplayText
				if display == "" {
					display = t.Value
				}
				c = &classCount{iri: t.Value, display: display}
				byIRI[t.Value] = c
			}
			c.count += n
		}
	}

	classes = make([]classCount, 0, len(byIRI))
	for _, c := range byIRI {
		classes = append(classes, *c)
	}
	sort.Slice(classes, func(i, j int) bool {
		if classes[i].count != classes[j].count {
			return classes[i].count > classes[j].count
		}
		return classes[i].iri < classes[j].iri
	})
	return classes, failed
}

// handleDiscoverProperties lists distinct predicates used in the endpoint.
func (tc *toolContext) handleDiscoverProperties(ctx context.Context, request goMcp.CallToolRequest) (*goMcp.CallToolResult, error) {
	limit := getIntParam(request, "limit", 100)
	endpoint := getStringParam(request, "endpoint")

	query := fmt.Sprintf(queryDiscoverProperties, limit)
	r := tc.run(ctx, query, endpoint, false)
	return toMCPResult(r)
}

// handleGetResource returns all triples for a given IRI.
func (tc *toolContext) handleGetResource(ctx context.Context, request goMcp.CallToolRequest) (*goMcp.CallToolResult, error) {
	iri := getStringParam(request, "iri")
	if iri == "" {
		return goMcp.NewToolResultError("parameter 'iri' is required"), nil
	}
	endpoint := getStringParam(request, "endpoint")

	query := fmt.Sprintf(queryGetResource, iri)
	r := tc.run(ctx, query, endpoint, true)
	r.VisotoLink = tc.visotoLink(ctx, iri, r.EndpointUsed)
	return toMCPResult(r)
}

// handleSearchByLabel searches resources by label text.
func (tc *toolContext) handleSearchByLabel(ctx context.Context, request goMcp.CallToolRequest) (*goMcp.CallToolResult, error) {
	text := getStringParam(request, "text")
	if text == "" {
		return goMcp.NewToolResultError("parameter 'text' is required"), nil
	}
	typeFilter := getStringParam(request, "type_filter")
	endpoint := getStringParam(request, "endpoint")
	limit := getIntParam(request, "limit", 50)

	var query string
	if typeFilter != "" {
		query = fmt.Sprintf(querySearchByLabelWithType, text, typeFilter, limit)
	} else {
		query = fmt.Sprintf(querySearchByLabel, text, limit)
	}

	r := tc.run(ctx, query, endpoint, false)
	return toMCPResult(r)
}

// handleListNamedGraphs lists the named graphs in the endpoint. It tries
// GraphDB's onto:graphs system predicate first (fast, with per-graph triple
// estimates); stores that don't support it match 0 rows, triggering a
// portable rdf:type scan without counts.
func (tc *toolContext) handleListNamedGraphs(ctx context.Context, request goMcp.CallToolRequest) (*goMcp.CallToolResult, error) {
	limit := getIntParam(request, "limit", 500)
	endpoint := getStringParam(request, "endpoint")

	r := tc.run(ctx, fmt.Sprintf(queryListNamedGraphsGraphDB, limit), endpoint, false)
	if r.Error == "" && r.RowCount == 0 {
		r = tc.run(ctx, fmt.Sprintf(queryListNamedGraphsPortable, limit), endpoint, false)
		if r.Error == "" {
			r.Hints = append(r.Hints, "This endpoint does not support GraphDB's onto:graphs system predicate; used a portable rdf:type scan instead (slower on large stores; triple counts unavailable).")
		}
	} else if r.Error == "" {
		tc.addTripleEstimates(&r, endpoint)
	}
	if r.Error == "" && r.RowCount == limit {
		r.Hints = append(r.Hints, fmt.Sprintf("Result may be truncated at limit=%d — raise the limit parameter to see more graphs.", limit))
	}
	return toMCPResult(r)
}

// addTripleEstimates enriches each graph row with an approximate triple count
// from GraphDB's sys:statistics pseudo-graph, then sorts rows by size
// descending. Fast path only — other stores return wrong results or errors
// for the statistics pseudo-graph.
func (tc *toolContext) addTripleEstimates(r *toolResult, endpoint string) {
	queries := make([]sparql.ExtractedQuery, 0, len(r.Results))
	for _, row := range r.Results {
		if iri, ok := row["graph"].(string); ok && iri != "" {
			queries = append(queries, sparql.ExtractedQuery{
				ID:       iri,
				Query:    fmt.Sprintf(queryGraphTripleCountGraphDB, iri),
				Endpoint: endpoint,
			})
		}
	}
	counts := tc.preprocessor.ExecuteQueriesParallel(queries, tc.cfg.GetTimeout(), "")

	enriched := false
	for _, row := range r.Results {
		iri, _ := row["graph"].(string)
		result, ok := counts[iri]
		if !ok || result.Error != "" || len(result.Bindings) == 0 {
			continue
		}
		if v, found := result.Bindings[0]["triples"]; found {
			if n, err := strconv.Atoi(v.Value); err == nil {
				row["triples_estimate"] = n
				enriched = true
			}
		}
	}
	if !enriched {
		return
	}

	sort.SliceStable(r.Results, func(i, j int) bool {
		ni, iOk := r.Results[i]["triples_estimate"].(int)
		nj, jOk := r.Results[j]["triples_estimate"].(int)
		if iOk != jOk {
			return iOk // rows with an estimate sort before rows without
		}
		return ni > nj
	})
	r.Hints = append(r.Hints, "triples_estimate comes from GraphDB index statistics and is approximate.")
}

// handleCountInstances returns instance counts per class or for a specific class.
func (tc *toolContext) handleCountInstances(ctx context.Context, request goMcp.CallToolRequest) (*goMcp.CallToolResult, error) {
	classIRI := getStringParam(request, "class_iri")
	endpoint := getStringParam(request, "endpoint")

	var query string
	if classIRI != "" {
		query = fmt.Sprintf(queryCountInstancesForClass, classIRI)
	} else {
		query = queryCountInstances
	}

	r := tc.run(ctx, query, endpoint, false)
	return toMCPResult(r)
}
