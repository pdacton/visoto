package state

import (
	"path/filepath"
	"testing"
	"time"

	"hutzli.org/visoto/internal/pipeline/source"
)

func newStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "nested", "state.sqlite"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestWatermarkNeverMovesBackwards(t *testing.T) {
	s := newStore(t)

	if got, err := s.Watermark("src"); err != nil || !got.IsZero() {
		t.Fatalf("initial watermark = %v, %v; want zero", got, err)
	}

	newer := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
	older := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	if err := s.SetWatermark("src", newer); err != nil {
		t.Fatalf("SetWatermark: %v", err)
	}
	// A partial re-harvest of older data must not make the next run re-read the
	// whole catalogue.
	if err := s.SetWatermark("src", older); err != nil {
		t.Fatalf("SetWatermark: %v", err)
	}
	got, err := s.Watermark("src")
	if err != nil {
		t.Fatalf("Watermark: %v", err)
	}
	if !got.Equal(newer) {
		t.Errorf("watermark = %v, want %v", got, newer)
	}

	if err := s.ResetWatermark("src"); err != nil {
		t.Fatalf("ResetWatermark: %v", err)
	}
	if got, _ := s.Watermark("src"); !got.IsZero() {
		t.Errorf("watermark after reset = %v, want zero", got)
	}
}

func TestRunLifecycle(t *testing.T) {
	s := newStore(t)
	start := time.Date(2026, 9, 5, 4, 0, 0, 0, time.UTC)

	run, err := s.StartRun("src", start)
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	if run.ID != "20260905T040000Z" {
		t.Errorf("run ID = %q, want a sortable timestamp", run.ID)
	}

	run.DatasetsSeen = 12
	run.DistributionsSeen = 30
	run.QuadsWritten = 480
	if err := s.FinishRun(run, RunSucceeded, "", start.Add(90*time.Second)); err != nil {
		t.Fatalf("FinishRun: %v", err)
	}

	runs, err := s.RecentRuns("src", 10)
	if err != nil {
		t.Fatalf("RecentRuns: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("got %d runs, want 1", len(runs))
	}
	got := runs[0]
	if got.Status != RunSucceeded || got.DatasetsSeen != 12 || got.QuadsWritten != 480 {
		t.Errorf("run = %+v", got)
	}
	if got.EndedAt.Sub(got.StartedAt) != 90*time.Second {
		t.Errorf("duration = %v, want 90s", got.EndedAt.Sub(got.StartedAt))
	}
}

func TestUpsertPreservesStage(t *testing.T) {
	s := newStore(t)
	now := time.Unix(1000, 0).UTC()
	dist := source.Distribution{
		IRI:           "https://example.org/dist/1",
		DatasetIRI:    "https://example.org/dataset/1",
		DownloadURL:   "https://example.org/a.csv",
		DeclaredMedia: "text/csv",
		ByteSize:      100,
	}

	if _, err := s.UpsertDistributions("src", []source.Distribution{dist}, now); err != nil {
		t.Fatalf("UpsertDistributions: %v", err)
	}
	if err := s.SetStage(dist.IRI, StageProfiled, ""); err != nil {
		t.Fatalf("SetStage: %v", err)
	}

	// Re-reading the catalogue must refresh metadata without dragging the
	// distribution back to the start of the pipeline.
	dist.ByteSize = 200
	if _, err := s.UpsertDistributions("src", []source.Distribution{dist}, now.Add(time.Hour)); err != nil {
		t.Fatalf("re-upsert: %v", err)
	}
	row, ok, err := s.Distribution(dist.IRI)
	if err != nil || !ok {
		t.Fatalf("Distribution: %v, ok=%v", err, ok)
	}
	if row.Stage != StageProfiled {
		t.Errorf("stage = %q, want %q preserved across a re-harvest", row.Stage, StageProfiled)
	}
	if row.ByteSize != 200 {
		t.Errorf("byte size = %d, want the refreshed 200", row.ByteSize)
	}

	// A changed download URL does invalidate the work downstream of it.
	dist.DownloadURL = "https://example.org/b.csv"
	if _, err := s.UpsertDistributions("src", []source.Distribution{dist}, now.Add(2*time.Hour)); err != nil {
		t.Fatalf("re-upsert: %v", err)
	}
	row, _, _ = s.Distribution(dist.IRI)
	if row.Stage != StageDiscovered {
		t.Errorf("stage = %q, want reset to %q after the URL changed", row.Stage, StageDiscovered)
	}
}

func TestPendingAndStageCounts(t *testing.T) {
	s := newStore(t)
	now := time.Unix(1000, 0).UTC()

	dists := []source.Distribution{
		{IRI: "https://example.org/d/1", DatasetIRI: "ds", DownloadURL: "https://example.org/1.csv"},
		{IRI: "https://example.org/d/2", DatasetIRI: "ds", DownloadURL: "https://example.org/2.csv"},
		{IRI: "https://example.org/d/3", DatasetIRI: "ds", DownloadURL: "https://example.org/3.csv"},
	}
	n, err := s.UpsertDistributions("src", dists, now)
	if err != nil || n != 3 {
		t.Fatalf("UpsertDistributions n=%d err=%v", n, err)
	}

	if err := s.SetStage("https://example.org/d/2", StageFailed, "404"); err != nil {
		t.Fatalf("SetStage: %v", err)
	}

	pending, err := s.Pending("src", StageDiscovered, 10)
	if err != nil {
		t.Fatalf("Pending: %v", err)
	}
	if len(pending) != 2 {
		t.Errorf("got %d pending, want 2", len(pending))
	}

	counts, err := s.StageCounts("src")
	if err != nil {
		t.Fatalf("StageCounts: %v", err)
	}
	if counts[StageDiscovered] != 2 || counts[StageFailed] != 1 {
		t.Errorf("counts = %v", counts)
	}

	row, _, _ := s.Distribution("https://example.org/d/2")
	if row.Attempts != 1 || row.LastError != "404" {
		t.Errorf("failure not recorded: attempts=%d err=%q", row.Attempts, row.LastError)
	}

	// A later success clears the error so a transient failure leaves no residue.
	if err := s.SetStage("https://example.org/d/2", StageFetched, ""); err != nil {
		t.Fatalf("SetStage: %v", err)
	}
	row, _, _ = s.Distribution("https://example.org/d/2")
	if row.LastError != "" {
		t.Errorf("last error = %q, want cleared", row.LastError)
	}
}

func TestDistributionUnknown(t *testing.T) {
	s := newStore(t)
	_, ok, err := s.Distribution("https://example.org/missing")
	if err != nil {
		t.Fatalf("Distribution: %v", err)
	}
	if ok {
		t.Error("unknown distribution reported as found")
	}
}
