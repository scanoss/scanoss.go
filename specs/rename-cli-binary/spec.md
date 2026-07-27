# Feature Specification: Rename CLI binary to `scanoss-cli`

**Feature branch:** `feat/rename-cli-binary`
**Status:** Draft
**SDD Change:** `rename-cli-binary`

## Summary
The produced executable is currently named `scanoss`, which collides on the
`PATH` with the SCANOSS **scan engine** binary (also `scanoss`). A user who
installs both cannot have both on the `PATH` unambiguously. This change renames
the CLI's executable to `scanoss-cli` everywhere it is *built, distributed, and
invoked* — local `make build`, GoReleaser artifacts, the Docker image entrypoint,
CI, and docs — and updates the Cobra root command name so help/usage and shell
completion register under `scanoss-cli`.

The rename is deliberately limited to the **executable/artifact name** and the
command name. Project/distribution *identity* that does not sit on the `PATH`
(the GoReleaser `project_name`, the GHCR image repository `ghcr.io/scanoss/scanoss`,
OCI image labels) is left unchanged to minimise disruption. The Go package
directory is renamed too (`cmd/scanoss` → `cmd/scanoss-cli`) so that
`go install …/cmd/scanoss-cli@latest` produces the `scanoss-cli` binary directly,
with no manual rename.

The project is pre-1.0 and not yet in production, so this ships as a normal
`## [0.2.0]` change rather than a breaking one. Still, anyone with old install
scripts, download URLs, or `docker run ... scanoss` invocations should update
them; call it out in the release notes.

## User Scenarios & Testing

### Primary user story
As a user who has the SCANOSS scan engine (`scanoss`) installed, I want the Go
CLI to install as `scanoss-cli` so both tools coexist on my `PATH` without one
shadowing the other.

### Acceptance scenarios
1. **Given** a clean checkout, **when** I run `make build`, **then** the produced
   binary is `./scanoss-cli` (not `scanoss`), and `make clean` removes it.
2. **Given** the built binary, **when** I run `scanoss-cli --help`, **then** the
   usage line reads `scanoss-cli [command]` and the synopsis names `scanoss-cli`.
3. **Given** the built binary, **when** I run `scanoss-cli completion bash` and
   load it, **then** completion is registered for the command `scanoss-cli`.
4. **Given** a GoReleaser build, **when** artifacts are produced, **then** the
   executable inside each archive is `scanoss-cli` (`scanoss-cli.exe` on Windows)
   and the archive filenames are `scanoss-cli-<os>-<arch>...`.
5. **Given** the release Docker image, **when** I run
   `docker run --rm ghcr.io/scanoss/scanoss:latest --version`, **then** the
   entrypoint executes `scanoss-cli` and prints the version.
6. **Given** the CI build job, **when** it runs, **then** it builds `-o
   scanoss-cli` and its smoke tests invoke `./scanoss-cli`.
7. **Given** the release smoke-test job, **when** it extracts an archive, **then**
   it invokes the `scanoss-cli` binary and succeeds.

### Edge cases
- Windows: executable is `scanoss-cli.exe`; archive stays `.zip`.
- HarmonyOS build (`id: harmonyos`) uses the same `scanoss-cli` executable name;
  its archive becomes `scanoss-cli-harmonyos-arm64`.
- Docker `--output /src/...` non-root write behavior is unchanged (unrelated).

## Requirements
- **FR-1** `make build` produces `scanoss-cli`; `make clean` removes it
  (`Makefile` `BIN` default → `scanoss-cli`).
- **FR-2** Cobra root command `Use` is `scanoss-cli` (`cmd/root.go`), so help,
  usage, and the `completion` command register under `scanoss-cli`. The
  `--version` template prints `scanoss-cli <version>`.
- **FR-2b** In-binary command examples and user-facing runtime hints use
  `scanoss-cli`: every cobra `Example:` block in `cmd/*.go`, the interrupted-scan
  resume hint (`cmd/scan.go` → `scanoss-cli results <id>`), and the doc comment in
  `pkg/scanoss/scan.go`.
- **FR-3** GoReleaser (`.goreleaser.yaml`) sets `binary: scanoss-cli` on both the
  `scanoss` and `harmonyos` builds; archive `name_template`s produce
  `scanoss-cli-...` filenames.
- **FR-4** Dockerfile copies and sets `ENTRYPOINT` to `scanoss-cli`
  (`/usr/local/bin/scanoss-cli`).
- **FR-5** CI (`.github/workflows/ci.yml`) builds `-o scanoss-cli` and invokes
  `./scanoss-cli` in its smoke steps.
- **FR-6** Release workflow (`.github/workflows/release.yml`) references the new
  archive filenames and invokes the `scanoss-cli` binary in smoke tests.
- **FR-7** Docs describe installing/invoking `scanoss-cli`: `README.md`
  (install + Quick-start command examples), `CLIENT_HELP.md` (all CLI invocation
  examples and prose), `.github/workflows/README.md`, the GoReleaser release
  footer, and `libscanoss/docs/*` CLI subprocess/exec examples (`nodejs.md`,
  `integration.md`). Non-binary references are preserved: `scanoss.json`,
  `api.scanoss.com`, PURLs (`pkg:github/scanoss/engine`), the "scanoss raw"
  inventory format name, "scanoss git tag", the Go SDK API (`scanoss.New(...)`),
  the `libscanoss` shared-library artifacts, and the SDK package path
  `pkg/scanoss`.
- **FR-8** `CHANGELOG.md` `## [0.2.0]` records the rename under a `### Changed`
  entry.
- **NFR-1** No source/behavior change beyond names: `go build ./...`, `go vet
  ./...`, and the test suite stay green.
- **NFR-2** The rename is complete and consistent — no stray reference to the old
  executable name survives in build/dist/docs paths that are in scope.

## Decisions (open for review)
These were judgment calls made to keep the change coherent; flag any to revert:
- **Archive filenames renamed** to `scanoss-cli-*` (not just the inner binary),
  for consistency. This changes download URLs.
- **Kept unchanged** (project/product identity, not a `PATH` binary): GoReleaser
  `project_name: scanoss`, GHCR image repo `ghcr.io/scanoss/scanoss`, OCI
  `image.title: scanoss` labels, and `config.AppName = "scanoss"` — the tool name
  embedded in generated SBOM metadata (`Tool: scanoss`). Changing `AppName` would
  alter output documents and break fixtures; it is product identity, not the
  binary path.

## Out of scope
- **`go install` name — now addressed.** The package directory is renamed
  `cmd/scanoss` → `cmd/scanoss-cli`, so
  `go install github.com/scanoss/scanoss.go/cmd/scanoss-cli@latest` produces the
  `scanoss-cli` binary directly (Go names it after the directory). No manual
  rename is needed.
- Renaming the GHCR image repository or the GoReleaser project name.
- Any code, flag, or runtime behavior change.
