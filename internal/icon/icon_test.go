package icon

import (
	"os"
	"path/filepath"
	"testing"
)

// withIcons points the package cache at a temp directory holding the named
// files, so the tests state their own fixtures instead of depending on whatever
// happens to be in static/img/resource.
func withIcons(t *testing.T, files ...string) {
	t.Helper()
	dir := t.TempDir()
	for _, name := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("<svg/>"), 0o600); err != nil {
			t.Fatalf("write fixture %s: %v", name, err)
		}
	}
	if err := Init(dir, nil); err != nil {
		t.Fatalf("Init(%s): %v", dir, err)
	}
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

	got := Resolve(Subject{IRI: "https://schema.ld.admin.ch/Canton"}, Cell)
	if want := BasePath + "Canton.svg"; got != want {
		t.Errorf("Resolve(class IRI) = %q, want %q", got, want)
	}
}

// TestResolveByType is check (b), the case this feature exists for: an instance
// IRI whose local name matches nothing, resolved through its rdf:type.
func TestResolveByType(t *testing.T) {
	withIcons(t, "Canton.svg")

	got := Resolve(Subject{IRI: "https://ld.admin.ch/canton/1", Types: []string{"https://schema.ld.admin.ch/Canton"}}, Cell)
	if want := BasePath + "Canton.svg"; got != want {
		t.Errorf("Resolve(instance IRI, [Canton]) = %q, want %q", got, want)
	}
}

// TestResolveDirectMatchBeatsType pins the priority: an icon named after the
// resource itself is more specific than one named after any of its classes.
func TestResolveDirectMatchBeatsType(t *testing.T) {
	withIcons(t, "Canton.svg", "Municipality.svg")

	got := Resolve(Subject{IRI: "https://schema.ld.admin.ch/Municipality", Types: []string{"https://schema.ld.admin.ch/Canton"}}, Cell)
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
	got := Resolve(Subject{IRI: "https://ld.admin.ch/canton/1", Types: types}, Cell)
	if want := BasePath + "Canton.svg"; got != want {
		t.Errorf("Resolve(multi-typed) = %q, want the exact match %q", got, want)
	}
}

// TestResolveFallbackWhenNoExact is the other half of the two-pass: with no
// exact match anywhere, the fallback is the right answer rather than nothing.
func TestResolveFallbackWhenNoExact(t *testing.T) {
	withIcons(t, "Canton.svg", "DefinedTerm.fallback.svg")

	got := Resolve(Subject{IRI: "https://example.org/term/42", Types: []string{"http://schema.org/DefinedTerm"}}, Cell)
	if want := BasePath + "DefinedTerm.fallback.svg"; got != want {
		t.Errorf("Resolve() = %q, want %q", got, want)
	}
}

// TestResolveOwnFallback covers a resource whose own local name has only a
// fallback icon — it should be used before consulting the types.
func TestResolveOwnFallback(t *testing.T) {
	withIcons(t, "Canton.svg", "DefinedTerm.fallback.svg")

	got := Resolve(Subject{IRI: "http://schema.org/DefinedTerm", Types: []string{"https://schema.ld.admin.ch/Canton"}}, Cell)
	if want := BasePath + "DefinedTerm.fallback.svg"; got != want {
		t.Errorf("Resolve() = %q, want the resource's own fallback %q", got, want)
	}
}

// TestResolveMisses is check 4: every shape of "nothing matches" returns the
// empty string rather than a bogus path, so each caller can apply its own
// default (none in tables, default.svg in the page header) — see TestResolveDefaults.
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
			if got := Resolve(Subject{IRI: tt.iri, Types: tt.types}, Cell); got != "" {
				t.Errorf("Resolve(%q, %v) = %q, want \"\"", tt.iri, tt.types, got)
			}
		})
	}
}

// TestNames checks the shape the JS islands depend on: bare names for real
// icons, a ".fallback" suffix for the weaker ones, so appending ".svg" rebuilds
// the file name in both cases.
func TestNames(t *testing.T) {
	withIcons(t, "Canton.svg", "DefinedTerm.fallback.svg", "notanicon.txt")

	names := Current().Names()
	if !names["Canton"] {
		t.Error("Names() missing \"Canton\"")
	}
	if !names["DefinedTerm.fallback"] {
		t.Error("Names() missing \"DefinedTerm.fallback\"")
	}
	if names["DefinedTerm"] {
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

	if got := Resolve(Subject{IRI: "https://schema.ld.admin.ch/Canton"}, Cell); got != "" {
		t.Errorf("Resolve(Canton) = %q after re-Init without it, want \"\"", got)
	}
	if got := Resolve(Subject{IRI: "https://schema.ld.admin.ch/Municipality"}, Cell); got == "" {
		t.Error("Resolve(Municipality) = \"\" after Init with it")
	}
}

// TestResolveDefaults pins what a miss means per Kind: nothing in a cell, the
// generic stand-ins elsewhere. A hit is the same path whatever the kind.
func TestResolveDefaults(t *testing.T) {
	withIcons(t, "Canton.svg")

	miss := Subject{IRI: "https://example.org/x/1"}
	tests := []struct {
		kind Kind
		want string
	}{
		{Cell, ""},
		{Page, BasePath + "default.svg"},
		{Class, BasePath + "defaultClass.svg"},
		{Instance, BasePath + "defaultInstance.svg"},
	}
	for _, tt := range tests {
		if got := Resolve(miss, tt.kind); got != tt.want {
			t.Errorf("Resolve(miss, %d) = %q, want %q", tt.kind, got, tt.want)
		}
		if got := Resolve(Subject{IRI: "https://schema.ld.admin.ch/Canton"}, tt.kind); got != BasePath+"Canton.svg" {
			t.Errorf("Resolve(hit, %d) = %q, want the icon itself", tt.kind, got)
		}
	}
}

// TestVersionTracksIconSet guards the cache-busting contract of /api/icons.json:
// the same set yields the same version, a different set a different one.
func TestVersionTracksIconSet(t *testing.T) {
	withIcons(t, "Canton.svg", "DefinedTerm.fallback.svg")
	v1 := Current().Version()
	withIcons(t, "DefinedTerm.fallback.svg", "Canton.svg")
	if v2 := Current().Version(); v2 != v1 {
		t.Errorf("Version changed for the same set: %q -> %q", v1, v2)
	}
	withIcons(t, "Canton.svg", "DefinedTerm.svg")
	if v3 := Current().Version(); v3 == v1 {
		t.Error("Version unchanged after a fallback became a real icon")
	}
	if v1 == "" {
		t.Error("Version is empty")
	}
}

// TestManifestAndConfig check the shapes visoto-icons.js reads.
func TestManifestAndConfig(t *testing.T) {
	withIcons(t, "Canton.svg")

	if m := Current().Manifest(); !m.Names["Canton"] {
		t.Error("Manifest.Names missing \"Canton\"")
	}

	c := Config()
	if c.Base != BasePath {
		t.Errorf("Base = %q, want %q", c.Base, BasePath)
	}
	if c.ManifestURL != ManifestPath+"?v="+Current().Version() {
		t.Errorf("ManifestURL = %q, want it versioned", c.ManifestURL)
	}
	for key, kind := range map[string]Kind{"page": Page, "class": Class, "instance": Instance} {
		if got := c.Defaults[key]; got != kind.Default() {
			t.Errorf("Defaults[%q] = %q, want %q", key, got, kind.Default())
		}
	}
}

// TestResolveOverride: a usable icon from the data beats every rule and every
// default; an unusable one is ignored as if absent.
func TestResolveOverride(t *testing.T) {
	withIcons(t, "Canton.svg", "Person.svg", "DefinedTerm.fallback.svg")

	canton := Subject{IRI: "https://schema.ld.admin.ch/Canton"}
	tests := []struct {
		name, override, want string
	}{
		{"absolute https URL", "https://img.example/zh.svg", "https://img.example/zh.svg"},
		{"plain http URL ignored (CSP)", "http://img.example/zh.png", BasePath + "Canton.svg"},
		{"site-relative path", "/static/img/custom/zh.svg", "/static/img/custom/zh.svg"},
		{"icon name", "Person", BasePath + "Person.svg"},
		{"icon file name", "Person.svg", BasePath + "Person.svg"},
		{"fallback icon name", "DefinedTerm.fallback", BasePath + "DefinedTerm.fallback.svg"},
		{"surrounding space", "  Person ", BasePath + "Person.svg"},
		{"unknown name ignored", "Nope", BasePath + "Canton.svg"},
		{"bare name of a fallback-only icon ignored", "DefinedTerm", BasePath + "Canton.svg"},
		{"javascript scheme ignored", "javascript:alert(1)", BasePath + "Canton.svg"},
		{"data URL ignored", "data:image/svg+xml,<svg/>", BasePath + "Canton.svg"},
		{"protocol-relative ignored", "//evil.example/x.svg", BasePath + "Canton.svg"},
		{"URL without host ignored", "https:///x.svg", BasePath + "Canton.svg"},
		{"empty ignored", "", BasePath + "Canton.svg"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := canton
			s.Override = tt.override
			if got := Resolve(s, Cell); got != tt.want {
				t.Errorf("Resolve(Override %q) = %q, want %q", tt.override, got, tt.want)
			}
		})
	}

	// It also beats a default: a resource with nothing else still gets it.
	if got := Resolve(Subject{IRI: "https://example.org/x/1", Override: "Person"}, Page); got != BasePath+"Person.svg" {
		t.Errorf("Resolve(miss + Override, Page) = %q, want the override", got)
	}
}
