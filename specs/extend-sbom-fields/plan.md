# Implementation Plan: extend SBOM fields

**Spec:** `./spec.md` · **Issue:** [#7](https://github.com/scanoss/scanoss.go/issues/7)

## Approach
Two independent, additive changes in `pkg/sbom`. Nothing about the public `Generate`
signature changes; new struct fields and functional options are opt-in, so existing callers
(the CLI, current SDK users) are byte-for-byte unaffected unless they set the new fields.

## Touch points

### A. Richer vulnerability model
- `inventory.go` — add optional fields to `Vulnerability`: `CVSSScore *float64`,
  `CVSSVector string`, `CVSSMethod string`, `CWEs []int`, `EPSSScore *float64`.
- `cyclonedx.go` — `cycloneDXVulnerabilities`: build the `ratings[]` from CVSS when present
  (`Score`, `Vector`, `Method`), else the current qualitative severity; set `CWEs`; add a
  `scanoss:epss_score` property when `EPSSScore != nil`. (Confirm the exact cyclonedx-go
  field names during implementation: `VulnerabilityRating.{Score,Vector,Method}`,
  `Vulnerability.CWEs`, `Vulnerability.Properties`.)
- `parse_cyclonedx.go` — read those back (score/vector/method → CVSS fields, `cwes` → CWEs,
  `scanoss:epss_score` property → EPSSScore) so `convert` round-trips them.
- `spdxlite.go` — no change (SPDX ignores vulnerabilities).

### B. Configurable document metadata
- `options.go` — add `toolName`, `author`, `timestamp` to `options`; add `WithTool`,
  `WithAuthor`, `WithTimestamp`; keep current defaults; add `resolvedTimestamp()` (defaults
  to `time.Now().UTC()` when unset). New imports: `time`, `internal/config`.
- `cyclonedx.go` — `buildCycloneDX` metadata uses `o.author`, `o.resolvedTimestamp()`, and
  `o.toolName` (`metadata.tools`).
- `spdxlite.go` — `buildSPDXLite` creationInfo uses `o.toolName` / `o.author` /
  `o.resolvedTimestamp()`.

## Testing
- `cyclonedx_test.go` — a vuln with CVSS → ratings carry score/vector/method; CWEs present;
  EPSS property present; a severity-only vuln unchanged.
- `roundtrip_test.go` — extend the CycloneDX case with CVSS/CWE/EPSS and assert they survive
  `Generate → ParseCycloneDX`.
- `options_test.go` — `WithTool`/`WithAuthor`/`WithTimestamp` apply; defaults unchanged.
- `spdxlite_test.go` — tool/author/created reflect overrides; SPDX still ignores vulns.

## Risks
- **EPSS has no native CycloneDX field** → represented as a property (namespaced
  `scanoss:epss_score`); documented as best-effort. Not a correctness risk.
- Keep everything additive — verify no existing golden/test output changes when the new
  fields/options are unset.
