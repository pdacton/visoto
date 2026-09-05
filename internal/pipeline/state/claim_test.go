package state

import (
	"testing"
	"time"

	"hutzli.org/visoto/internal/pipeline/signature"
	"hutzli.org/visoto/internal/pipeline/source"
)

func seed(t *testing.T, s *Store, n int) time.Time {
	t.Helper()
	now := time.Unix(1000, 0).UTC()
	dists := make([]source.Distribution, 0, n)
	for i := 0; i < n; i++ {
		iri := "https://example.org/d/" + string(rune('a'+i))
		dists = append(dists, source.Distribution{
			IRI: iri, DatasetIRI: "ds", DownloadURL: iri + ".csv",
		})
	}
	if _, err := s.UpsertDistributions("src", dists, now); err != nil {
		t.Fatalf("UpsertDistributions: %v", err)
	}
	return now
}

func TestClaimIsExclusive(t *testing.T) {
	s := newStore(t)
	now := seed(t, s, 5)

	first, err := s.ClaimBatch("worker-1", "src", StageDiscovered, 3, time.Minute, now)
	if err != nil {
		t.Fatalf("ClaimBatch: %v", err)
	}
	if len(first) != 3 {
		t.Fatalf("worker-1 claimed %d, want 3", len(first))
	}

	// A second worker must get the rest, never the same rows.
	second, err := s.ClaimBatch("worker-2", "src", StageDiscovered, 3, time.Minute, now)
	if err != nil {
		t.Fatalf("ClaimBatch: %v", err)
	}
	if len(second) != 2 {
		t.Fatalf("worker-2 claimed %d, want the remaining 2", len(second))
	}
	claimed := map[string]bool{}
	for _, d := range append(first, second...) {
		if claimed[d.IRI] {
			t.Fatalf("%s was handed to two workers", d.IRI)
		}
		claimed[d.IRI] = true
	}

	// Nothing left to claim.
	third, err := s.ClaimBatch("worker-3", "src", StageDiscovered, 3, time.Minute, now)
	if err != nil {
		t.Fatalf("ClaimBatch: %v", err)
	}
	if len(third) != 0 {
		t.Errorf("worker-3 claimed %d, want 0", len(third))
	}
}

func TestExpiredLeaseIsReclaimable(t *testing.T) {
	s := newStore(t)
	now := seed(t, s, 2)

	if _, err := s.ClaimBatch("dying-worker", "src", StageDiscovered, 2, time.Minute, now); err != nil {
		t.Fatalf("ClaimBatch: %v", err)
	}
	if n, _ := s.Claimed("src", now); n != 2 {
		t.Errorf("claimed = %d, want 2 in flight", n)
	}

	// A lock held by a dead process would be a stuck queue; a lease is
	// self-healing.
	later := now.Add(2 * time.Minute)
	reclaimed, err := s.ClaimBatch("fresh-worker", "src", StageDiscovered, 2, time.Minute, later)
	if err != nil {
		t.Fatalf("ClaimBatch: %v", err)
	}
	if len(reclaimed) != 2 {
		t.Errorf("reclaimed %d after the lease expired, want 2", len(reclaimed))
	}
	if n, _ := s.Claimed("src", later); n != 2 {
		t.Errorf("claimed = %d after reclaim", n)
	}
}

func TestAdvancingStageDropsTheClaim(t *testing.T) {
	s := newStore(t)
	now := seed(t, s, 1)

	claimed, err := s.ClaimBatch("worker-1", "src", StageDiscovered, 1, time.Hour, now)
	if err != nil || len(claimed) != 1 {
		t.Fatalf("ClaimBatch: %v (%d rows)", err, len(claimed))
	}
	if err := s.SetStage(claimed[0].IRI, StageFetched, ""); err != nil {
		t.Fatalf("SetStage: %v", err)
	}

	// Holding the lease after the work is done would idle the row for an hour.
	if n, _ := s.Claimed("src", now); n != 0 {
		t.Errorf("claimed = %d after advancing the stage, want 0", n)
	}
	next, err := s.ClaimBatch("worker-2", "src", StageFetched, 1, time.Hour, now)
	if err != nil || len(next) != 1 {
		t.Errorf("the next stage could not claim it: %v (%d rows)", err, len(next))
	}
}

func TestFailingAlsoDropsTheClaim(t *testing.T) {
	s := newStore(t)
	now := seed(t, s, 1)
	claimed, _ := s.ClaimBatch("worker-1", "src", StageDiscovered, 1, time.Hour, now)
	if err := s.SetStage(claimed[0].IRI, StageFailed, "404"); err != nil {
		t.Fatalf("SetStage: %v", err)
	}
	if n, _ := s.Claimed("src", now); n != 0 {
		t.Errorf("claimed = %d after a failure, want 0", n)
	}
}

func TestReleaseHandsWorkBack(t *testing.T) {
	s := newStore(t)
	now := seed(t, s, 2)

	claimed, err := s.ClaimBatch("worker-1", "src", StageDiscovered, 2, time.Hour, now)
	if err != nil {
		t.Fatalf("ClaimBatch: %v", err)
	}
	iris := []string{claimed[0].IRI, claimed[1].IRI}
	if err := s.Release(iris...); err != nil {
		t.Fatalf("Release: %v", err)
	}

	// A worker shutting down cleanly should not make the next one wait out an
	// hour-long lease.
	again, err := s.ClaimBatch("worker-2", "src", StageDiscovered, 2, time.Hour, now)
	if err != nil {
		t.Fatalf("ClaimBatch: %v", err)
	}
	if len(again) != 2 {
		t.Errorf("reclaimed %d after release, want 2", len(again))
	}
}

func TestClaimValidatesArguments(t *testing.T) {
	s := newStore(t)
	now := seed(t, s, 1)
	if _, err := s.ClaimBatch("", "src", StageDiscovered, 1, time.Minute, now); err == nil {
		t.Error("an anonymous claim should be refused")
	}
	if _, err := s.ClaimBatch("w", "src", StageDiscovered, 1, 0, now); err == nil {
		t.Error("a zero lease would never expire and should be refused")
	}
}

func sig(name string, values ...string) signature.Descriptor {
	sk := signature.NewSketch()
	for _, v := range values {
		sk.Add(v)
	}
	return signature.Descriptor{
		Name:          name,
		Datatype:      "http://www.w3.org/2001/XMLSchema#integer",
		PatternClass:  "999",
		DistinctCount: int64(len(values)),
		RowCount:      int64(len(values) * 4),
		Sketch:        sk,
	}
}

func TestObserveSignatureMergesSketches(t *testing.T) {
	s := newStore(t)
	now := time.Unix(2000, 0).UTC()

	// The same column in two different distributions, with different values.
	key, err := s.ObserveSignature(sig("bfs_nr", "261", "351"), now)
	if err != nil {
		t.Fatalf("ObserveSignature: %v", err)
	}
	// A different spelling of the same column must land on the same key.
	key2, err := s.ObserveSignature(sig("BFS-Nr.", "1061", "2701"), now.Add(time.Hour))
	if err != nil {
		t.Fatalf("ObserveSignature: %v", err)
	}
	if key != key2 {
		t.Fatalf("spellings produced different keys: %q vs %q", key, key2)
	}

	row, ok, err := s.Signature(key)
	if err != nil || !ok {
		t.Fatalf("Signature: %v, ok=%v", err, ok)
	}
	if row.Observations != 2 {
		t.Errorf("observations = %d, want 2", row.Observations)
	}

	// The stored sketch must now cover both value sets, so a field holding
	// either one is recognizably related to the signature.
	for _, v := range []string{"261", "1061"} {
		probe := signature.NewSketch()
		probe.Add(v)
		if row.Sketch.Jaccard(probe) == 0 {
			t.Errorf("merged sketch lost %q", v)
		}
	}
	if row.LastSeen.Equal(row.FirstSeen) {
		t.Error("last_seen should have advanced on the second observation")
	}
}

func TestObserveSignatureRequiresASketch(t *testing.T) {
	s := newStore(t)
	desc := sig("x", "1")
	desc.Sketch = nil
	if _, err := s.ObserveSignature(desc, time.Now()); err == nil {
		t.Error("a descriptor without a sketch should be refused")
	}
}

func TestSignaturesPageByKey(t *testing.T) {
	s := newStore(t)
	now := time.Unix(2000, 0).UTC()
	for _, name := range []string{"alpha", "beta", "gamma", "delta"} {
		if _, err := s.ObserveSignature(sig(name, "1", "2"), now); err != nil {
			t.Fatalf("ObserveSignature: %v", err)
		}
	}
	if n, _ := s.CountSignatures(); n != 4 {
		t.Fatalf("stored %d signatures, want 4", n)
	}

	var seen []string
	cursor := ""
	for {
		page, err := s.Signatures(cursor, 2)
		if err != nil {
			t.Fatalf("Signatures: %v", err)
		}
		if len(page) == 0 {
			break
		}
		for _, row := range page {
			seen = append(seen, row.Key)
		}
		cursor = page[len(page)-1].Key
	}
	if len(seen) != 4 {
		t.Errorf("walked %d signatures, want 4", len(seen))
	}
	for i := 1; i < len(seen); i++ {
		if seen[i] <= seen[i-1] {
			t.Errorf("keys are not strictly ascending: %v", seen)
		}
	}
}

func TestRecordLinksIsSymmetric(t *testing.T) {
	s := newStore(t)
	now := time.Unix(3000, 0).UTC()

	if err := s.RecordLinks([]SignatureLink{
		{A: "zeta", B: "alpha", Jaccard: 0.82},
		{A: "alpha", B: "zeta", Jaccard: 0.82}, // same edge, other direction
		{A: "alpha", B: "alpha", Jaccard: 1.0}, // self-similarity is meaningless
	}, now); err != nil {
		t.Fatalf("RecordLinks: %v", err)
	}

	links, err := s.Links("alpha", 0.5, 10)
	if err != nil {
		t.Fatalf("Links: %v", err)
	}
	if len(links) != 1 {
		t.Fatalf("got %d links, want 1 stored once: %+v", len(links), links)
	}
	if links[0].A != "alpha" || links[0].B != "zeta" {
		t.Errorf("edge not normalized: %+v", links[0])
	}

	// Both endpoints must find it.
	if other, _ := s.Links("zeta", 0.5, 10); len(other) != 1 {
		t.Errorf("the other endpoint found %d links, want 1", len(other))
	}
	// The threshold must filter.
	if strong, _ := s.Links("alpha", 0.9, 10); len(strong) != 0 {
		t.Errorf("threshold ignored: %+v", strong)
	}
}
