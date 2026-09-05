package state

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"hutzli.org/visoto/internal/pipeline/signature"
)

// SignatureRow is a stored field signature and the accumulated sketch of every
// field that has mapped to it.
type SignatureRow struct {
	Key              string
	NormalizedName   string
	Datatype         string
	PatternClass     string
	CardinalityClass string
	Observations     int64
	DistinctEstimate int64
	Sketch           *signature.Sketch
	FirstSeen        time.Time
	LastSeen         time.Time
}

// SignatureLink is one similarity edge between two signatures.
type SignatureLink struct {
	A          string
	B          string
	Jaccard    float64
	ComputedAt time.Time
}

// migrateSignatures creates the signature tables.
//
// Sketches live here and never in RDF: they are a compression artefact of other
// people's data, not something a SPARQL consumer can use, and they are rewritten
// on every observation. Only the resulting signature IRI reaches the triplestore.
func (s *Store) migrateSignatures() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS field_signatures (
			key               TEXT PRIMARY KEY,
			normalized_name   TEXT NOT NULL,
			datatype          TEXT NOT NULL,
			pattern_class     TEXT NOT NULL,
			cardinality_class TEXT NOT NULL,
			observations      INTEGER NOT NULL DEFAULT 0,
			distinct_estimate INTEGER NOT NULL DEFAULT 0,
			sketch            BLOB NOT NULL,
			first_seen        INTEGER NOT NULL,
			last_seen         INTEGER NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_sig_name ON field_signatures(normalized_name);

		CREATE TABLE IF NOT EXISTS signature_links (
			a           TEXT    NOT NULL,
			b           TEXT    NOT NULL,
			jaccard     REAL    NOT NULL,
			computed_at INTEGER NOT NULL,
			PRIMARY KEY (a, b)
		);
		CREATE INDEX IF NOT EXISTS idx_link_b ON signature_links(b, jaccard DESC);
	`)
	if err != nil {
		return fmt.Errorf("migrate signature tables: %w", err)
	}
	return nil
}

// ObserveSignature records that a field with this descriptor was profiled.
//
// Repeated observations of the same signature merge their sketches, so the
// stored sketch converges on the union of every value set that has mapped to it.
// That is what makes a signature the value universe of a concept rather than of
// one file.
func (s *Store) ObserveSignature(desc signature.Descriptor, now time.Time) (string, error) {
	if desc.Sketch == nil {
		return "", errors.New("signature descriptor carries no sketch")
	}
	key := desc.Key()
	unix := now.UTC().Unix()

	tx, err := s.db.Begin()
	if err != nil {
		return "", fmt.Errorf("begin observe: %w", err)
	}
	defer tx.Rollback()

	var existing []byte
	err = tx.QueryRow(`SELECT sketch FROM field_signatures WHERE key = ?`, key).Scan(&existing)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		// First sighting.
	case err != nil:
		return "", fmt.Errorf("read signature %s: %w", key, err)
	default:
		prior, decodeErr := signature.DecodeSketch(existing)
		if decodeErr != nil {
			return "", fmt.Errorf("stored sketch for %s: %w", key, decodeErr)
		}
		merged := prior.Clone()
		merged.Merge(desc.Sketch)
		desc.Sketch = merged
	}

	_, err = tx.Exec(`
		INSERT INTO field_signatures (
			key, normalized_name, datatype, pattern_class, cardinality_class,
			observations, distinct_estimate, sketch, first_seen, last_seen
		) VALUES (?, ?, ?, ?, ?, 1, ?, ?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET
			observations      = field_signatures.observations + 1,
			distinct_estimate = MAX(field_signatures.distinct_estimate, excluded.distinct_estimate),
			sketch            = excluded.sketch,
			last_seen         = excluded.last_seen`,
		key, signature.NormalizeName(desc.Name), desc.Datatype, desc.PatternClass,
		desc.CardinalityClass(), desc.DistinctCount, desc.Sketch.Encode(), unix, unix)
	if err != nil {
		return "", fmt.Errorf("upsert signature %s: %w", key, err)
	}
	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("commit observe: %w", err)
	}
	return key, nil
}

// Signature reads one signature row.
func (s *Store) Signature(key string) (SignatureRow, bool, error) {
	rows, err := s.db.Query(`
		SELECT key, normalized_name, datatype, pattern_class, cardinality_class,
		       observations, distinct_estimate, sketch, first_seen, last_seen
		  FROM field_signatures WHERE key = ?`, key)
	if err != nil {
		return SignatureRow{}, false, err
	}
	defer rows.Close()

	out, err := scanSignatures(rows)
	if err != nil || len(out) == 0 {
		return SignatureRow{}, false, err
	}
	return out[0], true, nil
}

// Signatures pages through stored signatures in key order, which is what the
// similarity job walks. Paging by key rather than offset keeps the walk stable
// while new signatures are being written.
func (s *Store) Signatures(afterKey string, limit int) ([]SignatureRow, error) {
	rows, err := s.db.Query(`
		SELECT key, normalized_name, datatype, pattern_class, cardinality_class,
		       observations, distinct_estimate, sketch, first_seen, last_seen
		  FROM field_signatures
		 WHERE key > ?
		 ORDER BY key ASC
		 LIMIT ?`, afterKey, limit)
	if err != nil {
		return nil, fmt.Errorf("list signatures: %w", err)
	}
	defer rows.Close()
	return scanSignatures(rows)
}

// CountSignatures returns how many signatures are stored.
func (s *Store) CountSignatures() (int64, error) {
	var n int64
	err := s.db.QueryRow(`SELECT COUNT(*) FROM field_signatures`).Scan(&n)
	return n, err
}

// RecordLinks stores similarity edges. Each edge is written once, with the
// lexicographically smaller key first, so the relation is stored symmetrically
// without being stored twice.
func (s *Store) RecordLinks(links []SignatureLink, now time.Time) error {
	if len(links) == 0 {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin record links: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT INTO signature_links (a, b, jaccard, computed_at) VALUES (?, ?, ?, ?)
		ON CONFLICT(a, b) DO UPDATE SET jaccard = excluded.jaccard,
		                                computed_at = excluded.computed_at`)
	if err != nil {
		return fmt.Errorf("prepare link insert: %w", err)
	}
	defer stmt.Close()

	unix := now.UTC().Unix()
	for _, l := range links {
		a, b := l.A, l.B
		if a == b {
			continue // a signature is trivially similar to itself
		}
		if a > b {
			a, b = b, a
		}
		if _, err := stmt.Exec(a, b, l.Jaccard, unix); err != nil {
			return fmt.Errorf("record link %s~%s: %w", a, b, err)
		}
	}
	return tx.Commit()
}

// Links returns the signatures most similar to key, strongest first.
func (s *Store) Links(key string, minJaccard float64, limit int) ([]SignatureLink, error) {
	rows, err := s.db.Query(`
		SELECT a, b, jaccard, computed_at FROM signature_links
		 WHERE (a = ? OR b = ?) AND jaccard >= ?
		 ORDER BY jaccard DESC LIMIT ?`, key, key, minJaccard, limit)
	if err != nil {
		return nil, fmt.Errorf("list links for %s: %w", key, err)
	}
	defer rows.Close()

	var out []SignatureLink
	for rows.Next() {
		var l SignatureLink
		var computed int64
		if err := rows.Scan(&l.A, &l.B, &l.Jaccard, &computed); err != nil {
			return nil, err
		}
		l.ComputedAt = time.Unix(computed, 0).UTC()
		out = append(out, l)
	}
	return out, rows.Err()
}

func scanSignatures(rows *sql.Rows) ([]SignatureRow, error) {
	var out []SignatureRow
	for rows.Next() {
		var r SignatureRow
		var sketch []byte
		var firstSeen, lastSeen int64
		if err := rows.Scan(&r.Key, &r.NormalizedName, &r.Datatype, &r.PatternClass,
			&r.CardinalityClass, &r.Observations, &r.DistinctEstimate, &sketch,
			&firstSeen, &lastSeen); err != nil {
			return nil, err
		}
		decoded, err := signature.DecodeSketch(sketch)
		if err != nil {
			return nil, fmt.Errorf("stored sketch for %s: %w", r.Key, err)
		}
		r.Sketch = decoded
		r.FirstSeen = time.Unix(firstSeen, 0).UTC()
		r.LastSeen = time.Unix(lastSeen, 0).UTC()
		out = append(out, r)
	}
	return out, rows.Err()
}
