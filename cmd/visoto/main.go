// to run this code, use the command: go run cmd/visoto/main.go
// to build this code you must install gin-gonic package first using: go get github.com/gin-gonic/gin

package main

import (
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
	"hutzli.org/visoto/internal/config"
	"hutzli.org/visoto/internal/logger"
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

func homeHandler(c *gin.Context) {

	// native gin template rendering
	c.HTML(http.StatusOK, "home.html", "")
}

func resourceHandler(c *gin.Context) {

	// native gin template rendering
	c.HTML(http.StatusOK, "resource.html", "")
}

func embeddedHandler(c *gin.Context) {

	// extract iri from path & validate
	iri := strings.TrimPrefix(c.Param("path"), "/")
	if _, err := url.ParseRequestURI(iri); err != nil {
		c.String(http.StatusBadRequest, "incorrect resource iri: %v", iri)
		return
	}

	log := logger.Get()
	log.Debug("processing embedded request",
		slog.String("path", c.Param("path")),
		slog.String("iri", iri))

	// Get language preference from request
	acceptLanguage := c.GetHeader("Accept-Language")

	// extract SPARQL query from template and retrieve data
	data, err := sparqlPreproc.ProcessTemplateFile("templates/pages/embedded.html", iri, acceptLanguage)
	if err != nil {
		c.String(http.StatusInternalServerError, "Preprocessing error: %v", err)
		return
	}

	log.Debug("SPARQL data retrieved", slog.Any("data", data))

	// native gin template rendering
	c.HTML(http.StatusOK, "embedded.html", data)
}

func main() {

	// Load configuration
	cfg, err := config.Load("visoto.config")
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

	// Initialize SPARQL preprocessor with config values
	sparqlPreproc = sparql.New(sparql.Config{
		EndpointURL: cfg.Application.SparqlEndpoint,
		Timeout:     cfg.GetTimeout(),
	})

	// Create router
	router := gin.Default()

	// Load templates and register with Gin
	router.HTMLRender = templates.Load("./templates")

	// Define routing rules
	router.StaticFile("/favicon.ico", "./static/img/favicon.svg")
	router.Static("/static", "static")
	router.GET("/", homeHandler)
	router.GET("/home", homeHandler)
	router.GET("/ping", func(c *gin.Context) { c.String(http.StatusOK, "pong") })
	router.GET("/resource/*path", resourceHandler)
	router.GET("/embedded/*path", embeddedHandler)

	// Start server with configured port
	log.Info("server starting",
		slog.String("address", "127.0.0.1"+cfg.GetPort()),
		slog.String("url", "http://127.0.0.1"+cfg.GetPort()))

	if err := router.Run("127.0.0.1" + cfg.GetPort()); err != nil {
		log.Error("server failed to start",
			slog.String("error", err.Error()))
		os.Exit(1)
	}
}
