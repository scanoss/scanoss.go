# Feature Specification: configurable minimum file size, defaulting to none

**Feature branch:** `feat/min-file-size`
**Status:** Draft
**SDD Change:** `min-file-size`
**Ticket:** [#23](https://github.com/scanoss/scanoss.go/issues/23)

## Summary
Stop skipping small files by default, and let the threshold be set per run:

```console
$ scanoss-cli scan .                     # no minimum — every file is fingerprinted
$ scanoss-cli scan . --min-size 100      # opt back into the old 100-byte floor
```

Today the CLI drops every file under 100 bytes. The threshold is hardcoded, absent
from `--help` and from the docs, and there is no flag to change it — so a user who
notices files missing from a scan has no way to act on it.

### Impact
Measured on a 12 000-entry source tree with default flags: **2 405 files** are
dropped by the size floor alone, out of 5 181 collected — roughly 46 % more files
would be scanned without it. Almost all of them are small source files, none of
them empty.

Two properties make this worse than a plain default:

- **It is silent in the wrong place.** The floor is applied twice (see the plan),
  and the second application drops files after they have already been counted as
  collected, so the "Filtered N files" line under-reports.
- **It is asymmetric.** `--max-size` already exists and already treats `0` as "no
  bound". There is no reason the lower bound should be both fixed and invisible
  when the upper one is neither.

## User Scenarios & Testing

### Primary user story
As a user scanning a project, I want every file in my tree to be fingerprinted
unless I say otherwise, and I want to raise the floor myself when small files are
noise rather than signal.

### Acceptance scenarios
1. **Given** a project containing a 40-byte source file, **when** I run `scan` with
   no size flags, **then** that file is fingerprinted and appears in the WFP.
2. **Given** the same project, **when** I run `scan --min-size 100`, **then** the
   file is skipped and counted in the "Filtered N files" line.
3. **Given** `--min-size 100 --max-size 1048576`, **when** I scan, **then** only
   files within `[100, 1048576]` are collected.
4. **When** I run `wfp --min-size 100`, **then** the same threshold applies to the
   fingerprint-only path.
4a. **Given** any tree and any combination of collection flags, **when** I run
   `wfp` and `scan --save-wfp` with the same flags, **then** both emit the same
   set of `file=` entries.
5. **When** I pass a negative `--min-size`, **then** the run fails with a message
   naming the flag, before any file is read.
6. **When** `--min-size` exceeds a non-zero `--max-size`, **then** the run fails
   rather than silently collecting nothing.
7. **Given** a `scanoss.json` with a `skip.sizes` rule, **when** I also pass
   `--min-size`, **then** both apply — the rule is scoped to its patterns, the flag
   is global.
8. **Given** a `skip.sizes` rule scoped to one pattern (say `**/*.ts`), **when** a
   small file *outside* that pattern is collected with no `--min-size`, **then** it
   is kept. A project that scopes a minimum to one file type has decided the bound
   does not apply elsewhere, and nothing built-in may override that.
9. **Given** a tree holding an empty file and a symbolic link, **when** I scan it
   with any combination of flags, **then** neither is fingerprinted and both are
   counted as filtered.

### Edge cases
- Zero-byte files are **never** collected, whatever the bounds say. An empty file
  has no content to match, so fingerprinting it produces a WFP entry with a zero
  hash and no lines — bytes uploaded that no scan can act on. Measured on a
  12 000-entry tree: 65 such entries. This is not a bound a caller can set to 0;
  it is the absence of content, so it is not configurable.
- Symbolic links are **never** collected either, for the same reason stated
  differently: a link has no content of its own. Its target is collected on its
  own when it is inside the tree, so following the link would report the same
  bytes twice under two names; when the target is outside the tree, or broken, it
  is not this scan's to report. Symlinked *directories* are already not descended
  into, and stay that way.
- `--min-size 0` is indistinguishable from omitting the flag. That is intended: `0`
  means "no minimum", the same convention `--max-size 0` already uses for
  "unlimited".
- A single-file target (`scan ./file.go`) bypasses collection entirely today and is
  unaffected by either size flag. Unchanged by this work.

## Requirements
- **FR-1** The built-in minimum file size defaults to **0** (no minimum), replacing
  the current 100.
- **FR-2** `scan` and `wfp` accept `--min-size <bytes>`, symmetric with the existing
  `--max-size`, where `0` disables the bound.
- **FR-3** The threshold is enforced **once**, at collection. No later stage may
  re-apply a size floor of its own.
- **FR-4** `--min-size` is rejected when negative, or when it exceeds a non-zero
  `--max-size`. Validation happens before collection.
- **FR-5** `filter.Options.MinSize` keeps working for SDK callers, with `0` now
  meaning "no minimum" rather than "use the built-in 100".
- **FR-6** The count reported by `OnCollect` ("Filtered N files") continues to
  include files dropped by the size bound.
- **FR-7** `wfp` collects files exactly the way `scan` does: it accepts the same
  `--default-filters`, `--gitignore` and `--settings` flags, and honours
  `scanoss.json`. A command whose purpose is to show what would be fingerprinted
  must not filter differently from the command that fingerprints.
- **FR-8** The size bounds keep a built-in default *and* an override, and the two
  are separate concerns:
  - the default is `DefaultMinFileSize`/`DefaultMaxFileSize`, carried by
    `DefaultOptions`/`ScanOptions`/`IngestOptions` so an SDK caller that starts
    from a constructor gets a documented value, and by the CLI flag defaults so
    there is one source of truth rather than a repeated literal;
  - the override is `Options.MinSize`/`MaxSize` (the `--min-size`/`--max-size`
    flags), and it is applied as its own source, so `--default-filters=false`
    does not discard it.
- **FR-9** Entries there is no point fingerprinting — zero-byte files and
  symbolic links — are never collected, on every path (`Collect` and
  `NewMatcher`), independently of the defaults, the size bounds and anything else
  a caller can switch off. They are not configurable: excluding them states a
  fact about the entry rather than a policy about the scan.
- **NFR-1** No new dependencies. No change to `scanoss.json`'s format — `skip.sizes`
  keeps its current meaning and composes with the flag.
- **NFR-2** A scan with no size bounds must not get slower: with both bounds at 0 no
  size matcher is built at all.

## Out of scope
- The other built-in skip lists — skipped directory names, directory suffixes,
  skipped extensions and file-name endings. Each is a separate decision with its own
  evidence.
- Making `.gitignore` honour nested files rather than only the tree root.
- Storing the threshold in `~/.scanoss/settings.json`. It describes a project, not a
  machine, so `scanoss.json` is the right home if it ever needs to persist.
