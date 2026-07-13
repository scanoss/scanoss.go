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
