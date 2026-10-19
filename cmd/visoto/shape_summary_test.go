package main

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// shapeStore answers the summary query with a node shape targeting
// schema:Event that has one required date property; with status != 200 it
// fails the summary query, with empty it returns no rows.
func shapeStore(status int, empty bool) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), "SELECT+DISTINCT+%3Fd+%3Fs+%3Fp+%3Fo") || empty {
			fmt.Fprint(w, `{"head":{"vars":[]},"results":{"bindings":[]}}`)
			return
		}
		if status != http.StatusOK {
			http.Error(w, "out of memory", status)
			return
		}
		const (
			sh    = "http://www.w3.org/ns/shacl#"
			shape = `{"type":"uri","value":"https://example.org/EventShape"}`
			ps    = `{"type":"bnode","value":"ps"}`
		)
		row := func(d int, s, p, o string) string {
			return fmt.Sprintf(`{"d":{"type":"literal","value":"%d"},"s":%s,"p":{"type":"uri","value":"%s"},"o":%s}`, d, s, p, o)
		}
		rows := []string{
			row(0, shape, sh+"targetClass", `{"type":"uri","value":"http://schema.org/Event"}`),
			row(0, shape, sh+"property", ps),
			row(1, ps, sh+"path", `{"type":"uri","value":"http://schema.org/startDate"}`),
			row(1, ps, sh+"minCount", `{"type":"literal","value":"1"}`),
			row(1, ps, sh+"maxCount", `{"type":"literal","value":"1"}`),
			row(1, ps, sh+"datatype", `{"type":"uri","value":"http://www.w3.org/2001/XMLSchema#date"}`),
		}
		fmt.Fprintf(w, `{"head":{"vars":["d","s","p","o"]},"results":{"bindings":[%s]}}`, strings.Join(rows, ","))
	}))
}

func setupShapeSummary(t *testing.T, endpointURL string) *gin.Engine {
	r := setupBnodeTree(t, endpointURL)
	r.GET("/api/shape-summary", resolveQueryLang(), shapeSummaryHandler)
	return r
}

func TestShapeSummaryHandler(t *testing.T) {
	srv := shapeStore(http.StatusOK, false)
	defer srv.Close()
	r := setupShapeSummary(t, srv.URL)

	w := get(r, "/api/shape-summary?iri=https%3A%2F%2Fexample.org%2FEventShape&lang=en")
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	for _, want := range []string{
		"data-shape-summary",
		"Applies to",
		"Start date", // humanized local name: the store resolves no labels
		"required, exactly one",
		"a date",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("card lacks %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "&lt;no value&gt;") || strings.Contains(body, "<no value>") {
		t.Error("card renders an unset value")
	}
	if cc := w.Header().Get("Cache-Control"); cc == "no-store" {
		t.Error("a summary must be cacheable")
	}

	// German page: the phrases come from the catalog.
	if w := get(r, "/api/shape-summary?iri=https%3A%2F%2Fexample.org%2FEventShape&lang=de"); !strings.Contains(w.Body.String(), "erforderlich") {
		t.Errorf("German card lacks translated phrases:\n%s", w.Body.String())
	}
}

func TestShapeSummaryHandlerEmptyErrorInvalid(t *testing.T) {
	t.Run("no shape", func(t *testing.T) {
		srv := shapeStore(http.StatusOK, true)
		defer srv.Close()
		if w := get(setupShapeSummary(t, srv.URL), "/api/shape-summary?iri=https%3A%2F%2Fexample.org%2Fplain"); w.Code != http.StatusOK || w.Body.Len() != 0 {
			t.Errorf("no shape: %d %q, want an empty 200", w.Code, w.Body.String())
		}
	})
	t.Run("failed query", func(t *testing.T) {
		srv := shapeStore(http.StatusInternalServerError, false)
		defer srv.Close()
		w := get(setupShapeSummary(t, srv.URL), "/api/shape-summary?iri=https%3A%2F%2Fexample.org%2FEventShape")
		if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "alert-danger") || w.Header().Get("Cache-Control") != "no-store" {
			t.Errorf("failed query: %d cc=%q, want an uncached error card", w.Code, w.Header().Get("Cache-Control"))
		}
	})
	t.Run("invalid", func(t *testing.T) {
		srv := shapeStore(http.StatusOK, true)
		defer srv.Close()
		r := setupShapeSummary(t, srv.URL)
		for _, url := range []string{
			"/api/shape-summary",
			"/api/shape-summary?iri=https%3A%2F%2Fexample.org%2F%3E%20%7D",
		} {
			if w := get(r, url); w.Code != http.StatusBadRequest {
				t.Errorf("%s: %d, want 400", url, w.Code)
			}
		}
	})
}
