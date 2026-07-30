# Changelog — SCANOSS CLI

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Fixed

- **A scan whose session expired no longer hangs.** The status endpoint reports six states; the SDK
  recognised two. `expired` is terminal — the session is gone and its id cannot be retried — but it
  fell through to "still running", so an expired scan was polled until the caller gave up. For the
  CLI that meant hanging until Ctrl-C, and `results <old-id>` never returned. A state this client
  does not recognise still means "keep waiting", so a server that adds one breaks nothing.

- **The scan progress bar no longer runs backwards.** The server scans in passes, each counting its
  own units from zero — files, then the subset needing snippet matching, then component lookups —
  and the bar was fed those numbers directly, so it reached 100%, dropped to 2%, and finished a
  successful scan drawn at 0%. It also kept the first pass's total forever, so the later passes were
  drawn against the wrong denominator. Each pass now owns a share of the bar, which only grows.
- **Every enrichment layer appears when it starts**, not when its first response arrives. They run
  concurrently but answer at very different speeds — one endpoint measured 6-10s per request against
  ~500ms for the others — so a layer that only showed up once it had an answer looked like one that
  had not started.

### Changed

- **BREAKING — progress is reported per stage, through typed contracts.** `Progress`, `ProgressFunc`
  and `WithProgress` are gone, and with them `PipelineProgress`, `DecorationPipeline.OnProgress` and
  `DecorationPipeline.Snapshot`. Two interfaces replace them:

  ```go
  type ScanReporter interface {
      Fingerprinting(done, total int)
      Uploading(done, total int)
      Scanning(env scanossapi.ScanEnvelope)   // the server's envelope, whole
  }

  type DecorationReporter interface { Decorating(service string, done, total int) }
  ```

  Each stage now has the signature of what it actually has, rather than being told apart by a unit
  string. They are registered **per call**, not per client — `WithScanReporter` is a `ScanOption`,
  `WithDecorationReporter` a new `DecorateOption` — because a client is long-lived and shared while
  an observer belongs to one operation. Every decoration method takes `opts ...DecorateOption`.
- **BREAKING — `pkg/scanpipeline` reports every layer through one channel**, `Options.OnProgress`,
  replacing `OnCollect`, `OnFingerprint`, `OnDependencies` and the SDK's separate callback. Each
  update carries a layer, a status and a counter that only grows, whichever layer produced it.
- **BREAKING — `pkg/scanpipeline` no longer knows about flags.** `Layer`, `Set` and `ParseLayers`
  moved to the CLI: `--include` values are the CLI's vocabulary. `Options` takes `Services
  []scanoss.Service` and `SourceDeclared bool`; `Build` and `Enrich` take the services and a
  reporter.
- **BREAKING — `--default-filters` is replaced by `--all-extensions` and `--all-folders`** on `scan`
  and `wfp`. The built-in lists exclude very different amounts — measured on a 7586-file tree,
  dropping the file rules admitted 4489 more files and dropping the directory rules 110 — so one
  switch for both was too blunt. `--all-extensions` covers extensions, name endings and exact names
  together. In the library, `filter.Options.Defaults` splits into `FileDefaults` and
  `FolderDefaults`.
- **BREAKING — `--all-hidden` now reaches version-control metadata.** `.git` and friends are
  excluded because they are hidden, like any other dotted entry, so the flag includes them. This
  reverses the 0.4.0 note that version-control metadata is never collected: it is now the caller's
  choice, and matches the reference implementation, where the same flag has the same reach. What a
  scan uploads is fingerprints — hashes and paths, not contents.
- The scan status is polled every **2 seconds** rather than 5. A whole pass could pass between two
  polls, freezing the display for stretches that read as a hang.
- `Filtered N files` is now written through the progress channel, so it no longer appears when the
  output is not a terminal. See the known issue below.

### Known issues

- Output written through the progress renderer — `Filtered N files` and the `Scan id` block — is not
  emitted when stderr is not a terminal, so it is missing from CI logs. The scan id is the handle
  for resuming an interrupted scan.

## [0.4.0] - 2026-07-29

### Added
- **`config`** — store `api-url`, `api-key`, `proxy` and `ca-cert` in
  `~/.scanoss/settings.json` instead of repeating flags: `config set`, `get`, `list`, `unset`,
  `path`. The API key is never displayed. Every setting resolves as
  **flag > `SCANOSS_<KEY>` > config file > default**.
- **`--proxy` and `--ca-cert`** on every command that reaches the API. `--proxy` overrides
  `HTTP_PROXY`/`HTTPS_PROXY` for one run and honours `NO_PROXY`; `--ca-cert` adds a PEM file's
  certificates to the system pool, verification still on. PAC is not supported.
- **`scanoss.NewHTTPClient`** builds an `*http.Client` with the same proxy and CA settings.
- **`--min-size` / `--max-size`** on `scan` and `wfp`, and **`--all-hidden`** to include dotfiles
  (version-control metadata stays excluded).
- **`--default-filters`, `--gitignore` and `--settings` on `wfp`**, so it collects files exactly
  the way `scan` does. **`--settings` on `dependencies`** likewise.
- **SDK:** `filter.FingerprintOptions`, `filter.DependencyOptions`, `filter.HiddenSource`,
  `settings.FingerprintFilter()`, `settings.DependencyFilter()`.

### Changed
- Files under 100 bytes are no longer skipped.
- **`pkg/filter` is the single source of filtering rules**, applied once during collection. The
  fingerprint layer no longer filters, so `GenerateWFP` and `GenerateFingerprint` fingerprint
  whatever they are given — a caller passing a list that did not come from collection now gets
  every file. **This is the only change here that fails silently rather than at compile time.**
- **SDK, breaking** — removed: `wfp.ShouldSkipFile`, `filter.NewMatcher`,
  `filter.DefaultSkippedDirs`, `filter.IngestOptions`, and `filter.Defaults`'
  `MinSize`/`MaxSize`. Use `filter.Build(filter.DefaultSource(filter.StdDefaults()))` plus
  `manifests.Is` to compose rules; `filter.CommonSkippedDirs` /`ScanOnlySkippedDirs`
  /`DependencyOnlySkippedDirs`; `filter.DependencyOptions`; and `filter.Options.MinSize`/`MaxSize`.
  Callers starting from `DefaultOptions`/`ScanOptions` need no change.

### Fixed
- **Version-control metadata is never collected.** `.git` could be fingerprinted and uploaded when
  the built-in filters and the hidden rule were both off — including `.git/config`, which can carry
  credentials.
- **License identifiers are validated against the SPDX list.** A non-canonical id is normalised;
  an unrecognised one, or a malformed expression, becomes a declared `LicenseRef` instead of
  producing an SBOM that fails validation.
- **SBOM packages carry the component name**, not the full PURL. CycloneDX gained a
  `serialNumber`, and the tool version moved to its own field.
- **`results` returns the same inventory as `scan`**, so a resumed scan can be rendered and
  converted. It accepts `--format` and `--include`.
- **`--default-filters=false` really disables them** — extension-skipped files were dropped anyway
  by a second, uncounted filter in the fingerprint layer.
- **`dependencies` honours `scanoss.json`** and reports what it filtered; `scan --include deps`
  and `dependencies` no longer disagree over `examples/`. A `skip` rule now also overrules the
  manifest exemption, so excluding a manifest by name works.
- `--min-size`/`--max-size` were ignored together with `--default-filters=false`.
- Zero-byte files and symbolic links are no longer fingerprinted, and the raw format emits
  `"components": []` rather than `null` when nothing matched.

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
