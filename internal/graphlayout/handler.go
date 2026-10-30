package graphlayout

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"hutzli.org/visoto/internal/config"
)

// API serves the /api/graph-layouts routes. A nil store (the database failed
// to open) answers 503 everywhere instead of keeping the routes away.
//
// The endpoint arrives as ?endpoint=<slug> on every route that takes one and
// is validated here; none of them reads the endpoint cookie.
type API struct {
	store *Store
	app   *config.ApplicationConfig
	log   *slog.Logger
}

// NewAPI builds the handlers over store (may be nil).
func NewAPI(store *Store, app *config.ApplicationConfig, log *slog.Logger) *API {
	return &API{store: store, app: app, log: log}
}

func (a *API) available(c *gin.Context) bool {
	if a.store == nil {
		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "shared layouts are unavailable"})
		return false
	}
	return true
}

// key reads endpoint (query) and page/graph (from the given values) into a Key.
func (a *API) key(c *gin.Context, page, graph string) (Key, bool) {
	// The configured spelling, since slugs match case-insensitively: a
	// differently-cased URL must not start its own history.
	ep := a.app.GetEndpointBySlug(c.Query("endpoint"))
	if ep == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "unknown or missing endpoint"})
		return Key{}, false
	}
	slug := ep.Slug
	canonical, ok := CanonicalPage(page)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid page"})
		return Key{}, false
	}
	if !ValidGraphID(graph) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid graph id"})
		return Key{}, false
	}
	return Key{Endpoint: slug, Page: canonical, Graph: graph}, true
}

// List is GET /api/graph-layouts?endpoint&page&graph: the page's history,
// its default, and whether sharing is open. Never cached: it changes with
// every share.
func (a *API) List(c *gin.Context) {
	if !a.available(c) {
		return
	}
	c.Header("Cache-Control", "no-store")
	k, ok := a.key(c, c.Query("page"), c.Query("graph"))
	if !ok {
		return
	}
	layouts, def, err := a.store.List(k)
	if err != nil {
		a.fail(c, "list", err)
		return
	}
	var defPtr *string
	if def != "" {
		defPtr = &def
	}
	c.JSON(http.StatusOK, gin.H{"writable": a.store.Writable(), "default": defPtr, "layouts": layouts})
}

// Get is GET /api/graph-layouts/:id: one snapshot with its key. Snapshots
// never change, so a hit is cacheable; the caller marks it.
func (a *API) Get(c *gin.Context) bool {
	if !a.available(c) {
		return false
	}
	id := c.Param("id")
	if !ValidID(id) {
		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusNotFound, gin.H{"error": "layout not found"})
		return false
	}
	l, err := a.store.Get(id)
	if err != nil {
		c.Header("Cache-Control", "no-store")
	}
	if errors.Is(err, ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "layout not found"})
		return false
	}
	if err != nil {
		a.fail(c, "get", err)
		return false
	}
	c.JSON(http.StatusOK, l)
	return true
}

// Share is POST /api/graph-layouts?endpoint: body {page, graph, snapshot}.
// Stores the snapshot, makes it the page default, answers 201 {id}.
func (a *API) Share(c *gin.Context) {
	if !a.writable(c) {
		return
	}
	var body struct {
		Page     string          `json:"page"`
		Graph    string          `json:"graph"`
		Snapshot json.RawMessage `json:"snapshot"`
	}
	if !a.bind(c, &body) {
		return
	}
	k, ok := a.key(c, body.Page, body.Graph)
	if !ok {
		return
	}
	id, err := a.store.Share(k, body.Snapshot)
	switch {
	case errors.Is(err, ErrInvalid):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case errors.Is(err, ErrFull):
		c.JSON(http.StatusInsufficientStorage, gin.H{"error": err.Error()})
	case err != nil:
		a.fail(c, "share", err)
	default:
		c.JSON(http.StatusCreated, gin.H{"id": id})
	}
}

// SetDefault is PUT /api/graph-layouts/default?endpoint&page&graph: body {id}.
// Points the page default at one of its snapshots ("Make default", revert).
func (a *API) SetDefault(c *gin.Context) {
	if !a.writable(c) {
		return
	}
	var body struct {
		ID string `json:"id"`
	}
	if !a.bind(c, &body) {
		return
	}
	k, ok := a.key(c, c.Query("page"), c.Query("graph"))
	if !ok {
		return
	}
	err := a.store.SetDefault(k, body.ID)
	switch {
	case errors.Is(err, ErrNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "layout not found for this page"})
	case err != nil:
		a.fail(c, "set default", err)
	default:
		c.Status(http.StatusNoContent)
	}
}

func (a *API) writable(c *gin.Context) bool {
	if !a.available(c) {
		return false
	}
	if !a.store.Writable() {
		c.JSON(http.StatusForbidden, gin.H{"error": "sharing layouts is turned off"})
		return false
	}
	return true
}

func (a *API) bind(c *gin.Context, v any) bool {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, int64(a.store.BodyLimit()))
	if err := json.NewDecoder(c.Request.Body).Decode(v); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "layout too large"})
		} else {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON body"})
		}
		return false
	}
	return true
}

func (a *API) fail(c *gin.Context, what string, err error) {
	a.log.Error("graph layouts: "+what+" failed", slog.String("error", err.Error()))
	c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
}
