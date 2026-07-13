# Implementation Plan: TLS certificate verification toggle

**Spec:** [`spec.md`](./spec.md) · **Tasks:** [`tasks.md`](./tasks.md)

## Technical context
- **Language/module:** Go 1.22, `github.com/scanoss/scanoss.go`.
- **Two HTTP clients to cover (asymmetric APIs):**
  - `pkg/scanoss` decoration SDK — has an options pattern
    (`pkg/scanoss/client.go:41-103`, incl. `WithHTTPClient`). Add a sibling
    `WithInsecureTLS(bool)` Option.
  - `pkg/api` scan client — no options; three constructors hardcode
    `&http.Client{}` (`pkg/api/client.go:24-58`). Add a post-construction method
    `SetInsecureTLS(bool)` so all three are covered without signature changes.
- **Reused building blocks:**
  - Decoration flag wiring: `addPurlServiceFlags` + `runPurlService`
    (`cmd/purlcommon.go:19`, `:117`, client built at `:130`).
  - Scan flag wiring: flag registration ~`cmd/scan.go:186`, client built at
    `api.NewClientWithMode(...)` `cmd/scan.go:322`.
  - Existing bool-flag convention (`--default-filters`, `--gitignore`).
- **Reference implementation:** scanoss.py `scanossapi.py` (`ignore_cert_errors`
  → `verify=False`) and `cli.py` (`--ignore-cert-errors`).
- **No new dependencies** — stdlib `crypto/tls`, `net/http`.

## Design overview
A single insecure transport — `&http.Transport{TLSClientConfig:
&tls.Config{InsecureSkipVerify: true}}` — is installed on the relevant client
only when the toggle is true; otherwise the client is left at its secure default.
The toggle is surfaced idiomatically per package (SDK option vs. method) and
driven by one CLI flag per command group.

```
--ignore-cert-errors ─┬─► decoration cmds ─► scanoss.New(..., WithInsecureTLS(true))
                      └─► scan cmd        ─► apiClient.SetInsecureTLS(true)
```

## Key changes
- `pkg/scanoss/client.go`: add `WithInsecureTLS(insecure bool) Option` (import
  `crypto/tls`). When true, replace `c.http` with an insecure-transport client.
- `pkg/api/client.go`: add `func (c *Client) SetInsecureTLS(insecure bool)`
  (import `crypto/tls`). When true, replace `c.client` with an insecure-transport
  client. Constructors unchanged.
- `cmd/purlcommon.go`: register `--ignore-cert-errors` in `addPurlServiceFlags`;
  in `runPurlService` read it, append `scanoss.WithInsecureTLS(v)`, and warn on
  stderr when set. Covers all four decoration commands automatically.
- `cmd/scan.go`: register the flag; read it; call `apiClient.SetInsecureTLS(v)`
  after construction; warn on stderr when set.
- `README.md` + `CHANGELOG.md`: document the flag (Added) and note it is insecure.

## Commit conventions
- **Conventional Commits** (`feat:`, `fix:`, `test:`, `docs:`, `refactor:`).
- **Atomic commits** — one logical change per commit (roughly one task/sub-task).
- **Short** commit subjects (imperative mood).

## Risks / trade-offs
- Security footgun by design — mitigated by insecure-by-name flag, help text, and
  a runtime warning; never default.
- `dependencies` command intentionally not covered (separate client); listed as a
  known gap to avoid silent partial coverage.
