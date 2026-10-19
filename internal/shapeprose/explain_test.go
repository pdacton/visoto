package shapeprose

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"hutzli.org/visoto/internal/bnodetree"
	"hutzli.org/visoto/internal/config"
	"hutzli.org/visoto/internal/sparql"
)

var update = flag.Bool("update", false, "rewrite the golden files in testdata")

var testPrefixes = []config.Prefix{
	{Name: "sh", URI: sh}, {Name: "rdf", URI: rdf}, {Name: "rdfs", URI: rdfs}, {Name: "xsd", URI: xsd},
	{Name: "dct", URI: dct}, {Name: "dcat", URI: "http://www.w3.org/ns/dcat#"}, {Name: "foaf", URI: "http://xmlns.com/foaf/0.1/"},
	{Name: "schema", URI: schema}, {Name: "cube", URI: cube}, {Name: "meta", URI: meta}, {Name: "qudt", URI: qudt},
	{Name: "skos", URI: "http://www.w3.org/2004/02/skos/core#"}, {Name: "prov", URI: prov}, {Name: "adms", URI: "http://www.w3.org/ns/adms#"},
	{Name: "vcard", URI: "http://www.w3.org/2006/vcard/ns#"}, {Name: "time", URI: timeNS},
}

// loadRows reads a fixture: one JSON array per row, [d, s, p, o], each term
// ["u", iri] / ["b", id] / ["l", value, lang, datatype]. The fixtures are real
// responses of the summary query (QueryTo(3) with RootTriples and
// WithProperties), taken from QLever and LINDAS on 2026-10-19.
func loadRows(t *testing.T, name string) *bnodetree.Graph {
	t.Helper()
	f, err := os.Open(filepath.Join("testdata", name+".rows"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	term := func(raw []any) sparql.Binding {
		b := sparql.Binding{Value: raw[1].(string)}
		switch raw[0] {
		case "u":
			b.Type = "uri"
		case "b":
			b.Type = "bnode"
		default:
			b.Type, b.Lang, b.Datatype, b.DisplayText = "literal", raw[2].(string), raw[3].(string), b.Value
		}
		return b
	}
	var rows []bnodetree.Row
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		var r []any
		if err := json.Unmarshal(sc.Bytes(), &r); err != nil {
			t.Fatal(err)
		}
		rows = append(rows, bnodetree.Row{
			Depth: int(r[0].(float64)),
			S:     term(r[1].([]any)), P: term(r[2].([]any)), O: term(r[3].([]any)),
		})
	}
	return bnodetree.NewGraph(rows)
}

// dump renders a Summary as plain text, one fact per line, for golden files.
func dump(s *Summary) string {
	var b strings.Builder
	for _, p := range s.Targets {
		fmt.Fprintf(&b, "TARGET %s\n", partString(p))
	}
	if s.Closed {
		fmt.Fprintf(&b, "CLOSED except %s\n", itemsString(s.Ignored))
	}
	if s.Self != nil {
		b.WriteString("SELF\n")
		dumpLine(&b, s.Self, "  ")
	}
	for _, l := range s.Lines {
		dumpLine(&b, l, "")
	}
	return b.String()
}

func dumpLine(b *strings.Builder, l *Line, indent string) {
	fmt.Fprintf(b, "%s- %q (%s)", indent, l.Label, l.Path)
	for _, p := range l.Parts {
		fmt.Fprintf(b, " | %s", partString(p))
	}
	if l.Severity != "" {
		fmt.Fprintf(b, " [%s]", l.Severity)
	}
	if l.Deactivated {
		b.WriteString(" [deactivated]")
	}
	b.WriteString("\n")
	for _, d := range l.Descriptions {
		fmt.Fprintf(b, "%s    desc: %s\n", indent, d)
	}
	for _, f := range l.Fallbacks {
		fmt.Fprintf(b, "%s    also: %s %s\n", indent, f.Pred.CURIE(), itemsString(f.Items))
	}
	for _, g := range l.Groups {
		fmt.Fprintf(b, "%s    %s:\n", indent, g.Kind)
		for _, c := range g.Lines {
			dumpLine(b, c, indent+"      ")
		}
	}
}

func partString(p Part) string {
	s := p.Kind
	if p.Code != "" {
		s += "." + p.Code
	}
	if p.N != 0 || p.M != 0 {
		s += fmt.Sprintf("(%d,%d)", p.N, p.M)
	}
	if len(p.Items) > 0 {
		s += " " + itemsString(p.Items)
	}
	if p.More > 0 {
		s += fmt.Sprintf(" +%d", p.More)
	}
	for _, in := range p.Inner {
		s += " {" + partString(in) + "}"
	}
	return s
}

func itemsString(items []Item) string {
	var b strings.Builder
	for _, it := range items {
		switch it.Sep {
		case "comma":
			b.WriteString(", ")
		case "":
		default:
			b.WriteString(" " + it.Sep + " ")
		}
		switch {
		case it.Term != nil:
			fmt.Fprintf(&b, "%s[%s]", it.Term.Label, it.Term.CURIE())
		case it.Code != "" && it.Text == "":
			b.WriteString("#" + it.Code)
		default:
			b.WriteString(fmt.Sprintf("%q", it.Text))
			if it.Code != "" {
				b.WriteString("/" + it.Code)
			}
		}
	}
	return b.String()
}

func golden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name+".golden")
	if *update {
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run with -update to create it)", err)
	}
	if got != string(want) {
		t.Errorf("%s differs from golden (run with -update after checking):\n%s", name, got)
	}
}

func TestGolden(t *testing.T) {
	cases := []struct{ fixture, shape, lang string }{
		{"dcatap-dataset", "https://register.ld.admin.ch/mds/shapes/dcat-ap/3.0.0#DatasetShape", "en"},
		{"dcatap-catalogrecord", "https://register.ld.admin.ch/mds/shapes/dcat-ap/3.0.0#CatalogRecordShape", "en"},
		{"dcatapch-dataset", "https://register.ld.admin.ch/mds/shapes/dcat-ap-ch/3.0.1#DatasetShape", "de"},
		{"dcatapch-datasetseries", "https://register.ld.admin.ch/mds/shapes/dcat-ap-ch/3.0.1#DatasetSeriesShape", "en"},
		{"lindas-lake-constraint", "https://environment.ld.admin.ch/foen/hydro/lake/shape", "en"},
		{"i14y-grenzgaenger", "https://register.ld.admin.ch/i14y/dataset/5-30-Bew-GG/structure/erteilte_grenzgängerbewilligungen", "en"},
	}
	for _, c := range cases {
		t.Run(c.fixture, func(t *testing.T) {
			g := loadRows(t, c.fixture)
			s := Explain(g, c.shape, Options{Lang: c.lang, Prefixes: testPrefixes})
			if s.Empty() {
				t.Fatal("empty summary")
			}
			golden(t, c.fixture, dump(s))
		})
	}
}

// --- synthetic shapes ---------------------------------------------------------

type fixture struct{ rows []bnodetree.Row }

func u(v string) sparql.Binding  { return sparql.Binding{Type: "uri", Value: v} }
func bn(v string) sparql.Binding { return sparql.Binding{Type: "bnode", Value: v} }
func lit(v string) sparql.Binding {
	return sparql.Binding{Type: "literal", Value: v, DisplayText: v}
}

func (f *fixture) add(d int, s sparql.Binding, p string, o sparql.Binding) {
	f.rows = append(f.rows, bnodetree.Row{Depth: d, S: s, P: u(p), O: o})
}

// list adds an RDF list of items under cell prefix c, owned by a node at depth d.
func (f *fixture) list(d int, c string, items ...sparql.Binding) sparql.Binding {
	for i, it := range items {
		cell := bn(fmt.Sprintf("%s%d", c, i))
		f.add(d, cell, rdfFirst, it)
		next := u(rdfNil)
		if i < len(items)-1 {
			next = bn(fmt.Sprintf("%s%d", c, i+1))
		}
		f.add(d, cell, rdfRest, next)
	}
	return bn(c + "0")
}

// TestCoverage: every SHACL Core predicate is consumed by a rule (or read as
// metadata), so a missing rule fails here instead of quietly becoming an
// "Also:" line. sh:sparql (SHACL-SPARQL) is the deliberate exception.
func TestCoverage(t *testing.T) {
	const shapeIRI = "https://example.org/Shape"
	f := &fixture{}
	root, ps, alt1, alt2, q, nested := u(shapeIRI), bn("ps"), bn("a1"), bn("a2"), bn("q"), bn("n")
	f.add(0, root, rdfType, u(sh+"NodeShape"))
	for _, p := range []string{"targetClass", "targetNode", "targetSubjectsOf", "targetObjectsOf"} {
		f.add(0, root, sh+p, u("https://example.org/T"))
	}
	f.add(0, root, sh+"closed", lit("true"))
	f.add(0, root, sh+"ignoredProperties", f.list(0, "ig", u(rdfType)))
	f.add(0, root, sh+"property", ps)

	f.add(1, ps, sh+"path", u("https://example.org/p"))
	for p, o := range map[string]sparql.Binding{
		"name": lit("p"), "description": lit("d"), "message": lit("m"), "severity": u(sh + "Warning"),
		"order": lit("1"), "group": u("https://example.org/G"), "deactivated": lit("false"),
		"defaultValue": lit("x"), "minCount": lit("1"), "maxCount": lit("2"),
		"class": u("https://example.org/C"), "datatype": u(xsd + "string"), "nodeKind": u(sh + "Literal"),
		"minInclusive": lit("1"), "maxInclusive": lit("9"), "minExclusive": lit("0"), "maxExclusive": lit("10"),
		"minLength": lit("1"), "maxLength": lit("5"), "pattern": lit("^a"), "flags": lit("i"),
		"uniqueLang": lit("true"), "hasValue": lit("v"),
		"equals": u("https://example.org/e"), "disjoint": u("https://example.org/d"),
		"lessThan": u("https://example.org/l"), "lessThanOrEquals": u("https://example.org/le"),
		"node": u("https://example.org/OtherShape"), "qualifiedMinCount": lit("1"), "qualifiedMaxCount": lit("3"),
		"qualifiedValueShapesDisjoint": lit("true"), "sparql": bn("sp"),
	} {
		f.add(1, ps, sh+p, o)
	}
	f.add(1, ps, sh+"languageIn", f.list(1, "lg", lit("de"), lit("fr")))
	f.add(1, ps, sh+"in", f.list(1, "in", lit("a"), lit("b")))
	f.add(1, ps, sh+"or", f.list(1, "or", alt1, alt2))
	f.add(2, alt1, sh+"datatype", u(xsd+"date"))
	f.add(2, alt2, sh+"datatype", u(xsd+"dateTime"))
	f.add(1, ps, sh+"xone", f.list(1, "xo", alt1, alt2))
	f.add(1, ps, sh+"and", f.list(1, "an", alt1, alt2))
	f.add(1, ps, sh+"not", nested)
	f.add(2, nested, sh+"class", u("https://example.org/Draft"))
	f.add(1, ps, sh+"qualifiedValueShape", q)
	f.add(2, q, sh+"class", u("https://example.org/Q"))

	s := Explain(bnodetree.NewGraph(f.rows), shapeIRI, Options{Lang: "en", Prefixes: testPrefixes})
	if len(s.Targets) != 4 || !s.Closed || len(s.Ignored) != 1 || len(s.Lines) != 1 {
		t.Fatalf("targets/closed/lines wrong:\n%s", dump(s))
	}
	l := s.Lines[0]
	if len(l.Fallbacks) != 1 || l.Fallbacks[0].Pred.IRI != sh+"sparql" {
		t.Errorf("only sh:sparql may fall back, got %+v\n%s", l.Fallbacks, dump(s))
	}
	kinds := map[string]bool{}
	for _, p := range l.Parts {
		kinds[p.Kind] = true
	}
	// No "nodeKind": it is consumed, but not shown next to a class or datatype.
	for _, k := range []string{"card", "class", "datatype", "in", "hasValue", "range", "minEx", "maxEx",
		"length", "pattern", "languageIn", "uniqueLang", "qualified", "equals", "disjoint", "lessThan",
		"lessThanOrEquals", "node", "not", "default"} {
		if !kinds[k] {
			t.Errorf("no %q part:\n%s", k, dump(s))
		}
	}
	if l.Label != "P" || l.Severity != "warning" || l.Message != "m" || len(l.Descriptions) != 1 || !l.hasOrder || l.Group == "" {
		t.Errorf("metadata not read: %+v", l)
	}
	// sh:or and sh:xone of single datatypes flatten into the datatype part;
	// sh:and never flattens.
	if len(l.Groups) != 1 || l.Groups[0].Kind != "allOf" {
		t.Errorf("groups = %+v, want one allOf", l.Groups)
	}
}

func TestCardinality(t *testing.T) {
	for _, c := range []struct {
		min, max int
		code     string
	}{
		{-1, -1, "any"}, {0, -1, "any"}, {1, -1, "oneOrMore"}, {2, -1, "atLeast"},
		{1, 1, "exactlyOne"}, {3, 3, "exactly"}, {1, 4, "range"}, {0, 1, "atMostOne"},
		{-1, 5, "atMost"}, {0, 0, "none"},
	} {
		if got := cardinality(c.min, c.max).Code; got != c.code {
			t.Errorf("min %d max %d: %s, want %s", c.min, c.max, got, c.code)
		}
	}
}

func TestHumanize(t *testing.T) {
	for in, want := range map[string]string{
		"accrualPeriodicity":        "Accrual periodicity",
		"LinguisticSystem":          "Linguistic system",
		"spatialResolutionInMeters": "Spatial resolution in meters",
		"hasURL":                    "Has URL",
		"ktr_10":                    "Ktr 10",
		"Agent":                     "Agent",
	} {
		if got := humanize(in); got != want {
			t.Errorf("humanize(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPropertyShapeAndPaths(t *testing.T) {
	const psIRI = "https://example.org/ps"
	f := &fixture{}
	inv := bn("inv")
	f.add(0, u(psIRI), sh+"path", inv)
	f.add(1, inv, sh+"inversePath", u("http://www.w3.org/ns/dcat#inSeries"))
	f.add(0, u(psIRI), sh+"minCount", lit("1"))
	s := Explain(bnodetree.NewGraph(f.rows), psIRI, Options{Prefixes: testPrefixes})
	if !s.PropertyShape || len(s.Lines) != 1 {
		t.Fatalf("property shape: %s", dump(s))
	}
	l := s.Lines[0]
	if l.Path != "^dcat:inSeries" || l.Label != "In series" || l.PathTerm != nil {
		t.Errorf("inverse path: %q %q %v", l.Path, l.Label, l.PathTerm)
	}
	if len(l.Parts) < 2 || l.Parts[1].Kind != "inverse" {
		t.Errorf("inverse part missing: %s", dump(s))
	}
	if !(&Summary{}).Empty() || s.Empty() {
		t.Error("Empty")
	}
}
