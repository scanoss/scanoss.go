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
`fileViper` opens the settings file and, for resolution, the instance also gets
`SetEnvPrefix("SCANOSS")` + `AutomaticEnv` so the environment variable names are derived
rather than spelled out twice. One Viper detail to know: with `SetConfigFile` a missing
file surfaces as `fs.ErrNotExist`, not `viper.ConfigFileNotFoundError` — that one is only
produced by path search — so the "not configured yet" case checks for it explicitly.

Viper resolves the value; `walk` repeats the ladder to name the source. Two rungs need
care, both correctness rather than style:

- **The config rung needs the value, not just `InConfig`.** Viper treats a
  present-but-empty config value as found and returns `""` ahead of the flag default,
  which contradicts the "empty means unset" rule. (The env rung needs no equivalent
  guard: `AutomaticEnv` already treats an empty variable as unset unless `AllowEmptyEnv`
  is on, which it is not.)
- **The default rung reads the registry, not the flag.** `config list` declares no API
  flags and must still explain where `api-url` came from, so the built-in default belongs
  to the setting rather than to any command's flag — which also keeps
  `config.DefaultAPIURL` the single source for it.

An unexported `resolver` holds the viper instance and the flag set, so the file is read
once per call and the ladder above has somewhere to live. It loops the registry to
`BindPFlag` each key it finds a flag for.

Callers never see it. The package exposes three functions instead, one per question a
caller actually asks:

| function | answers | used by |
| --- | --- | --- |
| `ResolveAPI(flags)` | the endpoint and key this command should use | the six read sites |
| `Resolve(flags, key)` | one setting's value and source | `config get` |
| `ResolveAll(flags)` | every setting with its source, sorted | `config list` |

Keeping `resolver` private is deliberate: the resolution machinery is not something a
caller should be able to depend on, and Go expresses that through what is exported — not
through which file the code sits in.

`BindPFlag` needs a non-nil flag, so a command that declares only one of the two (or
neither) skips that binding — which is what keeps `wfp` and `sbom` unaffected, and what
lets `config list` resolve with no API flags at all.

The debug line records the source per key, and the value only for non-secret keys, so an
API key never reaches a log.

### Key naming: dashes outside, snake_case inside
Settings are named `api-url`/`api-key` on the command line — the same strings as the
flags — and stored `api_url`/`api_key` in the file. Exactly one spelling is accepted per
setting; the file's own form is not a second way to type a key, so there is one thing to
document and one error to explain.

The mapping is the registry's `cli` field rather than a `-`↔`_` transform. Stated per key,
it lets a future setting whose command-line name is not a mechanical rewrite of its stored
name exist, and it stops an unrecognized key from being silently rewritten into something
that looks familiar.

The boundary is one function. `cliconfig.StoredKey` translates a command argument to the
stored key; the Go API (`Set`, `Unset`, `Resolve`) speaks stored keys and its own
`KeyAPIURL`/`KeyAPIKey` constants. Everything the CLI *prints* — confirmations, `list`,
`get`, errors, and the `--verbose` log — goes through `CLIKey`, so output can be pasted
back into a command.

**Deliberately narrow.** `ResolveAPI` covers `api_url` and `api_key` only. `chunk-size`,
`workers`, and `ignore-cert-errors` — also read by `clientOptions` — stay flag-only and
keep reading straight from cobra.

### The write path
`viper.WriteConfig` serializes Viper's **merged** view, so a write from the resolving
instance would persist env- and flag-derived values into the file: `config set api-url X`
with `SCANOSS_API_KEY` exported would write that CI token to disk. It also writes `0644`
and truncates in place instead of renaming.

So writes never touch it. `Set` and `Unset` both go through one `mutate` helper that:

1. reads the current file with a **file-only** Viper instance — no `AutomaticEnv`, no
   `BindPFlag`, so the merged view cannot leak in;
2. applies the change to the plain `map[string]any` that `AllSettings()` returns, which
   carries every key found and so preserves unrecognized ones (FR-4) for free;
3. serializes with `json.MarshalIndent` (a map marshals with sorted keys, so the file
   stays diff-friendly) and writes through `atomicWrite`: create the directory `0700` if
   missing, write a sibling temp file `0600`, then rename.

`TestSetDoesNotPersistEnvironmentValues` is the regression test for step 1.

## Key changes
- `internal/cliconfig/` (new) — one package, one file per concern, so a file name says
  what is in it:
  - **`registry.go` — what settings exist.** `KeyAPIURL = "api_url"`,
    `KeyAPIKey = "api_key"`, and per key a `cli` name, a built-in `def`, and a `secret`
    flag. Exported: `StoredKey(input)` and `CLIKey(key)` for the two directions,
    `IsRecognized(key)`, `IsSecret(key)`, and `EnvName(key)` →
    `"SCANOSS_" + strings.ToUpper(key)`, matching Viper's `SetEnvPrefix` derivation so the
    resolver and `AutomaticEnv` cannot disagree. `recognizedKeys`, `cliKeys` and
    `defaultOf` stay private — nothing outside the package needs them.
  - **`store.go` — the file.** `Path()` → `$HOME/.scanoss/settings.json` via
    `os.UserHomeDir()`, behind a package var so tests can inject a temp home. `Set`/`Unset`
    as above (write instance + `atomicWrite`), and `Load`/`Get`/`Keys` for the unrecognized
    keys `config list` reports.
  - **`resolve.go` — which source wins.** `API{URL, Key}`, `Source`,
    `Setting{Key, Value, Source}`, the private `resolver`, and the three exported entry
    points `ResolveAPI` / `Resolve` / `ResolveAll`.
  - **`doc.go`** — the package comment, matching `pkg/settings/doc.go`.
  - **Typed error.** `UnknownKeyError{Key}` rendering
    `unrecognized key "x"; recognized keys are: api-key, api-url`, so `set`/`get`/`unset`
    share one message and `cmd` only propagates.
- `cmd/config.go` (new): `configCmd` + `set`/`get`/`list`/`unset`/`path`, built on the
  module's registry and storage. `cobra.ExactArgs` per subcommand, secret keys rendered
  as a constant `********` by a single `display(key, value)` helper used by both `list`
  and `get` (no reveal flag exists), and `set api-url` normalized through `normalizeURL`
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
- `README.md`: a "Configuration" section after the command table — precedence chain, both
  env vars, `config` examples, the never-displayed-key rule, and the user-config vs.
  project-`scanoss.json` clarification. Lands with T006, never ahead of it.
- `CHANGELOG.md`: `[Unreleased] → Added`.

## Testing strategy
- Tests mirror the source split: `registry_test.go` covers the vocabulary and needs no
  filesystem, `store_test.go` carries the shared helpers plus every file-level case, and
  `resolve_test.go` covers the ladder.
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
- **Viper lowercases keys and treats `.` as a nesting delimiter.** Confirmed in
  implementation: a hand-edited `"MyCustom_Key"` is rewritten as `"mycustom_key"`, so key
  preservation is case-insensitive rather than verbatim. Harmless for the flat snake_case
  convention and pinned by a test; a future key containing a dot would be silently nested.
- **A new command could read the flag directly** and silently ignore the config file.
  Mitigated structurally: three of the six sites are shared helpers, and the guard test
  fails the build otherwise.
- **A stored `api_key` silences the no-key banner** — intended (FR-9), but an invalid
  stored key now surfaces as a 401 rather than up front. `renderAPIError`
  (`cmd/auth.go:89`) already explains 401s.
- **Secret at rest in plaintext**, mode `0600` only. Keychain support is out of scope.
- **Name overloading** with `pkg/settings` / `--settings`. Mitigated by naming the command
  `config` and the package `internal/cliconfig`, plus the README note.
- ~~**`dependencies` hardcodes its URL default** instead of `config.DefaultAPIURL`.~~
  Fixed alongside T005: it was the last hardcoded copy of the endpoint outside the two
  constants, so the default could have drifted from every other command's.
