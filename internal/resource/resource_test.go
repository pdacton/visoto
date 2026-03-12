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
