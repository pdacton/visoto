package parser

// Package parser handles HTML template parsing for SPARQL custom elements
// and orchestrates query execution via the sparql package.

import (
	"context"
	"fmt"
	"os"
	"time"

	"hutzli.org/visoto/internal/sparql"
)

// ----- Preprocessor -----

// defaultComponentsDir is the path to the components directory relative to the working directory.
const defaultComponentsDir = "./templates/components"

// Preprocessor handles template parsing and SPARQL query execution for templates
type Preprocessor struct {
	sp            *sparql.Preprocessor
	timeout       time.Duration
	componentsDir string
}

// New creates a new Preprocessor with the given configuration
func New(config sparql.QueryInput) *Preprocessor {
	return &Preprocessor{
		sp:            sparql.New(config),
		timeout:       config.Timeout,
		componentsDir: defaultComponentsDir,
	}
}

// SparqlPreprocessor returns the underlying sparql.Preprocessor for callers
// that need direct access to query execution (e.g. search package).
func (p *Preprocessor) SparqlPreprocessor() *sparql.Preprocessor {
	return p.sp
}

// ----- Template processing -----

// ProcessTemplateFile reads a template file, extracts and executes SPARQL queries,
// and returns the template data with query results
func (p *Preprocessor) ProcessTemplateFile(filepath string, iri string, acceptLanguage string) (TemplateData, error) {

	// Read template file
	content, err := os.ReadFile(filepath)
	if err != nil {
		return TemplateData{}, fmt.Errorf("failed to read template file: %w", err)
	}

	// Extract SPARQL queries from template using DOM parser
	queries, err := extractQueriesDOM(string(content))
	if err != nil {
		return TemplateData{}, fmt.Errorf("failed to extract queries: %w", err)
	}

	// Discover and add queries from included component files
	if p.componentsDir != "" {
		existingIDs := make(map[string]bool, len(queries))
		for _, q := range queries {
			existingIDs[q.ID] = true
		}
		includes := extractTemplateIncludes(string(content))
		extra, err := loadComponentQueries(p.componentsDir, includes, existingIDs)
		if err != nil {
			return TemplateData{}, fmt.Errorf("component queries: %w", err)
		}
		queries = append(queries, extra...)
	}

	// Replace the entity placeholder `??` with the provided IRI in each query.
	// The IRI comes from the request, so it is validated first: one that could
	// close the <...> term would inject graph patterns into every query on the page.
	// An empty iri means "no substitution" (plain pages call it that way), so the
	// placeholder is left untouched rather than validated.
	if iri != "" {
		for i := range queries {
			scoped, err := sparql.SubstituteEntity(queries[i].Query, iri)
			if err != nil {
				return TemplateData{}, fmt.Errorf("resource IRI: %w", err)
			}
			queries[i].Query = scoped
		}
	}

	// Execute queries in parallel with language preference
	results := p.sp.ExecuteQueriesParallel(queries, p.timeout, acceptLanguage)

	// Create TemplateData with results
	data := TemplateData{
		ResourceIRI:  iri,
		QueryResults: results,
	}

	return data, nil
}

// ----- Query execution (delegated to sparql.Preprocessor) -----

// ExecuteQuery executes a raw SPARQL query and returns simplified results.
// Pass sparql.WithTypes() to additionally resolve resource icons.
func (p *Preprocessor) ExecuteQuery(query string, resolveLabels bool, acceptLanguage string, endpoint string, opts ...sparql.Option) (sparql.QueryResult, error) {
	return p.sp.ExecuteQuery(query, resolveLabels, acceptLanguage, endpoint, opts...)
}

// ExecuteQueryWithContext executes a raw SPARQL query bound to the given context,
// allowing the caller to enforce a per-request timeout.
func (p *Preprocessor) ExecuteQueryWithContext(ctx context.Context, query string, resolveLabels bool, acceptLanguage string, endpoint string, opts ...sparql.Option) (sparql.QueryResult, error) {
	return p.sp.ExecuteQueryWithContext(ctx, query, resolveLabels, acceptLanguage, endpoint, opts...)
}

// FinalizeQuery returns the query as it would be sent to the endpoint (PREFIXes
// prepended, visoto:dispLang resolved) without executing it.
func (p *Preprocessor) FinalizeQuery(query string, acceptLanguage string) string {
	return p.sp.FinalizeQuery(query, acceptLanguage)
}

// QueryTypes queries the SPARQL endpoint for the rdf:type of a given IRI
func (p *Preprocessor) QueryTypes(iri string) ([]string, error) {
	return p.sp.QueryTypes(iri)
}

// QueryIsClass checks whether the given IRI is a class
func (p *Preprocessor) QueryIsClass(iri string) (bool, error) {
	return p.sp.QueryIsClass(iri)
}
