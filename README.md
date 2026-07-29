# SCANOSS CLI — Go Implementation

Command-line tool and Go SDK for scanning source code and querying the SCANOSS
platform. It fingerprints a project with WFP (Winnowing FingerPrint), uploads the
fingerprints to the SCANOSS v3 API, and can decorate results with vulnerabilities,
licenses, cryptography, geoprovenance, copyright, dependency, and component data.

See [CHANGELOG.md](CHANGELOG.md) for release notes and the
[releases page](https://github.com/scanoss/scanoss.go/releases) for the latest
version.

## Architecture

- `cmd/` — the CLI (Cobra); `cmd/scanoss-cli` is the `go install` entrypoint.
- `pkg/` — the reusable Go SDK: scan and decoration services, fingerprinting,
  file filtering, SBOM read/write, and the low-level API client.
- `internal/` — private helpers (config, version).
- `libscanoss/` — C shared library with Python and Node.js wrappers.

OpenAPI types come from the published SDK `github.com/scanoss/scanoss.api-sdk`
(imported as `scanossapi`); there is no local codegen step.

## Installation

### `go install`

```bash
go install github.com/scanoss/scanoss.go/cmd/scanoss-cli@latest
```

This installs the CLI as `scanoss-cli` (Go names the binary after its package
directory) — matching the examples below and avoiding a clash with the SCANOSS
scan engine (also `scanoss`) on your `PATH`.

### Prebuilt binary

Download the archive for your platform from the
[releases page](https://github.com/scanoss/scanoss.go/releases), extract it, and
move the `scanoss-cli` binary onto your `PATH`:

```bash
# Linux (amd64) — adjust the archive for your OS/arch
tar xzf scanoss-cli-linux-amd64.tar.gz
sudo mv scanoss-cli /usr/local/bin/
```

On Windows, unzip the `.zip` and add the folder to your `PATH`. On macOS, an
unsigned direct download may be quarantined by Gatekeeper — clear it with
`xattr -d com.apple.quarantine ./scanoss-cli`. Verify a download against
`checksums.txt` with `sha256sum -c --ignore-missing checksums.txt`.

### Docker

Multi-arch images (`linux/amd64`, `linux/arm64`) are published to GHCR on each
release. Mount the code to scan and pass the CLI arguments:

```bash
docker run --rm -v "$PWD:/src" ghcr.io/scanoss/scanoss:latest \
  scan /src --api-key "$SCANOSS_API_KEY" > results.json
```

Use `:latest` or a version tag (e.g. `:0.1.0`).

The image runs as a non-root user, so writing output *into* the mounted folder
(`--output /src/results.json`) can fail with a permission error. Either redirect
stdout on the host as above, or run the container as your own user so writes are
owned by you:

```bash
docker run --rm --user "$(id -u):$(id -g)" -v "$PWD:/src" \
  ghcr.io/scanoss/scanoss:latest scan /src --api-key "$SCANOSS_API_KEY" --output /src/results.json
```

### Build from source

```bash
git clone https://github.com/scanoss/scanoss.go.git
cd scanoss.go
make build          # or: go build -o scanoss-cli ./cmd/scanoss-cli
```

## Quick start

```bash
# Scan a project and save JSON results (default endpoint needs an API key)
scanoss-cli scan ./my-project --api-key "$SCANOSS_API_KEY" --output results.json

# Generate fingerprints only
scanoss-cli wfp ./my-project > project.wfp

# Refresh vulnerabilities/licenses on an existing inventory (no re-scan)
scanoss-cli enrich results.json --include vulns,licenses --api-key "$SCANOSS_API_KEY" > enriched.json
```

Add `-v` / `--verbose` to any command for structured debug logging on stderr — the
scan flow, each API request (method/URL/status/duration), and fingerprinting.
Stdout stays reserved for results, so logs never corrupt `--output` or piped JSON.

## Commands

| Command | Purpose |
|---------|---------|
| `scan <path>` | Fingerprint a folder/file, scan against the SCANOSS v3 API, and output results (`--format raw`/`spdx`/`cyclonedx`; opt into dependency/vuln/license/crypto/geo layers with `--include`). |
| `scan wfp <wfp>` | Scan a pre-generated WFP file (no fingerprinting). |
| `wfp <path>` | Generate WFP fingerprints only (no upload). |
| `results <scan-id>` | Resume or poll a scan by its id. |
| `sbom <input>` | Produce an SBOM from a raw inventory, or convert between formats, offline (cyclonedx/spdx). |
| `enrich <input>` | Add purl-keyed layers (vulns/licenses/crypto/geo) to a `raw` or SBOM file. |
| `dependencies [path]` | Extract local dependencies, or query direct/transitive deps for a PURL. |
| `attributions [sbom]` | Attribution text from an SBOM file or a PURL. |
| `vulnerabilities` | Known vulnerabilities / CPEs for components. |
| `cryptography` | Algorithms, library hints, and version ranges. |
| `licenses` | Declared licenses, attribution files, per-file evidence. |
| `geoprovenance` | Component origin and contributor countries. |
| `copyright` | Copyright evidence and holders. |
| `components` | Search, versions, and lifecycle status. |
| `config` | Store settings in `~/.scanoss/settings.json` (see [Configuration](#configuration)). |

The default endpoint (`https://api.scanoss.com`) requires an API key; a custom API
URL (e.g. an on-prem deployment) may run keyless.

See **[CLIENT_HELP.md](CLIENT_HELP.md)** for full usage — every command and
subcommand with flags, examples, the `scanoss.json` reference (BOM + skip rules),
SBOM output formats, and default values.

## Configuration

Store your credentials once instead of passing `--api-key` on every command:

```bash
scanoss-cli config set api-key SC_abc123def456
scanoss-cli scan ./my-project --output results.json
```

Settings use the same names as the flags: `api-key`, `api-url`, `proxy` and `ca-cert`.
They live in `~/.scanoss/settings.json`:

```json
{
  "api_key": "SC_abc123def456",
  "api_url": "https://api.scanoss.com",
  "proxy": "http://proxy.example.com:8080",
  "ca_cert": "/etc/ssl/corp-ca.pem"
}
```

The command line always uses the dashed names; `snake_case` is the file's format, not a
second way to type a key.

### Precedence

Every setting resolves the same way, and each has a matching flag:

```
--flag  >  environment variable  >  ~/.scanoss/settings.json  >  built-in default
```

The environment variable is the setting name in upper case with a `SCANOSS_` prefix:
`SCANOSS_API_KEY`, `SCANOSS_API_URL`, `SCANOSS_PROXY`, `SCANOSS_CA_CERT`. An empty value
from the environment or the file is treated as unset and falls through to the next source.

```bash
scanoss-cli config set api-url https://scanoss.internal.example.com

# 1. the stored value is used
scanoss-cli scan .

# 2. the environment overrides the file
SCANOSS_API_URL=https://scanoss.staging.example.com scanoss-cli scan .

# 3. the flag overrides both
SCANOSS_API_URL=https://scanoss.staging.example.com \
  scanoss-cli scan . --api-url https://api.scanoss.com
```

`--verbose` reports which source won for each setting (the source only, never the key's
value).

### Inspecting

`config list` shows the value each command will actually use, and where it came from:

```console
$ scanoss-cli config list
api-key  ********                              (env: SCANOSS_API_KEY)
api-url  https://scanoss.internal.example.com  (config file)

Config file: /Users/you/.scanoss/settings.json
```

**The API key is never printed.** `list` and `get` always render it as `********` — there
is no flag that reveals it, so it cannot land in your shell history or a CI log. `config
get api-key` therefore only tells you whether it is set (exit code `0` or `1`). Scripts
that need the value should use `$SCANOSS_API_KEY`; to read your own file, open it
directly:

```bash
cat "$(scanoss-cli config path)"
```

Non-secret values print normally, so `config get` composes:

```console
$ scanoss-cli config get api-url
https://scanoss.internal.example.com
```

### On-prem endpoint

A custom API URL may run keyless, so pointing the CLI at an internal deployment is one
command:

```bash
scanoss-cli config set api-url https://scanoss.internal.example.com
scanoss-cli scan .
```

### Proxy and custom CA

`HTTP_PROXY`, `HTTPS_PROXY` and `NO_PROXY` are honoured with no flags. `--proxy` overrides
them for one run, and `--ca-cert` trusts a CA the system pool does not have — an internal
endpoint, or a proxy that intercepts TLS:

```bash
scanoss-cli scan . --proxy http://proxy.example.com:8080
scanoss-cli scan . --ca-cert /etc/ssl/corp-ca.pem
```

The CA is *added* to the system pool, so the public API keeps working, and verification
stays on — unlike `--ignore-cert-errors`. Both flags work on every command that reaches
the API. Proxy auto-configuration (PAC) is not supported: read the proxy out of the PAC
and pass it with `--proxy`.

Both can be stored, so neither flag has to be repeated:

```bash
scanoss-cli config set proxy http://proxy.example.com:8080
scanoss-cli config set ca-cert /etc/ssl/corp-ca.pem
scanoss-cli scan .
```

A stored `proxy` takes precedence over `HTTP_PROXY`/`HTTPS_PROXY`. `--ignore-cert-errors`
is not storable — turning off verification stays a per-run choice.

### CI

Use the environment instead of a config file — no `config set`, and no key on the
command line where it would land in build logs:

```yaml
- name: SCANOSS scan
  env:
    SCANOSS_API_KEY: ${{ secrets.SCANOSS_API_KEY }}
  run: scanoss-cli scan . --output results.json
```

### Rotating and removing

```bash
scanoss-cli config set api-key SC_newkey789   # overwrite in place
scanoss-cli config unset api-key              # remove the key
scanoss-cli config path                       # print the file location
```

Hand-editing the file is supported, and keys this version does not recognize are left
untouched by `config set`.

## Go SDK — Decoration Pipeline

Beyond the CLI, `pkg/scanoss` is a Go SDK for the SCANOSS services. The
**pipeline** runs a configurable set of decoration services over the same PURLs in
parallel, reports per-service progress, and returns one object keyed by service.
Chunking and the worker pool are handled internally.

```go
import "github.com/scanoss/scanoss.go/pkg/scanoss"

client := scanoss.New(
    scanoss.WithAPIKey(os.Getenv("SCANOSS_API_KEY")),
    scanoss.WithChunkSize(20), // PURLs per request
    scanoss.WithWorkers(10),   // max concurrent requests
    // scanoss.WithLogger(logger), // optional: route SDK diagnostics via log/slog (default slog.Default())
)

comps := scanoss.Components("pkg:github/scanoss/engine")

pipe := client.DecorationPipeline(
    scanoss.ServiceVulnerabilities,
    scanoss.ServiceLicenses,
)
pipe.Add(scanoss.ServiceCryptographyAlgorithms, scanoss.ServiceGeoprovenanceOrigin)

res, err := pipe.Run(context.Background(), comps)
if err != nil {
    log.Fatal(err) // only if every service failed
}
fmt.Println(res.String())

for svc, e := range res.Errors { // per-service failures are recorded, not fatal
    log.Printf("%s failed: %v", svc, e)
}
```

### Per-service progress

`OnProgress` delivers a snapshot keyed by service, serially (no locking needed):

```go
pipe.OnProgress(func(pp scanoss.PipelineProgress) {
    for name, p := range pp.Services {
        fmt.Printf("%-26s %d/%d %s\n", name, p.Done, p.Total, p.Unit)
    }
})
res, _ := pipe.Run(ctx, comps)
snapshot := pipe.Snapshot() // or pull the current state on a render tick
```

### Version requirements

`scanoss.Components(...)` produces entries with no version. When a version matters,
build the components directly:

```go
comps := []scanoss.Component{
    {Purl: "pkg:github/scanoss/engine", Requirement: "4.17.21"},
    {Purl: "pkg:github/scanoss/engine", Requirement: "5.4.7"},
}
```

### A single service (without the pipeline)

Each decoration service is a grouped handle on the client:

```go
res, err := client.Vulnerabilities.Components(ctx, comps) // *scanossapi.VulnerabilitiesResponse
// also: client.Licenses.Attribution, client.Cryptography.Algorithms,
//       client.Geoprovenance.Origin, client.Copyright.Evidence, ...
```

### Scanning from the SDK

```go
client := scanoss.New(scanoss.WithAPIKey(os.Getenv("SCANOSS_API_KEY")))
result, err := client.Scan.Folder(ctx, "./my-project")
// resume by id: client.Scan.Wait(ctx, scanID)
```

### Proxy and custom CA from the SDK

```go
hc, err := scanoss.NewHTTPClient(scanoss.HTTPClientOptions{
    Proxy:      "http://proxy.example.com:8080", // empty honours HTTP(S)_PROXY
    CACertFile: "/etc/ssl/corp-ca.pem",          // added to the system pool
})
if err != nil {
    return err
}
client := scanoss.New(scanoss.WithAPIKey(key), scanoss.WithHTTPClient(hc))
```

## Development

```bash
make build         # build the CLI
make test          # unit tests
make test-race     # tests with the race detector
make lint          # golangci-lint
make check         # fmt-check + vet + lint + test (run before committing)
```

## License

See [LICENSE](LICENSE).
