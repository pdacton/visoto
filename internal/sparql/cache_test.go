package sparql

import (
	"testing"
)

// The generic cache behaviour — miss, set, hit, expiry, sweep — is tested in
// internal/cache, which owns the implementation. What stays here is the part
// specific to labels: the key, and the round trip through it.

// TestLabelCacheKey covers what is specific to labels: the same IRI under
// different language preferences is a different cache entry, and equivalent
// preference sets in a different order are the same one.
func TestLabelCacheKey(t *testing.T) {
	iri := "http://example.com/thing"

	if labelCacheKey(iri, []string{"en"}) == labelCacheKey(iri, []string{"de"}) {
		t.Error("labelCacheKey() must distinguish languages")
	}
	if labelCacheKey(iri, []string{"en", "de"}) != labelCacheKey(iri, []string{"de", "en"}) {
		t.Error("labelCacheKey() must be order-independent")
	}
}

func TestLabelCacheRoundTrip(t *testing.T) {
	iri := "http://example.com/label-cache"
	langs := []string{"en", "de"}

	if _, found := getCachedLabel(iri, langs); found {
		t.Error("getCachedLabel() should miss for an uncached IRI")
	}

	setCachedLabel(iri, "Test Label", langs)

	label, found := getCachedLabel(iri, langs)
	if !found {
		t.Fatal("getCachedLabel() should hit after set")
	}
	if label != "Test Label" {
		t.Errorf("getCachedLabel() = %q, want %q", label, "Test Label")
	}

	// A different language preference is a different entry, not a stale hit.
	if _, found := getCachedLabel(iri, []string{"fr"}); found {
		t.Error("getCachedLabel() should miss for a different language set")
	}
}
