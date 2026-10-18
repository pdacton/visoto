# Plan: Blank-node tree

Status: draft, not implemented. Mockup:
`/tmp/claude-0/-home-user-visoto/63e81188-b902-5567-b904-d77567da85bf/scratchpad/bnode-tree-mockup.html`

## Goal

Make the outgoing blank nodes of a resource readable. Today they are a dead
end: the Data view's `resourceTriples` query keeps only `isLiteral`/`isIRI`
outgoing values (`templates/layout/base.html`), so outgoing blank nodes are not
shown at all, and incoming ones show up as `genid-…` links that lead nowhere.

## Scope (round one)

- Outgoing blank nodes only, followed recursively from one root IRI.
- Two ways the tree appears, both using the same partial:
  - **a) Added by hand** to a page template:
    `{{ template "bnodeTree" (dict "iri" .ResourceIRI) }}`
  - **b) Added automatically** at the bottom of the Data view of every resource
    (`trigger "showData"`). Nothing is rendered when the resource has no outgoing
    blank nodes.
- No config file. Captions, title predicates and sort keys are hard-coded maps in
  Go; user-facing strings go through i18n.

Out of scope: incoming blank nodes / "where used", editing, Graph Explorer
integration, a warning badge for U+FFFD encoding damage (possible follow-up).

## Partial parameters

| Param | Default | Meaning |
|---|---|---|
| `iri` | required | Root resource |
| `property` | all | Only expand blank nodes under this predicate (CURIE or IRI) |
| `via` | none | Single IRI predicate (CURIE or IRI, no property paths) to follow before expanding, e.g. `cube:observationConstraint` on a cube page |
| `collapsed` | false | Start with every node closed, each showing a one-line summary |
| `trigger` | `load` | HTMX trigger; the Data view passes `showData` |
| `title` / `icon` | "Nested structure" / `brackets` | Card header |

Depth limit is fixed at 5 and cannot be overridden.

## Rendering rules

| Pattern | Display |
|---|---|
| Blank-node object | Nested key/value block; header = `rdf:type` chips + best label |
| `rdf:first`/`rdf:rest` chain | Numbered list; `rdf:nil` shown as `( )`; list cells do not count toward depth |
| `sh:or` / `sh:and` / `sh:xone` / `sh:not` | List captioned "any of" / "all of" / "exactly one of" / "not" |
| `sh:in`, `owl:oneOf`, `owl:unionOf`, `owl:intersectionOf` | Chips with a count; first 8 shown, rest behind "+ n more" |
| IRI | Resolved label as a link (`sparql.ResourceHref`), CURIE in grey |
| Literal | Value + datatype or language tag; `cube:Undefined` gets its own badge |
| Repeated predicate | One key, values stacked |
| Sibling blank nodes | Sorted by `sh:order`, then by label |
| Same blank node reached twice | Full block the first time with a numbered marker; later "↑ same node as ①" (also prevents cycles) |
| Depth limit hit | Dashed line: "n nested blank nodes beyond depth 5 · m triples not shown" |
| Malformed list | Falls back to plain key/value display, with a small warning |

Best-label order: `schema:name`, `rdfs:label`, `skos:prefLabel`, `sh:name`,
`dct:title`, `vcard:fn`, `foaf:name`. Language chosen with the usual display
language rules.

Card header like `sparqlTable`: collapse-all-nodes toggle; ⋮ menu with
*Download* (Turtle, JSON-LD, N-Triples, Copy as Turtle) and *SPARQL query*
(Execute on endpoint); chevron to collapse the card.

## Architecture

### 1. Query (Go, new package `internal/bnodetree`)

- **One SELECT per root**, returning `?s ?p ?o` rows. Blank-node labels are only
  stable within one result set, so everything comes back in a single query.
  SELECT over SPARQL JSON results avoids needing an RDF parser (none in `go.mod`).
- UNION of one branch per depth: depth *n* walks *n* hops of blank nodes from
  the root (`FILTER isBlank` at each hop) and returns that hop's outgoing
  triples. List cells come from `rdf:rest*/rdf:first` inside the same branches.
- Query to **depth 6**, render 5. The depth-6 rows are only counted, which gives
  the "beyond depth 5" indicator.
- Labels for every IRI object and every `rdf:type` value are fetched in the
  same query, with the `visoto:dispLang` filter (reuse how other `/api/*`
  handlers build label queries).
- `GRAPH ?g` handling follows `resourceTriples`: a named-graph form plus a
  default-graph form with `MINUS`, so blank nodes from different graphs don't
  merge and default-graph stores (Stadt Zürich, RINF) still work.
- `property` and `via` restrict the first hop.
- Row cap 5000; when hit, the card shows a "truncated" note.
- The term type (iri / bnode / literal + datatype + lang) must survive from the
  SPARQL JSON binding; check that the `internal/sparql` client keeps it (unlike
  `QueryTypes`, which drops it).

### 2. Tree builder (`internal/bnodetree`)

- Methods on a `Tree` struct: build from rows → detect lists → classify
  enumerations → mark shared nodes → sort → apply depth limit and counts.
- Serializers from the same triples: N-Triples (flat), Turtle (nested `[ ]` and
  `( )` from the tree, shared nodes as `_:` labels), JSON-LD (expanded form). No
  new dependency.

### 3. Route

`GET /api/bnode-tree?iri=&endpoint=&lang=[&property=&via=&collapsed=&format=]`

- Registered with `epFromURL, langFromURL` like the other `/api/*` routes. The
  response depends only on the URL (no cookies), so it stays cacheable.
- Without `format`: HTML fragment (card). No blank nodes: empty response, and
  the placeholder removes itself (`hx-swap="outerHTML"`), so nothing is
  rendered.
- `format=ttl|jsonld|nt`: download of the same subgraph.
- "Execute on endpoint" reuses the existing mechanism in `sparql-table.js`
  (same config field); this feature must not send the browser anything more
  than it receives today.

### 4. Templates

- `templates/partials/bnode-tree.html`: placeholder card (skeleton, like
  `sparqlAsyncTable`) with `hx-get` to the route and the parameters above.
- `templates/partials/bnode-node.html`: recursive `{{ template "bnodeNode" }}`
  for one node.
- `templates/layout/base.html`: append
  `{{ template "bnodeTree" (dict "iri" .ResourceIRI "trigger" "showData") }}`
  after the "All Triples" card in `#resource-data-view`.
- Collapsing uses native `<details>`; markup only, no inline JS.

### 4b. "All Triples" table: blank-node rows

- Extend `resourceTriples` with outgoing blank-node branches (`FILTER(isBlank(?value))`,
  `BIND("blank node" AS ?kind)`, `OPTIONAL { ?value a ?type }`), in both the
  named-graph and default-graph forms like the existing branches.
- The value cell shows a "blank node" badge and its type instead of a
  `genid-…` link. It links to the tree card further down, to the block for the
  same predicate (`#bnode-<predicate>`). A link to the exact node is not
  possible: the table and the tree come from two queries, and blank-node labels
  don't carry over between result sets.
- Incoming rows whose subject is a blank node get the same badge (no dead link),
  but no jump target, since the tree only shows outgoing structure.
- Check how `sparqlTable` renders cell values: the badge probably needs a
  small change in the value formatter in `static/js/sparql-table.js`, keyed off
  the term type rather than the `genid-` prefix.

### 5. Frontend

- `static/js/bnode-tree.js`, keyed off `data-bnode-tree`: collapse/expand all,
  Copy as Turtle, Execute on endpoint. Loaded in `base.html`.
- `static/css/bnode_overrides.css` for the few rules Tabler doesn't cover
  (key/value grid, depth borders, dashed "more" line). Narrower key column in
  nested levels so deep trees don't drift right.
- No new runtime asset directory, so `Dockerfile` / `deploy.sh` stay unchanged.

### 6. i18n

New keys (card title, captions "any of" etc., "+ n more", "same node as",
"beyond depth", "empty list", "truncated", download labels) in all five
catalogs (de, en, fr, it, rm).

### 7. Docs

- `docs/templating.md`: `bnodeTree` partial section with parameters and an
  example with `via`.
- `CLAUDE.md`: add `bnodeTree` to the partials list.
- `graphify update .` after the code changes.

## Tests

- **Unit (`internal/bnodetree`)**: fixtures from real LINDAS data:
  - `BLW_t_01/1/shape/`: six property shapes, `sh:or`, `sh:in` (23), `meta:dataKind`
  - `BLW_t_01/1`: contact points, work examples, multilingual names
  - synthetic cases: cycle, shared node, depth 7, empty list, malformed list,
    row cap hit
- **Query builder**: golden-string tests for depth, `property`, `via`.
- **Handler**: empty response for a resource without blank nodes; `format=`
  outputs parse back to the same triples (N-Triples round trip).
- **Manual**: run on port 8061, check the shape and cube pages in Playwright,
  plus one non-LINDAS endpoint with default-graph triples.

## Order of work

1. `internal/bnodetree` query + tree builder + unit tests
2. Serializers + tests
3. Route
4. Partials + Data view integration
5. "All Triples" blank-node rows
6. JS + CSS
7. i18n
8. Docs, `graphify update .`

## Decisions

- `via` takes a single predicate only.
- The "All Triples" table shows outgoing blank nodes as rows, linking to the tree.
- Row cap 5000 and 8 enumeration chips before "+ n more" are fixed values.
