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
