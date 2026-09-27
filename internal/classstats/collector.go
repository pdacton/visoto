// Package classstats counts the instances of every used class, once a day, on
// the endpoints that opt in (class_stats = true), and keeps the history in
// SQLite (./data/classstats.db).
//
// On GraphDB (LINDAS) each class is counted from the index statistics — one
// query per class, 0.2 s even for millions of instances (see queries.go); other
// stores get a live COUNT per class with a timeout and a run budget.
//
// Readers: the Graph Explorer class tree (/api/class-tree), the monitoring page,
// the MCP tools and the Prometheus /metrics endpoint.
package classstats

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"hutzli.org/visoto/internal/config"
	"hutzli.org/visoto/internal/logger"
	"hutzli.org/visoto/internal/sparql"
)

// Methods recorded per count.
const (
	MethodStats   = "stats"   // GraphDB index statistics (approximate)
	MethodCount   = "count"   // live COUNT(*)
	MethodTimeout = "timeout" // timed out, or the run budget was spent
	MethodError   = "error"
)

const (
	runHour       = 3 // daily run at 03:00 local time
	staleAfter    = 24 * time.Hour
	listTimeout   = 60 * time.Second
	statsTimeout  = 20 * time.Second
	sizeTimeout   = 10 * time.Second
	classTimeout  = 60 * time.Second
	defaultBudget = 30 * time.Minute
)

// Snapshot is one endpoint's latest completed run.
type Snapshot struct {
	Day      string           // YYYY-MM-DD (UTC) of the run
	Counts   map[string]int64 // class IRI → instances (classes with a count only)
	Triples  *int64           // store size, nil when unknown
	Engine   string           // "graphdb" (statistics) or "sparql" (live counts)
	Started  time.Time
	Finished time.Time
	Classes  int // classes listed
	Failures int // classes without a count (timeout / error)
}

// Collector runs the daily counts and answers questions about them.
type Collector struct {
	cfg    *config.ApplicationConfig
	db     *sql.DB
	client *http.Client

	mu      sync.RWMutex
	latest  map[string]*Snapshot // slug → latest snapshot
	running map[string]bool

	budget   time.Duration
	stopOnce sync.Once
	stopCh   chan struct{}
	now      func() time.Time
}

// New opens (or creates) dataDir/classstats.db and loads the latest snapshots.
func New(cfg *config.ApplicationConfig, dataDir string) (*Collector, error) {
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("classstats: create data dir: %w", err)
	}
	db, err := openDB(filepath.Join(dataDir, "classstats.db"))
	if err != nil {
		return nil, fmt.Errorf("classstats: open db: %w", err)
	}
	c := newCollector(cfg, db)
	if err := c.loadLatest(); err != nil {
		db.Close()
		return nil, fmt.Errorf("classstats: load: %w", err)
	}
	return c, nil
}

func newCollector(cfg *config.ApplicationConfig, db *sql.DB) *Collector {
	return &Collector{
		cfg:     cfg,
		db:      db,
		client:  &http.Client{},
		latest:  make(map[string]*Snapshot),
		running: make(map[string]bool),
		budget:  defaultBudget,
		stopCh:  make(chan struct{}),
		now:     time.Now,
	}
}

// Enabled reports whether any endpoint collects.
func (c *Collector) Enabled() bool {
	for _, ep := range c.cfg.SparqlEndpoints {
		if ep.ClassStats {
			return true
		}
	}
	return false
}

// Start launches the scheduler: a catch-up run for every endpoint whose last
// run is older than a day, then one run per day at 03:00 local time.
func (c *Collector) Start() {
	if !c.Enabled() {
		return
	}
	go c.schedule()
}

// Stop ends the scheduler; a run in progress stops at its next class.
func (c *Collector) Stop() {
	c.stopOnce.Do(func() { close(c.stopCh) })
}

func (c *Collector) schedule() {
	c.runAll(true)
	for {
		now := c.now()
		next := time.Date(now.Year(), now.Month(), now.Day(), runHour, 0, 0, 0, now.Location())
		if !next.After(now) {
			next = next.AddDate(0, 0, 1)
		}
		select {
		case <-time.After(next.Sub(now)):
			c.runAll(false)
		case <-c.stopCh:
			return
		}
	}
}

// runAll counts every opted-in endpoint, one after the other. onlyStale skips
// endpoints counted within the last day (startup catch-up).
func (c *Collector) runAll(onlyStale bool) {
	for _, ep := range c.cfg.SparqlEndpoints {
		if !ep.ClassStats {
			continue
		}
		if onlyStale {
			if s := c.Latest(ep.Slug); s != nil && c.now().Sub(s.Finished) < staleAfter {
				continue
			}
		}
		select {
		case <-c.stopCh:
			return
		default:
		}
		if _, err := c.Run(context.Background(), ep.Slug); err != nil {
			logger.Get().Warn("classstats: run failed", slog.String("endpoint", ep.Slug), slog.String("error", err.Error()))
		}
	}
}

// Run counts one endpoint now and stores the result as today's snapshot.
func (c *Collector) Run(ctx context.Context, slug string) (*Snapshot, error) {
	ep := c.cfg.GetEndpointBySlug(slug)
	if ep == nil || !ep.ClassStats {
		return nil, fmt.Errorf("endpoint %q does not collect class stats", slug)
	}
	c.mu.Lock()
	if c.running[ep.Slug] {
		c.mu.Unlock()
		return nil, fmt.Errorf("endpoint %q: a run is already in progress", slug)
	}
	c.running[ep.Slug] = true
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		delete(c.running, ep.Slug)
		c.mu.Unlock()
	}()

	log := logger.Get()
	started := c.now()
	snap := &Snapshot{Day: started.UTC().Format("2006-01-02"), Counts: map[string]int64{}, Started: started}

	// Store size, and whether GraphDB statistics are there to use.
	if n, err := c.count(ctx, ep, queryStatsTriples, statsTimeout); err == nil && n > 0 {
		snap.Engine = "graphdb"
		snap.Triples = &n
	} else {
		snap.Engine = "sparql"
		if n, err := c.count(ctx, ep, queryTriples, sizeTimeout); err == nil {
			snap.Triples = &n
		}
	}

	rows, err := c.selectRows(ctx, ep, ClassTreeQuery, listTimeout)
	if err != nil {
		return nil, fmt.Errorf("class list: %w", err)
	}
	classes := uniqueClasses(rows)
	snap.Classes = len(classes)

	methods := make(map[string]string, len(classes))
	deadline := started.Add(c.budget)
	for _, class := range classes {
		select {
		case <-c.stopCh:
			return nil, fmt.Errorf("stopped")
		default:
		}
		if c.now().After(deadline) {
			methods[class] = MethodTimeout
			continue
		}
		n, method := c.countClass(ctx, ep, class, snap.Engine == "graphdb")
		methods[class] = method
		if method == MethodStats || method == MethodCount {
			snap.Counts[class] = n
		}
	}
	snap.Failures = len(classes) - len(snap.Counts)
	snap.Finished = c.now()

	if err := c.save(ep.Slug, snap, methods); err != nil {
		return nil, fmt.Errorf("save: %w", err)
	}
	c.mu.Lock()
	c.latest[ep.Slug] = snap
	c.mu.Unlock()

	log.Info("classstats: run done",
		slog.String("endpoint", ep.Slug),
		slog.String("engine", snap.Engine),
		slog.Int("classes", snap.Classes),
		slog.Int("failures", snap.Failures),
		slog.Duration("took", snap.Finished.Sub(started)))
	return snap, nil
}

// countClass counts one class: from the statistics when the store has them
// (a 0 there is re-checked live — the class is known to be in use), else live.
func (c *Collector) countClass(ctx context.Context, ep *config.SparqlEndpoint, class string, stats bool) (int64, string) {
	term, err := sparql.IRITerm(class)
	if err != nil {
		return 0, MethodError
	}
	if stats {
		if n, err := c.count(ctx, ep, fmt.Sprintf(queryStatsClass, term), statsTimeout); err == nil && n > 0 {
			return n, MethodStats
		}
	}
	n, err := c.count(ctx, ep, fmt.Sprintf(queryClass, term), classTimeout)
	switch {
	case err == nil:
		return n, MethodCount
	case isTimeout(err):
		return 0, MethodTimeout
	default:
		return 0, MethodError
	}
}

func uniqueClasses(rows []map[string]string) []string {
	seen := map[string]bool{}
	var out []string
	for _, r := range rows {
		if cl := r["class"]; cl != "" && !seen[cl] {
			seen[cl] = true
			out = append(out, cl)
		}
	}
	return out
}

// --- SPARQL transport --------------------------------------------------------
// A small client of its own rather than sparql.Preprocessor: the collector
// needs the endpoint's credentials (ApplyAuth — the Docker-private QLever) and
// a per-query timeout, and none of the preprocessor's label/prefix handling.

func (c *Collector) count(ctx context.Context, ep *config.SparqlEndpoint, query string, timeout time.Duration) (int64, error) {
	rows, err := c.selectRows(ctx, ep, query, timeout)
	if err != nil {
		return 0, err
	}
	if len(rows) == 0 || rows[0]["n"] == "" {
		return 0, nil
	}
	return strconv.ParseInt(rows[0]["n"], 10, 64)
}

// selectRows runs a SELECT against a configured endpoint and returns each
// binding's values by variable name (language tags and datatypes dropped).
func (c *Collector) selectRows(ctx context.Context, ep *config.SparqlEndpoint, query string, timeout time.Duration) ([]map[string]string, error) {
	res, err := c.selectJSON(ctx, ep, query, timeout)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]string, 0, len(res.Results.Bindings))
	for _, b := range res.Results.Bindings {
		row := make(map[string]string, len(b))
		for k, v := range b {
			row[k] = v.Value
		}
		out = append(out, row)
	}
	return out, nil
}

// Term is one RDF term of a SPARQL JSON result.
type Term struct {
	Type     string `json:"type"`
	Value    string `json:"value"`
	Lang     string `json:"xml:lang,omitempty"`
	Datatype string `json:"datatype,omitempty"`
}

// Results is a SPARQL 1.1 JSON result document.
type Results struct {
	Head struct {
		Vars []string `json:"vars"`
	} `json:"head"`
	Results struct {
		Bindings []map[string]Term `json:"bindings"`
	} `json:"results"`
}

func (c *Collector) selectJSON(ctx context.Context, ep *config.SparqlEndpoint, query string, timeout time.Duration) (*Results, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ep.URL, strings.NewReader(query))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/sparql-query")
	req.Header.Set("Accept", "application/sparql-results+json")
	ep.ApplyAuth(req)
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		io.Copy(io.Discard, resp.Body)
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	var res Results
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	return &res, nil
}

// ClassTree runs ClassTreeQuery against the endpoint (for /api/class-tree).
func (c *Collector) ClassTree(ctx context.Context, ep *config.SparqlEndpoint) (*Results, error) {
	return c.selectJSON(ctx, ep, ClassTreeQuery, listTimeout)
}

func isTimeout(err error) bool {
	if err == context.DeadlineExceeded {
		return true
	}
	type timeout interface{ Timeout() bool }
	for e := err; e != nil; {
		if t, ok := e.(timeout); ok && t.Timeout() {
			return true
		}
		u, ok := e.(interface{ Unwrap() error })
		if !ok {
			break
		}
		e = u.Unwrap()
	}
	return strings.Contains(err.Error(), "deadline exceeded")
}
