# Tasks: apply bom.remove inside the scanner

**Spec:** `./spec.md` · **Plan:** `./plan.md` · **Issue:** [#43](https://github.com/scanoss/scanoss.go/issues/43)

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
  docs/refactor-only commits without user-facing change carry no entry.

---

## Task 1 — Move the bom.remove engine into the SDK  (refactor) — **DONE**
- [x] New `pkg/scanoss/bomremove.go`: `ApplyBOMRemove(result *ScanResult, bom *settings.BOM)`
  plus helpers `filePurls`, `shouldRemove`, `stripVersion`, `matchPath` (moved from
  `pkg/postprocess`, dropping the `scanoss.` qualifier — same package now).
- [x] `pkg/scanoss` imports `pkg/settings` (no cycle).
- [x] `pkg/postprocess.ApplyBomRemoveResult` becomes a one-line deprecated wrapper over
  `scanoss.ApplyBOMRemove`.
- [x] Move the `ApplyBomRemoveResult` tests → `pkg/scanoss/bomremove_test.go` (renamed to
  `ApplyBOMRemove`).
- [x] Also removed the dead raw-JSON variant here (Task 4) — its shared helpers had to move
  with the engine (see plan §2).
- *Done:* engine lives in `scanoss`; tree compiles; CLI still works via the wrapper. FR-005.
- *Commit:* `refactor(scan): move bom.remove engine into the SDK`

## Task 2 — Add `WithBOM` and apply it in the scan flow  (feature) — **DONE**
- [x] `scanOptions.bom *settings.BOM` + `WithBOM(*settings.BOM) ScanOption` (one option
  carrying the whole BOM).
- [x] In `scanService.scan()`, after `Wait`: if `o.bom != nil && env.Result != nil`, call
  `ApplyBOMRemove(env.Result, o.bom)` (remove + include-precedence). One site → covers
  `Folder`/`Files`/`WFP`. Doc comment notes a pre-scan hook for `include`/`identify` is a
  future extension on the same `o.bom` (out of scope).
- [x] SDK tests: `TestScanAppliesBOM` (filters + prunes) and `TestScanWithoutBOMUnchanged`
  (no option → untouched) (FR-001..003).
- [x] **Changelog** (`[Unreleased] › Added`): SDK `WithBOM` scan option.
- *Done:* FR-001, FR-002, FR-003.
- *Commit:* `feat(scan): apply bom rules in the scan flow via WithBOM`

## Task 3 — Rewire the CLI scan path  (refactor) — **DONE**
- [x] `cmd/scan.go`: build `[]scanoss.ScanOption`, append `WithBOM(&scanSettings.BOM)` when
  `scanSettings.HasBOM()`; pass to `Scan.WFP`.
- [x] Delete the explicit `postprocess.ApplyBomRemoveResult(...)` block and the now-unused
  `pkg/postprocess` import.
- [x] `cmd/scan_test.go` still passes (output unchanged, FR-004).
- *Done:* FR-004; CLI no longer post-processes by hand.
- *Commit:* `refactor(cli): apply bom via the scan option`

## Task 4 — Retire the dead raw-JSON variant  (chore) — **DONE (folded into Task 1)**
The raw variant + its helpers/types (`buildRemovedEntry`, `resultEntry`, `removedEntry`,
`ScanResults`) and tests were deleted in Task 1, because the shared helpers had to move
with the engine (plan §2). `pkg/postprocess` is now just the deprecated wrapper. No
separate commit.

## Task 5 — Verify — **DONE**
- [x] `go build ./... && go vet ./... && gofmt -l . && go test ./... -race` clean.
- [~] Manual: covered by the SDK end-to-end test (`TestScanAppliesBOM`); a live CLI run
  against a `scanoss.json` `bom.remove` is pending a server (or the local hardcode).

## Commit sequence (as executed)
1. `docs: add scan-apply-bom-remove SDD plan` — `specs/scan-apply-bom-remove/*`.
2. `refactor(scan): move bom.remove engine into the SDK` — Task 1 (incl. Task 4 deletion).
3. `feat(scan): apply bom rules in the scan flow via WithBOM` — Task 2 (+ changelog).
4. `refactor(cli): apply bom via the scan option` — Task 3.
5. `docs: mark scan-apply-bom-remove tasks done` — this update.

_No commit is made until you review the diff (per project rule)._
