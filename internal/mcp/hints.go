package mcp

import (
	"fmt"
	"strings"

	"hutzli.org/visoto/internal/config"
	"hutzli.org/visoto/internal/sparql"
)

// buildHints inspects a QueryResult and returns actionable hint strings for the MCP client.
// alternativeEndpoints is the list of all configured endpoints (used to suggest alternatives).
func buildHints(result sparql.QueryResult, allEndpoints []config.SparqlEndpoint, prefixes []config.Prefix) []string {
	var hints []string

	if result.Error == "" && len(result.Bindings) == 0 {
		hints = append(hints,
			"Query succeeded but returned no results. Consider: "+
				"(1) Check for IRI typos or encoding issues. "+
				"(2) Try a different endpoint — data may only exist on prod vs. int. "+
				"(3) Use discover_classes or discover_properties to explore what is in this endpoint. "+
				"(4) Broaden your query by removing FILTER conditions.")
		return hints
	}

	if result.Error == "" {
		return hints
	}

	err := result.Error

	// Timeout
	if strings.Contains(err, "timeout") || strings.Contains(err, "deadline") || strings.Contains(err, "context canceled") {
		hints = append(hints,
			"Query timed out. Try: "+
				"(1) Add LIMIT (e.g. LIMIT 100) to restrict result size. "+
				"(2) Simplify the WHERE clause — avoid patterns like ?s ?p ?o without filters. "+
				"(3) Use a more specific starting pattern.")
		return hints
	}

	// HTTP 400 — syntax error
	if strings.Contains(err, "400") {
		hints = append(hints,
			"SPARQL syntax error (HTTP 400). Check: "+
				"(1) Unclosed curly braces { }. "+
				"(2) Missing dot . between triple patterns. "+
				"(3) Invalid prefix — known prefixes: "+joinPrefixNames(prefixes)+". "+
				"(4) Malformed IRI (missing < > around URIs). "+
				"(5) Incorrect keyword casing or unsupported SPARQL feature.")
		return hints
	}

	// HTTP 401 / 403 — auth
	if strings.Contains(err, "401") || strings.Contains(err, "403") {
		hints = append(hints,
			"Endpoint rejected the request with an authentication error. "+
				"This endpoint may require credentials not configured in Visoto.")
		return hints
	}

	// HTTP 404 — wrong URL
	if strings.Contains(err, "404") {
		hints = append(hints,
			fmt.Sprintf("Endpoint URL not found (HTTP 404). Check that the endpoint URL %q is correct. "+
				"Known endpoints: %s.", result.Endpoint, joinEndpointNames(allEndpoints)))
		return hints
	}

	// HTTP 502 / 503 — unavailable
	if strings.Contains(err, "502") || strings.Contains(err, "503") {
		others := endpointNamesExcept(allEndpoints, result.Endpoint)
		hints = append(hints,
			"Endpoint is temporarily unavailable (HTTP 502/503). "+
				"Try an alternative endpoint: "+strings.Join(others, ", ")+".")
		return hints
	}

	// Unknown prefix in error body
	if strings.Contains(strings.ToLower(err), "unknown prefix") || strings.Contains(strings.ToLower(err), "unresolved prefix") {
		hints = append(hints,
			"A prefix was used that is not declared. "+
				"Known prefixes configured in Visoto: "+joinPrefixNames(prefixes)+". "+
				"Either use one of these, add a PREFIX declaration at the top of your query, or use the full IRI.")
		return hints
	}

	// Connection refused / DNS failure
	if strings.Contains(err, "connection refused") || strings.Contains(err, "no such host") || strings.Contains(err, "dial") {
		others := endpointNamesExcept(allEndpoints, result.Endpoint)
		msg := "Cannot reach the SPARQL endpoint. Check network connectivity."
		if len(others) > 0 {
			msg += " Try an alternative endpoint: " + strings.Join(others, ", ") + "."
		}
		hints = append(hints, msg)
		return hints
	}

	// Generic fallback
	hints = append(hints,
		"Query failed with error: "+err+". "+
			"Try: (1) check_endpoint to verify the endpoint is reachable. "+
			"(2) Simplify the query. "+
			"(3) Try a different endpoint.")

	return hints
}

func joinPrefixNames(prefixes []config.Prefix) string {
	names := make([]string, 0, len(prefixes))
	for _, p := range prefixes {
		names = append(names, p.Name+":")
	}
	return strings.Join(names, " ")
}

func joinEndpointNames(endpoints []config.SparqlEndpoint) string {
	names := make([]string, 0, len(endpoints))
	for _, ep := range endpoints {
		names = append(names, ep.Name)
	}
	return strings.Join(names, ", ")
}

func endpointNamesExcept(endpoints []config.SparqlEndpoint, excludeURL string) []string {
	var names []string
	for _, ep := range endpoints {
		if ep.URL != excludeURL {
			names = append(names, ep.Name)
		}
	}
	return names
}
