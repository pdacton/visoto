package app

import (
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestTestHostGuardsPluginRoutes(t *testing.T) {
	savedCfg, savedAuthz := cfg, authz
	t.Cleanup(func() { cfg, authz = savedCfg, savedAuthz })
	gin.SetMode(gin.TestMode)
	router := gin.New()
	h := TestHost(router, "", t.TempDir(), Store{Slug: "public"}, Store{Slug: "private"})
	if err := (testPlugin{}).Init(h); err != nil {
		t.Fatal(err)
	}
	h.Handle(http.MethodGet, "/data", ActionBrowse, func(c *gin.Context) { c.String(http.StatusOK, "data") })

	if rec := serveAs(router, "GET", "/data?endpoint=private", ""); rec.Code != http.StatusForbidden {
		t.Errorf("anonymous private: %d, want 403", rec.Code)
	}
	if rec := serveAs(router, "GET", "/data?endpoint=private", "u"); rec.Code != http.StatusOK {
		t.Errorf("signed-in private: %d, want 200", rec.Code)
	}
	if rec := serveAs(router, "GET", authCheckPath, "", "X-Forwarded-Uri", "/data?endpoint=private"); rec.Code != http.StatusForbidden {
		t.Errorf("auth check anonymous private: %d, want 403", rec.Code)
	}
}
