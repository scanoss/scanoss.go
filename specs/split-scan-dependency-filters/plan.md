# Implementation Plan: split scan vs dependency filters

## Touch points

- `pkg/manifests/manifests.go` — new leaf package: name/pattern constants,
  `Patterns`, `Is(path) bool`.
- `pkg/dependencies/parser.go` — `parserMap` keys reference `manifests.*`.
- `pkg/dependencies/manifests_sync_test.go` — drift-guard test.
- `pkg/filter/collect.go` — `Options.PreserveDependencyManifests`,
  `ScanOptions()`, `IngestOptions()`, `keepMatcher`, `NewMatcher(Options)`,
  refactor `Collect` to use `keepMatcher`, `absPath` helper.
- `pkg/filter/preserve_manifests_test.go` — flag + `ScanOptions`/`IngestOptions`.
- `pkg/manifests/manifests_test.go` — `Is` coverage.
- `CHANGELOG.md` — `[Unreleased]`.

## `pkg/manifests`

Canonical identifiers as constants (exact base names + the `*.csproj` glob), a
`Patterns` slice, and `Is(path)` matching on the base name (so nested manifests
are recognised), using `filepath.Match` for globs.

## `pkg/dependencies`

Swap the string-literal `parserMap` keys for the `manifests.*` constants (values
— the parser funcs — unchanged). Add `TestManifestsMatchParsers` asserting the
parser's `SupportedFiles()` set equals `manifests.Patterns`, so adding a parser
without registering its manifest (or vice versa) fails CI.

## `pkg/filter`

- `Options.PreserveDependencyManifests bool` — documented as a files-only keep
  override; default false.
- `keepMatcher{base Matcher, preserveManifests bool}` — `Match` returns
  `base.Match(rel, info)` unless `preserveManifests && !info.IsDir() &&
  manifests.Is(rel)`, in which case it keeps (returns false).
- `NewMatcher(o Options) Matcher` — builds the composite from `DefaultSource` +
  `SettingsSource` (no `.gitignore`) and wraps it in `keepMatcher`. For streaming
  callers.
- `Collect` — build `skip := &keepMatcher{base: Build(sources...), preserveManifests: o.PreserveDependencyManifests}`
  (sources still include `.gitignore` when enabled) and use `skip.Match` in both
  the dir and file branches. Factor the abs-path conversion into `absPath`.
- `ScanOptions()` = `{Defaults, GitIgnore}`; `IngestOptions()` = same +
  `PreserveDependencyManifests: true`.

## Verification

- `make check` (fmt + vet + lint + test).
- Existing `filter`/`dependencies` suites unchanged and green (REQ-1).
- New tests cover REQ-2/3/5 and the `Scan`/`Ingest` split.
