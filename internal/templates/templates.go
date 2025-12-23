package templates

// TODO: change embedded link prefix from "/embedded/" to "/resource/" in renderBinding function

import (
	"log/slog"
	"path/filepath"

	"github.com/gin-contrib/multitemplate"
	"hutzli.org/visoto/internal/logger"
)

// TODO: take template paths as parameters from config file instead of hardcoded paths

// Package-level multitemplate renderer instance (singleton)
var renderer multitemplate.Renderer

// MustLoad loads Go HTML templates from templates/layout and templates/pages
// Creates a multitemplate renderer where each page is combined with all layouts
// This allows reusing layout templates with multiple page definitions
// Exits the program if templates cannot be loaded (fail-fast approach)
func Load(templatesDir string) multitemplate.Renderer {
	r := multitemplate.NewRenderer()

	// Compile list of layout templates
	layouts, err := filepath.Glob(templatesDir + "/layout/*.html")
	if err != nil {
		panic(err.Error())
	}

	// Compile list of page templates
	pages, err := filepath.Glob(templatesDir + "/pages/*.html")
	if err != nil {
		panic(err.Error())
	}

	// Compile list of class templates
	classes, err := filepath.Glob(templatesDir + "/classes/*.html")
	if err != nil {
		panic(err.Error())
	}

	// Compile list of instance templates
	instances, err := filepath.Glob(templatesDir + "/instances/*.html")
	if err != nil {
		panic(err.Error())
	}

	// Combine all template lists
	allTemplates := append(pages, classes...)
	allTemplates = append(allTemplates, instances...)

	// Generate templates map: one template set for each page
	// Each page gets combined with all layouts
	for _, page := range allTemplates {
		layoutCopy := make([]string, len(layouts))
		copy(layoutCopy, layouts)
		files := append(layoutCopy, page)
		// AddFromFilesFuncs takes a name, funcMap, and files to include
		r.AddFromFilesFuncs(filepath.Base(page), funcMap, files...)
	}

	// Store in singleton
	renderer = r

	// Log successful loading
	log := logger.Get()
	log.Debug("templates loaded",
		slog.Int("layouts", len(layouts)),
		slog.Int("pages", len(pages)),
		slog.Int("classes", len(classes)),
		slog.Int("instances", len(instances)))

	return r
}

// Get returns the default multitemplate renderer instance
// If not initialized, panics (must call MustLoad first)
func Get() multitemplate.Renderer {
	if renderer == nil {
		panic("templates not loaded - call MustLoad first")
	}
	return renderer
}
