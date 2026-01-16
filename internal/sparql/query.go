package sparql

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"hutzli.org/visoto/internal/config"
	"hutzli.org/visoto/internal/logger"
)

// buildPrefixBlock generates SPARQL PREFIX declarations from config
func buildPrefixBlock(prefixes []config.Prefix) string {
	if len(prefixes) == 0 {
		return ""
	}

	var sb strings.Builder
	for _, prefix := range prefixes {
		sb.WriteString("PREFIX ")
		sb.WriteString(prefix.Name)
		sb.WriteString(": <")
		sb.WriteString(strings.Trim(prefix.URI, "<>"))
		sb.WriteString(">\n")
	}
	sb.WriteString("\n")
	return sb.String()
}

// hasExistingPrefixes checks if query already contains PREFIX declarations
func hasExistingPrefixes(query string) bool {
	trimmed := strings.TrimSpace(query)
	return strings.HasPrefix(strings.ToUpper(trimmed), "PREFIX")
}

// queryNeedsPrefixes checks if a query uses any prefixed names (e.g., rdf:type, schema:name)
// Returns true if the query contains patterns like "word:word" that aren't URLs
// TODO: I don't think this is a very robust implementation
func queryNeedsPrefixes(query string) bool {
	// Simple heuristic: look for common SPARQL prefixed names
	// Matches patterns like "rdf:type", "schema:name", etc.
	// But excludes "http:", "https:", "file:" which are URLs
	commonPrefixes := []string{
		"rdf:", "rdfs:", "owl:", "xsd:",
		"schema:", "foaf:", "dc:", "dcterms:",
		"skos:", "sch:", "regch:", "ref:",
	}

	queryUpper := strings.ToUpper(query)
	for _, prefix := range commonPrefixes {
		if strings.Contains(queryUpper, strings.ToUpper(prefix)) {
			return true
		}
	}

	return false
}

// querySparqlEndpoint sends a SPARQL query to the specified endpoint and returns the response
func (p *Preprocessor) querySparqlEndpoint(endpointURL, query string) ([]byte, error) {

	// prepend the query with PREFIX declarations if missing and needed
	if !hasExistingPrefixes(query) && queryNeedsPrefixes(query) {
		query = buildPrefixBlock(p.config.Prefixes) + query
	}

	// Prepare the request parameters
	params := url.Values{}
	params.Set("query", query)
	params.Set("format", "application/sparql-results+json")
	encodedParams := params.Encode()

	// Create the HTTP request
	req, err := http.NewRequest("POST", endpointURL, strings.NewReader(encodedParams))
	if err != nil {
		log := logger.Get()
		log.Error("failed to create HTTP request",
			slog.String("error", err.Error()),
			slog.String("endpoint", endpointURL))
		return nil, fmt.Errorf("failed to create request: %w", err)
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
		return nil, fmt.Errorf("failed to execute request: %w", err)
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
			slog.String("query", query))

		return nil, fmt.Errorf("HTTP error: %d %s", resp.StatusCode, resp.Status)
	}

	// Read the response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log := logger.Get()
		log.Error("failed to read response body",
			slog.String("error", err.Error()),
			slog.String("endpoint", endpointURL))
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// fmt.Println("=== SPARQL Response Body ===")
	// fmt.Println(string(body))
	// fmt.Println("=== End SPARQL Response Body ===")

	return body, nil
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

// executeQuery executes a single SPARQL query and returns simplified result
func (p *Preprocessor) executeQuery(endpointURL, query string, resolveLabels bool, acceptLanguage string) QueryResult {
	response, err := p.querySparqlEndpoint(endpointURL, query)
	if err != nil {
		return QueryResult{Error: fmt.Sprintf("Query execution failed: %v", err)}
	}

	var sparqlResp sparqlResponse
	if err := json.Unmarshal(response, &sparqlResp); err != nil {
		return QueryResult{Error: fmt.Sprintf("Failed to parse response: %v", err)}
	}

	result := simplifyBindings(sparqlResp)

	// Perform label enrichment if requested
	if resolveLabels {
		initLabelCache() // Ensure cleanup goroutine started

		iris := extractIRIs(result)
		if len(iris) > 0 {
			languages := parseAcceptLanguage(acceptLanguage)
			labelMap := fetchLabels(p, endpointURL, iris, languages)
			enrichWithLabels(&result, labelMap)
		}
	}

	return result
}

// QueryTypes queries the SPARQL endpoint for the rdf:type of a given IRI
// Returns a slice of type URIs
func (p *Preprocessor) QueryTypes(iri string) ([]string, error) {
	// Construct SPARQL query to get all types
	query := fmt.Sprintf("SELECT ?type WHERE { <%s> (a|owl:type) ?type }", iri)

	// Execute query
	response, err := p.querySparqlEndpoint(p.config.EndpointURL, query)
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
					Result: QueryResult{Error: "Query timeout exceeded"},
				}
				return
			default:
			}

			// Execute query with label resolution flag
			result := p.executeQuery(endpointURL, query.Query, query.ResolveLabels, acceptLanguage)
			resultsChan <- queryExecutionResult{ID: query.ID, Result: result}
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
func (p *Preprocessor) ExecuteQuery(query string) (QueryResult, error) {
	response, err := p.querySparqlEndpoint(p.config.EndpointURL, query)
	if err != nil {
		return QueryResult{Error: err.Error()}, err
	}

	var sparqlResp sparqlResponse
	if err := json.Unmarshal(response, &sparqlResp); err != nil {
		return QueryResult{Error: err.Error()}, err
	}

	return simplifyBindings(sparqlResp), nil
}
