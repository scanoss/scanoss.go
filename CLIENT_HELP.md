# SCANOSS Go Client — Usage Help

Task-oriented recipes for the `scanoss` CLI and Go SDK. For the exhaustive flag
list of any command, run `scanoss <command> --help`. For an overview and
installation, see the [README](README.md).

- [Global options](#global-options)
- [Authentication & endpoints](#authentication--endpoints)
- [TLS / certificates](#tls--certificates)
- [Fingerprinting (`wfp`)](#fingerprinting-wfp)
- [Scanning (`scan`)](#scanning-scan)
- [Resuming a scan (`results`)](#resuming-a-scan-results)
- [SBOM (`sbom`)](#sbom-sbom)
- [Enrich (`enrich`)](#enrich-enrich)
- [Dependencies (`dependencies`)](#dependencies-dependencies)
- [Attributions (`attributions`)](#attributions-attributions)
- [Decoration commands](#decoration-commands)
- [`scanoss.json` reference](#scanossjson-reference)
- [Default values](#default-values)
- [Go SDK](#go-sdk)

## Global options

- `-v, --verbose` — enable debug logging to **stderr** (default: warnings and
  errors only). Logs never go to stdout, so they don't interfere with `--output`
  or piped results. Shows the scan flow, each API request (method/URL/status/
  duration), fingerprinting, and more.
- `--version` — print the version (single-sourced from the git tag).
- `--help` — help for any command or subcommand.

```bash
scanoss --help
scanoss scan --help
scanoss --version
```

## Authentication & endpoints

The default endpoint is `https://api.scanoss.com` and **requires an API key**. A
custom endpoint (e.g. an on-prem deployment) via `--api-url` may run keyless.

```bash
# Default endpoint: pass a key (a subscription is required)
scanoss scan ./my-project --api-key "$SCANOSS_API_KEY"

# Reference the key from the environment
scanoss vulnerabilities --purl 'pkg:github/scanoss/engine' --api-key "$SCANOSS_API_KEY"

# Custom / on-prem endpoint may run without a key
scanoss scan ./my-project --api-url https://scanoss.internal.example.com
```

> **Note:** targeting the default endpoint without `--api-key` fails fast with a
> banner (no request is sent). On a `401`, the CLI prints an "Unauthorized" hint.

## TLS / certificates

For self-signed or internal endpoints you can skip TLS verification. There are no
proxy/PAC/custom-CA options — only:

```bash
scanoss scan ./my-project \
  --api-url https://scanoss.internal.example.com \
  --ignore-cert-errors        # INSECURE: disables TLS verification
```

## Fingerprinting (`wfp`)

Generate WFP fingerprints without contacting the API.

```bash
# Fingerprint a folder (to stdout)
scanoss wfp ./my-project

# Fingerprint a single file, save to a .wfp file
scanoss wfp ./src/main.go > main.wfp

# More workers
scanoss wfp ./my-project --threads 20
```

Flags: `-t, --threads` (10), `-o, --output`.

## Scanning (`scan`)

Fingerprint a folder or file, upload the WFP to the SCANOSS v3 API, and poll until
the scan completes.

```bash
# Scan a folder
scanoss scan ./my-project --api-key "$SCANOSS_API_KEY"

# Scan a single file
scanoss scan ./src/main.go --api-key "$SCANOSS_API_KEY"

# Save results to a file
scanoss scan ./my-project --api-key "$SCANOSS_API_KEY" --output results.json
```

**Tune workers and the upload block size.** The assembled WFP is uploaded in
parallel blocks; `--chunk-size` sets the block size in bytes.

```bash
scanoss scan ./my-project \
  --api-key "$SCANOSS_API_KEY" \
  --threads 20 \
  --chunk-size 2097152          # 2 MiB blocks (default 1 MiB)
```

**Keep the generated WFP** alongside the scan:

```bash
scanoss scan ./my-project --api-key "$SCANOSS_API_KEY" --save-wfp project.wfp
```

**Scan a pre-generated WFP file** (no fingerprinting):

```bash
scanoss scan wfp project.wfp --api-key "$SCANOSS_API_KEY"
```

> **How it works:** blocks are POSTed to `/v3/wfp/scan` as
> `application/octet-stream` with a `Content-Range` header and a shared
> `X-Scan-Id`. The CLI then polls `GET /v3/wfp/scan/<scan-id>`. The scan id is
> printed as soon as the upload finishes, so an interrupted scan is resumable
> (see [`results`](#resuming-a-scan-results)).

Flags (persistent flags are shared with `scan wfp`): `--api-url`, `--api-key`,
`-f, --format` (`raw`/`spdx`/`cyclonedx`), `--include` (extra output layers — see
[Output layers](#output-layers---include)), `-o, --output`, `--settings`,
`--chunk-size` (1 MiB), `--ignore-cert-errors`, `-t, --threads` (10),
`--save-wfp`, `--max-size` (0 = unlimited),
`--default-filters` (true), `--gitignore` (true).

### Skipping files

Files are filtered before scanning (build dirs, vendored deps, generated/binary
files, oversized files). Toggle the sources with flags, or configure project rules
in `scanoss.json` — see [`scanoss.json` reference](#scanossjson-reference).

```bash
# Disable the built-in default filters and .gitignore, cap file size at 1 MiB
scanoss scan ./my-project \
  --api-key "$SCANOSS_API_KEY" \
  --default-filters=false \
  --gitignore=false \
  --max-size 1048576
```

**scanoss.json equivalent** (per-project skip rules):

```json
{
  "settings": {
    "skip": {
      "patterns": { "scanning": ["dist/**", "**/*.min.js", "docs/"] },
      "sizes":    { "scanning": [{ "patterns": ["*.bin"], "min": 0, "max": 1048576 }] }
    }
  }
}
```

### BOM context

Configure a Bill of Materials in `scanoss.json` (auto-detected in the target, or
pass `--settings`):

```json
{
  "bom": {
    "include": [{ "purl": "pkg:github/scanoss/engine" }],
    "remove":  [{ "purl": "pkg:github/scanoss/scanoss" }]
  }
}
```

```bash
scanoss scan ./my-project --api-key "$SCANOSS_API_KEY"            # auto-detected
scanoss scan ./my-project --api-key "$SCANOSS_API_KEY" --settings my-config.json
```

> **Note:** only **`bom.remove`** is currently applied — client-side, after
> results come back, matching components are dropped (`bom.include` only protects
> its PURLs from that removal). `bom.include` is **not yet honored server-side**,
> and `identify`/`ignore`/`replace` are not applied.

### Output layers (`--include`)

By default a scan reports only the components it detected. `--include` opts into extra output
layers, gathered over both detected and declared components:

| Layer | What it adds |
|-------|--------------|
| `deps` | Declared dependencies parsed from the project's manifests (`package.json`, `go.mod`, …) and resolved. Needs a source tree, so it is ignored for `scan wfp`. |
| `vulns` | Known vulnerabilities. |
| `licenses` | Declared/concluded licenses per component. |
| `crypto` | Cryptographic algorithms. |
| `geo` | Contributor geographic provenance. |

```bash
scanoss scan ./my-project --api-key "$SCANOSS_API_KEY" --include deps,vulns,licenses
```

Gathering follows `--include`, narrowed to what the chosen `--format` can render: a layer the
format can't represent is **skipped** (not gathered) with an up-front `Skipping <layer>` message.
`raw` renders every layer; `cyclonedx` drops `crypto`/`geo`; `spdx` drops `vulns`/`crypto`/`geo`.

### SBOM output

```bash
scanoss scan ./my-project --api-key "$SCANOSS_API_KEY" --format spdx      --output sbom-spdx.json
scanoss scan ./my-project --api-key "$SCANOSS_API_KEY" --format cyclonedx --output sbom-cdx.json
scanoss scan ./my-project --api-key "$SCANOSS_API_KEY" --format raw       --output results.json
```

- `raw` — the neutral inventory (components tagged by `scope`, per-component layers inline, and a
  flat vulnerabilities list) wrapped in a versioned envelope. **Default.**
- `spdx` — SPDX 2.3.
- `cyclonedx` — CycloneDX 1.7 (licenses, evidence, vulnerabilities).

Components that share the same identity (PURL + version) are collapsed into one — so the same
package listed in both `package.json` and `package-lock.json`, or detected and also declared, is
emitted once; different versions of the same PURL are kept. In SPDX, multiple licenses on a
component are combined with `AND`.

## Resuming a scan (`results`)

Retrieve the results of a scan by the id printed during `scan` (works after a
CTRL+C — the uploaded WFP is resumable):

```bash
scanoss results <scan-id> --api-key "$SCANOSS_API_KEY" --output results.json
```

Polls `GET /v3/wfp/scan/<scan-id>` until complete.

## SBOM (`sbom`)

Produce an SBOM from a scanoss raw inventory, or convert an existing SBOM between formats —
offline. The input format is detected from the file content.

```bash
# SPDX -> CycloneDX
scanoss sbom bom.spdx.json --format cyclonedx --output bom.cdx.json

# CycloneDX -> SPDX
scanoss sbom bom.cdx.json --format spdx --output bom.spdx.json

# scanoss raw result -> CycloneDX or SPDX
scanoss sbom results.json --format cyclonedx --output bom.cdx.json
```

Inputs: a scanoss **raw inventory** (the `scan` raw output), CycloneDX, or SPDX (JSON) — detected
from content. Target (`-f, --format`): `cyclonedx` or `spdx`. Conversion is **best-effort**: data
the target can't represent is dropped — e.g. SPDX 2.3 has no vulnerability model, so
vulnerabilities are omitted (with a warning) when converting to spdx.

Flags: `-f, --format` (`cyclonedx`/`spdx`), `-o, --output`.

## Enrich (`enrich`)

Decorate an existing inventory or SBOM with purl-keyed layers through the SCANOSS API — no
source tree, no fingerprinting, no re-scan. Because it is keyed purely by PURL, it is
**re-runnable** (e.g. weekly) to refresh the layers against the same file.

```bash
# Refresh vulns/licenses/crypto on a raw inventory (raw in, raw out)
scanoss enrich inv.json --include vulns,licenses,crypto --api-key "$SCANOSS_API_KEY" > enriched.json

# Enrich an SPDX document (spdx in, spdx out)
scanoss enrich sbom.spdx.json --include licenses --api-key "$SCANOSS_API_KEY" > enriched.spdx.json

# Enrich a CycloneDX document and convert to SPDX in one pass
scanoss enrich sbom.cdx.json --include licenses --format spdx --api-key "$SCANOSS_API_KEY" > enriched.spdx.json
```

Inputs: a scanoss **raw inventory** (the `scan` raw output), CycloneDX, or SPDX (JSON) —
detected from the file content. Layers (`--include`): the purl-keyed layers `vulns`,
`licenses`, `crypto`, `geo`. `deps` is **not** an enrich layer — dependency analysis needs a
manifest/source tree and can't be derived from a components list, so `--include deps` errors.

The output format **defaults to the input's** (raw→raw, cyclonedx→cyclonedx, spdx→spdx); use
`-f, --format` to convert in the same pass. A layer the output format can't represent is
**skipped** up front with a notice (`spdx` skips vulns/crypto/geo, `cyclonedx` skips
crypto/geo) — the same capability rules as `scan`. Enrichment is non-fatal: a failed service
is logged and skipped, and a partial result is still written.

Flags: `--include`, `-f, --format` (`raw`/`spdx`/`cyclonedx`), `-o, --output`, plus the auth
flags (`--api-url`, `--api-key`, `--ignore-cert-errors`).

## Dependencies (`dependencies`)

Two modes.

**Local mode** — parse manifest files under a path and query the API:

```bash
scanoss dependencies ./my-project --extract-local --output deps.json
```

**API mode** — query a component's dependencies (`--requirement` is optional):

```bash
# Direct dependencies
scanoss dependencies --purl 'pkg:github/scanoss/engine' --requirement '5.4.7' \
  --api-key "$SCANOSS_API_KEY"

# Transitive dependencies, custom depth/limit
scanoss dependencies --purl 'pkg:github/scanoss/engine' --requirement '5.4.7' --transient \
  --depth 5 --limit 20 --api-key "$SCANOSS_API_KEY"
```

Flags: `--extract-local`, `--purl`, `--requirement` (optional), `--transient`,
`--depth` (10), `--limit` (10), `--api-url`, `--api-key`, `--ignore-cert-errors`,
`-o, --output`.

Endpoints: direct → `POST /v3/dependencies/dependencies`;
transitive → `POST /v3/dependencies/transitive`.

## Attributions (`attributions`)

Generate attribution text from an SBOM file, or from a single PURL. Provide
**either** the file **or** `--purl` (not both).

```bash
# From an SBOM file
scanoss attributions sbom.json --output attributions.txt

# From a PURL (a temporary SBOM is created for you)
scanoss attributions --purl "pkg:github/scanoss/engine@v5.4.19" \
  --api-key "$SCANOSS_API_KEY" --output attributions.txt
```

Uploads to `POST /sbom/attribution`. Flags: `--purl`, `--api-url`, `--api-key`,
`--ignore-cert-errors`, `-o, --output`.

## Decoration commands

Query the SCANOSS v3 API about components. Each command is a parent with one
subcommand per operation; running it bare uses the **default** operation. Input is
a list of PURLs (repeat `--purl`, or pass `--input`), split into chunks and queried
concurrently.

**Shared flags:** `--purl` (repeatable), `--requirement` (default version/range),
`--input` (PURL file), `--chunk-size` (PURLs per request, 10), `-t, --workers`
(max concurrent requests, 5), `--api-url`, `--api-key`, `--ignore-cert-errors`,
`-o, --output`.

The `--input` file is either newline-delimited `purl[,requirement]`, or JSON
`{"components":[{"purl":"...","requirement":"..."}]}`.

### `vulnerabilities` — `components` (default), `cpes`

```bash
scanoss vulnerabilities --purl 'pkg:github/scanoss/engine' --api-key "$SCANOSS_API_KEY"
scanoss vulnerabilities cpes --purl 'pkg:github/scanoss/engine' --api-key "$SCANOSS_API_KEY"
```

### `cryptography` — `algorithms` (default), `algorithms-range`, `versions-range`, `hints`, `hints-range`

The version or range goes in `--requirement`.

```bash
scanoss cryptography --purl 'pkg:github/scanoss/engine' --requirement '5.0.1' --api-key "$SCANOSS_API_KEY"
scanoss cryptography algorithms-range --purl 'pkg:github/scanoss/engine' --requirement '>5.0.0' --api-key "$SCANOSS_API_KEY"
scanoss cryptography hints-range --purl 'pkg:github/scanoss/engine' --requirement '>5.0.0' --api-key "$SCANOSS_API_KEY"
```

### `licenses` — `declared` (default), `attribution`, `evidence`

```bash
scanoss licenses --purl 'pkg:github/scanoss/engine' --api-key "$SCANOSS_API_KEY"             # declared
scanoss licenses attribution --purl 'pkg:github/scanoss/engine' --api-key "$SCANOSS_API_KEY"
scanoss licenses evidence --purl 'pkg:github/scanoss/engine' --api-key "$SCANOSS_API_KEY"
```

### `geoprovenance` — `origin` (default), `countries`

```bash
scanoss geoprovenance --purl 'pkg:github/scanoss/engine' --requirement '5.4.7' --api-key "$SCANOSS_API_KEY"
scanoss geoprovenance countries --purl 'pkg:github/scanoss/engine' --api-key "$SCANOSS_API_KEY"
```

### `copyright` — `evidence` (default), `holders`

```bash
scanoss copyright --purl 'pkg:github/scanoss/engine' --api-key "$SCANOSS_API_KEY"
scanoss copyright holders --purl 'pkg:github/scanoss/engine' --api-key "$SCANOSS_API_KEY"
```

### `components` — `search` (default), `versions`, `status`

`search` and `versions` take their own flags instead of the PURL list.

```bash
# Search by vendor/component/term
scanoss components --vendor scanoss --component engine --limit 20 --api-key "$SCANOSS_API_KEY"
scanoss components search --search engine --purl-type github --limit 20 --offset 0 --api-key "$SCANOSS_API_KEY"

# Known versions (with licenses) for a purl
scanoss components versions --purl 'pkg:github/scanoss/engine' --limit 50 --api-key "$SCANOSS_API_KEY"

# Lifecycle status for a PURL list
scanoss components status --purl 'pkg:github/scanoss/engine' --requirement '1.2.3' --api-key "$SCANOSS_API_KEY"
```

`components search` flags: `--search`, `--vendor`, `--component` (at least one),
`--purl-type` (default `github`), `--limit`, `--offset`.
`components versions` flags: `--purl`, `--limit`.

## `scanoss.json` reference

`scanoss.json` (or `settings.json`) is auto-detected in the scan target, or passed
with `--settings`. It carries BOM context and file-skip rules.

```json
{
  "bom": {
    "include":  [{ "purl": "pkg:github/scanoss/engine" }],
    "identify": [],
    "ignore":   [],
    "remove":   [{ "purl": "pkg:github/scanoss/scanoss" }],
    "replace":  []
  },
  "settings": {
    "skip": {
      "patterns": { "scanning": ["dist/**", "**/*.min.js"] },
      "sizes":    { "scanning": [{ "patterns": ["*.bin"], "min": 0, "max": 1048576 }] }
    }
  }
}
```

- **`bom`** — only `bom.remove` is applied today (client-side, post-scan);
  `bom.include` protects its PURLs from removal but is not yet honored
  server-side; `identify`/`ignore`/`replace` are not applied.
- **`settings.skip`** — keyed by operation (`scanning`, `fingerprinting`,
  `dependencies`). `patterns` are gitignore-style globs; `sizes` set per-pattern
  byte bounds (`0` disables a bound).

## Default values

| Setting | Default | Notes |
|---------|---------|-------|
| `--api-url` | `https://api.scanoss.com` | Default endpoint requires `--api-key` |
| `--threads` (scan/wfp) | `10` | Fingerprint workers |
| `--format` | `raw` | `raw` / `spdx` / `cyclonedx` |
| `--chunk-size` (scan) | `1048576` | WFP upload block size (bytes) |
| `--chunk-size` (decoration) | `10` | PURLs per request |
| `--workers` (decoration) | `5` | Max concurrent requests |
| `--depth` / `--limit` (deps) | `10` / `10` | Transitive only |

## Go SDK

Everything above is also available as a Go SDK (`pkg/scanoss`). See the
[README](README.md#go-sdk--decoration-pipeline) for the decoration pipeline,
per-service progress, and scanning from Go.

```go
client := scanoss.New(scanoss.WithAPIKey(os.Getenv("SCANOSS_API_KEY")))
result, err := client.Scan.Folder(ctx, "./my-project")

res, err := client.Vulnerabilities.Components(ctx, scanoss.Components("pkg:github/scanoss/engine"))
```
