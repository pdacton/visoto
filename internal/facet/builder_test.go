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
