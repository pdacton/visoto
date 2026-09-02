package tree

import (
	"strings"
	"testing"
)

func minimalRoles() map[string]string {
	return map[string]string{
		RoleRoots:    `SELECT ?node ?label WHERE { ?node skos:topConceptOf <x:s> ; skos:prefLabel ?label }`,
		RoleChildren: `SELECT ?node ?label WHERE { ?node skos:broader ?parent ; skos:prefLabel ?label }`,
	}
}

func TestValidateRolesRequiresRootsAndChildren(t *testing.T) {
	for _, missing := range []string{RoleRoots, RoleChildren} {
		roles := minimalRoles()
		delete(roles, missing)
		err := ValidateRoles("t", roles)
		if err == nil {
			t.Fatalf("missing %q accepted", missing)
		}
		if !strings.Contains(err.Error(), missing) {
			t.Errorf("error %q does not name the missing role %q", err, missing)
		}
	}
}

func TestValidateRolesRejectsUnknownRole(t *testing.T) {
	roles := minimalRoles()
	roles["ancestors"] = `SELECT ?node WHERE { ?node skos:broader ?parent }`
	err := ValidateRoles("t", roles)
	if err == nil {
		t.Fatal("unknown role accepted")
	}
	if !strings.Contains(err.Error(), "ancestors") || !strings.Contains(err.Error(), RoleParents) {
		t.Errorf("error %q should name the bad role and list the valid ones", err)
	}
}

func TestValidateRolesAcceptsAllKnown(t *testing.T) {
	roles := minimalRoles()
	roles[RoleParents] = `SELECT ?node ?parent WHERE { ?node skos:broader ?parent }`
	roles[RoleSearch] = `SELECT ?node WHERE { ?node rdfs:label ?l FILTER(CONTAINS(?l, ?__token__)) }`
	roles[RoleNodeData] = `SELECT ?node ?extra WHERE { ?node ex:extra ?extra }`
	if err := ValidateRoles("t", roles); err != nil {
		t.Fatalf("valid role set rejected: %v", err)
	}
}

// The gap the plan identified: a children query that binds ?parent itself makes the
// server's VALUES intersect rather than parameterise, silently returning the wrong
// level with no error.
func TestValidateReservedVarsRejectsBoundParent(t *testing.T) {
	cases := []struct{ name, query string }{
		{"BIND AS", `SELECT ?node WHERE { ?node skos:broader ?parent . BIND(<x:root> AS ?parent) }`},
		{"VALUES", `SELECT ?node WHERE { VALUES ?parent { <x:a> <x:b> } ?node skos:broader ?parent }`},
		{"VALUES tuple", `SELECT ?node WHERE { VALUES (?parent ?other) { (<x:a> 1) } ?node skos:broader ?parent }`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateReservedVars("t", RoleChildren, tc.query)
			if err == nil {
				t.Fatal("a query binding ?parent was accepted")
			}
			if !strings.Contains(err.Error(), "parent") {
				t.Errorf("error %q does not name ?parent", err)
			}
		})
	}
}

func TestValidateReservedVarsAcceptsFreeParent(t *testing.T) {
	ok := []string{
		`SELECT ?node ?label WHERE { ?node skos:broader ?parent ; skos:prefLabel ?label }`,
		// FILTER reads the variable; it does not bind it.
		`SELECT ?node WHERE { ?node skos:broader ?p . FILTER(?p = ?parent) }`,
		// A projected ?parent is read, not bound.
		`SELECT ?node ?parent WHERE { ?node skos:broader ?parent }`,
	}
	for _, q := range ok {
		if err := ValidateReservedVars("t", RoleChildren, q); err != nil {
			t.Errorf("valid children query rejected: %v\n%s", err, q)
		}
	}
}

func TestValidateReservedVarsRequiresTheVariable(t *testing.T) {
	// A children query with no ?parent cannot be scoped to the expanding node: it
	// would return the same level regardless of which node was opened.
	err := ValidateReservedVars("t", RoleChildren, `SELECT ?node WHERE { ?node a skos:Concept }`)
	if err == nil {
		t.Fatal("a children query without ?parent was accepted")
	}
	if !strings.Contains(err.Error(), "never mentions") {
		t.Errorf("unexpected error: %v", err)
	}
}

// ?parentLabel must not satisfy the ?parent requirement.
func TestValidateReservedVarsPrefixIsNotAMatch(t *testing.T) {
	err := ValidateReservedVars("t", RoleChildren, `SELECT ?node WHERE { ?node ex:p ?parentLabel }`)
	if err == nil {
		t.Fatal("?parentLabel was accepted as ?parent")
	}
}

func TestValidateReservedVarsSearchNeedsToken(t *testing.T) {
	if err := ValidateReservedVars("t", RoleSearch, `SELECT ?node WHERE { ?node a skos:Concept }`); err == nil {
		t.Fatal("a search query ignoring ?__token__ was accepted")
	}
	q := `SELECT ?node WHERE { ?node rdfs:label ?l FILTER(CONTAINS(LCASE(?l), LCASE(?__token__))) }`
	if err := ValidateReservedVars("t", RoleSearch, q); err != nil {
		t.Errorf("valid search query rejected: %v", err)
	}
}

func TestValidateReservedVarsRootsTakesNone(t *testing.T) {
	// The roots query has no bound input, so nothing is reserved in it.
	if err := ValidateReservedVars("t", RoleRoots, `SELECT ?node WHERE { ?node skos:topConceptOf <x:s> }`); err != nil {
		t.Errorf("roots query rejected: %v", err)
	}
}

func TestWarnOrderByUnprojected(t *testing.T) {
	cases := []struct {
		name  string
		query string
		want  []string
	}{
		{
			name:  "the trap",
			query: `SELECT ?node WHERE { ?node skos:broader ?parent } ORDER BY ?label`,
			want:  []string{"label"},
		},
		{
			name:  "projected, no warning",
			query: `SELECT ?node ?label WHERE { ?node skos:broader ?parent } ORDER BY ?label`,
			want:  nil,
		},
		{
			name:  "no ORDER BY",
			query: `SELECT ?node WHERE { ?node skos:broader ?parent }`,
			want:  nil,
		},
		{
			name:  "SELECT * projects everything",
			query: `SELECT * WHERE { ?node skos:broader ?parent } ORDER BY ?label`,
			want:  nil,
		},
		{
			name:  "DISTINCT",
			query: `SELECT DISTINCT ?node WHERE { ?node skos:broader ?parent } ORDER BY ?label`,
			want:  []string{"label"},
		},
		{
			name:  "ORDER BY with a function",
			query: `SELECT ?node WHERE { ?node skos:broader ?parent } ORDER BY LCASE(?label)`,
			want:  []string{"label"},
		},
		{
			name:  "ORDER BY before LIMIT",
			query: `SELECT ?node WHERE { ?node skos:broader ?parent } ORDER BY ?label LIMIT 200`,
			want:  []string{"label"},
		},
		{
			name:  "prefix of a projected var is still missing",
			query: `SELECT ?node ?labelText WHERE { ?node skos:broader ?parent } ORDER BY ?label`,
			want:  []string{"label"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := WarnOrderByUnprojected(tc.query)
			if len(got) != len(tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("got %v, want %v", got, tc.want)
				}
			}
		})
	}
}

func TestValidateReturnsWarningsNotErrors(t *testing.T) {
	roles := minimalRoles()
	roles[RoleChildren] = `SELECT ?node WHERE { ?node skos:broader ?parent } ORDER BY ?label`
	warnings, err := Validate("t", roles)
	if err != nil {
		t.Fatalf("an ORDER BY warning must not fail the boot: %v", err)
	}
	if len(warnings) != 1 {
		t.Fatalf("got %d warnings, want 1: %v", len(warnings), warnings)
	}
	if !strings.Contains(warnings[0], "label") {
		t.Errorf("warning does not name the variable: %q", warnings[0])
	}
}

func TestValidateFailsOnFatalBeforeWarning(t *testing.T) {
	roles := minimalRoles()
	delete(roles, RoleChildren)
	if _, err := Validate("t", roles); err == nil {
		t.Fatal("missing children accepted")
	}
}

func TestRequiresParents(t *testing.T) {
	cases := []struct {
		focus, search, flatOnly, want bool
	}{
		{focus: true, want: true},
		{search: true, want: true},
		{search: true, flatOnly: true, want: false},
		{focus: true, search: true, flatOnly: true, want: true},
		{want: false},
	}
	for _, tc := range cases {
		if got := RequiresParents(tc.focus, tc.search, tc.flatOnly); got != tc.want {
			t.Errorf("RequiresParents(%v,%v,%v) = %v, want %v", tc.focus, tc.search, tc.flatOnly, got, tc.want)
		}
	}
}
