# Tasks: extend SBOM fields

**Spec:** `./spec.md` · **Plan:** `./plan.md` · **Issue:** [#7](https://github.com/scanoss/scanoss.go/issues/7)

Atomic, one commit each; tree builds and `make check` stays green after every step.

- [ ] **T1 — Richer vulnerability model + CycloneDX rendering.** Add optional
  `CVSSScore/CVSSVector/CVSSMethod/CWEs/EPSSScore` to `sbom.Vulnerability`; render CVSS as a
  `ratings[]` entry (score/vector/method), `cwes`, and EPSS as a `scanoss:epss_score`
  property in `cyclonedx.go`. Severity-only path unchanged. Tests. (FR-001–FR-003)

- [ ] **T2 — Round-trip the new fields.** Extend `ParseCycloneDX` to read the CVSS
  rating / `cwes` / EPSS property back into the model; extend the round-trip test. (FR-004)

- [ ] **T3 — Configurable document metadata.** Add `WithTool`/`WithAuthor`/`WithTimestamp`
  (+ `resolvedTimestamp`) in `options.go`; wire `cyclonedx.go` metadata and `spdxlite.go`
  creationInfo to use them; defaults preserve current output. Tests. (FR-005, FR-006)

- [ ] **T4 — Docs + changelog.** `CHANGELOG.md` (Added, under `[Unreleased]`); a short SDK
  note in `README.md`/`CLIENT_HELP.md` if warranted.

## Commit sequence
1. `docs: add extend-sbom-fields SDD plan` — `specs/extend-sbom-fields/*` (after review).
2. T1 — `feat(sbom): richer optional vulnerability fields (CVSS/CWE/EPSS)`.
3. T2 — `feat(sbom): round-trip CVSS/CWE/EPSS in the CycloneDX reader`.
4. T3 — `feat(sbom): configurable document metadata (tool/author/timestamp)`.
5. T4 — `docs: note extend SBOM fields`.

## Notes
- Additive/backward-compatible; no new dependencies; `pkg/sbom` stays pure.
- Lives on `feat/convert-command` alongside the convert work.
- No `Inventory` serialization / json tags (decoupled; embedders persist their own model).
