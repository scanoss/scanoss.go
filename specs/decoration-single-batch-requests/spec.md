# Feature Specification: single & batch requests to decoration services

**Feature branch:** `feat/decoration-single-batch-requests`
**Status:** Draft
**Tracking issue:** scanoss/scanoss#25

## Summary
Every decoration service in `pkg/scanoss` is currently a **batch** endpoint: a
caller always sends a list of PURLs that is chunked and POSTed concurrently.
The SCANOSS API also exposes a **single-component** lookup (an HTTP `GET`) for
each decoration. This feature adds first-class single-component support to the
SDK, selected explicitly by method naming:

- **plural method = batch** (existing): `Vulnerabilities(ctx, purls []string)` → `POST`
- **singular method = single** (new): `Vulnerability(ctx, purl string)` → `GET`

The mechanism is driven by a per-`Service` **request builder** (a function that
shapes the HTTP request), so a service file keeps owning its own endpoint(s) —
consistent with the existing source-based layout — and a future request shape is a
new builder rather than an edit to a central dispatch.

### Services in scope (endpoints from the v3 OpenAPI spec)
Single and batch **share one v3 path**: single = `GET` (`?purl=&requirement=`);
batch = `POST` (`{"components":[...]}`). They are kept as separate `Service` vars
that repeat the same path string.

| Decoration | Methods (single / batch) | v3 path |
|---|---|---|
| Vulnerabilities | `Component` / `Components` | `/v3/vulnerabilities/vulnerabilities` |
| Vulnerability CPEs | `Cpe` / `Cpes` | `/v3/vulnerabilities/cpes` |
| Cryptography algorithms | `Algorithm` / `Algorithms` | `/v3/cryptography/algorithms` |
| Cryptography algorithms in range | `AlgorithmInRange` / `AlgorithmsInRange` | `/v3/cryptography/algorithms/range` |
| Cryptography versions in range | `VersionInRange` / `VersionsInRange` | `/v3/cryptography/algorithms/versions/range` |
| Cryptography hints | `Hint` / `Hints` | `/v3/cryptography/hints` |
| Cryptography hints in range | `HintInRange` / `HintsInRange` | `/v3/cryptography/hints/range` |
| Geoprovenance origin | `Origin` / `Origins` | `/v3/geoprovenance/origin` |
| Geoprovenance countries | `Country` / `Countries` | `/v3/geoprovenance/countries` |
| Components status | `StatusOne` / `Status` | `/v3/components/status` |

**POST-only** decorations (batch body only, no single `GET`):

| Decoration | Method | v3 path |
|---|---|---|
| License attribution | `Licenses.Attribution` | `/v3/license/attribution` |
| License evidence | `Licenses.Evidence` | `/v3/license/evidence` |
| Copyright evidence | `Copyright.Evidence` | `/v3/copyright/evidence` |
| Copyright holders | `Copyright.Holders` | `/v3/copyright/holders` |

**Query-only** `GET` lookups on the Components service (keyed by query params, not a
component body): `Components.Search` (`/v3/components/search`) and
`Components.Versions` (`/v3/components/versions`).

## User Scenarios & Testing

### Primary user story
As a developer using the `pkg/scanoss` SDK, when I only need to decorate one
component I want a direct single-component call (`client.Vulnerability(ctx, purl)`)
that returns the same `Result` type as the batch call — instead of wrapping a
single PURL in a slice and paying for the batch request shape.

### Acceptance scenarios
1. **Given** a single PURL, **when** I call a singular method
   (e.g. `Vulnerability`), **then** the SDK issues one `GET` to that decoration's
   single-component endpoint with the PURL as a query parameter and returns a
   `*Result`.
2. **Given** a list of PURLs, **when** I call a plural method
   (e.g. `Vulnerabilities`), **then** the SDK behaves exactly as today: a chunked,
   concurrent batch `POST` whose responses are merged.
3. **Given** a `Result` from either path, **when** I call `Merged()`,
   `Unmarshal(v)`, or `String()`, **then** the API is identical regardless of
   single vs batch origin.
4. **Given** I am an SDK consumer, **when** I look at the public API, **then** I see
   only the per-service methods and the pipeline — the low-level fan-out engine is
   not callable from outside the package.
5. **Given** a PURL with a version requirement, **when** I issue a single `GET`,
   **then** the requirement is sent as a query parameter.

### Edge cases
- Empty PURL passed to a singular method → error (nothing to query).
- A single (`GET`) service handed more than one component → error (single endpoints
  take exactly one component).
- Non-200 from a single `GET` → same error formatting as the batch path.
- Caller cancels mid-request → the `GET` aborts via `ctx` like the `POST` path.

## Requirements

### Functional
- **FR-001** A `Service` MUST be a pure endpoint descriptor (`name`, `endpoint`) with
  all fields unexported and no request logic. The same shape serves component and
  non-component endpoints.
- **FR-002** The batch/single choice MUST be expressed by *which engine method* runs
  (`decorate` vs `decorateOne`), selected by the service method called — not by a
  shape flag, builder, or central switch.
- **FR-003** Each decoration MUST be a **grouped service**: a handle on the Client
  (`client.Vulnerabilities`, …) of a **per-service interface** type, implemented by an
  unexported service struct. The compiler MUST enforce the contract (`var _ XAPI =
  xService{}`).
- **FR-003a** The low-level engine MUST NOT be part of the public API. SDK consumers
  (including the CLI) MUST reach the API only through the service handles and
  `DecorationPipeline`.
- **FR-004** Every decoration whose v3 path supports a single `GET` MUST expose a
  single-component method (takes a `Component`) alongside its batch method (`POST`,
  takes `[]Component` so version requirements survive). `Components(purls ...string)`
  bridges the PURL-only batch case. **POST-only** decorations (license attribution/
  evidence, copyright evidence/holders) expose batch methods only.
- **FR-005** A single `GET` MUST send the component's PURL (and optional version
  requirement) as query parameters to that decoration's single-component endpoint.
- **FR-006** Both paths MUST return the same `*Result` type so downstream consumers
  (`Merged`, `Unmarshal`, `String`) are unchanged.
- **FR-007** A single (`GET`) service MUST accept exactly one component; more than
  one is an error.
- **FR-008** The single `GET` MUST honor caller cancellation and reuse the same
  auth headers and non-200 error handling as the batch `POST` (one shared transport).

### Non-functional
- **NFR-001** Both paths MUST share one transport choke point (auth, execute, status
  check) rather than duplicating it.
- **NFR-002** `DecorationPipeline`'s public behaviour MUST be unchanged (it stays
  batch-only and keeps taking `Service` values).

## Out of scope
- Fanning a singular method out over multiple components (single = exactly one).
- A CLI flag/command for single-component lookups.
- Adding single `GET` support to `DecorationPipeline`.

## Key entities
- **Service handles** — `client.Vulnerabilities`/`Licenses`/`Cryptography`/
  `Geoprovenance`/`Copyright`/`Components`, each of an exported per-service interface
  (`VulnerabilityAPI`, `CopyrightAPI`, `ComponentsAPI`, …) implemented by an
  unexported service struct holding the `*Client`. Wired in `New`. The interface is
  the service's contract (consumers can mock one service).
- **ComponentSearch** — the filter struct for `Components.Search`
  (`Search`/`Vendor`/`Component`/`PurlType`/`Limit`/`Offset`).
- **Service** — pure endpoint descriptor `{name, endpoint}`, fields unexported. The
  type and its vars are exported (the pipeline + the service methods use them).
- **decorate** (unexported) — the batch engine: chunk → concurrent → merge, POSTing
  `{"components":[…]}` per chunk. Not part of the public API.
- **decorateOne** (unexported) — the single path: one `GET ?purl=&requirement=` built
  from a `Component`, wrapped in a `*Result`. No chunking/pool.
- **do** (unexported) — the single transport choke point (ctx + auth + execute +
  status check) returning `[]byte`, reused by `decorate`, `decorateOne`, and every
  future non-component thin method.
- **Component** — a PURL plus an optional version requirement (unchanged).
- **Result** — the merged response wrapper, shared by both paths (unchanged). A
  single `GET` yields a single-`component` object; a batch `POST` yields a
  `components` array. The SDK does not reshape one into the other.
