# Tasks: one filter source, one filtering pass

**Spec:** [`spec.md`](./spec.md) · **Plan:** [`plan.md`](./plan.md) · **Ticket:** [#25](https://github.com/scanoss/scanoss.go/issues/25)

## Conventions
- Conventional Commits; atomic; short subjects; no AI/co-author trailers.
- Every code task ships with unit tests.
- `make check` clean before presenting each commit.
- **Every task must leave the collected file set unchanged** unless it is the
  task that deliberately fixes it (T007). That is the constraint, not a wish.

## Phase 0 — A net before touching anything

- [x] **T001** A characterisation test that records what each operation collects
      today, so any later drift is visible rather than argued about.
      Fixture: a tree with `venv/x.go`, `examples/go.mod`, `examples/main.go`,
      `dist/package.json`, `node_modules/y.js`, `a.png`, `main.go`, `go.mod`,
      `pom.xml`, `Gemfile`, `README`, `Makefile`, a `.gitignore` and a
      `scanoss.json`.
      Assert the exact file set for: `scan`/`wfp` collection, dependency
      collection (via the current `collectFilesRecursively`), and extraction
      (the rules composed the way an external consumer composes them).
      This test must pass **unchanged** through T002–T006.

## Phase 1 — One source of rules

- [x] **T002** `pkg/filter/defaults.go`: split the directory list into
      `CommonSkippedDirs`, `ScanOnlySkippedDirs` and `DependencyOnlySkippedDirs`,
      `StdDefaults` composes the first two; `DefaultSkippedDirs` is removed.
      Add `.whl` to `DefaultSkippedExts` — the one extension `wfp.filteredExt`
      has and the collection does not — so T005 removes nothing that was being
      skipped.
      Names, extensions and endings stay as single shared lists: the only
      per-operation difference is the manifests, already handled by
      `PreserveDependencyManifests` (see the plan).
      Tests: the scanning set equals its current 14 entries, asserted against a
      hardcoded list rather than recomposed from the same lists the code uses;
      the three lists are disjoint.

- [x] **T003** `pkg/filter/collect.go`: `FingerprintOptions()`,
      `DependencyOptions()` and `DependencyDefaults()`. The dependency profile
      sets `GitIgnore: false`, `PreserveDependencyManifests: true` and its own
      directory list.
      Tests: each profile's fields, including that only the directory list may
      differ between them. (depends on T002)

- [x] **T004 [P]** `pkg/settings/settings.go`: `DependencyFilter()`, the missing
      sibling of `ScanFilter`/`FingerprintFilter`. `filterFor` already handles
      `OperationDependencies`; only the exported wrapper is absent.
      Tests: reads `skip.patterns.dependencies` and **not** the scanning or
      fingerprinting sections; nil-safe.

## Phase 2 — One filtering pass

Ordering matters: T005 removes the duplicate, T006 gives the one caller that
relied on it its own filter. Landing T005 without T006 changes `libscanoss`.

- [x] **T005** Delete `filteredExt` and `ShouldSkipFile` from
      `pkg/fingerprint/wfp`, and the call at `pkg/scanner/worker.go:90`. Both
      call sites must go: leaving the one inside `GenerateFingerprint` keeps the
      worker filtering by proxy.
      The hidden-file check in the worker stays — out of scope, no drift.
      Tests: a `.png` handed to `GenerateWFP` is fingerprinted; T001 still passes
      (the CLI paths collect the same set, because the collection already
      excluded those files).

- [x] **T006** `libscanoss/core/libscanoss.go`: apply
      the rules it composes itself before fingerprinting, in both
      `GenerateWFP` and `GenerateWFPJSON`. It is the only caller that reaches
      `GenerateFingerprint` without collecting, so this preserves its behaviour.
      Tests: a `.png` path returns empty, as it does today. (depends on T005)

## Phase 3 — Every layer states its profile

- [x] **T007** `cmd/dependencies.go`: replace `collectFilesRecursively` (38
      lines of `filepath.Walk` with an embedded directory list) with
      `filter.Collect` using `DependencyOptions()` and
      `settings.Resolve` + `DependencyFilter()`.
      This is the task that deliberately changes behaviour, in two ways only:
      `scanoss.json`'s `skip.patterns.dependencies` starts being honoured, and
      whatever it drops is now counted.
      Tests: the manifest set matches T001's recorded baseline; a
      `skip.patterns.dependencies` rule is honoured; `.gitignore` is **not**
      applied. (depends on T003, T004)

- [x] **T008 [P]** `cmd/scan.go` and `cmd/wfp.go`: build their options from
      `ScanOptions()` / `FingerprintOptions()`, overriding only what
      comes from flags, instead of assembling the literal by hand.
      Tests: both still collect T001's baseline set. (depends on T003)

## Phase 3b — One profile per stage, not per entry point

`scan --include deps` and `dependencies ./proj` disagree today: the first
inherits the scanning profile for its manifest collection
(`scanpipeline.go:147`), the second uses its own walk. So the same product
answers differently depending on which door you came through — `examples/` is
searched by one and not the other.

The pipeline already runs two collections; what is missing is that the second
one uses the profile of its **stage** rather than the profile of the command.

- [x] **T012** `pkg/scanpipeline`: build the dependency collection from
      `filter.DependencyOptions()` instead of inheriting `opts.Filter`, carrying
      over only what comes from the user (size bounds, `--default-filters`,
      `--gitignore`) and the dependencies section of `scanoss.json`.
      `filter.IngestOptions` is then dead and is removed: two dependency
      profiles were only ever an artefact of the two entry points.

      Deliberate behaviour change: `scan --include deps` starts finding
      manifests under `examples/`, matching what the standalone command already
      does. That is the point — one answer per question, not per door.

      Tests: both entry points return the same manifest set for the same tree,
      asserted against each other rather than against a literal, so they cannot
      drift apart again. (depends on T003, T004)

## Phase 4 — Docs & verification

- [x] **T009 [P]** `CHANGELOG.md`: **Fixed** — `--default-filters=false` now
      restores extension-skipped files, and the filtered count includes what the
      lower layers used to drop silently; `dependencies` now honours
      `scanoss.json`. **Changed, SDK** — `GenerateWFP`/`GenerateFingerprint` no
      longer filter, so a caller passing a list that did not come from collection
      gets every file; `wfp.ShouldSkipFile` is gone, use
      `filter.NewMatcher`. This is the only silent change in the release and must
      read as such.

- [x] **T010** Verification: T001 passes unchanged for scanning and
      fingerprinting; the dependency baseline matches except for the two
      deliberate additions; `--default-filters=false` keeps `a.png`;
      `grep -rn "ShouldSkipFile\|filteredExt"` returns nothing outside the
      CHANGELOG. `make check` and `go test -race ./...` clean.

## Phase 5 — Draw the boundary

`NewMatcher` existed for callers that cannot walk a tree. It applied entry-level
rules (extension, name, ending, size) and silently ignored directory rules,
because those need a traversal it does not have: given
`node_modules/left-pad/index.js` the info describes index.js, so nothing
matches and the answer is "keep".

The first attempt completed it, walking rel's ancestors. It worked, and it was
the wrong call: extracting an archive is not scanoss.go's problem, and guessing
how a caller's input arrives is how a package ends up with an API that is half
right for everyone.

- [x] **T011** Remove `NewMatcher`. Export what a caller actually needs — the
      skip lists, `DefaultSource`/`SizeSource`/`UnscannableSource`/`Build`, and
      `manifests.Is` for the manifest exemption — so it composes the rules and
      applies them with its own traversal.

      `libscanoss` becomes the first such consumer: it fingerprints single files
      and now composes `Build(DefaultSource(StdDefaults()))` explicitly.

      Tests: the characterisation test composes the same way an external
      consumer would, which is also how it verifies the exported pieces are
      sufficient — if something is missing, that test cannot be written.

## Phase 6 — The last duplicated rule

Found while sizing an `--all-hidden` flag: the hidden-entry check was written in
four places — twice inline in `Collect`, once in `pkg/scanner/worker.go`, and
again in an external SDK consumer that composes the rules itself. The same shape
as the extension list, in the line next to the one this SDD had already cleaned.

- [x] **T013** `pkg/filter`: make it a source (`HiddenSource`, `hiddenMatcher`)
      with `Options.IncludeHidden` to switch it off, drop the two inline checks
      in `Collect` and the one in the worker, and add `--all-hidden` to `scan`
      and `wfp`.

      `.git` moves to `CommonSkippedDirs`: with the rule switchable it would
      otherwise be walked, and on a working checkout it is usually larger than
      the project and holds compressed objects nothing can match. This is a
      deliberate widening of the shared list, so the two directory-set tests
      were updated with the reason.

      It also closes a hole in the reported count: hidden *directories* were
      pruned before the counter ran, so they never appeared in "Filtered N
      files" while hidden files did.

      Tests: the scanning and dependency sets, updated for `.git`; the worker
      fingerprints a dotfile handed to it directly (the policy moved out);
      end-to-end, `--all-hidden` collects `.env` and `.config/tool.go` but never
      `.git`.

## Follow-ups (not this change)
- `dist`/`build`/`target` are preserved as-is in `DependencyOnlySkippedDirs`.
  Whether a `dist/package.json` is a declaration or build output deserves its
  own decision, with evidence.
- The hidden-file check is written in three places and is not configurable in
  any of them; an opt-out flag would be the natural fix.
- `filter.Options.Settings` still has to be filled by the caller, because
  `pkg/filter` cannot import `pkg/settings`. A helper that pairs an `Operation`
  with the right settings section would remove the last hand-wiring.
