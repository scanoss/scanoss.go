# Feature Specification: split scan vs dependency filters

> **Issue:** [#59](https://github.com/scanoss/scanoss.go/issues/59) ·
> **Branch:** `feat/split-scan-dependency-filters`

## Summary

Let a caller reuse the SDK's default file-filter but **preserve dependency
manifests** (`package.json`, `go.mod`, `pom.xml`, …) that the default rules would
otherwise drop. Today the scan filter and the manifest set are entangled by
extension — `DefaultSkippedExts` includes `.json`, `.xml`, `.mod`, `.sum`,
`.toml`, `.gradle`, `.pom`, `.lock` — so any consumer that applies the default
filter to a tree it will later parse for dependencies loses the manifests.

Introduce a single, self-describing knob — `filter.Options.PreserveDependencyManifests`
— plus a per-entry matcher constructor (`filter.NewMatcher`) so streaming callers
(archive extractors) can filter exactly the way `Collect` does. The manifest
truth lives in one place, a new leaf package `pkg/manifests`, shared by both
`pkg/filter` and `pkg/dependencies`. **Default false → behavior unchanged.**

## Motivation

- The extension is ambiguous: `.json` matches both `package.json` (a manifest)
  and `data.json` (not). There is no subset of the extension lists that equals
  "the manifests" — they can only be identified by exact base name, which the
  dependency parser already does (`DependencyParser.IsSupportedFile`).
- Consumers (e.g. an inspector's archive extractor) want to prune scan-junk to
  disk while keeping the manifests their dependency stage reads. Without a keep
  hook they must fall back to directory-only pruning.
- The manifest name list is currently implicit in `dependencies.parserMap` keys;
  a filter that needs to recognise manifests must not duplicate it (drift).

## Decisions

- **New leaf package `pkg/manifests`** holds the canonical manifest names/patterns
  as constants + `Is(path) bool`. It imports nothing internal, so both
  `pkg/filter` and `pkg/dependencies` can depend on it without a cycle.
- **`pkg/dependencies` sources its `parserMap` keys from `pkg/manifests`** (the
  constants), and a drift-guard test asserts `SupportedFiles()` == `manifests.Patterns`.
- **`filter.Options.PreserveDependencyManifests bool`** — a keep/allowlist that
  overrides skips **for files only** (directories are matched unchanged, so
  skipped dirs like `node_modules` are still not descended, and manifests nested
  inside them stay excluded). It overrides all base rules, including size.
- **`filter.NewMatcher(o Options) Matcher`** — a per-path skip matcher built from
  the same `Options` used by `Collect`, for callers that evaluate entries one at a
  time (streaming) instead of walking a tree. It applies defaults + `Settings`
  and honours `PreserveDependencyManifests`; it does **not** apply `.gitignore`
  (that needs the whole tree — use `Collect`).
- **`ScanOptions()` / `IngestOptions()`** name the two common configurations:
  scan (skip manifests) vs ingest (keep manifests).
- `Collect` is refactored to apply the same keep logic (via an internal
  `keepMatcher`), so walk-based and per-entry callers behave identically.

## Requirements

- **REQ-1** With `PreserveDependencyManifests` false, `Collect`/`NewMatcher`
  behave exactly as before (no manifest is kept that a rule skips).
- **REQ-2** With it true, a file whose base name is a known manifest is retained
  even if a default/settings rule (extension, name, ending, size) would skip it.
- **REQ-3** The keep applies to files only; skipped directories are still not
  descended.
- **REQ-4** `NewMatcher(o).Match(rel, info)` yields the same file decision as
  `Collect` for the same `o` (excluding `.gitignore`).
- **REQ-5** `manifests.Patterns` and the dependency parser's supported files
  cannot drift (guarded by a test).

## Out of scope

- `.gitignore` support in `NewMatcher` (needs a tree; `Collect` owns it).
- Any change to what the scan itself fingerprints — manifests remain skipped for
  scanning; this only lets *other* stages keep them.
