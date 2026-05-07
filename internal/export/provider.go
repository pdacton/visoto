package export

import (
	"errors"
	"io"

	"hutzli.org/visoto/internal/config"
)

// ErrNotApplicable is returned by a provider when it cannot handle the given endpoint
// (e.g. GraphDB provider called against a non-GraphDB URL).
// ExportWithFallback uses this sentinel to advance to the next provider.
var ErrNotApplicable = errors.New("export provider not applicable for this endpoint")

// ExportParams holds all inputs an export provider needs.
type ExportParams struct {
	GraphIRIs []string               // one or more named graph IRIs to export
	Format    string                 // MIME type, e.g. "text/turtle"
	Endpoint  *config.SparqlEndpoint // must not be nil
}

// Provider is the interface every export backend must implement.
type Provider interface {
	Name() string
	Export(params ExportParams) (io.ReadCloser, error)
}

// Registry manages the set of available export providers.
type Registry struct {
	providers map[string]Provider
}

var globalRegistry = &Registry{providers: make(map[string]Provider)}

// RegisterProvider adds p to the global registry. Called from provider init() functions.
func RegisterProvider(p Provider) {
	globalRegistry.providers[p.Name()] = p
}

// GetProvider retrieves a provider by name. Returns false if not found.
func GetProvider(name string) (Provider, bool) {
	p, ok := globalRegistry.providers[name]
	return p, ok
}

// allProviders returns providers in the canonical fallback order: graphdb → gsp → construct.
// A fixed order slice is used instead of map iteration because Go map iteration is non-deterministic.
func allProviders() []Provider {
	order := []string{"graphdb", "gsp", "construct"}
	out := make([]Provider, 0, len(order))
	for _, name := range order {
		if p, ok := globalRegistry.providers[name]; ok {
			out = append(out, p)
		}
	}
	return out
}
