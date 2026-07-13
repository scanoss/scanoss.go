# Implementation Plan: Scan input filtering (defaults + scanoss.json)

**Spec:** [`spec.md`](./spec.md) · **Tasks:** [`tasks.md`](./tasks.md)
**Tracking issue:** scanoss/scanoss#17

## Technical context
- **Language/module:** Go 1.22, `github.com/scanoss/scanoss.go`.
- **New package:** `pkg/filter` (standalone, no `internal/*` or other `pkg/*` deps).
- **Reused building blocks:**
  - `settings.Load` / `settings.Detect` / `settings.Resolve`
    (`pkg/settings/settings.go:55-126`) — already implement auto-detect-at-root,
    explicit `--settings` path priority, and default-to-scan-folder. Reused as-is.
  - `scanner.CollectFiles` (`pkg/scanner/worker.go:133`) — the directory walk to
    be routed through `pkg/filter`.
  - `output.MergeJSONResults` (`pkg/output/writer.go:55`) — unchanged; the scan
    output stays results-only (the filtered list is not added to it).
  - `config.MinFileSize` (`internal/config/config.go`) — preserved as the default
    minimum (100 bytes).
- **New third-party dependency:** a gitignore-spec matcher (see Component 1).
- **Reference implementations:** scanoss.py `src/scanoss/file_filters.py` and
  `src/scanoss/scanoss_settings.py`; SBOM-Workbench
  `src/main/workspace/filtering/defaultFilter.ts` (`defaultBannedList`).

## Design overview
A `scanoss.json`, the built-in defaults, and a `.gitignore` are each loaded as a
list of small **matchers**, the lists are concatenated and **deduplicated** by a
stable key, and the result is wrapped in one **composite** matcher that is
evaluated once per file during a single `filepath.WalkDir`. This is the composite
pattern applied to Go: leaf matchers are the parts, the composite is the whole and
is itself a matcher. Defaults are exported and always apply; an SDK caller
overrides or extends them only by explicitly supplying custom filters. The walk
returns the files to scan; skipped files are simply dropped (only a count is
kept). No list of skipped files is tracked or returned, so matchers decide a
plain boolean — there is no per-file reason to carry.

```
defaults ─┐
scanoss.json ─┼─► []Matcher  ──Build (concat + dedupe)──►  *Composite ──Collect (one WalkDir)──► CollectResult{Files, SkippedCount}
.gitignore ─┘
```

### Component 1 — Matcher + Composite (`pkg/filter/matcher.go`, new)
```go
// Matcher is the leaf AND the composite (composite pattern: composite is-a Matcher).
type Matcher interface {
    Match(rel string, info os.FileInfo) bool // true = skip this path
    Key() string // stable identity for dedup, e.g. "ext:.png", "dir:vendor", "glob:**/*.min.js"
}

type Composite struct{ matchers []Matcher }
func (c *Composite) Match(rel string, info os.FileInfo) bool // true if ANY matcher skips
func (c *Composite) Key() string { return "composite" }
```
Leaf matchers, one concern each (small structs, each carrying its `Key`):
`extMatcher`, `nameMatcher` (exact filename), `dirMatcher` (dir name or suffix
such as `.egg-info`), `globMatcher` (gitignore-style, wraps the matching lib),
`sizeMatcher` (min/max, optionally pattern-scoped). No `Reason` type — skipped
files are not tracked, so a boolean is sufficient.

**Glob/gitignore matching:** use a gitignore-spec-compatible matcher for parity
with scanoss.py's `GitIgnoreSpec` (negation + anchoring semantics) for both
`.gitignore` files and `scanoss.json` `skip.patterns`. **Recommended:**
`github.com/sabhiram/go-gitignore`. (#17 suggested `github.com/bmatcuk/doublestar`;
doublestar is pure-glob and misses gitignore negation/anchoring — noted as a
fallback only.) Final choice confirmed at implementation.

### Component 2 — Sources (`pkg/filter/sources.go`, new)
Each source turns one origin into matchers:
```go
func DefaultSource(o DefaultOptions) []Matcher          // from the exported default lists
func SettingsSource(skip Skip, folders Folders) []Matcher // scanoss.json skip.patterns/sizes/folders
func GitIgnoreSource(root string) ([]Matcher, error)    // parses .gitignore files in the tree
```
`Skip`/`Folders` here are **plain local structs** (mirrors of the `pkg/settings`
types) so `pkg/filter` stays free of a `pkg/settings` dependency; the caller maps
`settings.*` → `filter.*`. Operation type (`scanning` vs `fingerprinting`) selects
which `skip.patterns`/`skip.sizes` key `SettingsSource` reads (default `scanning`).

### Component 3 — Build + dedupe (`pkg/filter/build.go`, new)
```go
func Build(sources ...[]Matcher) *Composite // concat all, drop duplicate Key()s (first wins), wrap in Composite
```
"Remove the repeated ones": e.g. `.png` in both defaults and a `scanoss.json`
pattern collapses to a single matcher, so a file is excluded once and evaluated
once. Order = defaults → scanoss.json → .gitignore; on a duplicate `Key()` the
later source's copy is dropped.

### Component 4 — Defaults (`pkg/filter/defaults.go`, new)
Canonical, exported defaults ported from `file_filters.py` (cross-checked against
`defaultFilter.ts`):
```go
var DefaultSkippedDirs    = []string{"nbproject","nbbuild","nbdist","__pycache__",
    "venv","_yardoc","eggs","wheels","htmlcov","__pypackages__","example","examples", /*+ node_modules, vendor*/}
var DefaultSkippedDirExts = []string{".egg-info"}
var DefaultSkippedFiles   = []string{"gradlew","gradlew.bat","mvnw","mvnw.cmd",
    "gradle-wrapper.jar","maven-wrapper.jar","thumbs.db","babel.config.js",
    "license.txt","license.md","copying.lib","makefile"}
var DefaultSkippedExts    = []string{ /* full ~150-entry list from file_filters.py; + .mod, .sum from Workbench */ }
const DefaultMinFileSize  = 100 // bytes; preserves config.MinFileSize
const DefaultMaxFileSize  = 0   // 0 = unlimited (matches scanoss.py default)
```
Workbench also bans `.mod`/`.sum` extensions and `node_modules`/`vendor` dirs —
folded into the lists so Go covers Workbench parity too.

### Component 5 — Collect (SDK entry, `pkg/filter/collect.go`, new)
```go
type CollectResult struct {
    Files        []string // absolute paths to scan
    SkippedCount int      // how many files were excluded (no per-file list kept)
}
type Options struct {
    SkipDirs, SkipFiles, SkipExtensions []string // override/extend defaults
    MinSize, MaxSize int64
    UseGitIgnore bool      // default true
    UseDefaults  bool      // default true
    Settings     *Settings // mirrored scanoss.json skip/folders, may be nil
    HFH          bool      // seam, default false
    Operation    string    // "scanning" (default) | "fingerprinting"
}
func Collect(root string, o Options) (*CollectResult, error)
```
`Collect` builds the composite from the enabled sources, then walks `root` with
`filepath.WalkDir`: a directory that matches returns `fs.SkipDir`; a file that
matches is dropped (`SkippedCount++`), otherwise it is appended to `Files`. No
list of skipped files is retained. Paths are returned absolute (current API
contract). Symlinked dirs are not followed.

### Component 6 — `scanoss.json` schema (`pkg/settings/settings.go`, additive)
Today only `bom` is modeled. Add (no breakage):
```go
type Settings struct {
    BOM      BOM    `json:"bom"`
    Settings Tuning `json:"settings,omitempty"` // new
}
type Tuning  struct { Skip Skip `json:"skip,omitempty"`; Folders Folders `json:"folders,omitempty"` }
type Skip    struct {
    Patterns map[string][]string   `json:"patterns,omitempty"` // keys: scanning, fingerprinting
    Sizes    map[string][]SizeRule `json:"sizes,omitempty"`
}
type SizeRule struct { Patterns []string `json:"patterns"`; Min int64 `json:"min"`; Max int64 `json:"max"` }
type Folders  struct { Include []string `json:"include,omitempty"`; Exclude []string `json:"exclude,omitempty"` }
```
`Load`/`Detect`/`Resolve` are reused unchanged for detection/priority/default-root.

### Component 7 — Wire `scanner.CollectFiles` (`pkg/scanner/worker.go`)
- `CollectFiles(root string)` → calls `filter.Collect(root, defaults)` and returns
  `(*filter.CollectResult, error)`; keep a back-compat shim if any caller wants
  just `[]string`.
- Add `CollectFilesWithOptions(root string, o filter.Options) (*filter.CollectResult, error)`
  as the SDK entry.
- The worker's per-file binary/min-size guard (`worker.go:54-104`) stays as a
  safety net. The fingerprinters' `ShouldSkipFile` extension lists are left in
  place (no behavior change); de-duping them against `pkg/filter` is a follow-up.

### Component 8 — Skipped files are dropped (not tracked, not returned)
No list of skipped files is kept anywhere; at most a count is surfaced.
- **SDK:** `filter.Collect` returns `CollectResult{Files, SkippedCount}`.
  `orchestrator.ScanResult` (`pkg/orchestrator/scanner.go:28`) MAY gain a
  `SkippedCount int` (numeric only). No `Filtered` slice on either type.
- **Scan output (all formats — plain/spdxlite/cyclonedx):** unchanged shape; it
  contains only scan results. No envelope, no sidecar, no filtered list.
- **CLI feedback:** only a one-line stderr summary **count**
  (`Filtered N files`) so a user sees filtering happened. The list itself is
  never printed or stored.

### Component 9 — New CLI flags (`cmd/scan.go:165` `init`)
- `--max-size <bytes>` — configurable max (default 0 = unlimited).
- `--default-filters` — apply default skip lists (default `true`; `=false` disables).
- `--gitignore` — honor `.gitignore` (default `true`; `=false` disables).
- (No `--filtered-output`: there is no filtered list to write — skipped files are dropped, only counted.)
- `scanoss.json` detection reuses the existing `--settings` flag.

## File changes
| File | Change |
|---|---|
| `pkg/filter/matcher.go` | new — `Matcher`, leaf matchers, `Composite` |
| `pkg/filter/sources.go` | new — `DefaultSource`/`SettingsSource`/`GitIgnoreSource` |
| `pkg/filter/build.go` | new — `Build(...)`: concat + dedupe by `Key()` |
| `pkg/filter/defaults.go` | new — exported default skip lists + size consts |
| `pkg/filter/collect.go` | new — `Options`, `Collect`, `CollectResult{Files, SkippedCount}` |
| `pkg/filter/filter_test.go` | new — per-matcher, dedup, and scanoss.py parity tests |
| `pkg/settings/settings.go` | additive — `Tuning`/`Skip`/`SizeRule`/`Folders` |
| `pkg/scanner/worker.go` | `CollectFiles` → `pkg/filter`; add `CollectFilesWithOptions` |
| `pkg/orchestrator/scanner.go` | optional `ScanResult.SkippedCount` (numeric only; no filtered list) |
| `cmd/scan.go` | build filter from settings, new flags, stderr summary count (output unchanged) |
| `go.mod` / `go.sum` | add gitignore-matching dependency |
| `README.md` | document defaults, `settings.skip`/`folders`, new flags, SDK usage |

## Alternatives considered
- **`bmatcuk/doublestar` for patterns** (per #17) — fallback only; it is pure-glob
  and lacks gitignore negation/anchoring, so it would diverge from scanoss.py
  parity. Prefer a gitignore-spec lib.
- **Tracking/returning the filtered list** (in output or as SDK data) — rejected
  per requirement: skipped files are just dropped. Only a count is kept, surfaced
  as a stderr summary; the scan output stays results-only. This is why matchers
  return a boolean rather than a reason.
- **One monolithic filter function with flags** — rejected in favor of the
  composite (small leaves + per-source loaders + one aggregator) per the simple
  architecture requirement (NFR-004).
- **Porting HFH variants now** — deferred; only a `HFH bool` seam is added.

## Testing strategy
**Unit tests** (`pkg/filter`): each leaf matcher; `Build` dedupe (same `Key()`
collapses to one matcher); `Collect` over a fixture tree (`node_modules/`,
`__pycache__/`, `vendor/`, `a.png`, `b.md`, a 50-byte file, a normal `.go`)
asserting the kept set (`Files`) is exactly the expected files and `SkippedCount`
matches the number excluded; per-pattern size rules; `folders.include/exclude`;
`.gitignore` honoring.

**Parity test** (#17 AC): a fixture tree + `scanoss.json` produces the same kept
set as scanoss.py for the same input.

**CLI end-to-end:** `scanoss scan ./fixture -o out.json` → `out.json` contains
only scan results (no filtered list), file count = scanned set; stderr summary
count present. Auto-detect vs explicit `--settings` both resolve.

**Back-compat:** a tree with no `scanoss.json` still scans with improved defaults.

## Engineering conventions
- **Conventional Commits** (`feat:`, `fix:`, `test:`, `docs:`, `refactor:`).
- **Atomic commits** — one logical change per commit (roughly one task/sub-task).
- **No AI/assistant references** in commit messages — no "generated by", no
  co-author trailers.
- **Short** commit subjects (imperative, ≤ ~50 chars).
- Every task ships with **unit tests**; the feature is covered end-to-end.

## Risks & rollout
- **New dependency** (gitignore matcher) — small, widely used; vet license.
- **Scan output shape is unchanged** — skipped files are dropped (only counted),
  so no output-format migration. (`scanner.CollectFiles` return type does change
  for SDK callers — note in CHANGELOG; pre-1.0.)
- `pkg/settings` change is additive (no migration).
- After merge, **cut a tagged release** so the downstream consumer can pin (#17 AC).
