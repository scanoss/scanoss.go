# Tasks: store proxy and ca-cert in the config file

**Spec:** [`spec.md`](./spec.md) · **Plan:** [`plan.md`](./plan.md) · **Ticket:** [#20](https://github.com/scanoss/scanoss.go/issues/20)

## Conventions
- Conventional Commits; atomic; short subjects; no AI/co-author trailers.
- Every code task ships with unit tests.
- `make check` clean before presenting each commit.

## Phase 1 — The keys
- [ ] **T001** `internal/cliconfig/registry.go`: add `KeyProxy = "proxy"` and
      `KeyCACert = "ca_cert"` with registry entries `{cli: "proxy"}` and
      `{cli: "ca-cert"}` — no default, not secret.
      Tests: both recognized in the CLI spelling and rejected in the stored one (as the
      existing keys are); neither reports as secret; `EnvName` gives `SCANOSS_PROXY` and
      `SCANOSS_CA_CERT`; `CLIKeys` lists all four sorted.

## Phase 2 — Resolution
- [ ] **T002** `internal/cliconfig/resolve.go`: `Transport{Proxy, CACertFile}` and
      `ResolveTransport(flags) (Transport, error)`, built on the existing private
      resolver so the file is read once.
      Tests: each rung — flag, environment, config file, unset — mirroring the
      `ResolveAPI` table. (depends on T001)

## Phase 3 — CLI wiring
- [ ] **T003** `cmd/apicommon.go`: `newHTTPClient` takes the two values from
      `cliconfig.ResolveTransport` instead of reading the flags directly.
      `ignore-cert-errors` still comes from the flag — it is not a config key.
      Tests: a stored `ca-cert` reaches the transport with no flag passed; `--proxy`
      overrides a stored value. (depends on T002)
- [ ] **T004** `cmd/config.go`: `config set proxy` rejects a value without an
      `https://`/`http://` scheme, reusing `hasHTTPScheme` beside the `api-url` arm.
      Tests: rejected without a scheme, accepted with one, and
      `config set ignore-cert-errors` is still an unrecognized key.

## Phase 4 — Docs & verification
- [ ] **T005 [P]** `README.md`: one line in the Configuration section — these two can be
      stored like `api-key`, so the flags need not be repeated.
- [ ] **T006 [P]** `CHANGELOG.md`: extend the existing `config` entry rather than adding a
      new one; the command is unreleased, so it should read as having had four keys from
      the start.
- [ ] **T007** End-to-end: `config set ca-cert` against the local CA server from
      `proxy-support` T009, then a run with no flag; `config list` shows both keys with
      their source. `make check` and `go test -race ./...` clean.
