package sparqlio

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"hutzli.org/visoto/internal/pipeline/rdf"
)

const selectResponse = `{
  "head": { "vars": [ "s", "p", "o" ] },
  "results": { "bindings": [
    { "s": {"type":"uri","value":"http://example.org/d1"},
      "p": {"type":"uri","value":"http://purl.org/dc/terms/title"},
      "o": {"type":"literal","xml:lang":"de","value":"Bevölkerung"} },
    { "s": {"type":"uri","value":"http://example.org/d1"},
      "p": {"type":"uri","value":"http://www.w3.org/ns/dcat#byteSize"},
      "o": {"type":"literal","datatype":"http://www.w3.org/2001/XMLSchema#decimal","value":"2411"} },
    { "s": {"type":"bnode","value":"b0"},
      "p": {"type":"uri","value":"http://example.org/p"},
      "o": {"type":"literal","value":"plain"} },
    { "s": {"type":"uri","value":"http://example.org/d1"},
      "p": {"type":"uri","value":"http://example.org/weird"},
      "o": {"type":"triple","value":"nope"} }
  ] }
}`

func newTestServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return srv
}

func TestSelectPreservesTermTypes(t *testing.T) {
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Content-Type"); got != "application/sparql-query" {
			t.Errorf("Content-Type = %q", got)
		}
		if got := r.Header.Get("User-Agent"); got != "visoto-harvest/test" {
			t.Errorf("User-Agent = %q", got)
		}
		w.Header().Set("Content-Type", "application/sparql-results+json")
		w.Write([]byte(selectResponse))
	})

	c := NewClient(srv.URL, WithUserAgent("visoto-harvest/test"))
	res, err := c.Select(context.Background(), "SELECT * WHERE { ?s ?p ?o }")
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	if len(res.Bindings) != 4 {
		t.Fatalf("got %d bindings, want 4", len(res.Bindings))
	}

	title := res.Bindings[0]["o"]
	if title.Kind != rdf.KindLiteral || title.Lang != "de" || title.Value != "Bevölkerung" {
		t.Errorf("language tag lost: %+v", title)
	}
	size := res.Bindings[1]["o"]
	if size.Datatype != rdf.NSXSD+"decimal" {
		t.Errorf("datatype lost: %+v", size)
	}
	if res.Bindings[2]["s"].Kind != rdf.KindBlank {
		t.Errorf("bnode lost: %+v", res.Bindings[2]["s"])
	}
	// A quoted triple has no N-Quads representation, so it must be dropped, not
	// coerced into a literal that would corrupt the load.
	if _, ok := res.Bindings[3]["o"]; ok {
		t.Error("unrepresentable term should be dropped")
	}
}

func TestBindingAccessors(t *testing.T) {
	b := Binding{
		"iri": rdf.IRI("http://example.org/x"),
		"lit": rdf.Literal("text"),
	}
	if got := b.IRI("iri"); got != "http://example.org/x" {
		t.Errorf("IRI() = %q", got)
	}
	if got := b.IRI("lit"); got != "" {
		t.Errorf("IRI() on a literal = %q, want empty", got)
	}
	if got := b.Str("lit"); got != "text" {
		t.Errorf("Str() = %q", got)
	}
	if got := b.Str("absent"); got != "" {
		t.Errorf("Str() on unbound = %q, want empty", got)
	}
	if _, ok := b.Term("absent"); ok {
		t.Error("Term() reported an unbound variable as bound")
	}
}

func TestStatusErrorRetryable(t *testing.T) {
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "slow down", http.StatusTooManyRequests)
	})
	_, err := NewClient(srv.URL).Select(context.Background(), "SELECT * WHERE {}")
	if err == nil {
		t.Fatal("expected an error")
	}
	se, ok := err.(*StatusError)
	if !ok {
		t.Fatalf("error type = %T, want *StatusError", err)
	}
	if se.Code != http.StatusTooManyRequests || !se.Retryable() {
		t.Errorf("429 should be retryable, got %+v", se)
	}

	srv2 := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad query", http.StatusBadRequest)
	})
	_, err = NewClient(srv2.URL).Select(context.Background(), "nonsense")
	if se, ok := err.(*StatusError); !ok || se.Retryable() {
		t.Errorf("400 should not be retryable, got %v", err)
	}
}

func TestUpdateSendsUpdateContentType(t *testing.T) {
	var seen string
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		seen = r.Header.Get("Content-Type")
		w.WriteHeader(http.StatusNoContent)
	})
	if err := NewClient(srv.URL, WithBearer("tok")).Update(context.Background(), "CLEAR GRAPH <urn:g>"); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if seen != "application/sparql-update" {
		t.Errorf("Content-Type = %q", seen)
	}
}

func TestAuthHeaders(t *testing.T) {
	var auth string
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		w.Write([]byte(`{"boolean":true}`))
	})
	if _, err := NewClient(srv.URL, WithBearer("tok")).Ask(context.Background(), "ASK {}"); err != nil {
		t.Fatalf("Ask: %v", err)
	}
	if auth != "Bearer tok" {
		t.Errorf("Authorization = %q", auth)
	}

	if _, err := NewClient(srv.URL, WithBasicAuth("u", "p")).Ask(context.Background(), "ASK {}"); err != nil {
		t.Fatalf("Ask: %v", err)
	}
	if auth == "" || auth == "Bearer tok" {
		t.Errorf("basic auth not applied, got %q", auth)
	}
}

func TestEscape(t *testing.T) {
	if got, want := Escape(`a"b\c`), `a\"b\\c`; got != want {
		t.Errorf("Escape = %q, want %q", got, want)
	}
	if got := Escape("a\nb"); got != `a\nb` {
		t.Errorf("Escape newline = %q", got)
	}
}

func TestEscapeIRIRejectsInjection(t *testing.T) {
	if _, ok := EscapeIRI("http://example.org/ok"); !ok {
		t.Error("valid IRI rejected")
	}
	for _, bad := range []string{"", "http://x/> } INSERT DATA { <a> <b> <c", "http://x/ y", "http://x/\n"} {
		if _, ok := EscapeIRI(bad); ok {
			t.Errorf("EscapeIRI(%q) accepted a malformed IRI", bad)
		}
	}
}
