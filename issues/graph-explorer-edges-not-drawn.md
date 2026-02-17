# Graph Explorer: Edges not drawn with 4+ nodes

**Status:** Open
**Component:** Graph Explorer (Ontodia fork)
**Library:** [zazuko/graph-explorer](https://github.com/zazuko/graph-explorer) v1.3.0
**Also affects:** https://lindas.admin.ch/graph-explorer/

## Problem

When more than 3 nodes are on the canvas, edges/links stop being drawn. No error is visible to the user; the graph simply shows unconnected nodes.

## Root cause

The library's `linksInfo()` method constructs a SPARQL query with **two VALUES clauses** — one for source, one for target — using the same set of element IRIs:

```sparql
SELECT ?source ?type ?target
WHERE {
    { ?source ?type ?target }
    VALUES (?source) { (<iri1>) (<iri2>) (<iri3>) (<iri4>) ... }
    VALUES (?target) { (<iri1>) (<iri2>) (<iri3>) (<iri4>) ... }
}
```

This creates a Cartesian product on a fully unbound triple pattern. The LINDAS endpoint (Stardog) crashes with a `java.lang.StackOverflowError` when there are **4 or more IRIs** in the VALUES clauses. The HTTP response is a 500 with:

```json
{
  "message": "com.complexible.stardog.plan.eval.ExecutionException: java.lang.StackOverflowError"
}
```

With 3 IRIs (3x3 = 9 combinations) the query succeeds. With 4 IRIs (4x4 = 16 combinations) it fails.

## Where the query is defined

- **Template:** `OWLStatsSettings.linksInfoQuery` in [sparqlDataProviderSettings.ts:350-355](https://github.com/zazuko/graph-explorer/blob/master/src/graph-explorer/data/sparql/sparqlDataProviderSettings.ts)
- **Called from:** `linksInfo()` in [sparqlDataProvider.ts ~line 434](https://github.com/zazuko/graph-explorer/blob/master/src/graph-explorer/data/sparql/sparqlDataProvider.ts)
- **Template variables:** `${ids}` (IRI list, used for both source and target VALUES) and `${linkConfigurations}` (triple pattern, defaults to `{ ?source ?type ?target }`)

## Verified behaviour

```
3 IRIs in dual VALUES  -> HTTP 200 (works)
4 IRIs in dual VALUES  -> HTTP 500 (StackOverflowError)
6 IRIs in dual VALUES  -> HTTP 500
8 IRIs in dual VALUES  -> HTTP 500
1 bound source + VALUES target (8 IRIs) -> HTTP 200 (works)
VALUES source (8 IRIs) + no target constraint -> HTTP 200 (works)
Single VALUES (?source ?target) with explicit pairs -> HTTP 200 (works)
```

## Possible fixes

### 1. Override `linksInfoQuery` to drop the target VALUES

```js
settings.linksInfoQuery = `
  SELECT ?source ?type ?target WHERE {
    \${linkConfigurations}
    VALUES (?source) {\${ids}}
  }
`;
```

This removes the target constraint entirely. The query returns all outgoing links from the source elements. This works if the response handler or model filters out links to elements not on the canvas. **Needs verification** — may return too many results on large datasets.

### 2. Override `linksInfo()` to issue per-element queries

Override the data provider's `linksInfo` method to query each element individually (N queries instead of 1), then merge and deduplicate results. Each query has only 1 IRI in the source VALUES, so it stays within Stardog's limit. The downside is O(N) HTTP requests.

### 3. Use combined VALUES pairs

Instead of two separate VALUES clauses, use a single `VALUES (?source ?target)` with explicit pairs. This avoids the Cartesian product. However, the template variable `${ids}` is a flat list and can't be easily expanded into pairs without patching the library itself.

### 4. File upstream issue

Report on [github.com/zazuko/graph-explorer](https://github.com/zazuko/graph-explorer/issues) since this affects all Stardog-backed deployments including the official LINDAS graph explorer.

## Current state in codebase

A partial workaround is in `templates/pages/ontodia.html` (monkey-patch on `dataProvider.linksInfo`) but it combines batches of 3 into groups of 6, which still exceeds Stardog's limit. **The workaround does not work yet** and needs to be replaced with one of the approaches above.
