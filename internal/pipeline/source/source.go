// Package source defines the catalogue ingestion interface and its registry.
//
// The load-bearing decision here is that adapters emit DCAT-AP, never their
// native shape (R-SRC-1..6). opendata.swiss speaks CKAN JSON, data.europa.eu
// speaks SPARQL, a future source may speak something else again; everything
// downstream of this package sees one vocabulary. Adding a source is a new
// package plus one Register call, with no change anywhere else.
package source

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"hutzli.org/visoto/internal/config"
	"hutzli.org/visoto/internal/pipeline/rdf"
)

// Distribution is one downloadable file or API, extracted from the DCAT-AP the
// adapter emitted. It exists as a struct — rather than being re-read out of the
// quads — because it is the fetch stage's work item and lands in SQLite as one
// row.
type Distribution struct {
	IRI         string
	DatasetIRI  string
	DownloadURL string

	// DeclaredMedia is dcat:mediaType as the portal states it. Advisory only:
	// portal-declared media types are wrong often enough that detection is by
	// content, and a mismatch is itself a quality signal (R-SNF-1, R-SNF-2).
	DeclaredMedia  string
	DeclaredFormat string
	ByteSize       int64
	Licence        string
	Modified       time.Time
}

// Record is one dcat:Dataset with its distributions, already normalized to
// DCAT-AP. Quads carry the metadata verbatim for the catalogue graph (R-CAT-1);
// Distributions is the same information reduced to what later stages need.
type Record struct {
	DatasetIRI    string
	Quads         []rdf.Quad
	Distributions []Distribution
}

// EmitFunc receives each harvested record. Returning an error aborts the
// harvest, which is how the runner propagates a cancelled context or a failed
// load without every adapter re-implementing that check.
type EmitFunc func(Record) error

// Source harvests one catalogue.
type Source interface {
	// Name is the configured source name; it reaches graph IRIs and state keys.
	Name() string

	// Harvest emits every dataset modified at or after since. A zero since means
	// a full harvest. Adapters must respect ctx cancellation between pages.
	Harvest(ctx context.Context, since time.Time, emit EmitFunc) error
}

// Factory builds a Source from its configuration. Adapters register one per
// type they implement.
type Factory func(cfg config.PipelineSource, opts Options) (Source, error)

// Options carries the pipeline-wide settings an adapter needs but should not
// read from the global config itself, so adapters stay unit-testable.
type Options struct {
	UserAgent string
	Minter    *rdf.Minter
}

var (
	registryMu sync.RWMutex
	registry   = make(map[string]Factory)
)

// Register adds a factory under a type name. It is called from adapter package
// init functions, mirroring the provider registry in internal/export.
func Register(typeName string, f Factory) {
	registryMu.Lock()
	defer registryMu.Unlock()
	registry[typeName] = f
}

// New builds the adapter named by cfg.Type.
func New(cfg config.PipelineSource, opts Options) (Source, error) {
	registryMu.RLock()
	f, ok := registry[cfg.Type]
	registryMu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("unknown source type %q for source %q (known types: %v)", cfg.Type, cfg.Name, Types())
	}
	return f(cfg, opts)
}

// Types lists the registered adapter types, sorted, for error messages and the
// CLI's `sources` command.
func Types() []string {
	registryMu.RLock()
	defer registryMu.RUnlock()
	out := make([]string, 0, len(registry))
	for t := range registry {
		out = append(out, t)
	}
	sort.Strings(out)
	return out
}
