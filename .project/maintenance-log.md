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
