package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The iconVar dict parameter was replaced by <sparql-column … icon>, and the
// migration touched ~50 templates. initAsyncIndex already fails startup on a
// for= that names no query, but it cannot catch the opposite mistake: a
// declaration that is well-formed yet attached to the wrong id, which costs a
// table its icons silently. These pin a representative table of each kind.
func TestIconColumnsAreDeclared(t *testing.T) {
	if err := initAsyncIndex("../../templates"); err != nil {
		t.Fatalf("initAsyncIndex(): %v", err)
	}

	tests := []struct {
		name    string
		set, id string
		want    string
	}{
		// Async, cube-shaped: the case the feature exists for — ?canton binds
		// instance IRIs, so the icon can only come from rdf:type.
		{"async cube table", "pages/energy.html", "energyCantonPrices", "canton"},
		// Async class-instance table: the column holds a class IRI, which already
		// worked by name before this change and must not regress.
		{"async class instances", "classes/default.html", "instances", "class"},
		// Sync table: no handler folds the declaration in, so this one is only
		// reachable through the columnIconVars lookup at render time.
		{"sync relationships", "pages/resource.html", "outgoing", "type"},
		{"sync relationships, incoming", "pages/resource.html", "incoming", "type"},
		// Declared in a layout rather than beside its own page.
		{"layout-declared table", "classes/default.html", "schemaSubclasses", "subclass"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := findColumns(tt.set, tt.id).IconVars(); got != tt.want {
				t.Errorf("findColumns(%q, %q).IconVars() = %q, want %q", tt.set, tt.id, got, tt.want)
			}
		})
	}
}

// TestQueryOptionsGatesTheTypeQuery pins what makes icons affordable: the rdf:type
// round trip is requested only when some column will actually render an icon.
func TestQueryOptionsGatesTheTypeQuery(t *testing.T) {
	if got := queryOptions(""); got != nil {
		t.Errorf("queryOptions(\"\") = %v, want nil (no type query without an icon column)", got)
	}
	if got := queryOptions("canton"); len(got) != 1 {
		t.Errorf("queryOptions(%q) returned %d options, want 1", "canton", len(got))
	}
	// Several icon columns are one list, and still one type query.
	if got := queryOptions("cube,dimension"); len(got) != 1 {
		t.Errorf("queryOptions(%q) returned %d options, want 1", "cube,dimension", len(got))
	}
}

// TestNoIconOrBadgeVarParamsRemain guards the other half of the migration: the
// iconVar= and badgeVar= dict parameters are gone from every template, so nobody
// re-adds a table that declares a row icon or a badge the old way and silently
// gets neither. Both roles are now declaration-only, on sync and async tables
// alike, so a leftover param is dead text that looks like it works.
func TestNoIconOrBadgeVarParamsRemain(t *testing.T) {
	// Matches the dict key as templates write it: "iconVar" / "badgeVar" in quotes.
	// The plural .iconVars/.badgeVars the handlers fold in are a different string
	// and deliberately not matched.
	banned := regexp.MustCompile(`"(icon|badge)Var"`)

	err := filepath.WalkDir("../../templates", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".html") {
			return err
		}
		body, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		for i, line := range strings.Split(string(body), "\n") {
			if banned.MatchString(line) {
				t.Errorf("%s:%d declares a removed param — use <sparql-column … icon|badge> instead:\n  %s",
					path, i+1, strings.TrimSpace(line))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking templates: %v", err)
	}
}
