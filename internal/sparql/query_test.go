package sparql

import (
	"strings"
	"testing"

	"hutzli.org/visoto/internal/config"
)

func TestBuildPrefixBlock(t *testing.T) {
	tests := []struct {
		name     string
		prefixes []config.Prefix
		want     string
	}{
		{
			name:     "empty prefix list",
			prefixes: []config.Prefix{},
			want:     "",
		},
		{
			name: "single prefix",
			prefixes: []config.Prefix{
				{Name: "rdf", URI: "http://www.w3.org/1999/02/22-rdf-syntax-ns#"},
			},
			want: "PREFIX rdf: <http://www.w3.org/1999/02/22-rdf-syntax-ns#>\n\n",
		},
		{
			name: "multiple prefixes",
			prefixes: []config.Prefix{
				{Name: "rdf", URI: "http://www.w3.org/1999/02/22-rdf-syntax-ns#"},
				{Name: "rdfs", URI: "http://www.w3.org/2000/01/rdf-schema#"},
				{Name: "schema", URI: "http://schema.org/"},
			},
			want: "PREFIX rdf: <http://www.w3.org/1999/02/22-rdf-syntax-ns#>\n" +
				"PREFIX rdfs: <http://www.w3.org/2000/01/rdf-schema#>\n" +
				"PREFIX schema: <http://schema.org/>\n\n",
		},
		{
			name: "prefix with angle brackets",
			prefixes: []config.Prefix{
				{Name: "rdf", URI: "<http://www.w3.org/1999/02/22-rdf-syntax-ns#>"},
			},
			want: "PREFIX rdf: <http://www.w3.org/1999/02/22-rdf-syntax-ns#>\n\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildPrefixBlock(tt.prefixes)
			if got != tt.want {
				t.Errorf("buildPrefixBlock() =\n%q\nwant\n%q", got, tt.want)
			}
		})
	}
}

func TestHasExistingPrefixes(t *testing.T) {
	tests := []struct {
		name  string
		query string
		want  bool
	}{
		{
			name:  "query with PREFIX",
			query: "PREFIX rdf: <http://www.w3.org/1999/02/22-rdf-syntax-ns#>\nSELECT * WHERE { ?s ?p ?o }",
			want:  true,
		},
		{
			name:  "query with lowercase prefix",
			query: "prefix rdf: <http://www.w3.org/1999/02/22-rdf-syntax-ns#>\nSELECT * WHERE { ?s ?p ?o }",
			want:  true,
		},
		{
			name:  "query with PREFIX and leading whitespace",
			query: "  \n\tPREFIX rdf: <http://www.w3.org/1999/02/22-rdf-syntax-ns#>\nSELECT * WHERE { ?s ?p ?o }",
			want:  true,
		},
		{
			name:  "query without PREFIX",
			query: "SELECT * WHERE { ?s rdf:type schema:Person }",
			want:  false,
		},
		{
			name:  "query with PREFIX in middle",
			query: "SELECT * WHERE { ?s PREFIX ?o }",
			want:  false,
		},
		{
			name:  "empty query",
			query: "",
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hasExistingPrefixes(tt.query)
			if got != tt.want {
				t.Errorf("hasExistingPrefixes() = %v, want %v for query:\n%s", got, tt.want, tt.query)
			}
		})
	}
}

func TestPrefixPrepending(t *testing.T) {
	// Create test prefixes
	prefixes := []config.Prefix{
		{Name: "rdf", URI: "http://www.w3.org/1999/02/22-rdf-syntax-ns#"},
		{Name: "schema", URI: "http://schema.org/"},
	}

	// Create a test preprocessor
	preproc := New(Config{
		EndpointURL: "http://example.com/sparql",
		Prefixes:    prefixes,
	})

	tests := []struct {
		name          string
		query         string
		shouldPrepend bool
	}{
		{
			name:          "query without PREFIX should get prefixes prepended",
			query:         "SELECT * WHERE { ?s rdf:type schema:Person }",
			shouldPrepend: true,
		},
		{
			name: "query with existing PREFIX should not get duplicates",
			query: `PREFIX rdf: <http://www.w3.org/1999/02/22-rdf-syntax-ns#>
SELECT * WHERE { ?s rdf:type schema:Person }`,
			shouldPrepend: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// We can't directly call querySparqlEndpoint as it makes HTTP requests
			// But we can test the helper functions
			hasPrefixes := hasExistingPrefixes(tt.query)

			if tt.shouldPrepend && hasPrefixes {
				t.Error("Expected query to not have prefixes, but hasExistingPrefixes returned true")
			}
			if !tt.shouldPrepend && !hasPrefixes {
				t.Error("Expected query to have prefixes, but hasExistingPrefixes returned false")
			}

			// Test that buildPrefixBlock would produce valid output
			if tt.shouldPrepend {
				prefixBlock := buildPrefixBlock(preproc.config.Prefixes)
				if !strings.Contains(prefixBlock, "PREFIX rdf:") {
					t.Error("Expected prefix block to contain 'PREFIX rdf:'")
				}
				if !strings.Contains(prefixBlock, "PREFIX schema:") {
					t.Error("Expected prefix block to contain 'PREFIX schema:'")
				}
			}
		})
	}
}
