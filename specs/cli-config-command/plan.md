# Implementation Plan: persisted CLI settings (`config` command)

**Spec:** [`spec.md`](./spec.md) · **Tasks:** [`tasks.md`](./tasks.md) · **Ticket:** [#17](https://github.com/scanoss/scanoss.go/issues/17)

## Technical context
- **Language/module:** Go 1.25, `github.com/scanoss/scanoss.go`; CLI on `spf13/cobra`.
- **New dependency:** `github.com/spf13/viper` for reading the config file, deriving the
  environment variable names, and resolving the value of each key. `spf13/pflag` is
  already an indirect dependency (via cobra) and becomes direct.
- **Where `--api-url`/`--api-key` are declared** (six registration sites, all keeping
  their current flags and defaults — no flag changes in this feature):
  `cmd/purlcommon.go:44` `addAPIFlags` (persistent → the five PURL service commands via
  `addPurlServiceFlags`, plus `components`), `cmd/scan.go:322` (persistent → `scan wfp`
  inherits), `cmd/results.go:58`, `cmd/enrich.go:63`, `cmd/attributions.go:86`, and
  `cmd/dependencies.go` (which hardcodes the URL literal instead of
  `config.DefaultAPIURL`).
- **Where they are *read* — the sites this feature actually touches.** Six, and three are
  shared choke points:
  - `checkAuth` (`cmd/auth.go:66`) — the no-key guard, called from 9 places.
  - `clientOptions` (`cmd/purlcommon.go:167`) — SDK options builder for every PURL
    service command and `components`.
  - `buildScanClient` (`cmd/scan.go` ~`:510`) — SDK client for `scan`, `scan wfp`, and
    `enrich` (`cmd/enrich.go:121`).
  - `cmd/results.go:74`, `cmd/attributions.go:109`, `cmd/dependencies.go` (~`:400`).

  So eleven commands are served by six reads: three shared helpers cover nine of them,
  and only `results`, `attributions`, and `dependencies` read directly. `enrich`
  registers the flags but never reads them itself.
- **Cobra detail that shapes the resolver:** a flag always carries a value (`--api-url`
  defaults to `https://api.scanoss.com`), so the returned string cannot tell you whether
  the user chose it. `flags.Changed(name)` is the only reliable "the user typed this"
  signal, and it is what gives the flag its top rung — both for Viper (`BindPFlag`
  consults it internally) and for the provenance helper below.

## Design overview
One CLI-only module owns the whole concern — the file, the key registry, the environment
variable names, and the precedence between them. Commands ask it for the effective value
instead of reading a flag and guessing.

```
cmd/*  ──► cliconfig.ResolveAPI(flags) ──┬─ flag set (Changed)       ──► that value
                                         ├─ $SCANOSS_API_URL/_KEY    ──► that value
                                         ├─ ~/.scanoss/settings.json ──► that value
                                         └─ none                     ──► flag default
```

Viper resolves the **value**; a small helper repeats the same ladder to report the
**source**, which `config list` needs (FR-11) and Viper does not expose. The duplication
is deliberate and guarded by a consistency test (see Testing strategy).

Two properties are the point of this shape:

- **Flags keep meaning "what the user typed."** Nothing mutates flag state, so a value
  read in `RunE` is never something the user did not write.
- **`internal/` makes the SDK boundary a compiler guarantee.** `pkg/scanoss` and
  `pkg/api` *cannot* import `internal/cliconfig`, so NFR-2 is enforced by the build
  rather than by convention. Env and file resolution stay CLI-only by construction, and
  Viper never reaches the SDK.

### The resolver

```go
func newViper() (*viper.Viper, error) {
	path, err := Path()
	if err != nil {
		return nil, err
	}
	v := viper.New()
	v.SetConfigFile(path)
	v.SetConfigType("json")
	v.SetEnvPrefix("SCANOSS")
	v.AutomaticEnv() // api_url → SCANOSS_API_URL

	// With SetConfigFile a missing file is *fs.PathError, not ConfigFileNotFoundError
	// (that one is only produced by path search), so both cases are checked.
	if err := v.ReadInConfig(); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	return v, nil
}

// resolve reports the effective value of one key and where it came from.
// Viper resolves the value; this ladder exists to name the source.
func resolve(v *viper.Viper, flags *pflag.FlagSet, key, flagName string) (value, source string) {
	f := flags.Lookup(flagName)
	if f != nil && f.Changed {
		return v.GetString(key), "flag"
	}
	if env := EnvName(key); os.Getenv(env) != "" {
		return v.GetString(key), "env: " + env
	}
	// InConfig alone is not enough: an empty value in the file counts as unset, and
	// Viper would otherwise return "" in preference to the flag default.
	if v.InConfig(key) && v.GetString(key) != "" {
		return v.GetString(key), "config file"
	}
	if f != nil {
		return f.DefValue, "default"
	}
	return "", "unset"
}
```

Two adjustments to the original sketch, both correctness rather than style:

- **The empty-value guard on the config rung.** Viper treats a present-but-empty config
  value as found and returns `""` ahead of the pflag default, which contradicts the
  "empty means unset" rule. Checking the value as well as `InConfig` restores it.
  (Viper's own `AutomaticEnv` already treats an empty *env* var as unset unless
  `AllowEmptyEnv` is enabled, so the env rung needs no equivalent guard — the helper's
  `os.Getenv(env) != ""` matches that default.)
- **An explicit `default` rung returning `f.DefValue`**, so the source is nameable and
  the value does not depend on Viper's last-resort pflag fallback.

`ResolveAPI` wires the two flags and returns both keys:

```go
func ResolveAPI(flags *pflag.FlagSet) (API, error) {
	v, err := newViper()
	if err != nil {
		return API{}, err
	}
	if err := v.BindPFlag(KeyAPIURL, flags.Lookup("api-url")); err != nil {
		return API{}, err
	}
	if err := v.BindPFlag(KeyAPIKey, flags.Lookup("api-key")); err != nil {
		return API{}, err
	}
	url, urlSource := resolve(v, flags, KeyAPIURL, "api-url")
	key, keySource := resolve(v, flags, KeyAPIKey, "api-key")

	slog.Debug("resolved api_url", "source", urlSource, "value", url)
	slog.Debug("resolved api_key", "source", keySource) // secret: source only
	return API{URL: url, Key: key}, nil
}
```

`BindPFlag` needs a non-nil flag, so a command that declares only one of the two (or
neither) must be handled — `ResolveAPI` skips the binding and the rung when
`flags.Lookup` returns nil, which is also what keeps `wfp` and `sbom` unaffected.

**Deliberately narrow.** `ResolveAPI` covers `api_url` and `api_key` only. `chunk-size`,
`workers`, and `ignore-cert-errors` — also read by `clientOptions` — stay flag-only and
keep reading straight from cobra.

### The write path
`viper.WriteConfig` serializes Viper's **merged** view, so a write from the resolving
instance would persist env- and flag-derived values into the file: `config set api_url X`
with `SCANOSS_API_KEY` exported would write that CI token to disk. Writes therefore use a
separate instance with neither `AutomaticEnv` nor `BindPFlag`, and the file is serialized
by us to keep `0600` and the atomic rename (`WriteConfig` writes `0644` and truncates in
place):

```go
func Set(key, value string) error {
	if !IsRecognized(key) {
		return &UnknownKeyError{Key: key}
	}
	path, err := Path()
	if err != nil {
		return err
	}
	// No AutomaticEnv, no BindPFlag: this instance must see the file and nothing else.
	w := viper.New()
	w.SetConfigFile(path)
	w.SetConfigType("json")
	if err := w.ReadInConfig(); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("parsing %s: %w", path, err)
	}
	w.Set(key, value)

	data, err := json.MarshalIndent(w.AllSettings(), "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return atomicWrite(path, data, 0o600) // temp file in the same dir, then rename
}
```

`w.AllSettings()` carries every key read from the file, so unrecognized keys are
preserved (FR-4) without extra work.

## Key changes
- `internal/cliconfig/cliconfig.go` (new) — one package:
  - **Storage.** `Path()` → `$HOME/.scanoss/settings.json` via `os.UserHomeDir()`, behind
    a package var so tests can inject a temp home. `Set`/`Unset` as above (write
    instance + `atomicWrite`), `Load`/`Keys` for `config list`.
  - **Registry.** `KeyAPIURL = "api_url"`, `KeyAPIKey = "api_key"`, `RecognizedKeys()`
    (sorted), `IsRecognized(key)`, `IsSecret(key)` (`api_key` is secret, so a future
    secret key inherits the never-display rule), `EnvName(key)` →
    `"SCANOSS_" + strings.ToUpper(key)`, matching Viper's `SetEnvPrefix` derivation so
    the helper and `AutomaticEnv` cannot disagree.
  - **Resolution.** `API struct{ URL, Key string }`, `ResolveAPI(flags *pflag.FlagSet)`,
    `newViper`, and `resolve` as above. A `Source` variant (or `ResolveAPIWithSources`)
    exposes the per-key source for `config list`.
  - **Typed error.** `UnknownKeyError{Key}` rendering
    `unrecognized key "x"; recognized keys are: api_key, api_url`, so `set`/`get`/`unset`
    share one message and `cmd` only propagates.
- `cmd/config.go` (new): `configCmd` + `set`/`get`/`list`/`unset`/`path`, built on the
  module's registry and storage. `cobra.ExactArgs` per subcommand, secret keys rendered
  as a constant `********` by a single `display(key, value)` helper used by both `list`
  and `get` (no reveal flag exists), and `set api_url` normalized through `normalizeURL`
  (`cmd/auth.go:59`). Help text per T005.
- **Six call sites** switch from `cmd.Flags().GetString(...)` to
  `cliconfig.ResolveAPI(cmd.Flags())`, propagating the error instead of discarding it
  with `_`: `cmd/auth.go:66` (`checkAuth`), `cmd/purlcommon.go:167` (`clientOptions`),
  `buildScanClient` (`cmd/scan.go`), `cmd/results.go:74`, `cmd/attributions.go:109`,
  `cmd/dependencies.go`. Resolving inside `checkAuth` is what makes a stored key satisfy
  the no-key guard (FR-9); the three shared helpers cover nine commands at once.
- **`cmd/root.go` is untouched.** No `PersistentPreRunE` hook, so nothing can disable
  resolution by shadowing it (cobra runs only the closest hook in the chain, and
  `cmd/root.go:47` is currently the only one).
- `go.mod` / `go.sum`: add `github.com/spf13/viper`, promote `github.com/spf13/pflag` to
  a direct requirement.
- `README.md`: per [`readme-configuration.md`](./readme-configuration.md).
- `CHANGELOG.md`: `[Unreleased] → Added`.

## Testing strategy
- `internal/cliconfig` storage: table tests with `t.Setenv` on the injected home —
  missing file, directory created from nothing, `~/.scanoss` present as a file,
  unrecognized-key preservation, `0600`/`0700` modes, malformed JSON, empty value as
  unset. Assert the write instance never persists an env-derived value: export
  `SCANOSS_API_KEY`, run `Set(KeyAPIURL, …)`, and assert `api_key` is absent from the
  file on disk. That is the regression test for the `WriteConfig` trap.
- **Consistency test for the duplicated ladder.** One table over the four rungs — flag
  only, env only, file only, nothing — asserting that the value Viper returns and the
  source `resolve` reports agree at every rung, plus empty-env and empty-file-value
  falling through. This is what keeps the two encodings of the chain from drifting.
- Resolution tests build a real `*pflag.FlagSet` (Viper's `BindPFlag` needs the concrete
  type, so there is no narrow interface to fake) — three lines per case.
- `cmd/config_test.go`: subcommands against a temp home — masking (assert the literal key
  value is absent from captured output, not merely that stars are present), unknown-key
  rejection, `get` on an unset key.
- `cmd/auth_test.go`: extend so a stored key satisfies the no-key guard.
- **Guard test** in `cmd/`: walk the package's own `.go` sources and fail if
  `GetString("api-key")` / `GetString("api-url")` appears outside the approved call
  sites, so a future command cannot silently bypass resolution.
- Windows: `os.UserHomeDir()` reads `USERPROFILE`; tests set both so CI stays green.

## Commit conventions
- **Conventional Commits** (`feat:`, `test:`, `docs:`).
- **Atomic commits** — one logical change per commit (one task ≈ one commit).
- **Short** imperative subjects; no AI/co-author trailers.
- `CHANGELOG.md` updated in the product-changing commits.

## Risks / trade-offs
- **The precedence ladder exists twice** — inside Viper for the value, in `resolve` for
  the source. Accepted because Viper exposes no per-key source API (`viper.Debug()`
  prints all layers to stdout and is not usable programmatically), and `config list`
  needs the source (FR-11). Mitigated by the consistency test above; if a rung is ever
  added, both encodings must change together and the test fails otherwise.
- **`viper.WriteConfig` would leak env and flag values into the file.** Never call it;
  writes go through the file-only instance plus `atomicWrite`. Covered by a dedicated
  test rather than a comment.
- **Numeric precision on rewrite.** Viper decodes JSON into `map[string]any` via
  `encoding/json`, so numbers pass through `float64`. A hand-edited integer beyond
  float64's exact range (>2^53) does not round-trip precisely. Acceptable while no
  recognized key is numeric; revisit when one is added.
- **Dependency weight** — Viper pulls roughly 10–12 modules (afero, fsnotify, cast,
  mapstructure, go-toml, gotenv, locafero, conc, ini, yaml.v3, x/text) into a tree that
  currently has 13 indirect dependencies. Accepted deliberately in exchange for the
  resolution and env-derivation it provides, and for consistency with the wider
  cobra/viper idiom.
- **Viper lowercases keys and treats `.` as a nesting delimiter.** Harmless for the flat
  snake_case keys here; a future key containing a dot would be silently nested.
- **A new command could read the flag directly** and silently ignore the config file.
  Mitigated structurally: three of the six sites are shared helpers, and the guard test
  fails the build otherwise.
- **A stored `api_key` silences the no-key banner** — intended (FR-9), but an invalid
  stored key now surfaces as a 401 rather than up front. `renderAPIError`
  (`cmd/auth.go:89`) already explains 401s.
- **Secret at rest in plaintext**, mode `0600` only. Keychain support is out of scope.
- **Name overloading** with `pkg/settings` / `--settings`. Mitigated by naming the command
  `config` and the package `internal/cliconfig`, plus the README note.
- **`dependencies` hardcodes its URL default** instead of `config.DefaultAPIURL`. Same
  literal value, so resolution behaves correctly; tidying it is out of scope.
