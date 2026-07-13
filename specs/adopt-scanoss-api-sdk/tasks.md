# Tasks: adopt the published scanoss.api Go SDK for model types

**Spec:** `./spec.md` · **Plan:** `./plan.md`

## Working rules (apply to every task)
- **Conventional Commits** (`feat:`, `fix:`, `refactor:`, `docs:`, `chore:`, `test:`).
- **Short** commit subjects (imperative, ≤ ~50 chars); no multi-line body unless asked.
- **No AI/assistant references** in commits (no `Co-Authored-By: Claude`, no "Generated
  with…" trailers).
- **Review each change individually before committing** — present the diff and wait.
- **No commit without review** — never commit until it's explicitly approved.
- **Never push to remote automatically** — pushing is a separate, explicit request.
- Each task is **atomic** and maps **1:1 to one commit** that builds green and passes
  `go test`. Every product-changing task updates `CHANGELOG.md` in the same commit;
  docs/refactor-/chore-only commits without user-facing change carry no entry.
- **Do not run `make generate` / `make sync-spec`** — those targets are being removed and the
  local spec is going away.

---

## Task 0 — Prerequisite: publish a fit `sdk/go` release (external, blocker)
- [x] **Content landed in `v0.4.2`.** The `scanoss.api` release `v0.4.2` (commit `7b8bdf2`)
  already contains: the friendly-name Option-A renames (`ScanEnvelope`, `ScanResult`,
  `ScanServer`, `KnowledgeBase`, `FileResult`, `MatchResult`, `ComponentResult`; `LineRange`
  unchanged) **and** the pointer `server`/`knowledge_base` fix — verified in that tag's
  `sdk/go/types.gen.go` (`Server *ScanServer`). All ~27 consumed types are exported.
- [ ] **Cut the path-prefixed submodule tag `sdk/go/v0.4.2`** (on the same commit as `v0.4.2`).
  The release created only a **bare/root** `v0.4.2` tag; Go cannot pin the `sdk/go` submodule
  from a root tag (verified: `go list -m …/sdk/go@v0.4.2` → "unknown revision sdk/go/v0.4.2").
  Command: `git tag sdk/go/v0.4.2 v0.4.2 && git push origin sdk/go/v0.4.2`.
- *Not a `scanoss` commit.* This is the only remaining external step; once the prefixed tag
  is pushed, Task 1 can pin `@v0.4.2`.

## Task 1 — Pin the external SDK dependency  (chore)
- [ ] `go get github.com/scanoss/scanoss.api/sdk/go@v0.4.2` (resolves the `sdk/go/v0.4.2` tag
  from Task 0). Blocked until that path-prefixed tag exists — a bare `v0.4.2` will not resolve.
- [ ] `go mod tidy` so the SDK is a **direct** `require` and `go.sum` is consistent.
- [ ] Do **not** add a committed `replace` directive.
- *Note:* the tree still won't build until Task 2 (expected — the working tree is already
  mid-migration and broken). Keep this commit to the go.mod/go.sum change only.
- *Commit:* `chore(deps): pin scanoss.api sdk/go module`

## Task 2 — Delete the scan-result aliases, use external types  (refactor)
- [ ] In `pkg/scanoss/scan_result.go`, **delete** the whole domain-alias block (`ScanEnvelope`,
  `ScanResult`, `ScanServer`, `KnowledgeBase`, `FileResult`, `MatchResult`, `LineRange`,
  `ComponentResult`). Keep `parseScanEnvelope` (retyped to return
  `scanossapi.ScanEnvelope`) and the `scanStateCompleted`/`scanStateFailed` constants;
  import `scanossapi` and drop the local `openapi` import.
- [ ] In the same commit, requalify the in-package scan-flow users so the package still
  compiles: `pkg/scanoss/scan.go` (`ScanEnvelope` → `scanossapi.ScanEnvelope` on
  `scan`/`Status`/`Wait` and locals) and `pkg/scanoss/bomremove.go` (`ApplyBOMRemove` param →
  `*scanossapi.ScanResult`; helper types → `scanossapi.FileResult`/`scanossapi.ComponentResult`).
- [ ] Verify `pkg/scanoss` compiles: `go build ./pkg/scanoss/...` (fixes the `scan.go:161`
  break).
- *Refactor of internal types; public `pkg/scanoss` signatures now use `scanossapi.*` — the
  user-facing note lands once in Task 7, not here.*
- *Commit:* `refactor(scan): use external sdk scan-result types`

## Task 3 — Rewire remaining importers to the external SDK  (refactor)
- [ ] In each of `pkg/scanoss/{copyright,cryptography,vulnerabilities,geoprovenance,components,
  licenses}.go`, `cmd/{copyright,cryptography,licenses,geoprovenance,components,
  vulnerabilities}.go`, and `pkg/sbom/scansource/{scansource,scansource_test}.go`:
  swap the import to `scanossapi "github.com/scanoss/scanoss.api/sdk/go"` and requalify every
  `openapi.X` → `scanossapi.X` (type names unchanged).
- [ ] Requalify the files that used the now-deleted aliases: `cmd/scan.go` (`ScanResult`),
  `pkg/postprocess/bomremove.go` (wrapper param → `*scanossapi.ScanResult`), and the tests
  `pkg/scanoss/{scan_test,scan_result_test,bomremove_test}.go`, `cmd/scan_test.go`,
  `pkg/sbom/scansource/scansource_test.go` (every `ScanEnvelope`/`ScanResult`/`FileResult`/
  `MatchResult`/`ComponentResult` literal → the `scanossapi.*` name).
- [ ] Confirm no `pkg/scanoss/openapi` importers remain:
  `grep -rn "scanoss/pkg/scanoss/openapi" --include='*.go' .` returns nothing.
- [ ] `go build ./...` and `go test ./...` are green.
- *Mechanical requalification; no behavior change — no CHANGELOG entry.*
- *Commit:* `refactor: requalify openapi types to external sdk`

## Task 4 — Remove the local codegen package  (chore)
- [ ] Delete `pkg/scanoss/openapi/` in full (`types.gen.go`, `doc.go`, `version.go`,
  `version_test.go`). Nothing references `SpecVersion`/`APIVersion` outside the package.
- [ ] `go build ./...` and `go test ./...` stay green.
- *Removes generated code the project no longer owns; no user-facing behavior change.*
- *Commit:* `chore: drop local generated openapi package`

## Task 5 — Remove the committed spec and codegen config  (chore)
- [ ] Delete `api/openapi.yaml` and `api/oapi-codegen.yaml` (remove the `api/` dir if empty).
- *Commit:* `chore: remove committed openapi spec and codegen config`

## Task 6 — Remove the codegen Makefile targets  (chore)
- [ ] Delete the `sync-spec`, `generate`, and `regenerate` targets and the vars
  `OAPI_CODEGEN_VERSION`, `SPEC_REPO`, `SPEC_PATH`, `SPEC_REF`. No target depends on them.
- [ ] Sanity-check `make check` still parses/runs (its recipe is unchanged).
- *Commit:* `chore(build): drop openapi codegen make targets`

## Task 7 — Record the SDK-source switch in CHANGELOG  (docs)
- [ ] Under `## [Unreleased] → ### Changed`, note that the OpenAPI model types now come from
  the external `github.com/scanoss/scanoss.api/sdk/go` module instead of local codegen, and
  — **Breaking** — that `pkg/scanoss` no longer re-exports the domain aliases (`ScanResult`,
  `ScanEnvelope`, `FileResult`, …): those **same type names** now live in the `scanossapi`
  package (the `scanoss.api` spec renamed the schema keys to the friendly vocabulary), so
  `pkg/scanoss`'s signatures reference them there (scan methods return
  `scanossapi.ScanEnvelope`, `ApplyBOMRemove` takes `*scanossapi.ScanResult`, service methods
  return `scanossapi.*` responses). The identifiers are unchanged; SDK consumers just update
  the import path/qualifier (`scanoss.X` → `scanossapi.X`).
- *This is the single user-facing entry for the change; the code commits above are internal
  refactors/chores. If preferred, fold this note into Task 3's commit instead of a separate
  docs commit — keep exactly one CHANGELOG entry for the switch.*
- *Commit:* `docs(changelog): note external openapi sdk adoption`

## Task 8 — Verify  (chore)
- [ ] `make check` (fmt-check + vet + lint + test) is green.
- [ ] `go build ./...` succeeds; `go mod tidy` leaves `go.mod`/`go.sum` unchanged.
- [ ] `grep -rn "pkg/scanoss/openapi" .` finds only these spec docs, nothing in code or
  Makefile.
- *No commit unless verification surfaces a fix.*
