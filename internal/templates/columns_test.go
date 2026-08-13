package templates

import (
	"testing"

	"hutzli.org/visoto/internal/column"
)

// The sync sparqlTable path has no handler to fold <sparql-column> declarations
// into its params — the dict is written inline in the page template — so the
// partial calls columnIconVar while rendering. These tests pin that lookup, which
// is otherwise only exercised by rendering a real page.

func TestColumnIconVar(t *testing.T) {
	index := map[string]map[string]column.Table{
		"pages/resource.html": {
			"outgoing": {{Var: "type", Icon: true}},
			"plain":    {{Var: "label"}}, // declared, but not as the icon column
		},
	}
	SetColumnLookup(func(set, id string) column.Table { return index[set][id] })
	t.Cleanup(func() { SetColumnLookup(nil) })

	tests := []struct {
		name    string
		set, id string
		want    string
	}{
		{"declared icon column", "pages/resource.html", "outgoing", "type"},
		{"table with no icon column", "pages/resource.html", "plain", ""},
		{"unknown id in a known set", "pages/resource.html", "nope", ""},
		{"id from another set is not visible", "pages/other.html", "outgoing", ""},
		// The async fragment path renders through a standalone partial set where
		// {{ templateSet }} is empty; there the handler supplies iconVar instead,
		// so an empty set must resolve to nothing rather than guessing.
		{"empty set", "", "outgoing", ""},
		{"empty id", "pages/resource.html", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := columnIconVar(tt.set, tt.id); got != tt.want {
				t.Errorf("columnIconVar(%q, %q) = %q, want %q", tt.set, tt.id, got, tt.want)
			}
		})
	}
}

// TestColumnIconVarWithoutLookup covers startup order: the func map is built
// before main registers the index, and a template rendered in between (or in a
// test that skips startup) must degrade to "no icon column" rather than panic.
func TestColumnIconVarWithoutLookup(t *testing.T) {
	SetColumnLookup(nil)
	if got := columnIconVar("pages/resource.html", "outgoing"); got != "" {
		t.Errorf("columnIconVar() = %q with no lookup registered, want \"\"", got)
	}
}
