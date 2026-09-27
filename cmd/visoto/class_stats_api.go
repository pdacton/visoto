package main

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"hutzli.org/visoto/internal/classstats"
)

// classStats is the daily class-instance collector (nil when it failed to open).
var classStats *classstats.Collector

// --- /api/class-tree ----------------------------------------------------------

// classTreeCacheTTL bounds how long one endpoint's class tree is reused in
// memory; a new snapshot day replaces it sooner.
const classTreeCacheTTL = 6 * time.Hour

type classTreeEntry struct {
	day  string
	at   time.Time
	body []byte
}

var (
	classTreeMu    sync.Mutex
	classTreeCache = map[string]classTreeEntry{}
)

// classTreeHandler serves GET /api/class-tree?endpoint=<slug>: the Graph
// Explorer class tree as SPARQL JSON (?class ?label ?parent ?instcount). The
// classes come live from classstats.ClassTreeQuery (0.4 s on LINDAS, where GE's
// own counting query times out); ?instcount comes from the latest daily
// snapshot of the endpoint or its class_stats_from source, and is absent before
// the first run. Pure function of the URL, like every /api route.
//
// Endpoints without class statistics get 404, and Graph Explorer then keeps its
// own class-tree query (static/js/ge-adapter.js), which on a small store also
// finds classes that are used without being declared.
func classTreeHandler(c *gin.Context) {
	ep := activeEndpoint(c)
	if ep == nil || classStats == nil || ep.ClassStatsSlug() == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "no class tree for this endpoint"})
		return
	}
	snap, _ := classStats.For(ep.Slug)
	day := ""
	if snap != nil {
		day = snap.Day
	}

	classTreeMu.Lock()
	e, ok := classTreeCache[ep.Slug]
	classTreeMu.Unlock()
	if !ok || e.day != day || time.Since(e.at) > classTreeCacheTTL {
		res, err := classStats.ClassTree(c.Request.Context(), ep)
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
			return
		}
		body, err := json.Marshal(withInstanceCounts(res, snap))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		e = classTreeEntry{day: day, at: time.Now(), body: body}
		classTreeMu.Lock()
		classTreeCache[ep.Slug] = e
		classTreeMu.Unlock()
	}
	markCacheable(c)
	c.Data(http.StatusOK, "application/sparql-results+json", e.body)
}

// withInstanceCounts adds ?instcount to every row whose class the snapshot
// counted.
func withInstanceCounts(res *classstats.Results, snap *classstats.Snapshot) *classstats.Results {
	if snap == nil {
		return res
	}
	res.Head.Vars = append(res.Head.Vars, "instcount")
	for _, b := range res.Results.Bindings {
		if n, ok := snap.Counts[b["class"].Value]; ok {
			b["instcount"] = classstats.Term{
				Type:     "literal",
				Value:    strconv.FormatInt(n, 10),
				Datatype: "http://www.w3.org/2001/XMLSchema#integer",
			}
		}
	}
	return res
}

// --- monitoring page ----------------------------------------------------------

// classStatsEndpoint is one row of GET /api/class-stats/status.
type classStatsEndpoint struct {
	Slug     string `json:"slug"`
	Name     string `json:"name"`
	Day      string `json:"day,omitempty"`
	Engine   string `json:"engine,omitempty"`
	Triples  *int64 `json:"triples,omitempty"`
	Classes  int    `json:"classes"`
	Failures int    `json:"failures"`
	Finished string `json:"finished,omitempty"`
	Seconds  int    `json:"seconds"`
}

// classStatsStatusHandler serves GET /api/class-stats/status: every collecting
// endpoint with its latest run. Not URL-pure (no endpoint param), so it takes
// the page tier's revalidating cache policy, like /api/monitoring/status.
func classStatsStatusHandler(c *gin.Context) {
	out := []classStatsEndpoint{}
	for i := range cfg.Application.SparqlEndpoints {
		ep := &cfg.Application.SparqlEndpoints[i]
		if !ep.ClassStats {
			continue
		}
		row := classStatsEndpoint{Slug: ep.Slug, Name: ep.Name}
		if classStats != nil {
			if s := classStats.Latest(ep.Slug); s != nil {
				row.Day, row.Engine, row.Triples = s.Day, s.Engine, s.Triples
				row.Classes, row.Failures = s.Classes, s.Failures
				row.Finished = s.Finished.UTC().Format(time.RFC3339)
				row.Seconds = int(s.Finished.Sub(s.Started).Seconds())
			}
		}
		out = append(out, row)
	}
	c.JSON(http.StatusOK, out)
}

// classStatsSource resolves ?endpoint= to the slug whose snapshots serve it.
func classStatsSource(c *gin.Context) (string, bool) {
	if classStats == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "class statistics not available"})
		return "", false
	}
	_, src := classStats.For(c.Query("endpoint"))
	if src == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "no class statistics for this endpoint"})
		return "", false
	}
	return src, true
}

func queryDays(c *gin.Context, def int) int {
	days, err := strconv.Atoi(c.Query("days"))
	if err != nil || days < 1 || days > 3650 {
		return def
	}
	return days
}

// classStatsChangesHandler serves GET /api/class-stats/changes?endpoint=&days=.
func classStatsChangesHandler(c *gin.Context) {
	src, ok := classStatsSource(c)
	if !ok {
		return
	}
	ch, err := classStats.Changes(src, queryDays(c, 1))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if ch == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "no snapshot yet"})
		return
	}
	c.JSON(http.StatusOK, ch)
}

// classStatsSeriesHandler serves GET /api/class-stats/series?endpoint=&class=&days=.
func classStatsSeriesHandler(c *gin.Context) {
	src, ok := classStatsSource(c)
	if !ok {
		return
	}
	class := c.Query("class")
	if class == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing class"})
		return
	}
	pts, err := classStats.Series(src, class, queryDays(c, 365))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, pts)
}

// --- /metrics -----------------------------------------------------------------

// metricsHandler serves GET /metrics in the Prometheus text format, for a
// Grafana Cloud "Metrics Endpoint" scrape job. It needs
// "Authorization: Bearer <metrics_token>"; without a configured token the route
// answers 404, so it is never open by accident. no-store keeps Souin out.
func metricsHandler(token string, cs ...prometheus.Collector) gin.HandlerFunc {
	reg := prometheus.NewRegistry()
	reg.MustRegister(collectors.NewGoCollector(), collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	for _, c := range cs {
		reg.MustRegister(c)
	}
	h := promhttp.HandlerFor(reg, promhttp.HandlerOpts{})
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		if token == "" {
			c.Status(http.StatusNotFound)
			return
		}
		got, found := strings.CutPrefix(c.GetHeader("Authorization"), "Bearer ")
		if !found || subtle.ConstantTimeCompare([]byte(got), []byte(token)) != 1 {
			c.Header("WWW-Authenticate", `Bearer realm="metrics"`)
			c.Status(http.StatusUnauthorized)
			return
		}
		h.ServeHTTP(c.Writer, c.Request)
	}
}
