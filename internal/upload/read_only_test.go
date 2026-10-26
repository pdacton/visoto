package upload

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"
	"hutzli.org/visoto/internal/config"
)

// Writes to an endpoint without write credentials are refused before anything
// is fetched or sent, so a public endpoint never sees the request.
func TestWritesToReadOnlyEndpointAreRefused(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var hits atomic.Int32
	store := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
	}))
	defer store.Close()

	cfg := &config.ApplicationConfig{SparqlEndpoints: []config.SparqlEndpoint{
		{Name: "Public", URL: store.URL, Slug: "public"},
	}}
	router := gin.New()
	router.POST("/api/upload", UploadHandler(cfg))
	router.DELETE("/api/named-graphs", DeleteNamedGraphHandler(cfg))

	form := url.Values{"endpoint": {"public"}, "graphURI": {"urn:x:g"}, "url": {"https://example.org/data.ttl"}}
	requests := []*http.Request{
		httptest.NewRequest(http.MethodPost, "/api/upload", strings.NewReader(form.Encode())),
		httptest.NewRequest(http.MethodDelete, "/api/named-graphs?endpoint=public&graph=urn:x:g", nil),
	}
	requests[0].Header.Set("Content-Type", "application/x-www-form-urlencoded")

	for _, req := range requests {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Errorf("%s %s: status %d, want 403", req.Method, req.URL.Path, rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "read-only") {
			t.Errorf("%s %s: body %q does not say read-only", req.Method, req.URL.Path, rec.Body.String())
		}
	}
	if n := hits.Load(); n != 0 {
		t.Errorf("endpoint received %d requests, want 0", n)
	}
}

func TestWritable(t *testing.T) {
	cases := []struct {
		ep   config.SparqlEndpoint
		want bool
	}{
		{config.SparqlEndpoint{}, false},
		{config.SparqlEndpoint{AccessToken: "t"}, true},
		{config.SparqlEndpoint{Username: "u", Password: "p"}, true},
	}
	for _, c := range cases {
		if got := c.ep.Writable(); got != c.want {
			t.Errorf("%+v: Writable() = %v, want %v", c.ep, got, c.want)
		}
	}
}
