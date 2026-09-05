package rdf

import (
	"bufio"
	"io"
	"strings"
)

// Quad is one statement in a named graph. An empty Graph means the default graph,
// which the pipeline never uses deliberately — every statement it writes belongs
// to a run-scoped or cumulative graph (§6.3 of the pipeline plan).
type Quad struct {
	Subject   Term
	Predicate Term
	Object    Term
	Graph     string
}

// NewQuad is the constructor the emitters use, so the argument order is stated
// in exactly one place.
func NewQuad(s, p, o Term, graph string) Quad {
	return Quad{Subject: s, Predicate: p, Object: o, Graph: graph}
}

// Valid reports whether the quad is well-formed enough to serialize. Source data
// is untrusted, so incomplete statements are dropped rather than written out as
// syntax errors that would fail a whole bulk load.
func (q Quad) Valid() bool {
	if q.Subject.IsZero() || q.Predicate.IsZero() || q.Object.IsZero() {
		return false
	}
	if q.Predicate.Kind != KindIRI {
		return false
	}
	return q.Subject.Kind != KindLiteral
}

// String renders the quad as one N-Quads line, without the trailing newline.
func (q Quad) String() string {
	var b strings.Builder
	b.WriteString(q.Subject.String())
	b.WriteByte(' ')
	b.WriteString(q.Predicate.String())
	b.WriteByte(' ')
	b.WriteString(q.Object.String())
	if q.Graph != "" {
		b.WriteByte(' ')
		b.WriteString(IRI(q.Graph).String())
	}
	b.WriteString(" .")
	return b.String()
}

// InGraph returns a copy of the quad routed to a different graph. Emitters build
// statements without knowing their destination graph; the loader assigns it.
func (q Quad) InGraph(graph string) Quad {
	q.Graph = graph
	return q
}

// Writer serializes quads as N-Quads, skipping malformed ones.
type Writer struct {
	w       *bufio.Writer
	written int64
	skipped int64
}

// NewWriter wraps w in a buffered N-Quads writer.
func NewWriter(w io.Writer) *Writer {
	return &Writer{w: bufio.NewWriterSize(w, 64*1024)}
}

// Write serializes one quad. An invalid quad is counted and skipped, not an error:
// one malformed statement from a source catalogue must not abort a whole harvest
// (R-NFR-5).
func (w *Writer) Write(q Quad) error {
	if !q.Valid() {
		w.skipped++
		return nil
	}
	if _, err := w.w.WriteString(q.String()); err != nil {
		return err
	}
	if err := w.w.WriteByte('\n'); err != nil {
		return err
	}
	w.written++
	return nil
}

// WriteAll serializes a batch, stopping at the first I/O error.
func (w *Writer) WriteAll(quads []Quad) error {
	for _, q := range quads {
		if err := w.Write(q); err != nil {
			return err
		}
	}
	return nil
}

// Written returns the number of quads serialized.
func (w *Writer) Written() int64 { return w.written }

// Skipped returns the number of malformed quads dropped.
func (w *Writer) Skipped() int64 { return w.skipped }

// Flush empties the buffer into the underlying writer.
func (w *Writer) Flush() error { return w.w.Flush() }
