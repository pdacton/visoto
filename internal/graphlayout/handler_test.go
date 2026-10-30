package graphlayout

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"hutzli.org/visoto/internal/config"
)

func testRouter(t *testing.T, cfg config.GraphLayoutsConfig) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	app := &config.ApplicationConfig{SparqlEndpoints: []config.SparqlEndpoint{{Name: "LINDAS", Slug: "lindas-prod"}}}
	api := NewAPI(testStore(t, cfg), app, slog.New(slog.NewTextHandler(io.Discard, nil)))
	r := gin.New()
	r.GET("/api/graph-layouts", api.List)
	r.POST("/api/graph-layouts", api.Share)
	r.PUT("/api/graph-layouts/default", api.SetDefault)
	r.GET("/api/graph-layouts/:id", func(c *gin.Context) { api.Get(c) })
	return r
}

func do(r *gin.Engine, method, target, body string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(method, target, strings.NewReader(body)))
	return w
}

func shareBody(page string, snapshot []byte) string {
	b, _ := json.Marshal(map[string]any{"page": page, "graph": "graph", "snapshot": json.RawMessage(snapshot)})
	return string(b)
}

func TestAPIRoundTrip(t *testing.T) {
	r := testRouter(t, config.GraphLayoutsConfig{Mode: "open"})

	// Differently-cased slug and extra params land on the same key.
	w := do(r, "POST", "/api/graph-layouts?endpoint=LINDAS-PROD", shareBody("/resource?iri=urn%3Ax&endpoint=lindas-prod&layout=q", snap("a", 2, "")))
	if w.Code != http.StatusCreated {
		t.Fatalf("share: %d %s", w.Code, w.Body)
	}
	var created struct{ ID string }
	json.Unmarshal(w.Body.Bytes(), &created)

	w = do(r, "GET", "/api/graph-layouts?endpoint=lindas-prod&graph=graph&page=%2Fresource%3Firi%3Durn%253Ax", "")
	if w.Code != http.StatusOK || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("list: %d %q %s", w.Code, w.Header().Get("Cache-Control"), w.Body)
	}
	var list struct {
		Writable bool
		Default  *string
		Layouts  []Summary
	}
	json.Unmarshal(w.Body.Bytes(), &list)
	if !list.Writable || list.Default == nil || *list.Default != created.ID || len(list.Layouts) != 1 {
		t.Fatalf("list = %s", w.Body)
	}

	w = do(r, "GET", "/api/graph-layouts/"+created.ID, "")
	if w.Code != http.StatusOK || !bytes.Contains(w.Body.Bytes(), []byte(`"page":"/resource?iri=urn%3Ax"`)) {
		t.Fatalf("get: %d %s", w.Code, w.Body)
	}

	w = do(r, "PUT", "/api/graph-layouts/default?endpoint=lindas-prod&graph=graph&page=%2Fresource%3Firi%3Durn%253Ax", `{"id":"`+created.ID+`"}`)
	if w.Code != http.StatusNoContent {
		t.Fatalf("set default: %d %s", w.Code, w.Body)
	}
}

func TestAPIErrors(t *testing.T) {
	open := testRouter(t, config.GraphLayoutsConfig{Mode: "open", MaxBytes: 300})
	off := testRouter(t, config.GraphLayoutsConfig{})
	cases := []struct {
		name   string
		r      *gin.Engine
		method string
		target string
		body   string
		want   int
	}{
		{"off refuses share", off, "POST", "/api/graph-layouts?endpoint=lindas-prod", shareBody("/x.html", snap("a", 1, "")), http.StatusForbidden},
		{"off refuses default", off, "PUT", "/api/graph-layouts/default?endpoint=lindas-prod&page=/x.html&graph=graph", `{"id":"0000000000"}`, http.StatusForbidden},
		{"off still lists", off, "GET", "/api/graph-layouts?endpoint=lindas-prod&page=/x.html&graph=graph", "", http.StatusOK},
		{"unknown endpoint", open, "POST", "/api/graph-layouts?endpoint=nope", shareBody("/x.html", snap("a", 1, "")), http.StatusBadRequest},
		{"absolute page", open, "POST", "/api/graph-layouts?endpoint=lindas-prod", shareBody("https://evil.example/", snap("a", 1, "")), http.StatusBadRequest},
		{"not a canvas", open, "POST", "/api/graph-layouts?endpoint=lindas-prod", shareBody("/x.html", []byte(`{"format":"x"}`)), http.StatusBadRequest},
		{"too large", open, "POST", "/api/graph-layouts?endpoint=lindas-prod", shareBody("/x.html", snap("a", 20, "")), http.StatusRequestEntityTooLarge},
		{"bad graph id", open, "GET", "/api/graph-layouts?endpoint=lindas-prod&page=/x.html&graph=a%20b", "", http.StatusBadRequest},
		{"unknown id", open, "GET", "/api/graph-layouts/0000000000", "", http.StatusNotFound},
		{"malformed id", open, "GET", "/api/graph-layouts/x", "", http.StatusNotFound},
		{"default for unknown id", open, "PUT", "/api/graph-layouts/default?endpoint=lindas-prod&page=/x.html&graph=graph", `{"id":"0000000000"}`, http.StatusNotFound},
	}
	for _, tc := range cases {
		if w := do(tc.r, tc.method, tc.target, tc.body); w.Code != tc.want {
			t.Errorf("%s: got %d (%s), want %d", tc.name, w.Code, w.Body, tc.want)
		}
	}
}
