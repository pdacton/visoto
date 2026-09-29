# Graph Explorer — overview, layout, selection and saving

Status: **feature request**, no code written. Requirement IDs (GL-n) are stable —
cite them in issues and PRs. IDs are never renumbered; amended requirements keep
their ID.

Applies to all three Graph Explorer embeds: the resource graph
(`static/js/sparql-graph.js`, browse mode), the owl:Ontology UML diagram (same
file, construct mode) and the schema view (`static/js/schema-graph.js`). "Ontology
diagram" below means the latter two. Implementation plan: `graph-layout-plan.md`.

**Scope of a canvas:** one canvas per `sparqlGraph` instance in a template. A page
may contain several graphs; each has its own toolbar, selection, history and saved
state, keyed by page URL + the partial's `id` parameter.

---

## 1. Why

**Goal:** the user gains an overview of an ontology or RDF dataset and can quickly
add, rearrange and remove elements on the canvas — and save the result. Usage must
stay simple and accessible: few visible controls by default, every action reachable
without hidden gestures, keyboard and touch included.

Today node placement is one `workspace.forceLayout()` call after the links load:
WebCola with a fixed 200 px link length, 30 iterations, whole graph only. Nodes
added from the connections menu land on a fixed 300 px half-circle with no overlap
check. There is no way to lay out part of the diagram, pick another algorithm,
select more than one node, keep a hand-made arrangement, or save the canvas.

## 2. Principles

- **Progressive disclosure.** Default toolbar: **Add resource · Layout ▾ ·
  Pan/Select · Undo/Redo · Fit · Details · Language ▾ · Save / Export ▾ · ?** (the
  existing Fullscreen button stays in the card header). Selection actions appear only while something is selected; expert
  settings (GL-20–23) sit in a collapsed "Layout options" panel.
- **No hidden-only gestures.** Every shortcut (Shift+drag, Ctrl/Cmd+click,
  right-click) has a visible equivalent.
- **Respect the user's arrangement.** Hand-placed nodes are not scattered by later
  layouts (GL-26).
- **Never flood the canvas.** Large additions need confirmation (GL-15).

## 3. User steps

### Page load

- **GL-1** The user opens a page with a diagram.
- **GL-2** The diagram is rendered with the page's default algorithm: Network for
  the resource graph, Tree → for the ontology diagram.
- **GL-3** *(merged into GL-35)* The layout choice is restored with the autosaved
  canvas; a page without one uses the GL-2 default. No separate layout preference.
- **GL-4** A spinner shows while the layout computes; afterwards the diagram
  zooms to fit.

### Selection

- **GL-5** *(amended)* A **Pan / Select** toggle in the toolbar sets what a plain
  drag on empty canvas does; the canvas opens in **Pan** mode. In Select mode it draws a selection box that replaces
  the selection. **Shift+drag** is the shortcut for a box in Pan mode. Adding to a
  selection is done with GL-6.
- **GL-6** **Ctrl/Cmd+click** toggles a node in or out of the selection (Cmd on
  macOS, where Ctrl+click is the context menu). In Select mode a plain tap toggles
  too (touch).
- **GL-7** *(amended)* Selected nodes are marked by outline **and** a check mark —
  never by colour alone; the toolbar shows a count ("12 selected").
- **GL-8** Click on empty canvas or **Esc** clears the selection; **Ctrl+A** selects
  all.
- **GL-9** Dragging any selected node moves the whole selection.

### Class tree (left sidebar)

- **GL-10** *(amended)* Right-click, long-press, or a visible "⋮" button on a class
  opens: **Select all**, **Remove all**. A class includes its subclasses
  (`rdf:type/rdfs:subClassOf*`), matching the nesting shown in the tree.
  - Select all — select that class's nodes already on the diagram.
  - Remove all — remove that class's nodes from the diagram.
  - *"Add all" dropped:* GE's Instances panel already lists a class's instances
    for adding.
- **GL-49** An eye toggle per class in the class tree **hides / shows** that class's
  nodes on the canvas without removing them. Hidden nodes are left out of layouts
  and exports; the tree shows which classes are hidden. Saved with the canvas.

### Toolbar

- **GL-11** *(amended)* Layout menu, plain names with icons: **Network** (force),
  **Tree ↓** (hierarchical top-down), **Tree →** (left-to-right), **Radial**.
- **GL-12** Selection actions, enabled when ≥ 1 node is selected:
  **Expand all**, **Remove**, **Keep only these**, **Layout selection**,
  **Select neighbours**, **Pin / Unpin** (GL-26). **Fit** (always visible) zooms to
  the selection when there is one, otherwise to the whole canvas.
- **GL-13** Layout selection uses the current algorithm, keeps every unselected
  node fixed, and keeps the selection's centroid where it was.
- **GL-14** Nodes added by an expand (halo menu or Expand all) are placed without
  moving existing nodes.
- **GL-15** Any addition shows the number of nodes to be added and asks for
  confirmation when it exceeds **20**.
- **GL-41** *(amended)* Every toolbar control has an `aria-label` and a visible
  focus ring. Keyboard shortcuts: Esc, Ctrl/Cmd+A, Del, Ctrl/Cmd+Z / Shift+Z; a
  **?** button lists them (see GL-53).
- **GL-44** **Toolbar parity with GE's toolbar**, which ours replaces — nothing it
  offers today is lost:
  - **Save / Export ▾:** Save as… / My diagrams (GL-50), Download / Open file
    (GL-36), Export Turtle (GL-38), **SVG**, **PNG**, **Print**; **Reset diagram**
    (GL-35) and **Clear all** at the bottom, both with confirmation.
  - **Language ▾:** label language switch, as in GE today; defaults to the site
    language (`site-lang`).
- **GL-47** **Add resource:** a search field that finds resources by label in the
  endpoint (same lookup as GE's Instances panel) or accepts a pasted IRI; the chosen
  resource is added and placed without moving existing nodes (GL-14).
- **GL-48** **Details** toggle: collapse or expand the detail pane (property card)
  of all nodes at once — compact nodes for an overview, full cards for reading.
  Applies to the selection if there is one, otherwise to all nodes.
- **GL-51** **Align / distribute** (selection actions, ≥ 2 nodes): align left,
  centre, right, top, middle, bottom; distribute horizontally / vertically. Aligned
  nodes are pinned (GL-26).
- **GL-53** **Help:** the **?** button shows the shortcut list and links to a short
  guide page on using the graph (i18n, like all pages).

### Layout behaviour

- **GL-16** Choosing an algorithm computes new positions and redraws — the
  selection if there is one, otherwise the whole graph. Edges stay straight (link
  vertices cleared); no edge routing.
- **GL-17** Disconnected components are always packed side by side, never
  scattered.
- **GL-18** *(amended)* **Undo / Redo** buttons in the toolbar (and Ctrl/Cmd+Z,
  Ctrl/Cmd+Shift+Z) cover **every** canvas change, one step per user action:
  - Visoto actions: layouts (GL-16, Redraw GL-23), Layout selection, pin/unpin,
    group drag (GL-9), Expand all, Remove, Keep only these, class-tree Add/Remove
    all (GL-10).
  - GE's native actions: node drag, halo remove, connections-menu add, drag-drop
    from the class tree / instances panel, Clear all, link-type visibility, GE's
    own Force layout.
  - Also: hide/show class (GL-49), align/distribute (GL-51), Add resource (GL-47),
    collapse/expand all details (GL-48).
  - Buttons are disabled when there is nothing to undo/redo; the tooltip names the
    step ("Undo: Layout — Tree ↓").
  - Not undo steps: selection, pan/zoom, the GL-21 reverse toggles themselves (the
    Redraw they trigger is), and preferences.
  - **Reset diagram** and **Open file** (GL-35, GL-36) ask for confirmation and
    start a fresh history. History is not persisted across page reloads.
- **GL-19** **Radial** is a radial explosion from a centre node: the single
  selected node, else the page's own resource, else the highest-degree node.
  Nodes sit on rings by hop distance; each subtree gets an angular wedge sized by
  its node count, so tree edges do not cross. Non-tree edges are drawn but do not
  drive placement.
- **GL-26** **Pinning.** A node the user drags is pinned automatically (small pin
  badge). Every layout treats pinned nodes as fixed. Clicking the badge, or
  Pin / Unpin on a selection, toggles it.

### Layout options panel (collapsed by default; nothing selected)

- **GL-20** Lists every property (link type) on the diagram with its edge count.
- **GL-21** *(amended)* Per property: **reverse for layout**.
  - Reverse affects Tree ↓ and Tree → only; arrows keep their RDF direction. It has
    no effect on Network or Radial, which ignore edge direction.
  - There is no separate "exclude": a property **hidden** (GE's Connections panel)
    is also left out of the layout input of every algorithm, Radial included
    (centre choice and rings). Nothing is hidden by default (see §4).
- **GL-22** Reversed by default: `rdf:type`, `rdfs:subClassOf`, `skos:broader`,
  `schema:isPartOf` — so superclasses / broader concepts / wholes sit on top.
  User changes are remembered per property IRI (`localStorage`), shared by both
  diagrams. The partial's existing `hideTypeEdges` parameter only sets the
  *initial* visibility of `rdf:type` / `rdfs:subClassOf`; the user can show them
  again.
- **GL-23** A dedicated **Redraw** button applies reverse/visibility changes; it is
  highlighted while there are unapplied changes.

### Orientation

- **GL-32** **Find on canvas:** a search field that highlights and zooms to
  matching nodes already on the diagram (GE's search queries the endpoint, not the
  canvas).

### Saving

A saved canvas holds: node IRIs, positions, pinned state, link-type visibility,
reverse settings (GL-21), layout choice, endpoint slug, language. Labels
and data are re-fetched on load, so a saved canvas never shows stale labels; IRIs
that no longer resolve are shown as bare IRIs, not dropped.

- **GL-35** *(amended)* **Autosave** per graph instance (page URL + graph `id`) in
  `localStorage`; reopening the page restores each graph's canvas. The save
  stores a fingerprint of the graph's starting resources; if the page now seeds
  different ones, the restored canvas shows "Page content changed — reset?". A **Reset diagram** action (Save ▾) returns to the page default.
- **GL-36** **Download** the canvas as a `.visoto-graph.json` file (based on GE's
  `SerializedDiagram`) and **Open** such a file onto the canvas. The file carries
  the endpoint slug; opening it switches to that endpoint (absorbs GL-37).
- **GL-50** **Save as…** stores the canvas under a name in the browser
  (`localStorage`); **My diagrams** lists saved canvases (name, endpoint, date) to
  open into this graph, rename or delete.
- **GL-38** **Export as Turtle:** the triples currently drawn on the canvas, next to
  the existing SVG/PNG export.

### Accessibility

- **GL-39** **List view** toggle: the canvas as a table of nodes and edges (reusing
  `sparqlTable` styling), the screen-reader equivalent of the diagram; selection is
  shared between both views.
- **GL-40** Touch: Select mode (GL-5), long-press for menus (GL-10), hit targets
  ≥ 44 px.
- **GL-41** — see Toolbar.

### Robustness

- **GL-45** Endpoint errors and timeouts during load, expand or add show an inline
  message on the canvas with **Retry**; an expand or search that finds nothing says
  so instead of silently doing nothing.
- **GL-46** A running layout shows a spinner with **Cancel**. Above ~500 nodes the
  user is warned that layouts may be slow before one starts.

### Ontology diagram

- **GL-52** **Filter by namespace:** a list of the namespaces present on the
  diagram, each with a hide/show toggle (e.g. hide external vocabularies such as
  `schema:` or `owl:`). Builds on the existing shading of external classes; works
  like GL-49 (non-destructive, saved with the canvas).

### Ontology files (not in the triple store)

Design and rationale: §8. Terms: a **source** is a URL (GitHub or any HTTP(S)
host) or a local file; **preview** shows it without writing anywhere; **load**
writes it into a triple store as a named graph.

- **GL-54** **Open an ontology file** from a URL or a local file (drag & drop).
  Formats: Turtle, N-Triples, RDF/XML (`.rdf`/`.owl`/`.xml`), JSON-LD — as
  `/api/upload` already recognises them (`mimeForExtension`). GitHub `blob/` URLs are rewritten to their raw form; a
  URL may pin a branch, tag or commit.
- **GL-55** **Preview is the default:** the file is shown immediately without
  being written to any store. It gets its own page — the owl:Ontology page layout
  (UML diagram, term tables) rendered from the file — at a URL that identifies
  the source (`/ontology-file?src=<url>`), so it is shareable, bookmarkable and a
  pure, cacheable function of the URL. A local file is addressed by content hash
  (`?sha256=…`) and lives only as long as the server's cache.
- **GL-56** Everything reachable from the preview stays inside the preview: a
  node or term link opens that term's page **scoped to the file**, not the
  endpoint (where it may not exist). A banner states the source, its version
  (GL-60) and "Not in a triple store — preview".
- **GL-57** For each term, the preview says whether the IRI **also exists in the
  current endpoint**, with a link to the endpoint's page; the diagram marks such
  terms (e.g. a small badge), so the user sees what the file adds or overrides.
- **GL-58** **Load into triple store** is an explicit action on the preview page,
  never automatic. It reuses `/api/upload` (URL mode) and needs a writable
  endpoint with credentials; it is hidden when no such endpoint is configured.
  After loading, the banner links to the loaded ontology's normal page.
- **GL-59** **Target graph** for a load is derived, not typed: the ontology's
  `owl:versionIRI` if present, else `urn:ontology:<ontology-IRI>@<version>` (GL-60).
  Reloading the same version replaces that graph; a different version goes into
  its own graph. The user can override the name before loading.
- **GL-60** **Version identity** of a source, in this order: `owl:versionIRI`,
  `owl:versionInfo`, the pinned Git ref / commit, else the content hash (short).
  Shown on the banner, stored with loads, part of the preview cache key.
- **GL-61** **Multiple versions:** where the endpoint holds more than one graph
  whose `owl:Ontology` has the same IRI (or where the preview's ontology IRI
  already exists in the endpoint), the owl:Ontology page shows a **version
  selector**, and the diagram and term tables are scoped to the selected
  version's graph — never a silent union of two versions.
- **GL-62** A **term page** for an IRI defined in several graphs shows a
  "Defined in" list (graph, ontology, version) and marks values that differ
  between them (label, comment, domain, range, superclass).
- **GL-63** `owl:imports` are listed on the preview page. Imported terms appear on
  the diagram as external (existing shading), not fetched; an **Open import**
  link previews the imported ontology on its own (GL-55).
- **GL-64** Limits and safety: size cap (configurable, e.g. 20 MB), fetch timeout,
  the existing SSRF guard (`allow_private_upload_urls`); parse errors are shown
  with line/column instead of an empty page.

### Across the board

- **GL-24** Everything works identically on the resource graph and the ontology
  diagram.
- **GL-25** All strings go through i18n (de, en, fr, it, rm).

### Retired and deferred IDs

Not reused. Deferred items live in `graph-later.md`.

| ID | Status | Reason |
|---|---|---|
| GL-3 | merged → GL-35 | one stored state instead of two |
| GL-27 | deferred | animation; undo covers a surprising layout |
| GL-28 | deferred | Class map starting view |
| GL-29 | removed | multi-node per-property picker; Expand all + GL-15 suffices, GE's menu covers one node |
| GL-30 | removed | "+N more" placeholder would be a fake model node (breaks save/export/undo) |
| GL-31 | removed | focus mode overlaps Select neighbours + selection highlight |
| GL-33 | removed | legend; colours come from a resolver, cards show the type |
| GL-34 | removed | hover card duplicates GE's node card and IRI link |
| GL-37 | merged → GL-36 | |
| GL-42 | deferred | share link, needs a decision on anonymous writes |
| GL-43 | removed | first-visit hint; the Pan/Select toggle is visible |

### Known limitations

- Blank nodes are not shown (`acceptBlankNodes: false`), so OWL restrictions and
  other blank-node structures in ontologies are not visible. Deliberate for now.

## 4. Why `rdf:type` is not excluded by default

Excluding `rdf:type` is the usual remedy when one class node is linked to many
instances: in Network it becomes a hub that pulls everything together, and in
Radial the highest-degree fallback (GL-19) would pick the class as centre, putting
every instance in ring 1. It is left **included** because instances of a class are
frequently linked to each other only through their shared class node — excluding
the type edge would then fragment the graph into loose components, and the
class-grouping it gives is usually what the user wants to see. The user can
leave it out by hiding `rdf:type` (GL-21).

## 5. Suggested delivery order

1. GL-44, 11, 16–19, 13, 18, 45–46 — own toolbar with GE parity, layout engines
   and menu, Undo/Redo from the start so every later action lands in the same
   history, error handling.
2. GL-5–9, 12, 26, 51 — selection, selection actions, pinning, align/distribute.
3. GL-35–36, 50 — autosave, file save/open, named saves.
4. GL-20–23, 47–48 — layout options panel, Add resource, Details toggle.
5. GL-10, 49, 52, 32 — class-tree menu and hide, namespace filter, find on canvas.
6. GL-38–41, 53 — Turtle export, list view, touch and a11y polish, help page.
7. GL-54–64 — ontology files: preview store + entry route, then load, then
   version-aware pages (§8).

## 6. Implementation notes (from the brainstorming, non-binding)

- Everything above is reachable through GE 2.1.0's public API; no fork. Checked
  against the 2.1.0 `.d.ts` and `dist/graph-explorer.js`:
  - `GE.calculateLayout` / `GE.applyLayout` / `GE.forceLayout`, the latter with
    `fixedElements` and `selectedElements` (GL-13, GL-14, GL-26).
  - `calculateLayout`'s `layoutFunction` is synchronous; ELK (`elkjs`) is async,
    so ELK modes build the ELK graph from `model.elements` / `model.links`, await
    `elk.layout()`, then `setPosition` inside `model.history.startBatch()`
    (GL-18).
  - Shift/Ctrl/Alt+drag does not pan (`shouldStartPanning`), so it is free for the
    selection box; modifier clicks are ignored by the editor but still reach
    `onPointerUp` (GL-5, GL-6). `workspace._getPaperArea().pageToPaperCoords()`
    converts coordinates. `editor.setSelection([...])` accepts many elements. A
    plain-drag Select mode (GL-5) needs a capture-phase pointer listener that
    suppresses GE's panning.
  - Multi-selection feedback: `view.setHighlighter()` (public) or
    `view._setElementDecorator()` (internal); the latter also suits the pin badge.
  - `editor.events.on('addElements', …)` fires after halo-menu placement (GL-14).
  - Details toggle (GL-48): `element.setExpanded(bool)` per element, in one batch.
  - Hiding classes / namespaces (GL-49, GL-52): GE has no element visibility flag;
    remove the elements into a Visoto-held "hidden" set and re-add them on show
    (keeping positions, restoring their links from the model's link cache or by
    re-requesting links), each as one undoable command.
  - Undo (GL-18): GE already records its native actions in `model.history`
    (`CommandHistory`: drag via `RestoreGeometry`, `removeItems`, connections-menu
    add, drag-drop, link-type visibility, `forceLayout`), but has **no** undo
    button or Ctrl+Z binding — `workspace.undo()` / `redo()` exist unused. Visoto
    actions must go through the same history: wrap in `history.startBatch(title)`
    / `batch.store()`, and use `GE.Command.create` / `Command.effect` for state
    outside GE's model (pinned flags). Button state from the `historyChanged`
    event plus `undoStack` / `redoStack` lengths; titles from `Command.title`.
  - Saving: `model.exportLayout()` / `model.importLayout({ diagram })`; pinned
    state and GL-21 settings are Visoto metadata alongside it.
  - Hidden link types (GL-21): filter them out of the links passed to the layout;
    GE's own `calculateLayout` only checks node visibility.
- Engines: Network = WebCola (built-in); Tree ↓ / Tree → = ELK `layered` with
  `elk.direction` DOWN / RIGHT; Radial = ELK `radial`; component packing =
  ELK `separateConnectedComponents` or own shelf packing.
- `elkjs` is loaded lazily from jsDelivr on first use (the Mermaid ELK plugin's
  bundled copy is not reachable from GE). Main thread is fine below ~500 nodes.
- Keep the layout logic (ELK graph building, reversal, hidden-type filtering,
  pinning) in a module that does not depend on GE internals, so it survives a
  later move to Reactodia.
- GE's own right-hand **Connections** panel stays; it is the expand UI when a node
  is selected. The GL-20 panel is Visoto markup, not injected into GE's React DOM.

## 7. Alternative considered — Reactodia

`@reactodia/workspace` (0.35.2, LGPL-2.1+), checked against its source:

- **Covers:** GL-5 (Shift+drag box, plus a pan/select pointer-mode toggle in the
  zoom control), GL-6 (but Shift+click, not Ctrl/Cmd), GL-7, GL-8 (`Mod+A`),
  GL-9, GL-13 (`performLayout({ selectedElements, fixedElements, layoutFunction,
  animate, zoomToFit, signal })`), GL-16 (async `LayoutFunction`), GL-18, layout
  worker, animation (deferred GL-27). Selection actions exist but dock around the
  selection box, not in a toolbar. `colaFlowLayout` gives a flow-constrained
  hierarchy — usable, but not ELK layered.
- **Partial:** GL-12 (its "Expand" expands the node template, not neighbours),
  GL-14 (same `editor:addElements` event as GE), GL-21 (visibility only), GL-25
  (translation mechanism, `en` only).
- **Missing:** GL-10, 11, 15, 17, 19–23, 26, 32, 35–41, 44–53 — the same work in
  either library.

Costs: ESM-only with a React peer dependency (import map or a Vite build), and a
rewrite of `sparql-graph.js`, `schema-graph.js`, `graph-memory-store.js` and the
CSS overrides. It saves mainly the selection block. Worth a time-boxed spike
against LINDAS via `/api/sparql`; it does not block the layout work. Forking GE is
the last resort.

## 8. Ontology files — design discussion

### 8.1 Where is the ontology shown if it is not in the store?

Every Visoto page is built from SPARQL against an endpoint, so a file-only
ontology has no page by default, and every link out of its diagram would land on
an empty `/resource` page. Options:

| Option | What renders | Cost |
|---|---|---|
| a. Browser-only: parse in JS (N3.js), project to the UML diagram in JS | the diagram only; term links dead | duplicates the `$ontologyDiagram` CONSTRUCT in JS |
| b. Browser SPARQL store (Oxigraph WASM) | the diagram, same CONSTRUCT unchanged | several MB of WASM; still only the diagram |
| **c. Server-side preview store** exposed as a **virtual endpoint** per source | **all templates** (owl:Ontology, owl:Class, schema view, tables) unchanged, scoped to the file | an in-process or sidecar SPARQL store; new service in `docker-compose.yml`, `Dockerfile`, `deploy.sh` |

**Recommendation: c.** It keeps "every page is SPARQL" intact: the preview is just
another endpoint whose slug identifies the source (`file:<hash>`), so GL-56's
"links stay inside the preview" falls out of the existing `endpoint=<slug>`
mechanism. Candidate stores: the existing Docker-private QLever (if it accepts
fast ad-hoc loads/updates — to verify) or a small Oxigraph sidecar (in-memory,
SPARQL 1.1 + `LOAD`). Evict previews by LRU / TTL.

The preview page (`/ontology-file`) is a thin entry route: fetch → hash → load
into the preview store if not cached → redirect to the owl:Ontology resource page
with `endpoint=file:<hash>`. That keeps `/resource` a pure function of its URL.

GL-57 ("also in the endpoint") needs one extra query per page against the user's
current endpoint, e.g. a `VALUES`-batched `ASK`/`SELECT` over the term IRIs.

### 8.2 Load into the store always, or as an option?

**Option, default preview.** Reasons:
- LINDAS prod and most configured endpoints are read-only; writes need a token.
- Loading is persistent and shared: it changes what every other user sees and
  can introduce a second version of an ontology next to the official one (8.3).
- Most "look at this ontology" use cases are exploratory; a preview is instant
  and leaves nothing behind.
- The upload pipeline already exists (`/api/upload`, `[[ontologies]]`,
  named-graph listing / delete), so "load" is a button on the preview, not new
  infrastructure.

### 8.3 Versioning

- **Identity** = ontology IRI + version (GL-60). A Git commit SHA is the most
  reliable version for GitHub sources; `owl:versionInfo` strings are free text and
  not necessarily unique.
- **Isolation**: one version per named graph (GL-59). Never merge two versions
  into one graph — the same `rdfs:label`, `rdfs:domain`, `rdfs:range` would then
  carry values from both, and the diagram could not tell them apart.
- **Queries must become graph-aware** where more than one version exists (GL-61):
  the owl:Ontology CONSTRUCT and term tables get an optional `GRAPH ?g` scope
  chosen by the version selector. On an endpoint whose default graph is the
  union of its named graphs (to check per endpoint, LINDAS included), unscoped
  queries would otherwise show conflated terms.
- **Conflicts on term pages** (GL-62): same IRI, different definitions →
  "Defined in" list with the differing values marked.
- **Deferred:** a **version diff** view (terms added / removed / changed between
  two versions), and following `owl:priorVersion` chains. Both fit naturally on
  top of GL-61 but are not needed for a first release.

### 8.4 Open questions

1. Preview store: reuse the private QLever or add an Oxigraph sidecar? Needs a
   quick load-time test with T2 (RiC, 593 terms) as a `.ttl`.
2. Should previews be available to anonymous users (server cost, abuse) or only
   when a config flag enables them?
3. Private GitHub repositories (token per user) — out of scope for the first
   release?
