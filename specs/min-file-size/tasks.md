# Tasks: configurable minimum file size, defaulting to none

**Spec:** [`spec.md`](./spec.md) · **Plan:** [`plan.md`](./plan.md) · **Ticket:** [#23](https://github.com/scanoss/scanoss.go/issues/23)

## Conventions
- Conventional Commits; atomic; short subjects; no AI/co-author trailers.
- Every code task ships with unit tests.
- `make check` clean before presenting each commit.
- `[P]` marks tasks that can be done in parallel with their siblings.

## Phase 1 — Remove the floor

Both tasks must land before the flag exists, and **T002 must not be skipped**: with
T001 alone, files between 0 and 100 bytes pass collection and are then dropped
silently by the worker, with no filtered count. That is worse than today.

- [ ] **T001** `pkg/filter/defaults.go`: `DefaultMinFileSize` 100 → 0, and replace
      the comment that justifies the old value with one describing the new meaning
      (`0` = no minimum). Update the `Options.MinSize` doc comment in `collect.go`
      to match.
      Tests: `StdDefaults().MinSize == 0`; a 40-byte file survives `Collect` with
      `DefaultOptions()`; `Options{MinSize: 100}` still drops it and increments
      `SkippedCount`; with both bounds 0, `DefaultSource` emits no `size:` matcher.

- [ ] **T002** `pkg/scanner/worker.go`: drop `stat.Size() < config.MinFileSize` from
      the worker loop and delete `MinFileSize` from `internal/config/config.go`.
      Size policy belongs to collection; the hidden-file check stays.
      Tests: a 40-byte file submitted to `WorkerPool` produces a fingerprint.
      (depends on T001)

## Phase 2 — The flag

- [ ] **T003** `cmd/helpers.go`: `validateSizeBounds(min, max int64) error` —
      rejects a negative bound, and `min > max` when `max` is non-zero; `0` is
      always valid on either side.
      Tests: table over the valid and the three invalid combinations.

- [ ] **T004** `cmd/scan.go`: add
      `--min-size` (`Int64`, default 0, "Minimum file size in bytes to scan
      (0 = no minimum)"), call `validateSizeBounds` before building the pipeline,
      and set `MinSize` in the existing `filter.Options` literal.
      Tests: the flag reaches `filter.Options`; each invalid combination errors
      before collection; `--min-size 0` is accepted. (depends on T003)

- [ ] **T005** `cmd/wfp.go`: add `--min-size` and `--max-size` with the same help
      text, validate them, and collect via
      `scanner.CollectFilesWithOptions(targetPath, o)` — `filter.ScanOptions()` plus
      the two bounds — instead of `scanner.CollectFiles`. Leave `CollectFiles` in
      place for its other callers.
      Tests: both flags reach the collection options; invalid combinations error.
      (depends on T003)

## Phase 3 — Docs & verification

- [ ] **T006 [P]** `CLIENT_HELP.md`: add `--min-size` to the `scan` flag list
      (line ~132) and to `wfp`'s; in "Skipping files", state that there is no
      minimum by default and show `--min-size 100` as the way to restore the old
      behaviour. Note that the flag is global while `scanoss.json`'s `skip.sizes` is
      scoped to its patterns, and that the two compose.

- [ ] **T007 [P]** `CHANGELOG.md` under `## [Unreleased]`: a **Changed** entry, not
      a Fixed one — say plainly that files under 100 bytes were previously skipped
      and now are not, that scans will therefore be larger, and that `--min-size
      100` restores the old behaviour. Plus an **Added** entry for the flag on
      `scan` and `wfp`.

- [ ] **T008** Verification on a fixture tree that exercises both the global bound
      and a scoped rule. Files: `tiny.ts` (50 B), `ok.ts` (500 B), `huge.ts` (2 MB),
      `tiny.rs` (50 B), and a `scanoss.json` carrying
      `skip.sizes.scanning: [{patterns: ["**/*.ts"], min: 100, max: 1048576}]`.
      - default run → keeps `ok.ts` **and `tiny.rs`** (scenario 8: the scoped rule
        must not reach a file outside its patterns, and nothing built-in may drop
        it either). Today `tiny.rs` is lost to the hardcoded floor — this is the
        assertion that fails before T001 and passes after.
      - `--min-size 100` → keeps `ok.ts` only.
      - `--min-size 100 --max-size 150` → keeps nothing, reports four filtered.

      Confirm `wfp` and `scan --save-wfp` emit the same `file=` set for the same
      flags — that is what proves the floor is gone from both enforcement points.
      `make check` and `go test -race ./...` clean.

## Follow-ups (not this change)
- The remaining built-in skip lists: skipped directory names, directory suffixes,
  extensions and file-name endings — none of them documented or overridable from the
  CLI beyond the all-or-nothing `--default-filters`.
- `.gitignore` is read only at the tree root; nested `.gitignore` files are ignored.
