package cache

import (
	"testing"
	"time"
)

// TestTTLCache replaces the former TestLabelCache in internal/sparql. That test
// reached into the label cache's internals directly (labelCache.Store with a
// hand-built entry struct), which no longer exist now that every caller shares
// one implementation. The behaviour it checked — miss, set, hit, expiry — is
// checked here against the shared cache instead; the label-specific part, the
// key, is still covered by TestLabelCacheKey in internal/sparql.
func TestTTLCache(t *testing.T) {
	c := New[string](time.Hour)

	if _, found := c.Get("absent"); found {
		t.Error("Get() should miss for an unset key")
	}

	c.Set("k", "v")
	got, found := c.Get("k")
	if !found {
		t.Fatal("Get() should hit after Set")
	}
	if got != "v" {
		t.Errorf("Get() = %q, want %q", got, "v")
	}
}

// TestTTLCacheExpiry checks that an entry past its TTL reads as a miss and is
// dropped, so correctness never depends on the background sweeper having run.
func TestTTLCacheExpiry(t *testing.T) {
	c := New[string](-time.Second) // everything is already expired

	c.Set("k", "v")
	if _, found := c.Get("k"); found {
		t.Error("Get() should miss for an expired entry")
	}
	if _, stillThere := c.entries.Load("k"); stillThere {
		t.Error("Get() should delete the expired entry it walked over")
	}
}

func TestTTLCacheSweep(t *testing.T) {
	c := New[string](time.Hour)
	c.Set("fresh", "v")

	expired := New[string](-time.Second)
	expired.Set("stale", "v")

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
