package state

import (
	"database/sql"
	"fmt"
	"time"

	"hutzli.org/visoto/internal/pipeline/source"
)

// DistributionRow is one distribution's job state.
type DistributionRow struct {
	IRI            string
	DatasetIRI     string
	Source         string
	DownloadURL    string
	DeclaredMedia  string
	DeclaredFormat string
	ByteSize       int64
	Licence        string
	Modified       time.Time
	Stage          Stage
	ETag           string
	LastModified   string
	ContentHash    string
	DetectedMedia  string
	StructureHash  string
	Attempts       int
	LastError      string
	FirstSeen      time.Time
	LastSeen       time.Time
}

// UpsertDistributions records the distributions of one harvested dataset.
//
// Catalogue metadata is refreshed on every sighting, but the stage is not: a
// distribution already profiled must not be dragged back to "discovered" just
// because the catalogue was re-read. Only a changed download URL resets it, since
// that genuinely invalidates everything downstream.
func (s *Store) UpsertDistributions(sourceName string, dists []source.Distribution, now time.Time) (int64, error) {
	if len(dists) == 0 {
		return 0, nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return 0, fmt.Errorf("begin upsert: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT INTO distributions (
			iri, dataset_iri, source, download_url, declared_media, declared_format,
			byte_size, licence, modified, stage, attempts, first_seen, last_seen
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0, ?, ?)
		ON CONFLICT(iri) DO UPDATE SET
			dataset_iri     = excluded.dataset_iri,
			source          = excluded.source,
			download_url    = excluded.download_url,
			declared_media  = excluded.declared_media,
			declared_format = excluded.declared_format,
			byte_size       = excluded.byte_size,
			licence         = excluded.licence,
			modified        = excluded.modified,
			last_seen       = excluded.last_seen,
			stage           = CASE
				WHEN distributions.download_url IS NOT excluded.download_url THEN ?
				ELSE distributions.stage
			END`)
	if err != nil {
		return 0, fmt.Errorf("prepare upsert: %w", err)
	}
	defer stmt.Close()

	var n int64
	unix := now.UTC().Unix()
	for _, d := range dists {
		if d.IRI == "" {
			continue
		}
		var modified int64
		if !d.Modified.IsZero() {
			modified = d.Modified.UTC().Unix()
		}
		_, err := stmt.Exec(
			d.IRI, d.DatasetIRI, sourceName, d.DownloadURL, d.DeclaredMedia, d.DeclaredFormat,
			d.ByteSize, d.Licence, modified, string(StageDiscovered), unix, unix,
			string(StageDiscovered))
		if err != nil {
			return n, fmt.Errorf("upsert distribution %s: %w", d.IRI, err)
		}
		n++
	}
	if err := tx.Commit(); err != nil {
		return n, fmt.Errorf("commit upsert: %w", err)
	}
	return n, nil
}

// Pending returns distributions waiting at a stage, oldest sighting first, so a
// resumed run picks up where the last one left off.
func (s *Store) Pending(sourceName string, stage Stage, limit int) ([]DistributionRow, error) {
	query := `SELECT iri, dataset_iri, source, COALESCE(download_url,''), COALESCE(declared_media,''),
	                 COALESCE(declared_format,''), byte_size, COALESCE(licence,''), modified, stage,
	                 COALESCE(etag,''), COALESCE(last_modified,''), COALESCE(content_hash,''),
	                 COALESCE(detected_media,''), COALESCE(structure_hash,''), attempts,
	                 COALESCE(last_error,''), first_seen, last_seen
	            FROM distributions
	           WHERE stage = ?`
	args := []any{string(stage)}
	if sourceName != "" {
		query += ` AND source = ?`
		args = append(args, sourceName)
	}
	query += ` ORDER BY last_seen ASC, iri ASC LIMIT ?`
	args = append(args, limit)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list pending at %s: %w", stage, err)
	}
	defer rows.Close()
	return scanDistributions(rows)
}

// SetStage advances a distribution, clearing the error on success and counting
// the attempt on failure. Attempt counting is what keeps a permanently broken
// URL from being retried forever.
func (s *Store) SetStage(iri string, stage Stage, errMsg string) error {
	// Advancing a stage also drops the claim: the work this lease covered is done,
	// and holding it would idle the row until the lease expired.
	if stage == StageFailed {
		_, err := s.db.Exec(`
			UPDATE distributions
			   SET stage = ?, last_error = ?, attempts = attempts + 1,
			       claimed_by = NULL, claimed_until = 0
			 WHERE iri = ?`, string(stage), errMsg, iri)
		return err
	}
	_, err := s.db.Exec(`
		UPDATE distributions
		   SET stage = ?, last_error = NULL, claimed_by = NULL, claimed_until = 0
		 WHERE iri = ?`, string(stage), iri)
	return err
}

// StageCounts returns how many distributions sit at each stage, for the CLI's
// status command and the run summary.
func (s *Store) StageCounts(sourceName string) (map[Stage]int64, error) {
	query := `SELECT stage, COUNT(*) FROM distributions`
	args := []any{}
	if sourceName != "" {
		query += ` WHERE source = ?`
		args = append(args, sourceName)
	}
	query += ` GROUP BY stage`

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("count stages: %w", err)
	}
	defer rows.Close()

	out := make(map[Stage]int64)
	for rows.Next() {
		var stage string
		var n int64
		if err := rows.Scan(&stage, &n); err != nil {
			return nil, err
		}
		out[Stage(stage)] = n
	}
	return out, rows.Err()
}

// CountDistributions returns the total number of known distributions.
func (s *Store) CountDistributions(sourceName string) (int64, error) {
	query := `SELECT COUNT(*) FROM distributions`
	args := []any{}
	if sourceName != "" {
		query += ` WHERE source = ?`
		args = append(args, sourceName)
	}
	var n int64
	err := s.db.QueryRow(query, args...).Scan(&n)
	return n, err
}

// Distribution reads one row, reporting false when it is unknown.
func (s *Store) Distribution(iri string) (DistributionRow, bool, error) {
	rows, err := s.db.Query(`
		SELECT iri, dataset_iri, source, COALESCE(download_url,''), COALESCE(declared_media,''),
		       COALESCE(declared_format,''), byte_size, COALESCE(licence,''), modified, stage,
		       COALESCE(etag,''), COALESCE(last_modified,''), COALESCE(content_hash,''),
		       COALESCE(detected_media,''), COALESCE(structure_hash,''), attempts,
		       COALESCE(last_error,''), first_seen, last_seen
		  FROM distributions WHERE iri = ?`, iri)
	if err != nil {
		return DistributionRow{}, false, err
	}
	defer rows.Close()

	out, err := scanDistributions(rows)
	if err != nil || len(out) == 0 {
		return DistributionRow{}, false, err
	}
	return out[0], true, nil
}

func scanDistributions(rows *sql.Rows) ([]DistributionRow, error) {
	var out []DistributionRow
	for rows.Next() {
		var d DistributionRow
		var modified, firstSeen, lastSeen int64
		var stage string
		if err := rows.Scan(&d.IRI, &d.DatasetIRI, &d.Source, &d.DownloadURL, &d.DeclaredMedia,
			&d.DeclaredFormat, &d.ByteSize, &d.Licence, &modified, &stage,
			&d.ETag, &d.LastModified, &d.ContentHash, &d.DetectedMedia, &d.StructureHash,
			&d.Attempts, &d.LastError, &firstSeen, &lastSeen); err != nil {
			return nil, err
		}
		if modified > 0 {
			d.Modified = time.Unix(modified, 0).UTC()
		}
		d.Stage = Stage(stage)
		d.FirstSeen = time.Unix(firstSeen, 0).UTC()
		d.LastSeen = time.Unix(lastSeen, 0).UTC()
		out = append(out, d)
	}
	return out, rows.Err()
}
