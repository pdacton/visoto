// to run this code, use the command: go run cmd/visoto/main.go
// to build this code you must install gin-gonic package first using: go get github.com/gin-gonic/gin

package main

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
	"hutzli.org/visoto/internal/chat"
	"hutzli.org/visoto/internal/config"
	"hutzli.org/visoto/internal/logger"
	"hutzli.org/visoto/internal/resource"
	"hutzli.org/visoto/internal/search"
	"hutzli.org/visoto/internal/sparql"
	"hutzli.org/visoto/internal/templates"
)

// SparqlResponse represents the JSON response structure from a SPARQL endpoint
// the annotations are used by Unmarshal to map the JSON fields to Go struct fields
// uppercase field names are exported and accessible by the json package
type SparqlResponse struct {
	Head struct {
		Vars []string `json:"vars"`
	} `json:"head"`
	Results struct {
		Bindings []map[string]struct {
			Type  string `json:"type"`
			Value string `json:"value"`
		} `json:"bindings"`
	} `json:"results"`
}

// Package-level SPARQL preprocessor instance
var sparqlPreproc *sparql.Preprocessor

// Package-level config instance
var cfg *config.Config

// Package-level search instance
var searcher *search.Searcher

func homeHandler(c *gin.Context) {

	log := logger.Get()
	log.Debug("processing home request")

	// Set the page parameter to home.html
	c.Params = append(c.Params, gin.Param{Key: "page", Value: "home.html"})

	// Delegate to staticPageHandler
	staticPageHandler(c)
}

func resourceHandler(c *gin.Context) {

	// extract iri from path
	iri := strings.TrimPrefix(c.Param("path"), "/")

	// log request
	log := logger.Get()
	log.Debug("----> processing resource request",
		slog.String("path", c.Param("path")),
		slog.String("iri", iri))

	// create Resource instance
	r, err := resource.New(iri, cfg.RDF.ParsedPrefixes)
	if err != nil {
		log := logger.Get()
		log.Error("invalid resource IRI", slog.String("iri", iri), slog.String("error", err.Error()))
		c.String(http.StatusBadRequest, err.Error())
		return
	}

	// Get language preference from request
	acceptLanguage := c.GetHeader("Accept-Language")

	// Resolve template based on IRI and RDF types
	if err := r.ResolveTemplate(sparqlPreproc, cfg.RDF.TypePriority, cfg.RDF.ParsedPrefixes); err != nil {
		log.Error("template resolution failed",
			slog.String("iri", iri),
			slog.String("error", err.Error()))
		// Continue with fallback already set by ResolveTemplate
	}

	// TODO: refactor, move into Resource method
	// extract SPARQL query from template and retrieve data
	r.Data, err = sparqlPreproc.ProcessTemplateFile(r.TemplatePath, r.GetIRI(), acceptLanguage)
	if err != nil {
		c.String(http.StatusInternalServerError, "Preprocessing error: %v", err)
		return
	}

	// log.Debug("SPARQL data retrieved", slog.Any("data", r.Data))

	// native gin template rendering
	c.HTML(http.StatusOK, r.TemplateName, r.Data)
}

func staticPageHandler(c *gin.Context) {

	log := logger.Get()
	log.Debug("processing static page request")

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

	// Process SPARQL queries in template (WITHOUT IRI substitution)
	data, err := sparqlPreproc.ProcessTemplateFile(templatePath, "", acceptLanguage)
	if err != nil {
		log.Error("SPARQL processing failed",
			slog.String("page", pageName),
			slog.String("error", err.Error()))
		c.String(http.StatusInternalServerError, "Error processing page: %v", err)
		return
	}

	log.Debug("rendering static page",
		slog.String("templateName", templateName),
		slog.Int("queryResults", len(data.QueryResults)))

	// Render template
	c.HTML(http.StatusOK, templateName, data)
}

func main() {

	// Load configuration
	var err error
	cfg, err = config.Load("visoto.config")
	if err != nil {
		// Config loading failed, but cfg contains defaults
		fmt.Fprintf(os.Stderr, "WARNING: Failed to load config file, using defaults: %v\n", err)
	}

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

	// Initialize SPARQL preprocessor with config values
	sparqlPreproc = sparql.New(sparql.Config{
		EndpointURL: cfg.Application.SparqlEndpoint,
		Timeout:     cfg.GetTimeout(),
		Prefixes:    cfg.RDF.ParsedPrefixes,
	})

	// Initialize search with Stardog provider
	searcher = search.New(sparqlPreproc, "stardog")
	search.SetDefaultProvider("stardog")

	// Initialize icon cache
	if err := resource.InitIconCache("./static/img/resource"); err != nil {
		log.Warn("failed to initialize icon cache",
			slog.String("error", err.Error()))
	}

	// Create router
	router := gin.Default()

	// Load templates and register with Gin
	router.HTMLRender = templates.Load("./templates")

	// Define routing rules
	router.StaticFile("/favicon.ico", "./static/img/favicon.svg")
	router.StaticFile("/robots.txt", "./static/robots.txt")
	router.Static("/static", "static")
	router.GET("/", homeHandler)
	router.GET("/ping", func(c *gin.Context) { c.String(http.StatusOK, "pong") })
	router.GET("/search", searcher.Handler())
	router.POST("/api/chat", chat.Handler(cfg.Application.GeminiAPIKey))
	router.GET("/resource/*path", resourceHandler)
	router.GET("/:page", staticPageHandler)

	// Start server with configured port
	// Bind to 0.0.0.0 to allow connections from outside (required for Docker)
	log.Info("server starting",
		slog.String("address", "0.0.0.0"+cfg.GetPort()),
		slog.String("url", "http://localhost"+cfg.GetPort()))

	if err := router.Run("0.0.0.0" + cfg.GetPort()); err != nil {
		log.Error("server failed to start",
			slog.String("error", err.Error()))
		os.Exit(1)
	}
}
