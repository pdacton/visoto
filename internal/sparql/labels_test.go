package sparql

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseAcceptLanguage(t *testing.T) {
	tests := []struct {
		name   string
		header string
		want   []string
	}{
		{
			name:   "empty header returns default",
			header: "",
			want:   []string{"en"},
		},
		{
			name:   "single language",
			header: "en-US",
			want:   []string{"en-US", "en"},
		},
		{
			name:   "multiple languages with quality values",
			header: "en-US,fr;q=0.9,de;q=0.8",
			want:   []string{"en-US", "en", "fr", "de"},
		},
		{
			name:   "languages without region codes",
			header: "en,fr,de",
			want:   []string{"en", "fr", "de"},
		},
		{
			name:   "languages sorted by quality",
			header: "de;q=0.7,en-US;q=0.9,fr",
			want:   []string{"fr", "en-US", "en", "de"},
		},
		{
			name:   "duplicate base language",
			header: "en-US,en-GB,en",
			want:   []string{"en-US", "en", "en-GB"}, // en deduplicated after first insertion
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseAcceptLanguage(tt.header)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parseAcceptLanguage() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestUnwrapQuotes(t *testing.T) {
	cases := map[string]string{
		`"1000/00893 Bundesverwaltung (Bern)"`: `1000/00893 Bundesverwaltung (Bern)`,
		`plain`:                                `plain`,
		`"Foo" und "Bar"`:                      `"Foo" und "Bar"`,
		`"starts only`:                         `"starts only`,
		`""`:                                   ``,
		`"`:                                    `"`,
	}
	for in, want := range cases {
		if got := unwrapQuotes(in); got != want {
			t.Errorf("unwrapQuotes(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestExtractLastSegment(t *testing.T) {
	tests := []struct {
		name string
		iri  string
		want string
	}{
		{
			name: "path segment",
			iri:  "http://example.com/resource/Person",
			want: "Person",
		},
		{
			name: "fragment",
			iri:  "http://example.com/ns#property",
			want: "property",
		},
		{
			name: "trailing slash",
			iri:  "http://example.com/path/to/resource/",
			want: "resource",
		},
		{
			name: "simple URL",
			iri:  "http://example.com/bar",
			want: "bar",
		},
		{
			name: "fragment has priority over path",
			iri:  "http://example.com/path/to#fragment",
			want: "fragment",
		},
		{
			name: "no slash or hash",
			iri:  "http://example.com",
			want: "example.com", // Extracts domain name
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractLastSegment(tt.iri)
			if got != tt.want {
				t.Errorf("extractLastSegment() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestExtractIRIs(t *testing.T) {
	tests := []struct {
		name   string
		result QueryResult
		want   []string
	}{
		{
			name: "mixed types",
			result: QueryResult{
				Vars: []string{"s", "p", "o"},
				Bindings: []map[string]Binding{
					{
						"s": {Type: "uri", Value: "http://example.com/subject", DisplayText: ""},
						"p": {Type: "uri", Value: "http://example.com/predicate", DisplayText: ""},
						"o": {Type: "literal", Value: "literal value", DisplayText: ""},
					},
					{
						"s": {Type: "uri", Value: "http://example.com/subject2", DisplayText: ""},
						"p": {Type: "uri", Value: "http://example.com/predicate", DisplayText: ""}, // duplicate
						"o": {Type: "literal", Value: "another literal", DisplayText: ""},
					},
				},
			},
			want: []string{
				"http://example.com/subject",
				"http://example.com/predicate",
				"http://example.com/subject2",
			},
		},
		{
			name: "empty result",
			result: QueryResult{
				Vars:     []string{},
				Bindings: []map[string]Binding{},
			},
			want: []string{},
		},
		{
			name: "only literals",
			result: QueryResult{
				Vars: []string{"o"},
				Bindings: []map[string]Binding{
					{
						"o": {Type: "literal", Value: "literal1", DisplayText: ""},
					},
					{
						"o": {Type: "literal", Value: "literal2", DisplayText: ""},
					},
				},
			},
			want: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractIRIs(tt.result)

			// Sort both slices for comparison since order doesn't matter
			if len(got) != len(tt.want) {
				t.Errorf("extractIRIs() length = %v, want %v", len(got), len(tt.want))
				return
			}

			// Check all wanted IRIs are present
			gotMap := make(map[string]bool)
			for _, iri := range got {
				gotMap[iri] = true
			}

			for _, wantIRI := range tt.want {
				if !gotMap[wantIRI] {
					t.Errorf("extractIRIs() missing IRI: %v", wantIRI)
				}
			}
		})
	}
}

func TestBareCodeParse(t *testing.T) {
	if got := parseAcceptLanguage("fr"); len(got) != 1 || got[0] != "fr" {
		t.Errorf("parseAcceptLanguage(\"fr\") = %v, want [fr]", got)
	}
}

// rankLanguages: requested first (as base codes), then the site languages,
// no repeats; "" (untagged) keeps its configured place; unsafe codes dropped.
func TestRankLanguages(t *testing.T) {
	p := New(QueryInput{LabelLanguages: []string{"de", "fr", "it", "en", "rm", ""}})
	got := strings.Join(p.rankLanguages([]string{"en-US", "x'y", "de"}), ",")
	if want := "en,de,fr,it,rm,"; got != want {
		t.Errorf("rankLanguages = %q, want %q", got, want)
	}
}

// bestProperties: the "label" magic property's alternatives in written order,
// a built-in default without one, and multi-step paths skipped.
func TestBestProperties(t *testing.T) {
	p := New(QueryInput{MagicProperties: map[string]string{
		"label":       "schema:name | rdfs:label|<http://ex.org/t>",
		"description": "rdfs:comment|schema:address/schema:name",
	}})
	if got := strings.Join(p.bestProperties("label"), ","); got != "schema:name,rdfs:label,<http://ex.org/t>" {
		t.Errorf("bestProperties(label) = %q", got)
	}
	if got := strings.Join(p.bestProperties("description"), ","); got != "rdfs:comment" {
		t.Errorf("bestProperties(description) = %q, want the path dropped", got)
	}
	if got := New(QueryInput{}).bestProperties("label"); strings.Join(got, "|") != DefaultLabelPath {
		t.Errorf("bestProperties without config = %v, want the default path", got)
	}
}

// buildBestQuery ranks language before property: the language rank is the
// first two characters of the concatenation, the property rank the next two.
func TestBuildBestQuery(t *testing.T) {
	if buildBestQuery(nil, []string{"rdfs:label"}, []string{"en"}) != "" {
		t.Error("no IRIs should give an empty query")
	}
	q := buildBestQuery([]string{"http://ex.org/a"}, []string{"rdfs:label", "schema:name"}, []string{"en", ""})
	for _, want := range []string{
		"VALUES ?iri { <http://ex.org/a> }",
		`(rdfs:label "00")`,
		`(schema:name "01")`,
		`IF(?lang = "en" || STRSTARTS(?lang, "en-"), "00", 1/0)`,
		`IF(?lang = "", "01", 1/0)`,
		`"99"`,
		"CONCAT(?lRank, ?pRank, STR(?val))",
		"SUBSTR(?best, 5)",
	} {
		if !strings.Contains(q, want) {
			t.Errorf("buildBestQuery missing %q:\n%s", want, q)
		}
	}
}

func TestEnrichWithLabels(t *testing.T) {
	result := QueryResult{
		Vars: []string{"s", "p", "o"},
		Bindings: []map[string]Binding{
			{
				"s": {Type: "uri", Value: "http://example.com/subject", DisplayText: "http://example.com/subject"},
				"p": {Type: "uri", Value: "http://schema.org/name", DisplayText: "http://schema.org/name"},
				"o": {Type: "literal", Value: "John Doe", DisplayText: "John Doe"},
			},
		},
	}

	labelMap := map[string]string{
		"http://example.com/subject": "Subject Label",
		"http://schema.org/name":     "name",
	}

	enrichWithLabels(&result, labelMap)

	// Check URI bindings got labels
	if result.Bindings[0]["s"].DisplayText != "Subject Label" {
		t.Errorf("enrichWithLabels() subject Lol = %v, want %v", result.Bindings[0]["s"].DisplayText, "Subject Label")
	}

	if result.Bindings[0]["p"].DisplayText != "name" {
		t.Errorf("enrichWithLabels() predicate Lol = %v, want %v", result.Bindings[0]["p"].DisplayText, "name")
	}

	// Check literal binding unchanged
	if result.Bindings[0]["o"].DisplayText != "John Doe" {
		t.Errorf("enrichWithLabels() literal Lol should remain unchanged, got %v", result.Bindings[0]["o"].DisplayText)
	}
}

// Helper function
func containsString(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && (s == substr || len(s) >= len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || containsInMiddle(s, substr)))
}

func containsInMiddle(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
