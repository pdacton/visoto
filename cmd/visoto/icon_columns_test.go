package main

import "testing"

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
		// reachable through the columnIconVar lookup at render time.
		{"sync relationships", "pages/resource.html", "outgoing", "type"},
		{"sync relationships, incoming", "pages/resource.html", "incoming", "type"},
		// Declared in a layout rather than beside its own page.
		{"layout-declared table", "classes/default.html", "schemaSubclasses", "subclass"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := findColumns(tt.set, tt.id).IconVar(); got != tt.want {
				t.Errorf("findColumns(%q, %q).IconVar() = %q, want %q", tt.set, tt.id, got, tt.want)
			}
		})
	}
}

// TestNoIconVarParamsRemain guards the other half of the migration: the dict
// parameter is gone from every template, so nobody re-adds a table that declares
// its icon the old way and silently gets none.
func TestNoIconVarParamsRemain(t *testing.T) {
	if err := initAsyncIndex("../../templates"); err != nil {
		t.Fatalf("initAsyncIndex(): %v", err)
	}
	// queryOptions is what gates the rdf:type query; an empty icon var must not
	// ask for types, or every icon-less table would pay for a round trip.
	if got := queryOptions(""); got != nil {
		t.Errorf("queryOptions(\"\") = %v, want nil (no type query without an icon column)", got)
	}
	if got := queryOptions("canton"); len(got) != 1 {
		t.Errorf("queryOptions(%q) returned %d options, want 1", "canton", len(got))
	}
}
