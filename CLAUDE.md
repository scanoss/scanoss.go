# CLAUDE.md

Guidance for Claude Code (and other agents) working in this repository.

## Commit & workflow rules
- **Conventional Commits** — `feat:`, `fix:`, `refactor:`, `docs:`, `chore:`, `test:`.
- **Short subjects** — imperative, ≤ ~50 chars; no multi-line body unless explicitly asked.
- **No AI/assistant references** — never add `Co-Authored-By: Claude` or "Generated with…"
  trailers to commits.
- **Review before commit** — present each change and wait for explicit approval; never
  commit unreviewed work.
- **Never push automatically** — pushing to a remote is always a separate, explicit request.
- **Atomic commits** — one logical change per commit; keep the tree building and tests green.
- **CHANGELOG** — every product-changing commit updates `CHANGELOG.md` under
  `## [Unreleased]`; docs- or refactor-only commits with no user-facing change don't.

## Build, test, lint
- `make build` — build the CLI binary.
- `make test` / `make test-race` — run unit tests (race detector).
- `make lint` — run golangci-lint.
- `make check` — fmt-check + vet + lint + test; run before presenting or committing.
- `make generate` — regenerate the OpenAPI model types.

## Planning (SDD)
Non-trivial work is planned as an SDD under `specs/<feature>/{spec,plan,tasks}.md` before
implementation. Tasks are atomic and map 1:1 to commits (see the commit rules above).

## Agent skills

### Issue tracker
GitHub Issues on `scanoss/scanoss.go`, via the `gh` CLI. See `docs/agents/issue-tracker.md`.

### Triage labels
The five canonical roles, each label string equal to its name. See `docs/agents/triage-labels.md`.

### Domain docs
Single-context — `CONTEXT.md` + `docs/adr/` at the repo root. See `docs/agents/domain.md`.

### Relationship to Planning (SDD)
GitHub issues are the *unit of work* — what to build, in what order, with blocking edges. An SDD
under `specs/<feature>/` is the *design record* for work large enough to need one. A ticket may
reference an SDD; it does not replace the commit rules above.
