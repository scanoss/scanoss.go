# Tasks: scan output layers (SOURCE → ENRICH → RENDER)

**Spec:** `./spec.md` · **Plan:** `./plan.md` · **Issue:** [#4](https://github.com/scanoss/scanoss.go/issues/4)

Atomic, one commit each; tree builds and `make check` stays green after every step.

## Phase 1 — fused path (`scan --include`)
- [x] **T1 — Component model.** `Component.Scope` (`detected`|`declared`) + `DeclaredIn`; one
  `Components` list; per-component inline layers (`Licenses`/`Cryptography`/`Geoprovenance`);
  `FileEvidence` carries the full match detail (`source_hash`/`file_hash`/`match_percentage`/
  `oss_file_path`/line ranges); JSON tags on all types. (component model)
- [x] **T2 — Decouple gather from format + `pkg/scanpipeline`.** Extracted the SOURCE→ENRICH
  orchestration into `pkg/scanpipeline` (`Run`/`Build`, `Set`/`ParseLayers`); `cmd/scan.go` is
  thin and drives gathering from `--include` ∩ the format's capabilities. (FR-001, FR-002)
- [x] **T3 — Rename `plain` → `raw`.** Default format value `raw`; `validateOutputFormat` +
  flag help updated. `raw` = the `Inventory` in a versioned envelope. (FR-005)
- [x] **T4 — SOURCE: dependencies.** `--include deps` resolves manifest deps into
  `scope:"declared"` components (reusing the files `Run` already collected — no second walk);
  a `scan wfp` (no tree) sources none. (FR-003)
- [x] **T5 — ENRICH via decoration pipeline.** `--include vulns,licenses,crypto,geo` →
  `client.DecorationPipeline(...)` over the component PURLs → attached to the `Inventory`
  (copyright dropped as a layer). (FR-004)
- [x] **T6a — Effective-set gathering + skip messages.** `cmd` gathers `--include` ∩ format
  capabilities; a layer the format can't render is **skipped (not gathered)** with an up-front
  `Skipping <layer>: the <format> format cannot represent it` line (`spdx` drops vulns/crypto/geo;
  `cyclonedx` drops crypto/geo). The layer set is the pipeline's enable/disable knob; the
  pipeline stays format-agnostic. (FR-006)
- [x] **T6c — CLI progress & output.** Live per-layer enrichment bars via `vbauerster/mpb`
  (Unlicense, `cmd`-only) under an `Enriching components` header, styled to match the sequential
  `schollz` bars; a `Results written to <path>` line when `--output` is set. (UX)
- [ ] **T6b — Dependency graph (deferred).** Declared deps render as ordinary components; the
  CycloneDX `dependencies[]` graph and SPDX `DEPENDS_ON` edges are not built. (FR-006)
- [ ] **T7 — Rename `convert` → `sbom` (deferred).** `cmd/convert.go` unchanged this pass. (FR-007)

## Phase 2 — composable pipe (standalone `enrich`, CycloneDX interchange)
- [ ] **T8 — CycloneDX interchange fidelity.** Preserve `scope` + per-component layers across
  `ParseCycloneDX`/`Generate` (via component properties, like `url_hash`). Round-trip tests.
- [ ] **T9 — `enrich` command.** Read CycloneDX → decoration pipeline over its PURLs → write
  CycloneDX; reject `--include deps`. (FR-008)
- [ ] **T10 — Pipe smoke test.** `scan --format cyclonedx | enrich --include vulns | sbom
  --format spdx` end-to-end; stdin/stdout wiring.

## Cross-cutting
- [ ] **T11 — Tests.** `--include` matrix; decoupling (vulns gathered even for spdx, omitted at
  render + warned); deps-need-tree; `raw` default; rename.
- [ ] **T12 — Docs + changelog.** `CLIENT_HELP.md` (scan `--include`, `raw` default, `sbom`,
  Phase-2 `enrich`), `README.md` (commands: `convert`→`sbom`). **At the end of the
  implementation, refactor `CHANGELOG.md` to describe the final state** — add the new entries
  (layers / `--include`) **and update the existing entries affected by the renames**
  (`convert`→`sbom`, `plain`→`raw`), rather than merely appending.

## Commit sequence (as shipped)
The SDD update (`specs/scan-output-layers/*`) is amended into the existing
`chore(spec): add scan output layers SSD plan` commit. The code lands as:
1. `chore(deps): bump scanoss.api-sdk to v0.5.0` — `go.mod`, `go.sum`.
2. `refactor(scanoss): export Service.Name` — `pkg/scanoss/*` (address pipeline results by name).
3. `feat(sbom): component scope, inline layers, file match detail, JSON encoding` — `pkg/sbom/inventory.go` (T1).
4. `feat(scansource): source scope + file match detail; map crypto & geo` — `pkg/sbom/scansource/scansource.go`.
5. `feat(scanpipeline): scan pipeline — Run + Build (SOURCE→ENRICH)` — `pkg/scanpipeline/` (T2).
6. `feat(scan): --include layers, raw format, capability warnings, thin cmd` — `cmd/scan.go`,
   `cmd/scan_test.go`, `internal/config/config.go` (T2–T5, T6a) + `CHANGELOG.md`.
7. `chore(deps): add github.com/vbauerster/mpb` — `go.mod`, `go.sum`.
8. `feat(scan): live per-layer enrichment progress + output path message` — `cmd/scan.go` (T6c).

## Notes
- FR-001 (refined): the fused `scan` gathers `--include` ∩ format capabilities (skipping the
  rest, un-fetched); the pipeline stays format-agnostic; full decoupling is reserved for the
  composable pipe (Phase 2), whose target format is not fixed.
- ENRICH reuses `client.DecorationPipeline(...)` rather than adding per-service fetch logic.
- The `Inventory` carries JSON tags and `raw` wraps it in a versioned envelope; `sbom.Generate`
  is a first-class SDK entry point (a consumer builds an `Inventory` → CycloneDX/SPDX; only
  `Purl` is required per component, everything else is optional). `pkg/sbom` stays SDK-free.
- `plain`→`raw` is a breaking format-value rename; acceptable pre-release; noted in CHANGELOG.
- Deferred to follow-ups: dependency graph + capability table (T6), `convert`→`sbom` (T7),
  and all of Phase 2 (`enrich` command + pipe, T8–T10).
