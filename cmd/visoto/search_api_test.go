package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestSearchAPIRequiresQuery(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/api/search", searchAPIHandler)
	for _, url := range []string{"/api/search", "/api/search?q=%20%20"} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, url, nil))
		if w.Code != http.StatusBadRequest {
			t.Errorf("%s: status %d, want 400", url, w.Code)
		}
	}
}
