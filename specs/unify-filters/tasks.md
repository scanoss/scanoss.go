# Tasks: one filter source, one filtering pass

**Spec:** [`spec.md`](./spec.md) · **Plan:** [`plan.md`](./plan.md) · **Ticket:** [#25](https://github.com/scanoss/scanoss.go/issues/25)

## Conventions
- Conventional Commits; atomic; short subjects; no AI/co-author trailers.
- Every code task ships with unit tests.
- `make check` clean before presenting each commit.
- **Every task must leave the collected file set unchanged** unless it is the
  task that deliberately fixes it (T007). That is the constraint, not a wish.

## Phase 0 — A net before touching anything

- [ ] **T001** A characterisation test that records what each operation collects
      today, so any later drift is visible rather than argued about.
      Fixture: a tree with `venv/x.go`, `examples/go.mod`, `examples/main.go`,
      `dist/package.json`, `node_modules/y.js`, `a.png`, `main.go`, `go.mod`,
      `pom.xml`, `Gemfile`, `README`, `Makefile`, a `.gitignore` and a
      `scanoss.json`.
      Assert the exact file set for: `scan`/`wfp` collection, dependency
      collection (via the current `collectFilesRecursively`), and extraction
      (`filter.NewMatcher` with `PreserveDependencyManifests`).
      This test must pass **unchanged** through T002–T006.

## Phase 1 — One source of rules

- [ ] **T002** `pkg/filter/defaults.go`: split the directory list into
      `CommonSkippedDirs`, `ScanOnlySkippedDirs` and `DependencyOnlySkippedDirs`,
      with `DefaultSkippedDirs` composed from the first two.
      Add `.whl` to `DefaultSkippedExts` — the one extension `wfp.filteredExt`
      has and the collection does not — so T005 removes nothing that was being
      skipped.
      Names, extensions and endings stay as single shared lists: the only
      per-operation difference is the manifests, already handled by
      `PreserveDependencyManifests` (see the plan).
      Tests: `DefaultSkippedDirs` equals its current 14 literals, asserted
      against a hardcoded list — it is public contract, read directly by
      consumers to build their own prune sets; the three lists are disjoint.

- [ ] **T003** `pkg/filter/collect.go`: `Operation`, `OptionsFor(op)` and
      `DependencyDefaults()`. `OpScan`/`OpFingerprint` give today's scanning
      profile; `OpDependencies` gives `GitIgnore: false`,
      `PreserveDependencyManifests: true` and the dependency directory set.
      Tests: each profile's fields; `OptionsFor(OpScan)` collects the same set as
      today's `ScanOptions()`. (depends on T002)

- [ ] **T004 [P]** `pkg/settings/settings.go`: `DependencyFilter()`, the missing
      sibling of `ScanFilter`/`FingerprintFilter`. `filterFor` already handles
      `OperationDependencies`; only the exported wrapper is absent.
      Tests: reads `skip.patterns.dependencies` and **not** the scanning or
      fingerprinting sections; nil-safe.

## Phase 2 — One filtering pass

Ordering matters: T005 removes the duplicate, T006 gives the one caller that
relied on it its own filter. Landing T005 without T006 changes `libscanoss`.

- [ ] **T005** Delete `filteredExt` and `ShouldSkipFile` from
      `pkg/fingerprint/wfp`, and the call at `pkg/scanner/worker.go:90`. Both
      call sites must go: leaving the one inside `GenerateFingerprint` keeps the
      worker filtering by proxy.
      The hidden-file check in the worker stays — out of scope, no drift.
      Tests: a `.png` handed to `GenerateWFP` is fingerprinted; T001 still passes
      (the CLI paths collect the same set, because the collection already
      excluded those files).

- [ ] **T006** `libscanoss/core/libscanoss.go`: apply
      `filter.NewMatcher(filter.ScanOptions())` before fingerprinting, in both
      `GenerateWFP` and `GenerateWFPJSON`. It is the only caller that reaches
      `GenerateFingerprint` without collecting, so this preserves its behaviour.
      Tests: a `.png` path returns empty, as it does today. (depends on T005)

## Phase 3 — Every layer states its profile

- [ ] **T007** `cmd/dependencies.go`: replace `collectFilesRecursively` (38
      lines of `filepath.Walk` with an embedded directory list) with
      `filter.Collect` using `OptionsFor(OpDependencies)` and
      `settings.Resolve` + `DependencyFilter()`.
      This is the task that deliberately changes behaviour, in two ways only:
      `scanoss.json`'s `skip.patterns.dependencies` starts being honoured, and
      whatever it drops is now counted.
      Tests: the manifest set matches T001's recorded baseline; a
      `skip.patterns.dependencies` rule is honoured; `.gitignore` is **not**
      applied. (depends on T003, T004)

- [ ] **T008 [P]** `cmd/scan.go` and `cmd/wfp.go`: build their options from
      `OptionsFor(OpScan)` / `OptionsFor(OpFingerprint)`, overriding only what
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

- [ ] **T012** `pkg/scanpipeline`: build the dependency collection from
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

- [ ] **T009 [P]** `CHANGELOG.md`: **Fixed** — `--default-filters=false` now
      restores extension-skipped files, and the filtered count includes what the
      lower layers used to drop silently; `dependencies` now honours
      `scanoss.json`. **Changed, SDK** — `GenerateWFP`/`GenerateFingerprint` no
      longer filter, so a caller passing a list that did not come from collection
      gets every file; `wfp.ShouldSkipFile` is gone, use
      `filter.NewMatcher`. This is the only silent change in the release and must
      read as such.

- [ ] **T010** Verification: T001 passes unchanged for scanning and
      fingerprinting; the dependency baseline matches except for the two
      deliberate additions; `--default-filters=false` keeps `a.png`;
      `grep -rn "ShouldSkipFile\|filteredExt"` returns nothing outside the
      CHANGELOG. `make check` and `go test -race ./...` clean.

## Phase 5 — Close the other half of the filter

`NewMatcher` is the entry point for callers that cannot walk a tree — archive
extraction, streaming. It applies extension, name and ending rules correctly,
and silently ignores directory rules: those only inspect `info`, and for a file
inside `node_modules/` the `info` is the file. Verified:

    NewMatcher(ScanOptions()).Match("node_modules/left-pad/index.js", fileInfo)
    → false

`Collect` never hits this because it prunes with `SkipDir` while walking, so the
question is never asked. The gap is only visible from outside a walk — which is
why a consumer ends up reimplementing exactly the missing half.

- [ ] **T011** `pkg/filter`: make `NewMatcher` apply directory rules to every
      ancestor segment of `rel`, so it answers the same as `Collect` for the same
      entry.
      - An `ancestorMatcher` wrapper, applied **only** in `NewMatcher` —
        `Collect` already prunes while walking and would just re-evaluate.
      - The manifest exemption must NOT rescue an entry skipped because of an
        ancestor: in `Collect` a `node_modules/foo/package.json` is never
        reached, so the two paths would otherwise disagree. Ancestor check first,
        without the exemption; entry check second, with it.

      Tests: a table of paths (`node_modules/x/index.js`, `venv/lib/x.py`,
      `foo.egg-info/PKG-INFO`, `src/main.go`) matched by both `NewMatcher` and
      `Collect` over an equivalent tree, asserting they agree — that equivalence
      is the requirement, not the wrapper.

## Follow-ups (not this change)
- `dist`/`build`/`target` are preserved as-is in `DependencyOnlySkippedDirs`.
  Neither scanoss.js nor scanoss.py skips them anywhere; whether a
  `dist/package.json` is a declaration or build output deserves its own
  decision, with evidence.
- The hidden-file check is written in three places and is not configurable in
  any of them. scanoss.py exposes `--all-hidden`.
- `filter.Options.Settings` still has to be filled by the caller, because
  `pkg/filter` cannot import `pkg/settings`. A helper that pairs an `Operation`
  with the right settings section would remove the last hand-wiring.
