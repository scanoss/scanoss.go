# Feature Specification: Retry-After on the SDK transport

**Feature branch:** `feat/scan-batch-v3`
**Issue:** [#33](https://github.com/scanoss/scanoss.go/issues/33)
**Status:** Draft

## Summary
Honor the HTTP **`Retry-After`** response header in the SDK's single transport
choke point (`pkg/scanoss` `Client.do`). When the server replies with
**429 Too Many Requests** or **503 Service Unavailable** carrying a `Retry-After`
header, the client waits the indicated time and retries the request, instead of
failing immediately. Because every request path funnels through `do`, this covers
**all** SDK traffic uniformly:

- decoration services (`postComponents`, `getResult` → vulnerabilities, licenses,
  cryptography, geoprovenance, copyright, components),
- the v3 scan chunk upload (`uploadChunk`),
- the scan status poll (`Status`).

Reference: [MDN `Retry-After`](https://developer.mozilla.org/en-US/docs/Web/HTTP/Reference/Headers/Retry-After).

**Responsibility split (important):** the **server** decides *how long* to wait
(the `Retry-After` value); the SDK only **obeys** it — it does not compute its own
backoff. The **SDK/caller** decides *how many times* to retry (`WithMaxRetries`,
default 5) and applies a safety ceiling (`maxRetryAfter`) so a pathological server
value can't stall a call indefinitely. No `Retry-After` header → no retry.

## User Scenarios & Testing

### Primary user story
As an SDK/CLI user hitting a rate-limited or temporarily-unavailable SCANOSS API,
my request transparently waits the server-requested time and retries, rather than
erroring out — without me writing any retry logic.

### Acceptance scenarios
1. **Given** a request that returns `429` with `Retry-After: 1`, **when** I call
   any SDK method, **then** the client waits ~1s and retries; on the subsequent
   `200`/`202` it returns success.
2. **Given** a `503` with `Retry-After` as an HTTP-date in the near future,
   **then** the client waits until that time (clamped to ≥ 0) and retries.
3. **Given** a chunk upload that gets `429 + Retry-After`, **then** the chunk is
   re-sent (its body replayed) after the wait; the scan still completes.
4. **Given** repeated `429`s, **then** the client retries up to `maxRetries` and,
   if still failing, returns the last error (no infinite loop).
5. **Given** `ctx` is cancelled while waiting on `Retry-After`, **then** the call
   returns `ctx.Err()` promptly (the wait is interruptible).

### Edge cases
- `Retry-After` absent on a 429/503 → no special handling; normal error path.
- Unparseable `Retry-After` value → treated as absent (normal error).
- A request with a body but no replay capability (`GetBody == nil`) → **not**
  retried (can't safely resend); returns the error. (Our POST callers use
  `bytes.NewReader`, so `http.NewRequest` sets `GetBody` automatically; GETs have
  no body.)
- An absurd `Retry-After` (e.g. hours) → clamped to `maxRetryAfter`.

## Requirements

### Functional
- **FR-001** In `Client.do`, when the response status is
  `http.StatusTooManyRequests` (429) or `http.StatusServiceUnavailable` (503) and
  a parseable `Retry-After` header is present, wait the indicated duration and
  retry the request.
- **FR-002** Parse `Retry-After` in both forms: **delta-seconds** (integer) and
  **HTTP-date** (`http.ParseTime`; `time.Until(t)` clamped to ≥ 0).
- **FR-003** The wait is **context-cancellable** (`select` on `time.After` vs
  `ctx.Done()`), returning `ctx.Err()` if cancelled.
- **FR-004** Retries are bounded by a **configurable** maximum via the
  `WithMaxRetries(n)` client option, **defaulting to 5** when unset (and when
  `n <= 0`). After the cap, return the last response/error unchanged. The wait
  *duration* per retry is the server's `Retry-After` value (the SDK does not
  compute backoff) — only the *count* and the `maxRetryAfter` clamp are SDK-side.
- **FR-005** The retried request **replays its body** via `req.GetBody()`. If a
  request has a body but no `GetBody`, it is not retried.
- **FR-006** Each `Retry-After` wait is clamped to a `maxRetryAfter` ceiling
  (configurable via `WithMaxRetryAfter`, default `DefaultMaxRetryAfter` = 5m;
  `<= 0` means no cap) to bound a pathological server value.
- **FR-007** Applies to **every** caller of `do` (decoration, upload, poll) —
  centralized, no per-caller code.
- **FR-008** `WithMaxRetries(n int) Option` sets the retry cap; default 5. It is
  stored on the client's composed **transport** (`c.transport.maxRetries`, read by
  `transport.do`) — one client-level knob governing all services. The wait clamp
  is likewise configurable: `WithMaxRetryAfter(d) Option` → `c.transport.maxRetryAfter`
  (default `DefaultMaxRetryAfter`).
- **FR-009** The HTTP transport is a composed type (`Client` *has-a* `transport`),
  not a method on `Client` — auth, status, and retries live on `transport.do`;
  `Client` keeps config + service wiring. (Mirrors `http.Client`→`RoundTripper`.)

### Non-functional
- **NFR-001** Parallel-safe: each in-flight request (e.g. concurrent chunk
  uploads, concurrent decoration chunks) waits/retries independently; `do` holds
  no shared retry state.
- **NFR-002** No behavior change for non-429/503 responses or responses without
  `Retry-After`.
- **NFR-003** Build, vet, gofmt-clean, tests pass (incl. `-race`).

## Open decisions
1. **Configurability.** _Resolved._ Both knobs are **client options**:
   `WithMaxRetries(n)` (**default 5**) and `WithMaxRetryAfter(d)` (the wait clamp,
   **default 5m**). **Per-use-case differences** (e.g. scan retries more
   than decoration) are achieved by using **separate `Client` instances** — the
   `Client` is the unit of config. Only *different limits within a single shared
   client* would need per-call options (a larger change, deferred).
2. **Trigger set.** 429/503 only (recommended) vs. any non-2xx carrying
   `Retry-After`. **Recommend 429/503** (the conventional rate-limit/unavailable
   pair).
3. **Generic backoff** (retrying transient errors *without* `Retry-After`) is
   **out of scope** — header-driven only.

## Out of scope
- Generic retry/backoff for network errors or other 5xx without `Retry-After`.
- Per-request retry budgets or jitter.
- The legacy `pkg/api` client (being retired; not worth changing).
