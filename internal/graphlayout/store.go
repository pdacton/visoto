// Package graphlayout stores Graph Explorer layouts shared between users: the
// canvases graph-kit.js snapshots (static/js/graph-store.js, format
// "visoto-graph" v1), kept server-side so a page shows the same arrangement on
// every machine.
//
// Snapshots are immutable and append-only. A page's default is a pointer to
// one of them, so moving it back is a revert, and a share link (?layout=<id>)
// keeps showing what was shared. History is bounded: each page keeps its
// newest config.GraphLayoutsConfig.Keep() snapshots plus its default, and the
// whole table stops accepting rows at RowLimit().
package graphlayout

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"time"

	_ "modernc.org/sqlite"

	"hutzli.org/visoto/internal/config"
)

const (
	format  = "visoto-graph"
	version = 1
)

var (
	ErrNotFound = errors.New("layout not found")
	ErrFull     = errors.New("layout storage full")
	ErrInvalid  = errors.New("not a visoto-graph v1 layout")
)

// Key identifies one graph on one page of one endpoint: the server-side twin
// of graph-store.js's autosave key.
type Key struct {
	Endpoint string `json:"endpoint"` // slug
	Page     string `json:"page"`     // CanonicalPage form
	Graph    string `json:"graph"`    // graph id on the page
}

// Summary is one snapshot as the history list shows it (no diagram).
type Summary struct {
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"createdAt"`
	Nodes     int       `json:"nodes"`
	Layout    string    `json:"layout,omitempty"`
}

// Layout is one stored snapshot.
type Layout struct {
	Summary
	Key
	Snapshot json.RawMessage `json:"snapshot"`
}

// Store is the SQLite-backed layout store.
type Store struct {
	db    *sql.DB
	cfg   config.GraphLayoutsConfig
	now   func() time.Time
	newID func() string
}

// New opens (or creates) dataDir/graph_layouts.db.
func New(cfg config.GraphLayoutsConfig, dataDir string) (*Store, error) {
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("graphlayout: create data dir: %w", err)
	}
	db, err := openDB(filepath.Join(dataDir, "graph_layouts.db"))
	if err != nil {
		return nil, fmt.Errorf("graphlayout: open db: %w", err)
	}
	return newStore(cfg, db), nil
}

func newStore(cfg config.GraphLayoutsConfig, db *sql.DB) *Store {
	return &Store{db: db, cfg: cfg, now: time.Now, newID: randomID}
}

func openDB(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	// One connection serialises the share transaction (insert, point, prune)
	// instead of failing concurrent writers with SQLITE_BUSY.
	db.SetMaxOpenConns(1)
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS layouts (
			id         TEXT    PRIMARY KEY,
			endpoint   TEXT    NOT NULL,
			page       TEXT    NOT NULL,
			graph      TEXT    NOT NULL,
			hash       TEXT    NOT NULL,
			snapshot   TEXT    NOT NULL,
			nodes      INTEGER NOT NULL,
			layout     TEXT    NOT NULL,
			created_at INTEGER NOT NULL,
			author     TEXT,
			UNIQUE (endpoint, page, graph, hash)
		);
		CREATE INDEX IF NOT EXISTS idx_layouts_page ON layouts(endpoint, page, graph, created_at);
		CREATE TABLE IF NOT EXISTS defaults (
			endpoint   TEXT    NOT NULL,
			page       TEXT    NOT NULL,
			graph      TEXT    NOT NULL,
			layout_id  TEXT    NOT NULL,
			updated_at INTEGER NOT NULL,
			PRIMARY KEY (endpoint, page, graph)
		);
	`)
	if err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

// Close closes the database.
func (s *Store) Close() error { return s.db.Close() }

// Writable reports whether the config lets layouts be shared.
func (s *Store) Writable() bool { return s.cfg.Writable() }

// BodyLimit is the largest request body a write accepts.
func (s *Store) BodyLimit() int { return s.cfg.BodyLimit() }

// List returns the page's snapshots, newest first, and its default id ("" for
// none).
func (s *Store) List(k Key) ([]Summary, string, error) {
	rows, err := s.db.Query(`
		SELECT id, created_at, nodes, layout FROM layouts
		WHERE endpoint = ? AND page = ? AND graph = ?
		ORDER BY created_at DESC, rowid DESC`, k.Endpoint, k.Page, k.Graph)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	out := []Summary{}
	for rows.Next() {
		var sum Summary
		var created int64
		if err := rows.Scan(&sum.ID, &created, &sum.Nodes, &sum.Layout); err != nil {
			return nil, "", err
		}
		sum.CreatedAt = time.UnixMilli(created).UTC()
		out = append(out, sum)
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}
	var def string
	err = s.db.QueryRow(`SELECT layout_id FROM defaults WHERE endpoint = ? AND page = ? AND graph = ?`,
		k.Endpoint, k.Page, k.Graph).Scan(&def)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, "", err
	}
	return out, def, nil
}

// Get returns one snapshot by id.
func (s *Store) Get(id string) (*Layout, error) {
	var l Layout
	var snap string
	var created int64
	err := s.db.QueryRow(`
		SELECT id, endpoint, page, graph, snapshot, nodes, layout, created_at
		FROM layouts WHERE id = ?`, id).
		Scan(&l.ID, &l.Endpoint, &l.Page, &l.Graph, &snap, &l.Nodes, &l.Layout, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	l.Snapshot = json.RawMessage(snap)
	l.CreatedAt = time.UnixMilli(created).UTC()
	return &l, nil
}

// Share stores snapshot for k, makes it the page default and prunes the
// page's history. Sharing a canvas identical to a kept one reuses that row
// (moved to the top) instead of adding another.
func (s *Store) Share(k Key, snapshot []byte) (string, error) {
	meta, hash, err := inspect(snapshot)
	if err != nil {
		return "", err
	}
	now := s.now().UnixMilli()

	tx, err := s.db.Begin()
	if err != nil {
		return "", err
	}
	defer tx.Rollback()

	var id string
	err = tx.QueryRow(`SELECT id FROM layouts WHERE endpoint = ? AND page = ? AND graph = ? AND hash = ?`,
		k.Endpoint, k.Page, k.Graph, hash).Scan(&id)
	switch {
	case err == nil:
		if _, err := tx.Exec(`UPDATE layouts SET created_at = ? WHERE id = ?`, now, id); err != nil {
			return "", err
		}
	case errors.Is(err, sql.ErrNoRows):
		var total int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM layouts`).Scan(&total); err != nil {
			return "", err
		}
		if total >= s.cfg.RowLimit() {
			return "", ErrFull
		}
		id = s.newID()
		if _, err := tx.Exec(`
			INSERT INTO layouts (id, endpoint, page, graph, hash, snapshot, nodes, layout, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			id, k.Endpoint, k.Page, k.Graph, hash, string(snapshot), meta.nodes, meta.layout, now); err != nil {
			return "", err
		}
	default:
		return "", err
	}

	if err := setDefault(tx, k, id, now); err != nil {
		return "", err
	}
	if _, err := tx.Exec(`
		DELETE FROM layouts
		WHERE endpoint = ?1 AND page = ?2 AND graph = ?3
		  AND id NOT IN (SELECT layout_id FROM defaults WHERE endpoint = ?1 AND page = ?2 AND graph = ?3)
		  AND id NOT IN (SELECT id FROM layouts WHERE endpoint = ?1 AND page = ?2 AND graph = ?3
		                 ORDER BY created_at DESC, rowid DESC LIMIT ?4)`,
		k.Endpoint, k.Page, k.Graph, s.cfg.Keep()); err != nil {
		return "", err
	}
	return id, tx.Commit()
}

// SetDefault points k's default at id, which must be one of k's snapshots.
func (s *Store) SetDefault(k Key, id string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var n int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM layouts WHERE id = ? AND endpoint = ? AND page = ? AND graph = ?`,
		id, k.Endpoint, k.Page, k.Graph).Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	if err := setDefault(tx, k, id, s.now().UnixMilli()); err != nil {
		return err
	}
	return tx.Commit()
}

func setDefault(tx *sql.Tx, k Key, id string, now int64) error {
	_, err := tx.Exec(`
		INSERT INTO defaults (endpoint, page, graph, layout_id, updated_at) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (endpoint, page, graph) DO UPDATE SET layout_id = excluded.layout_id, updated_at = excluded.updated_at`,
		k.Endpoint, k.Page, k.Graph, id, now)
	return err
}

type snapshotMeta struct {
	nodes  int
	layout string
}

// inspect checks that snapshot is a visoto-graph v1 canvas and returns its
// list summary and a content hash. The hash leaves out savedAt, which changes
// on every share of an otherwise identical canvas.
func inspect(snapshot []byte) (snapshotMeta, string, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(snapshot, &fields); err != nil {
		return snapshotMeta{}, "", ErrInvalid
	}
	var head struct {
		Format  string `json:"format"`
		Version int    `json:"version"`
		Layout  string `json:"layout"`
		Diagram struct {
			LayoutData *struct {
				Elements []json.RawMessage `json:"elements"`
			} `json:"layoutData"`
		} `json:"diagram"`
	}
	if err := json.Unmarshal(snapshot, &head); err != nil ||
		head.Format != format || head.Version != version || head.Diagram.LayoutData == nil {
		return snapshotMeta{}, "", ErrInvalid
	}
	delete(fields, "savedAt")
	canonical, err := json.Marshal(fields) // map keys marshal sorted
	if err != nil {
		return snapshotMeta{}, "", err
	}
	sum := sha256.Sum256(canonical)
	return snapshotMeta{nodes: len(head.Diagram.LayoutData.Elements), layout: head.Layout},
		hex.EncodeToString(sum[:]), nil
}

var graphID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// ValidGraphID reports whether id looks like a graph id on a page.
func ValidGraphID(id string) bool { return graphID.MatchString(id) }

// ValidID reports whether id looks like a layout id.
func ValidID(id string) bool { return layoutID.MatchString(id) }

var layoutID = regexp.MustCompile(`^[0-9A-Za-z]{10}$`)

// CanonicalPage reduces a page URL (path plus query) to what identifies the
// page's content: the path and its ?iri=, if any. The endpoint is part of the
// Key already, and anything else (?layout=, tracking or UI params) must not
// fork a page's history.
func CanonicalPage(raw string) (string, bool) {
	if len(raw) > 4096 {
		return "", false
	}
	u, err := url.Parse(raw)
	if err != nil || u.Path == "" || u.Path[0] != '/' || u.Host != "" || u.Scheme != "" {
		return "", false
	}
	page := u.Path
	if iri := u.Query().Get("iri"); iri != "" {
		page += "?" + url.Values{"iri": {iri}}.Encode()
	}
	return page, true
}

const idAlphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

func randomID() string {
	b := make([]byte, 10)
	max := big.NewInt(int64(len(idAlphabet)))
	for i := range b {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			panic(err) // crypto/rand never fails on supported platforms
		}
		b[i] = idAlphabet[n.Int64()]
	}
	return string(b)
}
