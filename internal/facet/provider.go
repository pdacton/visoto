package facet

import "fmt"

// FacetProvider is the store-specific seam of faceted search. The core query
// assembly (VALUES/IN, typed ranges, DISTINCT enumeration) is portable and lives
// in builder.go; a provider owns only the pieces where a store's index changes
// the query text:
//
//   - EnumerateQuery: Phase-A value enumeration (a store may have a cheaper
//     count/estimate path than the portable GROUP BY).
//   - TextMatchClause: the free-text leg (portable CONTAINS, or a native FTS
//     SERVICE such as GraphDB onto:fts / QLever textSearch:).
//
// v1 registers only BaseFacetProvider; per-store overrides slot in behind this
// interface exactly like the internal/search providers.
type FacetProvider interface {
	Name() string
	EnumerateQuery(classIRI, keyVar string, spec FacetSpec, limit int) (string, error)
	TextMatchClause(fv, term string) (string, error)
}

// BaseFacetProvider is the portable SPARQL 1.1 implementation. It works on every
// store and is the fallback whenever a store-specific provider is not registered.
type BaseFacetProvider struct{}

func (BaseFacetProvider) Name() string { return "sparql-query" }

func (BaseFacetProvider) EnumerateQuery(classIRI, keyVar string, spec FacetSpec, limit int) (string, error) {
	return BuildFacetValuesQuery(classIRI, keyVar, spec, limit)
}

func (BaseFacetProvider) TextMatchClause(fv, term string) (string, error) {
	return PortableTextMatch(fv, term), nil
}

// ---- registry (mirrors internal/search) ----

var (
	providers       = map[string]FacetProvider{}
	defaultProvider = "sparql-query"
)

// RegisterProvider adds p to the global registry.
func RegisterProvider(p FacetProvider) { providers[p.Name()] = p }

// GetProvider returns the provider registered under name.
func GetProvider(name string) (FacetProvider, bool) {
	p, ok := providers[name]
	return p, ok
}

// SetDefaultProvider selects the provider returned by Default. It errors if name
// is not registered so a misconfigured facet_provider fails loudly at startup.
func SetDefaultProvider(name string) error {
	if _, ok := providers[name]; !ok {
		return fmt.Errorf("facet provider %q not registered", name)
	}
	defaultProvider = name
	return nil
}

// Default returns the configured default provider, or BaseFacetProvider if none
// was set.
func Default() FacetProvider {
	if p, ok := providers[defaultProvider]; ok {
		return p
	}
	return BaseFacetProvider{}
}

func init() { RegisterProvider(BaseFacetProvider{}) }
