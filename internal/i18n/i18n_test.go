package i18n

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

// writeCatalogs lays out a temporary locales dir and loads it.
func writeCatalogs(t *testing.T, files map[string]string, codes []string) *Catalogs {
	t.Helper()
	dir := t.TempDir()
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	c, err := Load(dir, codes)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	return c
}

func TestTranslateFallsBackToBase(t *testing.T) {
	c := writeCatalogs(t, map[string]string{
		"en.toml": "\"a.hello\" = \"Hello\"\n\"a.bye\" = \"Bye\"\n",
		"de.toml": "\"a.hello\" = \"Hallo\"\n", // a.bye deliberately untranslated
	}, []string{"de", "en", ""})

	tests := []struct {
		code, key, want string
	}{
		{"de", "a.hello", "Hallo"},
		{"de", "a.bye", "Bye"}, // per-key fallback to the base catalog
		{"en", "a.hello", "Hello"},
		{"", "a.hello", "Hello"},                        // the "no language" choice renders the base catalog
		{"de", "a.missing", missingMarker("a.missing")}, // no entry, no inline fallback
		{"zz", "a.hello", "Hello"},                      // unconfigured code degrades to base, never panics
	}
	for _, tt := range tests {
		if got := c.translate(tt.code, tt.key, "", nil); got != tt.want {
			t.Errorf("translate(%q, %q) = %q, want %q", tt.code, tt.key, got, tt.want)
		}
	}
}

func TestTranslateWithData(t *testing.T) {
	c := writeCatalogs(t, map[string]string{
		"en.toml": "\"greet\" = \"Hello {{.Name}}\"\n",
	}, []string{"en"})

	got := c.translate("en", "greet", "", map[string]any{"Name": "Otto"})
	if got != "Hello Otto" {
		t.Errorf("translate with data = %q, want %q", got, "Hello Otto")
	}
}

// TestInlineFallback covers the backstop rule in both directions: the inline
// text stands in for a key no catalog defines, and a catalog entry always beats
// the inline text when both exist.
func TestInlineFallback(t *testing.T) {
	c := writeCatalogs(t, map[string]string{
		"en.toml": "\"known\" = \"Known EN\"\n",
		"de.toml": "\"known\" = \"Known DE\"\n",
	}, []string{"de", "en"})

	tests := []struct {
		name, code, key, fallback, want string
	}{
		{"uncatalogued key uses the fallback", "en", "brand.new", "Brand new", "Brand new"},
		{"fallback applies in every language", "de", "brand.new", "Brand new", "Brand new"},
		{"catalog beats the fallback", "en", "known", "Ignored", "Known EN"},
		{"catalog beats it per language too", "de", "known", "Ignored", "Known DE"},
		{"no fallback and no entry marks the gap", "en", "gone", "", missingMarker("gone")},
	}
	for _, tt := range tests {
		if got := c.translate(tt.code, tt.key, tt.fallback, nil); got != tt.want {
			t.Errorf("%s: translate(%q, %q, %q) = %q, want %q",
				tt.name, tt.code, tt.key, tt.fallback, got, tt.want)
		}
	}
}

// TestInlineFallbackRendersData is the reason the fallback goes through
// go-i18n's DefaultMessage rather than being returned directly: placeholders
// must interpolate identically whether the text is inline or in a catalog.
func TestInlineFallbackRendersData(t *testing.T) {
	c := writeCatalogs(t, map[string]string{"en.toml": "\"x\" = \"X\"\n"}, []string{"en"})

	got := c.translate("en", "rows.showing", "Showing {{.N}} rows", map[string]any{"N": 20})
	if want := "Showing 20 rows"; got != want {
		t.Errorf("fallback with data = %q, want %q", got, want)
	}
}

// TestTranslateCountFallback pins the accepted imprecision: one inline string
// serves every plural form, so only a real catalog entry pluralizes properly.
func TestTranslateCountFallback(t *testing.T) {
	c := writeCatalogs(t, map[string]string{"en.toml": "\"x\" = \"X\"\n"}, []string{"en"})

	for _, count := range []int{1, 7} {
		got := c.translateCount("en", "rows", "{{.Count}} row(s)", count, nil)
		want := fmt.Sprintf("%d row(s)", count)
		if got != want {
			t.Errorf("translateCount(%d) with fallback = %q, want %q", count, got, want)
		}
	}
}

// TestSplitArgs locks the type-based convention the template call shapes rely
// on: a string is prose, anything else is the data map, in either order.
func TestSplitArgs(t *testing.T) {
	data := map[string]any{"N": 1}
	tests := []struct {
		name         string
		args         []any
		wantFallback string
		wantData     any
	}{
		{"no arguments", nil, "", nil},
		{"fallback only", []any{"Text"}, "Text", nil},
		{"data only", []any{data}, "", data},
		{"fallback then data", []any{"Text", data}, "Text", data},
		{"data then fallback", []any{data, "Text"}, "Text", data},
	}
	for _, tt := range tests {
		fallback, got := splitArgs(tt.args)
		if fallback != tt.wantFallback {
			t.Errorf("%s: fallback = %q, want %q", tt.name, fallback, tt.wantFallback)
		}
		if !reflect.DeepEqual(got, tt.wantData) {
			t.Errorf("%s: data = %v, want %v", tt.name, got, tt.wantData)
		}
	}
}

func TestTranslateCountSeedsCount(t *testing.T) {
	c := writeCatalogs(t, map[string]string{
		"en.toml": "[rows]\none = \"{{.Count}} row\"\nother = \"{{.Count}} rows\"\n",
	}, []string{"en"})

	if got := c.translateCount("en", "rows", "", 1, nil); got != "1 row" {
		t.Errorf("translateCount(1) = %q, want %q", got, "1 row")
	}
	if got := c.translateCount("en", "rows", "", 7, nil); got != "7 rows" {
		t.Errorf("translateCount(7) = %q, want %q", got, "7 rows")
	}
}

func TestLoadRequiresBaseCatalog(t *testing.T) {
	if _, err := Load(t.TempDir(), []string{"en"}); err == nil {
		t.Error("Load() with no en.toml = nil error, want failure")
	}
}

func TestLoadToleratesMissingTranslation(t *testing.T) {
	// A configured language with no catalog must warn, not fail the boot.
	c := writeCatalogs(t, map[string]string{
		"en.toml": "\"a\" = \"A\"\n",
	}, []string{"de", "en"})
	if got := c.translate("de", "a", "", nil); got != "A" {
		t.Errorf("translate with no de catalog = %q, want %q", got, "A")
	}
}

func TestKeysWalksNestedTables(t *testing.T) {
	// A plural message is a table but is one key; a namespace table is not.
	c := writeCatalogs(t, map[string]string{
		"en.toml": "\"flat.key\" = \"F\"\n\n[rows]\none = \"1\"\nother = \"n\"\n\n[nested]\ninner = \"I\"\n",
	}, []string{"en"})

	want := map[string]bool{"flat.key": true, "rows": true, "nested.inner": true}
	got := c.Keys()
	if len(got) != len(want) {
		t.Fatalf("Keys() = %v, want keys %v", got, want)
	}
	for _, k := range got {
		if !want[k] {
			t.Errorf("Keys() returned unexpected key %q", k)
		}
	}
}

func TestJSStrings(t *testing.T) {
	c := writeCatalogs(t, map[string]string{
		"en.toml": "\"js.loading\" = \"Loading\"\n\"topbar.search\" = \"Search\"\n",
		"de.toml": "\"js.loading\" = \"Lädt\"\n",
	}, []string{"de", "en"})

	got := c.JSStrings("de")
	if len(got) != 1 || got["js.loading"] != "Lädt" {
		t.Errorf("JSStrings(de) = %v, want only js.loading translated", got)
	}
}

func TestFuncMapBindsLanguage(t *testing.T) {
	c := writeCatalogs(t, map[string]string{
		"en.toml": "\"a\" = \"A\"\n",
		"de.toml": "\"a\" = \"Ä\"\n",
	}, []string{"de", "en"})

	de := c.FuncMap("de")["t"].(func(string, ...any) string)
	en := c.FuncMap("en")["t"].(func(string, ...any) string)
	if de("a") != "Ä" || en("a") != "A" {
		t.Errorf("FuncMap did not bind per language: de=%q en=%q", de("a"), en("a"))
	}

	siteLang := c.FuncMap("de")["siteLang"].(func() string)
	if siteLang() != "de" {
		t.Errorf("siteLang() = %q, want \"de\"", siteLang())
	}
}

// usedKeys scans the real templates/ and static/js trees for every message key
// referenced in code.
//
// Since the English text moved inline, the code — not en.toml — is what defines
// which keys exist. en.toml retains only the js.* strings, which have no
// template call site, so both consistency tests below start from this scan
// rather than from the base catalog's key set.
//
// Two call shapes: {{ t "key" … }} in templates, vsT('key', …) in static/js.
func usedKeys(t *testing.T) map[string]bool {
	t.Helper()

	tmplCall := regexp.MustCompile(`\b(?:t|tHTML|tn)\s+"([^"]+)"`)
	jsCall := regexp.MustCompile(`\bvsTf?\(\s*['"]([^'"]+)['"]`)
	scans := []struct {
		dir, ext string
		res      []*regexp.Regexp
	}{
		// Templates carry both shapes: {{ t }} in markup, and vsT() inside the
		// <script> blocks some pages still embed.
		{"../../templates", ".html", []*regexp.Regexp{tmplCall, jsCall}},
		{"../../static/js", ".js", []*regexp.Regexp{jsCall}},
	}

	used := make(map[string]bool)
	for _, scan := range scans {
		err := filepath.WalkDir(scan.dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || filepath.Ext(path) != scan.ext {
				return err
			}
			body, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			for _, re := range scan.res {
				for _, m := range re.FindAllSubmatch(body, -1) {
					used[string(m[1])] = true
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", scan.dir, err)
		}
	}
	if len(used) == 0 {
		t.Fatal("scanned no message keys; the scan itself is broken")
	}
	return used
}

// keysReadFromGo are referenced from Go rather than from a {{ t }} or vsT call,
// so no source scan can see them.
var keysReadFromGo = map[string]bool{
	"lang.de": true, "lang.fr": true, "lang.it": true,
	"lang.en": true, "lang.rm": true, "lang.none": true, // Catalogs.languageOptions
}

// TestBaseCatalogHoldsOnlyJSStrings pins the trimmed shape of en.toml: template
// English now lives inline at the call site, and the base catalog exists solely
// to supply the js.* island (and because Load treats it as mandatory). A
// template key reappearing here means someone re-duplicated a string.
func TestBaseCatalogHoldsOnlyJSStrings(t *testing.T) {
	base, err := Load("../../locales", []string{"en"})
	if err != nil {
		t.Fatalf("Load base: %v", err)
	}

	keys := base.Keys()
	if len(keys) == 0 {
		t.Fatal("base catalog defines no keys; the js.* island would be empty")
	}
	for _, k := range keys {
		if !strings.HasPrefix(k, "js.") {
			t.Errorf("en.toml defines %q; only js.* keys belong in the base catalog "+
				"now that template English lives inline", k)
		}
	}
}

// TestEveryJSKeyIsInTheBaseCatalog is the counterpart: js.* strings have no
// template call site to fall back on, so a vsT key missing from en.toml means
// every non-English locale silently loses that string.
func TestEveryJSKeyIsInTheBaseCatalog(t *testing.T) {
	base, err := Load("../../locales", []string{"en"})
	if err != nil {
		t.Fatalf("Load base: %v", err)
	}
	defined := make(map[string]bool)
	for _, k := range base.Keys() {
		defined[k] = true
	}

	for key := range usedKeys(t) {
		if strings.HasPrefix(key, "js.") && !defined[key] {
			t.Errorf("vsT uses %q but en.toml does not define it; translations "+
				"of that string can never be reached", key)
		}
	}
}

// TestTranslationsDefineNoUnknownKeys keeps a translation from drifting: a key
// no call site references is either a typo or a leftover, and either way it can
// never be reached. The reference set is the code scan, since the base catalog
// no longer enumerates the template keys.
func TestTranslationsDefineNoUnknownKeys(t *testing.T) {
	const localesDir = "../../locales"

	used := usedKeys(t)
	for _, code := range []string{"de", "fr", "it", "rm"} {
		path := filepath.Join(localesDir, code+".toml")
		if _, err := os.Stat(path); err != nil {
			continue // a missing translation is legal; Load only warns
		}
		for _, key := range baseKeys(path) {
			if !used[key] && !keysReadFromGo[key] {
				t.Errorf("%s.toml defines %q, which no template or script uses", code, key)
			}
		}
	}
}

// TestFormatNum covers the grouping conventions the metric cards depend on. The
// Swiss apostrophe for German is the point of numTag's special case, so it is
// asserted explicitly rather than left to whatever CLDR ships for plain "de".
func TestFormatNum(t *testing.T) {
	// U+2019 RIGHT SINGLE QUOTATION MARK is what CLDR uses for de-CH/rm, not an
	// ASCII apostrophe — spelled out here so a mismatch reads clearly on failure.
	const apos = "’"
	// French groups with a no-break space (U+00A0), not a plain space.
	const nbsp = "\u00a0"

	tests := []struct {
		code string
		in   any
		want string
	}{
		{"en", "781462", "781,462"},
		{"de", "781462", "781" + apos + "462"},
		{"it", "781462", "781.462"},
		{"rm", "781462", "781" + apos + "462"},
		{"fr", "781462", "781" + nbsp + "462"},

		// Small numbers are left alone; no separator below the grouping width.
		{"de", "0", "0"},
		{"en", "999", "999"},

		// Ints, because Go-side counts are not strings.
		{"de", 456536, "456" + apos + "536"},

		// Anything that is not a whole number passes through untouched: a failed
		// metric, a decimal, or a label must never be mangled or zeroed.
		{"de", "n/a", "n/a"},
		{"de", "", ""},
		{"de", "3.14", "3.14"},
		{"en", "abc", "abc"},
	}

	for _, tt := range tests {
		if got := formatNum(tt.code, tt.in); got != tt.want {
			t.Errorf("formatNum(%q, %v) = %q, want %q", tt.code, tt.in, got, tt.want)
		}
	}
}

// The func map must expose formatNum bound to its own language, for the same
// reason t is — a clone renders with the language it was bound to.
func TestFuncMapBindsFormatNum(t *testing.T) {
	c := writeCatalogs(t, map[string]string{
		"en.toml": `"probe" = "x"`,
		"de.toml": `"probe" = "x"`,
	}, []string{"en", "de"})
	for _, tt := range []struct{ code, want string }{
		{"de", "1" + "’" + "000"},
		{"en", "1,000"},
	} {
		fn, ok := c.FuncMap(tt.code)["formatNum"].(func(any) string)
		if !ok {
			t.Fatalf("FuncMap(%q) has no formatNum of the expected type", tt.code)
		}
		if got := fn("1000"); got != tt.want {
			t.Errorf("formatNum bound to %q = %q, want %q", tt.code, got, tt.want)
		}
	}
}
