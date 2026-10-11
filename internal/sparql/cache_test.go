package sparql

import (
	"testing"
)

// The generic cache behaviour — miss, set, hit, expiry, sweep — is tested in
// internal/cache, which owns the implementation. What stays here is the part
// specific to labels: the key, and the round trip through it.

// TestBestCacheKey: the same IRI is a different entry per magic property,
// endpoint and language ORDER — ranking is language first, so [en de] and
// [de en] can resolve to different labels.
func TestBestCacheKey(t *testing.T) {
	iri := "http://example.com/thing"
	base := bestCacheKey("label", "https://a.example/sparql", iri, []string{"en", "de"})
	for name, other := range map[string]string{
		"language":       bestCacheKey("label", "https://a.example/sparql", iri, []string{"fr", "de"}),
		"language order": bestCacheKey("label", "https://a.example/sparql", iri, []string{"de", "en"}),
		"endpoint":       bestCacheKey("label", "https://b.example/sparql", iri, []string{"en", "de"}),
		"property":       bestCacheKey("description", "https://a.example/sparql", iri, []string{"en", "de"}),
	} {
		if other == base {
			t.Errorf("bestCacheKey() must distinguish %s", name)
		}
	}
}
