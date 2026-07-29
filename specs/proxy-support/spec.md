# Feature Specification: proxy and custom CA support

**Feature branch:** `feat/proxy-support`
**Status:** Draft
**SDD Change:** `proxy-support`

## Summary
Let the CLI and the SDK reach the SCANOSS API through an HTTP proxy, and trust a CA that
is not in the system pool. Two flags:

```bash
scanoss-cli scan . --proxy http://proxy.example.com:8080
scanoss-cli scan . --ca-cert /etc/ssl/corp-ca.pem
```

With no flags, Go already honors `HTTP_PROXY`, `HTTPS_PROXY` and `NO_PROXY`; `--proxy`
overrides them for one run. The SDK gets the same capability through an exported
constructor.

### This also fixes a silent bug
`--ignore-cert-errors` currently disables proxy support. The three places that build an
insecure client each construct a fresh `http.Transport`, whose `Proxy` field is nil, so
the request bypasses `HTTPS_PROXY` and goes direct. Verified: with
`HTTPS_PROXY=http://proxy.invalid:8080` the default client fails on `proxyconnect` while
the insecure client succeeds by ignoring the proxy.

## User Scenarios & Testing

### Primary user story
As a developer on a corporate network, I want scans to go through our proxy and trust our
internal CA, so the CLI works without disabling certificate verification.

### Acceptance scenarios
1. **Given** `HTTPS_PROXY` is exported, **when** I run any API command with no proxy
   flag, **then** the request goes through that proxy.
2. **Given** `HTTPS_PROXY` is exported, **when** I pass `--proxy <url>`, **then** the flag
   wins for that run.
3. **Given** an endpoint signed by an internal CA, **when** I pass `--ca-cert <path>`,
   **then** the request succeeds with verification **on**.
4. **Given** a proxy that intercepts TLS and presents a certificate signed by an internal
   CA, **when** I run `scan . --ca-cert <path>` against the default public endpoint with no
   `--api-url`, **then** the request succeeds with verification **on**. This is the most
   common invocation of the feature: the CA applies to whatever endpoint the run targets,
   never only to internal ones.
5. **Given** `--proxy` and `--ca-cert` together, **then** both apply.
6. **Given** `--ca-cert` and `--ignore-cert-errors` together, **then** verification is off
   and a warning says the CA file has no effect.
7. **Given** `--ignore-cert-errors` and an exported `HTTPS_PROXY`, **then** the proxy is
   still used — the bug above is fixed.
8. **Given** an SDK consumer using the exported constructor, **then** proxy and CA
   settings apply to every SDK call.

### Observable behavior

```console
$ scanoss-cli scan . --proxy proxy.example.com:8080
Error: --proxy must start with https:// or http:// (got "proxy.example.com:8080")

$ scanoss-cli scan . --ca-cert /nonexistent.pem
Error: reading --ca-cert /nonexistent.pem: no such file or directory

$ scanoss-cli scan . --ca-cert /etc/hosts
Error: --ca-cert /etc/hosts contains no certificate

$ scanoss-cli scan . --ca-cert /etc/ssl/corp-ca.pem --ignore-cert-errors
WARN ignoring TLS certificate errors (insecure)
WARN --ca-cert has no effect with --ignore-cert-errors
```

### Edge cases
- No flags, no environment → today's behavior exactly.
- `NO_PROXY` keeps working, because the transport keeps Go's `ProxyFromEnvironment`.
- `--ca-cert` adds to the system pool rather than replacing it, so a run against the
  public API keeps working with an internal CA configured.
- An unreadable `--ca-cert` file is an error naming the path, before any request.
- `HTTP_PROXY`/`HTTPS_PROXY` keep Go's own forgiving handling, which accepts a scheme-less
  value. Only the flag is checked, so an existing working environment is never rejected.

## Requirements
- **FR-1** `--proxy <url>` sets the proxy for the run, overriding the environment. It must
  start with `https://` or `http://`; a value without one is rejected naming the value, the
  same rule and message shape as `config set api-url`. Go itself is forgiving here — it
  assumes `http://` for a scheme-less `HTTPS_PROXY` — but `url.Parse` reads
  `proxy.example.com:8080` as `scheme="proxy.example.com"` with no host, which reaches the
  user as `proxyconnect tcp: dial tcp :0`, a message that names neither the proxy nor the
  missing scheme.
- **FR-2** `--ca-cert <path>` adds the PEM's certificates to the system pool.
  Verification stays on.
- **FR-3** With neither flag, `HTTP_PROXY`/`HTTPS_PROXY`/`NO_PROXY` are honored.
- **FR-4** Both flags are available on every command that already offers
  `--ignore-cert-errors`: `scan` (+ `scan wfp`), `enrich`, `attributions`,
  `dependencies`, `components`, and the five PURL service commands. Plus `results`, which
  has none of the three today.
- **FR-5** The three settings compose: proxy, CA file and insecure are inputs to one
  transport, so no combination silently drops another.
- **FR-6** A `--ca-cert` file that cannot be read, or that holds no certificate, is an
  error naming the path. `AppendCertsFromPEM` returning false is the signal for the second:
  it reports that zero certificates were added, which is exactly the case worth refusing.
  It cannot say *why* — plain text, a private key and a corrupt certificate all look the
  same to it — and one message is enough, since the user's next step is the same either way.
- **FR-7** `--ca-cert` together with `--ignore-cert-errors` warns that the CA file has no
  effect, since verification is off. A warning, not an error: passing both is pointless
  rather than wrong, and failing would break a script that sets flags unconditionally.
- **FR-8** The SDK exposes the same capability as an exported constructor returning an
  `*http.Client` and an error, used with the existing `WithHTTPClient` option.
- **NFR-1** No new dependencies: `net/http`, `crypto/tls`, `crypto/x509`.
- **NFR-2** The transport clones `http.DefaultTransport`, so Go's proxy handling,
  timeouts and connection pooling are kept rather than re-declared.

## Out of scope
- **Proxy auto-configuration (PAC).** A PAC file is JavaScript, and Go has no stdlib
  support for executing it. The README says so, and says to read the proxy out of the PAC
  and pass `--proxy` instead.
- **Client certificates (mTLS).** `--ca-cert` is the CA to trust, not a certificate to
  present.
- **Proxy credentials as separate flags** — they go in the URL, which Go already parses.
- **Storing `proxy`/`ca_cert` in `~/.scanoss/settings.json`.** They fit the model, but
  this change is flags and environment only.
- **`pkg/api`** (used only by `libscanoss` for the C/Python/Node bindings) gains an
  injection point so it can be handed a configured client; wiring the bindings is
  separate work.
