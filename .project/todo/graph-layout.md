# Graph Explorer — overview, layout, selection and saving

Status: **feature request**, no code written. Requirement IDs (GL-n) are stable —
cite them in issues and PRs. IDs are never renumbered; amended requirements keep
their ID.

Applies to both Graph Explorer embeds: the resource graph
(`static/js/sparql-graph.js`) and the ontology diagram (`static/js/schema-graph.js`).

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

- **Progressive disclosure.** Default toolbar: **Layout ▾ · Pan/Select · Undo/Redo ·
  Fit · Save ▾**. Selection actions appear only while something is selected; expert
  settings (GL-20–23) sit in a collapsed "Layout options" panel.
- **No hidden-only gestures.** Every shortcut (Shift+drag, Ctrl/Cmd+click,
  right-click) has a visible equivalent.
- **Respect the user's arrangement.** Hand-placed nodes are not scattered by later
  layouts (GL-26).
- **Never flood the canvas.** Additions are bounded and chosen, not dumped (GL-15,
  GL-29–30).

## 3. User steps

### Page load

- **GL-1** The user opens a page with a diagram.
- **GL-2** The diagram is rendered with the page's default algorithm: Network for
  the resource graph, Tree → for the ontology diagram.
- **GL-3** A layout the user chose earlier is remembered per diagram type
  (`localStorage`) and wins over the default. A saved canvas (GL-37) wins over both.
- **GL-4** A spinner shows while the layout computes; afterwards the diagram
  zooms to fit.
- **GL-43** On first visit a dismissible one-line hint explains the basics
  ("Drag to pan · switch to Select to pick several nodes"); dismissal is remembered.

### Selection

- **GL-5** *(amended)* A **Pan / Select** toggle in the toolbar sets what a plain
  drag on empty canvas does. In Select mode it draws a selection box that replaces
  the selection. **Shift+drag** is the shortcut for a box in Pan mode;
  **Ctrl/Cmd+Shift+drag** adds to the selection.
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
  opens: **Select all**, **Add all**, **Remove all**. A class includes its
  subclasses (`rdf:type/rdfs:subClassOf*`), matching the nesting shown in the tree.
  - Select all — select that class's nodes already on the diagram.
  - Add all — load instances onto the diagram (GL-15 / GL-30 apply).
  - Remove all — remove that class's nodes from the diagram.

### Toolbar

- **GL-11** *(amended)* Layout menu, plain names with icons: **Network** (force),
  **Tree ↓** (hierarchical top-down), **Tree →** (left-to-right), **Radial**.
- **GL-12** Selection actions, enabled when ≥ 1 node is selected:
  **Expand…** (GL-29), **Remove**, **Keep only these**, **Layout selection**,
  **Select neighbours**, **Zoom to selection**, **Pin / Unpin** (GL-26).
- **GL-13** Layout selection uses the current algorithm, keeps every unselected
  node fixed, and keeps the selection's centroid where it was.
- **GL-14** Nodes added by an expand (halo menu or Expand…) are placed without
  moving existing nodes.
- **GL-15** Any addition shows the number of nodes to be added and asks for
  confirmation when it exceeds **20**.
- **GL-41** A **?** button lists all keyboard shortcuts; every toolbar control has
  an `aria-label`, a visible focus ring and a keyboard shortcut.

### Layout behaviour

- **GL-16** Choosing an algorithm computes new positions and redraws — the
  selection if there is one, otherwise the whole graph. Edges stay straight (link
  vertices cleared); no edge routing.
- **GL-17** Disconnected components are always packed side by side, never
  scattered.
- **GL-18** Every layout, add, remove, pin and move is undoable with Ctrl+Z.
- **GL-19** **Radial** is a radial explosion from a centre node: the single
  selected node, else the page's own resource, else the highest-degree node.
  Nodes sit on rings by hop distance; each subtree gets an angular wedge sized by
  its node count, so tree edges do not cross. Non-tree edges are drawn but do not
  drive placement.
- **GL-26** **Pinning.** A node the user drags is pinned automatically (small pin
  badge). Every layout treats pinned nodes as fixed. Clicking the badge, or
  Pin / Unpin on a selection, toggles it.
- **GL-27** Layout changes animate node positions (~300 ms) so the user can follow
  what moved; no animation under `prefers-reduced-motion`.

### Layout options panel (collapsed by default; nothing selected)

- **GL-20** Lists every property (link type) on the diagram with its edge count.
- **GL-21** Per property: **reverse for layout**, **exclude from layout**,
  **hide/show**.
  - Reverse affects Tree ↓ and Tree → only; arrows keep their RDF direction. It has
    no effect on Network or Radial, which ignore edge direction.
  - Exclude removes the property's edges from the layout input of every
    algorithm, Radial included (centre choice and rings). Nothing is excluded by
    default (see §4).
- **GL-22** Reversed by default: `rdf:type`, `rdfs:subClassOf`, `skos:broader`,
  `schema:isPartOf` — so superclasses / broader concepts / wholes sit on top.
  User changes are remembered per property IRI (`localStorage`), shared by both
  diagrams.
- **GL-23** A dedicated **Redraw** button applies the changes; it is highlighted
  while there are unapplied changes.

### Controlled growth and orientation

- **GL-29** **Expand…** on one or more nodes opens a per-property picker with
  counts ("member (312) · type (1) · name (1)"); the user ticks which properties
  to follow. Same pattern as GE's single-node connections menu, extended to a
  selection.
- **GL-30** When an addition exceeds the GL-15 limit and the user does not confirm
  all, the first 20 are added plus a **"+N more"** placeholder node; activating it
  adds the next page.
- **GL-31** **Focus mode:** with one node selected, everything except it and its
  direct neighbours is dimmed. Toggle in the toolbar.
- **GL-32** **Find on canvas:** a search field that highlights and zooms to
  matching nodes already on the diagram (GE's search queries the endpoint, not the
  canvas).
- **GL-33** **Legend** of class colours (from `typeStyleResolver`), collapsible.
- **GL-34** **Hover card** on a node: label, type(s), degree, link to its Visoto
  page. Also shown on keyboard focus.

### Saving

A saved canvas holds: node IRIs, positions, pinned state, link-type visibility,
reverse/exclude settings (GL-21), layout choice, endpoint slug, language. Labels
and data are re-fetched on load, so a saved canvas never shows stale labels; IRIs
that no longer resolve are shown as bare IRIs, not dropped.

- **GL-35** **Autosave** per page in `localStorage`; reopening the page restores the
  canvas. A **Reset diagram** action (Save ▾) returns to the page default.
- **GL-36** **Download** the canvas as a `.visoto-graph.json` file (based on GE's
  `SerializedDiagram`) and **Open** such a file onto the canvas.
- **GL-37** Opening a saved file on a page for a different endpoint warns and
  offers to switch endpoint.
- **GL-38** **Export as Turtle:** the triples currently drawn on the canvas, next to
  the existing SVG/PNG export.

### Accessibility

- **GL-39** **List view** toggle: the canvas as a table of nodes and edges (reusing
  `sparqlTable` styling), the screen-reader equivalent of the diagram; selection is
  shared between both views.
- **GL-40** Touch: Select mode (GL-5), long-press for menus (GL-10), hit targets
  ≥ 44 px.
- **GL-41** — see Toolbar.

### Across the board

- **GL-24** Everything works identically on the resource graph and the ontology
  diagram.
- **GL-25** All strings go through i18n (de, en, fr, it, rm).

### Later / to decide

- **GL-28** *(deferred)* **Class map** starting view for datasets: classes as nodes
  with instance counts, edges = properties used between their instances, weighted
  by frequency. One aggregate query, async + cached; slow on LINDAS without VoID or
  statistics. Data-side counterpart of the ontology diagram.
- **GL-42** *(to decide)* **Share link:** canvas stored server-side in the SQLite
  file under `./data`, opened via `/graph/<id>`. Needs a size cap, TTL and abuse
  limits for anonymous writes; must be a separate route so `/resource` stays a pure,
  cacheable function of the URL. Not encodable in the URL (IRIs blow the ~8 KB
  limit).

## 4. Why `rdf:type` is not excluded by default

Excluding `rdf:type` is the usual remedy when one class node is linked to many
instances: in Network it becomes a hub that pulls everything together, and in
Radial the highest-degree fallback (GL-19) would pick the class as centre, putting
every instance in ring 1. It is left **included** because instances of a class are
frequently linked to each other only through their shared class node — excluding
the type edge would then fragment the graph into loose components, and the
class-grouping it gives is usually what the user wants to see. The user can
exclude it per diagram via GL-21.

## 5. Suggested delivery order

1. GL-11, 16–19, 13 — layout engines and menu (ELK + WebCola).
2. GL-5–9, 12, 26 — selection, selection actions, pinning.
3. GL-35–36 — autosave and file save/open.
4. GL-20–23 — layout options panel.
5. GL-29–34, 10 — controlled growth, orientation, class-tree menu.
6. GL-38–41, 43, 27 — Turtle export, list view, a11y polish, animation.
7. GL-28, 42 — after a decision.

## 6. Implementation notes (from the brainstorming, non-binding)

- Everything above is reachable through GE 2.1.0's public API; no fork. Checked
  against the 2.1.0 `.d.ts` and `dist/graph-explorer.js`:
  - `GE.calculateLayout` / `GE.applyLayout` / `GE.forceLayout`, the latter with
    `fixedElements` and `selectedElements` (GL-13, GL-14, GL-26).
  - `calculateLayout`'s `layoutFunction` is synchronous; ELK (`elkjs`) is async,
    so ELK modes build the ELK graph from `model.elements` / `model.links`, await
    `elk.layout()`, then `setPosition` inside `model.history.startBatch()`
    (GL-18). Animation (GL-27) = interpolated `setPosition` over frames.
  - Shift/Ctrl/Alt+drag does not pan (`shouldStartPanning`), so it is free for the
    selection box; modifier clicks are ignored by the editor but still reach
    `onPointerUp` (GL-5, GL-6). `workspace._getPaperArea().pageToPaperCoords()`
    converts coordinates. `editor.setSelection([...])` accepts many elements. A
    plain-drag Select mode (GL-5) needs a capture-phase pointer listener that
    suppresses GE's panning.
  - Multi-selection feedback: `view.setHighlighter()` (public) or
    `view._setElementDecorator()` (internal); the latter also suits the pin badge.
  - `editor.events.on('addElements', …)` fires after halo-menu placement (GL-14).
  - Saving: `model.exportLayout()` / `model.importLayout({ diagram })`; pinned
    state and GL-21 settings are Visoto metadata alongside it.
- Engines: Network = WebCola (built-in); Tree ↓ / Tree → = ELK `layered` with
  `elk.direction` DOWN / RIGHT; Radial = ELK `radial`; component packing =
  ELK `separateConnectedComponents` or own shelf packing.
- `elkjs` is loaded lazily from jsDelivr on first use (the Mermaid ELK plugin's
  bundled copy is not reachable from GE). Main thread is fine below ~500 nodes.
- Keep the layout logic (ELK graph building, reversal, exclusion, pinning) in a
  module that does not depend on GE internals, so it survives a later move to
  Reactodia.
- GE's own right-hand **Connections** panel stays; it is the expand UI when a node
  is selected. The GL-20 panel is Visoto markup, not injected into GE's React DOM.

## 7. Alternative considered — Reactodia

`@reactodia/workspace` (0.35.2, LGPL-2.1+), checked against its source:

- **Covers:** GL-5 (Shift+drag box, plus a pan/select pointer-mode toggle in the
  zoom control), GL-6 (but Shift+click, not Ctrl/Cmd), GL-7, GL-8 (`Mod+A`),
  GL-9, GL-13 (`performLayout({ selectedElements, fixedElements, layoutFunction,
  animate, zoomToFit, signal })`), GL-16 (async `LayoutFunction`), GL-18, GL-27
  (`animate`), layout worker. Selection actions exist but dock around the
  selection box, not in a toolbar. `colaFlowLayout` gives a flow-constrained
  hierarchy — usable, but not ELK layered.
- **Partial:** GL-12 (its "Expand" expands the node template, not neighbours),
  GL-14 (same `editor:addElements` event as GE), GL-21 (visibility only), GL-25
  (translation mechanism, `en` only).
- **Missing:** GL-10, 11, 15, 17, 19–23, 29–43 — the same work in either library.

Costs: ESM-only with a React peer dependency (import map or a Vite build), and a
rewrite of `sparql-graph.js`, `schema-graph.js`, `graph-memory-store.js` and the
CSS overrides. It saves mainly the selection block. Worth a time-boxed spike
against LINDAS via `/api/sparql`; it does not block the layout work. Forking GE is
the last resort.
