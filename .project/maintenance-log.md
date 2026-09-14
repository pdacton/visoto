# Maintenance Log

Ledger for the `maintenance` skill (`.claude/skills/maintenance/SKILL.md`).
Run `/maintenance cdn`, `/maintenance docs` or `/maintenance audit`.

Its job is to record **decisions**, especially deferrals, so a later sweep does
not re-propose something already considered and rejected. Append newest first.

## Standing deferrals

Do not re-flag these without new information. If you think one should change,
say what changed.

| Item | Decision | Reason |
|---|---|---|
| `xlsx@0.18.5` | Keep, do not bump or re-flag | Two CVEs, both in the spreadsheet *parser*. Nothing in Visoto parses a spreadsheet — the library only serves Tabulator's `download("xlsx")`, and the one file input accepts RDF only. Unreachable. Migration path documented above the tag in `base.html`. |
| SRI on the 3 JS-loader files | Not a maintenance task | `mermaid-init.js` cannot express `integrity` on a bare ES module import. `sparql-graph.js` / `schema-graph.js` build their script tag dynamically — adding it is a code change; propose via `audit`. |
| Major version bumps | Always deferred by default | The boot smoke test only proves the server starts. A Tabler/Tabulator major could break layout or tables silently. Needs a human with a browser. |
| `govulncheck` pinned to v1.7.0 in CI | Keep pinned; bump only with the toolchain | `setup-go` sets `GOTOOLCHAIN=local`, so the scanner must build under the Go in `go.mod`. v1.8.0 needs Go >= 1.26 and fails the step. v1.7.0 is the newest that builds against 1.25. **Local runs can hide this** — an unpinned toolchain silently downloads 1.26.x, so `@latest` passes locally and fails in CI. Verify with `GOTOOLCHAIN=go<go.mod version>`. |

## Outstanding — found 2026-09-14, not yet actioned

Surfaced while auditing CLAUDE.md; none of these have been applied.

- **`mcp-go v0.58.0 → v1.0.0`** — major. Needs a changelog read and a test of
  the MCP tool surface.
- **`github.com/golang/protobuf`** — deprecated, pulled in transitively.
  Check whether anything still requires it or if it drops with another bump.
- **Go toolchain** — `go.mod` says `1.25.14`. Bumping the directive would also
  let CI move `govulncheck` off the v1.7.0 pin (see standing deferrals); do both
  in the same change, and re-verify with `GOTOOLCHAIN=go<new version>`.
- **`klauspost/compress v1.17.6 → v1.20.0`** — several majors behind,
  transitive.
- **graph-explorer 2.1.0 duplicated** in `sparql-graph.js:51` and
  `schema-graph.js:495`. Not a bug today (identical), but they can drift.
  Worth a shared constant.

## History

### 2026-09-14 — cdn (`/maintenance cdn`, branch `maintenance/cdn-2026-09-14`)

Bumped, with SRI recomputed and verified against the bytes the CDN actually
serves:

| Library | Old → New | Files |
|---|---|---|
| `@tabler/core` | 1.4.0 → 1.5.1 | `base.html` ×3 (tabler.css, tabler-themes.min.css, tabler.js) |
| `lucide` | 1.37.0 → 1.46.0 | `base.html` ×1 |

Verified in a browser with the cache hard-cleared via CDP: all four assets load
past SRI validation (a stale hash fails closed and would have blocked them),
`window.lucide` is live with 79 icons rendered, Tabler CSS applies, and a
Tabulator table renders 10 rows / 11 columns with header and body sharing an
x-origin — the known misalignment symptom is absent. Graph Explorer still mounts.
Console clean on all three pages.

Already current, no action: tabulator-tables 6.5.2, wunderbaum 0.14.1,
highlight.js 11.12.0, htmx 2.0.10, svg-pan-zoom 3.6.2, chart.js 4.5.1,
chartjs-adapter-date-fns 3.0.0, graph-explorer 2.1.0.

**Tabler 1.5 upgrade guide reviewed** (https://docs.tabler.io/ui/getting-started/upgrade
— supplied by the user mid-run; 1.5 carries real breaking changes that the version
number alone does not advertise). Checked each against the codebase:

| Breaking change | Status here |
|---|---|
| Bootstrap now bundled; remove a separate `bootstrap.bundle.min.js` | Not affected — `base.html` never loaded one, so no double-initialisation |
| `window.bootstrap.X` → `window.tabler.X` | Already compliant everywhere except one pre-existing bug, below |
| `data-bs-*` → `data-tblr-*` | Optional; both prefixes work. Left alone |
| `.badges-list`/`.tags-list`/`.markdown` renamed | Not used |
| `dist/libs` direct file paths moved | Not used |
| Separate RTL stylesheet dropped | Not used |
| Sass variable/`@use` changes | Not applicable — CDN build, no Sass compilation |

The one finding, **pre-existing and not caused by this bump**:
`static/js/search.js:57` calls a bare `new bootstrap.Collapse(...)`. `window.bootstrap`
has never existed on this site (Tabler exposes `window.tabler.bootstrap`, as the rest
of the codebase already uses and as comments in `sparql-graph.js` / `schema-graph.js`
explicitly warn). Confirmed in-browser: `ReferenceError: bootstrap is not defined`.
Dates to 4d8af01 (2026-01-16), so 1.4 was equally broken. Reachable only on `/search`
loaded with a filter pre-selected, where the advanced-filter accordion then fails to
auto-expand. Logged as `.project/issues/search-js-uses-bare-bootstrap-global.md`
rather than fixed here — `cdn` mode does not carry code changes.

**Tabler 1.5 hid both sidebars — found by the user, fixed in this branch.**
The bump shipped a layout regression that every automated check passed over.
1.5 assumes a page has *either* a horizontal navbar *or* a vertical one, and
ships a mutually exclusive pair of rules keyed off `data-bs-navbar-position`
on `<html>`:

| State | Effect |
|---|---|
| attribute absent | `> .navbar-vertical { display: none }` — both sidebars vanish |
| attribute set to `vertical` | hides the horizontal navbar — the topbar vanishes |

Visoto has both, as siblings inside `.page`, which is the one combination
neither branch allows. Setting the attribute is NOT the fix — verified, it just
swaps which element disappears. The branch that applies to us is undone in
`tabler_overrides.css`, scoped to Tabler's exact selector so the mobile overlay
and `d-print-none` keep working. The same selector also zeroes
`--tblr-sidebar-width`, so both properties are restored.

Separately, 1.5 introduced `--tblr-sidebar-width: 16rem` (256px) where 1.4
declared no sidebar width at all — 240px was intrinsic from content. Left alone
this sized the sidebar 16px wider than every offset derived from
`--vs-sidebar-width`. Tabler's variable is now pinned to ours, keeping one name
for the width.

**Testing lesson — the important one.** Every check in this sweep passed while
the site was visibly broken. They asserted that things *existed* (`window.lucide`
is an object, 79 SVGs, a Tabulator table has rows) and never that a layout
element was *visible*. `display: none` on a fixed-position sidebar does not throw,
does not log, and does not change any of those signals; `.page-wrapper` kept its
240px margin, so the page even looked deliberately laid out. A screenshot from
the user is what surfaced it. **After a CSS-framework bump, assert computed
`display` and a non-zero bounding box on the major layout containers — sidebar,
topbar, content — and check the content gutter is zero.** A library that "loads"
proves nothing about layout.

**Deferred**

- **`mermaid 11.17.2 → 12.0.0` + `@mermaid-js/layout-elk 0.2.3 → 1.0.0`** —
  both major, and a coupled pair: layout-elk 1.0 targets mermaid 12, so they
  move together or not at all. Both are bare ES module imports in
  `mermaid-init.js` with no SRI, so a break shows up only as a diagram that
  silently fails to render. Needs a human loading a page with a mermaid
  diagram. Changelog: https://github.com/mermaid-js/mermaid/releases

**Process note** — the skill's file table misses a sixth location:
`static/css/tabulator_overrides.css:3` names the Tabulator stylesheet it
overrides inside a comment. That one is documentation, not a loaded pin, so it
needs no SRI — but it will read as wrong the moment Tabulator moves. The `grep`
in the skill is what caught this; keep trusting it over the table. (The table's
count of 13 pins in `base.html` is correct, all 13 carrying SRI.)

Also: the boot check bound 8060 on the first attempt because `PORT=8061` was
omitted. The user's own server happened to be down so nothing was displaced, and
the specific PID was killed rather than `pkill`. The startup log line is the
tell — always confirm `port=:8061` before testing.

### 2026-09-14 — docs (`/maintenance docs`, branch `maintenance/docs-2026-09-14`)

First run of the skill. Fixed: graph-explorer skill CDN 1.3.0 → 2.1.0 and its
dead `templates/pages/ontodia.html` paths; the reference doc's stale label-property
snippet; `/ontodia` in the sparqlGraph partial comment; `./data/` described as JSON
time-series when monitoring moved to SQLite (architecture.md + CLAUDE.md); six
`internal/` packages missing from the README and architecture package tables; two
missing README routes (`cube-table`, `lazy-tree`) plus a `?src=` note that split the
route table and over-claimed (cube-table is exempt); `search_provider` listing four
of six backends in the example and configuration.md; `allow_private_upload_urls`
absent from the example; `sparqlCube` undocumented in the authoring guide.

Checked and found correct, no change needed:
- `docs/templating.md` on `<sparql-facet>` — already describes it as a retired
  element kept only so a leftover fails at startup. The parser still recognises it
  for exactly that reason; it is not a live tag and the doc does not claim it is.
- `Bicycle.svg` / `custom-icon.svg` in the icon and graph-explorer skills —
  illustrative examples in commands, not paths that must exist.
- `/api/x` and `/p` — test-only routes in `*_test.go`, correctly absent from the
  README.

Process note: the pre-flight clean-tree check fired on this run because the skill
and CLAUDE.md were still uncommitted. Committing the setup first, then branching,
kept the setup and the sweep as two reviewable diffs. Worth repeating.


### 2026-09-14 — docs (manual, pre-skill)

Rewrote `CLAUDE.md` against the codebase. Corrected: Graph Explorer 1.3.0 →
2.1.0; removed the reference to the deleted `templates/pages/ontodia.html`;
"no environment variables" → `PORT` overrides the config port; default endpoint
is `cached.lindas.admin.ch`, not `ld.admin.ch`; templates *define blocks*
rather than extend a layout. Added the `<sparql-*>` tag language, the i18n
requirement, the no-inline-JS rule and a pointer to `docs/templating.md`.

`govulncheck` run: 0 vulnerabilities reachable from this code; 3 in required
modules that are not called.

Not checked this pass: `docs/*.md` beyond `templating.md`, `README.md`,
`visoto.config.example`, and the five `SKILL.md` files. All covered by the
`/maintenance docs` run above, which fixed the `graph-explorer` skill's stale
CDN version among the rest.
