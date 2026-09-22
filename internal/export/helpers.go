package export

import (
	"io"
)

// multiReadCloser wraps io.MultiReader with a Close() that closes all underlying bodies.
type multiReadCloser struct {
	r       io.Reader
	closers []io.Closer
}

func (m *multiReadCloser) Read(p []byte) (int, error) { return m.r.Read(p) }

func (m *multiReadCloser) Close() error {
	var firstErr error
	for _, c := range m.closers {
		if err := c.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// ExtForMIME returns a file extension for a given RDF MIME type.
func ExtForMIME(mime string) string {
	switch mime {
	case "text/turtle":
		return ".ttl"
	case "application/n-quads":
		return ".nq"
	case "application/n-triples":
		return ".nt"
	case "application/rdf+xml":
		return ".rdf"
	case "application/ld+json":
		return ".jsonld"
	case "application/trig":
		return ".trig"
	default:
		return ".rdf"
	}
}
