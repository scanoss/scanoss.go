# Feature Specification: one filter source, one filtering pass

**Feature branch:** `refactor/unify-filters`
**Status:** Draft
**SDD Change:** `unify-filters`
**Ticket:** [#25](https://github.com/scanoss/scanoss.go/issues/25)

## Summary
Make `pkg/filter` the only place that decides which files an operation processes,
and make that decision happen exactly once, at collection. Every layer below —
the fingerprint worker, the WFP generator, the dependency command — stops
filtering and consumes what it is given.

This is a refactor. **No user-visible filtering result changes.**

## The problem

Four places decide to exclude files today, and only one of them is configurable
and reports what it dropped:

| # | Where | Excludes by | Configurable | Counted | Reads `scanoss.json` |
|---|---|---|---|---|---|
| 1 | `pkg/filter` | dirs, names, extensions, endings, size, `.gitignore`, `scanoss.json` | yes | yes | yes |
| 2 | `pkg/scanner/worker.go:90` | extension | no | no | no |
| 3 | `pkg/fingerprint/wfp.go:138` | extension | no | no | no |
| 4 | `cmd/dependencies.go:343` | its own embedded directory list | no | no | no |

Layers 2 and 3 are the same check applied twice in a row: the worker calls
`ShouldSkipFile`, then calls `GenerateFingerprint`, which calls it again.

Three consequences, all observed:

- **`--default-filters=false` under-delivers.** It restores files skipped by
  name or directory, but not by extension — layer 3 drops those again.
- **`Filtered N files` under-reports.** It counts only layer 1. Whatever layers
  2–4 discard vanishes with no record, so a user cannot tell "was not there"
  from "we dropped it".
- **The copies have drifted.** `wfp.filteredExt` and `filter.DefaultSkippedExts`
  differ by 14 entries; the directory lists by 11. Nothing checks that they
  agree, so the gap widens with every rule added to one of them.

## User Scenarios & Testing

### Primary user story
As a maintainer, I want one place to change when a filtering rule changes, and I
want a flag that says "do not filter" to actually not filter.

### Acceptance scenarios
1. **Given** any tree, **when** I run `scan` or `wfp` with default flags,
   **then** the set of fingerprinted files is byte-for-byte what it is today.
2. **Given** any tree, **when** I run `dependencies`, **then** the manifests
   found are the same ones found today.
3. **Given** `--default-filters=false`, **when** I run `scan` or `wfp`, **then**
   files skipped only by extension (e.g. `a.png`) **are** fingerprinted — today
   they are not.
4. **Given** a file dropped by any rule, **when** the run ends, **then** it is
   included in the `Filtered N files` count.
5. **Given** a `scanoss.json` with `skip.patterns.dependencies`, **when** I run
   `dependencies`, **then** those patterns are honoured — today they are ignored.
6. **Given** the same tree, **when** I compare `wfp` and `scan --save-wfp` with
   the same flags, **then** both cover the same files (already true; must stay
   true).

### Edge cases
- A caller that hands `GenerateWFP` a list which did **not** come from
  collection now gets every file fingerprinted. That is the contract change; see
  FR-7.
- `libscanoss` fingerprints single files without collecting. It keeps its
  current behaviour by applying `filter.NewMatcher` explicitly.
- Symlinked directories are still not descended into. Unchanged.

## Requirements

### Behaviour must not change
- **FR-1** `scan` and `wfp` collect exactly the same files as today, with default
  flags and with any combination of flags that works today.
- **FR-2** `dependencies` finds exactly the same manifests as today. Its
  directory exclusions (`node_modules`, `vendor`, `__pycache__`, `dist`,
  `build`, `target`) are preserved, and it keeps looking inside `venv`, `eggs`,
  `wheels`, `__pypackages__`, `examples` and the rest.
- **FR-3** `filter.DefaultSkippedDirs` keeps its exact current value. It is
  public and read directly by consumers, so it is part of the contract.
- **FR-4** `.gitignore` stays applied to scanning and fingerprinting, and stays
  **not** applied to dependencies. `.gitignore` answers "should this be
  versioned", not "is this a dependency" — a lock file excluded from git still
  declares what the project uses, and losing a declaration is worse than
  analysing one extra.

### Unification
- **FR-5** `pkg/filter` is the only source of filtering rules. No other package
  holds a skip list.
- **FR-6** Filtering happens **once** per file, at collection. No layer below
  re-applies a rule.
- **FR-7** Every layer states which profile it uses, through
  `filter.OptionsFor(op)` rather than assembling an `Options` literal by hand.
- **FR-8** Differences between operations are expressed as named deltas over a
  shared list, so the difference is visible in one place and inherits future
  additions.
- **NFR-1** No new dependencies. No change to `scanoss.json`'s format.

## Out of scope
- Changing which files any operation covers. Any such change is a separate
  decision with its own evidence — including `dist`/`build`/`target`, which stay
  exactly as they are.
- The hidden-file check, which is duplicated in `Collect` and in the worker but
  is not configurable anywhere and has no drift.
- Making `.gitignore` honour nested files rather than only the tree root.
