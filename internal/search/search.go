package search

import (
	"log/slog"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"hutzli.org/visoto/internal/logger"
	"hutzli.org/visoto/internal/sparql"
)

// Searcher orchestrates search operations
type Searcher struct {
	preprocessor *sparql.Preprocessor
	provider     Provider
}

// New creates a new Searcher instance
func New(preprocessor *sparql.Preprocessor, providerName string) *Searcher {
	provider := GetDefaultProvider()
	if providerName != "" {
		if p, ok := GetProvider(providerName); ok {
			provider = p
		}
	}

	if provider == nil {
		// Fallback to Stardog if no provider available
		provider = &StardogProvider{}
	}

	return &Searcher{
		preprocessor: preprocessor,
		provider:     provider,
	}
}

// ParseParams extracts and validates search parameters from request
func ParseParams(c *gin.Context) SearchParams {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))

	return SearchParams{
		Query:    c.Query("q"),
		Class:    c.Query("class"),
		Property: c.Query("property"),
		Limit:    limit,
	}
}

// Execute performs the search and returns results
func (s *Searcher) Execute(params SearchParams, acceptLanguage string) SearchResult {
	log := logger.Get()

	result := SearchResult{
		Query:    params.Query,
		Provider: s.provider.Name(),
	}

	// Build query using provider
	query, err := s.provider.BuildQuery(params)
	if err != nil {
		log.Error("failed to build search query",
			slog.String("error", err.Error()),
			slog.String("query", params.Query))
		result.Results = sparql.QueryResult{
			Error: "Failed to build search query: " + err.Error(),
		}
		return result
	}

	log.Debug("executing search query",
		slog.String("provider", s.provider.Name()),
		slog.String("search_query", params.Query),
		slog.String("class", params.Class),
		slog.String("property", params.Property),
		slog.String("sparql", query))

	// Execute via SPARQL preprocessor with label enrichment enabled
	queryResult, err := s.preprocessor.ExecuteQuery(query, true, acceptLanguage)
	if err != nil {
		log.Error("search query execution failed",
			slog.String("error", err.Error()),
			slog.String("query", params.Query))
		result.Results = sparql.QueryResult{
			Error: "Search query failed: " + err.Error(),
		}
		return result
	}

	result.Results = queryResult
	log.Debug("search completed",
		slog.String("query", params.Query),
		slog.Int("result_count", len(queryResult.Bindings)))

	return result
}

// Handler returns a Gin handler function for the search route
func (s *Searcher) Handler() gin.HandlerFunc {
	return func(c *gin.Context) {
		params := ParseParams(c)

		// Validate required query parameter
		if params.Query == "" {
			c.HTML(http.StatusOK, "pages/search.html", gin.H{
				"ClassFilters":    GetClassFilters(),
				"PropertyFilters": GetPropertyFilters(),
				"Error":           "",
			})
			return
		}

		// Execute search
		acceptLanguage := c.Request.Header.Get("Accept-Language")
		result := s.Execute(params, acceptLanguage)

		// Render results
		c.HTML(http.StatusOK, "pages/search.html", gin.H{
			"Query":            result.Query,
			"ClassFilters":     GetClassFilters(),
			"PropertyFilters":  GetPropertyFilters(),
			"SelectedClass":    params.Class,
			"SelectedProperty": params.Property,
			"SearchResults":    result.Results,
			"Provider":         result.Provider,
		})
	}
}
