package sparql

import (
	"testing"
	"time"
)

// TestTTLCache replaces the former TestLabelCache in labels_test.go. That test
// reached into the label cache's internals directly (labelCache.Store with a
// hand-built entry struct), which no longer exist now that both the label and
// type caches share one implementation. The behaviour it checked — miss, set,
// hit, expiry — is checked here against the shared cache instead, and
// TestLabelCacheKey below still covers the label-specific part: the key.
func TestTTLCache(t *testing.T) {
	c := newTTLCache[string](time.Hour)

	if _, found := c.get("absent"); found {
		t.Error("get() should miss for an unset key")
	}

	c.set("k", "v")
	got, found := c.get("k")
	if !found {
		t.Fatal("get() should hit after set")
	}
	if got != "v" {
		t.Errorf("get() = %q, want %q", got, "v")
	}
}

// TestTTLCacheExpiry checks that an entry past its TTL reads as a miss and is
// dropped, so correctness never depends on the background sweeper having run.
func TestTTLCacheExpiry(t *testing.T) {
	c := newTTLCache[string](-time.Second) // everything is already expired

	c.set("k", "v")
	if _, found := c.get("k"); found {
		t.Error("get() should miss for an expired entry")
	}
	if _, stillThere := c.entries.Load("k"); stillThere {
		t.Error("get() should delete the expired entry it walked over")
	}
}

func TestTTLCacheSweep(t *testing.T) {
	c := newTTLCache[string](time.Hour)
	c.set("fresh", "v")

	expired := newTTLCache[string](-time.Second)
	expired.set("stale", "v")

	now := time.Now()
	c.sweep(now)
	expired.sweep(now)

	if _, ok := c.entries.Load("fresh"); !ok {
		t.Error("sweep() dropped a live entry")
	}
	if _, ok := expired.entries.Load("stale"); ok {
		t.Error("sweep() kept an expired entry")
	}
}

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
