package templates

import (
	"bytes"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"hutzli.org/visoto/internal/i18n"
	"hutzli.org/visoto/internal/lang"
	"hutzli.org/visoto/internal/parser"
)

// testCodes mirrors the shipped language set, including the empty "no language"
// member, which is the one most likely to break a map-keyed lookup.
var testCodes = []string{"de", "fr", "en", ""}

// testLangs is testCodes as the configured language set the loaders take.
func testLangs() *lang.Set {
	langs := make([]lang.Language, 0, len(testCodes))
	for _, c := range testCodes {
		langs = append(langs, lang.Language{Code: c, Label: "label-" + lang.Key(c)})
	}
	return lang.New(langs, "en")
}

// loadTestCatalogs builds catalogs from a temporary locales dir whose de/fr
// entries differ from en, so a test can tell the variants apart by output.
func loadTestCatalogs(t *testing.T) *i18n.Catalogs {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"en.toml": "\"probe\" = \"PROBE-EN\"\n",
		"de.toml": "\"probe\" = \"PROBE-DE\"\n",
		"fr.toml": "\"probe\" = \"PROBE-FR\"\n",
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	cats, err := i18n.Load(dir, testCodes)
	if err != nil {
		t.Fatalf("i18n.Load() error = %v", err)
	}
	return cats
}

func TestLoad(t *testing.T) {
	const templatesDir = "../../templates"

	r := Load(templatesDir, loadTestCatalogs(t), testLangs())

	if r == nil {
		t.Fatal("Load() returned nil renderer")
	}

	// Verify expected template groups are non-empty
	for _, group := range []struct {
		name    string
		pattern string
	}{
		{"layouts", templatesDir + "/layout/*.html"},
		{"partials", templatesDir + "/partials/*.html"},
		{"pages", templatesDir + "/pages/*.html"},
		{"classes", templatesDir + "/classes/*.html"},
		{"instances", templatesDir + "/instances/*.html"},
	} {
		files, err := filepath.Glob(group.pattern)
		if err != nil {
			t.Fatalf("glob error for %s: %v", group.name, err)
		}
		if len(files) == 0 {
			t.Errorf("expected at least one %s template, got none", group.name)
		}
	}
}

// TestCurrentYear guards the footer's copyright against going stale again by
// checking it tracks the clock rather than returning a baked-in year.
func TestCurrentYear(t *testing.T) {
	if got, want := currentYear(), time.Now().Year(); got != want {
		t.Errorf("currentYear() = %d, want %d", got, want)
	}
}

// TestLoadRegistersEveryLanguageVariant guards the invariant renderName relies
// on: every page is registered under Name(code, …) for every configured code.
// A page missing one variant would 500 only for users in that language.
func TestLoadRegistersEveryLanguageVariant(t *testing.T) {
	const templatesDir = "../../templates"
	r := Load(templatesDir, loadTestCatalogs(t), testLangs())

	var pageNames []string
	for _, dir := range []string{"pages", "classes", "instances"} {
		files, err := filepath.Glob(filepath.Join(templatesDir, dir, "*.html"))
		if err != nil {
			t.Fatalf("glob %s: %v", dir, err)
		}
		for _, f := range files {
			pageNames = append(pageNames, filepath.Join(dir, filepath.Base(f)))
		}
	}
	if len(pageNames) == 0 {
		t.Fatal("no page templates found")
	}

	for _, name := range pageNames {
		for _, code := range testCodes {
			if inst := r.Instance(Name(code, name), nil); inst == nil {
				t.Errorf("no registered template for language %q page %q", code, name)
			}
		}
	}
}

// TestVariantsBindTheirOwnLanguage is the load-bearing test for the whole
// approach: html/template resolves functions at execute time, so a Clone whose
// func map is overridden must call ITS language's t, not the one the set was
// parsed with. If this breaks, every page silently renders in one language.
//
// It also renders through a {{ template }} include, so a regression in the
// file-grouping (which is what multitemplate provides) shows up here too.
func TestVariantsBindTheirOwnLanguage(t *testing.T) {
	cats := loadTestCatalogs(t)
	dir := t.TempDir()

	// A two-file set: the entry template includes a partial, and only the partial
	// calls t — so a passing test proves the binding reaches nested templates.
	entry := filepath.Join(dir, "base.html")
	partial := filepath.Join(dir, "part.html")
	if err := os.WriteFile(entry, []byte(`<main>{{ template "inner" . }}</main>`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(partial, []byte(`{{ define "inner" }}[{{ t "probe" }}|{{ siteLang }}]{{ end }}`), 0644); err != nil {
		t.Fatal(err)
	}

	base, err := parseSet([]string{entry, partial})
	if err != nil {
		t.Fatalf("parse set: %v", err)
	}

	for _, tc := range []struct{ code, want string }{
		{"de", "[PROBE-DE|de]"},
		{"fr", "[PROBE-FR|fr]"},
		{"en", "[PROBE-EN|en]"},
		{"", "[PROBE-EN|]"}, // no language: base catalog, but siteLang stays ""
	} {
		variant, err := base.Clone()
		if err != nil {
			t.Fatalf("clone for %q: %v", tc.code, err)
		}
		variant = variant.Funcs(cats.FuncMap(tc.code))

		var buf bytes.Buffer
		if err := variant.Execute(&buf, nil); err != nil {
			t.Fatalf("execute for %q: %v", tc.code, err)
		}
		if got := buf.String(); !strings.Contains(got, tc.want) {
			t.Errorf("language %q rendered %q, want it to contain %q", tc.code, got, tc.want)
		}
	}
}

// TestRenderRealPagePerLanguage renders an actual page through the registered
// variants, proving the full Load path (layouts + partials + page) executes and
// carries the language through to the rendered HTML.
func TestRenderRealPagePerLanguage(t *testing.T) {
	const templatesDir = "../../templates"
	r := Load(templatesDir, loadTestCatalogs(t), testLangs())

	for _, code := range testCodes {
		inst := r.Instance(Name(code, "pages/home.html"), parser.TemplateData{})
		if inst == nil {
			t.Fatalf("no instance for language %q", code)
		}
		rec := httptest.NewRecorder()
		if err := inst.Render(rec); err != nil {
			t.Fatalf("render home.html for language %q: %v", code, err)
		}
		body := rec.Body.String()
		if body == "" {
			t.Errorf("render home.html for language %q produced no output", code)
		}
		// The <html lang> attribute is driven by siteLang, so it is the cheapest
		// end-to-end proof that the right variant ran.
		if want := `lang="` + code + `"`; !strings.Contains(body, want) {
			t.Errorf("home.html for language %q does not contain %s", code, want)
		}
	}
}

func TestName(t *testing.T) {
	if got := Name("de", "pages/home.html"); got != "de:pages/home.html" {
		t.Errorf("Name(de) = %q", got)
	}
	// The empty language needs a non-empty key or its variant would collide with
	// a malformed ":pages/home.html" lookup.
	if got := Name("", "pages/home.html"); got != lang.Key("")+":pages/home.html" {
		t.Errorf("Name(\"\") = %q", got)
	}
}
