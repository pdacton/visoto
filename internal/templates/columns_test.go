package templates

import (
	"testing"

	"hutzli.org/visoto/internal/column"
)

// The sync sparqlTable path has no handler to fold <sparql-column> declarations
// into its params — the dict is written inline in the page template — so the
// partial calls columnIconVars / columnBadgeVars while rendering. These tests pin
// those lookups, which are otherwise only exercised by rendering a real page.

func TestColumnRoleLookups(t *testing.T) {
	index := map[string]map[string]column.Table{
		"pages/resource.html": {
			"outgoing": {{Var: "type", Icon: true}},
			"plain":    {{Var: "label"}}, // declared, but carries no rendering role
			// Both roles are comma-separated lists: a table may icon several IRI
			// columns and badge more than one (a status AND a version).
			"many": {
				{Var: "cube", Icon: true},
				{Var: "status", Badge: true},
				{Var: "dimension", Icon: true},
				{Var: "superseded", Badge: true},
			},
		},
	}
	SetColumnLookup(func(set, id string) column.Table { return index[set][id] })
	t.Cleanup(func() { SetColumnLookup(nil) })

	tests := []struct {
		name    string
		lookup  func(set, id string) string
		set, id string
		want    string
	}{
		{"declared icon column", columnIconVars, "pages/resource.html", "outgoing", "type"},
		{"table with no icon column", columnIconVars, "pages/resource.html", "plain", ""},
		{"every icon column, in document order", columnIconVars, "pages/resource.html", "many", "cube,dimension"},
		{"every badge column, in document order", columnBadgeVars, "pages/resource.html", "many", "status,superseded"},
		{"icon declaration is not a badge", columnBadgeVars, "pages/resource.html", "outgoing", ""},
		{"unknown id in a known set", columnIconVars, "pages/resource.html", "nope", ""},
		{"id from another set is not visible", columnIconVars, "pages/other.html", "outgoing", ""},
		// The async fragment path renders through a standalone partial set where
		// {{ templateSet }} is empty; there the handler folds the declarations into
		// the params instead, so an empty set must resolve to nothing rather than guess.
		{"empty set", columnIconVars, "", "outgoing", ""},
		{"empty id", columnIconVars, "pages/resource.html", "", ""},
		{"empty set, badge role", columnBadgeVars, "", "many", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.lookup(tt.set, tt.id); got != tt.want {
				t.Errorf("lookup(%q, %q) = %q, want %q", tt.set, tt.id, got, tt.want)
			}
		})
	}
}

// TestColumnLookupsWithoutRegistration covers startup order: the func map is built
// before main registers the index, and a template rendered in between (or in a
// test that skips startup) must degrade to "no such column" rather than panic.
func TestColumnLookupsWithoutRegistration(t *testing.T) {
	SetColumnLookup(nil)
	if got := columnIconVars("pages/resource.html", "outgoing"); got != "" {
		t.Errorf("columnIconVars() = %q with no lookup registered, want \"\"", got)
	}
	if got := columnBadgeVars("pages/resource.html", "outgoing"); got != "" {
		t.Errorf("columnBadgeVars() = %q with no lookup registered, want \"\"", got)
	}
}
