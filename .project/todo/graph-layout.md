# Graph Explorer — layout, selection and property panel

Status: **feature request**, no code written. Requirement IDs (GL-n) are stable —
cite them in issues and PRs.

Applies to both Graph Explorer embeds: the resource graph
(`static/js/sparql-graph.js`) and the ontology diagram (`static/js/schema-graph.js`).

---

## 1. Why

Node placement today is one `workspace.forceLayout()` call after the links load:
WebCola with a fixed 200 px link length, 30 iterations, whole graph only. Nodes
added from the connections menu land on a fixed 300 px half-circle with no overlap
check. There is no way to lay out part of the diagram, pick another algorithm, or
select more than one node.

## 2. User steps

### Page load

- **GL-1** The user opens a page with a diagram.
- **GL-2** The diagram is rendered with the page's default algorithm: Force for the
  resource graph, Left-to-right for the ontology diagram.
- **GL-3** A layout the user chose earlier is remembered per diagram type
  (`localStorage`) and wins over the default.
- **GL-4** A spinner shows while the layout computes; afterwards the diagram
  zooms to fit.

### Selection

- **GL-5** **Shift+drag** on empty canvas draws a selection box that replaces the
  selection; **Ctrl/Cmd+Shift+drag** adds to it. A plain drag still pans.
- **GL-6** **Ctrl/Cmd+click** toggles a node in or out of the selection (Cmd on
  macOS, where Ctrl+click is the context menu).
- **GL-7** Selected nodes are visibly marked; the toolbar shows a count
  ("12 selected").
- **GL-8** Click on empty canvas or **Esc** clears the selection; **Ctrl+A** selects
  all.
- **GL-9** Dragging any selected node moves the whole selection.

### Class tree (left sidebar)

- **GL-10** Right-click (or a "⋮" button) on a class opens: **Select all**,
  **Add all**, **Remove all**. A class includes its subclasses
  (`rdf:type/rdfs:subClassOf*`), matching the nesting shown in the tree.
  - Select all — select that class's nodes already on the diagram.
  - Add all — load every instance onto the diagram (GL-15 limit applies).
  - Remove all — remove that class's nodes from the diagram.

### Toolbar

- **GL-11** Layout menu: **Force**, **Hierarchy ↓**, **Left-to-right →**, **Radial**.
- **GL-12** Selection actions, enabled when ≥ 1 node is selected:
  **Expand all**, **Remove all**, **Layout selection**, **Select neighbours**,
  **Zoom to selection**.
- **GL-13** Layout selection uses the current algorithm, keeps every unselected
  node fixed, and keeps the selection's centroid where it was.
- **GL-14** Nodes added by an expand (halo menu or Expand all) are placed without
  moving existing nodes.
- **GL-15** Add all / Expand all show the number of nodes to be added and ask for
  confirmation when it exceeds **20**.

### Layout behaviour

- **GL-16** Choosing an algorithm computes new positions and redraws — the
  selection if there is one, otherwise the whole graph. Edges stay straight (link
  vertices cleared); no edge routing.
- **GL-17** Disconnected components are always packed side by side, never
  scattered.
- **GL-18** Every layout, add and remove is undoable with Ctrl+Z.
- **GL-19** **Radial** is a radial explosion from a centre node: the single
  selected node, else the page's own resource, else the highest-degree node.
  Nodes sit on rings by hop distance; each subtree gets an angular wedge sized by
  its node count, so tree edges do not cross. Non-tree edges are drawn but do not
  drive placement.

### Property panel (side panel, nothing selected)

- **GL-20** Lists every property (link type) on the diagram with its edge count.
- **GL-21** Per property: **reverse for layout**, **exclude from layout**,
  **hide/show**.
  - Reverse affects Hierarchy ↓ and Left-to-right → only; arrows keep their RDF
    direction. It has no effect on Force or Radial, which ignore edge direction.
  - Exclude removes the property's edges from the layout input of every
    algorithm, Radial included (centre choice and rings). Nothing is excluded by
    default.
- **GL-22** Reversed by default: `rdf:type`, `rdfs:subClassOf`, `skos:broader`,
  `schema:isPartOf` — so superclasses / broader concepts / wholes sit on top.
  User changes are remembered per property IRI (`localStorage`), shared by both
  diagrams.
- **GL-23** A dedicated **Redraw** button applies the changes; it is highlighted
  while there are unapplied changes.

### Across the board

- **GL-24** Everything works identically on the resource graph and the ontology
  diagram.
- **GL-25** All strings go through i18n (de, en, fr, it, rm).

## 3. Why `rdf:type` is not excluded by default

Excluding `rdf:type` is the usual remedy when one class node is linked to many
instances: in Force it becomes a hub that pulls everything together, and in Radial
the highest-degree fallback (GL-19) would pick the class as centre, putting every
instance in ring 1. It is left **included** because instances of a class are
frequently linked to each other only through their shared class node — excluding
the type edge would then fragment the graph into loose components, and the
class-grouping it gives is usually what the user wants to see. The user can exclude it per diagram via GL-21.

## 4. Implementation notes (from the brainstorming, non-binding)

- Everything above is reachable through GE 2.1.0's public API; no fork. Checked
  against the 2.1.0 `.d.ts` and `dist/graph-explorer.js`:
  - `GE.calculateLayout` / `GE.applyLayout` / `GE.forceLayout`, the latter with
    `fixedElements` and `selectedElements` (GL-13, GL-14).
  - `calculateLayout`'s `layoutFunction` is synchronous; ELK (`elkjs`) is async,
    so ELK modes build the ELK graph from `model.elements` / `model.links`, await
    `elk.layout()`, then `setPosition` inside `model.history.startBatch()`
    (GL-18).
  - Shift/Ctrl/Alt+drag does not pan (`shouldStartPanning`), so it is free for the
    selection box; modifier clicks are ignored by the editor but still reach
    `onPointerUp` (GL-5, GL-6). `workspace._getPaperArea().pageToPaperCoords()`
    converts coordinates. `editor.setSelection([...])` accepts many elements.
  - Multi-selection feedback: `view.setHighlighter()` (public) or
    `view._setElementDecorator()` (internal).
  - `editor.events.on('addElements', …)` fires after halo-menu placement (GL-14).
- Engines: Force = WebCola (built-in); Hierarchy / Left-to-right = ELK `layered`
  with `elk.direction` DOWN / RIGHT; Radial = ELK `radial`; component packing =
  ELK `separateConnectedComponents` or own shelf packing.
- `elkjs` is loaded lazily from jsDelivr on first use (the Mermaid ELK plugin's
  bundled copy is not reachable from GE). Main thread is fine below ~500 nodes.
- Keep the layout logic (ELK graph building, reversal, exclusion) in a module that
  does not depend on GE internals, so it survives a later move to Reactodia.
- GE's own right-hand **Connections** panel stays; it is the expand UI when a node
  is selected. The GL-20 panel is Visoto markup, not injected into GE's React DOM.

## 5. Alternative considered — Reactodia

`@reactodia/workspace` (0.35.2, LGPL-2.1+) already ships a rubber-band `Selection`
widget, `SelectionActionLayout`, an async `LayoutFunction` and a layout worker.
Costs: ESM-only with a React peer dependency (import map or a Vite build), and a
rewrite of `sparql-graph.js`, `schema-graph.js`, `graph-memory-store.js` and the
CSS overrides. Worth a time-boxed spike against LINDAS via `/api/sparql`; forking
GE is the last resort.
