package sparql

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"hutzli.org/visoto/internal/config"
	"hutzli.org/visoto/internal/logger"
)

// extractDeclaredPrefixes parses query and returns a set of prefix names already declared
// Returns map[string]bool where keys are prefix names (e.g., "rdf", "schema")
func extractDeclaredPrefixes(query string) map[string]bool {
	declared := make(map[string]bool)

	// Regex to match PREFIX declarations in SPARQL format
	// Matches: PREFIX name: <uri> or PREFIX name: uri
	// Case-insensitive for PREFIX keyword
	re := regexp.MustCompile(`(?i)PREFIX\s+(\w+):\s*<[^>]+>`)

	matches := re.FindAllStringSubmatch(query, -1)
	for _, match := range matches {
		if len(match) >= 2 {
			prefixName := strings.TrimSpace(match[1])
			declared[prefixName] = true
		}
	}

	return declared
}

// extractUsedPrefixes scans query and returns a set of prefix names that are used
// (e.g., finds "rdf:type", "schema:name" and extracts "rdf", "schema")
// Excludes http:, https:, file: which are not prefixes
func extractUsedPrefixes(query string) map[string]bool {
	used := make(map[string]bool)

	// Regex to match prefixed names: word followed by colon and word/path
	// Matches: rdf:type, schema:name, skos:broader, etc.
	// But NOT http:, https:, file: (common URL schemes)
	re := regexp.MustCompile(`\b(\w+):(?:[a-zA-Z_]|\/)`)

	matches := re.FindAllStringSubmatch(query, -1)
	for _, match := range matches {
		if len(match) >= 2 {
			prefixName := match[1]

			// Exclude URL schemes
			lowerPrefix := strings.ToLower(prefixName)
			if lowerPrefix == "http" || lowerPrefix == "https" || lowerPrefix == "file" {
				continue
			}

			used[prefixName] = true
		}
	}

	return used
}

// buildNeededPrefixBlock generates PREFIX declarations for prefixes that are:
// 1. Actually used in the query (in usedSet)
// 2. Not already declared in the query (not in declaredSet)
// 3. Available in the configuration
func buildNeededPrefixBlock(prefixes []config.Prefix, usedSet map[string]bool, declaredSet map[string]bool) string {
	if len(prefixes) == 0 {
		return ""
	}

	var sb strings.Builder
	addedAny := false

	for _, prefix := range prefixes {
		// Skip if this prefix is not used in the query
		if !usedSet[prefix.Name] {
			continue
		}

		// Skip if this prefix is already declared in the query
		if declaredSet[prefix.Name] {
			continue
		}

		sb.WriteString("PREFIX ")
		sb.WriteString(prefix.Name)
		sb.WriteString(": <")
		sb.WriteString(strings.Trim(prefix.URI, "<>"))
		sb.WriteString(">\n")
		addedAny = true
	}

	// Only add blank line if we actually added prefixes
	if addedAny {
		sb.WriteString("\n")
	}

	return sb.String()
}

// finalizeQuery adds PREFIX declarations to query if needed
func (p *Preprocessor) finalizeQuery(query string) string {
	// Extract which prefixes are already declared in the query
	declaredPrefixes := extractDeclaredPrefixes(query)

	// Extract which prefixes are actually used in the query
	usedPrefixes := extractUsedPrefixes(query)

	// Build prefix block with only needed (used but not declared) prefixes
	prefixBlock := buildNeededPrefixBlock(p.config.Prefixes, usedPrefixes, declaredPrefixes)

	return prefixBlock + query
}

// resolveEndpoint determines which endpoint URL to use
// Priority: 1) Direct URL, 2) Named endpoint lookup, 3) Default
func (p *Preprocessor) resolveEndpoint(endpointAttr string) string {
	if endpointAttr == "" {
		return p.config.EndpointURL // Use default
	}

	// Check if it's a named endpoint
	if url, exists := p.config.NamedEndpoints[endpointAttr]; exists {
		return url
	}

	// Treat as direct URL (allows full URLs in templates)
	return endpointAttr
}

// querySparqlEndpoint sends a SPARQL query to the specified endpoint and returns the response
func (p *Preprocessor) querySparqlEndpoint(endpointURL, query string) ([]byte, string, error) {

	// Finalize query with PREFIX declarations if needed
	finalizedQuery := p.finalizeQuery(query)

	// Prepare the request parameters
	params := url.Values{}
	params.Set("query", finalizedQuery)
	params.Set("format", "application/sparql-results+json")
	encodedParams := params.Encode()

	// Create the HTTP request
	req, err := http.NewRequest("POST", endpointURL, strings.NewReader(encodedParams))
	if err != nil {
		log := logger.Get()
		log.Error("failed to create HTTP request",
			slog.String("error", err.Error()),
			slog.String("endpoint", endpointURL))
		return nil, "", fmt.Errorf("failed to create request: %w", err)
	}

	// Set appropriate headers
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/sparql-results+json")

	// Log the full request details at debug level
	log := logger.Get()
	log.Debug("HTTP request details",
		slog.String("method", req.Method),
		slog.String("url", req.URL.String()),
		slog.String("content-type", req.Header.Get("Content-Type")),
		slog.String("accept", req.Header.Get("Accept")),
		slog.String("body", encodedParams),
		slog.Int("body_length", len(encodedParams)))

	// Execute the request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log := logger.Get()
		log.Error("failed to execute HTTP request",
			slog.String("error", err.Error()),
			slog.String("endpoint", endpointURL))
		return nil, "", fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	// Check for HTTP errors
	if resp.StatusCode != http.StatusOK {
		io.Copy(io.Discard, resp.Body) // Drain body to allow connection reuse
		log := logger.Get()
		log.Error("SPARQL endpoint returned HTTP error",
			slog.Int("status_code", resp.StatusCode),
			slog.String("status", resp.Status),
			slog.String("endpoint", endpointURL))

		// Log the encoded parameters for debugging
		log.Debug("encoded query parameters",
			slog.String("encoded_length", fmt.Sprintf("%d bytes", len(encodedParams))))

		// Log the query at debug level
		log.Debug("executing SPARQL query",
			slog.String("endpoint", endpointURL),
			slog.String("query", finalizedQuery))

		return nil, "", fmt.Errorf("HTTP error: %d %s", resp.StatusCode, resp.Status)
	}

	// Read the response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log := logger.Get()
		log.Error("failed to read response body",
			slog.String("error", err.Error()),
			slog.String("endpoint", endpointURL))
		return nil, "", fmt.Errorf("failed to read response body: %w", err)
	}

	// fmt.Println("=== SPARQL Response Body ===")
	// fmt.Println(string(body))
	// fmt.Println("=== End SPARQL Response Body ===")

	return body, finalizedQuery, nil
}

// simplifyBindings converts sparqlResponse to simplified QueryResult format
// we basically take out the Head and Results and keep the Vars and Bindings only
func simplifyBindings(resp sparqlResponse) QueryResult {
	result := QueryResult{
		Vars:     resp.Head.Vars,
		Bindings: make([]map[string]Binding, 0, len(resp.Results.Bindings)),
	}

	// apparently we have to copy each binding entry individually,
	// even though the structures of QueryResult.Bindings and sparqlResponse.Results.Bindings are identical
	// but go treats them as different types because of the tags
	for _, binding := range resp.Results.Bindings {
		simplified := make(map[string]Binding)
		for varName, varData := range binding {
			simplified[varName] = Binding{
				Type:  varData.Type,
				Value: varData.Value,
				Lol:   varData.Value,
			}
		}
		result.Bindings = append(result.Bindings, simplified)
	}

	return result
}


// QueryTypes queries the SPARQL endpoint for the rdf:type of a given IRI
// Returns a slice of type URIs
func (p *Preprocessor) QueryTypes(iri string) ([]string, error) {
	// Construct SPARQL query to get all types
	query := fmt.Sprintf("SELECT ?type WHERE { <%s> (a|owl:type) ?type }", iri)

	// Execute query
	response, _, err := p.querySparqlEndpoint(p.config.EndpointURL, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query types for IRI %s: %w", iri, err)
	}

	// Parse JSON response
	var sparqlResp sparqlResponse
	if err := json.Unmarshal(response, &sparqlResp); err != nil {
		return nil, fmt.Errorf("failed to parse SPARQL response: %w", err)
	}

	// Extract type URIs from bindings
	types := make([]string, 0, len(sparqlResp.Results.Bindings))
	for _, binding := range sparqlResp.Results.Bindings {
		if typeData, ok := binding["type"]; ok {
			types = append(types, typeData.Value)
		}
	}

	return types, nil
}

// executeQueriesParallel executes multiple queries concurrently with timeout
func (p *Preprocessor) executeQueriesParallel(endpointURL string, queries []extractedQuery, timeout time.Duration, acceptLanguage string) map[string]QueryResult {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	resultsChan := make(chan queryExecutionResult, len(queries))
	var wg sync.WaitGroup

	for _, q := range queries {

		wg.Add(1)
		go func(query extractedQuery) {
			defer wg.Done()

			// Check if context already cancelled
			select {
			case <-ctx.Done():
				resultsChan <- queryExecutionResult{
					ID:     query.ID,
					Result: QueryResult{
						Error:    "Query timeout exceeded",
						Query:    p.finalizeQuery(query.Query),
						Endpoint: p.resolveEndpoint(query.Endpoint),
					},
				}
				return
			default:
			}

			// Execute query with label resolution flag and endpoint from query
			queryResult, err := p.ExecuteQuery(query.Query, query.ResolveLabels, acceptLanguage, query.Endpoint)
			if err != nil {
				// Preserve the Query field from queryResult while updating the error message
				queryResult.Error = fmt.Sprintf("Query execution failed: %v", err)
			}
			resultsChan <- queryExecutionResult{ID: query.ID, Result: queryResult}
		}(q)
	}

	// Close channel when all goroutines finish
	go func() {
		wg.Wait()
		close(resultsChan)
	}()

	// Collect results
	results := make(map[string]QueryResult)
	for res := range resultsChan {
		results[res.ID] = res.Result
	}

	return results
}

// ExecuteQuery executes a raw SPARQL query and returns simplified results
// This method is useful for executing queries without template processing
func (p *Preprocessor) ExecuteQuery(query string, resolveLabels bool, acceptLanguage string, endpoint string) (QueryResult, error) {
	// Resolve endpoint (empty string = use default)
	targetEndpoint := p.resolveEndpoint(endpoint)

	// Finalize query with PREFIX declarations if needed (done early so it's available in error cases)
	finalizedQuery := p.finalizeQuery(query)

	response, _, err := p.querySparqlEndpoint(targetEndpoint, query)
	if err != nil {
		return QueryResult{Error: err.Error(), Query: finalizedQuery, Endpoint: targetEndpoint}, err
	}

	var sparqlResp sparqlResponse
	if err := json.Unmarshal(response, &sparqlResp); err != nil {
		return QueryResult{Error: err.Error(), Query: finalizedQuery, Endpoint: targetEndpoint}, err
	}

	result := simplifyBindings(sparqlResp)
	result.Query = finalizedQuery
	result.Endpoint = targetEndpoint

	// Perform label enrichment if requested
	if resolveLabels {
		initLabelCache() // Ensure cleanup goroutine started

		iris := extractIRIs(result)
		if len(iris) > 0 {
			languages := parseAcceptLanguage(acceptLanguage)
			labelMap := fetchLabels(p, targetEndpoint, iris, languages)
			enrichWithLabels(&result, labelMap)
		}
	}

	return result, nil
}
