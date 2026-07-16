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
- [x] **T6d — Dependency manifests via a dedicated filter (fix).** `--include deps` no longer
  reuses the fingerprint file set (from which the default filter strips manifests); it re-collects
  with `PreserveDependencyManifests` and parses only the manifests, so declared dependencies are
  actually sourced. (fix)
- [x] **T6e — Parallel scan + dependency resolution.** `Run` runs the scan (fingerprint → WFP)
  and the declared-dependency resolution concurrently, then enriches once both finish. `Build`
  split into `assemble` + `resolveDeclared` so the resolved components can be merged after the
  concurrent halves join. (perf)
- [x] **T6f — Rename component `files` → `evidence`.** The per-component matched-files array in
  the raw output is `evidence` (the field names inside each entry — `path`, `source_hash`,
  `match_type`, `oss_file_path`, … — are unchanged). (raw shape)
- [x] **T6g — Unify progress on mpb.** All commands (`scan`, `wfp`, `dependencies`, purl queries)
  render through one library (`vbauerster/mpb`) via shared `newProgress`/`addBar` helpers; `schollz`
  is dropped. `scan` renders all phases in one container (parallel phases side by side) and prints
  the `Scan id:` notice above the bars via `Progress.Write`. (UX)
- [x] **T6h — Deduplicate components (fix).** Components sharing an identity (PURL + version) are
  collapsed in `assemble` before enrichment — the same package from `package.json` + `package-lock.json`,
  or detected ∩ declared, would otherwise repeat in `raw` and clash on SBOM ids (SPDX `SPDXID`,
  CycloneDX `bom-ref`), making the documents invalid. Distinct versions are kept. (fix)
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
9. `fix(scanpipeline): source dependency manifests via a dedicated filter` — `pkg/scanpipeline/scanpipeline.go` (T6d).
10. `refactor(scanpipeline): run scan and dependency resolution in parallel` — `pkg/scanpipeline/scanpipeline.go` (T6e).
11. `refactor(sbom): rename component files to evidence` — `pkg/sbom/` (T6f).
12. `refactor(cmd): unify progress on mpb, drop schollz` — `cmd/` (`progress.go`, `scan.go`, `wfp.go`, `purlcommon.go`, `dependencies.go`), `go.mod`, `go.sum` (T6g).
13. `docs: document scan --include layers and the raw default` — `CLIENT_HELP.md`, `README.md` (partial T12; `convert`→`sbom` docs still with T7).
14. `fix(scanpipeline): deduplicate components to keep SBOM ids unique` — `pkg/scanpipeline/scanpipeline.go`, `CLIENT_HELP.md`, `CHANGELOG.md` (T6h).

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
