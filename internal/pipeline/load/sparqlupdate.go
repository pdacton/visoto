package load

import (
	"context"
	"fmt"
	"strings"

	"hutzli.org/visoto/internal/config"
	"hutzli.org/visoto/internal/pipeline/rdf"
	"hutzli.org/visoto/internal/pipeline/sparqlio"
)

func init() {
	Register(config.LoaderSparqlUpdate, newSparqlUpdate)
}

// SparqlUpdate writes graphs over the SPARQL protocol. Use it for incremental
// top-ups between re-indexes, not for a full harvest: QLever's query performance
// degrades with UPDATE volume (R-LOD-2).
type SparqlUpdate struct {
	client     *sparqlio.Client
	batchQuads int
	graphs     int
	written    int64
	skipped    int64
}

func newSparqlUpdate(opts Options) (Loader, error) {
	if strings.TrimSpace(opts.Endpoint) == "" {
		return nil, fmt.Errorf("sparql-update loader needs a target endpoint")
	}
	clientOpts := []sparqlio.Option{sparqlio.WithUserAgent(opts.UserAgent)}
	switch {
	case opts.AccessToken != "":
		clientOpts = append(clientOpts, sparqlio.WithBearer(opts.AccessToken))
	case opts.Username != "":
		clientOpts = append(clientOpts, sparqlio.WithBasicAuth(opts.Username, opts.Password))
	}
	batch := opts.BatchQuads
	if batch <= 0 {
		batch = 5000
	}
	return &SparqlUpdate{
		client:     sparqlio.NewClient(opts.Endpoint, clientOpts...),
		batchQuads: batch,
	}, nil
}

// Name returns the loader name.
func (s *SparqlUpdate) Name() string { return config.LoaderSparqlUpdate }

// BeginGraph drops the graph so the appends that follow define it completely.
//
// A crash between the drop and the first append leaves the graph empty rather
// than stale, which is the safe failure for a run-scoped graph: readers follow
// the current-graph pointer, which still names the previous run's graph.
func (s *SparqlUpdate) BeginGraph(ctx context.Context, graph string) error {
	safeGraph, ok := sparqlio.EscapeIRI(graph)
	if !ok {
		return fmt.Errorf("refusing to write malformed graph IRI %q", graph)
	}
	if err := s.client.Update(ctx, "DROP SILENT GRAPH <"+safeGraph+">"); err != nil {
		return fmt.Errorf("drop graph %s: %w", graph, err)
	}
	s.graphs++
	return nil
}

// Append inserts the quads into graph, in batches.
func (s *SparqlUpdate) Append(ctx context.Context, graph string, quads []rdf.Quad) error {
	if graph == "" {
		return fmt.Errorf("sparql-update loader requires a named graph")
	}
	safeGraph, ok := sparqlio.EscapeIRI(graph)
	if !ok {
		return fmt.Errorf("refusing to write malformed graph IRI %q", graph)
	}

	batch := make([]rdf.Quad, 0, s.batchQuads)
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		if err := s.insert(ctx, safeGraph, batch); err != nil {
			return err
		}
		batch = batch[:0]
		return nil
	}

	for _, q := range quads {
		q = q.InGraph(graph)
		if !q.Valid() {
			s.skipped++
			continue
		}
		batch = append(batch, q)
		if len(batch) >= s.batchQuads {
			if err := flush(); err != nil {
				return err
			}
		}
	}
	return flush()
}

// insert sends one INSERT DATA request. Terms are serialized through the same
// N-Triples writer the bulk path uses, so both loaders escape identically.
func (s *SparqlUpdate) insert(ctx context.Context, safeGraph string, quads []rdf.Quad) error {
	var b strings.Builder
	b.Grow(len(quads) * 128)
	b.WriteString("INSERT DATA { GRAPH <")
	b.WriteString(safeGraph)
	b.WriteString("> {\n")
	for _, q := range quads {
		b.WriteString(q.Subject.String())
		b.WriteByte(' ')
		b.WriteString(q.Predicate.String())
		b.WriteByte(' ')
		b.WriteString(q.Object.String())
		b.WriteString(" .\n")
	}
	b.WriteString("} }")

	if err := s.client.Update(ctx, b.String()); err != nil {
		return fmt.Errorf("insert %d quads into %s: %w", len(quads), safeGraph, err)
	}
	s.written += int64(len(quads))
	return nil
}

// Commit reports the totals. The data is already visible, so there is no next step.
func (s *SparqlUpdate) Commit(ctx context.Context) (Summary, error) {
	return Summary{
		Loader:       s.Name(),
		Graphs:       s.graphs,
		QuadsWritten: s.written,
		QuadsSkipped: s.skipped,
	}, nil
}

// Close is a no-op; the HTTP client needs no teardown.
func (s *SparqlUpdate) Close() error { return nil }
