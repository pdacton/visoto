package parser

import (
	"testing"

	"hutzli.org/visoto/internal/sparql"
)

func TestExtractQueriesDOM(t *testing.T) {
	tests := []struct {
		name     string
		template string
		want     []sparql.ExtractedQuery
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
			want: []sparql.ExtractedQuery{
				{
					ID:            "basic",
					Query:         "SELECT ?s ?p ?o WHERE {\n    ?s ?p ?o\n  } LIMIT 10",
					ResolveLabels: true,
				},
			},
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
			want: []sparql.ExtractedQuery{
				{
					ID:            "enriched",
					Query:         "SELECT ?s ?p ?o WHERE {\n    ?s ?p ?o\n  }",
					ResolveLabels: true,
				},
			},
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
			want: []sparql.ExtractedQuery{
				{
					ID:            "basic",
					Query:         "SELECT ?s ?p ?o WHERE {\n    ?s ?p ?o\n  }",
					ResolveLabels: false,
				},
			},
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
			want: []sparql.ExtractedQuery{
				{ID: "first", Query: "SELECT * WHERE { ?s ?p ?o } LIMIT 5", ResolveLabels: true},
				{ID: "second", Query: "SELECT ?x WHERE { ?x a ?type }", ResolveLabels: false},
				{ID: "third", Query: "SELECT ?label WHERE { ?s rdfs:label ?label }", ResolveLabels: true},
			},
		},
		{
			name:     "whitespace-only query ID",
			template: `<sparql-query id="  ">SELECT * WHERE { ?s ?p ?o }</sparql-query>`,
			wantErr:  false, // DOM parser strips whitespace; element is silently skipped (no id → not added)
			want:     []sparql.ExtractedQuery{},
		},
		{
			name:    "empty query content",
			template: `<sparql-query id="test"></sparql-query>`,
			wantErr: true,
			errMsg:  "empty query content for ID: test",
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
			wantErr: true,
			errMsg:  "duplicate query ID: duplicate",
		},
		{
			name:    "no queries in template",
			template: `<html><body>No SPARQL queries here</body></html>`,
			want:    []sparql.ExtractedQuery{},
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
			want: []sparql.ExtractedQuery{
				{
					ID:            "whitespace",
					Query:         "SELECT ?s ?p ?o\n  WHERE {\n    ?s ?p ?o\n  }",
					ResolveLabels: true,
				},
			},
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
			want: []sparql.ExtractedQuery{
				{
					ID:            "placeholder",
					Query:         "SELECT ?s ?p ?o WHERE {\n    BIND (?? AS ?s)\n    ?s ?p ?o\n  }",
					ResolveLabels: true,
				},
			},
		},
		{
			name: "single query with style attribute",
			template: `
<sparql-query id="test" style="display: none;">
  SELECT * WHERE { ?s ?p ?o }
</sparql-query>
`,
			want: []sparql.ExtractedQuery{
				{ID: "test", Query: "SELECT * WHERE { ?s ?p ?o }", ResolveLabels: true},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := extractQueriesDOM(tt.template)

			if tt.wantErr {
				if err == nil {
					t.Errorf("extractQueriesDOM() expected error but got none")
					return
				}
				if tt.errMsg != "" && err.Error() != tt.errMsg {
					t.Errorf("extractQueriesDOM() error = %v, want %v", err.Error(), tt.errMsg)
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
		},
		{
			name: "multiple different SPARQL elements",
			template: `
<sparql-query id="q1">SELECT * WHERE { ?s ?p ?o }</sparql-query>
<sparql-table id="t1">SELECT * WHERE { ?s ?p ?o }</sparql-table>
<sparql-tree id="tr1">SELECT * WHERE { ?s ?p ?o }</sparql-tree>
`,
			wantLen: 3,
		},
		{
			name: "element without id attribute",
			template: `
<sparql-query>SELECT * WHERE { ?s ?p ?o }</sparql-query>
`,
			wantLen: 0,
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
	tmpl := `
<sparql-query id="test" resolve-labels="true" style="display: none;" class="query" data-custom="value">
  SELECT * WHERE { ?s ?p ?o }
</sparql-query>
`

	elements, err := extractElements(tmpl)
	if err != nil {
		t.Fatalf("extractElements() error = %v", err)
	}

	if len(elements) != 1 {
		t.Fatalf("extractElements() returned %d elements, want 1", len(elements))
	}

	elem := elements[0]

	if elem.TagName != "sparql-query" {
		t.Errorf("TagName = %v, want sparql-query", elem.TagName)
	}
	if elem.ID != "test" {
		t.Errorf("ID = %v, want test", elem.ID)
	}

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

	if elem.Content != "SELECT * WHERE { ?s ?p ?o }" {
		t.Errorf("Content = %v, want SELECT * WHERE { ?s ?p ?o }", elem.Content)
	}
}

func TestExtractTextContent(t *testing.T) {
	tmpl := `
<sparql-query id="test">
  SELECT ?s ?p ?o
  WHERE {
    ?s ?p ?o
  }
  LIMIT 10
</sparql-query>
`

	elements, err := extractElements(tmpl)
	if err != nil {
		t.Fatalf("extractElements() error = %v", err)
	}
	if len(elements) != 1 {
		t.Fatalf("extractElements() returned %d elements, want 1", len(elements))
	}

	content := elements[0].Content
	for _, want := range []string{"SELECT", "WHERE", "LIMIT 10"} {
		if !containsSubstring(content, want) {
			t.Errorf("Content missing %q: %v", want, content)
		}
	}
}

func TestExtractAsyncElements(t *testing.T) {
	tmpl := `
<sparql-query id="q1">SELECT * WHERE { ?s ?p ?o }</sparql-query>
<sparql-async id="a1">SELECT COUNT(*) AS ?count WHERE { ?s ?p ?o }</sparql-async>
<sparql-async id="a2">SELECT COUNT(*) AS ?count WHERE { ?s a ?t }</sparql-async>
`
	elements, err := ExtractAsyncElements(tmpl)
	if err != nil {
		t.Fatalf("ExtractAsyncElements() error = %v", err)
	}
	if len(elements) != 2 {
		t.Fatalf("ExtractAsyncElements() returned %d elements, want 2", len(elements))
	}
	if elements[0].ID != "a1" || elements[1].ID != "a2" {
		t.Errorf("unexpected IDs: %v, %v", elements[0].ID, elements[1].ID)
	}
}

// Helper functions
func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) &&
		(s == substr || indexOfSubstring(s, substr) >= 0)
}

func indexOfSubstring(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
