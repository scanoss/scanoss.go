# Tasks: Rename CLI binary to `scanoss-cli`

**Spec:** [`spec.md`](./spec.md) · **Plan:** [`plan.md`](./plan.md)

`[P]` = parallelizable (different files). Paths relative to repo root.

## Conventions
- Conventional Commits; atomic; short subjects; no AI/co-author trailers.
- Pure rename — keep `go build ./...`, `go vet ./...`, and tests green.
- **Base:** built on the real `main` (`42ed8b8`), which includes the `sbom` and
  `enrich` commands and their examples — hence the extra in-binary/doc scope below.

## Phase 1 — Build & command name
- [ ] **T001** `Makefile`: change `BIN ?= scanoss` → `BIN ?= scanoss-cli` (line 6).
      Verify `make build` produces `./scanoss-cli` and `make clean` removes it.
      Commit: `refactor: build scanoss-cli binary via make`.
- [ ] **T002 [P]** `cmd/root.go`: `Use: "scanoss"` → `Use: "scanoss-cli"` (line 36).
      Verify `./scanoss-cli --help` shows `scanoss-cli [command]` and
      `completion` registers under `scanoss-cli`.
      Commit: `refactor: rename root command to scanoss-cli`.

## Phase 2 — Release artifacts & Docker
- [ ] **T003** `.goreleaser.yaml`: set `binary: scanoss-cli` on builds `scanoss`
      (:10) and `harmonyos` (:30); archive `name_template`s →
      `scanoss-cli-{{ .Os }}-{{ .Arch }}{{ with .Arm }}v{{ . }}{{ end }}` (:42) and
      `scanoss-cli-harmonyos-arm64` (:50); update release footer (:93-99, :110)
      to reference `scanoss-cli`. Leave `project_name`, image repo, and labels
      unchanged. Sanity-check with `goreleaser check` if available.
      Commit: `refactor: produce scanoss-cli release artifacts`.
- [ ] **T004 [P]** `Dockerfile`: `COPY` dest and `ENTRYPOINT` →
      `/usr/local/bin/scanoss-cli` (lines 7-8).
      Commit: `refactor: set docker entrypoint to scanoss-cli`.

## Phase 3 — CI consumers (must match Phase 1-2 output)
- [ ] **T005** `.github/workflows/ci.yml`: `go build -v -o scanoss-cli
      ./cmd/scanoss-cli` (:44) and update smoke calls `./scanoss-cli --version` /
      `./scanoss-cli wfp ...` (:48-49). (depends on T001)
      Commit: `ci: build and smoke-test scanoss-cli`.
- [ ] **T006** `.github/workflows/release.yml`: update archive matrix filenames
      to `scanoss-cli-*` (:93-97) and smoke calls — Unix `chmod +x scanoss-cli`,
      `./scanoss-cli --version|--help` (:108-110); Windows `.\scanoss-cli.exe
      --version|--help` (:116-117). (depends on T003)
      Commit: `ci: smoke-test scanoss-cli release archives`.

## Phase 3b — In-binary examples & runtime hints
- [ ] **T006b** `cmd/root.go`: `--version` template → `scanoss-cli {{.Version}}`
      and its comment. `cmd/*.go`: every cobra `Example:` block → `scanoss-cli
      <subcommand>`. `cmd/scan.go`: interrupted-scan resume hint → `scanoss-cli
      results <id>`. `pkg/scanoss/scan.go`: doc comment. Keep `config.AppName`,
      "scanoss raw" format name, PURLs, and SDK API untouched. Verify `--version`,
      `--help`, and a subcommand `--help` render `scanoss-cli`.
      Commit: `refactor: use scanoss-cli in command examples and hints`.

## Phase 4 — Docs & changelog
- [ ] **T007 [P]** `README.md`: update Prebuilt binary (extract/`mv scanoss-cli`),
      Docker example note if needed, and Build-from-source
      (`go build -o scanoss-cli ./cmd/scanoss-cli`) — lines 64-76, 85-106.
      Update the `go install` section to `…/cmd/scanoss-cli@latest` (now yields
      `scanoss-cli` directly).
      Commit: `docs: document scanoss-cli binary name`.
- [ ] **T007b [P]** Usage-example docs: `README.md` Quick-start invocations
      (incl. `enrich`), all `CLIENT_HELP.md` CLI invocations + prose,
      `.github/workflows/README.md` asset/binary names, and the `libscanoss/docs`
      CLI subprocess/exec examples (`nodejs.md` execSync, `integration.md`
      subprocess). Preserve non-binary references (`scanoss.json`,
      `api.scanoss.com`, PURLs, "scanoss raw", SDK API, `libscanoss-*` artifacts,
      package paths). Commit: `docs: use scanoss-cli in CLI usage examples`.
- [ ] **T008 [P]** `CHANGELOG.md`: add under `## [0.2.0]` a `### Changed`
      entry — CLI binary renamed `scanoss` → `scanoss-cli` (pre-1.0, not
      breaking; update install scripts / docker invocations).
      Commit: `docs: changelog for scanoss-cli rename`.

## Phase 5 — Verification
- [ ] **T009** Repo-wide grep for stray old-name references in scope:
      `grep -rnE '(-o scanoss\b|mv scanoss\b|\./scanoss\b|binary: scanoss\b|scanoss\.exe)' \
      Makefile .goreleaser.yaml Dockerfile .github README.md` — expect only
      intentional keeps (image repo, project_name, labels).
      Run `make build` → `./scanoss-cli --version` and `make check`. No commit
      (verification), or a follow-up fix commit if the grep surfaces a miss.
