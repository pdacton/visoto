package sparql

// options.go carries the per-query switches that only some callers want.
//
// Variadic on purpose. ExecuteQuery / ExecuteQueryWithContext have a dozen call
// sites across two packages, and only the three table handlers need type
// resolution — search and the MCP tools resolve labels but have no icon column
// and must not pay for a second round trip. A variadic option leaves every other
// call site compiling untouched.

// Option adjusts how a single query is executed.
type Option func(*options)

type options struct {
	// resolveTypes additionally fetches rdf:type for the IRIs in the result and
	// resolves each to a resource icon (QueryResult.Icons). Only meaningful
	// together with label resolution, which is what harvests the IRIs.
	resolveTypes bool
}

// WithTypes asks for rdf:type resolution so the result carries resource icons.
// Pass it only when something will actually render them — it costs one extra
// (cached, batched, parallel) query per uncached IRI set.
func WithTypes() Option {
	return func(o *options) { o.resolveTypes = true }
}

func newOptions(opts []Option) options {
	var o options
	for _, opt := range opts {
		if opt != nil {
			opt(&o)
		}
	}
	return o
}
