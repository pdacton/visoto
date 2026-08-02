package templates

// this package loads and registers Go HTML templates for use with the Gin multitemplate renderer.
// Each page template is combined with all shared layout and partial templates so they can be rendered by name via gin's c.HTML.
//
// Every page set is registered once per configured UI language, under a
// language-qualified name ("de:pages/home.html" — see Name). Template functions
// bind at parse time, so {{ t "key" }} can only work if the function it calls
// already knows the language; registering a variant per language is what lets
// templates stay free of language plumbing. The file grouping below is shared by
// all variants: each set is parsed once and then cloned per language with only
// its i18n functions overridden, so the extra languages cost clones, not parses.

import (
	"fmt"
	"html/template"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"

	"github.com/gin-contrib/multitemplate"
	"hutzli.org/visoto/internal/i18n"
	"hutzli.org/visoto/internal/lang"
	"hutzli.org/visoto/internal/logger"
)

// Name returns the registered template name for a page in a given language.
// Handlers pass the result to c.HTML; nothing else should build these strings.
func Name(code, templateName string) string {
	return lang.Key(code) + ":" + templateName
}

var templateIncludeRe = regexp.MustCompile(`{{\s*template\s+"([^"]+)"`)

// referencedComponents returns the subset of componentFiles that are referenced
// by {{ template "name" }} in the given page file.
func referencedComponents(pageFile string, componentFiles []string) []string {
	if len(componentFiles) == 0 {
		return nil
	}
	data, err := os.ReadFile(pageFile)
	if err != nil {
		return nil
	}
	matches := templateIncludeRe.FindAllSubmatch(data, -1)
	referenced := make(map[string]bool, len(matches))
	for _, m := range matches {
		referenced[string(m[1])] = true
	}
	var result []string
	for _, cf := range componentFiles {
		name := filepath.Base(cf)
		name = name[:len(name)-len(filepath.Ext(name))] // strip .html
		if referenced[name] {
			result = append(result, cf)
		}
	}
	return result
}

// Load loads Go HTML templates from templates/layout and templates/pages.
// Creates a multitemplate renderer where each page is combined with all layouts.
// This allows reusing layout templates with multiple page definitions.
// Each page is registered once per code in codes; render it via Name(code, …).
// Panics if templates cannot be loaded (fail-fast approach).
func Load(templatesDir string, cats *i18n.Catalogs, langs *lang.Set) multitemplate.Renderer {

	r := multitemplate.NewRenderer()
	log := logger.Get()

	codes := langs.Codes()
	if len(codes) == 0 {
		codes = []string{i18n.BaseCode}
	}

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

	// Compile list of component templates (optional — no panic if directory is absent)
	components, err := filepath.Glob(templatesDir + "/components/*.html")
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

	// Generate templates map: one template set for each page × language.
	// Each page gets combined with all layouts and partials.
	for _, page := range allTemplates {
		// layoutCopy prevents the append below from overwriting the shared backing
		// array on subsequent iterations when len < cap.
		layoutCopy := make([]string, len(layouts))
		copy(layoutCopy, layouts)
		files := append(append(append(layoutCopy, partials...), referencedComponents(page, components)...), page)
		// Use directory/filename as template name to avoid collisions between classes/ and instances/
		templateName := filepath.Join(filepath.Base(filepath.Dir(page)), filepath.Base(page))

		base, err := parseSet(files)
		if err != nil {
			panic("parse template set " + templateName + ": " + err.Error())
		}

		for _, code := range codes {
			variant, err := base.Clone()
			if err != nil {
				panic("clone template set " + templateName + ": " + err.Error())
			}
			r.Add(Name(code, templateName), variant.Funcs(i18nFuncs(cats, langs, code)))
		}
	}

	log.Debug("templates loaded",
		slog.Int("layouts", len(layouts)),
		slog.Int("partials", len(partials)),
		slog.Int("pages", len(pages)),
		slog.Int("classes", len(classes)),
		slog.Int("instances", len(instances)),
		slog.Int("languages", len(codes)))

	return r
}

// i18nFuncs is the language-bound half of a template's function map: the
// message lookups from the catalogs, plus the picker data from the language set.
// The split follows ownership — internal/i18n owns translated strings, and
// internal/lang owns the configured language set and its display labels, which
// come from visoto.config rather than from a catalog.
func i18nFuncs(cats *i18n.Catalogs, langs *lang.Set, code string) template.FuncMap {
	fm := cats.FuncMap(code)
	fm["siteLanguages"] = func() []lang.Option { return langs.Options(code) }
	return fm
}

// parseSet parses one page's file group into a template set with placeholder
// i18n functions, ready to be cloned per language.
//
// The set's own name must be filepath.Base(files[0]) — "base.html", the first
// layout — because multitemplate's Instance renders with Execute, which runs the
// template of that name. Clone preserves it, so every language variant still
// enters through base.html.
func parseSet(files []string) (*template.Template, error) {
	return template.New(filepath.Base(files[0])).
		Funcs(funcMap).
		Funcs(placeholderI18nFuncs()).
		ParseFiles(files...)
}

// placeholderI18nFuncs satisfies the parser for {{ t }}, {{ tHTML }}, {{ tn }}
// and {{ siteLang }}. The signatures must match Catalogs.FuncMap exactly, since
// html/template type-checks calls against whatever is registered at parse time
// even though the clone replaces the implementations before anything executes.
// If one of these ever fires, a template set was registered without a language.
//
// They return the call site's inline fallback when it has one, so that even in
// that broken case a page renders its English text rather than bare keys.
func placeholderI18nFuncs() template.FuncMap {
	return template.FuncMap{
		"t":     func(id string, args ...any) string { return placeholderText(id, args) },
		"tHTML": func(id string, args ...any) template.HTML { return template.HTML(placeholderText(id, args)) },
		"tn":    func(id string, _ any, args ...any) string { return placeholderText(id, args) },

		// Must return the value unchanged rather than "": if this ever fires, a
		// count should still render, just ungrouped.
		"formatNum":     func(v any) string { return fmt.Sprint(v) },
		"siteLang":      func() string { return i18n.BaseCode },
		"siteLanguages": func() []lang.Option { return nil },
		"jsStrings":     func() map[string]string { return nil },
	}
}

// placeholderText mirrors i18n's argument convention — a string argument is the
// inline fallback — without reaching for a catalog there is none of.
func placeholderText(id string, args []any) string {
	for _, a := range args {
		if s, ok := a.(string); ok {
			return s
		}
	}
	return id
}
