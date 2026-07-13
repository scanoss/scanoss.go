# Feature Specification: Scan input filtering (defaults + scanoss.json)

**Feature branch:** `feat/inspector-scan-filtering`
**Status:** Draft
**Tracking issue:** scanoss/scanoss#17
**SDD Change:** `inspector-scan-filtering`

## Summary
Add a reusable, standalone file-filtering package to scanoss that decides
which files a scan should process. Filters are loaded from three sources —
built-in **defaults**, a project's **`scanoss.json`**, and the tree's
**`.gitignore`** — merged into one deduplicated rule set, and applied in a single
pass over the source tree. The defaults live inside scanoss (canonical Go
source) and always apply; they are overridden only when an SDK consumer
explicitly supplies custom filters. Skipped files are simply excluded from the
scan: they are not scanned, the list of them is **not** tracked or returned, and
they do not appear in the scan output. At most a count of skipped files is
surfaced (for feedback / a trustworthy file count).

This mirrors scanoss.py's `file_filters.py` and SBOM-Workbench's
`defaultBannedList`, so a Go scan produces a comparable file set.

## User Scenarios & Testing

### Primary user story
As a developer scanning a project (via the CLI or the `pkg/filter` SDK), I want
build dirs, vendored deps, generated files, and oversized/irrelevant files
excluded automatically — honoring my `scanoss.json` and `.gitignore` — so my
results are clean and I can trust the file count. The excluded files are just
dropped from the scan; I don't need a list of them back, only (at most) how many
were skipped.

### Acceptance scenarios
1. **Given** a tree containing `node_modules/`, `__pycache__/`, `vendor/`, images
   and docs, **when** I scan it with defaults, **then** those files/dirs are
   excluded from the scan (absent from the scanned set); no list of them is
   returned.
2. **Given** a `scanoss.json` at the project root with `settings.skip.patterns`
   and `settings.skip.sizes`, **when** I scan, **then** files matching those
   patterns/sizes are excluded from the scan.
3. **Given** no `--settings` flag, **when** I scan a folder that contains a
   `scanoss.json` at its root, **then** it is auto-detected and applied.
4. **Given** a `--settings <path>` flag, **when** I scan, **then** that file is
   used instead of auto-detection; if no flag is given the default is the root of
   the scanned project.
5. **Given** a `.gitignore` in the tree, **when** I scan, **then** its patterns
   are honored and ignored files are excluded from the scan.
6. **Given** a configured maximum file size, **when** I scan, **then** files above
   it are excluded from the scan.
7. **Given** a rule that appears in more than one source (e.g. `.png` in defaults
   and in `scanoss.json`), **when** filters are built, **then** the duplicate is
   collapsed to a single rule (a file is excluded once, not multiple times).
8. **Given** any scan, **when** it completes, **then** the scan output contains
   only the scan results, no list of filtered files is returned anywhere, and the
   file count reflects the filtered (scanned) set, not the raw tree.
9. **Given** the SDK, **when** a caller overrides or extends the default skip
   lists / sizes / toggles, **then** the scan honors the caller's configuration.

### Edge cases
- No `scanoss.json` present → scan proceeds with defaults + `.gitignore` only.
- Empty tree, or every file filtered → the scanned set is empty (and the skipped
  count equals the tree size); this is not an error.
- A pattern present in multiple sources → applied once (dedup); a file is
  excluded once, not multiple times.
- Symlinked directories → not followed (avoid cycles/escape); documented behavior.
- `scanoss.json` present but with no `settings` section → only `bom` is read (as
  today) and defaults + `.gitignore` apply.
- Defaults explicitly disabled by the SDK/CLI → only `scanoss.json` + `.gitignore`
  (and size limits) apply.

## Requirements

### Functional
- **FR-001** The system MUST provide a single exported call that returns the set
  of files to scan, honoring defaults + `scanoss.json` + `.gitignore` + size
  limits. It MAY also return a count of skipped files, but MUST NOT return the
  list of skipped files themselves.
- **FR-002** The system MUST ship canonical default skip lists — directories,
  directory-name suffixes, exact filenames, and file extensions — as exported Go
  values, ported from scanoss.py `file_filters.py` (cross-checked against
  SBOM-Workbench `defaultBannedList`).
- **FR-003** The default skip lists MUST always apply on their own; an SDK caller
  MAY override or extend them (and the min/max size and source toggles) by
  explicitly supplying custom filters. Absent custom filters, the built-in
  defaults are used unchanged.
- **FR-004** The system MUST read `scanoss.json` `settings.skip.patterns`
  (gitignore-style globs, keyed by operation: `scanning`/`fingerprinting`).
- **FR-005** The system MUST read `scanoss.json` `settings.skip.sizes`
  (per-pattern `{patterns, min, max}`, keyed by operation) and apply them.
- **FR-006** The system MUST read `scanoss.json` `settings.folders`
  (`include`/`exclude`) and apply them.
- **FR-007** The system MUST honor `.gitignore` patterns found in the tree.
- **FR-008** The system MUST support a configurable maximum file size, and MUST
  preserve the existing minimum-size behavior (default 100 bytes).
- **FR-009** The system MUST auto-detect `scanoss.json` at the root of the scanned
  project when no explicit path is supplied; an explicit path MUST take priority;
  the default location is the root of the project being scanned.
- **FR-010** The system MUST merge rules from all sources and remove duplicates
  before applying them, then apply them in a single traversal of the tree.
- **FR-011** Filtered files MUST NOT be scanned, MUST NOT appear in the scan
  output, and the list of them MUST NOT be tracked or returned to any caller. The
  scan output MUST contain only the scan results. At most a **count** of skipped
  files MAY be surfaced (e.g. an stderr summary or a numeric field).
- **FR-012** The reported file count MUST reflect the filtered (scanned) set, not
  the raw extracted tree.
- **FR-013** Existing scans without a `scanoss.json` MUST continue to work, with
  improved defaults.

### Non-functional
- **NFR-001** The `pkg/filter` package MUST NOT depend on scanoss-internal
  packages (`internal/*`, other `pkg/*`), so it is consumable standalone.
- **NFR-002** Filter evaluation SHOULD be a single tree traversal; rule build
  (load + dedupe) happens once before traversal.
- **NFR-003** Behavior SHOULD match scanoss.py for the same input tree +
  `scanoss.json` (parity), validated by test.
- **NFR-004** The architecture MUST stay simple: a small matcher interface, small
  single-concern leaf matchers, per-source loaders, and one composite aggregator
  (composite pattern applied idiomatically to Go) — no deep layering.

## Out of scope
- High-file-hashing (HFH / folder-hashing) skip-list variants (leave a seam only).
- Tracking, returning, or reporting the list of skipped files anywhere (output or
  SDK). Skipped files are just excluded; only a count may be surfaced.
- Wiring the downstream consumer's scanner to this package (a separate story, after release).
- De-duplicating the fingerprinters' own `ShouldSkipFile` extension lists
  (follow-up).

## Key entities
- **Matcher** — an atomic filter rule (the leaf) and, via the composite, the
  aggregate; decides whether a path is skipped (boolean).
- **Source** — an origin of rules (defaults, `scanoss.json`, `.gitignore`) that
  produces matchers.
- **Composite** — the deduplicated set of matchers, itself a matcher; evaluated in
  one pass.
- **CollectResult** — the files to scan, plus (at most) a count of skipped files.
  No per-file list of skipped files is kept.
