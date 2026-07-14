# SCANOSS CLI — Go Implementation

Command-line tool and Go SDK for scanning source code and querying the SCANOSS
platform. It fingerprints a project with WFP (Winnowing FingerPrint), uploads the
fingerprints to the SCANOSS v3 API, and can decorate results with vulnerabilities,
licenses, cryptography, geoprovenance, copyright, dependency, and component data.

See [CHANGELOG.md](CHANGELOG.md) for release notes and the
[releases page](https://github.com/scanoss/scanoss.go/releases) for the latest
version.

## Architecture

```
scanoss/
├── cmd/                    # CLI commands (Cobra)
│   ├── root.go            # Root command
│   ├── scan.go            # scan (+ scan wfp subcommand)
│   ├── wfp.go             # wfp (fingerprint only)
│   ├── results.go         # results (resume/poll a scan by id)
│   ├── dependencies.go    # dependencies
│   ├── attributions.go    # attributions
│   ├── vulnerabilities.go / cryptography.go / licenses.go
│   ├── geoprovenance.go / copyright.go / components.go
│   ├── auth.go, purlcommon.go, helpers.go, httpclient.go
│   └── scanoss/           # main package — CLI entrypoint (go install target)
│
├── internal/
│   ├── config/            # Defaults
│   ├── models/            # Data models
│   └── version/           # Build/tag version (single source)
│
├── pkg/                    # Reusable packages
│   ├── scanoss/           # Go SDK (scan + decoration services, pipeline)
│   ├── scanner/           # Worker pool + file collection
│   ├── fingerprint/       # WFP fingerprinting
│   ├── filter/            # File filtering (defaults + scanoss.json + .gitignore)
│   ├── batch/             # Fingerprint batching
│   ├── dependencies/      # Local manifest dependency parsing
│   ├── sbom/              # SBOM generation (SPDX, CycloneDX)
│   ├── postprocess/       # BOM post-processing helpers
│   ├── output/            # Result writing
│   ├── settings/          # scanoss.json parsing (BOM + skip)
│   └── api/               # Low-level HTTP client
│
├── libscanoss/            # C-shared library + Python/Node wrappers
└── go.mod
```

OpenAPI types come from the published SDK
`github.com/scanoss/scanoss.api-sdk` (imported as `scanossapi`); there is no
local codegen step.

## Installation

### `go install`

```bash
go install github.com/scanoss/scanoss.go/cmd/scanoss@latest
```

### Prebuilt binary

Download the archive for your platform from the
[releases page](https://github.com/scanoss/scanoss.go/releases), extract it, and
move the `scanoss` binary onto your `PATH`:

```bash
# Linux (amd64) — adjust the archive for your OS/arch
tar xzf scanoss-linux-amd64.tar.gz
sudo mv scanoss /usr/local/bin/
```

On Windows, unzip the `.zip` and add the folder to your `PATH`. On macOS, an
unsigned direct download may be quarantined by Gatekeeper — clear it with
`xattr -d com.apple.quarantine ./scanoss`. Verify a download against
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
make build          # or: go build -o scanoss ./cmd/scanoss
```

## Quick start

```bash
# Scan a project and save JSON results (default endpoint needs an API key)
scanoss scan ./my-project --api-key "$SCANOSS_API_KEY" --output results.json

# Generate fingerprints only
scanoss wfp ./my-project > project.wfp
```

Add `-v` / `--verbose` to any command for structured debug logging on stderr — the
scan flow, each API request (method/URL/status/duration), and fingerprinting.
Stdout stays reserved for results, so logs never corrupt `--output` or piped JSON.

## Commands

| Command | Purpose |
|---------|---------|
| `scan <path>` | Fingerprint a folder/file, upload to the SCANOSS v3 API, poll for results. |
| `scan wfp <wfp>` | Scan a pre-generated WFP file (no fingerprinting). |
| `wfp <path>` | Generate WFP fingerprints only (no upload). |
| `results <scan-id>` | Resume or poll a scan by its id. |
| `convert <input>` | Convert an SBOM/result between formats offline (cyclonedx/spdx). |
| `dependencies [path]` | Extract local dependencies, or query direct/transitive deps for a PURL. |
| `attributions [sbom]` | Attribution text from an SBOM file or a PURL. |
| `vulnerabilities` | Known vulnerabilities / CPEs for components. |
| `cryptography` | Algorithms, library hints, and version ranges. |
| `licenses` | Declared licenses, attribution files, per-file evidence. |
| `geoprovenance` | Component origin and contributor countries. |
| `copyright` | Copyright evidence and holders. |
| `components` | Search, versions, and lifecycle status. |

The default endpoint (`https://api.scanoss.com`) requires `--api-key`; a custom
`--api-url` (e.g. an on-prem deployment) may run keyless.

See **[CLIENT_HELP.md](CLIENT_HELP.md)** for full usage — every command and
subcommand with flags, examples, the `scanoss.json` reference (BOM + skip rules),
SBOM output formats, and default values.

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
