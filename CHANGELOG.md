# Changelog — SCANOSS CLI

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- **`convert`** — offline conversion between the raw scanoss result, CycloneDX 1.7, and
  SPDX 2.3 (`scanoss convert <input> --format cyclonedx|spdx`). No scanning or API calls;
  the input format is detected from the file content. Best-effort: SPDX cannot represent
  vulnerabilities, so they are dropped (with a warning) when converting to spdx.

### Fixed
- SBOM export no longer emits the scanoss `url_hash` (a CRC64) as an `MD5` package
  checksum, which produced invalid-length checksums in SPDX. The `url_hash` is now
  preserved as metadata: an `OTHER` external reference in SPDX and a `scanoss:url_hash`
  property in CycloneDX.

## [0.1.0] - 2026-07-14

Initial public release of the SCANOSS Go CLI and SDK (`scanoss`).

### Added
- **Scan** a file or folder against the SCANOSS API (`scanoss scan`) with a
  streaming fingerprint → upload → poll pipeline; interrupted scans resume with
  `scanoss results <scan-id>`.
- **WFP fingerprinting** (`scanoss wfp`) using winnowing over a whole-file CRC64 hash.
- **Decoration commands** over the SCANOSS Services API v3: `vulnerabilities`,
  `licenses`, `cryptography`, `geoprovenance`, `copyright`, and `components`.
- **Dependencies** parsing and lookup (`scanoss dependencies`) and **attribution**
  generation (`scanoss attributions`).
- **SBOM export** in SPDX 2.3 and CycloneDX 1.7 (`scan --format spdx|cyclonedx`).
- **File filtering** via built-in defaults, `.gitignore`, and `scanoss.json`
  rules; client-side `bom.remove`.
- **Go SDK** (`pkg/scanoss`) and a **C shared library** (`libscanoss`) with
  Node.js and Python bindings.
- `-v, --verbose` enables structured debug logging (standard-library
  `log/slog`) to stderr — the scan flow, each API request (method/URL/status/
  duration), fingerprinting, and decoration. 

[0.1.0]: https://github.com/scanoss/scanoss.go/releases/tag/v0.1.0
