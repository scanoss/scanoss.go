# Implementation Plan: Rename CLI binary to `scanoss-cli`

**Spec:** [`spec.md`](./spec.md) · **Tasks:** [`tasks.md`](./tasks.md)

## Technical context
- **Language/module:** Go, `github.com/scanoss/scanoss.go`.
- **Nature of change:** pure rename — build/dist/docs strings + one Cobra field.
  No Go logic, flags, or runtime behavior change.
- **Sites to touch (all located):**
  - `Makefile:6` — `BIN ?= scanoss` (already parametrized via `$(BIN)` at build
    `:55` and clean `:70`, so only the default value changes).
  - `cmd/root.go:36` — `Use: "scanoss"`.
  - `.goreleaser.yaml` — `binary:` on builds `scanoss` (`:10`) and `harmonyos`
    (`:30`); archive `name_template`s (`:42`, `:50`); release footer (`:93-99`,
    `:110`).
  - `Dockerfile:7-8` — `COPY` destination and `ENTRYPOINT`.
  - `.github/workflows/ci.yml:44,48-49` — `go build -o scanoss` and smoke calls.
  - `.github/workflows/release.yml:93-97,108-117` — archive matrix + smoke calls
    (Unix `chmod +x scanoss`, `./scanoss ...`; Windows `.\scanoss.exe ...`).
  - `README.md:59,64-76,85-106` — `go install`, prebuilt binary, docker, build.
  - `CHANGELOG.md` — `## [Unreleased]`.
- **Left unchanged (per spec Decisions):** GoReleaser `project_name`, GHCR image
  repo `ghcr.io/scanoss/scanoss`, OCI `image.title` labels, dir `cmd/scanoss`.

## Design overview
The executable name lives in a handful of independent surfaces. Change each to
`scanoss-cli` (executable) and `scanoss-cli-*` (archive filenames), keeping
project/image *identity* strings as-is.

```
make build ──► BIN=scanoss-cli
goreleaser ──► binary: scanoss-cli  ├─ archives: scanoss-cli-<os>-<arch>
Dockerfile ──► ENTRYPOINT scanoss-cli
CI / release ► build & smoke-test scanoss-cli
cobra Use  ──► scanoss-cli (help, usage, completion registration)
docs       ──► install/extract/move scanoss-cli
```

## Key changes
- **Build:** `Makefile` `BIN ?= scanoss-cli`.
- **Command:** `cmd/root.go` `Use: "scanoss-cli"`.
- **Release config:** `.goreleaser.yaml` — `binary: scanoss-cli` (both builds),
  archive templates → `scanoss-cli-{{ .Os }}-{{ .Arch }}...` and
  `scanoss-cli-harmonyos-arm64`, footer references `scanoss-cli`.
- **Docker:** `Dockerfile` copy dest `/usr/local/bin/scanoss-cli` + matching
  `ENTRYPOINT`.
- **CI:** `ci.yml` `-o scanoss-cli` and `./scanoss-cli` smoke calls.
- **Release CI:** `release.yml` archive filenames + `scanoss-cli`/`scanoss-cli.exe`
  smoke calls.
- **Docs:** `README.md` install/build sections; `CHANGELOG.md` `### Changed`
  breaking-change note.

## Ordering rationale
Land the source-of-truth artifact producers first (Makefile, goreleaser,
Dockerfile, cobra), then the consumers that must match them (CI, release smoke
tests), then docs and changelog. This keeps every intermediate commit coherent:
CI is updated in the same or a later commit than the build output it invokes, so
the tree never references a name that isn't produced yet.

## Commit conventions
- **Conventional Commits** — this is a `refactor:` (no user-facing feature), with
  the changelog entry justifying a `## [Unreleased]` note despite being a refactor
  (it is a breaking rename users must act on).
- **Atomic commits** — one surface per commit where it keeps the tree coherent;
  build+cobra may pair with their direct consumers to avoid a broken CI ref.
- **Short** imperative subjects.

## Risks / trade-offs
- **Breaking change** for existing install scripts / download URLs / docker
  invocations — mitigated by the `CHANGELOG` note and release-notes callout;
  no code path depends on the name.
- **`go install` gap** — `cmd/scanoss` dir kept, so `go install` still yields
  `scanoss`. Documented in spec Out of scope; not silently ignored.
- **Missed reference risk** — mitigated by a final repo-wide grep for the old
  executable-name patterns (`-o scanoss`, `mv scanoss`, `./scanoss`,
  `binary: scanoss`) as a verification task.
