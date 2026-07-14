# Implementation Plan: offline format conversion (`scanoss convert`)

**Spec:** `./spec.md` · **Issue:** _to be created_

## Approach

Convert = **parse → `sbom.Inventory` → `sbom.Generate`**. The writers and the raw adapter
already exist, so the new code is: two readers in `pkg/sbom` (the inverse of the writers, on
top of the official decoders), a content-based format check, and a thin offline command.

`pkg/sbom` stays pure: the readers use only `cyclonedx-go` / `spdx/tools-golang`. The raw
path stays in `scansource` (it already imports the scan types). The command in `cmd/`
orchestrates all three.

## Touch points

### `pkg/sbom` — new readers (inverse of the writers)
- `parse_cyclonedx.go` — `ParseCycloneDX(data []byte) (Inventory, error)`:
  `cdx.NewBOMDecoder(bytes.NewReader(data), cdx.BOMFileFormatJSON).Decode(&bom)`, then map
  `bom.Components` → `Component` (name/purl from `PackageURL`, version, publisher→vendor,
  licenses, website externalRef→URL) and `bom.Vulnerabilities` → `Vulnerability`
  (id, severity from ratings, source, advisory URL, summary, affects→purls). Mirrors
  `cyclonedx.go`.
- `parse_spdx.go` — `ParseSPDX(data []byte) (Inventory, error)`: `spdxjson.Read(...)`, then
  map `doc.Packages` → `Component` (purl from the `PACKAGE-MANAGER` externalRef, version,
  supplier→vendor, homepage→URL, MD5 checksum→URLHash, split the `AND`-joined
  `licenseDeclared`/`licenseConcluded` back into `License` entries with the right
  acknowledgement). Mirrors `spdxlite.go`.
- Both are pure functions; unit-testable with literal documents.

### `cmd/convert.go` — the command
- `convert <input> --format <cyclonedx|spdx> [-o <file>]`, top-level, no auth/network flags.
- `identifyFormat(data []byte) (string, error)` — check markers (`bomFormat`, `spdxVersion`,
  else scanoss v3 shape); unrecognized → error.
- Dispatch to the right reader:
  - raw → `scansource.FromScanResult` (unmarshal into `*scanossapi.ScanResult` first)
  - cyclonedx → `sbom.ParseCycloneDX`
  - spdx → `sbom.ParseSPDX`
- Warn per layer the target can't represent (e.g. vulnerabilities for spdx).
- `sbom.Generate(inv, sbom.Format(target), sbom.WithProjectName(...))` → write via
  `pkg/output` (same writer the scan command uses).

## Reuse
- Writers: `sbom.Generate` (unchanged).
- Raw reader: `scansource.FromScanResult` (unchanged).
- Output: `pkg/output.NewWriter` (same as `scan`).
- Target validation mirrors `validateOutputFormat` (minus `plain`).

## Testing
- `pkg/sbom`: `ParseCycloneDX` / `ParseSPDX` on literal documents → expected `Inventory`.
- **Round-trip** (the strongest guarantee): `Inventory → Generate → Parse → Inventory'`,
  assert `Inventory == Inventory'` for the modeled fields. Cheap because both sides exist.
- `cmd/convert_test.go`: `identifyFormat` table (cyclonedx/spdx/raw/garbage); end-to-end
  convert for each direction; the cyclonedx→spdx vulnerability-dropped warning; invalid
  `--format` errors up front.

## Documentation
The new command must be documented before the feature is considered done:
- **`CLIENT_HELP.md`** — add a `Convert (`convert`)` section (usage, the three input
  formats, the content-based identification, the offline/no-network note, the
  cyclonedx→spdx vulnerability-drop warning) and a link in the table of contents at the top.
- **`README.md`** — add a `convert` row to the **Commands** table, plus a short one-liner
  example under the SBOM/SDK area.
- **`CHANGELOG.md`** — an **Added** entry for the `convert` command under the current
  release section, per the changelog convention (this is a user-facing change).

## Risks
- **Convention reversal** (SPDX): the writer encodes `PackageName = purl`, `SPDXID = md5`,
  licenses `AND`-joined. The reader must reverse these consistently — covered by round-trip
  tests.
- **Fidelity** limited to the current `Inventory` (Open decision 1); documented as
  best-effort. Not a correctness risk, a scope choice.
