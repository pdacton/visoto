package templates

import (
	"html/template"
	"log/slog"
	"path/filepath"

	"github.com/gin-contrib/multitemplate"
	"hutzli.org/visoto/internal/logger"
)

// TODO: take template paths as parameters from config file instead of hardcoded paths

// Package-level multitemplate renderer instance (singleton)
var renderer multitemplate.Renderer

// linkOrText returns an HTML link if the type is "uri", otherwise returns plain text
// binding should have Type, Value, and Lol fields
func linkOrText(binding interface{}) template.HTML {
	// The binding comes as a struct with Type, Value, and Lol fields
	type Binding struct {
		Type  string
		Value string
		Lol   string
	}

	// Try direct struct type assertion
	b, ok := binding.(struct {
		Type  string
		Value string
		Lol   string
	})
	if !ok {
		return ""
	}

	if b.Type == "uri" {
		return template.HTML(`<a href="/embedded/` + template.HTMLEscapeString(b.Value) + `">` + template.HTMLEscapeString(b.Lol) + `</a>`)
	}
	return template.HTML(template.HTMLEscapeString(b.Lol))
}

// MustLoad loads Go HTML templates from templates/layout and templates/pages
// Creates a multitemplate renderer where each page is combined with all layouts
// This allows reusing layout templates with multiple page definitions
// Exits the program if templates cannot be loaded (fail-fast approach)
func Load(templatesDir string) multitemplate.Renderer {
	r := multitemplate.NewRenderer()

	// Define custom template functions
	funcMap := template.FuncMap{
		"linkOrText": linkOrText,
	}

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

	// Generate templates map: one template set for each page
	// Each page gets combined with all layouts
	for _, page := range pages {
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
		slog.Int("pages", len(pages)))

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