package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"hutzli.org/visoto/internal/config"
	"hutzli.org/visoto/internal/parser"
)

// endpointsForTest installs a config with the given endpoints and returns a
// cleanup restoring the previous one.
func endpointsForTest(t *testing.T, eps ...config.SparqlEndpoint) {
	t.Helper()
	prev := cfg
	cfg = &config.Config{Application: config.ApplicationConfig{SparqlEndpoints: eps}}
	t.Cleanup(func() { cfg = prev })
}

// stamp runs stampEndpointData for a request and returns what the browser gets.
func stamp(t *testing.T, target string) parser.TemplateData {
	t.Helper()
	var data parser.TemplateData
	r := gin.New()
	r.GET("/p", resolveEndpoint(false), func(c *gin.Context) {
		stampEndpointData(c, &data)
		c.Status(http.StatusOK)
	})
	req := httptest.NewRequest(http.MethodGet, target, nil)
	r.ServeHTTP(httptest.NewRecorder(), req)
	return data
}

func TestProxyEndpointURLUsesSlug(t *testing.T) {
	endpointsForTest(t,
		config.SparqlEndpoint{Name: "LINDAS", Slug: "lindas-cached", URL: "https://cached.lindas.admin.ch/query", Default: true},
		config.SparqlEndpoint{Name: "QLever", Slug: "visoto-qlever", URL: "http://qlever:7001"},
	)

	got := stamp(t, "/p?endpoint=visoto-qlever").GraphQueryURL
	if want := "/api/sparql?endpoint=visoto-qlever"; got != want {
		t.Errorf("GraphQueryURL = %q, want %q", got, want)
	}
}

// The regression that would silently undo this whole change: the browser must
// never be handed the upstream URL, least of all the private QLever host.
func TestProxyEndpointURLNeverLeaksUpstream(t *testing.T) {
	endpointsForTest(t,
		config.SparqlEndpoint{Name: "QLever", Slug: "visoto-qlever", URL: "http://qlever:7001", Default: true},
	)

	got := stamp(t, "/p?endpoint=visoto-qlever").GraphQueryURL
	for _, leak := range []string{"qlever:7001", "http://", "https://"} {
		if strings.Contains(got, leak) {
			t.Errorf("GraphQueryURL %q leaked upstream detail %q", got, leak)
		}
	}
}

func TestProxyEndpointURLEscapesSlug(t *testing.T) {
	endpointsForTest(t,
		config.SparqlEndpoint{Name: "Odd", Slug: "a b&c", URL: "https://example.org/query", Default: true},
	)

	got := stamp(t, "/p").GraphQueryURL
	if want := "/api/sparql?endpoint=a+b%26c"; got != want {
		t.Errorf("GraphQueryURL = %q, want %q", got, want)
	}
}

func TestProxyEndpointURLBareConfig(t *testing.T) {
	// No endpoint list at all: the proxy resolves the scalar setting itself.
	prev := cfg
	cfg = &config.Config{Application: config.ApplicationConfig{SparqlEndpoint: "https://example.org/query"}}
	t.Cleanup(func() { cfg = prev })

	got := stamp(t, "/p").GraphQueryURL
	if want := "/api/sparql"; got != want {
		t.Errorf("GraphQueryURL = %q, want %q", got, want)
	}
}
