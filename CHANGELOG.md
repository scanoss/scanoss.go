# Changelog — SCANOSS CLI

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- **`config`** — store the API endpoint and key once in `~/.scanoss/settings.json` instead of
  passing `--api-key` on every command: `config set <key> <value>`, `config get <key>`,
  `config list`, `config unset <key>`, and `config path`. Keys are named `api-url` and `api-key`,
  the same as the flags; the file itself stores them `snake_case` (`api_url`, `api_key`). The file
  is created on first `config set` with mode `0600` in a `0700` directory, and keys it does not
  recognize are preserved on write. `config set api-url` requires an `https://` or `http://`
  scheme, so a bare host is rejected where it can still be fixed rather than failing later as an
  unsupported-protocol error from another command.
- Every command that accepts `--api-url`/`--api-key` now resolves each value as
  **flag > environment variable > `~/.scanoss/settings.json` > built-in default**. The environment
  variables are `SCANOSS_API_URL` and `SCANOSS_API_KEY`; passing the flag still overrides both. A
  stored key satisfies the no-key check, so the "no API key provided" banner no longer appears for
  a configured user. The API key is never displayed: `config list` and `config get` render it as
  `********`, with no flag to reveal it, and `--verbose` logs which source won without the value.

- **`--proxy` and `--ca-cert`** on every command that reaches the API. `--proxy <url>` overrides
  `HTTP_PROXY`/`HTTPS_PROXY` for one run and requires an `https://` or `http://` scheme;
  `--ca-cert <path>` trusts the certificates in a PEM file **in addition to** the system pool, so
  an internal endpoint and the public API both keep verifying. Verification stays on, which makes
  it the alternative to `--ignore-cert-errors` rather than a variation of it. Proxy
  auto-configuration (PAC) is not supported.
- **`scanoss.NewHTTPClient`** in the SDK builds an `*http.Client` from the same settings, for use
  with the existing `WithHTTPClient` option. `pkg/api` gained `SetHTTPClient` so the C, Python and
  Node bindings can be handed a configured client.

### Fixed

- `--ignore-cert-errors` no longer disables proxy support. The client it built came from a
  hand-made `http.Transport`, whose nil `Proxy` bypassed `HTTP_PROXY`/`HTTPS_PROXY` entirely — so
  anyone behind a proxy who used the flag to get past a certificate error had been silently going
  direct. The transport is now cloned from `http.DefaultTransport`, which keeps Go's proxy
  handling along with its timeouts and connection pooling. The same fix applies to the SDK's
  `WithInsecureTLS` and to `pkg/api`.

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

[0.2.0]: https://github.com/scanoss/scanoss.go/releases/tag/v0.2.0
[0.1.0]: https://github.com/scanoss/scanoss.go/releases/tag/v0.1.0
