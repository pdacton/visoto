package resource

import (
	"testing"

	"hutzli.org/visoto/internal/config"
)

func TestIRIExpansionAndShortening(t *testing.T) {
	// Setup test prefixes
	prefixes := []config.Prefix{
		{Name: "schema", URI: "<http://schema.org/>"},
		{Name: "rdf", URI: "<http://www.w3.org/1999/02/22-rdf-syntax-ns#>"},
		{Name: "regch", URI: "<http://register.ld.admin.ch/>"},
	}

	tests := []struct {
		name          string
		input         string
		expectedFull  string
		expectedShort string
	}{
		{
			name:          "Full IRI should be shortened",
			input:         "http://schema.org/Person",
			expectedFull:  "http://schema.org/Person",
			expectedShort: "schema:Person",
		},
		{
			name:          "Prefixed IRI should be expanded",
			input:         "schema:Organization",
			expectedFull:  "http://schema.org/Organization",
			expectedShort: "schema:Organization",
		},
		{
			name:          "IRI without matching prefix stays full",
			input:         "http://example.org/Something",
			expectedFull:  "http://example.org/Something",
			expectedShort: "http://example.org/Something",
		},
		{
			name:          "Custom prefix from config",
			input:         "regch:zefix/company/123",
			expectedFull:  "http://register.ld.admin.ch/zefix/company/123",
			expectedShort: "regch:zefix/company/123",
		},
		{
			name:          "RDF namespace prefix",
			input:         "rdf:type",
			expectedFull:  "http://www.w3.org/1999/02/22-rdf-syntax-ns#type",
			expectedShort: "rdf:type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, err := New(tt.input, prefixes)
			if err != nil {
				t.Fatalf("Error creating resource: %v", err)
			}

			if r.IRI != tt.expectedFull {
				t.Errorf("Full IRI mismatch:\n  got:  %s\n  want: %s", r.IRI, tt.expectedFull)
			}

			if r.ShortIRI != tt.expectedShort {
				t.Errorf("Short IRI mismatch:\n  got:  %s\n  want: %s", r.ShortIRI, tt.expectedShort)
			}
		})
	}
}

// TestShortenIRIPrefersLongestPrefix pins the nested-namespace rule: several
// configured prefixes sit inside another (meta: and relation: are both under
// cube:), and the shortened string is what template resolution turns into a
// filename. Matching in declaration order instead produced "cube:meta/X" — not a
// legal CURIE, and a different string than the "meta:X" the same resource is
// linked as elsewhere, so one resource resolved to two different templates
// depending on which link the user followed.
func TestShortenIRIPrefersLongestPrefix(t *testing.T) {
	// Declaration order deliberately puts the SHORTER namespace first, which is
	// how visoto.config lists them.
	prefixes := []config.Prefix{
		{Name: "cube", URI: "<https://cube.link/>"},
		{Name: "meta", URI: "<https://cube.link/meta/>"},
		{Name: "relation", URI: "<https://cube.link/relation/>"},
		{Name: "schema", URI: "<http://schema.org/>"},
	}

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"nested namespace wins over parent", "https://cube.link/meta/SharedDimension", "meta:SharedDimension"},
		{"second nested namespace", "https://cube.link/relation/StandardError", "relation:StandardError"},
		{"parent namespace still matches its own terms", "https://cube.link/Cube", "cube:Cube"},
		{"unrelated namespace unaffected", "http://schema.org/Person", "schema:Person"},
		{"no matching prefix stays full", "http://example.org/Thing", "http://example.org/Thing"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shortenIRI(tt.input, prefixes); got != tt.want {
				t.Errorf("shortenIRI(%q):\n  got:  %s\n  want: %s", tt.input, got, tt.want)
			}
		})
	}
}

func TestNamedGraphsQuery(t *testing.T) {
	tests := []struct {
		name     string
		iri      string
		expected string
		ok       bool
	}{
		{
			name:     "Valid IRI produces graph-membership query",
			iri:      "http://schema.org/Person",
			expected: "SELECT DISTINCT ?g WHERE { GRAPH ?g { <http://schema.org/Person> ?p ?o } } LIMIT 50",
			ok:       true,
		},
		{
			name: "Empty IRI is rejected",
			iri:  "",
			ok:   false,
		},
		{
			name: "IRI with angle bracket is rejected",
			iri:  "http://example.org/x>. } SELECT ?s WHERE { ?s ?p ?o",
			ok:   false,
		},
		{
			name: "IRI with whitespace is rejected",
			iri:  "http://example.org/a b",
			ok:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			query, ok := namedGraphsQuery(tt.iri)
			if ok != tt.ok {
				t.Fatalf("ok mismatch: got %v, want %v", ok, tt.ok)
			}
			if ok && query != tt.expected {
				t.Errorf("Query mismatch:\n  got:  %s\n  want: %s", query, tt.expected)
			}
		})
	}
}
