package harvest

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"hutzli.org/visoto/internal/config"
	"hutzli.org/visoto/internal/pipeline/rdf"
	"hutzli.org/visoto/internal/pipeline/source"
	"hutzli.org/visoto/internal/pipeline/state"
)

// fakeSource is a scripted adapter, so the runner is tested without a network.
type fakeSource struct {
	name    string
	records []source.Record
	err     error

	sawSince time.Time
	calls    int
}

func (f *fakeSource) Name() string { return f.name }

func (f *fakeSource) Harvest(ctx context.Context, since time.Time, emit source.EmitFunc) error {
	f.calls++
	f.sawSince = since
	for _, r := range f.records {
		if err := emit(r); err != nil {
			return err
		}
	}
	return f.err
}

// registerFake wires a scripted adapter under a unique type name.
func registerFake(t *testing.T, typeName string, fake *fakeSource) {
	t.Helper()
	source.Register(typeName, func(cfg config.PipelineSource, opts source.Options) (source.Source, error) {
		fake.name = cfg.Name
		return fake, nil
	})
}

func record(dataset string, modified string, dists ...source.Distribution) source.Record {
	ds := rdf.IRI(dataset)
	quads := []rdf.Quad{
		rdf.NewQuad(ds, rdf.A, rdf.DcatDataset, ""),
		rdf.NewQuad(ds, rdf.DctTitle, rdf.Literal("title of "+dataset), ""),
	}
	if modified != "" {
		quads = append(quads, rdf.NewQuad(ds, rdf.DctModified,
			rdf.TypedLiteral(modified, rdf.Xsd("dateTime")), ""))
	}
	for _, d := range dists {
		quads = append(quads, rdf.NewQuad(ds, rdf.DcatHasDist, rdf.IRI(d.IRI), ""))
	}
	return source.Record{DatasetIRI: dataset, Quads: quads, Distributions: dists}
}

func dist(iri, url string) source.Distribution {
	return source.Distribution{IRI: iri, DownloadURL: url, DeclaredMedia: "text/csv"}
}

// newRunner builds a runner over a temp work dir and state db, with a fixed clock.
func newRunner(t *testing.T, sc config.PipelineSource) (*Runner, *state.Store, *config.PipelineConfig) {
	t.Helper()
	dir := t.TempDir()
	cfg := &config.PipelineConfig{
		Enabled:   true,
		WorkDir:   dir,
		StateDB:   filepath.Join(dir, "state.sqlite"),
		Loader:    config.LoaderBulkFile,
		BaseIRI:   "https://example.org/id/",
		UserAgent: "visoto-harvest/test",
		Sources:   []config.PipelineSource{sc},
	}
	store, err := state.Open(cfg.StateDB)
	if err != nil {
		t.Fatalf("state.Open: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	r := NewRunner(cfg, nil, store)
	clock := time.Date(2026, 9, 5, 4, 0, 0, 0, time.UTC)
	r.now = func() time.Time {
		clock = clock.Add(time.Second)
		return clock
	}
	return r, store, cfg
}

// staged returns the N-Quads the run wrote.
func staged(t *testing.T, workDir string) string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(workDir, "nquads", "*.nq"))
	if err != nil || len(matches) != 1 {
		t.Fatalf("expected exactly one staged file, got %v (%v)", matches, err)
	}
	data, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatalf("read staged file: %v", err)
	}
	return string(data)
}

func TestRunSourceWritesCatalogAndState(t *testing.T) {
	sc := config.PipelineSource{Name: "test-src", Type: "fake-catalog", URL: "https://example.org/api", Enabled: true}
	fake := &fakeSource{records: []source.Record{
		record("https://example.org/dataset/a", "2024-06-01T00:00:00Z",
			dist("https://example.org/dist/a1", "https://example.org/a1.csv"),
			dist("https://example.org/dist/a2", "https://example.org/a2.csv")),
		record("https://example.org/dataset/b", "2024-07-15T00:00:00Z",
			dist("https://example.org/dist/b1", "https://example.org/b1.csv")),
	}}
	registerFake(t, "fake-catalog", fake)

	r, store, cfg := newRunner(t, sc)
	res, err := r.RunSource(context.Background(), sc, false)
	if err != nil {
		t.Fatalf("RunSource: %v", err)
	}

	if res.Run.DatasetsSeen != 2 || res.Run.DistributionsSeen != 3 {
		t.Errorf("datasets=%d distributions=%d, want 2/3",
			res.Run.DatasetsSeen, res.Run.DistributionsSeen)
	}
	if res.Run.Status != state.RunSucceeded {
		t.Errorf("status = %q", res.Run.Status)
	}
	if res.Incremental {
		t.Error("first run should be full")
	}
	if !fake.sawSince.IsZero() {
		t.Errorf("first run passed since=%v, want zero", fake.sawSince)
	}

	// Every distribution is queued for the fetch stage that follows.
	counts, err := store.StageCounts("test-src")
	if err != nil {
		t.Fatalf("StageCounts: %v", err)
	}
	if counts[state.StageDiscovered] != 3 {
		t.Errorf("discovered = %d, want 3", counts[state.StageDiscovered])
	}

	out := staged(t, cfg.WorkDir)
	catalogGraph := "https://example.org/id/graph/catalog/test-src/" + res.Run.ID
	if !strings.Contains(out, "<"+catalogGraph+"> .") {
		t.Errorf("catalogue quads not routed to %s", catalogGraph)
	}
	// Structure will hang off the very same distribution IRIs the catalogue
	// names (R-CAT-2), so those IRIs must appear verbatim.
	for _, want := range []string{"https://example.org/dataset/a", "https://example.org/dist/a1"} {
		if !strings.Contains(out, want) {
			t.Errorf("staged output is missing %s", want)
		}
	}
}

func TestRunSourceWritesProvenanceAndPointer(t *testing.T) {
	sc := config.PipelineSource{Name: "test-src", Type: "fake-prov", URL: "https://example.org/api", Enabled: true}
	registerFake(t, "fake-prov", &fakeSource{records: []source.Record{
		record("https://example.org/dataset/a", "2024-06-01T00:00:00Z"),
	}})

	r, _, cfg := newRunner(t, sc)
	res, err := r.RunSource(context.Background(), sc, false)
	if err != nil {
		t.Fatalf("RunSource: %v", err)
	}
	out := staged(t, cfg.WorkDir)

	runIRI := "https://example.org/id/run/" + res.Run.ID
	sourceIRI := "https://example.org/id/source/test-src"
	for _, want := range []string{
		"<" + runIRI + "> <" + rdf.A.Value + "> <" + rdf.VisoHarvestRun.Value + "> <https://example.org/id/graph/run>",
		"<" + sourceIRI + "> <" + rdf.VisoCurrentCatalogGraph.Value + "> <" + res.CatalogGraph + ">",
		"<https://example.org/id/graph/current/test-src>",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("staged output is missing:\n  %s\n\ngot:\n%s", want, out)
		}
	}

	// Run provenance is cumulative and the pointer graph is replaced, so they
	// must not share a graph.
	if strings.Contains(out, "<https://example.org/id/graph/run/test-src>") {
		t.Error("run provenance must go to the shared cumulative run graph")
	}
}

func TestSecondRunIsIncremental(t *testing.T) {
	sc := config.PipelineSource{Name: "test-src", Type: "fake-incr", URL: "https://example.org/api", Enabled: true}
	fake := &fakeSource{records: []source.Record{
		record("https://example.org/dataset/a", "2024-06-01T00:00:00Z"),
		record("https://example.org/dataset/b", "2024-07-15T00:00:00Z"),
	}}
	registerFake(t, "fake-incr", fake)

	r, _, _ := newRunner(t, sc)
	if _, err := r.RunSource(context.Background(), sc, false); err != nil {
		t.Fatalf("first run: %v", err)
	}
	res, err := r.RunSource(context.Background(), sc, false)
	if err != nil {
		t.Fatalf("second run: %v", err)
	}

	// The watermark is the newest dct:modified actually seen, not the wall clock:
	// a dataset modified during the harvest must not be skipped next time.
	want := time.Date(2024, 7, 15, 0, 0, 0, 0, time.UTC)
	if !fake.sawSince.Equal(want) {
		t.Errorf("second run since = %v, want %v", fake.sawSince, want)
	}
	if !res.Incremental {
		t.Error("second run should be incremental")
	}

	// -full ignores the watermark.
	if _, err := r.RunSource(context.Background(), sc, true); err != nil {
		t.Fatalf("full run: %v", err)
	}
	if !fake.sawSince.IsZero() {
		t.Errorf("full run since = %v, want zero", fake.sawSince)
	}
}

func TestFailedRunLeavesWatermarkAlone(t *testing.T) {
	sc := config.PipelineSource{Name: "test-src", Type: "fake-fail", URL: "https://example.org/api", Enabled: true}
	boom := errors.New("catalogue exploded")
	fake := &fakeSource{
		records: []source.Record{record("https://example.org/dataset/a", "2024-06-01T00:00:00Z")},
		err:     boom,
	}
	registerFake(t, "fake-fail", fake)

	r, store, cfg := newRunner(t, sc)
	res, err := r.RunSource(context.Background(), sc, false)
	if !errors.Is(err, boom) {
		t.Fatalf("RunSource error = %v, want the harvest error", err)
	}
	if res == nil {
		t.Fatal("a failed run must still be reported")
	}

	// The next run must re-read what this one may have missed.
	wm, err := store.Watermark("test-src")
	if err != nil {
		t.Fatalf("Watermark: %v", err)
	}
	if !wm.IsZero() {
		t.Errorf("watermark = %v, want zero after a failed run", wm)
	}

	runs, _ := store.RecentRuns("test-src", 1)
	if len(runs) != 1 || runs[0].Status != state.RunFailed {
		t.Errorf("run status = %+v, want failed", runs)
	}

	// What was harvested before the failure is still staged and inspectable.
	if !strings.Contains(staged(t, cfg.WorkDir), "https://example.org/dataset/a") {
		t.Error("partial harvest should still be staged")
	}
}

func TestRunAllContinuesPastAFailingSource(t *testing.T) {
	good := config.PipelineSource{Name: "good", Type: "fake-good", URL: "https://example.org/a", Enabled: true}
	bad := config.PipelineSource{Name: "bad", Type: "fake-bad", URL: "https://example.org/b", Enabled: true}
	registerFake(t, "fake-good", &fakeSource{records: []source.Record{
		record("https://example.org/dataset/a", "2024-06-01T00:00:00Z"),
	}})
	registerFake(t, "fake-bad", &fakeSource{err: errors.New("down")})

	r, _, cfg := newRunner(t, bad)
	cfg.Sources = []config.PipelineSource{bad, good}

	results, err := r.RunAll(context.Background(), false)
	if err == nil {
		t.Error("RunAll should report the failure")
	}
	if len(results) != 2 {
		t.Fatalf("got %d results, want one per source", len(results))
	}
	if results[0].Run.Status != state.RunFailed || results[1].Run.Status != state.RunSucceeded {
		t.Errorf("one broken portal must not block the rest: %+v", results)
	}
}

func TestCancelledContextStopsHarvest(t *testing.T) {
	sc := config.PipelineSource{Name: "test-src", Type: "fake-cancel", URL: "https://example.org/api", Enabled: true}
	registerFake(t, "fake-cancel", &fakeSource{records: []source.Record{
		record("https://example.org/dataset/a", "2024-06-01T00:00:00Z"),
	}})

	r, _, _ := newRunner(t, sc)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := r.RunSource(ctx, sc, false); !errors.Is(err, context.Canceled) {
		t.Errorf("error = %v, want context.Canceled", err)
	}
}

func TestUnknownSourceTypeFails(t *testing.T) {
	sc := config.PipelineSource{Name: "test-src", Type: "no-such-adapter", URL: "https://example.org", Enabled: true}
	r, _, _ := newRunner(t, sc)
	if _, err := r.RunSource(context.Background(), sc, false); err == nil {
		t.Error("an unknown source type should fail the run")
	}
}

func TestLimitStopsEarlyAndKeepsWatermark(t *testing.T) {
	sc := config.PipelineSource{Name: "test-src", Type: "fake-limit", URL: "https://example.org/api", Enabled: true}
	registerFake(t, "fake-limit", &fakeSource{records: []source.Record{
		record("https://example.org/dataset/a", "2024-06-01T00:00:00Z",
			dist("https://example.org/dist/a1", "https://example.org/a1.csv")),
		record("https://example.org/dataset/b", "2024-07-15T00:00:00Z"),
		record("https://example.org/dataset/c", "2024-08-01T00:00:00Z"),
	}})

	r, store, cfg := newRunner(t, sc)
	r.Limit = 2

	res, err := r.RunSource(context.Background(), sc, false)
	if err != nil {
		t.Fatalf("RunSource: %v", err)
	}
	if res.Run.DatasetsSeen != 2 {
		t.Errorf("datasets = %d, want the limit of 2", res.Run.DatasetsSeen)
	}
	if !res.Partial {
		t.Error("a limited run must be reported as partial")
	}
	if res.Run.Status != state.RunSucceeded {
		t.Errorf("status = %q; hitting the limit is not a failure", res.Run.Status)
	}

	// Advancing the watermark here would silently skip dataset c forever.
	wm, err := store.Watermark("test-src")
	if err != nil {
		t.Fatalf("Watermark: %v", err)
	}
	if !wm.IsZero() {
		t.Errorf("watermark = %v, want zero after a partial run", wm)
	}

	// The prefix that was harvested is complete, distributions included.
	out := staged(t, cfg.WorkDir)
	if !strings.Contains(out, "https://example.org/dataset/b") {
		t.Error("the second dataset should be staged")
	}
	if strings.Contains(out, "https://example.org/dataset/c") {
		t.Error("harvest continued past the limit")
	}
	if n, _ := store.CountDistributions("test-src"); n != 1 {
		t.Errorf("stored %d distributions, want 1 from the harvested prefix", n)
	}
}
