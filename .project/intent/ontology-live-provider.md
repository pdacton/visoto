# Ontology diagram: CONSTRUCT facade over a live endpoint

Status: **implemented** (`makeHybridProvider`, tests in `tests/js/graph-memory-store.test.js`). Not yet checked on QLever.

## Intent

The owl:Ontology diagram (`sparql-graph.js`, construct mode) keeps drawing
exactly what its CONSTRUCT produces. Past that first picture, Graph Explorer
behaves as if the triple store were behind it:

1. The initial canvas is unchanged (classes, generalizations, one association
   edge per property, attribute rows, placeholder boxes).
2. Clicking a class in the Classes pane lists its **instances** from the
   endpoint in the Instances pane.
3. The Connections menu on any node lists the diagram's edges **plus** the
   endpoint's (e.g. incoming `rdf:type` on FMIS), and placing them works.
4. Anything pulled in from the endpoint renders like in browse mode (data,
   icon, edges to what is already on the canvas).

## Why not GE's CompositeDataProvider

- `fetchAll` injects a "data provider" property row into every box, piles up
  duplicate properties (`_1`), and sums connection counts (FMIS `subClassOf`
  would read 2).
- `sequentialFetching` asks the endpoint only when the diagram's answer is
  empty, so Connections and instance search on a class node never reach it.

So a small composite of our own routes each `DataProvider` method explicitly.

## Design

New `VisotoMemoryGraph.makeHybridProvider(store, live)` in
`static/js/graph-memory-store.js`. `live` is the same `SparqlDataProvider`
browse mode builds (`VisotoGE.sparqlProvider(ENDPOINT_URL, settings)`), which
already posts through `/api/sparql?endpoint=<slug>`.

"Diagram node" = an id in `store.elements`. "Endpoint-safe" = an absolute
http(s) IRI; `urn:visoto:*` placeholders and attribute aliases never go to the
endpoint.

| Method | Source | Rule |
|---|---|---|
| `classTree`, `linkTypes` | diagram | unchanged — the facade |
| `classInfo`, `linkTypesInfo`, `propertyInfo` | diagram, endpoint for unknown ids | labels for link types/classes that only the endpoint brought in |
| `elementInfo` | diagram, endpoint for the rest | spill-over per IRI; diagram model wins, endpoint never merged into a diagram node (keeps the attribute rows clean) |
| `linksInfo` | diagram ∪ endpoint | endpoint links kept only when **at least one end is not a diagram node** — two class boxes never gain edges the CONSTRUCT did not draw |
| `linkTypesOf` | diagram ∪ endpoint | merged by link type id; counts per source, not summed, when the same edge exists in both (endpoint `rdfs:subClassOf` = diagram generalization) — take the max |
| `linkElements` | diagram ∪ endpoint | union of neighbours; diagram model for diagram nodes |
| `filter` with `refElementId` | diagram ∪ endpoint | Connections → "show elements" list |
| `filter` with `elementTypeId` (class click) | endpoint | instances of the class (intent 2) |
| `filter` text-only | diagram | unchanged |

Endpoint failures go through `kit.guardProvider` as in browse mode, but only
the `live` calls are guarded — a dead endpoint must not break the diagram.

`sparql-graph.js` construct mode: build `live` before `mountConstructed`, pass
`makeHybridProvider(store, live)` to both `kit.boot` provider hooks (fresh and
restore, so a saved layout containing pulled-in instances restores too).

Icons: pulled-in instances resolve through `typeStyleResolver` like browse
mode. The own-IRI `elementInfo` stamp from browse mode is applied to the
endpoint half as well.

## Decisions

- **D1 — class click lists instances only.** Today it lists the class and its
  subclasses (diagram nodes) so they can be dragged back after removal. The
  class can still be dragged from the tree itself. Recommended: instances only,
  per intent 2.
- **D2 — no endpoint edges between two diagram nodes** (see `linksInfo`).
  Otherwise endpoint triples such as `owl:equivalentClass` or a duplicate
  `subClassOf` would appear on the facade.
- **Out of scope:** "Add resource" endpoint search (`kit.searchEndpoint`) and
  the GL-10 superclass nesting stay off in construct mode.

## Steps

1. `makeHybridProvider` + unit tests in `tests/js/` with a stub `live`
   provider: routing table above, placeholder IRIs never forwarded, the
   diagram-node edge rule, the count merge.
2. Wire it into construct mode in `sparql-graph.js`.
3. Browser check on 8061, system-map ontology on `lindas-cached`:
   initial canvas identical; FMIS Connections shows `rdf:type` (in); class click
   lists FMIS instances; placing one draws its `rdf:type` edge to FMIS; save,
   reload, restore; RiC-O still loads (largest isDefinedBy ontology).
4. Test on the QLever endpoint too (different SparqlDataProvider behaviour),
   then `graphify update .`.

## Risks

- `filter` by type on the endpoint uses GE's OWLStats pattern; verify it
  returns instances (not subclasses) for a class with many instances, and that
  the Instances pane pages (GE's limit/offset) rather than timing out.
- A class node's `linkTypesOf` on the endpoint counts over the whole store;
  for a heavily used class this is the same query browse mode already runs.
