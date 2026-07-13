<!-- Migrated from openspec/changes/dependency-source-locations/tasks.md -->

# Tasks: Capture dependency source locations

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | 650–850 (impl ~350, tests ~400) |
| 400-line budget risk | High |
| Chained PRs recommended | Yes |
| Suggested split | PR 1 — foundation + line-based parsers · PR 2 — JSON parser (npm) · PR 3 — XML parsers (nuget, maven) |
| Delivery strategy | ask-on-risk |
| Chain strategy | pending |

Decision needed before apply: Yes
Chained PRs recommended: Yes
Chain strategy: pending
400-line budget risk: High

### Suggested Work Units

| Unit | Goal | Likely PR | Notes |
|------|------|-----------|-------|
| 1 | Types + utils + line-based parsers (golang, python, ruby, gradle, npm yarn.lock) | PR 1 | Base: feat/dependency-source-locations; self-contained; all line-scanners green |
| 2 | JSON parser (npm package.json / package-lock v1 & v2) | PR 2 | Base: PR 1 branch; `json.Decoder` token walk + `lineTextAt` helper |
| 3 | XML parsers: nuget (.csproj, packages.config) then maven (pom.xml single-pass collect-then-resolve) + `make check` | PR 3 | Base: PR 2 branch; most complex; mandated maven test; gate run here |

---

## Phase 1: Foundation — types and shared helpers

- [x] 1.1 **[RED]** In `pkg/dependencies/parsers/types_test.go`, add table-driven test `TestLocalPurlJsonRoundTrip`: assert `Line` and `DeclaredText` are present in marshaled JSON when set, absent (omitempty) when zero, and that existing fields (`Purl`, `Requirement`, `Scope`) are unchanged. *(Req: Source Location Fields; Scenario: Fields absent from JSON when location is unknown; Scenario: Existing callers are unaffected)*

- [x] 1.2 **[GREEN]** In `pkg/dependencies/parsers/types.go`, add `Line int` (`json:"line,omitempty"`) and `DeclaredText string` (`json:"declaredText,omitempty"`) to `LocalPurl`. *(Req: Source Location Fields)*

- [x] 1.3 **[RED]** In `pkg/dependencies/parsers/utils_test.go`, add table-driven test `TestOffsetToLine`: cover `off=0` (line 1), offset mid-first-line, offset just after first `\n`, offset at last byte, `off<0` (clamp to 1), `off>len` (clamp to last line), empty content, content with no newlines. *(Design: shared helper)*

- [x] 1.4 **[GREEN]** In `pkg/dependencies/parsers/utils.go`, add unexported `offsetToLine(content []byte, off int) int` — counts `\n` bytes in `content[:off]`, returns `1 + count`, clamping `off` to `[0, len(content)]`. Add `bytes` import. *(Design: shared helper)*

- [x] 1.5 **[RED]** In `pkg/dependencies/parsers/utils_test.go`, add `TestLineTextAt`: cover offset on first line (no leading `\n`), offset on a middle line, offset on the last line (no trailing `\n`), offset at a `\n` byte itself. *(Design: JSON `DeclaredText` helper)*

- [x] 1.6 **[GREEN]** In `pkg/dependencies/parsers/utils.go`, add unexported `lineTextAt(content []byte, off int) string` — walks backward to the preceding `\n` (or start), forward to the next `\n` (or end), returns `strings.TrimSpace` of that slice. *(Design: JSON DeclaredText helper)*

---

## Phase 2: Line-based parsers

- [x] 2.1 **[RED]** In `pkg/dependencies/parsers/golang_test.go`, extend (or add) table-driven tests asserting `Line` and `DeclaredText` for: a `go.mod` with blank lines and comments before the first `require` (counter not fooled), a require block with multiple entries, a `go.sum` entry, and a case where no dep matches (zero values). *(Req: Line-Based Parsers; Scenario: go.mod dependency line captured)*

- [x] 2.2 **[GREEN]** In `pkg/dependencies/parsers/golang.go`, add `lineNo int` counter (incremented each `scanner.Scan()`); capture `raw := scanner.Text()` before `PreprocessLine`; set `Line: lineNo, DeclaredText: strings.TrimSpace(raw)` on each appended `LocalPurl`. *(Design: Family 1 — golang.go)*

- [x] 2.3 **[RED]** In `pkg/dependencies/parsers/python_test.go`, add table-driven tests: `requirements.txt` with blank lines above the first dep; `pyproject.toml` with multiple deps on a single line sharing the same `Line`; zero-value case for unmappable entry. *(Req: Line-Based Parsers; Scenario: requirements.txt dependency line captured; Req: Unmappable Edge Cases)*

- [x] 2.4 **[GREEN]** In `pkg/dependencies/parsers/python.go`, add `lineNo` counter + `raw` capture pattern. For `pyproject.toml` multi-dep lines all deps share the same `lineNo` and `DeclaredText` (the whole line). *(Design: Family 1 — python.go)*

- [x] 2.5 **[RED]** In `pkg/dependencies/parsers/ruby_test.go`, add table-driven tests: `Gemfile` with source line and blank lines before gem declarations; `Gemfile.lock` entry; zero-value case. *(Req: Line-Based Parsers; Scenario: Gemfile dependency line captured)*

- [x] 2.6 **[GREEN]** In `pkg/dependencies/parsers/ruby.go`, add `lineNo` counter + `raw` capture pattern. *(Design: Family 1 — ruby.go)*

- [x] 2.7 **[RED]** In `pkg/dependencies/parsers/gradle_test.go`, add table-driven tests: compact single-line dep (`Line` = that line); multi-line extended dep (`Line` = block-start line, `DeclaredText` = joined trimmed buffer); zero-value case. *(Req: Line-Based Parsers; Design: Gradle multi-line rule)*

- [x] 2.8 **[GREEN]** In `pkg/dependencies/parsers/gradle.go`, add `lineNo` counter; for compact deps `Line = lineNo`; add `blockStartLine int` captured when `inMultiLine` is set; when dep is emitted `Line = blockStartLine`, `DeclaredText = strings.TrimSpace(multiLineBuffer)`. *(Design: Family 1 — gradle.go)*

- [x] 2.9 **[RED]** In `pkg/dependencies/parsers/npm_test.go`, add table-driven tests for the yarn.lock path: `Line` points at the `pkg@range:` entry line (not the `version` line); multiple packages; zero-value case for unmappable entry. *(Req: Line-Based Parsers; Scenario: yarn.lock entry line captured)*

- [x] 2.10 **[GREEN]** In `pkg/dependencies/parsers/npm.go` yarn.lock path, record `currentPackageLine int` when `yarnLockEntryRegex` matches; use that as `Line` and the trimmed entry line as `DeclaredText` when emitting. *(Design: Family 1 — npm.go yarn.lock)*

---

## Phase 3: JSON parser (npm package.json / package-lock.json)

- [x] 3.1 **[RED]** In `pkg/dependencies/parsers/npm_test.go`, add table-driven tests for the `json.Decoder` token-walk path:
  - `package.json`: first key in `dependencies` object; last key (trailing-comma vs none); scoped `@scope/name` key; `devDependencies` key; zero-value case for an unmappable entry. *(Req: JSON Manifest Parsers; Scenario: package.json dependency location captured; Design: JSON key-offset edge)*
  - `package-lock.json` v1: nested dep gets its nested line; deduplicated coordinate keeps first-occurrence line.
  - `package-lock.json` v2: path-keyed entry (e.g. `node_modules/react`) gets the entry line; root `""` key is skipped; nested `node_modules` key is skipped.

- [x] 3.2 **[GREEN]** In `pkg/dependencies/parsers/npm.go`, rewrite the `package.json` and `package-lock.json` extraction section to use `json.NewDecoder(bytes.NewReader(fileContent))`; capture `offBefore := dec.InputOffset()` before each `dec.Token()` key read; derive `Line = offsetToLine(fileContent, int(offBefore))` and `DeclaredText = lineTextAt(fileContent, int(offBefore))`; preserve scope logic and skip rules unchanged. *(Design: Family 2 — JSON parser)*

---

## Phase 4: XML parsers

- [x] 4.1 **[RED]** In `pkg/dependencies/parsers/nuget_test.go`, add table-driven tests:
  - Self-closing `<PackageReference Include="X" Version="Y" />` on a single line: `Line` = that line, `DeclaredText` = trimmed single tag.
  - Multi-attribute self-closing tag spanning one line (`.csproj`).
  - `packages.config` `<package id="X" version="Y" />` entry.
  - Zero-value case for unmappable entry. *(Req: XML Manifest Parsers; Scenario: .csproj PackageReference element captured; Req: Unmappable Edge Cases)*

- [x] 4.2 **[GREEN]** In `pkg/dependencies/parsers/nuget.go`, replace `xml.Unmarshal` with a single `xml.NewDecoder` token loop; for each target `StartElement` capture `prevOff` (before `d.Token()`), call `d.DecodeElement(&elem, &start)`, capture `endOff`; set `Line = offsetToLine(fileContent, int(prevOff))` and `DeclaredText = strings.TrimSpace(string(fileContent[prevOff:endOff]))`. *(Design: Family 2 — XML nuget)*

- [x] 4.3 **[RED]** In `pkg/dependencies/parsers/maven_test.go`, add a table-driven test with a single inline raw-string pom.xml fixture containing ALL of:
  - A `<dependencies>` block with at least two deps (one multi-line, one single-line).
  - A `<dependencyManagement>` block with a dep that shares a coordinate with a project-level dep (proves only project-level deps are emitted).
  - A `<plugin>` `<dependency>` that must NOT appear in output.
  - A duplicate coordinate in `<dependencies>` — output contains it once, at the first-occurrence line.
  - A `${property}` version where the property is defined in `<properties>` AFTER the `<dependency>` that uses it (proves collect-then-resolve).
  Assert: correct PURL set (byte-identical to current extraction), each dep's `Line` matches the start of its `<dependency>` tag, each dep's `DeclaredText` is the full trimmed block, the post-definition `${property}` version is resolved correctly. *(Req: XML Manifest Parsers; Scenario: pom.xml dependency element captured; Design: maven single-pass collect-then-resolve; Design: Risks — maven parent-context scoping and collect-then-resolve ordering)*

- [x] 4.4 **[GREEN]** In `pkg/dependencies/parsers/maven.go`, replace `xml.Unmarshal` with a single `xml.NewDecoder` pass: track parent-context depth to scope only project-level `<dependencies>` children; on each in-scope `<dependency>` `StartElement` capture `prevOff`, `DecodeElement` into existing `PomDependency` struct, capture `endOff`, append collected element + `(line, declaredText)`; also collect `<properties>` and project coordinates in the same pass; after the pass run existing `resolveVersion` + dedup logic over the collected slice. *(Design: Family 2 — maven single-pass collect-then-resolve)*

---

## Phase 5: Gate

- [ ] 5.1 Run `make check` (`fmt-check`, `go vet`, `golangci-lint`, `go test ./...`) and confirm all checks pass. *(Design: Verification; Proposal: make check gate)*

- [ ] 5.2 Smoke test: run `scanoss dependencies --extract-local` against a sample tree containing at least one file per ecosystem and confirm `line` and `declaredText` fields appear in the output JSON for each dep. *(Design: Verification)*
