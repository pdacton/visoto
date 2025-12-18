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

	"hutzli.org/visoto/internal/logger"
)

// querySparqlEndpoint sends a SPARQL query to the specified endpoint and returns the response
func querySparqlEndpoint(endpointURL, query string) ([]byte, error) {
	// Prepare the request parameters
	params := url.Values{}
	params.Set("query", query)
	params.Set("format", "application/sparql-results+json")

	// Create the HTTP request
	req, err := http.NewRequest("POST", endpointURL, strings.NewReader(params.Encode()))
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

	fmt.Println("=== SPARQL Response Body ===")
	fmt.Println(string(body))
	fmt.Println("=== End SPARQL Response Body ===")

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
func executeQuery(endpointURL, query string, resolveLabels bool, acceptLanguage string) QueryResult {
	response, err := querySparqlEndpoint(endpointURL, query)
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
			labelMap := fetchLabels(endpointURL, iris, languages)
			enrichWithLabels(&result, labelMap)
		}
	}

	return result
}

// executeQueriesParallel executes multiple queries concurrently with timeout
func executeQueriesParallel(endpointURL string, queries []extractedQuery, timeout time.Duration, acceptLanguage string) map[string]QueryResult {
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
			result := executeQuery(endpointURL, query.Query, query.ResolveLabels, acceptLanguage)
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
