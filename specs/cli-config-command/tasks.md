# Tasks: persisted CLI settings (`config` command)

**Spec:** [`spec.md`](./spec.md) · **Plan:** [`plan.md`](./plan.md) · **Ticket:** [#17](https://github.com/scanoss/scanoss.go/issues/17)

`[P]` = parallelizable (different files). Paths relative to repo root.

## Conventions
- Conventional Commits; atomic; short subjects; no AI/co-author trailers.
- Every code task ships with unit tests.
- `make check` clean before presenting each commit.

## Phase 0 — Dependency
- [ ] **T000** `go get github.com/spf13/viper`; promote `github.com/spf13/pflag` to a
      direct requirement in `go.mod`. `go mod tidy`, then `make check` clean (verify
      golangci-lint is happy with the new imports before any feature code depends on
      them). No functional change in this commit.

## Phase 1 — `internal/cliconfig`: storage
- [ ] **T001** `internal/cliconfig/cliconfig.go` (new, with a package comment stating it
      is the *user/CLI* config — not the project `scanoss.json` — and that it must stay
      out of `pkg/`): `Path()` (`$HOME/.scanoss/settings.json`, home resolved through an
      injectable package var); key registry (`KeyAPIURL`, `KeyAPIKey`,
      `RecognizedKeys()`, `IsRecognized()`, `IsSecret()` — `api_key` is secret —
      and `EnvName(key)` → `"SCANOSS_" + strings.ToUpper(key)`, matching Viper's
      `SetEnvPrefix` derivation so the resolver and `AutomaticEnv` cannot disagree);
      `UnknownKeyError{Key}`; `Set`/`Unset`; and `Load`/`Keys` for `config list`.
      **Writes never use `viper.WriteConfig`** — a file-only Viper instance (no
      `AutomaticEnv`, no `BindPFlag`) reads the current file, `w.Set` applies the change,
      then `json.MarshalIndent(w.AllSettings(), …)` plus `atomicWrite`: `MkdirAll` 0700,
      temp file 0600 in the same directory, rename.
      Tests: missing file → empty config; save into a home with no `.scanoss` directory
      creates the directory and the file; save when `~/.scanoss` exists as a file →
      error naming the path, nothing written; reads create nothing; malformed JSON →
      error naming the path; unrecognized keys preserved across a set+save; file mode
      0600 and dir mode 0700; empty-string value reads as unset. **Plus the leak
      regression (FR-4a):** with `SCANOSS_API_KEY` exported, `Set(KeyAPIURL, …)` leaves
      `api_key` absent from the file on disk. (depends on T000)

## Phase 2 — `internal/cliconfig`: resolution
- [ ] **T002** Same package: `newViper()` (`SetConfigFile` + `SetConfigType("json")` +
      `SetEnvPrefix("SCANOSS")` + `AutomaticEnv`; a missing file is `*fs.PathError`, not
      `ConfigFileNotFoundError`, so check `errors.Is(err, os.ErrNotExist)`; a parse error
      is wrapped with the path), `API struct{ URL, Key }`, the `resolve` helper, and
      `ResolveAPI(flags *pflag.FlagSet) (API, error)` — `BindPFlag` per key, skipping a
      key whose flag is absent so `wfp`/`sbom` are unaffected. Viper resolves the value;
      `resolve` names the source, with **the two corrections in the plan**: the config
      rung requires `InConfig(key) && GetString(key) != ""` (Viper returns a
      present-but-empty config value ahead of the pflag default), and an explicit
      `default` rung returns `f.DefValue`. `slog.Debug` records the source per key, plus
      the value **only for non-secret keys** (`api_url` logs `source=… value=…`;
      `api_key` logs `source=…` alone). Scope is `api_url`/`api_key` only —
      `chunk-size`, `workers`, `ignore-cert-errors` stay flag-only.
      Tests use a real `*pflag.FlagSet` (`BindPFlag` needs the concrete type): each rung
      of flag > env > file > default; empty env falls through to file; empty file value
      falls through to default; malformed config file → error. **Plus the consistency
      test:** one table over the four rungs asserting the value Viper returns and the
      source `resolve` reports agree — this is what keeps the two encodings of the chain
      from drifting. (depends on T001)

## Phase 3 — Wire the call sites
- [ ] **T003** Switch the six read sites to `cliconfig.ResolveAPI(cmd.Flags())`,
      propagating the error instead of discarding it with `_`: `cmd/auth.go:66`
      (`checkAuth` — this is what makes a stored key satisfy the no-key guard),
      `cmd/purlcommon.go:167` (`clientOptions` — covers the five PURL commands and
      `components`), `buildScanClient` (`cmd/scan.go` ~`:510` — covers `scan`,
      `scan wfp`, and `enrich`, which registers the flags but never reads them),
      `cmd/results.go:74`, `cmd/attributions.go:109`, `cmd/dependencies.go`.
      `cmd/root.go` is not touched.
      Tests: extend `cmd/auth_test.go` so a stored key satisfies the guard end-to-end.
      (depends on T002)
- [ ] **T004** Guard test in `cmd/`: walk the package's own `.go` sources and fail if
      `GetString("api-key")` / `GetString("api-url")` appears outside the approved call
      sites, so a future command cannot silently bypass resolution. (depends on T003)

## Phase 4 — `config` command
- [ ] **T005** `cmd/config.go` (new): `configCmd` registered on `rootCmd`, plus
      `set <key> <value>` (rejects unrecognized keys, listing the valid ones; normalizes
      `api_url` via `normalizeURL`) and `unset <key>`. Both save atomically and confirm
      on stderr with the resolved path. `configCmd` carries a `Long` in the house style
      (`cmd/attributions.go:65`, `cmd/scan.go:308`): prose stating the precedence chain,
      the two env var names, the never-displayed-key rule, and that this is unrelated to a
      project's `scanoss.json`; then an `Examples:` block covering set-then-scan, the
      on-prem URL, `list`, `unset`, and `path`. Subcommands get one-line `Short` text only,
      so the examples live in one place. Tests against a temp home. (depends on T001)
- [ ] **T006** `cmd/config.go`: `get <key>` (bare value to stdout, non-zero exit when
      unset), `list` (every recognized key sorted, showing the **effective** value and the
      winning source — `(default)` / `(config file)` / `(env: SCANOSS_API_KEY)` /
      `(unset)` — plus a trailing `Config file: <path>` line; stored unrecognized keys
      listed too), `path` (absolute path whether or not the file exists). Both `get` and
      `list` render secret keys
      through one `display(key, value)` helper returning a constant `********` — no
      reveal flag, no partial reveal, constant width.
      Tests: `api_key` never appears in `list` or `get` output (assert the literal value
      is absent from the captured stream, not merely that stars are present); the
      unset-key exit; the registry's secret marking drives the masking. (depends on T005)

## Phase 5 — Docs & verification
- [ ] **T007 [P]** `README.md`: "Configuration" section — precedence chain,
      `SCANOSS_API_KEY`/`SCANOSS_API_URL`, `config` examples, the never-displayed-key
      rule, the `0600` note, the `config` row in the command table, and the user-config
      vs. project-`scanoss.json` clarification. **Text is already drafted** in
      [`readme-configuration.md`](./readme-configuration.md) — apply it in the same commit
      as T006, never ahead of it, so `main` does not document a command that does not
      exist yet.
- [ ] **T008 [P]** `CHANGELOG.md`: `[Unreleased] → Added` entry for the `config` command
      and the env-var/config-file resolution.
- [ ] **T009** Manual end-to-end check: with no key, `scan .` shows the banner; after
      `config set api_key TOKEN` it runs; `SCANOSS_API_KEY` overrides the file;
      `--api-key` overrides both; `config set api_url` retargets requests; an
      unrecognized key in the file survives a `config set`. `make check` clean.
