package app

import (
	"log/slog"

	"github.com/gin-gonic/gin"
	"github.com/pdacton/visoto/internal/access"
	"github.com/pdacton/visoto/internal/config"
	"github.com/pdacton/visoto/internal/logger"
	"github.com/prometheus/client_golang/prometheus"
)

// The extension points of the server. The open-source build runs without
// plugins; the commercial edition passes its own to Main (UA-9).

// Action is what a route does with the store it addresses; see the constants.
type Action = access.Action

const (
	ActionPublic   = access.Public
	ActionBrowse   = access.Browse
	ActionSPARQL   = access.SPARQL
	ActionUpload   = access.Upload
	ActionAdmin    = access.Admin
	ActionSignedIn = access.SignedIn
)

// Access is one authorization decision: an Action on a store, optionally for
// one resource IRI.
type Access = access.Request

// Authorizer decides every guarded request. See access.Authorizer for the
// context contract: context.Background() means an anonymous caller.
type Authorizer = access.Authorizer

// Plugin extends the server. Init runs once, after the configuration, the
// catalogs and the core middlewares are set up and before any core route is
// registered — so middleware a plugin adds applies to every route.
type Plugin interface {
	Init(h *Host) error
}

// Store is a configured SPARQL endpoint as plugins see it. Its URL and
// credentials stay inside the core.
type Store struct {
	Slug    string
	Name    string
	Default bool
}

// Host is what a plugin gets to work with.
type Host struct {
	// ConfigPath is the visoto.config the server loaded; a plugin reads its own
	// section from it.
	ConfigPath string
	// DataDir is the directory for local state (SQLite files).
	DataDir string

	table      *routeTable
	collectors *[]prometheus.Collector
}

// DecodeConfig decodes visoto.config into v, with the core's ${VAR} and .env
// expansion; a plugin declares only its own section, e.g. a struct with an
// `Accounts … toml:"accounts"` field.
func (h *Host) DecodeConfig(v any) error { return config.DecodeFile(h.ConfigPath, v) }

// Logger is the server's structured logger.
func (h *Host) Logger() *slog.Logger { return logger.Get() }

// RegisterCollector adds a Prometheus collector to /metrics.
func (h *Host) RegisterCollector(c prometheus.Collector) { *h.collectors = append(*h.collectors, c) }

// Use adds middleware that runs on every route, before its guard — the place to
// put the caller's identity into the request context.
func (h *Host) Use(mw ...gin.HandlerFunc) { h.table.router.Use(mw...) }

// Handle registers a plugin route with the access it needs, through the same
// table and guard as the core routes.
func (h *Host) Handle(method, path string, action Action, handlers ...gin.HandlerFunc) {
	h.table.handle(method, path, action, handlers...)
}

// SetAuthorizer replaces the allow-all default. An Authorizer that also
// implements Denier answers refused requests itself.
func (h *Host) SetAuthorizer(a Authorizer) { authz = a }

// Stores lists the configured endpoints.
func (h *Host) Stores() []Store {
	out := make([]Store, 0, len(cfg.Application.SparqlEndpoints))
	def := cfg.Application.DefaultEndpoint()
	for _, ep := range cfg.Application.SparqlEndpoints {
		out = append(out, Store{Slug: ep.Slug, Name: ep.Name, Default: def != nil && def.Slug == ep.Slug})
	}
	return out
}

// RenderName maps a logical template name ("pages/account.html") onto the
// variant registered for the request's language, for plugin handlers that
// render one of the core's templates.
func RenderName(c *gin.Context, templateName string) string { return renderName(c, templateName) }
