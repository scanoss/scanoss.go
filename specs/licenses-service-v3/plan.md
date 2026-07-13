# Implementation Plan: v3 declared-licenses service + pipeline switch

**Spec:** [`spec.md`](./spec.md) · **Tasks:** [`tasks.md`](./tasks.md)

## Technical context
- **Module:** Go, `github.com/scanoss/scanoss.go`. SDK in `pkg/scanoss`, CLI in `cmd`.
- **Types already generated** in `pkg/scanoss/openapi` from the committed spec:
  `ComponentLicenseResponse`, `ComponentsLicenseResponse`, `LicenseDetailsResponse`,
  `ObligationsResponse` (+ `ComponentLicenseInfo`, `LicenseInfo`, `LicenseDetails`,
  `OSADLInfo`, `LookupStatusResponse`).
- **Reused, unchanged:** the engine `decorate` (batch POST), `decorateOne`
  (single GET `?purl=&requirement=`), `getResult` (GET with query), `As[T]`.

## Design

### Service vars (`pkg/scanoss/licenses.go`)
```go
var (
    ServiceLicenseAttribution  = Service{name: "license.attribution",  endpoint: "/v3/license/attribution"} // unchanged
    ServiceLicenseEvidence     = Service{name: "license.evidence",     endpoint: "/v3/license/evidence"}     // unchanged
    ServiceLicenses            = Service{name: "licenses",             endpoint: "/v3/licenses"}             // NEW batch
    ServiceLicense             = Service{name: "license",              endpoint: "/v3/licenses"}             // NEW single
    ServiceLicensesDetails     = Service{name: "licenses.details",     endpoint: "/v3/licenses/details"}     // NEW (by id)
    ServiceLicensesObligations = Service{name: "licenses.obligations", endpoint: "/v3/licenses/obligations"} // NEW (by id)
)
```

### Typed methods (extend `LicenseAPI`)
```go
type LicenseAPI interface {
    Attribution(ctx, []Component) (*openapi.AttributionResponse, error)        // unchanged
    Evidence(ctx, []Component)    (*openapi.LicenseEvidenceResponse, error)    // unchanged
    Components(ctx, []Component)  (*openapi.ComponentsLicenseResponse, error)  // NEW: declared licenses, batch
    Component(ctx, Component)     (*openapi.ComponentLicenseResponse, error)   // NEW: declared licenses, single
    Details(ctx, id string)      (*openapi.LicenseDetailsResponse, error)      // NEW: GET ?id=
    Obligations(ctx, id string)  (*openapi.ObligationsResponse, error)         // NEW: GET ?id=
}
```
Implementations mirror the existing pattern: `decorate`→`As` (Components), `decorateOne`
→`As` (Component), and for Details/Obligations a guarded `getResult(endpoint, url.Values{"id":{id}})`→`As`
(error on empty id). `var _ LicenseAPI = licenseService{}` enforces the contract.

### Pipeline switch
The pipeline takes `Service` values and runs `decorate` (batch POST) per service,
keyed by `Service.name`. Switching the license decoration = pass `ServiceLicenses`
(name `licenses`, POST `/v3/licenses`) instead of `ServiceLicenseAttribution`. No
pipeline-engine change. Update the callers that wire it:
- `pkg/scanoss/example_test.go` (`ExampleClient_DecorationPipeline`).
- `pkg/scanoss/decoration_pipeline_test.go`, `_progress_test.go`, `_e2e_test.go` —
  swap `ServiceLicenseAttribution` → `ServiceLicenses`, and update keyed assertions
  (`"license.attribution"` → `"licenses"`) and any stub-path expectations
  (`/v3/license/attribution` → `/v3/licenses`).

### CLI (Open decision 2 — resolved)
`cmd/licenses.go`: the **bare `licenses`** default becomes declared licenses
(`Licenses.Components`); add a `declared` subcommand (same call) plus keep
`attribution` and `evidence` subcommands:
- bare `RunE` and `declared` → `callLicenseDeclared` → `*openapi.ComponentsLicenseResponse`
  via `newPurlServiceCmdTyped` / `runPurlServiceTyped`.
- `attribution` → `callLicenseAttribution` (existing); `evidence` → `callLicenseEvidence`.

**Breaking (CLI):** bare `scanoss licenses` previously returned attribution; it now
returns declared licenses. (Details/Obligations are id-keyed, not PURL-list; not exposed
via the PURL-list runner here.)

## Files to modify
| File | Change |
|---|---|
| `pkg/scanoss/licenses.go` | add 4 `Service` vars; extend `LicenseAPI` + impl (Components/Component/Details/Obligations) |
| `pkg/scanoss/licenses_test.go` (new or existing) | typed tests: batch→`/v3/licenses` POST, single→GET `?purl=`, details/obligations→GET `?id=` |
| `pkg/scanoss/example_test.go` | pipeline example uses `ServiceLicenses` |
| `pkg/scanoss/decoration_pipeline_test.go` / `_progress_test.go` / `_e2e_test.go` | `ServiceLicenseAttribution`→`ServiceLicenses`; keys `license.attribution`→`licenses`; stub paths |
| `cmd/licenses.go` | bare default → declared licenses; add `declared` subcommand; keep `attribution`/`evidence` |
| `CHANGELOG.md` | note the new declared-licenses methods + pipeline default switch |

## Commit plan (atomic, conventional)
1. `feat(scanoss): add v3 declared-licenses service methods` — service vars + LicenseAPI
   methods + SDK tests.
2. `feat(scanoss): default the decoration pipeline to declared licenses` — pipeline
   callers (example + tests) switch from attribution to `/v3/licenses`.
3. `feat(cmd): default licenses command to declared licenses` — bare default +
   `declared` subcommand; keep `attribution`/`evidence`.
4. `docs: note declared-licenses service in changelog`.

## Testing strategy
- **Unit (httptest):** assert each method hits the right path/verb and decodes into the
  generated type with fields populated (declared `licenses[]`, `statement`).
- **Pipeline:** the e2e/keyed tests now assert the `licenses` key and `/v3/licenses` path.
- **Live (gated on `SCANOSS_API_KEY`):** `client.Licenses.Components(ctx, Components("pkg:npm/lodash"))`
  decodes to a non-empty result; `Details("MIT")` returns SPDX metadata.
- `go build ./...`; `go vet ./...`; `gofmt -l`; `go test ./...`; `make generate` no-op.

## Engineering conventions
- **Conventional Commits**; **atomic commits** (review before each); **no AI/assistant
  references**; **short** imperative subjects. Every change ships with tests.

## Risks & rollout
- **Behaviour change (pipeline):** consumers reading the pipeline result's license entry
  get `ComponentsLicenseResponse` (declared licenses) under key `licenses` instead of
  attribution under `license.attribution`. Pre-1.0; documented in the CHANGELOG. No
  production caller exists, so blast radius is examples/tests + downstream SDK users.
