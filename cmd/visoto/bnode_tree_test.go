package main

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"hutzli.org/visoto/internal/bnodetree"
	"hutzli.org/visoto/internal/config"
	"hutzli.org/visoto/internal/i18n"
	"hutzli.org/visoto/internal/lang"
	"hutzli.org/visoto/internal/templates"
)

// bnodeStore answers the blank-node query with a property shape holding a name,
// an sh:order and a two-member sh:in, and every other query (label lookups)
// with no rows. With empty=true it answers the tree query with no rows too.
func bnodeStore(empty bool) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if empty || !strings.Contains(string(body), "SELECT+DISTINCT+%3Fd+%3Fs+%3Fp+%3Fo") {
			fmt.Fprint(w, `{"head":{"vars":[]},"results":{"bindings":[]}}`)
			return
		}
		row := func(d, s, p, o string) string {
			return fmt.Sprintf(`{"d":{"type":"literal","value":"%s","datatype":"http://www.w3.org/2001/XMLSchema#integer"},"s":%s,"p":{"type":"uri","value":"%s"},"o":%s}`, d, s, p, o)
		}
		const (
			root  = `{"type":"uri","value":"https://example.org/shape"}`
			ps    = `{"type":"bnode","value":"ps"}`
			c0    = `{"type":"bnode","value":"c0"}`
			c1    = `{"type":"bnode","value":"c1"}`
			sh    = "http://www.w3.org/ns/shacl#"
			rdf   = "http://www.w3.org/1999/02/22-rdf-syntax-ns#"
			nilV  = `{"type":"uri","value":"` + rdf + `nil"}`
			nameV = `{"type":"literal","value":"Jahr","xml:lang":"de"}`
		)
		rows := []string{
			row("0", root, sh+"property", ps),
			row("1", ps, "http://schema.org/name", nameV),
			row("1", ps, sh+"order", `{"type":"literal","value":"1","datatype":"http://www.w3.org/2001/XMLSchema#integer"}`),
			row("1", ps, sh+"in", c0),
			row("1", c0, rdf+"first", `{"type":"uri","value":"https://example.org/A"}`),
			row("1", c0, rdf+"rest", c1),
			row("1", c1, rdf+"first", `{"type":"uri","value":"https://example.org/B"}`),
			row("1", c1, rdf+"rest", nilV),
		}
		fmt.Fprintf(w, `{"head":{"vars":["d","s","p","o"]},"results":{"bindings":[%s]}}`, strings.Join(rows, ","))
	}))
}

func setupBnodeTree(t *testing.T, endpointURL string) *gin.Engine {
	t.Helper()
	// InitPartials reads templates/partials/… relative to the repository root.
	t.Chdir("../..")
	siteLangs = lang.New(configLanguages(config.DefaultLanguages()), "en")
	cats, err := i18n.Load("locales", siteLangs.Codes())
	if err != nil {
		t.Fatal(err)
	}
	if err := templates.InitPartials(cats, siteLangs); err != nil {
		t.Fatal(err)
	}
	saved := cfg
	t.Cleanup(func() { cfg = saved })
	cfg = &config.Config{
		Application: config.ApplicationConfig{SparqlEndpoint: endpointURL, Timeout: 5},
		RDF: config.RDFConfig{ParsedPrefixes: []config.Prefix{
			{Name: "sh", URI: "http://www.w3.org/ns/shacl#"},
			{Name: "schema", URI: "http://schema.org/"},
		}},
	}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/api/bnode-tree", resolveQueryLang(), bnodeTreeHandler)
	return r
}

func get(r *gin.Engine, url string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, url, nil))
	return w
}

func TestBnodeTreeHandlerCard(t *testing.T) {
	srv := bnodeStore(false)
	defer srv.Close()
	r := setupBnodeTree(t, srv.URL)

	w := get(r, "/api/bnode-tree?iri=https%3A%2F%2Fexample.org%2Fshape&lang=de&anchor=blank-nodes&title=Struktur")
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	for _, want := range []string{
		`id="blank-nodes"`,
		"data-bnode-tree",
		"Struktur",
		`<span class="vs-bn-title">Jahr</span>`,
		"einer der Werte", // sh:in caption, German catalog
		"1 leerer Knoten", // plural form "one"
		`format=ttl`,
		`<span class="vs-bn-prefix">sh:</span>in`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("card lacks %q", want)
		}
	}
	if strings.Contains(body, "&lt;no value&gt;") || strings.Contains(body, "<no value>") {
		t.Error("card renders an unset value")
	}
}

func TestBnodeTreeHandlerDownloads(t *testing.T) {
	srv := bnodeStore(false)
	defer srv.Close()
	r := setupBnodeTree(t, srv.URL)
	base := "/api/bnode-tree?iri=https%3A%2F%2Fexample.org%2Fshape&format="

	w := get(r, base+"ttl")
	if w.Code != http.StatusOK || !strings.HasPrefix(w.Header().Get("Content-Type"), "text/turtle") {
		t.Fatalf("ttl: %d %s", w.Code, w.Header().Get("Content-Type"))
	}
	if ttl := w.Body.String(); !strings.Contains(ttl, "sh:property [") || !strings.Contains(ttl, "sh:in ( <https://example.org/A> <https://example.org/B> )") {
		t.Errorf("turtle:\n%s", ttl)
	}
	if w := get(r, base+"nt"); strings.Count(w.Body.String(), " .\n") != 8 {
		t.Errorf("n-triples:\n%s", w.Body.String())
	}
	if w := get(r, base+"jsonld"); !strings.Contains(w.Body.String(), `"@language": "de"`) {
		t.Errorf("json-ld:\n%s", w.Body.String())
	}
	if w := get(r, base+"xml"); w.Code != http.StatusBadRequest {
		t.Errorf("unknown format: %d", w.Code)
	}
}

// chainStore answers the tree queries with a chain root → n1 → … of `hops`
// blank nodes, cut to the depth each query asks for, and counts them. With
// failDeep the full-depth query gets a 500, as an out-of-memory QLever answers.
func chainStore(hops int, failDeep bool, treeQueries *int) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		q := string(body)
		if !strings.Contains(q, "SELECT+DISTINCT+%3Fd+%3Fs+%3Fp+%3Fo") {
			fmt.Fprint(w, `{"head":{"vars":[]},"results":{"bindings":[]}}`)
			return
		}
		*treeQueries++
		deep := strings.Contains(q, "BIND%286+AS+%3Fd%29")
		if deep && failDeep {
			http.Error(w, `{"exception":"Tried to allocate 8 MB, but only 4 MB were available"}`, http.StatusInternalServerError)
			return
		}
		node := func(i int) string {
			if i == 0 {
				return `{"type":"uri","value":"https://example.org/shape"}`
			}
			return fmt.Sprintf(`{"type":"bnode","value":"n%d"}`, i)
		}
		var rows []string
		for d := 0; d < hops && (deep || d <= bnodetree.ProbeDepth); d++ {
			rows = append(rows, fmt.Sprintf(`{"d":{"type":"literal","value":"%d"},"s":%s,"p":{"type":"uri","value":"https://example.org/next"},"o":%s}`, d, node(d), node(d+1)))
		}
		fmt.Fprintf(w, `{"head":{"vars":["d","s","p","o"]},"results":{"bindings":[%s]}}`, strings.Join(rows, ","))
	}))
}

func TestBnodeTreeHandlerShallow(t *testing.T) {
	var n int
	srv := chainStore(bnodetree.ProbeDepth, false, &n)
	defer srv.Close()
	r := setupBnodeTree(t, srv.URL)
	if w := get(r, "/api/bnode-tree?iri=https%3A%2F%2Fexample.org%2Fshape"); w.Code != http.StatusOK {
		t.Fatalf("status %d", w.Code)
	}
	if n != 1 {
		t.Errorf("shallow structure: %d tree queries, want 1 (the probe only)", n)
	}
}

func TestBnodeTreeHandlerDeep(t *testing.T) {
	var n int
	srv := chainStore(4, false, &n)
	defer srv.Close()
	r := setupBnodeTree(t, srv.URL)
	w := get(r, "/api/bnode-tree?iri=https%3A%2F%2Fexample.org%2Fshape")
	if n != 2 {
		t.Errorf("deep structure: %d tree queries, want 2 (probe, then full)", n)
	}
	if strings.Contains(w.Body.String(), "could not be loaded") {
		t.Error("full query succeeded, nothing may be marked as not loaded")
	}
}

func TestBnodeTreeHandlerDeepFails(t *testing.T) {
	var n int
	srv := chainStore(4, true, &n)
	defer srv.Close()
	r := setupBnodeTree(t, srv.URL)
	w := get(r, "/api/bnode-tree?iri=https%3A%2F%2Fexample.org%2Fshape")
	if w.Code != http.StatusOK {
		t.Fatalf("status %d", w.Code)
	}
	body := w.Body.String()
	if strings.Contains(body, "alert-danger mb-0") || !strings.Contains(body, "Deeper levels could not be loaded") {
		t.Errorf("want the probe result with a not-loaded marker, got:\n%s", body)
	}
	if cc := w.Header().Get("Cache-Control"); cc != "no-store" {
		t.Errorf("Cache-Control = %q, a fallback must not be cached", cc)
	}
}

func TestBnodeTreeHandlerEmptyAndInvalid(t *testing.T) {
	srv := bnodeStore(true)
	defer srv.Close()
	r := setupBnodeTree(t, srv.URL)

	w := get(r, "/api/bnode-tree?iri=https%3A%2F%2Fexample.org%2Fplain")
	if w.Code != http.StatusOK || w.Body.Len() != 0 {
		t.Errorf("no blank nodes: %d %q, want an empty 200", w.Code, w.Body.String())
	}
	for _, url := range []string{
		"/api/bnode-tree",
		"/api/bnode-tree?iri=https%3A%2F%2Fexample.org%2F%3E%20%7D",
		"/api/bnode-tree?iri=https%3A%2F%2Fexample.org%2Fx&via=not%20an%20iri",
	} {
		if w := get(r, url); w.Code != http.StatusBadRequest {
			t.Errorf("%s: %d, want 400", url, w.Code)
		}
	}
}
