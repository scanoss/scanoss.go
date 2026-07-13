# Implementation Plan: apply bom.remove inside the scanner

**Spec:** `./spec.md` · **Issue:** [#43](https://github.com/scanoss/scanoss.go/issues/43)
**Status:** Draft

## Approach
Move the `bom.remove` matching engine from `pkg/postprocess` into `pkg/scanoss`, expose a
single per-call `WithBOM(*settings.BOM)` scan option carrying the whole BOM, and apply the
relevant part in `scanService.scan()` — the single site all three entry points
(`Folder`/`Files`/`WFP`) funnel through. For #43 that means running `bom.remove` (with
`bom.include` precedence) **post-scan** on the result. The SDK owns the stage routing, so
future BOM behaviors (pre-scan `include`/`identify` server context, post-scan `replace`)
extend the *same* option — no `WithBOMRemove`/`WithBOMInclude` proliferation. The CLI
passes `WithBOM(&cfg.BOM)` and deletes its explicit call. `pkg/postprocess` becomes a thin
deprecated wrapper so any external callers keep working. Filters stay on `WithFilters`
(input stage) — untouched.

The move is required by the import graph: `scanoss` may import `settings` (no cycle) but
must not import `postprocess` (would cycle), so the engine — which needs both `ScanResult`
and `settings.BOM` — has to live in `scanoss`.

## Mechanics

### 1. Move the engine into `pkg/scanoss` (`pkg/scanoss/bomremove.go`)
Move the `ScanResult`-typed engine and its helpers out of `pkg/postprocess/bomremove.go`
into a new `pkg/scanoss/bomremove.go` (same package as `ScanResult`, so no `scanoss.`
qualifier and no import of `scanoss`):
- `ApplyBOMRemove(result *ScanResult, bom *settings.BOM)` (exported; was
  `postprocess.ApplyBomRemoveResult`).
- unexported helpers `filePurls`, `shouldRemove`, `stripVersion`, `matchPath`.
- `pkg/scanoss` gains `import "…/pkg/settings"` (no cycle — `settings` doesn't import
  `scanoss`).

### 2. `postprocess` becomes a deprecated wrapper (`pkg/postprocess/bomremove.go`)
- `ApplyBomRemoveResult(result *scanoss.ScanResult, bom *settings.BOM)` stays as a
  one-line delegating wrapper: `scanoss.ApplyBOMRemove(result, bom)`, marked
  `// Deprecated: use scanoss.ApplyBOMRemove …`.
- The unused raw-JSON `ApplyBomRemove(string, *settings.BOM) (string, error)` variant
  (old scan-result format, not called in non-test code) is **deleted** with its helpers
  (`buildRemovedEntry`, `resultEntry`, `removedEntry`, `ScanResults`) and tests — dead
  code for a format we no longer emit. (T4 confirms nothing else references it.)

### 3. Scan option + apply site (`pkg/scanoss/scan.go`)
- Add `bom *settings.BOM` to `scanOptions` and one option carrying the whole BOM:
  ```go
  // WithBOM applies the scan's bill-of-materials rules. Today: bom.remove (with
  // bom.include precedence) post-scan on the result. The SDK routes each BOM field to its
  // stage, so future behaviors (pre-scan include/identify context, post-scan replace)
  // extend this same option. nil is a no-op.
  func WithBOM(bom *settings.BOM) ScanOption {
      return func(o *scanOptions) { o.bom = bom }
  }
  ```
- Apply the **post-scan** part once, after `Wait`, in `scan()`:
  ```go
  env, err := s.Wait(ctx, scanID)
  if err != nil {
      return ScanEnvelope{}, err
  }
  if o.bom != nil && env.Result != nil {
      ApplyBOMRemove(env.Result, o.bom)   // remove + include-precedence (today)
  }
  return env, nil
  ```
  `ApplyBOMRemove` already no-ops on nil/empty rules (FR-003), so the guard just skips a
  nil result. A **pre-scan** hook (before upload) for `include`/`identify` server context
  is a future extension reading the same `o.bom` — not implemented here (out of scope).

### 4. CLI rewire (`cmd/scan.go`)
- Pass the BOM when settings define one, and delete the explicit post-scan call:
  ```go
  var scanOpts []scanoss.ScanOption
  scanOpts = append(scanOpts, scanoss.WithChunkBytes(chunkSize))
  if scanSettings != nil && scanSettings.HasBOM() {
      scanOpts = append(scanOpts, scanoss.WithBOM(&scanSettings.BOM))
  }
  res, err := client.Scan.WFP(ctx, wfp, scanOpts...)
  ```
- Remove the `postprocess.ApplyBomRemoveResult(res.Result, …)` block and the now-unused
  `pkg/postprocess` import.

## Tests
- **Move** the `ApplyBomRemoveResult` tests to `pkg/scanoss/bomremove_test.go`, renamed to
  cover `ApplyBOMRemove` (neutralization, `file_hash` kept, map pruning, `bom.include`
  precedence, multi-match). Delete the raw-JSON `ApplyBomRemove` tests along with that
  variant.
- **SDK** (`pkg/scanoss/scan_test.go`): a scan with `WithBOM` (rules that remove) returns a
  filtered result; without the option the result is unchanged (idempotent no-op).
- **CLI** (`cmd/scan_test.go`): unchanged expectations — output still matches today (FR-004).

## Verification
- `go build ./... && go vet ./... && gofmt -l . && go test ./... -race` clean.
- Manual: a project with a `scanoss.json` `bom.remove` rule → the matched files come back
  `match_type:"none"` and the orphaned components are gone; the same scan without removes
  is byte-identical to before.

## Risks
- **Import cycle.** Keep the engine in `scanoss`; `postprocess` may depend on `scanoss`
  (one direction), never the reverse. Verified by `go build`.
- **External users of `postprocess`.** Mitigated by keeping the deprecated wrapper rather
  than deleting `ApplyBomRemoveResult` outright.

## Open questions
- None blocking. Resume-path filtering (`results <id>`) is deferred (spec Decision 2).
