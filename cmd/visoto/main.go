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

// ---- Request helpers ----

// resolveSelectedEndpointName determines the selected endpoint name for this request.
// An explicit ?endpoint=<slug> query param takes precedence and refreshes the
// selectedEndpoint cookie to match, so shared links both work immediately and
// persist across subsequent navigation that drops the query string. Otherwise
// falls back to the selectedEndpoint cookie. Returns empty string if neither
// resolves to a configured endpoint.
func resolveSelectedEndpointName(c *gin.Context) string {
	if slug := c.Query("endpoint"); slug != "" {
		if ep := cfg.Application.GetEndpointBySlug(slug); ep != nil {
			// This helper runs several times per request (endpoint URL, tag, search
			// provider lookups); only the first call may emit the Set-Cookie header,
			// or the response carries duplicates.
			if _, alreadySet := c.Get("selectedEndpointCookieSet"); !alreadySet {
				c.Set("selectedEndpointCookieSet", true)
				// Write the Set-Cookie header directly rather than via c.SetCookie, which would
				// additionally url.QueryEscape the already-escaped value (double-encoding it) and
				// use '+' for spaces — a format the client-side decodeURIComponent (endpoint-switcher.js)
				// never converts back. url.PathEscape matches encodeURIComponent's %20 encoding instead.
				http.SetCookie(c.Writer, &http.Cookie{
					Name:     "selectedEndpoint",
					Value:    url.PathEscape(ep.Name),
					Path:     "/",
					MaxAge:   31536000,
					SameSite: http.SameSiteLaxMode,
				})
			}
			return ep.Name
		}
		logger.Get().Warn("unknown endpoint slug in URL", slog.String("slug", slug))
	}

	selected, err := c.Cookie("selectedEndpoint")
	if err != nil || selected == "" {
		return ""
	}
	// Decode the cookie value: browsers percent-encode non-ASCII characters (e.g. "Stadt Zürich" → "Stadt+Z%C3%BCrich")
	if decoded, err := url.QueryUnescape(selected); err == nil {
		return decoded
	}
	return selected
}

// resolveEndpointURL returns the SPARQL endpoint URL selected for this request
// (cookie override, else config default). Used to key per-endpoint caches so the
// endpoint switcher can't serve stale cross-endpoint data.
func resolveEndpointURL(c *gin.Context) string {
	endpoint := cfg.Application.SparqlEndpoint
	if name := resolveSelectedEndpointName(c); name != "" {
		if epURL, exists := cfg.Application.GetNamedEndpointsMap()[name]; exists {
			endpoint = epURL
		}
	}
	return endpoint
}

// prepareQueryInputs creates a preprocessor with user-selected default endpoint
func prepareQueryInputs(c *gin.Context) *parser.Preprocessor {

	// Determine default endpoint for this request either from cookie or config default
	namedEndpoints := cfg.Application.GetNamedEndpointsMap()
	endpoint := resolveEndpointURL(c)

	// Create query inputs for preprocessor
	return parser.New(sparql.QueryInput{
		EndpointURL:     endpoint,
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

	// Validate: must end with .html
	if !strings.HasSuffix(pageName, ".html") {
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

	// Add endpoints and resolved tag to template data
	data.SparqlEndpoints = cfg.Application.SparqlEndpoints
	selectedName := resolveSelectedEndpointName(c)
	data.EndpointTag = cfg.Application.ResolveEndpointTag(selectedName)
	// Expose the resolved endpoint URL so client-side pages (e.g. Graph Explorer)
	// query the same endpoint the user picked in the header.
	if ep := cfg.Application.GetEndpointByName(selectedName); ep != nil {
		data.EndpointURL = ep.URL
	} else {
		data.EndpointURL = cfg.Application.SparqlEndpoint
	}
	data.TemplateName = templateName

	log.Debug("rendering static page",
		slog.String("templateName", templateName),
		slog.Int("queryResults", len(data.QueryResults)))

	// Render template
	c.HTML(http.StatusOK, templateName, data)
}

func resourcePageHandler(c *gin.Context) {

	// extract iri from path
	iri := strings.TrimPrefix(c.Param("path"), "/")

	// Get language preference from request
	language := c.GetHeader("Accept-Language")

	// log request
	log := logger.Get()
	log.Debug("processing resource page request", slog.String("path", c.Param("path")), slog.String("iri", iri))

	// Create preprocessor with user-selected endpoint (inside cookie)
	preprocessor := prepareQueryInputs(c)

	// create Resource instance
	r, err := resource.New(iri, cfg.RDF.ParsedPrefixes)
	if err != nil {
		log.Error("invalid resource IRI", slog.String("iri", iri), slog.String("error", err.Error()))
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
		c.String(http.StatusInternalServerError, "Preprocessing error: %v", err)
		return
	}

	// Add endpoints and resolved tag to template data
	r.Data.SparqlEndpoints = cfg.Application.SparqlEndpoints
	selectedName := resolveSelectedEndpointName(c)
	r.Data.EndpointTag = cfg.Application.ResolveEndpointTag(selectedName)
	// Expose the resolved endpoint URL so client-side views (Graph, Schema) query
	// the same endpoint the user picked in the header.
	if ep := cfg.Application.GetEndpointByName(selectedName); ep != nil {
		r.Data.EndpointURL = ep.URL
	} else {
		r.Data.EndpointURL = cfg.Application.SparqlEndpoint
	}
	r.Data.ShortIRI = r.ShortIRI
	r.Data.TemplateName = r.TemplateName

	// native gin template rendering
	c.HTML(http.StatusOK, r.TemplateName, r.Data)
}

func searchHandler(c *gin.Context) {
	// Create preprocessor with user-selected endpoint
	preprocessor := prepareQueryInputs(c)

	// Resolve FTS provider from the active endpoint config; fall back to "stardog"
	providerName := "stardog"
	if ep := cfg.Application.GetEndpointByName(resolveSelectedEndpointName(c)); ep != nil && ep.SearchProvider != "" {
		providerName = ep.SearchProvider
	}
	searcher := search.New(preprocessor.SparqlPreprocessor(), providerName)

	// Parse search parameters
	params := search.ParseParams(c)

	// Validate required query parameter
	if params.Query == "" {
		c.HTML(http.StatusOK, "pages/search.html", gin.H{
			"ClassFilters":    search.GetClassFilters(),
			"PropertyFilters": search.GetPropertyFilters(),
			"SelectedLimit":   search.DefaultLimit,
			"Error":           "",
			"SparqlEndpoints": cfg.Application.SparqlEndpoints,
			"EndpointTag":     cfg.Application.ResolveEndpointTag(resolveSelectedEndpointName(c)),
		})
		return
	}

	// Execute search
	acceptLanguage := c.Request.Header.Get("Accept-Language")
	result := searcher.Execute(params, acceptLanguage)

	// Render results
	c.HTML(http.StatusOK, "pages/search.html", gin.H{
		"Query":            result.Query,
		"ClassFilters":     search.GetClassFilters(),
		"PropertyFilters":  search.GetPropertyFilters(),
		"SelectedClass":    params.Class,
		"SelectedProperty": params.Property,
		"SelectedLimit":    params.Limit,
		"SearchResults":    result.Results,
		"Provider":         result.Provider,
		"SparqlEndpoints":  cfg.Application.SparqlEndpoints,
		"EndpointTag":      cfg.Application.ResolveEndpointTag(resolveSelectedEndpointName(c)),
	})
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
func findAsyncQuery(id string) (string, bool) {
	for _, dir := range asyncQueryDirs {
		files, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, f := range files {
			if f.IsDir() || !strings.HasSuffix(f.Name(), ".html") {
				continue
			}
			content, err := os.ReadFile(dir + "/" + f.Name())
			if err != nil {
				continue
			}
			elements, err := parser.ExtractAsyncElements(string(content))
			if err != nil {
				continue
			}
			for _, el := range elements {
				if el.ID == id {
					return el.Content, true
				}
			}
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

	preprocessor := prepareQueryInputs(c)
	result, err := preprocessor.ExecuteQuery(query, false, acceptLanguage, "")

	count := "0"
	if err == nil && len(result.Bindings) > 0 {
		if b, ok := result.Bindings[0]["count"]; ok {
			count = b.Value
		}
	}

	c.Header("Content-Type", "text/html")
	c.String(http.StatusOK, count)
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
	// are unaffected.
	fullQuery := query
	if iri != "" {
		fullQuery = strings.ReplaceAll(query, "??", "<"+iri+">")
	}

	if keyVar != "" && iri != "" {
		endpoint := resolveEndpointURL(c)
		ctx, cancel := context.WithTimeout(c.Request.Context(), cfg.GetTimeout())
		searchProp := c.DefaultQuery("searchProp", "http://www.w3.org/2000/01/rdf-schema#label")
		total := cachedInstanceCount(ctx, preprocessor, id, endpoint, iri, keyVar, "", searchProp, acceptLanguage)
		cancel()
		if total > defaultMaxWorkingSet {
			params["workingSet"] = true
			params["iri"] = iri
			params["keyVar"] = keyVar
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
			renderTableFragment(c, params)
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
	renderTableFragment(c, params)
}

// renderTableFragment renders the sparqlTable partial and writes it as an HTML
// fragment, or a 500 on render failure.
func renderTableFragment(c *gin.Context, params map[string]any) {
	html, err := templates.RenderSparqlTable(params)
	if err != nil {
		c.String(http.StatusInternalServerError, "failed to render table")
		return
	}
	c.Header("Content-Type", "text/html")
	c.String(http.StatusOK, string(html))
}

// ---- Monitoring handlers ----

func monitoringPageHandler(c *gin.Context) {
	c.HTML(http.StatusOK, "pages/monitoring.html", gin.H{
		"SparqlEndpoints": cfg.Application.SparqlEndpoints,
		"EndpointTag":     cfg.Application.ResolveEndpointTag(resolveSelectedEndpointName(c)),
	})
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
			slog.Int("prefixes", len(cfg.RDF.ParsedPrefixes)))
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

	// Load templates and register with Gin
	router.HTMLRender = templates.Load("./templates")

	// Define routing rules
	router.StaticFile("/favicon.ico", "./static/img/favicon.svg")
	router.StaticFile("/robots.txt", "./static/robots.txt")
	router.Static("/static", "static")
	router.GET("/", func(c *gin.Context) {
		c.Params = append(c.Params, gin.Param{Key: "page", Value: "home.html"})
		staticPageHandler(c)
	})
	router.GET("/ping", func(c *gin.Context) { c.String(http.StatusOK, "pong") })
	router.GET("/search", searchHandler)
	mcpURL := fmt.Sprintf("http://localhost:%d/mcp", cfg.Application.Port)
	router.POST("/api/chat", chat.Handler(cfg.Application.GeminiAPIKey, mcpURL))
	router.POST("/api/upload", upload.UploadHandler(&cfg.Application))
	router.GET("/api/named-graphs", upload.NamedGraphsHandler(&cfg.Application))
	router.DELETE("/api/named-graphs", upload.DeleteNamedGraphHandler(&cfg.Application))
	router.GET("/api/export-graphs", upload.ExportNamedGraphsHandler(&cfg.Application))
	router.GET("/api/ontologies", upload.OntologiesHandler(&cfg.Application, cfg.Ontologies))
	router.GET("/monitoring", monitoringPageHandler)
	router.GET("/api/monitoring/status", monitoringStatusHandler)
	router.POST("/api/monitoring/toggle", monitoringToggleHandler)
	router.GET("/api/monitoring/data", monitoringDataHandler)
	router.GET("/api/metric/:id", metricHandler)
	router.GET("/api/async-table/:id", asyncTableHandler)
	router.GET("/api/async-table-data/:id", asyncTableDataHandler)
	router.GET("/resource/*path", resourcePageHandler)
	router.Any("/mcp", gin.WrapH(mcpHandler))
	router.GET("/health", gin.WrapH(mcpHandler))
	router.GET("/:page", staticPageHandler)

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
