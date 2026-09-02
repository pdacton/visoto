package tree

import (
	"regexp"
	"strings"
	"testing"
)

const childrenDecl = `SELECT ?node ?label WHERE { ?node skos:broader ?parent ; skos:prefLabel ?label }`

func TestChildrenQueryBindsWithValues(t *testing.T) {
	q, err := ChildrenQuery(childrenDecl, "", "http://example.org/c1", 0, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(q, `VALUES ?parent { <http://example.org/c1> }`) {
		t.Errorf("no VALUES binding in:\n%s", q)
	}
	// The declared text must survive verbatim — the IRI is bound, never substituted
	// for the variable.
	if !strings.Contains(q, "?node skos:broader ?parent") {
		t.Errorf("declared pattern was rewritten:\n%s", q)
	}
	if strings.Contains(q, "?node skos:broader <http://example.org/c1>") {
		t.Errorf("IRI was substituted for the variable instead of bound:\n%s", q)
	}
}

// The security boundary: a hostile IRI must be rejected, not escaped into the
// query. A ">" would close the term early and let the rest parse as syntax.
func TestQueryBuildersRejectHostileIRIs(t *testing.T) {
	hostile := []string{
		`http://e.org/x> . ?s ?p ?o . #`,
		`http://e.org/a b`,
		"http://e.org/\nSELECT",
		`relative/path`,
		``,
	}
	for _, iri := range hostile {
		t.Run(iri, func(t *testing.T) {
			if _, err := ChildrenQuery(childrenDecl, "", iri, 0, 0); err == nil {
				t.Errorf("ChildrenQuery accepted hostile IRI %q", iri)
			}
			if _, err := ParentsQuery(`SELECT ?node ?parent WHERE { ?node skos:broader ?parent }`, "", []string{iri}); err == nil {
				t.Errorf("ParentsQuery accepted hostile IRI %q", iri)
			}
		})
	}
}

func TestChildrenQueryRejectsHostilePageIRI(t *testing.T) {
	decl := `SELECT ?node WHERE { ?node skos:inScheme ?? ; skos:broader ?parent }`
	_, err := ChildrenQuery(decl, `http://e.org/s> . ?x ?y ?z . #`, "http://example.org/c1", 0, 0)
	if err == nil {
		t.Fatal("hostile page IRI accepted")
	}
}

func TestSubstitutesPageIRI(t *testing.T) {
	decl := `SELECT ?node WHERE { ?node skos:inScheme ?? ; skos:broader ?parent }`
	q, err := ChildrenQuery(decl, "http://example.org/scheme", "http://example.org/c1", 0, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(q, "??") {
		t.Errorf("?? not substituted:\n%s", q)
	}
	if !strings.Contains(q, "<http://example.org/scheme>") {
		t.Errorf("page IRI missing:\n%s", q)
	}
}

func TestMissingPageIRIIsAnErrorOnlyWhenUsed(t *testing.T) {
	withPlaceholder := `SELECT ?node WHERE { ?node skos:inScheme ?? ; skos:broader ?parent }`
	if _, err := ChildrenQuery(withPlaceholder, "", "http://example.org/c1", 0, 0); err == nil {
		t.Error("a query using ?? with no page IRI was accepted")
	}
	if _, err := ChildrenQuery(childrenDecl, "", "http://example.org/c1", 0, 0); err != nil {
		t.Errorf("a query not using ?? should not need a page IRI: %v", err)
	}
}

// Batching is what makes the recursive ancestor walk affordable: one query per
// level, not one per node.
func TestParentsQueryBatchesNodes(t *testing.T) {
	nodes := []string{"http://e.org/a", "http://e.org/b", "http://e.org/c"}
	q, err := ParentsQuery(`SELECT ?node ?parent WHERE { ?node skos:broader ?parent }`, "", nodes)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n := strings.Count(q, "VALUES ?node"); n != 1 {
		t.Errorf("got %d VALUES clauses, want exactly 1:\n%s", n, q)
	}
	for _, iri := range nodes {
		if !strings.Contains(q, "<"+iri+">") {
			t.Errorf("node %s missing from batch:\n%s", iri, q)
		}
	}
}

func TestParentsQueryRejectsEmptyBatch(t *testing.T) {
	if _, err := ParentsQuery(`SELECT ?node ?parent WHERE { ?node skos:broader ?parent }`, "", nil); err == nil {
		t.Error("empty node batch accepted")
	}
}

// A search term is the one free-text input. It must be contained as a literal,
// never spliced into query syntax.
func TestSearchQueryContainsHostileTerms(t *testing.T) {
	decl := `SELECT ?node WHERE { ?node rdfs:label ?l FILTER(CONTAINS(LCASE(?l), LCASE(?__token__))) }`
	hostile := []struct{ name, term string }{
		{"quote break-out", `x" ) } UNION { ?s ?p ?o . FILTER("`},
		{"backslash", `back\slash`},
		{"newline", "line1\nline2"},
	}
	for _, tc := range hostile {
		t.Run(tc.name, func(t *testing.T) {
			q, err := SearchQuery(decl, "", tc.term, 0)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			// Every quote inside the bound literal must be escaped. Find the VALUES
			// literal and check it is one balanced, escaped string.
			m := regexp.MustCompile(`VALUES \?__token__ \{ (".*") \}`).FindStringSubmatch(q)
			if m == nil {
				t.Fatalf("no literal binding found in:\n%s", q)
			}
			lit := m[1]
			// Strip escaped pairs; what remains must be exactly the two delimiters.
			stripped := strings.ReplaceAll(lit, `\\`, "")
			stripped = strings.ReplaceAll(stripped, `\"`, "")
			if strings.Count(stripped, `"`) != 2 {
				t.Errorf("unbalanced quotes — term escaped into syntax: %q", lit)
			}
			if strings.Contains(lit, "\n") {
				t.Errorf("raw newline survived into the literal: %q", lit)
			}
		})
	}
}

func TestSearchQueryRejectsEmptyTerm(t *testing.T) {
	decl := `SELECT ?node WHERE { ?node rdfs:label ?l FILTER(CONTAINS(?l, ?__token__)) }`
	for _, term := range []string{"", "   "} {
		if _, err := SearchQuery(decl, "", term, 0); err == nil {
			t.Errorf("empty term %q accepted", term)
		}
	}
}

// A declared trailing LIMIT is stripped so the caller's per-level limit is the one
// that applies; otherwise the smaller of the two would silently win.
func TestDeclaredLimitIsMovedOutside(t *testing.T) {
	decl := childrenDecl + " ORDER BY ?label LIMIT 50"
	q, err := ChildrenQuery(decl, "", "http://example.org/c1", 200, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(q, "LIMIT 50") {
		t.Errorf("declared LIMIT survived:\n%s", q)
	}
	if !strings.HasSuffix(strings.TrimSpace(q), "LIMIT 200") {
		t.Errorf("caller limit not applied at the end:\n%s", q)
	}
}

func TestLimitAndOffset(t *testing.T) {
	q, err := ChildrenQuery(childrenDecl, "", "http://example.org/c1", 200, 400)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(q, "LIMIT 200") || !strings.Contains(q, "OFFSET 400") {
		t.Errorf("limit/offset missing:\n%s", q)
	}
	// No limit means no clause at all.
	q, _ = ChildrenQuery(childrenDecl, "", "http://example.org/c1", 0, 0)
	if strings.Contains(q, "LIMIT") {
		t.Errorf("unrequested LIMIT added:\n%s", q)
	}
}

// REGRESSION. The binding must land INSIDE the declared query's own WHERE.
//
// Wrapping it instead — SELECT * WHERE { VALUES … { …declared… } } — makes the
// declared query a SUBQUERY, and SPARQL evaluates subqueries bottom-up: one that
// does not project ?parent ignores the outer VALUES entirely and returns every
// child of every parent. Caught against live data (a node with 3 children came
// back with 200 rows), silently and with no error, which is why this is asserted
// structurally rather than left to an integration test.
func TestValuesGoesInsideTheDeclaredWhere(t *testing.T) {
	q, err := ChildrenQuery(childrenDecl, "", "http://example.org/c1", 0, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Exactly one SELECT: the declared one. A second would mean a wrap.
	if n := strings.Count(strings.ToUpper(q), "SELECT"); n != 1 {
		t.Errorf("expected the declared SELECT only, found %d — the query was wrapped:\n%s", n, q)
	}
	// And the binding sits after that SELECT's WHERE brace, not before it.
	whereIdx := strings.Index(strings.ToUpper(q), "WHERE")
	valuesIdx := strings.Index(q, "VALUES ?parent")
	if valuesIdx < whereIdx {
		t.Errorf("VALUES precedes WHERE — it is outside the query body:\n%s", q)
	}
}

// The brace scan must find the right "{" even when the WHERE body opens with a
// subquery or a GRAPH block, and must not be fooled by a brace inside a literal.
func TestValuesInjectionFindsTheRightBrace(t *testing.T) {
	decls := []string{
		`SELECT ?node WHERE { { SELECT ?node ?parent WHERE { ?node skos:broader ?parent } LIMIT 10 } ?node rdfs:label ?l }`,
		`SELECT ?node WHERE { GRAPH ?g { ?node skos:broader ?parent } }`,
		`SELECT ?node WHERE { ?node rdfs:label "a { brace" ; skos:broader ?parent }`,
		`SELECT ?node WHERE { ?node skos:broader ?parent . # a { comment
		  ?node rdfs:label ?l }`,
		// WHERE is optional in SPARQL.
		`SELECT ?node { ?node skos:broader ?parent }`,
	}
	for _, decl := range decls {
		q, err := ChildrenQuery(decl, "", "http://example.org/c1", 0, 0)
		if err != nil {
			t.Fatalf("rejected a valid query: %v\n%s", err, decl)
		}
		vi := strings.Index(q, "VALUES ?parent")
		if vi < 0 {
			t.Errorf("no binding injected:\n%s", q)
			continue
		}
		// The binding must land after the opening brace of the body, and before the
		// pattern that uses the variable.
		bi := strings.Index(q, "{")
		di := strings.Index(q, "skos:broader")
		if vi < bi || (di > 0 && vi > di) {
			t.Errorf("binding misplaced (brace %d, values %d, use %d):\n%s", bi, vi, di, q)
		}
	}
}

// A query with no braced body cannot be parameterised, and must be rejected rather
// than silently wrapped in a way that would not constrain it.
func TestValuesInjectionRejectsUnparseableQuery(t *testing.T) {
	if _, err := ChildrenQuery(`DESCRIBE ?parent`, "", "http://example.org/c1", 0, 0); err == nil {
		t.Error("a query with no braced WHERE body was accepted")
	}
}

func TestModeFor(t *testing.T) {
	ordered := childrenDecl + " ORDER BY ?label"
	if mode, lim := ModeFor(ordered, 0); mode != LimitPage || lim != DefaultPageLimit {
		t.Errorf("ordered query: got %v/%d, want page/%d", mode, lim, DefaultPageLimit)
	}
	if mode, lim := ModeFor(childrenDecl, 0); mode != LimitCap || lim != DefaultCapLimit {
		t.Errorf("unordered query: got %v/%d, want cap/%d", mode, lim, DefaultCapLimit)
	}
	// An explicit limit overrides the default but not the mode.
	if mode, lim := ModeFor(ordered, 25); mode != LimitPage || lim != 25 {
		t.Errorf("explicit limit: got %v/%d, want page/25", mode, lim)
	}
}

func TestRootsQueryBindsNothing(t *testing.T) {
	q, err := RootsQuery(`SELECT ?node WHERE { ?node skos:topConceptOf ?? }`, "http://example.org/s", 0, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(q, "VALUES") {
		t.Errorf("roots query should bind nothing:\n%s", q)
	}
	if !strings.Contains(q, "<http://example.org/s>") {
		t.Errorf("page IRI not substituted:\n%s", q)
	}
}
