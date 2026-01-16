package search

import "fmt"

// Provider defines the interface for different full-text search backends
type Provider interface {
	Name() string
	BuildQuery(params SearchParams) (string, error)
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
