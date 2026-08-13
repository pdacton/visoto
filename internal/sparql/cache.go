package sparql

// cache.go holds the small expiring cache shared by the enrichment passes that
// run after a query returns: label resolution (labels.go) and rdf:type lookup
// (rdftypes.go).
//
// Both do the same thing — take the IRIs a result mentions, ask the endpoint one
// batched question about them, and remember the answer for a while — so they
// share one implementation and one sweeper goroutine rather than keeping a
// near-identical copy each.

import (
	"sync"
	"time"
)

const cleanupInterval = 15 * time.Minute

// ttlEntry is a cached value with its expiry.
type ttlEntry[V any] struct {
	value   V
	expires time.Time
}

// ttlCache is a concurrent map whose entries expire.
//
// Expired entries are dropped on read, so correctness never depends on the
// sweeper; the sweeper exists only to stop entries that are never read again
// from pinning memory forever.
type ttlCache[V any] struct {
	entries sync.Map // map[string]ttlEntry[V]
	ttl     time.Duration
}

// newTTLCache returns a cache and registers it with the shared sweeper. Call it
// from a package-level var so registration happens before any request runs.
func newTTLCache[V any](ttl time.Duration) *ttlCache[V] {
	c := &ttlCache[V]{ttl: ttl}
	registerCache(c)
	return c
}

func (c *ttlCache[V]) get(key string) (V, bool) {
	var zero V
	raw, ok := c.entries.Load(key)
	if !ok {
		return zero, false
	}
	entry, ok := raw.(ttlEntry[V])
	if !ok {
		return zero, false
	}
	if time.Now().After(entry.expires) {
		c.entries.Delete(key)
		return zero, false
	}
	return entry.value, true
}

func (c *ttlCache[V]) set(key string, value V) {
	c.entries.Store(key, ttlEntry[V]{value: value, expires: time.Now().Add(c.ttl)})
}

// sweep drops every entry that expired before now.
func (c *ttlCache[V]) sweep(now time.Time) {
	c.entries.Range(func(key, raw any) bool {
		if entry, ok := raw.(ttlEntry[V]); ok && now.After(entry.expires) {
			c.entries.Delete(key)
		}
		return true
	})
}

// ---- shared sweeper ----

// sweepable is what the cleanup goroutine needs from a cache; it exists so the
// registry can hold ttlCaches of different value types in one slice.
type sweepable interface {
	sweep(now time.Time)
}

var (
	cachesMu    sync.Mutex
	caches      []sweepable
	cleanupOnce sync.Once
)

func registerCache(c sweepable) {
	cachesMu.Lock()
	defer cachesMu.Unlock()
	caches = append(caches, c)
}

// initCaches starts the background sweeper, once per process. Called from the
// query path rather than an init() so a process that never queries (tests, the
// index builder) does not spawn a goroutine it has no use for.
func initCaches() {
	cleanupOnce.Do(func() {
		go cacheCleanup()
	})
}

func cacheCleanup() {
	ticker := time.NewTicker(cleanupInterval)
	defer ticker.Stop()

	for range ticker.C {
		now := time.Now()
		// Snapshot under the lock, sweep outside it: sweeping walks whole maps
		// and must not block a cache being registered.
		cachesMu.Lock()
		current := make([]sweepable, len(caches))
		copy(current, caches)
		cachesMu.Unlock()

		for _, c := range current {
			c.sweep(now)
		}
	}
}
