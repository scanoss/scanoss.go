# Feature Specification: persisted CLI settings (`config` command)

**Feature branch:** `feat/cli-config-command`
**Status:** Draft
**SDD Change:** `cli-config-command`
**Ticket:** [#17](https://github.com/scanoss/scanoss.go/issues/17) — the ticket carries the
scope, usage and acceptance criteria; design and rationale live here and in
[`plan.md`](./plan.md).

## Summary
Add a `config` command that persists CLI settings to `$HOME/.scanoss/settings.json`,
so a user sets their API credentials once instead of passing `--api-key` on every
invocation. Every command that already accepts `--api-url`/`--api-key` resolves its
effective value through one precedence chain:

```
--flag  >  environment variable  >  ~/.scanoss/settings.json  >  built-in default
```

Only `api_url` and `api_key` are supported as recognized keys in this change; the
file format, the key registry, and the resolution chain are built so a later key is
an additive change. Keys are `snake_case`; unrecognized keys already present in the
file are preserved untouched on write.

```json
{
  "api_key": "xxx",
  "api_url": "https://api.scanoss.com"
}
```

Environment variables: `SCANOSS_API_KEY`, `SCANOSS_API_URL`.

### Naming note
`pkg/settings` and the existing `--settings` flag already mean *the project's*
`scanoss.json`/`settings.json` (BOM + skip rules) — a different concept with a
colliding name. This feature therefore names the user-facing command `config` and
the Go package `internal/userconfig`, while the on-disk file stays
`~/.scanoss/settings.json` as specified. The distinction (project settings vs.
user config) is called out in the docs so the two are not confused.

## User Scenarios & Testing

### Primary user story
As a developer using the SCANOSS CLI daily, I want to store my API key and endpoint
once so that `scanoss-cli scan .` just works, while keeping the ability to override
either value per invocation with a flag or per shell with an environment variable.

### Acceptance scenarios
1. **Given** no config file, **when** I run `scanoss-cli config set api_key TOKEN`,
   **then** `~/.scanoss/settings.json` is created containing `{"api_key": "TOKEN"}`,
   the directory is mode `0700` and the file mode `0600`.
2. **Given** a stored `api_key`, **when** I run any API command with no `--api-key`
   and no env var, **then** the stored key is used and the no-key banner is not shown.
3. **Given** a stored `api_key`, **when** `SCANOSS_API_KEY` is set, **then** the env
   var wins; **when** `--api-key` is also passed, **then** the flag wins over both.
4. **Given** a stored `api_url`, **when** I run a command with no `--api-url`,
   **then** requests target the stored URL instead of the built-in default.
5. **Given** a config file carrying `api_key` plus a key this version does not
   recognize, **when** I run `config set api_url https://x`, **then** the resulting
   file has all three keys — the unrecognized key is preserved verbatim.
6. **Given** a stored config, **when** I run `config list`, **then** all keys are
   printed and `api_key` renders as a fixed run of asterisks (`********`) — never its
   value, never a prefix or suffix of it, and never a length that reveals the real
   one. There is no flag that reveals it.
7. **Given** a stored config, **when** I run `config get api_url`, **then** the bare
   value is printed to stdout (script-friendly, no decoration); **when** I run
   `config get api_key`, **then** it prints `********` — secret keys have no display
   path in this CLI. `config get api_key` therefore reports only whether the key is
   set, exiting non-zero when it is not.
8. **When** I run `config set some_unknown_key v`, **then** it fails with a non-zero
   exit and an error listing the recognized keys; the file is not modified.
9. **When** I run `config unset api_key`, **then** the key is removed from the file
   and the remaining keys are preserved.
10. **When** I run `config path`, **then** the absolute config file path is printed
    whether or not the file exists.
11. **Given** a malformed `settings.json`, **when** any command runs, **then** it
    fails with a clear error naming the file and the parse problem — it does not
    silently fall back to defaults.

### Observable behavior
These pin output shape, not just semantics. The README carries the tutorial set; these
are the cases a test should reproduce.

**`list` reports effective values with their source.** Fresh machine, nothing stored:

```console
$ scanoss-cli config list
api_key  (unset)
api_url  https://api.scanoss.com  (default)
```

After `config set api_key SC_abc123def456`, with `SCANOSS_API_KEY` also exported and a
stored `api_url`:

```console
$ scanoss-cli config list
api_key  ********                           (env: SCANOSS_API_KEY)
api_url  https://scanoss.internal.acme.com  (config file)

Config file: /Users/you/.scanoss/settings.json
```

The source annotation is load-bearing, not decoration: because `api_key` always renders
as `********`, the source is the *only* observable signal about it. Without it, "using
your stored key" and "using a different key from the environment" produce byte-identical
output — precisely the state a user needs to see when debugging a 401. The env source
names the variable so the reader knows what to unset.

**Precedence is demonstrable on `api_url`,** which is not secret:

```console
$ scanoss-cli config set api_url https://scanoss.internal.acme.com
$ scanoss-cli scan . --verbose
DEBUG resolved api_url source="config file" value=https://scanoss.internal.acme.com

$ SCANOSS_API_URL=https://staging.scanoss.com scanoss-cli scan . --verbose
DEBUG resolved api_url source=env value=https://staging.scanoss.com

$ SCANOSS_API_URL=https://staging.scanoss.com \
    scanoss-cli scan . --api-url https://api.scanoss.com --verbose
DEBUG resolved api_url source=flag value=https://api.scanoss.com
```

**A secret logs its source, never its value** — the same `IsSecret()` registry flag drives
both masking and logging:

```console
$ scanoss-cli scan . --verbose
DEBUG resolved api_key source="config file"
```

**`get` composes for non-secrets and answers only set/unset for secrets:**

```console
$ scanoss-cli config get api_url
https://scanoss.internal.acme.com

$ scanoss-cli config get api_key
********
```

**Failure output:**

```console
$ scanoss-cli config set api_token SC_abc123
Error: unrecognized key "api_token"; recognized keys are: api_key, api_url

$ scanoss-cli config get api_key      # nothing stored
Error: api_key is not set             # exit 1

$ scanoss-cli scan .                  # malformed settings.json
Error: parsing /Users/you/.scanoss/settings.json: invalid character '}' looking for beginning of object key string
```

### Edge cases
- No config file / empty file → behavior identical to today (defaults + flags).
- `~/.scanoss` missing → created by the write; reads never create anything, so a
  plain `scan`/`get`/`list` on a clean machine leaves no files behind.
- `~/.scanoss` exists as a *file*, or the directory is not writable → the `config`
  subcommand fails with an error naming the path; nothing is partially written.
- `~/.scanoss` exists with looser permissions than `0700` (e.g. pre-created by hand)
  → left as-is; only paths this command creates get the tightened mode.
- Empty string values in the file (`"api_key": ""`) are treated as unset.
- `config set api_url` trims whitespace and a trailing slash, matching the SDK's
  `WithAPIURL` normalization.
- Env var set to the empty string is treated as unset (falls through to the file).
- `$HOME` unresolvable → `config` subcommands error out; the resolution chain
  degrades to flags + env instead of failing the command.
- Non-`config` commands never *write* the file; only `config set`/`unset` do.

## Requirements
- **FR-1** New command `scanoss-cli config` with subcommands `set <key> <value>`,
  `get <key>`, `list`, `unset <key>`, `path`.
- **FR-2** Storage location `$HOME/.scanoss/settings.json`, JSON object, `snake_case`
  keys, written pretty-printed with keys in sorted order.
- **FR-3** Recognized keys: `api_url`, `api_key`. `set`/`unset`/`get` reject any
  other key with a message listing the recognized ones.
- **FR-4** Writes preserve unrecognized keys. Values round-trip through
  `map[string]any`, so JSON numbers pass through `float64`: an integer beyond float64's
  exact range (>2^53) is not preserved bit-for-bit. Acceptable while no recognized key is
  numeric.
- **FR-4a** A write never persists a value that came from the environment or a flag. With
  `SCANOSS_API_KEY` exported, `config set api_url <url>` must leave `api_key` absent from
  the file.
- **FR-5** A write creates whatever is missing: the `~/.scanoss` directory (including
  it not existing at all) and the `settings.json` file itself. No setup step, no
  `config init` — the first `config set` on a clean machine succeeds.
- **FR-6** Writes are atomic (temp file + rename in the same directory), file mode
  `0600`, directory mode `0700`.
- **FR-7** Effective-value resolution order for every command that exposes
  `--api-url`/`--api-key`: explicitly-set flag > env var (`SCANOSS_API_URL`,
  `SCANOSS_API_KEY`) > config file > existing built-in default.
- **FR-8** Resolution applies to all current API commands: `scan` (+ `scan wfp`),
  `results`, `enrich`, `attributions`, `dependencies`, `components`, and the PURL
  service commands (`vulnerabilities`, `licenses`, `cryptography`, `geoprovenance`,
  `copyright`).
- **FR-9** The no-key guard (`requireKeyForDefaultEndpoint`) sees resolved values, so
  a stored key satisfies it.
- **FR-10** A secret key is never displayed. `api_key` renders as a fixed `********` in
  every output path (`list`, `get`), with no reveal flag and no partial reveal; the
  asterisk count is constant so it does not leak the real length. Keys are marked
  secret in the registry, so a future secret key inherits this.
- **FR-11** `list` reports the **effective** value of every recognized key with the
  source that won (`(default)`, `(config file)`, `(env: SCANOSS_API_KEY)`), an `(unset)`
  marker where nothing resolves, and a trailing `Config file: <path>` line. Recognized
  keys are always listed, so `list` is never blank on a fresh machine; unrecognized keys
  present in the file are listed as stored values.
- **FR-12** A malformed or unreadable config file is a hard error, not a silent
  fallback.
- **NFR-1** Config reading, environment-variable naming, and value resolution use
  `github.com/spf13/viper`. Writing does **not**: `viper.WriteConfig` serializes Viper's
  merged view and would persist env- and flag-derived values into the file, so writes go
  through a file-only Viper instance plus our own atomic writer (FR-6).
- **NFR-2** The SDK (`pkg/scanoss`, `pkg/api`) keeps reading nothing from the
  environment or disk; env/file resolution is CLI-only.
- **NFR-3** Secret hygiene: the API key is never printed by any output path, never
  logged (including `--verbose`, which logs the winning *source* only), and never
  written with world-readable permissions.

## Out of scope
- **Any key beyond `api_url`/`api_key`.** `config set` rejects anything else. Adding a
  recognized key later is one entry in the key registry — no format change, no
  migration, and a hand-edited unknown key is preserved meanwhile — but wiring a new key
  into command *behavior* is separate work, not covered here.
- OS keychain / credential-helper storage; encryption at rest.
- Per-project or per-directory config, `$XDG_CONFIG_HOME`, or a `--config <path>`
  override flag.
- A `config edit` subcommand or interactive login flow.
- Persisting other flags (`--threads`, `--format`, `--ignore-cert-errors`, …).
- Migrating `pkg/settings` or renaming the project `--settings` flag.
