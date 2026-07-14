# Tasks: offline format conversion (`scanoss convert`)

**Spec:** `./spec.md` · **Plan:** `./plan.md` · **Issue:** _to be created_

Atomic, one commit each; tree builds and `make check` stays green after every step.

- [ ] **T1 — CycloneDX reader.** `pkg/sbom/parse_cyclonedx.go`:
  `ParseCycloneDX([]byte) (Inventory, error)` via `cdx.NewBOMDecoder`, mapping components +
  vulnerabilities into `Inventory` (inverse of `buildCycloneDX`). Unit test on a literal
  CycloneDX document. (FR-003)

- [ ] **T2 — SPDX reader.** `pkg/sbom/parse_spdx.go`:
  `ParseSPDX([]byte) (Inventory, error)` via `spdxjson.Read`, mapping packages into
  `Inventory` (purl from externalRef, split `AND`-joined licenses, checksum→URLHash;
  inverse of `buildSPDXLite`). Unit test on a literal SPDX document. (FR-004)

- [ ] **T3 — Round-trip tests.** In `pkg/sbom`: `Generate → Parse → Inventory` equality for
  both formats, over a representative `Inventory` (components, licenses, vulnerabilities).

- [ ] **T4 — `convert` command.** `cmd/convert.go`: offline `convert <input> --format
  <cyclonedx|spdx> [-o <file>]`; `identifyFormat` content check (cyclonedx/spdx/raw, else
  error); dispatch raw→`scansource.FromScanResult`, cyclonedx→`ParseCycloneDX`,
  spdx→`ParseSPDX`; `sbom.Generate` → `pkg/output`. Target validated up front. (FR-001,
  FR-002, FR-005, FR-007)

- [ ] **T5 — Lossy-conversion warnings.** Warn once per layer the target can't represent
  (e.g. `cyclonedx → spdx` drops vulnerabilities). (FR-006)

- [ ] **T6 — Command tests.** `cmd/convert_test.go`: `identifyFormat` table; end-to-end each
  direction (spdx↔cyclonedx, raw→each); vulnerability-dropped warning; invalid `--format`.

- [ ] **T7 — Docs + changelog.** Document the new command:
  - `CLIENT_HELP.md` — new `Convert (`convert`)` section (usage, the three input formats,
    content-based identification, offline note, lossy-conversion warning) + a table-of-
    contents link.
  - `README.md` — `convert` row in the **Commands** table + a short usage example.
  - `CHANGELOG.md` — **Added** entry for the `convert` command (user-facing change).

## Commit sequence
1. `docs: add convert-command SDD plan` — `specs/convert-command/*` (after review).
2. T1 — `feat(sbom): read CycloneDX into an Inventory`.
3. T2 — `feat(sbom): read SPDX into an Inventory`.
4. T3 — `test(sbom): round-trip Generate↔Parse for both formats`.
5. T4 — `feat(cmd): offline convert command (raw/cyclonedx/spdx → cyclonedx/spdx)`.
6. T5 — `feat(convert): warn on layers the target format can't represent`.
7. T6 — `test(convert): format identification + conversion directions`.
8. T7 — `docs: document the convert command`.

## Notes
- **No new dependencies** — reuses `cyclonedx-go` + `spdx/tools-golang` (already present).
- **Fidelity** is limited to the current `Inventory` model for v1 (spec Open decision 1); a
  "balanced" field set is a separate follow-up.
- **Independent of issue #4** — deps are carried only once `Inventory.Dependencies` exists.
