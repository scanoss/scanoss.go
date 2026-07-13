# Tasks: split scan vs dependency filters

Each task = one atomic commit; `go build`/`go vet`/`go test` green at every step.
Conventional Commit subjects, ≤~50 chars. CHANGELOG updated on the
product-changing commits. No AI/assistant trailers.

## Task 0 — SDD plan + changelog  (docs)
- [x] Add `specs/split-scan-dependency-filters/{spec,plan,tasks}.md`.
- [x] `CHANGELOG.md` `[Unreleased]`: record the new `pkg/manifests`,
      `filter.Options.PreserveDependencyManifests`, `filter.NewMatcher`,
      `filter.ScanOptions`/`IngestOptions`, and the dependencies single-source refactor.
- *Commit:* `docs: add split-scan-dependency-filters SDD plan`

## Task 1 — Shared manifest source of truth  (refactor)
- [x] Add `pkg/manifests` (constants, `Patterns`, `Is`).
- [x] `pkg/dependencies/parser.go`: key `parserMap` off `manifests.*` constants.
- [x] `pkg/dependencies`: drift-guard test `TestManifestsMatchParsers`
      (`SupportedFiles()` == `manifests.Patterns`).
- [x] `pkg/manifests`: `Is` unit test (manifests vs `data.json`/`config.xml`/glob).
- *Commit:* `refactor(dependencies): source manifest names from shared pkg/manifests`

## Task 2 — Preserve manifests in the filter  (feat)
- [x] `filter.Options.PreserveDependencyManifests bool` (files-only keep override).
- [x] `keepMatcher` + refactor `Collect` to use it (both dir and file branches);
      `absPath` helper.
- [x] `filter.NewMatcher(Options)` — per-entry matcher (defaults + settings, no
      gitignore), honoring the keep.
- [x] `filter.ScanOptions()` / `filter.IngestOptions()`.
- [x] Tests: flag keeps manifests over ext/size, drops non-manifest `.json`, and
      does not resurrect manifests inside skipped dirs; `Scan` vs `Ingest` split.
- [x] `CHANGELOG.md` `[Unreleased]` entries (see Task 0).
- [x] Verify: `make check`.
- *Commit:* `feat(filter): preserve dependency manifests via PreserveDependencyManifests`

## Notes
- Backward-compatible: default `PreserveDependencyManifests` false → unchanged.
- `.gitignore` intentionally not applied by `NewMatcher` (streaming has no tree).
- Consumer (SCANOSS inspector) uses `filter.NewMatcher(filter.IngestOptions())`
  at archive extraction to prune scan-junk while keeping manifests.
