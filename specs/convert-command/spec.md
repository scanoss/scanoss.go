# Feature Specification: offline SBOM/result format conversion (`scanoss convert`)

**Feature branch:** `feat/convert-command`
**Issue:** _to be created_
**Status:** Draft

## Summary
A new **offline** `scanoss convert` command that transforms an existing file between the
three formats the tool already understands — the raw SCANOSS v3 scan result, CycloneDX 1.7,
and SPDX 2.3 — **without scanning and without any API call**. It lets a user produce a
second format from data they already have, instead of re-scanning.

Conversion goes through the existing neutral `sbom.Inventory`: the input is parsed into an
`Inventory` (using the official libraries' decoders), then rendered to the target format
with the existing `sbom.Generate`. The only genuinely new code is the **readers**
(CycloneDX/SPDX → `Inventory`); the writers and the raw adapter already exist.

## Background / current state
- `pkg/sbom` today is **write-only**: `Generate(inv, format)` renders an `Inventory` to
  CycloneDX 1.7 (`buildCycloneDX`) or SPDX 2.3 (`buildSPDXLite`) via `cyclonedx-go` and
  `spdx/tools-golang`. There is no path from a document back to an `Inventory`.
- The raw path already exists: `scansource.FromScanResult(*scanossapi.ScanResult)` builds an
  `Inventory` from a v3 scan result.
- Both official libraries expose decoders (the inverse of the encoders we already use):
  `cdx.NewBOMDecoder(r, cdx.BOMFileFormatJSON).Decode(&bom)` and `spdxjson.Read(r)`. Parsing
  is done by the libraries; we only map their typed structs into `Inventory`. **No hand JSON
  parsing, no new dependencies.**

## Command surface
```
scanoss convert <input.json> --format <cyclonedx|spdx> [--output <file>]
```
- `<input.json>` — a raw scanoss v3 result, a CycloneDX JSON, or an SPDX JSON.
- `-f, --format` — target format: `cyclonedx` or `spdx` (not `raw`; converting *to* the raw
  scan result is not meaningful).
- `-o, --output` — output file (default: stdout).
- **Offline**: no `--api-url`/`--api-key`, no network, no fingerprinting.

Examples:
```
scanoss convert bom.spdx.json --format cyclonedx -o bom.cdx.json   # spdx  → cyclonedx
scanoss convert bom.cdx.json  --format spdx      -o bom.spdx.json  # cyclonedx → spdx
scanoss convert result.json   --format cyclonedx -o bom.cdx.json   # scanoss raw → cyclonedx
scanoss convert result.json   --format spdx      -o bom.spdx.json  # scanoss raw → spdx
```

## Input format identification (a content check, not a flag)
There is **no `--from` flag**. The command inspects the input's content and identifies the
format by an unambiguous marker; if none matches, it fails with a clear error.

| Input | Detected by |
|---|---|
| cyclonedx | top-level `"bomFormat": "CycloneDX"` |
| spdx | top-level `"spdxVersion": "SPDX-…"` |
| raw (scanoss v3) | scanoss result shape (`components`/`files`) and none of the above |

Unrecognized input → `error: unrecognized input: not a scanoss result, CycloneDX, or SPDX
document`.

## How it works — the neutral hub
```
<input> ──identify──▶ decode (official lib) ──map──▶ sbom.Inventory ──Generate(target)──▶ <output>
```
- **raw** → `scansource.FromScanResult` (exists).
- **cyclonedx** → `cdx.NewBOMDecoder` → `cdx.BOM` → `sbom.ParseCycloneDX` → `Inventory` (new).
- **spdx** → `spdxjson.Read` → `v2_3.Document` → `sbom.ParseSPDX` → `Inventory` (new).
- Output → `sbom.Generate(inv, target)` (exists): CycloneDX **1.7**, SPDX **2.3**, regardless
  of the input document's version (the decoders accept older minor versions; output is
  normalized).

## Fidelity — best-effort through the neutral model
Conversion is **best-effort, not a byte-exact round-trip**. Anything the `Inventory` does not
model is dropped. For v1 the readers map into the **current** `Inventory` (components,
licenses, vulnerabilities), matching what the writers already emit — so a value produced by
this tool round-trips, but fields outside the model (e.g. copyright, CVSS vectors, component
type) are not preserved. Extending `Inventory` for higher fidelity is an explicit open
decision (below), kept separate so it can also benefit the `scan` path.

### Lossy conversions warn
- **cyclonedx → spdx**: vulnerabilities are dropped (SPDX 2.3 has no vulnerability model) →
  `warning: spdx cannot represent vulnerabilities; N dropped`.
- Any layer the target format cannot represent warns the same way; the conversion still
  succeeds with what the target supports.

## User scenarios & acceptance
1. **Given** a CycloneDX JSON, **when** `convert x.cdx.json --format spdx`, **then** a valid
   SPDX 2.3 document is written; if the input had vulnerabilities, a warning notes they were
   dropped.
2. **Given** an SPDX JSON, **when** `convert x.spdx.json --format cyclonedx`, **then** a valid
   CycloneDX 1.7 document is written (no vulnerabilities, since SPDX had none).
3. **Given** a raw scanoss v3 result, **when** `convert result.json --format cyclonedx|spdx`,
   **then** the corresponding SBOM is written (same output as `scan -f <format>` would for
   the same result, minus any decoration the raw file didn't already contain).
4. **Given** an unrecognized or malformed file, **then** the command errors clearly without
   writing output.
5. **Given** `--format raw` (or any non-cyclonedx/spdx target), **then** the command errors
   up front.
6. **Round-trip:** a document produced by this tool's writer, read back and re-written, is
   semantically equal at the `Inventory` level (test guarantee).

## Requirements
### Functional
- **FR-001 (command)** Add a top-level, **offline** `convert <input> --format <target>
  [-o <file>]` command. No auth/URL/network flags. Target validated up front to
  `cyclonedx|spdx`.
- **FR-002 (identification)** Identify the input format from content markers (CycloneDX
  `bomFormat`, SPDX `spdxVersion`, else scanoss v3 shape). Unrecognized input → clear error,
  no output. No `--from` flag.
- **FR-003 (CycloneDX reader)** `sbom.ParseCycloneDX([]byte) (Inventory, error)` decodes with
  `cdx.NewBOMDecoder` and maps `bom.Components` → `Component` and `bom.Vulnerabilities` →
  `Vulnerability` (inverse of `buildCycloneDX`). Pure; imports only `cyclonedx-go`.
- **FR-004 (SPDX reader)** `sbom.ParseSPDX([]byte) (Inventory, error)` decodes with
  `spdxjson.Read` and maps `doc.Packages` → `Component` (purl from `PACKAGE-MANAGER`
  externalRef, version, supplier, licenses split from the `AND`-joined declared/concluded
  expressions) (inverse of `buildSPDXLite`). Pure; imports only `spdx/tools-golang`.
- **FR-005 (offline)** No network, no fingerprinting, no SDK scan client. `pkg/sbom` readers
  do not import `pkg/scanoss`; the raw path reuses `scansource.FromScanResult`.
- **FR-006 (lossy warnings)** When the target format cannot represent a populated layer
  (e.g. vulnerabilities for spdx), emit one warning per dropped layer and continue.
- **FR-007 (target validation)** `--format` accepts only `cyclonedx`/`spdx`; anything else
  (incl. `raw`/`plain`) errors before reading the input.

### Non-functional
- **NFR-001** No new dependencies — reuse `cyclonedx-go` and `spdx/tools-golang`.
- **NFR-002** `pkg/sbom` stays pure (imports only the two SBOM libraries); readers live
  beside the writers so a format change and its reader/writer stay together.
- **NFR-003** `make check` clean; `go test ./... -race` clean.

## Open decisions
1. **Fidelity bar.** v1 = current `Inventory` model (minimal), documented best-effort.
   Optional "balanced" upgrade: add `Component.{Type,Description,Copyright,Checksums,CPE}` and
   `Vulnerability` CVSS (+ writer support) — improves round-trip but touches the existing
   writers and the `scan` path. **Recommend minimal for v1**, balanced as a separate
   follow-up. _Confirm._
2. **Dependencies layer.** If/when `Inventory.Dependencies` lands (issue #4), the readers
   should also map CycloneDX `dependencies[]` / SPDX `DEPENDS_ON`. v1 does **not** depend on
   #4; deps are simply not carried until that model exists.
3. **Format identification location.** A small helper in `cmd/convert.go` (it must recognize
   the scanoss raw shape, which is CLI/SDK territory). _Recommended._

## Out of scope
- XML (CycloneDX), and SPDX tag-value / 2.2 / 3.0 input or output (JSON-only, SPDX 2.3,
  CycloneDX 1.7 for v1).
- Converting *to* the raw scanoss result.
- Any network enrichment (fetching licenses/vulnerabilities/dependencies) — that is the
  `scan` path (issue #4). `convert` only reshapes what is already in the file.
- Byte-exact round-trip or preserving fields outside the `Inventory` model (see Open
  decision 1).
