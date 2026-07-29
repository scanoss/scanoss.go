# Feature Specification: store proxy and ca-cert in the config file

**Feature branch:** `feat/proxy-support`
**Status:** Draft
**SDD Change:** `config-transport-keys`
**Ticket:** [#20](https://github.com/scanoss/scanoss.go/issues/20)

## Summary
Make `proxy` and `ca-cert` storable in `~/.scanoss/settings.json`, the way `api-url`
and `api-key` already are:

```console
$ scanoss-cli config set ca-cert /etc/ssl/corp-ca.pem
$ scanoss-cli scan .          # no flag needed
```

Today `--ca-cert` applies to one invocation. The certificate pool it builds is an
in-memory copy that dies with the process, so the flag has to be repeated on every
command. Both settings describe a machine or a network, which is what the config file
is for.

## User Scenarios & Testing

### Primary user story
As a developer behind a corporate proxy, I want to configure the proxy and our CA once
so that every command works without repeating two flags.

### Acceptance scenarios
1. **Given** `config set ca-cert <path>`, **when** I run any API command with no flag,
   **then** that CA is trusted.
2. **Given** `config set proxy <url>`, **when** I run any API command with no flag,
   **then** the request goes through that proxy.
3. **Given** a stored `proxy`, **when** `SCANOSS_PROXY` is set, **then** the environment
   wins; **when** `--proxy` is also passed, **then** the flag wins over both.
4. **Given** either key stored, **when** I run `config list`, **then** both appear in
   full with their source — neither is a secret.
5. **When** I run `config set proxy <no scheme>`, **then** it is rejected with the same
   message the flag gives.
6. **When** I run `config set ignore-cert-errors true`, **then** it fails as an
   unrecognized key.

### Edge cases
- A stored `ca-cert` pointing at a file that no longer exists fails the run with the
  same error the flag produces, naming the path.
- A stored `ca-cert` is ignored, with the existing warning, when
  `--ignore-cert-errors` is passed.
- `HTTP_PROXY`/`HTTPS_PROXY` keep working when no `proxy` is configured, but a stored one
  overrides them: they are not a rung of the ladder, only Go's fallback when the ladder
  yields nothing. `NO_PROXY` likewise applies only to that fallback — a configured proxy
  is used for every request. Both were already true of `--proxy`.

## Requirements
- **FR-1** `proxy` and `ca_cert` are recognized keys, set as `config set proxy <url>`
  and `config set ca-cert <path>` — dashes on the command line, `snake_case` in the file,
  as with the existing keys.
- **FR-2** Both resolve as **flag > environment > config file > built-in default**. The
  environment variables follow the existing derivation: `SCANOSS_PROXY` and
  `SCANOSS_CA_CERT`.
- **FR-3** Neither is secret: a proxy URL and a file path are not credentials, so
  `config list` and `config get` show them in full.
- **FR-4** `config set proxy` applies the same scheme check as `--proxy`, so a bad value
  is refused where it is typed rather than on the next command.
- **FR-5** The transport is built from the resolved values, not the raw flags.
- **FR-6** `ignore-cert-errors` is **not** storable. Persisting "never verify
  certificates" removes the deliberateness that makes it acceptable as a per-run flag.
- **NFR-1** No new dependencies, no change to the file format.

## Out of scope
- Any other flag (`--threads`, `--format`, …).
- Per-project config, or a `--config <path>` override.
