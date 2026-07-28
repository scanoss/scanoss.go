# Implementation Plan: SBOM generation module over a neutral inventory

**Spec:** `./spec.md`
**Status:** Draft

## Working agreements
- **Clean code** — match surrounding style; small, readable functions; no dead code or
  leftover scaffolding.
- **Unit tests** — every new package/function ships with table-driven unit tests (T7);
  no step is "done" without them.
- **Conventional commits**, **short** imperative subjects (≤ ~50 chars), **no AI
  references/trailers** (no `Co-Authored-By`, no "Generated with…").
- **Atomic commits** — one logical change each (see Commit sequence).
- **Lint locally at every stop** — `make check` (fmt-check + vet + lint + test) must pass
  before a step is presented; `make lint` runs golangci-lint per `.golangci.yml`.
- **Review gate** — present the diff and wait for explicit approval before **every**
  commit.

## Approach
Split the work along a purity boundary:

- **`pkg/sbom` (pure)** — neutral types + a single `Generate(inv, format, opts...)`
  entry point that renders CycloneDX (official `cyclonedx-go`, spec 1.7) or SPDX
  (official `spdx/tools-golang`, SPDX 2.3 Lite subset). Depends only on those two SBOM
  libraries; imports nothing from `pkg/scanoss`.
- **`pkg/sbom/scansource` (adapter, new)** — the only SDK-coupled code: builds an
  `sbom.Inventory` from a `*scanoss.ScanResult` and maps a decoration
  `*openapi.VulnerabilitiesResponse` into `[]sbom.Vulnerability`.
- **`cmd/scan.go`** — assembles the inventory via `scansource` (+ a vulnerability fetch
  for cyclonedx) and calls `sbom.Generate`.

a downstream consumer imports only `pkg/sbom` and builds its own `Inventory`. No import cycle:
`pkg/scanoss` ← `pkg/sbom/scansource` → `pkg/sbom`; `pkg/sbom` depends on neither.

## Public API
```go
// pkg/sbom (pure)
type Inventory struct { Components []Component; Vulnerabilities []Vulnerability }
type Component struct { Purl, Vendor, Name, Version, URL, URLHash, DownloadLocation string; Licenses []License; Files []FileEvidence }
type LicenseAcknowledgement string // AckDeclared | AckConcluded
type License struct { ID string; Acknowledgement LicenseAcknowledgement }
type LineRange struct { StartLine, EndLine int }
type FileEvidence struct { Path, MatchType string; InputLineRanges, OssLineRanges []LineRange }
type Vulnerability struct { ID, Severity, Source, URL, Summary string; Purls []string }
type Format string
const ( FormatCycloneDX Format = "cyclonedx"; FormatSPDX Format = "spdx" )
type Option func(*options)
func WithProjectName(name string) Option
func Generate(inv Inventory, format Format, opts ...Option) (string, error)

// pkg/sbom/scansource (SDK glue)
func FromScanResult(result *scanoss.ScanResult) sbom.Inventory
func LicensesFrom(resp *openapi.ComponentsLicenseResponse) map[string][]sbom.License
func VulnerabilitiesFrom(resp *openapi.VulnerabilitiesResponse) []sbom.Vulnerability
```

## Mechanics

### 1. Dependencies
`go get github.com/CycloneDX/cyclonedx-go@latest github.com/spdx/tools-golang@latest` +
`go mod tidy`.

### 2. `pkg/sbom` — pure module
- **`inventory.go`** — `Inventory`, `Component` (with `Licenses []License`), `License` +
  `LicenseAcknowledgement` (+ `AckDeclared`/`AckConcluded`), `FileEvidence`,
  `Vulnerability`, `Format` + constants. Pure helpers: `supplier`/`purlNamespace`,
  `md5Hash`. **No `splitLicenses`** (licenses are discrete IDs). No `pkg/scanoss` import.
- **`options.go`** — `type options struct { projectName string }`, `WithProjectName`,
  `newOptions(...)` applying the `"scanoss-project"` default.
- **`generate.go`** — `Generate(inv, format, opts...)`: switch on `format` → unexported
  `buildCycloneDX(inv, o)` / `buildSPDXLite(inv, o)`; unknown → error.
- **`cyclonedx.go`** — remove hand-rolled `CycloneDX*` structs. `buildCycloneDX`:
  - `cdx.NewBOM()`; metadata (timestamp, SCANOSS author, `application` component named
    `o.projectName`).
  - per `inv.Components[]` → `buildCycloneDXComponent` (type library; name=PURL;
    version/NOASSERTION; publisher=`supplier`; `PackageURL`+`BOMRef`=purl[@version];
    one license entry per `comp.Licenses` — `LicenseRef-*`→Name else ID, acknowledgement
    mapped from `License.Acknowledgement` (declared default); website external ref;
    `Evidence.Occurrences` from `comp.Files`).
  - `buildVulnerabilities(inv)`: base-`purl → []bom-ref` map from components; per
    `inv.Vulnerabilities` emit one `cdx.Vulnerability{ ID, Source{Name}, Ratings:
    [{Severity: mapSeverity(...)}], Description: Summary, Advisories: [{URL}], Affects }`
    (Affects = bom-refs whose base purl ∈ `Purls`). Attach `bom.Vulnerabilities`.
  - `mapSeverity(string) cdx.Severity` (case-insensitive; default `SeverityUnknown`).
  - occurrences: `cdx.EvidenceOccurrence{ Location: path }`; snippet → `Line` = first
    input-range start, `AdditionalContext` = ranges text.
  - encode: `NewBOMEncoder(JSON).SetPretty(true).EncodeVersion(bom, cdx.SpecVersion1_7)`
    into a `bytes.Buffer`.
- **`spdxlite.go`** — `buildSPDXLite(inv, o)`: build a `v2_3.Document` (tools-golang):
  document fields (`SPDX-2.3`, `CC0-1.0`, `SPDXRef-DOCUMENT`, name per `o.projectName`,
  deterministic `documentNamespace` = sha256 of name+purls), `CreationInfo` creators
  (`Tool: scanoss-<config.AppVersion>`, `Organization: SCANOSS`), one `v2_3.Package`
  per component (name=PURL, `SPDXRef-<md5(purl@version)>`, versionInfo, supplier,
  downloadLocation, homepage, `licenseDeclared` = AND-joined declared IDs (or NOASSERTION),
  `licenseConcluded` = AND-joined concluded IDs (or NOASSERTION), NOASSERTION copyright,
  `purl` external ref, MD5 checksum from `url_hash` when present, `filesAnalyzed:false`),
  and `OtherLicenses` for any `LicenseRef-*` (declared or concluded). Serialize via
  `spdxjson.Write(doc, &buf, spdxjson.Indent("  "))`. Ignores vulns/files.

### 3. `pkg/sbom/scansource` (new package)
- `FromScanResult(*scanoss.ScanResult) sbom.Inventory`: `v3Component` payload struct (no
  longer reads payload `licenses`); decode catalog payloads → `Component` (version:
  payload, else from `Files[].PurlVersions` by `url_hash`, else ""); populate
  `Component.Files` by joining `result.Files` on `url_hash`, sorted by path. Licenses and
  vulnerabilities left empty.
- `LicensesFrom(*openapi.ComponentsLicenseResponse) map[string][]sbom.License`: per base
  PURL, each `LicenseInfo.Id` → `License{ID, AckDeclared}`, deduped.
- `VulnerabilitiesFrom(*openapi.VulnerabilitiesResponse) []sbom.Vulnerability`: dedupe by
  id/cve, accumulate `Purls`, normalize severity to lower-case.
- Imports `pkg/sbom`, `pkg/scanoss`, `pkg/scanoss/openapi`. `compareVersions`/`versionParts`
  are private to `scansource`.

### 4. `cmd/scan.go` (`renderResult`)
- `--format` is validated up front in `runScan`/`runScanWFP` (`validateOutputFormat`).
- After bom.remove, `renderResult` dispatches:
  - `plain` → `json.MarshalIndent(res.Result, …)` (unchanged; no decoration).
  - `spdx`/`cyclonedx` → `inv := scansource.FromScanResult(res.Result)`; derive component
    PURLs and fetch the **licenses** decoration (`client.Licenses.Components(ctx, comps)`),
    attach declared licenses by base PURL via `scansource.LicensesFrom`; for `cyclonedx`
    **also** fetch `client.Vulnerabilities.Components(ctx, comps)` →
    `scansource.VulnerabilitiesFrom`. Each fetch non-fatal (warn + continue). Then
    `sbom.Generate(inv, sbom.Format(format), sbom.WithProjectName(...))`.
  - A shared `fetchLicenses`/`fetchVulnerabilities` helper keeps it tidy.
- Imports `pkg/sbom` + `pkg/sbom/scansource`; `encoding/json` stays (plain path).

## File-by-file
**New:** `pkg/sbom/inventory.go`, `pkg/sbom/options.go`, `pkg/sbom/generate.go`,
`pkg/sbom/scansource/scansource.go`.
**Modified:** `pkg/sbom/cyclonedx.go`, `pkg/sbom/spdxlite.go`, `pkg/sbom/component.go`
(trim to retained pure helpers, or fold into `inventory.go` and delete), `cmd/scan.go`,
`go.mod`/`go.sum`. **Removed:** `pkg/sbom/converter.go` (replaced by `generate.go`;
CLI does the `plain`/dispatch).

## Tests
- **`pkg/sbom` (pure, no SDK):** build an `Inventory` literal in-test (no scan fixture).
  - `generate_test.go`: dispatch + unknown-format error.
  - `cyclonedx_test.go`: decode via `cdx.NewBOMDecoder`; specVersion `1.7`; components;
    **licenses** — declared + concluded entries with correct `acknowledgement` (incl. the
    `GPL-2.0-only` ×2 case), LicenseRef→name; external refs; empty inventory.
    **Vulnerabilities:** `Inventory` with vulns → one `vulnerabilities[]` entry per
    distinct id, severity, `affects[].ref` == affected bom-refs; none when empty. **File
    evidence:** component with file+snippet `Files` → `evidence.occurrences[]` with
    locations + snippet ranges.
  - `spdxlite_test.go`: `Inventory` → structural asserts (versionInfo, supplier,
    `licenseDeclared`, `licenseConcluded`, externalRefs, checksum).
  - `options_test.go`: project-name default + override. `helpers_test.go`:
    `supplier`/`md5Hash` (no more `splitLicenses`).
- **`pkg/sbom/scansource`:** a `*scanoss.ScanResult` fixture → `FromScanResult`
  (extraction, version sources, file-evidence join, sort; no licenses); `LicensesFrom`
  (a `*openapi.ComponentsLicenseResponse` → declared licenses per PURL, deduped); and
  `VulnerabilitiesFrom` (dedupe by id, accumulated `Purls`, severity normalize).
- **`cmd/scan_test.go`:** mock the licenses decoration server (and vuln server) →
  `renderResult` for `spdx`/`cyclonedx` includes the declared licenses.

## Verification
- `make check` (fmt-check + vet + lint + test) clean at each stop; plus `go test ./...
  -race` for the race detector.
- Manual: real scan `-f cyclonedx` → `cyclonedx validate --input-version v1_7` OK with
  `vulnerabilities[]` and `evidence.occurrences`; `-f spdx` sane; `-f plain`
  byte-identical and makes no vuln call. SDK smoke: a tiny `main` that builds an
  `Inventory` and calls `sbom.Generate` importing only `pkg/sbom`.

## Risks / notes
- **Payload/vuln/range shapes** (`v3Component`, `ComponentVulnerabilityInfo`,
  `InputLineRanges`/`OssLineRanges`) inferred from `openapi` + `internal/models`; confirm against a
  real v3 result/decoration during manual verification.
- **`compareVersions` location**: needed by `scansource` for version resolution; either
  export a minimal helper from `pkg/sbom` or duplicate privately in `scansource` (keep
  `pkg/sbom` pure either way). Decide during impl.
- **specVersion 1.5 → 1.7** (approved). **New dep** `cyclonedx-go` in `pkg/sbom` only.
- **Broad rewrite** of `pkg/sbom` + its tests; behavior coverage preserved, now driven by
  `Inventory` literals instead of raw-JSON fixtures.