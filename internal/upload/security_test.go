package upload

import (
	"bytes"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/pdacton/visoto/internal/config"
)

// securityRouter wires the write and export routes against a writable test
// store that records every update and request it receives.
func securityRouter(t *testing.T, maxMB int) (*gin.Engine, *atomic.Int32, *[]string) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	var hits atomic.Int32
	var updates []string
	store := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		_ = r.ParseForm()
		if u := r.PostForm.Get("update"); u != "" {
			updates = append(updates, u)
		}
		_, _ = io.Copy(io.Discard, r.Body)
	}))
	t.Cleanup(store.Close)

	cfg := &config.ApplicationConfig{
		SparqlEndpoints:        []config.SparqlEndpoint{{Name: "Store", URL: store.URL, Slug: "store", AccessToken: "t"}},
		AllowPrivateUploadURLs: true, // the remote test servers below are on loopback
		MaxUploadMB:            maxMB,
	}
	router := gin.New()
	router.POST("/api/upload", UploadHandler(cfg))
	router.DELETE("/api/named-graphs", DeleteNamedGraphHandler(cfg))
	router.GET("/api/export-graphs", ExportNamedGraphsHandler(cfg))
	return router, &hits, &updates
}

func serve(router *gin.Engine, req *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// A ">" in a graph value used to close the IRI term and splice the rest into
// the query (export) or into a SPARQL Update (delete). Both are now refused
// before the store is contacted.
func TestGraphParamInjectionIsRefused(t *testing.T) {
	router, hits, _ := securityRouter(t, 0)
	payloads := []string{
		"https://example.org/g> { ?s ?p ?o } } #",
		"urn:x:g> ; DROP ALL ; #",
		"not an iri",
	}
	for _, p := range payloads {
		q := url.QueryEscape(p)
		for _, req := range []*http.Request{
			httptest.NewRequest(http.MethodGet, "/api/export-graphs?endpoint=store&graph="+q, nil),
			httptest.NewRequest(http.MethodDelete, "/api/named-graphs?endpoint=store&graph="+q, nil),
		} {
			if rec := serve(router, req); rec.Code != http.StatusBadRequest {
				t.Errorf("%s graph=%q: status %d, want 400", req.Method, p, rec.Code)
			}
		}
	}
	if n := hits.Load(); n != 0 {
		t.Errorf("store received %d requests, want 0", n)
	}
}

func TestDeleteNamedGraphSendsContainedTerm(t *testing.T) {
	router, _, updates := securityRouter(t, 0)
	req := httptest.NewRequest(http.MethodDelete, "/api/named-graphs?endpoint=store&graph=urn:x:g", nil)
	if rec := serve(router, req); rec.Code != http.StatusOK {
		t.Fatalf("status %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if len(*updates) != 1 || (*updates)[0] != "DROP SILENT GRAPH <urn:x:g>" {
		t.Errorf("updates = %q, want [DROP SILENT GRAPH <urn:x:g>]", *updates)
	}
}

func multipartUpload(t *testing.T, size int) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("file", "data.ttl")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = fw.Write(bytes.Repeat([]byte("#"), size))
	_ = mw.Close()
	req := httptest.NewRequest(http.MethodPost, "/api/upload?endpoint=store", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	return req
}

func TestFileUploadSizeCap(t *testing.T) {
	router, hits, _ := securityRouter(t, 1)

	if rec := serve(router, multipartUpload(t, 2<<20)); rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("2 MiB file: status %d, want 413", rec.Code)
	}
	if n := hits.Load(); n != 0 {
		t.Errorf("store received %d requests for an oversized file, want 0", n)
	}
	if rec := serve(router, multipartUpload(t, 512<<10)); rec.Code != http.StatusOK {
		t.Errorf("512 KiB file: status %d, want 200: %s", rec.Code, rec.Body.String())
	}
}

func urlUpload(remote string) *http.Request {
	form := url.Values{"url": {remote}}
	req := httptest.NewRequest(http.MethodPost, "/api/upload?endpoint=store", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return req
}

// The fetched body is capped too — whether the remote announces its length or
// streams it without one — and rejected rather than stored truncated.
func TestURLUploadSizeCap(t *testing.T) {
	router, _, _ := securityRouter(t, 1)
	big := bytes.Repeat([]byte("#"), 2<<20)

	announced := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/turtle")
		_, _ = w.Write(big)
	}))
	defer announced.Close()

	streamed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/turtle")
		for i := 0; i < len(big); i += 64 << 10 {
			_, _ = w.Write(big[i : i+64<<10])
			w.(http.Flusher).Flush() // forces chunked encoding: no Content-Length
		}
	}))
	defer streamed.Close()

	for name, srv := range map[string]*httptest.Server{"with Content-Length": announced, "chunked": streamed} {
		if rec := serve(router, urlUpload(srv.URL+"/data.ttl")); rec.Code != http.StatusRequestEntityTooLarge {
			t.Errorf("%s: status %d, want 413: %s", name, rec.Code, rec.Body.String())
		}
	}
}

func TestCappedReader(t *testing.T) {
	r := &cappedReader{r: strings.NewReader("12345"), left: 5}
	if b, err := io.ReadAll(r); err != nil || string(b) != "12345" || r.exceeded {
		t.Errorf("exactly at cap: %q, %v, exceeded=%v", b, err, r.exceeded)
	}
	r = &cappedReader{r: strings.NewReader("123456"), left: 5}
	if _, err := io.ReadAll(r); err != errUploadTooLarge || !r.exceeded {
		t.Errorf("over cap: err %v, exceeded=%v; want errUploadTooLarge", err, r.exceeded)
	}
}

// The dial-time check refuses a private address even when the hostname passed
// validation earlier (DNS rebinding).
func TestFetchDialerRefusesPrivateAddresses(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	if conn, err := fetchDialer(false).Dial("tcp", ln.Addr().String()); err == nil {
		conn.Close()
		t.Error("dial to loopback succeeded, want refusal")
	} else if !strings.Contains(err.Error(), "private or local address") {
		t.Errorf("unexpected error: %v", err)
	}
	if conn, err := fetchDialer(true).Dial("tcp", ln.Addr().String()); err != nil {
		t.Errorf("allow_private_upload_urls: dial failed: %v", err)
	} else {
		conn.Close()
	}
}
