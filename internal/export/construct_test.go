package export

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/pdacton/visoto/internal/config"
)

// The providers build the graph term with sparql.IRITerm themselves, so an
// unvalidated caller still cannot splice query syntax through them.
func TestProvidersRefuseInvalidGraphIRI(t *testing.T) {
	var hits atomic.Int32
	store := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits.Add(1) }))
	defer store.Close()
	ep := &config.SparqlEndpoint{URL: store.URL}

	bad := "https://example.org/g> { ?s ?p ?o } } #"
	if _, err := constructFetch(ep, bad, "text/turtle"); err == nil {
		t.Error("constructFetch accepted an IRI containing '>'")
	}
	if _, err := graphdbFetch(store.URL+"/repositories/r", bad, "text/turtle", ep); err == nil {
		t.Error("graphdbFetch accepted an IRI containing '>'")
	}
	if n := hits.Load(); n != 0 {
		t.Errorf("store received %d requests, want 0", n)
	}
}
