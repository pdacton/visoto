package parser

import (
	"strings"
	"testing"
)

const twoRoleBlock = `
<div class="card">
  <sparql-tree-queries for="conceptTree">
    <sparql-tree-query role="roots">
      SELECT ?node ?label WHERE { ?node skos:topConceptOf ?? }
    </sparql-tree-query>
    <sparql-tree-query role="children">
      SELECT ?node ?label WHERE { ?node skos:broader ?parent }
    </sparql-tree-query>
  </sparql-tree-queries>
</div>`

func TestExtractTreeQueriesReadsEachRoleSeparately(t *testing.T) {
	blocks, err := ExtractTreeQueries(twoRoleBlock)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(blocks) != 1 {
		t.Fatalf("got %d blocks, want 1", len(blocks))
	}
	b := blocks[0]
	if b.ID != "conceptTree" {
		t.Errorf("ID = %q, want conceptTree", b.ID)
	}
	if len(b.Roles) != 2 {
		t.Fatalf("got %d roles, want 2: %v", len(b.Roles), b.Roles)
	}
	// The whole reason this does not go through extractElements: that path would
	// concatenate both queries into one blob.
	if !strings.Contains(b.Roles["roots"], "skos:topConceptOf") {
		t.Errorf("roots query wrong: %q", b.Roles["roots"])
	}
	if strings.Contains(b.Roles["roots"], "skos:broader") {
		t.Errorf("roots query leaked the children query: %q", b.Roles["roots"])
	}
	if !strings.Contains(b.Roles["children"], "skos:broader") {
		t.Errorf("children query wrong: %q", b.Roles["children"])
	}
}

func TestExtractTreeQueriesPreservesQueryText(t *testing.T) {
	blocks, err := ExtractTreeQueries(`
	<sparql-tree-queries for="t">
	  <sparql-tree-query role="roots">SELECT ?node WHERE { ?node a ?? . FILTER(?x > 3 && ?y < 5) }</sparql-tree-query>
	</sparql-tree-queries>`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// SPARQL comparison operators must survive the HTML parser intact.
	got := blocks[0].Roles["roots"]
	if !strings.Contains(got, "?x > 3 && ?y < 5") {
		t.Errorf("query text mangled: %q", got)
	}
}

func TestExtractTreeQueriesMultipleBlocks(t *testing.T) {
	blocks, err := ExtractTreeQueries(twoRoleBlock + `
	<sparql-tree-queries for="otherTree">
	  <sparql-tree-query role="roots">SELECT ?node WHERE { ?node a owl:Class }</sparql-tree-query>
	  <sparql-tree-query role="children">SELECT ?node WHERE { ?node rdfs:subClassOf ?parent }</sparql-tree-query>
	</sparql-tree-queries>`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(blocks) != 2 {
		t.Fatalf("got %d blocks, want 2", len(blocks))
	}
	ids := map[string]bool{blocks[0].ID: true, blocks[1].ID: true}
	if !ids["conceptTree"] || !ids["otherTree"] {
		t.Errorf("unexpected ids: %v", ids)
	}
}

func TestExtractTreeQueriesAllRoles(t *testing.T) {
	blocks, err := ExtractTreeQueries(`
	<sparql-tree-queries for="t">
	  <sparql-tree-query role="roots">SELECT ?node WHERE { ?node a ?? }</sparql-tree-query>
	  <sparql-tree-query role="children">SELECT ?node WHERE { ?node skos:broader ?parent }</sparql-tree-query>
	  <sparql-tree-query role="parents">SELECT ?node ?parent WHERE { ?node skos:broader ?parent }</sparql-tree-query>
	  <sparql-tree-query role="search">SELECT ?node WHERE { ?node rdfs:label ?l FILTER(CONTAINS(?l, ?__token__)) }</sparql-tree-query>
	</sparql-tree-queries>`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(blocks[0].Roles) != 4 {
		t.Fatalf("got %d roles, want 4", len(blocks[0].Roles))
	}
}

// Documentation and Go template comments name the elements. A {{/* … */}} comment
// is NOT an HTML comment, so it is not skipped by the parser and arrives here as
// ordinary text — it must not be mistaken for a declaration.
func TestExtractTreeQueriesIgnoresProse(t *testing.T) {
	for _, src := range []string{
		`<p>Declare queries with <sparql-tree-queries> and <sparql-tree-query>.</p>`,
		`{{/* <sparql-tree-queries> holds one <sparql-tree-query> per role */}}`,
		`<!-- a <sparql-tree-queries> block declares the roles -->`,
	} {
		blocks, err := ExtractTreeQueries(src)
		if err != nil {
			t.Errorf("prose rejected: %v (src %q)", err, src)
		}
		if len(blocks) != 0 {
			t.Errorf("prose produced %d blocks: %q", len(blocks), src)
		}
	}
}

// A container with no for= names no tree, so nothing could ever read it. It is
// treated as prose rather than an error: that is the only signal that separates a
// mention of the tags from a declaration, since prose parses as a real container
// with real children (see parseTreeQueriesNode).
func TestExtractTreeQueriesIgnoresContainerWithoutFor(t *testing.T) {
	blocks, err := ExtractTreeQueries(
		`<sparql-tree-queries><sparql-tree-query role="roots">SELECT ?node WHERE {}</sparql-tree-query></sparql-tree-queries>`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(blocks) != 0 {
		t.Errorf("got %d blocks, want 0", len(blocks))
	}
}

func TestExtractTreeQueriesRejectsMalformed(t *testing.T) {
	cases := []struct {
		name, src, want string
	}{
		{
			name: "role child with no role attribute",
			src:  `<sparql-tree-queries for="t"><sparql-tree-query>SELECT ?node WHERE {}</sparql-tree-query></sparql-tree-queries>`,
			want: "has no role",
		},
		{
			name: "container with no roles",
			src:  `<sparql-tree-queries for="t"></sparql-tree-queries>`,
			want: "declares no",
		},
		{
			name: "empty role query",
			src:  `<sparql-tree-queries for="t"><sparql-tree-query role="roots">   </sparql-tree-query></sparql-tree-queries>`,
			want: "is empty",
		},
		{
			name: "duplicate role",
			src: `<sparql-tree-queries for="t">
			        <sparql-tree-query role="roots">SELECT ?node WHERE { ?node a ?? }</sparql-tree-query>
			        <sparql-tree-query role="roots">SELECT ?node WHERE { ?node a owl:Thing }</sparql-tree-query>
			      </sparql-tree-queries>`,
			want: "duplicate role",
		},
		{
			name: "orphan role query",
			src:  `<div><sparql-tree-query role="roots">SELECT ?node WHERE { ?node a ?? }</sparql-tree-query></div>`,
			want: "not inside",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ExtractTreeQueries(tc.src)
			if err == nil {
				t.Fatalf("expected an error mentioning %q", tc.want)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q does not mention %q", err, tc.want)
			}
		})
	}
}

// The lazy tree's elements must stay invisible to the existing extractors, and
// theirs to this one: the two declaration systems share a template file.
func TestExtractTreeQueriesDoesNotDisturbOtherExtractors(t *testing.T) {
	src := twoRoleBlock + `
	<sparql-async id="instances">SELECT ?s WHERE { ?s a ?? }</sparql-async>
	<sparql-columns for="instances"><sparql-column var="s" label="Thing"></sparql-column></sparql-columns>`

	async, err := ExtractAsyncElements(src)
	if err != nil {
		t.Fatalf("async extraction: %v", err)
	}
	if len(async) != 1 || async[0].ID != "instances" {
		t.Errorf("async elements = %+v, want the one instances query", async)
	}
	// The role queries must not have leaked into the async namespace.
	if strings.Contains(async[0].Content, "topConceptOf") {
		t.Errorf("tree query leaked into <sparql-async>: %q", async[0].Content)
	}

	blocks, err := ExtractTreeQueries(src)
	if err != nil {
		t.Fatalf("tree extraction: %v", err)
	}
	if len(blocks) != 1 {
		t.Fatalf("got %d tree blocks, want 1", len(blocks))
	}
}
