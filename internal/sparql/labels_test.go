package sparql

import (
	"reflect"
	"testing"
	"time"
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
			want:   []string{"en-US", "en", "en-GB", "en"}, // en appears twice: once from en-US, once explicit
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
				Bindings: []map[string]struct {
					Type  string
					Value string
					Lol   string
				}{
					{
						"s": {Type: "uri", Value: "http://example.com/subject", Lol: ""},
						"p": {Type: "uri", Value: "http://example.com/predicate", Lol: ""},
						"o": {Type: "literal", Value: "literal value", Lol: ""},
					},
					{
						"s": {Type: "uri", Value: "http://example.com/subject2", Lol: ""},
						"p": {Type: "uri", Value: "http://example.com/predicate", Lol: ""}, // duplicate
						"o": {Type: "literal", Value: "another literal", Lol: ""},
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
				Bindings: []map[string]struct {
					Type  string
					Value string
					Lol   string
				}{},
			},
			want: []string{},
		},
		{
			name: "only literals",
			result: QueryResult{
				Vars: []string{"o"},
				Bindings: []map[string]struct {
					Type  string
					Value string
					Lol   string
				}{
					{
						"o": {Type: "literal", Value: "literal1", Lol: ""},
					},
					{
						"o": {Type: "literal", Value: "literal2", Lol: ""},
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

func TestBuildLabelQuery(t *testing.T) {
	tests := []struct {
		name      string
		iris      []string
		languages []string
		wantEmpty bool
	}{
		{
			name:      "empty IRIs",
			iris:      []string{},
			languages: []string{"en"},
			wantEmpty: true,
		},
		{
			name:      "single IRI with language",
			iris:      []string{"http://example.com/Person"},
			languages: []string{"en", "de"},
			wantEmpty: false,
		},
		{
			name:      "multiple IRIs no language",
			iris:      []string{"http://example.com/Person", "http://example.com/name"},
			languages: []string{},
			wantEmpty: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildLabelQuery(tt.iris, tt.languages)

			if tt.wantEmpty {
				if got != "" {
					t.Errorf("buildLabelQuery() should return empty string for empty IRIs")
				}
				return
			}

			// Check query contains expected elements
			if got == "" {
				t.Errorf("buildLabelQuery() returned empty string unexpectedly")
			}

			// Should contain VALUES clause
			if !containsString(got, "VALUES ?iri") {
				t.Errorf("buildLabelQuery() missing VALUES clause")
			}

			// Should contain all IRIs
			for _, iri := range tt.iris {
				if !containsString(got, "<"+iri+">") {
					t.Errorf("buildLabelQuery() missing IRI: %v", iri)
				}
			}

			// Should contain UNION for priority
			if !containsString(got, "UNION") {
				t.Errorf("buildLabelQuery() missing UNION for priority")
			}

			// Should contain language filter if languages provided
			if len(tt.languages) > 0 {
				if !containsString(got, "FILTER") {
					t.Errorf("buildLabelQuery() missing FILTER for languages")
				}
				for _, lang := range tt.languages {
					if !containsString(got, lang) {
						t.Errorf("buildLabelQuery() missing language: %v", lang)
					}
				}
			}
		})
	}
}

func TestLabelCache(t *testing.T) {
	testIRI := "http://example.com/test-cache"
	testLabel := "Test Label"

	// Clear any existing entry for test IRI
	labelCache.Delete(testIRI)

	// Test cache miss
	if _, found := getCachedLabel(testIRI); found {
		t.Error("getCachedLabel() should return false for uncached IRI")
	}

	// Test cache set and get
	setCachedLabel(testIRI, testLabel)

	label, found := getCachedLabel(testIRI)
	if !found {
		t.Error("getCachedLabel() should return true for cached IRI")
	}
	if label != testLabel {
		t.Errorf("getCachedLabel() = %v, want %v", label, testLabel)
	}

	// Test expiration by manually setting an expired entry
	expiredIRI := "http://example.com/expired"
	labelCache.Store(expiredIRI, labelCacheEntry{
		label:      "Expired",
		expiration: time.Now().Add(-1 * time.Hour),
	})

	if _, found := getCachedLabel(expiredIRI); found {
		t.Error("getCachedLabel() should return false for expired entry")
	}
}

func TestEnrichWithLabels(t *testing.T) {
	result := QueryResult{
		Vars: []string{"s", "p", "o"},
		Bindings: []map[string]struct {
			Type  string
			Value string
			Lol   string
		}{
			{
				"s": {Type: "uri", Value: "http://example.com/subject", Lol: "http://example.com/subject"},
				"p": {Type: "uri", Value: "http://schema.org/name", Lol: "http://schema.org/name"},
				"o": {Type: "literal", Value: "John Doe", Lol: "John Doe"},
			},
		},
	}

	labelMap := map[string]string{
		"http://example.com/subject": "Subject Label",
		"http://schema.org/name":     "name",
	}

	enrichWithLabels(&result, labelMap)

	// Check URI bindings got labels
	if result.Bindings[0]["s"].Lol != "Subject Label" {
		t.Errorf("enrichWithLabels() subject Lol = %v, want %v", result.Bindings[0]["s"].Lol, "Subject Label")
	}

	if result.Bindings[0]["p"].Lol != "name" {
		t.Errorf("enrichWithLabels() predicate Lol = %v, want %v", result.Bindings[0]["p"].Lol, "name")
	}

	// Check literal binding unchanged
	if result.Bindings[0]["o"].Lol != "John Doe" {
		t.Errorf("enrichWithLabels() literal Lol should remain unchanged, got %v", result.Bindings[0]["o"].Lol)
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
