package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"hutzli.org/visoto/internal/config"
	"hutzli.org/visoto/internal/lang"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// newTestRouter wires the two middlewares under test with a handler the test
// controls, using the shipped language set.
func newTestRouter(handler gin.HandlerFunc) *gin.Engine {
	siteLangs = lang.New(configLanguages(config.DefaultLanguages()), "en")
	r := gin.New()
	r.Use(resolveLang())
	r.Use(etagMiddleware())
	r.GET("/p", handler)
	return r
}

func do(r *gin.Engine, headers map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/p", nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	if _, ok := headers[lang.SiteLangHeader]; ok && headers[lang.SiteLangHeader] == "" {
		req.Header[http.CanonicalHeaderKey(lang.SiteLangHeader)] = []string{""}
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func okHandler(body string) gin.HandlerFunc {
	return func(c *gin.Context) {
		markCacheable(c)
		c.String(http.StatusOK, body)
	}
}

func TestETagAndConditionalRequest(t *testing.T) {
	r := newTestRouter(okHandler("hello"))

	first := do(r, map[string]string{"Accept-Language": "fr-CH,fr;q=0.9"})
	if first.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", first.Code)
	}
	etag := first.Header().Get("ETag")
	if !strings.HasPrefix(etag, `"fr-v`) {
		t.Errorf("ETag = %q, want a \"fr-v…\" tag", etag)
	}
	if first.Body.String() != "hello" {
		t.Errorf("body = %q, want %q", first.Body.String(), "hello")
	}

	// Replaying with the tag must produce an empty 304 that still carries the
	// cache metadata, or the client has nothing to refresh its entry with.
	second := do(r, map[string]string{"Accept-Language": "fr-CH", "If-None-Match": etag})
	if second.Code != http.StatusNotModified {
		t.Fatalf("conditional status = %d, want 304", second.Code)
	}
	if second.Body.Len() != 0 {
		t.Errorf("304 carried a body: %q", second.Body.String())
	}
	if got := second.Header().Get("ETag"); got != etag {
		t.Errorf("304 ETag = %q, want %q", got, etag)
	}
	if second.Header().Get("Cache-Control") == "" || second.Header().Get("Vary") == "" {
		t.Error("304 dropped Cache-Control or Vary")
	}
}

func TestETagDiffersPerLanguage(t *testing.T) {
	// Same bytes, different language: the tags must not collide, or a shared
	// cache keyed on the tag could hand one language's page to another.
	r := newTestRouter(okHandler("same"))
	de := do(r, map[string]string{lang.SiteLangHeader: "de"}).Header().Get("ETag")
	fr := do(r, map[string]string{lang.SiteLangHeader: "fr"}).Header().Get("ETag")
	if de == fr {
		t.Errorf("ETags collide across languages: %q", de)
	}
	if !strings.HasPrefix(de, `"de-v`) || !strings.HasPrefix(fr, `"fr-v`) {
		t.Errorf("ETags not language-prefixed: de=%q fr=%q", de, fr)
	}
}

func TestETagDiffersPerBody(t *testing.T) {
	a := do(newTestRouter(okHandler("one")), nil).Header().Get("ETag")
	b := do(newTestRouter(okHandler("two")), nil).Header().Get("ETag")
	if a == b {
		t.Errorf("different bodies produced the same ETag %q", a)
	}
}

func TestVaryFollowsHeaderPresence(t *testing.T) {
	r := newTestRouter(okHandler("x"))

	noHeader := do(r, map[string]string{"Accept-Language": "de"})
	if got := noHeader.Header().Get("Vary"); got != "Accept-Language" {
		t.Errorf("Vary without X-Site-Lang = %q, want Accept-Language", got)
	}

	withHeader := do(r, map[string]string{lang.SiteLangHeader: "de"})
	if got := withHeader.Header().Get("Vary"); got != lang.SiteLangHeader {
		t.Errorf("Vary with X-Site-Lang = %q, want %s", got, lang.SiteLangHeader)
	}

	// Present-but-empty is the "no language" choice, and still means a cache in
	// front of us normalized the request.
	emptyHeader := do(r, map[string]string{lang.SiteLangHeader: ""})
	if got := emptyHeader.Header().Get("Vary"); got != lang.SiteLangHeader {
		t.Errorf("Vary with empty X-Site-Lang = %q, want %s", got, lang.SiteLangHeader)
	}
	if got := emptyHeader.Header().Get("ETag"); !strings.HasPrefix(got, `"-v`) {
		t.Errorf("ETag for the empty language = %q, want a \"-v…\" tag", got)
	}
}

func TestCacheControlSplit(t *testing.T) {
	cacheable := do(newTestRouter(okHandler("x")), nil).Header().Get("Cache-Control")
	if !strings.Contains(cacheable, "s-maxage=21600") || !strings.Contains(cacheable, "max-age=0") {
		t.Errorf("cacheable Cache-Control = %q, want max-age=0 with s-maxage", cacheable)
	}

	// Not marked cacheable: the browser policy is the same, but a shared cache
	// must not hold it for hours.
	plain := do(newTestRouter(func(c *gin.Context) { c.String(http.StatusOK, "x") }), nil)
	if cc := plain.Header().Get("Cache-Control"); strings.Contains(cc, "s-maxage") {
		t.Errorf("non-cacheable Cache-Control = %q, want no s-maxage", cc)
	}
}

func TestNoStoreIsLeftAlone(t *testing.T) {
	// Transient errors opt out explicitly; the middleware must not overwrite that
	// or a failed SPARQL query would be replayed from cache for hours.
	r := newTestRouter(func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		c.String(http.StatusOK, "transient failure")
	})
	w := do(r, nil)
	if got := w.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", got)
	}
	if w.Header().Get("ETag") != "" {
		t.Error("a no-store response was given an ETag")
	}
	if w.Body.String() != "transient failure" {
		t.Errorf("body = %q", w.Body.String())
	}
}

func TestNonOKIsNotETagged(t *testing.T) {
	r := newTestRouter(func(c *gin.Context) { c.String(http.StatusNotFound, "nope") })
	w := do(r, nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
	if w.Header().Get("ETag") != "" {
		t.Error("a 404 was given an ETag")
	}
	if w.Body.String() != "nope" {
		t.Errorf("404 body = %q, want it passed through", w.Body.String())
	}
}

func TestSkippedPathsPassThrough(t *testing.T) {
	siteLangs = lang.New([]lang.Language{{Code: "en", Label: "English"}}, "en")
	r := gin.New()
	r.Use(resolveLang())
	r.Use(etagMiddleware())
	r.GET("/ping", func(c *gin.Context) { c.String(http.StatusOK, "pong") })

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/ping", nil))
	if w.Body.String() != "pong" {
		t.Errorf("body = %q, want pong", w.Body.String())
	}
	if w.Header().Get("ETag") != "" {
		t.Error("/ping was ETagged")
	}
}

func TestETagMatches(t *testing.T) {
	tests := []struct {
		ifNoneMatch, etag string
		want              bool
	}{
		{"", `"a"`, false},
		{`"a"`, `"a"`, true},
		{`"b"`, `"a"`, false},
		{`*`, `"a"`, true},
		{`"x", "a", "y"`, `"a"`, true},
		{`W/"a"`, `"a"`, true}, // If-None-Match uses weak comparison
		{`"a"`, `W/"a"`, true},
	}
	for _, tt := range tests {
		if got := etagMatches(tt.ifNoneMatch, tt.etag); got != tt.want {
			t.Errorf("etagMatches(%q, %q) = %v, want %v", tt.ifNoneMatch, tt.etag, got, tt.want)
		}
	}
}
