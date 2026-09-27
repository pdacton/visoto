package classstats

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"

	"hutzli.org/visoto/internal/config"
)

// fakeEndpoint answers the collector's queries from a table. stats: the
// statistics graph (nil = not GraphDB, answers 0); live: live class counts.
type fakeEndpoint struct {
	mu      sync.Mutex
	classes []string
	stats   map[string]int64
	live    map[string]int64
	slow    map[string]bool // live count of these classes hangs
	queries []string
}

func (f *fakeEndpoint) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	q := string(body)
	f.mu.Lock()
	f.queries = append(f.queries, q)
	f.mu.Unlock()

	n := func(v int64) {
		fmt.Fprintf(w, `{"head":{"vars":["n"]},"results":{"bindings":[{"n":{"type":"literal","value":"%d"}}]}}`, v)
	}
	switch {
	case strings.Contains(q, "SELECT ?class ?label ?parent"):
		var rows []string
		for _, c := range f.classes {
			rows = append(rows, fmt.Sprintf(`{"class":{"type":"uri","value":%q},"label":{"type":"literal","value":"L","xml:lang":"en"}}`, c))
		}
		fmt.Fprintf(w, `{"head":{"vars":["class","label","parent"]},"results":{"bindings":[%s]}}`, strings.Join(rows, ","))
	case q == queryStatsTriples:
		if f.stats == nil {
			n(0)
		} else {
			n(1000)
		}
	case q == queryTriples:
		n(42)
	case strings.Contains(q, statisticsGraph):
		cl := classOf(q)
		n(f.stats[cl])
	default:
		cl := classOf(q)
		if f.slow[cl] {
			<-r.Context().Done()
			return
		}
		n(f.live[cl])
	}
}

func classOf(q string) string {
	i := strings.Index(q, "?s a <")
	if i < 0 {
		return ""
	}
	rest := q[i+len("?s a <"):]
	return rest[:strings.Index(rest, ">")]
}

func newTestCollector(t *testing.T, url string) *Collector {
	t.Helper()
	db, err := openDB(filepath.Join(t.TempDir(), "cs.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	cfg := &config.ApplicationConfig{SparqlEndpoints: []config.SparqlEndpoint{
		{Name: "store", URL: url, Slug: "store", ClassStats: true},
		{Name: "twin", URL: url, Slug: "twin", ClassStatsFrom: "store"},
		{Name: "other", URL: url, Slug: "other"},
	}}
	return newCollector(cfg, db)
}

const (
	cA = "https://example.org/A"
	cB = "https://example.org/B"
	cC = "https://example.org/C"
)

func TestRunGraphDBUsesStatistics(t *testing.T) {
	f := &fakeEndpoint{
		classes: []string{cA, cB, cA}, // repeated rows (labels) collapse
		stats:   map[string]int64{cA: 14_000_000, cB: 0},
		live:    map[string]int64{cB: 7},
	}
	srv := httptest.NewServer(f)
	defer srv.Close()
	c := newTestCollector(t, srv.URL)

	snap, err := c.Run(context.Background(), "store")
	if err != nil {
		t.Fatal(err)
	}
	if snap.Engine != "graphdb" || snap.Triples == nil || *snap.Triples != 1000 {
		t.Fatalf("engine/triples = %s/%v, want graphdb/1000", snap.Engine, snap.Triples)
	}
	if snap.Classes != 2 || snap.Counts[cA] != 14_000_000 || snap.Counts[cB] != 7 || snap.Failures != 0 {
		t.Fatalf("snapshot = %+v", snap)
	}
	for _, q := range f.queries {
		if strings.Contains(q, "VALUES") {
			t.Errorf("batched query defeats the statistics fast path: %s", q)
		}
	}
}

func TestRunPlainStoreCountsLive(t *testing.T) {
	f := &fakeEndpoint{classes: []string{cA, cB}, live: map[string]int64{cA: 3, cB: 5}}
	srv := httptest.NewServer(f)
	defer srv.Close()
	c := newTestCollector(t, srv.URL)

	snap, err := c.Run(context.Background(), "store")
	if err != nil {
		t.Fatal(err)
	}
	if snap.Engine != "sparql" || *snap.Triples != 42 || snap.Counts[cA] != 3 || snap.Counts[cB] != 5 {
		t.Fatalf("snapshot = %+v", snap)
	}
	for _, q := range f.queries {
		if strings.Contains(q, statisticsGraph) && q != queryStatsTriples {
			t.Errorf("per-class statistics query on a non-GraphDB store: %s", q)
		}
	}
}

func TestRunBudgetRecordsTimeouts(t *testing.T) {
	f := &fakeEndpoint{classes: []string{cA, cB, cC}, live: map[string]int64{cA: 1, cB: 2, cC: 3}}
	srv := httptest.NewServer(f)
	defer srv.Close()
	c := newTestCollector(t, srv.URL)
	// The run starts, checks the budget once for the first class, then the
	// clock jumps an hour: the 30-minute budget is spent after one class.
	base := time.Date(2026, 9, 27, 3, 0, 0, 0, time.UTC)
	calls := 0
	c.now = func() time.Time {
		calls++
		if calls <= 2 {
			return base
		}
		return base.Add(time.Hour)
	}

	snap, err := c.Run(context.Background(), "store")
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Counts) != 1 || snap.Failures != 2 {
		t.Fatalf("counts=%v failures=%d, want 1 counted, 2 timed out", snap.Counts, snap.Failures)
	}
	var timeouts int
	c.db.QueryRow(`SELECT COUNT(*) FROM class_counts WHERE method = ?`, MethodTimeout).Scan(&timeouts)
	if timeouts != 2 {
		t.Errorf("timeout rows = %d, want 2", timeouts)
	}
}

func TestRunRejectsNonCollectingEndpoint(t *testing.T) {
	c := newTestCollector(t, "http://unused")
	if _, err := c.Run(context.Background(), "twin"); err == nil {
		t.Error("Run on a class_stats_from endpoint should fail")
	}
}

func TestForResolvesSource(t *testing.T) {
	f := &fakeEndpoint{classes: []string{cA}, live: map[string]int64{cA: 1}}
	srv := httptest.NewServer(f)
	defer srv.Close()
	c := newTestCollector(t, srv.URL)
	if _, err := c.Run(context.Background(), "store"); err != nil {
		t.Fatal(err)
	}
	for slug, wantSrc := range map[string]string{"store": "store", "TWIN": "store", "other": "", "nope": ""} {
		snap, src := c.For(slug)
		if src != wantSrc || (wantSrc != "") != (snap != nil) {
			t.Errorf("For(%q) = %v, %q; want source %q", slug, snap != nil, src, wantSrc)
		}
	}
	var nilC *Collector
	if s, src := nilC.For("store"); s != nil || src != "" {
		t.Error("nil collector must answer nothing")
	}
}

func TestChangesAndReload(t *testing.T) {
	f := &fakeEndpoint{classes: []string{cA, cB, cC}, live: map[string]int64{cA: 100, cB: 50, cC: 10}}
	srv := httptest.NewServer(f)
	defer srv.Close()
	dir := t.TempDir()
	db, err := openDB(filepath.Join(dir, "cs.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	cfg := &config.ApplicationConfig{SparqlEndpoints: []config.SparqlEndpoint{{Name: "s", URL: srv.URL, Slug: "store", ClassStats: true}}}
	c := newCollector(cfg, db)

	day1 := time.Date(2026, 9, 26, 3, 0, 0, 0, time.UTC)
	c.now = func() time.Time { return day1 }
	if _, err := c.Run(context.Background(), "store"); err != nil {
		t.Fatal(err)
	}
	// Day 2: A grows, B drops by 60 %, C vanishes, D appears.
	cD := "https://example.org/D"
	f.classes = []string{cA, cB, cD}
	f.live = map[string]int64{cA: 110, cB: 20, cD: 5}
	c.now = func() time.Time { return day1.AddDate(0, 0, 1) }
	if _, err := c.Run(context.Background(), "store"); err != nil {
		t.Fatal(err)
	}

	ch, err := c.Changes("store", 1)
	if err != nil {
		t.Fatal(err)
	}
	if ch.From != "2026-09-26" || ch.To != "2026-09-27" {
		t.Fatalf("from/to = %s/%s", ch.From, ch.To)
	}
	got := map[string]Change{}
	for _, x := range ch.Changes {
		got[x.Class] = x
	}
	if len(got) != 4 {
		t.Fatalf("changes = %+v, want A, B, C, D", ch.Changes)
	}
	if got[cB].Delta != -30 || !got[cB].Drop || got[cA].Drop {
		t.Errorf("B = %+v, A = %+v", got[cB], got[cA])
	}
	if got[cC].After != nil || !got[cC].Drop {
		t.Errorf("vanished C = %+v", got[cC])
	}
	if got[cD].Before != nil || *got[cD].After != 5 {
		t.Errorf("new D = %+v", got[cD])
	}
	if ch.Changes[0].Class != cB { // |−30| is the largest change
		t.Errorf("first change = %s, want B", ch.Changes[0].Class)
	}

	// A fresh collector on the same DB finds the latest snapshot again.
	c2 := newCollector(cfg, db)
	if err := c2.loadLatest(); err != nil {
		t.Fatal(err)
	}
	if s := c2.Latest("store"); s == nil || s.Day != "2026-09-27" || s.Counts[cA] != 110 {
		t.Fatalf("reloaded snapshot = %+v", s)
	}
	pts, err := c2.Series("store", cA, 3650)
	if err != nil || len(pts) != 2 || *pts[0].Count != 100 || *pts[1].Count != 110 {
		t.Fatalf("series = %+v, %v", pts, err)
	}
}

func TestDiffIgnoresTimeouts(t *testing.T) {
	before := map[string]int64{cA: 1}
	after := map[string]int64{cB: 2}
	// A timed out today, B timed out yesterday: neither vanished nor new.
	got := diff(before, after, map[string]bool{cA: true, cB: true}, map[string]bool{cA: true, cB: true})
	if len(got) != 0 {
		t.Errorf("diff = %+v, want none", got)
	}
}

func TestPrometheusCollector(t *testing.T) {
	odd := `https://example.org/"quoted"\class`
	n := int64(99)
	c := newTestCollector(t, "http://unused")
	c.latest["store"] = &Snapshot{
		Day: "2026-09-27", Counts: map[string]int64{odd: 5}, Triples: &n, Engine: "graphdb",
		Started: time.Unix(100, 0), Finished: time.Unix(160, 0), Classes: 2, Failures: 1,
	}
	want := `
# HELP visoto_class_instances Instances of a class in the latest daily snapshot (GraphDB statistics are approximate).
# TYPE visoto_class_instances gauge
visoto_class_instances{class="https://example.org/\"quoted\"\\class",endpoint="store"} 5
# HELP visoto_store_triples Triples in the store at the latest daily snapshot.
# TYPE visoto_store_triples gauge
visoto_store_triples{endpoint="store"} 99
# HELP visoto_class_stats_run_duration_seconds How long the latest class-stats run took.
# TYPE visoto_class_stats_run_duration_seconds gauge
visoto_class_stats_run_duration_seconds{endpoint="store"} 60
# HELP visoto_class_stats_failures Classes the latest class-stats run could not count (timeout or error).
# TYPE visoto_class_stats_failures gauge
visoto_class_stats_failures{endpoint="store"} 1
`
	if err := testutil.CollectAndCompare(c, strings.NewReader(want),
		"visoto_class_instances", "visoto_store_triples", "visoto_class_stats_run_duration_seconds", "visoto_class_stats_failures"); err != nil {
		t.Error(err)
	}
}
