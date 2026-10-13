# Ontology diagram — CONSTRUCT requirements

Intent: an ontology (or application profile, or SHACL shapes graph) stored in
the triple store can be queried and visualized as a UML-style class diagram in
Graph Explorer, on the `owl:Ontology` instance page
(`templates/instances/owl%3AOntology.html`).

**Architecture: query normalizes, JS draws.** A few CONSTRUCT queries (one per
source vocabulary, run in parallel) reduce every way of describing a property
to one flat record, a *use*. A JS projection step turns members and uses into the
diagram. SPARQL only extracts facts; every drawing decision (datatype vs class,
placeholders, merging, multiplicity labels, external boxes) is made in JS, where
it is unit-testable and engine-independent.

Status tags: **[done]** = what the current query does, **[new]** = to build,
**[later]** = deferred until real data shows up (see §9), **[out]** = not
drawn. `O` below is the ontology IRI (`??` in the query).

---

## 1. Runtime constraints

| ID | Requirement | Status |
|---|---|---|
| C1 | Queries run in the browser, POSTed to `/api/sparql?endpoint=<slug>`. The proxy does no prefix expansion and no `visoto:dispLang`, so queries declare their own `PREFIX`es and language is chosen in JS. `??` is substituted client-side. | done |
| C2 | Same result on GraphDB (LINDAS) and QLever. A `{ … }` group must not rely on an outer variable being bound (QLever evaluates bottom-up), so every query carries the membership subselect itself. | done |
| C3 | Budget: the whole RiC-O diagram (~106 classes, ~535 associations, ~82 attributes, ~111 generalizations) renders in **≤ 5 s** on LINDAS prod and QLever. A query whose source is absent (no shapes, no restrictions) returns fast and empty. | new |
| C4 | Queries stay POST (long CONSTRUCTs exceed the GET URL limit). | done |

## 2. Membership — which terms belong to `O`

| ID | Requirement | Status |
|---|---|---|
| M1 | `t` is a member when `t rdfs:isDefinedBy O`. | done |
| M2 | `t` is a member when `O rdfs:member t` — for application profiles (DCAT-AP, DCAT-AP CH) that reuse terms whose `rdfs:isDefinedBy` points to another vocabulary. | done |
| M3 | M1 and M2 are alternatives combined by union: either, both, or mixed per term. | done |
| M4 | Fallback, **only if `O` asserts neither M1 nor M2 for any term**: a member is an IRI ≠ `O`, starting with `O`, typed `owl:Class`, `rdfs:Class`, `owl:ObjectProperty`, `owl:DatatypeProperty`, `owl:AnnotationProperty`, `rdf:Property`, `sh:NodeShape` or `sh:PropertyShape`. Only typed terms are scanned, never the whole store. | done (shapes new) |
| M5 | The membership subselect is written once (a Go template variable) and spliced into every query, so all queries and the "Defined Terms" table agree. | new |
| M6 | `owl:imports` is followed transitively: members of an imported ontology are members of `O`. (DCAT-AP CH v3 imports DCAT-AP 3.0.0; every shapes document imports its vocabulary.) | new |
| M7 | Shapes documents that import `O` (an `owl:Ontology` whose members are `sh:NodeShape`s and which `owl:imports O`) contribute their shapes to `O`'s diagram. Needed because profile vocabularies (DCAT-AP, DCAT-AP CH) deliberately carry no `rdfs:domain`/`rdfs:range`: all structure lives in the shapes. | new (Q6) |

## 3. Query output — the normalized form

Each query emits only these triples (`u:` = `urn:visoto:`):

| Triple | Meaning |
|---|---|
| `?t a u:Member` | `t` is a member of `O` |
| `?t a ?kind` | the term's type (`owl:Class`, `rdfs:Class`, `owl:ObjectProperty`, `owl:DatatypeProperty`, `owl:AnnotationProperty`, `rdf:Property`) |
| `?sub rdfs:subClassOf ?super` | a declared generalization (IRIs only, `owl:Thing` excluded) |
| `_:u a u:Use ; u:class ?C ; u:property ?P ; u:range ?R ; u:min ?n ; u:max ?n ; u:source "rdfs"\|"schema"\|"owl"\|"shacl" ; rdfs:label ?name` | one use of property `P`: class `C` (may be absent), range `R` (class or datatype, may be absent), optional multiplicity, its source, and an optional local name (`sh:name`) |
| `?x rdfs:label ?l` | a label of any referenced IRI, all languages |

Blank nodes are allowed for uses (the N-Triples parser must accept them).

## 4. Sources → uses

Each source vocabulary maps onto uses. `owl:unionOf` lists are expanded in the
query: one use per member class.

| Source | Class `C` | Range `R` | Multiplicity | Query |
|---|---|---|---|---|
| `P rdfs:domain D ; rdfs:range R` | D | R | — (`owl:FunctionalProperty` → max 1) | core — done |
| `P schema:domainIncludes D ; schema:rangeIncludes R` | each D | each R | — | core — new |
| `C rdfs:subClassOf [owl:onProperty P ; owl:allValuesFrom R]` | C | R | — | owl — new |
| `… owl:someValuesFrom R` | C | R | min 1 | owl — new |
| `… owl:(min\|max)?(Qualified)?Cardinality n [owl:onClass R]` | C | R if qualified | from n | owl — new |
| NodeShape `S` (`sh:targetClass C`, or `S` is itself a class) `sh:property [sh:path P]` | C (or S) | `sh:class R`; `sh:datatype T`; `sh:nodeKind sh:Literal` → `rdfs:Literal`; `sh:node S2` (S1) | `sh:minCount`/`sh:maxCount` | shacl — new |
| `sh:or` / `sh:xone` over `sh:class`/`sh:datatype`/`sh:nodeKind sh:Literal` | as above | one use per alternative | as above | shacl — new |

| ID | Requirement | Status |
|---|---|---|
| S1 | A property is a candidate when it is typed as a property (§3 kinds) or has any use. `rdf:type` and `rdfs:label` are skipped even when an ontology re-declares them (PROV does). | done |
| S2 | Only simple IRI `sh:path`s are used; complex paths are skipped. Property shapes may be blank nodes or skolem IRIs (`…/.well-known/genid/…`, MDS production); both are accepted. | new |
| S4 | Only property shapes with `sh:severity sh:Violation` or no severity are used. `sh:Warning`/`sh:Info` shapes (DCAT-AP CH: "recommended" `minCount 1`, "deprecated" `maxCount 0`, language-tag checks) are ignored. | new |
| S5 | `sh:node S2`: if `S2` has a `sh:targetClass`, the range is that class; otherwise `S2`'s own `sh:class`/`sh:datatype`/`sh:or` are read one level deep (e.g. DCAT-AP's `DateOrDateTimeDataType_Shape` = `xsd:date` or `xsd:dateTime` or …). An anonymous `sh:node [ … ]` that only constrains the value's own properties (MDS: `skos:inScheme` `sh:hasValue`) adds nothing. | new |
| S6 | Value-level constraints are ignored: `sh:pattern`, `sh:languageIn`, `sh:qualifiedValueShape [sh:languageIn …]`, `sh:message`, `sh:description`. | new |
| S3 | Labels are read from `rdfs:label`, `skos:prefLabel`, `schema:name`, `dct:title` (same order as `visoto:label`, written out) and emitted as `rdfs:label`. `sh:name` labels the use, not the property. | partly done |

## 5. Drawing rules (JS projection)

| ID | Requirement | Status |
|---|---|---|
| J1 | Every member class is a class box. A member shape with `sh:targetClass C` is drawn as the box of `C`, and `C` counts as a member (not external). | done (shapes new) |
| J2 | `rdfs:subClassOf` → generalization edge (hollow triangle) when **either end** is a member — profiles add axioms to other vocabularies' classes (DCAT-AP CH: `dct:Location rdfs:subClassOf dct:LocationPeriodOrJurisdiction`; MDS: `dcat:CatalogRecord rdfs:subClassOf mds:ResourceRecord`). Restrictions and `sh:node` are never generalizations. | done (either-end new) |
| J3 | A use's range is a **datatype** when it is `xsd:*`, `rdfs:Literal`, `rdf:langString`, `rdf:PlainLiteral`, `rdf:HTML`, `rdf:XMLLiteral`, `rdf:JSON`, or a schema.org datatype (`schema:Text`, `Number`, `Integer`, `Float`, `Boolean`, `Date`, `DateTime`, `Time`, `URL`). An `owl:DatatypeProperty` without range is `rdfs:Literal`. | done (schema.org new) |
| J4 | A datatype use is an attribute row `P: xsd:type` in `C`'s box; any other use is an association edge `C —P→ R`. Every property is owned by a class, never a free node. | done |
| J5 | Missing `C` or `R` → a dashed placeholder box: one per property for edges (no self-loops on one hub), one shared per label for attributes. Labelled "Thing" for OWL properties and "Resource" otherwise. A domain or range of `owl:Thing` or `rdfs:Resource` counts as missing (the MDS model uses both) rather than as an external box. | done (Thing/Resource new) |
| J6 | A referenced class that is not a member and not a datatype is an external box (shaded). | done |
| J7 | Several domains or ranges — intersection (`rdfs:domain` ×2) or union (`domainIncludes`, `unionOf`, `sh:or`) — are drawn the same: several arrows. | done |
| J8 | Merge by `(C, P)`: (a) uses with the same `(C, P, R)` are one edge or row; (b) a use without `R` takes the ranges of the other uses of `(C, P)` and is drawn on its own only if there are none — DCAT-AP CH splits cardinality (`*-shapes.ttl`) and class range (`*-range-shapes.ttl`) into two shapes for the same class; (c) a use without `C` (a global `rdfs:range`/`schema:rangeIncludes`) is drawn only if `P` has no use with a class — otherwise every profile property would also appear on a placeholder box. Multiplicity: SHACL wins over OWL, OWL over RDFS; several SHACL shapes on one `(C, P)` combine to the tightest bounds (largest min, smallest max). | new |
| J9 | Multiplicity is shown in UML notation (`1`, `0..1`, `1..*`; the default `0..*` is not shown): on the edge label (`P [1..*]`) or appended to the attribute row (`P: xsd:string [1]`). An edge label that differs per class needs its own link type, so the projection emits a per-edge link-type IRI (`urn:visoto:edge:…`) that still opens `P`. | new |
| J10 | A use's own label (`sh:name`) replaces `P`'s label on that edge or row only. | new |
| J11 | Each inverse pair (`owl:inverseOf`) stays two arrows. | done |
| J12 | The node, link-type and attribute labels shown come from the page language. | done |

The projection's output is the existing `graph-memory-store.js` contract
(class boxes, `urn:visoto:ExternalClass` / `urn:visoto:AnyResource` markers,
edges, literal attribute rows, labels), so `buildStore` and the schema diagram
are unaffected apart from J9's link-type IRIs.

## 6. Out of scope

Sidebar trees for classes and properties (separate queries), `rdfs:subPropertyOf`,
property chains, disjointness (`owl:disjointWith`, `sh:not`, `sh:disjoint`),
symmetric/transitive characteristics, SWRL / `sh:sparql` / SHACL-AF rules,
inference of any kind. The diagram draws what is asserted; neither the
open-world nor the closed-world reading is evaluated.

## 7. Decisions

- **Q1** Intersection and union are drawn the same: several arrows. → J7
- **Q2** OWL and SHACL describing the same `(C, P)` merge. → J8
- **Q3** Inverse pairs stay two arrows. → J11
- **Q4** Split into parallel queries by source (core RDFS/OWL + schema.org,
  OWL restrictions, SHACL), concatenated client-side before projection; the 5 s
  budget (C3) decides whether a query is split further.
- **Q5** External = not a member (J6). Under the namespace fallback, an
  untyped term in `O`'s namespace is now drawn as external rather than as a plain
  box; accepted.
- **Q6** A vocabulary's diagram pulls in the shapes documents that import it.
  → M7
- **Q7** No named-graph membership. Ontologies must declare membership
  (M1/M2) or use the namespace fallback (M4); the MDS model owner is asked to
  add `rdfs:member` to the MDS model, and an `owl:Ontology` with `rdfs:member`
  to the MDS shapes. Known limitation: `rdfs:domain`/`rdfs:range` axioms are
  global, so in a store holding several ontologies, one ontology's axioms on a
  shared property (MDS: `dct:hasPart rdfs:domain mds:DatasetStructure`) also
  show in another's diagram.

## 8. Deferred until data shows up

To be added only if the Phase 0 survey finds them on LINDAS or QLever:

- value enumerations (`owl:oneOf`, `sh:in`) as an enumeration box or an
  arrow to a concept scheme;
- `owl:hasKey` / `owl:InverseFunctionalProperty` → `{id}` on the row;
- `owl:hasValue` / `sh:hasValue` → `P = v` row;
- custom datatypes (`owl:onDatatype` + `owl:withRestrictions`) and facets;
- `sh:inversePath` (reversed edge), `sh:targetSubjectsOf`, untargeted shapes
  drawn as their own box.

## 9. Test material

`~/projects/metadata-swiss/mds-datamodel/` (loaded as fixtures, each file in
its own named graph):

| File | Exercises |
|---|---|
| `ontology/dcat-ap-3.0.0.ttl`, `dcat-ap-2.1.0.ttl` | M2 (rdfs:member, terms isDefinedBy their own vocabulary), no domain/range, `schema:rangeIncludes` with datatypes only |
| `ontology/dcat-ap-ch-v3.ttl`, `-v2.ttl` | M6 (`owl:imports` DCAT-AP), J2 (subClassOf on non-member classes) |
| `SHACL shapes/DCAT-AP CH v3/*-shapes.ttl` + `*-range-shapes.ttl` | M6/M7, S4 (Warning shapes), J8b (cardinality and class range in separate shapes), `sh:or` with `sh:nodeKind sh:Literal` |
| `SHACL shapes/From ODSn project/dcat-ap-ch.hub.shapes.ttl` | S5 (`sh:node` to a datatype-disjunction shape) |
| `ontology/mds-model-MVP.ttl` | once it has `rdfs:member` (Q7): M2 with foreign namespaces, the domain/range leak (Q7), J5 (`owl:Thing`/`rdfs:Resource` ranges), plain RDFS/OWL domain/range |
| `SHACL shapes/MDS_MVP/mds-resource-shapes.ttl` | once it has an `owl:Ontology` (Q7): S2 (skolem IRIs in production), S5 (anonymous `sh:node`), `sh:node` + `sh:class` together |
