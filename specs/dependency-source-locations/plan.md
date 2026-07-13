<!-- Migrated from openspec/changes/dependency-source-locations/design.md -->

# Design: Capture dependency source locations

## Context

`pkg/dependencies/parsers` has 7 ecosystem parsers, each a `ParserFunc`
(`func(fileContent []byte, filePath string) (*LocalDependency, error)`). They emit
`LocalPurl{Purl, Requirement, Scope}` and discard where in the manifest each
dependency was declared. This change adds `Line int` + `DeclaredText string` to
`LocalPurl` and populates them everywhere.

The parsers split cleanly into two families by *how they read input*, and that split
— not the ecosystem — drives the design:

- **Line-scanning parsers** (`golang.go`, `gradle.go`, `ruby.go`, `python.go`, and
  npm's `yarn.lock` path): already iterate with `bufio.Scanner` over
  `bytes.NewReader(fileContent)`. They *see* every line as they go; they simply never
  recorded the index. Capturing location here is a counter plus the raw line — cheap
  and exact.
- **Unmarshal parsers** (`npm.go` package.json/package-lock.json via `json.Unmarshal`,
  `maven.go` pom.xml and `nuget.go` .csproj/packages.config via `xml.Unmarshal`):
  decode straight into structs. Unmarshal throws away byte position, so there is *no*
  line to read. This is the central engineering problem of the change and the reason
  this phase is opus.

The locked contract (do not relitigate):

- `LocalPurl` gains `Line int` (`json:"line,omitempty"`, 1-based, `0` = unknown) and
  `DeclaredText string` (`json:"declaredText,omitempty"`, trimmed).
- Line-based: `Line` = declaration line; `DeclaredText` = trimmed line.
- XML: `Line` = element START line (`<dependency>` / `<PackageReference>`);
  `DeclaredText` = the FULL element block, multi-line, trimmed.
- DoD: every dep in a common well-formed manifest gets a non-zero line; `Line:0`
  only for genuinely unmappable edge cases (and that edge case is tested).
- Backward compat: `omitempty`; the api.scanoss.com request stays byte-identical.

## Goals / Non-Goals

Goals:
- Populate `Line` + `DeclaredText` for every extracted dependency across all 7
  parsers, robustly, for common well-formed manifests.
- Keep PURL / requirement / scope extraction byte-for-byte unchanged.
- Smallest robust change; lint-clean idiomatic Go; test-first.

Non-Goals:
- Any downstream consumption (separate repo/story).
- Changing the set of supported ecosystems or the PURL/requirement/scope logic.
- Recovering locations from pathological/malformed input — `Line:0` is the documented
  contract for those.
- A general-purpose "located decoder" abstraction. We add the *minimum* shared helper.

## Backward-compatibility re-confirmation

Re-checked the only call sites that touch `LocalPurl` in a serialization path
(`cmd/dependencies.go`):

- `queryDirectDependenciesWithFiles` (~L444–460) builds
  `map[string]string{"purl": purl.Purl}` — copies *only* `purl`.
- `queryTransitiveDependenciesWithFiles` (~L476–488) builds
  `map[string]string{"purl": ..., "requirement": ...}` — copies *only* `purl` and
  `requirement`.

Neither marshals `LocalPurl` directly, so the new fields cannot reach the
api.scanoss.com payload and cannot trip server-side validation. The `--extract-local`
/ `--output` JSON path *does* marshal `LocalPurl`; there the new fields appear only
when set (omitempty), so existing output for any dep we cannot locate is unchanged.
No other call site in the repo marshals `LocalPurl`.

## Decision

### Family 1 — Line-scanning parsers: track an index, capture the raw text

`bufio.Scanner` does not expose a line number, so we maintain a 1-based counter
incremented once per `scanner.Scan()` iteration, and we capture the *raw* line
(`scanner.Text()` before trimming/comment-stripping) so `DeclaredText` is the trimmed
original, not the post-processed token.

The minimal, uniform edit per scanning loop:

```go
scanner := bufio.NewScanner(bytes.NewReader(fileContent))
lineNo := 0
for scanner.Scan() {
    lineNo++
    raw := scanner.Text()          // capture BEFORE PreprocessLine/TrimComments
    line := PreprocessLine(raw)    // existing logic, unchanged
    ...
    result.Purls = append(result.Purls, LocalPurl{
        Purl:         purl,
        Requirement:  version,
        Scope:        scope,        // where applicable
        Line:         lineNo,
        DeclaredText: strings.TrimSpace(raw),
    })
}
```

Per-parser nuances:

- **golang.go** (`go.mod`, `go.sum`): one append per matched line; `lineNo` and
  `strings.TrimSpace(raw)` apply directly. Inside the `require (` block each entry is
  its own physical line — exact.
- **python.go** (`requirements.txt`): one dep per line — exact. (`pyproject.toml`): the
  current scanner can extract *multiple* quoted deps from one physical line; all of
  them share that line's number and `DeclaredText` (the whole line). Correct per the
  contract — the declaration line is the same physical line.
- **ruby.go** (`Gemfile`, `Gemfile.lock`): one dep per matched line — exact.
- **gradle.go**: the compact path is one dep per line — exact. The multi-line extended
  path buffers several physical lines into `multiLineBuffer` before emitting one dep.
  Capture the line number at the START of the buffered block (the line where
  `inMultiLine` is first set, or the compact line otherwise) and set `DeclaredText` to
  the *joined buffered block* trimmed. See "Gradle multi-line" below for the exact rule.
- **npm.go `yarn.lock`**: the dep is emitted when the `version "x"` line is matched,
  but the human-meaningful declaration is the *package entry* line
  (`pkg@^1.0.0:`) that precedes it. Record `lineNo` when `yarnLockEntryRegex` matches
  (the entry line) into a `currentPackageLine int` alongside `currentPackage`, and use
  that as `Line`; `DeclaredText` = the trimmed entry line. This points triage at the
  declaration, not the resolved-version detail line.

These are pure additions: no existing branch, regex, or append condition changes, so
PURL/requirement/scope output is identical.

#### Gradle multi-line rule

`ParseBuildGradle` accumulates `implementation(...)` style calls that open `(` on one
line and close `)` on a later line. Add a `blockStartLine int` captured when the block
opens (the line that sets `inMultiLine = true`) or, for compact/single-line deps, the
current line. When the dep is emitted, `Line = blockStartLine` and
`DeclaredText = strings.TrimSpace(multiLineBuffer-or-raw-line)`. For the extended
format that spans lines, `DeclaredText` is the trimmed joined buffer (already built in
`multiLineBuffer`), giving a faithful multi-line declared text — consistent with the
XML "full element block" spirit.

### Family 2 — Unmarshal parsers: stream tokens with `InputOffset`, map offset→line

**Decision: streaming token decode with `Decoder.InputOffset()`, mapped to a line via a
shared `offsetToLine` helper.** This is the robust, production-grade choice; the
regex/second-pass alternative is rejected below.

Both stdlib decoders expose a byte offset into the *original* input, and both are
available under this module's `go 1.22` (`toolchain go1.24.4`):

- `(*json.Decoder).InputOffset() int64` — returns the offset just past the most
  recently returned token. Available since **Go 1.14**.
- `(*xml.Decoder).InputOffset() int64` — same semantics for XML tokens. Available
  since **Go 1.4** (long predates our floor).

These are *not assumptions*: both are documented stdlib methods present well below
go 1.22, so no go.mod bump is required.

Strategy: keep decoding from `bytes.NewReader(fileContent)` so the byte offset is into
the same `fileContent` we already hold, capture the offset at the precise token that
*starts* the dependency element, then convert offset→1-based line with a single shared
helper over `fileContent`. Because the offset is taken at the moment the decoder reads
*that specific element*, there is no ambiguity with same-named strings elsewhere: we
match by *position in the token stream*, not by string search.

#### Shared helper (added to `utils.go`)

```go
// offsetToLine returns the 1-based line number containing byte index off in content.
// It counts '\n' bytes strictly before off. off<=0 or off>len(content) yields the
// nearest valid line; callers pass a real token offset so the common path is exact.
func offsetToLine(content []byte, off int) int {
    if off < 0 {
        off = 0
    }
    if off > len(content) {
        off = len(content)
    }
    return 1 + bytes.Count(content[:off], []byte{'\n'})
}
```

This is O(off) per lookup. Dependency counts in real manifests are small (tens to low
hundreds), so the aggregate cost is negligible; we deliberately avoid a prebuilt
line-index table to keep the change minimal. (If profiling ever flags it, a one-pass
`[]int` newline-offset slice with `sort.Search` is a drop-in upgrade — noted, not built.)

`utils.go` already imports `strings`; this adds `bytes`. One small, lint-clean helper,
unexported, shared by both XML parsers and (via a sibling JSON helper described below)
the npm parser.

#### XML parsers (maven.go, nuget.go) — element START line + full block

XML is the cleaner case because `InputOffset()` after reading a `StartElement` token
points at the byte just past `<dependency>` / `<PackageReference ...>`, and the element
is bounded by its matching `EndElement`. The common per-element shape (used by all three
XML files) is:

1. Construct `d := xml.NewDecoder(bytes.NewReader(fileContent))`.
2. Token-loop. When we see the `StartElement` we care about
   (`dependency` for pom, `PackageReference` for csproj, `package` for packages.config),
   record `startOff := d.InputOffset()` *measured before consuming the start tag* and
   the start line. Concretely: capture `prevOff := d.InputOffset()` at the top of each
   loop iteration *before* `d.Token()`; when the returned token is the target
   `StartElement`, `startOff = prevOff` is the byte offset of the `<` of that tag, so
   `Line = offsetToLine(fileContent, startOff)` is the element START line.
3. `DecodeElement(&elem, &start)` to decode that element into the existing struct
   (`PomDependency` / `CsprojPackageReference` / `PackagesConfigPackage`) — reusing the
   *exact* existing field tags and decode behavior, so coordinate extraction is
   unchanged. After `DecodeElement` returns, `endOff := d.InputOffset()` is just past
   the `</...>` end tag (or `/>` self-close).
4. `DeclaredText = strings.TrimSpace(string(fileContent[startOff:endOff]))` — the FULL
   element block, multi-line, trimmed, exactly per contract. For self-closing
   `<PackageReference Include=".." Version=".." />` this is the single tag; for a
   multi-line `<dependency>…</dependency>` it is the whole block.

This binds each dependency's location to *its own element by construction* — there is no
separate index to keep in sync.

Why this is robust:

- **Right element guaranteed**: we slice the bytes between the start and end offsets of
  the *element the decoder just handed us*. A `groupId` string that happens to equal
  another dependency's name elsewhere cannot be mismatched — we never string-search.
- **Nested / duplicate coordinates**: maven dedups by PURL *after* extraction; the
  `seen[purl]` map means only the FIRST occurrence of a duplicate coordinate is
  appended. We record the line on each collected element and emit the first occurrence's
  line, so the reported line corresponds to the dep we actually emit. Document this:
  duplicates point at their first declaration. (No behavior change — dedup already existed.)
- **Right element set guaranteed**: parent-context tracking (below) means only
  project-level `<dependencies>` children are located — `<dependencyManagement>` and
  plugin `<dependency>` elements are skipped, matching the current extraction set.

Implementation note (maven) — **single streaming pass, collect-then-resolve**:

The current code decodes the *whole* `PomProject` and only extracts project-level
`<dependencies>` (the struct has `Dependencies PomDependencies` mapped to
`project > dependencies`; there is **no** `<dependencyManagement>` field, so management
deps and plugin deps are already OUT of the extraction set). We preserve exactly that
set. We do NOT keep `xml.Unmarshal` plus a separate location pass — zipping two passes by
element index is a *silent* failure mode: any element one pass counts and the other does
not (e.g. a `<dependencyManagement>` or plugin `<dependency>`) shifts every later line
number with no error. Instead, ONE `xml.Decoder` traversal does everything:

1. Walk the document with a single `xml.Decoder`. Track the parent context so we only
   treat a `<dependency>` as a real dependency when it sits under the **project-level
   `<dependencies>`** element (matching today's `xml:"dependencies"` mapping) — i.e. the
   `<dependencies>` that is a direct child of `<project>`. `<dependency>` elements under
   `<dependencyManagement>` or inside `<plugin>`/`<build>` are SKIPPED, so the extraction
   set stays byte-identical to the current `Unmarshal`-based code.
2. On each in-scope `<dependency>` `StartElement`: record `startOff` (the common shape
   above), `DecodeElement(&dep, &start)` into the **existing** `PomDependency` struct
   (same tags, same decode → coordinate extraction unchanged), capture `endOff`, and
   slice `DeclaredText = strings.TrimSpace(string(fileContent[startOff:endOff]))` — the
   full multi-line `<dependency>…</dependency>` block. Append the decoded element plus
   its `(line, declaredText)` to an in-memory slice. Location is bound to *this* element
   by construction; there is no index to align.
3. During the SAME pass, also collect the `<properties>` map (the existing
   `PomProperties.UnmarshalXML` is reused verbatim via `DecodeElement` when we hit the
   `<properties>` start element) plus `project.version` / `project.groupId` /
   `project.artifactId`. We do NOT resolve versions on the fly, because Maven properties
   may be declared out of document order (a `<dependency>` can reference a `${prop}`
   defined later in the file). Collect first, resolve after.
4. **After** the pass, run the existing logic over the collected elements: build the
   `props` map exactly as today, call `resolveVersion(dep.Version, props,
   string(fileContent))` per collected dependency, apply qualifiers, and dedup with the
   existing `seen[purl]` map. `resolveVersion`'s behavior is byte-identical — only its
   *input source* changes from `pom.Dependencies.Dependency` to the collected slice. The
   location/`DeclaredText` ride along on each collected element, so the emitted
   `LocalPurl` carries the line of the element it came from. Dedup still keeps the first
   occurrence; that first occurrence's line is reported.

`resolveVersion` is safe to apply post-pass: it is a pure function of `(versionString,
props, fileContent)` with no dependence on the decoded tree, so collecting elements first
and resolving afterward changes nothing about its output.

For nuget (.csproj, packages.config) the structs are flat and self-closing; the single
streaming token pass *replaces* `xml.Unmarshal` directly: loop, on each target
`StartElement` capture start offset, `DecodeElement` into the existing struct, capture
end offset, append `LocalPurl` with location. No properties/version-resolution coupling,
so no collect-then-resolve step is needed — it was already single-pass in this design.

#### JSON parser (npm.go) — `json.Decoder` token walk for objects, offset→line

`package.json` (`dependencies` / `devDependencies` are `map[string]string`) and
`package-lock.json` (v1 nested `dependencies`, v2 `packages` keyed by node_modules
path) decode via `json.Unmarshal` today. JSON maps are unordered after unmarshal AND
carry no offset, so we switch the *location-bearing* read to a `json.Decoder` token
walk:

1. `dec := json.NewDecoder(bytes.NewReader(fileContent))`.
2. Walk tokens to the relevant object (`dependencies`, `devDependencies`, `packages`,
   or nested `dependencies`). For each object we read its keys via `dec.Token()`; when
   the decoder returns a key string, `dec.InputOffset()` is the offset just past that
   key token — i.e. the position of the *key*, which IS the declaration line for that
   dependency in package.json (`"react": "^18.0.0"`). `Line = offsetToLine(fileContent,
   keyOff)` where `keyOff` is the offset captured *before* reading the value, adjusted
   to the start of the key (see helper below).
3. `DeclaredText`: per the locked contract, JSON deps are line-based-style, so
   `DeclaredText = strings.TrimSpace(<the physical line containing the key>)`. Reuse
   `offsetToLine`'s sibling: extract the full physical line for an offset via a small
   `lineTextAt(content []byte, off int) string` helper (find the surrounding `\n`
   bounds, trim). For package.json this yields `"react": "^18.0.0",` trimmed — exactly
   the declared line.

Key-offset precision: `json.Decoder.InputOffset()` returns the offset of the byte just
*after* the token it just read. To get the START of the key token (so the line maps to
the line the key sits on, which is what we want for `DeclaredText`), we capture
`offBefore := dec.InputOffset()` *before* the `dec.Token()` that returns the key, then
the key occupies `[offBefore, offAfter)`. Using `offBefore` for `offsetToLine` is
robust because the key's opening quote is on the same line as its value in all common
formatters; for the rare key-on-its-own-line case the start offset still resolves to the
correct (key) line, which is the more meaningful one.

Why the token walk and not "unmarshal then regex-locate the key":

- A key like `"version"` appears hundreds of times in a lockfile; a regex/string search
  keyed on the name would match the wrong occurrence. The token walk gives the *exact*
  offset of the specific key the decoder is currently positioned on — zero ambiguity.
- Nested/duplicate names (the same package under different parents in v1 lockfiles)
  each get their own token offset, so each emitted `LocalPurl` gets its own correct
  line.

Scope and dedup nuances preserved:

- package.json: `dependencies` vs `devDependencies` scope is known from which object
  we are walking — unchanged.
- package-lock v1 recursion: the recursive walk now carries offsets per nested key, so
  nested deps get their nested-line location. (Today these emit deps with no scope; that
  is unchanged.)
- package-lock v2 `packages`: keyed by `node_modules/...` path; the existing skip rules
  (root `""`, nested `node_modules`) are applied on the key as we walk — unchanged. The
  key's offset gives the declaration line of that package entry.

This is more code than the line-scanners but it is the correct robust technique and it
reuses `offsetToLine`. The structs (`PackageJSON`, `PackageLock*`) remain for any path
we keep on `Unmarshal`, but the location-bearing extraction moves to the token walk.

### Rejected alternative — regex / position-aware second pass keyed by coordinate

Considered: keep `Unmarshal` as-is, then for each extracted dependency run a regex over
`fileContent` to find its coordinate and read the line. **Rejected** because:

- **Ambiguity**: `groupId`/name/version strings recur (a version `"1.0.0"` or a common
  name like `commons-lang3`, or `"version"` keys in lockfiles) — a search cannot tell
  *which* occurrence is the declaration. The contract demands the right element, not a
  plausible one.
- **Fragility across formatting**: attributes vs child elements, namespaces, comments,
  CDATA, and whitespace all break naive regexes; we would be reimplementing a parser
  badly.
- **DoD risk**: it would produce confident-but-wrong lines, the worst failure mode for
  triage evidence. The token/offset approach is wrong-by-construction-impossible: the
  offset IS the position of the element we emitted.

All three XML parsers now use a single streaming pass; maven additionally collects
`<properties>` in that same pass and resolves versions afterward (collect-then-resolve),
so no second traversal and no index-zip exists anywhere. This eliminates the silent
misalignment failure mode entirely — location is bound to each element by construction.

## Risks / Trade-offs

- **Maven parent-context scoping**: the single pass must locate ONLY project-level
  `<dependencies>` children, skipping `<dependency>` under `<dependencyManagement>`,
  `<plugin>`, or `<build>`, so the extraction set stays byte-identical to the current
  `Unmarshal`-based code. Required test: a pom containing BOTH `<dependencies>` and
  `<dependencyManagement>` (plus a duplicate coordinate, a multi-line `<dependency>`
  block, and a `${property}` version) proving the right element set is located AND
  resolved — only the `<dependencies>` deps appear, with correct lines, full-block
  `DeclaredText`, and the property-resolved version. There is no index-zip, so the old
  silent-misalignment failure mode is gone; the remaining risk is the scoping predicate
  itself, which this test pins down.
- **Maven collect-then-resolve ordering**: versions are resolved AFTER the pass so a
  `${prop}` referenced before its `<properties>` definition still resolves. The test
  above must place the property definition such that it would fail under on-the-fly
  resolution, to lock the ordering.
- **JSON key-offset edge**: `InputOffset()` semantics (offset *after* the token) require
  the capture-before-read pattern. A unit test must assert the exact line for a key that
  is the first in its object, the last (trailing comma vs none), and a scoped
  `@angular/core` key.
- **`Line:0` cases**: genuinely unmappable inputs (e.g. a dep synthesized without a
  source token, or malformed manifest where the decoder errors before the element) keep
  `Line:0` and empty `DeclaredText`. One explicit test per family documents this.
- **`DeclaredText` size**: XML full-block text can be large for verbose `<dependency>`
  blocks. Acceptable — it is bounded by the element and `omitempty` keeps absent ones
  free. No truncation in scope.
- **Performance**: `offsetToLine` is O(offset); fine for manifest-sized inputs. Flagged
  the `[]int` index upgrade if ever needed.

## Verification

Test-first (Strict TDD), table-driven, following the existing `parser_test.go`
convention: inline byte-literal manifest content passed directly to `parsers.ParseXxx`,
asserting `result.Purls[i].Line` and `.DeclaredText` alongside the existing
`.Purl`/`.Requirement` assertions. No external golden files needed (matches current
convention); where a fixture is large (a realistic pom/lockfile), an inline raw-string
literal is the "golden manifest" for that ecosystem.

Per ecosystem, assert:

- **golang/python/ruby**: exact line and trimmed text for each dep; a manifest with
  blank lines and comments above the deps to prove the counter is not fooled.
- **gradle**: compact single-line dep line; a multi-line extended dep asserting `Line` =
  block start and `DeclaredText` = joined trimmed block.
- **npm yarn.lock**: `Line` points at the `pkg@range:` entry line, not the `version`
  line.
- **npm package.json**: scope correctness plus line of each key, including a scoped
  `@scope/name` key and the first/last key in the object.
- **npm package-lock v1 & v2**: nested dep gets its nested line; v2 path-keyed entry
  gets the entry line; skip rules unchanged.
- **maven pom.xml**: a pom with BOTH `<dependencies>` and `<dependencyManagement>` →
  only the project-level `<dependencies>` deps are located/emitted; multi-line
  `<dependency>` block → start line + full trimmed block; a duplicate coordinate →
  first-occurrence line; a `${property}` version (with the property defined *after* the
  dependency that uses it) → still resolved correctly, proving collect-then-resolve.
- **nuget .csproj & packages.config**: self-closing `<PackageReference .../>` →
  single-line `DeclaredText`; multi-line variant → block.
- **`Line:0` edge**: one test per family for an unmappable/synthesized dep.

Regression guard: a test asserting the api.scanoss.com request maps still contain only
`purl`/`requirement` is out of scope for these parser tests but the design relies on the
re-confirmation above; existing `cmd` tests (if any) remain green because the maps are
unchanged. `make check` (fmt-check, vet, golangci-lint, test) must pass.
