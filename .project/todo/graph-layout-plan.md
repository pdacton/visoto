# Graph Explorer — Implementation Plan

Companion to `graph-layout.md`. Requirement IDs (GL-n) cite that document.
Two parts: **Part A** is Visoto-only and ships on GE 2.1.0's public API from the
CDN; **Part B** moves the generic pieces into Graph Explorer upstream and then
deletes the Part A workarounds. Part A never waits for Part B.

---

## 0. What the code already gives us

Verified, not assumed:

| Need | Exists | Where |
|---|---|---|
| GE embed, browse mode (resource graph) | ✔ | `static/js/sparql-graph.js`, partial `sparqlGraph` |
| GE embed, construct mode (owl:Ontology UML diagram) | ✔ | same file, `-construct` island + `graph-memory-store.js` |
| GE embed, schema view | ✔ | `static/js/schema-graph.js`, partial `schemaGraph` (base layout) |
| Several graphs per page | ✔ | partial `id` param → `data-sparql-graph-id` |
| Fullscreen toggle | ✔ | `sparql-graph.js` `setupFullscreen` |
| Undo history recording GE's own actions | ✔ | `model.history` (`CommandHistory`), no UI |
| Layout hooks | ✔ | `GE.calculateLayout` / `applyLayout` / `forceLayout` (sync) |
| JS i18n | ✔ | `window.vsT` reading `js.*` catalog keys (`static/js/i18n.js`) |
| ELK | ✘ for GE | only inside Mermaid (`mermaid-init.js`); needs its own `elkjs` |
| JS test harness | ✘ | no `package.json`; verification is Playwright against a running server |

**Findings the spec did not anticipate:**

1. **Three embeds, not two.** `graph-layout.md` names `schema-graph.js` as "the
   ontology diagram"; the owl:Ontology UML diagram is actually `sparql-graph.js`
   in construct mode. GL-24 must cover all three.
2. **Duplication.** `sparql-graph.js` and `schema-graph.js` each carry their own
   CDN loader, readiness poll, island reader and link templates. Building GL
   features twice is not an option → Phase A0 extracts a shared layer first.

---

## 1. Architecture seam

```
sparql-graph.js ─┐                      ┌─ graph-layout.js   (pure: no GE, no DOM)
schema-graph.js ─┴─► graph-kit.js ──────┼─ ge-adapter.js     (ALL GE-API / GE-internal calls)
                     (toolbar, panels,  └─ elkjs (lazy, CDN)
                      selection, save)
```

- **`graph-layout.js`** — takes a plain `{nodes, edges}` graph (ids, sizes,
  positions, fixed flags, edge type), returns positions. Knows ELK and WebCola
  options, reversal, hidden-type filtering, centre choice, packing. Survives any
  library change (GE upstream, Reactodia).
- **`ge-adapter.js`** — the only file that touches GE. Every workaround that Part B
  replaces is here and tagged `// GE-UPSTREAM: B<n>` so it can be found with grep.
- **`graph-kit.js`** — `VisotoGraph.attach(workspace, rootEl, opts)` → controller
  per instance (toolbar wiring, selection, history helpers, save/restore, panels).
  Keyed off `data-graph-kit` markers; no inline JS (CLAUDE.md).
- **Markup:** new partial `templates/partials/graph-toolbar.html` (+ panels),
  included by both `sparqlGraph` and `schemaGraph`. Strings via `{{ t }}` in
  markup and `vsT('js.graph.*')` in JS.

---

## Part A — Local changes (Visoto only)

Each phase ends deployable. Workarounds are listed per phase with the Part B step
that retires them.

### A0 — Foundation: shared layer, own toolbar, undo, errors
GL-44, GL-18, GL-45, GL-46, GL-24, GL-25.

- Extract loader / readiness / island reader into `graph-kit.js`; both embed files
  call `VisotoGraph.attach`.
- Add SRI (`integrity` + `crossorigin`) to the GE script loader — today it has
  none, unlike the Tabler / Tabulator links in `base.html`.
- **Touch spike:** check whether GE 2.1 reacts to touch/pointer events at all;
  if it is mouse-only, re-scope GL-40 before A2.
- `hideToolbar: true`; render `graph-toolbar.html` with parity (GL-44): Layout ▾
  (placeholder until A1, runs GE force), Undo/Redo, Fit, Language ▾, Save /
  Export ▾ (SVG, PNG, Print, Clear all for now).
- Undo/Redo (GL-18): buttons + Ctrl/Cmd+Z / Shift+Z → `workspace.undo()/redo()`;
  state from `history.events 'historyChanged'` + stack lengths; tooltip from
  `Command.title`. Helper `kit.batch(title, fn)` for every later Visoto action.
- Errors (GL-45): wrap provider calls, inline alert with Retry; empty-result
  message. Spinner with Cancel (GL-46) via an `AbortController` checked between
  layout steps.
- **Workaround:** replacing GE's toolbar instead of extending it → B4.
- **Exit:** all three embeds render as today with the new toolbar; undo works for
  GE-native drag/remove/add; nothing from GE's old toolbar is missing.

### A1 — Layout engines and menu
GL-2, GL-11, GL-13, GL-16, GL-17, GL-19, GL-26 (fixed flag only).

- `graph-layout.js`: Network (WebCola via `GE.forceLayout` adapter, tuned), Tree ↓
  / Tree → (ELK `layered`, DOWN / RIGHT), Radial (ELK `radial`, centre rule
  GL-19), component packing (GL-17). Default reversals (GL-22) applied here.
- `elkjs@0.12.0` (`elk.bundled.js`) lazy-loaded from jsDelivr with SRI on first ELK
  use. Licence EPL-2.0 OR GPL-3.0-or-later — loaded unmodified from the CDN; note
  it next to the other third-party libraries in the docs.
- Apply: positions set inside one `kit.batch("Layout — Tree ↓")`, link vertices
  cleared; selection-only layout keeps the centroid (GL-13).
- Per-graph defaults (GL-2): browse → Network, construct/schema → Tree →.
- **Workaround:** async ELK path next to GE's sync `calculateLayout` → B2.
- **Exit:** the four layouts on all three embeds, undoable, cancellable; ontology
  diagram readable top-down with superclasses on top.

### A2 — Selection, selection actions, pinning
GL-5–9, GL-12, GL-26, GL-51, GL-41 (shortcuts).

- Pan/Select toggle; box via capture-phase pointer listener +
  `pageToPaperCoords`; Ctrl/Cmd+click via `onPointerUp`; `editor.setSelection`.
- Selection marking (outline + check) and pin badge via `_setElementDecorator`.
- Group drag via `changePosition` deltas inside one batch.
- Toolbar selection group: Expand all, Remove, Keep only these, Layout selection,
  Select neighbours, Pin/Unpin, Align/distribute; Fit becomes selection-aware.
- **Workarounds:** box selection, multi-selection rendering, group drag → B3;
  internal `_setElementDecorator` → B3.
- **Exit:** everything reachable by mouse, keyboard and touch (Select mode).

### A3 — Saving
GL-35, GL-36, GL-50 (GL-3, GL-37 merged).

- Serialized form: `model.exportLayout()` + Visoto metadata (pins, GL-21 settings,
  hidden classes/namespaces, layout choice, endpoint slug, language, format
  version).
- Autosave key: page URL + graph `id`; debounced on `historyChanged`.
- Reset / Open with confirmation → fresh history. Save as… / My diagrams in
  `localStorage`. Download / Open `.visoto-graph.json`; slug switches endpoint.
- **Exit:** reload restores each graph on a multi-graph page independently.

### A4 — Layout options, Add resource, Details, growth limits
GL-20–23, GL-47, GL-48, GL-14, GL-15.

- Collapsed "Layout options" panel (Visoto markup) with Reverse + Redraw; hidden
  link types (GE's Connections panel) filtered out of layout input.
- Add resource: label lookup through the provider's lookup / pasted IRI.
- Details toggle: `element.setExpanded()` for selection or all, one batch.
- Additions over 20 confirm (GL-15); new nodes placed with existing ones fixed
  (GL-14, via `editor.events 'addElements'`).

### A5 — Class tree, filters, find
GL-10, GL-49, GL-52, GL-32.

- Class-tree ⋮ menu (Select all / Remove all, `subClassOf*`); eye toggle.
- Namespace filter on construct/schema embeds.
- Find on canvas: highlight + zoom via `setHighlighter`.
- **Workaround:** hide = remove + re-add with link restore → B5.

### A6 — Export, list view, accessibility, docs
GL-38, GL-39, GL-40, GL-41, GL-53.

- Turtle export from the model's elements + visible links.
- List view: nodes/edges table in the same card, shared selection.
- a11y pass (aria-labels, focus rings, 44 px targets), help page.
- Docs: `docs/templating.md` (partial params), `graph-explorer` skill,
  `docs/ontodia-graph-explorer-references.md`; locale keys in all five catalogs.

---

## Part B — Graph Explorer upstream (separate step)

Starts after A2 is stable, so every proposal is backed by a working Visoto
implementation.

### B0 — Proposal issue at zazuko/graph-explorer
One issue listing B1–B6 as independent extension points, asking which PRs are
welcome. **Gate:** if declined, stop Part B and run the Reactodia spike
(`graph-layout.md` §7) instead.

### B1 — Undo/Redo in the default toolbar + hotkeys
Smallest PR; builds trust. Retires nothing in Visoto (we keep our toolbar until
B4), but removes our hotkey binding.

### B2 — Async layout hook
`forceLayout`-style entry point accepting an async `layoutFunction` with
`fixedElements` / `selectedElements`, recorded in history. Retires the A1 async
path in `ge-adapter.js`.

### B3 — Multi-selection
Box selection (modifier or pointer mode), multi-selection rendering, group drag.
Retires the A2 pointer listeners and `_setElementDecorator` use.

### B4 — Extensible toolbar
Slots / items added to the default toolbar instead of replacing it. Lets Visoto
drop its GE-parity buttons (GL-44).

### B5 — Element hidden state
A visibility flag on elements respected by rendering, links and export. Retires
the remove/re-add workaround (GL-49, GL-52).

### B6 — Optional: expand/collapse all, align/distribute
Only if B1–B5 land smoothly.

### Adoption
- Upstream work happens in a GE fork checkout, outside this repo.
- Visoto stays on the CDN release; **no self-hosted GE bundle** (would need a new
  asset dir in `Dockerfile` and `deploy.sh`) unless explicitly decided.
- On each GE release: bump `GE_SRC` + SRI via the `maintenance` skill, then delete
  the matching `GE-UPSTREAM: Bn` workarounds from `ge-adapter.js`.

---

## 2. Working agreements

- **One branch** (`claude/graph-explorer-node-placement-o594d0`), **one or more
  distinct commits per phase**, each phase's commits self-contained and
  deployable. No per-phase branches or PRs.
- After code changes: `go build ./... && go test ./...`, `graphify update .`,
  restart the server (templates load at startup).
- i18n keys: `graph.*` in templates, `js.graph.*` for `vsT`, in all five
  catalogs (de, en, fr, it, rm).
- Every phase's exit additionally requires: all three embeds checked on the test
  set below, and undo covering every action the phase added.

### Running autonomously

- Per phase: build + test → verify the test set in headless Chromium (Playwright
  MCP or a script) with screenshots → commit → push → short report.
- Ambiguity: pick the simplest option consistent with `graph-layout.md` and record
  it in the decisions log below instead of stopping.
- Stop and ask only for: a spec contradiction that changes user-visible behaviour,
  a GE limitation with no workaround, anything destructive or outside this repo.
- Review cadence: pause for review after **A0** and **A1** (toolbar and layouts set
  the look and feel); A2–A6 run through.
- Setup: `cp visoto.config.example visoto.config` if missing; run on a port other
  than the one already in use (`PORT=8061 go run ./cmd/visoto/`).

### Decisions log

| Phase | Decision | Why |
|---|---|---|
| A0 | Visoto supplies its own remembering `CommandHistory` (`ge-adapter.js`), passed as the Workspace `history` prop. Batches nest; `batch.history` is the history itself; `execute` keeps the forward action's title. | GE 2.1.0 records into `model.history` but ships only `NonRememberingHistory` (undo throws). GE calls `model.history.execute` inside open batches, so commands must land in the innermost batch. Inverse commands carry the inverse's title ("Add element" for a remove). |
| A0 | Touch spike, from source: GE 2.1 is mouse-only (React `onMouseDown` + document `mousemove`/`mouseup`, no pointer or touch handlers). Taps work (emulated click), the paper area pans by native scrolling, but touch drag of a node does nothing. **GL-40 re-scope:** A2's Select-mode listener uses Pointer Events, which also gives touch node drag and box selection; no GE change needed. | Checked in `paperArea.tsx`/`elementLayer.tsx`; not yet on a device. |
| A0 | API: `VisotoGraph.create(id)` before GE loads, then `kit.attach(workspace)`, instead of `attach(workspace, rootEl, opts)`. | The kit shows the loading spinner and load errors (with Retry) before a workspace exists. |
| A0 | `ge-adapter.js` holds the loader (with SRI), history, render, SparqlDataProvider construction and every toolbar command. Embed files still call the diagram model directly (`createElement`, `importLayout`, `setPosition`, the island packing). | Moving model calls belongs with `graph-layout.js` in A1; A0 stays a no-behaviour-change refactor plus toolbar. |
| A0 | Toolbar keeps **Zoom in / Zoom out** next to Fit. | GL-44 parity: GE's toolbar had them; wheel zoom needs Ctrl. |
| A0 | Clear all confirms with `window.confirm`; it is one undo step ("Clear all"). | Simplest accessible confirmation; Reset diagram and its dialog arrive in A3. |
| A0 | Language menu: Deutsch / English / Français / Italiano (native names), default = site language, else English. No Romansh. | GL-44 default; LINDAS labels are practically never `rm`. |
| A0 | GL-45: a failed provider call stays pending while the inline message shows; Retry re-runs every pending call, Dismiss rejects them as before. `classTree` is not guarded. Empty page-level results (construct / schema query) show an info message; GE's own search and connections panels already say "no results". | `classTree` over all of LINDAS times out on every page and only fills the sidebar — a permanent banner. Expand-all "found nothing" lands with Expand all (A2). |
| A0 | Hotkeys (Ctrl/Cmd+Z, Ctrl/Cmd+Shift+Z, Ctrl+Y) go to the graph last pointed at or focused; ignored while typing. | Several graphs per page (T1 has three). |
| A0 | GL-46 Cancel in A0 only aborts before a layout starts (the spinner paints first); GE's force layout is synchronous. | Real mid-run cancel comes with async ELK in A1. |
| A0 | `zoomOptions.min` 0.05 (GE default 0.2) for every embed. | Fit could not fit T4's 341 nodes at 0.2 — the overview never appeared. |
| A0 | Fixed two pre-existing bugs found on the test set: (1) T3 drew nothing — seeding raced `importLayout`'s async `linkTypes()` → "Link type … already exists"; all embeds now seed after `importLayout` settles. (2) `classInfo` was asked for blank-node type ids (`<b0_genid…>`) → 400 for the whole batch; the adapter filters to absolute IRIs. | Both reproduced on the pre-A0 code; T3 is in the test set. |
| A0 | Left as found (pre-existing, not A0): schema view labels the anchor box "rdf:langString" on T1 (its `rdfs:label` attribute row collides with the box label); GE's SVG/PNG export requests `/undefined` (404, export still succeeds); the Classes sidebar spins forever when `classTree` times out. | Out of A0's scope; noted for later phases. |
| A1 | Radial is Visoto's own code (BFS tree, wedges by subtree node count, ring radius grown until each box fits its wedge's arc), not ELK `radial`. | GL-19 prescribes the centre rule and wedge sizing exactly; own code is pure, deterministic and unit-tested, and needs no ELK download for Radial. |
| A1 | Network = GE's WebCola step (`InternalApi.groupForceLayout` + `groupRemoveOverlaps`, the recipe of GE's `forceLayout`) run per component, link length from box size (120–320) instead of a flat 200. | GE's public `forceLayout` needs the model; the InternalApi functions take plain nodes, so `graph-layout.js` stays GE-free with the step injected. |
| A1 | Every algorithm lays out components one by one and packs them in rows (largest first, target 1.6:1); a component with over half the area gets its own row, islands go below. The construct-mode `packIslands` from 8375095 is removed — GL-17 packing supersedes it. | GL-17 "always packed"; one packing rule for all four layouts. |
| A1 | Fixed nodes (GL-26 flag; nothing sets it before A2): the component is laid out, translated so its fixed nodes' centroid is where it was, fixed nodes put back exactly; free components pack below. | ELK layered has no pinned positions; this keeps pins exact for every engine. |
| A1 | Selection layout needs ≥ 2 selected nodes (GE selects one node on click); a single selected node is Radial's centre instead. | GL-13 / GL-19; before A2 there is no multi-select, so selection layout is wired but only reachable after A2. |
| A1 | Cancel discards the computed result (no undo step, layout choice unchanged); a running ELK call is not interrupted. The > 500 warning applies to user-started layouts only, not the page's first layout. | elkjs has no abort; results arrive < 1 s for 600 nodes in node. The page's own first layout must not ask. |
| A1 | Tree ↓ / → use every visible edge for layering, reversed per GL-22. On association-heavy ontologies (T2, RiC: 110 boxes) the result is wide and diagonal. | Spec: hidden link types are the lever (GL-21). **Reviewed 2026-09-27: kept as is** (hierarchy-only layering declined). |
| A2 | Selected and pinned nodes are marked by a generated per-graph `<style>` (attribute selectors on `data-element-id`: outline + check badge top-left, pin badge top-right), not `view._setElementDecorator`. | The decorator takes React elements, and the CDN bundle does not expose React; GE remounts its DOM freely, and attribute rules survive that. |
| A2 | Box selection selects nodes the box touches (intersects), replacing the selection; a click (< 4 px) on empty canvas in Select mode clears it. | Friendlier than "fully inside" for large expanded cards; GL-5 says replace. |
| A2 | Group drag and Ctrl/Cmd+click ride on GE's paper pointer events; the auto-pin of dragged nodes (GL-26) is executed inside GE's still-open drag batch, so one undo takes back move and pin. | GE's editor handler runs first and selects the dragged/clicked node alone; ours then restores or toggles the intended selection. |
| A2 | Touch: Select mode handles touch itself (Pointer Events, `touch-action: none`): drag moves the node or its selection and pins it, tap toggles. Pan mode leaves touch to GE (tap selects) and native scrolling (pans). | Follows the A0 touch finding; GE has no touch drag. Verified with synthesized touch pointer events, not on a device. |
| A2 | Expand all is complete here: neighbours from the provider's `linkElements` (100 per node, both directions), GL-15 confirmation above 20, GL-14 placement by a Network pass with every existing node fixed, "No further neighbours found." when empty (GL-45), one undo step. | Cheaper to do now than to revisit in A4; A4 keeps GL-14 for halo-menu additions. |
| A2 | **GE bug worked around:** `ElementLayer.requestRedraw(el, None)` returns early (`forAll \| 0 === forAll`), so an element added or removed without a following selection change never renders — undo of Expand all left removed nodes on screen. The adapter forces a layer redraw after every history change (`element.redraw()`, or a language round trip when the canvas is empty). | Tagged `GE-UPSTREAM: B1`; worth reporting with the undo PR. |
| A2 | Delete stays GE's own (keyup on document, removes the selection, undoable as "Remove"). A press outside every graph makes no graph active, so Ctrl+A and Esc return to the page. | GE's Delete acts on every graph that has a selection — acceptable with at most three graphs, noted for B3. |
| A3 | Autosave key = page path + query (so the endpoint too) + graph `id`; written 0.8 s after a history change or a label-language change, never for an untouched page default — the next visit then gets the current page default. | GL-35; a canvas nobody changed should not freeze the page. |
| A3 | Menus: Save / Export ▾ holds Save as…, My diagrams…, Download file, Open file… and SVG / PNG / Print; **Reset diagram** sits in the ⋮ menu next to Clear all. | Follows the A0 review (destructive actions under ⋮); GL-44 lists both at the bottom of the menu. |
| A3 | Named saves: an index key plus one key per save in `localStorage`; Save as… and Rename use `window.prompt`; My diagrams is a Tabler modal listing every save in this browser (name, endpoint, date), each openable into the current graph. | GL-50; simplest accessible UI. Quota errors show a warning message (plan risk). |
| A3 | Opening a canvas saved on another endpoint reloads this page with `?endpoint=<slug>` and hands the canvas over through `sessionStorage` (read once). | GL-36 "switches to that endpoint"; resource pages are pure functions of the URL, so the switch is a navigation. |
| A3 | Fingerprint = hash of the sorted starting IRIs (browse: seed IRIs; construct / schema: the constructed node IRIs). A mismatch keeps the restored canvas and shows an info message with a **Reset diagram** action. | GL-35. |
| A3 | Restore re-fetches labels and links through the provider (`importLayout` with `validateLinks`); pins, layout choice and label language come from Visoto's part of the file. Undo history starts empty after a restore, reset or open. | GL-18, GL-35. Link-type visibility rides on GE's `linkTypeOptions`; GL-21 reverse settings and GL-49/52 hidden classes join the file in A4/A5. |
| A4 | Add resource (GL-47) on browse graphs searches through a new `GET /api/search?q=&endpoint=&lang=&limit=` (`cmd/visoto/search_api.go`): the /search page's full-text lookup (endpoint's FTS provider, CONTAINS fallback) as JSON, a pure function of the URL like every /api route. Constructed and schema diagrams search their own nodes through the in-memory provider. A pasted `http(s)` IRI is added directly; a resource already on the canvas is selected instead. | GE's `filter({text})` is a regex scan over the whole endpoint and had not answered on LINDAS after 6 s; the Lucene-backed search answers in about a second. |
| A4 | Layout options (GL-20–23) open from Layout ▾ → "Layout options…" as a panel docked top-right; one row per link type on the canvas (label, edge count, reverse switch). Hidden link types are not listed — GE removes their links — and are shown again from GE's Connections panel. Toggles are not undo steps; Redraw runs the current layout as one. | GL-18 excludes the toggles themselves; GL-21 names GE's Connections panel as the hide control. |
| A4 | Reverse settings: only differences from the GL-22 defaults, in `localStorage` (`visoto-graph:reversed`), shared by all graphs; a saved canvas carries them and re-applies them on restore. | GL-22 + the A3 saved-canvas contents. |
| A4 | Details (GL-48): expands every target if any is collapsed, otherwise collapses all; targets are the selection, else every node; one undo step. | Toggle semantics for a single button. |
| A4 | GE connections-menu additions: `editor.onAddElementsInConnectionMenu` is wrapped on the instance — confirmation above 20 (GL-15) before GE adds anything, then a Network pass with every existing node fixed (GL-14) after GE's own ring placement. Drag-and-drop from the class tree / instances panel is left where it was dropped and not confirmed. | The wrapper is the only pre-add hook; a drop position is the user's choice. `GE-UPSTREAM: B2`. |
| A5 | Classes (GL-10, GL-49) and namespaces (GL-52) live in one Visoto panel ("Classes and namespaces", toolbar button), not in GE's left class tree: a flat list of the classes of the nodes on the canvas (eye toggle, count, ⋮ Select all / Remove all), then the namespaces (eye toggle, count). Right-click / long-press on GE's tree is not wired; the panel's ⋮ is the visible equivalent GL-10 allows. | GE's class tree comes from `classTree()`, which times out on LINDAS, and is React DOM that re-renders; the panel works on every endpoint. |
| A5 | "A class includes its subclasses": browse graphs ask the endpoint `?sub rdfs:subClassOf+ ?sup` with both ends bound by VALUES to the classes on the canvas; constructed diagrams have nothing to nest (their nodes' types are metaclasses). The list itself is flat. | Bounded, cheap query; nesting in the list can follow if wanted. |
| A5 | Hiding removes the nodes through GE's commands and keeps a record each (IRI, position, expanded, types, pinned); showing re-creates them (new element ids, pins re-applied) and reloads data and links. A node stays hidden while any class or namespace rule matches. One undo step per toggle; rules and records are saved with the canvas. Blank-node types are not listed. | GE has no visibility flag (`GE-UPSTREAM: B5`). |
| A5 | Namespace = IRI up to its last `#` or `/`; common vocabularies shown by prefix (rdf:, owl:, schema:, prov:, …). Rows sort by count, then name, so they do not jump while toggling. | GL-52. |
| A5 | Find on canvas (GL-32): toolbar search; label or IRI contains the text; matches highlighted through GE's public `setHighlighter`, the rest blurred, view zoomed to the matches; Enter selects them; closing the field clears the highlight. | Enter-to-select makes Find feed the selection actions. |
| A6 | Turtle export (GL-38) writes what is drawn: each node's types and labels, the property rows of expanded nodes, every visible edge; common prefixes only when used; checked with rdflib (243 triples for T1). Writer is pure (`graph-turtle.js`, unit-tested). | GL-38 "the triples currently drawn". |
| A6 | List view (GL-39) is Tabler tables (nodes with a selection checkbox, type, edge count; edges source / property / target) over the canvas area, not `sparqlTable`/Tabulator. Selection is shared both ways. | The data is in memory and small; Tabulator would add a data round trip through the server for nothing. |
| A6 | Help (GL-53): a ? dropdown with the shortcut list and a link to `/graph-help.html` (i18n page). a11y (GL-40, 41): every control has `aria-label`/`title`; `:focus-visible` rings on graph controls; 44 px targets under `pointer: coarse`. | GL-41, GL-40. A full screen-reader pass on a device is still open. |
| A6 | Docs: `docs/templating.md` (partial parameters + what every graph gets), `graph-explorer` skill (file map + GE 2.1 traps), `docs/ontodia-graph-explorer-references.md`, CLAUDE.md, NOTICE (elkjs). Locale keys in all five catalogs throughout. | Plan A6. |

### Test set (LINDAS prod, `endpoint=lindas-prod`)

| # | Page | Covers |
|---|---|---|
| T1 | `/resource?iri=http://www.w3.org/ns/prov-o%23` (owl:Ontology, 30 classes) | UML construct diagram (`ontologyDiagram`) + Graph view + Schema view on one page → all three embeds, multi-graph autosave; > 20 incoming `rdfs:isDefinedBy` for GL-15 |
| T2 | `/resource?iri=https://www.ica.org/standards/RiC/ontology` (owl:Ontology, 593 terms) | large construct diagram: ELK speed, GL-46 Cancel / > 500 warning |
| T3 | `/resource?iri=https://agriculture.ld.admin.ch/system-map/S7DUZFs3emPOPYErT` (schema:SoftwareApplication) | browse mode with two graphs (`dependencyGraph` + Graph view) |
| T4 | class page `schema:SoftwareApplication` (`systemDiagram`) | browse mode on a class template, expand from a hub node |

**Environment note:** this cloud container's network policy blocks
`cached.lindas.admin.ch` (proxy 403), so a locally started server cannot reach
LINDAS here. Browser verification runs on a machine with LINDAS access (local
dev, or the deployed instance after push); pure `graph-layout.js` logic can be
checked here with `node --test`.

### Status

Part A done on this branch (A0–A6, 2026-09-27): every phase checked on T1–T4 in
headless Chromium; JS unit tests `node --test tests/js/`.

GL-54, GL-55 and GL-56–59 done (2026-09-28), checked on T1 in headless Chromium:
pan to a panel entry (`panelIri` reads the class tree's `href` and an Instances
entry's `<IRI>` title), node colours (scoped rules, inlined into the DOM for the
moment GE clones it on export), hover via GE's own `setHighlighter`. The
Instances-panel half of GL-54 is unverified: T1's classes have no instances on
LINDAS.

Part B on hold (2026-09-27): Zazuko has not answered, and Part A already gives users
everything B1–B5 would. Part B would only retire the `GE-UPSTREAM` workarounds;
revisit if Zazuko responds or a GE release breaks one of them.

## 3. Risks

| Risk | Mitigation |
|---|---|
| GE-internal `_setElementDecorator` changes in a GE release | Isolated in `ge-adapter.js`; fallback to public `setHighlighter` |
| Capture-phase pointer listener fights GE's own drag handling | Only active in Select mode or with Shift; verify with Playwright |
| ELK slow on large graphs | Lazy load, Cancel (GL-46), > 500-node warning |
| `localStorage` quota with many named saves | Store positions compactly; surface quota errors in the Save menu |
| Upstream PRs stall | Part A is complete without them; B0 gate falls back to Reactodia spike |
| No JS unit tests | Keep `graph-layout.js` pure; optionally add `node --test` specs (no deps) for it |

## 4. File inventory

New: `static/js/graph-kit.js`, `static/js/ge-adapter.js`,
`static/js/graph-layout.js`, `templates/partials/graph-toolbar.html`,
`static/css/graph_overrides.css` (if Tabler classes are not enough), help page
template.
Changed: `static/js/sparql-graph.js`, `static/js/schema-graph.js`,
`templates/partials/sparql-graph.html`, `templates/partials/schema-graph.html`,
`locales/*.toml`, docs listed in A6.
