package sparql

import (
	"strings"
	"testing"

	"hutzli.org/visoto/internal/config"
)

func TestExtractDeclaredPrefixes(t *testing.T) {
	tests := []struct {
		name  string
		query string
		want  map[string]bool
	}{
		{
			name:  "query with PREFIX",
			query: "PREFIX rdf: <http://www.w3.org/1999/02/22-rdf-syntax-ns#>\nSELECT * WHERE { ?s ?p ?o }",
			want:  map[string]bool{"rdf": true},
		},
		{
			name:  "query with lowercase prefix keyword",
			query: "prefix rdf: <http://www.w3.org/1999/02/22-rdf-syntax-ns#>\nSELECT * WHERE { ?s ?p ?o }",
			want:  map[string]bool{"rdf": true},
		},
		{
			name:  "query with multiple prefixes",
			query: "PREFIX rdf: <http://www.w3.org/1999/02/22-rdf-syntax-ns#>\nPREFIX schema: <http://schema.org/>\nSELECT * WHERE { ?s ?p ?o }",
			want:  map[string]bool{"rdf": true, "schema": true},
		},
		{
			name:  "query without PREFIX",
			query: "SELECT * WHERE { ?s rdf:type schema:Person }",
			want:  map[string]bool{},
		},
		{
			name:  "empty query",
			query: "",
			want:  map[string]bool{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractDeclaredPrefixes(tt.query)
			if len(got) != len(tt.want) {
				t.Errorf("extractDeclaredPrefixes() returned %d prefixes, want %d", len(got), len(tt.want))
			}
			for prefix := range tt.want {
				if !got[prefix] {
					t.Errorf("extractDeclaredPrefixes() missing prefix %q", prefix)
				}
			}
		})
	}
}

func TestExtractUsedPrefixes(t *testing.T) {
	tests := []struct {
		name  string
		query string
		want  map[string]bool
	}{
		{
			name:  "query with single prefix usage",
			query: "SELECT * WHERE { ?s rdf:type ?o }",
			want:  map[string]bool{"rdf": true},
		},
		{
			name:  "query with multiple prefix usages",
			query: "SELECT * WHERE { ?s rdf:type schema:Person . ?s skos:prefLabel ?label }",
			want:  map[string]bool{"rdf": true, "schema": true, "skos": true},
		},
		{
			name:  "query with URL (should exclude http/https)",
			query: "SELECT * WHERE { ?s ?p <http://example.com/resource> }",
			want:  map[string]bool{},
		},
		{
			name:  "query with both prefixes and URLs",
			query: "SELECT * WHERE { ?s rdf:type <https://example.com/Person> }",
			want:  map[string]bool{"rdf": true},
		},
		{
			name:  "empty query",
			query: "",
			want:  map[string]bool{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractUsedPrefixes(tt.query)
			if len(got) != len(tt.want) {
				t.Errorf("extractUsedPrefixes() returned %d prefixes, want %d. Got: %v", len(got), len(tt.want), got)
			}
			for prefix := range tt.want {
				if !got[prefix] {
					t.Errorf("extractUsedPrefixes() missing prefix %q", prefix)
				}
			}
		})
	}
}

func TestBuildNeededPrefixBlock(t *testing.T) {
	prefixes := []config.Prefix{
		{Name: "rdf", URI: "http://www.w3.org/1999/02/22-rdf-syntax-ns#"},
		{Name: "rdfs", URI: "http://www.w3.org/2000/01/rdf-schema#"},
		{Name: "schema", URI: "http://schema.org/"},
		{Name: "skos", URI: "http://www.w3.org/2004/02/skos/core#"},
	}

	tests := []struct {
		name        string
		usedSet     map[string]bool
		declaredSet map[string]bool
		wantContain []string
		wantOmit    []string
	}{
		{
			name:        "all prefixes used, none declared",
			usedSet:     map[string]bool{"rdf": true, "schema": true},
			declaredSet: map[string]bool{},
			wantContain: []string{"PREFIX rdf:", "PREFIX schema:"},
			wantOmit:    []string{"PREFIX rdfs:", "PREFIX skos:"},
		},
		{
			name:        "some prefixes already declared",
			usedSet:     map[string]bool{"rdf": true, "schema": true},
			declaredSet: map[string]bool{"rdf": true},
			wantContain: []string{"PREFIX schema:"},
			wantOmit:    []string{"PREFIX rdf:", "PREFIX rdfs:", "PREFIX skos:"},
		},
		{
			name:        "all prefixes declared",
			usedSet:     map[string]bool{"rdf": true, "schema": true},
			declaredSet: map[string]bool{"rdf": true, "schema": true},
			wantContain: []string{},
			wantOmit:    []string{"PREFIX rdf:", "PREFIX schema:"},
		},
		{
			name:        "no prefixes used",
			usedSet:     map[string]bool{},
			declaredSet: map[string]bool{},
			wantContain: []string{},
			wantOmit:    []string{"PREFIX rdf:", "PREFIX schema:"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildNeededPrefixBlock(prefixes, tt.usedSet, tt.declaredSet)
			for _, want := range tt.wantContain {
				if !strings.Contains(got, want) {
					t.Errorf("buildNeededPrefixBlock() should contain %q, got:\n%s", want, got)
				}
			}
			for _, omit := range tt.wantOmit {
				if strings.Contains(got, omit) {
					t.Errorf("buildNeededPrefixBlock() should not contain %q, got:\n%s", omit, got)
				}
			}
		})
	}
}

func TestFinalizeQuery(t *testing.T) {
	prefixes := []config.Prefix{
		{Name: "rdf", URI: "http://www.w3.org/1999/02/22-rdf-syntax-ns#"},
		{Name: "schema", URI: "http://schema.org/"},
		{Name: "schch", URI: "https://schema.ld.admin.ch/"},
	}

	preproc := New(QueryInput{
		EndpointURL: "http://example.com/sparql",
		Prefixes:    prefixes,
	})

	tests := []struct {
		name        string
		query       string
		wantContain []string
		wantOmit    []string
	}{
		{
			name:        "query without prefixes should get needed prefixes added",
			query:       "SELECT * WHERE { ?s rdf:type schema:Person }",
			wantContain: []string{"PREFIX rdf:", "PREFIX schema:", "SELECT * WHERE"},
			wantOmit:    []string{"PREFIX schch:"},
		},
		{
			name:        "query with schch prefix usage",
			query:       "SELECT * WHERE { ?s a schch:TerminologyCollection }",
			wantContain: []string{"PREFIX schch:", "SELECT * WHERE"},
			wantOmit:    []string{"PREFIX rdf:", "PREFIX schema:"},
		},
		{
			name:        "query with existing prefix should not duplicate",
			query:       "PREFIX rdf: <http://www.w3.org/1999/02/22-rdf-syntax-ns#>\nSELECT * WHERE { ?s rdf:type schema:Person }",
			wantContain: []string{"PREFIX schema:", "PREFIX rdf:", "SELECT * WHERE"},
			wantOmit:    []string{"PREFIX schch:"},
		},
		{
			name:        "query with all needed prefixes declared",
			query:       "PREFIX rdf: <http://www.w3.org/1999/02/22-rdf-syntax-ns#>\nPREFIX schema: <http://schema.org/>\nSELECT * WHERE { ?s rdf:type schema:Person }",
			wantContain: []string{"PREFIX rdf:", "PREFIX schema:", "SELECT * WHERE"},
			wantOmit:    []string{"PREFIX schch:"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := preproc.finalizeQuery(tt.query)
			for _, want := range tt.wantContain {
				if !strings.Contains(got, want) {
					t.Errorf("finalizeQuery() should contain %q, got:\n%s", want, got)
				}
			}
			for _, omit := range tt.wantOmit {
				// Count occurrences - if prefix was in original query, that's OK
				// We just don't want it added if it wasn't there
				originalCount := strings.Count(tt.query, omit)
				finalCount := strings.Count(got, omit)
				if finalCount > originalCount {
					t.Errorf("finalizeQuery() should not add %q, got:\n%s", omit, got)
				}
			}
		})
	}
}
