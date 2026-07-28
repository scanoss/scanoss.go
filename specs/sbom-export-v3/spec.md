# Feature Specification: SBOM generation module (CycloneDX + SPDX Lite) over a neutral inventory

**Feature branch:** `feat/sbom-export-v3`
**Issue:** [#14](https://github.com/scanoss/scanoss.go/issues/14)
**Status:** Draft

## Summary
Expose a **standalone, pure SBOM-generation module** that turns a neutral inventory of
components (and their vulnerabilities + file evidence) into a CycloneDX or SPDX Lite
document — usable by **both** the `scanoss` CLI and external consumers
through `scanoss` as an SDK.

Generation is an **on-demand action over an inventory**, decoupled from scanning: the
caller assembles an `sbom.Inventory` from whatever data it holds and calls one function,
`sbom.Generate(inv, format, …)`. The CLI assembles its inventory from a live v3 scan
result; a downstream consumer assembles it from its stored identifications (a user clicks "export →
CycloneDX"). Neither scanning nor any network/SDK call is required at generation time.

Both formats are produced with their **official libraries** — CycloneDX via
[`cyclonedx-go`](https://github.com/CycloneDX/cyclonedx-go) (spec **1.7**) and SPDX via
[`spdx/tools-golang`](https://github.com/spdx/tools-golang) (SPDX **2.3**, Lite field
subset) — removing all hand-rolled serialization. This fixes #14 (the old hand-rolled
CycloneDX exporter emitted `license.acknowledgement` under spec `1.5`, where the schema
rejects it).

### Background / current state (why this is needed)
- The exporters in `pkg/sbom` parse the **old** scan-result shape (`map[string][]entry`,
  via `scanEntry`/`ExtractComponents`) and are wired to nothing. `scanoss scan
  -f cyclonedx|spdx` only prints `Warning: not yet supported for v3 scan results`
  and writes raw JSON (`cmd/scan.go`).
- `pkg/sbom/cyclonedx.go` is hand-rolled and declares `SpecVersion "1.5"` while emitting
  `acknowledgement` (invalid in 1.5 → #14).
- v3 data model (`pkg/scanoss/scan_result.go`): `ScanResult{ Files map[string]FileMatch;
  Components.Results []ComponentInfo }`; `ComponentInfo{ URLHash; Component
  json.RawMessage }`; `FileMatch{ URLIDs []string; PurlVersions []string; MatchType;
  InputLineRanges; OssLineRanges }`. The component catalog is already deduplicated; the
  raw `component` payload carries the canonical SCANOSS fields (`purl []string`,
  `licenses [{name, source}]`, `url`, `vendor`, `component`, …).
- Licenses and vulnerabilities come from **decoration services**, not the scan payload:
  `client.Licenses.Components → *openapi.ComponentsLicenseResponse` (per-component license
  IDs — the declared set) and `client.Vulnerabilities.Components →
  *openapi.VulnerabilitiesResponse`. Both return structured, already-discrete data (no
  string splitting). The licenses decoration has no declared/concluded flag; its licenses
  are **declared**, while **concluded** licenses come only from a downstream consumer's identifications.

## Packaging & layering (the shared module)
A hard purity boundary makes the module safe for any consumer to depend on:

```
pkg/sbom   ── PURE: Inventory → report. Only dependency: cyclonedx-go.
   ▲   ▲       Public: Inventory/Component/Vulnerability/FileEvidence, Format(+consts),
   │   │       Option(+WithProjectName), and ONE func Generate(inv, format, opts...).
   │   │       Imports NOTHING from pkg/scanoss.
   │   │
   │   └────────────── pkg/sbom/scansource  (SDK→Inventory adapters)
   │                       imports pkg/sbom + pkg/scanoss + pkg/scanoss/openapi
   │                       FromScanResult(...) , VulnerabilitiesFrom(...)
   │                            ▲
 a downstream consumer                    cmd/scan.go
 (imports only pkg/sbom;   (CLI: scan → scansource → sbom.Generate)
  builds Inventory from
  its own identifications)
```

- **`pkg/sbom`** depends only on `cyclonedx-go`. a downstream consumer can import it without pulling in
  the scan SDK or API client, and there is no import cycle (it imports nothing from
  `pkg/scanoss`).
- **`pkg/sbom/scansource`** holds the only SDK-coupled code: adapters that build an
  `sbom.Inventory` from a `*scanoss.ScanResult` and from a decoration
  `*openapi.VulnerabilitiesResponse`. The CLI uses it; a downstream consumer does not.
- One generator, two ways of assembling the inventory.

SDK usage (a downstream consumer — no scan, no network at report time):
```go
inv := buildInventoryFromIdentifications()  // a downstream consumer's stored components + vulnerabilities
report, _ := sbom.Generate(inv, sbom.Format(chosenFormat), sbom.WithProjectName(project))
```

## Data model — the SBOM inventory (`pkg/sbom`)
The generator depends only on these types — never on `pkg/scanoss`.

```go
// Inventory is the neutral, format-agnostic bill of materials: the components to emit
// and the vulnerabilities affecting them. Built directly by a consumer, or via the
// scansource adapters from a scan result.
type Inventory struct {
    Components      []Component
    Vulnerabilities []Vulnerability
}

// Component is one detected component to emit in the SBOM.
type Component struct {
    Purl             string         // base PURL, e.g. "pkg:github/scanoss/engine"
    Vendor           string         // supplier / namespace
    Name             string         // component name
    Version          string         // resolved version; "" → "NOASSERTION" in output
    URL              string         // homepage / source URL ("" → no externalReference)
    URLHash          string         // SCANOSS url_hash (SPDX package checksum)
    Licenses         []License      // declared and/or concluded licenses
    DownloadLocation string         // download location (defaults to URL)
    Files            []FileEvidence // scanned files that matched this component
}

// LicenseAcknowledgement is how a license was established for a component.
type LicenseAcknowledgement string

const (
    AckDeclared  LicenseAcknowledgement = "declared"  // stated by the project (decoration)
    AckConcluded LicenseAcknowledgement = "concluded" // determined by review (a downstream consumer)
)

// License is a single license on a component, with its acknowledgement. The same id
// may appear twice (e.g. declared and concluded).
type License struct {
    ID              string                 // SPDX id or "LicenseRef-*", e.g. "GPL-2.0-only"
    Acknowledgement LicenseAcknowledgement // declared (default) | concluded
}

// FileEvidence is one scanned file that matched a component (a CycloneDX
// evidence.occurrence). For snippet matches it carries the matched line ranges.
type FileEvidence struct {
    Path            string      // scanned file path (the occurrence "location")
    MatchType       string      // "file" (whole file) | "snippet"
    InputLineRanges []LineRange // matched line ranges in the scanned file (snippet only)
    OssLineRanges   []LineRange // matched line ranges in the OSS component (snippet only)
}

// LineRange is a matched line range (inclusive), mirroring the scan engine's shape so
// ranges are never encoded to text and parsed back.
type LineRange struct {
    StartLine int // first line of the matched range
    EndLine   int // last line of the matched range
}

// Vulnerability is one known vulnerability affecting one or more components, in a
// format independent of the decoration API's wire type.
type Vulnerability struct {
    ID       string   // advisory/CVE id, e.g. "CVE-2021-1234" (dedupe key)
    Severity string   // normalized lower-case: critical|high|medium|low|none|"" (unknown)
    Source   string   // advisory source name, e.g. "NVD"
    URL      string   // advisory URL (optional)
    Summary  string   // short description (optional)
    Purls    []string // base PURLs of affected components (join key to Component.Purl)
}
```

## API

### Pure module (`pkg/sbom`)
```go
type Format string
const (
    FormatCycloneDX Format = "cyclonedx"
    FormatSPDX  Format = "spdx"
)

type Option func(*options)
func WithProjectName(name string) Option   // names the top-level component / SPDX doc;
                                           // default "scanoss-project" when empty

// Generate renders the inventory to the requested SBOM format. It is the single public
// entry point. Unknown format → error. An empty inventory → a valid, empty document.
func Generate(inv Inventory, format Format, opts ...Option) (string, error)
```
- There is exactly **one** generation function. The per-format builders
  (`buildCycloneDX`, `buildSPDXLite`) are **unexported**. Both real consumers select the
  format at runtime (a CLI flag, a consumer-side button), so a typed format + one dispatcher
  fits; format-specific public wrappers would be redundant surface.
- Vulnerabilities and file evidence are **inventory data**, not options — the only option
  today is `WithProjectName`.

### SDK adapters (`pkg/sbom/scansource`) — CLI-only glue
```go
func FromScanResult(result *scanoss.ScanResult) sbom.Inventory
func LicensesFrom(resp *openapi.ComponentsLicenseResponse) map[string][]sbom.License // base PURL -> declared licenses
func VulnerabilitiesFrom(resp *openapi.VulnerabilitiesResponse) []sbom.Vulnerability
```

## User Scenarios & Testing

### Primary user story
As an SBOM consumer (the CLI, or a downstream consumer via the SDK) I assemble an inventory of the
components I care about and call one function to get a schema-valid CycloneDX or SPDX
Lite document — with no scan and no network call required at generation time.

### Acceptance scenarios
1. **Given** an inventory with components, **when** I call `Generate(inv,
   FormatCycloneDX)`, **then** the output is a CycloneDX **1.7** document that validates
   (`cyclonedx validate --input-version v1_7`), one `library` component per inventory
   entry, each `Component.Licenses` rendered as a `licenses[].license` with its
   `acknowledgement`.
2. **Given** the same inventory, **when** `Generate(inv, FormatSPDX)`, **then** a
   valid SPDX Lite JSON document listing the same components, with declared license IDs in
   `licenseDeclared` and concluded ones in `licenseConcluded`.
2b. **Given** a component with both a declared and a concluded `GPL-2.0-only` (plus a
   concluded-only `3D-Slicer-1.0`), **when** `Generate(inv, FormatCycloneDX)`, **then** the
   `licenses[]` has three entries: `GPL-2.0-only`/declared, `GPL-2.0-only`/concluded, and
   `3D-Slicer-1.0`/concluded.
3. **Given** an empty inventory, **then** both formats produce a valid, empty-ish
   document without error.
4. **Given** an unknown format, **then** `Generate` returns an error.
5. **Given** an inventory whose `Vulnerabilities` is non-empty, **when**
   `Generate(inv, FormatCycloneDX)`, **then** the document has a top-level
   `vulnerabilities[]`, one entry per distinct id, each with a severity rating and an
   `affects[].ref` resolving to the `bom-ref` of every affected component; still
   1.7-valid.
6. **Given** a component with file/snippet matches in `Files`, **when**
   `Generate(inv, FormatCycloneDX)`, **then** that component has `evidence.occurrences[]`
   (one per file, `location` = path; snippet occurrences carry the matched line ranges);
   still 1.7-valid.
7. **Given** the CLI `scanoss scan ./proj -f cyclonedx`, **then** it builds the
   inventory from the v3 result via `scansource.FromScanResult`, fetches the licenses
   decoration (declared licenses) and the vulnerabilities decoration, calls
   `sbom.Generate`, and writes a valid 1.7 document; `-f spdx` likewise but fetches only
   licenses (no vulnerabilities); `-f plain` is unchanged (raw v3 JSON, no decoration
   call).
8. **Given** an SDK consumer with stored identifications and **no** live scan,
   **when** it builds an `sbom.Inventory` and calls `sbom.Generate(inv, fmt,
   WithProjectName("x"))`, **then** it gets the same document the CLI would, importing
   only `pkg/sbom` (not the scan SDK).

### Edge cases
- Component with no resolvable version → `version` = `NOASSERTION`; PURL has no trailing
  `@`.
- `LicenseRef-*` license → CycloneDX `name` (not `id`); standard SPDX id → `id`.
- Component with no `url` → no `externalReferences` entry.
- Component matched by multiple files → one component, multiple `evidence.occurrences`
  (deterministic order, sorted by path).

## Requirements

### Functional
- **FR-001 (remove old-results)** Delete the old-results path: `scanEntry`,
  `ExtractComponents(rawJSON string)`, `stripPurlVersion`, and any helper that exists
  only to parse `map[string][]entry`. Keep version compare, supplier/namespace, `MD5Hash`
  where still used. **`splitLicenses` is removed** — licenses arrive as discrete IDs from
  decoration, never as a joined string to split.
- **FR-002 (pure module — types + one entry point)** `pkg/sbom` defines `Inventory`,
  `Component`, `License` (+ `LicenseAcknowledgement`, `AckDeclared`/`AckConcluded`),
  `FileEvidence`, `Vulnerability`, `Format` (+ `FormatCycloneDX`,
  `FormatSPDX`), `Option` (+ `WithProjectName`), and a single exported function
  `Generate(inv Inventory, format Format, opts ...Option) (string, error)` dispatching to
  unexported `buildCycloneDX`/`buildSPDXLite`. `pkg/sbom` imports only the two SBOM
  libraries (`cyclonedx-go`, `spdx/tools-golang`) — no `pkg/scanoss`. Empty inventory /
  unknown format handled per scenarios 3–4.
- **FR-003 (CycloneDX render)** `buildCycloneDX` builds a BOM with `cyclonedx-go` and
  serializes via `NewBOMEncoder(JSON).SetPretty(true).EncodeVersion(bom,
  cdx.SpecVersion1_7)`. Output: `specVersion 1.7`, `bomFormat CycloneDX`, metadata (UTC
  RFC3339 timestamp, author `SCANOSS`, top-level `application` component named per
  `WithProjectName`), one `library` component per entry (`name` = PURL, `version` or
  `NOASSERTION`, `publisher` = supplier, `purl` with `@version` when known, stable
  `bom-ref` = `purl[@version]`), one `licenses[].license` entry per `Component.Licenses`
  with its `acknowledgement` (`declared`|`concluded`; `LicenseRef-*` → name else id) — see
  FR-007, a `website` externalReference when the component has a URL,
  `evidence.occurrences[]` from `Files` (FR-005), and a top-level `vulnerabilities[]`
  from `Inventory.Vulnerabilities` (FR-006).
- **FR-004 (SPDX render)** `buildSPDXLite` produces an SPDX **2.3** JSON document via the
  official `github.com/spdx/tools-golang` library (`spdx/v2/v2_3` model + `json` writer),
  restricted to the SPDX Lite field subset: document (`SPDX-2.3`, `CC0-1.0`,
  `SPDXRef-DOCUMENT`, name per `WithProjectName`, a deterministic `documentNamespace`),
  creation info (`Tool: scanoss-<version>`, `Organization: SCANOSS`, timestamp), one
  package per component (name = PURL, `SPDXRef-<md5(purl@version)>`, versionInfo,
  supplier, downloadLocation, homepage, `licenseDeclared` = `AND`-joined declared license
  IDs (or NOASSERTION), `licenseConcluded` = `AND`-joined concluded IDs (or NOASSERTION),
  `copyrightText` = NOASSERTION, a `purl` PACKAGE-MANAGER externalRef, an MD5 checksum
  from `url_hash` when present, `filesAnalyzed:false`), and `hasExtractedLicensingInfos`
  for any `LicenseRef-*` license (declared or concluded). Vulnerabilities and file
  evidence do not apply to SPDX.
- **FR-005 (file evidence)** When a component has `Files`, CycloneDX emits
  `evidence.occurrences[]`: one occurrence per file, `location` = path; for snippet
  matches, `line` = the first input range's start and `additionalContext` = the full
  matched ranges as text (e.g. `"input lines 12-48; oss lines 100-136"`); whole-file
  matches set only `location` (Open decision 6).
- **FR-006 (vulnerabilities are inventory data)** CycloneDX emits one `cdx.Vulnerability`
  per **distinct** `Vulnerability.ID`, with `Source{Name}`, a `Ratings[]` severity
  (`mapSeverity`: lower-cased critical/high/medium/low/none → `cdx.Severity`, else
  `unknown`), `Description` = summary, optional advisory `URL`, and `Affects[]` = the
  `bom-ref`s of every component whose base PURL is in the vuln's `Purls`. Empty
  `Inventory.Vulnerabilities` → no `vulnerabilities[]`.
- **FR-007 (licenses + vulnerabilities via decoration → inventory)** Licenses and
  vulnerabilities are **inventory data sourced from decoration services**, not the scan
  payload. `pkg/sbom/scansource` provides the only SDK-coupled code:
  - `FromScanResult(*scanoss.ScanResult) sbom.Inventory` — iterate the deduplicated
    `Components.Results`, decode each raw `component` payload into a `Component` (purl,
    vendor, name, url; version from payload, else from `Files[].PurlVersions` joined by
    `url_hash`, else "" — Open decision 1), and populate each component's `Files` by
    joining `result.Files` on `url_hash` (`URLIDs`), carrying
    `MatchType`/`InputLineRanges`/`OssLineRanges`, sorted by path. **Licenses and
    vulnerabilities are left empty** (attached from decoration).
  - `LicensesFrom(*openapi.ComponentsLicenseResponse) map[string][]sbom.License` — per
    base PURL, each `LicenseInfo.Id` as a `License{ID, AckDeclared}` (deduped). No
    splitting; the decoration returns discrete IDs.
  - `VulnerabilitiesFrom(*openapi.VulnerabilitiesResponse) []sbom.Vulnerability` — map +
    dedupe by id (fallback `cve`), accumulating affected `Purls`.
  - Concluded licenses are not produced by `scansource` — they come only from a consumer
    (a downstream consumer) setting `Component.Licenses` with `AckConcluded` entries.
- **FR-008 (CLI wiring)** `cmd/scan.go`, after bom.remove: for `-f plain` it writes the
  raw v3 result JSON as today (no SBOM, no decoration call). For `-f cyclonedx|spdx`
  it builds `inv := scansource.FromScanResult(res.Result)`, then fetches the **licenses**
  decoration (`client.Licenses.Components(ctx, comps)`) and attaches the declared licenses
  to each component by base PURL (`LicensesFrom`); for `cyclonedx` it **also** fetches
  vulnerabilities (`client.Vulnerabilities.Components`, Open decision 5) →
  `inv.Vulnerabilities`. Each decoration fetch is non-fatal: on failure it warns and
  continues without that data (Open decision 4). Then `out, _ := sbom.Generate(inv,
  sbom.Format(outputFormat), sbom.WithProjectName(filepath.Base(scanPath)))` → write. The
  warning branch is removed; `--format`, `config.Format*`/`DefaultFormat` are retained and
  now functional; `scanPath` is threaded into `uploadAndWrite`. `--format` is validated up
  front (FR-009).
- **FR-009 (fail-fast format validation)** `--format` is validated at the **start** of
  `runScan`/`runScanWFP` (via `validateOutputFormat`, before any fingerprinting or
  upload). An invalid value (e.g. the old `spdxlite`) errors immediately with `invalid
  output format %q: must be plain, spdx, or cyclonedx` and does no work.

### Non-functional
- **NFR-001** Two new dependencies, both in `pkg/sbom`: `github.com/CycloneDX/cyclonedx-go`
  and `github.com/spdx/tools-golang` (Apache-2.0 OR GPL-2.0-or-later — elect Apache-2.0).
  Vulnerabilities reuse the existing SDK + `openapi` types. No other deps.
- **NFR-002** `go build ./... && go vet ./... && gofmt -l . && go test ./... -race`
  clean.
- **NFR-003** Layering: `pkg/sbom` imports only `cyclonedx-go` + `spdx/tools-golang`;
  `pkg/sbom/scansource`
  imports `pkg/sbom` + `pkg/scanoss` + `pkg/scanoss/openapi`; `pkg/scanoss` imports
  neither. No import cycle. No change to fingerprinting, upload/poll, bom.remove,
  `--output`, or other subcommands.

## Open decisions
0. **Licenses sourced from the decoration service** (`client.Licenses.Components`), not
   the scan payload — **decided** (user request): the declared license set comes from the
   licenses decoration for both CycloneDX and SPDX; a bare `FromScanResult` (no decoration
   call) yields components with no licenses until decoration runs. Concluded licenses come
   only from a downstream consumer. The scan payload's `licenses` field is no longer read.
1. **Component version source.** When the catalog payload lacks `version`, resolve from
   referencing files' `purl_versions` (join by `url_hash`, highest wins), else
   `NOASSERTION`. **Recommended.** _Confirm, or payload-only._
2. **SPDX via the official `spdx/tools-golang` library** (SPDX 2.3, Lite field subset) —
   **decided**: both exporters now use their official library, removing all hand-rolled
   serialization. (Was: keep hand-rolled. Changed at the user's request.)
3. **`projectName` for `scan wfp <file>`** = `filepath.Base` of the `.wfp` path, same
   `scanoss-project` fallback. **Recommended.** _Confirm._
4. **Vulnerability-fetch failure** (CLI): warn + emit SBOM without vulnerabilities (don't
   fail the command). **Recommended.** _Confirm._
5. **How the CLI fetches vulnerabilities**: direct `client.Vulnerabilities.Components(ctx,
   comps)` (simplest). _Alternative:_ the `DecorationPipeline` (progress UI, heavier).
   **Recommend direct.** _Confirm._
6. **File-range representation** in occurrences: `line` = first input range start +
   `additionalContext` = full ranges text (CycloneDX occurrences have no native range).
   **Recommended.** _Confirm._
7. **Does the CLI keep generating at scan time?** Per "generate from identifications, not
   during scanning," the *module* never scans — but the CLI is a consumer that builds its
   inventory from a live scan. **Recommend keeping `scan -f cyclonedx|spdx`** (the CLI
   just assembles its inventory differently from a downstream consumer). _Confirm, or restrict SBOM
   output to a separate generate-from-saved-data step only._

## Out of scope
- **Removing the v2 scan client (`pkg/api`, `pkg/batch`).** Still used by the
  `libscanoss/core` Python/Node bindings. (`pkg/scanner` + `internal/models` are the
  active v3 fingerprinting layer; this feature does not depend on the old result shape.)
- **Exposing `pkg/sbom` through the `libscanoss` (Python/Node) bindings.** If a downstream consumer is
  non-Go and consumes via the bindings, wiring the module into `libscanoss/core` is a
  separate effort.
- A CLI command to generate an SBOM from a previously-saved scan-result/identifications
  file (depends on Open decision 7).
- CycloneDX health/crypto data, SPDX relationships, XML output, `acknowledgement:
  concluded`, license text, hashes.
