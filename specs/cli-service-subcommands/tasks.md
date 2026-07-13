# Tasks: per-service CLI subcommands

**Spec:** [`spec.md`](./spec.md) · **Plan:** [`plan.md`](./plan.md)

`[P]` = can run in parallel (different files, no ordering dependency).
Paths are relative to the repo root.

## Conventions
- **Conventional Commits** (`feat:`, `refactor:`, `test:`, `docs:`).
- **Atomic commits** — one logical change per commit; **review before each commit**.
- **No AI/assistant references** in commit messages.
- **Short** imperative commit subjects (≤ ~50 chars).
- Every code change ships with **unit tests**.

## Phase 1 — Shared plumbing (refactor commit)
- [ ] **T001** `cmd/purlcommon.go`: add `newPurlServiceCmd(use, short, long, call)`;
      register the shared flags as **persistent** on a parent (update
      `addPurlServiceFlags`); factor `newClient(cmd)` and `writeResult(cmd, res)` out of
      `runPurlService` for reuse by the bespoke `components` runners. No behaviour change.

## Phase 2 — Restructure existing service commands (feat commit)
- [ ] **T002** `cmd/vulnerabilities.go`: parent (default op `components`) + `components`
      + `cpes`; `Args: cobra.NoArgs` on the parent.
- [ ] **T003 [P]** `cmd/cryptography.go`: parent (default op `algorithms`) +
      `algorithms`, `algorithms-range`, `versions-range`, `hints`, `hints-range`; remove
      `--algorithms/--libraries/--range` and `runCryptography`.
- [ ] **T004 [P]** `cmd/geoprovenance.go`: parent (default op `origin`) + `origin`,
      `countries`; remove `--origin/--countries`.
- [ ] **T005 [P]** `cmd/licenses.go`: parent (default op `attribution`) + `attribution`
      + `evidence`.

## Phase 3 — New service commands (feat commits)
- [ ] **T006** `cmd/copyright.go` (new): parent (default op `evidence`) + `evidence`,
      `holders`; register on root.
- [ ] **T007** `cmd/components.go` (new): parent (default op `search`) + `search`
      (bespoke: `--search/--vendor/--component/--purl-type/--limit/--offset`, flags on
      the parent), `versions` (bespoke: `--purl/--limit`), `status` (PURL-list); register
      on root.

## Phase 4 — Tests
- [ ] **T008** `cmd/*_test.go`: per parent, assert registered subcommand names and that
      inherited shared flags are present on a subcommand.
- [ ] **T009** Routing tests with a stub server: execute `rootCmd` with subcommand args
      and assert the v3 path hit per subcommand (incl. `components search`/`versions`
      query params).

## Phase 5 — Docs
- [ ] **T010** `CHANGELOG.md` (`[Unreleased]`): note the subcommand restructure
      (removed mode flags; new `copyright`/`components` commands; new `cpes` /
      `versions-range` coverage).

## Final verification
- [ ] `go build ./...`; `go vet ./cmd/`; `gofmt -l cmd/`.
- [ ] `go test ./...`.
- [ ] Manual: `scanoss cryptography --help` lists 5 ops; `scanoss components
      search --vendor angular` hits `/v3/components/search`; flagless
      `scanoss vulnerabilities --purl … ` still works (default op).

## Out of scope (future)
- Single-component (`GET`) lookups from the CLI.
- Typed CLI output.
- Ruleset download, dependencies, `*-contents` endpoints.
