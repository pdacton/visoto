package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/pdacton/visoto/internal/access"
	"github.com/pdacton/visoto/internal/config"
	"github.com/pdacton/visoto/internal/graphlayout"
	"github.com/pdacton/visoto/internal/lang"
	"github.com/pdacton/visoto/internal/logger"
	mcpserver "github.com/pdacton/visoto/internal/mcp"
	"github.com/pdacton/visoto/internal/sparql"
)

// testUser is the context key the test plugin stores the caller under.
type testUser struct{}

// testPlugin stands in for the accounts edition: the X-Test-User header is the
// signed-in user, "private" a store only a signed-in user may browse and
// "write" the only store anyone may upload to.
type testPlugin struct{}

func (testPlugin) Init(h *Host) error {
	h.Use(func(c *gin.Context) {
		if u := c.GetHeader("X-Test-User"); u != "" {
			c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), testUser{}, u))
		}
	})
	h.SetAuthorizer(testAuthorizer{})
	h.Handle(http.MethodGet, "/plugin/hello", ActionPublic, func(c *gin.Context) { c.String(http.StatusOK, "hello") })
	return nil
}

type testAuthorizer struct{}

func (testAuthorizer) Authorize(ctx context.Context, r access.Request) bool {
	signedIn := ctx.Value(testUser{}) != nil
	switch {
	case r.Action == access.Upload:
		return signedIn && r.Store == "write"
	case r.Action == access.Admin:
		return false
	case r.Store == "private":
		return signedIn
	}
	return true
}

// routedServer builds the full route table with plugins, against a config of
// three stores; the stores themselves are never contacted by these tests.
func routedServer(t *testing.T, plugins ...Plugin) (*gin.Engine, *routeTable) {
	t.Helper()
	savedCfg, savedAuthz, savedLangs := cfg, authz, siteLangs
	t.Cleanup(func() { cfg, authz, siteLangs = savedCfg, savedAuthz, savedLangs })
	siteLangs = lang.New(configLanguages(config.DefaultLanguages()), "en")
	cfg = &config.Config{Application: config.ApplicationConfig{
		SparqlEndpoints: []config.SparqlEndpoint{
			{Name: "Public", Slug: "public", URL: "http://127.0.0.1:1/public", Default: true, ClassStats: true},
			{Name: "Private", Slug: "private", URL: "http://127.0.0.1:1/private", ClassStats: true},
			{Name: "Write", Slug: "write", URL: "http://127.0.0.1:1/write"},
		},
		SparqlEndpoint: "http://127.0.0.1:1/public",
	}}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(resolveLang(), etagMiddleware())
	deps := routeDeps{
		mcp:     mcpserver.NewServer(cfg, sparql.New(sparql.QueryInput{EndpointURL: cfg.Application.SparqlEndpoint}), nil, func() access.Authorizer { return authz }),
		layouts: graphlayout.NewAPI(nil, &cfg.Application, logger.Get()),
	}
	table, err := buildRoutes(router, deps, plugins)
	if err != nil {
		t.Fatal(err)
	}
	return router, table
}

func serveAs(router *gin.Engine, method, target, user string, header ...string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, nil)
	if user != "" {
		req.Header.Set("X-Test-User", user)
	}
	for i := 0; i+1 < len(header); i += 2 {
		req.Header.Set(header[i], header[i+1])
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// Every router entry was registered through the route table, with an action —
// a route added straight onto the router would skip its guard (UA-98).
func TestEveryRouteDeclaresAccess(t *testing.T) {
	router, table := routedServer(t, testPlugin{})
	declared := map[string]bool{}
	for _, r := range table.routes {
		if r.action == "" {
			t.Errorf("%s %s declares no action", r.method, r.path)
		}
		if r.any {
			declared["ANY "+r.path] = true
			continue
		}
		declared[r.method+" "+r.path] = true
	}
	for _, r := range router.Routes() {
		if !declared[r.Method+" "+r.Path] && !declared["ANY "+r.Path] {
			t.Errorf("%s %s is on the router but not in the route table", r.Method, r.Path)
		}
	}
	if !declared["GET /plugin/hello"] {
		t.Error("plugin route missing from the table")
	}
}

func TestGuardRefusesAndAllows(t *testing.T) {
	router, _ := routedServer(t, testPlugin{})
	cases := []struct {
		name, method, target, user string
		want                       int
	}{
		{"anonymous browse of a private store", "GET", "/api/graph-layouts?endpoint=private", "", 403},
		{"anonymous resource page on a private store", "GET", "/resource?iri=urn:x&endpoint=private", "", 403},
		{"anonymous upload", "POST", "/api/upload?endpoint=write", "", 403},
		{"upload to a store without the right", "POST", "/api/upload?endpoint=public", "u", 403},
		{"admin route", "POST", "/api/cache/purge", "u", 403},
		{"anonymous drop of a graph", "DELETE", "/api/named-graphs?endpoint=write&graph=urn:x:g", "", 403},
		{"public route", "GET", "/ping", "", 200},
		{"plugin route", "GET", "/plugin/hello", "", 200},
	}
	for _, tc := range cases {
		if rec := serveAs(router, tc.method, tc.target, tc.user); rec.Code != tc.want {
			t.Errorf("%s: status %d, want %d", tc.name, rec.Code, tc.want)
		}
	}
	// A signed-in user passes the guard; the handler then answers (503: no layout store).
	if rec := serveAs(router, "GET", "/api/graph-layouts?endpoint=private", "u"); rec.Code == http.StatusForbidden {
		t.Errorf("signed-in browse of a private store refused")
	}
}

// A response an anonymous visitor could not get never reaches the shared cache
// (UA-99); one the anonymous visitor could get keeps the core's policy.
func TestPersonalResponsesAreNoStore(t *testing.T) {
	router, _ := routedServer(t, testPlugin{})
	rec := serveAs(router, "GET", "/api/graph-layouts?endpoint=private", "u")
	// no-store is the guarantee; a handler's own error path may drop "private".
	if cc := rec.Header().Get("Cache-Control"); !strings.Contains(cc, "no-store") {
		t.Errorf("private store: Cache-Control %q, want no-store", cc)
	}
	rec = serveAs(router, "GET", "/api/graph-layouts?endpoint=public", "u")
	if cc := rec.Header().Get("Cache-Control"); strings.Contains(cc, "private") {
		t.Errorf("public store: Cache-Control %q, want the shared policy", cc)
	}
}

// Caddy's forward_auth check answers exactly what the real request would meet.
func TestAuthCheck(t *testing.T) {
	router, _ := routedServer(t, testPlugin{})
	check := func(uri, user string) int {
		return serveAs(router, "GET", authCheckPath, user, "X-Forwarded-Method", "GET", "X-Forwarded-Uri", uri).Code
	}
	if got := check("/resource?iri=urn:x&endpoint=private", ""); got != http.StatusForbidden {
		t.Errorf("anonymous, private store: %d, want 403", got)
	}
	if got := check("/resource?iri=urn:x&endpoint=private", "u"); got != http.StatusNoContent {
		t.Errorf("signed in, private store: %d, want 204", got)
	}
	if got := check("/static/js/upload.js", "u"); got != http.StatusNoContent {
		t.Errorf("static asset: %d, want 204", got)
	}
	if got := check("no-slash", "u"); got != http.StatusBadRequest {
		t.Errorf("malformed uri: %d, want 400", got)
	}
}

// Store listings name only the stores the caller may browse, and a listing
// that includes a private one is itself private (UA-48).
func TestStoreListingsAreFiltered(t *testing.T) {
	router, _ := routedServer(t, testPlugin{})

	anon := serveAs(router, "GET", "/api/class-stats/status", "")
	if strings.Contains(anon.Body.String(), `"private"`) {
		t.Errorf("anonymous listing names the private store: %s", anon.Body.String())
	}
	user := serveAs(router, "GET", "/api/class-stats/status", "u")
	if !strings.Contains(user.Body.String(), `"private"`) {
		t.Errorf("signed-in listing lacks the private store: %s", user.Body.String())
	}
	if cc := user.Header().Get("Cache-Control"); !strings.Contains(cc, "no-store") {
		t.Errorf("personal listing: Cache-Control %q, want no-store", cc)
	}
}

// A page the shared cache may store lists only what an anonymous visitor may
// browse, even for a signed-in caller — it is served to everyone.
func TestVisibleEndpointsOnSharedPages(t *testing.T) {
	routedServer(t, testPlugin{})
	slugs := func(user string, store string) []string {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest("GET", "/", nil)
		if user != "" {
			c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), testUser{}, user))
		}
		c.Set(accessKey, access.Request{Action: access.Browse, Store: store})
		eps, _ := visibleEndpoints(c)
		var out []string
		for _, ep := range eps {
			out = append(out, ep.Slug)
		}
		return out
	}
	if got := strings.Join(slugs("u", "public"), ","); got != "public,write" {
		t.Errorf("shared page for a signed-in user lists %q, want public,write", got)
	}
	if got := strings.Join(slugs("u", "private"), ","); got != "public,private,write" {
		t.Errorf("private page lists %q, want public,private,write", got)
	}
}

// Without plugins nothing changes: every route is open.
func TestOpenSourceDefaultAllowsAll(t *testing.T) {
	router, _ := routedServer(t)
	if rec := serveAs(router, "GET", authCheckPath, "", "X-Forwarded-Uri", "/resource?iri=urn:x&endpoint=private"); rec.Code != http.StatusNoContent {
		t.Errorf("auth check without plugins: %d, want 204", rec.Code)
	}
	if rec := serveAs(router, "GET", "/api/class-stats/status", ""); !strings.Contains(rec.Body.String(), `"private"`) {
		t.Errorf("listing without plugins hides a store: %s", rec.Body.String())
	}
}

// Static files must be revalidated, or a browser keeps running stale JS/CSS
// after an edit (Caddy sets its own max-age in production).
func TestStaticRevalidates(t *testing.T) {
	router, _ := routedServer(t, testPlugin{})
	w := serveAs(router, "GET", "/static/js/graph-kit.js", "")
	if got := w.Header().Get("Cache-Control"); got != "no-cache" {
		t.Errorf("Cache-Control = %q, want no-cache", got)
	}
}
