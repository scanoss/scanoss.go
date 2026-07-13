# Implementation Plan: single & batch requests to decoration services

**Spec:** [`spec.md`](./spec.md) · **Tasks:** [`tasks.md`](./tasks.md)
**Tracking issue:** scanoss/scanoss#25

## Technical context
- **Language/module:** Go, `github.com/scanoss/scanoss.go`.
- **Package:** `pkg/scanoss` (SDK) + `cmd` (the CLI, a consumer of the SDK).
- **Endpoint source of truth:** the SCANOSS Services API **v3** OpenAPI spec. Each
  decoration shares **one path** for both shapes: a single lookup is a `GET`
  (`purl`/`requirement` as query params) and a batch is a `POST`
  (`{"components":[...]}` body) on that same path. Single and batch are kept as
  **separate `Service` vars** that repeat the same v3 path string (no shared const
  for now).
- **Reused building blocks:** the chunk + bounded-worker + merge engine (today
  `Query`, becomes the unexported `decorate`), `Result`/`Result.Merged()`,
  `Components(purls ...string)`, `componentsRequest`.
- **No new third-party dependencies** (`net/url` for query params).

## Design goals (validated)
1. Support **single** (GET) and **batch** (POST) component requests, exposed as
   **grouped per-service APIs** on the Client (`client.Vulnerabilities.Components` /
   `.Component`), each backed by a **per-service interface** the compiler enforces.
2. Keep the request engine **internal** — SDK consumers (and the CLI) reach the API
   only through the per-service handles and the pipeline.
3. Keep it **simple**: no builder/`requestFunc`/shape machinery. `Service` is pure
   data; the batch/single choice is just *which engine method* the service method
   calls (`decorate` vs `decorateOne`).

## Design — two engine methods + one transport
Single doesn't use the fan-out (it's one request), so batch and single are genuinely
distinct methods sharing only the transport `do`.

### Component 1 — `Service` is a pure descriptor (`pkg/scanoss/service.go`)
```go
type Service struct {
    name     string // stable id, used for logging/progress tagging
    endpoint string // REST path, relative to the API base URL
}
```
Fields unexported; the type and its vars are exported (the pipeline takes them). No
`verb`/`shape`/`build`. The same shape serves component and non-component endpoints.

### Component 2 — engine + transport (`pkg/scanoss/decorate.go`, renamed from query.go), all unexported
```go
// decorate: batch — chunk + bounded worker pool + merge; POST {"components":[...]} per chunk.
func (c *Client) decorate(ctx context.Context, svc Service, comps []Component) (*Result, error)

// decorateOne: single — one GET ?purl=&requirement= built from comp, wrapped in *Result.
func (c *Client) decorateOne(ctx context.Context, svc Service, comp Component) (*Result, error)

// do: the single transport choke point — ctx + auth + execute + status check.
func (c *Client) do(ctx context.Context, req *http.Request) ([]byte, error)
```
- `decorate` is today's `Query` engine, renamed and unexported; its worker builds the
  POST body inline (a small `postComponents` helper) and calls `do`.
- `decorateOne` builds the GET URL from `comp.Purl`/`comp.Requirement`, calls `do`,
  and wraps the body: `&Result{responses: []json.RawMessage{body}}`. No chunking/pool.
- `do` returns raw `[]byte` (shape-agnostic transport; the JSON assumption lives in
  the callers). A future binary endpoint reuses it unchanged.
- Remove the leftover `fmt.Println("endpoint", endpoint)` debug line.

Taking a `Component` (not a `purl string`) in `decorateOne` keeps it symmetric with
batch and supports the single endpoint's `requirement` query param.

### Component 3 — grouped per-service APIs (handles on Client)
Each decoration is a small service type holding the `*Client`, implementing a
per-service interface (`var _ XAPI = xService{}` fails to compile if a method is
missing). Endpoints stay as `Service` vars in the file; methods call the engine.
```go
// vulnerabilities.go — single + batch share the v3 path (string repeated)
var (
    ServiceVulnerabilities = Service{name: "vulnerabilities", endpoint: "/v3/vulnerabilities/vulnerabilities"}
    ServiceVulnerability   = Service{name: "vulnerability",  endpoint: "/v3/vulnerabilities/vulnerabilities"}
    // … cpes (both → /v3/vulnerabilities/cpes) …
)
type VulnerabilityAPI interface {
    Components(ctx context.Context, comps []Component) (*Result, error)
    Component(ctx context.Context, comp Component) (*Result, error)
    Cpes(ctx context.Context, comps []Component) (*Result, error)
    Cpe(ctx context.Context, comp Component) (*Result, error)
}
type vulnerabilityService struct{ c *Client }
var _ VulnerabilityAPI = vulnerabilityService{}
func (s vulnerabilityService) Components(ctx context.Context, comps []Component) (*Result, error) {
    return s.c.decorate(ctx, ServiceVulnerabilities, comps)     // endpoint ↔ method
}
func (s vulnerabilityService) Component(ctx context.Context, comp Component) (*Result, error) {
    return s.c.decorateOne(ctx, ServiceVulnerability, comp)
}
```
`client.go`: a field per service (`Vulnerabilities VulnerabilityAPI`, `Licenses`,
`Cryptography`, `Geoprovenance`, `Copyright`, `Components`), wired in `New`
(`c.Vulnerabilities = vulnerabilityService{c}`). Usage:
`client.Vulnerabilities.Component(ctx, comp)` / `.Components(ctx, comps)`.

Per-service surfaces: **Vulnerabilities** `Components/Component/Cpes/Cpe`;
**Licenses** (v3) `Attribution/Evidence` (POST-only, take `[]Component`);
**Copyright** (v3, new) `Evidence/Holders` (POST-only); **Cryptography**
`Algorithms/Algorithm`, `AlgorithmsInRange/AlgorithmInRange`,
`VersionsInRange/VersionInRange`, `Hints/Hint`, `HintsInRange/HintInRange`;
**Geoprovenance** `Origins/Origin`, `Countries/Country`; **Components** (v3, new)
`Search(ComponentSearch)`, `Versions(purl, limit)`, `Status`/`StatusOne`. Returns
are `*Result` (typed responses are a later, additive step).

### Component 4 — pipeline + CLI
- `DecorationPipeline.Run` calls `decorate(ctx, svc, components)` (batch-only,
  unchanged public API).
- CLI: `runPurlService(cmd, call decorateFunc)` takes a **closure over the service
  method** (`func(c, ctx, comps){ return c.Vulnerabilities.Components(ctx, comps) }`)
  and calls it; it never touches the engine. crypto/geo pick the closure by flag.
  `decorateFunc` = `func(*scanoss.Client, context.Context, []scanoss.Component) (*scanoss.Result, error)`.

### Full service map (v3 OpenAPI)
Single and batch **share one path**: single = `GET` (`?purl=&requirement=`), batch =
`POST` (`{"components":[...]}`). Kept as separate `Service` vars repeating the path.

| Decoration | Method(s) | v3 path (single `GET` + batch `POST`) |
|---|---|---|
| Vulnerabilities | `Component`/`Components` | `/v3/vulnerabilities/vulnerabilities` |
| Vulnerability CPEs | `Cpe`/`Cpes` | `/v3/vulnerabilities/cpes` |
| Cryptography algorithms | `Algorithm`/`Algorithms` | `/v3/cryptography/algorithms` |
| Cryptography algorithms in range | `AlgorithmInRange`/`AlgorithmsInRange` | `/v3/cryptography/algorithms/range` |
| Cryptography versions in range | `VersionInRange`/`VersionsInRange` | `/v3/cryptography/algorithms/versions/range` |
| Cryptography hints | `Hint`/`Hints` | `/v3/cryptography/hints` |
| Cryptography hints in range | `HintInRange`/`HintsInRange` | `/v3/cryptography/hints/range` |
| Geoprovenance origin | `Origin`/`Origins` | `/v3/geoprovenance/origin` |
| Geoprovenance countries | `Country`/`Countries` | `/v3/geoprovenance/countries` |
| Components status | `StatusOne`/`Status` | `/v3/components/status` |

**POST-only** decorations (one path, batch body only — no single `GET`):

| Decoration | Method | v3 path (`POST`) |
|---|---|---|
| License attribution | `Licenses.Attribution` | `/v3/license/attribution` |
| License evidence | `Licenses.Evidence` | `/v3/license/evidence` |
| Copyright evidence | `Copyright.Evidence` | `/v3/copyright/evidence` |
| Copyright holders | `Copyright.Holders` | `/v3/copyright/holders` |

### Non-component (query-only) endpoints
The v3 **Components** service adds two `GET` lookups keyed by query params rather than
a component body, implemented as thin methods over `getResult`:
- `Components.Search(ctx, ComponentSearch)` → `GET /v3/components/search`
  (`search`/`vendor`/`component`/`purl_type`/`limit`/`offset`).
- `Components.Versions(ctx, purl, limit)` → `GET /v3/components/versions`
  (`purl`/`limit`).

Out of scope for now: the v3 `dependencies`, `file-contents`/`metadata-contents`/
`notice-contents`, cryptography `reachability`, and ruleset download (binary, a future
thin `do` method since `do` returns `[]byte`).

## Public API surface
EXPORTED: per-service handles on the Client (`Vulnerabilities VulnerabilityAPI`,
`Licenses LicenseAPI`, `Cryptography CryptographyAPI`, `Geoprovenance GeoprovenanceAPI`,
`Copyright CopyrightAPI`, `Components ComponentsAPI`) and their interfaces;
`ComponentSearch`; `DecorationPipeline` (+ `Service` vars); `Client`/`New`/options;
`Component`/`Components`; `Result`; `Progress`. **Unexported:** `decorate`,
`decorateOne`, `getResult`, `do`, the `Service` fields, and the service impl structs.
No public low-level entry point.

## Response shapes — how single results are used
- **Batch** → `{"components": [ … ], "status": {…}}`; `Merged()` concatenates the
  `components` arrays across chunks.
- **Single** → `{"component": { … }, "status": {…}}` (object, not array); one
  response, so `Merged()` returns it verbatim.
- Both come back as the same `*Result`; the SDK does **not** reshape single↔batch.

## File changes
| File | Change |
|---|---|
| `pkg/scanoss/client.go` | add service-handle fields (`Vulnerabilities`/`Licenses`/`Cryptography`/`Geoprovenance`/`Copyright`/`Components`); wire in `New` |
| `pkg/scanoss/service.go` | `Service{name, endpoint}` (pure descriptor) |
| `pkg/scanoss/decorate.go` | renamed from `query.go`; `Query`→`decorate`; `decorateOne` + `getResult` + `do` (all unexported); drop debug print |
| `pkg/scanoss/vulnerabilities.go` | v3 paths; `VulnerabilityAPI` + `vulnerabilityService` + `var _` + `Components/Component/Cpes/Cpe` (return `*Result`) |
| `pkg/scanoss/licenses.go` | v3 rework: `LicenseAPI` + `licenseService` + `Attribution/Evidence` (POST-only) |
| `pkg/scanoss/copyright.go` | **new**: `CopyrightAPI` + `copyrightService` + `Evidence/Holders` (POST-only) |
| `pkg/scanoss/components.go` | **new**: `ComponentsAPI` + `componentsService` + `ComponentSearch` + `Search/Versions/Status/StatusOne` |
| `pkg/scanoss/cryptography.go` | v3 paths; `CryptographyAPI` + `cryptographyService` + 10 methods (algorithms/hints × exact/range + versions/range) |
| `pkg/scanoss/geoprovenance.go` | v3 paths; `GeoprovenanceAPI` + `geoprovenanceService` + `Origins/Origin/Countries/Country` |
| `pkg/scanoss/decoration_pipeline.go` | unchanged (`Run` calls `decorate(ctx, svc, components)`) |
| `cmd/{vulnerabilities,licenses,cryptography,geoprovenance}.go` | pass closures (`c.Vulnerabilities.Components`, …); `licenses` → `Licenses.Attribution`; crypto/geo by flag |
| tests/examples | `client.X(...)` → `client.Svc.Method(...)`; v3 paths |

## Alternatives considered (rejected)
- **Builder func / `requestFunc` (passed by method or stored on `Service`)** — adds a
  shape abstraction; single is a different *operation* (one request, no fan-out), so a
  separate `decorateOne` is simpler than parameterising one engine.
- **Shape enum + switch in `decorate`** — a central dispatch for what is just "call
  the other method".
- **Public `Decorate` + exported builders** — leaks the engine and enables
  endpoint↔shape mismatch.
- **Auto-select by component count** — a 1-element batch is valid; the choice is
  explicit via plural/singular method names.

## Testing strategy
**Unit tests** (`httptest.Server`, `-race`):
- `decorateOne` issues a `GET` with the right `purl`/`requirement` query params and
  returns a `*Result` whose `Merged()` equals the body.
- `decorate` issues chunked `POST`s with `{"components":[…]}` and merges.
- A singular method (`Vulnerability`) hits the singular endpoint; a plural method
  batches+merges; `DecorationPipeline` unchanged.

**Manual smoke** (API key): `client.Vulnerability(ctx, scanoss.Component{Purl:"pkg:..."})`
→ one GET; `client.Vulnerabilities(ctx, scanoss.Components(purls...))` → batch POST;
CLI output unchanged.

## Engineering conventions
- **Conventional Commits**; **atomic commits** (review before each); **no
  AI/assistant references**; **short** imperative subjects. Every code task ships
  with tests.

## Risks & rollout
- **Breaking (pre-1.0, in-repo):** public `Query` removed (engine private); batch
  methods change `[]string` → `[]Component`. Update all in-repo call sites (cmd,
  pipeline, tests, examples, README) in the same change. PURL-only callers wrap with
  `Components(...)`.
- **Behaviour unchanged:** same endpoints, batch POST, flags, CLI output.
- Confirm against papi that the `*`-marked endpoints are live before exposing them.
