# Feature Specification: v3 declared-licenses service + pipeline switch

**Feature branch:** TBD (off `main`)
**Status:** Draft

## Summary
The latest OpenAPI v3 spec adds a new **License service** family under `/v3/licenses`
(the v3 License service), distinct from the existing `/v3/license/attribution`
and `/v3/license/evidence`:

| Endpoint | Shape | Response model (already generated) |
|---|---|---|
| `GET /v3/licenses` (`?purl=&requirement=`) | single component | `ComponentLicenseResponse` |
| `POST /v3/licenses` | batch (`{components:[…]}`) | `ComponentsLicenseResponse` |
| `GET /v3/licenses/details?id=` | by license id | `LicenseDetailsResponse` |
| `GET /v3/licenses/obligations?id=` | by license id | `ObligationsResponse` |

These return the **declared licenses** of a component (purl → list of SPDX
`LicenseInfo` + a combined `statement`), plus SPDX-registry / OSADL metadata for a
license id. The response types already exist in `pkg/scanoss/openapi` (generated from
the committed spec), so this feature is **wiring the service methods + switching the
pipeline**, not regenerating types.

Two deliverables:
1. **Expose the new service** on `client.Licenses` (declared licenses single/batch +
   details + obligations), returning the generated typed models.
2. **Switch the DecorationPipeline** to use the declared-licenses batch service
   (`/v3/licenses`) instead of attribution (`/v3/license/attribution`), since declared
   licenses are the natural per-component decoration; attribution (LICENSE/NOTICE file
   contents) stays available as its own method but is no longer the pipeline default.

## User Scenarios & Testing

### Primary user story
As an SDK user decorating components, `client.Licenses.Components(ctx, comps)` returns
each component's **declared licenses** (typed `*openapi.ComponentsLicenseResponse`), and
a `DecorationPipeline` that includes the license service yields declared licenses per
component — not attribution file blobs.

### Acceptance scenarios
1. **Given** components, **when** I call `client.Licenses.Components(ctx, comps)`,
   **then** the SDK POSTs `{components:[…]}` to `/v3/licenses` and returns
   `*openapi.ComponentsLicenseResponse`.
2. **Given** one component, **when** I call `client.Licenses.Component(ctx, comp)`,
   **then** the SDK GETs `/v3/licenses?purl=&requirement=` → `*openapi.ComponentLicenseResponse`.
3. **Given** a license id, **when** I call `Details(id)` / `Obligations(id)`, **then**
   the SDK GETs `/v3/licenses/details?id=` / `/v3/licenses/obligations?id=` →
   `*openapi.LicenseDetailsResponse` / `*openapi.ObligationsResponse`.
4. **Given** a `DecorationPipeline` including the license service, **when** `Run`
   executes, **then** the license entry hits `/v3/licenses` (batch POST) and is keyed
   `licenses` in the result (not `license.attribution`).
5. **Given** existing `Attribution` / `Evidence` methods, **then** they are unchanged
   and still hit `/v3/license/{attribution,evidence}`.

### Edge cases
- `Details("")` / `Obligations("")` → error (no license id).
- Empty component list to `Components` → same "no components" error as other batch calls.

## Requirements

### Functional
- **FR-001** Add `Service` vars for the new endpoints: `ServiceLicenses` (batch),
  `ServiceLicense` (single), `ServiceLicensesDetails`, `ServiceLicensesObligations` —
  single and batch repeat the same `/v3/licenses` path (the single/batch convention).
- **FR-002** Extend `LicenseAPI` with typed methods returning the generated models:
  declared licenses batch + single, `Details(id)`, `Obligations(id)`. The compiler
  enforces the interface (`var _ LicenseAPI = licenseService{}`).
- **FR-003** `Details`/`Obligations` are keyed by **license id** (query `?id=`), via the
  existing `getResult` GET path — not by components.
- **FR-004** The `DecorationPipeline` default license service MUST be the
  declared-licenses **batch** service (`ServiceLicenses`, `/v3/licenses`). Update the
  example and pipeline tests that currently pass `ServiceLicenseAttribution`.
- **FR-005** `Attribution` and `Evidence` remain available and unchanged.
- **FR-006** No spec regeneration: the response types already exist in
  `pkg/scanoss/openapi`. (`make generate` must remain a no-op against the committed spec.)

### Non-functional
- **NFR-001** Reuse the existing engine (`decorate`/`decorateOne`/`getResult`/`As[T]`);
  no engine changes.
- **NFR-002** All commits build, vet, gofmt-clean, and pass tests (incl. the live test
  gated on `SCANOSS_API_KEY`).

## Open decisions (resolve during review)
1. **Method naming for declared licenses.** Proposed `Components` / `Component`
   (consistent with the other services). Alternatives: `Declared` / `DeclaredOne`, or
   `Lookup` / `LookupOne`. `Components`/`Component` reads slightly oddly on
   `client.Licenses` but matches the rest of the SDK. **Recommend `Components`/`Component`.**
2. **CLI surface.** _Resolved:_ the **bare `scanoss licenses`** default becomes
   **declared licenses** (`Licenses.Components`), to mirror the pipeline switch.
   `attribution` and `evidence` remain available as explicit subcommands. So:
   `licenses` (= `declared`) | `attribution` | `evidence`. **Breaking (CLI):** the bare
   command previously returned attribution; it now returns declared licenses.
3. **Details/Obligations scope.** Include both now, or defer (they're license-id
   lookups, not component decorations, and don't participate in the pipeline)?
   **Recommend include** (small, completes the service).

## Out of scope
- Regenerating types (already present).
- Removing `Attribution`/`Evidence`.
- A production pipeline caller (none exists today; only examples/tests configure it).
