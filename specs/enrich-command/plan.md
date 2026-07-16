# Implementation Plan: online inventory enrichment (`scanoss enrich`)

**Spec:** `./spec.md` · **Issue:** [#9](https://github.com/scanoss/scanoss.go/issues/9)

## Approach

Enrich = **parse → `sbom.Inventory` → `scanpipeline.Enrich` → render**. Every stage but the
middle one already exists; the middle one exists too, just unexported. So the work is two
small pieces:

1. **Export** `pkg/scanpipeline`'s `enrich` as `Enrich`, and route the scan path through it
   (pure refactor, no behavior change).
2. A thin **`enrich` command** that identifies + parses the input (cyclonedx / spdx / raw
   inventory — reusing the `sbom` readers), wires auth + progress (like `scan`), reuses the
   format-capability warnings, calls `Enrich`, and renders in the input's format by default.

`convert` is left untouched. `pkg/sbom` stays pure; the `sbom.Component`→`scanoss.Component`
mapping stays in `pkg/scanpipeline`, exactly where it is today.

## Touch points

### `pkg/scanpipeline/scanpipeline.go` — export the enrichment step
- Rename the unexported `enrich(ctx, client, *sbom.Inventory, Set)` → exported
  `Enrich(ctx, *scanoss.Client, *sbom.Inventory, Set)`; keep the body verbatim (build
  `[]scanoss.Component` from `inv.Components`, run `DecorationPipeline`, attach layers in place,
  non-fatal per service).
- Update the one internal caller (`assemble`) to call `Enrich`. No other change; existing
  scanpipeline tests must stay green (behavior-preserving).
- Doc comment: note it is the reusable purl-keyed enrichment step shared by `scan` and `enrich`.

### `cmd/enrich.go` (new) — the command
- `enrich <input> --include <layers> [--format <target>] [-o <file>]` + `--api-url`/`--api-key`/
  `--ignore-cert-errors`; top-level (`rootCmd.AddCommand`).
- A small local `identifyAndParse(data) (sbom.Inventory, format string, error)` (three classes,
  reusing the `sbom` readers; no changes to `convert`):
  - cyclonedx (`bomFormat == "CycloneDX"`) → `sbom.ParseCycloneDX`, format `cyclonedx`
  - spdx (`spdxVersion != ""`) → `sbom.ParseSPDX`, format `spdx`
  - else → `json.Unmarshal` into `sbom.Inventory`, format `raw` (a v3 scan result — `components`
    an object, not an array — fails here and errors; not accepted for now)
- Flow in `runEnrich`:
  1. `checkAuth(cmd)`.
  2. Read the file; `identifyAndParse` → `(inv, inputFormat)`.
  3. `layers := scanLayers(cmd)` (reuse). If `layers.Has(LayerDeps)`: **error** — deps is not a
     valid enrich layer (can't be derived from a components list); no output.
  4. `outputFormat` = `--format` if set, else `inputFormat`; `validateOutputFormat`-style check
     to `raw|spdx|cyclonedx`.
  5. `reportSkippedLayers(outputFormat, layers)` + `layers = effectiveLayers(outputFormat,
     layers)` (reuse, unchanged).
  6. `prog := &scanProgress{}`; `client := buildScanClient(cmd, prog)` (reuse — gives the
     per-layer progress bars via `WithProgress`).
  7. `scanpipeline.Enrich(ctx, client, &inv, layers)`; `prog.finish()`.
  8. `emitInventory(cmd, inv, args[0])` (reuse — renders + writes, prints `Results written to`).
- Empty-args → `cmd.Help()`, matching the other commands.

## Reuse (the constraint: don't reinvent)
- **Format-capability warnings:** `formatLayers`, `unsupportedLayers`, `effectiveLayers`,
  `reportSkippedLayers` (cmd/scan.go) — used as-is.
- **Layer parsing:** `scanLayers` / `scanpipeline.ParseLayers`.
- **Client + progress:** `buildScanClient` + `scanProgress` (the `purls`-unit bars already cover
  the decoration layers).
- **Enrichment:** `scanpipeline.Enrich` (newly exported, same code the scan uses).
- **Parsers:** `sbom.ParseCycloneDX` / `sbom.ParseSPDX` (unchanged; raw is a plain unmarshal).
- **Render + write:** `emitInventory` / `renderInventory` / `sbom.Generate` / `pkg/output`.
- **Auth/ctx/errors:** `checkAuth`, `createCancellableContext`, `renderAPIError`.

## Testing
- **`pkg/scanpipeline`:** existing tests cover the (now exported) `Enrich`; add one direct test
  that `Enrich` decorates an inventory built by hand (not from a scan) — proves it works from the
  enrich entry point, not only via `assemble`.
- **`cmd/enrich_test.go`:** with a stub/mock decoration client —
  - `identifyAndParse` table (cyclonedx / spdx / raw inventory; a v3 scan result and garbage both
    error);
  - raw-in/raw-out default format;
  - spdx-in/spdx-out with `vulns` skipped (warning asserted);
  - cyclonedx-in `--format spdx` (enrich + convert in one pass);
  - `--include deps` warns and is ignored;
  - unrecognized input errors, no output;
  - missing key on default endpoint fails `checkAuth`.
- `make check` + `go test ./... -race` clean.

## Documentation (required before done)
- **`CLIENT_HELP.md`** — new `Enrich (`enrich`)` section: usage, the input formats + content
  identification, `--include` (purl-layers only; deps not supported), default-format-follows-input,
  the reused format-capability skip warning, the online/auth note, and the weekly re-run use case.
  Add a table-of-contents link.
- **`README.md`** — an `enrich <input>` row in the **Commands** table + a one-line example
  (e.g. the weekly `enrich inv.json --include vulns`).
- **`CHANGELOG.md`** — an **Added** entry under `## [Unreleased]` for the `enrich` command
  (user-facing).

## Risks
- **Behavior drift on the scan path** from the `enrich`→`Enrich` rename. Mitigation: it is a pure
  extract-and-rename with the same callers; scanpipeline's existing tests guard it.
- **Version-less PURLs** in some SBOM inputs weaken the decoration join (purl+version). This is
  inherent to the input, not a regression; enrich gathers what the API returns for the base PURL,
  same as the scan path.
