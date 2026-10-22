package sparql

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"hutzli.org/visoto/internal/icon"
)

// iconEndpoint answers the three questions a table with an icon column asks —
// the query itself, rdf:type and the best "icon" value — and counts the last.
func iconEndpoint(t *testing.T, iconQueries *int32) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("query")
		if q == "" {
			body, _ := io.ReadAll(r.Body)
			if form, err := url.ParseQuery(string(body)); err == nil && form.Get("query") != "" {
				q = form.Get("query")
			} else {
				q = string(body)
			}
		}
		w.Header().Set("Content-Type", "application/sparql-results+json")
		switch {
		case strings.Contains(q, "schema.org/thumbnail"):
			atomic.AddInt32(iconQueries, 1)
			fmt.Fprint(w, `{"head":{"vars":["iri","label"]},"results":{"bindings":[
				{"iri":{"type":"uri","value":"https://ld.admin.ch/canton/1"},"label":{"type":"literal","value":"https://img.example/zh.svg"}}]}}`)
		case strings.Contains(q, "?iri (rdf:type|owl:type) ?type"):
			fmt.Fprint(w, `{"head":{"vars":["iri","type"]},"results":{"bindings":[
				{"iri":{"type":"uri","value":"https://ld.admin.ch/canton/1"},"type":{"type":"uri","value":"https://schema.ld.admin.ch/Canton"}},
				{"iri":{"type":"uri","value":"https://ld.admin.ch/canton/2"},"type":{"type":"uri","value":"https://schema.ld.admin.ch/Canton"}}]}}`)
		case strings.Contains(q, "MIN(?ranked)"): // labels
			fmt.Fprint(w, `{"head":{"vars":["iri","label"]},"results":{"bindings":[]}}`)
		default:
			fmt.Fprint(w, `{"head":{"vars":["canton"]},"results":{"bindings":[
				{"canton":{"type":"uri","value":"https://ld.admin.ch/canton/1"}},
				{"canton":{"type":"uri","value":"https://ld.admin.ch/canton/2"}}]}}`)
		}
	}))
}

// TestIconOverrideBeatsType: with an "icon" magic property, a resource's own
// icon value wins over its type's icon; a resource without one keeps the type's.
func TestIconOverrideBeatsType(t *testing.T) {
	withIcons(t, "Canton.svg")
	var iconQueries int32
	srv := iconEndpoint(t, &iconQueries)
	defer srv.Close()

	p := New(QueryInput{
		EndpointURL:     srv.URL,
		Timeout:         10 * time.Second,
		MagicProperties: map[string]string{IconProperty: "<http://schema.org/thumbnail>"},
	})
	result, err := p.ExecuteQueryWithContext(t.Context(), "SELECT ?canton { ?canton a ?c }", true, "en", "", WithTypes())
	if err != nil {
		t.Fatal(err)
	}

	if got := result.Icons["https://ld.admin.ch/canton/1"]; got != "https://img.example/zh.svg" {
		t.Errorf("Icons[canton/1] = %q, want the override", got)
	}
	if got := result.Icons["https://ld.admin.ch/canton/2"]; got != icon.BasePath+"Canton.svg" {
		t.Errorf("Icons[canton/2] = %q, want the type's icon", got)
	}
	if iconQueries != 1 {
		t.Errorf("icon queries = %d, want 1", iconQueries)
	}
}

// TestIconOverrideUnconfigured: without the magic property no icon query is sent.
func TestIconOverrideUnconfigured(t *testing.T) {
	withIcons(t, "Canton.svg")
	var iconQueries int32
	srv := iconEndpoint(t, &iconQueries)
	defer srv.Close()

	p := New(QueryInput{EndpointURL: srv.URL, Timeout: 10 * time.Second})
	result, err := p.ExecuteQueryWithContext(t.Context(), "SELECT ?canton { ?canton a ?c }", true, "en", "", WithTypes())
	if err != nil {
		t.Fatal(err)
	}
	if got := result.Icons["https://ld.admin.ch/canton/1"]; got != icon.BasePath+"Canton.svg" {
		t.Errorf("Icons[canton/1] = %q, want the type's icon", got)
	}
	if iconQueries != 0 {
		t.Errorf("icon queries = %d, want none", iconQueries)
	}
}
