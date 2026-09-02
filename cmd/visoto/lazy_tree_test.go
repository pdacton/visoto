package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"hutzli.org/visoto/internal/parser"
	"hutzli.org/visoto/internal/sparql"
)

// --- Node shaping -----------------------------------------------------------

func bindingRow(pairs map[string][2]string) map[string]sparql.Binding {
	row := map[string]sparql.Binding{}
	for k, v := range pairs {
		row[k] = sparql.Binding{Value: v[0], DisplayText: v[1], Type: "uri"}
	}
	return row
}

func TestNodesFromUsesLabelAsTitle(t *testing.T) {
	result := sparql.QueryResult{
		Vars: []string{"node", "label"},
		Bindings: []map[string]sparql.Binding{
			bindingRow(map[string][2]string{"node": {"http://e.org/a", ""}, "label": {"Alpha", "Alpha"}}),
		},
	}
	nodes := nodesFrom(result, parser.TreeQueries{})
	if len(nodes) != 1 {
		t.Fatalf("got %d nodes, want 1", len(nodes))
	}
	if nodes[0].Key != "http://e.org/a" {
		t.Errorf("key = %q", nodes[0].Key)
	}
	if nodes[0].Title != "Alpha" {
		t.Errorf("title = %q, want Alpha", nodes[0].Title)
	}
}

func TestNodesFromFallsBackToIRI(t *testing.T) {
	result := sparql.QueryResult{
		Vars:     []string{"node"},
		Bindings: []map[string]sparql.Binding{bindingRow(map[string][2]string{"node": {"http://e.org/a", ""}})},
	}
	nodes := nodesFrom(result, parser.TreeQueries{})
	if nodes[0].Title != "http://e.org/a" {
		t.Errorf("title = %q, want the IRI", nodes[0].Title)
	}
}

// Optimistic expanders: with no ?hasChildren projected every node is lazy, and the
// client drops the expander when a level comes back empty.
func TestNodesFromOptimisticLazyByDefault(t *testing.T) {
	result := sparql.QueryResult{
		Vars:     []string{"node"},
		Bindings: []map[string]sparql.Binding{bindingRow(map[string][2]string{"node": {"http://e.org/a", "A"}})},
	}
	if !nodesFrom(result, parser.TreeQueries{})[0].Lazy {
		t.Error("node without ?hasChildren should be optimistically lazy")
	}
}

func TestNodesFromHonoursHasChildren(t *testing.T) {
	mk := func(v string) sparql.QueryResult {
		return sparql.QueryResult{
			Vars: []string{"node", "hasChildren"},
			Bindings: []map[string]sparql.Binding{{
				"node":        {Value: "http://e.org/a", DisplayText: "A", Type: "uri"},
				"hasChildren": {Value: v, Type: "literal"},
			}},
		}
	}
	for _, tc := range []struct {
		val  string
		want bool
	}{{"true", true}, {"false", false}, {"1", true}, {"0", false}} {
		if got := nodesFrom(mk(tc.val), parser.TreeQueries{})[0].Lazy; got != tc.want {
			t.Errorf("hasChildren=%q → lazy=%v, want %v", tc.val, got, tc.want)
		}
	}
}

// A node carrying two labels multiplies rows. It must render once.
func TestNodesFromDeduplicates(t *testing.T) {
	result := sparql.QueryResult{
		Vars: []string{"node", "label"},
		Bindings: []map[string]sparql.Binding{
			bindingRow(map[string][2]string{"node": {"http://e.org/a", ""}, "label": {"Alpha", "Alpha"}}),
			bindingRow(map[string][2]string{"node": {"http://e.org/a", ""}, "label": {"Alpha-DE", "Alpha-DE"}}),
			bindingRow(map[string][2]string{"node": {"http://e.org/b", ""}, "label": {"Beta", "Beta"}}),
		},
	}
	nodes := nodesFrom(result, parser.TreeQueries{})
	if len(nodes) != 2 {
		t.Fatalf("got %d nodes, want 2 (the duplicate should collapse)", len(nodes))
	}
}

// Extra variables must land as top-level keys, because Wunderbaum exposes
// non-reserved source properties as node.data[name] — what the render callback in
// sparql-tree.js reads.
func TestTreeNodeMarshalFlattensExtras(t *testing.T) {
	n := treeNode{
		Key: "http://e.org/a", Title: "A", Lazy: true,
		Extra: map[string]treeCell{"code": {Value: "01", Label: "01", Type: "literal"}},
	}
	raw, err := json.Marshal(n)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, nested := got["Extra"]; nested {
		t.Errorf("Extra was nested rather than flattened: %s", raw)
	}
	cell, ok := got["code"].(map[string]any)
	if !ok {
		t.Fatalf("code not a top-level object: %s", raw)
	}
	if cell["value"] != "01" {
		t.Errorf("code.value = %v", cell["value"])
	}
}

func TestNodesFromSkipsReservedVarsInExtras(t *testing.T) {
	result := sparql.QueryResult{
		Vars: []string{"node", "label", "parent", "hasChildren", "score", "code"},
		Bindings: []map[string]sparql.Binding{{
			"node":        {Value: "http://e.org/a", Type: "uri"},
			"label":       {Value: "A", DisplayText: "A"},
			"parent":      {Value: "http://e.org/root", Type: "uri"},
			"hasChildren": {Value: "true"},
			"score":       {Value: "0.9"},
			"code":        {Value: "01", DisplayText: "01"},
		}},
	}
	extras := nodesFrom(result, parser.TreeQueries{})[0].Extra
	for _, reserved := range []string{"node", "label", "parent", "hasChildren", "score"} {
		if _, present := extras[reserved]; present {
			t.Errorf("reserved var %q leaked into extras", reserved)
		}
	}
	if _, ok := extras["code"]; !ok {
		t.Error("extra var code missing")
	}
}

func TestParentPairs(t *testing.T) {
	result := sparql.QueryResult{
		Bindings: []map[string]sparql.Binding{
			{"node": {Value: "http://e.org/c"}, "parent": {Value: "http://e.org/b"}},
			{"node": {Value: "http://e.org/x"}}, // no parent: skipped
		},
	}
	pairs := parentPairs(result)
	if len(pairs) != 1 {
		t.Fatalf("got %d pairs, want 1", len(pairs))
	}
	if pairs[0]["parent"] != "http://e.org/b" {
		t.Errorf("pair = %v", pairs[0])
	}
}

// --- Routing and input validation -------------------------------------------

func lazyTreeTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/api/lazy-tree/:id/:role", lazyTreeHandler)
	return r
}

func doTreeRequest(t *testing.T, url string) (int, levelEnvelope) {
	t.Helper()
	w := httptest.NewRecorder()
	lazyTreeTestRouter().ServeHTTP(w, httptest.NewRequest(http.MethodGet, url, nil))
	var env levelEnvelope
	_ = json.Unmarshal(w.Body.Bytes(), &env)
	return w.Code, env
}

// An unknown id must 404 rather than reaching any query path.
func TestLazyTreeUnknownIDIs404(t *testing.T) {
	asyncIdx = &asyncIndex{}
	code, env := doTreeRequest(t, "/api/lazy-tree/nope/roots?src=pages/x.html")
	if code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", code)
	}
	if env.Error == "" {
		t.Error("no error message")
	}
}

func TestLazyTreeUnknownRoleIs404(t *testing.T) {
	asyncIdx = &asyncIndex{trees: map[string]map[string]parser.TreeQueries{
		"pages/x.html": {"t": {ID: "t", Roles: map[string]string{"roots": "SELECT ?node WHERE {}"}}},
	}}
	code, _ := doTreeRequest(t, "/api/lazy-tree/t/ancestors?src=pages/x.html")
	if code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", code)
	}
}

// Missing required inputs are rejected before a query is built, and never cached.
func TestLazyTreeRejectsMissingInputs(t *testing.T) {
	asyncIdx = &asyncIndex{trees: map[string]map[string]parser.TreeQueries{
		"pages/x.html": {"t": {ID: "t", Roles: map[string]string{
			"roots":    "SELECT ?node WHERE { ?node a ex:C }",
			"children": "SELECT ?node WHERE { ?node ex:broader ?parent }",
			"parents":  "SELECT ?node ?parent WHERE { ?node ex:broader ?parent }",
			"search":   "SELECT ?node WHERE { ?node rdfs:label ?l FILTER(CONTAINS(?l, ?__token__)) }",
		}}},
	}}
	cases := []struct{ name, url string }{
		{"children without parent", "/api/lazy-tree/t/children?src=pages/x.html"},
		{"parents without node", "/api/lazy-tree/t/parents?src=pages/x.html"},
		{"search without q", "/api/lazy-tree/t/search?src=pages/x.html"},
		{"search with blank q", "/api/lazy-tree/t/search?src=pages/x.html&q=%20%20"},
		{"focus without node", "/api/lazy-tree/t/focus?src=pages/x.html"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			lazyTreeTestRouter().ServeHTTP(w, httptest.NewRequest(http.MethodGet, tc.url, nil))
			if w.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400", w.Code)
			}
			if cc := w.Header().Get("Cache-Control"); cc != "no-store" {
				t.Errorf("Cache-Control = %q, want no-store — a 400 must not be cached", cc)
			}
		})
	}
}

// A hostile IRI must be rejected at the boundary, never escaped into a query.
func TestLazyTreeRejectsHostileIRIs(t *testing.T) {
	asyncIdx = &asyncIndex{trees: map[string]map[string]parser.TreeQueries{
		"pages/x.html": {"t": {ID: "t", Roles: map[string]string{
			"roots":    "SELECT ?node WHERE { ?node a ex:C }",
			"children": "SELECT ?node WHERE { ?node ex:broader ?parent }",
			"parents":  "SELECT ?node ?parent WHERE { ?node ex:broader ?parent }",
		}}},
	}}
	hostile := "http://e.org/x%3E+.+%3Fs+%3Fp+%3Fo+.+%23" // "x> . ?s ?p ?o . #"
	for _, url := range []string{
		"/api/lazy-tree/t/children?src=pages/x.html&parent=" + hostile,
		"/api/lazy-tree/t/focus?src=pages/x.html&node=" + hostile,
	} {
		w := httptest.NewRecorder()
		lazyTreeTestRouter().ServeHTTP(w, httptest.NewRequest(http.MethodGet, url, nil))
		if w.Code != http.StatusBadRequest {
			t.Errorf("hostile IRI accepted (status %d) at %s", w.Code, url)
		}
	}
}

// Roles the tree did not declare are refused with an explanation, not a 500.
func TestLazyTreeUndeclaredOptionalRoles(t *testing.T) {
	asyncIdx = &asyncIndex{trees: map[string]map[string]parser.TreeQueries{
		"pages/x.html": {"t": {ID: "t", Roles: map[string]string{
			"roots":    "SELECT ?node WHERE { ?node a ex:C }",
			"children": "SELECT ?node WHERE { ?node ex:broader ?parent }",
		}}},
	}}
	cases := []struct{ url, want string }{
		{"/api/lazy-tree/t/parents?src=pages/x.html&node=http://e.org/a", "parents"},
		{"/api/lazy-tree/t/search?src=pages/x.html&q=x", "search"},
		{"/api/lazy-tree/t/node-data?src=pages/x.html&node=http://e.org/a", "node-data"},
		{"/api/lazy-tree/t/focus?src=pages/x.html&node=http://e.org/a", "parents"},
	}
	for _, tc := range cases {
		code, env := doTreeRequest(t, tc.url)
		if code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400", tc.url, code)
		}
		if !strings.Contains(env.Error, tc.want) {
			t.Errorf("%s: error %q does not name the missing role %q", tc.url, env.Error, tc.want)
		}
	}
}

func TestLazyTreeRejectsOversizedBatch(t *testing.T) {
	asyncIdx = &asyncIndex{trees: map[string]map[string]parser.TreeQueries{
		"pages/x.html": {"t": {ID: "t", Roles: map[string]string{
			"roots":    "SELECT ?node WHERE { ?node a ex:C }",
			"children": "SELECT ?node WHERE { ?node ex:broader ?parent }",
			"parents":  "SELECT ?node ?parent WHERE { ?node ex:broader ?parent }",
		}}},
	}}
	var sb strings.Builder
	sb.WriteString("/api/lazy-tree/t/parents?src=pages/x.html")
	for i := 0; i <= maxBatchNodes; i++ {
		sb.WriteString("&node=http://e.org/n")
		sb.WriteString(strings.Repeat("x", 1))
	}
	code, env := doTreeRequest(t, sb.String())
	if code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for an oversized batch", code)
	}
	if !strings.Contains(env.Error, "too many") {
		t.Errorf("error = %q", env.Error)
	}
}

// Scoping: a tree declared by one set must be invisible to another.
func TestLazyTreeIDsAreSetScoped(t *testing.T) {
	asyncIdx = &asyncIndex{trees: map[string]map[string]parser.TreeQueries{
		"pages/a.html": {"t": {ID: "t", Roles: map[string]string{
			"roots": "SELECT ?node WHERE {}", "children": "SELECT ?node WHERE { ?node ex:b ?parent }"}}},
	}}
	if _, found := findTreeQueries("pages/a.html", "t"); !found {
		t.Error("tree not found from its own set")
	}
	if _, found := findTreeQueries("pages/b.html", "t"); found {
		t.Error("tree leaked into another template set")
	}
}

// --- Startup gate -----------------------------------------------------------

// A malformed declaration must abort the boot, naming the file. Written to a temp
// template set so the shipped templates stay untouched.
func TestInitAsyncIndexRejectsBadTreeDeclarations(t *testing.T) {
	cases := []struct {
		name, block, want string
	}{
		{
			name: "missing children role",
			block: `<sparql-tree-queries for="t">
			          <sparql-tree-query role="roots">SELECT ?node WHERE { ?node a ex:C }</sparql-tree-query>
			        </sparql-tree-queries>`,
			want: "children",
		},
		{
			name: "unknown role",
			block: `<sparql-tree-queries for="t">
			          <sparql-tree-query role="roots">SELECT ?node WHERE { ?node a ex:C }</sparql-tree-query>
			          <sparql-tree-query role="children">SELECT ?node WHERE { ?node ex:b ?parent }</sparql-tree-query>
			          <sparql-tree-query role="ancestors">SELECT ?node WHERE {}</sparql-tree-query>
			        </sparql-tree-queries>`,
			want: "unknown role",
		},
		{
			// The gap the plan found: a bound ?parent makes the server's VALUES
			// intersect instead of parameterise, silently returning a wrong level.
			name: "children binds the reserved variable",
			block: `<sparql-tree-queries for="t">
			          <sparql-tree-query role="roots">SELECT ?node WHERE { ?node a ex:C }</sparql-tree-query>
			          <sparql-tree-query role="children">SELECT ?node WHERE { VALUES ?parent { &lt;http://e.org/a&gt; } ?node ex:b ?parent }</sparql-tree-query>
			        </sparql-tree-queries>`,
			want: "parent",
		},
		{
			name: "children cannot be scoped",
			block: `<sparql-tree-queries for="t">
			          <sparql-tree-query role="roots">SELECT ?node WHERE { ?node a ex:C }</sparql-tree-query>
			          <sparql-tree-query role="children">SELECT ?node WHERE { ?node a ex:C }</sparql-tree-query>
			        </sparql-tree-queries>`,
			want: "never mentions",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			writeTreeFixture(t, dir, tc.block)
			err := initAsyncIndex(dir)
			if err == nil {
				t.Fatalf("startup accepted a declaration it should reject (%s)", tc.name)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q does not mention %q", err, tc.want)
			}
		})
	}
}

func TestInitAsyncIndexRejectsDuplicateTreeID(t *testing.T) {
	dir := t.TempDir()
	block := `<sparql-tree-queries for="dup">
	            <sparql-tree-query role="roots">SELECT ?node WHERE { ?node a ex:C }</sparql-tree-query>
	            <sparql-tree-query role="children">SELECT ?node WHERE { ?node ex:b ?parent }</sparql-tree-query>
	          </sparql-tree-queries>`
	writeTreeFixture(t, dir, block)
	// A second declaration of the same id, in a partial the page includes.
	mustWrite(t, filepath.Join(dir, "partials", "dup.html"),
		`{{ define "dupPartial" }}`+block+`{{ end }}`)
	mustWrite(t, filepath.Join(dir, "pages", "index.html"),
		`{{ define "content" }}{{ template "dupPartial" . }}{{ end }}`)

	err := initAsyncIndex(dir)
	if err == nil {
		t.Fatal("duplicate tree id accepted")
	}
	if !strings.Contains(err.Error(), "dup") || !strings.Contains(err.Error(), "duplicate") {
		t.Errorf("error %q should name the duplicate id", err)
	}
}

// A valid declaration in the shipped template layout must index cleanly.
func TestInitAsyncIndexAcceptsValidTree(t *testing.T) {
	dir := t.TempDir()
	writeTreeFixture(t, dir, `<sparql-tree-queries for="conceptTree">
	    <sparql-tree-query role="roots">SELECT ?node ?label WHERE { ?node skos:topConceptOf ?? ; skos:prefLabel ?label } ORDER BY ?label</sparql-tree-query>
	    <sparql-tree-query role="children">SELECT ?node ?label WHERE { ?node skos:broader ?parent ; skos:prefLabel ?label } ORDER BY ?label</sparql-tree-query>
	    <sparql-tree-query role="parents">SELECT ?node ?parent WHERE { ?node skos:broader ?parent }</sparql-tree-query>
	  </sparql-tree-queries>`)
	if err := initAsyncIndex(dir); err != nil {
		t.Fatalf("valid declaration rejected: %v", err)
	}
	var found bool
	for set := range asyncIdx.trees {
		if _, ok := findTreeQueries(set, "conceptTree"); ok {
			found = true
		}
	}
	if !found {
		t.Error("valid tree not indexed")
	}
}

// writeTreeFixture lays out the minimum template set the indexer walks, with the
// declaration block in the page.
func writeTreeFixture(t *testing.T, dir, block string) {
	t.Helper()
	mustWrite(t, filepath.Join(dir, "layout", "base.html"),
		`{{ define "base" }}<html><body>{{ template "content" . }}</body></html>{{ end }}`)
	mustWrite(t, filepath.Join(dir, "pages", "tree.html"),
		`{{ define "content" }}`+block+`{{ end }}`)
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
