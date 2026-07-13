<!-- Migrated from openspec/changes/dependency-source-locations (proposal + capability spec). Work already shipped. -->

# Proposal: Capture dependency source locations

## Why

The dependency parser (`pkg/dependencies`) extracts PURLs from manifest files but
its output struct `LocalPurl` carries only `Purl`, `Requirement`, and `Scope`. It
**discards where in the manifest each dependency was declared.**

A downstream consumer (SCANOSS Earnie) needs to show, as triage evidence, the exact
source location of each declared dependency: the line number and the raw declared
text of the line in the manifest. With the current output that is impossible —
the consumer would have to re-implement and re-run all 7 ecosystem parsers itself
just to recover a line number the SDK already saw and threw away.

The information is cheap to capture at parse time and expensive to reconstruct
afterwards. This change captures it once, at the source, for every ecosystem.

## What Changes

- Add two optional fields to `LocalPurl` (`pkg/dependencies/parsers/types.go`):
  - `Line int` (`json:"line,omitempty"`) — 1-based line number in the manifest
    where the dependency is declared; `0` means unknown.
  - `DeclaredText string` (`json:"declaredText,omitempty"`) — the raw declared
    line/snippet as it appears in the manifest, trimmed.
- Populate both fields in **all 7 parsers** for the dependencies they extract:
  - Line-based parsers (already scan line-by-line, so the line number is cheap):
    `golang.go`, `gradle.go`, `ruby.go`, `python.go`, and `npm.go`'s lockfile/
    yarn path.
  - Unmarshal-based parsers (the hard cases — see Risks): `npm.go`
    (package.json, package-lock.json), `maven.go` (pom.xml), `nuget.go`
    (.csproj, packages.config).
- Tests across every ecosystem asserting the line number and declared text for the
  extracted dependencies. Implementation is **test-first** (Strict TDD); the
  `make check` gate (fmt-check, vet, golangci-lint, test) must stay green.

Both fields are optional and `omitempty`, so existing JSON output and existing Go
callers are unaffected when a parser cannot determine a location.

## Impact

**Modified Package:**
- `pkg/dependencies/parsers/types.go` — two new optional fields on `LocalPurl`.
- `pkg/dependencies/parsers/{golang,gradle,ruby,python,npm,maven,nuget}.go` —
  each parser populates `Line` + `DeclaredText`.

**Behavior:**
- New fields appear in extraction output (`--extract-local`, `--output`) when a
  location is known; they are omitted when unknown. PURL / requirement / scope
  extraction is unchanged.
- No new CLI flags, no change to the set of supported ecosystems.

**Backward Compatibility:**
- Additive and `omitempty`: existing consumers that ignore the new fields see no
  change. See Risks for the SCANOSS API request path, which was verified separate.

**Not in scope:**
- Any Earnie-side consumption of the new fields (separate repo / story).
- Changing the PURL, requirement, or scope extraction logic itself.
- Transitive-dependency resolution via the SCANOSS API.

## Risks

**Unmarshal-based parsers are the central engineering risk.** `npm.go`,
`maven.go`, and `nuget.go` parse via `json.Unmarshal` / `xml.Unmarshal`, which
decode straight into structs and **discard byte position**, so there is no line
number to read. An accurate line requires moving from unmarshal-into-struct toward
streaming token decoding — `json.Decoder` / `xml.Decoder` expose `InputOffset()`,
which can be mapped back to a line — or an equivalent robust technique. The
concrete per-parser approach is **deferred to the design phase**; this proposal
only names the risk and the leading direction.

**SCANOSS API request serialization — verified, low risk.** `LocalPurl` is also
used in the scan-mode HTTP request to `api.scanoss.com`. The request body is NOT
built by marshaling `LocalPurl` directly — it is hand-assembled into
`map[string]string` / `map[string]interface{}` that copy only `purl` (and
`requirement` for the transitive path). See `cmd/dependencies.go`
`queryDirectDependenciesWithFiles` (lines ~444–466) and
`queryTransitiveDependenciesWithFiles` (lines ~476–488). Because the new fields
are never copied into those maps, adding them to `LocalPurl` cannot reach the API
payload and cannot trip server-side validation. This should be re-checked during
design in case other call sites marshal `LocalPurl` directly.


---

## Capability spec: dependency-parsing

# Dependency Parsing — Source Location Capture

## Purpose

Define requirements for capturing the manifest line number and raw declared text
for every dependency extracted by the SDK's seven ecosystem parsers, enabling
downstream consumers to display source-location triage evidence without
re-parsing manifests.

## Requirements

### Requirement: Source Location Fields on LocalPurl

The `LocalPurl` struct MUST expose two optional, backward-compatible fields:
`Line int` (`json:"line,omitempty"`, 1-based, `0` = unknown) and
`DeclaredText string` (`json:"declaredText,omitempty"`, trimmed raw declaration).

Both fields MUST be omitted from JSON output when their zero values are present,
so existing consumers and the SCANOSS API request payload are unaffected.

#### Scenario: Fields present when location is known

- **GIVEN** a well-formed manifest file is parsed by any of the seven ecosystem parsers
- **WHEN** the parser successfully extracts a dependency
- **THEN** the resulting `LocalPurl.Line` is a positive integer equal to the 1-based line of the declaration
- **AND** `LocalPurl.DeclaredText` is the non-empty trimmed text of that declaration

#### Scenario: Fields absent from JSON when location is unknown

- **GIVEN** a dependency whose source location cannot be determined
- **WHEN** the parser produces the `LocalPurl`
- **THEN** `Line` is `0` and `DeclaredText` is the empty string
- **AND** neither field appears in the marshaled JSON output

#### Scenario: Existing callers are unaffected

- **GIVEN** code that constructs or reads `LocalPurl` without referencing `Line` or `DeclaredText`
- **WHEN** compiled against the updated struct
- **THEN** it compiles without modification and behaves identically to before

---

### Requirement: Line-Based Manifest Parsers Populate Source Location

Parsers for line-oriented manifests (go.mod, go.sum, requirements.txt,
pyproject.toml, Gemfile, Gemfile.lock, build.gradle, and the line-scanned paths
in package-lock.json and yarn.lock) MUST set `Line` to the 1-based line number
of the dependency declaration and `DeclaredText` to that line, trimmed.

All seven parsers MUST produce a non-zero `Line` and non-empty `DeclaredText`
for every dependency found in a common, well-formed manifest. `Line: 0` is
permitted only for genuinely unmappable edge cases.

#### Scenario: go.mod dependency line captured

- **GIVEN** a `go.mod` file containing `require github.com/foo/bar v1.2.3` on line 5
- **WHEN** the golang parser processes the file
- **THEN** the resulting `LocalPurl.Line` is `5`
- **AND** `LocalPurl.DeclaredText` is `"require github.com/foo/bar v1.2.3"` (trimmed)

#### Scenario: requirements.txt dependency line captured

- **GIVEN** a `requirements.txt` file where `requests==2.28.0` appears on line 3
- **WHEN** the python parser processes the file
- **THEN** `LocalPurl.Line` is `3`
- **AND** `LocalPurl.DeclaredText` is `"requests==2.28.0"`

#### Scenario: Gemfile dependency line captured

- **GIVEN** a `Gemfile` where `gem 'rails', '~> 7.0'` appears on line 8
- **WHEN** the ruby parser processes the file
- **THEN** `LocalPurl.Line` is `8`
- **AND** `LocalPurl.DeclaredText` is `"gem 'rails', '~> 7.0'"`

#### Scenario: yarn.lock entry line captured

- **GIVEN** a `yarn.lock` file where a package entry starts on line 12
- **WHEN** the npm parser processes the lockfile path
- **THEN** `LocalPurl.Line` is `12`
- **AND** `LocalPurl.DeclaredText` is the trimmed declaration line for that entry

---

### Requirement: JSON Manifest Parsers Populate Source Location

Parsers for JSON manifests (package.json, package-lock.json) MUST derive the
1-based line number via token-level decoding (not simple `json.Unmarshal` into
a struct) and set `Line` and `DeclaredText` accordingly.

#### Scenario: package.json dependency location captured

- **GIVEN** a `package.json` where `"react": "^18.0.0"` appears within the
  `dependencies` object at line 7
- **WHEN** the npm parser processes the file
- **THEN** `LocalPurl.Line` is `7`
- **AND** `LocalPurl.DeclaredText` is `"\"react\": \"^18.0.0\""` (trimmed)

---

### Requirement: XML Manifest Parsers Populate Source Location

Parsers for XML manifests (pom.xml, .csproj, packages.config) MUST set `Line`
to the line where the dependency element STARTS (the opening tag) and
`DeclaredText` to the full element block, multi-line, trimmed, so consumers
receive the complete coordinate as a copy-pasteable snippet.

#### Scenario: pom.xml dependency element captured

- **GIVEN** a `pom.xml` where a `<dependency>` block starts on line 20
  and spans lines 20–25 with `groupId`, `artifactId`, and `version` children
- **WHEN** the maven parser processes the file
- **THEN** `LocalPurl.Line` is `20`
- **AND** `LocalPurl.DeclaredText` is the full trimmed `<dependency>…</dependency>` block

#### Scenario: .csproj PackageReference element captured

- **GIVEN** a `.csproj` file where `<PackageReference Include="Newtonsoft.Json" Version="13.0.1" />`
  appears on line 14
- **WHEN** the nuget parser processes the file
- **THEN** `LocalPurl.Line` is `14`
- **AND** `LocalPurl.DeclaredText` is the trimmed element text

---

### Requirement: Unmappable Edge Cases Use Zero Values

When a parser genuinely cannot determine the source location of a dependency
(e.g., synthesized entries, corrupted manifests, or parser limitations), it
MUST set `Line` to `0` and leave `DeclaredText` empty. These cases MUST be
explicitly tested.

#### Scenario: Unmappable entry produces zero Line

- **GIVEN** a manifest or lockfile containing an entry whose line cannot be
  determined by the parser
- **WHEN** the parser processes that entry
- **THEN** `LocalPurl.Line` is `0`
- **AND** `LocalPurl.DeclaredText` is the empty string
- **AND** neither field appears in the JSON output for that entry

---

### Requirement: SCANOSS API Payload Backward Compatibility

Adding `Line` and `DeclaredText` to `LocalPurl` MUST NOT alter the HTTP request
payload sent to `api.scanoss.com`. The request body is assembled by copying only
`purl` (and `requirement` for the transitive path) into a `map[string]string`;
the new fields MUST NOT be copied into those maps.

#### Scenario: API request payload unchanged after field addition

- **GIVEN** the SDK is used to scan dependencies against the SCANOSS API
- **WHEN** `queryDirectDependenciesWithFiles` or `queryTransitiveDependenciesWithFiles`
  assembles the request body
- **THEN** the serialized JSON payload is byte-identical to the payload produced
  before the `Line` and `DeclaredText` fields were added
- **AND** no server-side validation error occurs due to unexpected fields
