# Implementation Plan: store proxy and ca-cert in the config file

**Spec:** [`spec.md`](./spec.md) · **Tasks:** [`tasks.md`](./tasks.md) · **Ticket:** [#20](https://github.com/scanoss/scanoss.go/issues/20)

## Technical context
- The groundwork exists. `internal/cliconfig` already owns the key registry, the file,
  and the precedence chain; everything user-facing derives from the registry, so most of
  this change is two entries in one map.
- `newHTTPClient` (`cmd/apicommon.go`) currently reads the two values straight off the
  flags with `cmd.Flags().GetString`. That is the only place that has to start resolving
  instead of reading.
- `cliconfig.ResolveAPI(flags)` returns `API{URL, Key}` for the two existing keys. There
  is no equivalent for the transport pair.

## Design overview
Two registry entries plus one resolver function.

```go
// internal/cliconfig/registry.go
var registry = map[string]keySpec{
	KeyAPIURL:  {cli: "api-url", def: config.DefaultAPIURL},
	KeyAPIKey:  {cli: "api-key", secret: true},
	KeyProxy:   {cli: "proxy"},    // new
	KeyCACert:  {cli: "ca-cert"},  // new
}
```

Neither carries `def` (no built-in default) or `secret`. `EnvName` derives
`SCANOSS_PROXY` and `SCANOSS_CA_CERT` from the key names, matching what viper's
`AutomaticEnv` does, so the environment rung needs no new code.

Everything that reads the registry follows for free: `config set`/`get`/`unset`
validation, the unrecognized-key message, `config list` (both appear with their source),
and the precedence chain.

### Resolving them together
`resolve.go` gains a sibling to `ResolveAPI`, so the file is read once per command
rather than once per key:

```go
// Transport is the resolved transport configuration.
type Transport struct {
	Proxy      string
	CACertFile string
}

func ResolveTransport(flags *pflag.FlagSet) (Transport, error)
```

Symmetric with `ResolveAPI`, and it keeps the private `resolver` private.

### Where the CLI changes
`newHTTPClient` swaps two flag reads for one call:

```go
transport, err := cliconfig.ResolveTransport(cmd.Flags())
```

`ignore-cert-errors` keeps being read from the flag — it is deliberately not a config
key (FR-6), so there is nothing to resolve.

### Validation
`config set proxy` must reject a scheme-less value like `--proxy` does. The check lives
in `cmd/config.go` beside the `api-url` normalization, which is already a per-key `if`;
this adds a second arm. Nothing moves into `cliconfig`, which stays free of CLI policy.

## Key changes
- `internal/cliconfig/registry.go`: `KeyProxy`, `KeyCACert`, and their registry entries.
- `internal/cliconfig/resolve.go`: `Transport` and `ResolveTransport`.
- `cmd/apicommon.go`: `newHTTPClient` resolves instead of reading flags.
- `cmd/config.go`: the scheme check for `config set proxy`.
- `README.md`, `CHANGELOG.md`.

## Testing strategy
- `internal/cliconfig`: the two new keys through the existing registry table (recognized,
  not secret, correct `EnvName`, correct CLI spelling); `ResolveTransport` through the
  four rungs, reusing the pattern that covers `ResolveAPI`.
- `cmd`: a stored `ca-cert` reaches the transport with no flag; `--proxy` overrides a
  stored one; `config set proxy` without a scheme is rejected; `config set
  ignore-cert-errors` is unrecognized.
- End to end against the local CA server already used for `proxy-support` T009: store the
  CA with `config set`, then run with no flag and confirm the handshake succeeds.

## Commit conventions
Conventional Commits, atomic, short subjects, no AI/co-author trailers. `CHANGELOG.md` in
the product-changing commit.

## Risks / trade-offs
**A stored `ca-cert` can rot.** A path that is later moved or deleted breaks every
command until it is unset, where a flag would only break the run that used it. The error
names the path, which is the best available signal; the alternative — ignoring an
unreadable stored file — would silently drop the CA and fail later with an unknown-authority
error instead.

**`SCANOSS_PROXY` is new vocabulary.** `HTTP_PROXY`/`HTTPS_PROXY` are the conventional
names and keep working; `SCANOSS_PROXY` exists only because the registry derives an
environment variable per key. It is documented as the scanoss-specific override rather
than as the way to set a proxy.
