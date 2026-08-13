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
	"sort"
	"strings"
	"sync"
	"time"

	"hutzli.org/visoto/internal/config"
	"hutzli.org/visoto/internal/logger"
)

// Preprocessor handles SPARQL query execution
type Preprocessor struct {
	config     QueryInput
	httpClient *http.Client
}

// New creates a new Preprocessor with the given configuration
func New(config QueryInput) *Preprocessor {
	return &Preprocessor{
		config:     config,
		httpClient: &http.Client{},
	}
}

// --- Prefix substitutions ---

var (
	reDeclaredPrefixes = regexp.MustCompile(`(?i)PREFIX\s+(\w+):\s*<[^>]+>`)
	reUsedPrefixes     = regexp.MustCompile(`\b(\w+):(?:[a-zA-Z_]|\/)`)
)

// extractDeclaredPrefixes parses query and returns a set of prefix names already declared
// Returns map[string]bool where keys are prefix names (e.g., "rdf", "schema")
func extractDeclaredPrefixes(query string) map[string]bool {
	declared := make(map[string]bool)
	matches := reDeclaredPrefixes.FindAllStringSubmatch(query, -1)
	for _, match := range matches {
		if len(match) >= 2 {
			declared[strings.TrimSpace(match[1])] = true
		}
	}
	return declared
}

// extractUsedPrefixes scans query and returns a set of prefix names that are used
// (e.g., finds "rdf:type", "schema:name" and extracts "rdf", "schema")
// Excludes http:, https:, file: which are not prefixes
func extractUsedPrefixes(query string) map[string]bool {
	used := make(map[string]bool)
	matches := reUsedPrefixes.FindAllStringSubmatch(query, -1)
	for _, match := range matches {
		if len(match) >= 2 {
			lowerPrefix := strings.ToLower(match[1])
			if lowerPrefix == "http" || lowerPrefix == "https" || lowerPrefix == "file" || lowerPrefix == "visoto" {
				continue
			}
			used[match[1]] = true
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
		if !usedSet[prefix.Name] || declaredSet[prefix.Name] {
			continue
		}
		sb.WriteString("PREFIX ")
		sb.WriteString(prefix.Name)
		sb.WriteString(": <")
		sb.WriteString(strings.Trim(prefix.URI, "<>"))
		sb.WriteString(">\n")
		addedAny = true
	}

	if addedAny {
		sb.WriteString("\n")
	}

	return sb.String()
}

// topLanguage returns the highest-priority language code from an Accept-Language header,
// falling back to "en" if the header is empty or unparseable.
func topLanguage(acceptLanguage string) string {
	langs := parseAcceptLanguage(acceptLanguage)
	if len(langs) == 0 {
		return "en"
	}
	// RDF language tags on LINDAS are base codes ("en", "de"), so an "en-US"
	// browser preference must compare as "en" or every lang() filter misses.
	return strings.SplitN(langs[0], "-", 2)[0]
}

// expandMagicProperties replaces configured visoto:<key> tokens with their
// property paths from [rdf.magic_properties] in visoto.config. The expansion is
// wrapped in parentheses so alternations (a|b|c) stay intact when the token is
// used inside a larger property path. Keys are applied longest-first and matched
// with a trailing word boundary so one key never partially matches another token.
// "dispLang" is reserved for the built-in language substitution.
func (p *Preprocessor) expandMagicProperties(query string) string {
	if len(p.config.MagicProperties) == 0 || !strings.Contains(query, "visoto:") {
		return query
	}
	keys := make([]string, 0, len(p.config.MagicProperties))
	for k := range p.config.MagicProperties {
		if k == "dispLang" {
			continue
		}
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if len(keys[i]) != len(keys[j]) {
			return len(keys[i]) > len(keys[j])
		}
		return keys[i] < keys[j]
	})
	for _, k := range keys {
		re := regexp.MustCompile(`visoto:` + regexp.QuoteMeta(k) + `\b`)
		query = re.ReplaceAllString(query, "("+p.config.MagicProperties[k]+")")
	}
	return query
}

// finalizeQuery substitutes magic tokens and adds PREFIX declarations to a query.
// visoto:dispLang is replaced with the browser's top language preference (e.g. "de");
// other visoto:<key> tokens come from [rdf.magic_properties] in visoto.config.
// Magic properties expand before prefix extraction, so prefixes used inside an
// expansion are declared automatically.
func (p *Preprocessor) finalizeQuery(query string, acceptLanguage string) string {
	lang := topLanguage(acceptLanguage)
	query = strings.ReplaceAll(query, "visoto:dispLang", fmt.Sprintf("%q", lang))
	query = p.expandMagicProperties(query)
	declaredPrefixes := extractDeclaredPrefixes(query)
	usedPrefixes := extractUsedPrefixes(query)
	prefixBlock := buildNeededPrefixBlock(p.config.Prefixes, usedPrefixes, declaredPrefixes)
	return prefixBlock + query
}

// FinalizeQuery is the exported form of finalizeQuery: it returns the query as it
// would be sent to the endpoint (PREFIXes prepended, visoto:dispLang resolved)
// WITHOUT executing it. Used by the working-set table shell to embed the same
// query string the inline path would surface, so the "Execute on endpoint" button
// opens the identical query without a network round-trip.
func (p *Preprocessor) FinalizeQuery(query string, acceptLanguage string) string {
	return p.finalizeQuery(query, acceptLanguage)
}

// --- Endpoint resolution ---

// resolveEndpoint determines which endpoint URL to use
// Priority: 1) Named endpoint lookup, 2) Direct URL, 3) Default
// a direct URL can be provided in the template <sparql-query endpoint="https://dbpedia.org/sparql">
// the end client can also provide a identifier for a named endpoint
func (p *Preprocessor) resolveEndpoint(endpointAttr string) string {
	if endpointAttr == "" {
		return p.config.EndpointURL
	}
	if url, exists := p.config.NamedEndpoints[endpointAttr]; exists {
		return url
	}
	return endpointAttr // direct URL provided in query attribute (e.g., endpoint="https://dbpedia.org/sparql")
}

// --- send actual query ---

// querySparqlEndpoint sends a SPARQL query to the specified endpoint and returns the response.
// The query must already be finalized (PREFIX declarations added).
func (p *Preprocessor) querySparqlEndpoint(ctx context.Context, endpointURL, query string) ([]byte, error) {
	params := url.Values{}
	params.Set("query", query)
	params.Set("format", "application/sparql-results+json")
	encodedParams := params.Encode()

	req, err := http.NewRequestWithContext(ctx, "POST", endpointURL, strings.NewReader(encodedParams))
	if err != nil {
		log := logger.Get()
		log.Error("failed to create HTTP request",
			slog.String("error", err.Error()),
			slog.String("endpoint", endpointURL))
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/sparql-results+json")

	log := logger.Get()
	log.Debug("HTTP request details",
		slog.String("method", req.Method),
		slog.String("url", req.URL.String()),
		slog.String("content-type", req.Header.Get("Content-Type")),
		slog.String("accept", req.Header.Get("Accept")),
		slog.Int("body_length", len(encodedParams)))

	resp, err := p.httpClient.Do(req)
	if err != nil {
		log.Error("failed to execute HTTP request",
			slog.String("error", err.Error()),
			slog.String("endpoint", endpointURL))
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, resp.Body)
		log.Error("SPARQL endpoint returned HTTP error",
			slog.Int("status_code", resp.StatusCode),
			slog.String("status", resp.Status),
			slog.String("endpoint", endpointURL))
		return nil, fmt.Errorf("HTTP error: %d %s", resp.StatusCode, resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Error("failed to read response body",
			slog.String("error", err.Error()),
			slog.String("endpoint", endpointURL))
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	return body, nil
}

// --- Response parsing ---

// simplifyBindings converts sparqlResponse to simplified QueryResult format
// we basically take out the Head and Results and keep the Vars and Bindings only
func simplifyBindings(resp sparqlResponse) QueryResult {
	result := QueryResult{
		Vars:     resp.Head.Vars,
		Bindings: make([]map[string]Binding, 0, len(resp.Results.Bindings)),
	}

	// copy each binding individually: sparqlResponse bindings lack the DisplayText field
	// that QueryResult.Binding has, so the types differ and a direct assignment isn't possible
	for _, binding := range resp.Results.Bindings {
		simplified := make(map[string]Binding)
		for varName, varData := range binding {
			simplified[varName] = Binding{
				Type:        varData.Type,
				Value:       varData.Value,
				DisplayText: varData.Value,
			}
		}
		result.Bindings = append(result.Bindings, simplified)
	}

	return result
}

// --- Query execution (private) ---

// executeQueryWithContext executes a SPARQL query using the provided context (supports cancellation/timeout)
func (p *Preprocessor) executeQueryWithContext(ctx context.Context, query string, resolveLabels bool, acceptLanguage string, endpoint string, opts ...Option) (QueryResult, error) {
	settings := newOptions(opts)
	targetEndpoint := p.resolveEndpoint(endpoint)
	finalizedQuery := p.finalizeQuery(query, acceptLanguage)

	response, err := p.querySparqlEndpoint(ctx, targetEndpoint, finalizedQuery)
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

	if resolveLabels {
		initCaches()
		iris := extractIRIs(result)
		if len(iris) > 0 {
			languages := parseAcceptLanguage(acceptLanguage)

			// Labels and types are two independent questions about the same set of
			// IRIs, so they go to the endpoint concurrently: the wall-clock cost is
			// the slower of the two, not their sum. Each degrades on its own — a
			// failed type query leaves labels intact and vice versa.
			var labelMap map[string]string
			var typeMap map[string][]string
			var wg sync.WaitGroup

			wg.Add(1)
			go func() {
				defer wg.Done()
				labelMap = fetchLabels(p, targetEndpoint, iris, languages)
			}()

			if settings.resolveTypes {
				wg.Add(1)
				go func() {
					defer wg.Done()
					typeMap = fetchTypes(p, targetEndpoint, iris)
				}()
			}
			wg.Wait()

			enrichWithLabels(&result, labelMap)
			if settings.resolveTypes {
				enrichWithIcons(&result, typeMap)
			}
		}
	}

	return result, nil
}

// ExecuteQueriesParallel executes multiple queries concurrently with timeout
func (p *Preprocessor) ExecuteQueriesParallel(queries []ExtractedQuery, timeout time.Duration, acceptLanguage string) map[string]QueryResult {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	resultsChan := make(chan queryExecutionResult, len(queries))
	var wg sync.WaitGroup

	for _, q := range queries {
		wg.Add(1)
		go func(query ExtractedQuery) {
			defer wg.Done()

			select {
			case <-ctx.Done():
				resultsChan <- queryExecutionResult{
					ID: query.ID,
					Result: QueryResult{
						Error:    "Query timeout exceeded",
						Query:    p.finalizeQuery(query.Query, acceptLanguage),
						Endpoint: p.resolveEndpoint(query.Endpoint),
					},
				}
				return
			default:
			}

			var opts []Option
			if query.ResolveTypes {
				opts = append(opts, WithTypes())
			}
			queryResult, err := p.executeQueryWithContext(ctx, query.Query, query.ResolveLabels, acceptLanguage, query.Endpoint, opts...)
			if err != nil {
				queryResult.Error = fmt.Sprintf("Query execution failed: %v", err)
			}
			resultsChan <- queryExecutionResult{ID: query.ID, Result: queryResult}
		}(q)
	}

	go func() {
		wg.Wait()
		close(resultsChan)
	}()

	results := make(map[string]QueryResult)
	for res := range resultsChan {
		results[res.ID] = res.Result
	}

	return results
}

// --- Public functions ---

// ExecuteQuery executes a raw SPARQL query and returns simplified results
// This method is useful for executing queries without template processing
func (p *Preprocessor) ExecuteQuery(query string, resolveLabels bool, acceptLanguage string, endpoint string, opts ...Option) (QueryResult, error) {
	return p.executeQueryWithContext(context.Background(), query, resolveLabels, acceptLanguage, endpoint, opts...)
}

// ExecuteQueryWithContext executes a raw SPARQL query bound to the given context,
// so callers can enforce a per-request timeout (unlike ExecuteQuery, which uses
// context.Background()). Used by the working-set table endpoint, where a
// single working-set query can run several seconds against a large class.
func (p *Preprocessor) ExecuteQueryWithContext(ctx context.Context, query string, resolveLabels bool, acceptLanguage string, endpoint string, opts ...Option) (QueryResult, error) {
	return p.executeQueryWithContext(ctx, query, resolveLabels, acceptLanguage, endpoint, opts...)
}

// QueryIsClass checks whether the given IRI is a class by looking for:
// - rdfs:subClassOf relationships (as subject or object)
// - incoming rdf:type statements (implicitly defined classes)
func (p *Preprocessor) QueryIsClass(iri string) (bool, error) {
	query := fmt.Sprintf(`ASK {
		{ <%s> rdfs:subClassOf ?x }
		UNION
		{ ?x rdfs:subClassOf <%s> }
		UNION
		{ ?x a <%s> }
	}`, iri, iri, iri)

	finalizedQuery := p.finalizeQuery(query, "")
	response, err := p.querySparqlEndpoint(context.Background(), p.config.EndpointURL, finalizedQuery)
	if err != nil {
		return false, fmt.Errorf("failed to query class status for IRI %s: %w", iri, err)
	}

	var askResp struct {
		Boolean bool `json:"boolean"`
	}
	if err := json.Unmarshal(response, &askResp); err != nil {
		return false, fmt.Errorf("failed to parse ASK response: %w", err)
	}

	return askResp.Boolean, nil
}

// QueryTypes queries the SPARQL endpoint for the rdf:type of a given IRI
// Returns a slice of type URIs
func (p *Preprocessor) QueryTypes(iri string) ([]string, error) {
	query := fmt.Sprintf("SELECT ?type WHERE { <%s> a ?type }", iri)

	finalizedQuery := p.finalizeQuery(query, "")
	response, err := p.querySparqlEndpoint(context.Background(), p.config.EndpointURL, finalizedQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to query types for IRI %s: %w", iri, err)
	}

	var sparqlResp sparqlResponse
	if err := json.Unmarshal(response, &sparqlResp); err != nil {
		return nil, fmt.Errorf("failed to parse SPARQL response: %w", err)
	}

	types := make([]string, 0, len(sparqlResp.Results.Bindings))
	for _, binding := range sparqlResp.Results.Bindings {
		if typeData, ok := binding["type"]; ok {
			types = append(types, typeData.Value)
		}
	}

	return types, nil
}
