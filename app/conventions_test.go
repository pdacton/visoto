package app

// Project conventions, enforced. Each test here replaces a rule that used to
// live only in prose (CLAUDE.md, a code comment, someone's memory) and had
// already been broken at least once without anything noticing.
//
// The tests read the source tree, not the running server: they are cheap, need
// no endpoint, and fail with the file and line to fix.

import (
	"bytes"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/pdacton/visoto/internal/config"
)

// repoRoot is the repository root, relative to this package's test directory.
const repoRoot = ".."

// sourceFiles returns the files under root (relative to the repo root) with one
// of the extensions, skipping minified bundles.
func sourceFiles(t *testing.T, root string, exts ...string) []string {
	t.Helper()
	var out []string
	err := filepath.WalkDir(filepath.Join(repoRoot, root), func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		if strings.Contains(d.Name(), ".min.") {
			return nil
		}
		for _, ext := range exts {
			if strings.HasSuffix(path, ext) {
				out = append(out, path)
				break
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) == 0 {
		t.Fatalf("no %v files under %s — has the directory moved?", exts, root)
	}
	sort.Strings(out)
	return out
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

// lineOf is the 1-based line of byte offset off in body.
func lineOf(body string, off int) int {
	return strings.Count(body[:off], "\n") + 1
}

// rel shortens a path to the form it has from the repo root.
func rel(path string) string {
	return filepath.ToSlash(strings.TrimPrefix(path, repoRoot+string(filepath.Separator)))
}

// jsComment matches a line that is only a comment, so prose naming a pattern is
// not mistaken for code using it.
var jsComment = regexp.MustCompile(`^\s*(//|/?\*)`)

// Comments are blanked rather than removed, so line numbers survive.
var (
	// A block comment that starts its own line. One opening mid-line is left
	// alone: "/*" also occurs inside strings ('/api/*').
	jsBlockComment = regexp.MustCompile(`(?ms)^[ \t]*/\*.*?\*/`)
	htmlComment    = regexp.MustCompile(`(?s)<!--.*?-->`)
)

func blank(body string, comments ...*regexp.Regexp) string {
	for _, re := range comments {
		body = re.ReplaceAllStringFunc(body, func(c string) string {
			return strings.Map(func(r rune) rune {
				if r == '\n' {
					return r
				}
				return ' '
			}, c)
		})
	}
	return body
}

// codeMatches returns "file:line" for every match of re outside a comment.
func codeMatches(t *testing.T, path string, re *regexp.Regexp) []string {
	t.Helper()
	var out []string
	for i, line := range strings.Split(blank(readFile(t, path), jsBlockComment), "\n") {
		if jsComment.MatchString(line) {
			continue
		}
		for range re.FindAllStringIndex(line, -1) {
			out = append(out, fmt.Sprintf("%s:%d", rel(path), i+1))
		}
	}
	return out
}

// A NUL or other control byte makes grep, ripgrep and `file` treat a source file
// as binary and skip it — silently. static/js/sparql-lazy-tree.js carried a
// literal NUL inside a string for months, so every search for its callers came
// back empty. Write the escape (\u0000) instead of the byte.
func TestSourceFilesAreText(t *testing.T) {
	var files []string
	files = append(files, sourceFiles(t, "templates", ".html")...)
	files = append(files, sourceFiles(t, "static/js", ".js")...)
	files = append(files, sourceFiles(t, "static/css", ".css")...)
	files = append(files, sourceFiles(t, "locales", ".toml")...)
	files = append(files, sourceFiles(t, "tests", ".js")...)
	for _, dir := range []string{"app", "internal", "cmd"} {
		files = append(files, sourceFiles(t, dir, ".go")...)
	}
	for _, path := range files {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for i, b := range body {
			if b < 0x20 && b != '\n' && b != '\r' && b != '\t' {
				t.Errorf("%s:%d: control byte 0x%02x — tools read the file as binary and skip it; write it as an escape",
					rel(path), bytes.Count(body[:i], []byte("\n"))+1, b)
				break
			}
		}
	}
}

// The endpoint slug, the language and the template set reach /api requests from
// exactly one place, static/js/api-url.js, whose htmx:configRequest hook has no
// route list. Three per-file hooks with their own path allowlists preceded it,
// and /api/cube-table was in none of them. A second listener would bring the
// lists back.
func TestSingleHTMXRequestHook(t *testing.T) {
	listener := regexp.MustCompile(`htmx:configRequest['"]\s*,`)
	var found []string
	for _, path := range sourceFiles(t, "static/js", ".js") {
		found = append(found, codeMatches(t, path, listener)...)
	}
	if len(found) != 1 || !strings.HasPrefix(found[0], "static/js/api-url.js:") {
		t.Errorf("htmx:configRequest listeners: %v — want exactly one, in static/js/api-url.js "+
			"(add the param to its defaults() rather than registering another hook)", found)
	}
}

// Every literal /api path written in a template or a translated string is a
// registered route. A mistyped hx-get fails only at runtime, as a card that
// flashes and disappears.
func TestTemplateAPIPathsAreRoutes(t *testing.T) {
	_, table := routedServer(t)
	var routes []*regexp.Regexp
	for _, r := range table.routes {
		pattern := regexp.QuoteMeta(r.path)
		pattern = regexp.MustCompile(`:[A-Za-z_]+`).ReplaceAllString(pattern, `[^/]+`)
		pattern = regexp.MustCompile(`\\\*[A-Za-z_]+`).ReplaceAllString(pattern, `.*`)
		routes = append(routes, regexp.MustCompile("^"+pattern+"$"))
	}

	// A quoted /api/… literal, up to its query string. A {{ … }} action stands
	// in for one path segment's worth of text.
	literal := regexp.MustCompile(`["'](/api/(?:[^"'?\\{\s]|\{\{[^}]*\}\})*)`)
	action := regexp.MustCompile(`\{\{[^}]*\}\}`)

	files := append(sourceFiles(t, "templates", ".html"), sourceFiles(t, "locales", ".toml")...)
	seen := 0
	for _, path := range files {
		body := readFile(t, path)
		for _, m := range literal.FindAllStringSubmatchIndex(body, -1) {
			seen++
			apiPath := action.ReplaceAllString(body[m[2]:m[3]], "x")
			matched := false
			for _, re := range routes {
				if re.MatchString(apiPath) {
					matched = true
					break
				}
			}
			if !matched {
				t.Errorf("%s:%d: %q matches no registered route", rel(path), lineOf(body, m[2]), body[m[2]:m[3]])
			}
		}
	}
	if seen == 0 {
		t.Fatal("found no /api paths in the templates — the pattern no longer matches how they are written")
	}
}

// A link to a resource page whose IRI comes from data is built by
// sparql.ResourceHref (Go, and `resourceHref` in templates) or
// visotoResourceHref (JS) — the two places that know the URL scheme and its
// escaping. A link to one fixed, known resource may be written out
// (href="/resource?iri=schema:Person"): prose and translated strings cannot call
// a function.
func TestResourceLinksUseTheBuilders(t *testing.T) {
	inTemplate := regexp.MustCompile(`/resource\?iri=[^"'<>\s\\]*\{\{`)
	for _, path := range append(sourceFiles(t, "templates", ".html"), sourceFiles(t, "locales", ".toml")...) {
		body := readFile(t, path)
		for _, m := range inTemplate.FindAllStringIndex(body, -1) {
			t.Errorf(`%s:%d: resource link built from a template value — use href="{{ resourceHref <iri> }}"`,
				rel(path), lineOf(body, m[0]))
		}
	}

	inJS := regexp.MustCompile(`/resource\?iri=['"]\s*\+`)
	for _, path := range sourceFiles(t, "static/js", ".js") {
		if rel(path) == "static/js/api-url.js" {
			continue
		}
		for _, at := range codeMatches(t, path, inJS) {
			t.Errorf("%s: resource link concatenated by hand — use visotoResourceHref(iri)", at)
		}
	}

	inGo := regexp.MustCompile(`"/resource\?iri="`)
	for _, dir := range []string{"app", "internal", "cmd"} {
		for _, path := range sourceFiles(t, dir, ".go") {
			if strings.HasSuffix(path, "_test.go") || rel(path) == "internal/sparql/types.go" {
				continue
			}
			for _, at := range codeMatches(t, path, inGo) {
				t.Errorf("%s: resource link concatenated by hand — use sparql.ResourceHref(iri)", at)
			}
		}
	}
}

// Templates emit markup and <template>/JSON config islands; behaviour lives in
// static/js. The entries below are the deliberate exceptions, each explained
// where it stands: they must run before the first paint.
func TestNoInlineScriptsInTemplates(t *testing.T) {
	allowed := map[string]int{
		"templates/layout/base.html": 2, // theme and split-layout head start
	}
	script := regexp.MustCompile(`(?i)<script\b([^>]*)>`)
	handler := regexp.MustCompile(`(?i)\son(click|dblclick|change|input|submit|load|error|focus|blur|key(up|down|press)|mouse(over|out|down|up|enter|leave))\s*=`)

	for _, path := range sourceFiles(t, "templates", ".html") {
		body := blank(readFile(t, path), htmlComment, goTemplateComment)
		inline := 0
		for _, m := range script.FindAllStringSubmatchIndex(body, -1) {
			attrs := body[m[2]:m[3]]
			if strings.Contains(attrs, "src=") || strings.Contains(attrs, "application/json") ||
				strings.Contains(attrs, "application/ld+json") {
				continue
			}
			inline++
			if inline > allowed[rel(path)] {
				t.Errorf("%s:%d: inline <script> — move it to static/js, keyed off a data-<name> marker",
					rel(path), lineOf(body, m[0]))
			}
		}
		if inline < allowed[rel(path)] {
			t.Errorf("%s: %d inline <script> blocks, %d allowed — lower the allowance in this test",
				rel(path), inline, allowed[rel(path)])
		}
		for _, m := range handler.FindAllStringIndex(body, -1) {
			t.Errorf("%s:%d: inline event handler attribute — bind it in static/js instead", rel(path), lineOf(body, m[0]))
		}
	}
}

// JS writes a vocabulary term as a CURIE and expands it through
// VisotoPrefixes.fromPage(), which carries the active endpoint's prefixes — an
// endpoint may redeclare one (schema: is http:// on some stores and https:// on
// others), and a hard-coded namespace then matches nothing there.
//
// The files below predate the rule and are frozen at their current count: the
// number may only go down. A file not listed may hard-code none.
func TestJSHardCodedNamespacesOnlyShrink(t *testing.T) {
	grandfathered := map[string]int{
		"static/js/graph-memory-store.js":  6,
		"static/js/graph-turtle.js":        3,
		"static/js/ontology-projection.js": 15,
		"static/js/schema-graph.js":        3,
		"static/js/sparql-graph.js":        5,
		// Not a violation, the pattern to copy: each term is a CURIE expanded
		// through the page prefixes, and the IRI is only the fallback for an
		// endpoint that does not declare the prefix.
		"static/js/sparql-map.js": 4,
	}
	// Namespace IRIs of RDF vocabularies. The W3C document namespaces that are
	// not vocabularies (SVG, XHTML, MathML, XML) are not matched.
	namespace := regexp.MustCompile(`https?://(www\.w3\.org/(1999/02/22-rdf-syntax-ns|2000/01/rdf-schema|2001/XMLSchema|2002/07/owl|2004/02/skos|ns/)|schema\.org/|purl\.org/|xmlns\.com/|www\.opengis\.net/)`)

	for _, path := range sourceFiles(t, "static/js", ".js") {
		got, want := len(codeMatches(t, path, namespace)), grandfathered[rel(path)]
		switch {
		case got > want:
			t.Errorf("%s: %d hard-coded vocabulary namespaces, %d allowed — write the term as a CURIE and expand it with VisotoPrefixes.fromPage().expand()",
				rel(path), got, want)
		case got < want:
			t.Errorf("%s: %d hard-coded vocabulary namespaces, %d allowed — lower the allowance in this test to %d",
				rel(path), got, want, got)
		}
	}
}

// Graph Explorer's SPARQL goes through the /api/sparql proxy and must be a
// POST: a GET puts the whole query in the URL, and long link queries exceed the
// ~8 KB limit and lose every edge without an error. Raw fetch()es to the proxy
// cannot regress this way — the route is registered for POST only — so only the
// provider's own method setting needs holding.
func TestGraphExplorerQueriesStayPOST(t *testing.T) {
	construct := regexp.MustCompile(`new\s+[A-Za-z_.]*SparqlDataProvider\(`)
	seen := 0
	for _, path := range sourceFiles(t, "static/js", ".js") {
		body := readFile(t, path)
		if strings.Contains(body, "SparqlQueryMethod.GET") {
			t.Errorf("%s: SparqlQueryMethod.GET — Graph Explorer queries must stay POST", rel(path))
		}
		for _, m := range construct.FindAllStringIndex(body, -1) {
			seen++
			window := body[m[0]:min(len(body), m[0]+400)]
			if !strings.Contains(window, "SparqlQueryMethod.POST") {
				t.Errorf("%s:%d: SparqlDataProvider constructed without queryMethod: SparqlQueryMethod.POST (the library default is GET)",
					rel(path), lineOf(body, m[0]))
			}
		}
	}
	if seen == 0 {
		t.Fatal("found no SparqlDataProvider construction — the pattern no longer matches")
	}
}

// An IRI is written into a query as a <…> term in exactly one place:
// sparql.IRI.Term (reached through ParseIRI, IRITerm or MustIRI), which only
// ever holds a validated IRI. Splicing a string between angle brackets anywhere
// else is how request input closes the term early and injects graph patterns —
// /search did exactly that with its ?class= and ?property= until this test
// existed.
//
// The entries below write angle brackets for something that is not a query.
func TestNoRawIRISplices(t *testing.T) {
	allowed := map[string]int{
		"internal/sparql/terms.go":        1, // IRI.Term itself
		"internal/config/config.go":       3, // parses PREFIX declarations out of visoto.config
		"internal/bnodetree/serialize.go": 5, // writes N-Triples and Turtle downloads, from result bindings
		"internal/parser/tree.go":         1, // an error message naming an element
	}
	splice := regexp.MustCompile("\"<\" ?\\+|\\+ ?\">\"|<%s>|<%v>|<%q>")

	for _, dir := range []string{"app", "internal", "cmd"} {
		for _, path := range sourceFiles(t, dir, ".go") {
			if strings.HasSuffix(path, "_test.go") {
				continue
			}
			var lines []int
			for i, line := range strings.Split(readFile(t, path), "\n") {
				if !jsComment.MatchString(line) && splice.MatchString(line) {
					lines = append(lines, i+1)
				}
			}
			want := allowed[rel(path)]
			switch {
			case len(lines) > want:
				t.Errorf("%s: lines %v splice a string between angle brackets (%d allowed) — "+
					"take a sparql.IRI and write iri.Term(), or use sparql.IRITerm(s)", rel(path), lines, want)
			case len(lines) < want:
				t.Errorf("%s: %d angle-bracket splices, %d allowed — lower the allowance in this test to %d",
					rel(path), len(lines), want, len(lines))
			}
		}
	}
}

// A class or instance template is found by its filename: the URL-encoded CURIE
// of the type when a declared prefix covers it, else the URL-encoded full IRI
// (docs/templating.md). A file that follows neither form is never resolved and
// nothing says so; two files naming one type leave it to lookup order which
// renders.
func TestTemplateFilenamesFollowConvention(t *testing.T) {
	cfg, err := config.Load(filepath.Join(repoRoot, "visoto.config.example"))
	if err != nil {
		t.Fatalf("load visoto.config.example: %v", err)
	}
	namespaces := map[string]string{}
	for _, p := range cfg.RDF.ParsedPrefixes {
		namespaces[p.Name] = strings.Trim(p.URI, "<>")
	}

	for _, dir := range []string{"classes", "instances"} {
		entries, err := os.ReadDir(filepath.Join(repoRoot, "templates", dir))
		if err != nil {
			t.Fatal(err)
		}
		owner := map[string]string{} // full type IRI → filename
		for _, e := range entries {
			name := e.Name()
			where := "templates/" + dir + "/" + name
			if e.IsDir() || !strings.HasSuffix(name, ".html") {
				t.Errorf("%s: only .html template files belong here", where)
				continue
			}
			if name == "default.html" {
				continue
			}
			decoded, err := url.QueryUnescape(strings.TrimSuffix(name, ".html"))
			if err != nil {
				t.Errorf("%s: not a URL-encoded type name: %v", where, err)
				continue
			}
			if want := url.QueryEscape(decoded) + ".html"; want != name {
				t.Errorf("%s: not in canonical encoding, the resolver looks for %s", where, want)
			}

			var iri string
			if strings.HasPrefix(decoded, "http://") || strings.HasPrefix(decoded, "https://") || strings.HasPrefix(decoded, "urn:") {
				iri = decoded
				// The longest namespace wins and a tie goes to the first name, as
				// in shortenIRI (internal/resource).
				best := ""
				for prefix, ns := range namespaces {
					if ns == "" || !strings.HasPrefix(decoded, ns) {
						continue
					}
					if best == "" || len(ns) > len(namespaces[best]) || (len(ns) == len(namespaces[best]) && prefix < best) {
						best = prefix
					}
				}
				if best != "" {
					t.Errorf("%s: prefix %q covers this type — name the file %s",
						where, best, url.QueryEscape(best+":"+strings.TrimPrefix(decoded, namespaces[best]))+".html")
				}
			} else {
				prefix, local, ok := strings.Cut(decoded, ":")
				ns, declared := namespaces[prefix]
				if !ok || !declared {
					t.Errorf("%s: %q is neither a full IRI nor a CURIE with a prefix declared in visoto.config.example [rdf] — the template is never resolved",
						where, decoded)
					continue
				}
				iri = ns + local
			}
			if prev, dup := owner[iri]; dup {
				t.Errorf("%s and %s both name <%s>", where, prev, iri)
			}
			owner[iri] = name
		}
	}
}

// The server reads its templates, static files and catalogs from disk, so a
// directory it needs must be in the image (Dockerfile) and on the build host
// (deploy.sh copies the tree directory by directory). Missing from either, the
// container starts without it and crash-loops — which no local run and no CI
// step sees, because both run from a full checkout.
func TestRuntimeDirsAreDeployed(t *testing.T) {
	dockerfile := readFile(t, filepath.Join(repoRoot, "Dockerfile"))
	deploy := readFile(t, filepath.Join(repoRoot, "deploy.sh"))

	shipped := map[string]bool{}
	for _, dir := range runtimeAssetDirs {
		name := strings.TrimPrefix(dir, "./")
		shipped[name] = true
		if !regexp.MustCompile(`(?m)^COPY --from=builder /app/` + name + ` \./` + name + `\s*$`).MatchString(dockerfile) {
			t.Errorf("Dockerfile: runtime stage does not COPY --from=builder /app/%s ./%s", name, name)
		}
	}

	// Everything the remote build needs: the runtime assets and every directory
	// holding Go source.
	needed := map[string]bool{}
	for name := range shipped {
		needed[name] = true
	}
	top, err := os.ReadDir(repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	var topDirs []string
	for _, e := range top {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		topDirs = append(topDirs, e.Name())
		hasGo := false
		_ = filepath.WalkDir(filepath.Join(repoRoot, e.Name()), func(path string, d fs.DirEntry, err error) error {
			if err == nil && !d.IsDir() && strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go") {
				hasGo = true
				return filepath.SkipAll
			}
			return nil
		})
		if hasGo {
			needed[e.Name()] = true
		}
	}
	// The directories go up in one `rsync --delete` (backslash-continued over
	// several lines); --delete is what keeps files deleted here from surviving there.
	joined := strings.ReplaceAll(deploy, "\\\n", " ")
	syncLine := regexp.MustCompile(`(?m)^.*\brsync\b.*\$\{REMOTE_DIR\}/.*$`).FindString(joined)
	if syncLine == "" {
		t.Fatal("deploy.sh: no rsync of the source directories to ${REMOTE_DIR}/")
	}
	if !strings.Contains(syncLine, "--delete") {
		t.Error("deploy.sh: the rsync lacks --delete, so files deleted here survive there")
	}
	words := map[string]bool{}
	for _, w := range strings.Fields(syncLine) {
		words[w] = true
	}
	for name := range needed {
		if !words[name] {
			t.Errorf("deploy.sh: the rsync does not ship %s to the build host", name)
		}
	}

	// The list is only worth checking if it is complete: any path into a
	// top-level directory written in the server's Go code names a directory the
	// binary reads at runtime.
	runtimeOnly := map[string]bool{strings.TrimPrefix(dataDir, "./"): true}
	literal := regexp.MustCompile(`"(?:\./)?([A-Za-z_]+)(?:/[^"]*)?"`)
	for _, dir := range []string{"app", "internal", "cmd"} {
		for _, path := range sourceFiles(t, dir, ".go") {
			if strings.HasSuffix(path, "_test.go") {
				continue
			}
			for i, line := range strings.Split(readFile(t, path), "\n") {
				if jsComment.MatchString(line) {
					continue
				}
				for _, m := range literal.FindAllStringSubmatch(line, -1) {
					name := m[1]
					if !strings.HasPrefix(m[0], `"./`) && !strings.Contains(m[0], "/") {
						continue // a bare word, not a path
					}
					isTop := false
					for _, d := range topDirs {
						if d == name {
							isTop = true
						}
					}
					if !isTop || shipped[name] || runtimeOnly[name] {
						continue
					}
					t.Errorf("%s:%d: reads %s at runtime, but %q is not in runtimeAssetDirs (app/server.go) — add it there, to the Dockerfile and to deploy.sh",
						rel(path), i+1, m[0], name)
				}
			}
		}
	}
}
