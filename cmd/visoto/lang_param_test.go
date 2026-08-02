package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"hutzli.org/visoto/internal/config"
	"hutzli.org/visoto/internal/lang"
)

// newLangRouter wires resolveQueryLang the way the /api routes do and echoes the
// resolved code, so a test can assert on what the handler would have used.
func newLangRouter() *gin.Engine {
	siteLangs = lang.New(configLanguages(config.DefaultLanguages()), "en")
	r := gin.New()
	r.Use(resolveLang())
	r.GET("/api/x", resolveQueryLang(), func(c *gin.Context) {
		// Prefixed so an empty resolved code is still a distinguishable body.
		c.String(http.StatusOK, "lang="+queryLang(c))
	})
	return r
}

func getLang(t *testing.T, r *gin.Engine, url string, headers map[string]string) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, url, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("GET %s = %d, want 200", url, w.Code)
	}
	body := w.Body.String()
	if len(body) < 5 || body[:5] != "lang=" {
		t.Fatalf("unexpected body %q", body)
	}
	return body[5:]
}

func TestQueryLangFromURL(t *testing.T) {
	r := newLangRouter()

	tests := []struct {
		name string
		url  string
		want string
	}{
		{"plain code", "/api/x?lang=fr", "fr"},
		{"region subtag stripped", "/api/x?lang=fr-CH", "fr"},
		{"uppercase normalized", "/api/x?lang=FR", "fr"},
		// An unconfigured code must degrade to the default rather than error, so a
		// stale link keeps working — mirrors an unknown endpoint slug.
		{"unknown code falls back", "/api/x?lang=zz", "en"},
		{"absent means default", "/api/x", "en"},
		// The critical case: "" is a configured code (the "no language" choice),
		// so an explicitly empty param must NOT collapse into the default.
		{"empty code is honored", "/api/x?lang=", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := getLang(t, r, tt.url, nil); got != tt.want {
				t.Errorf("queryLang(%s) = %q, want %q", tt.url, got, tt.want)
			}
		})
	}
}

// The /api tier must be a pure function of the URL: neither the site-lang cookie
// nor Accept-Language may influence it, or a shared cache would serve one user's
// language to everyone.
func TestQueryLangIgnoresCookieAndHeader(t *testing.T) {
	r := newLangRouter()
	headers := map[string]string{
		"Cookie":          "site-lang=fr",
		"Accept-Language": "it-CH,it;q=0.9",
	}

	if got := getLang(t, r, "/api/x?lang=de", headers); got != "de" {
		t.Errorf("with ?lang=de, cookie=fr, header=it: got %q, want \"de\"", got)
	}
	if got := getLang(t, r, "/api/x", headers); got != "en" {
		t.Errorf("without ?lang: got %q, want the default \"en\" (cookie/header must not leak)", got)
	}
}

// resolveQueryLang doubles as the URL-purity declaration, so the two facts can
// never drift apart.
func TestResolveQueryLangMarksURLPure(t *testing.T) {
	siteLangs = lang.New(configLanguages(config.DefaultLanguages()), "en")
	r := gin.New()
	var pure bool
	r.GET("/api/x", resolveQueryLang(), func(c *gin.Context) {
		pure = isURLPure(c)
		c.String(http.StatusOK, "ok")
	})
	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/x?lang=fr", nil))
	if !pure {
		t.Error("resolveQueryLang did not mark the request URL-pure")
	}
}

// queryLang must not panic or return garbage on a route without the middleware.
func TestQueryLangWithoutMiddleware(t *testing.T) {
	siteLangs = lang.New(configLanguages(config.DefaultLanguages()), "en")
	r := gin.New()
	var got string
	r.GET("/p", func(c *gin.Context) {
		got = queryLang(c)
		c.String(http.StatusOK, "ok")
	})
	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/p?lang=fr", nil))
	if got != "en" {
		t.Errorf("queryLang without middleware = %q, want the default \"en\"", got)
	}
}

func TestInstanceCountKeyIncludesLang(t *testing.T) {
	de := instanceCountKey("http://ep", "id", "http://class", "", "de")
	fr := instanceCountKey("http://ep", "id", "http://class", "", "fr")
	if de == fr {
		t.Errorf("count cache key collides across languages: %q", de)
	}
	same := instanceCountKey("http://ep", "id", "http://class", "", "de")
	if de != same {
		t.Errorf("count cache key is not stable: %q vs %q", de, same)
	}
}
