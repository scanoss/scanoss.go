# Implementation Plan: SCANOSS decoration pipeline

**Spec:** [`spec.md`](./spec.md) · **Tasks:** [`tasks.md`](./tasks.md)
**Tracking issue:** internal

## Technical context
- **Language/module:** Go 1.22, `github.com/scanoss/scanoss.go`.
- **Package:** `pkg/scanoss` (existing SDK).
- **Reused building blocks:**
  - `Client.Query(ctx, Service, []Component)` — the chunk + bounded-worker engine
    (`pkg/scanoss/query.go`).
  - `Result` + `Result.Merged()` — per-service merged JSON (`query.go`).
  - `Service{name, endpoint}` values declared per service file.
- **No new third-party dependencies.**

## Design overview
A `DecorationPipeline` wraps a `*Client` and an ordered set of `Service` values, runs each
selected service concurrently via the existing `Query`, and assembles a result
keyed by service. Progress is reported through a redesigned, service-tagged
progress event so a caller (or a future multi-bar renderer) can show per-service
progress.

### Component 1 — Progress event redesign (prerequisite)
The current `WithProgress(func(done, total int))` carries no service identity and
counts chunks; it cannot support a pipeline. Replace with a structured event:

```go
// pkg/scanoss/progress.go (new)
type Progress struct {
    Service string // Service.name, e.g. "vulnerabilities"
    Done    int    // units completed
    Total   int    // total units
    Unit    string // "purls"
}
type ProgressFunc func(Progress)
```
- `client.go`: `onProgress ProgressFunc`; `WithProgress(fn ProgressFunc)`.
- `query.go`: count purls in the collection loop — `done += len(chunks[r.idx])`,
  `total = len(components)` — emit `Progress{Service: svc.name, Done, Total, Unit: "purls"}`.

### Component 1b — Public purls→Components helper (`pkg/scanoss/component.go`)
The pipeline (and `Query`) input is `[]Component`. Provide an exported helper so
callers holding only PURL strings can build it easily; back the existing
unexported `toComponents` with it:
```go
// Components builds a []Component from PURL strings, with empty requirements.
// Accepts a slice via Components(purls...). For per-component requirements,
// construct []Component{{Purl, Requirement}} directly.
func Components(purls ...string) []Component
```
Usage: `scanoss.Components("pkg:npm/lodash", "pkg:pypi/requests")` or
`scanoss.Components(purls...)`.

### Component 2 — DecorationPipeline (new `pkg/scanoss/decoration_pipeline.go`)
```go
type DecorationPipeline struct {
    client   *Client
    services []Service // ordered, deduped by Service.name
}
func (c *Client) DecorationPipeline(services ...Service) *DecorationPipeline
func (p *DecorationPipeline) Add(services ...Service) *DecorationPipeline    // append if absent
func (p *DecorationPipeline) Remove(services ...Service) *DecorationPipeline // drop by name
func (p *DecorationPipeline) Services() []Service
func (p *DecorationPipeline) OnProgress(fn func(PipelineProgress)) *DecorationPipeline // per-service progress snapshot (see Component 4)
func (p *DecorationPipeline) Run(ctx context.Context, components []Component) (*PipelineResult, error)
```
`Run` launches one goroutine per service (each calling `p.client.Query`),
collected with a `sync.WaitGroup` + channel. **`Run` is a barrier**: it calls
`wg.Wait()` so it returns only after *every* service goroutine has finished
(success or failure) — no service is left running. Wall-clock ≈ the slowest
service, not the sum. Concurrent `Query` on one `Client` is safe (Client is
read-only after `New`; `http.Client` is concurrency-safe). Cancellation flows
through `ctx` into each `Query` (already handled); on cancel, services still
return promptly and `Run` waits for them before returning.

### Component 3 — Output
```go
type PipelineResult struct {
    Services map[string]*Result // keyed by Service.name
    Errors   map[string]error   // services that failed entirely
}
func (pr *PipelineResult) MarshalJSON() ([]byte, error) // {"<service>": <merged>, …}
func (pr *PipelineResult) String() string               // pretty JSON
```
`MarshalJSON` calls each `Result.Merged()`, so each key holds the full merged
object `{components, status}`. `Run` returns an error only if every service
failed; otherwise it returns the result with `Errors` populated.

### Component 4 — DecorationPipeline progress (per-service snapshot for a UI)
The pipeline aggregates the per-service `Progress` events into a snapshot keyed
by service, so a UI can render each service's progress individually without
managing any concurrency itself:
```go
// PipelineProgress is a snapshot of every service's current progress.
type PipelineProgress struct {
    Services map[string]Progress // key = service name; value = its Done/Total/Unit
}
func (p *DecorationPipeline) OnProgress(fn func(PipelineProgress)) *DecorationPipeline
```
- The pipeline owns a thread-safe `map[string]Progress`. Internally it receives
  each underlying service's `Progress` events (via the client's progress hook),
  updates that service's entry, and invokes the consumer's `OnProgress` handler
  **serially** with a copy of the full snapshot. The consumer just stores/renders
  it — **no mutex, no channels** on the consumer side.
- Optional pull alternative: `func (p *DecorationPipeline) Snapshot() PipelineProgress`
  returns a thread-safe copy for UIs that render on a tick.
- The lower-level client `WithProgress(ProgressFunc)` still exists for
  single-service calls; the pipeline's `OnProgress` is the aggregated view.

## File changes
| File | Change |
|---|---|
| `pkg/scanoss/progress.go` | new — `Progress`, `ProgressFunc` |
| `pkg/scanoss/component.go` | export `Components(purls ...string) []Component`; route `toComponents` through it |
| `pkg/scanoss/client.go` | `onProgress`/`WithProgress` signature |
| `pkg/scanoss/query.go` | emit purl-counted, service-tagged `Progress` |
| `pkg/scanoss/decoration_pipeline.go` | new — `DecorationPipeline`, `Run`, `PipelineResult`, `PipelineProgress`, `OnProgress`/`Snapshot` |
| `cmd/purlcommon.go` | adapt `WithProgress` closure (mechanical) |
| `pkg/scanoss/decoration_pipeline_test.go` | new — pipeline tests |
| `pkg/scanoss/example_test.go` | godoc example |

## Alternatives considered
- **DecorationPipeline-owned progress type/aggregator** — rejected; the tagged `Progress`
  already carries enough, rendering is the consumer's concern.
- **Sequential execution** — rejected; requirement is parallel and services are
  independent / I/O-bound.
- **Short family keys (`cryptography`)** — deferred; `Service.name` avoids
  collisions when two crypto variants run; alias can be added later.
- **`Run(ctx, []string)`** — rejected as primary; `[]Component` preserves
  per-component requirements and composes with `Query`.

## Testing strategy
**Unit tests** (per component, `httptest.Server`, `-race`):
- Progress events carry the right service and count purls.
- `Components` helper builds the expected `[]Component`.
- `Add`/`Remove` incl. dedupe by name.
- `MarshalJSON` shape; `Run` error semantics (partial / all-fail).

**End-to-end test** (`-race`): a stub SCANOSS server backs all decoration
endpoints; build input via `Components(...)`, configure a multi-service pipeline,
`Run`, and assert (a) the combined keyed output contains every service's merged
result, (b) services ran in parallel (max in-flight > 1), and (c) `OnProgress`
delivered per-service snapshots ending at `Done==Total` for each service.

## Engineering conventions
- **Conventional Commits** (`feat:`, `fix:`, `test:`, `docs:`, `refactor:`).
- **Atomic commits** — one logical change per commit (roughly one task/sub-task).
- **No AI/assistant references** in commit messages — no "generated by" notes,
  no co-author trailers.
- **Short** commit subjects (imperative, ≤ ~50 chars).
- Every task ships with **unit tests**; the feature is covered by **end-to-end
  tests** as above.

## Risks & rollout
- **Breaking change:** `WithProgress` signature. Impact is internal
  (`cmd/purlcommon.go`, tests/examples); SDK is pre-1.0. Update callers in the
  same change.
- Additive otherwise; no data migration.
