# Changelog — SCANOSS CLI

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Fixed

- **A transient network error no longer discards a whole scan.** A DNS blip on one WFP
  chunk used to fail the entire upload, and one while polling threw away work the server
  had already done. Network errors, truncated responses and 429/5xx are now retried with
  exponential backoff; a `Retry-After` is still obeyed first.
- **A chunk the server already holds no longer fails the scan.** When a retry's predecessor
  landed but its response was lost, the server answers `409 RANGE_CONFLICT` — the upload is
  complete, so it now counts as success and the scan proceeds to polling.
- **A partial enrichment no longer passes for a complete one.** When some requests of a layer
  fail, the components left without data were indistinguishable from components the service had
  nothing to say about; they are now named on stderr.

### Added

- **`Config.RetryBackoffBase`** — the first backoff wait, doubled per attempt (default 250ms).

### Changed

- **`Config.MaxRetries` covers every transient failure**, not only a 429/503 carrying
  `Retry-After`. A negative value now disables retries, as `Timeout` does.
- **BREAKING — `Config.MaxRetryAfter` is now `Config.MaxServerRetryWait`**: it bounds the
  wait the server asks for, not the one the SDK computes.
- **BREAKING — `scansource.LicenseKey` is now `scansource.Key`**: it keys a component at a
  version, which every per-component layer joins on, not just licenses.
- **BREAKING — the decoration pipeline result is typed per layer.** `PipelineResult.Services`
  (a `map[string]*Result`) gives way to one field per layer — `Licenses`, `Cryptography`,
  `Geoprovenance`, `Vulnerabilities` — each a `*Layer[T]` holding the decoded response and the
  chunks it lost. A response that cannot be decoded is now recorded in `Errors`.
- **BREAKING — `ChunkError.Index` is now `ChunkError.Purls`**: it names the components left
  without data instead of an internal batch number a caller cannot act on.

### Removed

- **`scanoss.As`, `scanoss.Result` and its methods are no longer exported.** No exported function
  returned a `*Result`, so the pair documented an API a caller had no way to obtain a value for.
  `Result.String` and `Result.Responses` had no callers at all; `Unmarshal` existed only for `As`.

### Fixed

- **A component's `rank`, `release_date` and `artifact_name` reach the inventory.** All three
  were dropped, so two components matching the same file looked equally strong.
- **A request is bounded by a 120s timeout** (`Config.Timeout`; negative disables it).
  Waiting for a server that accepted the request and went quiet had no bound before.
- **Any 2xx counts as success.** A `201` or `204` used to surface as an error.

### Removed

- **The `attributions` command** — `POST /sbom/attribution` is not in the v3 contract.
  Use `licenses attribution`.
- **`pkg/api` and `pkg/batch`** — a second transport stack with no context, retries or
  timeout, used only by the C bindings, which now go through `pkg/scanoss`.
- **`scanoss.NewHTTPClient` and `scanoss.HTTPClientOptions`** are now private.

### Changed

- **`scanoss.New(Config) (*Client, error)`** replaces `scanoss.Option` and the `With*`
  client options; `Proxy`, `CACertFile`, `InsecureTLS` and `Timeout` are now fields.
- **`WithScanIDNotify` is a `ScanOption`**, passed to the scan call, not to the client.

## [0.5.0] - 2026-07-30

### Added

- **`bom.replace` is applied.** A scan result whose match is covered by a replace rule is now
  re-pointed at the rule's `replace_with` component; previously the rule's PURL was sent to the API
  but the result was never rewritten, so the section did half its job in silence. Rules are scoped
  by `purl`, `path` or both, and where several cover one file the most specific wins. Applied after
  `bom.remove`, and only to matches that survived it. The `license` field of a replace rule is not
  supported yet.

### Fixed

- **The scan progress bar no longer runs backwards.** The server scans in passes that each restart
  their own counter, and the bar was fed those numbers directly: it reached 100%, dropped back, and
  finished a successful scan drawn near empty.
- **A scan whose session expired no longer hangs.** `expired` is terminal, but was treated as "still
  running", so the CLI polled a dead session until interrupted.
- **A command invoked without its required argument now fails.** It printed its help to stdout and
  exited `0`, so `scanoss-cli scan $DIR` with an unset variable reported success having scanned
  nothing and fed usage text to whatever consumed the results. Help now goes to stderr and the exit
  code is non-zero. A namespace command with no subcommand (`scanoss-cli config`) still succeeds:
  it was not asked to do anything.
- **`scanpipeline.Run` rejects a nil `Options.Client` instead of panicking.** The client was only
  reached inside the scan goroutine, so the panic could not be recovered by the caller and `Run`
  returned a nil error as the process was torn down.
- **Upload progress is reported once per block, in order.** Every upload worker reported
  concurrently, so `ScanReporter.Uploading` received duplicated and out-of-order counts: an upload
  bar ran backwards and settled below 100% because the last report was not the highest. Calls are
  now serialised, which also means a reporter needs no lock of its own.
- Enrichment layers appear when they start rather than when their first response arrives.

### Changed

- **BREAKING — progress is reported per stage.** `Progress`, `ProgressFunc`, `WithProgress`,
  `PipelineProgress`, `DecorationPipeline.OnProgress` and `.Snapshot` are removed. Use
  `ScanReporter` and `DecorationReporter`, registered per call with `WithScanReporter` and
  `WithDecorationReporter`; every decoration method takes `opts ...DecorateOption`.
- **BREAKING — `pkg/scanpipeline` reports every layer through `Options.OnProgress`**, replacing
  `OnCollect`, `OnFingerprint`, `OnDependencies` and the SDK's separate callback.
- **BREAKING — `pkg/scanpipeline` no longer parses `--include`.** `Layer`, `Set` and `ParseLayers`
  moved to the CLI; `Options` takes `Services` and `SourceDeclared`; `Build` and `Enrich` changed
  signature.
- **BREAKING — `--default-filters` is replaced by `--all-extensions` and `--all-folders`** on `scan`
  and `wfp`. In the library, `filter.Options.Defaults` splits into `FileDefaults` and
  `FolderDefaults`.
- The scan status is polled every 2 seconds rather than 5.

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

[0.6.0]: https://github.com/scanoss/scanoss.go/compare/v0.5.0...v0.6.0
[0.5.0]: https://github.com/scanoss/scanoss.go/compare/v0.4.0...v0.5.0
[0.4.0]: https://github.com/scanoss/scanoss.go/compare/v0.3.0...v0.4.0
[0.3.0]: https://github.com/scanoss/scanoss.go/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/scanoss/scanoss.go/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/scanoss/scanoss.go/releases/tag/v0.1.0
