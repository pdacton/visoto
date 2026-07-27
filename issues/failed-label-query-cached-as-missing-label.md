# A failed label query is cached for 1h as if the IRI had no label

**Status:** Open
**Component:** label resolution (`internal/sparql/labels.go`)
**Pre-existing:** yes — unrelated to faceted search, but first observed while testing it

## Problem

`fetchLabelsBatch` treats two very different situations identically:

1. the label query **failed** (network error, timeout, endpoint 5xx), and
2. the query succeeded but the IRI genuinely **has no label**.

Both fall through to the same fallback — `extractLastSegment(iri)` — and both are
written into the shared label cache via `setCachedLabel` with the full
`cacheTTL = 1 * time.Hour`.

The result: one transient endpoint blip poisons labels process-wide for an hour.
Users see raw IRI tails instead of names, and nothing in the UI indicates the
labels are degraded rather than absent.

## Observed symptom

During testing, the legal-form facet dropdown showed `0107`, `0106`, `0101`
instead of `Gesellschaft mit beschränkter Haftung GMBH / SARL`, `Aktiengesellschaft`,
`Einzelunternehmen`.

The IRIs do have labels — `<https://ld.admin.ch/ech/97/legalforms/0107>` carries
`schema:name "Gesellschaft mit beschränkter Haftung GMBH / SARL"` — and the
resolver's own query returns them correctly when run directly.

Server log showed the actual cause:

```
level=WARN msg="label query failed, using fallback"
  error="failed to execute request: Post \"https://ld.admin.ch/query/\":
         read tcp ... read: connection reset by peer" iri_count=15
```

Restarting the process (clearing the in-memory cache) restored proper labels
immediately, confirming the fallback had been cached rather than re-fetched.

## Proposed fix

Distinguish the two cases in `fetchLabelsBatch`:

- **Query failed** → use the fallback for *this response* so the page still
  renders, but do **not** call `setCachedLabel` (or cache it with a much shorter
  TTL, e.g. 30s, to avoid hammering a struggling endpoint). The next request then
  retries.
- **Query succeeded, IRI absent from results** → genuine "no label"; cache the
  fallback for the full hour as today.

This mirrors the pattern already used elsewhere in the codebase for transient
failures — e.g. `cachedInstanceCount` deliberately returns `0` without caching on
error, and several handlers set `Cache-Control: no-store` when a SPARQL call fails
so a transient error is never cached as if it were real data.

## Where

`internal/sparql/labels.go` — `fetchLabelsBatch`, both early-return branches
(request error and JSON parse error) plus the trailing "apply fallback for IRIs
without labels" loop, which is the only one that should keep caching.
