# Feature Specification: Drop osskb.org — default to api.scanoss.com + clear auth errors

**Feature branch:** `feat/drop-osskb`
**Issue:** [#37](https://github.com/scanoss/scanoss.go/issues/37)
**Status:** Draft

## Summary
Remove every trace of `osskb.org` from the CLI/SDK and make **`https://api.scanoss.com`**
the one default endpoint. When a user runs against that default endpoint **without**
an API key, the CLI must fail fast with a clear, branded "subscription required"
message (ASCII banner, npm-style) pointing to https://www.scanoss.com — instead of
the obscure network/`Unauthorized` failure the Python CLI produces (`cannot reach
osskb.org`). When a request to **any** endpoint comes back unauthorized, the gateway
error must be rendered clearly on the console, not as a generic `API returned status
401`.

On-prem and custom deployments must keep working **without** a key: the no-key guard
fires **only** against the default SCANOSS endpoint. A custom `--api-url` (e.g.
`https://elgato.scanoss.com`) proceeds straight through, key or no key.

> Whoever wants the free OSS KB (`osskb.org`) uses the legacy scanners. This Go
> client targets the SCANOSS API by default.

### Current state (why this is needed)
- `internal/config/config.go:5` — CLI `DefaultAPIURL = "https://api.osskb.org"`, used
  by `scan`, `results`, `attributions` flag defaults. The SDK
  (`pkg/scanoss/client.go:37`) already defaults to `https://api.scanoss.com`, and the
  purl-service commands already use the SDK default — so the CLI is **inconsistent**
  today (some commands hit osskb, some hit scanoss.com).
- `/v3` is part of each service **endpoint path** (`/v3/components/search`, etc.), not
  the base URL — so the target base URL is simply `https://api.scanoss.com`.
- `cmd/scan.go:292-296` rewrites the URL to `config.PremiumAPIURL` ("using premium
  endpoint") when a key is supplied without `--api-url`. Once the default *is*
  scanoss.com, this substitution is dead code.
- The SDK transport (`pkg/scanoss/transport.go:78-79`) collapses every non-2xx into
  `fmt.Errorf("API returned status %d: %s", …)`. There is no typed/sentinel error, so
  the CLI cannot single out 401 for a clear message.

## User Scenarios & Testing

### Primary user story
As a user running `scanoss scan ./proj` with no flags, I either get results (if a
key is configured) or an immediate, unmistakable message telling me a subscription is
required and where to get one — never a confusing "cannot reach …" or bare 401.

### Acceptance scenarios
1. **Given** no `--api-url` and no `--api-key`, **when** I run `scan`/`results`/a
   service command, **then** the CLI prints the SCANOSS ASCII banner + "subscription
   required" message and exits non-zero **without** making a network request.
2. **Given** `--api-key <valid>` and no `--api-url`, **then** the command runs against
   `https://api.scanoss.com` and succeeds (no "using premium endpoint" line).
3. **Given** `--api-url https://elgato.scanoss.com` and **no** `--api-key`, **then**
   the command proceeds to that endpoint normally (no no-key guard) — on-prem works
   keyless.
4. **Given** any endpoint returns **401 Unauthorized**, **then** the CLI prints a clear
   `Unauthorized` message (key missing/invalid → check your API key), not a generic
   `API returned status 401`.
5. **Given** `grep -rni osskb .` over the repo, **then** there are no references in code
   or current docs (historical CHANGELOG entries excepted — see Open decisions).

### Edge cases
- `--api-url https://api.scanoss.com` passed **explicitly** with no key → treated the
  same as the default (it *is* the default endpoint): no-key guard fires. The guard
  keys off the **effective URL equaling the default**, not off whether the flag was
  changed.
- Trailing slash on `--api-url` (`https://api.scanoss.com/`) → normalized before the
  default-endpoint comparison so the guard still recognizes it.
- A 401 from a **custom** on-prem endpoint (bad key) → still rendered with the clear
  `Unauthorized` message (scenario 4 is endpoint-agnostic).
- Non-401 errors (404/500/network) → unchanged from today.

## Requirements

### Functional
- **FR-001** CLI `DefaultAPIURL` becomes `https://api.scanoss.com` (single source of
  truth; equal to `scanoss.DefaultAPIURL`). No `osskb` string remains in code.
- **FR-002** Remove `config.PremiumAPIURL` and the premium-endpoint substitution in
  `cmd/scan.go` (default is now the premium endpoint; the substitution is a no-op).
- **FR-003** A shared CLI pre-flight guard: when the **effective API URL equals the
  default SCANOSS endpoint** **and** no `--api-key` is set, print the SCANOSS ASCII
  banner + subscription message to stderr and return a non-zero exit **before** any
  request. Applies to `scan`, `results`, `attributions`, and the purl-service
  commands.
- **FR-004** A custom `--api-url` (anything ≠ the default) **bypasses** FR-003 entirely
  — runs with or without a key (on-prem keyless support).
- **FR-005** The SDK transport surfaces 401 distinctly: a typed/sentinel error
  (`ErrUnauthorized` or a `StatusError{Code,Body}` exposing `StatusCode`) returned from
  `transport.do` on `http.StatusUnauthorized`, so callers can detect it. Other statuses
  keep the existing `API returned status %d` error.
- **FR-006** The CLI detects the 401 error (`errors.Is`/`errors.As`) and prints a clear
  `Unauthorized` message (suggest checking/setting `--api-key`). Covers both the scan
  path and the single-shot service paths.
- **FR-007** The ASCII banner mirrors the npm/Python-CLI style and points to
  `https://www.scanoss.com`. (Reference layout below.)
- **FR-008** Scrub `osskb` from current docs (`README.md`, `libscanoss/` docs) and
  update default-URL mentions to `https://api.scanoss.com`.

### Non-functional
- **NFR-001** No behavior change for authorized requests or non-401 errors.
- **NFR-002** Guard logic is shared (one helper), not duplicated per command.
- **NFR-003** Build, vet, gofmt-clean, tests pass (incl. `-race`).

### Reference banner (FR-007)
```
   ____   ____    _    _   _  ___  ____ ____
  / ___| / ___|  / \  | \ | |/ _ \/ ___/ ___|
  \___ \| |     / _ \ |  \| | | | \___ \___ \
   ___) | |___ / ___ \| |\  | |_| |___) |__) |
  |____/ \____/_/   \_\_| \_|\___/|____/____/

  [!] No API key provided (--api-key)

  A subscription is required to use the SCANOSS API.
  Get yours at: https://www.scanoss.com
```

## Open decisions
1. **Historical CHANGELOG entries.** `CHANGELOG.md:199` records a past release whose
   API URL was `osskb.org`. **Recommend leaving historical entries as-is** (a changelog
   records what shipped); scrub only forward-looking docs. _Confirm._
2. **Typed error shape.** `StatusError{StatusCode int; Body string}` (general, reusable
   for future status-specific handling) vs. a narrow `var ErrUnauthorized = errors.New(…)`
   sentinel. **Recommend `StatusError`** + an `errors.As` check for 401 — costs the same
   and generalizes. _Confirm._
3. **Guard exit vs. attempt.** Pre-flight fail (no request, per scanoss-py) vs. let the
   request 401 and render the clear error. **Recommend pre-flight** for the
   default-endpoint-no-key case (fast, unambiguous), while FR-005/006 still handle a
   real 401 from a keyed/custom request. _Confirm._

## Out of scope
- Reading a key from env/config file (`SCANOSS_API_KEY`) — separate enhancement.
- Reworking decoration per-chunk partial-failure semantics (a 401 still marks chunks
  failed in `Result.Failed`; the clear message is rendered at the CLI for the scan and
  single-shot paths).
- The legacy `pkg/api` client (being retired).
