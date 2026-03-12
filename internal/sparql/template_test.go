package sparql

import (
	"fmt"
	"testing"
)

func TestExtractQueries(t *testing.T) {
	tests := []struct {
		name     string
		template string
		want     []extractedQuery
		wantErr  bool
		errMsg   string
	}{
		{
			name: "single query without resolve-labels attribute",
			template: `
<sparql-query id="basic">
  SELECT ?s ?p ?o WHERE {
    ?s ?p ?o
  } LIMIT 10
</sparql-query>
`,
			want: []extractedQuery{
				{
					ID:            "basic",
					Query:         "SELECT ?s ?p ?o WHERE {\n    ?s ?p ?o\n  } LIMIT 10",
					ResolveLabels: true, // default
				},
			},
			wantErr: false,
		},
		{
			name: "single query with resolve-labels=true",
			template: `
<sparql-query id="enriched" resolve-labels="true">
  SELECT ?s ?p ?o WHERE {
    ?s ?p ?o
  }
</sparql-query>
`,
			want: []extractedQuery{
				{
					ID:            "enriched",
					Query:         "SELECT ?s ?p ?o WHERE {\n    ?s ?p ?o\n  }",
					ResolveLabels: true,
				},
			},
			wantErr: false,
		},
		{
			name: "single query with resolve-labels=false",
			template: `
<sparql-query id="basic" resolve-labels="false">
  SELECT ?s ?p ?o WHERE {
    ?s ?p ?o
  }
</sparql-query>
`,
			want: []extractedQuery{
				{
					ID:            "basic",
					Query:         "SELECT ?s ?p ?o WHERE {\n    ?s ?p ?o\n  }",
					ResolveLabels: false,
				},
			},
			wantErr: false,
		},
		{
			name: "multiple queries with mixed resolve-labels",
			template: `
<html>
<sparql-query id="first">
  SELECT * WHERE { ?s ?p ?o } LIMIT 5
</sparql-query>

<div>Some content</div>

<sparql-query id="second" resolve-labels="false">
  SELECT ?x WHERE { ?x a ?type }
</sparql-query>

<sparql-query id="third" resolve-labels="true">
  SELECT ?label WHERE { ?s rdfs:label ?label }
</sparql-query>
</html>
`,
			want: []extractedQuery{
				{
					ID:            "first",
					Query:         "SELECT * WHERE { ?s ?p ?o } LIMIT 5",
					ResolveLabels: true, // default
				},
				{
					ID:            "second",
					Query:         "SELECT ?x WHERE { ?x a ?type }",
					ResolveLabels: false,
				},
				{
					ID:            "third",
					Query:         "SELECT ?label WHERE { ?s rdfs:label ?label }",
					ResolveLabels: true,
				},
			},
			wantErr: false,
		},
		{
			name:     "whitespace-only query ID",
			template: `<sparql-query id="  ">SELECT * WHERE { ?s ?p ?o }</sparql-query>`,
			want:     nil,
			wantErr:  true,
			errMsg:   "empty query ID found",
		},
		{
			name:     "empty query content",
			template: `<sparql-query id="test"></sparql-query>`,
			want:     nil,
			wantErr:  true,
			errMsg:   "empty query content for ID: test",
		},
		{
			name: "duplicate query ID",
			template: `
<sparql-query id="duplicate">
  SELECT ?s WHERE { ?s ?p ?o }
</sparql-query>
<sparql-query id="duplicate">
  SELECT ?x WHERE { ?x ?y ?z }
</sparql-query>
`,
			want:    nil,
			wantErr: true,
			errMsg:  "duplicate query ID: duplicate",
		},
		{
			name:     "no queries in template",
			template: `<html><body>No SPARQL queries here</body></html>`,
			want:     []extractedQuery{},
			wantErr:  false,
		},
		{
			name: "query with extra whitespace",
			template: `
<sparql-query   id="whitespace"   resolve-labels="true"  >

  SELECT ?s ?p ?o
  WHERE {
    ?s ?p ?o
  }

</sparql-query>
`,
			want: []extractedQuery{
				{
					ID:            "whitespace",
					Query:         "SELECT ?s ?p ?o\n  WHERE {\n    ?s ?p ?o\n  }",
					ResolveLabels: true,
				},
			},
			wantErr: false,
		},
		{
			name: "query with resolve-labels and whitespace",
			template: `
<sparql-query id="styled" resolve-labels="true">
  SELECT ?s WHERE { ?s a ?type }
</sparql-query>
`,
			want: []extractedQuery{
				{
					ID:            "styled",
					Query:         "SELECT ?s WHERE { ?s a ?type }",
					ResolveLabels: true,
				},
			},
			wantErr: false,
		},
		{
			name: "query with resolve-labels false",
			template: `
<sparql-query id="multi" resolve-labels="false">
  SELECT * WHERE { ?s ?p ?o }
</sparql-query>
`,
			want: []extractedQuery{
				{
					ID:            "multi",
					Query:         "SELECT * WHERE { ?s ?p ?o }",
					ResolveLabels: false,
				},
			},
			wantErr: false,
		},
		{
			name: "query with placeholder",
			template: `
<sparql-query id="placeholder" resolve-labels="true">
  SELECT ?s ?p ?o WHERE {
    BIND (?? AS ?s)
    ?s ?p ?o
  }
</sparql-query>
`,
			want: []extractedQuery{
				{
					ID:            "placeholder",
					Query:         "SELECT ?s ?p ?o WHERE {\n    BIND (?? AS ?s)\n    ?s ?p ?o\n  }",
					ResolveLabels: true,
				},
			},
			wantErr: false,
		},
		{
			name: "query with prefixes",
			template: `
<sparql-query id="prefixes">
PREFIX rdfs: <http://www.w3.org/2000/01/rdf-schema#>
PREFIX skos: <http://www.w3.org/2004/02/skos/core#>

SELECT ?label WHERE {
  ?s rdfs:label ?label .
}
</sparql-query>
`,
			want: []extractedQuery{
				{
					ID:            "prefixes",
					Query:         "PREFIX rdfs: <http://www.w3.org/2000/01/rdf-schema#>\nPREFIX skos: <http://www.w3.org/2004/02/skos/core#>\n\nSELECT ?label WHERE {\n  ?s rdfs:label ?label .\n}",
					ResolveLabels: true,
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := extractQueries(tt.template)

			// Check error expectations
			if tt.wantErr {
				if err == nil {
					t.Errorf("extractQueries() expected error but got none")
					return
				}
				if tt.errMsg != "" && err.Error() != tt.errMsg {
					t.Errorf("extractQueries() error = %v, want %v", err.Error(), tt.errMsg)
				}
				return
			}

			if err != nil {
				t.Errorf("extractQueries() unexpected error = %v", err)
				return
			}

			// Check results
			if len(got) != len(tt.want) {
				t.Errorf("extractQueries() returned %d queries, want %d", len(got), len(tt.want))
				return
			}

			for i := range got {
				if got[i].ID != tt.want[i].ID {
					t.Errorf("extractQueries()[%d].ID = %v, want %v", i, got[i].ID, tt.want[i].ID)
				}
				if got[i].Query != tt.want[i].Query {
					t.Errorf("extractQueries()[%d].Query = %v, want %v", i, got[i].Query, tt.want[i].Query)
				}
				if got[i].ResolveLabels != tt.want[i].ResolveLabels {
					t.Errorf("extractQueries()[%d].ResolveLabels = %v, want %v", i, got[i].ResolveLabels, tt.want[i].ResolveLabels)
				}
			}
		})
	}
}

func TestExtractQueries_RegexEdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		template string
		wantLen  int
	}{
		{
			name: "nested angle brackets in query",
			template: `
<sparql-query id="brackets">
  SELECT ?s WHERE {
    ?s <http://example.com/predicate> <http://example.com/object>
  }
</sparql-query>
`,
			wantLen: 1,
		},
		{
			name: "multiline query with various formatting",
			template: `
<sparql-query id="multiline">
SELECT ?subject ?predicate ?object
WHERE {
  ?subject ?predicate ?object .
  FILTER (lang(?object) = "en")
}
ORDER BY ?subject
LIMIT 100
</sparql-query>
`,
			wantLen: 1,
		},
		{
			name: "query with comments",
			template: `
<sparql-query id="comments">
# This is a comment
SELECT ?s ?p ?o
WHERE {
  ?s ?p ?o  # inline comment
}
</sparql-query>
`,
			wantLen: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := extractQueries(tt.template)
			if err != nil {
				t.Errorf("extractQueries() unexpected error = %v", err)
				return
			}
			if len(got) != tt.wantLen {
				t.Errorf("extractQueries() returned %d queries, want %d", len(got), tt.wantLen)
			}
		})
	}
}

func TestExtractQueries_ResolveLabelsDefault(t *testing.T) {
	// Test that omitting the attribute defaults to true
	template := `<sparql-query id="test">SELECT * WHERE { ?s ?p ?o }</sparql-query>`

	got, err := extractQueries(template)
	if err != nil {
		t.Fatalf("extractQueries() error = %v", err)
	}

	if len(got) != 1 {
		t.Fatalf("extractQueries() returned %d queries, want 1", len(got))
	}

	if !got[0].ResolveLabels {
		t.Error("extractQueries() ResolveLabels should default to true when attribute is omitted")
	}
}

func TestExtractQueries_Integration(t *testing.T) {
	// Test with a realistic template similar to resource.html
	// Note: Current regex only supports id and resolve-labels attributes, not style/class
	template := `
<!DOCTYPE html>
<html>
<head>
  <title>Resource View</title>
</head>
<body>
  <h1>Resource Information</h1>

  <sparql-query id="resource">
    SELECT ?s ?p ?o WHERE {
      BIND (?? AS ?s)
      ?s ?p ?o
    } LIMIT 100
  </sparql-query>

  <sparql-query id="types" resolve-labels="true">
    SELECT DISTINCT ?type WHERE {
      BIND (?? AS ?s)
      ?s a ?type
    }
  </sparql-query>

  <sparql-query id="raw" resolve-labels="false">
    SELECT ?uri WHERE {
      ?uri ?p ??
    } LIMIT 10
  </sparql-query>

  <table>
    {{ range .QueryResults.resource.Bindings }}
    <tr>
      <td>{{ .s.DisplayText }}</td>
      <td>{{ .p.DisplayText }}</td>
      <td>{{ .o.DisplayText }}</td>
    </tr>
    {{ end }}
  </table>
</body>
</html>
`

	got, err := extractQueries(template)
	if err != nil {
		t.Fatalf("extractQueries() error = %v", err)
	}

	if len(got) != 3 {
		t.Fatalf("extractQueries() returned %d queries, want 3", len(got))
	}

	// Check IDs
	expectedIDs := []string{"resource", "types", "raw"}
	for i, id := range expectedIDs {
		if got[i].ID != id {
			t.Errorf("extractQueries()[%d].ID = %v, want %v", i, got[i].ID, id)
		}
	}

	// Check ResolveLabels flags
	expectedResolveLabels := []bool{true, true, false}
	for i, expected := range expectedResolveLabels {
		if got[i].ResolveLabels != expected {
			t.Errorf("extractQueries()[%d].ResolveLabels = %v, want %v", i, got[i].ResolveLabels, expected)
		}
	}

	// Check that placeholder is preserved
	if !containsSubstring(got[0].Query, "??") {
		t.Error("extractQueries() should preserve ?? placeholder in query")
	}
}

// Helper function
func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) &&
		(s == substr ||
			(len(s) > len(substr) && indexOfSubstring(s, substr) >= 0))
}

func indexOfSubstring(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

func TestExtractElements(t *testing.T) {
	tests := []struct {
		name     string
		template string
		wantLen  int
		wantErr  bool
	}{
		{
			name: "single sparql-query with multiple attributes",
			template: `
<sparql-query id="test" resolve-labels="true" style="display: none;" class="query">
  SELECT * WHERE { ?s ?p ?o }
</sparql-query>
`,
			wantLen: 1,
			wantErr: false,
		},
		{
			name: "sparql-query with style attribute that regex misses",
			template: `
<sparql-query id="resource" style="display: none;">
  SELECT ?s ?p ?o WHERE {
    BIND (?? AS ?s)
    ?s ?p ?o
  } LIMIT 10
</sparql-query>
`,
			wantLen: 1,
			wantErr: false,
		},
		{
			name: "multiple different SPARQL elements",
			template: `
<sparql-query id="q1">SELECT * WHERE { ?s ?p ?o }</sparql-query>
<sparql-table id="t1">SELECT * WHERE { ?s ?p ?o }</sparql-table>
<sparql-tree id="tr1">SELECT * WHERE { ?s ?p ?o }</sparql-tree>
`,
			wantLen: 3,
			wantErr: false,
		},
		{
			name: "element without id attribute",
			template: `
<sparql-query>SELECT * WHERE { ?s ?p ?o }</sparql-query>
`,
			wantLen: 0, // Element with error is not added
			wantErr: false,
		},
		{
			name: "template with Go template syntax",
			template: `
<div>{{ .SomeVariable }}</div>
<sparql-query id="test">
  SELECT ?s WHERE { ?s ?p ?o }
</sparql-query>
{{ range .Items }}
  <p>{{ . }}</p>
{{ end }}
`,
			wantLen: 1,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := extractElements(tt.template)

			if tt.wantErr {
				if err == nil {
					t.Error("extractElements() expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("extractElements() unexpected error = %v", err)
				return
			}

			if len(got) != tt.wantLen {
				t.Errorf("extractElements() returned %d elements, want %d", len(got), tt.wantLen)
			}
		})
	}
}

func TestExtractElements_Attributes(t *testing.T) {
	template := `
<sparql-query id="test" resolve-labels="true" style="display: none;" class="query" data-custom="value">
  SELECT * WHERE { ?s ?p ?o }
</sparql-query>
`

	elements, err := extractElements(template)
	if err != nil {
		t.Fatalf("extractElements() error = %v", err)
	}

	if len(elements) != 1 {
		t.Fatalf("extractElements() returned %d elements, want 1", len(elements))
	}

	elem := elements[0]

	// Check TagName
	if elem.TagName != "sparql-query" {
		t.Errorf("TagName = %v, want sparql-query", elem.TagName)
	}

	// Check ID
	if elem.ID != "test" {
		t.Errorf("ID = %v, want test", elem.ID)
	}

	// Check all attributes are captured
	expectedAttrs := map[string]string{
		"id":             "test",
		"resolve-labels": "true",
		"style":          "display: none;",
		"class":          "query",
		"data-custom":    "value",
	}

	for key, expectedVal := range expectedAttrs {
		if gotVal, exists := elem.Attributes[key]; !exists {
			t.Errorf("Attribute %s not found", key)
		} else if gotVal != expectedVal {
			t.Errorf("Attribute %s = %v, want %v", key, gotVal, expectedVal)
		}
	}

	// Check content
	expectedContent := "SELECT * WHERE { ?s ?p ?o }"
	if elem.Content != expectedContent {
		t.Errorf("Content = %v, want %v", elem.Content, expectedContent)
	}
}

func TestExtractQueriesDOM(t *testing.T) {
	tests := []struct {
		name     string
		template string
		want     []extractedQuery
		wantErr  bool
	}{
		{
			name: "single query with style attribute",
			template: `
<sparql-query id="test" style="display: none;">
  SELECT * WHERE { ?s ?p ?o }
</sparql-query>
`,
			want: []extractedQuery{
				{
					ID:            "test",
					Query:         "SELECT * WHERE { ?s ?p ?o }",
					ResolveLabels: true, // default
				},
			},
			wantErr: false,
		},
		{
			name: "query with resolve-labels=false",
			template: `
<sparql-query id="test" resolve-labels="false" class="hidden">
  SELECT * WHERE { ?s ?p ?o }
</sparql-query>
`,
			want: []extractedQuery{
				{
					ID:            "test",
					Query:         "SELECT * WHERE { ?s ?p ?o }",
					ResolveLabels: false,
				},
			},
			wantErr: false,
		},
		{
			name: "duplicate query IDs",
			template: `
<sparql-query id="dup">SELECT ?s WHERE { ?s ?p ?o }</sparql-query>
<sparql-query id="dup">SELECT ?x WHERE { ?x ?y ?z }</sparql-query>
`,
			want:    nil,
			wantErr: true,
		},
		{
			name: "empty query content",
			template: `
<sparql-query id="empty"></sparql-query>
`,
			want:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := extractQueriesDOM(tt.template)

			if tt.wantErr {
				if err == nil {
					t.Error("extractQueriesDOM() expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("extractQueriesDOM() unexpected error = %v", err)
				return
			}

			if len(got) != len(tt.want) {
				t.Errorf("extractQueriesDOM() returned %d queries, want %d", len(got), len(tt.want))
				return
			}

			for i := range got {
				if got[i].ID != tt.want[i].ID {
					t.Errorf("Query[%d].ID = %v, want %v", i, got[i].ID, tt.want[i].ID)
				}
				if got[i].Query != tt.want[i].Query {
					t.Errorf("Query[%d].Query = %v, want %v", i, got[i].Query, tt.want[i].Query)
				}
				if got[i].ResolveLabels != tt.want[i].ResolveLabels {
					t.Errorf("Query[%d].ResolveLabels = %v, want %v", i, got[i].ResolveLabels, tt.want[i].ResolveLabels)
				}
			}
		})
	}
}

func TestExtractQueriesDOM_CompareWithRegex(t *testing.T) {
	// Test that DOM parser produces same results as regex for simple cases
	tests := []string{
		`<sparql-query id="test">SELECT * WHERE { ?s ?p ?o }</sparql-query>`,
		`<sparql-query id="test" resolve-labels="true">SELECT * WHERE { ?s ?p ?o }</sparql-query>`,
		`<sparql-query id="test" resolve-labels="false">SELECT * WHERE { ?s ?p ?o }</sparql-query>`,
		`
<sparql-query id="first">SELECT ?s WHERE { ?s ?p ?o }</sparql-query>
<sparql-query id="second" resolve-labels="false">SELECT ?x WHERE { ?x ?y ?z }</sparql-query>
`,
	}

	for i, template := range tests {
		t.Run(fmt.Sprintf("case_%d", i), func(t *testing.T) {
			gotDOM, errDOM := extractQueriesDOM(template)
			gotRegex, errRegex := extractQueries(template)

			// Both should succeed or fail together
			if (errDOM != nil) != (errRegex != nil) {
				t.Errorf("Error mismatch: DOM err=%v, Regex err=%v", errDOM, errRegex)
				return
			}

			if errDOM != nil {
				return // Both failed, that's fine
			}

			// Check same number of results
			if len(gotDOM) != len(gotRegex) {
				t.Errorf("Length mismatch: DOM=%d, Regex=%d", len(gotDOM), len(gotRegex))
				return
			}

			// Check each query matches
			for j := range gotDOM {
				if gotDOM[j].ID != gotRegex[j].ID {
					t.Errorf("Query[%d].ID: DOM=%v, Regex=%v", j, gotDOM[j].ID, gotRegex[j].ID)
				}
				if gotDOM[j].Query != gotRegex[j].Query {
					t.Errorf("Query[%d].Query: DOM=%v, Regex=%v", j, gotDOM[j].Query, gotRegex[j].Query)
				}
				if gotDOM[j].ResolveLabels != gotRegex[j].ResolveLabels {
					t.Errorf("Query[%d].ResolveLabels: DOM=%v, Regex=%v", j, gotDOM[j].ResolveLabels, gotRegex[j].ResolveLabels)
				}
			}
		})
	}
}

func TestExtractTextContent(t *testing.T) {
	// This would require creating html.Node structures manually
	// For now, we test it indirectly through extractElements
	template := `
<sparql-query id="test">
  SELECT ?s ?p ?o
  WHERE {
    ?s ?p ?o
  }
  LIMIT 10
</sparql-query>
`

	elements, err := extractElements(template)
	if err != nil {
		t.Fatalf("extractElements() error = %v", err)
	}

	if len(elements) != 1 {
		t.Fatalf("extractElements() returned %d elements, want 1", len(elements))
	}

	// Content should have whitespace normalized
	content := elements[0].Content
	if !containsSubstring(content, "SELECT") {
		t.Errorf("Content missing SELECT keyword: %v", content)
	}
	if !containsSubstring(content, "WHERE") {
		t.Errorf("Content missing WHERE keyword: %v", content)
	}
	if !containsSubstring(content, "LIMIT 10") {
		t.Errorf("Content missing LIMIT clause: %v", content)
	}
}
