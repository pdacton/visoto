package templates

// this package loads and registers Go HTML templates for use with the Gin multitemplate renderer.
// Each page template is combined with all shared layout and partial templates so they can be rendered by name via gin's c.HTML.

import (
	"log/slog"
	"path/filepath"

	"github.com/gin-contrib/multitemplate"
	"hutzli.org/visoto/internal/logger"
)

// Load loads Go HTML templates from templates/layout and templates/pages.
// Creates a multitemplate renderer where each page is combined with all layouts.
// This allows reusing layout templates with multiple page definitions.
// Panics if templates cannot be loaded (fail-fast approach).
func Load(templatesDir string) multitemplate.Renderer {

	r := multitemplate.NewRenderer()
	log := logger.Get()

	// Compile list of layout templates
	layouts, err := filepath.Glob(templatesDir + "/layout/*.html")
	if err != nil {
		panic(err.Error())
	}

	// Compile list of partial templates
	partials, err := filepath.Glob(templatesDir + "/partials/*.html")
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

	// Warn if any glob returns no results — likely a misconfigured path
	for _, group := range []struct {
		name  string
		files []string
	}{
		{"layouts", layouts},
		{"partials", partials},
		{"pages", pages},
	} {
		if len(group.files) == 0 {
			log.Warn("no templates found", slog.String("group", group.name), slog.String("dir", templatesDir))
		}
	}

	// Combine all template lists
	allTemplates := append(pages, classes...)
	allTemplates = append(allTemplates, instances...)

	// Generate templates map: one template set for each page.
	// Each page gets combined with all layouts and partials.
	for _, page := range allTemplates {
		// layoutCopy prevents the append below from overwriting the shared backing
		// array on subsequent iterations when len < cap.
		layoutCopy := make([]string, len(layouts))
		copy(layoutCopy, layouts)
		files := append(append(layoutCopy, partials...), page)
		// Use directory/filename as template name to avoid collisions between classes/ and instances/
		templateName := filepath.Join(filepath.Base(filepath.Dir(page)), filepath.Base(page))
		r.AddFromFilesFuncs(templateName, funcMap, files...)
	}

	log.Debug("templates loaded",
		slog.Int("layouts", len(layouts)),
		slog.Int("partials", len(partials)),
		slog.Int("pages", len(pages)),
		slog.Int("classes", len(classes)),
		slog.Int("instances", len(instances)))

	return r
}
