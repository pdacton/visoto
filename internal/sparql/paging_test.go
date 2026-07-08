package sparql

import (
	"strings"
	"testing"
)

func TestStripTrailingLimitOffset(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"no limit", "SELECT ?s WHERE { ?s a <C> }", "SELECT ?s WHERE { ?s a <C> }"},
		{"trailing limit", "SELECT ?s WHERE { ?s a <C> } LIMIT 20000", "SELECT ?s WHERE { ?s a <C> }"},
		{"limit and offset", "SELECT ?s WHERE { ?s a <C> } LIMIT 100 OFFSET 50", "SELECT ?s WHERE { ?s a <C> }"},
		{"trailing whitespace", "SELECT ?s WHERE { ?s a <C> } LIMIT 5\n\n", "SELECT ?s WHERE { ?s a <C> }"},
		{"lowercase", "SELECT ?s WHERE { ?s a <C> } limit 5", "SELECT ?s WHERE { ?s a <C> }"},
		{"limit inside body is kept", "SELECT ?s WHERE { { SELECT ?s { ?s a <C> } LIMIT 5 } }",
			"SELECT ?s WHERE { { SELECT ?s { ?s a <C> } LIMIT 5 } }"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := StripTrailingLimitOffset(tt.in); got != tt.want {
				t.Errorf("StripTrailingLimitOffset() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestStringLiteral(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"migros", `"migros"`},
		{`a"b`, `"a\"b"`},
		{`a\b`, `"a\\b"`},
		{"line1\nline2", `"line1\nline2"`},
	}
	for _, tt := range tests {
		if got := StringLiteral(tt.in); got != tt.want {
			t.Errorf("StringLiteral(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestMembershipTriplePattern(t *testing.T) {
	re := MembershipTriplePattern("org")
	matches := []string{
		"?org a <https://schema.ld.admin.ch/ZefixOrganisation> .",
		"?org rdf:type <https://schema.ld.admin.ch/ZefixOrganisation> .",
		"?org <http://www.w3.org/1999/02/22-rdf-syntax-ns#type> <https://x/C> .",
	}
	for _, m := range matches {
		if !re.MatchString(m) {
			t.Errorf("expected pattern to match %q", m)
		}
	}
	nonMatches := []string{
		"?org a ?class .",                    // class is a variable, not a literal IRI
		"?other a <https://x/C> .",           // different key variable
		"?organization a <https://x/C> .",    // longer var name must not match ?org
	}
	for _, m := range nonMatches {
		if re.MatchString(m) {
			t.Errorf("expected pattern NOT to match %q", m)
		}
	}
}

func TestMembershipBody(t *testing.T) {
	class := "https://schema.ld.admin.ch/ZefixOrganisation"

	// Browse mode: bare membership triple, no filter.
	browse := MembershipBody(class, "org", "", "http://schema.org/name")
	if want := "?org a <" + class + ">"; browse != want {
		t.Errorf("browse body = %q, want %q", browse, want)
	}

	// Search with name property: matches the property OR the key IRI, lowercased.
	search := MembershipBody(class, "org", "Migros", "http://schema.org/name")
	for _, sub := range []string{
		"OPTIONAL { ?org <http://schema.org/name> ?__match . }",
		`CONTAINS(LCASE(STR(?__match)), "migros")`,
		`CONTAINS(LCASE(STR(?org)), "migros")`,
	} {
		if !strings.Contains(search, sub) {
			t.Errorf("search body missing %q\n got: %s", sub, search)
		}
	}

	// Search with no name property: match the key IRI string alone (no OPTIONAL).
	iriOnly := MembershipBody(class, "org", "abc", "")
	if strings.Contains(iriOnly, "OPTIONAL") {
		t.Errorf("iri-only search should not emit OPTIONAL: %s", iriOnly)
	}
	if !strings.Contains(iriOnly, `CONTAINS(LCASE(STR(?org)), "abc")`) {
		t.Errorf("iri-only search missing key-IRI filter: %s", iriOnly)
	}
}

func TestBuildPagedQuery_FastPath(t *testing.T) {
	class := "https://schema.ld.admin.ch/ZefixOrganisation"
	declared := "SELECT ?org ?name WHERE { ?org a ?? . OPTIONAL { ?org <http://schema.org/name> ?name } } LIMIT 20000"

	got := BuildPagedQuery(declared, class, "org", "", "http://schema.org/name", 1000, 2000)

	// ?? substituted.
	if strings.Contains(got, "??") {
		t.Errorf("?? placeholder not substituted: %s", got)
	}
	// Membership triple replaced by a paginated subquery.
	if !strings.Contains(got, "ORDER BY ?org LIMIT 1000 OFFSET 2000") {
		t.Errorf("expected paginated subquery, got: %s", got)
	}
	// The declared trailing LIMIT is gone; the OPTIONAL is preserved.
	if strings.Contains(got, "LIMIT 20000") {
		t.Errorf("declared LIMIT not stripped: %s", got)
	}
	if !strings.Contains(got, "OPTIONAL { ?org <http://schema.org/name> ?name }") {
		t.Errorf("OPTIONAL body not preserved: %s", got)
	}
	// Fast path replaces in place — no outer "SELECT * WHERE { {subquery}\n{declared} }" wrapper.
	if strings.Contains(got, "SELECT * WHERE") {
		t.Errorf("fast path should not use the wrapping fallback: %s", got)
	}
}

func TestBuildPagedQuery_Fallback(t *testing.T) {
	class := "https://x/C"
	// Membership expressed via a variable (BIND) — the regex won't match, so the
	// whole query must be wrapped with deterministic paging on the key var.
	declared := "SELECT ?inst WHERE { BIND(?? AS ?class) ?inst a ?class }"

	got := BuildPagedQuery(declared, class, "inst", "", "", 500, 0)

	if !strings.Contains(got, "SELECT * WHERE") {
		t.Errorf("expected wrapping fallback, got: %s", got)
	}
	if !strings.Contains(got, "ORDER BY ?inst LIMIT 500 OFFSET 0") {
		t.Errorf("fallback still applies paging on key var, got: %s", got)
	}
	if strings.Contains(got, "??") {
		t.Errorf("?? placeholder not substituted in fallback: %s", got)
	}
}

func TestBuildPagedQuery_SearchInjected(t *testing.T) {
	class := "https://x/C"
	declared := "SELECT ?org WHERE { ?org a ?? }"

	got := BuildPagedQuery(declared, class, "org", "Migros", "http://schema.org/name", 100, 0)
	if !strings.Contains(got, `CONTAINS(LCASE(STR(?org)), "migros")`) {
		t.Errorf("search filter not injected into paged query: %s", got)
	}
}

func TestDistinctKeyCount(t *testing.T) {
	result := QueryResult{
		Bindings: []map[string]Binding{
			{"org": {Value: "A"}, "name": {Value: "n1"}},
			{"org": {Value: "A"}, "name": {Value: "n2"}}, // same key, multi-valued OPTIONAL
			{"org": {Value: "B"}},
			{"name": {Value: "orphan"}}, // no key binding
		},
	}
	if got := DistinctKeyCount(result, "org"); got != 2 {
		t.Errorf("DistinctKeyCount = %d, want 2", got)
	}
	if got := DistinctKeyCount(QueryResult{}, "org"); got != 0 {
		t.Errorf("DistinctKeyCount(empty) = %d, want 0", got)
	}
}
