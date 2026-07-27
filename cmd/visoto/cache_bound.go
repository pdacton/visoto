package main

import "time"

// Bounded in-process TTL caches.
//
// The per-process caches in this package (instance counts, facet value lists) are
// keyed partly on request input — the class IRI, the search term, Accept-Language.
// A plain map therefore grows without limit: expired entries are only ever
// overwritten when the SAME key is requested again, so a caller varying the key
// mints a permanent entry each time. That is both a slow memory leak under normal
// traffic and an unbounded-growth lever for an untrusted caller.
//
// sweepExpired keeps them bounded with no extra dependency or goroutine: it is
// called on the write path, drops everything already past its TTL, and reports
// whether there is room to insert. Callers hold the cache's mutex.

// maxCacheEntries caps any one of these caches. Well past what real browsing
// produces (a handful of classes × languages), small enough that a hostile caller
// can't grow the process.
const maxCacheEntries = 1000

// expiringEntry is implemented by the cache entry types so one sweep serves all.
type expiringEntry interface{ expiredAt(now time.Time) bool }

// sweepExpired deletes every entry whose TTL has passed and reports whether the
// map has room for one more. Must be called with the cache's mutex held.
func sweepExpired[T expiringEntry](cache map[string]T) bool {
	now := time.Now()
	for k, v := range cache {
		if v.expiredAt(now) {
			delete(cache, k)
		}
	}
	return len(cache) < maxCacheEntries
}
