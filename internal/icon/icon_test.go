package icon

import (
	"os"
	"path/filepath"
	"testing"
)

// withIcons points the package cache at a temp directory holding the named
// files, so the tests state their own fixtures instead of depending on whatever
// happens to be in static/img/resource.
//
// A name with a slash ("rdf/Property.svg") lands in a namespace subfolder,
// resolved against testNamespaces.
func withIcons(t *testing.T, files ...string) {
	t.Helper()
	dir := writeIcons(t, files...)
	if err := Init(dir, testNamespaces, nil); err != nil {
		t.Fatalf("Init(%s): %v", dir, err)
	}
}

var testNamespaces = map[string]string{
	"rdf":    "http://www.w3.org/1999/02/22-rdf-syntax-ns#",
	"schema": "http://schema.org/",
}

func writeIcons(t *testing.T, files ...string) string {
	t.Helper()
	dir := t.TempDir()
	for _, name := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatalf("mkdir for %s: %v", name, err)
		}
		if err := os.WriteFile(path, []byte("<svg/>"), 0o600); err != nil {
			t.Fatalf("write fixture %s: %v", name, err)
		}
	}
	return dir
}

func TestLocalName(t *testing.T) {
	tests := []struct {
		name string
		iri  string
		want string
	}{
		{"class IRI, last path segment", "https://schema.ld.admin.ch/Canton", "Canton"},
		{"fragment wins over path", "http://www.w3.org/2004/02/skos/core#Concept", "Concept"},
		{"instance IRI yields its id", "https://ld.admin.ch/canton/1", "1"},
		{"percent-encoded prefix", "schema%3APerson", "Person"},
		{"prefixed form", "skos:Concept", "Concept"},
		{"trailing slash ignored", "https://example.org/Person/", "Person"},
		{"plus is not a space", "https://example.org/A+B", "A+B"},
		{"empty stays empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := LocalName(tt.iri); got != tt.want {
				t.Errorf("LocalName(%q) = %q, want %q", tt.iri, got, tt.want)
			}
		})
	}
}

// TestResolveDirectMatch is check (a): the cell holds a class IRI, which is what
// already worked before types were resolved. It must not regress.
func TestResolveDirectMatch(t *testing.T) {
	withIcons(t, "Canton.svg", "Class.svg")

	got := Resolve("https://schema.ld.admin.ch/Canton", nil)
	if want := BasePath + "Canton.svg"; got != want {
		t.Errorf("Resolve(class IRI) = %q, want %q", got, want)
	}
}

// TestResolveByType is check (b), the case this feature exists for: an instance
// IRI whose local name matches nothing, resolved through its rdf:type.
func TestResolveByType(t *testing.T) {
	withIcons(t, "Canton.svg")

	got := Resolve("https://ld.admin.ch/canton/1", []string{"https://schema.ld.admin.ch/Canton"})
	if want := BasePath + "Canton.svg"; got != want {
		t.Errorf("Resolve(instance IRI, [Canton]) = %q, want %q", got, want)
	}
}

// TestResolveDirectMatchBeatsType pins the priority: an icon named after the
// resource itself is more specific than one named after any of its classes.
func TestResolveDirectMatchBeatsType(t *testing.T) {
	withIcons(t, "Canton.svg", "Municipality.svg")

	got := Resolve("https://schema.ld.admin.ch/Municipality", []string{"https://schema.ld.admin.ch/Canton"})
	if want := BasePath + "Municipality.svg"; got != want {
		t.Errorf("Resolve() = %q, want the resource's own icon %q", got, want)
	}
}

// TestResolveExactTypeBeatsFallback is the multi-typed case. LINDAS resources
// routinely carry a generic class alongside a specific one, and the generic one
// is often listed first — an exact match anywhere in the list must still win
// over a .fallback match anywhere in the list.
func TestResolveExactTypeBeatsFallback(t *testing.T) {
	withIcons(t, "Canton.svg", "DefinedTerm.fallback.svg")

	types := []string{
		"http://schema.org/DefinedTerm",     // only a fallback icon, listed first
		"https://schema.ld.admin.ch/Canton", // a real icon, listed second
	}
	got := Resolve("https://ld.admin.ch/canton/1", types)
	if want := BasePath + "Canton.svg"; got != want {
		t.Errorf("Resolve(multi-typed) = %q, want the exact match %q", got, want)
	}
}

// TestResolveFallbackWhenNoExact is the other half of the two-pass: with no
// exact match anywhere, the fallback is the right answer rather than nothing.
func TestResolveFallbackWhenNoExact(t *testing.T) {
	withIcons(t, "Canton.svg", "DefinedTerm.fallback.svg")

	got := Resolve("https://example.org/term/42", []string{"http://schema.org/DefinedTerm"})
	if want := BasePath + "DefinedTerm.fallback.svg"; got != want {
		t.Errorf("Resolve() = %q, want %q", got, want)
	}
}

// TestResolveOwnFallback covers a resource whose own local name has only a
// fallback icon — it should be used before consulting the types.
func TestResolveOwnFallback(t *testing.T) {
	withIcons(t, "Canton.svg", "DefinedTerm.fallback.svg")

	got := Resolve("http://schema.org/DefinedTerm", []string{"https://schema.ld.admin.ch/Canton"})
	if want := BasePath + "DefinedTerm.fallback.svg"; got != want {
		t.Errorf("Resolve() = %q, want the resource's own fallback %q", got, want)
	}
}

// TestResolveMisses is check 4: every shape of "nothing matches" returns the
// empty string rather than a bogus path, so each caller can apply its own
// default (none in tables, default.svg in the page header).
func TestResolveMisses(t *testing.T) {
	withIcons(t, "Canton.svg", "DefinedTerm.fallback.svg")

	tests := []struct {
		name  string
		iri   string
		types []string
	}{
		{"untyped IRI with no matching name", "https://ld.admin.ch/canton/1", nil},
		{"typed, but no icon for that type", "https://example.org/x/1", []string{"http://example.org/Unknown"}},
		{"empty type list entries", "https://example.org/x/1", []string{""}},
		{"empty IRI and no types", "", nil},
		{"literal-looking value", "just a literal", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Resolve(tt.iri, tt.types); got != "" {
				t.Errorf("Resolve(%q, %v) = %q, want \"\"", tt.iri, tt.types, got)
			}
		})
	}
}

// TestNames checks the shape the JS islands depend on: bare names for real
// icons, a ".fallback" suffix for the weaker ones, each mapped to its file.
func TestNames(t *testing.T) {
	withIcons(t, "Canton.svg", "DefinedTerm.fallback.svg", "notanicon.txt")

	names := Names()
	if names["Canton"] != "Canton.svg" {
		t.Errorf("Names()[Canton] = %q, want Canton.svg", names["Canton"])
	}
	if names["DefinedTerm.fallback"] != "DefinedTerm.fallback.svg" {
		t.Errorf("Names()[DefinedTerm.fallback] = %q", names["DefinedTerm.fallback"])
	}
	if _, ok := names["DefinedTerm"]; ok {
		t.Error("Names() has bare \"DefinedTerm\"; only the .fallback form exists")
	}
	if len(names) != 2 {
		t.Errorf("Names() has %d entries, want 2 (non-svg files must be ignored)", len(names))
	}
}

// TestInitReplaces guards the re-scan: Init is idempotent, so a second scan of a
// different directory must not leave the first one's names behind.
func TestInitReplaces(t *testing.T) {
	withIcons(t, "Canton.svg")
	withIcons(t, "Municipality.svg")

	if got := Resolve("https://schema.ld.admin.ch/Canton", nil); got != "" {
		t.Errorf("Resolve(Canton) = %q after re-Init without it, want \"\"", got)
	}
	if got := Resolve("https://schema.ld.admin.ch/Municipality", nil); got == "" {
		t.Error("Resolve(Municipality) = \"\" after Init with it")
	}
}

// TestResolveNamespaced is issue #45: the same local name in two ontologies gets
// two icons. The namespaced one serves its namespace in IRI, CURIE and
// percent-encoded form; every other namespace still gets the root icon.
func TestResolveNamespaced(t *testing.T) {
	withIcons(t, "Property.svg", "rdf/Property.svg")

	rdfProp := BasePath + "rdf/Property.svg"
	tests := []struct {
		iri  string
		want string
	}{
		{"http://www.w3.org/1999/02/22-rdf-syntax-ns#Property", rdfProp},
		{"rdf:Property", rdfProp},
		{"rdf%3AProperty", rdfProp},
		{"http://schema.org/Property", BasePath + "Property.svg"},
		{"https://example.org/onto/Property", BasePath + "Property.svg"},
	}
	for _, tt := range tests {
		if got := Resolve(tt.iri, nil); got != tt.want {
			t.Errorf("Resolve(%q) = %q, want %q", tt.iri, got, tt.want)
		}
	}
	if got := Resolve("https://example.org/p/1", []string{"rdf:Property"}); got != rdfProp {
		t.Errorf("Resolve(typed rdf:Property) = %q, want %q", got, rdfProp)
	}
}

// TestResolveNamespacedKeepsFallbackOrder: a namespace match beats a root match,
// but a namespaced FALLBACK must not beat a root EXACT icon — fallback means
// "generic", wherever the file lives.
func TestResolveNamespacedKeepsFallbackOrder(t *testing.T) {
	withIcons(t, "Canton.svg", "schema/DefinedTerm.fallback.svg")

	types := []string{"http://schema.org/DefinedTerm", "https://schema.ld.admin.ch/Canton"}
	if got, want := Resolve("https://ld.admin.ch/canton/1", types), BasePath+"Canton.svg"; got != want {
		t.Errorf("Resolve() = %q, want the root exact match %q", got, want)
	}
	if got, want := Resolve("https://example.org/t/1", types[:1]), BasePath+"schema/DefinedTerm.fallback.svg"; got != want {
		t.Errorf("Resolve() = %q, want %q", got, want)
	}
}

// TestNamesNamespaced: namespaced icons reach the JS islands under both the full
// IRI and the CURIE, with their folder in the path.
func TestNamesNamespaced(t *testing.T) {
	withIcons(t, "rdf/Property.svg", "schema/DefinedTerm.fallback.svg")

	names := Names()
	want := map[string]string{
		"http://www.w3.org/1999/02/22-rdf-syntax-ns#Property": "rdf/Property.svg",
		"rdf:Property":                           "rdf/Property.svg",
		"http://schema.org/DefinedTerm.fallback": "schema/DefinedTerm.fallback.svg",
		"schema:DefinedTerm.fallback":            "schema/DefinedTerm.fallback.svg",
	}
	for k, v := range want {
		if names[k] != v {
			t.Errorf("Names()[%q] = %q, want %q", k, names[k], v)
		}
	}
	if _, ok := names["Property"]; ok {
		t.Error("a namespaced icon must not register a bare local name")
	}
}

// TestInitUnknownFolder: a folder naming an undeclared prefix is a startup
// error, not an icon set that silently never matches. "_" folders are skipped.
func TestInitUnknownFolder(t *testing.T) {
	if err := Init(writeIcons(t, "_sheet/icon-sheet.svg", "Canton.svg"), testNamespaces, nil); err != nil {
		t.Errorf("Init with a _ folder: %v", err)
	}
	if err := Init(writeIcons(t, "nope/Thing.svg"), testNamespaces, nil); err == nil {
		t.Error("Init with an undeclared prefix folder succeeded, want an error")
	}
}
