// this is the main package of the visoto application, responsible for
// initializing configuration, logger, SPARQL preprocessor, and starting the web server with defined routes and handlers.
// to run this code, use the command: go run cmd/visoto/main.go
// to build this code you must install gin-gonic package first using: go get github.com/gin-gonic/gin

package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"hutzli.org/visoto/internal/chat"
	"hutzli.org/visoto/internal/config"
	"hutzli.org/visoto/internal/i18n"
	"hutzli.org/visoto/internal/lang"
	"hutzli.org/visoto/internal/logger"
	mcpserver "hutzli.org/visoto/internal/mcp"
	"hutzli.org/visoto/internal/monitor"
	"hutzli.org/visoto/internal/parser"
	"hutzli.org/visoto/internal/resource"
	"hutzli.org/visoto/internal/search"
	"hutzli.org/visoto/internal/sparql"
	"hutzli.org/visoto/internal/templates"
	"hutzli.org/visoto/internal/upload"
)

// ---- Package-level state ----

var cfg *config.Config
var mon *monitor.Monitor

// siteLangs is the configured UI language set, built once from visoto.config.
var siteLangs *lang.Set

// catalogs holds the UI message catalogs, loaded once from ./locales.
var catalogs *i18n.Catalogs

// Successful, SPARQL-backed responses (metric/table/resource handlers) call
// markCacheable to declare themselves storable by Caddy's cache module. Those
// routes resolve their endpoint from the URL only (resolveEndpoint with
// allowCookie=false) and never emit Set-Cookie, so the response is a pure
// function of the URL and the negotiated language, and safe for a shared cache.
// The Cache-Control and Vary values themselves are assembled centrally — see
// cacheControlFor in etag.go, which explains why the browser is told to
// revalidate while Souin is still allowed to serve for hours.

// ---- Request helpers ----

// activeEndpointKey is the gin-context key under which resolveEndpoint stores
// the request's resolved *config.SparqlEndpoint.
const activeEndpointKey = "activeEndpoint"

// configLanguages converts the configured languages into the form
// internal/lang takes. The two structs are deliberately separate so the language
// resolver does not depend on the config loader.
func configLanguages(in []config.Language) []lang.Language {
	out := make([]lang.Language, 0, len(in))
	for _, l := range in {
		out = append(out, lang.Language{Code: l.Code, Label: l.Label})
	}
	return out
}

// activeLangKey / langViaHeaderKey are the gin-context keys under which
// resolveLang stores the request's UI language and whether it arrived via the
// X-Site-Lang header.
const (
	activeLangKey    = "activeLang"
	langViaHeaderKey = "langViaHeader"
)

// resolveLang resolves the request's UI language once, into the context.
//
// Unlike resolveEndpoint there is only one variant: the language is safe to read
// from a cookie even on routes in the shared Caddy/Souin cache, because in
// production Caddy normalizes the cookie and Accept-Language into X-Site-Lang
// *before* the cache and folds that header into the cache key. The cookie branch
// inside lang.FromRequest therefore only ever runs in dev, where no shared cache
// exists. See internal/lang.
func resolveLang() gin.HandlerFunc {
	return func(c *gin.Context) {
		code, viaHeader := siteLangs.FromRequest(c.Request)
		c.Set(activeLangKey, code)
		c.Set(langViaHeaderKey, viaHeader)
	}
}

// activeLang returns the UI language resolved by resolveLang, falling back to
// the configured default when the middleware has not run (e.g. in tests).
func activeLang(c *gin.Context) string {
	if v, ok := c.Get(activeLangKey); ok {
		if code, ok := v.(string); ok {
			return code
		}
	}
	return siteLangs.Default()
}

// renderName maps a logical template name ("pages/home.html") onto the variant
// registered for this request's language. Every c.HTML call must go through it —
// the bare name is not registered with the renderer.
func renderName(c *gin.Context, templateName string) string {
	return templates.Name(activeLang(c), templateName)
}

// langViaHeader reports whether the request carried an X-Site-Lang header, i.e.
// whether a normalizing cache sits in front of us. Decides the Vary header.
func langViaHeader(c *gin.Context) bool {
	v, ok := c.Get(langViaHeaderKey)
	if !ok {
		return false
	}
	b, _ := v.(bool)
	return b
}

// resolveEndpoint returns a middleware that resolves the active SPARQL endpoint
// exactly once per request into the context: ?endpoint=<slug> → (optionally) the
// selectedEndpoint cookie (also a slug) → config default.
//
// allowCookie must be false on routes cached by the shared Caddy/Souin cache
// (/resource and the /api fragment routes): their cache key is URL-only, so the
// response must be a pure function of the URL — a cookie-dependent response
// rendered for one user would be served to everyone. Uncached entry pages
// (/, static pages, /search, /monitoring) honor the cookie as the user's saved
// preference; from there the frontend propagates the slug via URLs.
//
// The cookie is written by the client only (endpoint-switcher.js); the server
// never sets it. Stale values — a removed endpoint's slug, or a legacy
// name-valued cookie — simply match no endpoint and fall through to the default.
func resolveEndpoint(allowCookie bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		var ep *config.SparqlEndpoint
		if slug := c.Query("endpoint"); slug != "" {
			if ep = cfg.Application.GetEndpointBySlug(slug); ep == nil {
				logger.Get().Warn("unknown endpoint slug in URL", slog.String("slug", slug))
			}
		}
		if ep == nil && allowCookie {
			if slug, err := c.Cookie("selectedEndpoint"); err == nil && slug != "" {
				ep = cfg.Application.GetEndpointBySlug(slug)
			}
		}
		if ep == nil {
			ep = cfg.Application.DefaultEndpoint()
		}
		if ep != nil {
			c.Set(activeEndpointKey, ep)
		}
	}
}

// activeEndpoint returns the endpoint resolved by the resolveEndpoint middleware,
// or nil when none is configured (bare sparql_endpoint-only configs) or the
// route has no resolver attached.
func activeEndpoint(c *gin.Context) *config.SparqlEndpoint {
	if v, ok := c.Get(activeEndpointKey); ok {
		if ep, ok := v.(*config.SparqlEndpoint); ok {
			return ep
		}
	}
	return nil
}

// activeEndpointURL returns the resolved endpoint's URL, falling back to the
// scalar sparql_endpoint config value when no endpoint list is configured.
func activeEndpointURL(c *gin.Context) string {
	if ep := activeEndpoint(c); ep != nil {
		return ep.URL
	}
	return cfg.Application.SparqlEndpoint
}

// stampEndpointData fills the endpoint-related template fields shared by every
// server-rendered page from the request's resolved endpoint.
func stampEndpointData(c *gin.Context, data *parser.TemplateData) {
	data.SparqlEndpoints = cfg.Application.SparqlEndpoints
	if ep := activeEndpoint(c); ep != nil {
		data.SelectedEndpointName = ep.Name
		data.SelectedEndpointSlug = ep.Slug
		data.EndpointTag = ep.Tag
	}
	data.EndpointURL = activeEndpointURL(c)
}

// endpointTemplateData is stampEndpointData's counterpart for handlers that
// render with an ad-hoc gin.H instead of parser.TemplateData.
func endpointTemplateData(c *gin.Context, h gin.H) gin.H {
	h["SparqlEndpoints"] = cfg.Application.SparqlEndpoints
	if ep := activeEndpoint(c); ep != nil {
		h["SelectedEndpointName"] = ep.Name
		h["SelectedEndpointSlug"] = ep.Slug
		h["EndpointTag"] = ep.Tag
	}
	return h
}

// prepareQueryInputs creates a preprocessor defaulting to the request's resolved endpoint
func prepareQueryInputs(c *gin.Context) *parser.Preprocessor {

	namedEndpoints := cfg.Application.GetNamedEndpointsMap()

	// Create query inputs for preprocessor
	return parser.New(sparql.QueryInput{
		EndpointURL:     activeEndpointURL(c),
		Timeout:         cfg.GetTimeout(),
		Prefixes:        cfg.RDF.ParsedPrefixes,
		NamedEndpoints:  namedEndpoints,
		MagicProperties: cfg.RDF.MagicProperties,
	})
}

// parseDuration maps a query param like "1h","24h","7d","30d","3M" to a time.Duration.
func parseDuration(s string) time.Duration {
	switch s {
	case "1h":
		return 1 * time.Hour
	case "24h":
		return 24 * time.Hour
	case "7d":
		return 7 * 24 * time.Hour
	case "30d":
		return 30 * 24 * time.Hour
	case "3M":
		return 90 * 24 * time.Hour
	default:
		return 24 * time.Hour
	}
}

// ---- Page handlers ----

func staticPageHandler(c *gin.Context) {

	log := logger.Get()

	// Extract page name from URL parameter
	pageName := c.Param("page")

	// Validate: must end with .html and must be a bare filename — the gin route
	// (/:page) already rejects extra path segments, but guard against traversal
	// explicitly so the template path below can never escape templates/pages/.
	if !strings.HasSuffix(pageName, ".html") ||
		strings.Contains(pageName, "..") || strings.ContainsAny(pageName, `/\`) {
		c.String(http.StatusNotFound, "Page not found")
		return
	}

	// Get language preference from request
	acceptLanguage := c.GetHeader("Accept-Language")

	// Construct template paths
	templatePath := "templates/pages/" + pageName
	templateName := "pages/" + pageName

	log.Debug("processing static page request",
		slog.String("page", pageName),
		slog.String("templatePath", templatePath),
		slog.String("templateName", templateName))

	// Check if template file exists
	if _, err := os.Stat(templatePath); os.IsNotExist(err) {
		log.Debug("template not found", slog.String("path", templatePath))
		c.String(http.StatusNotFound, "Page not found")
		return
	}

	// Create preprocessor with user-selected endpoint
	preprocessor := prepareQueryInputs(c)

	// Process SPARQL queries in template (WITHOUT IRI substitution, use per-request preprocessor)
	data, err := preprocessor.ProcessTemplateFile(templatePath, "", acceptLanguage)
	if err != nil {
		log.Error("SPARQL processing failed",
			slog.String("page", pageName),
			slog.String("error", err.Error()))
		c.String(http.StatusInternalServerError, "Error processing page: %v", err)
		return
	}

	// Add endpoints, active selection, and resolved URL (for client-side views
	// like Graph Explorer) to template data
	stampEndpointData(c, &data)
	data.TemplateName = templateName
	// Public origin for pages that render absolute URLs (e.g. the MCP URL on /connect.html)
	data.BaseURL = mcpserver.BaseURLFromRequest(c.Request, cfg.Application.Port)

	log.Debug("rendering static page",
		slog.String("templateName", templateName),
		slog.Int("queryResults", len(data.QueryResults)))

	// Render template
	c.HTML(http.StatusOK, renderName(c, templateName), data)
}

// legacyResourceRedirectHandler permanently redirects old path-form resource
// URLs (/resource/<iri>) to the canonical query form (/resource?iri=<iri>).
// Other query params (e.g. ?endpoint=) are carried over; the browser keeps
// any #view fragment across the redirect on its own.
func legacyResourceRedirectHandler(c *gin.Context) {
	iri := strings.TrimPrefix(c.Param("path"), "/")
	target := "/resource?iri=" + url.QueryEscape(iri)
	if raw := c.Request.URL.RawQuery; raw != "" {
		target += "&" + raw
	}
	c.Redirect(http.StatusMovedPermanently, target)
}

func resourcePageHandler(c *gin.Context) {

	// extract iri from query param
	iri := c.Query("iri")

	// Get language preference from request
	language := c.GetHeader("Accept-Language")

	// log request
	log := logger.Get()
	log.Debug("processing resource page request", slog.String("iri", iri))

	// Create preprocessor with user-selected endpoint (inside cookie)
	preprocessor := prepareQueryInputs(c)

	// create Resource instance
	r, err := resource.New(iri, cfg.RDF.ParsedPrefixes)
	if err != nil {
		log.Error("invalid resource IRI", slog.String("iri", iri), slog.String("error", err.Error()))
		c.Header("Cache-Control", "no-store")
		c.String(http.StatusBadRequest, err.Error())
		return
	}

	// Resolve template based on IRI and RDF types (use per-request preprocessor)
	if err := r.ResolveTemplate(preprocessor, cfg.RDF.TypePriority, cfg.RDF.ParsedPrefixes); err != nil {
		log.Error("template resolution failed",
			slog.String("iri", iri),
			slog.String("error", err.Error()))
		// Continue with fallback already set by ResolveTemplate
	}

	// TODO: refactor, move into Resource method
	// extract SPARQL query from template and retrieve data (use per-request preprocessor)
	r.Data, err = preprocessor.ProcessTemplateFile(r.TemplatePath, r.IRI, language)
	if err != nil {
		c.Header("Cache-Control", "no-store")
		c.String(http.StatusInternalServerError, "Preprocessing error: %v", err)
		return
	}

	// Add endpoints, active selection, and resolved URL (for client-side views
	// like Graph and Schema) to template data
	stampEndpointData(c, &r.Data)
	r.Data.ShortIRI = r.ShortIRI
	r.Data.TemplateName = r.TemplateName

	// List the named graphs containing this resource (shown in the IRI dropdown)
	r.FetchNamedGraphs(c.Request.Context(), preprocessor, cfg.RDF.ParsedPrefixes)

	markCacheable(c)
	// native gin template rendering
	c.HTML(http.StatusOK, renderName(c, r.TemplateName), r.Data)
}

func searchHandler(c *gin.Context) {
	// Create preprocessor with user-selected endpoint
	preprocessor := prepareQueryInputs(c)

	// Resolve FTS provider from the active endpoint config; fall back to "stardog"
	providerName := "stardog"
	if ep := activeEndpoint(c); ep != nil && ep.SearchProvider != "" {
		providerName = ep.SearchProvider
	}
	searcher := search.New(preprocessor.SparqlPreprocessor(), providerName)

	// Parse search parameters
	params := search.ParseParams(c)

	// Validate required query parameter
	if params.Query == "" {
		c.HTML(http.StatusOK, renderName(c, "pages/search.html"), endpointTemplateData(c, gin.H{
			"ClassFilters":    search.GetClassFilters(),
			"PropertyFilters": search.GetPropertyFilters(),
			"SelectedLimit":   search.DefaultLimit,
			"Error":           "",
		}))
		return
	}

	// Execute search
	acceptLanguage := c.Request.Header.Get("Accept-Language")
	result := searcher.Execute(params, acceptLanguage)

	// Render results
	c.HTML(http.StatusOK, renderName(c, "pages/search.html"), endpointTemplateData(c, gin.H{
		"Query":            result.Query,
		"ClassFilters":     search.GetClassFilters(),
		"PropertyFilters":  search.GetPropertyFilters(),
		"SelectedClass":    params.Class,
		"SelectedProperty": params.Property,
		"SelectedLimit":    params.Limit,
		"SearchResults":    result.Results,
		"Provider":         result.Provider,
	}))
}

// asyncQueryDirs are the template directories scanned for <sparql-async id=...>
// declarations. Pages host metric queries; layouts host the resource Data view;
// instances/classes host per-type resource tables; components are included for
// forward-looking async consumers.
var asyncQueryDirs = []string{
	"templates/pages",
	"templates/layout",
	"templates/instances",
	"templates/classes",
	"templates/components",
}

// findAsyncQuery locates the query text of a <sparql-async id=id> element across
// all async template directories. Shared by metricHandler and asyncTableHandler.
// The underlying scan is memoized and invalidated on template file changes (see
// scanTemplateElements), since this runs on every async request.
func findAsyncQuery(id string) (string, bool) {
	for _, el := range scanTemplateElements(parser.ExtractAsyncElements, "async") {
		if el.ID == id {
			return el.Content, true
		}
	}
	return "", false
}

// metricHandler serves /api/metric/:id — called by HTMX to lazily load metric counts on the home page.
// It reads the sparql-async element with the matching id, executes its query, and returns the count.
func metricHandler(c *gin.Context) {
	id := c.Param("id")
	acceptLanguage := c.GetHeader("Accept-Language")

	query, found := findAsyncQuery(id)
	if !found {
		c.String(http.StatusNotFound, "0")
		return
	}

	// Optional resource scoping, same substitution as asyncTableHandler: lets
	// instance templates declare per-resource metrics with the ?? placeholder.
	// The IRI is request input, so it is validated before substitution.
	if iri := c.Query("iri"); iri != "" {
		scoped, err := sparql.SubstituteEntity(query, iri)
		if err != nil {
			c.Header("Cache-Control", "no-store")
			c.String(http.StatusBadRequest, "0")
			return
		}
		query = scoped
	}

	preprocessor := prepareQueryInputs(c)
	result, err := preprocessor.ExecuteQuery(query, false, acceptLanguage, "")

	count := "0"
	if err == nil && len(result.Bindings) > 0 {
		if b, ok := result.Bindings[0]["count"]; ok {
			count = b.Value
		}
	}

	// The response carries the finalized query and endpoint alongside the count so
	// the card's "Execute on endpoint" action can open it in YASGUI — the same data
	// the sparqlTable partial gets from its QueryResult. A failed query still gets
	// them (ExecuteQuery may return early with an empty result), so the action keeps
	// working on an errored metric, mirroring sparqlTable hanging its own YASGUI
	// action off the card rather than the rows.
	finalQuery, endpoint := result.Query, result.Endpoint
	if finalQuery == "" {
		finalQuery = preprocessor.FinalizeQuery(query, acceptLanguage)
	}
	if endpoint == "" {
		endpoint = activeEndpointURL(c)
	}

	html, renderErr := templates.RenderSparqlMetricValue(activeLang(c), map[string]any{
		"queryId":  id,
		"count":    count,
		"query":    finalQuery,
		"endpoint": endpoint,
	})
	if renderErr != nil {
		c.Header("Cache-Control", "no-store")
		c.Header("Content-Type", "text/html")
		c.String(http.StatusOK, count)
		return
	}

	if err != nil {
		// A transient SPARQL failure must never be cached as if it were a real
		// (likely wrong) count.
		c.Header("Cache-Control", "no-store")
	} else {
		markCacheable(c)
	}
	c.Header("Content-Type", "text/html")
	c.String(http.StatusOK, string(html))
}

// asyncTableHandler serves /api/async-table/:id — called by HTMX to lazily load a
// full SPARQL result table (the sparqlAsyncTable partial). It finds the matching
// <sparql-async> query, optionally scopes it to a resource IRI (?? -> <iri>),
// executes it, and returns a rendered sparqlTable HTML fragment.
//
// Presentation params are read from the query string so a single generic handler
// can serve many tables: title, icon, iconVar, badgeVar.
func asyncTableHandler(c *gin.Context) {
	id := c.Param("id")
	acceptLanguage := c.GetHeader("Accept-Language")

	query, found := findAsyncQuery(id)
	if !found {
		c.String(http.StatusNotFound, "unknown async query id")
		return
	}

	params := map[string]any{
		"id":       id,
		"title":    c.Query("title"),
		"icon":     c.Query("icon"),
		"iconVar":  c.Query("iconVar"),
		"badgeVar": c.Query("badgeVar"),
		"groupBy":  c.Query("groupBy"),
		// facetFor (set for faceted tables) = the base query id; the rendered
		// sparqlTable fragment uses it to wire header-attached facet controls from
		// the page's <sparql-facet> elements. Empty for ordinary async tables.
		"facetFor": c.Query("facetFor"),
		// Non-empty string is truthy in the sparqlTable template's {{ if $collapsed }};
		// absent param yields "" (falsy). The template may still force-collapse an
		// empty or errored result regardless of this hint.
		"collapsed": c.Query("collapsed"),
	}

	// Auto-detect the working-set mode: a class-instance query has a "?key a ??"
	// membership triple, so its key var is derivable AND we're scoped to a class
	// IRI. Count the class (cheap, cached) and, above the threshold, render the
	// working-set shell (Tabulator loads a bounded set from
	// /api/async-table-data/:id). Everything else — attribute/relationship tables
	// (BIND(?? AS ?s), no key var) and small classes — renders inline as before.
	iri := c.Query("iri")
	keyVar := sparql.DeriveKeyVar(query)
	preprocessor := prepareQueryInputs(c)

	// Substitute the entity placeholder once, up front, so both the inline path
	// (which executes it) and the working-set shell (which embeds it for the
	// "Execute on endpoint" button) use the same full query. Queries without ??
	// are unaffected. The IRI is request input, so it is validated first — an
	// unvalidated one could close the <...> term and inject graph patterns.
	fullQuery := query
	if iri != "" {
		scoped, err := sparql.SubstituteEntity(query, iri)
		if err != nil {
			c.Header("Cache-Control", "no-store")
			c.String(http.StatusBadRequest, "invalid iri")
			return
		}
		fullQuery = scoped
	}

	if keyVar != "" && iri != "" {
		endpoint := activeEndpointURL(c)
		ctx, cancel := context.WithTimeout(c.Request.Context(), cfg.GetTimeout())
		searchProp := c.DefaultQuery("searchProp", "http://www.w3.org/2000/01/rdf-schema#label")
		total := cachedInstanceCount(ctx, preprocessor, id, endpoint, iri, keyVar, "", searchProp, acceptLanguage)
		cancel()
		if total > defaultMaxWorkingSet {
			params["workingSet"] = true
			params["iri"] = iri
			params["keyVar"] = keyVar
			// The fragment's Tabulator data fetches (/api/async-table-data) must
			// carry the endpoint slug explicitly so the shared cache keys them per
			// endpoint. This fragment is itself cached keyed by its own ?endpoint=,
			// so embedding the slug server-side is self-consistent.
			if ep := activeEndpoint(c); ep != nil {
				params["endpointSlug"] = ep.Slug
			}
			params["total"] = total
			params["complete"] = false
			params["searchProp"] = c.Query("searchProp") // author hint; may be empty
			params["max"] = c.Query("max")
			// Embed the full class query (finalized: PREFIXes added, ?? substituted)
			// so the card's "Execute on endpoint" button opens it in YASGUI, exactly
			// like an inline table. The working-set data itself is loaded separately
			// and lazily from /api/async-table-data/:id — this query is NOT executed
			// here, so query cost stays flat.
			params["result"] = sparql.QueryResult{
				Query:    preprocessor.FinalizeQuery(fullQuery, acceptLanguage),
				Endpoint: endpoint,
			}
			// Never executes the class query itself, so always cheap and safe to cache.
			renderTableFragment(c, params, true)
			return
		}
	}

	// Inline path: execute the full query and embed the whole result set.
	result, err := preprocessor.ExecuteQuery(fullQuery, true, acceptLanguage, "")
	if err != nil {
		// Surface the error inside the table card (sparqlTable renders result.Error).
		result.Error = err.Error()
	}
	params["result"] = result
	renderTableFragment(c, params, err == nil)
}

// renderTableFragment renders the sparqlTable partial and writes it as an HTML
// fragment, or a 500 on render failure. cacheable controls whether the caller
// wants this response cached (false for transient query errors, which must
// never be served stale for hours).
func renderTableFragment(c *gin.Context, params map[string]any, cacheable bool) {
	html, err := templates.RenderSparqlTable(activeLang(c), params)
	if err != nil {
		c.String(http.StatusInternalServerError, "failed to render table")
		return
	}
	if cacheable {
		markCacheable(c)
	} else {
		c.Header("Cache-Control", "no-store")
	}
	c.Header("Content-Type", "text/html")
	c.String(http.StatusOK, string(html))
}

// ---- Monitoring handlers ----

func monitoringPageHandler(c *gin.Context) {
	c.HTML(http.StatusOK, renderName(c, "pages/monitoring.html"), endpointTemplateData(c, gin.H{}))
}

func monitoringStatusHandler(c *gin.Context) {
	if mon == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "monitoring not available"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"enabled":   mon.IsEnabled(),
		"endpoints": mon.LatestStatus(),
	})
}

func monitoringToggleHandler(c *gin.Context) {
	if mon == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "monitoring not available"})
		return
	}
	mon.SetEnabled(!mon.IsEnabled())
	c.JSON(http.StatusOK, gin.H{"enabled": mon.IsEnabled()})
}

func monitoringDataHandler(c *gin.Context) {
	if mon == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "monitoring not available"})
		return
	}

	dur := parseDuration(c.DefaultQuery("range", "24h"))
	seriesMap, err := mon.QuerySeries(dur)
	if err != nil {
		log := logger.Get()
		log.Error("monitoring data query failed", slog.String("error", err.Error()))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "query failed"})
		return
	}

	// Build series in endpoint config order
	type point [2]int64 // [unix_ms, duration_ms]
	type series struct {
		Name string  `json:"name"`
		URL  string  `json:"url"`
		Data []point `json:"data"`
	}
	var out []series
	for _, ep := range cfg.Application.SparqlEndpoints {
		metrics := seriesMap[ep.URL]
		pts := make([]point, 0, len(metrics))
		for _, m := range metrics {
			pts = append(pts, point{m.MeasuredAt.UnixMilli(), m.DurationMs})
		}
		out = append(out, series{Name: ep.Name, URL: ep.URL, Data: pts})
	}
	c.JSON(http.StatusOK, gin.H{"series": out})
}

// ---- Main ----

func main() {

	// Load configuration
	var err error
	cfg, err = config.Load("visoto.config")

	// Initialize logger
	logger.MustInit(logger.Config{Level: cfg.Logging.Level, Format: cfg.Logging.Format, Output: cfg.Logging.Output})

	log := logger.Get()
	if err != nil {
		log.Warn("config file not found, using defaults",
			slog.String("error", err.Error()))
	} else {
		log.Info("config loaded successfully",
			slog.String("port", cfg.GetPort()),
			slog.String("endpoint", cfg.Application.SparqlEndpoint),
			slog.Duration("timeout", cfg.GetTimeout()),
			slog.Int("prefixes", len(cfg.RDF.ParsedPrefixes)),
			slog.Any("languages", cfg.Application.LanguageCodes()))
	}

	// Build the UI language set before anything that renders or resolves a
	// language. Config load already validated its shape.
	siteLangs = lang.New(configLanguages(cfg.Application.Languages), cfg.Application.DefaultLanguage)

	// Message catalogs must exist before templates are parsed: every template set
	// is cloned once per language with that language's translation functions.
	catalogs, err = i18n.Load("./locales", siteLangs.Codes())
	if err != nil {
		log.Error("failed to load message catalogs", slog.String("error", err.Error()))
		os.Exit(1)
	}
	if err := templates.InitPartials(catalogs, siteLangs); err != nil {
		log.Error("failed to initialize partial templates", slog.String("error", err.Error()))
		os.Exit(1)
	}

	// Validate Gemini API key
	if cfg.Application.GeminiAPIKey == "" {
		log.Warn("Gemini API key not configured - chat feature will not work",
			slog.String("config_key", "application.gemini_api_key"),
			slog.String("config_file", "visoto.config"))
	}

	// Initialize icon cache
	if err := resource.InitIconCache("./static/img/resource"); err != nil {
		log.Warn("failed to initialize icon cache",
			slog.String("error", err.Error()))
	}

	// Initialize endpoint monitor (data stored in ./data/)
	mon, err = monitor.New(&cfg.Application, "./data")
	if err != nil {
		log.Warn("failed to initialize endpoint monitor",
			slog.String("error", err.Error()))
	} else {
		mon.Start()
		log.Info("endpoint monitor started",
			slog.Bool("enabled", mon.IsEnabled()))
	}

	// Build MCP handler (fixed default endpoint, no per-request cookie logic)
	mcpPreprocessor := sparql.New(sparql.QueryInput{
		EndpointURL:     cfg.Application.SparqlEndpoint,
		Timeout:         cfg.GetTimeout(),
		Prefixes:        cfg.RDF.ParsedPrefixes,
		NamedEndpoints:  cfg.Application.GetNamedEndpointsMap(),
		MagicProperties: cfg.RDF.MagicProperties,
	})
	mcpHandler := mcpserver.NewServer(cfg, mcpPreprocessor)

	// Create router
	router := gin.Default()

	// The UI language applies to every rendered route, so unlike the endpoint
	// resolver this runs globally rather than per route class. It must precede
	// etagMiddleware, which stamps the resolved language into the ETag and picks
	// the Vary header from how that language arrived.
	router.Use(resolveLang())
	router.Use(etagMiddleware())

	// Load templates and register with Gin. Every page is registered once per
	// configured language; handlers name the variant via templates.Name.
	router.HTMLRender = templates.Load("./templates", catalogs, siteLangs)

	// Define routing rules
	router.StaticFile("/favicon.ico", "./static/img/favicon.svg")
	router.StaticFile("/robots.txt", "./static/robots.txt")
	router.Static("/static", "static")
	// Endpoint resolution per route class: routes cached by the shared Caddy/Souin
	// cache must be a pure function of the URL (no cookie); uncached entry pages
	// honor the selectedEndpoint cookie. See resolveEndpoint.
	epFromURL := resolveEndpoint(false)
	epFromURLOrCookie := resolveEndpoint(true)
	router.GET("/", epFromURLOrCookie, func(c *gin.Context) {
		c.Params = append(c.Params, gin.Param{Key: "page", Value: "home.html"})
		staticPageHandler(c)
	})
	router.GET("/ping", func(c *gin.Context) { c.String(http.StatusOK, "pong") })
	router.GET("/search", epFromURLOrCookie, searchHandler)
	mcpURL := fmt.Sprintf("http://localhost:%d/mcp", cfg.Application.Port)
	router.POST("/api/chat", chat.Handler(cfg.Application.GeminiAPIKey, mcpURL))
	router.POST("/api/upload", upload.UploadHandler(&cfg.Application))
	router.GET("/api/named-graphs", upload.NamedGraphsHandler(&cfg.Application))
	router.DELETE("/api/named-graphs", upload.DeleteNamedGraphHandler(&cfg.Application))
	router.GET("/api/export-graphs", upload.ExportNamedGraphsHandler(&cfg.Application))
	router.GET("/api/ontologies", upload.OntologiesHandler(&cfg.Application, cfg.Ontologies))
	router.GET("/monitoring", epFromURLOrCookie, monitoringPageHandler)
	router.GET("/api/monitoring/status", monitoringStatusHandler)
	router.POST("/api/monitoring/toggle", monitoringToggleHandler)
	router.GET("/api/monitoring/data", monitoringDataHandler)
	router.POST("/api/cache/purge", cachePurgeHandler)
	router.GET("/api/metric/:id", epFromURL, metricHandler)
	router.GET("/api/async-table/:id", epFromURL, asyncTableHandler)
	router.GET("/api/async-table-data/:id", epFromURL, asyncTableDataHandler)
	router.GET("/api/faceted-table/:id", epFromURL, facetedTableHandler)
	router.GET("/api/facet-values/:id/:var", epFromURL, facetValuesHandler)
	router.GET("/resource", epFromURL, resourcePageHandler)
	router.GET("/resource/*path", legacyResourceRedirectHandler)
	router.Any("/mcp", gin.WrapH(mcpHandler))
	router.GET("/health", gin.WrapH(mcpHandler))
	router.GET("/:page", epFromURLOrCookie, staticPageHandler)

	// Start server with configured port
	// Bind to 0.0.0.0 to allow connections from outside (required for Docker)
	log.Info("server starting",
		slog.String("address", "0.0.0.0"+cfg.GetPort()),
		slog.String("url", "http://localhost"+cfg.GetPort()),
		slog.String("mcp", "http://localhost"+cfg.GetPort()+"/mcp"))

	if err := router.Run("0.0.0.0" + cfg.GetPort()); err != nil {
		log.Error("server failed to start",
			slog.String("error", err.Error()))
		os.Exit(1)
	}
}
