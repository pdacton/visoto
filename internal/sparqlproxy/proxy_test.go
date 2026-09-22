package sparqlproxy

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"hutzli.org/visoto/internal/config"
)

// upstream is a fake SPARQL endpoint that records what it received, and
// whether it was called at all.
type upstream struct {
	srv     *httptest.Server
	called  bool
	gotBody string
	gotHdr  http.Header
	status  int
	body    string
	ctype   string
}

func newUpstream(t *testing.T) *upstream {
	t.Helper()
	u := &upstream{status: http.StatusOK, body: `{"results":{"bindings":[]}}`, ctype: "application/sparql-results+json"}
	u.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u.called = true
		u.gotHdr = r.Header.Clone()
		b, _ := io.ReadAll(r.Body)
		u.gotBody = string(b)
		w.Header().Set("Content-Type", u.ctype)
		w.WriteHeader(u.status)
		_, _ = w.Write([]byte(u.body))
	}))
	t.Cleanup(u.srv.Close)
	return u
}

// newProxy wires a Proxy whose only endpoint points at the fake upstream.
func newProxy(t *testing.T, ep config.SparqlEndpoint) (*gin.Engine, *config.ApplicationConfig) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	cfg := &config.ApplicationConfig{SparqlEndpoints: []config.SparqlEndpoint{ep}}
	p := New(cfg, 5*time.Second)
	r := gin.New()
	r.POST(Path, p.Handler())
	return r, cfg
}

func post(r *gin.Engine, target, ctype, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, target, strings.NewReader(body))
	if ctype != "" {
		req.Header.Set("Content-Type", ctype)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestProxyForwardsReadQuery(t *testing.T) {
	up := newUpstream(t)
	r, _ := newProxy(t, config.SparqlEndpoint{Name: "Fake", Slug: "fake", URL: up.srv.URL, Default: true})

	q := "SELECT * WHERE { ?s ?p ?o }"
	w := post(r, Path+"?endpoint=fake", queryContentType, q)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %q)", w.Code, w.Body.String())
	}
	if !up.called {
		t.Fatal("upstream was not called")
	}
	if up.gotBody != q {
		t.Errorf("upstream body = %q, want %q", up.gotBody, q)
	}
	if got := up.gotHdr.Get("Content-Type"); got != queryContentType {
		t.Errorf("upstream Content-Type = %q, want %q", got, queryContentType)
	}
	if got := w.Header().Get("Content-Type"); got != "application/sparql-results+json" {
		t.Errorf("client Content-Type = %q, want the upstream's", got)
	}
	if got := w.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", got)
	}
	if got := w.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Errorf("X-Content-Type-Options = %q, want nosniff", got)
	}
}

func TestProxyRejectsUpdate(t *testing.T) {
	for _, q := range []string{
		"INSERT DATA { <a> <b> <c> }",
		"DELETE WHERE { ?s ?p ?o }",
		"DROP ALL",
		"LOAD <http://evil.example/data.ttl>",
		"# SELECT * WHERE {}\nINSERT DATA { <a> <b> <c> }",
	} {
		up := newUpstream(t)
		r, _ := newProxy(t, config.SparqlEndpoint{Slug: "fake", URL: up.srv.URL, Default: true})

		w := post(r, Path+"?endpoint=fake", queryContentType, q)

		if w.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want 400 for %q", w.Code, q)
		}
		// The point of the test: it never reached the endpoint.
		if up.called {
			t.Errorf("upstream WAS called for a write query: %q", q)
		}
	}
}

func TestProxyAttachesAuthServerSide(t *testing.T) {
	t.Run("bearer token", func(t *testing.T) {
		up := newUpstream(t)
		r, _ := newProxy(t, config.SparqlEndpoint{Slug: "fake", URL: up.srv.URL, Default: true, AccessToken: "s3cr3t"})
		post(r, Path+"?endpoint=fake", queryContentType, "SELECT * WHERE { ?s ?p ?o }")
		if got := up.gotHdr.Get("Authorization"); got != "Bearer s3cr3t" {
			t.Errorf("Authorization = %q, want Bearer s3cr3t", got)
		}
	})

	t.Run("no credentials means no header", func(t *testing.T) {
		up := newUpstream(t)
		r, _ := newProxy(t, config.SparqlEndpoint{Slug: "fake", URL: up.srv.URL, Default: true})
		post(r, Path+"?endpoint=fake", queryContentType, "SELECT * WHERE { ?s ?p ?o }")
		if got := up.gotHdr.Get("Authorization"); got != "" {
			t.Errorf("Authorization = %q, want none", got)
		}
	})

	// A rejected write must never cause credentials to be sent.
	t.Run("write query never carries the token", func(t *testing.T) {
		up := newUpstream(t)
		r, _ := newProxy(t, config.SparqlEndpoint{Slug: "fake", URL: up.srv.URL, Default: true, AccessToken: "s3cr3t"})
		post(r, Path+"?endpoint=fake", queryContentType, "INSERT DATA { <a> <b> <c> }")
		if up.called {
			t.Fatal("upstream called for a write query")
		}
	})
}

func TestProxyStripsClientHeaders(t *testing.T) {
	up := newUpstream(t)
	r, _ := newProxy(t, config.SparqlEndpoint{Slug: "fake", URL: up.srv.URL, Default: true, AccessToken: "config-token"})

	req := httptest.NewRequest(http.MethodPost, Path+"?endpoint=fake", strings.NewReader("SELECT * WHERE { ?s ?p ?o }"))
	req.Header.Set("Content-Type", queryContentType)
	req.Header.Set("Cookie", "site-lang=de; selectedEndpoint=evil")
	req.Header.Set("Authorization", "Bearer attacker-token")
	req.Header.Set("Referer", "http://evil.example/")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if got := up.gotHdr.Get("Cookie"); got != "" {
		t.Errorf("Cookie leaked upstream: %q", got)
	}
	if got := up.gotHdr.Get("Referer"); got != "" {
		t.Errorf("Referer leaked upstream: %q", got)
	}
	// The config's credentials must win over anything the client sent.
	if got := up.gotHdr.Get("Authorization"); got != "Bearer config-token" {
		t.Errorf("Authorization = %q, want the CONFIG token, not the client's", got)
	}
}

func TestProxyUnknownSlug(t *testing.T) {
	up := newUpstream(t)
	r, _ := newProxy(t, config.SparqlEndpoint{Slug: "fake", URL: up.srv.URL, Default: true})

	w := post(r, Path+"?endpoint=nope", queryContentType, "SELECT * WHERE { ?s ?p ?o }")

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
	if up.called {
		t.Error("upstream called for an unknown slug")
	}
}

func TestProxyRejectsWrongContentType(t *testing.T) {
	for _, ct := range []string{"", "application/x-www-form-urlencoded", "text/plain", "application/json"} {
		up := newUpstream(t)
		r, _ := newProxy(t, config.SparqlEndpoint{Slug: "fake", URL: up.srv.URL, Default: true})
		w := post(r, Path+"?endpoint=fake", ct, "SELECT * WHERE { ?s ?p ?o }")
		if w.Code != http.StatusUnsupportedMediaType {
			t.Errorf("Content-Type %q: status = %d, want 415", ct, w.Code)
		}
		if up.called {
			t.Errorf("Content-Type %q: upstream was called", ct)
		}
	}
}

func TestProxyAcceptsContentTypeWithCharset(t *testing.T) {
	up := newUpstream(t)
	r, _ := newProxy(t, config.SparqlEndpoint{Slug: "fake", URL: up.srv.URL, Default: true})
	w := post(r, Path+"?endpoint=fake", queryContentType+"; charset=utf-8", "SELECT * WHERE { ?s ?p ?o }")
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestProxyBodyTooLarge(t *testing.T) {
	up := newUpstream(t)
	r, _ := newProxy(t, config.SparqlEndpoint{Slug: "fake", URL: up.srv.URL, Default: true})

	big := "SELECT * WHERE { ?s ?p \"" + strings.Repeat("x", maxQueryBytes+1024) + "\" }"
	w := post(r, Path+"?endpoint=fake", queryContentType, big)

	if w.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("status = %d, want 413", w.Code)
	}
	if up.called {
		t.Error("upstream called for an oversize body")
	}
}

func TestProxyUpstreamDownHidesURL(t *testing.T) {
	// A URL that cannot be dialled.
	const dead = "http://127.0.0.1:1/query"
	r, _ := newProxy(t, config.SparqlEndpoint{Slug: "fake", URL: dead, Default: true})

	w := post(r, Path+"?endpoint=fake", queryContentType, "SELECT * WHERE { ?s ?p ?o }")

	if w.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want 502", w.Code)
	}
	// The whole point of the proxy is that the upstream URL stays server-side.
	if strings.Contains(w.Body.String(), dead) || strings.Contains(w.Body.String(), "127.0.0.1") {
		t.Errorf("response leaked the upstream URL: %s", w.Body.String())
	}
}

func TestProxyUpstreamErrorPassedThrough(t *testing.T) {
	up := newUpstream(t)
	up.status = http.StatusBadRequest
	up.body = "Parse error at line 1: unexpected token"
	up.ctype = "text/plain"

	r, _ := newProxy(t, config.SparqlEndpoint{Slug: "fake", URL: up.srv.URL, Default: true})
	w := post(r, Path+"?endpoint=fake", queryContentType, "SELECT bogus")

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want the upstream's 400", w.Code)
	}
	if !strings.Contains(w.Body.String(), "Parse error") {
		t.Errorf("upstream error body not passed through: %q", w.Body.String())
	}
}

func TestProxyBareConfigFallback(t *testing.T) {
	up := newUpstream(t)
	gin.SetMode(gin.TestMode)
	// No sparqlEndpoints list at all — just the scalar setting.
	cfg := &config.ApplicationConfig{SparqlEndpoint: up.srv.URL}
	r := gin.New()
	r.POST(Path, New(cfg, 5*time.Second).Handler())

	w := post(r, Path, queryContentType, "SELECT * WHERE { ?s ?p ?o }")

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %q)", w.Code, w.Body.String())
	}
	if !up.called {
		t.Error("upstream not called in bare-config mode")
	}
}

func TestNegotiateAccept(t *testing.T) {
	tests := []struct{ in, want string }{
		{"", "application/sparql-results+json"},
		{"application/sparql-results+json", "application/sparql-results+json"},
		{"text/turtle", "text/turtle"},
		// "*/*" means no preference: must fall through to SPARQL JSON, or the
		// endpoint picks its own default (LINDAS answers CSV).
		{"*/*", "application/sparql-results+json"},
		{"*/*;q=0.8", "application/sparql-results+json"},
		{"text/html", "application/sparql-results+json"},
		{"application/sparql-results+json;q=0.9", "application/sparql-results+json;q=0.9"},
	}
	for _, tc := range tests {
		if got := negotiateAccept(tc.in); got != tc.want {
			t.Errorf("negotiateAccept(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
