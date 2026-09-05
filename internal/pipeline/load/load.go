// Package load writes minted quads into the triplestore.
//
// Two implementations sit behind one interface because QLever's two ingestion
// paths have opposite cost profiles: bulk N-Quads plus a re-index is fast and
// total, while SPARQL UPDATE is incremental but degrades query performance as
// volume grows — a caveat this project already documents in
// scripts/qlever-start-dev.sh (R-LOD-1, R-LOD-2).
package load

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"hutzli.org/visoto/internal/pipeline/rdf"
)

// Loader writes graphs into a triplestore. Implementations must be safe for a
// single run's sequential use; they are not required to be concurrent.
type Loader interface {
	// Name identifies the loader in logs and config.
	Name() string

	// BeginGraph makes graph empty, so the appends that follow define it
	// completely. Call it for a graph the run replaces (a run-scoped structure
	// graph, a source's current-graph pointers); skip it for a cumulative graph
	// the run only adds to (run provenance, signatures).
	BeginGraph(ctx context.Context, graph string) error

	// Append adds quads to graph. Splitting write from replace is what lets a
	// catalogue stream: a 100k-dataset harvest never has to be held in memory to
	// be written as one graph (R-NFR-3).
	Append(ctx context.Context, graph string, quads []rdf.Quad) error

	// Commit finalizes the run's writes and reports what a human has to do next,
	// if anything — for the bulk-file loader, that is triggering the re-index.
	Commit(ctx context.Context) (Summary, error)

	// Close releases resources. Safe to call after Commit.
	Close() error
}

// Summary is what a completed load amounted to.
type Summary struct {
	Loader       string
	Graphs       int
	QuadsWritten int64
	QuadsSkipped int64

	// Artifacts are files the loader produced, e.g. staged N-Quads.
	Artifacts []string

	// NextStep is human-readable guidance when the load is not yet visible to
	// readers, such as the re-index a bulk-file load requires. Empty when the
	// data is already queryable.
	NextStep string
}

// Factory builds a loader from its options.
type Factory func(Options) (Loader, error)

// Options carries everything a loader might need. Fields not relevant to a given
// loader are ignored.
type Options struct {
	// WorkDir is where a file-based loader stages its output.
	WorkDir string

	// RunID scopes staged filenames so concurrent or repeated runs never collide.
	RunID string

	// Endpoint, and the credentials with it, for a protocol-based loader.
	Endpoint    string
	AccessToken string
	Username    string
	Password    string
	UserAgent   string

	// BatchQuads caps how many quads go into one update request.
	BatchQuads int
}

var (
	registryMu sync.RWMutex
	registry   = make(map[string]Factory)
)

// Register adds a loader factory under a name, from package init functions.
func Register(name string, f Factory) {
	registryMu.Lock()
	defer registryMu.Unlock()
	registry[name] = f
}

// New builds the named loader.
func New(name string, opts Options) (Loader, error) {
	registryMu.RLock()
	f, ok := registry[name]
	registryMu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("unknown loader %q (known loaders: %v)", name, Names())
	}
	return f(opts)
}

// ReplaceGraph is the convenience form for a graph small enough to hold in
// memory: begin it, then write it in one call.
func ReplaceGraph(ctx context.Context, l Loader, graph string, quads []rdf.Quad) error {
	if err := l.BeginGraph(ctx, graph); err != nil {
		return err
	}
	return l.Append(ctx, graph, quads)
}

// Names lists the registered loaders, sorted.
func Names() []string {
	registryMu.RLock()
	defer registryMu.RUnlock()
	out := make([]string, 0, len(registry))
	for n := range registry {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}
