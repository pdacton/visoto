package main

import (
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/gin-contrib/multitemplate"
	"github.com/gin-gonic/gin"
	"hutzli.org/visoto/internal/config"
	"hutzli.org/visoto/internal/sparql"
)

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

	fmt.Printf("path: %s\n", c.Param("path"))
	fmt.Printf("iri: %s\n", iri)

	// Get language preference from request
	acceptLanguage := c.GetHeader("Accept-Language")

	// extract SPARQL query from template and retrieve data
	data, err := sparqlPreproc.ProcessTemplateFile("templates/pages/embedded.html", iri, acceptLanguage)
	if err != nil {
		c.String(http.StatusInternalServerError, "Preprocessing error: %v", err)
		return
	}

	fmt.Printf("data: %v\n", data)

	// native gin template rendering
	c.HTML(http.StatusOK, "embedded.html", data)
}

// load templates and create template renderers for each page,
// consisting of all layouts (body, header, ..) and one specific page (home, resource, ...)
// this is necessary to use reuse the same layout templates (multiple definitions of pageContent)
// see gin-contrib/multitemplate documentation for details
func loadTemplates(templatesDir string) multitemplate.Renderer {
	r := multitemplate.NewRenderer()

	// compile list of layout templates
	layouts, err := filepath.Glob(templatesDir + "/layout/*.html")
	if err != nil {
		panic(err.Error())
	}
	// compile list of page templates
	pages, err := filepath.Glob(templatesDir + "/pages/*.html")
	if err != nil {
		panic(err.Error())
	}

	// Generate templates map: one template set for each page
	for _, page := range pages {
		layoutCopy := make([]string, len(layouts))
		copy(layoutCopy, layouts)
		files := append(layoutCopy, page)
		// AddFromFiles takes a name for the template set (e.g., "index.html") and the files to include
		r.AddFromFiles(filepath.Base(page), files...)
	}
	return r
}

func main() {

	// Load configuration
	cfg, err := config.Load("visoto.config")
	if err != nil {
		// Log warning but continue with defaults
		fmt.Printf("WARNING: Failed to load config file, using defaults: %v\n", err)
	} else {
		fmt.Println("Configuration loaded successfully")
		fmt.Printf("- Port: %s\n", cfg.GetPort())
		fmt.Printf("- SPARQL Endpoint: %s\n", cfg.Application.SparqlEndpoint)
		fmt.Printf("- Timeout: %s\n", cfg.GetTimeout())
		fmt.Printf("- Prefixes loaded: %d\n", len(cfg.RDF.ParsedPrefixes))
	}

	// setup the gin router and load templates
	router := gin.Default()
	router.HTMLRender = loadTemplates("./templates")

	// Initialize SPARQL preprocessor with config values
	sparqlPreproc = sparql.New(sparql.Config{
		EndpointURL: cfg.Application.SparqlEndpoint,
		Timeout:     cfg.GetTimeout(),
	})

	// define the routes
	router.StaticFile("/favicon.ico", "./static/img/favicon.svg")
	router.Static("/static", "static")
	router.GET("/", homeHandler)
	router.GET("/home", homeHandler)
	router.GET("/ping", func(c *gin.Context) { c.String(http.StatusOK, "pong") })
	router.GET("/ping", func(c *gin.Context) { c.String(http.StatusOK, "pong") })
	router.GET("/resource/*path", resourceHandler)
	router.GET("/embedded/*path", embeddedHandler)

	// fire up the web server with configured port
	router.Run(cfg.GetPort())

}
