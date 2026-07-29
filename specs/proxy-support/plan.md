# Implementation Plan: proxy and custom CA support

**Spec:** [`spec.md`](./spec.md) · **Tasks:** [`tasks.md`](./tasks.md)

## Technical context
- **Go 1.25**, CLI on `spf13/cobra`. No new dependencies: `net/http`, `crypto/tls`,
  `crypto/x509`.
- **Three places build an HTTP client today**, each constructing a fresh
  `http.Transport`: `pkg/scanoss/client.go:152` (`WithInsecureTLS`),
  `pkg/api/client.go:88` (`SetInsecureTLS`), and `cmd/httpclient.go:36`
  (`newHTTPClient`, used by `attributions` at `cmd/attributions.go:236` for its raw
  multipart upload).
- **`cmd` does not import `pkg/api`.** The CLI's SDK is `pkg/scanoss`; `pkg/api` is used
  only by `libscanoss/core/libscanoss.go:124` for the C/Python/Node bindings. So the
  change is `pkg/scanoss` + `cmd`.
- **The API flags are declared in five places, four of them by hand.** `addAPIFlags`
  (`cmd/purlcommon.go:45`) already declares exactly `api-url`, `api-key`,
  `ignore-cert-errors` and `output`, but only the five PURL commands and `components` call
  it. `scan` (`:331`), `enrich` (`:67`), `attributions` (`:89`) and `dependencies` (`:112`)
  declare the same four by hand, and `results` declares three — it is missing
  `ignore-cert-errors`. The duplication is not carelessness: the shared helper lives in a
  file named `purlcommon.go`, so nobody writing `scan` had a reason to look for it there.

### The bug this builds on
A zero-value `http.Transport` has `Proxy == nil` — no proxy at all — while
`http.DefaultTransport` carries `Proxy: http.ProxyFromEnvironment`. Verified with
`HTTPS_PROXY=http://proxy.invalid:8080`:

| client | result |
| --- | --- |
| `&http.Client{}` | `proxyconnect tcp: lookup proxy.invalid: no such host` |
| `&http.Client{Transport: &http.Transport{TLSClientConfig: …}}` | succeeds, proxy ignored |

Cloning `DefaultTransport` instead of building one fixes it, and is why the environment
case needs no code of ours.

## Design overview
One constructor, three inputs, shared by the CLI and SDK consumers:

```go
// pkg/scanoss
type TransportOptions struct {
	Proxy      string // proxy URL; empty honors HTTP(S)_PROXY / NO_PROXY
	CACertFile string // extra CA PEM, added to the system pool
	Insecure   bool   // skip verification entirely — insecure
}

func NewHTTPClient(opts TransportOptions) (*http.Client, error)
```

Used with the option that already exists, so the SDK needs no new option:

```go
hc, err := scanoss.NewHTTPClient(scanoss.TransportOptions{
	Proxy:      "http://proxy.example.com:8080",
	CACertFile: "/etc/ssl/corp-ca.pem",
})
if err != nil {
	return err
}
client := scanoss.New(scanoss.WithAPIKey(key), scanoss.WithHTTPClient(hc))
```

**Why a constructor and not `WithProxy`/`WithCACert` options.** `New` returns `*Client`
with no error, so an option that reads a file would have to defer its failure to the first
request. Returning `(*http.Client, error)` puts the error where the cause is, and gives
one implementation for both the CLI and the SDK.

`WithInsecureTLS` stays, reimplemented over `NewHTTPClient` so it stops dropping the proxy.

## Key changes
- `pkg/scanoss/httpclient.go` (new — `transport.go` already holds the retry transport,
  so the two names split cleanly: how we retry versus how we connect): `TransportOptions`,
  `NewHTTPClient`. Clone
  `http.DefaultTransport`, then set what was asked for. The proxy is checked for an
  `https://`/`http://` prefix first and rejected naming the value otherwise — `url.Parse`
  reads a scheme-less `host:port` as a scheme with no host, which surfaces as
  `proxyconnect tcp: dial tcp :0`. Then `http.ProxyURL` for the proxy,
  `SystemCertPool` + `AppendCertsFromPEM` for the CA — a false return means zero
  certificates were added, which is an error naming the path — and `InsecureSkipVerify` for
  insecure. The bool is the whole check: `pem.Decode` and `x509.ParseCertificate` could say
  *which* way the file is wrong, but the user's next step is the same, so that is a
  follow-up if it is ever asked for.
  Each is left untouched when unset, so an empty `Proxy` keeps `ProxyFromEnvironment`.
- `pkg/scanoss/client.go`: `WithInsecureTLS` delegates to `NewHTTPClient`.
- `pkg/api/client.go`: add `SetHTTPClient(*http.Client)`; `SetInsecureTLS` delegates to
  it. No behaviour change for existing callers.
- `cmd/httpclient.go` is deleted. It exists to build an insecure client for the one raw
  HTTP call in `attributions`, and keeping it would mean a second file named
  `httpclient.go` whose only job is to forward to the first. The CLI builds its client the
  same way everywhere.
- `cmd/apicommon.go` (new): `addAPIFlags` moves here from `purlcommon.go`, and the five
  commands that declare the four API flags by hand call it instead. A neutral file name is
  the point — the helper was invisible inside `purlcommon.go`. `clientOptions` stays where
  it is: it also reads `chunk-size` and `workers`, which belong to the PURL input.
- `cmd/`: add `--proxy` and `--ca-cert` **inside `addAPIFlags`**, so two lines reach all
  eleven commands. Read them where `--ignore-cert-errors` is already read and pass all
  three into one `NewHTTPClient` call injected with `WithHTTPClient`. Where the insecure
  warning is already emitted, add a second line when a CA file was also given: it has no
  effect with verification off.
- `README.md` + `CHANGELOG.md`.

## Testing strategy
- `pkg/scanoss`: `NewHTTPClient` table tests — proxy set and unset (an empty `Proxy` must
  leave `ProxyFromEnvironment` in place), a PEM reaching `RootCAs`, an unreadable file
  erroring with the path, `Insecure` setting the flag.
- **Regression for the bug:** with `HTTPS_PROXY` set, a client built with `Insecure: true`
  must still resolve a proxy — the assertion that fails before this change.
- End to end against a local HTTPS server with a self-signed CA: the request fails without
  `--ca-cert` and succeeds with it, verification on. No local proxy needed — asserting the
  resolved `Proxy` is enough and keeps the test hermetic.
- `cmd`: the flags reach the builder.

## Commit conventions
- Conventional Commits, atomic, short imperative subjects, no AI/co-author trailers.
- `CHANGELOG.md` in the product-changing commits.

## Risks / trade-offs

**The consolidation changes four commands from local to persistent flags.** `addAPIFlags`
uses `PersistentFlags()`; the four that declare by hand use `Flags()`. With no subcommands
the two behave the same, and `scan` actively wants persistent so `scan wfp` inherits — but
that is an assumption to confirm against the existing tests, not to take on faith.


**The fix can break a working setup.** Today `--ignore-cert-errors` bypasses the proxy, so
someone whose network exports `HTTPS_PROXY` for a proxy that blocks `api.scanoss.com` has a
scan that works *because* of the bug. After this change their request goes through that
proxy and starts failing, and from where they stand the upgrade broke the scan. That is why
the CHANGELOG needs a `Fixed` line and not only `Added` for the new flags: it is the only
place they will find the explanation.

**`--ca-cert` cannot express "trust only my CA".** The PEM is added to the system pool, so
the CLI trusts the public authorities *and* the internal one.

Trusting only the internal one — pinning — is stricter: a fraudulent certificate from any
public authority would not be accepted. But replacing the pool breaks the ordinary case,
because the public endpoint's certificate is signed by a public authority. With the pool
replaced, `--ca-cert corp-ca.pem` would make an internal endpoint work and make
`https://api.scanoss.com` fail with `x509: certificate signed by unknown authority` — and
`--ca-cert` is the kind of flag that ends up in an alias or a CI job, so the same setting
would silently break every run against the public API.

Nobody has asked for pinning, so the permissive option wins and the strict one is
unavailable.
