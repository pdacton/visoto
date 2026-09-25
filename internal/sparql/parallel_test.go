package sparql

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// TestExecuteQueriesParallelNCapsConcurrency checks that no more than
// maxConcurrent requests reach the endpoint at once, and that every query
// still gets a result.
func TestExecuteQueriesParallelNCapsConcurrency(t *testing.T) {
	var inFlight, peak int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&inFlight, 1)
		for {
			old := atomic.LoadInt32(&peak)
			if n <= old || atomic.CompareAndSwapInt32(&peak, old, n) {
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
		atomic.AddInt32(&inFlight, -1)
		w.Header().Set("Content-Type", "application/sparql-results+json")
		fmt.Fprint(w, `{"head":{"vars":["n"]},"results":{"bindings":[{"n":{"type":"literal","value":"1"}}]}}`)
	}))
	defer srv.Close()

	p := New(QueryInput{EndpointURL: srv.URL, Timeout: 10 * time.Second})
	queries := make([]ExtractedQuery, 12)
	for i := range queries {
		queries[i] = ExtractedQuery{ID: fmt.Sprint(i), Query: fmt.Sprintf("SELECT (%d AS ?n) {}", i)}
	}

	results := p.ExecuteQueriesParallelN(queries, 3, 10*time.Second, "")

	if got := atomic.LoadInt32(&peak); got > 3 {
		t.Errorf("peak concurrent requests = %d, want <= 3", got)
	}
	if len(results) != len(queries) {
		t.Fatalf("got %d results, want %d", len(results), len(queries))
	}
	for id, r := range results {
		if r.Error != "" {
			t.Errorf("query %s error = %q", id, r.Error)
		}
	}
}
