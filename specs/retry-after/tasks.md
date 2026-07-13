# Tasks: Retry-After on the SDK transport

**Spec:** `./spec.md` · **Plan:** `./plan.md`

## T1 — Extract the transport type  (refactor, no behavior change)
- [ ] `pkg/scanoss/transport.go`: `type httpTransport struct { httpClient *http.Client;
  apiKey string; maxRetries int }`; move `do` here as `(*httpTransport).do`.
- [ ] `pkg/scanoss/client.go`: replace `apiKey`/`http` fields with
  `transport *httpTransport`; `New` builds `c.transport` (default `maxRetries: 5`);
  wire `WithAPIKey`, `WithHTTPClient`, `WithInsecureTLS` to `c.transport`.
- [ ] Update call sites `c.do(...)` → `c.transport.do(...)` (`postComponents`,
  `getResult`, `uploadChunk`, `Status`).
- *Done:* build/vet/test green; **behavior identical** (no retry yet); `decorate.go`
  keeps only the decoration engine.
- → commit `refactor(scanoss): extract transport type (composition)`.

## T2 — Retry-After in `transport.do`  (`pkg/scanoss/transport.go`)
- [ ] `WithMaxRetries(n int) Option` (ignore `n <= 0`) → `c.transport.maxRetries`.
- [ ] `WithMaxRetryAfter(d) Option` (ignore `d <= 0`) → `c.transport.maxRetryAfter`;
  `const DefaultMaxRetryAfter = 5 * time.Minute` (`<= 0` = no cap).
- [ ] `retryAfter(resp) (time.Duration, bool)` — 429/503 + parseable `Retry-After`
  (integer seconds or `http.ParseTime` → `time.Until` ≥ 0), clamped via `clampDelay`.
- [ ] `sleepCtx(ctx, d) error` — interruptible wait.
- [ ] Wrap `do` in a bounded loop (`t.maxRetries`); replay body via `req.GetBody()`
  (no `GetBody` → no retry); on 429/503 + `Retry-After` and attempts remain →
  `sleepCtx` then continue; else existing behavior (200/202 success, else error).
- *Done:* non-429/503 behavior unchanged.

## T3 — Tests  (`pkg/scanoss/retry_test.go`)
- [ ] httptest: `429 + Retry-After: 1` then `200` → a decoration call succeeds
  after one retry.
- [ ] httptest: `429 + Retry-After: 1` then `202` → `uploadChunk` / `Scan.WFP`
  succeeds; server verifies the **replayed body** matches on the retry.
- [ ] HTTP-date form parses and waits.
- [ ] `maxRetries` exhausted → error returned.
- [ ] ctx cancelled during the wait → `ctx.Err()`.
- *Done:* `go test ./pkg/scanoss/ -race` green.

## T4 — Verify & docs
- [ ] `go build ./... && go vet ./... && gofmt -l . && go test ./...` clean.
- [ ] Note Retry-After behavior in `README.md` / `CHANGELOG.md` (SDK retries
  rate-limited/unavailable requests automatically).

## Commit
- `docs: add retry-after SDD plan` — `specs/retry-after/*` (done; this update
  amends it).
- `refactor(scanoss): extract transport type (composition)` — T1, pure restructure.
- `feat(scanoss): honor Retry-After (429/503) in the transport` — T2 + tests + docs.
