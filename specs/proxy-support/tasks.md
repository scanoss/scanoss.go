# Tasks: proxy and custom CA support

**Spec:** [`spec.md`](./spec.md) · **Plan:** [`plan.md`](./plan.md)

`[P]` = parallelizable (different files). Paths relative to repo root.

## Conventions
- Conventional Commits; atomic; short subjects; no AI/co-author trailers.
- Every code task ships with unit tests.
- `make check` clean before presenting each commit.

## Phase 0 — One home for the API flags
- [x] **T000** `cmd/apicommon.go` (new): move `addAPIFlags` out of `purlcommon.go`, whose
      name hid it from every command that is not a PURL one. Have `scan`, `enrich`,
      `attributions`, `dependencies` and `results` call it instead of declaring `api-url`,
      `api-key`, `ignore-cert-errors` and `output` by hand — `results` gains
      `ignore-cert-errors`, which it is missing today. No behaviour change otherwise; the
      one thing to confirm rather than assume is that moving those four commands from
      `Flags()` to the helper's `PersistentFlags()` leaves the existing tests green.
      This is what turns T004 into two lines in one file.

## Phase 1 — The transport
- [x] **T001** `pkg/scanoss/httpclient.go` (new — `transport.go` is taken by the retry
      transport): `TransportOptions{Proxy, CACertFile,
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
- [x] **T002** `pkg/scanoss/client.go`: `WithInsecureTLS` delegates to `NewHTTPClient`, so
      it stops dropping the proxy. (depends on T001)

## Phase 2 — CLI wiring
- [x] **T003** Delete `cmd/httpclient.go`. Its `newHTTPClient(insecure)` has one caller
      (`cmd/attributions.go:236`) and would otherwise become a wrapper that calls
      `scanoss.NewHTTPClient` — a second file with the same name whose only job is to
      forward. The caller builds its client the same way every other command will.
      (depends on T001)
- [x] **T004** Add `--proxy` and `--ca-cert` inside `addAPIFlags` — two lines in
      `cmd/apicommon.go`, reaching all eleven commands. (depends on T000)
- [x] **T005** Read the three flags where `--ignore-cert-errors` is already read
      (`clientOptions`, `buildScanClient`, `runAttributions`, `runDependencies`,
      `runResults`) and pass them as one `NewHTTPClient` call injected with
      `WithHTTPClient`. Warn that `--ca-cert` has no effect when `--ignore-cert-errors` is
      also set (FR-7), beside the insecure warning that already exists.
      Tests: the flags reach the builder; both flags together produce the warning.
      (depends on T003, T004)

## Phase 3 — Bindings hook
- [x] **T006 [P]** `pkg/api/client.go`: add `SetHTTPClient(*http.Client)` and have
      `SetInsecureTLS` delegate to it, so `libscanoss` can be handed a configured client.

## Phase 4 — Docs & verification
- [x] **T007 [P]** `README.md`: `--proxy` and `--ca-cert`, that
      `HTTP(S)_PROXY`/`NO_PROXY` work with no flags, that `--ca-cert` adds to the system
      pool with verification on, the SDK snippet, and a line on PAC not being supported.
- [x] **T008 [P]** `CHANGELOG.md`: `Added` for the flags and the SDK constructor;
      **`Fixed`** for `--ignore-cert-errors` no longer bypassing the proxy.
- [x] **T009** End-to-end check, all against the built binary with an isolated environment
      and a local HTTPS server signed by its own CA:
      - **`--ca-cert`** — without it the handshake fails with
        `x509: certificate signed by unknown authority`; with it the request lands (the
        server logs `GET /v3/wfp/scan/abc` and the CLI complains about the *body*, not the
        certificate). Verification stays on throughout.
      - **the environment** — `HTTPS_PROXY` alone is honoured (`proxyconnect … lookup
        envproxy.invalid`), and `--proxy` overrides it (`… lookup flagproxy.invalid`).
      - **the regression** — `--ignore-cert-errors` with `HTTPS_PROXY` set now goes through
        the proxy, on all three client paths: `results` (SDK), `licenses` (PURL) and
        `attributions` (raw HTTP).
      - **proven against `main`** — the same `licenses --ignore-cert-errors` command built
        from `c3311ce` ignored the proxy and reached `api.scanoss.com` directly, returning a
        real 401. It was not merely skipping the proxy: it egressed straight to the internet,
        API key included, in a network where the proxy is meant to be the only way out.
      - `make check` clean, `go test -race ./...` clean, 36 tests across the feature.
