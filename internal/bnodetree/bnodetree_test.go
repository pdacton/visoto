package bnodetree

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"testing"

	"hutzli.org/visoto/internal/config"
	"hutzli.org/visoto/internal/sparql"
)

const (
	shape   = "https://agriculture.ld.admin.ch/agroscope/BLW_t_01/1/shape/"
	cubeIRI = "https://agriculture.ld.admin.ch/agroscope/BLW_t_01/1"
	schema  = "http://schema.org/"
	xsd     = "http://www.w3.org/2001/XMLSchema#"
)

var testPrefixes = []config.Prefix{
	{Name: "sh", URI: shNS},
	{Name: "schema", URI: schema},
	{Name: "xsd", URI: xsd},
	{Name: "rdf", URI: rdfNS},
	{Name: "cube", URI: "https://cube.link/"},
	{Name: "meta", URI: "https://cube.link/meta/"},
	{Name: "time", URI: "http://www.w3.org/2006/time#"},
}

// --- fixture helpers ---------------------------------------------------------

func iri(v string) sparql.Binding { return sparql.Binding{Type: "uri", Value: v, DisplayText: v} }
func bn(v string) sparql.Binding  { return sparql.Binding{Type: "bnode", Value: v, DisplayText: v} }
func lit(v string) sparql.Binding { return sparql.Binding{Type: "literal", Value: v, DisplayText: v} }
func langLit(v, l string) sparql.Binding {
	return sparql.Binding{Type: "literal", Value: v, DisplayText: v, Lang: l}
}
func typed(v, dt string) sparql.Binding {
	return sparql.Binding{Type: "literal", Value: v, DisplayText: v, Datatype: dt}
}

type fixture struct{ rows []Row }

func (f *fixture) add(d int, s sparql.Binding, p string, o sparql.Binding) {
	f.rows = append(f.rows, Row{Depth: d, S: s, P: iri(p), O: o})
}

// list adds an RDF list with the given cell prefix, owned by a node at depth d.
func (f *fixture) list(d int, cell string, items ...sparql.Binding) sparql.Binding {
	for i, it := range items {
		c := bn(fmt.Sprintf("%s%d", cell, i))
		f.add(d, c, rdfFirst, it)
		if i == len(items)-1 {
			f.add(d, c, rdfRest, iri(rdfNil))
		} else {
			f.add(d, c, rdfRest, bn(fmt.Sprintf("%s%d", cell, i+1)))
		}
	}
	return bn(cell + "0")
}

// shapeFixture mirrors BLW_t_01/1/shape/: four of its property shapes, with
// sh:order, an sh:or of two datatype shapes, a 23-member sh:in and a nested
// meta:dataKind.
func shapeFixture() *fixture {
	f := &fixture{}
	root := iri(shape)
	// Deliberately added out of sh:order.
	area, year, nutz, kult := bn("area"), bn("year"), bn("nutz"), bn("kult")
	for _, ps := range []sparql.Binding{area, nutz, year, kult} {
		f.add(0, root, shNS+"property", ps)
	}

	f.add(1, year, shPath, iri("https://agriculture.ld.admin.ch/agroscope/BLW_t_01/Year"))
	f.add(1, year, schema+"name", langLit("Jahr", "de"))
	f.add(1, year, shOrder, typed("1", xsd+"integer"))
	f.add(1, year, shNS+"datatype", iri(xsd+"gYear"))
	f.add(1, year, rdfType, iri("https://cube.link/KeyDimension"))
	dk := bn("dk")
	f.add(1, year, "https://cube.link/meta/dataKind", dk)
	f.add(2, dk, rdfType, iri("http://www.w3.org/2006/time#GeneralDateTimeDescription"))
	f.add(2, dk, "http://www.w3.org/2006/time#unitType", iri("http://www.w3.org/2006/time#unitYear"))

	f.add(1, nutz, shPath, iri("https://agriculture.ld.admin.ch/agroscope/BLW_t_01/nutzungsarten"))
	f.add(1, nutz, schema+"name", langLit("Nutzungsart", "de"))
	f.add(1, nutz, shOrder, typed("2", xsd+"integer"))
	var members []sparql.Binding
	for i := 1; i <= 23; i++ {
		members = append(members, iri(fmt.Sprintf("https://agriculture.ld.admin.ch/agroscope/BLW_t_01/1/NA%d", i)))
	}
	f.add(1, nutz, shNS+"in", f.list(1, "nin", members...))

	f.add(1, kult, schema+"name", langLit("Kultur", "de"))
	f.add(1, kult, shOrder, typed("3", xsd+"integer"))

	f.add(1, area, shPath, iri(schema+"area"))
	f.add(1, area, schema+"name", langLit("Fläche", "de"))
	f.add(1, area, shOrder, typed("4", xsd+"integer"))
	f.add(1, area, shNS+"minInclusive", typed("12", xsd+"integer"))
	o1, o2 := bn("orm1"), bn("orm2")
	f.add(1, area, shNS+"or", f.list(1, "orc", o1, o2))
	f.add(2, o1, shNS+"datatype", iri(xsd+"integer"))
	f.add(2, o2, shNS+"datatype", iri("https://cube.link/Undefined"))
	return f
}

func build(f *fixture, lang string) (*Graph, *Tree) {
	g := NewGraph(f.rows)
	return g, Build(g, Options{Lang: lang, Prefixes: testPrefixes})
}

func propByCURIE(props []*Prop, curie string) *Prop {
	for _, p := range props {
		if p.Pred.CURIE() == curie {
			return p
		}
	}
	return nil
}

// --- tree --------------------------------------------------------------------

func TestShapeTree(t *testing.T) {
	_, tree := build(shapeFixture(), "de")
	if len(tree.Roots) != 1 {
		t.Fatalf("roots = %d, want 1", len(tree.Roots))
	}
	props := propByCURIE(tree.Roots[0].Props, "sh:property")
	if props == nil || len(props.Values) != 4 {
		t.Fatalf("sh:property values = %v", props)
	}
	var titles []string
	for _, v := range props.Values {
		titles = append(titles, v.Node.Title)
	}
	if got := strings.Join(titles, ","); got != "Jahr,Nutzungsart,Kultur,Fläche" {
		t.Errorf("order by sh:order = %s", got)
	}

	year := props.Values[0].Node
	if year.Order != "1" || len(year.Types) != 1 || year.Types[0].Local != "KeyDimension" {
		t.Errorf("year header = order %q types %+v", year.Order, year.Types)
	}
	if year.Props[0].Pred.CURIE() != "sh:path" {
		t.Errorf("sh:path not first: %s", year.Props[0].Pred.CURIE())
	}
	if propByCURIE(year.Props, "sh:order") != nil {
		t.Error("sh:order should be a badge, not a property")
	}
	dk := propByCURIE(year.Props, "meta:dataKind").Values[0]
	if dk.Kind != KindNode || dk.Node.Level != 2 || dk.Node.Types[0].Local != "GeneralDateTimeDescription" {
		t.Errorf("dataKind = %+v", dk)
	}

	in := propByCURIE(props.Values[1].Node.Props, "sh:in")
	if in.Caption != "oneOf" {
		t.Errorf("sh:in caption = %q", in.Caption)
	}
	enum := in.Values[0]
	if enum.Kind != KindEnum || enum.Count != 23 || len(enum.Items) != EnumShown || len(enum.Rest) != 23-EnumShown {
		t.Errorf("enum = kind %s count %d shown %d rest %d", enum.Kind, enum.Count, len(enum.Items), len(enum.Rest))
	}

	or := propByCURIE(props.Values[3].Node.Props, "sh:or")
	if or.Caption != "anyOf" || or.Values[0].Kind != KindList || len(or.Values[0].Items) != 2 {
		t.Fatalf("sh:or = %+v", or)
	}
	member := or.Values[0].Items[1]
	if member.Kind != KindNode || member.Node.Level != 2 {
		t.Errorf("list member level = %+v (list cells must not use up a level)", member)
	}
	if !member.Node.Flat() {
		t.Errorf("[ sh:datatype … ] should render flat: %+v", member.Node)
	}
	if props.Values[3].Node.Flat() {
		t.Error("a titled node must keep its header")
	}
	min := propByCURIE(props.Values[3].Node.Props, "sh:minInclusive").Values[0]
	if min.Text != "12" || min.Datatype != "xsd:integer" {
		t.Errorf("literal = %+v", min)
	}

	// 4 property shapes + dataKind + 2 sh:or members; list cells excluded.
	if tree.BlankNodes != 7 {
		t.Errorf("BlankNodes = %d, want 7", tree.BlankNodes)
	}
}

func TestTitleFallsBackToPath(t *testing.T) {
	f := &fixture{}
	ps := bn("ps")
	f.add(0, iri(shape), shNS+"property", ps)
	f.add(1, ps, shPath, sparql.Binding{Type: "uri", Value: "https://cube.link/observedBy", DisplayText: "observed by"})
	f.add(1, ps, shNS+"nodeKind", iri(shNS+"IRI"))
	f.add(1, ps, schema+"url", iri("https://visualize.admin.ch/en/browse?dataset=x"))
	_, tree := build(f, "")
	n := tree.Roots[0].Props[0].Values[0].Node
	if n.Title != "observed by" {
		t.Errorf("title = %q, want the sh:path label", n.Title)
	}
	if u := propByCURIE(n.Props, "schema:url").Values[0]; u.Term.Label != "https://visualize.admin.ch/en/browse?dataset=x" {
		t.Errorf("URL with a query string should keep its full form, got %q", u.Term.Label)
	}
}

func TestTitleLanguage(t *testing.T) {
	f := &fixture{}
	w := bn("w")
	f.add(0, iri(cubeIRI), schema+"workExample", w)
	f.add(1, w, schema+"name", langLit("SPARQL Endpoint mit Vorauswahl des Graph", "de"))
	f.add(1, w, schema+"name", langLit("SPARQL Endpoint with graph preselection", "en"))
	f.add(1, w, schema+"name", langLit("SPARQL Endpoint avec présélection du graphe", "fr"))
	for lang, want := range map[string]string{
		"fr": "SPARQL Endpoint avec présélection du graphe",
		"de": "SPARQL Endpoint mit Vorauswahl des Graph",
		"rm": "SPARQL Endpoint with graph preselection", // no rm: English wins
	} {
		_, tree := build(f, lang)
		if got := tree.Roots[0].Props[0].Values[0].Node.Title; got != want {
			t.Errorf("lang %s: title %q, want %q", lang, got, want)
		}
	}
	_, tree := build(f, "de")
	names := propByCURIE(tree.Roots[0].Props[0].Values[0].Node.Props, "schema:name")
	if len(names.Values) != 3 || names.Values[0].Lang != "de" {
		t.Errorf("repeated predicate not grouped/sorted: %+v", names.Values)
	}
}

func TestSharedAndCyclic(t *testing.T) {
	f := &fixture{}
	root := iri(cubeIRI)
	jane, loop := bn("jane"), bn("loop")
	f.add(0, root, "http://ex.org/creator", jane)
	f.add(0, root, "http://ex.org/maintainer", jane)
	f.add(1, jane, schema+"name", lit("Jane Doe"))
	f.add(0, root, "http://ex.org/loop", loop)
	f.add(1, loop, "http://ex.org/self", loop)

	_, tree := build(f, "")
	creator := propByCURIE(tree.Roots[0].Props, "http://ex.org/creator").Values[0]
	maint := propByCURIE(tree.Roots[0].Props, "http://ex.org/maintainer").Values[0]
	if creator.Kind != KindNode || creator.Node.Anchor == 0 {
		t.Fatalf("first occurrence = %+v", creator)
	}
	if maint.Kind != KindRef || maint.Ref != creator.Node.Anchor || maint.RefTitle != "Jane Doe" {
		t.Errorf("second occurrence = %+v", maint)
	}
	l := propByCURIE(tree.Roots[0].Props, "http://ex.org/loop").Values[0]
	self := l.Node.Props[0].Values[0]
	if self.Kind != KindRef || self.Ref != l.Node.Anchor {
		t.Errorf("cycle not cut: %+v", self)
	}
}

func TestDepthLimit(t *testing.T) {
	f := &fixture{}
	prev := iri(cubeIRI)
	for level := 0; level <= queryDepth; level++ {
		next := bn(fmt.Sprintf("n%d", level+1))
		f.add(level, prev, "http://ex.org/next", next)
		if level > 0 {
			f.add(level, prev, "http://ex.org/level", lit(fmt.Sprint(level)))
		}
		prev = next
	}
	_, tree := build(f, "")
	v := tree.Roots[0].Props[0].Values[0]
	for level := 1; level <= MaxDepth; level++ {
		if v.Kind != KindNode || v.Node.Level != level {
			t.Fatalf("level %d: %+v", level, v)
		}
		v = propByCURIE(v.Node.Props, "http://ex.org/next").Values[0]
	}
	if v.Kind != KindMore || v.MoreNodes != 1 || v.MoreTriples != 2 {
		t.Errorf("beyond depth = %+v", v)
	}
}

// chain builds root → n1 → n2 → … with one blank node per level, levels 0..last.
func chain(last int) *fixture {
	f := &fixture{}
	prev := iri(cubeIRI)
	for level := 0; level <= last; level++ {
		next := bn(fmt.Sprintf("n%d", level+1))
		f.add(level, prev, "http://ex.org/next", next)
		prev = next
	}
	return f
}

func TestNeedsDeeper(t *testing.T) {
	// Levels 0..ProbeDepth fetched, and the node at ProbeDepth points further.
	if !NeedsDeeper(chain(ProbeDepth).rows, ProbeDepth) {
		t.Error("a blank object at the probe depth needs the full query")
	}
	// The structure ends above the probe depth.
	if NeedsDeeper(chain(ProbeDepth-1).rows, ProbeDepth) {
		t.Error("a structure ending above the probe depth needs no second query")
	}
	// A literal at the last level is no reason to go deeper.
	f := chain(ProbeDepth - 1)
	f.add(ProbeDepth, bn(fmt.Sprintf("n%d", ProbeDepth)), "http://ex.org/level", lit("2"))
	if NeedsDeeper(f.rows, ProbeDepth) {
		t.Error("only blank objects at the last level count")
	}
}

func TestCut(t *testing.T) {
	g := NewGraph(chain(ProbeDepth).rows)
	g.Cut = ProbeDepth
	tree := Build(g, Options{Prefixes: testPrefixes})
	v := tree.Roots[0].Props[0].Values[0]
	for level := 1; level <= ProbeDepth; level++ {
		if v.Kind != KindNode {
			t.Fatalf("level %d: %+v", level, v)
		}
		v = propByCURIE(v.Node.Props, "http://ex.org/next").Values[0]
	}
	if v.Kind != KindMore || !v.Cut || v.MoreNodes != 1 {
		t.Errorf("below the cut = %+v, want a not-loaded marker", v)
	}
}

func TestEmptyAndMalformedLists(t *testing.T) {
	f := &fixture{}
	root := iri(shape)
	ps, bad := bn("ps"), bn("bad")
	f.add(0, root, shNS+"property", ps)
	f.add(1, ps, shNS+"ignoredProperties", iri(rdfNil))
	f.add(1, ps, shNS+"in", bad)
	f.add(1, bad, rdfFirst, lit("a"))
	f.add(1, bad, rdfFirst, lit("b")) // two rdf:first: not a list
	f.add(1, bad, rdfRest, iri(rdfNil))

	_, tree := build(f, "")
	node := tree.Roots[0].Props[0].Values[0].Node
	if v := propByCURIE(node.Props, "sh:ignoredProperties").Values[0]; v.Kind != KindNil {
		t.Errorf("rdf:nil = %+v", v)
	}
	v := propByCURIE(node.Props, "sh:in").Values[0]
	if v.Kind != KindNode || !v.Node.Malformed {
		t.Errorf("malformed list = %+v", v)
	}
}

func TestRowCapAndEmpty(t *testing.T) {
	f := &fixture{}
	for i := 0; i <= RowCap; i++ {
		f.add(0, iri(cubeIRI), "http://ex.org/p", bn(fmt.Sprint(i)))
	}
	g, tree := build(f, "")
	if !g.Truncated || !tree.Truncated {
		t.Error("row cap not flagged")
	}
	if NewGraph(nil).Empty() != true {
		t.Error("empty graph not empty")
	}
}

func TestRowsFrom(t *testing.T) {
	res := sparql.QueryResult{Bindings: []map[string]sparql.Binding{
		{"d": lit("1"), "s": bn("x"), "p": iri(schema + "name"), "o": lit("y")},
		{"d": lit("oops"), "s": bn("x"), "p": iri(schema + "name"), "o": lit("y")},
	}}
	rows := RowsFrom(res)
	if len(rows) != 1 || rows[0].Depth != 1 {
		t.Errorf("rows = %+v", rows)
	}
}

func TestCompact(t *testing.T) {
	ps := []config.Prefix{{Name: "cube", URI: "<https://cube.link/>"}, {Name: "meta", URI: "https://cube.link/meta/"}}
	if p, l := Compact("https://cube.link/meta/dataKind", ps); p != "meta" || l != "dataKind" {
		t.Errorf("longest match: %s:%s", p, l)
	}
	if p, l := Compact("https://example.org/a/b", ps); p != "" || l != "b" {
		t.Errorf("no match: %q %q", p, l)
	}
}

// --- query -------------------------------------------------------------------

func TestQuery(t *testing.T) {
	q := Request{IRI: shape}.Query()
	for _, want := range []string{
		"VALUES ?n0 { <" + shape + "> }",
		"BIND(6 AS ?d)",
		fmt.Sprintf("LIMIT %d", RowCap+1),
		"<" + rdfRest + ">*/<" + rdfFirst + ">",
	} {
		if !strings.Contains(q, want) {
			t.Errorf("query lacks %q", want)
		}
	}
	if strings.Contains(q, "rdf:") {
		t.Error("query must not depend on the rdf: prefix")
	}
	if strings.Contains(q, "BIND(7 AS ?d)") {
		t.Error("query goes deeper than MaxDepth+1")
	}
	if n := strings.Count(q, "UNION\n  {"); n != 2*queryDepth {
		t.Errorf("top-level branches = %d, want %d", n+1, 2*queryDepth+1)
	}

	probe := Request{IRI: shape}.QueryTo(ProbeDepth)
	if !strings.Contains(probe, fmt.Sprintf("BIND(%d AS ?d)", ProbeDepth)) ||
		strings.Contains(probe, fmt.Sprintf("BIND(%d AS ?d)", ProbeDepth+1)) {
		t.Error("probe query must stop at ProbeDepth")
	}
	if n := strings.Count(probe, "UNION\n  {"); n != 2*ProbeDepth {
		t.Errorf("probe branches = %d, want %d", n+1, 2*ProbeDepth+1)
	}

	q = Request{IRI: cubeIRI, Property: schema + "contactPoint", Via: "https://cube.link/observationConstraint"}.Query()
	if !strings.Contains(q, "<"+cubeIRI+"> <https://cube.link/observationConstraint> ?n0 . FILTER(isIRI(?n0))") {
		t.Error("via root pattern missing")
	}
	if !strings.Contains(q, "?n0 <"+schema+"contactPoint> ?n1") || strings.Contains(q, "?h1") {
		t.Error("property must fix the first hop")
	}
}

// --- serializers ---------------------------------------------------------------

func TestNTriplesRoundTrip(t *testing.T) {
	g, _ := build(shapeFixture(), "")
	nt := g.NTriples()
	lines := strings.Split(strings.TrimSpace(nt), "\n")
	if len(lines) != len(shapeFixture().rows) {
		t.Fatalf("N-Triples has %d lines, fixture %d triples", len(lines), len(shapeFixture().rows))
	}
	// Same triple set up to blank-node relabelling: compare with labels erased.
	norm := func(s string) string {
		var out []string
		for _, f := range strings.Fields(s) {
			if strings.HasPrefix(f, "_:") {
				f = "_:"
			}
			out = append(out, f)
		}
		return strings.Join(out, " ")
	}
	var got, want []string
	for _, l := range lines {
		got = append(got, norm(l))
	}
	l := &labeler{ids: map[string]string{}}
	for _, r := range shapeFixture().rows {
		want = append(want, norm(fmt.Sprintf("%s <%s> %s .", ntTerm(r.S, l), r.P.Value, ntTerm(r.O, l))))
	}
	sort.Strings(got)
	sort.Strings(want)
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("triple sets differ\n got: %v\nwant: %v", got, want)
	}
	if !strings.Contains(nt, `"Fläche"@de`) || !strings.Contains(nt, `"12"^^<`+xsd+`integer>`) {
		t.Error("literal forms wrong")
	}
}

func TestTurtle(t *testing.T) {
	f := shapeFixture()
	f.add(1, bn("kult"), schema+"description", lit("line\n\"quoted\""))
	g, _ := build(f, "")
	ttl := g.Turtle(testPrefixes)
	for _, want := range []string{
		"@prefix sh: <" + shNS + "> .",
		"<" + shape + ">",
		"sh:property [",
		"sh:or ( [",
		`"line\n\"quoted\""`,
		"a cube:KeyDimension",
	} {
		if !strings.Contains(ttl, want) {
			t.Errorf("turtle lacks %q:\n%s", want, ttl)
		}
	}
	if strings.Contains(ttl, "_:") {
		t.Errorf("a tree without shared nodes needs no labels:\n%s", ttl)
	}

	f2 := &fixture{}
	jane := bn("jane")
	f2.add(0, iri(cubeIRI), "http://ex.org/a", jane)
	f2.add(0, iri(cubeIRI), "http://ex.org/b", jane)
	f2.add(1, jane, schema+"name", lit("Jane"))
	g2, _ := build(f2, "")
	if ttl := g2.Turtle(testPrefixes); strings.Count(ttl, "_:b0") != 3 {
		t.Errorf("shared node should be labelled and referenced twice:\n%s", ttl)
	}
}

func TestJSONLD(t *testing.T) {
	g, _ := build(shapeFixture(), "")
	out, err := g.JSONLD()
	if err != nil {
		t.Fatal(err)
	}
	var nodes []map[string]any
	if err := json.Unmarshal(out, &nodes); err != nil {
		t.Fatal(err)
	}
	if nodes[0]["@id"] != shape {
		t.Errorf("first node = %v", nodes[0]["@id"])
	}
	if !strings.Contains(string(out), `"@language": "de"`) || !strings.Contains(string(out), `"@type": [`) {
		t.Errorf("JSON-LD lacks language or @type:\n%s", out)
	}
}

func TestSkolemList(t *testing.T) {
	// i14y writes sh:in as a list of skolem IRIs instead of blank nodes.
	const genid = "https://register.ld.admin.ch/i14y/x/.well-known/genid/"
	f := &fixture{}
	root := iri(shape)
	c0, c1 := iri(genid+"c0"), iri(genid+"c1")
	f.add(0, root, shNS+"in", c0)
	f.add(0, c0, rdfFirst, lit("1"))
	f.add(0, c0, rdfRest, c1)
	f.add(0, c1, rdfFirst, lit("2"))
	f.add(0, c1, rdfRest, iri(rdfNil))

	g, tree := build(f, "")
	if len(g.Roots) != 1 {
		t.Fatalf("roots = %v, skolem list cells must not become roots", g.Roots)
	}
	v := tree.Roots[0].Props[0].Values[0]
	if v.Kind != KindEnum || v.Count != 2 {
		t.Errorf("skolem sh:in = %+v, want an enumeration of 2", v)
	}
	if items, ok := g.List(c0); !ok || len(items) != 2 {
		t.Errorf("Graph.List = %v %v", items, ok)
	}
	if !NeedsDeeper([]Row{{Depth: 2, S: bn("x"), P: iri(shNS + "in"), O: c0}}, 2) {
		t.Error("a skolem IRI at the last level needs a deeper query")
	}
	// A skolem IRI whose triples were not fetched stays an ordinary link.
	f2 := &fixture{}
	f2.add(0, root, shNS+"property", bn("ps"))
	f2.add(1, bn("ps"), shNS+"node", iri(genid+"unfetched"))
	_, tree2 := build(f2, "")
	if v := propByCURIE(tree2.Roots[0].Props[0].Values[0].Node.Props, "sh:node").Values[0]; v.Kind != KindIRI {
		t.Errorf("unfetched skolem IRI = %+v, want a link", v)
	}
}

func TestQueryOptions(t *testing.T) {
	plain := Request{IRI: shape}.QueryTo(1)
	full := Request{IRI: shape, RootTriples: true, WithProperties: true}.QueryTo(1)

	if !strings.Contains(plain, "?n0 ?p ?o . FILTER(isBlank(?o) ||") {
		t.Error("without RootTriples level 0 keeps only node-valued triples")
	}
	if !strings.Contains(full, "    ?n0 ?p ?o .\n") {
		t.Error("RootTriples must return all of the root's triples")
	}
	want := "UNION { <" + shape + "> <" + shNS + "property> ?n0 . FILTER(isIRI(?n0)) }"
	if !strings.Contains(full, want) || strings.Contains(plain, want) {
		t.Error("WithProperties must add the IRI property shapes as roots")
	}
	if !strings.Contains(plain, `CONTAINS(STR(?n1), "/.well-known/genid/")`) {
		t.Error("hops must follow skolem IRIs")
	}
	via := Request{IRI: cubeIRI, Via: "https://cube.link/observationConstraint", WithProperties: true}.QueryTo(1)
	if strings.Contains(via, shNS+"property> ?n0") {
		t.Error("WithProperties is ignored with Via")
	}
}
