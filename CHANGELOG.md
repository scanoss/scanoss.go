# Changelog — SCANOSS CLI

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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
- **`--all-hidden`** on `scan` and `wfp` — fingerprint dotfiles and dotted folders. `.git` stays
  excluded either way.
- **`--settings` on `dependencies`**, which had no way to point at a `scanoss.json`.
- **`filter.FingerprintOptions`, `filter.DependencyOptions`, `filter.HiddenSource`** and
  **`settings.DependencyFilter()`** — one collection profile per layer, and the hidden-entry rule
  as a source like the others.

### Changed
- Files under 100 bytes are no longer skipped
- **`pkg/filter` is the single source of filtering rules**, applied once during collection. The
  fingerprint layer no longer filters, so `GenerateWFP` and `GenerateFingerprint` fingerprint
  whatever they are given — a caller passing a list that did not come from collection now gets
  every file. This is the only change here that fails silently rather than at compile time; the
  documented path (`scanner.CollectFiles*` first) is unaffected.
- **SDK, breaking** — removed: `wfp.ShouldSkipFile`, `filter.NewMatcher`,
  `filter.DefaultSkippedDirs`, `filter.IngestOptions`, and the `MinSize`/`MaxSize` fields of
  `filter.Defaults`. Replacements: compose rules with
  `filter.Build(filter.DefaultSource(filter.StdDefaults()))` plus `manifests.Is`; use
  `filter.CommonSkippedDirs`/`ScanOnlySkippedDirs`/`DependencyOnlySkippedDirs`,
  `filter.DependencyOptions`, and `filter.Options.MinSize`/`MaxSize` or `filter.SizeSource`.
  Callers that start from `DefaultOptions`/`ScanOptions` need no change.

### Fixed
- Zero-byte files and symbolic links are no longer fingerprinted. An empty file produced a WFP
  entry with a zero hash and no lines, and a link reported the same content twice — its target is
  collected on its own. Neither is configurable: they state a fact about the entry rather than a
  policy about the scan.
- `--max-size` (and the new `--min-size`) were silently ignored when combined with
  `--default-filters=false`: the bounds were applied from inside the built-in default filters, so
  switching those off discarded them. They are now applied independently.
- `--default-filters=false` now really disables them. Files skipped only by extension were dropped
  anyway, because the fingerprint layer kept its own copy of the extension list and re-applied it
  after collection — without counting them as filtered.
- `dependencies` honours `scanoss.json`. `skip.patterns.dependencies` is part of the published
  schema and did nothing: the command walked the tree itself and never read the file. It now uses
  the shared filter and reports what it skipped.
- `scan --include deps` and `dependencies` no longer disagree: the first inherited the scanning
  profile when collecting manifests, so it skipped `examples/` while the other did not.
- Hidden *directories* were pruned before the filtered counter ran, so they never appeared in
  "Filtered N files" while hidden files did.
- `results` emitted the API response verbatim instead of the inventory `scan` produces, so a
  resumed scan could not be rendered or converted (`sbom results.json --format cyclonedx` failed).
  It now returns the same document and accepts `--format` and `--include`.
- The raw format emitted `"components": null` when a scan matched nothing, instead of an empty
  list. CycloneDX and SPDX were already correct.
- The `libscanoss` examples pointed at an endpoint this CLI does not support, and passed a full
  URL where the API expects a base one — so the path was duplicated. They now use
  `https://api.scanoss.com`.


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
