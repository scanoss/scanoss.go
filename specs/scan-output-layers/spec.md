# Feature Specification: scan output layers — SOURCE → ENRICH → RENDER

**Feature branch:** `feat/scan-output-layers`
**Issue:** [#4](https://github.com/scanoss/scanoss.go/issues/4)
**Basis:** the three-stage model (SOURCE → ENRICH → RENDER) + `--include` UX from the #4
discussion, plus two follow-through decisions (see *Impact on `convert`* below).

## Summary
Add opt-in output **layers** (dependencies, vulnerabilities, and — by the same mechanism —
licenses, cryptography, geoprovenance) to the scan output, under one governing
principle:

> **Gathering follows `--include`, narrowed to what the chosen output format can render — the
> fused `scan` command never fetches a layer that format would only drop. The scan pipeline
> itself stays format-agnostic; the composable path (Phase 2), whose target format isn't fixed,
> gathers the full requested set.**

Everything is modeled as three stages over a single neutral, in-memory `Inventory`:

```
 SOURCE (needs the tree)      ENRICH (needs only PURLs)        RENDER (needs only Inventory)
  scan → detected components   decoration pipeline over the      raw / cyclonedx / spdx
  deps → declared components   PURL set: vulnerabilities,         (each drops what it
        (manifest parsing)     licenses, cryptography,            can't represent)
                               geoprovenance
```

The same core is usable two ways: **fused in one command** (`scan --include … --format …`)
or **as a composable pipe** (`scan → enrich → sbom`), see Phase 2.

## Formats
There are exactly **three** formats (there is no separate `plain` or `inventory` format):
- **`raw`** — the neutral `Inventory` in a versioned envelope (`schema_version` + `metadata` +
  `components` + `vulnerabilities`). Detected and declared components share **one `components`
  list**, tagged by `scope` (`detected` | `declared`); per-component layers (`licenses`,
  `cryptography`, `geoprovenance`) attach **inline** on each component; `vulnerabilities` is a
  flat top-level list joined by PURL. Layers are present only when gathered. **Default, implicit**
  (also `--format raw`). A bare scan with no `--include` renders the components only and does no
  decoration work.
- **`cyclonedx`** — CycloneDX 1.7.
- **`spdx`** — SPDX 2.3 (Lite).

The neutral `Inventory` is the in-memory type the stages pass data with; `raw` is just its JSON
encoding (the SBOM formats are lossy projections of it).

## Background / current state
- `cmd/scan.go` currently couples gathering to format: `if format == config.FormatCycloneDX
  { inv.Vulnerabilities = fetchVulnerabilities(...) }`. Vulns are fetched *because the format
  is cyclonedx*, not because the user asked. This is the exact coupling to remove.
- `pkg/sbom` is a pure `Inventory → document` renderer (`Generate`), already enriched with
  CVSS/CWE/EPSS and configurable metadata (#7). Readers `ParseCycloneDX`/`ParseSPDX` and
  `scansource.FromScanResult` exist.
- The SDK already ships a **decoration pipeline** (`client.DecorationPipeline(...)`, see the
  README) that runs decoration services concurrently over a PURL set — this is the ENRICH
  engine.
- The **`convert`** command (offline: a saved result/doc → cyclonedx/spdx) already exists — it
  is the seed of the RENDER stage.
- `pkg/dependencies` + `pkg/manifests` parse manifests — the SOURCE `deps` primitive.

## The two axes (the core idea)
- **What we gather** = the requested layers (`--include`) ∩ what the target format can render.
  The fused `scan` command skips (does not fetch) a layer its format can't show, to avoid wasted
  work. The pipeline itself is format-agnostic — the intersection is computed by the caller (`cmd`).
- **What we render** = the format's capabilities.

`dependencies` is **source-derived** (needs the tree) → it lives in SOURCE and can only run
where files exist. Every other layer is **purl-derived** → it lives in ENRICH and runs through
the decoration pipeline over the `Inventory`'s PURLs.

## UX — one command, `--include` list
```bash
scanoss scan ./my-project                                    # raw (implicit default), no extra work
scanoss scan ./my-project --format raw                       # same, explicit
scanoss scan ./my-project --include deps
scanoss scan ./my-project --include deps,vulns
scanoss scan ./my-project --include deps,vulns,licenses,crypto

scanoss scan ./my-project --include deps,vulns --format cyclonedx   # format only projects
scanoss scan ./my-project --include deps       --format spdx

# A layer the format can't show → skipped (not gathered):
scanoss scan ./my-project --include deps,vulns --format spdx
#   Skipping vulnerabilities: not supported by the spdx format
```
`--include` accepts short aliases mapping to the primitives: `deps`, `vulns`, `licenses`,
`crypto`, `geo`. One list flag — no boolean `--include-*` flags.

## ENRICH via the decoration pipeline
The purl-keyed layers are gathered through the SDK's **decoration pipeline**
(`client.DecorationPipeline(...)`), which already runs the decoration services concurrently
over one PURL set and returns per-service results. ENRICH:
1. collects the PURLs from the `Inventory`'s components (detected + declared),
2. runs the requested services through the decoration pipeline,
3. attaches results to the `Inventory` — per-component layers (licenses, cryptography,
   geoprovenance) inline on each component by PURL; vulnerabilities as the flat top-level list
   joined by base PURL.

ENRICH is a thin adapter over the decoration pipeline and adds no per-service fetch logic.

## Components: one list, tagged by scope
Detected and declared components live in **one `Inventory.Components` slice, tagged by `scope`**
(`detected` | `declared`), not a separate `Dependencies` list. ENRICH decorates the union of
PURLs origin-agnostically; RENDER filters by `scope` only when it must (SPDX `DEPENDS_ON`,
CycloneDX `dependencies[]`). Declared components also carry `DeclaredIn` (the manifest path).

## Commands
- **`scan <path> --include <layers> --format <raw|cyclonedx|spdx>`** — SOURCE (always) +
  ENRICH + RENDER fused. `deps` is a SOURCE layer (needs the tree → error on `scan wfp`).
  Default `--format raw`.
- **`sbom <doc> --format <cyclonedx|spdx>`** — RENDER stage, offline. Reads a raw result,
  a CycloneDX, or an SPDX document and projects to the target format. **This is the renamed
  `convert` command** (see below).
- **`enrich <cyclonedx-doc> --include <purl-layers>`** *(Phase 2)* — ENRICH stage as a
  standalone command: reads a CycloneDX document, runs the decoration pipeline over its PURLs,
  writes an enriched CycloneDX. **Cannot** do `--include deps` (needs the tree). Re-runnable
  (e.g. weekly against a fresh vuln DB). CycloneDX is the interchange because it is the only
  format rich enough to carry every layer losslessly.

Piped form (Phase 2):
```bash
scanoss scan ./my-project --include deps --format cyclonedx \
  | scanoss enrich --include vulns,licenses \
  | scanoss sbom --format spdx
```

## Impact on `convert` → renamed to `sbom`
The current offline `convert` command **is** the RENDER stage. It is **renamed to `sbom`**.
Because the scan output now carries extra layers (declared deps, vulnerabilities, per-component
intelligence) when rendered to cyclonedx, the RENDER command must consume and project them.
Its inputs are the three real formats it already auto-detects (**raw, cyclonedx, spdx**);
there is no `inventory` or `plain` input. Pre-release, so the rename is an acceptable breaking
change; `convert` is removed (no alias).

## Capability table
```
raw       : all layers (the neutral inventory in a versioned envelope)
spdx      : components (detected + declared) + licenses — NOT vulns / crypto / geo
cyclonedx : components + licenses + vulnerabilities — NOT crypto / geo
```
In the fused `scan` command, `cmd` intersects the requested layers with this table: layers the
format can't render are **skipped (not gathered)** and reported up front. The pipeline never sees
the format.

## Requirements
### Functional
- **FR-001 (gather the effective set)** The fused `scan` gathers `--include` ∩ the target
  format's capabilities; a layer the format can't render is skipped (not fetched) and reported.
  The pipeline stays format-agnostic — `cmd` computes the intersection. (Refines the original
  "format never influences gathering" rule, which now applies only to the composable pipe whose
  format is not fixed.)
- **FR-002 (`--include`)** `scan` accepts `--include <csv>` with aliases `deps`, `vulns`,
  `licenses`, `crypto`, `geo`. Unknown alias → error. Empty/absent → no layers,
  output byte-identical to today.
- **FR-003 (SOURCE deps)** `--include deps` parses manifests via `pkg/dependencies` and adds
  `scope:"declared"` components to the `Inventory`. Only valid where a tree exists
  (`scan <path>`, not `scan wfp`).
- **FR-004 (ENRICH via decoration pipeline)** purl-keyed layers are gathered through
  `client.DecorationPipeline(...)` over the union of component PURLs and attached to the
  `Inventory` (vulnerabilities flat + joined by PURL; per-component layers inline).
- **FR-005 (formats)** `--format` accepts `raw` (default, implicit + explicit), `cyclonedx`,
  `spdx`. No `plain`/`inventory`. `raw` is the neutral inventory in a versioned envelope.
- **FR-006 (RENDER + capability table)** Each format renders the layers it supports; a requested
  layer it can't render is skipped up front (not gathered), reported to the user. Deferred:
  dependency-graph rendering (CycloneDX `dependencies[]`, SPDX `DEPENDS_ON`).
- **FR-007 (`sbom` command = renamed `convert`)** Rename `convert` → `sbom`; render offline to
  cyclonedx/spdx from a raw/cyclonedx/spdx input; remove `convert`.
- **FR-008 (`enrich` command, Phase 2)** Reads a CycloneDX doc, applies purl-keyed layers via
  the decoration pipeline, writes CycloneDX. Rejects `--include deps`.
- **FR-009 (SDK purity)** `raw` = the raw `scanossapi.ScanResult`; the `Inventory` is a
  CLI-internal type. `pkg/scanoss` scan types unchanged.

### Non-functional
- **NFR-001** `pkg/sbom` stays pure/offline; ENRICH/SOURCE live in `cmd` + the SDK clients.
  No new third-party dependencies for the core.
- **NFR-002** `--include` unset ⇒ zero behavior change vs today (no API calls, no parsing).
- **NFR-003** `make check` + `go test ./... -race` clean.

## Out of scope (v1)
- **Transitive dependencies** — direct/declared only. Every `declared` component is a direct
  dep of the project root; renderers synthesize `project → dep` `DEPENDS_ON` without an
  explicit edge list.
- Serial `enrich-vulns | enrich-licenses | …` chains — ENRICH runs the decoration pipeline
  (concurrent) in one process.

## Open decisions
1. **CycloneDX as the ENRICH/pipe interchange (Phase 2).** CycloneDX is the only real format
   rich enough to round-trip every layer (scope via a property, crypto/geo via
   properties). Confirm CycloneDX as the pipe carrier, or defer the standalone `enrich`/pipe
   entirely to a later feature. _Recommend: ship Phase 1 (fused `scan --include`) first; treat
   `enrich`/pipe as Phase 2._
2. **`raw` naming.** The current internal value is `plain`; this renames it to `raw` (breaking,
   pre-release). Confirm the user-facing name is `raw`.
3. **Layer aliases** — final names (`deps`/`dependencies`, `vulns`/`vulnerabilities`, …) and
   whether both long and short forms are accepted.
