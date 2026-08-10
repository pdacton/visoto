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

var (
	templateIncludeRe = regexp.MustCompile(`{{-?\s*template\s+"([^"]+)"`)
	templateDefineRe  = regexp.MustCompile(`{{-?\s*(?:define|block)\s+"([^"]+)"`)
)

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

// templateGroups holds the template files of each directory, the raw material
// every template set is assembled from.
type templateGroups struct {
	layouts    []string
	partials   []string
	components []string
	pages      []string
	classes    []string
	instances  []string
}

// globGroups collects the template files by directory. An absent directory
// yields an empty group rather than an error — only a malformed pattern fails,
// which cannot happen with the fixed patterns below but is checked anyway.
func globGroups(templatesDir string) (templateGroups, error) {
	var g templateGroups
	for _, spec := range []struct {
		dir string
		out *[]string
	}{
		{"layout", &g.layouts},
		{"partials", &g.partials},
		{"components", &g.components},
		{"pages", &g.pages},
		{"classes", &g.classes},
		{"instances", &g.instances},
	} {
		files, err := filepath.Glob(filepath.Join(templatesDir, spec.dir, "*.html"))
		if err != nil {
			return templateGroups{}, fmt.Errorf("glob %s: %w", spec.dir, err)
		}
		*spec.out = files
	}
	return g, nil
}

// entries returns the page-like templates — the ones that get a set each.
func (g templateGroups) entries() []string {
	all := make([]string, 0, len(g.pages)+len(g.classes)+len(g.instances))
	all = append(all, g.pages...)
	all = append(all, g.classes...)
	all = append(all, g.instances...)
	return all
}

// files returns the parse-order file list of the set entered through page.
func (g templateGroups) files(page string) []string {
	// A fresh slice per set: appending onto the shared layouts backing array
	// would overwrite the previous set's entries whenever len < cap.
	files := make([]string, 0, len(g.layouts)+len(g.partials)+len(g.components)+1)
	files = append(files, g.layouts...)
	files = append(files, g.partials...)
	files = append(files, referencedComponents(page, g.components)...)
	return append(files, page)
}

// setFiles maps every set name to its parse-order file list.
func (g templateGroups) setFiles() map[string][]string {
	sets := make(map[string][]string, len(g.pages)+len(g.classes)+len(g.instances))
	for _, page := range g.entries() {
		sets[SetName(page)] = g.files(page)
	}
	return sets
}

// SetName returns the name a page-like template's set is registered under.
// Directory and filename, so classes/ and instances/ variants of the same type
// do not collide.
func SetName(page string) string {
	return filepath.Join(filepath.Base(filepath.Dir(page)), filepath.Base(page))
}

// SetFiles maps each template set's name to the files it is parsed from, in
// parse order: every layout and partial, the components the page references,
// and the page itself.
//
// This is the single authoritative answer to "which files are in scope for this
// page". Load parses each list into a template set, and the async query index in
// cmd/visoto extracts <sparql-async> and <sparql-facet> declarations from the very
// same list — so an async query is reachable from exactly the pages whose markup
// could reference it, and the two namespaces cannot drift apart. Anything that
// needs this grouping must call here rather than deriving it again.
func SetFiles(templatesDir string) (map[string][]string, error) {
	g, err := globGroups(templatesDir)
	if err != nil {
		return nil, err
	}
	return g.setFiles(), nil
}

// ValidateIncludes reports the first {{ template "name" }} in sets that names a
// template its own set does not parse. Such an include is legal Go template
// syntax and survives parsing — it only fails when the page is executed, as a
// 500 on whichever route happens to render it first.
//
// The check exists because referencedComponents is deliberately shallow: it
// scans the page file alone, so a component that included another component
// would leave that second component out of the set. Nothing does this today, and
// this is what keeps it that way.
//
// It takes the mapping rather than a directory so the caller can validate the
// exact grouping it is about to use (see SetFiles).
func ValidateIncludes(sets map[string][]string) error {
	type fileRefs struct{ defines, includes []string }

	// One read per file, not one per (file, set): every layout and partial
	// appears in all 73 sets.
	seen := make(map[string]fileRefs)
	names := func(re *regexp.Regexp, data []byte) []string {
		var out []string
		for _, m := range re.FindAllSubmatch(data, -1) {
			out = append(out, string(m[1]))
		}
		return out
	}
	for _, files := range sets {
		for _, path := range files {
			if _, done := seen[path]; done {
				continue
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return fmt.Errorf("read %s: %w", path, err)
			}
			seen[path] = fileRefs{defines: names(templateDefineRe, data), includes: names(templateIncludeRe, data)}
		}
	}

	for set, files := range sets {
		defined := make(map[string]bool, len(files)*2)
		for _, path := range files {
			// ParseFiles registers every file under its base name — that is how
			// base.html reaches {{ template "topbar.html" }}.
			defined[filepath.Base(path)] = true
			for _, name := range seen[path].defines {
				defined[name] = true
			}
		}
		for _, path := range files {
			for _, name := range seen[path].includes {
				if !defined[name] {
					return fmt.Errorf("%s: {{ template %q }} is not defined in template set %s", path, name, set)
				}
			}
		}
	}
	return nil
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

	g, err := globGroups(templatesDir)
	if err != nil {
		panic(err.Error())
	}

	// Warn if any glob returns no results — likely a misconfigured path
	for _, group := range []struct {
		name  string
		files []string
	}{
		{"layouts", g.layouts},
		{"partials", g.partials},
		{"pages", g.pages},
	} {
		if len(group.files) == 0 {
			log.Warn("no templates found", slog.String("group", group.name), slog.String("dir", templatesDir))
		}
	}

	// Generate templates map: one template set for each page × language.
	// Each page gets combined with all layouts and partials.
	for _, page := range g.entries() {
		templateName := SetName(page)

		base, err := parseSet(g.files(page))
		if err != nil {
			panic("parse template set " + templateName + ": " + err.Error())
		}

		// Bound per set rather than per language: the set name is what the async
		// query index keys on, and it is the same across a set's language variants.
		// Overriding after parse works because html/template resolves functions at
		// execute time, and Clone carries the func map into every variant — so the
		// value cannot be wrong, it is fixed when the set is registered.
		base = base.Funcs(template.FuncMap{"templateSet": func() string { return templateName }})

		for _, code := range codes {
			variant, err := base.Clone()
			if err != nil {
				panic("clone template set " + templateName + ": " + err.Error())
			}
			r.Add(Name(code, templateName), variant.Funcs(i18nFuncs(cats, langs, code)))
		}
	}

	log.Debug("templates loaded",
		slog.Int("layouts", len(g.layouts)),
		slog.Int("partials", len(g.partials)),
		slog.Int("pages", len(g.pages)),
		slog.Int("classes", len(g.classes)),
		slog.Int("instances", len(g.instances)),
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
