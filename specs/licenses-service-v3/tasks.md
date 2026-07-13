# Tasks: v3 declared-licenses service + pipeline switch

**Spec:** [`spec.md`](./spec.md) · **Plan:** [`plan.md`](./plan.md)

## Conventions
- **Conventional Commits**; **atomic commits** (review before each); **no AI refs**;
  **short** imperative subjects. Every code change ships with tests.

## Phase 1 — SDK: declared-licenses service
- [ ] **T001** `pkg/scanoss/licenses.go`: add `Service` vars `ServiceLicenses` (batch),
      `ServiceLicense` (single), `ServiceLicensesDetails`, `ServiceLicensesObligations`.
- [ ] **T002** Extend `LicenseAPI` + `licenseService` with typed methods:
      `Components` → `*openapi.ComponentsLicenseResponse` (decorate),
      `Component` → `*openapi.ComponentLicenseResponse` (decorateOne),
      `Details(id)` → `*openapi.LicenseDetailsResponse` (getResult `?id=`, error on empty),
      `Obligations(id)` → `*openapi.ObligationsResponse` (getResult `?id=`, error on empty).
      Keep `var _ LicenseAPI = licenseService{}`.
- [ ] **T003** SDK tests (httptest): batch → POST `/v3/licenses`; single → GET
      `/v3/licenses?purl=&requirement=`; `Details`/`Obligations` → GET `…?id=` (+ empty-id
      error); assert decode into the generated types.

## Phase 2 — Pipeline switch (attribution → declared licenses)
- [ ] **T010** `pkg/scanoss/example_test.go`: pipeline example uses `ServiceLicenses`.
- [ ] **T011** `decoration_pipeline_test.go` / `_progress_test.go` / `_e2e_test.go`:
      swap `ServiceLicenseAttribution` → `ServiceLicenses`; update keyed assertions
      (`"license.attribution"` → `"licenses"`) and stub path expectations
      (`/v3/license/attribution` → `/v3/licenses`).

## Phase 3 — CLI
- [ ] **T020** `cmd/licenses.go`: make the **bare `licenses`** default call
      `Licenses.Components` (declared licenses); add a `declared` subcommand (same call)
      via `newPurlServiceCmdTyped`/`runPurlServiceTyped`; keep `attribution` and
      `evidence` subcommands. **Breaking (CLI):** bare command now returns declared
      licenses instead of attribution.

## Phase 4 — Docs
- [ ] **T030** `CHANGELOG.md` (`[Unreleased]`): new declared-licenses methods
      (`Components`/`Component`/`Details`/`Obligations`); the pipeline default switch to
      `/v3/licenses`; and the **breaking CLI change** (bare `licenses` now returns
      declared licenses, not attribution).

## Final verification
- [ ] `make generate` is a no-op (types already match the committed spec).
- [ ] `go build ./...`; `go vet ./...`; `gofmt -l pkg/scanoss/ cmd/`.
- [ ] `go test ./...` (+ live with `SCANOSS_API_KEY`).
- [ ] `go doc ./pkg/scanoss LicenseAPI` shows the new typed methods.

## Open decisions (carry from spec)
1. Method naming: `Components`/`Component` (recommended) vs `Declared`/`DeclaredOne`.
2. CLI: _resolved_ — bare `licenses` default = declared licenses (aligned with the
   pipeline); `attribution`/`evidence` are subcommands.
3. Include `Details`/`Obligations` now (recommended) vs defer.
