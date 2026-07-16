# Tasks: online inventory enrichment (`scanoss enrich`)

**Spec:** `./spec.md` · **Plan:** `./plan.md` · **Issue:** [#9](https://github.com/scanoss/scanoss.go/issues/9)

Atomic, one commit each; tree builds and `make check` stays green after every step.

- [x] **T1 — Export the enrichment step.** `pkg/scanpipeline`: rename `enrich` →
  `Enrich(ctx, *scanoss.Client, *sbom.Inventory, Set)` (body verbatim), route `assemble` through
  it, update the doc comment to name it the shared purl-keyed enrichment step. No behavior change;
  existing tests stay green. Add a direct `Enrich` test over a hand-built inventory. (FR-003)

- [x] **T2 — `enrich` command.** `cmd/enrich.go`: online `enrich <input> --include <layers>
  [--format <target>] [-o <file>]` with the scan auth flags + `checkAuth`. Local
  `identifyAndParse` (cyclonedx→`ParseCycloneDX`, spdx→`ParseSPDX`, else raw→`Unmarshal` into
  `sbom.Inventory`; a v3 scan result / garbage errors) → enrich via `scanpipeline.Enrich` →
  render via `emitInventory`. `--format` defaults to the input format; validated to
  `raw|spdx|cyclonedx`. Reuse `buildScanClient`/`scanProgress`. (FR-001, FR-002, FR-004, FR-007,
  FR-008, FR-009)

- [x] **T3 — deps rejection + format-capability warnings.** In `runEnrich`: **error** on
  `--include deps` (not a valid enrich layer — deps can't be derived from a components list);
  apply `reportSkippedLayers` + `effectiveLayers` for the output format — reusing the existing
  helpers, no duplication. (FR-005, FR-006)

- [x] **T4 — Command tests.** `cmd/enrich_test.go` (stub decoration client): `identifyAndParse`
  table (cyclonedx/spdx/raw; v3 result + garbage error); raw→raw default; spdx→spdx with `vulns`
  skipped (notice asserted); cyclonedx `--format spdx` (enrich+convert); `--include deps`
  errors; a v3 scan result / unrecognized input errors; missing key fails `checkAuth`.

- [x] **T5 — Docs + changelog.** Document the command:
  - `CLIENT_HELP.md` — new `Enrich (`enrich`)` section (usage, input formats + identification,
    purl-layers/`deps`-unsupported, default-format-follows-input, reused skip warning, online/auth
    note, weekly re-run) + a table-of-contents link.
  - `README.md` — `enrich <input>` row in the **Commands** table + a one-line example.
  - `CHANGELOG.md` — **Added** entry under `## [Unreleased]`; if the `convert` envelope fix ships
    here, a **Fixed** entry too.

## Commit sequence
1. `docs: add enrich-command SDD plan` — `specs/enrich-command/*` (after review).
2. T1 — `refactor(scanpipeline): export the purl-keyed Enrich step`.
3. T2 — `feat(cmd): add the online enrich command`.
4. T3 — `feat(enrich): reject deps and skip layers the format can't render`.
5. T4 — `test(enrich): input formats, skipped layers, auth`.
6. T5 — `docs: document the enrich command`.

## Notes
- **No new dependencies**; `pkg/sbom` stays free of the scan SDK (mapping stays in
  `pkg/scanpipeline`).
- **`convert` untouched** — enrich has its own three-class identify/parse; a verbatim v3 scan
  result is out of scope for now (spec Open decision 2).
- **Confirm Open decision 1** (component-extraction stays in `scanpipeline.Enrich`, not a new
  `sbom` method) before starting T1.
