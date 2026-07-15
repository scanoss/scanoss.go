# Implementation Plan: scan output layers (SOURCE → ENRICH → RENDER)

**Spec:** `./spec.md` · **Issue:** [#4](https://github.com/scanoss/scanoss.go/issues/4)

## Approach
Build the three stages around the existing pure `pkg/sbom.Inventory` (an internal in-memory
type — not a user format). The invariant that shapes every change: **gathering is a function
of `--include`; rendering is a function of the format.** Deliver in two phases — the fused
`scan --include` path first (SOURCE + ENRICH + RENDER inside one process), then the standalone
`enrich` command + pipe.

## Formats
Three real formats: `raw` (default, implicit + explicit; the `plain` value renamed to `raw`),
`cyclonedx`, `spdx`. No `plain`/`inventory`. `raw` is the neutral `Inventory` serialized to JSON
inside a versioned envelope (`schema_version` + `metadata`), assembled in `cmd` (not `sbom`).

## Pipeline — `pkg/scanpipeline` (thin `cmd`)
The SOURCE → ENRICH orchestration lives in a reusable package, not in `cmd`:
- **`Run(ctx, Options)`** — the full flow over a source path: collect files (filters) →
  fingerprint (`pkg/scanner`) → scan (`client.Scan.WFP`) → source declared dependencies from the
  **same** collected files (`pkg/dependencies`, no second walk) → enrich → `Inventory`. Progress
  for the scan and each layer flows through the SDK's own `scanoss.WithProgress` (per `Service`);
  only fingerprinting needs an `OnFingerprint` hook, and `OnCollect` reports the filtered count.
- **`Build(ctx, client, result, layers, declared)`** — the lower half (scan result → enriched
  `Inventory`), for callers that already have a result (e.g. `scan wfp`, or an SDK consumer).
- `cmd/scan.go` stays thin: parse flags → build the client + `Options` → `Run`/`Build` → render.

## SOURCE
- `--include`-driven gather (never `--format`): parse the CSV → a layer set. `deps` is a SOURCE
  layer — parse manifests (`pkg/dependencies`) into `scope:"declared"` components appended to the
  single `Components` list; the purl-keyed layers are ENRICH.
- `--include deps` needs a tree; a `scan wfp` (no tree) simply sources no dependencies.
- `--format` default `raw`; cyclonedx/spdx via the render path.

## Component model — `pkg/sbom`
- Add `Component.Scope` (`"detected"` | `"declared"`) and `Component.DeclaredIn`. Detected
  components and declared dependencies share **one `Inventory.Components` list**, tagged by
  `scope`, so ENRICH decorates the union origin-agnostic.
- Per-component ENRICH layers (licenses, cryptography, geoprovenance) attach **inline** on
  `Component`; vulnerabilities are the flat top-level `Inventory.Vulnerabilities` list joined by
  PURL.
- The `Inventory` carries JSON tags; the `raw` output wraps it in a versioned envelope
  (`schema_version` + `metadata`). The Phase 2 pipe interchange is CycloneDX, handled by the
  existing readers/writers.

## ENRICH — decoration pipeline
- A shared helper: given an `Inventory` + requested purl-keyed layers, collect the union of
  component PURLs and run them through **`client.DecorationPipeline(...)`** (concurrent), then
  attach results (vulnerabilities flat + joined by PURL; per-component layers inline).
- Reuses the pipeline the SDK already exposes rather than adding per-service fetch logic.

## RENDER
- `sbom.Generate(inv, format, opts...)` renders `cyclonedx`/`spdx`; `raw` is the envelope
  assembled in `cmd`. Each format renders what it can and drops the rest.
- **Effective-set gathering (built):** `cmd` holds a per-format capability table and gathers only
  `--include` ∩ capabilities — `spdx` drops `vulns`/`crypto`/`geo`, `cyclonedx` drops
  `crypto`/`geo`. Each dropped layer is **skipped (not gathered)** with an up-front
  `Skipping <layer>: the <format> format cannot represent it` line, so no wasted API calls. The
  pipeline stays format-agnostic — `cmd` passes it the already-trimmed layer set.
- Declared dependencies render as ordinary components (in the one `Components` list, tagged
  `scope:"declared"`). **Deferred:** the CycloneDX `dependencies[]` graph and SPDX `DEPENDS_ON`
  edges (T6), and the `convert` → `sbom` command rename (T7).

## `scansource` adapter (SDK → Inventory)
- `FromScanResult` builds the `Inventory` from a scan result: components (keyed identity) with
  their file evidence (`source_hash`, `file_hash`, `match_type`, `match_percentage`,
  `oss_file_path`, line ranges) preserving the file↔component relation; `scope:"detected"`.
- `LicensesFrom` / `CryptographyFrom` / `GeoprovenanceFrom` map decoration responses;
  `VulnerabilitiesFrom` builds the flat list; `DeclaredFrom` maps resolved manifest deps to
  `scope:"declared"` components.

## CLI progress & output
- **Sequential phases** (fingerprint → upload → server poll) render as single-line
  `schollz/progressbar` bars, one at a time — kept on schollz so the mid-upload `Scan id:` notice
  can print without corrupting a persistent multi-line area.
- **Enrichment** runs the decoration layers concurrently, so it renders **one live bar per layer**
  via `vbauerster/mpb` (Unlicense, `cmd`-only — the SDK/`pkg/*` never pull it) under an
  `Enriching components` header. Bars are styled to match the schollz look (`|████…|` + a single
  percentage); `finish()` aborts any incomplete bar so `Wait()` never blocks. Layer labels are
  friendly (`geoprovenance.origin` → `provenance`, etc.).
- When `--output` is set, a final `Results written to <path>` line confirms where the output went.

## Phase 2 — `enrich` + pipe
- `cmd/enrich.go`: read a CycloneDX doc (`ParseCycloneDX`) → run the decoration pipeline over
  its PURLs → write CycloneDX (`Generate`). Reject `--include deps`.
- Pipe glue: `scan --format cyclonedx | enrich --include … | sbom --format spdx`.

## Testing
- Pure `pkg/sbom`: `scope` rendering, capability drop + warning, deps rendering in both
  formats, CycloneDX scope/layer round-trip (for the Phase-2 interchange).
- `cmd/scan`: `--include` parsing; decoupling (assert `--include vulns --format spdx` still
  *fetches* vulns via the pipeline but the SPDX output omits them + warns); `deps` needs a tree.
- `cmd/sbom`: rename; raw/cyclonedx/spdx inputs → cyclonedx/spdx outputs.
- Phase 2: `cmd/enrich` over CycloneDX; pipe smoke test.

## Changelog
`CHANGELOG.md` (under `[Unreleased]`) records the shipped changes:
- **Add:** output layers via `scan --include` (deps/vulns/licenses/crypto/geo); `pkg/scanpipeline`.
- **Changed:** the `plain` format → `raw` (now the neutral inventory in a versioned envelope).

The `convert` → `sbom` rename is deferred, so the existing `convert` CHANGELOG entry is left as-is.

## Risks and notes
- **No wasted work.** The fused `scan` skips gathering a layer its format can't render (e.g.
  `--include vulns --format spdx` does not call the vulnerability service). This trims gathering
  by `--format`, which the original FR-001 forbade — a deliberate refinement: the trimming lives
  in `cmd`, the pipeline stays format-agnostic, and the composable pipe (Phase 2, format unknown)
  still gathers the full requested set.
- **`plain` → `raw`** is a breaking format-value rename, acceptable pre-release; recorded in the
  CHANGELOG.
