package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"hutzli.org/visoto/internal/config"
)

func TestSearchAPIRequiresQuery(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/api/search", searchAPIHandler)
	for _, url := range []string{"/api/search", "/api/search?q=%20%20"} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, url, nil))
		if w.Code != http.StatusBadRequest {
			t.Errorf("%s: status %d, want 400", url, w.Code)
		}
	}
}

func TestResourceIconAPIRejectsInvalidIRI(t *testing.T) {
	prev := cfg
	cfg = &config.Config{Application: config.ApplicationConfig{SparqlEndpoint: "https://example.org/query"}}
	t.Cleanup(func() { cfg = prev })
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/api/resource-icon", resourceIconAPIHandler)
	for _, url := range []string{"/api/resource-icon", "/api/resource-icon?iri=%3E%20DROP", "/api/resource-icon?iri=no-scheme"} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, url, nil))
		if w.Code != http.StatusBadRequest {
			t.Errorf("%s: status %d, want 400", url, w.Code)
		}
	}
}
