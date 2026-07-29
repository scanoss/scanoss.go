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

- [x] **T001** `pkg/filter/defaults.go`: `DefaultMinFileSize` 100 → 0, and replace
      the comment that justifies the old value with one describing the new meaning
      (`0` = no minimum). Update the `Options.MinSize` doc comment in `collect.go`
      to match.
      Tests: `StdDefaults().MinSize == 0`; a 40-byte file survives `Collect` with
      `DefaultOptions()`; `Options{MinSize: 100}` still drops it and increments
      `SkippedCount`; with both bounds 0, `DefaultSource` emits no `size:` matcher.

- [x] **T002** `pkg/scanner/worker.go`: drop `stat.Size() < config.MinFileSize` from
      the worker loop and delete `MinFileSize` from `internal/config/config.go`.
      Size policy belongs to collection; the hidden-file check stays.
      Tests: a 40-byte file submitted to `WorkerPool` produces a fingerprint.
      (depends on T001)

## Phase 2 — The flag

- [x] **T003** `cmd/helpers.go`: `validateSizeBounds(min, max int64) error` —
      rejects a negative bound, and `min > max` when `max` is non-zero; `0` is
      always valid on either side.
      Tests: table over the valid and the three invalid combinations.

- [x] **T004** `cmd/scan.go`: add
      `--min-size` (`Int64`, default 0, "Minimum file size in bytes to scan
      (0 = no minimum)"), call `validateSizeBounds` before building the pipeline,
      and set `MinSize` in the existing `filter.Options` literal.
      Tests: the flag reaches `filter.Options`; each invalid combination errors
      before collection; `--min-size 0` is accepted. (depends on T003)

- [x] **T005** `cmd/wfp.go`: add `--min-size` and `--max-size` with the same help
      text, validate them, and collect via
      `scanner.CollectFilesWithOptions(targetPath, o)` — `filter.ScanOptions()` plus
      the two bounds — instead of `scanner.CollectFiles`. Leave `CollectFiles` in
      place for its other callers.
      Tests: both flags reach the collection options; invalid combinations error.
      (depends on T003)

## Phase 3 — Docs & verification

- [x] **T006 [P]** `CLIENT_HELP.md`: add `--min-size` to the `scan` flag list
      (line ~132) and to `wfp`'s; in "Skipping files", state that there is no
      minimum by default and show `--min-size 100` as the way to restore the old
      behaviour. Note that the flag is global while `scanoss.json`'s `skip.sizes` is
      scoped to its patterns, and that the two compose.

- [x] **T007 [P]** `CHANGELOG.md` under `## [Unreleased]`: a **Changed** entry, not
      a Fixed one — say plainly that files under 100 bytes were previously skipped
      and now are not, that scans will therefore be larger, and that `--min-size
      100` restores the old behaviour. Plus an **Added** entry for the flag on
      `scan` and `wfp`.

- [x] **T008** Verification on a fixture tree that exercises both the global bound
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

## Phase 4 — Decouple the bounds from `--default-filters`

Runs **before T006**, so the docs never describe a limitation that is about to
disappear.

- [x] **T010** `pkg/filter`: give the size bounds their own source instead of
      housing them in `Defaults`.

      Today `Options.MinSize`/`MaxSize` are folded into the defaults struct by
      `o.defaults()` (`collect.go:150-155`), and that struct is only consulted
      under `if o.Defaults` — so `--default-filters=false` silently discards both
      bounds. Verified: `scan --min-size 100` reports `Filtered 1 files`, while
      `scan --min-size 100 --default-filters=false` filters nothing.

      The justification died with T001. `DefaultMinFileSize` and
      `DefaultMaxFileSize` are now both 0, so `StdDefaults()` contributes no size
      bound at all; a size matcher exists only because a user passed a flag, and
      `Defaults` is merely a pass-through for that input.

      Keep a default **and** an override — they are separate concerns, and the
      bug was conflating them:
      - Remove `MinSize`/`MaxSize` from `Defaults`, `StdDefaults` and
        `o.defaults()`; drop the size branch from `DefaultSource`.
      - Add `SizeSource(min, max int64) []Matcher`, a sibling of `DefaultSource` /
        `SettingsSource` / `GitIgnoreSource`, and append it in **both** `Collect`
        and `NewMatcher` (`collect.go:120-128`, the streaming-extraction path) —
        otherwise the two entry points disagree.
      - Carry the default in `DefaultOptions`/`ScanOptions`/`IngestOptions`
        (`MinSize: DefaultMinFileSize`, `MaxSize: DefaultMaxFileSize`) so an SDK
        caller starting from a constructor gets a documented value instead of an
        implicit zero — without this the two constants have no remaining use.
      - Have the CLI flag defaults read the same constants rather than repeating
        `0`, so there is one source of truth.

      Tests: `Options{MinSize: 100, Defaults: false}` drops a 40-byte file (the
      case that fails today); the bounds still apply with `Defaults: true`;
      `NewMatcher` honours them with `Defaults` both on and off; each constructor
      carries `DefaultMinFileSize`/`DefaultMaxFileSize`.
      `CHANGELOG.md`: a **Fixed** line for the flags, and a **Changed** line for
      the removed `filter.Defaults` fields — `pkg/filter` is public, so this
      breaks any external caller that set them.

## Phase 5 — Close the `wfp` gap

T005 leaves `wfp` half-configurable: it honours the two size bounds but still
cannot turn off the built-in filters or `.gitignore`, and ignores `scanoss.json`
entirely. A `wfp` run and a `scan` run over the same tree can therefore disagree
on which files they cover, which defeats the command's stated purpose —
debugging, and generating WFPs for offline processing.

- [x] **T009** `cmd/wfp.go`: add the remaining collection flags, so `wfp` filters
      exactly the way `scan` does.
      - `--default-filters` (default true) and `--gitignore` (default true), same
        spellings, defaults and help text as `scan`.
      - `--settings <path>`, resolved with `settings.Resolve(settingsFlag,
        targetPath)` as `scan` does, feeding `filter.Options.Settings`.
      - Use the **fingerprinting** operation, not scanning: `scanoss.json`
        separates `skip.patterns.fingerprinting` from `skip.patterns.scanning`,
        and `wfp` is the fingerprinting path. `pkg/settings` exports only
        `ScanFilter()` today — add the `FingerprintFilter()` sibling beside it
        (`filterFor` already takes the operation).
      - Replace the `filter.ScanOptions()` base from T005 with an `Options`
        literal built from all five inputs, mirroring `cmd/scan.go`.

      Tests: each flag reaches the collection options; `--default-filters=false`
      keeps a file the defaults would drop; a `scanoss.json`
      `skip.patterns.fingerprinting` rule is honoured by `wfp` and a
      `skip.patterns.scanning` rule is **not** (that is what proves the operation
      is right); `pkg/settings` gains a `FingerprintFilter` unit test.
      Docs: extend the `wfp` flag list in `CLIENT_HELP.md` (T006) and add an
      **Added** line to `CHANGELOG.md`. (depends on T005)

## Follow-ups (not this change)
- The remaining built-in skip lists: skipped directory names, directory suffixes,
  extensions and file-name endings — none of them documented or overridable from the
  CLI beyond the all-or-nothing `--default-filters`.
- `.gitignore` is read only at the tree root; nested `.gitignore` files are ignored.
