# Implementation Plan: configurable minimum file size, defaulting to none

**Spec:** [`spec.md`](./spec.md) · **Tasks:** [`tasks.md`](./tasks.md) · **Ticket:** [#23](https://github.com/scanoss/scanoss.go/issues/23)

## Technical context

The threshold is not in one place. It is in **two**, applied at different stages,
and only the first one is visible in the filter package:

1. `pkg/filter/defaults.go:40` — `DefaultMinFileSize int64 = 100`, folded into
   `StdDefaults()` (`sources.go:56`) and turned into a `sizeMatcher` during
   collection.
2. `internal/config/config.go:41` — `MinFileSize = 100`, applied *again* inside the
   fingerprint worker: `pkg/scanner/worker.go:84` skips any file where
   `stat.Size() < config.MinFileSize`.

Changing only (1) would produce a flag that lies: files between the new threshold
and 100 bytes would survive collection, be counted as collected, and then vanish
silently in the worker with no "filtered" tally. **Both must change together** —
this is the main risk in the change and the reason T001 and T002 are separate,
ordered commits rather than one.

Supporting facts:
- `Options.MinSize` already exists (`collect.go:51`) with the sentinel
  `if o.MinSize != 0 { d.MinSize = o.MinSize }` (`collect.go:150`). Once the default
  is 0 the sentinel becomes a no-op — both branches yield the same value — so it can
  stay as-is and keeps `MaxSize`'s shape.
- `DefaultSource` only appends a `sizeMatcher` when `d.MinSize > 0 || d.MaxSize > 0`
  (`sources.go:96`). With both at 0 the matcher disappears entirely, which satisfies
  NFR-2 for free.
- `scan` already reads `--max-size` at `cmd/scan.go:371` and passes it through
  `filter.Options` into `scanpipeline.Run`. `--min-size` follows the identical path.
- `wfp` has no filter flags at all and calls `scanner.CollectFiles(targetPath)`
  (`cmd/wfp.go:92`), which hardcodes `filter.DefaultOptions()`. It needs the
  options-taking sibling that already exists, `CollectFilesWithOptions`.
- `libscanoss/core/libscanoss.go:79,105` also calls `CollectFiles`. It inherits the
  new default with no signature change — intended, and worth a CHANGELOG mention.

## Design overview

### 1. One default, set to zero

```go
// pkg/filter/defaults.go
// DefaultMinFileSize is the minimum file size (bytes) to scan. 0 means no
// minimum: every file is collected unless another rule skips it.
const DefaultMinFileSize int64 = 0
```

The existing comment justifies 100 as a historical value; it goes with the constant.

### 2. One enforcement point

`pkg/scanner/worker.go` drops its size clause:

```go
-		if stat.Name()[0] == '.' || stat.Size() < config.MinFileSize {
+		if stat.Name()[0] == '.' {
```

and `config.MinFileSize` is deleted. Collection owns size policy; the worker's job
is fingerprinting. The hidden-file check stays — it is cheap, has no configurable
counterpart, and removing it is a separate question.

This is a behaviour change for anyone driving `scanner.WorkerPool` directly with an
unfiltered list. That is an SDK-internal path with no CLI equivalent, and the fix
for such a caller is to collect through `pkg/filter` first, which is what every
in-tree caller already does.

### 3. The flag

`--min-size`, on both commands, mirroring `--max-size`:

```go
scanCmd.Flags().Int64("min-size", 0, "Minimum file size in bytes to scan (0 = no minimum)")
wfpCmd.Flags().Int64("min-size", 0, "Minimum file size in bytes to scan (0 = no minimum)")
wfpCmd.Flags().Int64("max-size", 0, "Maximum file size in bytes to scan (0 = unlimited)")
```

`wfp` gains `--max-size` too: it would be odd for the fingerprint-only command to
accept a floor but not a ceiling, and it costs one line.

The name follows the flag it pairs with. `--min-size`/`--max-size` with `0` meaning
"no bound" on both sides is self-documenting, and reuses a convention the CLI
already ships rather than introducing a second vocabulary for the same idea.

### 4. Validation

Both commands validate before doing any work, in a shared helper beside the other
`cmd` helpers:

```go
// cmd/helpers.go
func validateSizeBounds(min, max int64) error
```

- `min < 0` or `max < 0` → error naming the flag.
- `max > 0 && min > max` → error; a run that can collect nothing is a mistake, not a
  configuration.

`0` is never an error on either side.

### 5. Wiring

`scan` adds one line to the existing `filter.Options` literal (`cmd/scan.go:396`):

```go
Filter: filter.Options{
    MinSize:   minSize,
    MaxSize:   maxSize,
    ...
```

`wfp` swaps `scanner.CollectFiles(targetPath)` for
`scanner.CollectFilesWithOptions(targetPath, o)` built from
`filter.ScanOptions()` plus the two bounds. `CollectFiles` stays for compatibility.

### 6. Closing the `wfp` gap (T009)

The two size bounds alone leave `wfp` filtering differently from `scan` whenever
`--default-filters`, `--gitignore` or a `scanoss.json` is in play, so T009 gives it
the remaining inputs and drops the `filter.ScanOptions()` base for the same
`Options` literal `cmd/scan.go` builds.

One asymmetry to get right: `scan` passes `scanSettings.ScanFilter()`, but
`scanoss.json` distinguishes `skip.patterns.scanning` from
`skip.patterns.fingerprinting`, and `wfp` is the fingerprinting path.
`filterFor(operation)` already handles both; only the `ScanFilter()` wrapper is
exported, so `pkg/settings` needs a `FingerprintFilter()` sibling.

## Key changes
- `pkg/filter/defaults.go` — `DefaultMinFileSize` 100 → 0, comment corrected.
- `pkg/filter/collect.go` — doc comment on `Options.MinSize` (`0` = no minimum).
- `internal/config/config.go` — `MinFileSize` deleted.
- `pkg/scanner/worker.go` — size clause dropped from the worker.
- `cmd/scan.go` — `--min-size` flag, validation, one field in `filter.Options`.
- `cmd/wfp.go` — `--min-size`/`--max-size`, validation, options-based collection;
  then `--default-filters`/`--gitignore`/`--settings` (T009).
- `pkg/settings/settings.go` — `FingerprintFilter()` beside `ScanFilter()` (T009).
- `cmd/helpers.go` — `validateSizeBounds`.
- `CLIENT_HELP.md`, `CHANGELOG.md`.

## Testing strategy
- `pkg/filter`: `StdDefaults().MinSize == 0`; a 40-byte file survives
  `Collect` with `DefaultOptions()`; `Options{MinSize: 100}` drops it and counts it
  in `SkippedCount`; with both bounds 0, `DefaultSource` produces no `size:` matcher
  (assert on `Key()`, which is what NFR-2 actually claims).
- `pkg/scanner`: a 40-byte file submitted to `WorkerPool` yields a fingerprint —
  the regression test for the second floor, and the one that would have caught this
  class of bug.
- `cmd`: `--min-size` reaches `filter.Options` on both commands; the three
  validation failures (negative min, negative max, min > max) each error before
  collection; `--min-size 0` is accepted.
- End to end on a fixture tree carrying a scoped `scanoss.json` rule
  (`skip.sizes.scanning` for `**/*.ts`) alongside files inside and outside that
  pattern — see T008 for the exact expectations. The load-bearing assertion is that
  a small file *outside* the rule's patterns survives a default run: it fails today
  because the built-in floor overrides the project's scoping, and passes after T001.
  The same run through `wfp` produces a WFP with the same `file=` set as
  `scan --save-wfp`.

## Commit conventions
Conventional Commits, atomic, short subjects, no AI/co-author trailers.
`CHANGELOG.md` in the product-changing commits.

## Risks / trade-offs

**Scans get bigger.** Removing the floor adds roughly 46 % more files on a measured
12 000-entry tree, which means more fingerprinting, a larger WFP upload, and more
server-side matching. That is the point — those files were being dropped without
being reported — but it is a visible performance and result-count change on the next
release, and belongs in the CHANGELOG as such rather than buried as a fix.

**More noise in results.** Very small files match a lot of things spuriously. Users
who liked the old behaviour get `--min-size 100`, and the CHANGELOG should say so
explicitly rather than leaving them to find the flag.

**Deleting `config.MinFileSize` is a breaking change for SDK consumers** who
imported it. It lives in `internal/`, so it is unimportable outside this module —
this is safe by construction, which is worth stating so the next reader does not
re-litigate it.
