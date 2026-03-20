package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
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

// visotoLink returns the Visoto resource URL for a given IRI.
func (tc *toolContext) visotoLink(iri string) string {
	return fmt.Sprintf("http://localhost:%d/resource/%s", tc.cfg.Application.Port, url.QueryEscape(iri))
}

// --- helper: execute one query and wrap in toolResult ---

func (tc *toolContext) run(query, endpoint string, resolveLabels bool) toolResult {
	result, _ := tc.preprocessor.ExecuteQuery(query, resolveLabels, "", endpoint)

	rows := make([]map[string]any, 0, len(result.Bindings))
	for _, binding := range result.Bindings {
		row := make(map[string]any, len(binding))
		for k, v := range binding {
			row[k] = v.DisplayText
			if v.Type == "uri" {
				row[k+"_visoto_link"] = tc.visotoLink(v.Value)
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
		"endpoint":    result.Endpoint,
		"status":      status,
		"latency_ms":  latency.Milliseconds(),
		"error":       result.Error,
		"hints":       hints,
	}, "", "  ")
	if err != nil {
		return goMcp.NewToolResultErrorf("marshal error: %v", err), nil
	}
	return goMcp.NewToolResultText(string(data)), nil
}

// handleRunSPARQLQuery executes a raw SPARQL SELECT query.
func (tc *toolContext) handleRunSPARQLQuery(_ context.Context, request goMcp.CallToolRequest) (*goMcp.CallToolResult, error) {
	query := getStringParam(request, "query")
	if query == "" {
		return goMcp.NewToolResultError("parameter 'query' is required"), nil
	}
	endpoint := getStringParam(request, "endpoint")
	resolveLabels := getBoolParam(request, "resolve_labels", false)

	r := tc.run(query, endpoint, resolveLabels)
	return toMCPResult(r)
}

// handleDiscoverClasses lists distinct RDF types in the endpoint.
func (tc *toolContext) handleDiscoverClasses(_ context.Context, request goMcp.CallToolRequest) (*goMcp.CallToolResult, error) {
	limit := getIntParam(request, "limit", 100)
	endpoint := getStringParam(request, "endpoint")

	query := fmt.Sprintf(queryDiscoverClasses, limit)
	r := tc.run(query, endpoint, false)
	return toMCPResult(r)
}

// handleDiscoverProperties lists distinct predicates used in the endpoint.
func (tc *toolContext) handleDiscoverProperties(_ context.Context, request goMcp.CallToolRequest) (*goMcp.CallToolResult, error) {
	limit := getIntParam(request, "limit", 100)
	endpoint := getStringParam(request, "endpoint")

	query := fmt.Sprintf(queryDiscoverProperties, limit)
	r := tc.run(query, endpoint, false)
	return toMCPResult(r)
}

// handleGetResource returns all triples for a given IRI.
func (tc *toolContext) handleGetResource(_ context.Context, request goMcp.CallToolRequest) (*goMcp.CallToolResult, error) {
	iri := getStringParam(request, "iri")
	if iri == "" {
		return goMcp.NewToolResultError("parameter 'iri' is required"), nil
	}
	endpoint := getStringParam(request, "endpoint")

	query := fmt.Sprintf(queryGetResource, iri)
	r := tc.run(query, endpoint, true)
	r.VisotoLink = tc.visotoLink(iri)
	return toMCPResult(r)
}

// handleSearchByLabel searches resources by label text.
func (tc *toolContext) handleSearchByLabel(_ context.Context, request goMcp.CallToolRequest) (*goMcp.CallToolResult, error) {
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

	r := tc.run(query, endpoint, false)
	return toMCPResult(r)
}

// handleCountInstances returns instance counts per class or for a specific class.
func (tc *toolContext) handleCountInstances(_ context.Context, request goMcp.CallToolRequest) (*goMcp.CallToolResult, error) {
	classIRI := getStringParam(request, "class_iri")
	endpoint := getStringParam(request, "endpoint")

	var query string
	if classIRI != "" {
		query = fmt.Sprintf(queryCountInstancesForClass, classIRI)
	} else {
		query = queryCountInstances
	}

	r := tc.run(query, endpoint, false)
	return toMCPResult(r)
}
