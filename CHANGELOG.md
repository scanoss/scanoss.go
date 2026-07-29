# Changelog — SCANOSS CLI

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Fixed
- **`--default-filters=false` now really disables the default filters.** Files skipped only by
  extension (`.png`, `.md`, …) were dropped anyway: the flag did switch them off during
  collection, but the fingerprint layer kept its own copy of the extension list and discarded them
  again afterwards — without counting them as filtered.
- **`dependencies` honours `scanoss.json`.** `skip.patterns.dependencies` is part of the published
  settings schema and did nothing: the command walked the tree itself and never read the file. It
  also honours the shared skip lists now, and reports what it filtered.
- **`scan --include deps` and `dependencies` no longer disagree.** The first inherited the scanning
  profile when collecting manifests, so it skipped `examples/` while the standalone command did
  not. Manifest collection is its own stage with its own profile, and both find the same set.

### Changed
- **SDK, breaking** — `pkg/filter` is now the only place that holds filtering rules, and filtering
  happens once, during collection:
  - `wfp.ShouldSkipFile` and its extension list are **removed**. `GenerateWFP` and
    `GenerateFingerprint` no longer filter: a caller passing a list that did not come from
    collection now gets every file fingerprinted. The documented path — `scanner.CollectFiles*`
    first — is unaffected. **This is the only change that fails silently rather than at compile
    time.**
  - `filter.NewMatcher` is **removed**. It only ever answered for the entry, not the path, so a
    caller outside a tree walk got half a filter and had to rebuild the other half. Filtering a
    tree is this package's job (`filter.Collect`); filtering an archive being extracted is the
    caller's. Compose the rules from the exported pieces —
    `filter.Build(filter.DefaultSource(filter.StdDefaults()))`, plus `manifests.Is` for the
    manifest exemption — and apply them with your own traversal.
  - `filter.DefaultSkippedDirs` is **removed**, replaced by `CommonSkippedDirs`,
    `ScanOnlySkippedDirs` and `DependencyOnlySkippedDirs`. With three lists, "default" named none
    of them. `CommonSkippedDirs` is what a pre-filter can safely prune before it knows which
    operation will consume the result.
  - `filter.IngestOptions` is **removed**, replaced by `filter.DependencyOptions`. Two dependency
    profiles were an artefact of two entry points.

### Added
- **`--all-hidden`** on `scan` and `wfp` — fingerprint dotfiles and dotted folders, which were
  excluded with no way to opt in. `.git` stays excluded either way: on a working checkout it is
  usually larger than the project and holds compressed objects nothing can match.
- **`filter.HiddenSource` and `filter.Options.IncludeHidden`** — the hidden-entry rule is a source
  like the others instead of a check buried in the walk, so a caller applying the rules itself can
  apply this one too. It also closes a hole in the reported count: hidden *directories* were pruned
  before the counter, so they were never included in "Filtered N files".
- **`filter.FingerprintOptions` and `filter.DependencyOptions`** — one collection profile per
  layer, so each states which rules it uses instead of restating what they are.
- **`settings.DependencyFilter()`** alongside `ScanFilter` and `FingerprintFilter`.
- **`--settings` on `dependencies`**, which had no way to point at a `scanoss.json`.

## [0.4.0] - 2026-07-29

### Added
- **`config`** — store settings in `~/.scanoss/settings.json` instead of repeating flags:
  `config set`, `get`, `list`, `unset`, `path`. Keys are `api-url`, `api-key`, `proxy` and
  `ca-cert`; the file itself uses `snake_case`. The API key is never displayed.
- Every setting resolves as **flag > `SCANOSS_<KEY>` > config file > built-in default**, so a
  stored `proxy` or `ca-cert` also applies with no flag. `--ignore-cert-errors` is not storable.
- **`--proxy` and `--ca-cert`** on every command that reaches the API. `--proxy` overrides
  `HTTP_PROXY`/`HTTPS_PROXY` for one run and still honours `NO_PROXY`; `--ca-cert` trusts a PEM
  file's certificates in addition to the system pool, with verification still on. PAC is not
  supported.
- **`scanoss.NewHTTPClient`** builds an `*http.Client` with the same proxy and CA settings, for use
  with the existing `WithHTTPClient` option. `pkg/api` gained `SetHTTPClient`.
- **`--min-size`** on `scan` and `wfp` — skip files below a size in bytes.
- **`--max-size` on `wfp`**.
- **`--default-filters`, `--gitignore` and `--settings` on `wfp`** — it now collects files exactly
  the way `scan` does, so the two no longer disagree on which files they cover. `scanoss.json` is
  read for the **fingerprinting** operation (`skip.patterns.fingerprinting`), not the scanning one.
- **`settings.FingerprintFilter()`** alongside `ScanFilter()`, for SDK callers that fingerprint.

### Changed
- Files under 100 bytes are no longer skipped
- **SDK, breaking** — `filter.Defaults` no longer carries `MinSize`/`MaxSize`, and neither does
  `filter.StdDefaults()`. Size is caller input, not a built-in skip list: set
  `filter.Options.MinSize`/`MaxSize`, or build the matcher directly with the new
  `filter.SizeSource(min, max)`. `filter.DefaultOptions`/`ScanOptions`/`IngestOptions` carry the
  built-in bounds, so callers that start from a constructor need no change.

### Fixed
- Zero-byte files and symbolic links are no longer fingerprinted. An empty file produced a WFP
  entry with a zero hash and no lines, and a link reported the same content twice — its target is
  collected on its own. Neither is configurable: they are not policy, and both match what the
  Python and JS clients already do.
- `--max-size` (and the new `--min-size`) were silently ignored when combined with
  `--default-filters=false`: the bounds were applied from inside the built-in default filters, so
  switching those off discarded them. They are now applied independently, in both the directory
  walk and the streaming-extraction path.


## [0.3.0] - 2026-07-28

### Changed
- **BREAKING** — the raw inventory format now reports matched line ranges as structured objects
  (`{"start_line": 82, "end_line": 209}`) instead of `"82-209"` strings, matching the shape the scan
  engine already returns. `schema_version` is `2.0`; a `1.0` document carrying string ranges no
  longer parses. CycloneDX and SPDX output is unchanged.

## [0.2.0] - 2026-07-27

### Changed

- Renamed the CLI binary from `scanoss` to `scanoss-cli`. The `go install` path is now
  `go install github.com/scanoss/scanoss.go/cmd/scanoss-cli@latest`.

## [0.1.0] - 2026-07-16

Initial release of the SCANOSS Go CLI and SDK (`scanoss`).

### Added

- **`scan <path>`** — fingerprint a file or folder (WFP winnowing), upload to the SCANOSS v3 batch
  API in parallel chunks, and poll to completion; an interrupted scan resumes with
  `scanoss results <scan-id>`. `--format raw|spdx|cyclonedx` selects the output and
  `--include deps,vulns,licenses,crypto,geo` opts into extra layers, narrowed to what the chosen
  format can render (a layer it can't represent is skipped with an up-front notice).
  `scan wfp <file>` scans a pre-generated WFP.
- **`wfp <path>`** — generate WFP fingerprints only, without uploading.
- **`results <scan-id>`** — resume or poll a previous scan by its id.
- **`sbom <input>`** — offline: produce an SBOM from a scanoss raw inventory, or convert between
  CycloneDX 1.7 and SPDX 2.3 (input format detected from content). No API calls; best-effort (data
  a target can't represent — e.g. SPDX vulnerabilities — is dropped with a warning).
- **`enrich <input>`** — online: decorate an existing raw inventory, CycloneDX, or SPDX with the
  purl-keyed layers `vulns`/`licenses`/`crypto`/`geo`, with no source tree or re-scan (re-runnable
  to refresh). The output format defaults to the input's; `--format` converts in the same pass.
- **`dependencies [path]`** — parse local manifests, or query direct/transitive dependencies for a
  PURL.
- **`attributions [sbom]`** — attribution text from an SBOM file or a PURL.
- **Decoration commands** over the SCANOSS Services API v3: `vulnerabilities`, `licenses`,
  `cryptography`, `geoprovenance`, `copyright`, and `components`.
- **`raw` output** (default) — a neutral inventory in a versioned envelope (`schema_version` +
  `metadata` + `components` + `vulnerabilities`): detected and declared components in one
  `components` list tagged by `scope`, per-component layers (`licenses`, `cryptography`,
  `geoprovenance`) inline, and vulnerabilities as a flat top-level list. Each component records
  where it came from in `evidence` — scanned files that matched (`match_type` `file`/`snippet`) or
  the manifest that declared it (`match_type` `declared`). A bare scan (no `--include`) makes no
  decoration calls.
- **SBOM export** — SPDX 2.3 and CycloneDX 1.7, with the SCANOSS `url_hash` preserved (an SPDX
  `OTHER` external reference / a CycloneDX `scanoss:url_hash` property) and vulnerability detail
  (CVSS score/vector/method, CWE, EPSS).
- **Declared dependencies** (`--include deps`) are sourced directly from the project's manifests
  (`package.json`, `go.mod`, …) and decorated alongside the scan matches — no dependency-resolution
  round trip.
- **File filtering** — built-in defaults, `.gitignore`, and `scanoss.json` rules; client-side
  `bom.remove`.
- **Progress & status output** — live progress bars for every phase (fingerprint, upload, server,
  dependency parsing, each enrichment layer); status notices with icons and color (`⚠` warnings,
  `ℹ` info, `✓` success) on an interactive terminal (Windows 10+ included), disabled when
  piped/redirected or `NO_COLOR` is set; a `Results written to <path>` line for `--output`.
- **`-v, --verbose`** — structured `log/slog` debug output on stderr (the scan flow, each API
  request with method/URL/status/duration, fingerprinting, and decoration).
- **Go SDK** (`pkg/scanoss`) — scan and decoration services with a parallel decoration pipeline;
  `pkg/scanpipeline` assembles a neutral `sbom.Inventory`, and `pkg/sbom` reads and writes
  CycloneDX and SPDX (with `WithTool`/`WithAuthor`/`WithTimestamp` document-metadata options).
- **C shared library** (`libscanoss`) with Node.js and Python bindings.

[0.4.0]: https://github.com/scanoss/scanoss.go/compare/v0.3.0...v0.4.0
[0.3.0]: https://github.com/scanoss/scanoss.go/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/scanoss/scanoss.go/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/scanoss/scanoss.go/releases/tag/v0.1.0
