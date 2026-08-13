package sparql

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"hutzli.org/visoto/internal/icon"
)

func TestBuildTypeQuery(t *testing.T) {
	iris := []string{
		"https://ld.admin.ch/canton/1",
		"http://schema.org/Person",
	}
	query := buildTypeQuery(iris)

	for _, want := range []string{
		"VALUES ?iri {",
		"<https://ld.admin.ch/canton/1>",
		"<http://schema.org/Person>",
		"?iri (rdf:type|owl:type) ?type .",
		"PREFIX rdf: <http://www.w3.org/1999/02/22-rdf-syntax-ns#>",
		"PREFIX owl: <http://www.w3.org/2002/07/owl#>",
	} {
		if !strings.Contains(query, want) {
			t.Errorf("buildTypeQuery() missing %q:\n%s", want, query)
		}
	}

	// The point of running this as a separate query is that it stays trivial for
	// the endpoint. Guard the properties that make it so: no OPTIONAL to plan
	// around, and no aggregation to unpack.
	for _, unwanted := range []string{"OPTIONAL", "GROUP BY", "GROUP_CONCAT", "SAMPLE", "MIN("} {
		if strings.Contains(query, unwanted) {
			t.Errorf("buildTypeQuery() should not contain %q:\n%s", unwanted, query)
		}
	}
}

func TestBuildTypeQueryEmpty(t *testing.T) {
	if got := buildTypeQuery(nil); got != "" {
		t.Errorf("buildTypeQuery(nil) = %q, want \"\"", got)
	}
}

// withIcons points the icon cache at fixtures so enrichWithIcons has something
// to resolve against, independent of static/img/resource.
func withIcons(t *testing.T, files ...string) {
	t.Helper()
	dir := t.TempDir()
	for _, name := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("<svg/>"), 0o600); err != nil {
			t.Fatalf("write fixture %s: %v", name, err)
		}
	}
	if err := icon.Init(dir, nil); err != nil {
		t.Fatalf("icon.Init: %v", err)
	}
}

func TestEnrichWithIcons(t *testing.T) {
	withIcons(t, "Canton.svg")

	result := QueryResult{
		Vars: []string{"canton", "total"},
		Bindings: []map[string]Binding{
			{
				"canton": {Type: "uri", Value: "https://ld.admin.ch/canton/1", DisplayText: "Zürich"},
				"total":  {Type: "literal", Value: "21.5", DisplayText: "21.5"},
			},
			{
				"canton": {Type: "uri", Value: "https://ld.admin.ch/canton/2", DisplayText: "Bern"},
				"total":  {Type: "literal", Value: "23.1", DisplayText: "23.1"},
			},
		},
	}
	typeMap := map[string][]string{
		"https://ld.admin.ch/canton/1": {"https://schema.ld.admin.ch/Canton"},
		"https://ld.admin.ch/canton/2": {"https://schema.ld.admin.ch/Canton"},
	}

	enrichWithIcons(&result, typeMap)

	want := icon.BasePath + "Canton.svg"
	if got := result.Icons["https://ld.admin.ch/canton/1"]; got != want {
		t.Errorf("Icons[canton/1] = %q, want %q", got, want)
	}
	if got := result.Icons["https://ld.admin.ch/canton/2"]; got != want {
		t.Errorf("Icons[canton/2] = %q, want %q", got, want)
	}
	// Literals are not IRIs and must never appear in the map.
	if _, ok := result.Icons["21.5"]; ok {
		t.Error("Icons contains a literal value")
	}
	if len(result.Icons) != 2 {
		t.Errorf("Icons has %d entries, want 2 (one per distinct IRI)", len(result.Icons))
	}
}

// TestEnrichWithIconsNoMatches is the "misses are inert" case: a result whose
// IRIs resolve to nothing carries no map at all, rather than a map of empties.
func TestEnrichWithIconsNoMatches(t *testing.T) {
	withIcons(t, "Canton.svg")

	result := QueryResult{
		Vars: []string{"thing"},
		Bindings: []map[string]Binding{
			{"thing": {Type: "uri", Value: "https://example.org/thing/1", DisplayText: "Thing"}},
		},
	}
	enrichWithIcons(&result, map[string][]string{
		"https://example.org/thing/1": {"http://example.org/Unknown"},
	})

	if result.Icons != nil {
		t.Errorf("Icons = %v, want nil when nothing resolves", result.Icons)
	}
}

// TestEnrichWithIconsUntyped covers the case an empty type map produces: the
// direct-name path still applies, so a class IRI in the column resolves even
// when the type query returned nothing for it.
func TestEnrichWithIconsUntyped(t *testing.T) {
	withIcons(t, "Canton.svg")

	result := QueryResult{
		Vars: []string{"class"},
		Bindings: []map[string]Binding{
			{"class": {Type: "uri", Value: "https://schema.ld.admin.ch/Canton", DisplayText: "Canton"}},
		},
	}
	enrichWithIcons(&result, nil)

	if got, want := result.Icons["https://schema.ld.admin.ch/Canton"], icon.BasePath+"Canton.svg"; got != want {
		t.Errorf("Icons[Canton] = %q, want %q", got, want)
	}
}

func TestTypeCache(t *testing.T) {
	iri := "http://example.com/type-cache"
	types := []string{"http://schema.org/Person"}

	if _, found := getCachedTypes(iri); found {
		t.Error("getCachedTypes() should miss for an unseen IRI")
	}

	setCachedTypes(iri, types)
	got, found := getCachedTypes(iri)
	if !found {
		t.Fatal("getCachedTypes() should hit after set")
	}
	if len(got) != 1 || got[0] != types[0] {
		t.Errorf("getCachedTypes() = %v, want %v", got, types)
	}

	// An untyped IRI caches as a hit holding nothing — that is an answer, and
	// caching it is what stops every page view re-asking about the same IRIs.
	untyped := "http://example.com/untyped"
	setCachedTypes(untyped, nil)
	got, found = getCachedTypes(untyped)
	if !found {
		t.Error("getCachedTypes() should hit for a cached-empty IRI")
	}
	if len(got) != 0 {
		t.Errorf("getCachedTypes() = %v, want empty", got)
	}
}

// TestTypeCacheIgnoresLanguage pins the property that makes this cache cheaper
// than the label cache: types do not vary by language, so one visitor's lookup
// serves every other language.
func TestTypeCacheIgnoresLanguage(t *testing.T) {
	iri := "http://example.com/lang-independent"
	setCachedTypes(iri, []string{"http://schema.org/Person"})

	if _, found := getCachedTypes(iri); !found {
		t.Error("type cache must be keyed by IRI alone, independent of language")
	}
}

func TestChunk(t *testing.T) {
	items := []string{"a", "b", "c", "d", "e"}

	got := chunk(items, 2)
	if len(got) != 3 {
		t.Fatalf("chunk() produced %d batches, want 3", len(got))
	}
	if len(got[2]) != 1 || got[2][0] != "e" {
		t.Errorf("last batch = %v, want [e]", got[2])
	}
	if n := len(chunk(nil, 10)); n != 0 {
		t.Errorf("chunk(nil) produced %d batches, want 0", n)
	}
	if got := chunk(items, 10); len(got) != 1 || len(got[0]) != 5 {
		t.Errorf("chunk() with size > len should produce one full batch, got %v", got)
	}
}
