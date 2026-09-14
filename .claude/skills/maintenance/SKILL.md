---
name: maintenance
description: Run a deliberate Visoto maintenance sweep - CDN library bumps with SRI, docs/CLAUDE.md drift, or a dependency security audit - each on its own branch with a PR. Use ONLY when explicitly asked to run maintenance or a named sweep, never as a side effect of ordinary edits.
---

# Visoto Maintenance

Periodic upkeep that no automated tool covers. Dependabot already handles Go
modules, GitHub Actions and Docker bases (see `.github/dependabot.yml`), and CI
runs `govulncheck` and `go mod tidy -diff` on every PR. **Do not duplicate
those here.** This skill exists for the work that needs judgement.

Three modes. Run ONE per invocation — never mix them in a branch or PR, because
they have completely different review costs:

| Mode | What it does | Commits? |
|---|---|---|
| `cdn` | Bump CDN-pinned frontend libraries, recompute SRI | Yes |
| `docs` | Fix CLAUDE.md / docs / config-example drift | Yes |
| `audit` | Security + major-version + deprecation review | **No** |

With no mode given: read `.project/maintenance-log.md`, report when each mode
last ran and what is outstanding, then stop. Do not pick a mode yourself.

## Before any mode

1. Confirm the tree is clean (`git status`). If not, stop and say so.
2. Read `.project/maintenance-log.md` — it records deferred decisions with
   reasons. **Never re-propose something the log has already deferred** unless
   the reason no longer holds; say why it no longer holds.
3. Branch: `maintenance/<mode>-YYYY-MM-DD`. Never work on `main`.

## Verification gate

Every mode that commits MUST pass all of these before opening a PR:

```
gofmt -l .          # must print nothing
go vet ./...
go test ./...
go build -o /tmp/visoto-maint ./cmd/visoto/ && PORT=8061 /tmp/visoto-maint
curl -sf http://localhost:8061/ping    # must return pong, then kill the PID
```

The boot check is not optional for template changes: templates parse at
**runtime**, so `go build` cannot catch a broken one. Only booting can. Kill
the specific PID from `ss` — never `pkill -f cmd/visoto`, which would orphan
the user's own server on 8060.

If the gate fails, revert the offending change, keep the rest, and report the
failure in the PR body. Never open a red PR.

---

## Mode: cdn

Frontend libraries are CDN `<script>`/`<link>` tags with no `package.json`, so
Dependabot is structurally blind to them. This is the highest-value mode.

### The pins live in five files

| File | Pins | SRI |
|---|---|---|
| `templates/layout/base.html` | 13 | Yes |
| `templates/pages/monitoring.html` | 2 (chart.js, adapter) | Yes |
| `static/js/mermaid-init.js` | 2 (mermaid, layout-elk) | No — impossible |
| `static/js/sparql-graph.js` | 1 (graph-explorer) | No |
| `static/js/schema-graph.js` | 1 (graph-explorer) | No |

Find them all — do not trust this table to stay current:

```
grep -rno "https://\(cdn\.jsdelivr\.net/npm\|unpkg\.com\)/[^\"' ]*" templates/ static/js/
```

### Rules

- **Bump patch and minor only.** A major version goes in the PR body under
  "Deferred" plus the log — never applied. The boot smoke test only proves the
  server starts; it would not catch a Tabulator or Tabler major silently
  breaking table rendering or layout.
- **graph-explorer is pinned in two files** (`sparql-graph.js`,
  `schema-graph.js`). They must stay identical. Bump both or neither.
- **Recompute SRI on every bump**, for the two files that carry it:
  ```
  curl -sL <url> | openssl dgst -sha384 -binary | openssl base64 -A
  ```
  A stale hash makes the asset fail closed — the page loads with the library
  silently missing. Verify the new hash is actually different from the old.
- **Do not add SRI to the three JS files.** `mermaid-init.js` explains why it
  cannot: `integrity` is not expressible on a bare ES module import. The
  graph-explorer loaders build their `<script>` tag dynamically; adding SRI
  there is a real change, not maintenance — propose it in `audit`, don't do it
  here.
- **xlsx@0.18.5 is a deliberate exception.** The comment above it in
  `base.html` documents two CVEs that are unreachable in this codebase and the
  migration path off the CDN. Leave the version alone and do not re-flag it;
  the log records this.

### After bumping

Beyond the standard gate, load the pages that actually exercise the changed
library (`/monitoring.html` for chart.js, a page with a graph for
graph-explorer, a table page for Tabulator) and check the browser console for
errors. Playwright MCP silently serves stale `static/js` and `static/css` —
hard-clear the cache via CDP before trusting what you see.

PR body: a table of `library | old → new | why`, then a "Deferred" section for
majors with the changelog link and what would need checking.

---

## Mode: docs

Prose only. No code changes — if you find a bug, log it in `.project/issues/`
instead.

Verify against the actual code, not against what the doc claims:

- `CLAUDE.md` — every factual claim (versions, paths, ports, file names,
  defaults). This drifts fastest.
- `docs/templating.md` — the authoritative template authoring guide; check the
  custom `<sparql-*>` tags and partial parameters still match `internal/parser`
  and `templates/partials/`.
- `docs/architecture.md`, `docs/configuration.md`, `docs/deployment.md`,
  `docs/getting-started.md`
- `visoto.config.example` vs `visoto.config` — the example is committed and the
  real one is gitignored, so a new config key is easy to forget. Check every
  section and key exists in the example, with no secrets.
- `README.md` — routes, project structure, prerequisites.
- `.claude/skills/*/SKILL.md` — these go stale too. `graph-explorer` has
  claimed a wrong CDN version before.

CI copies `visoto.config.example` to `visoto.config` for its smoke test, so a
broken example breaks CI — run the gate even though this mode is prose.

Keep CLAUDE.md short. It is an instruction file, not documentation: state
conventions and point at `docs/` for detail. Resist re-adding generic advice
("use idiomatic Go", framework links) — it was deliberately removed.

---

## Mode: audit

**Report only. Never commit a code change in this mode.** Output goes to
`.project/issues/<slug>.md` (one file per finding, matching the existing
convention there) and a summary in the log. No PR unless the user asks.

Cover:

- **Go modules**: `go list -m -u all` for majors and known-deprecated modules
  Dependabot will not auto-open. `github.com/golang/protobuf` is already
  flagged deprecated.
- **Go toolchain**: the `go` directive in `go.mod`. Dependabot's `gomod`
  ecosystem does not bump it.
- **`govulncheck` in depth**: CI runs it as a gate; here, run it with
  `-show verbose` and reason about the unreachable findings — whether a
  "module requires but code doesn't call" result is still true after recent
  changes.
- **Code review** of anything touching untrusted input: `internal/upload`
  (SSRF guard), `internal/sparql` query construction and the `??` → `<iri>`
  injection surface, `internal/chat`, and the MCP server's exposed tools.
- **Cache correctness as a security property**: cacheable routes (`/resource`,
  `/api/*`) must remain pure functions of the URL. Reading the endpoint cookie
  in one of them is a cross-user cache-poisoning bug, and has been before.

For each finding give severity, the concrete reachable path (not just the CVE
title), and what fixing it would cost. A finding with no reachable path is
worth recording as explicitly-not-a-problem so the next audit skips it.

---

## Finishing

Every run appends to `.project/maintenance-log.md`: date, mode, what changed,
and every deferred decision with its reason. The log is what stops the next
sweep from re-litigating the same `mcp-go v1.0.0` bump.

If any code changed, run `graphify update .` so the knowledge graph stays
current.

Deploy note for whoever merges a `cdn` or `docs` PR: `templates/` and
`static/` load from disk at runtime, and prod sits behind a Souin/Caddy cache.
A deploy needs a cache purge or it serves the old assets.
