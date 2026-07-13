# Implementation Plan: Retry-After on the SDK transport

**Spec:** `./spec.md`
**Status:** Draft

## Approach
Two steps, in order:

**1. Extract the transport into an `httpTransport` *type* (composition).** Today
`do` is a method on `*Client` reading `c.apiKey`/`c.http` — the transport is
*fused into* the Client. Instead, `Client` should **have-a** transport (mirroring
how `http.Client` composes a `RoundTripper`). Introduce an `httpTransport` struct
that owns the HTTP mechanics (auth, execute, status, retries) and is held by
`Client` as `c.transport`. Services call `c.transport.do(...)`. This is a pure
restructure — no behavior change.

**2. Add `Retry-After` to `transport.do`.** Since every request path
(`postComponents`, `getResult`, `uploadChunk`, `Status`) funnels through the one
`do`, the retry logic lives there once and covers decoration, upload, and poll.

## Mechanics

### The transport type (composition)
```go
// transport.go — owns HTTP mechanics; knows nothing about services.
type httpTransport struct {
    httpClient *http.Client
    apiKey     string
    maxRetries int
}

func (t *httpTransport) do(ctx context.Context, req *http.Request) ([]byte, *http.Response, error) { ... }
```
```go
// client.go — owns config + service wiring; HAS-A transport.
type Client struct {
    apiURL     string
    transport  *httpTransport    // ← composition (replaces apiKey/http/maxRetries fields)
    chunkSize  int
    workers    int
    onProgress ProgressFunc
    onScanID   func(string)
    Scan       ScanAPI
    Licenses   LicenseAPI
    // …other services…
}
```
Call sites change `c.do(...)` → `c.transport.do(...)` (4 sites: `postComponents`,
`getResult`, `uploadChunk`, `Status`). HTTP-level options configure `c.transport`:
`WithAPIKey` → `transport.apiKey`, `WithHTTPClient`/`WithInsecureTLS` → `transport.httpClient`,
`WithMaxRetries` → `transport.maxRetries`. `New` constructs `c.transport` with defaults
(`&http.Client{}`, `maxRetries: 5`). Kept a concrete struct (no interface) until a
mock/swap need actually appears.

### Request body replay
On a retry the original body reader is consumed, so before re-sending we reset it:
```go
if req.GetBody != nil {
    body, err := req.GetBody()
    if err != nil { /* give up retrying */ }
    req.Body = body
}
```
`http.NewRequest` auto-populates `GetBody` for `*bytes.Reader`/`*bytes.Buffer`/
`*strings.Reader` bodies — which is what `postComponents` (`bytes.NewReader(body)`)
and `uploadChunk` (`bytes.NewReader(block)`) use; GET paths have no body. If a
body exists without `GetBody`, we do **not** retry.

### The loop (sketch)
```go
func (t *httpTransport) do(ctx context.Context, req *http.Request) ([]byte, *http.Response, error) {
    req = req.WithContext(ctx)
    if t.apiKey != "" { req.Header.Set("x-api-key", t.apiKey) }
    for attempt := 0; ; attempt++ {
        if attempt > 0 {
            if req.GetBody == nil { /* can't replay → return last */ }
            b, err := req.GetBody()
            if err != nil { return nil, nil, err }
            req.Body = b
        }
        resp, err := t.httpClient.Do(req)
        if err != nil { return nil, nil, fmt.Errorf("error making request: %w", err) }
        body, rerr := io.ReadAll(resp.Body)
        resp.Body.Close()
        if rerr != nil { return body, resp, fmt.Errorf("error reading response: %w", rerr) }

        if d, ok := retryAfter(resp); ok && attempt < t.maxRetries {
            if werr := sleepCtx(ctx, d); werr != nil { return body, resp, werr }
            continue
        }
        if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
            return body, resp, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
        }
        return body, resp, nil
    }
}
```

### Helpers (in `transport.go`)
- `retryAfter(resp) (time.Duration, bool)` — `ok` when `resp.StatusCode` is
  `http.StatusTooManyRequests` (429) or `http.StatusServiceUnavailable` (503) and
  `Retry-After` parses; returns the wait (integer seconds, or `http.ParseTime` →
  `time.Until(t)` ≥ 0), clamped to `maxRetryAfter`.
- `clampDelay(d)` — caps to `maxRetryAfter`.
- `sleepCtx(ctx, d) error` — `select { <-time.After(d); <-ctx.Done() → ctx.Err() }`.

### Config & constants
- `maxRetries` lives on the **transport** (`transport.maxRetries`), set by
  `WithMaxRetries(n) Option` (ignore `n <= 0`), **defaulting to 5** in `New`.
  `do` reads `t.maxRetries`. The server's `Retry-After` drives the *wait*; the
  client only caps the *count*.
- `maxRetryAfter` (the wait clamp) is also a transport field, set by
  `WithMaxRetryAfter(d) Option`, **defaulting to `DefaultMaxRetryAfter` (5m)**;
  `<= 0` means no cap.
```go
const DefaultMaxRetryAfter = 5 * time.Minute

func WithMaxRetries(n int) Option {
    return func(c *Client) { if n > 0 { c.transport.maxRetries = n } }
}
func WithMaxRetryAfter(d time.Duration) Option {
    return func(c *Client) { if d > 0 { c.transport.maxRetryAfter = d } }
}
// New: c.transport = &httpTransport{ httpClient: &http.Client{},
//        maxRetries: DefaultMaxRetries, maxRetryAfter: DefaultMaxRetryAfter }
```

## File-by-file
**New**
- `pkg/scanoss/transport.go` — the `httpTransport` struct + `(*httpTransport).do` (moved
  from `decorate.go`) **with** the retry loop, plus `retryAfter` / `clampDelay` /
  `sleepCtx` and the `maxRetryAfter` constant.

**Modified**
- `pkg/scanoss/client.go` — replace the `apiKey`/`http` fields with a
  `transport *httpTransport`; add `maxRetries` to `httpTransport`; `New` builds
  `c.transport` (default `maxRetries: 5`); HTTP-level options (`WithAPIKey`,
  `WithHTTPClient`, `WithInsecureTLS`, new `WithMaxRetries`) configure `c.transport`.
- `pkg/scanoss/decorate.go` — **remove** `do` (now in `transport.go`); keep the
  decoration engine. Call sites `c.do(...)` → `c.transport.do(...)` (`postComponents`,
  `getResult`).
- `pkg/scanoss/scan.go` / `scan_transport.go` — call sites `c.do(...)` →
  `c.transport.do(...)` (`Status`, `uploadChunk`).

**Tests**
- `pkg/scanoss/retry_test.go` (or extend `scan_test.go`): httptest server that
  returns `429 + Retry-After: 1` then `200`/`202`; assert one retry then success,
  for both a decoration call and a chunk upload. HTTP-date form. `maxRetries`
  exhaustion → error. ctx-cancel during wait → `ctx.Err()`. Body replay verified
  (server checks the second request carries the same body).

## Risks / notes
- **`do` now does I/O retries** — keep the loop tight and the cap small so a stuck
  server can't hang a call indefinitely (bounded by `maxRetries × maxRetryAfter`
  worst case; ctx still cancels).
- **Concurrent callers** (decoration worker pool, parallel chunk uploads) each
  retry independently — acceptable; a smarter global backoff is out of scope.
- **Decoration partial-failure semantics unchanged**: after `maxRetries`, the
  chunk still surfaces as a failed chunk in `Result.Failed`, exactly as today.

## Verification
- Unit tests above, `-race`.
- `go build ./... && go vet ./... && gofmt -l && go test ./...` clean.
- Manual: point at an endpoint that 429s (or a stub) and confirm the wait/retry.
