# GitHub Actions Workflows

Automated workflows for building, testing, and releasing the scanoss project.

## Workflows

### 1. CI (`ci.yml`)

**Triggers**: pushes and pull requests.

**Jobs**:
- **test** — Go unit tests + `go vet`.
- **build-cli** — builds the `scanoss-cli` CLI (`go build -o scanoss-cli ./cmd/scanoss`).
- **build-and-test-libscanoss** — builds the shared library on Linux and macOS and exercises the Python and Node.js wrappers.
- **lint** — `gofmt` check + `golangci-lint`.

### 2. Release (`release.yml`)

**Triggers**: tags matching `v*` (e.g. `v0.1.0`); also `workflow_dispatch`.

Releases are produced by **[GoReleaser](https://goreleaser.com)**, configured in
[`.goreleaser.yaml`](../../.goreleaser.yaml).

**Jobs**:
- **build-libscanoss** — reusable workflow (`build-libscanoss.yml`) that builds the C shared libraries; their artifacts are attached to the release.
- **release** — packages the libscanoss libraries into `release-extra/`, then runs GoReleaser, which:
  - cross-compiles the CLI for all platforms,
  - creates per-platform archives + a single `checksums.txt`,
  - builds and pushes multi-arch **Docker images** to `ghcr.io/scanoss/scanoss` (`:{version}`, `:latest`),
  - creates a **draft** GitHub Release with the binaries, the libscanoss archives, and install notes.
- **test-release-binaries** — downloads the release archives and smoke-tests `--version`/`--help` on Linux, macOS, and Windows.

**Release assets** (CLI):

```
scanoss-cli-linux-amd64.tar.gz
scanoss-cli-linux-arm64.tar.gz
scanoss-cli-linux-armv7.tar.gz
scanoss-cli-harmonyos-arm64.tar.gz   # same binary as linux/arm64
scanoss-cli-darwin-amd64.tar.gz
scanoss-cli-darwin-arm64.tar.gz
scanoss-cli-windows-amd64.zip
scanoss-cli-windows-arm64.zip
checksums.txt
```

Plus the libscanoss shared-library archives (`libscanoss-<os>-<arch>.{tar.gz,zip}`)
and the Docker images on GHCR.

> **Credentials**: the release uses only the built-in `GITHUB_TOKEN` (with
> `contents: write` + `packages: write` for GHCR). No extra secrets are required.
> A Homebrew formula is intentionally not published (it would need a token for the
> separate `homebrew-dist` tap).

### 3. Build libscanoss (`build-libscanoss.yml`)

Reusable workflow that cross-compiles the shared library:

- **Linux** (`gcc-aarch64-linux-gnu` for arm64) → `libscanoss-linux-{arch}.so`
- **macOS** (native Intel/ARM) → `libscanoss-darwin-{arch}.dylib`
- **Windows** (MinGW) → `libscanoss-cli-windows-amd64.dll`

It also loads each library through the Python wrapper as a smoke test.

## Creating a release

```bash
git tag v0.1.0
git push origin v0.1.0
```

The release workflow runs automatically, builds everything, and creates a
**draft** release. Review the draft on the [Releases page](../../releases), then
publish it.

To dry-run without a tag: Actions → "Release scanoss CLI" → "Run workflow" → enter
a version (e.g. `v0.0.0-dev`).

## Installing a released build

```bash
# Linux amd64 — adjust the archive for your OS/arch
wget https://github.com/scanoss/scanoss.go/releases/download/<tag>/scanoss-cli-linux-amd64.tar.gz
tar xzf scanoss-cli-linux-amd64.tar.gz
sudo mv scanoss-cli /usr/local/bin/
scanoss-cli --version
```

On Windows, unzip `scanoss-cli-windows-amd64.zip` and add the folder to `PATH`. See the
[README](../../README.md) and [CLIENT_HELP.md](../../CLIENT_HELP.md) for `go install`,
Docker, and full usage.

## Troubleshooting

- **Release not created** — the tag must match `v*` and be pushed; the `release`
  job runs only on tags (`github.ref_type == 'tag'`).
- **Build fails on a platform** — check the failed job's logs in the
  [Actions tab](../../actions).
- **GoReleaser config** — validate locally with `goreleaser check`.

## Related documentation

- [libscanoss README](../../libscanoss/README.md)
- [Python integration](../../libscanoss/docs/python.md)
- [Node.js integration](../../libscanoss/docs/nodejs.md)
- [Multi-language integration](../../libscanoss/docs/integration.md)
