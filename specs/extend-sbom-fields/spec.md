# Feature Specification: extend SBOM fields (richer vulnerabilities + configurable metadata)

**Feature branch:** `feat/convert-command`
**Issue:** [#7](https://github.com/scanoss/scanoss.go/issues/7)

## Summary
Two **additive, backward-compatible** enhancements to `pkg/sbom` so embedders can produce
richer, provenance-accurate SBOMs while building the `Inventory` from their own data. No
serialization contract is introduced — embedders persist their own model and map to
`sbom.Inventory` in-memory at render time (deliberately decoupled).

1. **Richer, optional vulnerability model** — CVSS (score/vector/method), CWEs, EPSS.
2. **Configurable document metadata** — tool, author, timestamp.

## Background / current state
- `sbom.Generate(inv, format, opts...)` renders CycloneDX 1.7 / SPDX 2.3 from a neutral
  `Inventory`. `Vulnerability` currently carries only `ID/Severity/Source/URL/Summary/Purls`
  — a qualitative severity, no CVSS/CWE/EPSS.
- Both writers hardcode the author to `config.OrganizationName` (`SCANOSS`) and the document
  timestamp to `time.Now()`; the only option today is `WithProjectName`.
- Downstream consumers have CVSS/EPSS data and expect it in the output (e.g. tools like
  Dependency-Track/GUAC), and embedders need to record their own tool/author and a
  point-in-time timestamp.

## Requirements

### Functional
- **FR-001 (optional vuln fields)** Add to `sbom.Vulnerability`, all optional (nil/empty ⇒
  absent, no behavior change): `CVSSScore *float64`, `CVSSVector string`,
  `CVSSMethod string`, `CWEs []int`, `EPSSScore *float64`.
- **FR-002 (CycloneDX rendering)** In `cycloneDXVulnerabilities`: when CVSS data is present,
  emit a `ratings[]` entry with `score`, `vector`, and `method`; emit `cwes` when present.
  When no CVSS is set, keep today's single qualitative-severity rating. EPSS is rendered as
  a vulnerability `property` (`scanoss:epss_score`) since CycloneDX has no native EPSS field.
- **FR-003 (SPDX unaffected)** SPDX has no vulnerability model; these fields are ignored for
  SPDX, exactly as `Vulnerabilities` is today.
- **FR-004 (round-trip)** The CycloneDX reader (`ParseCycloneDX`) reads the new fields back
  (ratings score/vector/method, cwes, EPSS property) so `convert` preserves them.
- **FR-005 (metadata options)** Add `WithTool(name)`, `WithAuthor(name)`,
  `WithTimestamp(t time.Time)`. Defaults preserve current behavior: tool `<AppName>-<version>`,
  author `SCANOSS`, timestamp `time.Now().UTC()` (resolved when unset).
- **FR-006 (writers use options)** CycloneDX `metadata` (author, timestamp, `tools`) and SPDX
  `creationInfo` (tool `Creator`, organization `Creator`, `Created`) use the resolved options.

### Non-functional
- **NFR-001** Additive/backward-compatible: existing CLI and SDK callers unaffected; new
  fields/options are opt-in.
- **NFR-002** `pkg/sbom` stays pure/offline; no new dependencies.
- **NFR-003** `make check` clean; `go test ./... -race` clean.

## User scenarios & acceptance
1. A vuln with `Severity` only → CycloneDX rating unchanged from today (one qualitative
   rating).
2. A vuln with `CVSSScore`/`CVSSVector`/`CVSSMethod` → CycloneDX `ratings[]` carries the
   numeric score, vector, and method; still 1.7-valid.
3. A vuln with `CWEs` → CycloneDX `cwes[]`; with `EPSSScore` → a `scanoss:epss_score`
   property.
4. `Generate(inv, fmt, WithTool("my-tool"), WithAuthor("Acme"), WithTimestamp(t))` → the
   document's tool/author/timestamp reflect the overrides; without them, defaults are
   byte-unchanged from today.
5. Round-trip: an inventory with CVSS/CWE/EPSS → CycloneDX → `ParseCycloneDX` reproduces the
   modeled fields.

## Out of scope
- `Inventory` JSON serialization / json tags (embedders persist their own model — decoupled).
- Triage provenance embedded in the document (consumer stamps it on its own snapshot metadata).
- Dependency graph in `Inventory`.
- EPSS in SPDX; any SPDX vulnerability representation (SPDX 2.3 has none).
