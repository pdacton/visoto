package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"hutzli.org/visoto/internal/classstats"
	"hutzli.org/visoto/internal/config"
)

func TestMetricsHandlerToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		token, auth string
		want        int
	}{
		{"", "", http.StatusNotFound},
		{"", "Bearer anything", http.StatusNotFound}, // no token configured: never open
		{"s3cret", "", http.StatusUnauthorized},
		{"s3cret", "Bearer wrong", http.StatusUnauthorized},
		{"s3cret", "s3cret", http.StatusUnauthorized}, // scheme required
		{"s3cret", "Bearer s3cret", http.StatusOK},
	}
	for _, tt := range tests {
		r := gin.New()
		r.GET("/metrics", metricsHandler(tt.token))
		req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
		if tt.auth != "" {
			req.Header.Set("Authorization", tt.auth)
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != tt.want {
			t.Errorf("token %q, auth %q: status %d, want %d", tt.token, tt.auth, w.Code, tt.want)
		}
		if got := w.Header().Get("Cache-Control"); got != "no-store" {
			t.Errorf("Cache-Control = %q, want no-store", got)
		}
		if w.Code == http.StatusOK && !strings.Contains(w.Body.String(), "go_goroutines") {
			t.Error("metrics body lacks the Go runtime metrics")
		}
	}
}

// classTreeStore answers the class-tree query with two classes and every
// count with 3.
func classTreeStore() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if strings.Contains(string(body), "SELECT ?class ?label ?parent") {
			fmt.Fprint(w, `{"head":{"vars":["class","label","parent"]},"results":{"bindings":[
				{"class":{"type":"uri","value":"https://example.org/A"},"label":{"type":"literal","value":"A","xml:lang":"de"}},
				{"class":{"type":"uri","value":"https://example.org/B"},"parent":{"type":"uri","value":"https://example.org/A"}}]}}`)
			return
		}
		fmt.Fprint(w, `{"head":{"vars":["n"]},"results":{"bindings":[{"n":{"type":"literal","value":"3"}}]}}`)
	}))
}

func TestClassTreeHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	srv := classTreeStore()
	defer srv.Close()
	app := &config.ApplicationConfig{SparqlEndpoints: []config.SparqlEndpoint{
		{Name: "store", URL: srv.URL, Slug: "store", ClassStats: true},
		{Name: "twin", URL: srv.URL, Slug: "twin", ClassStatsFrom: "store"},
		{Name: "plain", URL: srv.URL, Slug: "plain"},
	}}
	cs, err := classstats.New(app, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	saved := classStats
	classStats = cs
	defer func() { classStats = saved }()
	classTreeMu.Lock()
	classTreeCache = map[string]classTreeEntry{}
	classTreeMu.Unlock()

	serve := func(slug string) *httptest.ResponseRecorder {
		r := gin.New()
		r.GET("/api/class-tree", func(c *gin.Context) {
			c.Set(activeEndpointKey, app.GetEndpointBySlug(slug))
		}, classTreeHandler)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/class-tree?endpoint="+slug, nil))
		return w
	}
	counts := func(w *httptest.ResponseRecorder) map[string]string {
		var res classstats.Results
		if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
			t.Fatal(err)
		}
		out := map[string]string{}
		for _, b := range res.Results.Bindings {
			out[b["class"].Value] = b["instcount"].Value
		}
		return out
	}

	if w := serve("plain"); w.Code != http.StatusNotFound {
		t.Errorf("endpoint without class stats: status %d, want 404 (GE keeps its own query)", w.Code)
	}
	// Before the first run: the tree, without counts.
	w := serve("twin")
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body)
	}
	if got := counts(w); len(got) != 2 || got["https://example.org/A"] != "" {
		t.Errorf("before a run: %v, want two classes without counts", got)
	}
	// After a run, the twin serves the store's counts (the new day replaces the cache).
	if _, err := cs.Run(context.Background(), "store"); err != nil {
		t.Fatal(err)
	}
	if got := counts(serve("twin")); got["https://example.org/A"] != "3" || got["https://example.org/B"] != "3" {
		t.Errorf("after a run: %v, want instcount 3 for both", got)
	}
}
