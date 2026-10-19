# Plan: SHACL shape summary

Status: implemented 2026-10-19 (`internal/shapeprose`, `GET /api/shape-summary`,
`templates/partials/shape-summary.html`). Decisions and the data findings that
shaped it are at the end.

## Goal

Explain a SHACL shape in plain language, so a reader who does not know SHACL
understands what data the shape asks for. Today a shape page shows the raw
structure: the property-shapes table and the nested-structure tree. Both are
accurate, but you have to know SHACL to read them.

Actual output for the DCAT-AP CH 3.0.1 `DatasetShape` (local QLever):

> **Applies to** every Dataset.
> - **Description** `dct:description` — required, one or more · a resource or a nested structure · a value · at least one of them: in German, French, Italian or English
>   (the "resource" part comes from a damaged `sh:or` in the loaded data, see the notes below)
> - **Publisher** `dct:publisher` — required, exactly one · of type Agent
> - **Release date** `dct:issued` — required, exactly one · a date or a date and time
> - … one line per property path

## Prior art

No existing tool turns SHACL into prose, and none we could embed (see the
research notes from 2026-10-19). What we borrow from it:

- **SHACL Play documentation generator** (Sparna, Java): its doc tables decide
  the field set and grouping (property, cardinality, expected value,
  description). We do not run it: it is a Java service and produces tables, not
  sentences.
- **Validator default messages**: one stock message per constraint component.
  We key our sentence templates the same way, by constraint component.
- **SHACL 1.2 `sh:intent`** together with `sh:name`, `sh:description` and
  `sh:message`: text written by the publisher. Where it exists we show it, and
  generate nothing for that part.
- **xpSHACL**: rules first, LLM only to rephrase. This is the model for the
  optional "Explain" follow-up below.

## Approach

**Rule-based and deterministic.** SHACL Core is a closed vocabulary of about 30
constraint components. Each maps to a phrase template, and the templates are
translated through the existing catalogs. The output is correct by construction,
needs no LLM, can be cached like any `/api` fragment, and works the same on
every endpoint.

**Short structured phrases, not full sentences.** Each property gets one line:
`**Label** (CURIE): <cardinality>; <value type>; <further constraints>.` Full
sentences sound better in English but break down across languages. In German,
"a/an" before a label depends on the noun's grammatical gender, which we cannot
know. Lines built from fixed phrases joined by ";" translate safely and are
easier to scan.

## Scope (round one)

Pages:
- `instances/sh:NodeShape.html`: summary card above the property-shapes table.
- `instances/sh:PropertyShape.html`: one line for this property shape.
- `instances/cube:Constraint.html`: summary card above the Dimensions table,
  using only the generic SHACL rules. cube.link terms such as `qudt:scaleType`
  and `meta:dataKind` go through the fallback (decision 4).

Out of scope: validating data against shapes, the shape-diagram view, `sh:sparql`
and SHACL-AF rules (they get a fallback line), editing.

## What gets explained

| Group | SHACL terms | Phrase (en) |
|---|---|---|
| Targets | `sh:targetClass`, `targetNode`, `targetSubjectsOf`, `targetObjectsOf`; implicit class target (the shape is also `rdfs:Class`) | "Applies to every {Class}" / "Applies to {node}" / "Applies to everything that has {property}" |
| Cardinality | `sh:minCount`, `sh:maxCount` | Combined: required/optional × exactly one / at most n / at least n / between n and m / any number; `maxCount 0` → "not allowed" |
| Value type | `sh:class`, `sh:datatype`, `sh:nodeKind` | "each a {Class}"; datatypes in plain words (`xsd:string` text, `rdf:langString` text with a language tag, `xsd:integer` whole number, `xsd:decimal` number, `xsd:date` date, `xsd:dateTime` date and time, `xsd:boolean` yes/no, `xsd:anyURI` web address; otherwise the CURIE); `sh:IRI` "a link to a resource", `sh:Literal` "a value", `sh:BlankNode` "a nested structure" |
| Range | `minInclusive`, `maxInclusive`, `minExclusive`, `maxExclusive` | "from {a} to {b}", "greater than {a}", … |
| Text | `minLength`, `maxLength`, `pattern` (+`flags`), `languageIn`, `uniqueLang` | "at most {n} characters", "matching `{pattern}`", "in German, French or Italian", "one per language" |
| Fixed values | `sh:hasValue`, `sh:in` | "always {v}", "one of: A, B, C" (first 8, then "+ n more") |
| Pairs | `equals`, `disjoint`, `lessThan`, `lessThanOrEquals` | "same as {property}", "different from {property}", "before {property}" |
| Nested shapes | `sh:node`, `sh:property` inside a value shape, `qualifiedValueShape` + `qualifiedMin/MaxCount` | Named shape → "must match {Shape}" (link); blank shape → nested lines, one level deep |
| Logic | `sh:or`, `sh:xone`, `sh:and`, `sh:not` | "either … or …", "exactly one of …", "all of …", "not …". Each alternative is explained recursively, down to 2 levels; beyond that, "see the nested structure" |
| Closed | `sh:closed`, `sh:ignoredProperties` | "No other properties are allowed (except {list})." |
| Severity | `sh:severity` | Default (Violation): nothing. `sh:Warning` / `sh:Info`: the line reads "recommended" instead of "required" (decision 3) |
| Deactivated | `sh:deactivated true` | Line greyed out, marked "not checked" |
| Publisher text | `sh:name`, `sh:description`, `sh:intent`, `sh:message` | `sh:name` replaces the property label; `sh:description` / `sh:intent` shown under the line; `sh:message` as a tooltip |
| Order | `sh:order`, `sh:group` | Lines sorted by `sh:order`, then by label; grouped under the `sh:group` label if present |
| Paths | `sh:path` IRI, `inversePath`, sequence, `alternativePath`, `zeroOrMore`/`oneOrMore`/`zeroOrOnePath` | "{p}", "{p} of (inverse)", "{p1} → {p2}", "{p1} or {p2}", "{p}, repeated" |
| Anything else | Unknown predicates on a shape | Fallback line "Also: {CURIE} {value}", so nothing is dropped silently |

## Architecture

### 1. Data

The input is a set of triples, the same shape of data the blank-node tree
already uses. The shape and its property shapes, plus their nested
`sh:or` / `sh:in` / path structures, come from **one result set**.

Reuse `internal/bnodetree`'s query builder, which already has the probe/deep
staging that keeps QLever within memory, and add two options:

- `RootTriples`: level 0 returns all of the root's triples, not only the
  blank-node-valued ones. This brings back `sh:targetClass`, `sh:closed`, and so on.
- `WithProperties`: the roots are the shape **and** its IRI property shapes
  (`{ VALUES ?n0 {<shape>} } UNION { <shape> sh:property ?n0 FILTER isIRI(?n0) }`).
  On LINDAS 8,225 of 8,382 property shapes are IRIs (i14y), so without this
  their constraints would be missing.

Skolem IRIs: i14y writes `sh:in` lists with `/.well-known/genid/` IRIs instead
of blank nodes. The list walk must follow these like blank nodes. That needs a
`isBlank(?x) || STRSTARTS(STR(?x), …/.well-known/genid/)` test in the hop
pattern, and that same change helps the nested-structure card too. **Measure it
on QLever first:** a string test inside a hop may cost more than `isBlank`.

Labels: the query already runs with label resolution (`DisplayText`), so classes,
properties and values arrive with display labels in the page language.

### 2. Verbalizer (Go, new package `internal/shapeprose`)

- `Explain(graph, shapeIRI, opts) *Summary`. `Summary` is a render model in the
  style of `bnodetree.Tree`: `Targets []Phrase`, `Lines []Line` (each with
  `Label`, `Path`, `Parts []Phrase`, `Notes`, `Children []Line`), `Closed`,
  `Fallbacks`.
- A `Phrase` is an i18n key plus arguments, **not** finished text. The template
  translates it, so the Go side stays language-neutral and the same `Summary`
  can later be serialized for the LLM follow-up.
- One small function per constraint group (cardinality, value type, range,
  text, …), each registered against the predicates it consumes. Predicates no
  rule consumes become fallbacks. A test asserts that every SHACL Core
  predicate is consumed by some rule, so a missing rule fails the test instead
  of quietly falling back.

### 3. Route

`GET /api/shape-summary?iri=…&endpoint=…&lang=…[&format=md]`. Like every
`/api` route it is a pure function of the URL. It returns the card fragment,
an empty 200 when the resource has no shape content, or Markdown with
`format=md` (for "Copy as text" and the LLM follow-up).

### 4. Templates

- Partial `shapeSummary` (placeholder + `shapeSummaryCard`), parameters
  `iri`, `collapsed`, `trigger`, `title` (default "Summary"), `icon`.
- Values: IRIs as links (`sparql.ResourceHref`) with the CURIE in grey, like
  the tree.
- Phrase arguments are escaped **before** substitution, and the message is then
  rendered as trusted HTML. That is the `tHTML` rule: the catalog is trusted,
  the data is not.

### 5. i18n

- Keys `shape.*`: about 60 phrases. Cardinality and counts use plural forms
  (`tn`). English defaults go inline at the call site, with translations in
  de/fr/it; rm falls back to English.
- Plain-language datatype names go in their own key group `shape.datatype.*`.

## Tests

- Golden tests on fixtures taken from real shapes: DCAT-AP `CatalogShape`
  (QLever), a LINDAS `cube:Constraint` with `sh:in`, an i14y node shape with IRI
  property shapes and skolem lists, and a synthetic shape covering every
  component plus nested `sh:or`.
- A coverage test checking that every SHACL Core predicate is consumed by a rule.
- The i18n key-consistency tests that already exist.
- Live check on both endpoints, including QLever query memory.

## Order of work

1. Query options (`RootTriples`, `WithProperties`, skolem lists) with tests,
   measured on QLever.
2. `internal/shapeprose`: cardinality, value type, targets, closed and
   fallback first, against the DCAT-AP fixture.
3. Route + partial on the NodeShape page; check in a browser.
4. Remaining components (range, text, pairs, logic, nested, paths, severity).
5. PropertyShape and Constraint pages; i18n de/fr/it; docs (`templating.md`).
6. Optional follow-up: an "Explain" button that sends the Markdown summary
   (not the raw RDF) to the chat assistant for a prose paraphrase.

## Open decisions

1. **Phrase style**: short structured lines (recommended), or full sentences
   in English only for now?
2. **Placement**: a card above the property-shapes table (recommended), or a
   new tab next to Data/Schema?
3. **Severity**: should `sh:Warning` / `sh:Info` turn "required" into
   "recommended"? Application profiles use severity this way, but SHACL itself
   does not define the meaning. Check how DCAT-AP marks its recommended
   properties before deciding.
4. **cube.link terms** on constraint pages: generic fallback in round one, or
   dedicated phrases for `qudt:scaleType`, `qudt:unit`, `meta:dataKind` and the
   dimension types right away?
5. **Skolem IRIs in the nested-structure card**: change both features
   together (recommended), or only the summary?

## Decisions (2026-10-19)

1. **Phrase style:** short structured lines, parts joined by "·". Classes read
   "of type X", with no article.
2. **Placement:** a card above the property-shapes table. It is collapsed on
   constraint pages, where the Dimensions table stays the main view.
3. **Severity:** a badge ("warning" / "info") when every merged property shape
   has that severity. The wording does not change.
4. **cube.link:** dedicated phrases for the dimension type, scale, unit, data
   kind, standard-error relation and hierarchy.
5. **Skolem IRIs** are walked like blank nodes, both in the summary and in the
   nested-structure card.

## Implementation notes

- **Query:** a single fixed `QueryTo(3)`. A merged DCAT-AP CH shape nests
  property shape → qualified value shape → `sh:languageIn` list. That is 0.8–1.3 s
  on QLever. The tree card's depth-6 query still runs QLever out of memory on
  these shapes; the card falls back to the probe result.
- **Merging:** property shapes are merged by path, using the highest minCount,
  the lowest maxCount and deduplicated values. Without this the DCAT-AP shapes
  show 2–3 lines per property.
- **Display tweaks:**
  - No labels on QLever: local names are humanized ("accrualPeriodicity" →
    "Accrual periodicity").
  - A node kind is not shown next to a class or datatype.
  - `minInclusive` = `maxInclusive` reads "always".
- **Corrupted data:** the DCAT-AP shapes loaded into the local QLever have
  damaged `sh:or` lists. Members have no triples, and property shapes have
  stray `sh:or` values that are no list. Empty alternatives are skipped; a
  stray value shows as "Also: sh:or".
- **Tests:**
  - golden files from real responses (6 fixtures: DCAT-AP, DCAT-AP CH, a LINDAS
    cube constraint, i14y)
  - a coverage test over every SHACL Core predicate
  - handler tests
- **Not done:** `format=md` output. "Copy as text" copies the rendered card
  instead. The ontology-page "Shapes" card is round two.
