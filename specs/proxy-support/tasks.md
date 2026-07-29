# Tasks: proxy and custom CA support

**Spec:** [`spec.md`](./spec.md) · **Plan:** [`plan.md`](./plan.md)

`[P]` = parallelizable (different files). Paths relative to repo root.

## Conventions
- Conventional Commits; atomic; short subjects; no AI/co-author trailers.
- Every code task ships with unit tests.
- `make check` clean before presenting each commit.

## Phase 0 — One home for the API flags
- [ ] **T000** `cmd/apicommon.go` (new): move `addAPIFlags` out of `purlcommon.go`, whose
      name hid it from every command that is not a PURL one. Have `scan`, `enrich`,
      `attributions`, `dependencies` and `results` call it instead of declaring `api-url`,
      `api-key`, `ignore-cert-errors` and `output` by hand — `results` gains
      `ignore-cert-errors`, which it is missing today. No behaviour change otherwise; the
      one thing to confirm rather than assume is that moving those four commands from
      `Flags()` to the helper's `PersistentFlags()` leaves the existing tests green.
      This is what turns T004 into two lines in one file.

## Phase 1 — The transport
- [ ] **T001** `pkg/scanoss/transport.go` (new): `TransportOptions{Proxy, CACertFile,
      Insecure}` and `NewHTTPClient(opts) (*http.Client, error)`, built by cloning
      `http.DefaultTransport`. Set only what was asked for: the proxy must start with
      `https://` or `http://` and is rejected naming the value otherwise, then
      `http.ProxyURL`; `SystemCertPool` + `AppendCertsFromPEM` for the CA, erroring with the
      path when the file cannot be read and when it holds no certificate;
      `InsecureSkipVerify` for insecure.
      Tests: each input alone and combined; an empty `Proxy` keeps `ProxyFromEnvironment`;
      a scheme-less proxy is rejected; an unreadable CA file and a certificate-free one
      both error with the path; **and the regression — with `HTTPS_PROXY` set,
      `Insecure: true` must still resolve a proxy.**
- [ ] **T002** `pkg/scanoss/client.go`: `WithInsecureTLS` delegates to `NewHTTPClient`, so
      it stops dropping the proxy. (depends on T001)

## Phase 2 — CLI wiring
- [ ] **T003** `cmd/httpclient.go`: `newHTTPClient` takes proxy, CA file and insecure and
      delegates to `scanoss.NewHTTPClient`, returning its error. Update its one caller,
      `cmd/attributions.go:236`. (depends on T001)
- [ ] **T004** Add `--proxy` and `--ca-cert` inside `addAPIFlags` — two lines in
      `cmd/apicommon.go`, reaching all eleven commands. (depends on T000)
- [ ] **T005** Read the three flags where `--ignore-cert-errors` is already read
      (`clientOptions`, `buildScanClient`, `runAttributions`, `runDependencies`,
      `runResults`) and pass them as one `NewHTTPClient` call injected with
      `WithHTTPClient`. Warn that `--ca-cert` has no effect when `--ignore-cert-errors` is
      also set (FR-7), beside the insecure warning that already exists.
      Tests: the flags reach the builder; both flags together produce the warning.
      (depends on T003, T004)

## Phase 3 — Bindings hook
- [ ] **T006 [P]** `pkg/api/client.go`: add `SetHTTPClient(*http.Client)` and have
      `SetInsecureTLS` delegate to it, so `libscanoss` can be handed a configured client.

## Phase 4 — Docs & verification
- [ ] **T007 [P]** `README.md`: `--proxy` and `--ca-cert`, that
      `HTTP(S)_PROXY`/`NO_PROXY` work with no flags, that `--ca-cert` adds to the system
      pool with verification on, the SDK snippet, and a line on PAC not being supported.
- [ ] **T008 [P]** `CHANGELOG.md`: `Added` for the flags and the SDK constructor;
      **`Fixed`** for `--ignore-cert-errors` no longer bypassing the proxy.
- [ ] **T009** End-to-end check: a local HTTPS server with a self-signed CA fails without
      `--ca-cert` and succeeds with it; `HTTPS_PROXY` is honored with no flags and
      `--proxy` overrides it; `--ignore-cert-errors` with `HTTPS_PROXY` now uses the proxy.
      `make check` and `go test -race ./...` clean.
