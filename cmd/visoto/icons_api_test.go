package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"hutzli.org/visoto/internal/icon"
)

func TestIconsHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "Canton.svg"), []byte("<svg/>"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := icon.Init(dir, nil); err != nil {
		t.Fatal(err)
	}

	router := gin.New()
	router.Use(etagMiddleware())
	router.GET(icon.ManifestPath, iconsHandler)

	tests := []struct {
		name, url, cacheControl string
	}{
		{"current version is immutable", icon.ManifestURL(), iconsImmutable},
		{"stale version revalidates", icon.ManifestPath + "?v=stale", "no-cache"},
		{"no version revalidates", icon.ManifestPath, "no-cache"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, tt.url, nil))
			if w.Code != http.StatusOK {
				t.Fatalf("status = %d", w.Code)
			}
			if got := w.Header().Get("Cache-Control"); got != tt.cacheControl {
				t.Errorf("Cache-Control = %q, want %q", got, tt.cacheControl)
			}
			if w.Header().Get("ETag") != "" {
				t.Error("manifest must bypass the ETag middleware")
			}
			var m icon.Manifest
			if err := json.Unmarshal(w.Body.Bytes(), &m); err != nil {
				t.Fatalf("body: %v", err)
			}
			if !m.Names["Canton"] {
				t.Errorf("manifest = %+v", m)
			}
		})
	}
}
