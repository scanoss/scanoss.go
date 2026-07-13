# Tasks: single & batch requests to decoration services

**Spec:** [`spec.md`](./spec.md) · **Plan:** [`plan.md`](./plan.md)
**Tracking issue:** scanoss/scanoss#25

`[P]` = can run in parallel (different files, no ordering dependency).
Paths are relative to the repo root.

## Conventions
- **Conventional Commits** (`feat:`, `fix:`, `test:`, `docs:`, `refactor:`).
- **Atomic commits** — one logical change per commit; **review before each commit**.
- **No AI/assistant references** in commit messages (no co-author trailers).
- **Short** imperative commit subjects (≤ ~50 chars).
- Every code task ships with **unit tests**.

## Phase 1 — Internal engine (refactor commit)
- [ ] **T001** `pkg/scanoss/query.go` → `decorate.go`: `Query` → unexported
      `decorate(ctx, svc, []Component)`; add `decorateOne(ctx, svc, Component)` and
      `getResult(ctx, endpoint, url.Values)`; factor `do(ctx, *http.Request) ([]byte, error)`
      (ctx + auth + execute + status); drop the debug print. `Service` stays `{name, endpoint}`.

## Phase 2 — Grouped per-service APIs (feat commit)
- [ ] **T002** `pkg/scanoss/client.go`: add service-handle fields
      (`Vulnerabilities VulnerabilityAPI`, `Licenses`, `Cryptography`, `Geoprovenance`,
      `Copyright`, `Components`); wire them in `New` (`c.Vulnerabilities = vulnerabilityService{c}` …).
- [ ] **T003 [P]** `vulnerabilities.go`: v3 paths; `VulnerabilityAPI` +
      `vulnerabilityService` + `var _` + `Components/Component/Cpes/Cpe` (return `*Result`).
- [ ] **T005 [P]** `cryptography.go`: v3 paths; `CryptographyAPI` + `cryptographyService`
      + the 10 methods (algorithms/hints × exact/range + versions/range).
- [ ] **T006 [P]** `geoprovenance.go`: v3 paths; `GeoprovenanceAPI` +
      `geoprovenanceService` + `Origins/Origin/Countries/Country`.
- [ ] **T007** `cmd/{vulnerabilities,licenses,cryptography,geoprovenance}.go`: pass
      closures (`c.Vulnerabilities.Components`, …); `licenses` → `Licenses.Attribution`;
      crypto/geo pick the closure by flag.
- [ ] **T008** Update tests/examples: `client.X(...)` → `client.Svc.Method(...)`; v3 paths.

## Phase 2b — v3 services (feat commits)
- [ ] **T010 [P]** `licenses.go`: v3 rework — `LicenseAPI` + `licenseService` +
      `Attribution/Evidence` (POST-only); drop v2 `Components/Component/Details/Obligations`.
- [ ] **T011 [P]** `copyright.go` (new): `CopyrightAPI` + `copyrightService` +
      `Evidence/Holders` (POST-only).
- [ ] **T012 [P]** `components.go` (new): `ComponentsAPI` + `componentsService` +
      `ComponentSearch` + `Search/Versions/Status/StatusOne`.

## Phase 3 — Tests
- [ ] **T009** `decorate_single_test.go`: a single method (`Vulnerabilities.Component`)
      hits the shared v3 path with `purl`/`requirement`; `Components.Versions(purl, limit)`
      → `GET …?purl=&limit=`. Plural + `DecorationPipeline` unchanged.

## Final verification
- [ ] `go build ./...`; `go vet ./pkg/scanoss/ ./cmd/`; `gofmt -l pkg/scanoss/ cmd/`.
- [ ] `go test -race -count=1 ./pkg/scanoss/`; `go test ./...`.
- [ ] `go doc ./pkg/scanoss` shows the service handles + interfaces; engine private.
- [ ] Confirm the v3 endpoints (esp. copyright, components, license attribution/evidence)
      are live before release.

## Out of scope (future)
- Typed response structs (additive, on top of this shape).
- CLI command/flag for single lookups, and CLI commands for the new copyright/
  components services (the CLI stays batch over the existing commands).
- `decorateOne` inside `DecorationPipeline` (stays batch-only).
- v3 `dependencies`, `file-contents`/`metadata-contents`/`notice-contents`,
  cryptography `reachability`, ruleset download (binary, future thin `do` method).