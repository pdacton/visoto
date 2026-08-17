package search

import (
	"context"
	"fmt"

	"hutzli.org/visoto/internal/sparql"
)

// Provider defines the interface for different full-text search backends
type Provider interface {
	Name() string
	BuildQuery(params SearchParams) (string, error)
}

// ExecuteFunc runs a SPARQL query against the endpoint the request resolved to.
// It is the only reach a DiscoveringProvider gets over the endpoint: providers
// never hold a client of their own.
type ExecuteFunc func(ctx context.Context, query string) (sparql.QueryResult, error)

// SearchContext carries the per-request state a discovering provider needs.
//
// It exists because providers are shared singletons in the global registry (see
// RegisterProvider), so the endpoint URL, the deadline, and the ability to run a
// query MUST NOT be stored on a provider struct — two concurrent searches
// against different endpoints would overwrite each other's fields. They travel
// with the request instead.
type SearchContext struct {
	Ctx         context.Context
	Params      SearchParams
	EndpointURL string      // cache key ONLY — never used to dial
	Execute     ExecuteFunc // pre-bound to the active endpoint
}

// DiscoveringProvider is the optional capability a Provider implements when it
// must interrogate the endpoint before it can write a query — for instance
// asking a GraphDB instance which Lucene connectors exist.
//
// Searcher.Execute type-asserts for it; a provider that does not implement it is
// driven through the plain BuildQuery path, unchanged.
type DiscoveringProvider interface {
	Provider
	BuildQueryWithContext(sc SearchContext) (string, error)
}

// Registry manages available search providers
type Registry struct {
	providers map[string]Provider
	default_  string
}

var globalRegistry = &Registry{
	providers: make(map[string]Provider),
}

// RegisterProvider adds a provider to the global registry
func RegisterProvider(p Provider) {
	globalRegistry.providers[p.Name()] = p
}

// GetProvider retrieves a provider by name
func GetProvider(name string) (Provider, bool) {
	p, ok := globalRegistry.providers[name]
	return p, ok
}

// SetDefaultProvider sets the default provider name
func SetDefaultProvider(name string) error {
	if _, ok := globalRegistry.providers[name]; !ok {
		return fmt.Errorf("provider %s not registered", name)
	}
	globalRegistry.default_ = name
	return nil
}

// GetDefaultProvider returns the default provider
func GetDefaultProvider() Provider {
	if globalRegistry.default_ == "" {
		// Fallback to first registered provider if no default set
		for _, p := range globalRegistry.providers {
			return p
		}
		return nil
	}
	return globalRegistry.providers[globalRegistry.default_]
}
