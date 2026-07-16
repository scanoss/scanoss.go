# Feature Specification: online inventory enrichment (`scanoss enrich`)

**Feature branch:** `feat/enrich-command`
**Issue:** [#9](https://github.com/scanoss/scanoss.go/issues/9)
**Status:** Draft

## Summary
A new **online** `scanoss enrich` command that takes an existing inventory or SBOM — the
scanoss raw envelope, CycloneDX, or SPDX — and **decorates its components with the requested
purl-keyed layers** (`vulns`, `licenses`, `crypto`, `geo`) through the SCANOSS decoration API,
then renders the enriched result in the chosen output format. It needs **only the inventory**:
no source tree, no fingerprinting, no re-scan. Because it is keyed purely by PURL, it is
**re-runnable** (e.g. weekly) to refresh vulnerabilities/licenses against the same file.

```
scanoss enrich inv.json       --include vulns,licenses,crypto            > enriched.json
scanoss enrich sbom.spdx.json --include vulns,licenses,crypto            > enriched.spdx.json
scanoss enrich sbom.cdx.json  --include vulns,licenses --format spdx     > enriched.spdx.json
```

## Background / current state
The whole pipeline this command needs already exists in pieces:

- **Parsing (input → `sbom.Inventory`)** — `sbom.ParseCycloneDX` / `sbom.ParseSPDX` (added for
  `convert`). The **raw inventory envelope** produced by `scan`
  (`{schema_version, metadata, components, vulnerabilities}`) is an embedded `sbom.Inventory`
  and unmarshals straight into one.
- **Enrichment (components → layers)** — `pkg/scanpipeline`'s **unexported** `enrich(ctx,
  client, *sbom.Inventory, Set)` already builds `[]scanoss.Component` from `inv.Components` and
  gathers the purl-layers over them through `client.DecorationPipeline(...)`, attaching
  licenses/crypto/geo inline and vulnerabilities as the flat top-level list — origin-agnostic
  (detected + declared alike). It just isn't reachable from outside the package.
- **Format-capability warnings** — `cmd/scan.go` already owns `formatLayers`,
  `unsupportedLayers`, `effectiveLayers`, and `reportSkippedLayers`: the exact "this format
  can't render this layer, so it's skipped" logic the constraints ask us to reuse.
- **Rendering (`Inventory` → output)** — `renderInventory` (raw envelope) / `sbom.Generate`
  (cyclonedx/spdx), written through `pkg/output`.

So the genuinely new code is small: a thin command, one refactor to **export** the enrichment
step, and a small input-identification helper (cyclonedx / spdx / raw inventory).

## Command surface
```
scanoss enrich <input> --include <layers> [--format <target>] [-o <file>]
                       [--api-url <url>] [--api-key <key>] [--ignore-cert-errors]
```
- `<input>` — a scanoss **raw inventory** (the `scan` raw output), a CycloneDX JSON, or an SPDX
  JSON. Input format is **detected from content**, no `--from` flag (same approach as `convert`).
  A verbatim v3 scan result is **not** an accepted input for now (see Out of scope).
- `--include` — the layers to gather: `vulns`, `licenses`, `crypto`, `geo` (comma-separated).
  `deps` is **not** supported here (it needs a source tree, not an inventory) — see FR-005.
- `-f, --format` — output format: `raw`, `spdx`, or `cyclonedx`. **Defaults to the input's
  format** (raw→raw, cyclonedx→cyclonedx, spdx→spdx), so enrich round-trips the shape unless you
  ask to convert at the same time.
- `-o, --output` — output file (default: stdout).
- **Online** — it calls the decoration API, so it takes the same auth flags as `scan`
  (`--api-url`, `--api-key`, `--ignore-cert-errors`) and enforces `checkAuth`.

## How it works — the same neutral hub, plus one API round-trip
```
<input> ─identify─▶ parse ─▶ sbom.Inventory ─enrich(layers)─▶ sbom.Inventory' ─render(format)─▶ <output>
          content            (ParseCycloneDX/         (DecorationPipeline,        (renderInventory /
          marker             ParseSPDX /                purl-keyed)                 sbom.Generate)
                             raw→Inventory)
```
1. **Identify + parse** the input into an `Inventory` (reusing the `sbom` readers; raw =
   `json.Unmarshal` into `Inventory`).
2. **Enrich** in place with the requested purl-layers via the exported
   `scanpipeline.Enrich(ctx, client, &inv, layers)` — the same code path the `scan` command uses.
3. **Render** the enriched `Inventory` in the output format (default = input format), through
   the same writer as `scan`/`convert`.

## Input format identification
Content-based, no `--from` flag — three input classes with a **1:1** default output format:

| Input | Detected by | Parsed via | Default output |
|---|---|---|---|
| `cyclonedx` | `"bomFormat": "CycloneDX"` | `sbom.ParseCycloneDX` | `cyclonedx` |
| `spdx` | `"spdxVersion": "SPDX-…"` | `sbom.ParseSPDX` | `spdx` |
| `raw` (inventory) | neither of the above (the `scan` raw envelope) | `json.Unmarshal` → `sbom.Inventory` | `raw` |

The raw case needs no positive marker: after ruling out cyclonedx/spdx, the bytes are unmarshalled
into an `sbom.Inventory`. This also **rejects a verbatim v3 scan result for free** — its
`components` is an object keyed by `url_hash`, not the inventory's array, so it fails to unmarshal
and errors cleanly. Unrecognized/unparseable input → clear error, no output (as `convert`).

## Re-runnability (weekly refresh)
Enrichment is **keyed by PURL (+ version)** and **replaces** the requested layers: per-component
`licenses`/`cryptography`/`geoprovenance` are overwritten for the requested layer, and
`vulnerabilities` is rebuilt as the flat top-level list. Layers **not** requested are left
untouched. So `enrich inv.json --include vulns` run weekly on the same file refreshes vulns
against the current advisory data without disturbing anything else — the command is effectively
idempotent for a fixed component set.

## Format-capability warnings (reused, not reinvented)
Exactly as `scan`: the requested layers are narrowed to what the **output** format can render,
and each dropped layer is reported up front on stderr — `Skipping <layer>: not supported by the
<format> format`. `spdx` skips `vulns`/`crypto`/`geo`; `cyclonedx` skips `crypto`/`geo`; `raw`
keeps everything. This uses the **same** `formatLayers` / `unsupportedLayers` /
`effectiveLayers` / `reportSkippedLayers` code already in `cmd/`.

## User scenarios & acceptance
1. **Given** a raw inventory (`scan` output), **when** `enrich inv.json --include
   vulns,licenses,crypto`, **then** a raw inventory is written with those three layers gathered
   over its components; other data is preserved.
2. **Given** an SPDX document, **when** `enrich sbom.spdx.json --include vulns,licenses`, **then**
   the output is SPDX (default = input format); `vulns` is **skipped** with an up-front warning
   (SPDX can't represent it) and `licenses` is gathered.
3. **Given** a CycloneDX document, **when** `enrich sbom.cdx.json --include vulns,licenses
   --format spdx`, **then** an SPDX document is written; `vulns` is skipped with a warning; enrich
   and convert-to-spdx happen in one pass.
4. **Given** `--include deps`, **then** the command errors up front — deps is not a valid enrich
   layer (dependency analysis needs a manifest/source tree, not a components list) — and no output
   is written.
5. **Given** a decoration service that errors, **then** a warning is logged and a **partial**
   enriched inventory is still written (non-fatal, as in the scan pipeline).
6. **Given** an unrecognized/malformed input, **then** the command errors clearly without writing
   output.
7. **Given** no `--api-key` on the default endpoint, **then** `checkAuth` fails up front (same as
   `scan`).
8. **Re-run:** running the same `enrich … --include vulns` twice on the same file yields the same
   layer set the second time as the first (refresh, not accumulate).

## Requirements
### Functional
- **FR-001 (command)** Add a top-level, **online** `enrich <input> --include <layers>
  [--format <target>] [-o <file>]` command with the scan auth flags (`--api-url`, `--api-key`,
  `--ignore-cert-errors`) and `checkAuth`. `--format` defaults to the detected input format and
  is validated to `raw|spdx|cyclonedx`.
- **FR-002 (identification + parse)** Identify the input from content markers and parse into an
  `Inventory`, recognizing three cases: cyclonedx (`bomFormat`), spdx (`spdxVersion`), else raw
  inventory (`json.Unmarshal` → `sbom.Inventory`). Unrecognized/unparseable → clear error, no
  output. Reuses the `sbom` readers; no `--from` flag.
- **FR-003 (enrichment, exported)** Export the scan pipeline's enrichment step as
  `scanpipeline.Enrich(ctx, *scanoss.Client, *sbom.Inventory, Set)` and have **both** the
  `scan` path (`assemble`) and the new command call it. It maps `inv.Components` →
  `[]scanoss.Component` (purl + version), gathers the requested purl-layers through
  `client.DecorationPipeline(...)`, and attaches them in place. No behavior change on the scan
  path (pure extract-and-rename).
- **FR-004 (purl-layers only)** `enrich` gathers only the purl-keyed layers `vulns`, `licenses`,
  `crypto`, `geo`. Requested layers refresh (replace) their data; unrequested layers are left
  untouched (re-runnable).
- **FR-005 (deps rejected)** `deps` is not a valid enrich layer: dependency analysis needs a
  manifest/source tree and cannot be derived from a components list. `--include deps` errors up
  front (no output). Only the purl-keyed layers `vulns`/`licenses`/`crypto`/`geo` are accepted.
- **FR-006 (format-capability warnings, reused)** Narrow the requested layers to what the output
  format can render and report each skipped layer up front, using the **existing**
  `formatLayers`/`unsupportedLayers`/`effectiveLayers`/`reportSkippedLayers` — no second copy.
- **FR-007 (default format = input)** With no `--format`, render in the detected input format
  (raw→raw, cyclonedx→cyclonedx, spdx→spdx); `--format` overrides to convert in the same pass.
- **FR-008 (render, reused)** Render via `renderInventory` / `sbom.Generate` and write through
  `pkg/output` — the same rendering/writing `scan` and `convert` use.
- **FR-009 (non-fatal enrichment)** A failed decoration service is logged and skipped; a partial
  inventory is still rendered (same contract as `scanpipeline`).

### Non-functional
- **NFR-001 (no duplicated logic)** Reuse the format-capability warnings, the input
  identification/parse, the client/progress wiring, and the rendering already in `cmd/` and
  `pkg/`. New code is the command glue, the `Enrich` export, and the envelope-aware identify case.
- **NFR-002 (purity preserved)** `pkg/sbom` stays free of the scan SDK; the
  `sbom.Component`→`scanoss.Component` mapping stays in `pkg/scanpipeline` (where the SDK already
  lives). No new dependencies.
- **NFR-003** `make check` clean; `go test ./... -race` clean.

## Open decisions
1. **"Extract components" placement (the user's question).** The `sbom.Inventory` →
   `[]scanoss.Component` mapping cannot live on `sbom.Inventory` itself, because `pkg/sbom` must
   not import the scan SDK (its package doc guarantees this). **Recommendation:** keep the
   mapping inside the exported `scanpipeline.Enrich` (it already does exactly this today);
   optionally add a *neutral* `sbom.Inventory` helper later if a non-SDK consumer needs the raw
   `(purl, version)` pairs. Enrich itself needs no new `sbom` method. _Confirm._
2. **Accept a verbatim v3 scan result as enrich input?** _Resolved: no, for now._ `raw` **is** the
   inventory envelope (the `scan` raw output). A verbatim v3 scan result is out of scope for v1
   (it errors on parse); it can be added later via `scansource.FromScanResult` if a real need
   appears. `convert` is left untouched.

## Out of scope
- The `deps` layer / any source-tree work (FR-005) — enrich is inventory-only.
- Fingerprinting, scanning, WFP handling.
- New output/input formats beyond raw/cyclonedx/spdx (JSON only), or fidelity beyond the current
  `Inventory` model (same bar as `convert`).
- A **verbatim v3 scan result** as input (Open decision 2) — only the raw inventory envelope and
  the two SBOM formats are accepted for now.
- Caching or diffing successive weekly runs — enrich just refreshes; comparison is a consumer's job.
