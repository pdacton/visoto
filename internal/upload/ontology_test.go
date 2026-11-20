package upload

import (
	"strings"
	"testing"

	"github.com/pdacton/visoto/internal/sparql"
)

func TestNamespaceOf(t *testing.T) {
	cases := map[string]string{
		"https://cube.link/Cube":              "https://cube.link/",
		"http://www.w3.org/2002/07/owl#Class": "http://www.w3.org/2002/07/owl#",
		"http://qudt.org/schema/qudt/Unit":    "http://qudt.org/schema/qudt/",
		"https://example.org":                 "",
		"urn:x:y":                             "",
	}
	for in, want := range cases {
		if got := namespaceOf(in); got != want {
			t.Errorf("namespaceOf(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDominantNamespace(t *testing.T) {
	terms := []string{
		"https://cube.link/Cube", "https://cube.link/Observation", "https://cube.link/observation",
		"http://www.w3.org/ns/shacl#NodeShape",
	}
	if got := dominantNamespace(terms); got != "https://cube.link/" {
		t.Errorf("dominantNamespace = %q, want https://cube.link/", got)
	}
	// A tie resolves lexically, so the guess is stable across runs.
	if got := dominantNamespace([]string{"https://b.org/X", "https://a.org/Y"}); got != "https://a.org/" {
		t.Errorf("tie = %q, want https://a.org/", got)
	}
	if got := dominantNamespace(nil); got != "" {
		t.Errorf("no terms = %q, want empty", got)
	}
}

func TestOntologyInsert(t *testing.T) {
	d := ontologyDeclaration{
		graph:  sparql.MustIRI("urn:ontology:cube.link/cube"),
		label:  `cube "link"`,
		source: "https://cube.link/",
	}
	got := d.insert(sparql.MustIRI("https://cube.link/"))
	for _, want := range []string{
		"INSERT DATA { GRAPH <urn:ontology:cube.link/cube> {",
		"<https://cube.link/> " + rdfTypeTerm + " " + owlOntologyTerm,
		rdfsCommentTerm + ` "` + generatedComment + `"`,
		rdfsLabelTerm + ` "cube \"link\""`,
		dctermsSrcTerm + " <https://cube.link/>",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("insert missing %q in:\n%s", want, got)
		}
	}

	// File mode: no source, no label — neither triple is written.
	d.source, d.label = "", ""
	got = d.insert(sparql.MustIRI("https://cube.link/"))
	if strings.Contains(got, dctermsSrcTerm) || strings.Contains(got, rdfsLabelTerm) {
		t.Errorf("file-mode insert carries source or label:\n%s", got)
	}
}
