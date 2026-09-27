package classstats

import (
	"database/sql"
	"sort"
	"time"

	_ "modernc.org/sqlite"
)

// Kept indefinitely: ~600 rows per endpoint and day.
func openDB(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS class_counts (
			endpoint TEXT NOT NULL,
			day      TEXT NOT NULL,
			class    TEXT NOT NULL,
			count    INTEGER,
			method   TEXT NOT NULL,
			PRIMARY KEY (endpoint, day, class)
		);
		CREATE INDEX IF NOT EXISTS idx_class_counts_class ON class_counts(endpoint, class, day);
		CREATE TABLE IF NOT EXISTS runs (
			endpoint    TEXT    NOT NULL,
			day         TEXT    NOT NULL,
			engine      TEXT    NOT NULL,
			triples     INTEGER,
			started_at  INTEGER NOT NULL,
			finished_at INTEGER NOT NULL,
			classes     INTEGER NOT NULL,
			failures    INTEGER NOT NULL,
			PRIMARY KEY (endpoint, day)
		);
	`)
	if err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

// save replaces the endpoint's snapshot for snap.Day (a second run on the same
// day overwrites the first).
func (c *Collector) save(slug string, snap *Snapshot, methods map[string]string) error {
	tx, err := c.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM class_counts WHERE endpoint = ? AND day = ?`, slug, snap.Day); err != nil {
		return err
	}
	stmt, err := tx.Prepare(`INSERT INTO class_counts (endpoint, day, class, count, method) VALUES (?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for class, method := range methods {
		var count any
		if n, ok := snap.Counts[class]; ok {
			count = n
		}
		if _, err := stmt.Exec(slug, snap.Day, class, count, method); err != nil {
			return err
		}
	}
	var triples any
	if snap.Triples != nil {
		triples = *snap.Triples
	}
	if _, err := tx.Exec(`INSERT OR REPLACE INTO runs (endpoint, day, engine, triples, started_at, finished_at, classes, failures)
		VALUES (?,?,?,?,?,?,?,?)`,
		slug, snap.Day, snap.Engine, triples, snap.Started.Unix(), snap.Finished.Unix(), snap.Classes, snap.Failures); err != nil {
		return err
	}
	return tx.Commit()
}

// loadLatest reads each endpoint's most recent run into memory.
func (c *Collector) loadLatest() error {
	rows, err := c.db.Query(`SELECT r.endpoint, r.day, r.engine, r.triples, r.started_at, r.finished_at, r.classes, r.failures
		FROM runs r JOIN (SELECT endpoint, MAX(day) AS day FROM runs GROUP BY endpoint) m
		ON r.endpoint = m.endpoint AND r.day = m.day`)
	if err != nil {
		return err
	}
	var snaps []struct {
		slug string
		s    *Snapshot
	}
	for rows.Next() {
		s := &Snapshot{Counts: map[string]int64{}}
		var slug string
		var triples sql.NullInt64
		var started, finished int64
		if err := rows.Scan(&slug, &s.Day, &s.Engine, &triples, &started, &finished, &s.Classes, &s.Failures); err != nil {
			rows.Close()
			return err
		}
		if triples.Valid {
			n := triples.Int64
			s.Triples = &n
		}
		s.Started, s.Finished = time.Unix(started, 0), time.Unix(finished, 0)
		snaps = append(snaps, struct {
			slug string
			s    *Snapshot
		}{slug, s})
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, x := range snaps {
		counts, err := c.countsOn(x.slug, x.s.Day)
		if err != nil {
			return err
		}
		x.s.Counts = counts
		c.latest[x.slug] = x.s
	}
	return nil
}

func (c *Collector) countsOn(slug, day string) (map[string]int64, error) {
	rows, err := c.db.Query(`SELECT class, count FROM class_counts WHERE endpoint = ? AND day = ? AND count IS NOT NULL`, slug, day)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int64{}
	for rows.Next() {
		var class string
		var n int64
		if err := rows.Scan(&class, &n); err != nil {
			return nil, err
		}
		out[class] = n
	}
	return out, rows.Err()
}

// Latest is the endpoint's own latest snapshot (nil if none). Callers must not
// modify it.
func (c *Collector) Latest(slug string) *Snapshot {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.latest[slug]
}

// For resolves an endpoint (any slug) to the snapshot that serves it — its own
// or its class_stats_from source — and that source's slug.
func (c *Collector) For(slug string) (*Snapshot, string) {
	if c == nil {
		return nil, ""
	}
	ep := c.cfg.GetEndpointBySlug(slug)
	if ep == nil {
		return nil, ""
	}
	src := ep.ClassStatsSlug()
	if src == "" {
		return nil, ""
	}
	if s := c.cfg.GetEndpointBySlug(src); s != nil {
		src = s.Slug // canonical case
	}
	return c.Latest(src), src
}

// Point is one day of a series.
type Point struct {
	Day   string `json:"day"`
	Count *int64 `json:"count"` // nil: not counted that day (timeout / error)
}

// Series is one class's daily counts, oldest first, over the last `days` days.
func (c *Collector) Series(slug, class string, days int) ([]Point, error) {
	since := c.now().UTC().AddDate(0, 0, -days).Format("2006-01-02")
	rows, err := c.db.Query(`SELECT day, count FROM class_counts WHERE endpoint = ? AND class = ? AND day > ? ORDER BY day`, slug, class, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Point{}
	for rows.Next() {
		var p Point
		var n sql.NullInt64
		if err := rows.Scan(&p.Day, &n); err != nil {
			return nil, err
		}
		if n.Valid {
			v := n.Int64
			p.Count = &v
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// Change is one class that differs between two snapshots.
type Change struct {
	Class  string   `json:"class"`
	Before *int64   `json:"before"` // nil: new class
	After  *int64   `json:"after"`  // nil: vanished
	Delta  int64    `json:"delta"`
	Pct    *float64 `json:"pct,omitempty"` // relative change; nil for new classes
	Drop   bool     `json:"drop"`          // shrank by more than DropThreshold
}

// DropThreshold flags a class that lost more than this share overnight — usually
// a failed import rather than real data loss.
const DropThreshold = 0.2

// Changes compares the latest snapshot with the latest one at least `days`
// days older. Sorted by the size of the change, largest first.
type Changes struct {
	From          string   `json:"from"` // "" when there is no older snapshot yet
	To            string   `json:"to"`
	TriplesBefore *int64   `json:"triplesBefore"`
	TriplesAfter  *int64   `json:"triplesAfter"`
	Classes       int      `json:"classes"`
	Changes       []Change `json:"changes"`
}

func (c *Collector) Changes(slug string, days int) (*Changes, error) {
	latest := c.Latest(slug)
	if latest == nil {
		return nil, nil
	}
	out := &Changes{To: latest.Day, TriplesAfter: latest.Triples, Classes: len(latest.Counts), Changes: []Change{}}
	to, err := time.Parse("2006-01-02", latest.Day)
	if err != nil {
		return nil, err
	}
	var from string
	var triples sql.NullInt64
	err = c.db.QueryRow(`SELECT day, triples FROM runs WHERE endpoint = ? AND day <= ? ORDER BY day DESC LIMIT 1`,
		slug, to.AddDate(0, 0, -days).Format("2006-01-02")).Scan(&from, &triples)
	if err == sql.ErrNoRows {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	out.From = from
	if triples.Valid {
		n := triples.Int64
		out.TriplesBefore = &n
	}
	before, err := c.countsOn(slug, from)
	if err != nil {
		return nil, err
	}
	beforeListed, err := c.listedOn(slug, from)
	if err != nil {
		return nil, err
	}
	afterListed, err := c.listedOn(slug, latest.Day)
	if err != nil {
		return nil, err
	}
	out.Changes = diff(before, latest.Counts, beforeListed, afterListed)
	return out, nil
}

// listedOn is every class a run listed, counted or not.
func (c *Collector) listedOn(slug, day string) (map[string]bool, error) {
	rows, err := c.db.Query(`SELECT class FROM class_counts WHERE endpoint = ? AND day = ?`, slug, day)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var class string
		if err := rows.Scan(&class); err != nil {
			return nil, err
		}
		out[class] = true
	}
	return out, rows.Err()
}

// diff lists classes whose count changed, appeared or vanished. The counts maps
// hold counted classes only; the listed sets hold every class a run found, so a
// class that one run listed but could not count (timeout) is neither "new" nor
// "vanished" — it is left out.
func diff(before, after map[string]int64, beforeListed, afterListed map[string]bool) []Change {
	var out []Change
	for class, a := range after {
		a := a
		b, ok := before[class]
		if !ok {
			if !beforeListed[class] {
				out = append(out, Change{Class: class, After: &a, Delta: a})
			}
			continue
		}
		if a == b {
			continue
		}
		b2 := b
		ch := Change{Class: class, Before: &b2, After: &a, Delta: a - b}
		if b > 0 {
			pct := float64(a-b) / float64(b)
			ch.Pct = &pct
			ch.Drop = pct < -DropThreshold
		}
		out = append(out, ch)
	}
	for class, b := range before {
		if _, ok := after[class]; !ok && !afterListed[class] {
			b := b
			pct := -1.0
			out = append(out, Change{Class: class, Before: &b, Delta: -b, Pct: &pct, Drop: true})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		ai, aj := abs(out[i].Delta), abs(out[j].Delta)
		if ai != aj {
			return ai > aj
		}
		return out[i].Class < out[j].Class
	})
	return out
}

func abs(n int64) int64 {
	if n < 0 {
		return -n
	}
	return n
}

// Top is the snapshot's classes by count, largest first, at most limit.
func (s *Snapshot) Top(limit int) []ClassCount {
	out := make([]ClassCount, 0, len(s.Counts))
	for class, n := range s.Counts {
		out = append(out, ClassCount{Class: class, Count: n})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Class < out[j].Class
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

// ClassCount is one class and its instance count.
type ClassCount struct {
	Class string `json:"class"`
	Count int64  `json:"count"`
}
