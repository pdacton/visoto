package parser

// Package parser handles HTML template parsing for SPARQL custom elements
// and orchestrates query execution via the sparql package.

import (
	"fmt"
	"os"
	"strings"
	"time"

	"hutzli.org/visoto/internal/sparql"
)

// ----- Preprocessor -----

// Preprocessor handles template parsing and SPARQL query execution for templates
type Preprocessor struct {
	sp      *sparql.Preprocessor
	timeout time.Duration
}

// New creates a new Preprocessor with the given configuration
func New(config sparql.QueryInput) *Preprocessor {
	return &Preprocessor{
		sp:      sparql.New(config),
		timeout: config.Timeout,
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

	// Replace the entity placeholder `??` with the provided IRI in each query
	for i := range queries {
		queries[i].Query = strings.ReplaceAll(queries[i].Query, "??", fmt.Sprintf("<%s>", iri))
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

// ExecuteQuery executes a raw SPARQL query and returns simplified results
func (p *Preprocessor) ExecuteQuery(query string, resolveLabels bool, acceptLanguage string, endpoint string) (sparql.QueryResult, error) {
	return p.sp.ExecuteQuery(query, resolveLabels, acceptLanguage, endpoint)
}

// QueryTypes queries the SPARQL endpoint for the rdf:type of a given IRI
func (p *Preprocessor) QueryTypes(iri string) ([]string, error) {
	return p.sp.QueryTypes(iri)
}
