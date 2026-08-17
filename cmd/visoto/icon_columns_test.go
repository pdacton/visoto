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

// TestGroupColumnsAreDeclared is the same guard for the groupBy migration, and it
// matters more: no template declared group before it, so all three of these tables
// changed from "grouped by a param" to "grouped by a declaration" at once. A
// container attached to the wrong id leaves the page rendering ungrouped with no
// error anywhere — the exact silent failure this file exists to catch.
func TestGroupColumnsAreDeclared(t *testing.T) {
	if err := initAsyncIndex("../../templates"); err != nil {
		t.Fatalf("initAsyncIndex(): %v", err)
	}

	tests := []struct {
		name    string
		set, id string
		want    string
	}{
		{"named graphs, attributes", "pages/namedGraphs.html", "namedGraphsList", "graph"},
		{"named graphs, classes", "pages/namedGraphs.html", "namedGraphs", "graph"},
		// Same column carries icon AND group — the flags are independent, and one
		// element describing one column keeps Find() and GroupVar() in agreement.
		{"red list, icon and group on one column", "pages/environment.html", "envRedList", "group"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := findColumns(tt.set, tt.id).GroupVar(); got != tt.want {
				t.Errorf("findColumns(%q, %q).GroupVar() = %q, want %q", tt.set, tt.id, got, tt.want)
			}
		})
	}

	// The environment table's one element must satisfy both roles at once.
	if got := findColumns("pages/environment.html", "envRedList").IconVars(); got != "group" {
		t.Errorf("envRedList IconVars() = %q, want %q — icon and group must coexist on one element", got, "group")
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

// TestNoRemovedTableParamsRemain guards the other half of the migration: the
// iconVar=, badgeVar= and groupBy= dict parameters are gone from every page
// template, so nobody re-adds a table that declares a row icon, a badge or a
// grouping the old way and silently gets none of them. All three roles are now
// declaration-only, on sync and async tables alike, so a leftover param is dead
// text that looks like it works.
func TestNoRemovedTableParamsRemain(t *testing.T) {
	// Matches the dict keys as templates write them, in quotes. The plural
	// .iconVars/.badgeVars the handlers fold in are a different string and
	// deliberately not matched.
	banned := regexp.MustCompile(`"(icon|badge)Var"|"groupBy"`)

	err := filepath.WalkDir("../../templates", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".html") {
			return err
		}
		// The partials are the other end of the plumbing: sparql-table.html reads
		// the folded-in keys and names "groupBy" in its config island, which is what
		// carries the resolved declaration to the frontend. Only callers are banned.
		if strings.Contains(filepath.ToSlash(path), "/partials/") {
			return nil
		}
		body, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		for i, line := range strings.Split(string(body), "\n") {
			if banned.MatchString(line) {
				t.Errorf("%s:%d declares a removed param — use <sparql-column … icon|badge|group> instead:\n  %s",
					path, i+1, strings.TrimSpace(line))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking templates: %v", err)
	}
}
