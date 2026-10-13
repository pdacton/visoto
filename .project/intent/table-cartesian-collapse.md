# `sparqlTable` — auto-collapse Cartesian products

Status: **feature request**, no code written. Requirement IDs (CC-n) are stable —
cite them in issues and PRs.

---

## 1. Why

**The main goal is to present tabular information in a user-friendly *and*
adaptive way** — adaptive in that the same result set is shown in whichever shape
fits what the user is currently doing: collapsed to one row per entity for
reading, expanded to one row per binding when they group by a multi-valued column
(CC-9) or ask for the raw bag (CC-8).

A SPARQL result set is a bag of *bindings*, not a list of *entities*. Any
`OPTIONAL { ?x a ?type }` — or any other multi-valued property — multiplies rows:
one row per (entity, value) pair. `sparqlTable` renders that bag verbatim, so an
instance with two `rdf:type`s occupies two visually near-identical rows.

Concretely, a class table listing instances should show **one row per instance**.
Today a multi-typed instance (Zazuko GmbH has 4 types) renders as 4 rows differing
only in the type cell, which reads as duplicate data.

The wanted behaviour: a Cartesian product yields one row per combination of
values, so a single entity occupies as many rows as it has differing attribute
values. Collapsing those rows around one principal attribute — the row's identity
— and compacting the differing values into a single cell gives a more
user-friendly display.

Note this inverts under grouping (CC-9): a group-by on a collapsed column needs
the entity in each of its groups, so that column must expand again.

This is the same defect already recorded for relationship tables in
`.project/issues/` territory — incoming/outgoing tables show one row per
destination type. That makes this request the general fix for both surfaces.

### Prior art — what was already rejected

On 2026-07-19 this was "fixed" query-side with `GROUP BY` + `SAMPLE(?t) AS ?type`
across 10 template queries, then **reverted at the user's request**. `SAMPLE`
collapses correctly but shows an *arbitrary single* type and hides the rest.

**CC-0** — A solution must keep **every** value visible. Do not re-propose
`SAMPLE`, and do not solve this by dropping values.

---

## 2. Scope

| In scope | Out of scope |
|---|---|
| Collapsing rows that describe one entity into one row. | Changing what the queries select. |
| Rendering the collapsed values in a single cell. | Deduplicating the *same* destination reached via several properties (contactPoint/creator/publisher → same org) — a distinct noise source, still unsolved. |
| Sync `<sparql-query>` and async `<sparql-async>` tables alike. | `sparqlGrid` / `sparqlTree`. |

**CC-1** — The collapse is a **presentation** concern, applied to the result set
after it arrives. Template queries stay as they are, so no per-template migration
is needed and nothing has to be re-audited for `GROUP BY` correctness.

---

## 3. Requirements

**CC-2 — Auto-detection.** Collapsing happens without a per-table opt-in; the
partial detects the condition. The intended rule: rows sharing the same value in
the table's **identity column** (the row's own IRI — the key var that
`sparql.DeriveKeyVar` already derives for class-instance tables) are one entity.

**CC-3 — Collapse only what actually varies.** For a group of rows sharing an
identity, a column whose values are all equal renders once. Only columns that
genuinely differ across the group render as multiple values in the cell.

**CC-4 — All values, in one cell.** Differing values stack inside the cell —
badges for types, a list otherwise — preserving each value's icon and link.

**CC-5 — Icons and links must survive.** This is the hard part and the reason
the query-side fix was tried first. The current `iconVar` / type-cell machinery
in `static/js/sparql-table.js` expects **one IRI binding per cell**; a
`GROUP_CONCAT` string breaks both the icon lookup (`internal/icon`, resolved
server-side into an IRI → icon-path map) and the resource link. The collapsed
cell therefore has to carry a *list of IRIs*, not a joined string, and the
formatter has to render one icon + link per value.

**CC-6 — Sorting, search, export, facets stay correct.** A collapsed table is
still sortable and searchable on a multi-valued column, a CSV/JSON/XLSX export
represents the several values unambiguously, and a facet over a collapsed column
keeps matching an entity when *any* of its values matches — the posture the
backend `FILTER EXISTS` already takes. Cross-check against limitation 6 in
`.project/issues/faceted-search-known-limitations.md`: `matchesLocally` compares
a single binding per row, which is exactly where a multi-valued column diverges.

**CC-7 — Row counts mean entities.** Anything user-facing that counts rows (the
row counter, "Search all N", working-set bounds) counts collapsed rows, or is
explicit that it counts bindings. Worth checking against the working-set model
for huge classes, where the bounded fetch is a binding count.

**CC-8 — On by default, user-switchable.** Collapsing is **on** for every table
without any template change (this follows from CC-2 — no per-table opt-in). The
**user** can turn it off per table from the table UI and see the raw binding bag,
one row per binding. Toggling is instant and does not re-query: both shapes are
derivable from the result set already in hand.

Sub-points:

- **Discoverability.** The control lives with the table's existing affordances
  (near search / export), not hidden in a menu. When a table is actually
  collapsing something, say so — e.g. the row counter noting that N bindings
  became M rows — so the user knows the switch is worth reaching for.
- **Persistence.** Undecided: remember the choice (per table, via the existing
  client-side preference storage) or reset to collapsed on each page load. Lean
  to remembering, keyed per table id.
- **Author override.** Separately from the user's switch, a template may declare
  a table as never-collapsed for cases where the raw bag *is* the data. Keep the
  two independent: an author's default should not take the user's control away.
- Turning collapsing off must leave sorting, search, export and facets working
  on the expanded rows — i.e. CC-6 holds in **both** states.

**CC-9 — Group-by overrides the collapse, per column.** Collapsing assumes the
row's identity is the grouping key. When the user groups the table by a different
column, that assumption no longer holds: a group-by on a **collapsed** column
requires the entity to appear under each of its values, so the rows must expand
again for that column. An instance with two types belongs in both type groups —
showing it once, in one group, with a two-valued cell would be wrong.

Collapsing and grouping are therefore **mutually exclusive on the same column**:

- Grouping by column X forces X back to one row per value.
- The table's **other** columns stay collapsed — expansion is scoped to the
  grouped column, not the whole table.
- Grouping by the identity column, or by a column that was not multi-valued,
  changes nothing.
- Ungrouping restores the collapsed view.

This interacts with CC-7: under a group-by, the same entity is legitimately
counted in several groups, so per-group counts sum to more than the table's
entity count. Group headers should be honest about which they show.

Open: whether this generalises — is the collapse simply "group by identity, with
every other differing column compacted", making CC-9 a re-parameterisation of
one mechanism rather than a special case? If so, Tabulator's own grouping may do
most of the work.

---

## 4. Open questions

1. **Where does the collapse run** — server-side in the handler/partial, or
   client-side in `sparql-table.js`? Client-side keeps one code path for sync and
   async tables; server-side keeps the wire payload smaller and makes CC-7 honest
   for the working-set model.
2. **Identity for non-class tables.** CC-2 leans on a derivable key var. Tables
   built with `BIND(?? AS ?s)` have none (limitation 2 in the faceted-search
   issue) — do they collapse on the first column, or not at all?
3. **How many values before a cell is truncated?** A "+3 more" affordance may be
   needed for pathological cases, and row height must stay uniform enough for
   Tabulator's virtual DOM.
4. **Interaction with the `vs-cell-wrap` / column-cap heuristic** for literal
   columns, which already governs cell height.

---

## 5. Acceptance

- A class table of a class with multi-typed instances shows one row per instance.
- Every type of a multi-typed instance is visible in that row's type cell, each
  with its own icon and working resource link.
- No template query is modified to achieve this.
- The user can switch collapsing off on that table and get one row per binding
  back, without a re-query, and switch it on again.
