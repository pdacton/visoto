package state

import (
	"fmt"
	"time"
)

// ClaimBatch leases up to limit distributions waiting at a stage, so that no two
// workers process the same one.
//
// This is what makes the pipeline's workers stateless. A worker holds nothing
// that is not recoverable: blobs are content-addressed, minted IRIs are a
// function of content rather than of who computed them, and a stage transition
// is recorded before the side effect it describes. Kill a worker mid-fetch and
// its lease expires; another picks the work up and produces byte-identical
// output. What cannot be stateless is the queue itself — something has to say
// who is working on what.
//
// The claim is a lease rather than a lock precisely because workers die: a lock
// held by a dead process is a stuck queue, whereas a lease that runs out is
// self-healing. Pick a lease comfortably longer than the stage's worst-case
// duration, since an expired lease means duplicate work, not corruption.
//
// The SELECT-then-UPDATE shape is deliberate. Under SQLite the surrounding
// transaction is enough, because there is one writer. It is also exactly the
// shape PostgreSQL implements with SELECT ... FOR UPDATE SKIP LOCKED, so moving
// the queue to a shared backend for multi-node runs replaces this method's body
// and nothing above it.
func (s *Store) ClaimBatch(worker, sourceName string, stage Stage, limit int, lease time.Duration, now time.Time) ([]DistributionRow, error) {
	if worker == "" {
		return nil, fmt.Errorf("a claim needs a worker identity")
	}
	if lease <= 0 {
		return nil, fmt.Errorf("a claim needs a positive lease")
	}
	unix := now.UTC().Unix()
	until := now.UTC().Add(lease).Unix()

	tx, err := s.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("begin claim: %w", err)
	}
	defer tx.Rollback()

	// Unclaimed, or claimed by a worker whose lease has run out.
	query := `SELECT iri FROM distributions
	           WHERE stage = ? AND claimed_until < ?`
	args := []any{string(stage), unix}
	if sourceName != "" {
		query += ` AND source = ?`
		args = append(args, sourceName)
	}
	query += ` ORDER BY last_seen ASC, iri ASC LIMIT ?`
	args = append(args, limit)

	rows, err := tx.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("select claimable: %w", err)
	}
	var iris []string
	for rows.Next() {
		var iri string
		if err := rows.Scan(&iri); err != nil {
			rows.Close()
			return nil, err
		}
		iris = append(iris, iri)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(iris) == 0 {
		return nil, nil
	}

	stmt, err := tx.Prepare(`UPDATE distributions SET claimed_by = ?, claimed_until = ? WHERE iri = ?`)
	if err != nil {
		return nil, fmt.Errorf("prepare claim: %w", err)
	}
	defer stmt.Close()
	for _, iri := range iris {
		if _, err := stmt.Exec(worker, until, iri); err != nil {
			return nil, fmt.Errorf("claim %s: %w", iri, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit claim: %w", err)
	}

	out := make([]DistributionRow, 0, len(iris))
	for _, iri := range iris {
		row, ok, err := s.Distribution(iri)
		if err != nil {
			return out, err
		}
		if ok {
			out = append(out, row)
		}
	}
	return out, nil
}

// Release drops a claim without advancing the stage, so a worker shutting down
// cleanly hands its work back immediately instead of making the next worker wait
// out the lease.
func (s *Store) Release(iris ...string) error {
	if len(iris) == 0 {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin release: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`UPDATE distributions SET claimed_by = NULL, claimed_until = 0 WHERE iri = ?`)
	if err != nil {
		return fmt.Errorf("prepare release: %w", err)
	}
	defer stmt.Close()
	for _, iri := range iris {
		if _, err := stmt.Exec(iri); err != nil {
			return fmt.Errorf("release %s: %w", iri, err)
		}
	}
	return tx.Commit()
}

// Claimed reports how many distributions currently hold a live lease, which is
// the number a status view should show as in flight.
func (s *Store) Claimed(sourceName string, now time.Time) (int64, error) {
	query := `SELECT COUNT(*) FROM distributions WHERE claimed_until >= ?`
	args := []any{now.UTC().Unix()}
	if sourceName != "" {
		query += ` AND source = ?`
		args = append(args, sourceName)
	}
	var n int64
	err := s.db.QueryRow(query, args...).Scan(&n)
	return n, err
}
