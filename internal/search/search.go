package search

import (
	"context"
	"log/slog"
	"strconv"

	"github.com/gin-gonic/gin"
	"hutzli.org/visoto/internal/cache"
	"hutzli.org/visoto/internal/logger"
	"hutzli.org/visoto/internal/sparql"
)

// Searcher orchestrates search operations
type Searcher struct {
	preprocessor *sparql.Preprocessor
	provider     Provider
	// endpointURL identifies the active endpoint for cache keying. The
	// preprocessor is already bound to it, so nothing dials with this.
	endpointURL string
}

// New creates a new Searcher instance
func New(preprocessor *sparql.Preprocessor, providerName, endpointURL string) *Searcher {
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

	// A discovering provider caches what it learns about the endpoint, and that
	// cache needs the shared sweeper running. Started here rather than from an
	// init() to keep the lazy-start property internal/cache argues for: a process
	// that never searches or queries spawns no goroutine.
	cache.Init()

	return &Searcher{
		preprocessor: preprocessor,
		provider:     provider,
		endpointURL:  endpointURL,
	}
}

// DefaultLimit is the default number of search results returned when no limit is specified.
const DefaultLimit = 50

// ParseParams extracts and validates search parameters from request
func ParseParams(c *gin.Context) SearchParams {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", strconv.Itoa(DefaultLimit)))

	// Default to rdfs:label when property key is absent (e.g. topbar search ?q=...).
	// Distinguish from the search page explicitly submitting property="" (Any Property).
	property := c.Query("property")
	if property == "" && !c.Request.URL.Query().Has("property") {
		property = "http://www.w3.org/2000/01/rdf-schema#label"
	}

	return SearchParams{
		Query:    c.Query("q"),
		Class:    c.Query("class"),
		Property: property,
		Limit:    limit,
	}
}

// Execute performs the search and returns results
func (s *Searcher) Execute(ctx context.Context, params SearchParams, acceptLanguage string) SearchResult {
	log := logger.Get()

	result := SearchResult{
		Query:    params.Query,
		Provider: s.provider.Name(),
	}

	// Build query using provider. A discovering provider needs to ask the
	// endpoint what it supports first, so it gets the request context, the
	// endpoint identity, and an executor; the rest are pure string builders.
	var query string
	var err error
	if dp, ok := s.provider.(DiscoveringProvider); ok {
		query, err = dp.BuildQueryWithContext(SearchContext{
			Ctx:         ctx,
			Params:      params,
			EndpointURL: s.endpointURL,
			Execute: func(ctx context.Context, q string) (sparql.QueryResult, error) {
				// resolveLabels=false: discovery reads machine-readable config,
				// not display data, and enrichment costs two extra round-trips
				// (precedent: metricHandler in cmd/visoto).
				return s.preprocessor.ExecuteQueryWithContext(ctx, q, false, "", "")
			},
		})
	} else {
		query, err = s.provider.BuildQuery(params)
	}
	// A provider that cannot serve this search at all is not a dead end: the
	// CONTAINS fallback below still can. This is the normal path for a
	// discovering provider whose index does not cover the searched property —
	// most properties are unindexed — so it must fall through, not return.
	var queryResult sparql.QueryResult
	var execErr error

	if err != nil {
		log.Debug("provider could not build a search query, falling back",
			slog.String("provider", s.provider.Name()),
			slog.String("error", err.Error()),
			slog.String("query", params.Query))
		execErr = err
	} else {
		log.Debug("executing search query",
			slog.String("provider", s.provider.Name()),
			slog.String("search_query", params.Query),
			slog.String("class", params.Class),
			slog.String("property", params.Property),
			slog.String("sparql", query))

		// Execute via SPARQL preprocessor with label enrichment enabled (empty endpoint = use default)
		queryResult, execErr = s.preprocessor.ExecuteQueryWithContext(ctx, query, true, acceptLanguage, "")
		if execErr != nil {
			log.Error("search query execution failed",
				slog.String("error", execErr.Error()),
				slog.String("query", params.Query))
		} else {
			result.Results = queryResult
		}
	}

	if execErr != nil {
		result.Results = sparql.QueryResult{
			Error: "Search query failed: " + execErr.Error(),
		}
	}

	// Fallback: retry with SPARQL FILTER(CONTAINS(...)) when native FTS returned
	// nothing, or failed outright.
	//
	// This fires even when the FTS query succeeded and legitimately found nothing,
	// and that is deliberate: an FTS index covers only the properties it was
	// configured for (on LINDAS, 13 label/identifier properties), so "no rows"
	// means "not in the index", NOT "not in the data". The CONTAINS scan still
	// finds matches on every unindexed literal. It costs a second, slower query on
	// no-result searches; that buys real recall and is the right trade. Do not
	// "optimize" this away by trusting an empty FTS result.
	//
	// The error case matters too: a malformed Lucene expression reaches GraphDB as
	// an HTTP 400, where plain CONTAINS over the same user text succeeds.
	//
	// SparqlQueryProvider is instantiated directly (not from registry) to avoid recursion
	// when search_provider = "sparql-query" is already set — that path is a no-op here.
	if execErr != nil || len(queryResult.Bindings) == 0 {
		fallbackProvider := &SparqlQueryProvider{}
		if fallbackQuery, buildErr := fallbackProvider.BuildQuery(params); buildErr == nil {
			if fallbackResult, fbErr := s.preprocessor.ExecuteQueryWithContext(ctx, fallbackQuery, true, acceptLanguage, ""); fbErr == nil && len(fallbackResult.Bindings) > 0 {
				result.Results = fallbackResult
				result.FallbackUsed = true
				log.Debug("FTS returned no results, used sparql-query fallback",
					slog.String("query", params.Query),
					slog.Int("fallback_result_count", len(fallbackResult.Bindings)))
			}
		}
	}

	log.Debug("search completed",
		slog.String("query", params.Query),
		slog.Int("result_count", len(result.Results.Bindings)),
		slog.Bool("fallback_used", result.FallbackUsed))

	return result
}

// The search route is served by searchHandler in cmd/visoto, which also stamps
// the endpoint data every page needs and names the template's language variant.
// A duplicate Handler method used to live here; it was unreferenced and rendered
// pages/search.html without that data, so it was removed rather than kept in
// sync with the language-qualified render path.
