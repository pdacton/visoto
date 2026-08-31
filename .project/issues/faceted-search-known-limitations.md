# Faceted search: known limitations

**Status:** Open (accepted for v1)
**Component:** faceted search (`internal/facet`, `static/js/faceted-table.js`,
`templates/partials/sparql-async-table.html`)

Design constraints accepted when faceted search shipped. None is a bug; each is a
boundary worth removing if it starts to hurt. Recorded so the reasons aren't lost.

## 1. `for=` is a global id namespace — FIXED

`findColumns` (`cmd/visoto/async_index.go`) resolves a `for="X"` within the
**template set** that declares it — the page plus the layouts, partials and
components it is parsed with — and every `/api` request names its set in `?src=`.
Two pages may reuse a base query id without interfering; within one set a duplicate
fails at startup rather than resolving by directory order.

## 2. Facets only work on class-instance tables

Both handlers require `?iri=` plus a derivable key var — i.e. a `?key a ??`
membership triple (`sparql.DeriveKeyVar`). Attribute/relationship tables built with
`BIND(?? AS ?s)` have no key var and cannot be faceted.

This follows from the injection strategy: `BuildFacetedQuery` anchors its
`FILTER EXISTS` blocks on the membership triple, which is also what keeps the query
fast (see the "don't wrap the base query as a subquery" rule in
`internal/sparql/paging.go`). A different anchor would be needed for other shapes.

## 3. The filtered var must be a displayed column

A header-attached control hangs off the column it filters, so every
`<sparql-column var="X">` requires `?X` to be SELECTed in the base query. A filter on
a property you don't want shown isn't expressible; a declaration whose var isn't a
column is skipped with a dev-console warning.

Note the `path` attribute still does real work — it *produces* the value, e.g.
`OPTIONAL { ?key <path> ?X }` — so `path` and the column need not use the same
property (`schema:name` can back an IRI-typed `?organization` column).

**Possible fix:** allow detached facet controls (a filter panel above the table)
for vars that aren't columns.

## 4. `path` is a single property path, not a graph pattern

`<sparql-column path="...">` is interpolated directly as the predicate of one triple
pattern. Property paths work (`schema:address/schema:addressLocality`), but shapes
needing an intermediate node with its own constraint do not.

Concrete example from the Zefix class template: `companyUID` lives behind

```sparql
?organization schema:identifier ?idNode .
?idNode schema:name "CompanyUID" ; schema:value ?companyUID .
```

which cannot be written as a `path`, so that column can't be faceted.

## 5. Portability of `xsd:` coercion is untested on non-GraphDB stores

`coerce()` in `internal/facet/builder.go` emits the full-IRI constructor
`<http://www.w3.org/2001/XMLSchema#decimal>(?fv)` rather than `xsd:decimal(?fv)`,
because nothing declares a `PREFIX xsd:` — not the query, not `visoto.config`'s
prefix list, not the preprocessor. LINDAS/GraphDB pre-declares `xsd:` implicitly,
which is **not** SPARQL 1.1 behaviour, so the CURIE form would fail with
"undefined prefix" on a conformant store.

The full-IRI form is correct by construction and unit-tested
(`TestRangeClauseDeclaresNoPrefix`), but has only been *executed* against LINDAS.
Worth confirming against QLever/Virtuoso/Fuseki when one is available.

`PREFIX xsd:` has since been added to `visoto.config`, so hand-written template
queries can use the `xsd:` CURIE directly and the preprocessor will declare it.
That is belt-and-braces for range facets — the builder still emits full IRIs and
does not depend on the config entry — but it removes the trap for anyone writing
`xsd:` in a `<sparql-async>` query by hand.

Note that bare `<http://...>` IRIs inside a `<sparql-async>` element are eaten by
the HTML parser (they look like tags), leaving a malformed query. Template queries
must use CURIEs for IRIs; this is why the prefix list matters.

## 6. Local preview vs. multi-valued facet properties

`matchesLocally` (`static/js/faceted-table.js`) compares a single binding per row,
while the backend `FILTER EXISTS` correctly matches an instance if *any* of its
values satisfies the constraint. For a multi-valued facet property the instant
local preview can therefore disagree with the authoritative backend result that
replaces it moments later.

Not yet observed in practice — the facets in use are effectively single-valued —
but it is a real divergence between the two tiers.
