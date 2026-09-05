// Package harvest runs the pipeline: it drives a source adapter, records what it
// found in the job store, and streams the harvested DCAT-AP into the triplestore.
//
// This is stage `discover` plus the catalogue half of `load` (M1). The fetch,
// sniff, profile and mint stages attach to the same Runner and the same job
// store, which is why the distribution rows are written here even though nothing
// reads them yet.
package harvest

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"hutzli.org/visoto/internal/config"
	"hutzli.org/visoto/internal/logger"
	"hutzli.org/visoto/internal/pipeline/load"
	"hutzli.org/visoto/internal/pipeline/rdf"
	"hutzli.org/visoto/internal/pipeline/source"
	"hutzli.org/visoto/internal/pipeline/state"
)

// flushEveryQuads bounds how much of a catalogue is held in memory before being
// handed to the loader. A catalogue of 100k datasets cannot be buffered whole
// (R-NFR-3), and a smaller batch costs only request overhead.
const flushEveryQuads = 5000

// Runner executes harvest runs.
type Runner struct {
	cfg    *config.PipelineConfig
	target *config.SparqlEndpoint
	store  *state.Store
	minter *rdf.Minter
	log    *slog.Logger

	// Limit stops a run after this many datasets. Zero means no limit. It exists
	// for validating a newly configured source against the live catalogue without
	// pulling all of it; a limited run is partial by construction, so it never
	// advances the watermark.
	Limit int

	// now is injectable so tests get deterministic run IDs.
	now func() time.Time
}

// errLimit stops a harvest that has hit Runner.Limit. It is a control signal,
// not a failure, and never escapes the runner.
var errLimit = errors.New("dataset limit reached")

// NewRunner builds a runner over an already-opened job store.
//
// target may be nil when the configured loader writes files rather than talking
// to an endpoint; a loader that needs one says so when it is built.
func NewRunner(cfg *config.PipelineConfig, target *config.SparqlEndpoint, store *state.Store) *Runner {
	return &Runner{
		cfg:    cfg,
		target: target,
		store:  store,
		minter: rdf.NewMinter(cfg.BaseIRI),
		log:    logger.Get().With(slog.String("component", "harvest")),
		now:    func() time.Time { return time.Now().UTC() },
	}
}

// Result reports what one source's run amounted to.
type Result struct {
	Run          *state.Run
	CatalogGraph string
	Load         load.Summary
	Incremental  bool
	Since        time.Time

	// Partial marks a run that stopped early because of Runner.Limit. Its
	// watermark is deliberately left where it was.
	Partial bool
}

// RunAll harvests every enabled source in config order, continuing past a source
// that fails so one broken portal cannot block the rest (R-NFR-5).
func (r *Runner) RunAll(ctx context.Context, full bool) ([]Result, error) {
	sources := r.cfg.EnabledSources()
	if len(sources) == 0 {
		return nil, fmt.Errorf("no enabled sources in [[pipeline.sources]]")
	}

	var results []Result
	var firstErr error
	for _, sc := range sources {
		res, err := r.RunSource(ctx, sc, full)
		if res != nil {
			results = append(results, *res)
		}
		if err != nil {
			r.log.Error("source harvest failed",
				slog.String("source", sc.Name), slog.String("error", err.Error()))
			if firstErr == nil {
				firstErr = err
			}
			if ctx.Err() != nil {
				break // cancellation is not a per-source failure
			}
			continue
		}
	}
	return results, firstErr
}

// RunSource harvests one source. It always returns a Result when a run was
// started, so a caller can report a partial harvest alongside the error.
func (r *Runner) RunSource(ctx context.Context, sc config.PipelineSource, full bool) (*Result, error) {
	since := time.Time{}
	if !full {
		var err error
		if since, err = r.store.Watermark(sc.Name); err != nil {
			return nil, err
		}
	}

	run, err := r.store.StartRun(sc.Name, r.now())
	if err != nil {
		return nil, err
	}
	log := r.log.With(slog.String("source", sc.Name), slog.String("run", run.ID))
	log.Info("harvest started",
		slog.Bool("full", since.IsZero()),
		slog.Time("since", since),
		slog.String("loader", r.cfg.Loader))

	result := &Result{
		Run:          run,
		CatalogGraph: r.minter.CatalogGraph(sc.Name, run.ID),
		Incremental:  !since.IsZero(),
		Since:        since,
	}

	err = r.harvest(ctx, sc, run, since, result, log)
	if err != nil {
		// The run is recorded as failed and the watermark is left where it was,
		// so the next run re-reads whatever this one may have missed.
		if ferr := r.store.FinishRun(run, state.RunFailed, err.Error(), r.now()); ferr != nil {
			log.Error("could not record run failure", slog.String("error", ferr.Error()))
		}
		return result, err
	}

	if err := r.store.FinishRun(run, state.RunSucceeded, "", r.now()); err != nil {
		return result, err
	}
	log.Info("harvest finished",
		slog.Int64("datasets", run.DatasetsSeen),
		slog.Int64("distributions", run.DistributionsSeen),
		slog.Int64("quads", run.QuadsWritten))
	return result, nil
}

// harvest is the body of a run, separated so RunSource owns the bookkeeping on
// every exit path.
func (r *Runner) harvest(ctx context.Context, sc config.PipelineSource, run *state.Run,
	since time.Time, result *Result, log *slog.Logger) error {

	adapter, err := source.New(sc, source.Options{
		UserAgent: r.cfg.UserAgent,
		Minter:    r.minter,
	})
	if err != nil {
		return err
	}

	loader, err := r.newLoader(sc, run)
	if err != nil {
		return err
	}
	defer loader.Close()

	catalogGraph := result.CatalogGraph
	if err := loader.BeginGraph(ctx, catalogGraph); err != nil {
		return fmt.Errorf("begin catalog graph: %w", err)
	}

	var (
		buffer  []rdf.Quad
		maxSeen time.Time
	)
	flush := func() error {
		if len(buffer) == 0 {
			return nil
		}
		if err := loader.Append(ctx, catalogGraph, buffer); err != nil {
			return fmt.Errorf("append to catalog graph: %w", err)
		}
		run.QuadsWritten += int64(len(buffer))
		buffer = buffer[:0]
		return nil
	}

	emit := func(rec source.Record) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		run.DatasetsSeen++
		buffer = append(buffer, rec.Quads...)

		if r.Limit > 0 && run.DatasetsSeen >= int64(r.Limit) {
			// Stop after recording this dataset, so the limit is inclusive and the
			// staged output is a complete prefix of the catalogue.
			if err := r.storeDistributions(sc.Name, rec, run, &maxSeen); err != nil {
				return err
			}
			return errLimit
		}

		if err := r.storeDistributions(sc.Name, rec, run, &maxSeen); err != nil {
			return err
		}

		if len(buffer) >= flushEveryQuads {
			return flush()
		}
		return nil
	}

	harvestErr := adapter.Harvest(ctx, since, emit)
	if errors.Is(harvestErr, errLimit) {
		harvestErr = nil
		result.Partial = true
		log.Info("stopped at dataset limit", slog.Int("limit", r.Limit))
	}

	// Flush and commit regardless of the outcome: what was harvested before a
	// failure is worth keeping and inspecting, and the summary tells the operator
	// what actually landed. The run is still recorded as failed, and the
	// watermark still stays put, so nothing is lost by staging it.
	if flushErr := flush(); harvestErr == nil {
		harvestErr = flushErr
	}

	// Provenance and the current-graph pointer describe a completed harvest, so
	// they are written only when one happened — and before the commit, which is
	// what makes the run's writes durable.
	if harvestErr == nil {
		harvestErr = r.writeProvenance(ctx, loader, sc, run, catalogGraph, maxSeen)
	}

	summary, commitErr := loader.Commit(ctx)
	result.Load = summary

	if harvestErr != nil {
		return harvestErr
	}
	if commitErr != nil {
		return commitErr
	}

	// A limited run saw only a prefix of the catalogue, so advancing the watermark
	// would silently skip everything it did not reach.
	if !maxSeen.IsZero() && !result.Partial {
		if err := r.store.SetWatermark(sc.Name, maxSeen); err != nil {
			return err
		}
		log.Debug("watermark advanced", slog.Time("watermark", maxSeen))
	}
	return nil
}

// storeDistributions records a record's distributions and tracks the watermark.
//
// The watermark advances to the newest dct:modified actually seen, not to "now":
// a dataset modified while the harvest is running must be picked up by the next
// run, not skipped because the clock moved past it.
func (r *Runner) storeDistributions(sourceName string, rec source.Record, run *state.Run, maxSeen *time.Time) error {
	n, err := r.store.UpsertDistributions(sourceName, rec.Distributions, r.now())
	if err != nil {
		return err
	}
	run.DistributionsSeen += n

	for _, d := range rec.Distributions {
		if d.Modified.After(*maxSeen) {
			*maxSeen = d.Modified
		}
	}
	if t, ok := recordModified(rec); ok && t.After(*maxSeen) {
		*maxSeen = t
	}
	return nil
}

// newLoader builds the configured loader for this run, scoping staged filenames
// by source so two sources harvested in one session cannot overwrite each other.
func (r *Runner) newLoader(sc config.PipelineSource, run *state.Run) (load.Loader, error) {
	opts := load.Options{
		WorkDir:    r.cfg.WorkDir,
		RunID:      rdf.Hash(sc.Name)[:8] + "-" + run.ID,
		UserAgent:  r.cfg.UserAgent,
		BatchQuads: r.cfg.UpdateBatchQuads,
	}
	if r.target != nil {
		opts.Endpoint = r.target.URL
		opts.AccessToken = r.target.AccessToken
		opts.Username = r.target.Username
		opts.Password = r.target.Password
	}
	return load.New(r.cfg.Loader, opts)
}

// recordModified reads the dataset's own dct:modified, which is what an
// incremental harvest filters on.
func recordModified(rec source.Record) (time.Time, bool) {
	var newest time.Time
	for _, q := range rec.Quads {
		if q.Subject.Value != rec.DatasetIRI || q.Predicate != rdf.DctModified {
			continue
		}
		if t, ok := source.ParseTime(q.Object.Value); ok && t.After(newest) {
			newest = t
		}
	}
	return newest, !newest.IsZero()
}
