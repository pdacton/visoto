package load

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"hutzli.org/visoto/internal/config"
	"hutzli.org/visoto/internal/pipeline/rdf"
)

func init() {
	Register(config.LoaderBulkFile, newBulkFile)
}

// BulkFile stages quads as N-Quads for a QLever bulk re-index. This is the
// default: a full harvest through SPARQL UPDATE would leave the endpoint slower
// than it found it.
//
// Graph replacement is expressed by the file itself rather than by a delete: the
// re-index rebuilds the whole index from the data directory, so the staged file
// *is* the new state of every graph it names. The manifest records which graphs
// those are, so an operator — or a later incremental loader — can tell what the
// re-index will replace.
type BulkFile struct {
	dir      string
	runID    string
	path     string
	file     *os.File
	writer   *rdf.Writer
	graphs   map[string]int64
	quadsIn  int64
	finished bool
}

func newBulkFile(opts Options) (Loader, error) {
	dir := filepath.Join(opts.WorkDir, "nquads")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create staging dir %s: %w", dir, err)
	}
	runID := opts.RunID
	if runID == "" {
		runID = time.Now().UTC().Format("20060102T150405Z")
	}
	path := filepath.Join(dir, runID+".nq")
	f, err := os.Create(path)
	if err != nil {
		return nil, fmt.Errorf("create staging file %s: %w", path, err)
	}
	return &BulkFile{
		dir:    dir,
		runID:  runID,
		path:   path,
		file:   f,
		writer: rdf.NewWriter(f),
		graphs: make(map[string]int64),
	}, nil
}

// Name returns the loader name.
func (b *BulkFile) Name() string { return config.LoaderBulkFile }

// Path returns the staged file, for logging and tests.
func (b *BulkFile) Path() string { return b.path }

// BeginGraph records that the run defines this graph. There is nothing to
// delete: a re-index rebuilds the store from the data directory, so the staged
// file is the new state of every graph it names.
func (b *BulkFile) BeginGraph(ctx context.Context, graph string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if graph == "" {
		return fmt.Errorf("bulk-file loader requires a named graph")
	}
	if _, ok := b.graphs[graph]; !ok {
		b.graphs[graph] = 0
	}
	return nil
}

// Append writes the quads into the staging file, routed to graph.
func (b *BulkFile) Append(ctx context.Context, graph string, quads []rdf.Quad) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if graph == "" {
		return fmt.Errorf("bulk-file loader requires a named graph")
	}
	// Writing after the commit would land in a buffer nothing will flush, so it
	// has to be an error rather than a silent loss.
	if b.finished {
		return fmt.Errorf("bulk-file loader: append to %s after commit", graph)
	}
	before := b.writer.Written()
	for _, q := range quads {
		if err := b.writer.Write(q.InGraph(graph)); err != nil {
			return fmt.Errorf("stage quad for %s: %w", graph, err)
		}
	}
	b.graphs[graph] += b.writer.Written() - before
	b.quadsIn += int64(len(quads))
	return nil
}

// Commit flushes the staging file and writes a manifest beside it.
func (b *BulkFile) Commit(ctx context.Context) (Summary, error) {
	if b.finished {
		return Summary{}, fmt.Errorf("bulk-file loader already committed")
	}
	if err := b.writer.Flush(); err != nil {
		return Summary{}, fmt.Errorf("flush staging file: %w", err)
	}
	if err := b.file.Sync(); err != nil {
		return Summary{}, fmt.Errorf("sync staging file: %w", err)
	}
	b.finished = true

	manifestPath := filepath.Join(b.dir, b.runID+".manifest.json")
	manifest := struct {
		RunID     string           `json:"run_id"`
		File      string           `json:"file"`
		WrittenAt time.Time        `json:"written_at"`
		Quads     int64            `json:"quads"`
		Skipped   int64            `json:"skipped"`
		Graphs    map[string]int64 `json:"graphs"`
	}{
		RunID:     b.runID,
		File:      filepath.Base(b.path),
		WrittenAt: time.Now().UTC(),
		Quads:     b.writer.Written(),
		Skipped:   b.writer.Skipped(),
		Graphs:    b.graphs,
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return Summary{}, fmt.Errorf("encode manifest: %w", err)
	}
	if err := os.WriteFile(manifestPath, data, 0o644); err != nil {
		return Summary{}, fmt.Errorf("write manifest %s: %w", manifestPath, err)
	}

	return Summary{
		Loader:       b.Name(),
		Graphs:       len(b.graphs),
		QuadsWritten: b.writer.Written(),
		QuadsSkipped: b.writer.Skipped(),
		Artifacts:    []string{b.path, manifestPath},
		NextStep: fmt.Sprintf(
			"copy %s into the QLever data directory and re-index (docker stop qlever && docker exec qlever qlever index && docker start qlever)",
			b.path),
	}, nil
}

// Close closes the staging file.
func (b *BulkFile) Close() error {
	if b.file == nil {
		return nil
	}
	err := b.file.Close()
	b.file = nil
	return err
}
