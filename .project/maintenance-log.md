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

## Outstanding — found 2026-09-14, not yet actioned

Surfaced while auditing CLAUDE.md; none of these have been applied.

- **`mcp-go v0.58.0 → v1.0.0`** — major. Needs a changelog read and a test of
  the MCP tool surface.
- **`github.com/golang/protobuf`** — deprecated, pulled in transitively.
  Check whether anything still requires it or if it drops with another bump.
- **Go toolchain** — `go.mod` says `1.25.14`; `govulncheck` pulled `1.26.8` to
  run. Consider bumping the directive.
- **`klauspost/compress v1.17.6 → v1.20.0`** — several majors behind,
  transitive.
- **graph-explorer 2.1.0 duplicated** in `sparql-graph.js:51` and
  `schema-graph.js:495`. Not a bug today (identical), but they can drift.
  Worth a shared constant.

## History

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
`visoto.config.example`, and the five `SKILL.md` files. The `graph-explorer`
skill is known to still claim CDN `1.3.0` — first `/maintenance docs` run
should fix it.
