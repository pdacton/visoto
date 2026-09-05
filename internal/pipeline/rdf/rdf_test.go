package rdf

import (
	"bytes"
	"strings"
	"testing"
)

func TestTermString(t *testing.T) {
	cases := []struct {
		name string
		term Term
		want string
	}{
		{"iri", IRI("http://example.org/a"), "<http://example.org/a>"},
		{"iri with space", IRI("http://example.org/a b"), `<http://example.org/a\u0020b>`},
		{"iri with angle", IRI("http://example.org/<x>"), `<http://example.org/\u003Cx\u003E>`},
		{"plain literal", Literal("hello"), `"hello"`},
		{"literal with quote", Literal(`say "hi"`), `"say \"hi\""`},
		{"literal with newline", Literal("a\nb"), `"a\nb"`},
		{"literal with backslash", Literal(`a\b`), `"a\\b"`},
		{"lang literal", LangLiteral("Bevölkerung", "de"), `"Bevölkerung"@de`},
		{"empty lang degrades", LangLiteral("x", "  "), `"x"`},
		{"typed literal", TypedLiteral("42", NSXSD+"integer"), `"42"^^<http://www.w3.org/2001/XMLSchema#integer>`},
		{"blank", Blank("b0"), "_:b0"},
		{"blank sanitized", Blank("a b/c"), "_:a_b_c"},
		{"blank leading dash", Blank("-x"), "_:_x"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.term.String(); got != tc.want {
				t.Errorf("String() = %s, want %s", got, tc.want)
			}
		})
	}
}

func TestQuadValid(t *testing.T) {
	s, p, o := IRI("http://example.org/s"), IRI("http://example.org/p"), Literal("o")
	cases := []struct {
		name string
		quad Quad
		want bool
	}{
		{"complete", NewQuad(s, p, o, "http://example.org/g"), true},
		{"no graph is still valid", NewQuad(s, p, o, ""), true},
		{"missing object", NewQuad(s, p, Term{}, "g"), false},
		{"missing subject", NewQuad(Term{}, p, o, "g"), false},
		{"literal predicate", NewQuad(s, Literal("p"), o, "g"), false},
		{"literal subject", NewQuad(Literal("s"), p, o, "g"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.quad.Valid(); got != tc.want {
				t.Errorf("Valid() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestQuadString(t *testing.T) {
	q := NewQuad(IRI("http://example.org/s"), DctTitle, LangLiteral("Titel", "de"), "http://example.org/g")
	want := `<http://example.org/s> <http://purl.org/dc/terms/title> "Titel"@de <http://example.org/g> .`
	if got := q.String(); got != want {
		t.Errorf("String() = %s, want %s", got, want)
	}

	def := q.InGraph("")
	if strings.Count(def.String(), "<") != 2 {
		t.Errorf("default graph quad must omit the graph term, got %s", def.String())
	}
	if q.Graph == "" {
		t.Error("InGraph must not mutate the receiver")
	}
}

func TestWriterSkipsInvalid(t *testing.T) {
	var buf bytes.Buffer
	w := NewWriter(&buf)
	s, p := IRI("http://example.org/s"), IRI("http://example.org/p")

	quads := []Quad{
		NewQuad(s, p, Literal("ok"), "g"),
		NewQuad(s, p, Term{}, "g"), // dropped, not fatal
		NewQuad(s, p, Literal("also ok"), "g"),
	}
	if err := w.WriteAll(quads); err != nil {
		t.Fatalf("WriteAll: %v", err)
	}
	if err := w.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	if w.Written() != 2 || w.Skipped() != 1 {
		t.Errorf("written=%d skipped=%d, want 2/1", w.Written(), w.Skipped())
	}
	if n := strings.Count(buf.String(), "\n"); n != 2 {
		t.Errorf("output has %d lines, want 2:\n%s", n, buf.String())
	}
}

func TestMinterIsDeterministic(t *testing.T) {
	m := NewMinter("https://example.org/id/")
	dist := "https://opendata.swiss/dataset/x/resource/a1b2"

	if got, want := m.StructureIRI(dist, "r7"), m.StructureIRI(dist, "r7"); got != want {
		t.Error("StructureIRI is not deterministic")
	}
	if m.StructureIRI(dist, "r7") == m.StructureIRI(dist, "r8") {
		t.Error("StructureIRI must be run-scoped")
	}
	if m.FieldIRI(dist, "r7", "col:1") == m.FieldIRI(dist, "r7", "col:2") {
		t.Error("FieldIRI must distinguish locators")
	}
	if m.Skolem(dist, "contact", "1") != m.Skolem(dist, "contact", "1") {
		t.Error("Skolem is not deterministic — re-runs would mint fresh IRIs")
	}
	if m.Skolem(dist, "contact", "1") == m.Skolem(dist, "contact", "2") {
		t.Error("Skolem must distinguish keys")
	}
	if m.SignatureIRI("k") != m.SignatureIRI("k") {
		t.Error("SignatureIRI is not deterministic")
	}
}

func TestMinterBaseNormalization(t *testing.T) {
	for _, base := range []string{"https://example.org/id", "https://example.org/id/"} {
		m := NewMinter(base)
		if got := m.Base(); got != "https://example.org/id/" {
			t.Errorf("NewMinter(%q).Base() = %q", base, got)
		}
	}
	if got := NewMinter("").Base(); got != DefaultBaseIRI {
		t.Errorf("empty base = %q, want %q", got, DefaultBaseIRI)
	}
	if got := NewMinter("https://example.org/ns#").Base(); got != "https://example.org/ns#" {
		t.Errorf("hash base must be preserved, got %q", got)
	}
}

func TestMinterGraphNames(t *testing.T) {
	m := NewMinter("https://example.org/id/")
	if got, want := m.CatalogGraph("opendata.swiss", "r7"), "https://example.org/id/graph/catalog/opendata-swiss/r7"; got != want {
		t.Errorf("CatalogGraph = %q, want %q", got, want)
	}
	if got, want := m.SourceIRI("Data Europa"), IRI("https://example.org/id/source/data-europa"); got != want {
		t.Errorf("SourceIRI = %v, want %v", got, want)
	}
}

func TestSlug(t *testing.T) {
	cases := map[string]string{
		"opendata.swiss": "opendata-swiss",
		"Data Europa":    "data-europa",
		"  spaced  ":     "spaced",
		"a//b":           "ab",
		"":               "unnamed",
		"!!!":            "unnamed",
	}
	for in, want := range cases {
		if got := slug(in); got != want {
			t.Errorf("slug(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestHashStability(t *testing.T) {
	// A change here silently re-mints every IRI in the store, so pin it.
	if got, want := Hash("http://example.org/a"), "6a2d6a47a2828fe0"; got != want {
		t.Errorf("Hash drifted: got %s, want %s — this invalidates every minted IRI", got, want)
	}
	if Hash("a", "b") == Hash("ab") {
		t.Error("Hash must separate its parts")
	}
}
