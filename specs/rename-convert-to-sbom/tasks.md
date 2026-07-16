# Tasks: rename `convert` → `sbom`

**Spec:** `./spec.md` · **Plan:** `./plan.md` · **Issue:** [#11](https://github.com/scanoss/scanoss.go/issues/11)

Atomic, one commit each; tree builds and `make check` stays green after every step.

- [x] **T1 — Rename the command.** `git mv cmd/convert.go cmd/sbom.go` and
  `git mv cmd/convert_test.go cmd/sbom_test.go`. Rename `convertCmd → sbomCmd`,
  `runConvert → runSbom`, `Use: "sbom <input>"`, and update the Short/Long help + examples.
  Rename the test helper/functions (`runConvertTest → runSbomTest`, `TestConvert_* → TestSbom_*`).
  No alias. Grep confirms no `scanoss convert`/`convertCmd`/`runConvert` remain. (FR-001, FR-002,
  FR-004)

- [x] **T2 — Docs + changelog.** Update the command name in `README.md` (Commands table),
  `CLIENT_HELP.md` (TOC anchor + section heading + the `scanoss convert` examples), and
  `CHANGELOG.md` (rename the `convert` entry to `sbom`). Leave the prose verb "convert" where it
  describes the action. (FR-003)

## Commit sequence
1. `docs: add rename-convert-to-sbom SDD plan` — `specs/rename-convert-to-sbom/*` (after review).
2. T1 — `refactor(cmd): rename the convert command to sbom`.
3. T2 — `docs: rename convert to sbom in the docs and changelog`.

## Notes
- Pure rename — no behavior change, no new dependencies, `pkg/sbom` untouched.
- `convert` is unreleased (not in the `v0.1.0` tag), so the hard rename breaks nothing shipped.
