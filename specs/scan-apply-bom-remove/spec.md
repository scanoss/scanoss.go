# Feature Specification: apply bom.remove inside the scanner

**Feature branch:** `feat/scan-result-restructure` (this branch) · **Issue:** [#43](https://github.com/scanoss/scanoss.go/issues/43)
**Status:** Draft

## Summary
Today the CLI applies `bom.remove` rules **explicitly** after a scan
(`cmd/scan.go` calls `postprocess.ApplyBomRemoveResult`). Move that post-processing
**inside the SDK scan flow** so a configured scan applies its own `bom.remove` rules and
returns an already-filtered result — callers no longer wire it up by hand.

The rules are supplied via a single per-call scan option, **`WithBOM(*settings.BOM)`**,
that carries the whole BOM block. The **SDK** decides which part of the BOM applies at
which stage, so future BOM behaviors don't add new options:

- **post-scan (on the result):** `bom.remove` (with `bom.include` as precedence) — this is
  what #43 implements. `bom.replace` later.
- **pre-scan (server context):** `bom.include` / `bom.identify` — a documented future
  extension, still out of scope for #43.

The CLI then just passes `WithBOM(&cfg.BOM)` and drops the explicit post-processing call.
File-collection filters are a **separate, input-stage** concern and stay on `WithFilters`
— they are not part of the BOM/scan-result contract.

## Current state (why this is needed)
- `cmd/scan.go` — explicit, post-scan `postprocess.ApplyBomRemoveResult(res.Result, &scanSettings.BOM)`
  call. Easy to forget; not part of the scan contract.
- `cmd/results.go` — the resume path (`Scan.Wait`) applies **no** `bom.remove`, so
  `scanoss results <id>` returns unfiltered results even when `scanoss.json` defines
  removes. Pre-existing inconsistency.
- `pkg/postprocess/bomremove.go` — holds the matching engine; imports both `pkg/scanoss`
  and `pkg/settings`.
- **Import direction:** `pkg/scanoss` imports neither `settings` nor `postprocess`;
  `settings` does not import `scanoss`. So `scanoss` **may** import `settings` (no cycle)
  but must **not** call `postprocess` (would cycle) — the engine has to move into
  `pkg/scanoss`.
- `pkg/scanoss/scan.go` — `Folder`/`Files`/`WFP` all funnel through `scanService.scan()`.

## Decisions (resolved from the issue's open questions)
1. **Apply site — `scan()`, not `Wait()`.** `scan()` is the smallest change (no interface
   change) and covers `scan` / `scan wfp`. `Wait()` would also cover the `results <id>`
   resume path but changes the `ScanAPI.Wait` signature. → **`scan()`**.
2. **Resume path (`results <id>`) — deferred.** It doesn't filter today; wiring it needs
   `results --settings` + applying at `Wait`. Out of scope here (no regression).
3. **One option carrying the whole BOM — `WithBOM(*settings.BOM)`.** Not a family of
   `WithBOMRemove` / `WithBOMInclude` / `WithBOMReplace`. The caller passes one coherent
   block; the SDK routes each field to its stage (post-scan now, pre-scan later). This
   avoids option proliferation as BOM handling grows. `scanoss` importing `settings`
   introduces no cycle. Filters stay on `WithFilters` (input stage), not folded in.
4. **`postprocess` fate — deprecated delegating wrapper.** Keep `ApplyBomRemoveResult` as a
   thin wrapper over the moved engine (deprecated) unless confirmed unused externally.

## Functional requirements
- **FR-001** A scan configured with `bom.remove` returns a `ScanResult` with matching
  entries neutralized (`match_type:"none"`, `matches` cleared, `file_hash` preserved) and
  components no longer referenced pruned from the catalog.
- **FR-002** `bom.include` precedence is preserved — protected PURLs are not removed.
- **FR-003** No rules / option unset → result unchanged (no-op, idempotent).
- **FR-004** CLI `scan` / `scan wfp` output matches today's, with the explicit
  `postprocess` call removed.
- **FR-005** The public engine lives in `pkg/scanoss` (`ApplyBOMRemove`); `pkg/scanoss`
  gains no dependency on `pkg/postprocess` (no import cycle).

## Out of scope
- Sending BOM *context* (`identify` / `ignore` / `include`) to the v3 server (the "BOM
  context not yet applied" note stays).
- `bom.replace` handling.
- The `results <id>` resume path (see Decision 2).
- Any change to fingerprinting, chunking, upload, or polling.
