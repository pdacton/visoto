package facet

import (
	"strings"
	"testing"
)

const baseQuery = `SELECT ?taxon ?name WHERE { ?taxon a <http://example.org/Taxon> . OPTIONAL { ?taxon <http://schema.org/name> ?name } }`

func TestBuildFacetValuesQuery(t *testing.T) {
	spec := FacetSpec{Var: "rank", Path: "dwc:taxonRank", Type: TypeIRI, Control: ControlSelect}
	q, err := BuildFacetValuesQuery("http://example.org/Taxon", "taxon", spec, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"SELECT ?rank (COUNT(DISTINCT ?taxon) AS ?count)",
		"?taxon a <http://example.org/Taxon> .",
		"?taxon dwc:taxonRank ?rank .",
		"GROUP BY ?rank ORDER BY DESC(?count) LIMIT 200",
	} {
		if !strings.Contains(q, want) {
			t.Errorf("missing %q in:\n%s", want, q)
		}
	}

	if _, err := BuildFacetValuesQuery("not-an-iri", "taxon", spec, 0); err == nil {
		t.Error("expected error for invalid class IRI")
	}
}

func TestBuildFacetedQueryEnum(t *testing.T) {
	con := FacetConstraint{
		Spec:   FacetSpec{Var: "rank", Path: "dwc:taxonRank", Type: TypeIRI, Control: ControlSelect},
		Values: []string{"http://ex/species", "http://ex/genus"},
	}
	q, err := BuildFacetedQuery(baseQuery, "taxon", []FacetConstraint{con}, BaseFacetProvider{})
	if err != nil {
		t.Fatal(err)
	}
	want := "FILTER EXISTS { ?taxon dwc:taxonRank ?__facet_rank . FILTER(?__facet_rank IN (<http://ex/species>, <http://ex/genus>)) }"
	if !strings.Contains(q, want) {
		t.Errorf("missing enum block:\n%s", q)
	}
	// Injected right after the membership triple, before the OPTIONAL.
	if strings.Index(q, "FILTER EXISTS") > strings.Index(q, "OPTIONAL") {
		t.Errorf("facet block not injected before OPTIONAL:\n%s", q)
	}
}

func TestBuildFacetedQueryRangeAndText(t *testing.T) {
	cons := []FacetConstraint{
		{
			Spec:   FacetSpec{Var: "year", Path: "dwc:year", Type: TypeNumber, Control: ControlRange},
			Values: []string{"1900", "2000"},
		},
		{
			Spec:   FacetSpec{Var: "name", Path: "schema:name", Type: TypeString, Control: ControlText},
			Values: []string{"Rosa"},
		},
	}
	q, err := BuildFacetedQuery(baseQuery, "taxon", cons, BaseFacetProvider{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(q, "FILTER(<http://www.w3.org/2001/XMLSchema#decimal>(?__facet_year) >= 1900 && <http://www.w3.org/2001/XMLSchema#decimal>(?__facet_year) <= 2000)") {
		t.Errorf("missing range block:\n%s", q)
	}
	if !strings.Contains(q, `FILTER(CONTAINS(LCASE(STR(?__facet_name)), "rosa"))`) {
		t.Errorf("missing text block:\n%s", q)
	}
	// Two facets → two existential blocks (AND across facets).
	if strings.Count(q, "FILTER EXISTS") != 2 {
		t.Errorf("expected 2 FILTER EXISTS, got:\n%s", q)
	}
}

func TestBuildFacetedQuerySkipsEmpty(t *testing.T) {
	con := FacetConstraint{
		Spec:   FacetSpec{Var: "rank", Path: "dwc:taxonRank", Type: TypeIRI, Control: ControlSelect},
		Values: []string{"", "  "},
	}
	q, err := BuildFacetedQuery(baseQuery, "taxon", []FacetConstraint{con}, BaseFacetProvider{})
	if err != nil {
		t.Fatal(err)
	}
	if q != baseQuery {
		t.Errorf("empty selection should be a no-op, got:\n%s", q)
	}
}

func TestBuildFacetedQueryNoValueOnly(t *testing.T) {
	con := FacetConstraint{
		Spec:   FacetSpec{Var: "rank", Path: "dwc:taxonRank", Type: TypeIRI, Control: ControlSelect},
		Values: []string{NoValueSentinel},
	}
	q, err := BuildFacetedQuery(baseQuery, "taxon", []FacetConstraint{con}, BaseFacetProvider{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(q, "FILTER NOT EXISTS { ?taxon dwc:taxonRank ?__facet_rank }") {
		t.Errorf("missing NOT EXISTS block:\n%s", q)
	}
	if strings.Contains(q, "FILTER EXISTS") || strings.Contains(q, "UNION") {
		t.Errorf("no-value-only should be a bare NOT EXISTS:\n%s", q)
	}
	// The sentinel must never be rendered as a term.
	if strings.Contains(q, NoValueSentinel) {
		t.Errorf("sentinel leaked into query:\n%s", q)
	}
}

func TestBuildFacetedQueryNoValueMixed(t *testing.T) {
	con := FacetConstraint{
		Spec:   FacetSpec{Var: "rank", Path: "dwc:taxonRank", Type: TypeIRI, Control: ControlSelect},
		Values: []string{"http://ex/species", NoValueSentinel},
	}
	q, err := BuildFacetedQuery(baseQuery, "taxon", []FacetConstraint{con}, BaseFacetProvider{})
	if err != nil {
		t.Fatal(err)
	}
	want := "{ FILTER EXISTS { ?taxon dwc:taxonRank ?__facet_rank . FILTER(?__facet_rank IN (<http://ex/species>)) } } UNION { FILTER NOT EXISTS { ?taxon dwc:taxonRank ?__facet_rank } }"
	if !strings.Contains(q, want) {
		t.Errorf("missing UNION of EXISTS/NOT EXISTS:\nwant %q\ngot\n%s", want, q)
	}
	if strings.Contains(q, NoValueSentinel) {
		t.Errorf("sentinel leaked into query:\n%s", q)
	}
}

func TestBuildFacetedQueryRangeNoValue(t *testing.T) {
	spec := FacetSpec{Var: "year", Path: "dwc:year", Type: TypeNumber, Control: ControlRange}

	// No-value only: empty bounds, NoValue flag set → bare NOT EXISTS.
	only := FacetConstraint{Spec: spec, Values: []string{"", ""}, NoValue: true}
	q, err := BuildFacetedQuery(baseQuery, "taxon", []FacetConstraint{only}, BaseFacetProvider{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(q, "FILTER NOT EXISTS { ?taxon dwc:year ?__facet_year }") {
		t.Errorf("range no-value-only should be a bare NOT EXISTS:\n%s", q)
	}
	if strings.Contains(q, "UNION") || strings.Contains(q, ">=") {
		t.Errorf("range no-value-only should carry no range clause:\n%s", q)
	}

	// Range + no-value: UNION of the coerced range and NOT EXISTS.
	mixed := FacetConstraint{Spec: spec, Values: []string{"1900", "2000"}, NoValue: true}
	q, err = BuildFacetedQuery(baseQuery, "taxon", []FacetConstraint{mixed}, BaseFacetProvider{})
	if err != nil {
		t.Fatal(err)
	}
	want := "{ FILTER EXISTS { ?taxon dwc:year ?__facet_year . FILTER(<http://www.w3.org/2001/XMLSchema#decimal>(?__facet_year) >= 1900 && <http://www.w3.org/2001/XMLSchema#decimal>(?__facet_year) <= 2000) } } UNION { FILTER NOT EXISTS { ?taxon dwc:year ?__facet_year } }"
	if !strings.Contains(q, want) {
		t.Errorf("missing range UNION no-value block:\nwant %q\ngot\n%s", want, q)
	}
}

func TestBuildFacetedQueryTextNoValue(t *testing.T) {
	spec := FacetSpec{Var: "name", Path: "schema:name", Type: TypeString, Control: ControlText}

	// No-value only: empty term, NoValue flag set → bare NOT EXISTS.
	only := FacetConstraint{Spec: spec, Values: []string{""}, NoValue: true}
	q, err := BuildFacetedQuery(baseQuery, "taxon", []FacetConstraint{only}, BaseFacetProvider{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(q, "FILTER NOT EXISTS { ?taxon schema:name ?__facet_name }") {
		t.Errorf("text no-value-only should be a bare NOT EXISTS:\n%s", q)
	}
	if strings.Contains(q, "UNION") || strings.Contains(q, "CONTAINS") {
		t.Errorf("text no-value-only should carry no CONTAINS clause:\n%s", q)
	}

	// Text + no-value: UNION of the CONTAINS match and NOT EXISTS.
	mixed := FacetConstraint{Spec: spec, Values: []string{"Rosa"}, NoValue: true}
	q, err = BuildFacetedQuery(baseQuery, "taxon", []FacetConstraint{mixed}, BaseFacetProvider{})
	if err != nil {
		t.Fatal(err)
	}
	want := `{ FILTER EXISTS { ?taxon schema:name ?__facet_name . FILTER(CONTAINS(LCASE(STR(?__facet_name)), "rosa")) } } UNION { FILTER NOT EXISTS { ?taxon schema:name ?__facet_name } }`
	if !strings.Contains(q, want) {
		t.Errorf("missing text UNION no-value block:\nwant %q\ngot\n%s", want, q)
	}
}

// ---- column mode (no path): filter the projected variable directly ----

func TestBuildFacetedQueryColumnMode(t *testing.T) {
	cases := []struct {
		name string
		con  FacetConstraint
		want string
	}{
		{
			"enum",
			FacetConstraint{
				Spec:   FacetSpec{Var: "kind", Type: TypeString, Control: ControlSelect},
				Values: []string{"incoming"},
			},
			`FILTER(?kind IN ("incoming"))`,
		},
		{
			"text",
			FacetConstraint{
				Spec:   FacetSpec{Var: "name", Type: TypeString, Control: ControlText},
				Values: []string{"Rosa"},
			},
			`FILTER(CONTAINS(LCASE(STR(?name)), "rosa"))`,
		},
		{
			"range",
			FacetConstraint{
				Spec:   FacetSpec{Var: "year", Type: TypeNumber, Control: ControlRange},
				Values: []string{"1900", "2000"},
			},
			"FILTER(<http://www.w3.org/2001/XMLSchema#decimal>(?year) >= 1900 && <http://www.w3.org/2001/XMLSchema#decimal>(?year) <= 2000)",
		},
		{
			// "(no value)" alone → !BOUND, the expression counterpart of NOT EXISTS.
			"no value only",
			FacetConstraint{
				Spec:   FacetSpec{Var: "graph", Type: TypeIRI, Control: ControlSelect},
				Values: []string{NoValueSentinel},
			},
			"FILTER(!BOUND(?graph))",
		},
		{
			// Concrete OR no-value: a single FILTER, never a UNION — a UNION of two
			// filter-only groups evaluates against the empty solution, where the
			// projected variable is unbound.
			"no value mixed",
			FacetConstraint{
				Spec:   FacetSpec{Var: "kind", Type: TypeString, Control: ControlSelect},
				Values: []string{"incoming", NoValueSentinel},
			},
			`FILTER((?kind IN ("incoming")) || !BOUND(?kind))`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			q, err := BuildFacetedQuery(baseQuery, "taxon", []FacetConstraint{tc.con}, BaseFacetProvider{})
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(q, tc.want) {
				t.Errorf("missing %q in:\n%s", tc.want, q)
			}
			if strings.Contains(q, "FILTER EXISTS") || strings.Contains(q, "UNION") {
				t.Errorf("column mode must not emit an existential block:\n%s", q)
			}
			if strings.Contains(q, NoValueSentinel) {
				t.Errorf("sentinel leaked into query:\n%s", q)
			}
		})
	}
}

// A column-mode FILTER must land in the outermost group, where every projected
// variable is in scope — not inside the OPTIONAL that happens to precede it.
func TestBuildFacetedQueryColumnModePlacement(t *testing.T) {
	con := FacetConstraint{
		Spec:   FacetSpec{Var: "name", Type: TypeString, Control: ControlSelect},
		Values: []string{"Rosa"},
	}
	q, err := BuildFacetedQuery(baseQuery, "taxon", []FacetConstraint{con}, BaseFacetProvider{})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Index(q, "FILTER(") < strings.LastIndex(q, "OPTIONAL") {
		t.Errorf("filter injected before the OPTIONAL closed:\n%s", q)
	}
	if !strings.HasSuffix(strings.TrimSpace(q), "}") {
		t.Errorf("filter injected outside the WHERE group:\n%s", q)
	}
}

// ---- root ----

// An explicit root anchors the existential filter, so a query whose membership
// triple is hidden behind a BIND (DeriveKeyVar returns "") still gets class-mode
// semantics.
func TestBuildFacetedQueryExplicitRoot(t *testing.T) {
	bindQuery := `SELECT ?type ?municipality ?canton WHERE { BIND (<http://example.org/M> AS ?type) ?municipality rdf:type ?type . }`
	con := FacetConstraint{
		Spec:   FacetSpec{Var: "canton", Root: "?municipality", Path: "schema:containedInPlace", Type: TypeIRI, Control: ControlSelect},
		Values: []string{"http://ex/zh"},
	}
	q, err := BuildFacetedQuery(bindQuery, "", []FacetConstraint{con}, BaseFacetProvider{})
	if err != nil {
		t.Fatal(err)
	}
	want := "FILTER EXISTS { ?municipality schema:containedInPlace ?__facet_canton . FILTER(?__facet_canton IN (<http://ex/zh>)) }"
	if !strings.Contains(q, want) {
		t.Errorf("missing rooted existential block:\n%s", q)
	}
}

// InstanceRoot filters the projected column: a pattern anchored on a constant is
// identical for every row and so cannot discriminate between rows.
func TestBuildFacetedQueryInstanceRootFiltersColumn(t *testing.T) {
	con := FacetConstraint{
		Spec:   FacetSpec{Var: "property", Root: InstanceRoot, Path: "?property", Type: TypeIRI, Control: ControlSelect},
		Values: []string{"http://schema.org/name"},
	}
	q, err := BuildFacetedQuery(baseQuery, "taxon", []FacetConstraint{con}, BaseFacetProvider{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(q, "FILTER(?property IN (<http://schema.org/name>))") {
		t.Errorf("instance root should filter the column:\n%s", q)
	}
	if strings.Contains(q, "FILTER EXISTS") {
		t.Errorf("instance root must not emit an existential block:\n%s", q)
	}
}

// Filtering an aggregate alias fails silently at the store (200 + zero rows), so
// it must be rejected here instead — unless the facet routes through a property
// path, which never names the alias.
func TestBuildFacetedQueryRejectsAggregateAlias(t *testing.T) {
	aggQuery := `SELECT ?municipality (GROUP_CONCAT(?postalCode; separator=", ") AS ?postalCodes) WHERE {
	  ?municipality a <http://example.org/M> .
	  OPTIONAL { ?municipality schema:postalCode ?postalCode }
	} GROUP BY ?municipality`

	column := FacetConstraint{
		Spec:   FacetSpec{Var: "postalCodes", Type: TypeString, Control: ControlText},
		Values: []string{"8001"},
	}
	_, err := BuildFacetedQuery(aggQuery, "municipality", []FacetConstraint{column}, BaseFacetProvider{})
	if err == nil {
		t.Fatal("expected a column facet on an aggregate alias to be rejected")
	}
	if !strings.Contains(err.Error(), "root=") {
		t.Errorf("error should point at the root/path fix, got: %v", err)
	}

	// Same var, routed through the property: allowed, and existential.
	routed := FacetConstraint{
		Spec:   FacetSpec{Var: "postalCodes", Root: "?municipality", Path: "schema:postalCode", Type: TypeString, Control: ControlText},
		Values: []string{"8001"},
	}
	q, err := BuildFacetedQuery(aggQuery, "municipality", []FacetConstraint{routed}, BaseFacetProvider{})
	if err != nil {
		t.Fatal(err)
	}
	want := `FILTER EXISTS { ?municipality schema:postalCode ?__facet_postalCodes . FILTER(CONTAINS(LCASE(STR(?__facet_postalCodes)), "8001")) }`
	if !strings.Contains(q, want) {
		t.Errorf("missing existential postal-code block:\n%s", q)
	}

	// A non-aggregate column of the same query stays filterable in column mode.
	plain := FacetConstraint{
		Spec:   FacetSpec{Var: "municipality", Type: TypeString, Control: ControlText},
		Values: []string{"Zug"},
	}
	if _, err := BuildFacetedQuery(aggQuery, "municipality", []FacetConstraint{plain}, BaseFacetProvider{}); err != nil {
		t.Errorf("non-aggregate column should stay filterable: %v", err)
	}
}

// A nested aggregate's own parentheses must not end the argument scan early: the
// flat [^)]* this replaced stopped at the inner ")" and missed the alias, letting
// through the exact shape the guard exists to catch.
func TestAggregateAliasReMatchesNestedParens(t *testing.T) {
	cases := []struct {
		name    string
		varName string
		text    string
	}{
		{"count distinct", "municipalities", "(COUNT(DISTINCT ?municipality) AS ?municipalities)"},
		{"group_concat separator", "postalCodes", `(GROUP_CONCAT(?postalCode; separator=", ") AS ?postalCodes)`},
		{"sum of if", "paid", "(SUM(IF(?a, 1, 0)) AS ?paid)"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if !aggregateAliasRe(tc.varName).MatchString(tc.text) {
				t.Errorf("aggregate alias not detected in %s", tc.text)
			}
		})
	}
}

// An aggregate computed in a subquery is projected outward as an ordinary
// variable, so a plain column FILTER on it is correct and must NOT be rejected.
// This is the templates/classes/schch%3ACanton.html shape: the counts come from
// "OPTIONAL { SELECT ... GROUP BY ?canton }" and no property path reaches them.
func TestBuildFacetedQueryAllowsNestedAggregateAlias(t *testing.T) {
	nested := `SELECT ?canton ?municipalities WHERE {
	  ?canton a <http://example.org/Canton> .
	  OPTIONAL {
	    SELECT ?canton (COUNT(DISTINCT ?municipality) AS ?municipalities) WHERE {
	      ?municipality schema:containedInPlace ?canton .
	    } GROUP BY ?canton
	  }
	}`

	con := FacetConstraint{
		Spec:   FacetSpec{Var: "municipalities", Type: TypeNumber, Control: ControlRange},
		Values: []string{"10", ""},
	}
	q, err := BuildFacetedQuery(nested, "canton", []FacetConstraint{con}, BaseFacetProvider{})
	if err != nil {
		t.Fatalf("nested aggregate should stay filterable in column mode: %v", err)
	}
	if !strings.Contains(q, "?municipalities") {
		t.Errorf("expected a column filter on the projected alias:\n%s", q)
	}
	// The filter belongs inside the outer WHERE group, never appended after the
	// subquery's GROUP BY — insertBeforeLastBrace's "}[^}]*$" anchor decides this.
	if strings.Contains(q, "GROUP BY ?canton\n\t  }\n\t}\n  FILTER") {
		t.Errorf("column filter escaped the outer WHERE group:\n%s", q)
	}
}

// An outer-projection aggregate keeps failing loudly: rejection is the whole
// point, since the store would answer 200 with zero rows instead.
func TestBuildFacetedQueryStillRejectsOuterAggregateAlias(t *testing.T) {
	aggQuery := `SELECT ?municipality (COUNT(DISTINCT ?postalCode) AS ?codes) WHERE {
	  ?municipality a <http://example.org/M> .
	  OPTIONAL { ?municipality schema:postalCode ?postalCode }
	} GROUP BY ?municipality`

	con := FacetConstraint{
		Spec:   FacetSpec{Var: "codes", Type: TypeNumber, Control: ControlRange},
		Values: []string{"3", ""},
	}
	_, err := BuildFacetedQuery(aggQuery, "municipality", []FacetConstraint{con}, BaseFacetProvider{})
	if err == nil {
		t.Fatal("expected an outer-projection aggregate alias to be rejected")
	}
	if !strings.Contains(err.Error(), "root=") {
		t.Errorf("error should point at the root/path fix, got: %v", err)
	}
}

// A query whose braces do not balance cannot be scoped, so the guard fails closed
// — an over-strict error beats a silently empty result.
func TestBuildFacetedQueryAggregateAliasUnreadableShapeRejects(t *testing.T) {
	truncated := `SELECT ?m (COUNT(?p) AS ?codes) WHERE {
	  ?m a <http://example.org/M> .
	  OPTIONAL { ?m schema:postalCode ?p }`

	con := FacetConstraint{
		Spec:   FacetSpec{Var: "codes", Type: TypeNumber, Control: ControlRange},
		Values: []string{"3", ""},
	}
	if _, err := BuildFacetedQuery(truncated, "m", []FacetConstraint{con}, BaseFacetProvider{}); err == nil {
		t.Fatal("an unreadable query shape should keep the conservative rejection")
	}
}

// Brace depth must ignore braces inside quoted literals, or a separator like "}"
// would make an outer aggregate look nested and re-open the silent failure.
func TestOuterProjectionAggregateIgnoresBracesInLiterals(t *testing.T) {
	q := `SELECT ?m (GROUP_CONCAT(?p; separator="}") AS ?codes) WHERE {
	  ?m a <http://example.org/M> .
	} GROUP BY ?m`

	isOuter, ok := outerProjectionAggregate(q, "codes")
	if !ok {
		t.Fatal("query should be readable")
	}
	if !isOuter {
		t.Error("a brace inside a string literal must not count as nesting")
	}
}

func TestFacetSpecValidate(t *testing.T) {
	if err := (FacetSpec{Var: "x", Root: "?y"}).Validate(); err == nil {
		t.Error("root without path should be rejected")
	}
	if err := (FacetSpec{Var: "x"}).Validate(); err != nil {
		t.Errorf("column mode should validate: %v", err)
	}
	if err := (FacetSpec{Var: "", Path: "a:b"}).Validate(); err == nil {
		t.Error("missing var should be rejected")
	}
}

// ---- enumeration shapes ----

func TestBuildColumnValuesQuery(t *testing.T) {
	spec := FacetSpec{Var: "kind", Type: TypeString, Control: ControlSelect}
	q, err := BuildColumnValuesQuery(baseQuery, spec, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"SELECT ?kind (COUNT(*) AS ?count) WHERE {",
		baseQuery, // the declared query is wrapped verbatim, LIMIT included
		"GROUP BY ?kind ORDER BY DESC(?count) LIMIT 200",
	} {
		if !strings.Contains(q, want) {
			t.Errorf("missing %q in:\n%s", want, q)
		}
	}

	// An inner ?count would collide with the outer aggregate alias.
	if _, err := BuildColumnValuesQuery(`SELECT ?x (COUNT(?y) AS ?count) WHERE { ?x ?p ?y }`, spec, 0); err == nil {
		t.Error("expected a ?count collision to be rejected")
	}
}

func TestBuildInstanceValuesQuery(t *testing.T) {
	spec := FacetSpec{Var: "property", Root: InstanceRoot, Path: "?property", Type: TypeIRI, Control: ControlSelect}
	q, err := BuildInstanceValuesQuery("https://ld.admin.ch/canton/1", spec, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"SELECT ?property (COUNT(*) AS ?count)",
		"<https://ld.admin.ch/canton/1> ?property ?property .",
		"GROUP BY ?property ORDER BY DESC(?count) LIMIT 200",
	} {
		if !strings.Contains(q, want) {
			t.Errorf("missing %q in:\n%s", want, q)
		}
	}
	if _, err := BuildInstanceValuesQuery("not-an-iri", spec, 0); err == nil {
		t.Error("expected error for invalid resource IRI")
	}
}

func TestBuildFacetedQueryRejectsInjection(t *testing.T) {
	con := FacetConstraint{
		Spec:   FacetSpec{Var: "rank", Path: "dwc:taxonRank", Type: TypeIRI, Control: ControlSelect},
		Values: []string{"http://ex/a> } UNION { ?s ?p ?o } #"},
	}
	if _, err := BuildFacetedQuery(baseQuery, "taxon", []FacetConstraint{con}, BaseFacetProvider{}); err == nil {
		t.Error("expected injection attempt to be rejected")
	}
}

// Range facets must not emit a CURIE for the xsd constructor: nothing declares a
// PREFIX xsd: (not the query, not visoto.config, not the preprocessor). LINDAS/
// GraphDB pre-declares it implicitly, but that is not SPARQL 1.1 behaviour, so a
// CURIE here fails with "undefined prefix" on a conformant store.
func TestRangeClauseDeclaresNoPrefix(t *testing.T) {
	cases := []struct {
		name   string
		spec   FacetSpec
		values []string
	}{
		{"number", FacetSpec{Var: "year", Path: "dwc:year", Type: TypeNumber, Control: ControlRange}, []string{"1900", "2000"}},
		{"date", FacetSpec{Var: "d", Path: "dct:date", Type: TypeDate, Control: ControlRange}, []string{"1900-01-01", "2000-12-31"}},
		{"one-sided", FacetSpec{Var: "year", Path: "dwc:year", Type: TypeNumber, Control: ControlRange}, []string{"1900", ""}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			q, err := BuildFacetedQuery(baseQuery, "taxon",
				[]FacetConstraint{{Spec: tc.spec, Values: tc.values}}, BaseFacetProvider{})
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(q, "xsd:") {
				t.Errorf("range clause uses an undeclared xsd: prefix:\n%s", q)
			}
			if !strings.Contains(q, "<http://www.w3.org/2001/XMLSchema#") {
				t.Errorf("expected full-IRI xsd constructor:\n%s", q)
			}
		})
	}
}
