# Implementation Plan: one filter source, one filtering pass

**Spec:** [`spec.md`](./spec.md) · **Tasks:** [`tasks.md`](./tasks.md) · **Ticket:** [#25](https://github.com/scanoss/scanoss.go/issues/25)

## Technical context

- `pkg/filter` already declares itself the canonical source (`defaults.go:34`:
  *"the canonical Go source"*) and is standalone by design — it imports no other
  scanoss package, so every other package may depend on it. `pkg/scanner`
  already does.
- `filter.NewMatcher` exists and is documented for *"callers that evaluate
  entries one at a time … instead of walking a tree"* — exactly `libscanoss`'s
  case. It has never been used from there.
- `settings.filterFor(operation)` already handles all three operations
  (`settings.go:145`, with `case OperationDependencies` at lines 115 and 128).
  Only the exported wrapper is missing.
- `pkg/dependencies` takes its parser keys **from `pkg/manifests`**
  (`parser.go:46`), the same source `PreserveDependencyManifests` uses, and
  `TestManifestsMatchParsers` guards the pair. So the manifest exemption and the
  parser cannot disagree.

### The constraint that shapes the design

`filter.DefaultSkippedDirs` is public and read directly by consumers to build
their own pruning sets. Its **value** is part of the contract, not just its
name. So the shared list cannot simply become the intersection: the composition
has to be arranged so that `DefaultSkippedDirs` keeps evaluating to the same 14
entries it has today.

## Design overview

### 1. Shared list plus named deltas

```go
// pkg/filter/defaults.go

// CommonSkippedDirs: skipped by every operation.
var CommonSkippedDirs = []string{"__pycache__", "node_modules", "vendor"}

// ScanOnlySkippedDirs: added when scanning or fingerprinting. Example code is
// not the product, and virtualenvs/eggs/wheels hold installed packages whose
// code is not the project's.
var ScanOnlySkippedDirs = []string{
	"nbproject", "nbbuild", "nbdist", "venv", "_yardoc",
	"eggs", "wheels", "htmlcov", "__pypackages__", "example", "examples",
}

// DependencyOnlySkippedDirs: added when collecting dependencies. Generated
// trees, whose manifests are build output rather than declarations.
var DependencyOnlySkippedDirs = []string{"dist", "build", "target"}

// DefaultSkippedDirs is the scanning set. Its VALUE is public contract —
// consumers build their own prune sets from it — so it must keep resolving to
// the same entries it always had.
var DefaultSkippedDirs = concat(CommonSkippedDirs, ScanOnlySkippedDirs)
```

`.git` and `.svn`, which `cmd/dependencies.go` lists today, are deliberately
absent: `Collect` prunes every dot-directory before any matcher runs, so they
are already covered.

A test asserts `DefaultSkippedDirs` equals the 14 literals it has today, so the
composition cannot silently change what consumers see (FR-3).

### 2. Names, extensions and endings: one shared list, no deltas

Unlike directories, these need no per-operation variants. The only real
difference — dependencies must see manifests that scanning discards by extension
(`.json`, `.mod`, `.toml`, `.xml`, `.lock`, …) — is already solved by
`PreserveDependencyManifests`, which exempts the 13 names `pkg/manifests`
recognises. That is strictly more precise than reopening whole extensions:
scanoss.js takes the latter route and ends up walking every `.json` in the
project to find one `package.json`.

Verified manifest by manifest: all 13 are either covered by a skipped extension
and rescued by the exemption, or have no extension at all (`Gemfile`). So a
delta here would fix nothing.

### 3. A profile per layer

One constructor per layer, not an enum and a dispatcher: each caller knows at
compile time which one it is, so a named function is clearer than a runtime
lookup. (`pkg/settings` does need an operation parameter — it selects between
three sections of the same JSON — but `pkg/filter` has nothing to select.)

```go
func ScanOptions() Options         // exists
func FingerprintOptions() Options  // new
func DependencyOptions() Options   // new
```

| | `Defaults` | `GitIgnore` | manifests | dirs |
|---|---|---|---|---|
| `ScanOptions` / `FingerprintOptions` | true | true | skipped | common + scan-only |
| `DependencyOptions` | true | **false** | **preserved** | common + dependency-only |

`GitIgnore: false` for dependencies preserves today's behaviour (FR-4).

`DefaultSkippedDirs` is removed rather than renamed: with three directory lists,
"default" identifies none of them. What a caller applying the rules by hand
needs is `CommonSkippedDirs` — the set no operation wants, and therefore the
most a pre-filter can safely prune before it knows what will consume the
result.

### 4. Filtering happens once

`pkg/scanner/worker.go:90` and `pkg/fingerprint/wfp.go:138` drop their
`ShouldSkipFile` calls; `filteredExt` and `ShouldSkipFile` are deleted. `.whl` —
the one extension the worker had and the collection did not — moves into
`DefaultSkippedExts`, so nothing starts being fingerprinted that was not before.

`libscanoss` gains the explicit matcher it always should have had, keeping its
behaviour unchanged.

### 5. `cmd/dependencies.go` collects like everyone else

`collectFilesRecursively` (38 lines of `filepath.Walk` with an embedded list)
becomes a `filter.Collect` call with `OptionsFor(OpDependencies)` plus
`settings.DependencyFilter()`.

Its file set is unchanged: today it collects everything and lets
`IsSupportedFile` decide; after, the default lists prune first and the manifest
exemption rescues the 13 the parser handles. Same 13 files, different route —
which is why a test pins the outcome rather than the mechanism.

## Key changes
- `pkg/filter/defaults.go` — shared list + deltas; `.whl`.
- `pkg/filter/collect.go` — `FingerprintOptions`, `DependencyOptions`;
  `NewMatcher` applies directory rules to the whole path (T011).
- `pkg/settings/settings.go` — `DependencyFilter()`.
- `pkg/scanner/worker.go` — drop the extension check.
- `pkg/fingerprint/wfp/wfp.go` — delete `filteredExt` and `ShouldSkipFile`.
- `libscanoss/core/libscanoss.go` — explicit matcher.
- `cmd/scan.go`, `cmd/wfp.go`, `cmd/dependencies.go` — consume `OptionsFor`.
- `CHANGELOG.md`.

## Testing strategy
- **Conformance:** `DefaultSkippedDirs` equals its current 14 literals; no skip
  list exists outside `pkg/filter` (a test that greps for a second one is not
  possible, so instead: `pkg/fingerprint/wfp` exports no filtering symbol).
- **No behaviour change:** for each operation, collect a fixture tree before and
  after and assert the file sets are identical. The fixture must include a
  `venv/`, an `examples/` with a `go.mod`, a `dist/package.json`, and one file
  of every skipped extension.
- **The fixes:** `--default-filters=false` now keeps `a.png`;
  `skip.patterns.dependencies` is honoured; the filtered count includes what the
  lower layers used to drop silently.
- **The seam:** a 40-byte `.png` handed straight to `GenerateWFP` is
  fingerprinted (the contract change, pinned deliberately).

## Commit conventions
Conventional Commits, atomic, short subjects, no AI/co-author trailers.
`CHANGELOG.md` in the product-changing commits.

## Risks / trade-offs

**The contract change is silent.** `GenerateWFP` and `GenerateFingerprint` keep
their signatures and stop filtering. A consumer passing an unfiltered list
compiles fine and gets more files. This is the only change that cannot fail
loudly, so it needs an explicit CHANGELOG entry rather than a footnote. The
documented path (`CollectFiles*` first) is unaffected.

**`dependencies` takes a different route to the same result.** Today: collect
everything, let the parser choose. After: prune first, rescue manifests. The
outcome is identical only because `pkg/manifests` and the parser share keys —
which `TestManifestsMatchParsers` already guarantees. If that test is ever
removed, this becomes fragile.

**Deleting `ShouldSkipFile` is a breaking SDK change**, but it fails at compile
time, which is the good kind. The SDK is 13 days old.
