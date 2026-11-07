package app

import (
	"context"
	"net/http"
	"net/http/httptest"

	"github.com/gin-gonic/gin"
	"github.com/pdacton/visoto/internal/access"
	"github.com/pdacton/visoto/internal/config"
)

// authz decides every guarded request. The open-source build allows all; a
// plugin replaces it through Host.SetAuthorizer.
var authz access.Authorizer = access.AllowAll{}

// route is one registered route and what it does with a store.
type route struct {
	method, path string
	action       access.Action
	any          bool // registered through Any (MCP)
}

// routeTable is the only way routes get onto the router. Every route declares
// its access.Action, and the table puts a guard in front of its handler, so a
// route cannot be added without saying what it does — TestEveryRouteDeclaresAccess
// fails on any router entry that bypassed the table.
type routeTable struct {
	router *gin.Engine
	routes []route
}

func newRouteTable(router *gin.Engine) *routeTable {
	return &routeTable{router: router}
}

// handle registers method path with the guard inserted after the route's own
// middlewares (the endpoint resolver must run first) and before its handler.
func (t *routeTable) handle(method, path string, action access.Action, handlers ...gin.HandlerFunc) {
	t.router.Handle(method, path, withGuard(action, handlers)...)
	t.routes = append(t.routes, route{method: method, path: path, action: action})
}

func (t *routeTable) GET(path string, action access.Action, h ...gin.HandlerFunc) {
	t.handle(http.MethodGet, path, action, h...)
}

func (t *routeTable) POST(path string, action access.Action, h ...gin.HandlerFunc) {
	t.handle(http.MethodPost, path, action, h...)
}

func (t *routeTable) PUT(path string, action access.Action, h ...gin.HandlerFunc) {
	t.handle(http.MethodPut, path, action, h...)
}

func (t *routeTable) DELETE(path string, action access.Action, h ...gin.HandlerFunc) {
	t.handle(http.MethodDelete, path, action, h...)
}

// Any registers every method, as gin's Any does.
func (t *routeTable) Any(path string, action access.Action, h ...gin.HandlerFunc) {
	t.router.Any(path, withGuard(action, h)...)
	t.routes = append(t.routes, route{method: "ANY", path: path, action: action, any: true})
}

// static serves a directory; the routes it adds (GET and HEAD) are public.
// Files carry Cache-Control: no-cache, so a browser revalidates them on every
// load (a cheap 304 via Last-Modified) instead of heuristically reusing an old
// copy after an edit. In production Caddy replaces the header with its own
// max-age for /static/* (Caddyfile).
func (t *routeTable) static(prefix, dir string) {
	group := t.router.Group(prefix, guard(access.Public), revalidate)
	group.Static("/", dir)
	t.routes = append(t.routes,
		route{method: http.MethodGet, path: prefix + "/*filepath", action: access.Public},
		route{method: http.MethodHead, path: prefix + "/*filepath", action: access.Public})
}

// staticFile serves one file; GET and HEAD, public.
func (t *routeTable) staticFile(path, file string) {
	t.router.Group("/", guard(access.Public)).StaticFile(path, file)
	t.routes = append(t.routes,
		route{method: http.MethodGet, path: path, action: access.Public},
		route{method: http.MethodHead, path: path, action: access.Public})
}

// revalidate marks a static response as reusable only after revalidation.
func revalidate(c *gin.Context) {
	c.Header("Cache-Control", "no-cache")
	c.Next()
}

func withGuard(action access.Action, handlers []gin.HandlerFunc) []gin.HandlerFunc {
	chain := make([]gin.HandlerFunc, 0, len(handlers)+1)
	chain = append(chain, handlers[:len(handlers)-1]...)
	chain = append(chain, guard(action), handlers[len(handlers)-1])
	return chain
}

// accessKey is the gin-context key under which guard stores the request's
// access.Request, for sharedSafe and the store listings.
const accessKey = "access"

// authCheckKey marks a request replayed by authCheckHandler: the guard answers
// it and the handler never runs.
type authCheckKey struct{}

// guard authorizes the request before its handler runs. Public routes are not
// asked about, only recorded.
func guard(action access.Action) gin.HandlerFunc {
	return func(c *gin.Context) {
		req := access.Request{Action: action, Store: requestStore(c), IRI: c.Query("iri")}
		c.Set(accessKey, req)
		allowed := action == access.Public || authz.Authorize(c.Request.Context(), req)
		if c.Request.Context().Value(authCheckKey{}) != nil {
			if allowed {
				c.AbortWithStatus(http.StatusNoContent)
			} else {
				c.AbortWithStatus(http.StatusForbidden)
			}
			return
		}
		if !allowed {
			deny(c, req)
			return
		}
		// A response an anonymous visitor could not get is personal and must never
		// enter the shared cache, whatever the handler declares later (UA-99).
		// Set before the handler writes, so streaming routes are covered too; the
		// ETag middleware leaves a no-store response alone.
		if !sharedSafe(c) {
			c.Header("Cache-Control", "private, no-store")
		}
	}
}

// Denier is optionally implemented by an Authorizer that answers a refused
// request itself — a redirect to sign-in for an anonymous caller, say. It must
// write the response; c is aborted afterwards either way.
type Denier interface {
	Deny(c *gin.Context, r access.Request)
}

func deny(c *gin.Context, req access.Request) {
	if d, ok := authz.(Denier); ok {
		d.Deny(c, req)
	}
	if !c.Writer.Written() {
		c.String(http.StatusForbidden, "forbidden")
	}
	c.Abort()
}

// requestStore is the slug of the store the request addresses: the endpoint the
// route's resolver chose, else ?endpoint=, else the default. It falls back the
// same way the handlers do (endpointFromSlug), so the guard and the handler
// never disagree about which store a request writes to.
func requestStore(c *gin.Context) string {
	if ep := activeEndpoint(c); ep != nil {
		return ep.Slug
	}
	if ep := cfg.Application.GetEndpointBySlug(c.Query("endpoint")); ep != nil {
		return ep.Slug
	}
	if ep := cfg.Application.DefaultEndpoint(); ep != nil {
		return ep.Slug
	}
	return ""
}

// sharedSafe reports whether this response may be stored by the shared cache:
// an anonymous visitor would be allowed the same request. A personal response
// (a private store, a restricted resource) never is (UA-99).
func sharedSafe(c *gin.Context) bool {
	v, ok := c.Get(accessKey)
	if !ok {
		return true
	}
	req, _ := v.(access.Request)
	return req.Action == access.Public || authz.Authorize(context.Background(), req)
}

// visibleEndpoints are the configured endpoints this response may list
// (UA-48). A response the shared cache may store lists what an anonymous
// visitor may browse — it is served to everyone — so a signed-in user's private
// stores never leak into it; the plugin adds those client-side. A private
// response lists the caller's own. personal reports whether the caller's list
// differs from the anonymous one, i.e. whether the list itself makes the
// response personal.
func visibleEndpoints(c *gin.Context) (eps []config.SparqlEndpoint, personal bool) {
	ctx := c.Request.Context()
	if sharedSafe(c) {
		ctx = context.Background()
	}
	for _, ep := range cfg.Application.SparqlEndpoints {
		req := access.Request{Action: access.Browse, Store: ep.Slug}
		if !authz.Authorize(ctx, req) {
			continue
		}
		eps = append(eps, ep)
		if !authz.Authorize(context.Background(), req) {
			personal = true
		}
	}
	return eps, personal
}

// listable reports whether a JSON listing may include store slug: the caller
// may browse it. Including one an anonymous visitor may not browse makes the
// response personal, so it is marked private, no-store (UA-48, UA-99).
func listable(c *gin.Context, slug string) bool {
	req := access.Request{Action: access.Browse, Store: slug}
	if !authz.Authorize(c.Request.Context(), req) {
		return false
	}
	if !authz.Authorize(context.Background(), req) {
		c.Header("Cache-Control", "private, no-store")
	}
	return true
}

// authCheckPath is Caddy's forward_auth target: for a signed-in request Caddy
// asks here before answering from its cache, which it does without Visoto
// otherwise seeing the request (architecture §2).
const authCheckPath = "/_auth/check"

// authCheckHandler answers whether the request Caddy is about to serve —
// X-Forwarded-Method and X-Forwarded-Uri, with this request's own headers and
// cookies — would be allowed. It replays it through the router in check-only
// mode, so the answer comes from exactly the resolvers and guard the real
// request would meet, without running its handler: 204 allowed, 403 refused.
// A path no route matches is allowed; the real request then 404s.
func authCheckHandler(router *gin.Engine) gin.HandlerFunc {
	return func(c *gin.Context) {
		method := c.GetHeader("X-Forwarded-Method")
		if method == "" {
			method = http.MethodGet
		}
		uri := c.GetHeader("X-Forwarded-Uri")
		if uri == "" || uri[0] != '/' {
			c.Status(http.StatusBadRequest)
			return
		}
		ctx := context.WithValue(c.Request.Context(), authCheckKey{}, true)
		replay, err := http.NewRequestWithContext(ctx, method, uri, nil)
		if err != nil {
			c.Status(http.StatusBadRequest)
			return
		}
		replay.Header = c.Request.Header.Clone()
		replay.Header.Del("X-Forwarded-Method")
		replay.Header.Del("X-Forwarded-Uri")
		replay.Host = c.Request.Host

		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, replay)
		if rec.Code == http.StatusForbidden {
			c.Status(http.StatusForbidden)
			return
		}
		c.Status(http.StatusNoContent)
	}
}
