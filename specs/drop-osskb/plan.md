# Implementation Plan: Drop osskb.org — default to api.scanoss.com + clear auth errors

**Spec:** `./spec.md`
**Status:** Draft

## Approach
Three independent slices, smallest-blast-radius first:

**1. Collapse the default endpoint to `api.scanoss.com` (CLI).** Point
`internal/config.DefaultAPIURL` at `https://api.scanoss.com`, delete `PremiumAPIURL`
and the premium-substitution block in `scan.go`. After this, every CLI command shares
one default that already equals `scanoss.DefaultAPIURL`. Pure config/cleanup — no new
behavior, just removes the osskb/scanoss.com split.

**2. No-key pre-flight guard (CLI).** One shared helper that, when the effective API
URL equals the default endpoint and no key is set, prints the banner and returns an
error before any request. Wired into each command's `RunE`. Custom URL → helper is a
no-op (on-prem keyless).

**3. Clear 401 surfacing (SDK + CLI).** Give `transport.do` a typed error on 401 so the
CLI can render a clear `Unauthorized` message. Since every request funnels through
`transport.do`, the SDK change is one place; the CLI rendering is at the command error
boundary.

Slices 1 and 2 deliver Julian's headline ask (no osskb, friendly no-key message);
slice 3 covers the "gateway errors visible in console" ask for the keyed/on-prem 401.

## Mechanics

### Slice 1 — default endpoint
```go
// internal/config/config.go
const (
    // DefaultAPIURL is the SCANOSS API endpoint. The CLI targets it unless
    // --api-url overrides it. /v3 is carried by the SDK service paths.
    DefaultAPIURL = "https://api.scanoss.com"
)
// remove PremiumAPIURL
```
- `cmd/scan.go` — delete the `if apiKey != "" && !Changed("api-url") { apiURL =
  config.PremiumAPIURL; print("using premium endpoint") }` block (lines ~292-296). The
  default is already the premium endpoint.
- `cmd/scan.go:80` `buildResultsCommand` — the `apiURL != config.DefaultAPIURL`
  comparison still works (now compares against scanoss.com); no change needed beyond
  the constant.
- `cmd/results.go:36`, `cmd/attributions.go:62` flag defaults already reference
  `config.DefaultAPIURL` — they inherit the new value automatically.

### Slice 2 — no-key guard
New shared helper in `cmd/` (e.g. `cmd/auth.go`):
```go
// requireKeyForDefaultEndpoint fails fast (no request) when the user targets the
// default SCANOSS endpoint with no API key — printing the banner. A custom endpoint
// (on-prem) is exempt: it may legitimately run keyless.
func requireKeyForDefaultEndpoint(apiURL, apiKey string) error {
    if apiKey != "" {
        return nil
    }
    if normalizeURL(apiURL) != scanoss.DefaultAPIURL { // custom endpoint → allow keyless
        return nil
    }
    fmt.Fprint(os.Stderr, scanossNoKeyBanner)
    return errNoAPIKey // sentinel; root prints nothing extra / sets non-zero exit
}

func normalizeURL(u string) string { return strings.TrimRight(strings.TrimSpace(u), "/") }
```
- `scanossNoKeyBanner` is the FR-007 const string.
- The comparison is against `scanoss.DefaultAPIURL` (the single default) after
  normalization, so an explicit `--api-url https://api.scanoss.com[/]` is also caught.
- Call at the top of each `RunE`: `scan` (both folder-scan and `--wfp` paths, i.e. in
  `uploadAndWrite` and the folder runner once the flags are read), `results`,
  `attributions`, and `runPurlServiceTyped`. A single call site in
  `clientOptions`/`newClient` is tempting but those return options (can't error cleanly)
  — keep it an explicit pre-flight in each `RunE` (small, obvious, testable).
- Suppress cobra's usage dump for this error (it's not a usage problem): either
  `SilenceUsage`/`SilenceErrors` on the affected commands and print the banner
  ourselves, or return a wrapped error the root already silences. Match whatever the
  repo already does for runtime errors.

### Slice 3 — typed 401 + clear render
```go
// pkg/scanoss/transport.go
type StatusError struct {
    StatusCode int
    Body       string
}
func (e *StatusError) Error() string {
    return fmt.Sprintf("API returned status %d: %s", e.StatusCode, e.Body)
}
```
In `transport.do`, replace the bare `fmt.Errorf("API returned status …")` with
`&StatusError{resp.StatusCode, string(respBody)}`. The `.Error()` text is identical, so
nothing that only prints the error changes; callers that *care* can now
`errors.As(err, &se)` and check `se.StatusCode == http.StatusUnauthorized`.

CLI rendering — a small helper used where command errors surface (root `Execute`
wrapper, or each `RunE`'s return path):
```go
func renderAPIError(err error) error {
    var se *scanoss.StatusError
    if errors.As(err, &se) && se.StatusCode == http.StatusUnauthorized {
        fmt.Fprintln(os.Stderr, "Unauthorized: missing or invalid API key. Pass --api-key, or check your subscription at https://www.scanoss.com")
        return err // keep non-zero exit
    }
    return err
}
```
Decoration note: per-chunk 401s are wrapped in `ChunkError` (`decorate.go:13`) and land
in `Result.Failed`; the clear render targets the **scan** and **single-shot service**
paths (the ones Julian's report is about). Surfacing 401 through decoration's
partial-failure path is out of scope (spec).

## File-by-file
**New**
- `cmd/auth.go` — `scanossNoKeyBanner` const, `requireKeyForDefaultEndpoint`,
  `normalizeURL`, `errNoAPIKey`, and (optionally) `renderAPIError`.

**Modified**
- `internal/config/config.go` — `DefaultAPIURL` → scanoss.com; remove `PremiumAPIURL`.
- `cmd/scan.go` — drop premium-substitution block; call the guard in the scan paths;
  route the final scan error through `renderAPIError`.
- `cmd/results.go`, `cmd/attributions.go` — call the guard; route error through
  `renderAPIError`.
- `cmd/purlcommon.go` — call the guard in `runPurlServiceTyped` (and the components
  search/versions runners); route error through `renderAPIError`.
- `pkg/scanoss/transport.go` — add `StatusError`; return it from `do` on non-2xx.
- `README.md`, `libscanoss/` docs — scrub osskb, update default-URL text.

**Tests**
- `cmd/auth_test.go` — table test for `requireKeyForDefaultEndpoint`: default+no-key →
  error+banner; default+key → nil; custom URL+no-key → nil; explicit default URL with
  trailing slash + no-key → error.
- `pkg/scanoss/` — extend transport/scan tests: a 401 response yields a
  `*StatusError` with `StatusCode == 401` (assert via `errors.As`).
- `cmd/subcommands_test.go` — a command invoked with default endpoint and no key exits
  non-zero and prints the banner (no network).

## Risks / notes
- **Guard must not fire for on-prem.** The exemption hinges on "URL ≠ default". Confirm
  `normalizeURL` matches the SDK's own `WithAPIURL` trimming (`strings.TrimRight(url,
  "/")`) so the comparison is consistent.
- **Explicit-default-URL** is intentionally treated as "default endpoint" (spec edge
  case) — keying off the value, not `Changed("api-url")`, makes that fall out naturally.
- **`StatusError` is a public SDK type** — additive, no breaking change; existing
  callers that compare error *strings* still match (`.Error()` unchanged).
- **CHANGELOG history** left intact (Open decision 1).

## Verification
- Unit tests above, `-race`.
- `go build ./... && go vet ./... && gofmt -l . && go test ./...` clean.
- `grep -rni osskb .` returns only (if any) approved historical CHANGELOG lines.
- Manual: `scanoss scan ./x` (no flags) → banner, non-zero, no request;
  `scanoss scan ./x --api-url https://scanoss.internal.example.com` → proceeds keyless;
  `--api-key bad` against default → clear `Unauthorized`.
