# Ontology diagram — implementation plan

Implements [ontology-construct.md](ontology-construct.md). Requirement IDs
(M5, J8, …) refer to that file.

**User gates (moderate):** the plan pauses for approval at the four
🚦 gates below, one at the end of each phase. Within a phase, work proceeds
without asking unless a requirement turns out to be unimplementable or data
contradicts an assumption. At each gate, the user gets: what changed,
before/after screenshots of the diagrams, timings against the 5 s budget (C3), and
the decisions made along the way.

Testing rules for every phase:
- Dev server on `PORT=8061`, never touching 8060; restart after template changes.
- Verify in Playwright after a hard cache clear (CDP `Network.clearBrowserCache`).
- Every query is run on **LINDAS prod and QLever** (C2) and timed.
- `go test ./...` and `node --test tests/js/` pass.

---

## Phase 0 — Survey and baseline (no code changes) — done

1. Find one test ontology per source on LINDAS and QLever:
   - isDefinedBy (RiC-O, QUDT), rdfs:member (DCAT-AP CH), namespace fallback
     (OWL Time, agriculture schemas) — already known;
   - schema.org `domainIncludes`/`rangeIncludes`;
   - OWL restrictions (`owl:onProperty` with all/some values, cardinalities);
   - SHACL NodeShapes reachable via M1/M2/M4.
   The metadata-swiss files (requirements §9) are the primary fixtures for
   profiles, imports and SHACL (the MDS files only once their owner adds
   membership, Q7): load each into its own
   named graph on the local test endpoint, and check whether any of them is
   already on LINDAS (`register.ld.admin.ch/mds/…`).
2. Check for the deferred features (§8): enumerations, `hasKey`, `hasValue`,
   custom datatypes, `sh:inversePath`, `sh:targetSubjectsOf`.
3. Confirm both engines return blank nodes from a CONSTRUCT template (fresh per
   solution), which the use records rely on.
4. Record the current query's timing and node/edge counts per test ontology on
   both endpoints (baseline).

Output: a "Test ontologies" table appended to this file.

🚦 **Gate 0** — approve the test set; decide per deferred feature whether data
justifies it, and whether a source with no data gets a small Turtle fixture on
the local test endpoint or is dropped.

## Phase 1 — Normalize/project split, same diagram — done

Goal: rebuild today's diagram on the new architecture with no visible change.

1. `graph-memory-store.js`: `parseNTriples` accepts blank-node subjects and
   objects.
2. New `static/js/ontology-projection.js` (loaded before `sparql-graph.js`):
   members, kinds, subClassOf and uses in → the existing store contract out.
   Implements J1–J7, J11, J12 for the RDFS source.
   Membership gains `owl:imports` (M6) here, since it changes the member set
   every query shares.
3. `templates/partials/sparql-graph.html` + `static/js/sparql-graph.js`:
   `construct` accepts a list of queries, posted in parallel; results are
   concatenated, passed through a projection hook when the island names one
   (`ontology`), then `buildStore`. Any query failing fails the diagram with
   the existing Retry; an empty result is not an error.
4. `templates/instances/owl%3AOntology.html`: membership subselect in one
   `$members` variable (M5); today's CONSTRUCT rewritten as the "core" query
   emitting §3's normalized form. Placeholder, literal and external logic
   leaves the SPARQL.
5. `tests/js/ontology-projection.test.js`: one case per J-rule, plus
   blank-node parsing.

Check: the RiC-O, QUDT, DCAT-AP CH, OWL Time and PROV diagrams match the
current ones (element/link counts, screenshots). The expected difference is Q5
(untyped namespace terms become external); list every occurrence.

🚦 **Gate 1** — approve the architecture change before new sources are added.

## Phase 2 — Core additions — done

1. schema.org `domainIncludes`/`rangeIncludes` uses (§4) and schema.org
   datatypes (J3).
2. Extra label sources (S3).
3. `owl:FunctionalProperty` → max 1, plus J8 (merge) and J9 (multiplicity
   labels, per-edge link types) in the projection, with tests. Recheck that
   `/_schema.html` still renders (it shares `graph-memory-store.js`).
4. Time it; if the core query exceeds its share of the budget, move schema.org
   into its own query.

🚦 **Gate 2** — review schema.org diagrams and how multiplicity looks.

## Phase 3 — OWL restrictions query — done

1. "owl" query: `allValuesFrom`, `someValuesFrom`, (qualified) cardinality →
   uses (§4).
2. Projection tests for OWL-vs-RDFS merging (J8).

🚦 **Gate 3** — review on the restriction test ontology.

## Phase 4 — SHACL query — done

1. "shacl" query: NodeShape target → class; `sh:property` with simple
   `sh:path` (S2); `sh:class`, `sh:node`, `sh:datatype`, `sh:or`/`sh:xone`,
   `sh:minCount`/`sh:maxCount`, `sh:name`.
   Severity filter (S4), `sh:nodeKind sh:Literal`, `sh:node` resolution (S5),
   skolem IRIs (S2).
2. Projection: J1 for shapes, J10 (`sh:name`), the J8 merge rules (a–c), with
   tests built from the DCAT-AP CH v3 shapes + range shapes pair.
3. Reverse imports (M7): shapes documents that import `O` join its diagram.
4. Enumerations (J13): `owl:oneOf` (in the owl query) and `sh:in`.

🚦 **Gate 4** — final review: all test ontologies on both endpoints, full
timing table.

## Phase 5 — Wrap-up (no gate) — done 2026-10-14

1. Rewrite the template's header comment for the new architecture.
2. `docs/templating.md`: the list form of `sparqlGraph`'s `construct` and the
   projection hook.
3. Mark requirements **[done]** in `ontology-construct.md`.
4. `graphify update .`; one commit per phase, no push without asking.

---

## Risks

- **Blank nodes:** if either engine mishandles template blank nodes (Phase 0,
  step 3), uses switch to minted IRIs `urn:visoto:use:<hash>` built with
  `IRI(CONCAT(…))`; the projection does not care which.
- **Budget:** the restriction and SHACL queries walk blank-node lists
  (`rdf:rest*/rdf:first`). If one exceeds the budget, split it further rather
  than loosen C3.
- **Shared layer:** per-edge link types (J9) touch `graph-memory-store.js`,
  which the schema diagram also uses; recheck it at Gate 2.

---

## Phase 0 results (2026-10-13)

### Test ontologies

| Ontology | Endpoint | Exercises | Baseline (current query) |
|---|---|---|---|
| RiC-O `https://www.ica.org/standards/RiC/ontology` | LINDAS | M1, restrictions (123), size reference | 1440 triples, 0.6 s |
| QUDT schema `http://qudt.org/schema/qudt` | LINDAS | M1, restrictions (381), `owl:oneOf` (12), `owl:imports` (5) | 852 triples, 0.3 s |
| QUDT units `http://qudt.org/vocab/unit` | LINDAS | M1 with 2935 non-schema members (units), `owl:imports` (4) | 18313 triples, 2.0 s — mostly labels of unit individuals |
| OWL Time `http://www.w3.org/2006/time` | LINDAS | M4 fallback, restrictions (52) | 281 triples, 0.2 s |
| PROV `http://www.w3.org/ns/prov#` | LINDAS | M1, re-declared `rdf:type`/`rdfs:label` (P3), imports (6) | 165 triples, 0.1 s |
| plant-protection shapes `https://agriculture.ld.admin.ch/plant-protection/shape/` | LINDAS | **real SHACL via M4**: 28 NodeShapes, targetClass, `sh:class`/`sh:datatype`/`sh:or`/`sh:in`/`sh:node`/min/maxCount, 2 untargeted shapes, a meta-shape targeting `owl:Class` | 2 triples (SHACL not supported yet) |
| plant-protection `https://agriculture.ld.admin.ch/plant-protection/` | LINDAS | M4, restrictions (10); its namespace also covers `…/shape/` | 117 triples, 0.2 s |
| MDS model `https://register.ld.admin.ch/mds/ontology/ist` | QLever | M1 (now has isDefinedBy, 112), domain/range leak (Q7) | 439 triples, 1.4 s |
| DCAT-AP 3.0.0 / 2.1.0 | QLever | M2, no domain/range, `rangeIncludes` datatypes | 443 / 436 triples, 0.5 s |
| DCAT-AP CH v2/v3 + all shapes files | QLever | M6, M7, S2–S5, J8b | **not uploaded yet** |

LINDAS has **no** `rdfs:member` at all and no `owl:Ontology` for schema.org or
`schema.ld.admin.ch` (where all 2486 `schema:domainIncludes` live), so
`domainIncludes`/`rangeIncludes` is exercised only by the DCAT-AP fixtures.
RiC-O is not on QLever, so the 5 s budget on QLever is measured on the
largest fixture instead.

### Deferred features (§8) — data found

| Feature | LINDAS usage |
|---|---|
| `owl:oneOf` | 13 (QUDT schema) |
| `owl:hasValue` | 13 |
| `sh:in` | 3 (plant-protection shapes) |
| `owl:onDatatype` / `withRestrictions` | 3 |
| `owl:InverseFunctionalProperty` | 1 |
| `owl:hasKey`, `sh:inversePath`, `sh:targetSubjectsOf` | 0 |
| untargeted NodeShapes | 2 (plant-protection) |

### Engine checks

- Template blank nodes: fresh per solution on both GraphDB and QLever. **But
  QLever names them per response (`_:u0_u`, `_:u1_u`, …), so two parallel
  responses reuse the same labels.** The parser must prefix blank-node labels
  per response before concatenating (Phase 1, step 1).
- The `/api/sparql` proxy requires `Content-Type: application/sparql-query`.

### Gate 0 decisions

Approved: imports refinement (M6), enumerations in Phase 4 (J13), Phase 1
starts before the remaining uploads.

### Findings for Gate 0

1. **`owl:imports` closure (M6) is too wide as written.** QUDT units imports the
   QUDT schema; following it would draw the whole QUDT schema on every QUDT
   vocabulary page. Proposed refinement: imported terms count as *known* (not
   external, their axioms apply), but only `O`'s own members and the uses of
   its shapes (M7) put boxes on the diagram.
2. **Labels only for drawn IRIs.** The QUDT units baseline is 18k triples
   because every member's labels are fetched. In the new architecture the label
   step is restricted to classes and properties, which fixes this.
