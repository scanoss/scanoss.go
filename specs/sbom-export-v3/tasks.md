# Tasks: SBOM generation module over a neutral inventory

**Spec:** `./spec.md` · **Plan:** `./plan.md` · **Issue:** [#14](https://github.com/scanoss/scanoss.go/issues/14)

## T1 — Dependencies  (`go.mod`, `go.sum`)
- [ ] `go get github.com/CycloneDX/cyclonedx-go@latest github.com/spdx/tools-golang@latest`;
  `go mod tidy`.

## T2 — Pure module types + entry point  (`pkg/sbom`)
- [ ] `inventory.go`: `Inventory`, `Component` (incl. `Licenses []License`, `Files`),
  `License` + `LicenseAcknowledgement` (+ `AckDeclared`/`AckConcluded`), `FileEvidence`,
  `Vulnerability`, `Format` + `FormatCycloneDX`/`FormatSPDX`. Pure helpers
  (`supplier`, `purlNamespace`, `md5Hash`). **No `splitLicenses`.** No `pkg/scanoss` import.
- [ ] `options.go`: `options{projectName}`, `WithProjectName`, `newOptions(...)` default.
- [ ] `generate.go`: `Generate(inv, format, opts...)` → `buildCycloneDX`/`buildSPDXLite`
  (unexported); unknown format → error.
- *Done:* `pkg/sbom` builds depending only on `cyclonedx-go`.

## T3 — CycloneDX render  (`pkg/sbom/cyclonedx.go`)
- [ ] Remove hand-rolled `CycloneDX*` structs.
- [ ] `buildCycloneDX(inv, o)` → `cdx.BOM` encoded with `EncodeVersion(bom,
  cdx.SpecVersion1_7)` (pretty JSON); metadata named per option.
- [ ] `buildCycloneDXComponent`: library; name=PURL; version/NOASSERTION; publisher;
  `PackageURL`+`BOMRef`=purl[@version]; one license entry per `comp.Licenses`
  (LicenseRef→Name else ID; `acknowledgement` from `License.Acknowledgement`); website
  external ref; `Evidence.Occurrences` from `comp.Files` (location=path; snippet → `Line`
  + ranges in `AdditionalContext`).
- [ ] `buildVulnerabilities(inv)`: one `cdx.Vulnerability` per distinct id; `mapSeverity`;
  `Affects[].Ref` = bom-refs whose base PURL ∈ `Purls`; set `bom.Vulnerabilities`. Empty
  → none.
- *Done:* schema-valid 1.7; `acknowledgement` nested under `license` (#14); vulns +
  occurrences present when supplied.

## T4 — SPDX render via tools-golang  (`pkg/sbom/spdxlite.go`)
- [ ] Remove hand-rolled SPDX structs. `buildSPDXLite(inv, o)`: build a `v2_3.Document`
  (document + creationInfo + one `v2_3.Package` per component + `OtherLicenses` for any
  `LicenseRef-*`); per package `licenseDeclared` = AND-joined declared IDs (or NOASSERTION),
  `licenseConcluded` = AND-joined concluded IDs (or NOASSERTION); deterministic
  `documentNamespace`; serialize via `spdxjson.Write` with `Indent("  ")`. SPDX 2.3, Lite
  field subset. Ignores vulnerabilities/files.

## T5 — scansource adapters  (`pkg/sbom/scansource/`, new package)
- [ ] `FromScanResult(*scanoss.ScanResult) sbom.Inventory`: `v3Component` (no payload
  `licenses`); decode catalog payloads → `Component` (version: payload, else
  `Files[].PurlVersions` by `url_hash`, else ""); populate `Component.Files` by joining
  `result.Files` on `url_hash`, sorted by path. Licenses/vulns left empty.
- [ ] `LicensesFrom(*openapi.ComponentsLicenseResponse) map[string][]sbom.License`: per
  base PURL, each `LicenseInfo.Id` → `License{ID, AckDeclared}`, deduped.
- [ ] `VulnerabilitiesFrom(*openapi.VulnerabilitiesResponse) []sbom.Vulnerability`:
  dedupe by id/cve; accumulate `Purls`; normalize severity lower-case.
- [ ] Imports `pkg/sbom` + `pkg/scanoss` + `pkg/scanoss/openapi`. `compareVersions`
  private to `scansource`.

## T6 — CLI wiring + decoration fetch  (`cmd/scan.go`)  *(validation/vuln parts done)*
- [x] `scanPath` threaded into `uploadAndWrite`; `--format` validated up front
  (`validateOutputFormat`); `renderResult` dispatches plain/spdx/cyclonedx; vuln fetch for
  cyclonedx; `pkg/sbom`+`scansource` imported; old `converter.go` removed.
- [ ] `renderResult`: for `spdx`/`cyclonedx`, fetch the **licenses** decoration
  (`client.Licenses.Components(ctx, comps)`) and attach declared licenses by base PURL via
  `scansource.LicensesFrom` (warn+continue on error). Keep the cyclonedx vuln fetch.

## T7 — Tests
- [ ] `pkg/sbom` (pure, `Inventory` literals — no scan fixture): `generate_test.go`
  (dispatch + unknown error); `cyclonedx_test.go` (1.7; components; **licenses** declared
  + concluded incl. `GPL-2.0-only` ×2; LicenseRef→name; external refs; empty;
  vulnerabilities per distinct id with `affects[].ref`; file `evidence.occurrences` incl.
  snippet ranges); `spdxlite_test.go` (SPDX-2.3 doc, packages, supplier, `licenseDeclared`,
  `licenseConcluded`, externalRef, checksum, hasExtractedLicensingInfos); `options_test.go`;
  `helpers_test.go` (`supplier`/`md5Hash`).
- [ ] `pkg/sbom/scansource`: `*scanoss.ScanResult` fixture → `FromScanResult` (extraction,
  version sources, file-evidence join + sort; no licenses); `LicensesFrom`
  (`*openapi.ComponentsLicenseResponse` → declared per PURL, deduped); `VulnerabilitiesFrom`
  (dedupe, accumulated `Purls`, severity normalize).
- [ ] `cmd/scan_test.go`: mock licenses (+ vuln) decoration servers → `renderResult`
  includes declared licenses for spdx/cyclonedx.

## T8 — Verify
- [ ] `make check` (fmt-check + vet + lint + test) clean; plus `go test ./... -race`.
- [ ] Manual: real scan `-f cyclonedx` → `cyclonedx validate --input-version v1_7` OK
  with `vulnerabilities[]` + `evidence.occurrences`; `-f spdx` sane; `-f plain`
  byte-identical, no vuln call. SDK smoke: a `main` importing only `pkg/sbom` builds an
  `Inventory` and calls `Generate`.

## T9 — Changelog  (`CHANGELOG.md`)
- [ ] `[Unreleased]`: **Added** — pure `pkg/sbom` generation module (`Generate` over a
  neutral `Inventory`); `--format cyclonedx|spdx` supported for v3 results; CycloneDX
  includes vulnerabilities (decoration) and file/snippet evidence. **Fixed** — CycloneDX
  via official `cyclonedx-go`, schema-valid 1.7 (fixes #14). **Removed** — legacy
  old-results SBOM extraction.

## Commit sequence
1. `docs: add sbom-export-v3 SDD plan` — `specs/sbom-export-v3/*` (after review).
2. T1-T2 — `refactor(sbom): pure inventory-based generation module + single Generate`.
3. T3-T4 — `fix(sbom): CycloneDX 1.7 via cyclonedx-go, with vulns + file evidence`.
4. T5 — `feat(sbom): scansource adapters (scan result → inventory)`.
5. T6 — `feat(scan): support --format cyclonedx|spdx for v3 results`.
6. T7 — `test(sbom): cover generation module + scansource`.
7. T9 — `docs(changelog): note SBOM generation module`.