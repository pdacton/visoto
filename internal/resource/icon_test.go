package resource

import (
	"os"
	"path/filepath"
	"testing"

	"hutzli.org/visoto/internal/icon"
	"hutzli.org/visoto/internal/parser"
	"hutzli.org/visoto/internal/sparql"
)

func withIcons(t *testing.T, files ...string) {
	t.Helper()
	dir := t.TempDir()
	for _, name := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("<svg/>"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := icon.Init(dir, nil); err != nil {
		t.Fatal(err)
	}
}

func cantonPage(override string) parser.TemplateData {
	return parser.TemplateData{
		ResourceIRI:  "https://ld.admin.ch/canton/1",
		IconOverride: override,
		QueryResults: map[string]sparql.QueryResult{
			"pageClasses": {Bindings: []map[string]sparql.Binding{
				{"class": {Type: "uri", Value: "https://schema.ld.admin.ch/Canton"}},
			}},
		},
	}
}

func TestGetIconForResource(t *testing.T) {
	withIcons(t, "Canton.svg", "Person.svg")

	tests := []struct {
		name string
		data interface{}
		want string
	}{
		{"by type", cantonPage(""), icon.BasePath + "Canton.svg"},
		{"override wins", cantonPage("Person"), icon.BasePath + "Person.svg"},
		{"unusable override ignored", cantonPage("javascript:alert(1)"), icon.BasePath + "Canton.svg"},
		{"no types, no override", parser.TemplateData{ResourceIRI: "https://example.org/x"}, icon.Page.Default()},
		{"not a resource page", map[string]string{}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := GetIconForResource(tt.data); got != tt.want {
				t.Errorf("GetIconForResource() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestIconOverrideForResource: the layout uses this to decide whether a
// template's own pageIcon block is bypassed, so it must be "" unless the
// resource's own icon is usable.
func TestIconOverrideForResource(t *testing.T) {
	withIcons(t, "Canton.svg", "Person.svg")

	if got := IconOverrideForResource(cantonPage("")); got != "" {
		t.Errorf("no override: got %q, want \"\"", got)
	}
	if got := IconOverrideForResource(cantonPage("https://img.example/zh.svg")); got != "https://img.example/zh.svg" {
		t.Errorf("URL override: got %q", got)
	}
	if got := IconOverrideForResource(cantonPage("Nope")); got != "" {
		t.Errorf("unknown name: got %q, want \"\" (the template's own icon must stay)", got)
	}
	if got := IconOverrideForResource("not template data"); got != "" {
		t.Errorf("non-resource page: got %q", got)
	}
}
