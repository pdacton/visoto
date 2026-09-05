// Package state is the pipeline's job store: the harvest watermark per source,
// the run history, and one row per distribution carrying the stage it has
// reached and the validators needed to skip work on the next run.
//
// It is deliberately SQLite and not the triplestore. Job state is high-churn,
// single-writer, and of no interest to a SPARQL consumer; putting it in the graph
// would mean rewriting nodes thousands of times per run for no query value
// (R-ARCH-2).
package state

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// Stage is the pipeline stage a distribution has reached.
type Stage string

const (
	StageDiscovered Stage = "discovered"
	StageFetched    Stage = "fetched"
	StageSniffed    Stage = "sniffed"
	StageProfiled   Stage = "profiled"
	StageLoaded     Stage = "loaded"
	StageSkipped    Stage = "skipped"
	StageFailed     Stage = "failed"
)

// RunStatus is the outcome of one harvest run.
type RunStatus string

const (
	RunRunning   RunStatus = "running"
	RunSucceeded RunStatus = "succeeded"
	RunFailed    RunStatus = "failed"
)

// Store is the SQLite-backed job store.
type Store struct {
	db   *sql.DB
	path string
}

// Open opens or creates the store at path, creating parent directories.
func Open(path string) (*Store, error) {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("create state dir %s: %w", dir, err)
		}
	}
	// A busy timeout matters even single-writer: the CLI and a scheduled run can
	// overlap, and failing a whole harvest on a momentary lock would be absurd.
	db, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(10000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)")
	if err != nil {
		return nil, fmt.Errorf("open state db %s: %w", path, err)
	}
	s := &Store{db: db, path: path}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

// Close releases the database.
func (s *Store) Close() error { return s.db.Close() }

// Path returns the file the store lives in, for logging.
func (s *Store) Path() string { return s.path }

func (s *Store) migrate() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS sources (
			name          TEXT PRIMARY KEY,
			watermark     INTEGER NOT NULL DEFAULT 0,
			last_run_id   TEXT,
			last_run_at   INTEGER
		);

		CREATE TABLE IF NOT EXISTS runs (
			id                 TEXT PRIMARY KEY,
			source             TEXT    NOT NULL,
			started_at         INTEGER NOT NULL,
			ended_at           INTEGER,
			status             TEXT    NOT NULL,
			datasets_seen      INTEGER NOT NULL DEFAULT 0,
			distributions_seen INTEGER NOT NULL DEFAULT 0,
			quads_written      INTEGER NOT NULL DEFAULT 0,
			message            TEXT
		);
		CREATE INDEX IF NOT EXISTS idx_runs_source ON runs(source, started_at DESC);

		CREATE TABLE IF NOT EXISTS distributions (
			iri             TEXT PRIMARY KEY,
			dataset_iri     TEXT NOT NULL,
			source          TEXT NOT NULL,
			download_url    TEXT,
			declared_media  TEXT,
			declared_format TEXT,
			byte_size       INTEGER NOT NULL DEFAULT 0,
			licence         TEXT,
			modified        INTEGER NOT NULL DEFAULT 0,
			stage           TEXT NOT NULL,
			etag            TEXT,
			last_modified   TEXT,
			content_hash    TEXT,
			detected_media  TEXT,
			structure_hash  TEXT,
			attempts        INTEGER NOT NULL DEFAULT 0,
			last_error      TEXT,
			first_seen      INTEGER NOT NULL,
			last_seen       INTEGER NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_dist_stage  ON distributions(source, stage);
		CREATE INDEX IF NOT EXISTS idx_dist_datset ON distributions(dataset_iri);
	`)
	if err != nil {
		return fmt.Errorf("migrate state db: %w", err)
	}
	return nil
}

// Watermark returns the dct:modified high-water mark for a source. A zero time
// means the source has never completed a run, so the next one is full.
func (s *Store) Watermark(name string) (time.Time, error) {
	var unix int64
	err := s.db.QueryRow(`SELECT watermark FROM sources WHERE name = ?`, name).Scan(&unix)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return time.Time{}, nil
	case err != nil:
		return time.Time{}, fmt.Errorf("read watermark for %s: %w", name, err)
	case unix <= 0:
		return time.Time{}, nil
	}
	return time.Unix(unix, 0).UTC(), nil
}

// SetWatermark advances a source's watermark. It never moves backwards, so a
// partial re-harvest of older data cannot cause the next run to re-read the
// whole catalogue.
func (s *Store) SetWatermark(name string, t time.Time) error {
	if t.IsZero() {
		return nil
	}
	_, err := s.db.Exec(`
		INSERT INTO sources (name, watermark) VALUES (?, ?)
		ON CONFLICT(name) DO UPDATE SET watermark = MAX(watermark, excluded.watermark)`,
		name, t.UTC().Unix())
	if err != nil {
		return fmt.Errorf("set watermark for %s: %w", name, err)
	}
	return nil
}

// ResetWatermark clears a source's watermark, forcing the next run to be full.
func (s *Store) ResetWatermark(name string) error {
	_, err := s.db.Exec(`UPDATE sources SET watermark = 0 WHERE name = ?`, name)
	return err
}

// Run is one execution of the pipeline over one source.
type Run struct {
	ID                string
	Source            string
	StartedAt         time.Time
	EndedAt           time.Time
	Status            RunStatus
	DatasetsSeen      int64
	DistributionsSeen int64
	QuadsWritten      int64
	Message           string
}

// StartRun records the beginning of a run and returns it. The ID is derived from
// the start time, so run IRIs sort chronologically and are readable in a graph
// name.
func (s *Store) StartRun(sourceName string, now time.Time) (*Run, error) {
	run := &Run{
		ID:        now.UTC().Format("20060102T150405Z"),
		Source:    sourceName,
		StartedAt: now.UTC(),
		Status:    RunRunning,
	}
	_, err := s.db.Exec(`
		INSERT INTO runs (id, source, started_at, status) VALUES (?, ?, ?, ?)`,
		run.ID, run.Source, run.StartedAt.Unix(), string(run.Status))
	if err != nil {
		return nil, fmt.Errorf("start run for %s: %w", sourceName, err)
	}
	return run, nil
}

// FinishRun records a run's outcome and counters.
func (s *Store) FinishRun(run *Run, status RunStatus, message string, endedAt time.Time) error {
	run.Status = status
	run.Message = message
	run.EndedAt = endedAt.UTC()
	_, err := s.db.Exec(`
		UPDATE runs
		   SET ended_at = ?, status = ?, message = ?,
		       datasets_seen = ?, distributions_seen = ?, quads_written = ?
		 WHERE id = ?`,
		run.EndedAt.Unix(), string(status), message,
		run.DatasetsSeen, run.DistributionsSeen, run.QuadsWritten, run.ID)
	if err != nil {
		return fmt.Errorf("finish run %s: %w", run.ID, err)
	}
	if status == RunSucceeded {
		_, err = s.db.Exec(`
			INSERT INTO sources (name, last_run_id, last_run_at) VALUES (?, ?, ?)
			ON CONFLICT(name) DO UPDATE SET last_run_id = excluded.last_run_id,
			                                last_run_at = excluded.last_run_at`,
			run.Source, run.ID, run.EndedAt.Unix())
		if err != nil {
			return fmt.Errorf("record last run for %s: %w", run.Source, err)
		}
	}
	return nil
}

// RecentRuns returns the most recent runs, newest first, across all sources or
// for one source when name is non-empty.
func (s *Store) RecentRuns(name string, limit int) ([]Run, error) {
	query := `SELECT id, source, started_at, COALESCE(ended_at, 0), status,
	                 datasets_seen, distributions_seen, quads_written, COALESCE(message, '')
	            FROM runs`
	args := []any{}
	if name != "" {
		query += ` WHERE source = ?`
		args = append(args, name)
	}
	query += ` ORDER BY started_at DESC LIMIT ?`
	args = append(args, limit)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list runs: %w", err)
	}
	defer rows.Close()

	var out []Run
	for rows.Next() {
		var r Run
		var started, ended int64
		var status string
		if err := rows.Scan(&r.ID, &r.Source, &started, &ended, &status,
			&r.DatasetsSeen, &r.DistributionsSeen, &r.QuadsWritten, &r.Message); err != nil {
			return nil, err
		}
		r.StartedAt = time.Unix(started, 0).UTC()
		if ended > 0 {
			r.EndedAt = time.Unix(ended, 0).UTC()
		}
		r.Status = RunStatus(status)
		out = append(out, r)
	}
	return out, rows.Err()
}
