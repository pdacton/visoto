// Package monitor provides periodic SPARQL endpoint health monitoring.
// It probes each configured endpoint every 5 minutes, stores results in SQLite,
// and retains data for 30 days. The monitor can be toggled on/off at runtime.
package monitor

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	_ "modernc.org/sqlite"

	"hutzli.org/visoto/internal/config"
	"hutzli.org/visoto/internal/logger"
)

const (
	probeInterval   = 5 * time.Minute
	retentionPeriod = 30 * 24 * time.Hour
	cleanupInterval = 6 * time.Hour
	probeQuery      = "ASK {}"
	stateFileName   = "monitoring_enabled.json"
)

// Metric is a single probe result.
type Metric struct {
	Endpoint   string    `json:"endpoint"`
	MeasuredAt time.Time `json:"measured_at"`
	DurationMs int64     `json:"duration_ms"` // -1 on error
	Status     string    `json:"status"`       // "ok" or error message
}

// EndpointStatus holds the most recent metric for one endpoint.
type EndpointStatus struct {
	Name   string  `json:"name"`
	URL    string  `json:"url"`
	Metric *Metric `json:"last,omitempty"`
}

// stateFile is the JSON structure persisted to disk.
type stateFile struct {
	Enabled bool `json:"enabled"`
}

// Monitor probes SPARQL endpoints and stores time-series in SQLite.
type Monitor struct {
	cfg      *config.ApplicationConfig
	db       *sql.DB
	dataDir  string
	enabled  atomic.Bool
	mu       sync.RWMutex
	latest   map[string]*Metric // endpoint URL → latest result
	stopOnce sync.Once
	stopCh   chan struct{}
}

// New creates a Monitor. Call Start() to begin probing.
func New(cfg *config.ApplicationConfig, dataDir string) (*Monitor, error) {
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("monitor: create data dir: %w", err)
	}

	db, err := openDB(filepath.Join(dataDir, "monitoring.db"))
	if err != nil {
		return nil, fmt.Errorf("monitor: open db: %w", err)
	}

	m := &Monitor{
		cfg:     cfg,
		db:      db,
		dataDir: dataDir,
		latest:  make(map[string]*Metric),
		stopCh:  make(chan struct{}),
	}

	// Load persisted enabled state (default: false / disabled)
	m.enabled.Store(m.loadState())

	return m, nil
}

// Start launches the background goroutines. Safe to call once.
func (m *Monitor) Start() {
	go m.runProber()
	go m.runCleanup()
}

// Stop gracefully stops background goroutines.
func (m *Monitor) Stop() {
	m.stopOnce.Do(func() { close(m.stopCh) })
}

// IsEnabled reports whether monitoring is currently active.
func (m *Monitor) IsEnabled() bool {
	return m.enabled.Load()
}

// SetEnabled toggles monitoring on or off and persists the choice.
func (m *Monitor) SetEnabled(v bool) {
	m.enabled.Store(v)
	m.saveState(v)
}

// LatestStatus returns the most-recent probe result for every configured endpoint.
func (m *Monitor) LatestStatus() []EndpointStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	out := make([]EndpointStatus, 0, len(m.cfg.SparqlEndpoints))
	for _, ep := range m.cfg.SparqlEndpoints {
		es := EndpointStatus{Name: ep.Name, URL: ep.URL}
		if metric, ok := m.latest[ep.URL]; ok {
			copy := *metric
			es.Metric = &copy
		}
		out = append(out, es)
	}
	return out
}

// QuerySeries returns time-series data for all endpoints within the given duration.
func (m *Monitor) QuerySeries(since time.Duration) (map[string][]Metric, error) {
	cutoff := time.Now().Add(-since).Unix()
	rows, err := m.db.Query(
		`SELECT endpoint, measured_at, duration_ms, status
		 FROM endpoint_metrics
		 WHERE measured_at >= ?
		 ORDER BY endpoint, measured_at`,
		cutoff,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string][]Metric)
	for rows.Next() {
		var ep string
		var ts, dur int64
		var status string
		if err := rows.Scan(&ep, &ts, &dur, &status); err != nil {
			return nil, err
		}
		result[ep] = append(result[ep], Metric{
			Endpoint:   ep,
			MeasuredAt: time.Unix(ts, 0),
			DurationMs: dur,
			Status:     status,
		})
	}
	return result, rows.Err()
}

// ----- internal helpers -----

func (m *Monitor) runProber() {
	log := logger.Get()
	// Probe immediately on first enable, then every 5 minutes.
	ticker := time.NewTicker(probeInterval)
	defer ticker.Stop()

	// Probe once at startup if enabled
	if m.IsEnabled() {
		m.probe()
	}

	for {
		select {
		case <-ticker.C:
			if m.IsEnabled() {
				m.probe()
			}
		case <-m.stopCh:
			log.Info("monitor: prober stopped")
			return
		}
	}
}

func (m *Monitor) runCleanup() {
	log := logger.Get()
	ticker := time.NewTicker(cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			cutoff := time.Now().Add(-retentionPeriod).Unix()
			res, err := m.db.Exec(`DELETE FROM endpoint_metrics WHERE measured_at < ?`, cutoff)
			if err != nil {
				log.Warn("monitor: cleanup failed", slog.String("error", err.Error()))
			} else {
				n, _ := res.RowsAffected()
				if n > 0 {
					log.Info("monitor: cleaned up old metrics", slog.Int64("deleted", n))
				}
			}
		case <-m.stopCh:
			return
		}
	}
}

func (m *Monitor) probe() {
	log := logger.Get()
	for _, ep := range m.cfg.SparqlEndpoints {
		metric := m.probeOne(ep)
		log.Info("monitor: probe",
			slog.String("endpoint", ep.Name),
			slog.Int64("duration_ms", metric.DurationMs),
			slog.String("status", metric.Status))

		_, err := m.db.Exec(
			`INSERT INTO endpoint_metrics (endpoint, measured_at, duration_ms, status) VALUES (?,?,?,?)`,
			ep.URL, metric.MeasuredAt.Unix(), metric.DurationMs, metric.Status,
		)
		if err != nil {
			log.Warn("monitor: db insert failed",
				slog.String("endpoint", ep.Name),
				slog.String("error", err.Error()))
		}

		m.mu.Lock()
		m.latest[ep.URL] = metric
		m.mu.Unlock()
	}
}

func (m *Monitor) probeOne(ep config.SparqlEndpoint) *Metric {
	client := &http.Client{Timeout: 15 * time.Second}
	start := time.Now()

	req, err := http.NewRequest(http.MethodPost, ep.URL, strings.NewReader(probeQuery))
	if err != nil {
		return &Metric{Endpoint: ep.URL, MeasuredAt: start, DurationMs: -1, Status: err.Error()}
	}
	req.Header.Set("Content-Type", "application/sparql-query")
	req.Header.Set("Accept", "application/sparql-results+json")

	resp, err := client.Do(req)
	elapsed := time.Since(start).Milliseconds()
	if err != nil {
		return &Metric{Endpoint: ep.URL, MeasuredAt: start, DurationMs: -1, Status: err.Error()}
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body) // drain body

	if resp.StatusCode >= 400 {
		return &Metric{Endpoint: ep.URL, MeasuredAt: start, DurationMs: elapsed,
			Status: fmt.Sprintf("HTTP %d", resp.StatusCode)}
	}
	return &Metric{Endpoint: ep.URL, MeasuredAt: start, DurationMs: elapsed, Status: "ok"}
}

func openDB(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS endpoint_metrics (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			endpoint    TEXT    NOT NULL,
			measured_at INTEGER NOT NULL,
			duration_ms INTEGER NOT NULL,
			status      TEXT    NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_endpoint_time
			ON endpoint_metrics(endpoint, measured_at);
	`)
	if err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func (m *Monitor) loadState() bool {
	data, err := os.ReadFile(filepath.Join(m.dataDir, stateFileName))
	if err != nil {
		return false // default: disabled
	}
	var s stateFile
	if err := json.Unmarshal(data, &s); err != nil {
		return false
	}
	return s.Enabled
}

func (m *Monitor) saveState(v bool) {
	data, _ := json.Marshal(stateFile{Enabled: v})
	path := filepath.Join(m.dataDir, stateFileName)
	if err := os.WriteFile(path, data, 0644); err != nil {
		log := logger.Get()
		log.Warn("monitor: failed to save state", slog.String("error", err.Error()))
	}
}
