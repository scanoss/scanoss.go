# Feature Specification: TLS certificate verification toggle

**Feature branch:** `feat/tls-verification`
**Status:** Draft
**SDD Change:** `tls-verification`

## Summary
Add an opt-in flag, `--ignore-cert-errors`, that disables TLS certificate
verification for SCANOSS API calls. It targets users pointing the CLI at a
custom, internal, or self-signed endpoint, who currently hit
`tls: failed to verify certificate: x509: certificate signed by unknown
authority` with no way to proceed. Default behavior is unchanged (verification
ON); the flag is a deliberate, per-invocation override. When enabled a warning
is printed to stderr.

Verification stays ON by default and must be explicitly disabled — "enable/
disable" is expressed as the presence/absence of the flag (boolean, default
false = verification enabled).

## User Scenarios & Testing

### Primary user story
As a developer scanning against a self-signed or internal SCANOSS endpoint, I
want to opt out of TLS certificate verification for that run so my scan and
decoration queries succeed, while the secure default is preserved for everyone
else.

### Acceptance scenarios
1. **Given** a self-signed API endpoint, **when** I run a decoration command
   (`vulnerabilities`/`licenses`/`geoprovenance`/`cryptography`) without the
   flag, **then** it fails with the x509 certificate error.
2. **Given** the same endpoint, **when** I add `--ignore-cert-errors`, **then**
   the request succeeds and a stderr warning notes verification is disabled.
3. **Given** a self-signed endpoint, **when** I run `scan ... --ignore-cert-errors`,
   **then** the scan upload succeeds and the warning is printed.
4. **Given** no flag, **when** any command runs against a properly-signed
   endpoint, **then** behavior is unchanged (verification ON, no warning).
5. **Given** the SDK, **when** a consumer constructs the client with
   `scanoss.New(scanoss.WithInsecureTLS(true))`, **then** TLS verification is
   disabled for that client.

### Edge cases
- Flag set against a valid endpoint → still works (verification simply skipped).
- Flag default (false) → identical to today's behavior, zero overhead.
- `dependencies` command is OUT OF SCOPE (separate HTTP path); documented gap.

## Requirements
- **FR-1** CLI flag `--ignore-cert-errors` (bool, default false) on the four
  decoration commands and the `scan` command.
- **FR-2** When enabled, the HTTP client used for those calls skips TLS
  certificate verification (`InsecureSkipVerify: true`).
- **FR-3** When enabled, print a one-line stderr warning.
- **FR-4** Default (flag absent) preserves current secure behavior exactly.
- **FR-5** SDK `pkg/scanoss` exposes the capability as a `WithInsecureTLS(bool)`
  option; `pkg/api` exposes it without breaking its constructor signatures.
- **NFR-1** No new third-party dependencies (stdlib `crypto/tls` only).
- **NFR-2** Security: the flag must read as insecure (name, help text, runtime
  warning); never the default.

## Out of scope
- `--ca-cert` custom CA bundle (possible future follow-up).
- The `dependencies` command's own HTTP client (`cmd/dependencies.go:515`).
